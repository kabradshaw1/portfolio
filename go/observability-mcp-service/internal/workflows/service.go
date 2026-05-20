package workflows

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/kabradshaw1/portfolio/go/observability-mcp-service/internal/history"
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

type Service struct {
	prometheus Prometheus
	loki       Loki
	jaeger     Jaeger
	maxLogs    int
	now        func() time.Time

	historyStore history.Store
	autoCapture  bool
}

func NewService(prometheus Prometheus, loki Loki, jaeger Jaeger, maxLogs int) *Service {
	if maxLogs <= 0 {
		maxLogs = 100
	}
	return &Service{prometheus: prometheus, loki: loki, jaeger: jaeger, maxLogs: maxLogs, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) WithHistory(store history.Store, autoCapture bool) *Service {
	s.historyStore = store
	s.autoCapture = autoCapture
	return s
}

func (s *Service) GetSystemHealth(ctx context.Context, window time.Duration, capture CaptureOptions) EvidenceBundle {
	b := s.bundle("get_system_health", window)
	s.addPrometheusSignals(ctx, &b, systemHealthQueries)
	s.finalize(&b)
	s.maybeRecordSnapshot(ctx, &b, capture)
	return b
}

func (s *Service) InvestigateCheckout(ctx context.Context, window time.Duration, capture CaptureOptions) EvidenceBundle {
	b := s.bundle("investigate_checkout", window)
	s.addPrometheusSignals(ctx, &b, checkoutQueries)
	s.addLogs(ctx, &b, []string{"go-order-service", "go-cart-service", "go-payment-service", "go-product-service"}, "")
	s.finalize(&b)
	s.maybeRecordSnapshot(ctx, &b, capture)
	return b
}

func (s *Service) InvestigateAIPipeline(ctx context.Context, window time.Duration, capture CaptureOptions) EvidenceBundle {
	b := s.bundle("investigate_ai_pipeline", window)
	s.addPrometheusSignals(ctx, &b, aiPipelineQueries)
	s.addLogs(ctx, &b, []string{"go-ai-service", "chat", "ingestion", "debug", "eval"}, "")
	s.finalize(&b)
	s.maybeRecordSnapshot(ctx, &b, capture)
	return b
}

func (s *Service) InvestigateEvalRun(ctx context.Context, window time.Duration, evalID string, capture CaptureOptions) EvidenceBundle {
	b := s.bundle("investigate_eval_run", window)
	s.addPrometheusSignals(ctx, &b, evalRunQueries(evalID))
	s.addLogs(ctx, &b, []string{"eval"}, evalID)
	s.finalize(&b)
	s.maybeRecordSnapshot(ctx, &b, capture)
	return b
}

func (s *Service) InvestigateStreamingAnalytics(ctx context.Context, window time.Duration, capture CaptureOptions) EvidenceBundle {
	b := s.bundle("investigate_streaming_analytics", window)
	s.addPrometheusSignals(ctx, &b, streamingAnalyticsQueries)
	s.addLogs(ctx, &b, []string{"go-analytics-service"}, "")
	s.finalize(&b)
	s.maybeRecordSnapshot(ctx, &b, capture)
	return b
}

func (s *Service) GetServiceEvidence(ctx context.Context, service string, window time.Duration, traceID string, capture CaptureOptions) EvidenceBundle {
	b := s.bundle("get_service_evidence", window)
	if !AllowedService(service) {
		b.AddError("input", "validate_service", fmt.Sprintf("service %q is not allowlisted", service))
		s.finalize(&b)
		s.maybeRecordSnapshot(ctx, &b, capture)
		return b
	}
	s.addPrometheusSignals(ctx, &b, serviceQueries(service))
	s.addLogs(ctx, &b, []string{service}, "")
	if traceID != "" {
		s.addTrace(ctx, &b, traceID)
	}
	s.finalize(&b)
	s.maybeRecordSnapshot(ctx, &b, capture)
	return b
}

func (s *Service) ListIncidents(ctx context.Context, filter history.ListFilter) ([]history.IncidentSummary, error) {
	if s.historyStore == nil {
		return nil, errors.New("history store is disabled")
	}
	return s.historyStore.ListIncidents(ctx, filter)
}

func (s *Service) GetIncidentHistory(ctx context.Context, key string) (history.IncidentHistory, error) {
	if s.historyStore == nil {
		return history.IncidentHistory{}, errors.New("history store is disabled")
	}
	return s.historyStore.GetIncidentHistory(ctx, key)
}

func (s *Service) AddIncidentNote(ctx context.Context, input history.AddNoteInput) (history.Event, error) {
	if s.historyStore == nil {
		return history.Event{}, errors.New("history store is disabled")
	}
	return s.historyStore.AddIncidentNote(ctx, input)
}

func (s *Service) CompareEvidenceSnapshots(ctx context.Context, baselineID, candidateID int64) (EvidenceComparison, error) {
	if s.historyStore == nil {
		return EvidenceComparison{}, errors.New("history store is disabled")
	}
	if candidateID <= 0 {
		candidateID = baselineID
	}

	baselineSnapshot, err := s.historyStore.GetSnapshot(ctx, baselineID)
	if err != nil {
		return EvidenceComparison{}, fmt.Errorf("get baseline snapshot %d: %w", baselineID, err)
	}
	candidateSnapshot, err := s.historyStore.GetSnapshot(ctx, candidateID)
	if err != nil {
		return EvidenceComparison{}, fmt.Errorf("get candidate snapshot %d: %w", candidateID, err)
	}

	baseline, err := evidenceBundleFromSnapshot(baselineSnapshot)
	if err != nil {
		return EvidenceComparison{}, fmt.Errorf("decode baseline snapshot %d: %w", baselineID, err)
	}
	candidate, err := evidenceBundleFromSnapshot(candidateSnapshot)
	if err != nil {
		return EvidenceComparison{}, fmt.Errorf("decode candidate snapshot %d: %w", candidateID, err)
	}

	comparison := EvidenceComparison{
		BaselineSnapshotID:  baselineID,
		CandidateSnapshotID: candidateID,
		Summary:             []string{},
	}
	if baseline.Status != candidate.Status {
		comparison.StatusChange = fmt.Sprintf("status changed from %s to %s", baseline.Status, candidate.Status)
		comparison.Summary = append(comparison.Summary, comparison.StatusChange)
	}
	if baseline.Partial != candidate.Partial {
		comparison.PartialChange = fmt.Sprintf("partial changed from %t to %t", baseline.Partial, candidate.Partial)
		comparison.Summary = append(comparison.Summary, comparison.PartialChange)
	}

	comparison.CountChanges = compareCounts(baseline, candidate)
	for _, delta := range comparison.CountChanges {
		comparison.Summary = append(comparison.Summary, fmt.Sprintf("%s %s from %s to %s", delta.Name, delta.Direction, delta.Before, delta.After))
	}
	comparison.SourceChanges = compareSources(baseline.Sources, candidate.Sources)
	for _, delta := range comparison.SourceChanges {
		comparison.Summary = append(comparison.Summary, fmt.Sprintf("source %s %s from %s to %s", delta.Name, delta.Direction, delta.Before, delta.After))
	}
	comparison.SignalChanges = compareSignals(baseline.Signals, candidate.Signals)
	for _, delta := range comparison.SignalChanges {
		comparison.Summary = append(comparison.Summary, fmt.Sprintf("signal %s %s from %s to %s", delta.Name, delta.Direction, delta.Before, delta.After))
	}
	return comparison, nil
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

func (s *Service) maybeRecordSnapshot(ctx context.Context, b *EvidenceBundle, capture CaptureOptions) {
	if s.historyStore == nil {
		return
	}
	shouldPersist := s.autoCapture || capture.IncidentKey != ""
	if capture.Persist != nil {
		shouldPersist = *capture.Persist
	}
	if !shouldPersist {
		return
	}
	incidentKey := capture.IncidentKey
	if incidentKey == "" {
		incidentKey = derivedIncidentKey(b, capture)
	}
	bundleJSON, err := json.Marshal(b)
	if err != nil {
		b.History = &HistoryMetadata{IncidentKey: incidentKey, Warning: "marshal evidence bundle: " + err.Error()}
		return
	}
	critical, warnings := countFindings(b.Findings)
	snapshot, err := s.historyStore.RecordSnapshot(ctx, history.SnapshotInput{
		IncidentKey:    incidentKey,
		IncidentTitle:  capture.IncidentTitle,
		Severity:       capture.Severity,
		Service:        capture.Service,
		Tool:           b.Tool,
		WindowFrom:     b.Window.From,
		WindowTo:       b.Window.To,
		WindowDuration: b.Window.Duration,
		Status:         b.Status,
		Partial:        b.Partial,
		CriticalCount:  critical,
		WarningCount:   warnings,
		SignalCount:    len(b.Signals),
		LogCount:       len(b.Logs),
		TraceCount:     len(b.Traces),
		SourceStatuses: historySourceStatuses(b.Sources),
		BundleJSON:     bundleJSON,
	})
	if err != nil {
		b.History = &HistoryMetadata{IncidentKey: incidentKey, Warning: err.Error()}
		return
	}
	b.History = &HistoryMetadata{IncidentKey: incidentKey, SnapshotID: snapshot.ID}
}

func derivedIncidentKey(b *EvidenceBundle, capture CaptureOptions) string {
	scope := capture.Service
	if scope == "" {
		scope = "global"
	}
	return fmt.Sprintf("auto:%s:%s:%s", keyPart(b.Tool), keyPart(scope), b.Window.To.UTC().Format("20060102T150405Z"))
}

func keyPart(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	var out strings.Builder
	previousDash := false
	for _, r := range value {
		writeDash := false
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out.WriteRune(r)
			previousDash = false
		case r == '-' || r == '_' || unicode.IsSpace(r):
			writeDash = true
		}
		if writeDash && out.Len() > 0 && !previousDash {
			out.WriteByte('-')
			previousDash = true
		}
	}
	return strings.Trim(out.String(), "-")
}

func countFindings(findings []Finding) (int, int) {
	var critical int
	var warnings int
	for _, finding := range findings {
		switch finding.Severity {
		case "critical":
			critical++
		case "warning":
			warnings++
		}
	}
	return critical, warnings
}

func historySourceStatuses(sources []SourceStatus) []history.SourceStatus {
	statuses := make([]history.SourceStatus, 0, len(sources))
	for _, source := range sources {
		statuses = append(statuses, history.SourceStatus{Name: source.Name, Status: source.Status})
	}
	return statuses
}

func evidenceBundleFromSnapshot(snapshot history.Snapshot) (EvidenceBundle, error) {
	var bundle EvidenceBundle
	if err := json.Unmarshal(snapshot.BundleJSON, &bundle); err != nil {
		return EvidenceBundle{}, err
	}
	return bundle, nil
}

func compareCounts(baseline, candidate EvidenceBundle) []ComparisonDelta {
	counts := []struct {
		name   string
		before int
		after  int
	}{
		{name: "finding_count", before: len(baseline.Findings), after: len(candidate.Findings)},
		{name: "log_count", before: len(baseline.Logs), after: len(candidate.Logs)},
		{name: "trace_count", before: len(baseline.Traces), after: len(candidate.Traces)},
	}
	deltas := make([]ComparisonDelta, 0, len(counts))
	for _, count := range counts {
		if count.before == count.after {
			continue
		}
		deltas = append(deltas, ComparisonDelta{
			Name:      count.name,
			Before:    strconv.Itoa(count.before),
			After:     strconv.Itoa(count.after),
			Direction: numericDirection(float64(count.before), float64(count.after)),
		})
	}
	return deltas
}

func compareSources(baseline, candidate []SourceStatus) []ComparisonDelta {
	before := sourceStatusByName(baseline)
	after := sourceStatusByName(candidate)
	names := sortedUnionKeys(before, after)
	deltas := make([]ComparisonDelta, 0, len(names))
	for _, name := range names {
		beforeStatus, beforeOK := before[name]
		afterStatus, afterOK := after[name]
		if beforeOK && afterOK && beforeStatus == afterStatus {
			continue
		}
		deltas = append(deltas, ComparisonDelta{
			Name:      name,
			Before:    beforeStatus,
			After:     afterStatus,
			Direction: presenceDirection(beforeOK, afterOK),
		})
	}
	return deltas
}

func compareSignals(baseline, candidate []Signal) []ComparisonDelta {
	before := signalValuesByName(baseline)
	after := signalValuesByName(candidate)
	names := sortedUnionKeys(before, after)
	deltas := make([]ComparisonDelta, 0, len(names))
	for _, name := range names {
		beforeValue, beforeOK := before[name]
		afterValue, afterOK := after[name]
		if beforeOK && afterOK && beforeValue == afterValue {
			continue
		}
		deltas = append(deltas, ComparisonDelta{
			Name:      name,
			Before:    beforeValue,
			After:     afterValue,
			Direction: signalDirection(beforeValue, afterValue, beforeOK, afterOK),
		})
	}
	return deltas
}

func sourceStatusByName(sources []SourceStatus) map[string]string {
	statuses := make(map[string]string, len(sources))
	for _, source := range sources {
		statuses[source.Name] = source.Status
	}
	return statuses
}

func signalValuesByName(signals []Signal) map[string]string {
	values := make(map[string]string, len(signals))
	for _, signal := range signals {
		if signal.Value == nil {
			continue
		}
		values[signal.Name] = strconv.FormatFloat(*signal.Value, 'f', -1, 64)
	}
	return values
}

func sortedUnionKeys(left, right map[string]string) []string {
	seen := make(map[string]struct{}, len(left)+len(right))
	for key := range left {
		seen[key] = struct{}{}
	}
	for key := range right {
		seen[key] = struct{}{}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func presenceDirection(beforeOK, afterOK bool) string {
	switch {
	case !beforeOK && afterOK:
		return "added"
	case beforeOK && !afterOK:
		return "removed"
	default:
		return "changed"
	}
}

func signalDirection(before, after string, beforeOK, afterOK bool) string {
	if beforeOK && afterOK {
		beforeValue, beforeErr := strconv.ParseFloat(before, 64)
		afterValue, afterErr := strconv.ParseFloat(after, 64)
		if beforeErr == nil && afterErr == nil && beforeValue != afterValue {
			return numericDirection(beforeValue, afterValue)
		}
	}
	return presenceDirection(beforeOK, afterOK)
}

func numericDirection(before, after float64) string {
	if after > before {
		return "increased"
	}
	return "decreased"
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
	hasData := len(b.Signals) > 0 || len(b.Logs) > 0 || len(b.Traces) > 0
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
