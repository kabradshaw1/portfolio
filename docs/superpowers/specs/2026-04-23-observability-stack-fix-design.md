# Observability Stack Fix + Debugging Recipes

**Date:** 2026-04-23
**Status:** Proposed
**Goal:** Restore the broken monitoring stack (Jaeger, Grafana, Promtail) so agents can debug multi-service saga failures using Loki log queries and Jaeger traces instead of SSH + kubectl logs. Add debugging recipes to CLAUDE.md and monitoring health smoke checks to CI.

## Overview

Three infrastructure fixes, one documentation update, and one CI addition:

1. **Fix Jaeger** — bump image from nonexistent `1.68` to `1.76.0`
2. **Fix Grafana** — correct datasource ConfigMap volume mount (directory vs. file)
3. **Fix Promtail** — add `__path__` relabel rule so kubernetes_sd targets are discovered
4. **CLAUDE.md debugging recipes** — teach agents to use Loki/Jaeger/Grafana before SSH
5. **CI monitoring smoke checks** — verify observability stack health on every deploy

---

## 1. Fix Jaeger

**File:** `k8s/monitoring/deployments/jaeger.yml`

**Change:** Update image tag from `jaegertracing/all-in-one:1.68` to `jaegertracing/all-in-one:1.76.0`. No other config changes needed — OTLP collector port (4317), query port (16686), and `QUERY_BASE_PATH=/jaeger` are unchanged across versions.

**Verification:** Pod reaches `Running` state. Go services stop logging `traces export: connection refused` errors against `jaeger-collector.monitoring.svc.cluster.local:4317`.

---

## 2. Fix Grafana

**Problem:** Grafana crashes with `Datasource provisioning error: read /etc/grafana/provisioning/datasources/datasources.yml: is a directory`. The datasource ConfigMap is being mounted as a directory instead of a file.

**Fix:** Inspect the Grafana deployment volume mount and ConfigMap. Ensure the ConfigMap key is mounted with `subPath` so it creates a file, not a directory. The exact fix depends on the current mount configuration — the implementing agent should read the Grafana deployment manifest and datasource ConfigMap, then correct the volume mount.

**Verification:** Grafana pod `1/1 Running` with zero restarts. Grafana UI loads at the configured ingress path. All three datasources (Prometheus, Loki, Jaeger) appear in Grafana → Connections → Data sources.

---

## 3. Fix Promtail

**Problem:** Promtail 3.0 reports "Unable to find any logs to tail" — zero active targets despite correct RBAC, service account, and pod log directory access at `/var/log/pods/`.

**Root cause:** The kubernetes_sd_configs `role: pod` discovers pods but the relabel config doesn't construct `__path__`, so Promtail doesn't know which log files to tail.

**Fix:** Add a relabel rule to the Promtail ConfigMap that constructs `__path__` from pod metadata:

```yaml
- source_labels:
    - __meta_kubernetes_namespace
    - __meta_kubernetes_pod_name
    - __meta_kubernetes_pod_container_name
  separator: /
  target_label: __path__
  replacement: /var/log/pods/$1/*.log
```

**Fallback:** If the relabel approach doesn't work, try:
- Bump Promtail to latest 3.x patch
- Switch to `static_configs` with path glob `/var/log/pods/*/*/*.log` and use relabel rules for label enrichment only

**Verification:** `curl localhost:3101/ready` returns "Ready" (not "Unable to find any logs"). `promtail_targets_active_total` > 0. Loki query `{namespace="go-ecommerce-qa"}` returns results.

---

## 4. CLAUDE.md Debugging Recipes

**Location:** New section "Debugging with Observability Tools" in CLAUDE.md, after the existing "Monitoring & Observability" section.

**Content:**

### Rule
Agents must query Loki/Jaeger before tailing individual pod logs. SSH + kubectl logs is a last resort for when observability tools are unavailable.

### Loki Log Queries
Access Loki via Grafana (Explore → Loki datasource) or via `kubectl port-forward svc/loki 3100:3100 -n monitoring` and curl.

- **Query by order ID across all services:** `{namespace="go-ecommerce-qa"} |= "<orderID>" | json`
- **Query errors for a specific service:** `{namespace="go-ecommerce-qa",app="go-order-service"} | json | level="ERROR"`
- **Query saga flow for an order:** `{namespace=~"go-ecommerce.*"} |= "<orderID>" | json` (spans prod and QA)
- **Readable output:** append `| line_format "{{.msg}}"`

### Jaeger Trace Lookup
Every structured log line includes `traceID`. Copy it from a Loki log entry, then look it up in Jaeger.

- Jaeger UI at `/jaeger` on the Grafana ingress, or `kubectl port-forward svc/jaeger 16686:16686 -n monitoring`
- A checkout trace spans: order-service → cart-service (gRPC) → product-service (gRPC) → payment-service (gRPC) → RabbitMQ publish → saga consumer

### Circuit Breaker Diagnosis
- Check Prometheus: `circuit_breaker_state{name="order-postgres"}` — 0=closed, 1=half-open, 2=open
- If open: check for poison messages: `kubectl exec -n java-tasks deploy/rabbitmq -- rabbitmqctl list_queues name messages`
- Fix: purge affected queue + restart the service to reset the breaker

### Common Patterns
- **Saga stuck in COMPENSATING:** stale RabbitMQ messages looping. Purge queue, mark orders as FAILED in DB, restart service.
- **Circuit breaker tripped (503s):** poison messages keeping breaker open. Purge queue, restart.
- **Traces not appearing:** check Jaeger pod status, verify OTEL endpoint in service config.

---

## 5. CI Monitoring Smoke Checks

**Location:** New step in the existing Deploy QA and Deploy Production jobs in `.github/workflows/ci.yml`, after the service restart/rollout step.

**Checks (via SSH):**

1. **Jaeger reachable:** `kubectl exec -n monitoring deploy/jaeger -- wget -qO- http://localhost:16686/ > /dev/null` (returns 0)
2. **Promtail ready:** `kubectl exec -n monitoring daemonset/promtail -- cat /proc/1/cmdline > /dev/null` + check Promtail metrics endpoint for active targets > 0
3. **Loki has data:** `kubectl port-forward svc/loki 3100:3100 -n monitoring` + query for recent entries in any namespace

**Failure behavior:** Fail the pipeline. Observability is critical infrastructure — if it's broken, we can't debug application issues, as we learned today.

**Rationale:** Jaeger was down for 7+ days before we noticed. A smoke check after each deploy would have caught it on the first run.

---

## Out of Scope

- No new dashboards or alert rules
- No changes to service-level tracing instrumentation (already correct)
- No Promtail version upgrade (fix config first, upgrade only if needed)
- No Grafana dashboard changes
- No changes to how services emit logs or traces
