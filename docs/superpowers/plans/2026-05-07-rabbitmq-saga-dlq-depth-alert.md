# RabbitMQ Saga DLQ Depth Alert Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add alerting and dashboard visibility for current saga DLQ queue depth using RabbitMQ broker metrics.

**Architecture:** Keep the existing application event-rate alert unchanged and add a separate Grafana managed alert against `rabbitmq_detailed_queue_messages` filtered to `queue="ecommerce.saga.dlq"`. Update the Go services dashboard source of truth with a focused RabbitMQ DLQ depth panel in the existing `Cache & RabbitMQ` row, then regenerate the dashboard ConfigMap with the repo sync script.

**Tech Stack:** Kubernetes ConfigMaps, Grafana managed alert provisioning, Grafana dashboard JSON, Prometheus PromQL, RabbitMQ Prometheus detailed queue metrics, repo YAML/JSON validation.

---

## File Structure

- Modify `k8s/monitoring/configmaps/grafana-alerting.yml`: add a new saga DLQ depth alert next to the existing `saga-dlq-accumulating` event-rate alert.
- Modify `monitoring/grafana/dashboards/go-services.json`: add a current DLQ depth panel to the existing `Cache & RabbitMQ` dashboard row.
- Regenerate `k8s/monitoring/configmaps/grafana-dashboards.yml`: keep embedded dashboard JSON synchronized with `monitoring/grafana/dashboards/go-services.json`.
- Verify existing Phase 1 files remain intact: `java/k8s/configmaps/rabbitmq-definitions.yml`, `java/k8s/deployments/rabbitmq.yml`, `java/k8s/services/rabbitmq.yml`, and `k8s/monitoring/configmaps/prometheus-config.yml`.

## Task 1: Add Saga DLQ Depth Alert

**Files:**
- Modify: `k8s/monitoring/configmaps/grafana-alerting.yml`

- [ ] **Step 1: Add a new alert after `saga-dlq-accumulating`**

Insert this rule immediately after the existing `saga-dlq-accumulating` rule and before `saga-step-error-rate`:

```yaml
          - uid: saga-dlq-depth-nonempty
            title: Saga DLQ Depth Nonempty
            noDataState: OK
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: >-
                    max by (vhost, queue) (
                      rabbitmq_detailed_queue_messages{queue="ecommerce.saga.dlq"}
                    )
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator:
                        type: gt
                        params:
                          - 0
                  refId: C
            for: 5m
            labels:
              severity: warning
            annotations:
              summary: "Saga DLQ {{ $labels.queue }} in RabbitMQ vhost {{ $labels.vhost }} has messages waiting"
```

- [ ] **Step 2: Confirm the existing event-rate alert is unchanged**

Run:

```bash
rg -n "uid: saga-dlq-accumulating|expr: increase\\(saga_dlq_messages_total\\[10m\\]\\)|uid: saga-dlq-depth-nonempty|rabbitmq_detailed_queue_messages\\{queue=\\\"ecommerce\\.saga\\.dlq\\\"\\}" k8s/monitoring/configmaps/grafana-alerting.yml
```

Expected: output includes both alert UIDs, the unchanged `increase(saga_dlq_messages_total[10m])` expression, and the new RabbitMQ queue-depth expression.

## Task 2: Add DLQ Depth Dashboard Panel

**Files:**
- Modify: `monitoring/grafana/dashboards/go-services.json`
- Regenerate: `k8s/monitoring/configmaps/grafana-dashboards.yml`

- [ ] **Step 1: Add a current depth panel in the existing RabbitMQ row**

In `monitoring/grafana/dashboards/go-services.json`, insert this panel after the existing `RabbitMQ Publish` panel and before the `AI Service Agent` row:

```json
    {
      "title": "Saga DLQ Depth",
      "type": "timeseries",
      "gridPos": {
        "h": 6,
        "w": 8,
        "x": 0,
        "y": 28
      },
      "id": 18,
      "datasource": {
        "type": "prometheus",
        "uid": ""
      },
      "targets": [
        {
          "expr": "max by (vhost, queue) (rabbitmq_detailed_queue_messages{queue=\"ecommerce.saga.dlq\"})",
          "legendFormat": "{{vhost}} {{queue}}",
          "refId": "A"
        }
      ],
      "fieldConfig": {
        "defaults": {
          "unit": "short",
          "custom": {
            "drawStyle": "line",
            "lineWidth": 1,
            "fillOpacity": 10,
            "showPoints": "never"
          }
        },
        "overrides": []
      },
      "options": {
        "tooltip": {
          "mode": "multi"
        },
        "legend": {
          "displayMode": "list",
          "placement": "bottom"
        }
      }
    }
```

Increment every existing dashboard panel ID from `18` onward by one so IDs remain unique, and move every panel with `gridPos.y >= 28` down by 6 grid units so the new panel does not overlap existing AI, streaming, or later sections. The first affected panels become:

```text
AI Service Agent row: id 19, y 34
Agent Turns by Outcome: id 20, y 35
Agent Turn Duration: id 21, y 35
Tool Cache Hit/Miss: id 22, y 35
Streaming Analytics row: id 23, y 41
```

- [ ] **Step 2: Regenerate the Grafana dashboards ConfigMap**

Run:

```bash
make grafana-sync
```

Expected output includes:

```text
regenerated (4 dashboards)
```

- [ ] **Step 3: Validate dashboard JSON and sync**

Run:

```bash
jq empty monitoring/grafana/dashboards/go-services.json
make grafana-sync-check
```

Expected: `jq` exits `0`, and sync check prints `grafana dashboards in sync (4 dashboards)`.

## Task 3: Validate Monitoring Manifests And Phase 1 Wiring

**Files:**
- Verify: `k8s/monitoring/configmaps/grafana-alerting.yml`
- Verify: `k8s/monitoring/configmaps/grafana-dashboards.yml`
- Verify: `k8s/monitoring/configmaps/prometheus-config.yml`
- Verify: `java/k8s/configmaps/rabbitmq-definitions.yml`
- Verify: `java/k8s/deployments/rabbitmq.yml`
- Verify: `java/k8s/services/rabbitmq.yml`

- [ ] **Step 1: Run focused YAML validation**

Run:

```bash
python3 - <<'PY'
from pathlib import Path
import json
import yaml

paths = [
    "k8s/monitoring/configmaps/grafana-alerting.yml",
    "k8s/monitoring/configmaps/grafana-dashboards.yml",
    "k8s/monitoring/configmaps/prometheus-config.yml",
    "java/k8s/configmaps/rabbitmq-definitions.yml",
    "java/k8s/deployments/rabbitmq.yml",
    "java/k8s/services/rabbitmq.yml",
]

for path in paths:
    docs = list(yaml.safe_load_all(Path(path).read_text()))
    print(f"ok yaml {path}")
    if path.endswith("grafana-dashboards.yml"):
        data = docs[0]["data"]
        json.loads(data["go-services.json"])
        print("ok json embedded go-services.json")
PY
```

Expected: every file prints `ok yaml`, and the embedded dashboard prints `ok json`.

- [ ] **Step 2: Confirm alert and scrape expressions**

Run:

```bash
rg -n "saga_dlq_messages_total|rabbitmq_detailed_queue_messages|ecommerce\\.saga\\.dlq|job_name: \"rabbitmq\"|/metrics/detailed|rabbitmq_prometheus|containerPort: 15692|port: 15692" \
  k8s/monitoring/configmaps/grafana-alerting.yml \
  k8s/monitoring/configmaps/prometheus-config.yml \
  java/k8s/configmaps/rabbitmq-definitions.yml \
  java/k8s/deployments/rabbitmq.yml \
  java/k8s/services/rabbitmq.yml
```

Expected: output shows the existing DLQ event counter, the new depth metric, the RabbitMQ scrape job, the Prometheus plugin, and port `15692`.

## Task 4: Render And Run Policy Checks

**Files:**
- Verify: `k8s/monitoring/kustomization.yaml`
- Verify: `java/k8s/kustomization.yaml`
- Verify: `k8s/overlays/qa-java/kustomization.yaml`

- [ ] **Step 1: Run the repo Kubernetes policy check**

Run:

```bash
scripts/k8s-policy-check.sh k8s/monitoring java/k8s
```

Expected: exits `0`.

- [ ] **Step 2: Render Kustomize targets when available locally**

Run:

```bash
if command -v kubectl >/dev/null 2>&1; then
  kubectl kustomize k8s/monitoring >/tmp/rabbitmq-monitoring-kustomize.yml
  kubectl kustomize java/k8s >/tmp/rabbitmq-java-kustomize.yml
  kubectl kustomize k8s/overlays/qa-java >/tmp/rabbitmq-qa-java-kustomize.yml
elif command -v kustomize >/dev/null 2>&1; then
  kustomize build k8s/monitoring >/tmp/rabbitmq-monitoring-kustomize.yml
  kustomize build java/k8s >/tmp/rabbitmq-java-kustomize.yml
  kustomize build k8s/overlays/qa-java >/tmp/rabbitmq-qa-java-kustomize.yml
else
  echo "SKIP: kubectl and kustomize are not available locally"
fi
```

Expected: renders exit `0`, or prints the local-tooling skip message.

- [ ] **Step 3: Confirm rendered output when render files exist**

Run:

```bash
if [ -f /tmp/rabbitmq-monitoring-kustomize.yml ] && [ -f /tmp/rabbitmq-java-kustomize.yml ]; then
  rg -n "saga-dlq-depth-nonempty|rabbitmq_detailed_queue_messages|ecommerce\\.saga\\.dlq|rabbitmq_prometheus|containerPort: 15692|port: 15692|job_name: \"rabbitmq\"" \
    /tmp/rabbitmq-monitoring-kustomize.yml \
    /tmp/rabbitmq-java-kustomize.yml
fi
```

Expected: output includes the new alert, dashboard metric, Phase 1 scrape job, plugin, and port wiring.

## Task 5: Preflight, Commit, Push, PR

**Files:**
- Verify all changed files.

- [ ] **Step 1: Run relevant local preflight**

Run:

```bash
make grafana-sync-check
scripts/k8s-policy-check.sh k8s/monitoring java/k8s
```

Expected: both commands exit `0`. For this manifest-only observability change, use the focused validation plus policy check if full `make preflight` is too broad or blocked by unrelated local tooling.

- [ ] **Step 2: Review the final diff**

Run:

```bash
git diff --check
git diff --stat
git diff -- k8s/monitoring/configmaps/grafana-alerting.yml monitoring/grafana/dashboards/go-services.json k8s/monitoring/configmaps/grafana-dashboards.yml
git diff -- java/k8s/configmaps/rabbitmq-definitions.yml java/k8s/deployments/rabbitmq.yml java/k8s/services/rabbitmq.yml k8s/monitoring/configmaps/prometheus-config.yml
```

Expected: no whitespace errors; diff shows Phase 1 broker metrics plus Phase 2 alert/dashboard changes.

- [ ] **Step 3: Commit all Phase 1 and Phase 2 files**

Run:

```bash
git add \
  docs/superpowers/plans/2026-05-07-rabbitmq-broker-metrics.md \
  docs/superpowers/plans/2026-05-07-rabbitmq-saga-dlq-depth-alert.md \
  java/k8s/configmaps/rabbitmq-definitions.yml \
  java/k8s/deployments/rabbitmq.yml \
  java/k8s/services/rabbitmq.yml \
  k8s/monitoring/configmaps/prometheus-config.yml \
  k8s/monitoring/configmaps/grafana-alerting.yml \
  monitoring/grafana/dashboards/go-services.json \
  k8s/monitoring/configmaps/grafana-dashboards.yml
git commit -m "feat: add rabbitmq saga dlq depth alert"
```

- [ ] **Step 4: Push and create PR to `qa`**

Run:

```bash
git push -u origin feature/rabbitmq-broker-metrics
gh pr create --base qa --head feature/rabbitmq-broker-metrics --title "Add RabbitMQ broker metrics and saga DLQ depth alert" --body-file /tmp/rabbitmq-broker-metrics-pr.md
```

Expected: branch pushes successfully and GitHub returns a PR URL.

## Self-Review

- Spec coverage: the plan preserves the existing `saga_dlq_messages_total` alert, adds a separate current-depth alert using `rabbitmq_detailed_queue_messages{queue="ecommerce.saga.dlq"}`, groups alert series by `vhost` and `queue`, annotates both labels, uses the existing RabbitMQ dashboard row, and verifies Phase 1 broker metrics wiring.
- Placeholder scan: no `TBD`, vague implementation steps, or missing commands remain.
- Type/name consistency: alert UID, dashboard title, PromQL metric, RabbitMQ queue label, file paths, and validation commands are consistent across tasks.
