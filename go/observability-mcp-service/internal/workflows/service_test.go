package workflows

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kabradshaw1/portfolio/go/observability-mcp-service/internal/history"
	"github.com/kabradshaw1/portfolio/go/observability-mcp-service/internal/observability"
)

func TestAllowedService(t *testing.T) {
	if !AllowedService("go-ai-service") {
		t.Fatal("go-ai-service should be allowed")
	}
	if !AllowedService("eval") {
		t.Fatal("eval should be allowed")
	}
	if AllowedService(".*") {
		t.Fatal("regex-like service should not be allowed")
	}
	if AllowedService("kube-system") {
		t.Fatal("unknown service should not be allowed")
	}
}

func TestEvidenceBundlePartial(t *testing.T) {
	bundle := NewBundle("get_system_health", "15m")
	bundle.AddError("prometheus", "query", "connection refused")
	if !bundle.Partial {
		t.Fatal("bundle should be partial after source error")
	}
	if len(bundle.Errors) != 1 {
		t.Fatalf("errors = %d", len(bundle.Errors))
	}
}

func TestGetServiceEvidenceRejectsUnknownServices(t *testing.T) {
	service := NewService(&fakePrometheus{}, &fakeLoki{}, &fakeJaeger{}, 10)
	got := service.GetServiceEvidence(context.Background(), "kube-system", time.Minute, "", CaptureOptions{})
	if !got.Partial || got.Status != "unknown" {
		t.Fatalf("bundle = %+v", got)
	}
}

func TestPrometheusFailurePlusLokiSuccessReturnsPartialWithLogs(t *testing.T) {
	service := NewService(&fakePrometheus{err: errors.New("prom down")}, &fakeLoki{}, nil, 10)
	got := service.GetServiceEvidence(context.Background(), "go-ai-service", time.Minute, "", CaptureOptions{})
	if !got.Partial || len(got.Errors) == 0 {
		t.Fatalf("expected partial errors: %+v", got)
	}
	if len(got.Logs) == 0 {
		t.Fatalf("expected logs: %+v", got)
	}
}

func TestInvestigateCheckoutQueriesCheckoutServices(t *testing.T) {
	prom := &fakePrometheus{}
	service := NewService(prom, nil, nil, 10)
	_ = service.InvestigateCheckout(context.Background(), time.Minute, CaptureOptions{})
	joined := strings.Join(prom.queries, "\n")
	for _, want := range []string{"order", "cart", "payment", "product"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("queries missing %s: %s", want, joined)
		}
	}
}

func TestInvestigateCheckoutPersistsWhenIncidentKeyProvided(t *testing.T) {
	store := &fakeHistoryStore{}
	service := NewService(&fakePrometheus{}, &fakeLoki{}, nil, 10).WithHistory(store, false)

	got := service.InvestigateCheckout(context.Background(), time.Minute, CaptureOptions{IncidentKey: "inc-1", IncidentTitle: "Checkout failures", Severity: "warning"})

	if got.History == nil || got.History.SnapshotID == 0 {
		t.Fatalf("expected persisted snapshot metadata, got %+v", got.History)
	}
	if len(store.snapshots) != 1 {
		t.Fatalf("snapshots recorded = %d", len(store.snapshots))
	}
	snapshot := store.snapshots[0]
	if snapshot.IncidentKey != "inc-1" || snapshot.IncidentTitle != "Checkout failures" || snapshot.Severity != "warning" {
		t.Fatalf("snapshot incident fields = %+v", snapshot)
	}
	if snapshot.Tool != "investigate_checkout" || snapshot.Status != got.Status || len(snapshot.BundleJSON) == 0 {
		t.Fatalf("snapshot evidence fields = %+v", snapshot)
	}
}

func TestPersistenceFailureAddsBundleWarningButKeepsEvidence(t *testing.T) {
	store := &fakeHistoryStore{err: errors.New("history unavailable")}
	service := NewService(&fakePrometheus{}, &fakeLoki{}, nil, 10).WithHistory(store, false)

	got := service.InvestigateCheckout(context.Background(), time.Minute, CaptureOptions{IncidentKey: "inc-1"})

	if got.History == nil || got.History.Warning != "history unavailable" {
		t.Fatalf("expected history warning, got %+v", got.History)
	}
	if len(got.Signals) == 0 || len(got.Logs) == 0 {
		t.Fatalf("expected evidence to be preserved: %+v", got)
	}
	if len(store.snapshots) != 1 {
		t.Fatalf("snapshots recorded = %d", len(store.snapshots))
	}
}

func TestHistoryDisabledDoesNotPersist(t *testing.T) {
	store := &fakeHistoryStore{}
	service := NewService(&fakePrometheus{}, &fakeLoki{}, nil, 10).WithHistory(store, false)

	got := service.InvestigateCheckout(context.Background(), time.Minute, CaptureOptions{})

	if got.History != nil {
		t.Fatalf("expected no history metadata, got %+v", got.History)
	}
	if len(store.snapshots) != 0 {
		t.Fatalf("snapshots recorded = %d", len(store.snapshots))
	}
}

func TestAutoCaptureDerivesIncidentKeyWhenMissing(t *testing.T) {
	store := &fakeHistoryStore{}
	service := NewService(&fakePrometheus{}, nil, nil, 10).WithHistory(store, true)
	service.now = func() time.Time { return time.Date(2026, 5, 20, 12, 34, 56, 0, time.UTC) }

	got := service.GetSystemHealth(context.Background(), time.Minute, CaptureOptions{})

	const wantKey = "auto:get-system-health:global:20260520T123456Z"
	if got.History == nil || got.History.IncidentKey != wantKey || got.History.SnapshotID == 0 {
		t.Fatalf("history metadata = %+v, want key %q with snapshot", got.History, wantKey)
	}
	if len(store.snapshots) != 1 {
		t.Fatalf("snapshots recorded = %d", len(store.snapshots))
	}
	if store.snapshots[0].IncidentKey != wantKey {
		t.Fatalf("snapshot incident key = %q, want %q", store.snapshots[0].IncidentKey, wantKey)
	}
}

func TestPersistTrueDerivesIncidentKeyWhenMissing(t *testing.T) {
	store := &fakeHistoryStore{}
	service := NewService(&fakePrometheus{}, nil, nil, 10).WithHistory(store, false)
	service.now = func() time.Time { return time.Date(2026, 5, 20, 12, 34, 56, 0, time.UTC) }
	persist := true

	got := service.GetServiceEvidence(context.Background(), "go-ai-service", time.Minute, "", CaptureOptions{Service: "Go AI Service", Persist: &persist})

	const wantKey = "auto:get-service-evidence:go-ai-service:20260520T123456Z"
	if got.History == nil || got.History.IncidentKey != wantKey || got.History.SnapshotID == 0 {
		t.Fatalf("history metadata = %+v, want key %q with snapshot", got.History, wantKey)
	}
	if len(store.snapshots) != 1 {
		t.Fatalf("snapshots recorded = %d", len(store.snapshots))
	}
	if store.snapshots[0].IncidentKey != wantKey {
		t.Fatalf("snapshot incident key = %q, want %q", store.snapshots[0].IncidentKey, wantKey)
	}
}

func TestPersistFalseSuppressesPersistence(t *testing.T) {
	store := &fakeHistoryStore{}
	service := NewService(&fakePrometheus{}, nil, nil, 10).WithHistory(store, true)
	persist := false

	got := service.GetSystemHealth(context.Background(), time.Minute, CaptureOptions{IncidentKey: "inc-1", Persist: &persist})

	if got.History != nil {
		t.Fatalf("expected no history metadata, got %+v", got.History)
	}
	if len(store.snapshots) != 0 {
		t.Fatalf("snapshots recorded = %d", len(store.snapshots))
	}
}

func TestNilHistoryStoreDoesNotPersist(t *testing.T) {
	service := NewService(&fakePrometheus{}, nil, nil, 10)

	got := service.GetSystemHealth(context.Background(), time.Minute, CaptureOptions{IncidentKey: "inc-1"})

	if got.History != nil {
		t.Fatalf("expected no history metadata, got %+v", got.History)
	}
}

func TestCompareEvidenceSnapshotsReportsStatusAndCountDeltas(t *testing.T) {
	store := &fakeHistoryStore{snapshotsByID: map[int64]history.Snapshot{
		1: snapshotWithBundle(t, 1, EvidenceBundle{
			Status:   "warning",
			Partial:  true,
			Findings: []Finding{{Severity: "warning", Title: "slow", Evidence: "latency"}},
			Logs:     []observability.LogLine{{Line: "first"}, {Line: "second"}, {Line: "third"}},
			Traces:   []observability.TraceSummary{{TraceID: "trace-1"}, {TraceID: "trace-2"}},
		}),
		2: snapshotWithBundle(t, 2, EvidenceBundle{
			Status:   "ok",
			Partial:  false,
			Findings: []Finding{{Severity: "critical", Title: "error", Evidence: "rate"}, {Severity: "warning", Title: "retry", Evidence: "rate"}},
			Logs:     []observability.LogLine{{Line: "only"}},
			Traces:   []observability.TraceSummary{{TraceID: "trace-1"}},
		}),
	}}
	service := NewService(nil, nil, nil, 10).WithHistory(store, false)

	got, err := service.CompareEvidenceSnapshots(context.Background(), 1, 2)

	if err != nil {
		t.Fatalf("CompareEvidenceSnapshots returned error: %v", err)
	}
	if got.BaselineSnapshotID != 1 || got.CandidateSnapshotID != 2 {
		t.Fatalf("snapshot ids = %d, %d", got.BaselineSnapshotID, got.CandidateSnapshotID)
	}
	if got.StatusChange != "status changed from warning to ok" {
		t.Fatalf("status change = %q", got.StatusChange)
	}
	if got.PartialChange != "partial changed from true to false" {
		t.Fatalf("partial change = %q", got.PartialChange)
	}
	assertDelta(t, got.CountChanges, "finding_count", "1", "2", "increased")
	assertDelta(t, got.CountChanges, "log_count", "3", "1", "decreased")
	assertDelta(t, got.CountChanges, "trace_count", "2", "1", "decreased")
	assertSummaryContains(t, got.Summary, "status changed from warning to ok")
	assertSummaryContains(t, got.Summary, "log_count decreased from 3 to 1")
}

func TestCompareEvidenceSnapshotsReportsSourceAvailabilityChanges(t *testing.T) {
	store := &fakeHistoryStore{snapshotsByID: map[int64]history.Snapshot{
		1: snapshotWithBundle(t, 1, EvidenceBundle{
			Sources: []SourceStatus{
				{Name: "loki", Status: "error"},
				{Name: "prometheus", Status: "ok"},
			},
		}),
		2: snapshotWithBundle(t, 2, EvidenceBundle{
			Sources: []SourceStatus{
				{Name: "loki", Status: "ok"},
				{Name: "jaeger", Status: "ok"},
			},
		}),
	}}
	service := NewService(nil, nil, nil, 10).WithHistory(store, false)

	got, err := service.CompareEvidenceSnapshots(context.Background(), 1, 2)

	if err != nil {
		t.Fatalf("CompareEvidenceSnapshots returned error: %v", err)
	}
	assertDelta(t, got.SourceChanges, "loki", "error", "ok", "changed")
	assertDelta(t, got.SourceChanges, "prometheus", "ok", "", "removed")
	assertDelta(t, got.SourceChanges, "jaeger", "", "ok", "added")
	assertSummaryContains(t, got.Summary, "source loki changed from error to ok")
}

func TestCompareEvidenceSnapshotsReportsSignalValueDeltas(t *testing.T) {
	beforeValue := 12.5
	afterValue := 2.25
	newValue := 1.5
	store := &fakeHistoryStore{snapshotsByID: map[int64]history.Snapshot{
		1: snapshotWithBundle(t, 1, EvidenceBundle{
			Signals: []Signal{
				{Name: "error_rate", Value: &beforeValue},
				{Name: "latency_p95", Value: &newValue},
			},
		}),
		2: snapshotWithBundle(t, 2, EvidenceBundle{
			Signals: []Signal{
				{Name: "error_rate", Value: &afterValue},
				{Name: "queue_depth", Value: &newValue},
			},
		}),
	}}
	service := NewService(nil, nil, nil, 10).WithHistory(store, false)

	got, err := service.CompareEvidenceSnapshots(context.Background(), 1, 2)

	if err != nil {
		t.Fatalf("CompareEvidenceSnapshots returned error: %v", err)
	}
	assertDelta(t, got.SignalChanges, "error_rate", "12.5", "2.25", "decreased")
	assertDelta(t, got.SignalChanges, "latency_p95", "1.5", "", "removed")
	assertDelta(t, got.SignalChanges, "queue_depth", "", "1.5", "added")
	assertSummaryContains(t, got.Summary, "signal error_rate decreased from 12.5 to 2.25")
}

func TestInvestigateAIPipelineIncludesExpectedQueryNames(t *testing.T) {
	service := NewService(&fakePrometheus{}, nil, nil, 10)
	got := service.InvestigateAIPipeline(context.Background(), time.Minute, CaptureOptions{})
	names := signalNames(got.Signals)
	for _, want := range []string{"ai_agent_turns_by_outcome", "rag_stage_latency_p95", "ollama_latency_p95", "eval_runs_total"} {
		if !slices.Contains(names, want) {
			t.Fatalf("signal names missing %s: %v", want, names)
		}
	}
}

func TestInvestigateEvalRunQueriesEvalSignalsAndLogs(t *testing.T) {
	prom := &fakePrometheus{}
	loki := &fakeLoki{}
	service := NewService(prom, loki, nil, 10)

	got := service.InvestigateEvalRun(context.Background(), time.Minute, "eval-123", CaptureOptions{})

	names := signalNames(got.Signals)
	for _, want := range []string{"eval_runs_total", "eval_item_duration_p95", "eval_upstream_failures"} {
		if !slices.Contains(names, want) {
			t.Fatalf("signal names missing %s: %v", want, names)
		}
	}
	if len(loki.services) != 1 || loki.services[0] != "eval" {
		t.Fatalf("loki services = %#v", loki.services)
	}
	if len(loki.patterns) != 1 || loki.patterns[0] != "eval-123" {
		t.Fatalf("loki patterns = %#v", loki.patterns)
	}
}

func TestInvestigateStreamingAnalyticsIncludesKafkaSignals(t *testing.T) {
	service := NewService(&fakePrometheus{}, nil, nil, 10)
	got := service.InvestigateStreamingAnalytics(context.Background(), time.Minute, CaptureOptions{})
	names := signalNames(got.Signals)
	for _, want := range []string{"kafka_consumer_lag", "kafka_consumer_errors"} {
		if !slices.Contains(names, want) {
			t.Fatalf("signal names missing %s: %v", want, names)
		}
	}
}

func TestSearchLogsEnforcesServiceAllowlist(t *testing.T) {
	service := NewService(nil, &fakeLoki{}, nil, 10)
	got := service.SearchLogs(context.Background(), ".*", time.Minute, "")
	if !got.Partial || len(got.Logs) != 0 {
		t.Fatalf("bundle = %+v", got)
	}
}

func TestGetTraceAddsTraceSummary(t *testing.T) {
	service := NewService(nil, nil, &fakeJaeger{}, 10)
	got := service.GetTrace(context.Background(), "trace-1")
	if len(got.Traces) != 1 || got.Traces[0].TraceID != "trace-1" {
		t.Fatalf("bundle = %+v", got)
	}
	if got.Status != "warning" {
		t.Fatalf("expected warning for error span, got %s", got.Status)
	}
}

type fakePrometheus struct {
	queries []string
	err     error
}

func (f *fakePrometheus) Query(_ context.Context, query string) ([]observability.MetricSample, error) {
	f.queries = append(f.queries, query)
	if f.err != nil {
		return nil, f.err
	}
	return []observability.MetricSample{{Metric: map[string]string{"service": "go-ai-service"}, Value: 1, Time: time.Now().UTC()}}, nil
}

type fakeLoki struct {
	services []string
	patterns []string
	err      error
}

func (f *fakeLoki) QueryLogs(_ context.Context, q observability.LogQuery) ([]observability.LogLine, bool, error) {
	f.services = append(f.services, q.Service)
	f.patterns = append(f.patterns, q.Pattern)
	if f.err != nil {
		return nil, false, f.err
	}
	return []observability.LogLine{{Time: time.Now().UTC(), Labels: map[string]string{"service": q.Service}, Line: "error line"}}, false, nil
}

type fakeJaeger struct {
	err error
}

func (f *fakeJaeger) Trace(_ context.Context, traceID string) (observability.TraceSummary, error) {
	if f.err != nil {
		return observability.TraceSummary{}, f.err
	}
	return observability.TraceSummary{TraceID: traceID, SpanCount: 1, Spans: []observability.TraceSpan{{Operation: "op", Error: true}}}, nil
}

type fakeHistoryStore struct {
	snapshots     []history.SnapshotInput
	snapshotsByID map[int64]history.Snapshot
	err           error
}

func (f *fakeHistoryStore) Close() error { return nil }

func (f *fakeHistoryStore) Migrate(context.Context) error { return nil }

func (f *fakeHistoryStore) RecordSnapshot(_ context.Context, in history.SnapshotInput) (history.Snapshot, error) {
	f.snapshots = append(f.snapshots, in)
	if f.err != nil {
		return history.Snapshot{}, f.err
	}
	return history.Snapshot{ID: int64(len(f.snapshots)), Tool: in.Tool, Status: in.Status}, nil
}

func (f *fakeHistoryStore) ListIncidents(context.Context, history.ListFilter) ([]history.IncidentSummary, error) {
	return nil, f.err
}

func (f *fakeHistoryStore) GetIncidentHistory(context.Context, string) (history.IncidentHistory, error) {
	return history.IncidentHistory{}, f.err
}

func (f *fakeHistoryStore) AddIncidentNote(context.Context, history.AddNoteInput) (history.Event, error) {
	return history.Event{}, f.err
}

func (f *fakeHistoryStore) GetSnapshot(_ context.Context, id int64) (history.Snapshot, error) {
	if f.err != nil {
		return history.Snapshot{}, f.err
	}
	snapshot, ok := f.snapshotsByID[id]
	if !ok {
		return history.Snapshot{}, history.ErrNotFound
	}
	return snapshot, nil
}

func signalNames(signals []Signal) []string {
	names := make([]string, 0, len(signals))
	for _, signal := range signals {
		names = append(names, signal.Name)
	}
	return names
}

func snapshotWithBundle(t *testing.T, id int64, bundle EvidenceBundle) history.Snapshot {
	t.Helper()
	bundleJSON, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("marshal bundle: %v", err)
	}
	return history.Snapshot{ID: id, BundleJSON: bundleJSON}
}

func assertDelta(t *testing.T, deltas []ComparisonDelta, name, before, after, direction string) {
	t.Helper()
	for _, delta := range deltas {
		if delta.Name == name {
			if delta.Before != before || delta.After != after || delta.Direction != direction {
				t.Fatalf("delta %s = %+v, want before=%q after=%q direction=%q", name, delta, before, after, direction)
			}
			return
		}
	}
	t.Fatalf("missing delta %q in %+v", name, deltas)
}

func assertSummaryContains(t *testing.T, summaries []string, want string) {
	t.Helper()
	if !slices.Contains(summaries, want) {
		t.Fatalf("summary missing %q in %+v", want, summaries)
	}
}
