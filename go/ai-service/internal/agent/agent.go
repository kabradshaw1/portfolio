package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/llm"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools"
)

// ErrMaxSteps is returned when the agent loop exceeds the configured step cap.
var ErrMaxSteps = errors.New("agent: max steps exceeded")

// Turn is one invocation of the agent — a user id plus the full conversation so far.
type Turn struct {
	UserID   string
	Messages []llm.Message
}

// Agent runs the LLM tool-calling loop.
type Agent struct {
	llm      llm.Client
	registry tools.Registry
	maxSteps int
	timeout  time.Duration
}

// New constructs an Agent.
func New(client llm.Client, registry tools.Registry, maxSteps int, timeout time.Duration) *Agent {
	return &Agent{llm: client, registry: registry, maxSteps: maxSteps, timeout: timeout}
}

// Run executes the loop. The emit callback receives every event in order.
// Infrastructure failures (LLM unreachable, ctx cancelled, max steps) are returned as errors.
// Tool-level failures are fed back into the conversation as tool results and do not return an error.
func (a *Agent) Run(ctx context.Context, turn Turn, emit func(Event)) error {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()

	messages := append([]llm.Message(nil), turn.Messages...)

	for step := 0; step < a.maxSteps; step++ {
		resp, err := a.llm.Chat(ctx, messages, a.registry.Schemas())
		if err != nil {
			emit(Event{Error: &ErrorEvent{Reason: err.Error()}})
			return fmt.Errorf("llm chat: %w", err)
		}

		if len(resp.ToolCalls) == 0 {
			emit(Event{Final: &FinalEvent{Text: resp.Content}})
			return nil
		}

		// Record the assistant's tool-call message in history.
		messages = append(messages, llm.Message{
			Role:      llm.RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		for _, call := range resp.ToolCalls {
			emit(Event{ToolCall: &ToolCallEvent{Name: call.Name, Args: call.Args}})

			tool, ok := a.registry.Get(call.Name)
			if !ok {
				errMsg := "unknown tool: " + call.Name
				emit(Event{ToolError: &ToolErrorEvent{Name: call.Name, Error: errMsg}})
				msg, _ := llm.ToolResultMessage(call.ID, call.Name, map[string]string{"error": errMsg})
				messages = append(messages, msg)
				continue
			}

			result, toolErr := safeCall(ctx, tool, call.Args, turn.UserID)
			if toolErr != nil {
				emit(Event{ToolError: &ToolErrorEvent{Name: call.Name, Error: toolErr.Error()}})
				msg, _ := llm.ToolResultMessage(call.ID, call.Name, map[string]string{"error": toolErr.Error()})
				messages = append(messages, msg)
				continue
			}

			emit(Event{ToolResult: &ToolResultEvent{Name: call.Name, Display: result.Display}})
			msg, err := llm.ToolResultMessage(call.ID, call.Name, result.Content)
			if err != nil {
				errMsg := "tool result not serializable: " + err.Error()
				emit(Event{ToolError: &ToolErrorEvent{Name: call.Name, Error: errMsg}})
				msg2, _ := llm.ToolResultMessage(call.ID, call.Name, map[string]string{"error": errMsg})
				messages = append(messages, msg2)
				continue
			}
			messages = append(messages, msg)
		}
	}

	emit(Event{Error: &ErrorEvent{Reason: ErrMaxSteps.Error()}})
	return ErrMaxSteps
}

// safeCall invokes a tool with a deferred recover so a panicking tool becomes an error.
func safeCall(ctx context.Context, t tools.Tool, args json.RawMessage, userID string) (result tools.Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("tool %q panicked: %v", t.Name(), r)
		}
	}()
	return t.Call(ctx, args, userID)
}
