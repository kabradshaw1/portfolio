//go:build eval

package evals

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/agent"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/llm"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools"
)

// ScriptedLLM returns canned ChatResponses in order. Simulates tool-call
// sequences without touching a real model.
type ScriptedLLM struct {
	Responses []llm.ChatResponse
	calls     int
}

func (s *ScriptedLLM) Chat(ctx context.Context, _ []llm.Message, _ []llm.ToolSchema) (llm.ChatResponse, error) {
	if s.calls >= len(s.Responses) {
		return llm.ChatResponse{}, errors.New("scripted LLM: unexpected extra call")
	}
	r := s.Responses[s.calls]
	s.calls++
	return r, nil
}

// EchoTool records what it was called with and returns a canned result.
type EchoTool struct {
	ToolName string
	Calls    int
	SeenArgs []json.RawMessage
	Result   tools.Result
	Err      error
}

func (e *EchoTool) Name() string            { return e.ToolName }
func (e *EchoTool) Description() string     { return "echo " + e.ToolName }
func (e *EchoTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (e *EchoTool) Call(ctx context.Context, args json.RawMessage, userID string) (tools.Result, error) {
	e.Calls++
	e.SeenArgs = append(e.SeenArgs, args)
	return e.Result, e.Err
}

// Run runs the agent with the given LLM script and registry and returns the
// ordered slice of events emitted. Uses a generous timeout so eval cases never
// hit the wall-clock bound.
func Run(scripted *ScriptedLLM, reg tools.Registry, turn agent.Turn, maxSteps int) ([]agent.Event, error) {
	a := agent.New(scripted, reg, metrics.NopRecorder{}, maxSteps, time.Minute)
	var events []agent.Event
	err := a.Run(context.Background(), turn, func(e agent.Event) { events = append(events, e) })
	return events, err
}
