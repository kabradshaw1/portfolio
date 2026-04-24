package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/guardrails"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/llm"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

// ErrMaxSteps is returned when the agent loop exceeds the configured step cap.
var ErrMaxSteps = apperror.Internal("MAX_STEPS_EXCEEDED", "agent: max steps exceeded")

// Turn is one invocation of the agent — a user id plus the full conversation so far.
type Turn struct {
	UserID   string
	Messages []llm.Message
}

// Agent runs the LLM tool-calling loop.
type Agent struct {
	llm      llm.Client
	registry tools.Registry
	rec      metrics.Recorder
	maxSteps int
	timeout  time.Duration
	model    string
}

// New constructs an Agent.
func New(client llm.Client, registry tools.Registry, rec metrics.Recorder, maxSteps int, timeout time.Duration) *Agent {
	if rec == nil {
		rec = metrics.NopRecorder{}
	}
	return &Agent{llm: client, registry: registry, rec: rec, maxSteps: maxSteps, timeout: timeout}
}

// WithModel sets the model name for Ollama metrics labeling.
func (a *Agent) WithModel(model string) *Agent {
	a.model = model
	return a
}

// Run executes the loop. The emit callback receives every event in order.
// Infrastructure failures (LLM unreachable, ctx cancelled, max steps) are returned as errors.
// Tool-level failures are fed back into the conversation as tool results and do not return an error.
func (a *Agent) Run(ctx context.Context, turn Turn, emit func(Event)) error {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()

	turnID := uuid.NewString()

	ctx, turnSpan := otel.Tracer("agent").Start(ctx, "agent.turn",
		trace.WithAttributes(
			attribute.String("agent.turn_id", turnID),
			attribute.String("agent.user_id", turn.UserID),
		),
	)
	defer turnSpan.End()
	startTime := time.Now()
	slog.Info("agent turn start",
		"turn_id", turnID,
		"user_id", turn.UserID,
		"message_count", len(turn.Messages),
		"model", a.model,
		"max_steps", a.maxSteps,
	)
	stepsCompleted := 0
	turn.Messages = guardrails.TruncateHistory(turn.Messages, guardrails.DefaultMaxHistory)
	messages := append([]llm.Message(nil), turn.Messages...)
	var toolsCalled []string

	for step := 0; step < a.maxSteps; step++ {
		llmCtx, llmSpan := otel.Tracer("agent").Start(ctx, "agent.llm_call",
			trace.WithAttributes(attribute.Int("agent.step", step)),
		)
		resp, err := a.llm.Chat(llmCtx, messages, a.registry.Schemas())
		llmSpan.End()
		if err == nil {
			operation := "chat"
			if step == a.maxSteps-1 {
				operation = "chat_final"
			}
			a.rec.RecordOllamaCall(a.model, operation, resp.RequestDuration, resp.PromptEvalCount, resp.EvalCount, resp.EvalDurationNs)
			slog.Info("llm call",
				"turn_id", turnID,
				"step", step,
				"prompt_tokens", resp.PromptEvalCount,
				"completion_tokens", resp.EvalCount,
				"duration_ms", resp.RequestDuration.Milliseconds(),
				"tool_call_count", len(resp.ToolCalls),
			)
		}
		if err != nil {
			emit(Event{Error: &ErrorEvent{Reason: err.Error()}})
			a.rec.RecordTurn("error", stepsCompleted, time.Since(startTime))
			slog.Info("agent turn",
				"turn_id", turnID,
				"user_id", turn.UserID,
				"steps", stepsCompleted,
				"tools_called", toolsCalled,
				"duration_ms", time.Since(startTime).Milliseconds(),
				"outcome", "error",
			)
			return fmt.Errorf("llm chat: %w", err)
		}

		if len(resp.ToolCalls) == 0 {
			outcome := "final"
			if guardrails.IsRefusal(resp.Content) {
				outcome = "refused"
			}
			a.rec.RecordTurn(outcome, stepsCompleted+1, time.Since(startTime))
			slog.Info("agent turn",
				"turn_id", turnID,
				"user_id", turn.UserID,
				"steps", stepsCompleted+1,
				"tools_called", toolsCalled,
				"duration_ms", time.Since(startTime).Milliseconds(),
				"outcome", outcome,
			)
			emit(Event{Final: &FinalEvent{Text: resp.Content}})
			return nil
		}

		stepsCompleted++

		// Record the assistant's tool-call message in history.
		messages = append(messages, llm.Message{
			Role:      llm.RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		for _, call := range resp.ToolCalls {
			emit(Event{ToolCall: &ToolCallEvent{Name: call.Name, Args: call.Args}})

			_, toolSpan := otel.Tracer("agent").Start(ctx, "agent.tool_call",
				trace.WithAttributes(attribute.String("tool.name", call.Name)),
			)

			tool, ok := a.registry.Get(call.Name)
			if !ok {
				errMsg := "unknown tool: " + call.Name
				emit(Event{ToolError: &ToolErrorEvent{Name: call.Name, Error: errMsg}})
				msg, _ := llm.ToolResultMessage(call.ID, call.Name, map[string]string{"error": errMsg})
				messages = append(messages, msg)
				a.rec.RecordTool(call.Name, "unknown", 0)
				slog.Warn("unknown tool requested",
					"turn_id", turnID,
					"step", step,
					"tool", call.Name,
				)
				toolSpan.End()
				continue
			}

			toolStart := time.Now()
			result, toolErr := safeCall(ctx, tool, call.Args, turn.UserID)
			outcome := "success"
			if toolErr != nil {
				outcome = "error"
			}
			a.rec.RecordTool(call.Name, outcome, time.Since(toolStart))
			toolsCalled = append(toolsCalled, call.Name)
			argsPreview := string(call.Args)
			if len(argsPreview) > 200 {
				argsPreview = argsPreview[:200] + "..."
			}
			slog.Info("tool call",
				"turn_id", turnID,
				"step", step,
				"tool", call.Name,
				"args_preview", argsPreview,
				"duration_ms", time.Since(toolStart).Milliseconds(),
				"success", toolErr == nil,
			)

			if toolErr != nil {
				emit(Event{ToolError: &ToolErrorEvent{Name: call.Name, Error: toolErr.Error()}})
				msg, _ := llm.ToolResultMessage(call.ID, call.Name, map[string]string{"error": toolErr.Error()})
				messages = append(messages, msg)
				toolSpan.End()
				continue
			}

			emit(Event{ToolResult: &ToolResultEvent{Name: call.Name, Display: result.Display}})
			msg, err := llm.ToolResultMessage(call.ID, call.Name, result.Content)
			if err != nil {
				errMsg := "tool result not serializable: " + err.Error()
				emit(Event{ToolError: &ToolErrorEvent{Name: call.Name, Error: errMsg}})
				msg2, _ := llm.ToolResultMessage(call.ID, call.Name, map[string]string{"error": errMsg})
				messages = append(messages, msg2)
				toolSpan.End()
				continue
			}
			messages = append(messages, msg)
			toolSpan.End()
		}
	}

	emit(Event{Error: &ErrorEvent{Reason: ErrMaxSteps.Error()}})
	a.rec.RecordTurn("max_steps", a.maxSteps, time.Since(startTime))
	slog.Info("agent turn",
		"turn_id", turnID,
		"user_id", turn.UserID,
		"steps", a.maxSteps,
		"tools_called", toolsCalled,
		"duration_ms", time.Since(startTime).Milliseconds(),
		"outcome", "max_steps",
	)
	return ErrMaxSteps
}

// safeCall invokes a tool with a deferred recover so a panicking tool becomes an error.
func safeCall(ctx context.Context, t tools.Tool, args json.RawMessage, userID string) (result tools.Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("tool %q panicked: %v", t.Name(), r)
			slog.Error("tool panic recovered",
				"tool", t.Name(),
				"panic", fmt.Sprintf("%v", r),
			)
		}
	}()
	return t.Call(ctx, args, userID)
}
