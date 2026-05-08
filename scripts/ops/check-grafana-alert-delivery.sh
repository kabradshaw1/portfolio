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
