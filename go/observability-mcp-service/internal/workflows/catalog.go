package workflows

var allowedServices = map[string]struct{}{
	"go-ai-service":        {},
	"go-order-service":     {},
	"go-cart-service":      {},
	"go-payment-service":   {},
	"go-product-service":   {},
	"go-analytics-service": {},
	"chat":                 {},
	"ingestion":            {},
	"debug":                {},
	"eval":                 {},
}

func AllowedService(name string) bool {
	_, ok := allowedServices[name]
	return ok
}

type querySpec struct {
	Name        string
	Query       string
	Unit        string
	Description string
}

var (
	systemHealthQueries = []querySpec{
		{Name: "go_service_request_rate", Query: `sum by (service) (rate(http_requests_total{service=~"go-.*"}[5m]))`, Unit: "requests/s"},
		{Name: "go_service_error_rate", Query: `sum by (service) (rate(http_requests_total{service=~"go-.*",status=~"5.."}[5m]))`, Unit: "errors/s"},
		{Name: "go_service_p95_latency", Query: `histogram_quantile(0.95, sum by (le, service) (rate(http_request_duration_seconds_bucket{service=~"go-.*"}[5m])))`, Unit: "seconds"},
		{Name: "kubernetes_pod_restarts", Query: `sum by (pod) (increase(kube_pod_container_status_restarts_total[15m]))`, Unit: "restarts"},
		{Name: "kafka_consumer_lag", Query: `max(kafka_consumer_lag)`, Unit: "messages"},
		{Name: "kafka_consumer_errors", Query: `sum(rate(kafka_consumer_errors_total[5m]))`, Unit: "errors/s"},
		{Name: "rabbitmq_saga_dlq_depth", Query: `sum(rabbitmq_queue_messages{queue=~".*dlq.*|.*DLQ.*"})`, Unit: "messages"},
		{Name: "circuit_breaker_open", Query: `sum(resilience_circuit_breaker_state{state="open"})`, Unit: "breakers"},
		{Name: "certificate_expiry_days", Query: `min(certmanager_certificate_expiration_timestamp_seconds - time()) / 86400`, Unit: "days"},
	}

	checkoutQueries = []querySpec{
		{Name: "checkout_red_request_rate", Query: `sum by (service) (rate(http_requests_total{service=~"go-(order|cart|payment|product)-service"}[5m]))`, Unit: "requests/s"},
		{Name: "checkout_red_error_rate", Query: `sum by (service) (rate(http_requests_total{service=~"go-(order|cart|payment|product)-service",status=~"5.."}[5m]))`, Unit: "errors/s"},
		{Name: "checkout_p95_latency", Query: `histogram_quantile(0.95, sum by (le, service) (rate(http_request_duration_seconds_bucket{service=~"go-(order|cart|payment|product)-service"}[5m])))`, Unit: "seconds"},
		{Name: "saga_step_p95_latency", Query: `histogram_quantile(0.95, sum by (le, step) (rate(checkout_saga_step_duration_seconds_bucket[5m])))`, Unit: "seconds"},
		{Name: "rabbitmq_publish_errors", Query: `sum(rate(rabbitmq_publish_errors_total[5m]))`, Unit: "errors/s"},
		{Name: "rabbitmq_saga_dlq_depth", Query: `sum(rabbitmq_queue_messages{queue=~".*checkout.*dlq.*|.*saga.*dlq.*"})`, Unit: "messages"},
		{Name: "payment_webhook_errors", Query: `sum(rate(payment_webhook_errors_total[5m]))`, Unit: "errors/s"},
		{Name: "circuit_breaker_open", Query: `sum(resilience_circuit_breaker_state{state="open"})`, Unit: "breakers"},
	}

	aiPipelineQueries = []querySpec{
		{Name: "ai_agent_turns_by_outcome", Query: `sum by (outcome) (rate(ai_agent_turns_total[5m]))`, Unit: "turns/s"},
		{Name: "ai_agent_duration_p95", Query: `histogram_quantile(0.95, sum by (le) (rate(ai_agent_duration_seconds_bucket[5m])))`, Unit: "seconds"},
		{Name: "ai_tool_call_rate", Query: `sum by (tool) (rate(ai_tool_calls_total[5m]))`, Unit: "calls/s"},
		{Name: "ai_tool_latency_p95", Query: `histogram_quantile(0.95, sum by (le, tool) (rate(ai_tool_duration_seconds_bucket[5m])))`, Unit: "seconds"},
		{Name: "rag_stage_latency_p95", Query: `histogram_quantile(0.95, sum by (le, stage) (rate(rag_stage_duration_seconds_bucket[5m])))`, Unit: "seconds"},
		{Name: "rag_errors", Query: `sum(rate(rag_errors_total[5m]))`, Unit: "errors/s"},
		{Name: "ollama_latency_p95", Query: `histogram_quantile(0.95, sum by (le) (rate(ollama_request_duration_seconds_bucket[5m])))`, Unit: "seconds"},
		{Name: "qdrant_errors", Query: `sum(rate(qdrant_errors_total[5m]))`, Unit: "errors/s"},
		{Name: "eval_runs_total", Query: `sum by (status) (rate(eval_runs_total[5m]))`, Unit: "runs/s"},
		{Name: "eval_upstream_failures", Query: `sum by (endpoint, failure_type) (rate(eval_upstream_failures_total[5m]))`, Unit: "failures/s"},
		{Name: "eval_stale_running_runs", Query: `max(eval_stale_running_runs)`, Unit: "runs"},
	}

	streamingAnalyticsQueries = []querySpec{
		{Name: "kafka_consumer_lag", Query: `max(kafka_consumer_lag{consumer_group="analytics-group"})`, Unit: "messages"},
		{Name: "analytics_events_consumed", Query: `sum by (topic) (rate(analytics_events_consumed_total[5m]))`, Unit: "events/s"},
		{Name: "kafka_consumer_errors", Query: `sum(rate(kafka_consumer_errors_total{consumer_group="analytics-group"}[5m]))`, Unit: "errors/s"},
		{Name: "analytics_red_request_rate", Query: `sum(rate(http_requests_total{service="go-analytics-service"}[5m]))`, Unit: "requests/s"},
		{Name: "analytics_red_error_rate", Query: `sum(rate(http_requests_total{service="go-analytics-service",status=~"5.."}[5m]))`, Unit: "errors/s"},
	}
)

func evalRunQueries(evalID string) []querySpec {
	return []querySpec{
		{Name: "eval_runs_total", Query: `sum by (status, requested_rerank) (rate(eval_runs_total[5m]))`, Unit: "runs/s"},
		{Name: "eval_item_duration_p95", Query: `histogram_quantile(0.95, sum by (le, stage, requested_rerank) (rate(eval_item_duration_seconds_bucket[5m])))`, Unit: "seconds"},
		{Name: "eval_upstream_failures", Query: `sum by (endpoint, failure_type, requested_rerank) (rate(eval_upstream_failures_total[5m]))`, Unit: "failures/s"},
		{Name: "eval_upstream_duration_p95", Query: `histogram_quantile(0.95, sum by (le, endpoint, requested_rerank) (rate(eval_upstream_request_duration_seconds_bucket[5m])))`, Unit: "seconds"},
		{Name: "eval_stale_running_runs", Query: `max(eval_stale_running_runs)`, Unit: "runs", Description: "Correlate these aggregate signals with eval_id-scoped logs."},
	}
}

func serviceQueries(service string) []querySpec {
	return []querySpec{
		{Name: "service_request_rate", Query: `sum(rate(http_requests_total{service="` + service + `"}[5m]))`, Unit: "requests/s"},
		{Name: "service_error_rate", Query: `sum(rate(http_requests_total{service="` + service + `",status=~"5.."}[5m]))`, Unit: "errors/s"},
		{Name: "service_p95_latency", Query: `histogram_quantile(0.95, sum by (le) (rate(http_request_duration_seconds_bucket{service="` + service + `"}[5m])))`, Unit: "seconds"},
		{Name: "go_runtime_goroutines", Query: `go_goroutines{service="` + service + `"}`, Unit: "goroutines"},
		{Name: "kubernetes_pod_ready", Query: `kube_pod_status_ready{condition="true",pod=~".*` + service + `.*"}`, Unit: "ready"},
		{Name: "kubernetes_container_restarts", Query: `sum(increase(kube_pod_container_status_restarts_total{pod=~".*` + service + `.*"}[15m]))`, Unit: "restarts"},
	}
}
