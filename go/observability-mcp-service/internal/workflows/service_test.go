package workflows

import (
	"context"
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
	snapshots []history.SnapshotInput
	err       error
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

func (f *fakeHistoryStore) GetSnapshot(context.Context, int64) (history.Snapshot, error) {
	return history.Snapshot{}, f.err
}

func signalNames(signals []Signal) []string {
	names := make([]string, 0, len(signals))
	for _, signal := range signals {
		names = append(names, signal.Name)
	}
	return names
}
