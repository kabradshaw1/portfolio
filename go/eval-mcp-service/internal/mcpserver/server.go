package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/evalapi"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/evalworkflow"
)

const workflowResourceURI = "eval://workflow"

const (
	minCompareInputs = 2
	maxCompareInputs = 5
)

type EvalService interface {
	StartExperiment(context.Context, evalworkflow.StartExperimentInput) (evalapi.Experiment, error)
	ListExperiments(context.Context) ([]evalapi.Experiment, error)
	GetExperiment(context.Context, string) (evalapi.Experiment, error)
	ListDatasets(context.Context) ([]evalapi.Dataset, error)
	StartRun(context.Context, evalworkflow.StartRunInput) (evalworkflow.StartRunResult, error)
	WaitForRun(context.Context, string) (evalworkflow.WaitResult, error)
	AttachRun(context.Context, string, string, string, string) error
	GetRun(context.Context, string) (evalapi.EvaluationDetail, error)
	Compare(context.Context, evalworkflow.CompareInput) (evalapi.Comparison, error)
	WorstCases(context.Context, evalworkflow.WorstCasesInput) (evalworkflow.WorstCasesResult, error)
	SummarizeExperiment(context.Context, string) (evalworkflow.ExperimentSummary, error)
	RecordConclusion(context.Context, evalworkflow.RecordConclusionInput) error
}

func New(service EvalService) *sdkmcp.Server {
	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "eval-mcp-service", Version: "0.1.0"}, nil)
	addPrompt(srv, "eval", "Eval", "Start an agent-led RAG eval experiment.", evalPromptHandler())
	addResource(srv, workflowResourceURI, "Eval Workflow", "Workflow instructions for agent-led RAG eval experiments.", workflowResourceHandler())
	addTool(srv, "start_eval_experiment", "Start or define a local eval experiment session.", startEvalExperimentSchema(), startEvalExperimentHandler(service))
	addTool(srv, "list_eval_experiments", "List local eval experiment sessions.", emptySchema(), listEvalExperimentsHandler(service))
	addTool(srv, "get_eval_experiment", "Get one local eval experiment with run labels.", experimentIDSchema(), getEvalExperimentHandler(service))
	addTool(srv, "list_eval_datasets", "List datasets from the eval API.", emptySchema(), listEvalDatasetsHandler(service))
	addTool(srv, "start_eval_run", "Start an eval API run and optionally attach it to an experiment label.", startEvalRunSchema(), startEvalRunHandler(service))
	addTool(srv, "wait_for_eval_run", "Poll one eval run until completion, failure, or timeout.", waitEvalRunSchema(), waitForEvalRunHandler(service))
	addTool(srv, "attach_eval_run", "Attach an existing eval run ID to a local experiment label.", attachEvalRunSchema(), attachEvalRunHandler(service))
	addTool(srv, "get_eval_run", "Fetch one eval run from the eval API.", evalIDSchema(), getEvalRunHandler(service))
	addTool(srv, "compare_eval_runs", "Compare eval runs by explicit IDs or experiment labels.", compareEvalRunsSchema(), compareEvalRunsHandler(service))
	addTool(srv, "get_worst_eval_cases", "Return the lowest-scoring per-query cases for a metric.", worstCasesSchema(), worstCasesHandler(service))
	addTool(srv, "summarize_eval_experiment", "Summarize baseline, candidates, and worst cases for an experiment.", experimentIDSchema(), summarizeExperimentHandler(service))
	addTool(srv, "record_eval_experiment_conclusion", "Record the approved conclusion for a local eval experiment.", recordConclusionSchema(), recordConclusionHandler(service))
	return srv
}

func addPrompt(srv *sdkmcp.Server, name, title, description string, handler sdkmcp.PromptHandler) {
	srv.AddPrompt(&sdkmcp.Prompt{Name: name, Title: title, Description: description}, handler)
}

func addResource(srv *sdkmcp.Server, uri, name, description string, handler sdkmcp.ResourceHandler) {
	srv.AddResource(&sdkmcp.Resource{URI: uri, Name: name, Title: name, Description: description, MIMEType: "text/markdown"}, handler)
}

func addTool(srv *sdkmcp.Server, name, description string, schema json.RawMessage, handler sdkmcp.ToolHandler) {
	srv.AddTool(&sdkmcp.Tool{Name: name, Description: description, InputSchema: schema}, handler)
}

func startEvalExperimentHandler(service EvalService) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var args struct {
			Name           string `json:"name"`
			DatasetID      string `json:"dataset_id"`
			Collection     string `json:"collection,omitempty"`
			BaselineEvalID string `json:"baseline_eval_id,omitempty"`
			FocusMetric    string `json:"focus_metric,omitempty"`
			Hypothesis     string `json:"hypothesis,omitempty"`
			Notes          string `json:"notes,omitempty"`
		}
		if err := decodeArgs(req, &args); err != nil {
			return toolError(err.Error()), nil
		}
		if strings.TrimSpace(args.Name) == "" {
			return toolError("name is required"), nil
		}
		if strings.TrimSpace(args.DatasetID) == "" {
			return toolError("dataset_id is required"), nil
		}
		in := evalworkflow.StartExperimentInput{
			Name:           args.Name,
			DatasetID:      args.DatasetID,
			Collection:     args.Collection,
			BaselineEvalID: args.BaselineEvalID,
			FocusMetric:    args.FocusMetric,
			Hypothesis:     args.Hypothesis,
			Notes:          args.Notes,
		}
		result, err := service.StartExperiment(ctx, in)
		return resultOrError(result, err), nil
	}
}

func listEvalExperimentsHandler(service EvalService) sdkmcp.ToolHandler {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		result, err := service.ListExperiments(ctx)
		return resultOrError(result, err), nil
	}
}

func getEvalExperimentHandler(service EvalService) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var in struct {
			ExperimentID string `json:"experiment_id"`
		}
		if err := decodeArgs(req, &in); err != nil {
			return toolError(err.Error()), nil
		}
		if strings.TrimSpace(in.ExperimentID) == "" {
			return toolError("experiment_id is required"), nil
		}
		result, err := service.GetExperiment(ctx, in.ExperimentID)
		return resultOrError(result, err), nil
	}
}

func listEvalDatasetsHandler(service EvalService) sdkmcp.ToolHandler {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		result, err := service.ListDatasets(ctx)
		return resultOrError(result, err), nil
	}
}

func startEvalRunHandler(service EvalService) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var args struct {
			DatasetID      string `json:"dataset_id"`
			Collection     string `json:"collection"`
			Notes          string `json:"notes,omitempty"`
			BaselineEvalID string `json:"baseline_eval_id,omitempty"`
			Rerank         bool   `json:"rerank,omitempty"`
			ExperimentID   string `json:"experiment_id,omitempty"`
			Label          string `json:"label,omitempty"`
		}
		if err := decodeArgs(req, &args); err != nil {
			return toolError(err.Error()), nil
		}
		if strings.TrimSpace(args.DatasetID) == "" {
			return toolError("dataset_id is required"), nil
		}
		if strings.TrimSpace(args.Collection) == "" {
			return toolError("collection is required"), nil
		}
		if strings.TrimSpace(args.Label) != "" && strings.TrimSpace(args.ExperimentID) == "" {
			return toolError("experiment_id is required when label is provided"), nil
		}
		if strings.TrimSpace(args.ExperimentID) != "" && strings.TrimSpace(args.Label) == "" {
			return toolError("label is required when experiment_id is set"), nil
		}
		in := evalworkflow.StartRunInput{
			DatasetID:      args.DatasetID,
			Collection:     args.Collection,
			Notes:          args.Notes,
			BaselineEvalID: args.BaselineEvalID,
			Rerank:         args.Rerank,
			ExperimentID:   args.ExperimentID,
			Label:          args.Label,
		}
		result, err := service.StartRun(ctx, in)
		return resultOrError(result, err), nil
	}
}

func waitForEvalRunHandler(service EvalService) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		evalID, err := evalIDFromRequest(req)
		if err != nil {
			return toolError(err.Error()), nil
		}
		result, err := service.WaitForRun(ctx, evalID)
		return resultOrError(result, err), nil
	}
}

func attachEvalRunHandler(service EvalService) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var in struct {
			ExperimentID string `json:"experiment_id"`
			Label        string `json:"label"`
			EvalID       string `json:"eval_id"`
			Notes        string `json:"notes,omitempty"`
		}
		if err := decodeArgs(req, &in); err != nil {
			return toolError(err.Error()), nil
		}
		if strings.TrimSpace(in.ExperimentID) == "" {
			return toolError("experiment_id is required"), nil
		}
		if strings.TrimSpace(in.Label) == "" {
			return toolError("label is required"), nil
		}
		if strings.TrimSpace(in.EvalID) == "" {
			return toolError("eval_id is required"), nil
		}
		if err := service.AttachRun(ctx, in.ExperimentID, in.Label, in.EvalID, in.Notes); err != nil {
			return toolError(err.Error()), nil
		}
		return jsonResult(map[string]bool{"ok": true}), nil
	}
}

func getEvalRunHandler(service EvalService) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		evalID, err := evalIDFromRequest(req)
		if err != nil {
			return toolError(err.Error()), nil
		}
		result, err := service.GetRun(ctx, evalID)
		return resultOrError(result, err), nil
	}
}

func compareEvalRunsHandler(service EvalService) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var args struct {
			EvalIDs      []string `json:"eval_ids,omitempty"`
			ExperimentID string   `json:"experiment_id,omitempty"`
			Labels       []string `json:"labels,omitempty"`
		}
		if err := decodeArgs(req, &args); err != nil {
			return toolError(err.Error()), nil
		}
		if len(args.EvalIDs) == 0 && (strings.TrimSpace(args.ExperimentID) == "" || len(args.Labels) == 0) {
			return toolError("eval_ids or experiment_id with labels is required"), nil
		}
		if len(args.Labels) > 0 && strings.TrimSpace(args.ExperimentID) == "" {
			return toolError("experiment_id is required when labels are provided"), nil
		}
		totalInputs := len(args.EvalIDs) + len(args.Labels)
		if totalInputs < minCompareInputs || totalInputs > maxCompareInputs {
			return toolError(fmt.Sprintf("compare requires %d to %d total eval_ids and labels, got %d", minCompareInputs, maxCompareInputs, totalInputs)), nil
		}
		for _, evalID := range args.EvalIDs {
			if strings.TrimSpace(evalID) == "" {
				return toolError("eval_ids must not contain empty values"), nil
			}
		}
		for _, label := range args.Labels {
			if strings.TrimSpace(label) == "" {
				return toolError("labels must not contain empty values"), nil
			}
		}
		in := evalworkflow.CompareInput{
			EvalIDs:      args.EvalIDs,
			ExperimentID: args.ExperimentID,
			Labels:       args.Labels,
		}
		result, err := service.Compare(ctx, in)
		return resultOrError(result, err), nil
	}
}

func worstCasesHandler(service EvalService) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var args struct {
			EvalID string `json:"eval_id"`
			Metric string `json:"metric,omitempty"`
			Limit  int    `json:"limit,omitempty"`
		}
		if err := decodeArgs(req, &args); err != nil {
			return toolError(err.Error()), nil
		}
		if strings.TrimSpace(args.EvalID) == "" {
			return toolError("eval_id is required"), nil
		}
		in := evalworkflow.WorstCasesInput{
			EvalID: args.EvalID,
			Metric: args.Metric,
			Limit:  args.Limit,
		}
		result, err := service.WorstCases(ctx, in)
		return resultOrError(result, err), nil
	}
}

func summarizeExperimentHandler(service EvalService) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var in struct {
			ExperimentID string `json:"experiment_id"`
		}
		if err := decodeArgs(req, &in); err != nil {
			return toolError(err.Error()), nil
		}
		if strings.TrimSpace(in.ExperimentID) == "" {
			return toolError("experiment_id is required"), nil
		}
		result, err := service.SummarizeExperiment(ctx, in.ExperimentID)
		return resultOrError(result, err), nil
	}
}

func recordConclusionHandler(service EvalService) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var in struct {
			ExperimentID string         `json:"experiment_id"`
			Decision     string         `json:"decision"`
			Conclusion   string         `json:"conclusion"`
			Evidence     map[string]any `json:"evidence"`
		}
		if err := decodeArgs(req, &in); err != nil {
			return toolError(err.Error()), nil
		}
		if strings.TrimSpace(in.ExperimentID) == "" {
			return toolError("experiment_id is required"), nil
		}
		if strings.TrimSpace(in.Decision) == "" {
			return toolError("decision is required"), nil
		}
		if strings.TrimSpace(in.Conclusion) == "" {
			return toolError("conclusion is required"), nil
		}
		if in.Evidence == nil {
			return toolError("evidence is required"), nil
		}
		if err := service.RecordConclusion(ctx, evalworkflow.RecordConclusionInput{
			ExperimentID: in.ExperimentID,
			Decision:     in.Decision,
			Conclusion:   in.Conclusion,
			Evidence:     in.Evidence,
		}); err != nil {
			return toolError(err.Error()), nil
		}
		return jsonResult(map[string]bool{"ok": true}), nil
	}
}

func evalPromptHandler() sdkmcp.PromptHandler {
	return func(context.Context, *sdkmcp.GetPromptRequest) (*sdkmcp.GetPromptResult, error) {
		return &sdkmcp.GetPromptResult{
			Description: "Start an agent-led RAG eval experiment.",
			Messages: []*sdkmcp.PromptMessage{{
				Role:    sdkmcp.Role("user"),
				Content: &sdkmcp.TextContent{Text: "Start or resume a RAG eval experiment. Use start_eval_experiment for local experiment state, list_eval_datasets to choose data, start_eval_run and wait_for_eval_run for baseline and candidate runs, compare_eval_runs to measure deltas, get_worst_eval_cases to inspect failures, summarize_eval_experiment for the final evidence packet, and record_eval_experiment_conclusion once the user approves the conclusion."},
			}},
		}, nil
	}
}

func workflowResourceHandler() sdkmcp.ResourceHandler {
	return func(context.Context, *sdkmcp.ReadResourceRequest) (*sdkmcp.ReadResourceResult, error) {
		return &sdkmcp.ReadResourceResult{
			Contents: []*sdkmcp.ResourceContents{{
				URI:      workflowResourceURI,
				MIMEType: "text/markdown",
				Text:     evalWorkflowInstructions(),
			}},
		}, nil
	}
}

func evalWorkflowInstructions() string {
	return strings.TrimSpace(`
# Eval Workflow

Use this local MCP server to keep RAG eval experiments explicit and reproducible.

1. Start or resume local experiment state with start_eval_experiment, list_eval_experiments, or get_eval_experiment.
2. Call list_eval_datasets before choosing a dataset unless the user already named a dataset ID.
3. Start baseline and candidate runs with start_eval_run, then call wait_for_eval_run until each run completes or fails.
4. Attach externally created runs with attach_eval_run when needed.
5. Use get_eval_run for individual run inspection, compare_eval_runs for metric deltas, and get_worst_eval_cases for the lowest-scoring per-query cases.
6. Call summarize_eval_experiment before presenting a recommendation.
7. Only call record_eval_experiment_conclusion after the user approves the conclusion.
`)
}

func evalIDFromRequest(req *sdkmcp.CallToolRequest) (string, error) {
	var in struct {
		EvalID string `json:"eval_id"`
	}
	if err := decodeArgs(req, &in); err != nil {
		return "", err
	}
	evalID := strings.TrimSpace(in.EvalID)
	if evalID == "" {
		return "", errors.New("eval_id is required")
	}
	return evalID, nil
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

func resultOrError(result any, err error) *sdkmcp.CallToolResult {
	if err != nil {
		return toolError(err.Error())
	}
	return jsonResult(result)
}

func jsonResult(result any) *sdkmcp.CallToolResult {
	data, err := json.Marshal(result)
	if err != nil {
		return toolError("result not serializable: " + err.Error())
	}
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: string(data)}},
	}
}

func toolError(message string) *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: message}},
		IsError: true,
	}
}

func emptySchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","additionalProperties":false}`)
}

func startEvalExperimentSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"dataset_id":{"type":"string"},"collection":{"type":"string"},"baseline_eval_id":{"type":"string"},"focus_metric":{"type":"string","enum":["faithfulness","answer_relevancy","context_precision","context_recall"]},"hypothesis":{"type":"string"},"notes":{"type":"string"}},"required":["name","dataset_id"],"additionalProperties":false}`)
}

func experimentIDSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"experiment_id":{"type":"string","minLength":1}},"required":["experiment_id"],"additionalProperties":false}`)
}

func startEvalRunSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"dataset_id":{"type":"string"},"collection":{"type":"string"},"notes":{"type":"string"},"baseline_eval_id":{"type":"string"},"rerank":{"type":"boolean"},"experiment_id":{"type":"string","minLength":1},"label":{"type":"string"}},"required":["dataset_id","collection"],"additionalProperties":false}`)
}

func waitEvalRunSchema() json.RawMessage {
	return evalIDSchema()
}

func attachEvalRunSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"experiment_id":{"type":"string","minLength":1},"label":{"type":"string"},"eval_id":{"type":"string"},"notes":{"type":"string"}},"required":["experiment_id","label","eval_id"],"additionalProperties":false}`)
}

func evalIDSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"eval_id":{"type":"string"}},"required":["eval_id"],"additionalProperties":false}`)
}

func compareEvalRunsSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","description":"Compare 2 to 5 total runs supplied as explicit eval_ids plus optional experiment labels.","properties":{"eval_ids":{"type":"array","description":"Explicit eval run IDs. eval_ids plus labels must total 2 to 5.","items":{"type":"string"},"minItems":1,"maxItems":5},"experiment_id":{"type":"string","minLength":1,"description":"Eval API experiment ID used to resolve labels."},"labels":{"type":"array","description":"Experiment run labels. eval_ids plus labels must total 2 to 5.","items":{"type":"string"},"minItems":1,"maxItems":5}},"additionalProperties":false}`)
}

func worstCasesSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"eval_id":{"type":"string"},"metric":{"type":"string","enum":["faithfulness","answer_relevancy","context_precision","context_recall"]},"limit":{"type":"integer","description":"Number of worst cases to return; omitted or non-positive values default to 5, and values over 20 are capped."}},"required":["eval_id"],"additionalProperties":false}`)
}

func recordConclusionSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"experiment_id":{"type":"string","minLength":1},"decision":{"type":"string","enum":["keep","revert","needs_more_data"]},"conclusion":{"type":"string"},"evidence":{"type":"object"}},"required":["experiment_id","decision","conclusion","evidence"],"additionalProperties":false}`)
}
