package evalworkflow

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/evalapi"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/store"
)

func TestStartExperimentDefaultsCollectionAndFocusMetric(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{}
	st := newFakeStore()
	svc := New(api, st, time.Millisecond, time.Second)

	got, err := svc.StartExperiment(ctx, StartExperimentInput{
		Name:      "rerank trial",
		DatasetID: "dataset-1",
	})
	if err != nil {
		t.Fatalf("StartExperiment error: %v", err)
	}
	if got.Collection != DefaultCollection {
		t.Fatalf("Collection = %q, want %q", got.Collection, DefaultCollection)
	}
	if got.FocusMetric != DefaultFocusMetric {
		t.Fatalf("FocusMetric = %q, want %q", got.FocusMetric, DefaultFocusMetric)
	}
	if st.createCalls[0].Collection != DefaultCollection || st.createCalls[0].FocusMetric != DefaultFocusMetric {
		t.Fatalf("CreateExperiment input = %#v", st.createCalls[0])
	}
}

func TestStartExperimentAttachesBaselineRunWhenProvided(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{}
	st := newFakeStore()
	svc := New(api, st, time.Millisecond, time.Second)

	got, err := svc.StartExperiment(ctx, StartExperimentInput{
		Name:           "rerank trial",
		DatasetID:      "dataset-1",
		BaselineEvalID: "eval-base",
	})
	if err != nil {
		t.Fatalf("StartExperiment error: %v", err)
	}
	if got.BaselineEvalID != "eval-base" {
		t.Fatalf("BaselineEvalID = %q, want %q", got.BaselineEvalID, "eval-base")
	}
	if len(st.attachCalls) != 1 {
		t.Fatalf("AttachRun calls = %d, want 1", len(st.attachCalls))
	}
	if st.attachCalls[0] != (attachCall{experimentID: got.ID, label: "baseline", evalID: "eval-base", notes: "baseline"}) {
		t.Fatalf("AttachRun call = %#v", st.attachCalls[0])
	}
}

func TestStartRunAttachesReturnedEvalIDWhenExperimentProvided(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{startResponse: evalapi.StartEvaluationResponse{ID: "eval-123", Status: "queued"}}
	st := newFakeStore()
	st.experiments[7] = store.Experiment{ID: 7}
	svc := New(api, st, time.Millisecond, time.Second)

	got, err := svc.StartRun(ctx, StartRunInput{
		DatasetID:      "dataset-1",
		Collection:     "kb",
		Notes:          "candidate notes",
		ExperimentID:   7,
		Label:          "candidate",
		BaselineEvalID: "eval-base",
		Rerank:         true,
	})
	if err != nil {
		t.Fatalf("StartRun error: %v", err)
	}
	if got.EvalID != "eval-123" || got.Status != "queued" {
		t.Fatalf("StartRunResult = %#v", got)
	}
	if len(api.startRequests) != 1 {
		t.Fatalf("StartEvaluation calls = %d, want 1", len(api.startRequests))
	}
	if api.startRequests[0].DatasetID != "dataset-1" || api.startRequests[0].Collection != "kb" || api.startRequests[0].BaselineEvalID != "eval-base" || !api.startRequests[0].Rerank {
		t.Fatalf("StartEvaluation request = %#v", api.startRequests[0])
	}
	if len(st.attachCalls) != 1 {
		t.Fatalf("AttachRun calls = %d, want 1", len(st.attachCalls))
	}
	if st.attachCalls[0] != (attachCall{experimentID: 7, label: "candidate", evalID: "eval-123", notes: "candidate notes"}) {
		t.Fatalf("AttachRun call = %#v", st.attachCalls[0])
	}
}

func TestStartRunPrevalidatesExperimentBeforeStartingRemoteRun(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{startResponse: evalapi.StartEvaluationResponse{ID: "eval-orphan", Status: "queued"}}
	st := newFakeStore()
	svc := New(api, st, time.Millisecond, time.Second)

	_, err := svc.StartRun(ctx, StartRunInput{
		DatasetID:    "dataset-1",
		Collection:   "kb",
		ExperimentID: 99,
		Label:        "candidate",
	})
	if err == nil {
		t.Fatal("StartRun error = nil, want missing experiment error")
	}
	if len(api.startRequests) != 0 {
		t.Fatalf("StartEvaluation calls = %d, want 0", len(api.startRequests))
	}
	if len(st.attachCalls) != 0 {
		t.Fatalf("AttachRun calls = %d, want 0", len(st.attachCalls))
	}
}

func TestStartRunRejectsExperimentDatasetMismatch(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{startResponse: evalapi.StartEvaluationResponse{ID: "eval-orphan", Status: "queued"}}
	st := newFakeStore()
	st.experiments[7] = store.Experiment{ID: 7, DatasetID: "dataset-1", Collection: "kb"}
	svc := New(api, st, time.Millisecond, time.Second)

	_, err := svc.StartRun(ctx, StartRunInput{
		DatasetID:    "dataset-2",
		Collection:   "kb",
		ExperimentID: 7,
		Label:        "candidate",
	})
	if err == nil || !strings.Contains(err.Error(), `requires dataset "dataset-1"; got "dataset-2"`) {
		t.Fatalf("StartRun error = %v, want dataset mismatch", err)
	}
	if len(api.startRequests) != 0 {
		t.Fatalf("StartEvaluation calls = %d, want 0", len(api.startRequests))
	}
}

func TestStartRunRejectsExperimentCollectionMismatch(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{startResponse: evalapi.StartEvaluationResponse{ID: "eval-orphan", Status: "queued"}}
	st := newFakeStore()
	st.experiments[7] = store.Experiment{ID: 7, DatasetID: "dataset-1", Collection: "kb"}
	svc := New(api, st, time.Millisecond, time.Second)

	_, err := svc.StartRun(ctx, StartRunInput{
		DatasetID:    "dataset-1",
		Collection:   "documents",
		ExperimentID: 7,
		Label:        "candidate",
	})
	if err == nil || !strings.Contains(err.Error(), `requires collection "kb"; got "documents"`) {
		t.Fatalf("StartRun error = %v, want collection mismatch", err)
	}
	if len(api.startRequests) != 0 {
		t.Fatalf("StartEvaluation calls = %d, want 0", len(api.startRequests))
	}
}

func TestStartRunUsesExperimentBaselineWhenNotProvided(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{startResponse: evalapi.StartEvaluationResponse{ID: "eval-123", Status: "queued"}}
	st := newFakeStore()
	st.experiments[7] = store.Experiment{ID: 7, DatasetID: "dataset-1", Collection: "kb", BaselineEvalID: "eval-base"}
	svc := New(api, st, time.Millisecond, time.Second)

	_, err := svc.StartRun(ctx, StartRunInput{
		DatasetID:    "dataset-1",
		Collection:   "kb",
		ExperimentID: 7,
		Label:        "candidate",
	})
	if err != nil {
		t.Fatalf("StartRun error: %v", err)
	}
	if len(api.startRequests) != 1 {
		t.Fatalf("StartEvaluation calls = %d, want 1", len(api.startRequests))
	}
	if api.startRequests[0].BaselineEvalID != "eval-base" {
		t.Fatalf("BaselineEvalID = %q, want %q", api.startRequests[0].BaselineEvalID, "eval-base")
	}
}

func TestWaitForRunReturnsCompletedRun(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{detailsByID: map[string][]evalapi.EvaluationDetail{
		"eval-1": {
			{ID: "eval-1", Status: "running"},
			{ID: "eval-1", Status: "completed"},
		},
	}}
	svc := New(api, newFakeStore(), time.Millisecond, time.Second)

	got, err := svc.WaitForRun(ctx, "eval-1")
	if err != nil {
		t.Fatalf("WaitForRun error: %v", err)
	}
	if got.Run.Status != "completed" || got.TimedOut {
		t.Fatalf("WaitResult = %#v", got)
	}
}

func TestWaitForRunTimesOutWithLatestStatus(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{detailsByID: map[string][]evalapi.EvaluationDetail{
		"eval-1": {
			{ID: "eval-1", Status: "queued"},
			{ID: "eval-1", Status: "running"},
		},
	}}
	svc := New(api, newFakeStore(), time.Millisecond, 3*time.Millisecond)

	got, err := svc.WaitForRun(ctx, "eval-1")
	if err == nil {
		t.Fatal("WaitForRun error = nil, want timeout")
	}
	if !got.TimedOut || got.Run.Status != "running" {
		t.Fatalf("WaitResult = %#v", got)
	}
}

func TestWaitForRunCancelsGetEvaluationWithWaitTimeout(t *testing.T) {
	parentCtx, cancelParent := context.WithCancel(context.Background())
	defer cancelParent()

	api := &blockingGetEvaluationAPI{entered: make(chan struct{})}
	svc := New(api, newFakeStore(), time.Hour, 5*time.Millisecond)

	type waitOutcome struct {
		result WaitResult
		err    error
	}
	outcomeCh := make(chan waitOutcome, 1)
	go func() {
		result, err := svc.WaitForRun(parentCtx, "eval-1")
		outcomeCh <- waitOutcome{result: result, err: err}
	}()

	select {
	case <-api.entered:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("GetEvaluation was not called")
	}

	select {
	case outcome := <-outcomeCh:
		if outcome.err == nil {
			t.Fatal("WaitForRun error = nil, want timeout")
		}
		if !outcome.result.TimedOut {
			t.Fatalf("WaitResult = %#v, want TimedOut", outcome.result)
		}
		if !strings.Contains(outcome.err.Error(), `wait for evaluation "eval-1" timed out after 5ms with latest status ""`) {
			t.Fatalf("WaitForRun error = %v, want helpful timeout", outcome.err)
		}
	case <-time.After(100 * time.Millisecond):
		cancelParent()
		t.Fatal("WaitForRun did not cancel GetEvaluation using waitTimeout")
	}

	select {
	case <-parentCtx.Done():
		t.Fatal("parent context was canceled")
	default:
	}
}

func TestWaitForRunParentDeadlineIsNotServiceTimeout(t *testing.T) {
	parentCtx, cancelParent := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancelParent()

	api := &blockingGetEvaluationAPI{entered: make(chan struct{})}
	svc := New(api, newFakeStore(), time.Hour, time.Minute)

	type waitOutcome struct {
		result WaitResult
		err    error
	}
	outcomeCh := make(chan waitOutcome, 1)
	go func() {
		result, err := svc.WaitForRun(parentCtx, "eval-1")
		outcomeCh <- waitOutcome{result: result, err: err}
	}()

	select {
	case <-api.entered:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("GetEvaluation was not called")
	}

	select {
	case outcome := <-outcomeCh:
		if !errors.Is(outcome.err, context.DeadlineExceeded) {
			t.Fatalf("WaitForRun error = %v, want parent deadline", outcome.err)
		}
		if outcome.result.TimedOut {
			t.Fatalf("WaitResult = %#v, want TimedOut=false", outcome.result)
		}
		if strings.Contains(outcome.err.Error(), "wait for evaluation") {
			t.Fatalf("WaitForRun error = %v, want parent context error", outcome.err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("WaitForRun did not return after parent deadline")
	}
}

func TestWorstCasesSortsAscendingByMetric(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{detailsByID: map[string][]evalapi.EvaluationDetail{
		"eval-1": {{
			ID:     "eval-1",
			Status: "completed",
			Results: []evalapi.QueryResult{
				queryResult("highest", 0.90),
				queryResult("lowest", 0.10),
				queryResult("middle", 0.40),
			},
		}},
	}}
	svc := New(api, newFakeStore(), time.Millisecond, time.Second)

	got, err := svc.WorstCases(ctx, WorstCasesInput{
		EvalID: "eval-1",
		Metric: "context_precision",
		Limit:  2,
	})
	if err != nil {
		t.Fatalf("WorstCases error: %v", err)
	}
	if got.Metric != "context_precision" {
		t.Fatalf("Metric = %q", got.Metric)
	}
	gotQueries := []string{got.Cases[0].Result.Query, got.Cases[1].Result.Query}
	if !reflect.DeepEqual(gotQueries, []string{"lowest", "middle"}) {
		t.Fatalf("queries = %#v", gotQueries)
	}
}

func TestCompareResolvesExperimentLabels(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{}
	st := newFakeStore()
	st.experiments[9] = store.Experiment{
		ID: 9,
		Runs: []store.RunLabel{
			{Label: "baseline", EvalID: "eval-base"},
			{Label: "candidate", EvalID: "eval-candidate"},
		},
	}
	svc := New(api, st, time.Millisecond, time.Second)

	if _, err := svc.Compare(ctx, CompareInput{ExperimentID: 9, Labels: []string{"baseline", "missing"}}); err == nil || !strings.Contains(err.Error(), "known labels: baseline, candidate") {
		t.Fatalf("Compare missing label error = %v", err)
	}

	if _, err := svc.Compare(ctx, CompareInput{ExperimentID: 9, Labels: []string{"baseline", "candidate"}}); err != nil {
		t.Fatalf("Compare error: %v", err)
	}
	if !reflect.DeepEqual(api.compareIDs, []string{"eval-base", "eval-candidate"}) {
		t.Fatalf("CompareEvaluations IDs = %#v", api.compareIDs)
	}
}

func TestCompareRequiresTwoToFiveIDs(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{}
	svc := New(api, newFakeStore(), time.Millisecond, time.Second)

	if _, err := svc.Compare(ctx, CompareInput{EvalIDs: []string{"eval-1"}}); err == nil || !strings.Contains(err.Error(), "requires 2 to 5 eval IDs") {
		t.Fatalf("Compare one ID error = %v", err)
	}
	if _, err := svc.Compare(ctx, CompareInput{EvalIDs: []string{"eval-1", "eval-2", "eval-3", "eval-4", "eval-5", "eval-6"}}); err == nil || !strings.Contains(err.Error(), "requires 2 to 5 eval IDs") {
		t.Fatalf("Compare six IDs error = %v", err)
	}
	if len(api.compareIDs) != 0 {
		t.Fatalf("CompareEvaluations IDs = %#v, want no call", api.compareIDs)
	}
}

func TestSummarizeExperimentReturnsBaselineCandidateAndWorstCases(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{detailsByID: map[string][]evalapi.EvaluationDetail{
		"eval-base": {{
			ID:     "eval-base",
			Status: "completed",
			Results: []evalapi.QueryResult{
				queryResult("base low", 0.20),
				queryResult("base high", 0.80),
			},
		}},
		"eval-candidate": {{
			ID:     "eval-candidate",
			Status: "completed",
			Results: []evalapi.QueryResult{
				queryResult("candidate low", 0.10),
				queryResult("candidate high", 0.90),
			},
		}},
	}}
	st := newFakeStore()
	st.experiments[3] = store.Experiment{
		ID:          3,
		FocusMetric: "context_precision",
		Runs: []store.RunLabel{
			{Label: "baseline", EvalID: "eval-base"},
			{Label: "candidate", EvalID: "eval-candidate"},
		},
	}
	svc := New(api, st, time.Millisecond, time.Second)

	got, err := svc.SummarizeExperiment(ctx, 3)
	if err != nil {
		t.Fatalf("SummarizeExperiment error: %v", err)
	}
	if got.Baseline == nil || got.Baseline.Label != "baseline" || got.Baseline.Run.ID != "eval-base" {
		t.Fatalf("Baseline = %#v", got.Baseline)
	}
	if len(got.Candidates) != 1 || got.Candidates[0].Label != "candidate" || got.Candidates[0].Run.ID != "eval-candidate" {
		t.Fatalf("Candidates = %#v", got.Candidates)
	}
	if !reflect.DeepEqual(api.compareIDs, []string{"eval-base", "eval-candidate"}) {
		t.Fatalf("CompareEvaluations IDs = %#v", api.compareIDs)
	}
	if len(got.WorstCases) != 2 {
		t.Fatalf("WorstCases length = %d, want 2", len(got.WorstCases))
	}
	if got.WorstCases[0].Label != "baseline" || got.WorstCases[0].Cases[0].Result.Query != "base low" {
		t.Fatalf("WorstCases[0] = %#v", got.WorstCases[0])
	}
}

func TestSummarizeExperimentComparesFirstFiveRuns(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{detailsByID: make(map[string][]evalapi.EvaluationDetail)}
	st := newFakeStore()
	exp := store.Experiment{
		ID:          4,
		FocusMetric: "context_precision",
	}
	for i := 1; i <= 6; i++ {
		label := fmt.Sprintf("run-%d", i)
		evalID := fmt.Sprintf("eval-%d", i)
		exp.Runs = append(exp.Runs, store.RunLabel{Label: label, EvalID: evalID})
		api.detailsByID[evalID] = []evalapi.EvaluationDetail{{
			ID:      evalID,
			Status:  "completed",
			Results: []evalapi.QueryResult{queryResult(label+" low", float64(i)/10)},
		}}
	}
	st.experiments[4] = exp
	svc := New(api, st, time.Millisecond, time.Second)

	got, err := svc.SummarizeExperiment(ctx, 4)
	if err != nil {
		t.Fatalf("SummarizeExperiment error: %v", err)
	}
	if got.Baseline == nil || len(got.Candidates) != 5 || len(got.WorstCases) != 6 {
		t.Fatalf("summary = %#v", got)
	}
	if !reflect.DeepEqual(api.compareIDs, []string{"eval-1", "eval-2", "eval-3", "eval-4", "eval-5"}) {
		t.Fatalf("CompareEvaluations IDs = %#v", api.compareIDs)
	}
}

func queryResult(query string, contextPrecision float64) evalapi.QueryResult {
	return evalapi.QueryResult{
		Query: query,
		Scores: evalapi.Scores{
			ContextPrecision: &contextPrecision,
		},
	}
}

type fakeAPI struct {
	datasets      []evalapi.Dataset
	startResponse evalapi.StartEvaluationResponse
	startRequests []evalapi.StartEvaluationRequest
	detailsByID   map[string][]evalapi.EvaluationDetail
	compareIDs    []string
	comparison    evalapi.Comparison
}

func (f *fakeAPI) ListDatasets(context.Context) ([]evalapi.Dataset, error) {
	return f.datasets, nil
}

func (f *fakeAPI) StartEvaluation(_ context.Context, in evalapi.StartEvaluationRequest) (evalapi.StartEvaluationResponse, error) {
	f.startRequests = append(f.startRequests, in)
	return f.startResponse, nil
}

func (f *fakeAPI) GetEvaluation(_ context.Context, id string) (evalapi.EvaluationDetail, error) {
	details := f.detailsByID[id]
	if len(details) == 0 {
		return evalapi.EvaluationDetail{}, errors.New("missing eval detail")
	}
	if len(details) == 1 {
		return details[0], nil
	}
	got := details[0]
	f.detailsByID[id] = details[1:]
	return got, nil
}

func (f *fakeAPI) CompareEvaluations(_ context.Context, ids []string) (evalapi.Comparison, error) {
	f.compareIDs = append([]string(nil), ids...)
	if f.comparison.Runs != nil || f.comparison.Deltas != nil {
		return f.comparison, nil
	}
	return evalapi.Comparison{Runs: nil, Deltas: map[string][]float64{}}, nil
}

type blockingGetEvaluationAPI struct {
	entered chan struct{}
}

func (b *blockingGetEvaluationAPI) ListDatasets(context.Context) ([]evalapi.Dataset, error) {
	return nil, nil
}

func (b *blockingGetEvaluationAPI) StartEvaluation(context.Context, evalapi.StartEvaluationRequest) (evalapi.StartEvaluationResponse, error) {
	return evalapi.StartEvaluationResponse{}, nil
}

func (b *blockingGetEvaluationAPI) GetEvaluation(ctx context.Context, _ string) (evalapi.EvaluationDetail, error) {
	close(b.entered)
	<-ctx.Done()
	return evalapi.EvaluationDetail{}, ctx.Err()
}

func (b *blockingGetEvaluationAPI) CompareEvaluations(context.Context, []string) (evalapi.Comparison, error) {
	return evalapi.Comparison{}, nil
}

type fakeStore struct {
	nextID      int64
	experiments map[int64]store.Experiment
	createCalls []store.CreateExperimentInput
	attachCalls []attachCall
	conclusions []recordConclusionCall
}

type attachCall struct {
	experimentID int64
	label        string
	evalID       string
	notes        string
}

type recordConclusionCall struct {
	experimentID int64
	conclusion   string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		nextID:      1,
		experiments: make(map[int64]store.Experiment),
	}
}

func (f *fakeStore) CreateExperiment(_ context.Context, in store.CreateExperimentInput) (int64, error) {
	f.createCalls = append(f.createCalls, in)
	id := f.nextID
	f.nextID++
	f.experiments[id] = store.Experiment{
		ID:             id,
		Name:           in.Name,
		DatasetID:      in.DatasetID,
		Collection:     in.Collection,
		BaselineEvalID: in.BaselineEvalID,
		FocusMetric:    in.FocusMetric,
		Hypothesis:     in.Hypothesis,
		Notes:          in.Notes,
	}
	return id, nil
}

func (f *fakeStore) ListExperiments(context.Context) ([]store.Experiment, error) {
	experiments := make([]store.Experiment, 0, len(f.experiments))
	for _, exp := range f.experiments {
		experiments = append(experiments, exp)
	}
	return experiments, nil
}

func (f *fakeStore) GetExperiment(_ context.Context, id int64) (store.Experiment, error) {
	exp, ok := f.experiments[id]
	if !ok {
		return store.Experiment{}, store.ErrNotFound
	}
	return exp, nil
}

func (f *fakeStore) AttachRun(_ context.Context, experimentID int64, label, evalID, notes string) error {
	f.attachCalls = append(f.attachCalls, attachCall{experimentID: experimentID, label: label, evalID: evalID, notes: notes})
	exp := f.experiments[experimentID]
	exp.Runs = append(exp.Runs, store.RunLabel{Label: label, EvalID: evalID, Notes: notes})
	f.experiments[experimentID] = exp
	return nil
}

func (f *fakeStore) RecordConclusion(_ context.Context, experimentID int64, conclusion string) error {
	f.conclusions = append(f.conclusions, recordConclusionCall{experimentID: experimentID, conclusion: conclusion})
	exp := f.experiments[experimentID]
	exp.Conclusion = conclusion
	f.experiments[experimentID] = exp
	return nil
}
