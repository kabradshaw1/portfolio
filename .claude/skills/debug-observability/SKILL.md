---
name: debug-observability
description: Debug Go service issues using Loki, Jaeger, and Grafana. Use when encountering runtime errors, checkout failures, saga stuck states, gRPC connection issues, or any service misbehavior in QA or prod.
---

# Debug with Observability Stack

**Rule: Use Grafana/Loki/Jaeger before SSH.** Every runtime issue should be diagnosable from the observability stack. SSH + `kubectl logs` is a last resort.

## Key Facts

- **5xx errors are always logged.** The `apperror.ErrorHandler()` middleware logs all AppErrors with `HTTPStatus >= 500` via `slog.Error` with code, message, status, and request_id. If you see a 500 in request logs, there will be a corresponding "server error" log entry.
- **QA uses a separate RabbitMQ vhost.** QA services connect to `amqp://...5672/qa` while prod uses the default vhost. Queues and exchanges are fully isolated between environments.
- **Webhook dashboard shows per-event-type breakdown.** The "Payment Webhooks" panel groups by `event_type` and `outcome` — check which specific Stripe event type is failing.
- **Saga stalled alert exists.** `saga-order-stalled` fires when orders reach `PAYMENT_CREATED` but none complete within 30 minutes.

## Step 1: Verify the Right Build is Deployed

Before investigating any issue, confirm the service is running the expected code. Query buildinfo from Loki:

```bash
START=$(python3 -c 'import time; print(int((time.time()-300)*1e9))')
END=$(python3 -c 'import time; print(int(time.time()*1e9))')

ssh debian "kubectl exec -n monitoring loki-0 -- wget -qO- \
  'http://localhost:3100/loki/api/v1/query_range?query=%7Bnamespace%3D%22go-ecommerce-qa%22%2Capp%3D%22<SERVICE>%22%7D+%7C%3D+%22service+started%22+%7C+json&limit=5&start=${START}&end=${END}'"
```

If no `"service started"` log exists, the image predates the buildinfo change. Check pod restart time:
```bash
ssh debian 'kubectl get pods -n go-ecommerce-qa -l app=<SERVICE> -o jsonpath="{.items[0].metadata.creationTimestamp}"'
```

## Step 2: Check Grafana Dashboards

Open the Go Services dashboard. Key rows:
- **Decomposed Services** — per-service request rate, error rate, p95 latency
- **Saga & Payment Health** — order status, circuit breaker state, webhook throughput, outbox publish
- **gRPC Client Health** — outbound gRPC request rate, error rate, latency by target service
- **Saga Step Duration** — p95 duration per saga step (shows which step is slow)
- **Certificate Expiry** — days until cert-manager certificates expire

## Step 3: Query Loki

All queries use `kubectl exec` into the Loki pod (reliable, no port-forwarding needed).

**Compute timestamps first:**
```bash
START=$(python3 -c 'import time; print(int((time.time()-3600)*1e9))')  # 1 hour ago
END=$(python3 -c 'import time; print(int(time.time()*1e9))')
```

### Query: Errors by service
```bash
ssh debian "kubectl exec -n monitoring loki-0 -- wget -qO- \
  'http://localhost:3100/loki/api/v1/query_range?query=%7Bnamespace%3D%22go-ecommerce-qa%22%2Capp%3D%22<SERVICE>%22%7D+%7C+json+%7C+level%3D%22ERROR%22&limit=30&start=${START}&end=${END}'"
```

### Query: Trace a specific order through the saga
```bash
ssh debian "kubectl exec -n monitoring loki-0 -- wget -qO- \
  'http://localhost:3100/loki/api/v1/query_range?query=%7Bnamespace%3D%22go-ecommerce-qa%22%7D+%7C%3D+%22<ORDER_ID>%22+%7C+json&limit=50&start=${START}&end=${END}'"
```

### Query: gRPC client failures (from interceptor)
```bash
ssh debian "kubectl exec -n monitoring loki-0 -- wget -qO- \
  'http://localhost:3100/loki/api/v1/query_range?query=%7Bnamespace%3D%22go-ecommerce-qa%22%7D+%7C%3D+%22gRPC+client+call+failed%22+%7C+json&limit=20&start=${START}&end=${END}'"
```

### Query: Stripe API calls
```bash
ssh debian "kubectl exec -n monitoring loki-0 -- wget -qO- \
  'http://localhost:3100/loki/api/v1/query_range?query=%7Bnamespace%3D%22go-ecommerce-qa%22%2Capp%3D%22go-payment-service%22%7D+%7C%3D+%22Stripe+API%22+%7C+json&limit=10&start=${START}&end=${END}'"
```

### Query: Saga events for a specific order
```bash
ssh debian "kubectl exec -n monitoring loki-0 -- wget -qO- \
  'http://localhost:3100/loki/api/v1/query_range?query=%7Bnamespace%3D%22go-ecommerce-qa%22%2Capp%3D%22go-order-service%22%7D+%7C%3D+%22<ORDER_ID>%22+%7C+json&limit=50&start=${START}&end=${END}'"
```

## Parsing Loki Output

Loki returns CRI-wrapped JSON. The actual log is nested inside a `log` field. **Always use this parser:**

```bash
| python3 -c "
import json, sys
data = json.load(sys.stdin)
for stream in data.get('data', {}).get('result', []):
    app = stream.get('stream', {}).get('app', 'unknown')
    for ts, line in stream.get('values', []):
        try:
            outer = json.loads(line)
            inner = outer.get('log', line)
            p = json.loads(inner.strip()) if isinstance(inner, str) else outer
        except:
            p = {}
        msg = p.get('msg', p.get('message', line[:200] if not p else ''))
        if not msg: continue
        level = p.get('level', '')
        err = p.get('error', '')
        tid = p.get('traceID', '')
        extras = ' '.join(f'{k}={p[k]}' for k in ['orderID','target','method','status','duration','currentStep','event','operation','gitSHA'] if p.get(k))
        print(f'[{app}] [{level}] {msg}' + (f' {extras}' if extras else ''))
        if err: print(f'  error: {err[:250]}')
        if tid: print(f'  traceID: {tid}')
"
```

## URL-Encoding Reference

| LogQL | URL-encoded |
|-------|-------------|
| `{namespace="X"}` | `%7Bnamespace%3D%22X%22%7D` |
| `{namespace="X",app="Y"}` | `%7Bnamespace%3D%22X%22%2Capp%3D%22Y%22%7D` |
| `\| json` | `+%7C+json` |
| `\| level="ERROR"` | `+%7C+level%3D%22ERROR%22` |
| `\|= "text"` | `+%7C%3D+%22text%22` |
| `{namespace=~"go-ecommerce.*"}` | `%7Bnamespace%3D~%22go-ecommerce.*%22%7D` |

## Jaeger Trace Lookup

Every structured log line includes `traceID`. Copy it from Loki output, then look up in Jaeger:

```bash
ssh debian 'kubectl port-forward svc/jaeger 16686:16686 -n monitoring &'
# Open http://localhost:16686/jaeger in browser
```

A checkout trace spans: order-service -> cart-service (gRPC) -> product-service (gRPC) -> payment-service (gRPC) -> RabbitMQ publish -> saga consumer.

## Saga Debugging Flow

### Step 1: Check the dashboard
The "Saga & Payment Health" and "Saga Step Duration" rows in the Go Services dashboard show step error rates, duration, and order status breakdown.

### Step 2: Query Loki for saga errors
```bash
ssh debian "kubectl exec -n monitoring loki-0 -- wget -qO- \
  'http://localhost:3100/loki/api/v1/query_range?query=%7Bnamespace%3D%22go-ecommerce-qa%22%2Capp%3D%22go-order-service%22%7D+%7C%3D+%22saga+event+handling+failed%22+%7C+json&limit=20&start=${START}&end=${END}'"
```

### Step 3: Check DLQ
```bash
ssh debian 'kubectl exec -n go-ecommerce-qa deploy/go-order-service -- wget -qO- http://localhost:8092/admin/dlq/messages?limit=10'
```

## Circuit Breaker Diagnosis

```bash
# Check breaker state via Prometheus (0=closed, 1=half-open, 2=open)
# In Grafana: query circuit_breaker_state{name="order-postgres"}
# Alert: circuit-breaker-open fires when state == 2 for 2 min

# Check for poison messages in RabbitMQ
ssh debian 'kubectl exec -n java-tasks deploy/rabbitmq -- rabbitmqctl list_queues name messages'

# If breaker is open: purge the offending queue + restart service
ssh debian 'kubectl scale deployment/go-order-service -n go-ecommerce-qa --replicas=0'
ssh debian 'kubectl exec -n java-tasks deploy/rabbitmq -- rabbitmqctl purge_queue saga.order.events'
ssh debian 'kubectl scale deployment/go-order-service -n go-ecommerce-qa --replicas=2'
```

## cert-manager Diagnosis

```bash
# Check if certificates are Ready
ssh debian 'kubectl get certificates -n go-ecommerce-qa'

# Check cert-manager pods
ssh debian 'kubectl get pods -n cert-manager'

# Check specific cert details
ssh debian 'kubectl describe certificate payment-grpc-tls -n go-ecommerce-qa'
```
