# System Health

Use `get_system_health` for a compact read-only snapshot across the portfolio
runtime.

Signals checked:

- Go service request rate, error rate, and p95 latency.
- Kubernetes readiness and restart metrics when kube-state-metrics is present.
- Kafka lag and consumer errors.
- RabbitMQ saga DLQ depth.
- Circuit breaker state.
- Certificate expiry when cert-manager metrics are present.

Interpretation:

- Missing metrics produce unknown evidence, not proof that a service is healthy.
- Non-zero RabbitMQ DLQ depth is critical because saga work is no longer flowing.
- Non-zero Kafka lag or open circuit breakers are warning signals.

Useful next tools:

- `get_service_evidence` for a single unhealthy service.
- `investigate_checkout`, `investigate_ai_pipeline`, or
  `investigate_streaming_analytics` for domain-specific evidence.
