package evalworkflow

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/evalapi"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/fixturecatalog"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/ingestionapi"
)

const (
	DefaultCollection  = "documents"
	DefaultFocusMetric = "context_precision"
	defaultWorstLimit  = 5
	maxWorstLimit      = 20
	minCompareEvalIDs  = 2
	maxCompareEvalIDs  = 5
)

type API interface {
	ListDatasets(context.Context) ([]evalapi.Dataset, error)
	CreateDataset(context.Context, evalapi.CreateDatasetRequest) (evalapi.CreateDatasetResponse, error)
	StartEvaluation(context.Context, evalapi.StartEvaluationRequest) (evalapi.StartEvaluationResponse, error)
	GetEvaluation(context.Context, string) (evalapi.EvaluationDetail, error)
	CompareEvaluations(context.Context, []string) (evalapi.Comparison, error)
	CreateExperiment(context.Context, evalapi.CreateExperimentRequest) (evalapi.Experiment, error)
	ListExperiments(context.Context) ([]evalapi.Experiment, error)
	GetExperiment(context.Context, string) (evalapi.Experiment, error)
	AttachExperimentRun(context.Context, string, evalapi.AttachExperimentRunRequest) (evalapi.Experiment, error)
	UpdateExperiment(context.Context, string, evalapi.UpdateExperimentRequest) (evalapi.Experiment, error)
}

type Ingestion interface {
	ListCollections(context.Context) ([]ingestionapi.Collection, error)
	GetCollectionConfig(context.Context, string) (map[string]any, error)
}

type Fixtures interface {
	List() ([]fixturecatalog.Fixture, error)
	Load(string) (fixturecatalog.Fixture, error)
}

type Service struct {
	api          API
	ingestion    Ingestion
	fixtures     Fixtures
	pollInterval time.Duration
	waitTimeout  time.Duration
}

type StartExperimentInput struct {
	Name           string
	DatasetID      string
	Collection     string
	BaselineEvalID string
	FocusMetric    string
	Hypothesis     string
	Notes          string
}

type StartRunInput struct {
	DatasetID      string
	Collection     string
	Notes          string
	BaselineEvalID string
	Rerank         bool
	ExperimentID   string
	Label          string
}

type StartRunResult struct {
	EvalID string
	Status string
}

type CreateDatasetResult struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type WaitResult struct {
	Run      evalapi.EvaluationDetail
	TimedOut bool
}

type CompareInput struct {
	EvalIDs      []string
	ExperimentID string
	Labels       []string
}

type WorstCasesInput struct {
	EvalID string
	Metric string
	Limit  int
}

type WorstCasesResult struct {
	EvalID string
	Metric string
	Cases  []WorstCase
}

type WorstCase struct {
	Result evalapi.QueryResult
	Score  *float64
}

type LabeledRun struct {
	Label string
	Run   evalapi.EvaluationDetail
}

type LabeledWorstCases struct {
	Label string
	WorstCasesResult
}

type ExperimentSummary struct {
	Experiment evalapi.Experiment
	Baseline   *LabeledRun
	Candidates []LabeledRun
	Comparison *evalapi.Comparison
	WorstCases []LabeledWorstCases
}

type RecordConclusionInput struct {
	ExperimentID string
	Decision     string
	Conclusion   string
	Evidence     map[string]any
}

func New(api API, ingestion Ingestion, fixtures Fixtures, pollInterval, waitTimeout time.Duration) *Service {
	return &Service{
		api:          api,
		ingestion:    ingestion,
		fixtures:     fixtures,
		pollInterval: pollInterval,
		waitTimeout:  waitTimeout,
	}
}

func (s *Service) StartExperiment(ctx context.Context, in StartExperimentInput) (evalapi.Experiment, error) {
	collection := in.Collection
	if collection == "" {
		collection = DefaultCollection
	}
	focusMetric := in.FocusMetric
	if focusMetric == "" {
		focusMetric = DefaultFocusMetric
	}
	if err := validateMetric(focusMetric); err != nil {
		return evalapi.Experiment{}, err
	}
	if err := s.validateCollectionExists(ctx, collection); err != nil {
		return evalapi.Experiment{}, err
	}

	return s.api.CreateExperiment(ctx, evalapi.CreateExperimentRequest{
		Name:           in.Name,
		DatasetID:      in.DatasetID,
		Collection:     collection,
		BaselineEvalID: in.BaselineEvalID,
		FocusMetric:    focusMetric,
		Hypothesis:     in.Hypothesis,
		Notes:          in.Notes,
		Status:         "running",
	})
}

func (s *Service) ListExperiments(ctx context.Context) ([]evalapi.Experiment, error) {
	return s.api.ListExperiments(ctx)
}

func (s *Service) GetExperiment(ctx context.Context, id string) (evalapi.Experiment, error) {
	return s.api.GetExperiment(ctx, id)
}

func (s *Service) ListDatasets(ctx context.Context) ([]evalapi.Dataset, error) {
	return s.api.ListDatasets(ctx)
}

func (s *Service) ListDatasetFixtures(context.Context) ([]fixturecatalog.Fixture, error) {
	return s.fixtures.List()
}

func (s *Service) CreateDatasetFromFixture(ctx context.Context, fixtureID string) (CreateDatasetResult, error) {
	fixture, err := s.fixtures.Load(fixtureID)
	if err != nil {
		return CreateDatasetResult{}, err
	}
	items := make([]evalapi.GoldenItem, 0, len(fixture.Items))
	for _, item := range fixture.Items {
		items = append(items, evalapi.GoldenItem{
			Query:           item.Query,
			ExpectedAnswer:  item.ExpectedAnswer,
			ExpectedSources: item.ExpectedSources,
		})
	}
	created, err := s.api.CreateDataset(ctx, evalapi.CreateDatasetRequest{Name: fixture.Name, Items: items})
	if err != nil {
		return CreateDatasetResult{}, err
	}
	return CreateDatasetResult{ID: created.ID, Name: fixture.Name}, nil
}

func (s *Service) ListRAGCollections(ctx context.Context) ([]ingestionapi.Collection, error) {
	return s.ingestion.ListCollections(ctx)
}

func (s *Service) GetRAGCollectionConfig(ctx context.Context, name string) (map[string]any, error) {
	return s.ingestion.GetCollectionConfig(ctx, name)
}

func (s *Service) StartRun(ctx context.Context, in StartRunInput) (StartRunResult, error) {
	if err := s.validateCollectionExists(ctx, in.Collection); err != nil {
		return StartRunResult{}, err
	}
	resp, err := s.api.StartEvaluation(ctx, evalapi.StartEvaluationRequest{
		DatasetID:       in.DatasetID,
		Collection:      in.Collection,
		Notes:           in.Notes,
		BaselineEvalID:  in.BaselineEvalID,
		Rerank:          in.Rerank,
		ExperimentID:    in.ExperimentID,
		ExperimentLabel: in.Label,
	})
	if err != nil {
		return StartRunResult{}, err
	}
	return StartRunResult{EvalID: resp.ID, Status: resp.Status}, nil
}

func (s *Service) WaitForRun(ctx context.Context, evalID string) (WaitResult, error) {
	timeout := s.waitTimeout
	if timeout <= 0 {
		timeout = time.Minute
	}
	pollInterval := s.pollInterval
	if pollInterval <= 0 {
		pollInterval = time.Second
	}

	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()
	timeoutFired := make(chan struct{})
	timeoutDone := make(chan struct{})
	defer close(timeoutDone)
	go func() {
		select {
		case <-timeoutTimer.C:
			close(timeoutFired)
		case <-timeoutDone:
		}
	}()

	var latest evalapi.EvaluationDetail
	for {
		callCtx, cancelCall := context.WithCancel(ctx)
		go func() {
			select {
			case <-timeoutFired:
				cancelCall()
			case <-callCtx.Done():
			}
		}()
		run, err := s.api.GetEvaluation(callCtx, evalID)
		cancelCall()
		if err != nil {
			select {
			case <-timeoutFired:
				return WaitResult{Run: latest, TimedOut: true}, waitTimeoutError(evalID, timeout, latest.Status)
			default:
			}
			return WaitResult{Run: latest}, err
		}
		latest = run
		if run.Status == "completed" || run.Status == "failed" {
			return WaitResult{Run: run}, nil
		}

		select {
		case <-ctx.Done():
			return WaitResult{Run: latest}, ctx.Err()
		case <-timeoutFired:
			return WaitResult{Run: latest, TimedOut: true}, waitTimeoutError(evalID, timeout, latest.Status)
		case <-time.After(pollInterval):
		}
	}
}

func (s *Service) AttachRun(ctx context.Context, experimentID, label, evalID, notes string) error {
	_, err := s.api.AttachExperimentRun(ctx, experimentID, evalapi.AttachExperimentRunRequest{
		EvaluationID: evalID,
		Label:        label,
		Notes:        notes,
	})
	return err
}

func (s *Service) GetRun(ctx context.Context, evalID string) (evalapi.EvaluationDetail, error) {
	return s.api.GetEvaluation(ctx, evalID)
}

func (s *Service) Compare(ctx context.Context, in CompareInput) (evalapi.Comparison, error) {
	ids := append([]string(nil), in.EvalIDs...)
	if in.ExperimentID != "" {
		resolved, err := s.resolveLabels(ctx, in.ExperimentID, in.Labels)
		if err != nil {
			return evalapi.Comparison{}, err
		}
		ids = append(ids, resolved...)
	}
	if err := validateCompareIDs(ids); err != nil {
		return evalapi.Comparison{}, err
	}
	if err := s.requireCompletedRuns(ctx, ids); err != nil {
		return evalapi.Comparison{}, err
	}
	return s.api.CompareEvaluations(ctx, ids)
}

func (s *Service) WorstCases(ctx context.Context, in WorstCasesInput) (WorstCasesResult, error) {
	metric := in.Metric
	if metric == "" {
		metric = DefaultFocusMetric
	}
	if err := validateMetric(metric); err != nil {
		return WorstCasesResult{}, err
	}
	limit := normalizeWorstLimit(in.Limit)

	run, err := s.api.GetEvaluation(ctx, in.EvalID)
	if err != nil {
		return WorstCasesResult{}, err
	}
	return worstCasesFromResults(in.EvalID, metric, limit, run.Results), nil
}

func worstCasesFromResults(evalID, metric string, limit int, results []evalapi.QueryResult) WorstCasesResult {
	cases := make([]WorstCase, 0, len(results))
	for _, result := range results {
		score := metricScore(result.Scores, metric)
		cases = append(cases, WorstCase{
			Result: result,
			Score:  score,
		})
	}
	sort.SliceStable(cases, func(i, j int) bool {
		return scoreSortValue(cases[i].Score) < scoreSortValue(cases[j].Score)
	})
	if len(cases) > limit {
		cases = cases[:limit]
	}
	return WorstCasesResult{EvalID: evalID, Metric: metric, Cases: cases}
}

func (s *Service) SummarizeExperiment(ctx context.Context, experimentID string) (ExperimentSummary, error) {
	exp, err := s.api.GetExperiment(ctx, experimentID)
	if err != nil {
		return ExperimentSummary{}, err
	}

	summary := ExperimentSummary{Experiment: exp}
	evalIDs := make([]string, 0, len(exp.Runs))
	for i, runLabel := range exp.Runs {
		run, err := s.api.GetEvaluation(ctx, runLabel.EvaluationID)
		if err != nil {
			return ExperimentSummary{}, fmt.Errorf("get run %q (%s): %w", runLabel.Label, runLabel.EvaluationID, err)
		}
		if run.Status != "completed" {
			return ExperimentSummary{}, fmt.Errorf("summarize requires completed runs; %s=%s", run.ID, run.Status)
		}
		labeledRun := LabeledRun{Label: runLabel.Label, Run: run}
		if i == 0 {
			summary.Baseline = &labeledRun
		} else {
			summary.Candidates = append(summary.Candidates, labeledRun)
		}
		evalIDs = append(evalIDs, runLabel.EvaluationID)

		metric := exp.FocusMetric
		if metric == "" {
			metric = DefaultFocusMetric
		}
		if err := validateMetric(metric); err != nil {
			return ExperimentSummary{}, fmt.Errorf("worst cases for %q: %w", runLabel.Label, err)
		}
		summary.WorstCases = append(summary.WorstCases, LabeledWorstCases{
			Label:            runLabel.Label,
			WorstCasesResult: worstCasesFromResults(runLabel.EvaluationID, metric, defaultWorstLimit, run.Results),
		})
	}
	if len(evalIDs) >= minCompareEvalIDs {
		compareIDs := evalIDs
		if len(compareIDs) > maxCompareEvalIDs {
			compareIDs = compareIDs[:maxCompareEvalIDs]
		}
		comparison, err := s.api.CompareEvaluations(ctx, compareIDs)
		if err != nil {
			return ExperimentSummary{}, err
		}
		summary.Comparison = &comparison
	}
	return summary, nil
}

func (s *Service) RecordConclusion(ctx context.Context, in RecordConclusionInput) error {
	_, err := s.api.UpdateExperiment(ctx, in.ExperimentID, evalapi.UpdateExperimentRequest{
		Status:     "completed",
		Decision:   in.Decision,
		Conclusion: in.Conclusion,
		Evidence:   in.Evidence,
	})
	return err
}

func waitTimeoutError(evalID string, timeout time.Duration, latestStatus string) error {
	return fmt.Errorf("wait for evaluation %q timed out after %s with latest status %q", evalID, timeout, latestStatus)
}

func validateCompareIDs(ids []string) error {
	if len(ids) < minCompareEvalIDs || len(ids) > maxCompareEvalIDs {
		return fmt.Errorf("compare requires %d to %d eval IDs, got %d", minCompareEvalIDs, maxCompareEvalIDs, len(ids))
	}
	return nil
}

func (s *Service) validateCollectionExists(ctx context.Context, collection string) error {
	collections, err := s.ingestion.ListCollections(ctx)
	if err != nil {
		return fmt.Errorf("list RAG collections: %w", err)
	}
	for _, candidate := range collections {
		if candidate.Name == collection {
			return nil
		}
	}
	return fmt.Errorf("retrieval collection %q does not exist; call list_rag_collections and choose an existing collection", collection)
}

func (s *Service) requireCompletedRuns(ctx context.Context, ids []string) error {
	var invalid []string
	for _, id := range ids {
		run, err := s.api.GetEvaluation(ctx, id)
		if err != nil {
			return fmt.Errorf("get run %q: %w", id, err)
		}
		if run.Status != "completed" {
			invalid = append(invalid, fmt.Sprintf("%s=%s", id, run.Status))
		}
	}
	if len(invalid) > 0 {
		return fmt.Errorf("compare requires completed runs; invalid statuses: %s", strings.Join(invalid, ", "))
	}
	return nil
}

func (s *Service) resolveLabels(ctx context.Context, experimentID string, labels []string) ([]string, error) {
	exp, err := s.api.GetExperiment(ctx, experimentID)
	if err != nil {
		return nil, err
	}
	byLabel := make(map[string]string, len(exp.Runs))
	known := make([]string, 0, len(exp.Runs))
	for _, run := range exp.Runs {
		byLabel[run.Label] = run.EvaluationID
		known = append(known, run.Label)
	}
	sort.Strings(known)

	ids := make([]string, 0, len(labels))
	var missing []string
	for _, label := range labels {
		evalID, ok := byLabel[label]
		if !ok {
			missing = append(missing, label)
			continue
		}
		ids = append(ids, evalID)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("experiment %s missing run labels %s; known labels: %s", experimentID, strings.Join(missing, ", "), strings.Join(known, ", "))
	}
	return ids, nil
}

func normalizeWorstLimit(limit int) int {
	if limit <= 0 {
		return defaultWorstLimit
	}
	if limit > maxWorstLimit {
		return maxWorstLimit
	}
	return limit
}

func validateMetric(metric string) error {
	switch metric {
	case "faithfulness", "answer_relevancy", "context_precision", "context_recall":
		return nil
	default:
		return fmt.Errorf("unsupported metric %q; supported metrics: faithfulness, answer_relevancy, context_precision, context_recall", metric)
	}
}

func metricScore(scores evalapi.Scores, metric string) *float64 {
	switch metric {
	case "faithfulness":
		return scores.Faithfulness
	case "answer_relevancy":
		return scores.AnswerRelevancy
	case "context_precision":
		return scores.ContextPrecision
	case "context_recall":
		return scores.ContextRecall
	default:
		return nil
	}
}

func scoreSortValue(score *float64) float64 {
	if score == nil {
		return math.Inf(1)
	}
	return *score
}
