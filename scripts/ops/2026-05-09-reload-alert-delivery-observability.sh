#!/usr/bin/env bash
# Restart Prometheus and Grafana so subPath-mounted monitoring config picks up
# the alert-delivery verification fixes.
# Idempotent: yes.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

echo "Applying committed monitoring manifests needed for alert-delivery verification..."
{
  printf '%s\n' '---'
  cat "${REPO_ROOT}/k8s/monitoring/configmaps/prometheus-config.yml"
  printf '%s\n' '---'
  cat "${REPO_ROOT}/k8s/monitoring/configmaps/grafana-datasource.yml"
  printf '%s\n' '---'
  cat "${REPO_ROOT}/k8s/monitoring/configmaps/grafana-dashboards.yml"
  printf '%s\n' '---'
  cat "${REPO_ROOT}/k8s/monitoring/deployments/prometheus.yml"
} | ssh debian "kubectl apply -f -"

ssh debian bash <<'REMOTE'
set -euo pipefail

NAMESPACE=monitoring
PROMETHEUS_DEPLOYMENT=prometheus
GRAFANA_DEPLOYMENT=grafana
PROMETHEUS_DATASOURCE_UID=PBFA97CFB590B2093

echo "Verifying desired Prometheus ConfigMap contains the Grafana scrape job..."
if ! kubectl get configmap prometheus-config -n "${NAMESPACE}" -o jsonpath='{.data.prometheus\.yml}' \
  | grep -q 'job_name: "grafana"'; then
  echo "ERROR: prometheus-config does not contain the Grafana scrape job." >&2
  exit 1
fi

echo "Verifying desired Grafana datasource ConfigMap declares prometheusType..."
if ! kubectl get configmap grafana-datasource -n "${NAMESPACE}" -o jsonpath='{.data.datasources\.yml}' \
  | sed -n '/uid: PBFA97CFB590B2093/,/name: Loki/p' \
  | grep -q 'prometheusType: Prometheus'; then
  echo "ERROR: Grafana Prometheus datasource prometheusType is not Prometheus." >&2
  exit 1
fi

echo "Restarting Prometheus to refresh subPath-mounted config..."
kubectl rollout restart "deployment/${PROMETHEUS_DEPLOYMENT}" -n "${NAMESPACE}"
kubectl rollout status "deployment/${PROMETHEUS_DEPLOYMENT}" -n "${NAMESPACE}" --timeout=120s

echo "Restarting Grafana to refresh datasource provisioning..."
kubectl rollout restart "deployment/${GRAFANA_DEPLOYMENT}" -n "${NAMESPACE}"
kubectl rollout status "deployment/${GRAFANA_DEPLOYMENT}" -n "${NAMESPACE}" --timeout=120s

echo "Verifying live Prometheus has the Grafana scrape target..."
if ! kubectl exec -n "${NAMESPACE}" "deployment/${PROMETHEUS_DEPLOYMENT}" -- \
  wget -qO- 'http://localhost:9090/api/v1/targets?state=any' \
  | python3 -c '
import json, sys
payload = json.load(sys.stdin)
for target in payload["data"].get("activeTargets", []):
    if target.get("labels", {}).get("job") == "grafana":
        raise SystemExit(0 if target.get("health") == "up" else 2)
raise SystemExit(1)
'; then
  echo "ERROR: live Prometheus does not have a healthy Grafana scrape target." >&2
  exit 1
fi

echo "Verifying live Grafana datasource has prometheusType..."
live_prometheus_type=$(
  kubectl exec -n "${NAMESPACE}" "deployment/${GRAFANA_DEPLOYMENT}" -- \
    wget -qO- "http://localhost:3000/api/datasources/uid/${PROMETHEUS_DATASOURCE_UID}" \
    | python3 -c 'import json, sys; print(json.load(sys.stdin).get("jsonData", {}).get("prometheusType", ""))'
)

if [[ "${live_prometheus_type}" != "Prometheus" ]]; then
  echo "ERROR: live Grafana datasource prometheusType is not Prometheus." >&2
  echo "actual: ${live_prometheus_type:-<empty>}" >&2
  exit 1
fi

echo "Alert-delivery observability config is live."
REMOTE
