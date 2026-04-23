# Observability Stack Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore Jaeger, Grafana, and Promtail so agents can debug saga failures via Loki log queries and Jaeger traces, add debugging recipes to CLAUDE.md, and add monitoring health smoke checks to CI.

**Architecture:** Three K8s manifest fixes (image bump, config correction, relabel rule), a CLAUDE.md documentation update, and a new CI step for monitoring health verification.

**Tech Stack:** Kubernetes manifests (YAML), Promtail config, GitHub Actions CI, Markdown

**Spec:** `docs/superpowers/specs/2026-04-23-observability-stack-fix-design.md`

---

### Task 1: Fix Jaeger — Bump Image Version

**Files:**
- Modify: `k8s/monitoring/deployments/jaeger.yml`

- [ ] **Step 1: Update Jaeger image tag**

In `k8s/monitoring/deployments/jaeger.yml`, change line 20:

```yaml
          image: jaegertracing/all-in-one:1.76.0
```

From the old value `jaegertracing/all-in-one:1.68`.

- [ ] **Step 2: Apply and verify on Debian server**

```bash
ssh debian 'kubectl apply -f -' < k8s/monitoring/deployments/jaeger.yml
ssh debian 'kubectl rollout status deployment/jaeger -n monitoring --timeout=120s'
ssh debian 'kubectl get pod -n monitoring -l app=jaeger'
```

Expected: Pod reaches `1/1 Running`. No `ImagePullBackOff`.

- [ ] **Step 3: Verify OTLP endpoint is reachable from Go services**

```bash
ssh debian 'kubectl exec -n go-ecommerce deploy/go-order-service -- wget -qO- --timeout=3 http://jaeger-collector.monitoring.svc.cluster.local:4317/ 2>&1 || echo "connection test done"'
```

The connection should not be refused (it may return an HTTP error since OTLP is gRPC, but the TCP connection should succeed).

- [ ] **Step 4: Commit**

```bash
git add k8s/monitoring/deployments/jaeger.yml
git commit -m "fix(monitoring): bump Jaeger image from nonexistent 1.68 to 1.76.0"
```

---

### Task 2: Fix Grafana — Resolve Datasource ConfigMap Mount

**Files:**
- Modify: `k8s/monitoring/deployments/grafana.yml` (potentially)

- [ ] **Step 1: Diagnose the actual mount issue**

The Grafana deployment already has `subPath: datasources.yml` on the volume mount (line 44) and the ConfigMap key is `datasources.yml`. This should work. The crash may be from a stale pod that was created before `subPath` was added.

First, check the crashing pod's creation time vs. the running pod:

```bash
ssh debian 'kubectl get pods -n monitoring -l app=grafana -o wide --show-labels'
```

- [ ] **Step 2: Delete the crashing pod and verify**

The CrashLooping pod is likely from a stale ReplicaSet. Delete it and verify the running pod is healthy:

```bash
ssh debian 'kubectl delete pod grafana-6f787965b5-c26ws -n monitoring'
ssh debian 'kubectl get pods -n monitoring -l app=grafana'
```

If only one pod remains and it's Running, the issue is resolved. If the remaining pod also crashes, we need to re-apply the deployment:

```bash
ssh debian 'kubectl apply -f -' < k8s/monitoring/deployments/grafana.yml
ssh debian 'kubectl rollout restart deployment/grafana -n monitoring'
ssh debian 'kubectl rollout status deployment/grafana -n monitoring --timeout=120s'
```

- [ ] **Step 3: Verify Grafana UI loads**

```bash
ssh debian 'kubectl exec -n monitoring deploy/grafana -- wget -qO- http://localhost:3000/api/health 2>&1'
```

Expected: `{"commit":"...","database":"ok","version":"..."}`

- [ ] **Step 4: Verify all three datasources are connected**

```bash
ssh debian 'kubectl exec -n monitoring deploy/grafana -- wget -qO- http://localhost:3000/api/datasources 2>&1'
```

Expected: JSON array with 3 entries (Prometheus, Loki, Jaeger). Verify Jaeger URL is `http://jaeger.monitoring.svc.cluster.local:16686`.

- [ ] **Step 5: Commit if any file changes were needed**

```bash
git add k8s/monitoring/deployments/grafana.yml
git commit -m "fix(monitoring): resolve Grafana datasource ConfigMap mount issue"
```

If no file changes were needed (just pod cleanup), skip this step.

---

### Task 3: Fix Promtail — Add __path__ Relabel Rule

**Files:**
- Modify: `k8s/monitoring/configmaps/promtail-config.yml`

- [ ] **Step 1: Add __path__ relabel rule to Promtail config**

In `k8s/monitoring/configmaps/promtail-config.yml`, add the following relabel rule to the `relabel_configs` list, BEFORE the existing app/namespace/pod/container rules:

```yaml
          - source_labels:
              - __meta_kubernetes_namespace
              - __meta_kubernetes_pod_name
              - __meta_kubernetes_pod_container_name
            separator: /
            target_label: __path__
            replacement: /var/log/pods/$1/*.log
```

The full `relabel_configs` section should look like:

```yaml
        relabel_configs:
          - source_labels:
              - __meta_kubernetes_namespace
              - __meta_kubernetes_pod_name
              - __meta_kubernetes_pod_container_name
            separator: /
            target_label: __path__
            replacement: /var/log/pods/$1/*.log
          - source_labels: [__meta_kubernetes_pod_label_app]
            target_label: app
          - source_labels: [__meta_kubernetes_namespace]
            target_label: namespace
          - source_labels: [__meta_kubernetes_pod_name]
            target_label: pod
          - source_labels: [__meta_kubernetes_container_name]
            target_label: container
```

- [ ] **Step 2: Also change log_level to info temporarily for debugging**

Change `log_level: warn` to `log_level: info` so we can see target discovery logs. We'll set it back to `warn` after confirming it works.

- [ ] **Step 3: Apply and restart Promtail**

```bash
ssh debian 'kubectl apply -f -' < k8s/monitoring/configmaps/promtail-config.yml
ssh debian 'kubectl delete pod -l app=promtail -n monitoring'
ssh debian 'kubectl wait --for=condition=ready pod -l app=promtail -n monitoring --timeout=60s'
```

- [ ] **Step 4: Verify Promtail has active targets**

```bash
ssh debian 'kubectl port-forward daemonset/promtail 3101:3101 -n monitoring &>/dev/null & sleep 3 && curl -s http://localhost:3101/ready && kill %1'
```

Expected: "Ready" (not "Unable to find any logs to tail")

Check metrics:

```bash
ssh debian 'kubectl port-forward daemonset/promtail 3101:3101 -n monitoring &>/dev/null & sleep 3 && curl -s http://localhost:3101/metrics | grep promtail_targets_active && kill %1'
```

Expected: `promtail_targets_active_total` > 0

- [ ] **Step 5: Verify Loki receives data**

```bash
ssh debian 'kubectl port-forward svc/loki 3100:3100 -n monitoring &>/dev/null & sleep 3 && curl -sG http://localhost:3100/loki/api/v1/label/namespace/values && kill %1'
```

Expected: JSON with namespace values including `go-ecommerce`, `go-ecommerce-qa`, etc.

- [ ] **Step 6: If Step 4 fails — try alternative approach**

If the `__path__` relabel doesn't work, replace the entire `scrape_configs` section with a static path approach:

```yaml
    scrape_configs:
      - job_name: kubernetes-pods
        static_configs:
          - targets:
              - localhost
            labels:
              job: kubernetes-pods
              __path__: /var/log/pods/*/*/*.log
        pipeline_stages:
          - cri: {}
          - json:
              expressions:
                level: level
                msg: msg
                traceID: traceID
          - labels:
              level:
          - output:
              source: msg
          - regex:
              expression: '/var/log/pods/(?P<namespace>[^/]+)_(?P<pod>[^/]+)_[^/]+/(?P<container>[^/]+)/.*'
              source: filename
          - labels:
              namespace:
              pod:
              container:
```

This uses a glob path to find all pod logs and extracts namespace/pod/container from the file path.

- [ ] **Step 7: Set log_level back to warn after confirming it works**

Change `log_level: info` back to `log_level: warn` in the ConfigMap. Apply and restart.

- [ ] **Step 8: Commit**

```bash
git add k8s/monitoring/configmaps/promtail-config.yml
git commit -m "fix(monitoring): add __path__ relabel rule so Promtail discovers pod log targets"
```

---

### Task 4: CLAUDE.md Debugging Recipes

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Add debugging section after "Monitoring & Observability"**

Add the following new section after the existing "Monitoring & Observability" section (after line ~232, before "Kafka Streaming Analytics"):

```markdown
## Debugging with Observability Tools

**Rule: Use Loki/Jaeger before SSH.** When debugging service issues, query the observability stack first. SSH + `kubectl logs` is a last resort for when the monitoring stack is unavailable.

### Loki Log Queries

Access via Grafana (Explore → Loki datasource) or via CLI:

```bash
# Port-forward to Loki
ssh debian 'kubectl port-forward svc/loki 3100:3100 -n monitoring &'

# Query by order ID across all services
curl -sG http://localhost:3100/loki/api/v1/query_range \
  --data-urlencode 'query={namespace=~"go-ecommerce.*"} |= "<orderID>" | json' \
  --data-urlencode 'limit=50'

# Query errors for a specific service
curl -sG http://localhost:3100/loki/api/v1/query_range \
  --data-urlencode 'query={namespace="go-ecommerce-qa",app="go-order-service"} | json | level="ERROR"' \
  --data-urlencode 'limit=20'
```

In Grafana Explore (Loki datasource):
- **By order ID:** `{namespace=~"go-ecommerce.*"} |= "<orderID>" | json`
- **By error level:** `{namespace="go-ecommerce-qa",app="go-order-service"} | json | level="ERROR"`
- **Readable output:** append `| line_format "{{.msg}}"`

### Jaeger Trace Lookup

Every structured log line includes `traceID`. Copy it from Loki, then look it up in Jaeger.

```bash
# Port-forward to Jaeger UI
ssh debian 'kubectl port-forward svc/jaeger 16686:16686 -n monitoring &'
# Open http://localhost:16686/jaeger in browser
```

A checkout trace spans: order-service → cart-service (gRPC) → product-service (gRPC) → payment-service (gRPC) → RabbitMQ publish → saga consumer.

In Grafana, Loki log entries with `traceID` have a clickable "View Trace" link that opens directly in Jaeger.

### Circuit Breaker Diagnosis

```bash
# Check breaker state via Prometheus (0=closed, 1=half-open, 2=open)
# In Grafana: query circuit_breaker_state{name="order-postgres"}

# Check for poison messages in RabbitMQ
ssh debian 'kubectl exec -n java-tasks deploy/rabbitmq -- rabbitmqctl list_queues name messages'

# If breaker is open: purge the offending queue + restart service
ssh debian 'kubectl scale deployment/go-order-service -n go-ecommerce-qa --replicas=0'
ssh debian 'kubectl exec -n java-tasks deploy/rabbitmq -- rabbitmqctl purge_queue saga.order.events'
ssh debian 'kubectl scale deployment/go-order-service -n go-ecommerce-qa --replicas=2'
```

### Common Debugging Patterns

| Symptom | Likely Cause | Fix |
|---------|-------------|-----|
| Saga stuck in COMPENSATING | Stale RabbitMQ messages looping | Purge queue, mark orders as FAILED in DB, restart service |
| 503 on order endpoints | Circuit breaker open from poison messages | Purge queue, restart service to reset breaker |
| Traces not appearing in Jaeger | Jaeger pod down or OTEL endpoint misconfigured | Check `kubectl get pod -n monitoring -l app=jaeger` |
| Loki returns empty results | Promtail not scraping | Check `kubectl port-forward daemonset/promtail 3101:3101 -n monitoring && curl localhost:3101/ready` |
```

- [ ] **Step 2: Verify the markdown renders correctly**

Read the file back and check for formatting issues.

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: add debugging recipes for Loki, Jaeger, and circuit breaker diagnosis"
```

---

### Task 5: CI Monitoring Health Smoke Checks

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Add monitoring health check step to QA deploy**

Add a new step after the "Deploy QA via SSH" step, before the smoke-qa job. This runs as part of the deploy-qa job, after service restarts:

Find the end of the deploy-qa step (after `$SSH "kubectl rollout status deployment -n go-ecommerce-qa --timeout=180s"`) and add before `rm -f ~/.ssh/deploy_key`:

```yaml
          # Verify monitoring stack health
          echo "Checking monitoring stack health..."
          if ! $SSH "kubectl get pod -n monitoring -l app=jaeger -o jsonpath='{.items[0].status.phase}'" | grep -q Running; then
            echo "::error::Jaeger is not running"
            exit 1
          fi
          if ! $SSH "kubectl get pod -n monitoring -l app=grafana -o jsonpath='{.items[0].status.phase}'" | grep -q Running; then
            echo "::error::Grafana is not running"
            exit 1
          fi
          if ! $SSH "kubectl get pod -n monitoring -l app=promtail -o jsonpath='{.items[0].status.phase}'" | grep -q Running; then
            echo "::error::Promtail is not running"
            exit 1
          fi
          echo "Monitoring stack healthy"
```

- [ ] **Step 2: Add the same check to prod deploy**

Find the end of the prod deploy step (after `$SSH "kubectl rollout status deployment -n go-ecommerce --timeout=180s"`) and add the same monitoring check block before `rm -f ~/.ssh/deploy_key`.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add monitoring stack health checks to QA and prod deploy"
```

---

### Task 6: Verify End-to-End Observability

**Files:** None (verification only)

- [ ] **Step 1: Generate test traffic**

Trigger a checkout on QA to produce logs and traces:

```bash
# Or just use the QA frontend at https://qa.kylebradshaw.dev/go/ecommerce
```

- [ ] **Step 2: Query Loki for the order**

```bash
ssh debian 'kubectl port-forward svc/loki 3100:3100 -n monitoring &>/dev/null & sleep 3 && curl -sG http://localhost:3100/loki/api/v1/query_range --data-urlencode "query={namespace=\"go-ecommerce-qa\",app=\"go-order-service\"} | json | level=\"INFO\"" --data-urlencode "limit=5" --data-urlencode "start=$(date -u -d "5 minutes ago" +%s)000000000" --data-urlencode "end=$(date -u +%s)000000000" && kill %1'
```

Expected: Non-empty results with log entries from order-service.

- [ ] **Step 3: Find a traceID and look it up in Jaeger**

From the Loki results, extract a `traceID` value. Look it up in Jaeger:

```bash
ssh debian 'kubectl port-forward svc/jaeger 16686:16686 -n monitoring &>/dev/null & sleep 3 && curl -s "http://localhost:16686/api/traces/<traceID>" && kill %1'
```

Expected: Trace with spans across multiple services.

- [ ] **Step 4: Push all changes and verify CI**

```bash
git push origin qa
```

Expected: CI passes including the new monitoring health checks.
