//go:build eval

package evals

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/agent"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/llm"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools"
)

func TestEval_CallsSearchProductsWithPriceFilter(t *testing.T) {
	searchTool := &EchoTool{ToolName: "search_products", Result: tools.Result{Content: []map[string]any{{"id": "p1"}}}}
	reg := tools.NewMemRegistry()
	reg.Register(searchTool)

	scripted := &ScriptedLLM{Responses: []llm.ChatResponse{
		{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "search_products", Args: json.RawMessage(`{"query":"jacket","max_price":150}`)}}},
		{Content: "Here are some jackets under $150."},
	}}

	events, err := Run(scripted, reg,
		agent.Turn{UserID: "user-1", Messages: []llm.Message{{Role: llm.RoleUser, Content: "find me a jacket under 150"}}},
		8)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if searchTool.Calls != 1 {
		t.Errorf("search_products calls = %d", searchTool.Calls)
	}
	if len(events) == 0 || events[len(events)-1].Final == nil {
		t.Errorf("expected final event, got %+v", events)
	}
}

func TestEval_RecoversFromToolError(t *testing.T) {
	badTool := &EchoTool{ToolName: "get_order", Err: errors.New("upstream 500")}
	reg := tools.NewMemRegistry()
	reg.Register(badTool)

	scripted := &ScriptedLLM{Responses: []llm.ChatResponse{
		{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "get_order", Args: json.RawMessage(`{"order_id":"x"}`)}}},
		{Content: "Sorry, I couldn't fetch that order right now."},
	}}

	events, err := Run(scripted, reg,
		agent.Turn{UserID: "user-1", Messages: []llm.Message{{Role: llm.RoleUser, Content: "where's order x"}}},
		8)
	if err != nil {
		t.Fatalf("run should not bubble tool error: %v", err)
	}
	if events[len(events)-1].Final == nil {
		t.Errorf("expected recovered final: %+v", events)
	}
}

func TestEval_HonorsMaxSteps(t *testing.T) {
	looper := &EchoTool{ToolName: "echo"}
	reg := tools.NewMemRegistry()
	reg.Register(looper)

	call := llm.ToolCall{ID: "c", Name: "echo", Args: json.RawMessage(`{}`)}
	scripted := &ScriptedLLM{Responses: []llm.ChatResponse{
		{ToolCalls: []llm.ToolCall{call}},
		{ToolCalls: []llm.ToolCall{call}},
		{ToolCalls: []llm.ToolCall{call}},
	}}

	_, err := Run(scripted, reg,
		agent.Turn{UserID: "u", Messages: []llm.Message{{Role: llm.RoleUser, Content: "spin"}}},
		3)
	if err == nil {
		t.Fatal("expected ErrMaxSteps")
	}
}
