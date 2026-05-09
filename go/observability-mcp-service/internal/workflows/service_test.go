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
	for _, want := range []string{"ai_agent_turns_by_outcome", "rag_stage_latency_p95", "ollama_latency_p95"} {
		if !slices.Contains(names, want) {
			t.Fatalf("signal names missing %s: %v", want, names)
		}
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
	err      error
}

func (f *fakeLoki) QueryLogs(_ context.Context, q observability.LogQuery) ([]observability.LogLine, bool, error) {
	f.services = append(f.services, q.Service)
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

func signalNames(signals []Signal) []string {
	names := make([]string, 0, len(signals))
	for _, signal := range signals {
		names = append(names, signal.Name)
	}
	return names
}
