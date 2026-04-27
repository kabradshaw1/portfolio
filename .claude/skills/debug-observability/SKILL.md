---
name: debug-observability
description: Debug service issues and triage alerts using Loki, Jaeger, Grafana, and Prometheus. Use when encountering runtime errors, alerts firing, post-incident health verification, checkout failures, saga stuck states, gRPC connection issues, circuit breaker issues, or any service misbehavior in QA or prod. Also use after resolving an incident to verify recovery and clear stale alerts.
---

# Debug with Observability Stack

**Rule: Use Grafana/Loki/Jaeger before SSH.** Every runtime issue should be diagnosable from the observability stack. SSH + `kubectl logs` is a last resort — the only exception is pods in CrashLoopBackOff where the monitoring target is dead.

## When to Use This Skill

- **Alerts firing** — triage whether alerts are real or stale (see Alert Triage below)
- **Post-incident** — verify cluster recovery and clear stale alerts (see Post-Incident Verification below)
- **Runtime errors** — 5xx responses, failed requests, timeouts
- **Saga issues** — stuck orders, DLQ accumulation, step errors
- **Circuit breakers** — open or flapping breakers
- **gRPC failures** — connection refused, TLS handshake errors
- **Performance** — high latency, low cache hit ratio

## Key Facts

- **5xx errors are always logged.** The `apperror.ErrorHandler()` middleware logs all AppErrors with `HTTPStatus >= 500` via `slog.Error` with code, message, status, and request_id.
- **QA uses a separate RabbitMQ vhost.** QA services connect to `amqp://...5672/qa` while prod uses the default vhost. Queues and exchanges are fully isolated.
- **Webhook dashboard shows per-event-type breakdown.** The "Payment Webhooks" panel groups by `event_type` and `outcome`.
- **Saga stalled alert exists.** `saga-order-stalled` fires when orders reach `PAYMENT_CREATED` but none complete within 30 minutes.

## Step 0: Check Recent Deploys and Kubernetes Warning Events

Before diving into service-specific debugging, check if a recent deploy or cluster-level event explains the issue.

### Deploy annotations

Grafana deploy annotations are posted by CI after every QA and prod rollout. Open any Grafana dashboard and look for vertical annotation markers — they show which namespace was deployed and the commit SHA.

To query annotations programmatically:
```bash
ssh debian "curl -s 'http://grafana.monitoring.svc.cluster.local:3000/api/annotations?from=$(date -d '2 hours ago' +%s)000&to=$(date +%s)000' | python3 -c \"
import json,sys
for a in json.load(sys.stdin):
    print(a.get('text','?') + ' [' + ','.join(a.get('tags',[])) + ']')
\""
```

If the issue started right after a deploy annotation, the deploy is the likely cause.

### Kubernetes Warning events (via Loki)

OOM kills, probe failures, evictions, and scheduling failures are forwarded to Loki by the `kube-event-exporter`. Query them:

```bash
# All Warning events in the last hour
scripts/loki-query --ns monitoring --filter "kube-event-exporter" --hours 1

# Or query Loki directly for structured labels
ssh debian "kubectl exec -n monitoring deploy/loki -- wget -qO- 'http://localhost:3100/loki/api/v1/query_range?query=%7Bjob%3D%22kube-event-exporter%22%7D&limit=20&start=$(date -d '1 hour ago' +%s)&end=$(date +%s)'"
```

Filter by namespace or reason:
```bash
# OOM kills across all namespaces
ssh debian "kubectl exec -n monitoring deploy/loki -- wget -qO- 'http://localhost:3100/loki/api/v1/query_range?query=%7Bjob%3D%22kube-event-exporter%22%2Creason%3D%22OOMKilling%22%7D&limit=20&start=$(date -d '1 hour ago' +%s)&end=$(date +%s)'"

# All warnings in go-ecommerce namespace
ssh debian "kubectl exec -n monitoring deploy/loki -- wget -qO- 'http://localhost:3100/loki/api/v1/query_range?query=%7Bjob%3D%22kube-event-exporter%22%2Cnamespace%3D%22go-ecommerce%22%7D&limit=20&start=$(date -d '1 hour ago' +%s)&end=$(date +%s)'"
```

Common Warning event reasons: `OOMKilling`, `Unhealthy` (probe failure), `BackOff` (CrashLoopBackOff), `FailedScheduling`, `Evicted`.

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

## Postgres Connection Debugging

All Go services set `application_name` on their DATABASE_URL, so `pg_stat_activity` shows which service owns each connection.

### Check connections per service

```bash
ssh debian "kubectl exec deployment/postgres -n java-tasks -- psql -U taskuser -d taskdb -c \"
SELECT application_name, datname, state, count(*)
FROM pg_stat_activity
WHERE datname NOT LIKE 'template%' AND datname != 'postgres'
GROUP BY application_name, datname, state
ORDER BY count DESC;\""
```

### Identify connection leaks

If total connections are high, look for services with disproportionate `idle` connections:
```bash
ssh debian "kubectl exec deployment/postgres -n java-tasks -- psql -U taskuser -d taskdb -c \"
SELECT application_name, state, count(*)
FROM pg_stat_activity
WHERE state = 'idle' AND datname NOT LIKE 'template%'
GROUP BY application_name, state
ORDER BY count DESC;\""
```

A healthy service should have connections roughly proportional to its pool size (typically `MaxConns` from pgxpool config). An abnormal count of `idle` connections from one service suggests a pool leak.

### Grafana panel

The PostgreSQL dashboard has a "Connections by Service" bar gauge panel showing `pg_stat_activity_count` grouped by `application_name`. Check this first for a quick visual.

## Alert Triage

When alerts fire, classify them before acting. Most alerts after an incident are stale.

### Step 1: Get all active alerts with classification

```bash
ssh debian "curl -s 'http://10.100.246.150:3000/api/alertmanager/grafana/api/v2/alerts' | python3 -c \"
import json,sys
alerts=json.load(sys.stdin)
if not alerts:
    print('No active alerts!')
    sys.exit()
nodata = [a for a in alerts if a['labels'].get('alertname') == 'DatasourceNoData']
restart = [a for a in alerts if 'restarted more than' in a['annotations'].get('summary','')]
real = [a for a in alerts if a['labels'].get('alertname') != 'DatasourceNoData' and 'restarted' not in a['annotations'].get('summary','')]
print(str(len(alerts)) + ' total: ' + str(len(nodata)) + ' NoData(stale), ' + str(len(restart)) + ' restart-storm(stale), ' + str(len(real)) + ' real')
for a in real:
    print('  REAL: ' + a['annotations'].get('summary','?')[:80])
for a in nodata[:3]:
    print('  STALE: ' + a['annotations'].get('summary','?')[:80] + ' [NoData]')
if len(nodata) > 3:
    print('  ... and ' + str(len(nodata)-3) + ' more NoData alerts')
\""
```

### Understanding alert types

| alertname | Meaning | Action |
|-----------|---------|--------|
| `DatasourceNoData` | Prometheus query returned empty — usually stale after pod restarts | No action — will auto-resolve as rate windows fill with data. Restart Grafana to clear immediately. |
| Pod restart storm (summary contains "restarted more than") | References pods that may no longer exist | Check if the named pod still exists: `kubectl get pod <name> -n <ns>`. If gone, it's stale. |
| Everything else | Potentially real | Investigate — check the specific metric via Prometheus. |

### Step 2: For "real" alerts, verify via Prometheus

```bash
# Circuit breakers — which are actually open right now?
ssh debian "curl -s 'http://10.98.36.175:9090/api/v1/query?query=circuit_breaker_state==2' | python3 -c \"
import json,sys
r=json.load(sys.stdin)['data']['result']
if not r: print('No open circuit breakers')
for m in r:
    name=m['metric'].get('name','?')
    svc=m['metric'].get('service','?')
    pod=m['metric'].get('pod','?')
    print('OPEN: ' + name + ' (' + svc + ', pod=' + pod + ')')
\""

# Check if the pod still exists (stale metric from terminated pod)
ssh debian "kubectl get pod <pod-name> -n go-ecommerce 2>&1"
```

### Step 3: Clear stale alerts

If all alerts are stale `DatasourceNoData` from a resolved incident, restart Grafana to reset alert state:

```bash
ssh debian "kubectl rollout restart deployment grafana -n monitoring && kubectl wait --for=condition=ready pod -l app=grafana -n monitoring --timeout=60s"
```

This works because provisioned alert rules (file-based, not database-backed) reset their state on restart. The `noDataState: OK` rules will then evaluate cleanly.

**Only do this when you've confirmed the cluster is healthy.** Don't clear alerts to silence a real problem.

## Post-Incident Verification

After resolving any incident, run this checklist before declaring recovery.

### Step 1: Pod health (all namespaces)

```bash
ssh debian "kubectl get pods --all-namespaces --no-headers | grep -v 'Running\|Completed' || echo 'All pods healthy'"
```

All pods should be Running with 0 restarts on current pods. Old pods from before the incident will show higher restart counts — that's expected as long as the *current* pods are clean.

### Step 2: Endpoint smoke test

```bash
ssh debian "curl -s -o /dev/null -w 'auth:%{http_code} ' http://192.168.49.2/go-auth/health && \
curl -s -o /dev/null -w 'products:%{http_code} ' http://192.168.49.2/go-products/products && \
curl -s -o /dev/null -w 'ai:%{http_code} ' http://192.168.49.2/ai-api/health && \
curl -s -o /dev/null -w 'orders:%{http_code} ' http://192.168.49.2/go-orders/health && \
curl -s -o /dev/null -w 'gql:%{http_code} ' http://192.168.49.2/graphql -X POST -H 'Content-Type: application/json' -d '{\"query\":\"{__typename}\"}' && \
echo ''"
```

All should return 200.

### Step 3: Postgres health

```bash
ssh debian "kubectl exec deployment/postgres -n java-tasks -- pg_isready -U taskuser -d taskdb"
```

### Step 4: Circuit breaker state

```bash
ssh debian "curl -s 'http://10.98.36.175:9090/api/v1/query?query=circuit_breaker_state==2' | python3 -c \"
import json,sys
r=json.load(sys.stdin)['data']['result']
if not r: print('All circuit breakers closed')
for m in r:
    pod=m['metric'].get('pod','?')
    print('OPEN: ' + m['metric'].get('name','?') + ' (' + m['metric'].get('service','?') + ', pod=' + pod + ')')
\""
```

If a breaker is open on a pod that still exists, restart that service. If the pod no longer exists, it's a stale Prometheus metric — will clear in ~5 minutes.

### Step 5: Classify remaining alerts

Run the Alert Triage Step 1 query above. If all alerts are `DatasourceNoData` or reference terminated pods, the cluster is healthy and alerts are stale. Clear with Grafana restart if needed.
