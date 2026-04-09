package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/llm"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools"
)

// --- fake llm.Client that returns canned responses in order ---

type fakeLLM struct {
	responses []llm.ChatResponse
	err       error
	calls     int
}

func (f *fakeLLM) Chat(ctx context.Context, msgs []llm.Message, ts []llm.ToolSchema) (llm.ChatResponse, error) {
	if f.err != nil {
		return llm.ChatResponse{}, f.err
	}
	if f.calls >= len(f.responses) {
		return llm.ChatResponse{}, errors.New("unexpected extra call")
	}
	r := f.responses[f.calls]
	f.calls++
	return r, nil
}

// --- fake tool ---

type scriptedTool struct {
	name   string
	result tools.Result
	err    error
	calls  int
}

func (s *scriptedTool) Name() string            { return s.name }
func (s *scriptedTool) Description() string     { return "" }
func (s *scriptedTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (s *scriptedTool) Call(ctx context.Context, args json.RawMessage, userID string) (tools.Result, error) {
	s.calls++
	return s.result, s.err
}

func collect(events *[]Event) func(Event) {
	return func(e Event) { *events = append(*events, e) }
}

func TestAgent_FinalOnFirstTurn(t *testing.T) {
	llmc := &fakeLLM{responses: []llm.ChatResponse{{Content: "hi there"}}}
	reg := tools.NewMemRegistry()
	a := New(llmc, reg, metrics.NopRecorder{}, 8, 5*time.Second)

	var events []Event
	err := a.Run(context.Background(), Turn{Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}}}, collect(&events))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(events) != 1 || events[0].Final == nil || events[0].Final.Text != "hi there" {
		t.Fatalf("expected single final event, got %+v", events)
	}
}

func TestAgent_ToolCallThenFinal(t *testing.T) {
	llmc := &fakeLLM{responses: []llm.ChatResponse{
		{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "echo", Args: json.RawMessage(`{"x":1}`)}}},
		{Content: "done"},
	}}
	tool := &scriptedTool{name: "echo", result: tools.Result{Content: map[string]any{"ok": true}}}
	reg := tools.NewMemRegistry()
	reg.Register(tool)

	a := New(llmc, reg, metrics.NopRecorder{}, 8, 5*time.Second)
	var events []Event
	err := a.Run(context.Background(), Turn{Messages: []llm.Message{{Role: llm.RoleUser, Content: "go"}}}, collect(&events))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if tool.calls != 1 {
		t.Errorf("expected tool called once, got %d", tool.calls)
	}
	// Expect: ToolCallEvent, ToolResultEvent, FinalEvent
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d: %+v", len(events), events)
	}
	if events[0].ToolCall == nil || events[0].ToolCall.Name != "echo" {
		t.Errorf("event[0] = %+v", events[0])
	}
	if events[1].ToolResult == nil || events[1].ToolResult.Name != "echo" {
		t.Errorf("event[1] = %+v", events[1])
	}
	if events[2].Final == nil || events[2].Final.Text != "done" {
		t.Errorf("event[2] = %+v", events[2])
	}
}

func TestAgent_UnknownToolRecoversAndContinues(t *testing.T) {
	llmc := &fakeLLM{responses: []llm.ChatResponse{
		{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "missing", Args: json.RawMessage(`{}`)}}},
		{Content: "ok"},
	}}
	a := New(llmc, tools.NewMemRegistry(), metrics.NopRecorder{}, 8, 5*time.Second)
	var events []Event
	err := a.Run(context.Background(), Turn{Messages: []llm.Message{{Role: llm.RoleUser, Content: "go"}}}, collect(&events))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	// Unknown tool becomes a tool_error event, loop continues, final answer lands.
	if events[len(events)-1].Final == nil {
		t.Errorf("expected final event at end, got %+v", events[len(events)-1])
	}
	foundErr := false
	for _, e := range events {
		if e.ToolError != nil && e.ToolError.Name == "missing" {
			foundErr = true
		}
	}
	if !foundErr {
		t.Errorf("expected a tool_error event for missing tool")
	}
}

func TestAgent_ToolErrorIsFedBackNotBubbled(t *testing.T) {
	llmc := &fakeLLM{responses: []llm.ChatResponse{
		{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "flaky", Args: json.RawMessage(`{}`)}}},
		{Content: "recovered"},
	}}
	reg := tools.NewMemRegistry()
	reg.Register(&scriptedTool{name: "flaky", err: errors.New("boom")})
	a := New(llmc, reg, metrics.NopRecorder{}, 8, 5*time.Second)
	var events []Event
	if err := a.Run(context.Background(), Turn{Messages: []llm.Message{{Role: llm.RoleUser, Content: "go"}}}, collect(&events)); err != nil {
		t.Fatalf("Run error (should not bubble tool error): %v", err)
	}
	if events[len(events)-1].Final == nil || events[len(events)-1].Final.Text != "recovered" {
		t.Errorf("expected recovered final, got %+v", events[len(events)-1])
	}
}

func TestAgent_MaxStepsCap(t *testing.T) {
	// Model never stops calling a tool.
	loopTool := llm.ToolCall{ID: "c", Name: "echo", Args: json.RawMessage(`{}`)}
	llmc := &fakeLLM{responses: []llm.ChatResponse{
		{ToolCalls: []llm.ToolCall{loopTool}},
		{ToolCalls: []llm.ToolCall{loopTool}},
		{ToolCalls: []llm.ToolCall{loopTool}},
	}}
	reg := tools.NewMemRegistry()
	reg.Register(&scriptedTool{name: "echo", result: tools.Result{Content: map[string]any{"ok": true}}})
	a := New(llmc, reg, metrics.NopRecorder{}, 3, 5*time.Second)
	var events []Event
	err := a.Run(context.Background(), Turn{Messages: []llm.Message{{Role: llm.RoleUser, Content: "go"}}}, collect(&events))
	if !errors.Is(err, ErrMaxSteps) {
		t.Fatalf("expected ErrMaxSteps, got %v", err)
	}
	if events[len(events)-1].Error == nil {
		t.Errorf("expected trailing error event, got %+v", events[len(events)-1])
	}
}

func TestAgent_LLMErrorBubbles(t *testing.T) {
	llmc := &fakeLLM{err: errors.New("ollama down")}
	a := New(llmc, tools.NewMemRegistry(), metrics.NopRecorder{}, 8, 5*time.Second)
	err := a.Run(context.Background(), Turn{Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}}}, func(Event) {})
	if err == nil {
		t.Fatal("expected error from LLM failure")
	}
}
