#!/usr/bin/env bash
set -euo pipefail

LOOKBACK_HOURS=24

while [[ $# -gt 0 ]]; do
  case "$1" in
    --hours)
      if [[ $# -lt 2 ]]; then
        echo "Error: --hours requires a value" >&2
        exit 2
      fi
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

if [[ "$LOOKBACK_HOURS" -gt 24 ]]; then
  echo "Error: --hours cannot exceed 24" >&2
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
try:
    payload = json.load(sys.stdin)
except json.JSONDecodeError as exc:
    print(f"ERROR: invalid API response JSON: {exc}", file=sys.stderr)
    raise SystemExit(1)
status = payload.get("status")
if status != "success":
    error_type = payload.get("errorType", "unknown")
    error = payload.get("error", "missing error message")
    print(f"ERROR: API query failed: status={status!r} errorType={error_type!r} error={error!r}", file=sys.stderr)
    raise SystemExit(1)
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
try:
    payload = json.load(sys.stdin)
except json.JSONDecodeError as exc:
    print(f"ERROR: invalid API response JSON: {exc}", file=sys.stderr)
    raise SystemExit(1)
status = payload.get("status")
if status != "success":
    error_type = payload.get("errorType", "unknown")
    error = payload.get("error", "missing error message")
    print(f"ERROR: API query failed: status={status!r} errorType={error_type!r} error={error!r}", file=sys.stderr)
    raise SystemExit(1)
results = payload.get("data", {}).get("result", [])
print(sum(len(stream.get("values", [])) for stream in results))
'
}

query_prom_scalar() {
  local label="$1"
  local query="$2"
  local output
  if ! output=$(prom_query "$query" | extract_scalar); then
    echo "ERROR: Prometheus query failed for ${label}" >&2
    return 1
  fi
  printf '%s\n' "$output"
}

query_loki_count() {
  local label="$1"
  local query="$2"
  local output
  if ! output=$(loki_query "$query" | extract_loki_count); then
    echo "ERROR: Loki query failed for ${label}" >&2
    return 1
  fi
  printf '%s\n' "$output"
}

status=0

echo "Grafana alert delivery check (${LOOKBACK_HOURS}h lookback)"

CANARY_PRESENT=$(query_prom_scalar "alert delivery canary series" 'count(GRAFANA_ALERTS{alertname="Alert Delivery Canary"})')
echo "canary_series=${CANARY_PRESENT}"
if [[ "${CANARY_PRESENT%.*}" -lt 1 ]]; then
  echo "ERROR: no GRAFANA_ALERTS series found for Alert Delivery Canary" >&2
  status=1
fi

NOTIFICATION_FAILURES=$(query_prom_scalar "notification failures" "sum(increase(grafana_alerting_notifications_failed_total[${LOOKBACK_HOURS}h])) or vector(0)")
echo "notification_failures=${NOTIFICATION_FAILURES}"
if python3 -c 'import sys; raise SystemExit(0 if float(sys.argv[1]) == 0 else 1)' "$NOTIFICATION_FAILURES"; then
  :
else
  echo "ERROR: Grafana notification failures detected" >&2
  status=1
fi

EVALUATION_FAILURES=$(query_prom_scalar "evaluation failures" "sum(increase(grafana_alerting_rule_evaluation_failures_total[${LOOKBACK_HOURS}h])) or vector(0)")
echo "evaluation_failures=${EVALUATION_FAILURES}"
if python3 -c 'import sys; raise SystemExit(0 if float(sys.argv[1]) == 0 else 1)' "$EVALUATION_FAILURES"; then
  :
else
  echo "ERROR: Grafana alert evaluation failures detected" >&2
  status=1
fi

LOG_ERRORS=$(query_loki_count "filtered Grafana alert error logs" '{namespace="monitoring", app="grafana"} |= "alert" |= "error"')
echo "filtered_grafana_alert_error_logs=${LOG_ERRORS}"
if [[ "$LOG_ERRORS" -gt 0 ]]; then
  echo "ERROR: filtered Grafana alerting error logs found" >&2
  status=1
fi

echo "supporting_log_query={namespace=\"monitoring\", app=\"grafana\"} |= \"alert\" |= \"error\""

exit "$status"
