//go:build eval

package evals

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/agent"
	mcpadapter "github.com/kabradshaw1/portfolio/go/ai-service/internal/mcp"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/llm"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
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

func TestEval_MCPRoundTrip(t *testing.T) {
	// Set up a local registry with one tool.
	innerTool := &EchoTool{
		ToolName: "search_products",
		Result:   tools.Result{Content: []map[string]any{{"id": "p1", "name": "Jacket"}}},
	}
	innerReg := tools.NewMemRegistry()
	innerReg.Register(innerTool)

	// Stand up an MCP server over this registry.
	mcpSrv := mcpadapter.NewServer(innerReg, mcpadapter.Defaults{UserID: "test-user"})

	// Connect an MCP client and discover tools.
	ctx := context.Background()

	// Use in-memory transports (SDK pattern: server connects first, then client)
	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	_, err := mcpSrv.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	sdkClient := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "eval", Version: "1.0.0"}, nil)
	session, err := sdkClient.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer session.Close()

	discovered, err := mcpadapter.DiscoverTools(ctx, session, "mcp")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}

	// Register MCP-wrapped tools into a fresh registry for the agent.
	agentReg := tools.NewMemRegistry()
	for _, d := range discovered {
		agentReg.Register(d)
	}

	// Script the LLM to call the MCP-prefixed tool, then give a final answer.
	scripted := &ScriptedLLM{Responses: []llm.ChatResponse{
		{ToolCalls: []llm.ToolCall{{
			ID:   "c1",
			Name: "mcp.search_products",
			Args: json.RawMessage(`{"query":"jacket"}`),
		}}},
		{Content: "Found a jacket for you."},
	}}

	events, err := Run(scripted, agentReg,
		agent.Turn{UserID: "user-1", Messages: []llm.Message{{Role: llm.RoleUser, Content: "find me a jacket"}}},
		8)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// The tool should have been called through the MCP round-trip.
	if innerTool.Calls != 1 {
		t.Errorf("expected 1 call to inner tool, got %d", innerTool.Calls)
	}
	if len(events) == 0 || events[len(events)-1].Final == nil {
		t.Errorf("expected final event, got %+v", events)
	}
}
