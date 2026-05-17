package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/observability-mcp-service/internal/config"
	"github.com/kabradshaw1/portfolio/go/observability-mcp-service/internal/workflows"
)

func TestNewReturnsServer(t *testing.T) {
	if New(&fakeWorkflow{}, testConfig()) == nil {
		t.Fatal("expected server")
	}
}

func TestWindowHandlerDecodesEmptyAndExplicitWindows(t *testing.T) {
	fake := &fakeWorkflow{}
	handler := windowHandler(testConfig(), fake.GetSystemHealth)
	result, err := handler(context.Background(), callReq(map[string]any{}))
	if err != nil || result.IsError {
		t.Fatalf("empty window result=%#v err=%v", result, err)
	}
	if fake.window != 15*time.Minute {
		t.Fatalf("default window = %s", fake.window)
	}
	result, err = handler(context.Background(), callReq(map[string]any{"window": "5m"}))
	if err != nil || result.IsError {
		t.Fatalf("explicit window result=%#v err=%v", result, err)
	}
	if fake.window != 5*time.Minute {
		t.Fatalf("explicit window = %s", fake.window)
	}
}

func TestInvalidServiceReturnsToolError(t *testing.T) {
	result, err := serviceEvidenceHandler(testConfig(), &fakeWorkflow{})(context.Background(), callReq(map[string]any{"service": "kube-system"}))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected tool error")
	}
}

func TestRunbookResourceReturnsMarkdown(t *testing.T) {
	result, err := runbookResourceHandler("observability://runbooks/checkout", "checkout")(context.Background(), &sdkmcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("resource handler returned error: %v", err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("contents = %d", len(result.Contents))
	}
	if !strings.Contains(result.Contents[0].Text, "Checkout Investigation") {
		t.Fatalf("unexpected runbook: %q", result.Contents[0].Text)
	}
}

func TestTraceHandlerRequiresTraceID(t *testing.T) {
	result, err := traceHandler(&fakeWorkflow{})(context.Background(), callReq(map[string]any{"trace_id": ""}))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error")
	}
}

func TestInvestigateEvalRunHandlerRequiresEvalID(t *testing.T) {
	result, err := investigateEvalRunHandler(testConfig(), &fakeWorkflow{})(context.Background(), callReq(map[string]any{}))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error")
	}
}

func TestInvestigateEvalRunHandlerReturnsBundle(t *testing.T) {
	fake := &fakeWorkflow{}
	result, err := investigateEvalRunHandler(testConfig(), fake)(context.Background(), callReq(map[string]any{
		"eval_id": "eval-123",
		"window":  "5m",
	}))
	if err != nil || result.IsError {
		t.Fatalf("handler failed: result=%#v err=%v", result, err)
	}
	if fake.evalID != "eval-123" || fake.window != 5*time.Minute {
		t.Fatalf("fake state: evalID=%q window=%s", fake.evalID, fake.window)
	}
}

type fakeWorkflow struct {
	window time.Duration
	evalID string
}

func (f *fakeWorkflow) GetSystemHealth(_ context.Context, window time.Duration) workflows.EvidenceBundle {
	f.window = window
	return workflows.NewBundle("get_system_health", window.String())
}

func (f *fakeWorkflow) InvestigateCheckout(context.Context, time.Duration) workflows.EvidenceBundle {
	return workflows.NewBundle("investigate_checkout", "15m")
}

func (f *fakeWorkflow) InvestigateAIPipeline(context.Context, time.Duration) workflows.EvidenceBundle {
	return workflows.NewBundle("investigate_ai_pipeline", "15m")
}

func (f *fakeWorkflow) InvestigateEvalRun(_ context.Context, window time.Duration, evalID string) workflows.EvidenceBundle {
	f.window = window
	f.evalID = evalID
	return workflows.NewBundle("investigate_eval_run", window.String())
}

func (f *fakeWorkflow) InvestigateStreamingAnalytics(context.Context, time.Duration) workflows.EvidenceBundle {
	return workflows.NewBundle("investigate_streaming_analytics", "15m")
}

func (f *fakeWorkflow) GetServiceEvidence(context.Context, string, time.Duration, string) workflows.EvidenceBundle {
	return workflows.NewBundle("get_service_evidence", "15m")
}

func (f *fakeWorkflow) SearchLogs(context.Context, string, time.Duration, string) workflows.EvidenceBundle {
	return workflows.NewBundle("search_logs", "15m")
}

func (f *fakeWorkflow) GetTrace(context.Context, string) workflows.EvidenceBundle {
	return workflows.NewBundle("get_trace", "15m")
}

func testConfig() config.Config {
	return config.Config{DefaultWindow: 15 * time.Minute, MaxWindow: time.Hour, MaxLogLines: 100, MaxTraceSpans: 100}
}

func callReq(args map[string]any) *sdkmcp.CallToolRequest {
	raw, err := json.Marshal(args)
	if err != nil {
		panic(err)
	}
	return &sdkmcp.CallToolRequest{Params: &sdkmcp.CallToolParamsRaw{Arguments: raw}}
}
