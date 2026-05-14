package mcpserver

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/evalapi"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/evalworkflow"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/store"
)

type fakeEvalService struct {
	startExperimentInput evalworkflow.StartExperimentInput
	startRunInput        evalworkflow.StartRunInput
	worstCasesInput      evalworkflow.WorstCasesInput
	startRunCalls        int
	compareCalls         int
	compareInput         evalworkflow.CompareInput
}

func (f *fakeEvalService) StartExperiment(_ context.Context, in evalworkflow.StartExperimentInput) (store.Experiment, error) {
	f.startExperimentInput = in
	return store.Experiment{
		ID:          11,
		Name:        in.Name,
		DatasetID:   in.DatasetID,
		Collection:  in.Collection,
		FocusMetric: in.FocusMetric,
		Hypothesis:  in.Hypothesis,
		Notes:       in.Notes,
		CreatedAt:   time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC),
	}, nil
}

func (f *fakeEvalService) ListExperiments(context.Context) ([]store.Experiment, error) {
	return []store.Experiment{{ID: 11, Name: "baseline-vs-rerank", DatasetID: "ds-1"}}, nil
}

func (f *fakeEvalService) GetExperiment(context.Context, int64) (store.Experiment, error) {
	return store.Experiment{ID: 11, Name: "baseline-vs-rerank", DatasetID: "ds-1"}, nil
}

func (f *fakeEvalService) ListDatasets(context.Context) ([]evalapi.Dataset, error) {
	return []evalapi.Dataset{{ID: "ds-1", Name: "RAG Smoke", ItemCount: 3}}, nil
}

func (f *fakeEvalService) StartRun(_ context.Context, in evalworkflow.StartRunInput) (evalworkflow.StartRunResult, error) {
	f.startRunCalls++
	f.startRunInput = in
	return evalworkflow.StartRunResult{EvalID: "eval-123", Status: "queued"}, nil
}

func (f *fakeEvalService) WaitForRun(context.Context, string) (evalworkflow.WaitResult, error) {
	return evalworkflow.WaitResult{Run: evalapi.EvaluationDetail{ID: "eval-123", Status: "completed"}}, nil
}

func (f *fakeEvalService) AttachRun(context.Context, int64, string, string, string) error {
	return nil
}

func (f *fakeEvalService) GetRun(context.Context, string) (evalapi.EvaluationDetail, error) {
	return evalapi.EvaluationDetail{ID: "eval-123", DatasetID: "ds-1", Status: "completed"}, nil
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

func (f *fakeEvalService) SummarizeExperiment(context.Context, int64) (evalworkflow.ExperimentSummary, error) {
	return evalworkflow.ExperimentSummary{Experiment: store.Experiment{ID: 11, Name: "baseline-vs-rerank"}}, nil
}

func (f *fakeEvalService) RecordConclusion(context.Context, int64, string) error {
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
		"get_eval_experiment",
		"get_eval_run",
		"get_worst_eval_cases",
		"list_eval_datasets",
		"list_eval_experiments",
		"record_eval_experiment_conclusion",
		"start_eval_experiment",
		"start_eval_run",
		"summarize_eval_experiment",
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

	var payload store.Experiment
	unmarshalTextResult(t, result, &payload)
	if payload.ID != 11 || payload.Name != "baseline-vs-rerank" {
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

func TestCompareEvalRunsRejectsSingleLabelWithoutCallingService(t *testing.T) {
	fake := &fakeEvalService{}
	result, err := compareEvalRunsHandler(fake)(context.Background(), callReq(map[string]any{
		"experiment_id": float64(11),
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
		"experiment_id": float64(11),
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
	for _, want := range []string{"start_eval_experiment", "list_eval_datasets", "start_eval_run", "wait_for_eval_run", "compare_eval_runs", "get_worst_eval_cases", "record_eval_experiment_conclusion"} {
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
	for _, want := range []string{"start_eval_experiment", "list_eval_datasets", "start_eval_run", "wait_for_eval_run", "compare_eval_runs", "get_worst_eval_cases", "record_eval_experiment_conclusion"} {
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
