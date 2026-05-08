# Grafana Alert Delivery Verification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prove Grafana alert delivery to Telegram with a provisioned canary, dashboard visibility, a read-only verification command, and a fix for the noisy `pg-auto-explain-stalled` Loki range failure.

**Architecture:** Keep the alerting path declarative: Grafana rules stay in the provisioned alerting ConfigMap, dashboards are edited in source JSON and synced into the Kubernetes ConfigMap, and runtime checks are read-only diagnostics. Prometheus should scrape Grafana `/metrics` so dashboard panels and the verification command use first-class alerting metrics instead of broad log scans.

**Tech Stack:** Kubernetes YAML, Grafana provisioned alerting, Grafana dashboard JSON, Prometheus scrape config, Bash, `curl`/`wget`, `ssh debian`, read-only `kubectl exec`.

---

### Task 1: Scrape Grafana Metrics

**Files:**
- Modify: `k8s/monitoring/configmaps/prometheus-config.yml`

- [ ] **Step 1: Add Grafana as a Prometheus scrape target**

Add this scrape config immediately after the existing `prometheus` scrape job:

```yaml
      - job_name: "grafana"
        metrics_path: /metrics
        static_configs:
          - targets: ["grafana.monitoring.svc.cluster.local:3000"]
```

The top of `k8s/monitoring/configmaps/prometheus-config.yml` should read:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: prometheus-config
  namespace: monitoring
data:
  prometheus.yml: |
    global:
      scrape_interval: 15s
      evaluation_interval: 15s

    scrape_configs:
      - job_name: "prometheus"
        static_configs:
          - targets: ["localhost:9090"]

      - job_name: "grafana"
        metrics_path: /metrics
        static_configs:
          - targets: ["grafana.monitoring.svc.cluster.local:3000"]

      - job_name: "windows"
        static_configs:
          - targets: ["host.minikube.internal:9182"]
```

- [ ] **Step 2: Validate the YAML parses**

Run:

```bash
ruby -e 'require "yaml"; YAML.load_file("k8s/monitoring/configmaps/prometheus-config.yml"); puts "ok"'
```

Expected: `ok`

- [ ] **Step 3: Commit the scrape target**

Run:

```bash
git add k8s/monitoring/configmaps/prometheus-config.yml
git commit -m "Add Grafana metrics scrape target"
```

Expected: commit includes only `k8s/monitoring/configmaps/prometheus-config.yml`.

### Task 2: Provision Alert Delivery Canary And Fix Noisy Loki Rule

**Files:**
- Modify: `k8s/monitoring/configmaps/grafana-alerting.yml`

- [ ] **Step 1: Add the alert delivery canary rule**

In the `Operational` alert group, add this rule before `circuit-breaker-open`:

```yaml
          - uid: alert-delivery-canary
            title: Alert Delivery Canary
            noDataState: OK
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: '((hour(vector(time())) == 15) and (minute(vector(time())) < 5))'
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
            for: 0s
            labels:
              severity: info
              purpose: alert-delivery-canary
            annotations:
              summary: "Daily alert delivery canary. No operator action required."
              description: "Fires daily from 15:00-15:05 UTC to verify Grafana alert delivery to Telegram."
```

This canary fires from 15:00-15:05 UTC, which is 09:00-09:05 America/Guatemala.

- [ ] **Step 2: Fix `pg-auto-explain-stalled` query range**

Replace the current `pg-auto-explain-stalled` block with this version:

```yaml
          - uid: pg-auto-explain-stalled
            title: Postgres auto_explain Plans Not Flowing
            noDataState: OK
            condition: C
            data:
              - refId: A
                relativeTimeRange: { from: 82800, to: 0 }
                datasourceUid: loki
                model:
                  expr: 'sum(count_over_time({namespace="java-tasks", app="postgres"} |= "auto_explain" [23h]))'
                  refId: A
              - refId: B
                relativeTimeRange: { from: 82800, to: 0 }
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange: { from: 82800, to: 0 }
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator: { type: lt, params: [1] }
                  refId: C
            for: 5m
            labels:
              severity: warning
            annotations:
              summary: "No auto_explain log lines in 23h — query observability is silently broken"
```

- [ ] **Step 3: Validate the alerting YAML parses**

Run:

```bash
ruby -e 'require "yaml"; YAML.load_file("k8s/monitoring/configmaps/grafana-alerting.yml"); puts "ok"'
```

Expected: `ok`

- [ ] **Step 4: Inspect the exact canary and range values**

Run:

```bash
rg -n "alert-delivery-canary|hour\\(vector\\(time\\(\\)\\)\\)|purpose: alert-delivery-canary|82800|\\[23h\\]|No auto_explain log lines in 23h" k8s/monitoring/configmaps/grafana-alerting.yml
```

Expected output includes all of these strings:

```text
alert-delivery-canary
hour(vector(time()))
purpose: alert-delivery-canary
82800
[23h]
No auto_explain log lines in 23h
```

- [ ] **Step 5: Commit alerting config**

Run:

```bash
git add k8s/monitoring/configmaps/grafana-alerting.yml
git commit -m "Add Grafana alert delivery canary"
```

Expected: commit includes only `k8s/monitoring/configmaps/grafana-alerting.yml`.

### Task 3: Add Alert Delivery Dashboard Row

**Files:**
- Modify: `monitoring/grafana/dashboards/system-overview.json`
- Modify: `k8s/monitoring/configmaps/grafana-dashboards.yml` through `make grafana-sync`

- [ ] **Step 1: Add the dashboard panels**

Append these panel objects to the end of the `panels` array in `monitoring/grafana/dashboards/system-overview.json`, after the existing `Go Services` panel. Keep the comma between the existing final panel and the new row.

```json
    {
      "title": "Alert Delivery",
      "type": "row",
      "collapsed": false,
      "gridPos": {
        "h": 1,
        "w": 24,
        "x": 0,
        "y": 31
      },
      "id": 23,
      "panels": []
    },
    {
      "title": "Notification Failures",
      "type": "stat",
      "gridPos": {
        "h": 4,
        "w": 6,
        "x": 0,
        "y": 32
      },
      "id": 24,
      "datasource": {
        "type": "prometheus",
        "uid": ""
      },
      "targets": [
        {
          "expr": "sum(increase(grafana_alerting_notifications_failed_total[24h])) or vector(0)",
          "legendFormat": "failures",
          "refId": "A"
        }
      ],
      "fieldConfig": {
        "defaults": {
          "unit": "short",
          "thresholds": {
            "mode": "absolute",
            "steps": [
              { "color": "green", "value": null },
              { "color": "red", "value": 1 }
            ]
          }
        },
        "overrides": []
      },
      "options": {
        "colorMode": "value",
        "graphMode": "none",
        "reduceOptions": {
          "calcs": ["lastNotNull"]
        }
      }
    },
    {
      "title": "Evaluation Failures",
      "type": "stat",
      "gridPos": {
        "h": 4,
        "w": 6,
        "x": 6,
        "y": 32
      },
      "id": 25,
      "datasource": {
        "type": "prometheus",
        "uid": ""
      },
      "targets": [
        {
          "expr": "sum(increase(grafana_alerting_rule_evaluation_failures_total[24h])) or vector(0)",
          "legendFormat": "failures",
          "refId": "A"
        }
      ],
      "fieldConfig": {
        "defaults": {
          "unit": "short",
          "thresholds": {
            "mode": "absolute",
            "steps": [
              { "color": "green", "value": null },
              { "color": "red", "value": 1 }
            ]
          }
        },
        "overrides": []
      },
      "options": {
        "colorMode": "value",
        "graphMode": "none",
        "reduceOptions": {
          "calcs": ["lastNotNull"]
        }
      }
    },
    {
      "title": "Notification Duration p95",
      "type": "timeseries",
      "gridPos": {
        "h": 4,
        "w": 6,
        "x": 12,
        "y": 32
      },
      "id": 26,
      "datasource": {
        "type": "prometheus",
        "uid": ""
      },
      "targets": [
        {
          "expr": "histogram_quantile(0.95, sum(rate(grafana_alerting_notification_latency_seconds_bucket[5m])) by (le))",
          "legendFormat": "p95",
          "refId": "A"
        }
      ],
      "fieldConfig": {
        "defaults": {
          "unit": "s",
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
    },
    {
      "title": "Canary Alert Active",
      "type": "stat",
      "gridPos": {
        "h": 4,
        "w": 6,
        "x": 18,
        "y": 32
      },
      "id": 27,
      "datasource": {
        "type": "prometheus",
        "uid": ""
      },
      "targets": [
        {
          "expr": "max(GRAFANA_ALERTS{alertname=\"Alert Delivery Canary\"}) or vector(0)",
          "legendFormat": "active",
          "refId": "A"
        }
      ],
      "fieldConfig": {
        "defaults": {
          "unit": "short",
          "thresholds": {
            "mode": "absolute",
            "steps": [
              { "color": "green", "value": null },
              { "color": "yellow", "value": 1 }
            ]
          }
        },
        "overrides": []
      },
      "options": {
        "colorMode": "value",
        "graphMode": "none",
        "reduceOptions": {
          "calcs": ["lastNotNull"]
        }
      }
    }
```

- [ ] **Step 2: Validate dashboard JSON**

Run:

```bash
python3 -m json.tool monitoring/grafana/dashboards/system-overview.json >/tmp/system-overview.json.checked
```

Expected: command exits 0.

- [ ] **Step 3: Sync dashboard ConfigMap**

Run:

```bash
make grafana-sync
```

Expected output includes:

```text
updated k8s/monitoring/configmaps/grafana-dashboards.yml
```

If the script prints a different success line, continue if `git diff -- k8s/monitoring/configmaps/grafana-dashboards.yml` shows only generated dashboard JSON drift from `system-overview.json`.

- [ ] **Step 4: Verify sync check passes**

Run:

```bash
make grafana-sync-check
```

Expected output includes:

```text
grafana dashboards in sync
```

- [ ] **Step 5: Commit dashboard changes**

Run:

```bash
git add monitoring/grafana/dashboards/system-overview.json k8s/monitoring/configmaps/grafana-dashboards.yml
git commit -m "Add alert delivery dashboard panels"
```

Expected: commit includes only the dashboard JSON and generated Grafana dashboard ConfigMap.

### Task 4: Add Read-Only Alert Delivery Verification Command

**Files:**
- Create: `scripts/ops/check-grafana-alert-delivery.sh`

- [ ] **Step 1: Create the verification script**

Create `scripts/ops/check-grafana-alert-delivery.sh` with this content:

```bash
#!/usr/bin/env bash
set -euo pipefail

LOOKBACK_HOURS=24

while [[ $# -gt 0 ]]; do
  case "$1" in
    --hours)
      LOOKBACK_HOURS="$2"
      shift 2
      ;;
    -h|--help)
      cat <<'USAGE'
Usage:
  scripts/ops/check-grafana-alert-delivery.sh [--hours N]

Read-only check for Grafana alert delivery health. Reports whether the alert
delivery canary is present, whether notification/evaluation failures occurred
inside the lookback window, and whether filtered Grafana logs show alerting
errors.
USAGE
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      exit 1
      ;;
  esac
done

if ! [[ "$LOOKBACK_HOURS" =~ ^[0-9]+$ ]] || [[ "$LOOKBACK_HOURS" -lt 1 ]]; then
  echo "Error: --hours must be a positive integer" >&2
  exit 2
fi

PROM_URL="http://prometheus.monitoring.svc.cluster.local:9090"
LOKI_URL="http://loki.monitoring.svc.cluster.local:3100"

prom_query() {
  local query="$1"
  local encoded
  encoded=$(python3 -c 'import sys, urllib.parse; print(urllib.parse.quote(sys.argv[1]))' "$query")
  ssh debian "kubectl exec -n monitoring deploy/prometheus -- wget -qO- '${PROM_URL}/api/v1/query?query=${encoded}'"
}

loki_query() {
  local query="$1"
  local encoded start end
  encoded=$(python3 -c 'import sys, urllib.parse; print(urllib.parse.quote(sys.argv[1]))' "$query")
  start=$(python3 -c 'import time, sys; print(int((time.time() - int(sys.argv[1]) * 3600) * 1000000000))' "$LOOKBACK_HOURS")
  end=$(python3 -c 'import time; print(int(time.time() * 1000000000))')
  ssh debian "kubectl exec -n monitoring loki-0 -- wget -qO- '${LOKI_URL}/loki/api/v1/query_range?query=${encoded}&limit=20&start=${start}&end=${end}'"
}

extract_scalar() {
  python3 -c '
import json, sys
payload = json.load(sys.stdin)
results = payload.get("data", {}).get("result", [])
if not results:
    print("0")
    raise SystemExit
value = results[0].get("value", ["", "0"])[1]
print(value)
'
}

extract_loki_count() {
  python3 -c '
import json, sys
payload = json.load(sys.stdin)
results = payload.get("data", {}).get("result", [])
print(sum(len(stream.get("values", [])) for stream in results))
'
}

status=0

echo "Grafana alert delivery check (${LOOKBACK_HOURS}h lookback)"

CANARY_PRESENT=$(prom_query 'count(GRAFANA_ALERTS{alertname="Alert Delivery Canary"})' | extract_scalar)
echo "canary_series=${CANARY_PRESENT}"
if [[ "${CANARY_PRESENT%.*}" -lt 1 ]]; then
  echo "ERROR: no GRAFANA_ALERTS series found for Alert Delivery Canary" >&2
  status=1
fi

NOTIFICATION_FAILURES=$(prom_query "sum(increase(grafana_alerting_notifications_failed_total[${LOOKBACK_HOURS}h])) or vector(0)" | extract_scalar)
echo "notification_failures=${NOTIFICATION_FAILURES}"
if python3 -c 'import sys; raise SystemExit(0 if float(sys.argv[1]) == 0 else 1)' "$NOTIFICATION_FAILURES"; then
  :
else
  echo "ERROR: Grafana notification failures detected" >&2
  status=1
fi

EVALUATION_FAILURES=$(prom_query "sum(increase(grafana_alerting_rule_evaluation_failures_total[${LOOKBACK_HOURS}h])) or vector(0)" | extract_scalar)
echo "evaluation_failures=${EVALUATION_FAILURES}"
if python3 -c 'import sys; raise SystemExit(0 if float(sys.argv[1]) == 0 else 1)' "$EVALUATION_FAILURES"; then
  :
else
  echo "ERROR: Grafana alert evaluation failures detected" >&2
  status=1
fi

LOG_ERRORS=$(loki_query '{namespace="monitoring", app="grafana"} |= "alert" |= "error"' | extract_loki_count)
echo "filtered_grafana_alert_error_logs=${LOG_ERRORS}"
if [[ "$LOG_ERRORS" -gt 0 ]]; then
  echo "ERROR: filtered Grafana alerting error logs found" >&2
  status=1
fi

echo "supporting_log_query={namespace=\"monitoring\", app=\"grafana\"} |= \"alert\" |= \"error\""

exit "$status"
```

- [ ] **Step 2: Make the script executable**

Run:

```bash
chmod +x scripts/ops/check-grafana-alert-delivery.sh
```

- [ ] **Step 3: Validate shell syntax**

Run:

```bash
bash -n scripts/ops/check-grafana-alert-delivery.sh
```

Expected: command exits 0 with no output.

- [ ] **Step 4: Validate help output**

Run:

```bash
scripts/ops/check-grafana-alert-delivery.sh --help
```

Expected output includes:

```text
Usage:
  scripts/ops/check-grafana-alert-delivery.sh [--hours N]
```

- [ ] **Step 5: Commit the script**

Run:

```bash
git add scripts/ops/check-grafana-alert-delivery.sh
git commit -m "Add Grafana alert delivery verification command"
```

Expected: commit includes only `scripts/ops/check-grafana-alert-delivery.sh`.

### Task 5: Final Local Verification

**Files:**
- Verify: `k8s/monitoring/configmaps/prometheus-config.yml`
- Verify: `k8s/monitoring/configmaps/grafana-alerting.yml`
- Verify: `monitoring/grafana/dashboards/system-overview.json`
- Verify: `k8s/monitoring/configmaps/grafana-dashboards.yml`
- Verify: `scripts/ops/check-grafana-alert-delivery.sh`

- [ ] **Step 1: Run YAML and JSON checks**

Run:

```bash
ruby -e 'require "yaml"; YAML.load_file("k8s/monitoring/configmaps/prometheus-config.yml"); YAML.load_file("k8s/monitoring/configmaps/grafana-alerting.yml"); puts "yaml ok"'
python3 -m json.tool monitoring/grafana/dashboards/system-overview.json >/tmp/system-overview.json.checked
```

Expected:

```text
yaml ok
```

- [ ] **Step 2: Run Grafana dashboard sync check**

Run:

```bash
make grafana-sync-check
```

Expected output includes:

```text
grafana dashboards in sync
```

- [ ] **Step 3: Run script syntax check**

Run:

```bash
bash -n scripts/ops/check-grafana-alert-delivery.sh
```

Expected: command exits 0 with no output.

- [ ] **Step 4: Inspect final diff**

Run:

```bash
git diff --stat HEAD~4..HEAD
```

Expected changed files are limited to:

```text
k8s/monitoring/configmaps/prometheus-config.yml
k8s/monitoring/configmaps/grafana-alerting.yml
monitoring/grafana/dashboards/system-overview.json
k8s/monitoring/configmaps/grafana-dashboards.yml
scripts/ops/check-grafana-alert-delivery.sh
```

- [ ] **Step 5: Record post-deploy verification command for the PR**

Include this in the PR body or handoff:

```bash
scripts/ops/check-grafana-alert-delivery.sh --hours 24
```

Expected after deployment:

```text
Grafana alert delivery check (24h lookback)
canary_series=1
notification_failures=0
evaluation_failures=0
filtered_grafana_alert_error_logs=0
```

If the command runs before the first 15:00-15:05 UTC canary window after deploy,
`canary_series` may still be absent. In that case, wait until after the canary
window and run the same command again.
