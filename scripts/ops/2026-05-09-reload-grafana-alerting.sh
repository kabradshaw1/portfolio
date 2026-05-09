#!/usr/bin/env bash
# Restart Grafana so subPath-mounted alerting provisioning picks up the current ConfigMap.
# This fixes stale pg-auto-explain-stalled alert state after the rule was reduced from 12h to 1h.
# Idempotent: yes.

set -euo pipefail

ssh debian bash <<'REMOTE'
set -euo pipefail

NAMESPACE=monitoring
DEPLOYMENT=grafana
RULE_UID=pg-auto-explain-stalled
EXPECTED_EXPR='sum(count_over_time({namespace="java-tasks", app="postgres"} |= "duration:" |= "plan:" [1h]))'

echo "Verifying desired Grafana alerting ConfigMap content..."
config_expr=$(
  kubectl get configmap grafana-alerting -n "${NAMESPACE}" -o jsonpath='{.data.alerting\.yml}' \
    | sed -n '/uid: pg-auto-explain-stalled/,/refId: B/p' \
    | sed -n 's/.*expr: '"'"'\(.*\)'"'"'.*/\1/p'
)

if [[ "${config_expr}" != "${EXPECTED_EXPR}" ]]; then
  echo "ERROR: grafana-alerting ConfigMap does not contain the expected expression." >&2
  echo "expected: ${EXPECTED_EXPR}" >&2
  echo "actual:   ${config_expr:-<empty>}" >&2
  exit 1
fi

echo "Restarting Grafana to refresh subPath-mounted provisioning file..."
kubectl rollout restart "deployment/${DEPLOYMENT}" -n "${NAMESPACE}"
kubectl rollout status "deployment/${DEPLOYMENT}" -n "${NAMESPACE}" --timeout=120s

echo "Verifying live Grafana provisioning API..."
live_expr=$(
  kubectl exec -n "${NAMESPACE}" "deployment/${DEPLOYMENT}" -- \
    wget -qO- "http://localhost:3000/api/v1/provisioning/alert-rules/${RULE_UID}" \
    | python3 -c 'import json, sys; print(json.load(sys.stdin)["data"][0]["model"]["expr"])'
)

if [[ "${live_expr}" != "${EXPECTED_EXPR}" ]]; then
  echo "ERROR: Grafana still serves a stale alert expression after rollout." >&2
  echo "expected: ${EXPECTED_EXPR}" >&2
  echo "actual:   ${live_expr:-<empty>}" >&2
  exit 1
fi

echo "Verifying Grafana has no active alerts..."
active_alerts=$(
  kubectl exec -n "${NAMESPACE}" "deployment/${DEPLOYMENT}" -- \
    wget -qO- 'http://localhost:3000/api/alertmanager/grafana/api/v2/alerts' \
    | python3 -c 'import json, sys; print(len(json.load(sys.stdin)))'
)

if [[ "${active_alerts}" != "0" ]]; then
  echo "ERROR: Grafana still has active alerts after rollout: ${active_alerts}" >&2
  exit 1
fi

echo "Grafana alerting provisioning is current and no alerts are active."
REMOTE
