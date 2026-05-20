package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/observability-mcp-service/internal/config"
	"github.com/kabradshaw1/portfolio/go/observability-mcp-service/internal/history"
	"github.com/kabradshaw1/portfolio/go/observability-mcp-service/internal/management"
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

func TestHistoryToolsAreRegistered(t *testing.T) {
	server := New(&fakeWorkflow{}, testConfig())
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "0.1.0"}, nil)
	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	serverSession, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer serverSession.Close()
	clientSession, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer clientSession.Close()

	tools, err := clientSession.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	got := make(map[string]bool)
	for _, tool := range tools.Tools {
		got[tool.Name] = true
	}
	for _, name := range []string{"list_incidents", "get_incident_history", "add_incident_note", "compare_evidence_windows", "list_management_actions", "preview_management_action", "execute_management_action", "get_management_action_history"} {
		if !got[name] {
			t.Fatalf("expected registered tool %q; got %v", name, got)
		}
	}
}

func TestManagementActionHandlers(t *testing.T) {
	fake := &fakeWorkflow{}
	result, err := previewManagementActionHandler(fake)(context.Background(), callReq(map[string]any{"action_id": "reload_grafana_alerting"}))
	if err != nil || result.IsError {
		t.Fatalf("preview failed: result=%#v err=%v", result, err)
	}
	if fake.managementRequest.ActionID != "reload_grafana_alerting" {
		t.Fatalf("request = %+v", fake.managementRequest)
	}

	result, err = executeManagementActionHandler(fake)(context.Background(), callReq(map[string]any{
		"action_id":      "reload_grafana_alerting",
		"incident_key":   "inc",
		"incident_title": "Incident",
	}))
	if err != nil || result.IsError {
		t.Fatalf("execute failed: result=%#v err=%v", result, err)
	}
	if fake.managementRequest.IncidentKey != "inc" {
		t.Fatalf("request = %+v", fake.managementRequest)
	}
}

func TestManagementActionHistoryHandlerValidatesLimit(t *testing.T) {
	result, err := managementActionHistoryHandler(&fakeWorkflow{})(context.Background(), callReq(map[string]any{"limit": 101}))
	if err != nil {
		t.Fatalf("handler transport error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error")
	}
}

func TestWindowHandlerPassesCaptureOptions(t *testing.T) {
	persist := false
	fake := &fakeWorkflow{}
	handler := windowHandler(testConfig(), fake.GetSystemHealth)

	result, err := handler(context.Background(), callReq(map[string]any{
		"window":         "5m",
		"incident_key":   "inc-1",
		"incident_title": "Checkout degraded",
		"severity":       "warning",
		"service":        "go-order-service",
		"persist":        persist,
	}))
	if err != nil || result.IsError {
		t.Fatalf("handler failed: result=%#v err=%v", result, err)
	}
	if fake.window != 5*time.Minute {
		t.Fatalf("window = %s", fake.window)
	}
	if fake.capture.IncidentKey != "inc-1" ||
		fake.capture.IncidentTitle != "Checkout degraded" ||
		fake.capture.Severity != "warning" ||
		fake.capture.Service != "go-order-service" {
		t.Fatalf("capture options = %#v", fake.capture)
	}
	if fake.capture.Persist == nil || *fake.capture.Persist != persist {
		t.Fatalf("persist = %#v", fake.capture.Persist)
	}
}

func TestAddIncidentNoteHandlerRequiresIncidentKeyAndNote(t *testing.T) {
	handler := addIncidentNoteHandler(&fakeWorkflow{})
	for _, tc := range []struct {
		name string
		args map[string]any
		want string
	}{
		{name: "incident key", args: map[string]any{"note": "checked logs"}, want: "incident_key is required"},
		{name: "note", args: map[string]any{"incident_key": "inc-1"}, want: "note is required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := handler(context.Background(), callReq(tc.args))
			if err != nil {
				t.Fatalf("handler returned transport error: %v", err)
			}
			if !result.IsError {
				t.Fatal("expected tool error")
			}
			if got := resultText(result); got != tc.want {
				t.Fatalf("error = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAddIncidentNoteHandlerRejectsInvalidStatus(t *testing.T) {
	result, err := addIncidentNoteHandler(&fakeWorkflow{})(context.Background(), callReq(map[string]any{
		"incident_key": "inc-1",
		"note":         "checked logs",
		"status":       "warning",
	}))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error")
	}
	if got := resultText(result); got != "status must be one of investigating, mitigated, resolved" {
		t.Fatalf("error = %q", got)
	}
}

func TestCompareEvidenceWindowsHandlerRequiresBaselineSnapshotID(t *testing.T) {
	handler := compareEvidenceWindowsHandler(&fakeWorkflow{})
	result, err := handler(context.Background(), callReq(map[string]any{"candidate_snapshot_id": 2}))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error")
	}
	if got := resultText(result); got != "baseline_snapshot_id is required" {
		t.Fatalf("error = %q", got)
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
	window            time.Duration
	evalID            string
	capture           workflows.CaptureOptions
	managementRequest management.ActionRequest
}

func (f *fakeWorkflow) GetSystemHealth(_ context.Context, window time.Duration, capture workflows.CaptureOptions) workflows.EvidenceBundle {
	f.window = window
	f.capture = capture
	return workflows.NewBundle("get_system_health", window.String())
}

func (f *fakeWorkflow) InvestigateCheckout(_ context.Context, window time.Duration, capture workflows.CaptureOptions) workflows.EvidenceBundle {
	f.window = window
	f.capture = capture
	return workflows.NewBundle("investigate_checkout", "15m")
}

func (f *fakeWorkflow) InvestigateAIPipeline(_ context.Context, window time.Duration, capture workflows.CaptureOptions) workflows.EvidenceBundle {
	f.window = window
	f.capture = capture
	return workflows.NewBundle("investigate_ai_pipeline", "15m")
}

func (f *fakeWorkflow) InvestigateEvalRun(_ context.Context, window time.Duration, evalID string, capture workflows.CaptureOptions) workflows.EvidenceBundle {
	f.window = window
	f.evalID = evalID
	f.capture = capture
	return workflows.NewBundle("investigate_eval_run", window.String())
}

func (f *fakeWorkflow) InvestigateStreamingAnalytics(_ context.Context, window time.Duration, capture workflows.CaptureOptions) workflows.EvidenceBundle {
	f.window = window
	f.capture = capture
	return workflows.NewBundle("investigate_streaming_analytics", "15m")
}

func (f *fakeWorkflow) GetServiceEvidence(_ context.Context, _ string, window time.Duration, _ string, capture workflows.CaptureOptions) workflows.EvidenceBundle {
	f.window = window
	f.capture = capture
	return workflows.NewBundle("get_service_evidence", "15m")
}

func (f *fakeWorkflow) SearchLogs(context.Context, string, time.Duration, string) workflows.EvidenceBundle {
	return workflows.NewBundle("search_logs", "15m")
}

func (f *fakeWorkflow) GetTrace(context.Context, string) workflows.EvidenceBundle {
	return workflows.NewBundle("get_trace", "15m")
}

func (f *fakeWorkflow) ListIncidents(context.Context, history.ListFilter) ([]history.IncidentSummary, error) {
	return []history.IncidentSummary{}, nil
}

func (f *fakeWorkflow) GetIncidentHistory(context.Context, string) (history.IncidentHistory, error) {
	return history.IncidentHistory{}, nil
}

func (f *fakeWorkflow) AddIncidentNote(context.Context, history.AddNoteInput) (history.Event, error) {
	return history.Event{}, nil
}

func (f *fakeWorkflow) CompareEvidenceSnapshots(context.Context, int64, int64) (workflows.EvidenceComparison, error) {
	return workflows.EvidenceComparison{}, nil
}

func (f *fakeWorkflow) ListManagementActions(context.Context) ([]management.Action, error) {
	return []management.Action{{ID: "reload_grafana_alerting"}}, nil
}

func (f *fakeWorkflow) PreviewManagementAction(_ context.Context, req management.ActionRequest) (management.ActionResult, error) {
	f.managementRequest = req
	return management.ActionResult{ActionID: req.ActionID, Status: management.StatusPreviewed}, nil
}

func (f *fakeWorkflow) ExecuteManagementAction(_ context.Context, req management.ActionRequest) (management.ActionResult, error) {
	f.managementRequest = req
	return management.ActionResult{ActionID: req.ActionID, Status: management.StatusSucceeded}, nil
}

func (f *fakeWorkflow) ListManagementActionHistory(context.Context, history.ManagementActionFilter) ([]history.Event, error) {
	return []history.Event{}, nil
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

func resultText(result *sdkmcp.CallToolResult) string {
	if len(result.Content) == 0 {
		return ""
	}
	text, ok := result.Content[0].(*sdkmcp.TextContent)
	if !ok {
		return ""
	}
	return text.Text
}
