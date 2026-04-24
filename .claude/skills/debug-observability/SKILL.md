---
name: debug-observability
description: Debug Go service issues using Loki, Jaeger, and Grafana. Use when encountering runtime errors, checkout failures, saga stuck states, gRPC connection issues, or any service misbehavior in QA or prod.
---

# Debug with Observability Stack

**Rule: Use Grafana/Loki/Jaeger before SSH.** Every runtime issue should be diagnosable from the observability stack. SSH + `kubectl logs` is a last resort.

## Key Facts

- **5xx errors are always logged.** The `apperror.ErrorHandler()` middleware logs all AppErrors with `HTTPStatus >= 500` via `slog.Error` with code, message, status, and request_id.
- **QA uses a separate RabbitMQ vhost.** QA services connect to `amqp://...5672/qa` while prod uses the default vhost. Queues and exchanges are fully isolated.
- **Webhook dashboard shows per-event-type breakdown.** The "Payment Webhooks" panel groups by `event_type` and `outcome`.
- **Saga stalled alert exists.** `saga-order-stalled` fires when orders reach `PAYMENT_CREATED` but none complete within 30 minutes.

## Step 1: Verify the Right Build is Deployed

```bash
scripts/loki-query --ns go-ecommerce --app <SERVICE> --filter "service started" --hours 1 --limit 5
```

If no results, the image predates buildinfo. Check pod creation time:
```bash
ssh debian 'kubectl get pods -n go-ecommerce -l app=<SERVICE> -o jsonpath="{.items[0].metadata.creationTimestamp}"'
```

For QA, use `--ns go-ecommerce-qa`.

## Step 2: Check Grafana Dashboards

Open the Go Services dashboard. Key rows:
- **Decomposed Services** — per-service request rate, error rate, p95 latency
- **Saga & Payment Health** — order status, circuit breaker state, webhook throughput, outbox publish
- **gRPC Client Health** — outbound gRPC request rate, error rate, latency by target service
- **Saga Step Duration** — p95 duration per saga step (shows which step is slow)
- **Certificate Expiry** — days until cert-manager certificates expire

## Step 3: Query Loki

Use the `scripts/loki-query` wrapper. It handles SSH, kubectl exec, URL encoding, and CRI JSON parsing.

```
scripts/loki-query --ns <namespace> [--app <app>] [--filter <text>] [--level <level>] [--limit <n>] [--hours <n>]
```

**Flags:**
| Flag | Default | Description |
|------|---------|-------------|
| `--ns` | (required) | K8s namespace: `go-ecommerce`, `go-ecommerce-qa`, `java-tasks`, `ai-services`, etc. |
| `--app` | all apps | Service name: `go-order-service`, `go-cart-service`, `go-analytics-service`, etc. |
| `--filter` | none | Text match in log line (order ID, error message, etc.) |
| `--level` | all | Log level: `ERROR`, `WARN`, `INFO`, `DEBUG` |
| `--limit` | 30 | Max results from Loki |
| `--hours` | 1 | Time window to search |

### Common queries

```bash
# Errors across all Go services (prod)
scripts/loki-query --ns go-ecommerce --level ERROR

# Trace a specific order through the saga
scripts/loki-query --ns go-ecommerce --filter "55e8be50-3afe-4461-82a8-cfada8b7f7a2"

# Saga failures in QA
scripts/loki-query --ns go-ecommerce-qa --app go-order-service --filter "saga event handling failed"

# gRPC client errors
scripts/loki-query --ns go-ecommerce --filter "gRPC client call failed"

# Stripe API calls
scripts/loki-query --ns go-ecommerce --app go-payment-service --filter "Stripe API"

# Analytics service over last 4 hours
scripts/loki-query --ns go-ecommerce --app go-analytics-service --hours 4 --limit 50
```

### Output format

Each log line is printed as:
```
[app] [LEVEL] message key=value key=value ...
  error: <error detail if present>
  traceID: <trace ID if present>
```

## Jaeger Trace Lookup

Every structured log line includes `traceID`. Copy it from loki-query output, then look up in Jaeger:

```bash
ssh debian 'kubectl port-forward svc/jaeger 16686:16686 -n monitoring &'
# Open http://localhost:16686/jaeger in browser
```

A checkout trace spans: order-service -> cart-service (gRPC) -> product-service (gRPC) -> payment-service (gRPC) -> RabbitMQ publish -> saga consumer.

## Saga Debugging Flow

1. **Check the dashboard:** "Saga & Payment Health" and "Saga Step Duration" rows show step error rates, duration, and order status breakdown.

2. **Query saga errors:**
   ```bash
   scripts/loki-query --ns go-ecommerce --app go-order-service --filter "saga event handling failed"
   ```

3. **Check DLQ:**
   ```bash
   ssh debian 'kubectl exec -n go-ecommerce deploy/go-order-service -- wget -qO- http://localhost:8092/admin/dlq/messages?limit=10'
   ```

## Circuit Breaker Diagnosis

```bash
# Check breaker state via Prometheus (0=closed, 1=half-open, 2=open)
# In Grafana: query circuit_breaker_state{name="order-postgres"}
# Alerts: circuit-breaker-open (sustained open), circuit-breaker-flapping (rapid state changes)

# Check for poison messages in RabbitMQ
ssh debian 'kubectl exec -n java-tasks deploy/rabbitmq -- rabbitmqctl list_queues name messages'

# If breaker is open: purge the offending queue + restart service
ssh debian 'kubectl scale deployment/go-order-service -n go-ecommerce --replicas=0'
ssh debian 'kubectl exec -n java-tasks deploy/rabbitmq -- rabbitmqctl purge_queue saga.order.events'
ssh debian 'kubectl scale deployment/go-order-service -n go-ecommerce --replicas=2'
```

## cert-manager Diagnosis

```bash
ssh debian 'kubectl get certificates -n go-ecommerce'
ssh debian 'kubectl get pods -n cert-manager'
ssh debian 'kubectl describe certificate payment-grpc-tls -n go-ecommerce'
```
