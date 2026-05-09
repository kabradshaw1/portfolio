package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/observability-mcp-service/internal/config"
	"github.com/kabradshaw1/portfolio/go/observability-mcp-service/internal/workflows"
	"github.com/kabradshaw1/portfolio/go/observability-mcp-service/resources/runbooks"
)

type WorkflowService interface {
	GetSystemHealth(context.Context, time.Duration) workflows.EvidenceBundle
	InvestigateCheckout(context.Context, time.Duration) workflows.EvidenceBundle
	InvestigateAIPipeline(context.Context, time.Duration) workflows.EvidenceBundle
	InvestigateStreamingAnalytics(context.Context, time.Duration) workflows.EvidenceBundle
	GetServiceEvidence(context.Context, string, time.Duration, string) workflows.EvidenceBundle
	SearchLogs(context.Context, string, time.Duration, string) workflows.EvidenceBundle
	GetTrace(context.Context, string) workflows.EvidenceBundle
}

func New(service WorkflowService, cfg config.Config) *sdkmcp.Server {
	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "observability-mcp-service", Version: "0.1.0"}, nil)
	addTool(srv, "get_system_health", "Return a compact system-wide observability evidence bundle.", windowSchema(), windowHandler(cfg, service.GetSystemHealth))
	addTool(srv, "investigate_checkout", "Build checkout and saga failure evidence.", windowSchema(), windowHandler(cfg, service.InvestigateCheckout))
	addTool(srv, "investigate_ai_pipeline", "Build AI/RAG pipeline evidence.", windowSchema(), windowHandler(cfg, service.InvestigateAIPipeline))
	addTool(srv, "investigate_streaming_analytics", "Build Kafka and analytics evidence.", windowSchema(), windowHandler(cfg, service.InvestigateStreamingAnalytics))
	addTool(srv, "get_service_evidence", "Return bounded evidence for one allowlisted service.", serviceEvidenceSchema(), serviceEvidenceHandler(cfg, service))
	addTool(srv, "search_logs", "Search recent logs for one allowlisted service.", searchLogsSchema(), searchLogsHandler(cfg, service))
	addTool(srv, "get_trace", "Fetch and summarize a Jaeger trace.", traceSchema(), traceHandler(service))
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

type windowInput struct {
	Window string `json:"window,omitempty"`
}

func windowHandler(cfg config.Config, fn func(context.Context, time.Duration) workflows.EvidenceBundle) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var in windowInput
		if err := decodeOptionalArgs(req, &in); err != nil {
			return toolError(err.Error()), nil
		}
		window, err := cfg.WindowOrDefault(in.Window)
		if err != nil {
			return toolError(err.Error()), nil
		}
		return jsonResult(fn(ctx, window)), nil
	}
}

func serviceEvidenceHandler(cfg config.Config, service WorkflowService) sdkmcp.ToolHandler {
	type input struct {
		Window  string `json:"window,omitempty"`
		Service string `json:"service"`
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
		return jsonResult(service.GetServiceEvidence(ctx, in.Service, window, in.TraceID)), nil
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
	return json.RawMessage(`{"type":"object","properties":{"window":{"type":"string"}},"additionalProperties":false}`)
}

func serviceEvidenceSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"window":{"type":"string"},"service":{"type":"string"},"trace_id":{"type":"string"}},"required":["service"],"additionalProperties":false}`)
}

func searchLogsSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"window":{"type":"string"},"service":{"type":"string"},"pattern":{"type":"string"}},"required":["service"],"additionalProperties":false}`)
}

func traceSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"trace_id":{"type":"string"}},"required":["trace_id"],"additionalProperties":false}`)
}
