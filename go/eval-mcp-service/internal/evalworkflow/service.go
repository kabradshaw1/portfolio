package evalworkflow

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/evalapi"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/store"
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
	StartEvaluation(context.Context, evalapi.StartEvaluationRequest) (evalapi.StartEvaluationResponse, error)
	GetEvaluation(context.Context, string) (evalapi.EvaluationDetail, error)
	CompareEvaluations(context.Context, []string) (evalapi.Comparison, error)
}

type Store interface {
	CreateExperiment(context.Context, store.CreateExperimentInput) (int64, error)
	ListExperiments(context.Context) ([]store.Experiment, error)
	GetExperiment(context.Context, int64) (store.Experiment, error)
	AttachRun(context.Context, int64, string, string, string) error
	RecordConclusion(context.Context, int64, string) error
}

type Service struct {
	api          API
	store        Store
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
	ExperimentID   int64
	Label          string
}

type StartRunResult struct {
	EvalID string
	Status string
}

type WaitResult struct {
	Run      evalapi.EvaluationDetail
	TimedOut bool
}

type CompareInput struct {
	EvalIDs      []string
	ExperimentID int64
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
	Experiment store.Experiment
	Baseline   *LabeledRun
	Candidates []LabeledRun
	Comparison *evalapi.Comparison
	WorstCases []LabeledWorstCases
}

func New(api API, st Store, pollInterval, waitTimeout time.Duration) *Service {
	return &Service{
		api:          api,
		store:        st,
		pollInterval: pollInterval,
		waitTimeout:  waitTimeout,
	}
}

func (s *Service) StartExperiment(ctx context.Context, in StartExperimentInput) (store.Experiment, error) {
	collection := in.Collection
	if collection == "" {
		collection = DefaultCollection
	}
	focusMetric := in.FocusMetric
	if focusMetric == "" {
		focusMetric = DefaultFocusMetric
	}
	if err := validateMetric(focusMetric); err != nil {
		return store.Experiment{}, err
	}

	id, err := s.store.CreateExperiment(ctx, store.CreateExperimentInput{
		Name:           in.Name,
		DatasetID:      in.DatasetID,
		Collection:     collection,
		BaselineEvalID: in.BaselineEvalID,
		FocusMetric:    focusMetric,
		Hypothesis:     in.Hypothesis,
		Notes:          in.Notes,
	})
	if err != nil {
		return store.Experiment{}, err
	}
	return s.store.GetExperiment(ctx, id)
}

func (s *Service) ListExperiments(ctx context.Context) ([]store.Experiment, error) {
	return s.store.ListExperiments(ctx)
}

func (s *Service) GetExperiment(ctx context.Context, id int64) (store.Experiment, error) {
	return s.store.GetExperiment(ctx, id)
}

func (s *Service) ListDatasets(ctx context.Context) ([]evalapi.Dataset, error) {
	return s.api.ListDatasets(ctx)
}

func (s *Service) StartRun(ctx context.Context, in StartRunInput) (StartRunResult, error) {
	if in.ExperimentID != 0 && in.Label != "" {
		if _, err := s.store.GetExperiment(ctx, in.ExperimentID); err != nil {
			return StartRunResult{}, err
		}
	}

	resp, err := s.api.StartEvaluation(ctx, evalapi.StartEvaluationRequest{
		DatasetID:      in.DatasetID,
		Collection:     in.Collection,
		Notes:          in.Notes,
		BaselineEvalID: in.BaselineEvalID,
		Rerank:         in.Rerank,
	})
	if err != nil {
		return StartRunResult{}, err
	}
	if in.ExperimentID != 0 && in.Label != "" {
		if err := s.store.AttachRun(ctx, in.ExperimentID, in.Label, resp.ID, in.Notes); err != nil {
			return StartRunResult{}, err
		}
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

func (s *Service) AttachRun(ctx context.Context, experimentID int64, label, evalID, notes string) error {
	return s.store.AttachRun(ctx, experimentID, label, evalID, notes)
}

func (s *Service) GetRun(ctx context.Context, evalID string) (evalapi.EvaluationDetail, error) {
	return s.api.GetEvaluation(ctx, evalID)
}

func (s *Service) Compare(ctx context.Context, in CompareInput) (evalapi.Comparison, error) {
	ids := append([]string(nil), in.EvalIDs...)
	if in.ExperimentID != 0 {
		resolved, err := s.resolveLabels(ctx, in.ExperimentID, in.Labels)
		if err != nil {
			return evalapi.Comparison{}, err
		}
		ids = append(ids, resolved...)
	}
	if err := validateCompareIDs(ids); err != nil {
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

func (s *Service) SummarizeExperiment(ctx context.Context, experimentID int64) (ExperimentSummary, error) {
	exp, err := s.store.GetExperiment(ctx, experimentID)
	if err != nil {
		return ExperimentSummary{}, err
	}

	summary := ExperimentSummary{Experiment: exp}
	evalIDs := make([]string, 0, len(exp.Runs))
	for i, runLabel := range exp.Runs {
		run, err := s.api.GetEvaluation(ctx, runLabel.EvalID)
		if err != nil {
			return ExperimentSummary{}, fmt.Errorf("get run %q (%s): %w", runLabel.Label, runLabel.EvalID, err)
		}
		labeledRun := LabeledRun{Label: runLabel.Label, Run: run}
		if i == 0 {
			summary.Baseline = &labeledRun
		} else {
			summary.Candidates = append(summary.Candidates, labeledRun)
		}
		evalIDs = append(evalIDs, runLabel.EvalID)

		metric := exp.FocusMetric
		if metric == "" {
			metric = DefaultFocusMetric
		}
		if err := validateMetric(metric); err != nil {
			return ExperimentSummary{}, fmt.Errorf("worst cases for %q: %w", runLabel.Label, err)
		}
		summary.WorstCases = append(summary.WorstCases, LabeledWorstCases{
			Label:            runLabel.Label,
			WorstCasesResult: worstCasesFromResults(runLabel.EvalID, metric, defaultWorstLimit, run.Results),
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

func (s *Service) RecordConclusion(ctx context.Context, experimentID int64, conclusion string) error {
	return s.store.RecordConclusion(ctx, experimentID, conclusion)
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

func (s *Service) resolveLabels(ctx context.Context, experimentID int64, labels []string) ([]string, error) {
	exp, err := s.store.GetExperiment(ctx, experimentID)
	if err != nil {
		return nil, err
	}
	byLabel := make(map[string]string, len(exp.Runs))
	known := make([]string, 0, len(exp.Runs))
	for _, run := range exp.Runs {
		byLabel[run.Label] = run.EvalID
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
		return nil, fmt.Errorf("experiment %d missing run labels %s; known labels: %s", experimentID, strings.Join(missing, ", "), strings.Join(known, ", "))
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
