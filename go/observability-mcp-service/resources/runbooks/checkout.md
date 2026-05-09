# Checkout Investigation

Use `investigate_checkout` when cart, order, payment, product, or saga behavior
looks unhealthy.

Signals checked:

- RED metrics for order, cart, payment, and product services.
- Saga step p95 latency.
- Circuit breaker state.
- RabbitMQ publish success and error rate.
- Saga DLQ depth.
- Payment webhook outcomes.
- Recent error, warn, and exception logs.

Interpretation:

- Non-zero saga DLQ depth is critical because checkout messages are no longer
  completing the normal flow.
- Circuit breakers above zero indicate dependency protection is active.
- Payment webhook errors can explain orders stuck after checkout session
  creation.

Useful next tools:

- `get_service_evidence` for a single failing service.
- `get_trace` when an order or log line includes a trace ID.
