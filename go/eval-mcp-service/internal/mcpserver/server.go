package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/evalapi"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/evalworkflow"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/fixturecatalog"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/ingestionapi"
)

const workflowResourceURI = "eval://workflow"

const (
	minCompareInputs = 2
	maxCompareInputs = 5
	minRetrievalTopK = 1
	maxRetrievalTopK = 20
	maxWorstLimit    = 20
)

var answerAPIKeySecretPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,100}$`)
var answerTierPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,50}$`)

type answerModelArgs struct {
	Tier         string
	Provider     string
	BaseURL      string
	Model        string
	APIKeySecret string
}

type EvalService interface {
	StartExperiment(context.Context, evalworkflow.StartExperimentInput) (evalapi.Experiment, error)
	ListExperiments(context.Context) ([]evalapi.Experiment, error)
	GetExperiment(context.Context, string) (evalapi.Experiment, error)
	ListDatasets(context.Context) ([]evalapi.Dataset, error)
	ListDatasetFixtures(context.Context) ([]fixturecatalog.Fixture, error)
	CreateDatasetFromFixture(context.Context, string) (evalworkflow.CreateDatasetResult, error)
	ListRAGCollections(context.Context) ([]ingestionapi.Collection, error)
	GetRAGCollectionConfig(context.Context, string) (map[string]any, error)
	CheckReadiness(context.Context, evalworkflow.ReadinessInput) (evalapi.RAGReadinessResponse, error)
	StartRun(context.Context, evalworkflow.StartRunInput) (evalworkflow.StartRunResult, error)
	WaitForRun(context.Context, string) (evalworkflow.WaitResult, error)
	AttachRun(context.Context, string, string, string, string) error
	GetRun(context.Context, string) (evalapi.EvaluationDetail, error)
	RunEvidence(context.Context, string) (evalworkflow.RunEvidence, error)
	Compare(context.Context, evalworkflow.CompareInput) (evalapi.Comparison, error)
	WorstCases(context.Context, evalworkflow.WorstCasesInput) (evalworkflow.WorstCasesResult, error)
	TriageRAGRegression(context.Context, evalworkflow.TriageInput) (map[string]any, error)
	SummarizeExperiment(context.Context, string) (evalworkflow.ExperimentSummary, error)
	RecordConclusion(context.Context, evalworkflow.RecordConclusionInput) error
	ListEvalItemDLQ(context.Context, int) (evalapi.DLQListResponse, error)
	ReplayEvalItemDLQ(context.Context, evalapi.ReplayDLQItemRequest) (evalapi.ReplayDLQItemResponse, error)
}

func New(service EvalService) *sdkmcp.Server {
	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "eval-mcp-service", Version: "0.1.0"}, nil)
	addPrompt(srv, "eval", "Eval", "Start an agent-led RAG eval experiment.", evalPromptHandler())
	addResource(srv, workflowResourceURI, "Eval Workflow", "Workflow instructions for agent-led RAG eval experiments.", workflowResourceHandler())
	addTool(srv, "start_eval_experiment", "Start or define a local eval experiment session.", startEvalExperimentSchema(), startEvalExperimentHandler(service))
	addTool(srv, "list_eval_experiments", "List local eval experiment sessions.", emptySchema(), listEvalExperimentsHandler(service))
	addTool(srv, "get_eval_experiment", "Get one local eval experiment with run labels.", experimentIDSchema(), getEvalExperimentHandler(service))
	addTool(srv, "list_eval_datasets", "List datasets from the eval API.", emptySchema(), listEvalDatasetsHandler(service))
	addTool(srv, "list_eval_dataset_fixtures", "List curated eval dataset fixtures available in the repo.", emptySchema(), listEvalDatasetFixturesHandler(service))
	addTool(srv, "create_eval_dataset", "Create an eval API dataset from a curated repo fixture.", createEvalDatasetSchema(), createEvalDatasetHandler(service))
	addTool(srv, "list_rag_collections", "List Qdrant retrieval collections from ingestion.", emptySchema(), listRAGCollectionsHandler(service))
	addTool(srv, "get_rag_collection_config", "Fetch ingestion metadata for one RAG collection.", ragCollectionConfigSchema(), getRAGCollectionConfigHandler(service))
	addTool(srv, "check_rag_eval_readiness", "Check whether a dataset and retrieval collection are ready for a RAG eval run.", checkRAGReadinessSchema(), checkRAGReadinessHandler(service))
	addTool(srv, "start_eval_run", "Start an eval API run and optionally attach it to an experiment label.", startEvalRunSchema(), startEvalRunHandler(service))
	addTool(srv, "wait_for_eval_run", "Poll one eval run until completion, failure, or timeout.", waitEvalRunSchema(), waitForEvalRunHandler(service))
	addTool(srv, "attach_eval_run", "Attach an existing eval run ID to a local experiment label.", attachEvalRunSchema(), attachEvalRunHandler(service))
	addTool(srv, "get_eval_run", "Fetch one eval run from the eval API.", evalIDSchema(), getEvalRunHandler(service))
	addTool(srv, "get_eval_run_evidence", "Summarize one eval run with configuration, status, and next-step guidance.", evalIDSchema(), runEvidenceHandler(service))
	addTool(srv, "compare_eval_runs", "Compare eval runs by explicit IDs or experiment labels.", compareEvalRunsSchema(), compareEvalRunsHandler(service))
	addTool(srv, "get_worst_eval_cases", "Return the lowest-scoring per-query cases for a metric.", worstCasesSchema(), worstCasesHandler(service))
	addTool(srv, "list_eval_item_dlq", "List eval item DLQ messages without removing them.", listEvalItemDLQSchema(), listEvalItemDLQHandler(service))
	addTool(srv, "replay_eval_item_dlq", "Explicitly replay one selected eval item DLQ message. This is mutating.", replayEvalItemDLQSchema(), replayEvalItemDLQHandler(service))
	addTool(srv, "triage_rag_regression", "Run RAG regression triage for an eval run using eval results, worst cases, and optional observability evidence.", triageRAGRegressionSchema(), triageRAGRegressionHandler(service))
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

func listEvalDatasetFixturesHandler(service EvalService) sdkmcp.ToolHandler {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		result, err := service.ListDatasetFixtures(ctx)
		return resultOrError(result, err), nil
	}
}

func createEvalDatasetHandler(service EvalService) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var args struct {
			Fixture string `json:"fixture"`
		}
		if err := decodeArgs(req, &args); err != nil {
			return toolError(err.Error()), nil
		}
		if strings.TrimSpace(args.Fixture) == "" {
			return toolError("fixture is required"), nil
		}
		result, err := service.CreateDatasetFromFixture(ctx, args.Fixture)
		return resultOrError(result, err), nil
	}
}

func listRAGCollectionsHandler(service EvalService) sdkmcp.ToolHandler {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		result, err := service.ListRAGCollections(ctx)
		return resultOrError(result, err), nil
	}
}

func getRAGCollectionConfigHandler(service EvalService) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var args struct {
			Name string `json:"name"`
		}
		if err := decodeArgs(req, &args); err != nil {
			return toolError(err.Error()), nil
		}
		if strings.TrimSpace(args.Name) == "" {
			return toolError("name is required"), nil
		}
		result, err := service.GetRAGCollectionConfig(ctx, args.Name)
		return resultOrError(result, err), nil
	}
}

func checkRAGReadinessHandler(service EvalService) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var args struct {
			DatasetID       string          `json:"dataset_id"`
			Collection      string          `json:"collection"`
			Rerank          bool            `json:"rerank,omitempty"`
			RetrievalConfig json.RawMessage `json:"retrieval_config,omitempty"`
			BaselineEvalID  string          `json:"baseline_eval_id,omitempty"`
			ExperimentID    string          `json:"experiment_id,omitempty"`
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
		retrievalConfig, err := parseRetrievalConfig(args.RetrievalConfig)
		if err != nil {
			return toolError(err.Error()), nil
		}
		result, err := service.CheckReadiness(ctx, evalworkflow.ReadinessInput{
			DatasetID:       args.DatasetID,
			Collection:      args.Collection,
			Rerank:          args.Rerank,
			RetrievalConfig: retrievalConfig,
			BaselineEvalID:  args.BaselineEvalID,
			ExperimentID:    args.ExperimentID,
		})
		return resultOrError(result, err), nil
	}
}

func startEvalRunHandler(service EvalService) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var args struct {
			DatasetID          string          `json:"dataset_id"`
			Collection         string          `json:"collection"`
			Notes              string          `json:"notes,omitempty"`
			BaselineEvalID     string          `json:"baseline_eval_id,omitempty"`
			Rerank             bool            `json:"rerank,omitempty"`
			ExperimentID       string          `json:"experiment_id,omitempty"`
			Label              string          `json:"label,omitempty"`
			RetrievalConfig    json.RawMessage `json:"retrieval_config,omitempty"`
			AnswerTier         *string         `json:"answer_tier,omitempty"`
			AnswerProvider     *string         `json:"answer_provider,omitempty"`
			AnswerBaseURL      *string         `json:"answer_base_url,omitempty"`
			AnswerModel        *string         `json:"answer_model,omitempty"`
			AnswerAPIKeySecret *string         `json:"answer_api_key_secret,omitempty"`
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
		retrievalConfig, err := parseRetrievalConfig(args.RetrievalConfig)
		if err != nil {
			return toolError(err.Error()), nil
		}
		answerOverride, err := validateAnswerModelArgs(args.AnswerTier, args.AnswerProvider, args.AnswerBaseURL, args.AnswerModel, args.AnswerAPIKeySecret)
		if err != nil {
			return toolError(err.Error()), nil
		}
		in := evalworkflow.StartRunInput{
			DatasetID:          args.DatasetID,
			Collection:         args.Collection,
			Notes:              args.Notes,
			BaselineEvalID:     args.BaselineEvalID,
			Rerank:             args.Rerank,
			ExperimentID:       args.ExperimentID,
			Label:              args.Label,
			RetrievalConfig:    retrievalConfig,
			AnswerTier:         answerOverride.Tier,
			AnswerProvider:     answerOverride.Provider,
			AnswerBaseURL:      answerOverride.BaseURL,
			AnswerModel:        answerOverride.Model,
			AnswerAPIKeySecret: answerOverride.APIKeySecret,
		}
		result, err := service.StartRun(ctx, in)
		return resultOrError(result, err), nil
	}
}

func validateAnswerModelArgs(tier, provider, baseURL, model, secret *string) (answerModelArgs, error) {
	if tier == nil && provider == nil && baseURL == nil && model == nil && secret == nil {
		return answerModelArgs{}, nil
	}
	for _, field := range []struct {
		name  string
		value *string
	}{
		{name: "answer_tier", value: tier},
		{name: "answer_provider", value: provider},
		{name: "answer_base_url", value: baseURL},
		{name: "answer_model", value: model},
		{name: "answer_api_key_secret", value: secret},
	} {
		if hasPadding(field.value) {
			return answerModelArgs{}, fmt.Errorf("%s must not include leading or trailing whitespace", field.name)
		}
	}
	normalized := answerModelArgs{
		Tier:         trimStringPtr(tier),
		Provider:     trimStringPtr(provider),
		BaseURL:      trimStringPtr(baseURL),
		Model:        trimStringPtr(model),
		APIKeySecret: trimStringPtr(secret),
	}
	if normalized.Provider == "" {
		return answerModelArgs{}, fmt.Errorf("answer_provider is required with answer override")
	}
	if normalized.Provider != "ollama" && normalized.Provider != "openai" && normalized.Provider != "anthropic" {
		return answerModelArgs{}, fmt.Errorf("answer_provider must be ollama, openai, or anthropic")
	}
	if normalized.Model == "" {
		return answerModelArgs{}, fmt.Errorf("answer_model is required with answer override")
	}
	if normalized.Tier != "" && !answerTierPattern.MatchString(normalized.Tier) {
		return answerModelArgs{}, fmt.Errorf("answer_tier must match ^[a-zA-Z0-9_-]{1,50}$")
	}
	if baseURL != nil && normalized.BaseURL == "" {
		return answerModelArgs{}, fmt.Errorf("answer_base_url must not be empty when provided")
	}
	if len(normalized.Model) > 100 {
		return answerModelArgs{}, fmt.Errorf("answer_model must be at most 100 characters")
	}
	if len(normalized.BaseURL) > 300 {
		return answerModelArgs{}, fmt.Errorf("answer_base_url must be at most 300 characters")
	}
	if (normalized.Provider == "openai" || normalized.Provider == "anthropic") && normalized.APIKeySecret == "" {
		return answerModelArgs{}, fmt.Errorf("answer_api_key_secret is required when answer_provider is %s", normalized.Provider)
	}
	lowered := strings.ToLower(normalized.APIKeySecret)
	if strings.HasPrefix(lowered, "sk-") || strings.HasPrefix(lowered, "sk_") || strings.HasPrefix(lowered, "bearer ") || strings.HasPrefix(lowered, "api-") {
		return answerModelArgs{}, fmt.Errorf("answer_api_key_secret must be an environment variable name")
	}
	if secret != nil && !answerAPIKeySecretPattern.MatchString(normalized.APIKeySecret) {
		return answerModelArgs{}, fmt.Errorf("answer_api_key_secret must be an environment variable name matching ^[A-Z][A-Z0-9_]{1,100}$")
	}
	return normalized, nil
}

func trimStringPtr(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func hasPadding(value *string) bool {
	return value != nil && *value != strings.TrimSpace(*value)
}

func parseRetrievalConfig(raw json.RawMessage) (*evalapi.RetrievalConfig, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("retrieval_config must be an object")
	}
	for field := range fields {
		if field != "top_k" {
			return nil, fmt.Errorf("retrieval_config.%s is not supported", field)
		}
	}

	var config evalapi.RetrievalConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return nil, fmt.Errorf("invalid retrieval_config: %w", err)
	}
	if config.TopK != nil {
		topK := *config.TopK
		if topK < minRetrievalTopK || topK > maxRetrievalTopK {
			return nil, fmt.Errorf("retrieval_config.top_k must be between 1 and 20")
		}
	}
	return &config, nil
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

func runEvidenceHandler(service EvalService) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		evalID, err := evalIDFromRequest(req)
		if err != nil {
			return toolError(err.Error()), nil
		}
		result, err := service.RunEvidence(ctx, evalID)
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

func listEvalItemDLQHandler(service EvalService) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var args struct {
			Limit int `json:"limit,omitempty"`
		}
		if err := decodeArgs(req, &args); err != nil {
			return toolError(err.Error()), nil
		}
		result, err := service.ListEvalItemDLQ(ctx, args.Limit)
		return resultOrError(result, err), nil
	}
}

func replayEvalItemDLQHandler(service EvalService) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var args struct {
			ItemID string `json:"item_id,omitempty"`
			Index  *int   `json:"index,omitempty"`
		}
		if err := decodeArgs(req, &args); err != nil {
			return toolError(err.Error()), nil
		}
		hasItemID := strings.TrimSpace(args.ItemID) != ""
		hasIndex := args.Index != nil
		if hasItemID == hasIndex {
			return toolError("provide exactly one of item_id or index"), nil
		}
		result, err := service.ReplayEvalItemDLQ(ctx, evalapi.ReplayDLQItemRequest{
			ItemID: strings.TrimSpace(args.ItemID),
			Index:  args.Index,
		})
		return resultOrError(result, err), nil
	}
}

func triageRAGRegressionHandler(service EvalService) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var args struct {
			EvalID               string `json:"eval_id"`
			BaselineEvalID       string `json:"baseline_eval_id,omitempty"`
			Metric               string `json:"metric,omitempty"`
			Limit                *int   `json:"limit,omitempty"`
			IncludeObservability bool   `json:"include_observability,omitempty"`
		}
		if err := decodeArgs(req, &args); err != nil {
			return toolError(err.Error()), nil
		}
		if strings.TrimSpace(args.EvalID) == "" {
			return toolError("eval_id is required"), nil
		}
		limit := 0
		if args.Limit != nil {
			limit = *args.Limit
			if limit < 1 || limit > maxWorstLimit {
				return toolError("limit must be between 1 and 20 when provided"), nil
			}
		}
		in := evalworkflow.TriageInput{
			EvalID:               args.EvalID,
			BaselineEvalID:       args.BaselineEvalID,
			Metric:               args.Metric,
			Limit:                limit,
			IncludeObservability: args.IncludeObservability,
		}
		result, err := service.TriageRAGRegression(ctx, in)
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
				Content: &sdkmcp.TextContent{Text: "Start or resume a RAG eval experiment. Datasets are golden questions and expected answers. Collections are Qdrant retrieval corpora. Never infer a collection from a dataset name. Use list_eval_dataset_fixtures and create_eval_dataset for curated repo fixtures, list_eval_datasets to choose existing API data, then call check_rag_eval_readiness before start_eval_experiment or start_eval_run. Treat blocked readiness as a stop condition and warning readiness as a caveated run condition. Run baseline to completion with wait_for_eval_run before starting rerank while runtime hardening is pending. For model ladder experiments, keep the judge model fixed and vary answer_tier, answer_provider, answer_model, retrieval_config, and rerank. Start one run at a time, wait for completion, compare completed or completed_with_failures runs, inspect worst cases, and record a conclusion only after the user approves it. Compare completed or partial runs with compare_eval_runs, inspect failures with get_worst_eval_cases, summarize_eval_experiment for the final evidence packet, and record_eval_experiment_conclusion once the user approves the conclusion. For eval item runtime failures, use list_eval_item_dlq to inspect safe DLQ metadata; call replay_eval_item_dlq only after operator approval because it is mutating and requires operator credentials."},
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
2. Datasets are golden questions and expected answers. Collections are Qdrant retrieval corpora. Never infer a collection from a dataset name.
3. Use list_eval_dataset_fixtures and create_eval_dataset for curated repo fixtures, or call list_eval_datasets before choosing an existing dataset unless the user already named a dataset ID.
4. Call check_rag_eval_readiness before start_eval_experiment or start_eval_run. Treat blocked readiness as a stop condition and warning readiness as a caveated run condition.
5. Start baseline with start_eval_run only after readiness is ready or warning, then call wait_for_eval_run. Run baseline to completion before starting rerank while runtime hardening is pending.
6. Start candidate runs with start_eval_run only after readiness is ready or warning, then call wait_for_eval_run until each run completes or fails.
7. For model ladder experiments, keep the judge model fixed and vary answer_tier, answer_provider, answer_model, retrieval_config, and rerank. Start one run at a time, wait for completion, compare completed or completed_with_failures runs, inspect worst cases, and record a conclusion only after the user approves it.
8. Attach externally created runs with attach_eval_run when needed.
9. Use get_eval_run for individual run inspection, compare_eval_runs for metric deltas, and get_worst_eval_cases for the lowest-scoring per-query cases. Compare completed or completed_with_failures runs.
10. For eval item runtime failures, use list_eval_item_dlq to inspect safe DLQ metadata. Only call replay_eval_item_dlq after operator approval because it is mutating and requires operator credentials.
11. Call summarize_eval_experiment before presenting a recommendation.
12. Only call record_eval_experiment_conclusion after the user approves the conclusion.
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
	return json.RawMessage(`{"type":"object","properties":{"dataset_id":{"type":"string"},"collection":{"type":"string"},"notes":{"type":"string"},"baseline_eval_id":{"type":"string"},"rerank":{"type":"boolean"},"experiment_id":{"type":"string","minLength":1},"label":{"type":"string"},"retrieval_config":{"type":"object","properties":{"top_k":{"type":"integer","minimum":1,"maximum":20}},"additionalProperties":false},"answer_tier":{"type":"string","pattern":"^[a-zA-Z0-9_-]{1,50}$","maxLength":50},"answer_provider":{"type":"string","enum":["ollama","openai","anthropic"]},"answer_base_url":{"type":"string","pattern":"^\\S(?:.*\\S)?$","minLength":1,"maxLength":300},"answer_model":{"type":"string","pattern":"^\\S(?:.*\\S)?$","minLength":1,"maxLength":100},"answer_api_key_secret":{"type":"string","pattern":"^[A-Z][A-Z0-9_]{1,100}$","minLength":2,"maxLength":101}},"allOf":[{"if":{"anyOf":[{"required":["answer_tier"]},{"required":["answer_provider"]},{"required":["answer_base_url"]},{"required":["answer_model"]},{"required":["answer_api_key_secret"]}]},"then":{"required":["answer_provider","answer_model"]}},{"if":{"properties":{"answer_provider":{"const":"openai"}},"required":["answer_provider"]},"then":{"required":["answer_api_key_secret"]}},{"if":{"properties":{"answer_provider":{"const":"anthropic"}},"required":["answer_provider"]},"then":{"required":["answer_api_key_secret"]}}],"required":["dataset_id","collection"],"additionalProperties":false}`)
}

func checkRAGReadinessSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"dataset_id":{"type":"string"},"collection":{"type":"string"},"rerank":{"type":"boolean"},"retrieval_config":{"type":"object","properties":{"top_k":{"type":"integer","minimum":1,"maximum":20}},"additionalProperties":false},"baseline_eval_id":{"type":"string"},"experiment_id":{"type":"string"}},"required":["dataset_id","collection"],"additionalProperties":false}`)
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

func createEvalDatasetSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"fixture":{"type":"string","minLength":1}},"required":["fixture"],"additionalProperties":false}`)
}

func ragCollectionConfigSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"name":{"type":"string","minLength":1}},"required":["name"],"additionalProperties":false}`)
}

func compareEvalRunsSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","description":"Compare 2 to 5 total runs supplied as explicit eval_ids plus optional experiment labels.","properties":{"eval_ids":{"type":"array","description":"Explicit eval run IDs. eval_ids plus labels must total 2 to 5.","items":{"type":"string"},"minItems":1,"maxItems":5},"experiment_id":{"type":"string","minLength":1,"description":"Eval API experiment ID used to resolve labels."},"labels":{"type":"array","description":"Experiment run labels. eval_ids plus labels must total 2 to 5.","items":{"type":"string"},"minItems":1,"maxItems":5}},"additionalProperties":false}`)
}

func worstCasesSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"eval_id":{"type":"string"},"metric":{"type":"string","enum":["faithfulness","answer_relevancy","context_precision","context_recall"]},"limit":{"type":"integer","description":"Number of worst cases to return; omitted or non-positive values default to 5, and values over 20 are capped."}},"required":["eval_id"],"additionalProperties":false}`)
}

func listEvalItemDLQSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"limit":{"type":"integer","minimum":1,"maximum":200}},"additionalProperties":false}`)
}

func replayEvalItemDLQSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"item_id":{"type":"string","minLength":1},"index":{"type":"integer","minimum":0}},"additionalProperties":false}`)
}

func triageRAGRegressionSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"eval_id":{"type":"string"},"baseline_eval_id":{"type":"string"},"metric":{"type":"string","enum":["faithfulness","answer_relevancy","context_precision","context_recall"]},"limit":{"type":"integer","minimum":1,"maximum":20},"include_observability":{"type":"boolean"}},"required":["eval_id"],"additionalProperties":false}`)
}

func recordConclusionSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"experiment_id":{"type":"string","minLength":1},"decision":{"type":"string","enum":["keep","revert","needs_more_data"]},"conclusion":{"type":"string"},"evidence":{"type":"object"}},"required":["experiment_id","decision","conclusion","evidence"],"additionalProperties":false}`)
}
