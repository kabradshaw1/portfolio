package workflows

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

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
	got := service.GetServiceEvidence(context.Background(), "kube-system", time.Minute, "")
	if !got.Partial || got.Status != "unknown" {
		t.Fatalf("bundle = %+v", got)
	}
}

func TestPrometheusFailurePlusLokiSuccessReturnsPartialWithLogs(t *testing.T) {
	service := NewService(&fakePrometheus{err: errors.New("prom down")}, &fakeLoki{}, nil, 10)
	got := service.GetServiceEvidence(context.Background(), "go-ai-service", time.Minute, "")
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
	_ = service.InvestigateCheckout(context.Background(), time.Minute)
	joined := strings.Join(prom.queries, "\n")
	for _, want := range []string{"order", "cart", "payment", "product"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("queries missing %s: %s", want, joined)
		}
	}
}

func TestInvestigateAIPipelineIncludesExpectedQueryNames(t *testing.T) {
	service := NewService(&fakePrometheus{}, nil, nil, 10)
	got := service.InvestigateAIPipeline(context.Background(), time.Minute)
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

	got := service.InvestigateEvalRun(context.Background(), time.Minute, "eval-123")

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
	got := service.InvestigateStreamingAnalytics(context.Background(), time.Minute)
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

func TestGetSystemHealthIncludesGrafanaAlerting(t *testing.T) {
	alerting := &fakeGrafanaAlerting{
		alerts: []observability.AlertInstance{{
			Name:    "HighErrorRate",
			State:   "active",
			RuleUID: "rule-123",
			Labels:  map[string]string{"alertname": "HighErrorRate"},
		}},
		rules: []observability.AlertRule{{
			UID:   "rule-123",
			Title: "HighErrorRate",
		}},
	}
	service := NewService(&fakePrometheus{}, nil, nil, 10)
	service.SetGrafanaAlerting(alerting)

	got := service.GetSystemHealth(context.Background(), time.Minute)

	if len(got.Alerts.ActiveAlerts) != 1 {
		t.Fatalf("alerts = %+v", got.Alerts)
	}
	if len(got.Alerts.Rules) != 1 {
		t.Fatalf("rules = %+v", got.Alerts)
	}
	if got.Status != "warning" {
		t.Fatalf("status = %s", got.Status)
	}
	if !slices.Contains(sourceStatuses(got.Sources), "grafana_alerting:ok") {
		t.Fatalf("sources = %+v", got.Sources)
	}
}

func TestGetSystemHealthSkipsGrafanaAlertingWhenUnconfigured(t *testing.T) {
	service := NewService(&fakePrometheus{}, nil, nil, 10)
	got := service.GetSystemHealth(context.Background(), time.Minute)
	if len(got.Alerts.ActiveAlerts) != 0 || len(got.Alerts.Rules) != 0 {
		t.Fatalf("alerts = %+v", got.Alerts)
	}
	if !slices.Contains(sourceStatuses(got.Sources), "grafana_alerting:skipped") {
		t.Fatalf("sources = %+v", got.Sources)
	}
}

func TestGetSystemHealthGrafanaAlertingFailureIsPartial(t *testing.T) {
	service := NewService(&fakePrometheus{}, nil, nil, 10)
	service.SetGrafanaAlerting(&fakeGrafanaAlerting{err: errors.New("grafana down")})

	got := service.GetSystemHealth(context.Background(), time.Minute)

	if !got.Partial {
		t.Fatalf("bundle should be partial: %+v", got)
	}
	if len(got.Signals) == 0 {
		t.Fatalf("prometheus evidence should remain: %+v", got)
	}
	if !slices.Contains(sourceStatuses(got.Sources), "grafana_alerting:error") {
		t.Fatalf("sources = %+v", got.Sources)
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
	return []observability.MetricSample{{Metric: map[string]string{"service": "go-ai-service"}, Value: 0, Time: time.Now().UTC()}}, nil
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

type fakeGrafanaAlerting struct {
	alerts []observability.AlertInstance
	rules  []observability.AlertRule
	err    error
}

func (f *fakeGrafanaAlerting) ActiveAlerts(_ context.Context) ([]observability.AlertInstance, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.alerts, nil
}

func (f *fakeGrafanaAlerting) AlertRules(_ context.Context) ([]observability.AlertRule, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.rules, nil
}

func signalNames(signals []Signal) []string {
	names := make([]string, 0, len(signals))
	for _, signal := range signals {
		names = append(names, signal.Name)
	}
	return names
}

func sourceStatuses(sources []SourceStatus) []string {
	statuses := make([]string, 0, len(sources))
	for _, source := range sources {
		statuses = append(statuses, source.Name+":"+source.Status)
	}
	return statuses
}
