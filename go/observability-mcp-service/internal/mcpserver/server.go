package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/observability-mcp-service/internal/config"
	"github.com/kabradshaw1/portfolio/go/observability-mcp-service/internal/history"
	"github.com/kabradshaw1/portfolio/go/observability-mcp-service/internal/management"
	"github.com/kabradshaw1/portfolio/go/observability-mcp-service/internal/workflows"
	"github.com/kabradshaw1/portfolio/go/observability-mcp-service/resources/runbooks"
)

type WorkflowService interface {
	GetSystemHealth(context.Context, time.Duration, workflows.CaptureOptions) workflows.EvidenceBundle
	InvestigateCheckout(context.Context, time.Duration, workflows.CaptureOptions) workflows.EvidenceBundle
	InvestigateAIPipeline(context.Context, time.Duration, workflows.CaptureOptions) workflows.EvidenceBundle
	InvestigateEvalRun(context.Context, time.Duration, string, workflows.CaptureOptions) workflows.EvidenceBundle
	InvestigateStreamingAnalytics(context.Context, time.Duration, workflows.CaptureOptions) workflows.EvidenceBundle
	GetServiceEvidence(context.Context, string, time.Duration, string, workflows.CaptureOptions) workflows.EvidenceBundle
	SearchLogs(context.Context, string, time.Duration, string) workflows.EvidenceBundle
	GetTrace(context.Context, string) workflows.EvidenceBundle
	ListIncidents(context.Context, history.ListFilter) ([]history.IncidentSummary, error)
	GetIncidentHistory(context.Context, string) (history.IncidentHistory, error)
	AddIncidentNote(context.Context, history.AddNoteInput) (history.Event, error)
	CompareEvidenceSnapshots(context.Context, int64, int64) (workflows.EvidenceComparison, error)
	ListManagementActions(context.Context) ([]management.Action, error)
	PreviewManagementAction(context.Context, management.ActionRequest) (management.ActionResult, error)
	ExecuteManagementAction(context.Context, management.ActionRequest) (management.ActionResult, error)
	ListManagementActionHistory(context.Context, history.ManagementActionFilter) ([]history.Event, error)
}

func New(service WorkflowService, cfg config.Config) *sdkmcp.Server {
	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "observability-mcp-service", Version: "0.1.0"}, nil)
	addTool(srv, "get_system_health", "Return a compact system-wide observability evidence bundle.", windowSchema(), windowHandler(cfg, service.GetSystemHealth))
	addTool(srv, "investigate_checkout", "Build checkout and saga failure evidence.", windowSchema(), windowHandler(cfg, service.InvestigateCheckout))
	addTool(srv, "investigate_ai_pipeline", "Build AI/RAG pipeline evidence.", windowSchema(), windowHandler(cfg, service.InvestigateAIPipeline))
	addTool(srv, "investigate_eval_run", "Build eval-run-specific RAG evidence from metrics and logs.", evalRunSchema(), investigateEvalRunHandler(cfg, service))
	addTool(srv, "investigate_streaming_analytics", "Build Kafka and analytics evidence.", windowSchema(), windowHandler(cfg, service.InvestigateStreamingAnalytics))
	addTool(srv, "get_service_evidence", "Return bounded evidence for one allowlisted service.", serviceEvidenceSchema(), serviceEvidenceHandler(cfg, service))
	addTool(srv, "search_logs", "Search recent logs for one allowlisted service.", searchLogsSchema(), searchLogsHandler(cfg, service))
	addTool(srv, "get_trace", "Fetch and summarize a Jaeger trace.", traceSchema(), traceHandler(service))
	addTool(srv, "list_incidents", "List persisted observability incidents.", listIncidentsSchema(), listIncidentsHandler(service))
	addTool(srv, "get_incident_history", "Return incident timeline and evidence summaries.", incidentHistorySchema(), incidentHistoryHandler(service))
	addTool(srv, "add_incident_note", "Append an incident note and optionally transition status.", addIncidentNoteSchema(), addIncidentNoteHandler(service))
	addTool(srv, "compare_evidence_windows", "Compare two persisted observability evidence snapshots.", compareEvidenceWindowsSchema(), compareEvidenceWindowsHandler(service))
	addTool(srv, "list_management_actions", "List cataloged observability management actions.", listManagementActionsSchema(), listManagementActionsHandler(service))
	addTool(srv, "preview_management_action", "Validate and preview a cataloged management action without executing it.", managementActionSchema(), previewManagementActionHandler(service))
	addTool(srv, "execute_management_action", "Execute an allowed cataloged management action.", managementActionSchema(), executeManagementActionHandler(service))
	addTool(srv, "get_management_action_history", "List persisted management action events.", managementActionHistorySchema(), managementActionHistoryHandler(service))
	addResource(srv, "observability://runbooks/system-health", "System Health Runbook", "Signals and interpretations for system health.", runbookResourceHandler("observability://runbooks/system-health", "system-health"))
	addResource(srv, "observability://runbooks/checkout", "Checkout Runbook", "Signals and interpretations for checkout incidents.", runbookResourceHandler("observability://runbooks/checkout", "checkout"))
	addResource(srv, "observability://runbooks/ai-pipeline", "AI Pipeline Runbook", "Signals and interpretations for AI/RAG incidents.", runbookResourceHandler("observability://runbooks/ai-pipeline", "ai-pipeline"))
	addResource(srv, "observability://runbooks/streaming-analytics", "Streaming Analytics Runbook", "Signals and interpretations for analytics incidents.", runbookResourceHandler("observability://runbooks/streaming-analytics", "streaming-analytics"))
	return srv
}

func addTool(srv *sdkmcp.Server, name, description string, schema json.RawMessage, handler sdkmcp.ToolHandler) {
	srv.AddTool(&sdkmcp.Tool{Name: name, Description: description, InputSchema: schema}, handler)
}

func addResource(srv *sdkmcp.Server, uri, name, description string, handler sdkmcp.ResourceHandler) {
	srv.AddResource(&sdkmcp.Resource{URI: uri, Name: name, Title: name, Description: description, MIMEType: "text/markdown"}, handler)
}

type investigationInput struct {
	Window        string `json:"window,omitempty"`
	IncidentKey   string `json:"incident_key,omitempty"`
	IncidentTitle string `json:"incident_title,omitempty"`
	Severity      string `json:"severity,omitempty"`
	Service       string `json:"service,omitempty"`
	Persist       *bool  `json:"persist,omitempty"`
}

func (in investigationInput) captureOptions() workflows.CaptureOptions {
	return workflows.CaptureOptions{
		IncidentKey:   in.IncidentKey,
		IncidentTitle: in.IncidentTitle,
		Severity:      in.Severity,
		Service:       in.Service,
		Persist:       in.Persist,
	}
}

func windowHandler(cfg config.Config, fn func(context.Context, time.Duration, workflows.CaptureOptions) workflows.EvidenceBundle) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var in investigationInput
		if err := decodeOptionalArgs(req, &in); err != nil {
			return toolError(err.Error()), nil
		}
		window, err := cfg.WindowOrDefault(in.Window)
		if err != nil {
			return toolError(err.Error()), nil
		}
		return jsonResult(fn(ctx, window, in.captureOptions())), nil
	}
}

func serviceEvidenceHandler(cfg config.Config, service WorkflowService) sdkmcp.ToolHandler {
	type input struct {
		investigationInput
		TraceID string `json:"trace_id,omitempty"`
	}
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var in input
		if err := decodeArgs(req, &in); err != nil {
			return toolError(err.Error()), nil
		}
		window, err := cfg.WindowOrDefault(in.Window)
		if err != nil {
			return toolError(err.Error()), nil
		}
		if !workflows.AllowedService(in.Service) {
			return toolError(fmt.Sprintf("service %q is not allowlisted", in.Service)), nil
		}
		capture := in.captureOptions()
		capture.Service = in.Service
		return jsonResult(service.GetServiceEvidence(ctx, in.Service, window, in.TraceID, capture)), nil
	}
}

func investigateEvalRunHandler(cfg config.Config, service WorkflowService) sdkmcp.ToolHandler {
	type input struct {
		investigationInput
		EvalID string `json:"eval_id"`
	}
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var in input
		if err := decodeArgs(req, &in); err != nil {
			return toolError(err.Error()), nil
		}
		if in.EvalID == "" {
			return toolError("eval_id is required"), nil
		}
		window, err := cfg.WindowOrDefault(in.Window)
		if err != nil {
			return toolError(err.Error()), nil
		}
		return jsonResult(service.InvestigateEvalRun(ctx, window, in.EvalID, in.captureOptions())), nil
	}
}

func searchLogsHandler(cfg config.Config, service WorkflowService) sdkmcp.ToolHandler {
	type input struct {
		Window  string `json:"window,omitempty"`
		Service string `json:"service"`
		Pattern string `json:"pattern,omitempty"`
	}
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var in input
		if err := decodeArgs(req, &in); err != nil {
			return toolError(err.Error()), nil
		}
		window, err := cfg.WindowOrDefault(in.Window)
		if err != nil {
			return toolError(err.Error()), nil
		}
		if !workflows.AllowedService(in.Service) {
			return toolError(fmt.Sprintf("service %q is not allowlisted", in.Service)), nil
		}
		return jsonResult(service.SearchLogs(ctx, in.Service, window, in.Pattern)), nil
	}
}

func traceHandler(service WorkflowService) sdkmcp.ToolHandler {
	type input struct {
		TraceID string `json:"trace_id"`
	}
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var in input
		if err := decodeArgs(req, &in); err != nil {
			return toolError(err.Error()), nil
		}
		if in.TraceID == "" {
			return toolError("trace_id is required"), nil
		}
		return jsonResult(service.GetTrace(ctx, in.TraceID)), nil
	}
}

func listIncidentsHandler(service WorkflowService) sdkmcp.ToolHandler {
	type input struct {
		Status   string `json:"status,omitempty"`
		Service  string `json:"service,omitempty"`
		Severity string `json:"severity,omitempty"`
		Limit    int    `json:"limit,omitempty"`
	}
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var in input
		if err := decodeOptionalArgs(req, &in); err != nil {
			return toolError(err.Error()), nil
		}
		result, err := service.ListIncidents(ctx, history.ListFilter{
			Status:   in.Status,
			Service:  in.Service,
			Severity: in.Severity,
			Limit:    in.Limit,
		})
		if err != nil {
			return toolError(err.Error()), nil
		}
		return jsonResult(result), nil
	}
}

func incidentHistoryHandler(service WorkflowService) sdkmcp.ToolHandler {
	type input struct {
		IncidentKey string `json:"incident_key"`
	}
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var in input
		if err := decodeArgs(req, &in); err != nil {
			return toolError(err.Error()), nil
		}
		if in.IncidentKey == "" {
			return toolError("incident_key is required"), nil
		}
		result, err := service.GetIncidentHistory(ctx, in.IncidentKey)
		if err != nil {
			return toolError(err.Error()), nil
		}
		return jsonResult(result), nil
	}
}

func addIncidentNoteHandler(service WorkflowService) sdkmcp.ToolHandler {
	type input struct {
		IncidentKey string `json:"incident_key"`
		Note        string `json:"note"`
		Status      string `json:"status,omitempty"`
	}
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var in input
		if err := decodeArgs(req, &in); err != nil {
			return toolError(err.Error()), nil
		}
		if in.IncidentKey == "" {
			return toolError("incident_key is required"), nil
		}
		if in.Note == "" {
			return toolError("note is required"), nil
		}
		if in.Status != "" && !history.ValidStatus(in.Status) {
			return toolError("status must be one of investigating, mitigated, resolved"), nil
		}
		result, err := service.AddIncidentNote(ctx, history.AddNoteInput{
			IncidentKey: in.IncidentKey,
			Note:        in.Note,
			Status:      in.Status,
		})
		if err != nil {
			return toolError(err.Error()), nil
		}
		return jsonResult(result), nil
	}
}

func compareEvidenceWindowsHandler(service WorkflowService) sdkmcp.ToolHandler {
	type input struct {
		BaselineSnapshotID  int64 `json:"baseline_snapshot_id"`
		CandidateSnapshotID int64 `json:"candidate_snapshot_id,omitempty"`
	}
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var in input
		if err := decodeArgs(req, &in); err != nil {
			return toolError(err.Error()), nil
		}
		if in.BaselineSnapshotID <= 0 {
			return toolError("baseline_snapshot_id is required"), nil
		}
		result, err := service.CompareEvidenceSnapshots(ctx, in.BaselineSnapshotID, in.CandidateSnapshotID)
		if err != nil {
			return toolError(err.Error()), nil
		}
		return jsonResult(result), nil
	}
}

func listManagementActionsHandler(service WorkflowService) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var in struct{}
		if err := decodeOptionalArgs(req, &in); err != nil {
			return toolError(err.Error()), nil
		}
		result, err := service.ListManagementActions(ctx)
		if err != nil {
			return toolError(err.Error()), nil
		}
		return jsonResult(result), nil
	}
}

func previewManagementActionHandler(service WorkflowService) sdkmcp.ToolHandler {
	return managementActionHandler(func(ctx context.Context, req management.ActionRequest) (management.ActionResult, error) {
		return service.PreviewManagementAction(ctx, req)
	})
}

func executeManagementActionHandler(service WorkflowService) sdkmcp.ToolHandler {
	return managementActionHandler(func(ctx context.Context, req management.ActionRequest) (management.ActionResult, error) {
		return service.ExecuteManagementAction(ctx, req)
	})
}

func managementActionHandler(fn func(context.Context, management.ActionRequest) (management.ActionResult, error)) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var in management.ActionRequest
		if err := decodeArgs(req, &in); err != nil {
			return toolError(err.Error()), nil
		}
		if in.ActionID == "" {
			return toolError("action_id is required"), nil
		}
		result, err := fn(ctx, in)
		if err != nil {
			return toolError(err.Error()), nil
		}
		return jsonResult(result), nil
	}
}

func managementActionHistoryHandler(service WorkflowService) sdkmcp.ToolHandler {
	type input struct {
		IncidentKey string `json:"incident_key,omitempty"`
		ActionID    string `json:"action_id,omitempty"`
		Status      string `json:"status,omitempty"`
		Decision    string `json:"decision,omitempty"`
		Limit       int    `json:"limit,omitempty"`
	}
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var in input
		if err := decodeOptionalArgs(req, &in); err != nil {
			return toolError(err.Error()), nil
		}
		if in.Limit > 100 {
			return toolError("limit must be <= 100"), nil
		}
		result, err := service.ListManagementActionHistory(ctx, history.ManagementActionFilter{
			IncidentKey: in.IncidentKey,
			ActionID:    in.ActionID,
			Status:      in.Status,
			Decision:    in.Decision,
			Limit:       in.Limit,
		})
		if err != nil {
			return toolError(err.Error()), nil
		}
		return jsonResult(result), nil
	}
}

func runbookResourceHandler(uri, name string) sdkmcp.ResourceHandler {
	return func(context.Context, *sdkmcp.ReadResourceRequest) (*sdkmcp.ReadResourceResult, error) {
		text, err := runbooks.Read(name)
		if err != nil {
			return nil, err
		}
		return &sdkmcp.ReadResourceResult{Contents: []*sdkmcp.ResourceContents{{URI: uri, MIMEType: "text/markdown", Text: text}}}, nil
	}
}

func decodeOptionalArgs(req *sdkmcp.CallToolRequest, out any) error {
	if req == nil || req.Params == nil || len(req.Params.Arguments) == 0 {
		return nil
	}
	return decodeArgs(req, out)
}

func decodeArgs(req *sdkmcp.CallToolRequest, out any) error {
	if req == nil || req.Params == nil || len(req.Params.Arguments) == 0 {
		return fmt.Errorf("arguments are required")
	}
	if err := json.Unmarshal(req.Params.Arguments, out); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	return nil
}

func jsonResult(result any) *sdkmcp.CallToolResult {
	data, err := json.Marshal(result)
	if err != nil {
		return toolError("result not serializable: " + err.Error())
	}
	return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: string(data)}}}
}

func toolError(message string) *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: message}}, IsError: true}
}

func windowSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"window":{"type":"string"},"incident_key":{"type":"string"},"incident_title":{"type":"string"},"severity":{"type":"string"},"service":{"type":"string"},"persist":{"type":"boolean"}},"additionalProperties":false}`)
}

func serviceEvidenceSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"window":{"type":"string"},"service":{"type":"string"},"trace_id":{"type":"string"},"incident_key":{"type":"string"},"incident_title":{"type":"string"},"severity":{"type":"string"},"persist":{"type":"boolean"}},"required":["service"],"additionalProperties":false}`)
}

func evalRunSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"window":{"type":"string"},"eval_id":{"type":"string"},"incident_key":{"type":"string"},"incident_title":{"type":"string"},"severity":{"type":"string"},"service":{"type":"string"},"persist":{"type":"boolean"}},"required":["eval_id"],"additionalProperties":false}`)
}

func searchLogsSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"window":{"type":"string"},"service":{"type":"string"},"pattern":{"type":"string"}},"required":["service"],"additionalProperties":false}`)
}

func traceSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"trace_id":{"type":"string"}},"required":["trace_id"],"additionalProperties":false}`)
}

func listIncidentsSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"status":{"type":"string","enum":["investigating","mitigated","resolved"]},"service":{"type":"string"},"severity":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":100}},"additionalProperties":false}`)
}

func incidentHistorySchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"incident_key":{"type":"string"}},"required":["incident_key"],"additionalProperties":false}`)
}

func addIncidentNoteSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"incident_key":{"type":"string"},"note":{"type":"string"},"status":{"type":"string","enum":["investigating","mitigated","resolved"]}},"required":["incident_key","note"],"additionalProperties":false}`)
}

func compareEvidenceWindowsSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"baseline_snapshot_id":{"type":"integer"},"candidate_snapshot_id":{"type":"integer"}},"required":["baseline_snapshot_id"],"additionalProperties":false}`)
}

func listManagementActionsSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
}

func managementActionSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"action_id":{"type":"string"},"args":{"type":"object"},"incident_key":{"type":"string"},"incident_title":{"type":"string"},"severity":{"type":"string"},"service":{"type":"string"}},"required":["action_id"],"additionalProperties":false}`)
}

func managementActionHistorySchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"incident_key":{"type":"string"},"action_id":{"type":"string"},"status":{"type":"string"},"decision":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":100}},"additionalProperties":false}`)
}
