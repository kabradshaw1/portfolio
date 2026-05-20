package workflows

import (
	"context"
	"fmt"
	"time"

	"github.com/kabradshaw1/portfolio/go/observability-mcp-service/internal/observability"
)

type Prometheus interface {
	Query(context.Context, string) ([]observability.MetricSample, error)
}

type Loki interface {
	QueryLogs(context.Context, observability.LogQuery) ([]observability.LogLine, bool, error)
}

type Jaeger interface {
	Trace(context.Context, string) (observability.TraceSummary, error)
}

type GrafanaAlerting interface {
	ActiveAlerts(context.Context) ([]observability.AlertInstance, error)
	AlertRules(context.Context) ([]observability.AlertRule, error)
}

type Service struct {
	prometheus      Prometheus
	loki            Loki
	jaeger          Jaeger
	grafanaAlerting GrafanaAlerting
	maxLogs         int
	now             func() time.Time
}

func NewService(prometheus Prometheus, loki Loki, jaeger Jaeger, maxLogs int) *Service {
	if maxLogs <= 0 {
		maxLogs = 100
	}
	return &Service{prometheus: prometheus, loki: loki, jaeger: jaeger, maxLogs: maxLogs, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) SetGrafanaAlerting(alerting GrafanaAlerting) {
	s.grafanaAlerting = alerting
}

func (s *Service) GetSystemHealth(ctx context.Context, window time.Duration) EvidenceBundle {
	b := s.bundle("get_system_health", window)
	s.addPrometheusSignals(ctx, &b, systemHealthQueries)
	s.addGrafanaAlerting(ctx, &b)
	s.finalize(&b)
	return b
}

func (s *Service) InvestigateCheckout(ctx context.Context, window time.Duration) EvidenceBundle {
	b := s.bundle("investigate_checkout", window)
	s.addPrometheusSignals(ctx, &b, checkoutQueries)
	s.addLogs(ctx, &b, []string{"go-order-service", "go-cart-service", "go-payment-service", "go-product-service"}, "")
	s.finalize(&b)
	return b
}

func (s *Service) InvestigateAIPipeline(ctx context.Context, window time.Duration) EvidenceBundle {
	b := s.bundle("investigate_ai_pipeline", window)
	s.addPrometheusSignals(ctx, &b, aiPipelineQueries)
	s.addLogs(ctx, &b, []string{"go-ai-service", "chat", "ingestion", "debug", "eval"}, "")
	s.finalize(&b)
	return b
}

func (s *Service) InvestigateEvalRun(ctx context.Context, window time.Duration, evalID string) EvidenceBundle {
	b := s.bundle("investigate_eval_run", window)
	s.addPrometheusSignals(ctx, &b, evalRunQueries(evalID))
	s.addLogs(ctx, &b, []string{"eval"}, evalID)
	s.finalize(&b)
	return b
}

func (s *Service) InvestigateStreamingAnalytics(ctx context.Context, window time.Duration) EvidenceBundle {
	b := s.bundle("investigate_streaming_analytics", window)
	s.addPrometheusSignals(ctx, &b, streamingAnalyticsQueries)
	s.addLogs(ctx, &b, []string{"go-analytics-service"}, "")
	s.finalize(&b)
	return b
}

func (s *Service) GetServiceEvidence(ctx context.Context, service string, window time.Duration, traceID string) EvidenceBundle {
	b := s.bundle("get_service_evidence", window)
	if !AllowedService(service) {
		b.AddError("input", "validate_service", fmt.Sprintf("service %q is not allowlisted", service))
		s.finalize(&b)
		return b
	}
	s.addPrometheusSignals(ctx, &b, serviceQueries(service))
	s.addLogs(ctx, &b, []string{service}, "")
	if traceID != "" {
		s.addTrace(ctx, &b, traceID)
	}
	s.finalize(&b)
	return b
}

func (s *Service) SearchLogs(ctx context.Context, service string, window time.Duration, pattern string) EvidenceBundle {
	b := s.bundle("search_logs", window)
	if !AllowedService(service) {
		b.AddError("input", "validate_service", fmt.Sprintf("service %q is not allowlisted", service))
		s.finalize(&b)
		return b
	}
	s.addLogs(ctx, &b, []string{service}, pattern)
	s.finalize(&b)
	return b
}

func (s *Service) GetTrace(ctx context.Context, traceID string) EvidenceBundle {
	b := s.bundle("get_trace", 0)
	s.addTrace(ctx, &b, traceID)
	s.finalize(&b)
	return b
}

func (s *Service) bundle(tool string, window time.Duration) EvidenceBundle {
	if window <= 0 {
		window = 15 * time.Minute
	}
	now := s.now().UTC()
	b := NewBundle(tool, window.String())
	b.Window = Window{From: now.Add(-window), To: now, Duration: window.String()}
	return b
}

func (s *Service) addPrometheusSignals(ctx context.Context, b *EvidenceBundle, queries []querySpec) {
	if s.prometheus == nil {
		b.Sources = append(b.Sources, SourceStatus{Name: "prometheus", Status: "skipped"})
		return
	}
	sourceOK := false
	for _, query := range queries {
		samples, err := s.prometheus.Query(ctx, query.Query)
		if err != nil {
			b.AddError("prometheus", query.Name, err.Error())
			continue
		}
		sourceOK = true
		signal := Signal{Name: query.Name, Query: query.Query, Samples: samples, Unit: query.Unit, Description: query.Description}
		if len(samples) > 0 {
			v := samples[0].Value
			signal.Value = &v
		}
		b.Signals = append(b.Signals, signal)
		s.addMetricFindings(b, signal)
	}
	status := "error"
	if sourceOK {
		status = "ok"
	}
	b.Sources = append(b.Sources, SourceStatus{Name: "prometheus", Status: status})
}

func (s *Service) addLogs(ctx context.Context, b *EvidenceBundle, services []string, pattern string) {
	if s.loki == nil {
		b.Sources = append(b.Sources, SourceStatus{Name: "loki", Status: "skipped"})
		return
	}
	sourceOK := false
	for _, service := range services {
		lines, truncated, err := s.loki.QueryLogs(ctx, observability.LogQuery{Service: service, Pattern: pattern, Start: b.Window.From, End: b.Window.To, Limit: s.maxLogs})
		if err != nil {
			b.AddError("loki", "query_logs:"+service, err.Error())
			continue
		}
		sourceOK = true
		b.Logs = append(b.Logs, lines...)
		if len(lines) > 0 {
			b.Findings = append(b.Findings, Finding{Severity: "warning", Title: "recent warning or error logs", Evidence: fmt.Sprintf("%d log lines for %s", len(lines), service)})
		}
		if truncated {
			b.Findings = append(b.Findings, Finding{Severity: "warning", Title: "log results truncated", Evidence: fmt.Sprintf("log search for %s exceeded %d lines", service, s.maxLogs)})
		}
	}
	status := "error"
	if sourceOK {
		status = "ok"
	}
	b.Sources = append(b.Sources, SourceStatus{Name: "loki", Status: status})
}

func (s *Service) addTrace(ctx context.Context, b *EvidenceBundle, traceID string) {
	if s.jaeger == nil {
		b.Sources = append(b.Sources, SourceStatus{Name: "jaeger", Status: "skipped"})
		return
	}
	trace, err := s.jaeger.Trace(ctx, traceID)
	if err != nil {
		b.AddError("jaeger", "trace", err.Error())
		b.Sources = append(b.Sources, SourceStatus{Name: "jaeger", Status: "error"})
		return
	}
	b.Sources = append(b.Sources, SourceStatus{Name: "jaeger", Status: "ok"})
	b.Traces = append(b.Traces, trace)
	for _, span := range trace.Spans {
		if span.Error {
			b.Findings = append(b.Findings, Finding{Severity: "warning", Title: "trace contains error span", Evidence: span.Operation})
			return
		}
	}
}

func (s *Service) addGrafanaAlerting(ctx context.Context, b *EvidenceBundle) {
	if s.grafanaAlerting == nil {
		b.Sources = append(b.Sources, SourceStatus{Name: "grafana_alerting", Status: "skipped"})
		return
	}
	alerts, err := s.grafanaAlerting.ActiveAlerts(ctx)
	if err != nil {
		b.AddError("grafana_alerting", "active_alerts", err.Error())
		b.Sources = append(b.Sources, SourceStatus{Name: "grafana_alerting", Status: "error"})
		return
	}
	rules, err := s.grafanaAlerting.AlertRules(ctx)
	if err != nil {
		b.AddError("grafana_alerting", "alert_rules", err.Error())
		b.Sources = append(b.Sources, SourceStatus{Name: "grafana_alerting", Status: "error"})
		return
	}
	b.Alerts = observability.AlertSummary{ActiveAlerts: alerts, Rules: matchingRules(alerts, rules)}
	b.Sources = append(b.Sources, SourceStatus{Name: "grafana_alerting", Status: "ok"})
	for _, alert := range alerts {
		if alert.State == "active" || alert.State == "firing" {
			title := "Grafana alert is firing"
			if alert.Name != "" {
				title = "Grafana alert is firing: " + alert.Name
			}
			b.Findings = append(b.Findings, Finding{Severity: "warning", Title: title, Evidence: alert.RuleUID})
		}
	}
}

func matchingRules(alerts []observability.AlertInstance, rules []observability.AlertRule) []observability.AlertRule {
	if len(alerts) == 0 || len(rules) == 0 {
		return nil
	}
	wanted := make(map[string]struct{}, len(alerts))
	for _, alert := range alerts {
		if alert.RuleUID != "" {
			wanted[alert.RuleUID] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return nil
	}
	matched := make([]observability.AlertRule, 0, len(wanted))
	for _, rule := range rules {
		if _, ok := wanted[rule.UID]; ok {
			matched = append(matched, rule)
		}
	}
	return matched
}

func (s *Service) addMetricFindings(b *EvidenceBundle, signal Signal) {
	if signal.Value == nil || *signal.Value <= 0 {
		return
	}
	switch signal.Name {
	case "kafka_consumer_lag":
		b.Findings = append(b.Findings, Finding{Severity: "warning", Title: "Kafka lag is non-zero", Evidence: fmt.Sprintf("%s=%g", signal.Name, *signal.Value)})
	case "rabbitmq_saga_dlq_depth":
		b.Findings = append(b.Findings, Finding{Severity: "critical", Title: "RabbitMQ saga DLQ has messages", Evidence: fmt.Sprintf("%s=%g", signal.Name, *signal.Value)})
	case "circuit_breaker_open":
		b.Findings = append(b.Findings, Finding{Severity: "warning", Title: "circuit breaker open", Evidence: fmt.Sprintf("%s=%g", signal.Name, *signal.Value)})
	}
}

func (s *Service) finalize(b *EvidenceBundle) {
	hasData := len(b.Signals) > 0 || len(b.Logs) > 0 || len(b.Traces) > 0 || len(b.Alerts.ActiveAlerts) > 0 || len(b.Alerts.Rules) > 0
	b.Status = "unknown"
	for _, finding := range b.Findings {
		if finding.Severity == "critical" {
			b.Status = "critical"
			return
		}
		if finding.Severity == "warning" {
			b.Status = "warning"
		}
	}
	if b.Status == "unknown" && hasData {
		b.Status = "ok"
	}
}
