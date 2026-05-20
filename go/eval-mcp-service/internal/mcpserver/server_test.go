package mcpserver

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/evalapi"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/evalworkflow"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/fixturecatalog"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/ingestionapi"
)

type fakeEvalService struct {
	startExperimentInput  evalworkflow.StartExperimentInput
	startRunInput         evalworkflow.StartRunInput
	worstCasesInput       evalworkflow.WorstCasesInput
	triageInput           evalworkflow.TriageInput
	startRunCalls         int
	compareCalls          int
	triageCalls           int
	compareInput          evalworkflow.CompareInput
	recordConclusionInput evalworkflow.RecordConclusionInput
}

func (f *fakeEvalService) StartExperiment(_ context.Context, in evalworkflow.StartExperimentInput) (evalapi.Experiment, error) {
	f.startExperimentInput = in
	notes := in.Notes
	return evalapi.Experiment{
		ID:          "exp-11",
		Name:        in.Name,
		DatasetID:   in.DatasetID,
		Collection:  in.Collection,
		FocusMetric: in.FocusMetric,
		Hypothesis:  in.Hypothesis,
		Notes:       &notes,
		CreatedAt:   "2026-05-13T12:00:00Z",
		UpdatedAt:   "2026-05-13T12:00:00Z",
	}, nil
}

func (f *fakeEvalService) ListExperiments(context.Context) ([]evalapi.Experiment, error) {
	return []evalapi.Experiment{{ID: "exp-11", Name: "baseline-vs-rerank", DatasetID: "ds-1"}}, nil
}

func (f *fakeEvalService) GetExperiment(context.Context, string) (evalapi.Experiment, error) {
	return evalapi.Experiment{ID: "exp-11", Name: "baseline-vs-rerank", DatasetID: "ds-1"}, nil
}

func (f *fakeEvalService) ListDatasets(context.Context) ([]evalapi.Dataset, error) {
	return []evalapi.Dataset{{ID: "ds-1", Name: "RAG Smoke", ItemCount: 3}}, nil
}

func (f *fakeEvalService) ListDatasetFixtures(context.Context) ([]fixturecatalog.Fixture, error) {
	return []fixturecatalog.Fixture{{ID: "rag-eval-dataset-product-docs.json", Name: "product-docs-rag-v1", Valid: true}}, nil
}

func (f *fakeEvalService) CreateDatasetFromFixture(context.Context, string) (evalworkflow.CreateDatasetResult, error) {
	return evalworkflow.CreateDatasetResult{ID: "ds-created", Name: "product-docs-rag-v1"}, nil
}

func (f *fakeEvalService) ListRAGCollections(context.Context) ([]ingestionapi.Collection, error) {
	return []ingestionapi.Collection{{Name: "documents", PointsCount: 15}}, nil
}

func (f *fakeEvalService) GetRAGCollectionConfig(context.Context, string) (map[string]any, error) {
	return map[string]any{"chunk_size": 1000}, nil
}

func (f *fakeEvalService) StartRun(_ context.Context, in evalworkflow.StartRunInput) (evalworkflow.StartRunResult, error) {
	f.startRunCalls++
	f.startRunInput = in
	return evalworkflow.StartRunResult{EvalID: "eval-123", Status: "queued"}, nil
}

func (f *fakeEvalService) WaitForRun(context.Context, string) (evalworkflow.WaitResult, error) {
	return evalworkflow.WaitResult{Run: evalapi.EvaluationDetail{ID: "eval-123", Status: "completed"}}, nil
}

func (f *fakeEvalService) AttachRun(context.Context, string, string, string, string) error {
	return nil
}

func (f *fakeEvalService) GetRun(context.Context, string) (evalapi.EvaluationDetail, error) {
	return evalapi.EvaluationDetail{ID: "eval-123", DatasetID: "ds-1", Status: "completed"}, nil
}

func (f *fakeEvalService) RunEvidence(context.Context, string) (evalworkflow.RunEvidence, error) {
	return evalworkflow.RunEvidence{EvalID: "eval-123", Status: "running", NextSteps: []string{"investigate_eval_run"}}, nil
}

func (f *fakeEvalService) Compare(_ context.Context, in evalworkflow.CompareInput) (evalapi.Comparison, error) {
	f.compareCalls++
	f.compareInput = in
	return evalapi.Comparison{Runs: []evalapi.EvaluationDetail{{ID: "eval-a"}, {ID: "eval-b"}}}, nil
}

func (f *fakeEvalService) WorstCases(_ context.Context, in evalworkflow.WorstCasesInput) (evalworkflow.WorstCasesResult, error) {
	f.worstCasesInput = in
	score := 0.42
	return evalworkflow.WorstCasesResult{
		EvalID: in.EvalID,
		Metric: in.Metric,
		Cases: []evalworkflow.WorstCase{{
			Result: evalapi.QueryResult{Query: "What is RAG?", Answer: "Retrieval augmented generation."},
			Score:  &score,
		}},
	}, nil
}

func (f *fakeEvalService) TriageRAGRegression(_ context.Context, in evalworkflow.TriageInput) (map[string]any, error) {
	f.triageCalls++
	f.triageInput = in
	return map[string]any{"status": "completed"}, nil
}

func (f *fakeEvalService) SummarizeExperiment(context.Context, string) (evalworkflow.ExperimentSummary, error) {
	return evalworkflow.ExperimentSummary{Experiment: evalapi.Experiment{ID: "exp-11", Name: "baseline-vs-rerank"}}, nil
}

func (f *fakeEvalService) RecordConclusion(_ context.Context, in evalworkflow.RecordConclusionInput) error {
	f.recordConclusionInput = in
	return nil
}

func TestServerRegistersPromptResourceAndTools(t *testing.T) {
	srv := New(&fakeEvalService{})

	if got := serverFeatureNames(t, srv, "prompts"); !slices.Equal(got, []string{"eval"}) {
		t.Fatalf("unexpected prompts: %v", got)
	}
	if got := serverFeatureNames(t, srv, "resources"); !slices.Equal(got, []string{workflowResourceURI}) {
		t.Fatalf("unexpected resources: %v", got)
	}
	wantTools := []string{
		"attach_eval_run",
		"compare_eval_runs",
		"create_eval_dataset",
		"get_eval_experiment",
		"get_eval_run",
		"get_eval_run_evidence",
		"get_rag_collection_config",
		"get_worst_eval_cases",
		"list_eval_dataset_fixtures",
		"list_eval_datasets",
		"list_eval_experiments",
		"list_rag_collections",
		"record_eval_experiment_conclusion",
		"start_eval_experiment",
		"start_eval_run",
		"summarize_eval_experiment",
		"triage_rag_regression",
		"wait_for_eval_run",
	}
	if got := serverFeatureNames(t, srv, "tools"); !slices.Equal(got, wantTools) {
		t.Fatalf("unexpected tools:\n got: %v\nwant: %v", got, wantTools)
	}
}

func TestStartEvalExperimentHandler(t *testing.T) {
	fake := &fakeEvalService{}
	result, err := startEvalExperimentHandler(fake)(context.Background(), callReq(map[string]any{
		"name":         "baseline-vs-rerank",
		"dataset_id":   "ds-1",
		"collection":   "documents",
		"focus_metric": "context_precision",
		"hypothesis":   "Reranking improves context precision.",
		"notes":        "Baseline first.",
	}))
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handler returned MCP error: %#v", result)
	}
	if fake.startExperimentInput.Name != "baseline-vs-rerank" || fake.startExperimentInput.DatasetID != "ds-1" {
		t.Fatalf("unexpected start experiment input: %#v", fake.startExperimentInput)
	}

	var payload evalapi.Experiment
	unmarshalTextResult(t, result, &payload)
	if payload.ID != "exp-11" || payload.Name != "baseline-vs-rerank" {
		t.Fatalf("unexpected experiment payload: %#v", payload)
	}
}

func TestStartEvalRunValidationError(t *testing.T) {
	fake := &fakeEvalService{}
	result, err := startEvalRunHandler(fake)(context.Background(), callReq(map[string]any{
		"collection": "documents",
	}))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected MCP tool error for missing dataset_id")
	}
	if fake.startRunCalls != 0 {
		t.Fatalf("service should not be called on validation error, got %d calls", fake.startRunCalls)
	}
}

func TestStartEvalRunForwardsRetrievalConfig(t *testing.T) {
	fake := &fakeEvalService{}
	result, err := startEvalRunHandler(fake)(context.Background(), callReq(map[string]any{
		"dataset_id":       "ds-1",
		"collection":       "documents",
		"rerank":           true,
		"retrieval_config": map[string]any{"top_k": float64(3)},
	}))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handler returned MCP error: %#v", result)
	}
	if fake.startRunInput.RetrievalConfig == nil || fake.startRunInput.RetrievalConfig.TopK == nil || *fake.startRunInput.RetrievalConfig.TopK != 3 {
		t.Fatalf("retrieval config = %#v", fake.startRunInput.RetrievalConfig)
	}
}

func TestStartEvalRunRejectsInvalidRetrievalConfig(t *testing.T) {
	for _, topK := range []float64{0, 21} {
		t.Run("top_k_range", func(t *testing.T) {
			fake := &fakeEvalService{}
			result, err := startEvalRunHandler(fake)(context.Background(), callReq(map[string]any{
				"dataset_id":       "ds-1",
				"collection":       "documents",
				"retrieval_config": map[string]any{"top_k": topK},
			}))
			if err != nil {
				t.Fatalf("handler returned transport error: %v", err)
			}
			if !result.IsError {
				t.Fatalf("expected MCP tool error")
			}
			if fake.startRunCalls != 0 {
				t.Fatalf("service should not be called on validation error, got %d calls", fake.startRunCalls)
			}
			if got := textResult(t, result); !strings.Contains(got, "retrieval_config.top_k must be between 1 and 20") {
				t.Fatalf("error = %q", got)
			}
		})
	}
}

func TestStartEvalRunRejectsUnknownRetrievalConfigField(t *testing.T) {
	fake := &fakeEvalService{}
	result, err := startEvalRunHandler(fake)(context.Background(), callReq(map[string]any{
		"dataset_id":       "ds-1",
		"collection":       "documents",
		"retrieval_config": map[string]any{"limit": float64(3)},
	}))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected MCP tool error")
	}
	if fake.startRunCalls != 0 {
		t.Fatalf("service should not be called on validation error, got %d calls", fake.startRunCalls)
	}
	if got := textResult(t, result); !strings.Contains(got, "retrieval_config.limit is not supported") {
		t.Fatalf("error = %q", got)
	}
}

func TestStartEvalRunSchemaIncludesRetrievalConfig(t *testing.T) {
	schema := string(startEvalRunSchema())
	for _, want := range []string{"retrieval_config", "top_k"} {
		if !strings.Contains(schema, want) {
			t.Fatalf("schema missing %q: %s", want, schema)
		}
	}
}

func TestCreateEvalDatasetHandlerRequiresFixture(t *testing.T) {
	result, err := createEvalDatasetHandler(&fakeEvalService{})(context.Background(), callReq(map[string]any{}))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected MCP tool error")
	}
}

func TestListRAGCollectionsHandler(t *testing.T) {
	result, err := listRAGCollectionsHandler(&fakeEvalService{})(context.Background(), callReq(map[string]any{}))
	if err != nil || result.IsError {
		t.Fatalf("handler failed: result=%#v err=%v", result, err)
	}
	var payload []ingestionapi.Collection
	unmarshalTextResult(t, result, &payload)
	if len(payload) != 1 || payload[0].Name != "documents" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestStartEvalRunRejectsLabelWithoutExperimentID(t *testing.T) {
	fake := &fakeEvalService{}
	result, err := startEvalRunHandler(fake)(context.Background(), callReq(map[string]any{
		"dataset_id": "ds-1",
		"collection": "documents",
		"label":      "candidate",
		"notes":      "Candidate run.",
		"rerank":     true,
	}))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected MCP tool error for label without experiment_id")
	}
	if fake.startRunCalls != 0 {
		t.Fatalf("service should not be called on validation error, got %d calls", fake.startRunCalls)
	}
}

func TestWorstCasesHandlerReturnsJSON(t *testing.T) {
	fake := &fakeEvalService{}
	result, err := worstCasesHandler(fake)(context.Background(), callReq(map[string]any{
		"eval_id": "eval-123",
		"metric":  "context_precision",
		"limit":   float64(3),
	}))
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handler returned MCP error: %#v", result)
	}
	if fake.worstCasesInput.EvalID != "eval-123" || fake.worstCasesInput.Limit != 3 {
		t.Fatalf("unexpected worst cases input: %#v", fake.worstCasesInput)
	}

	var payload evalworkflow.WorstCasesResult
	unmarshalTextResult(t, result, &payload)
	if payload.EvalID != "eval-123" || len(payload.Cases) != 1 || payload.Cases[0].Score == nil {
		t.Fatalf("unexpected worst cases payload: %#v", payload)
	}
}

func TestTriageRAGRegressionHandlerForwardsInput(t *testing.T) {
	fake := &fakeEvalService{}
	result, err := triageRAGRegressionHandler(fake)(context.Background(), callReq(map[string]any{
		"eval_id":               "eval-candidate",
		"baseline_eval_id":      "eval-base",
		"metric":                "context_precision",
		"limit":                 float64(5),
		"include_observability": true,
	}))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handler returned MCP error: %#v", result)
	}
	if fake.triageCalls != 1 {
		t.Fatalf("triage calls = %d, want 1", fake.triageCalls)
	}
	if fake.triageInput.EvalID != "eval-candidate" ||
		fake.triageInput.BaselineEvalID != "eval-base" ||
		fake.triageInput.Metric != "context_precision" ||
		fake.triageInput.Limit != 5 ||
		!fake.triageInput.IncludeObservability {
		t.Fatalf("unexpected triage input: %#v", fake.triageInput)
	}

	var payload map[string]any
	unmarshalTextResult(t, result, &payload)
	if payload["status"] != "completed" {
		t.Fatalf("unexpected triage payload: %#v", payload)
	}
}

func TestTriageRAGRegressionHandlerRejectsInvalidLimit(t *testing.T) {
	for _, limit := range []float64{0, 21} {
		t.Run("limit_range", func(t *testing.T) {
			fake := &fakeEvalService{}
			result, err := triageRAGRegressionHandler(fake)(context.Background(), callReq(map[string]any{
				"eval_id": "eval-123",
				"limit":   limit,
			}))
			if err != nil {
				t.Fatalf("handler returned transport error: %v", err)
			}
			if !result.IsError {
				t.Fatalf("expected MCP tool error")
			}
			if fake.triageCalls != 0 {
				t.Fatalf("service should not be called on validation error, got %d calls", fake.triageCalls)
			}
			if got := textResult(t, result); !strings.Contains(got, "limit must be between 1 and 20 when provided") {
				t.Fatalf("error = %q", got)
			}
		})
	}
}

func TestCompareEvalRunsRejectsSingleLabelWithoutCallingService(t *testing.T) {
	fake := &fakeEvalService{}
	result, err := compareEvalRunsHandler(fake)(context.Background(), callReq(map[string]any{
		"experiment_id": "exp-11",
		"labels":        []any{"candidate"},
	}))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected MCP tool error for one comparison label")
	}
	if fake.compareCalls != 0 {
		t.Fatalf("service should not be called on validation error, got %d calls", fake.compareCalls)
	}
}

func TestCompareEvalRunsRejectsSixTotalIDsAndLabels(t *testing.T) {
	fake := &fakeEvalService{}
	result, err := compareEvalRunsHandler(fake)(context.Background(), callReq(map[string]any{
		"eval_ids":      []any{"eval-a", "eval-b", "eval-c"},
		"experiment_id": "exp-11",
		"labels":        []any{"baseline", "candidate-a", "candidate-b"},
	}))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected MCP tool error for more than five comparison inputs")
	}
	if fake.compareCalls != 0 {
		t.Fatalf("service should not be called on validation error, got %d calls", fake.compareCalls)
	}
}

func TestCompareEvalRunsRejectsLabelsWithoutExperimentID(t *testing.T) {
	fake := &fakeEvalService{}
	result, err := compareEvalRunsHandler(fake)(context.Background(), callReq(map[string]any{
		"eval_ids": []any{"eval-a", "eval-b"},
		"labels":   []any{"candidate"},
	}))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected MCP tool error for labels without experiment_id")
	}
	if fake.compareCalls != 0 {
		t.Fatalf("service should not be called on validation error, got %d calls", fake.compareCalls)
	}
}

func TestGetEvalRunMalformedArgsReturnsDecodeError(t *testing.T) {
	result, err := getEvalRunHandler(&fakeEvalService{})(context.Background(), malformedCallReq(`{"eval_id":`))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected MCP tool error for malformed args")
	}
	text := textResult(t, result)
	if !strings.Contains(text, "invalid arguments:") {
		t.Fatalf("expected decode error, got %q", text)
	}
	if strings.Contains(text, "eval_id is required") {
		t.Fatalf("malformed args should not collapse to required-field error, got %q", text)
	}
}

func TestGetEvalRunEvidenceHandlerReturnsJSON(t *testing.T) {
	result, err := runEvidenceHandler(&fakeEvalService{})(context.Background(), callReq(map[string]any{
		"eval_id": "eval-123",
	}))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handler returned MCP error: %#v", result)
	}
	var payload evalworkflow.RunEvidence
	unmarshalTextResult(t, result, &payload)
	if payload.EvalID != "eval-123" || len(payload.NextSteps) == 0 {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestRecordConclusionHandlerSendsDecisionAndEvidence(t *testing.T) {
	fake := &fakeEvalService{}
	result, err := recordConclusionHandler(fake)(context.Background(), callReq(map[string]any{
		"experiment_id": "exp-1",
		"decision":      "keep",
		"conclusion":    "Keep reranking.",
		"evidence": map[string]any{
			"baseline_eval_id": "eval-base",
		},
	}))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handler returned MCP error: %#v", result)
	}
	if fake.recordConclusionInput.ExperimentID != "exp-1" || fake.recordConclusionInput.Decision != "keep" {
		t.Fatalf("record input = %#v", fake.recordConclusionInput)
	}
}

func TestEvalPromptHandler(t *testing.T) {
	result, err := evalPromptHandler()(context.Background(), &sdkmcp.GetPromptRequest{})
	if err != nil {
		t.Fatalf("prompt handler returned error: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("expected one prompt message, got %d", len(result.Messages))
	}
	text, ok := result.Messages[0].Content.(*sdkmcp.TextContent)
	if !ok {
		t.Fatalf("expected text prompt content, got %T", result.Messages[0].Content)
	}
	for _, want := range []string{"start_eval_experiment", "list_eval_dataset_fixtures", "create_eval_dataset", "list_eval_datasets", "list_rag_collections", "get_rag_collection_config", "start_eval_run", "wait_for_eval_run", "compare_eval_runs", "get_worst_eval_cases", "record_eval_experiment_conclusion", "Never infer a collection from a dataset name", "Compare only completed runs"} {
		if !strings.Contains(text.Text, want) {
			t.Fatalf("expected prompt to mention %s, got %q", want, text.Text)
		}
	}
}

func TestWorkflowResourceHandler(t *testing.T) {
	result, err := workflowResourceHandler()(context.Background(), &sdkmcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("resource handler returned error: %v", err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("expected one resource content item, got %d", len(result.Contents))
	}
	content := result.Contents[0]
	if content.URI != workflowResourceURI {
		t.Fatalf("unexpected resource URI: %q", content.URI)
	}
	if content.MIMEType != "text/markdown" {
		t.Fatalf("unexpected resource MIME type: %q", content.MIMEType)
	}
	for _, want := range []string{"start_eval_experiment", "list_eval_dataset_fixtures", "create_eval_dataset", "list_eval_datasets", "list_rag_collections", "get_rag_collection_config", "start_eval_run", "wait_for_eval_run", "compare_eval_runs", "get_worst_eval_cases", "record_eval_experiment_conclusion", "Never infer a collection from a dataset name", "Compare only completed runs"} {
		if !strings.Contains(content.Text, want) {
			t.Fatalf("expected workflow resource to mention %s, got %q", want, content.Text)
		}
	}
}

func callReq(args map[string]any) *sdkmcp.CallToolRequest {
	raw, err := json.Marshal(args)
	if err != nil {
		panic(err)
	}
	return &sdkmcp.CallToolRequest{Params: &sdkmcp.CallToolParamsRaw{Arguments: raw}}
}

func malformedCallReq(raw string) *sdkmcp.CallToolRequest {
	return &sdkmcp.CallToolRequest{Params: &sdkmcp.CallToolParamsRaw{Arguments: json.RawMessage(raw)}}
}

func textResult(t *testing.T, result *sdkmcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) != 1 {
		t.Fatalf("expected one content item, got %d", len(result.Content))
	}
	text, ok := result.Content[0].(*sdkmcp.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", result.Content[0])
	}
	return text.Text
}

func unmarshalTextResult(t *testing.T, result *sdkmcp.CallToolResult, out any) {
	t.Helper()
	if err := json.Unmarshal([]byte(textResult(t, result)), out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
}

func serverFeatureNames(t *testing.T, srv *sdkmcp.Server, fieldName string) []string {
	t.Helper()
	field := reflect.ValueOf(srv).Elem().FieldByName(fieldName)
	features := field.Elem().FieldByName("features")
	names := make([]string, 0, features.Len())
	for _, key := range features.MapKeys() {
		names = append(names, key.String())
	}
	slices.Sort(names)
	return names
}
