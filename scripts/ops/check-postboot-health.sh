#!/usr/bin/env bash
# Read-only postboot health check for the Debian-hosted portfolio stack.

set -euo pipefail

run_payload() {
  bash <<'REMOTE'
set -euo pipefail

status=0
APP_NAMESPACES=(ai-services ai-services-qa go-ecommerce go-ecommerce-qa java-tasks java-tasks-qa monitoring)

fail() {
  echo "ERROR: $*" >&2
  status=1
}

check_node_ready() {
  echo "Checking Kubernetes node readiness..."
  if ! kubectl get nodes --no-headers | awk '{ok = ok || $2 == "Ready"} END {exit ok ? 0 : 1}'; then
    fail "no Kubernetes node is Ready"
  fi
}

check_deployments_available() {
  local namespace unavailable
  echo "Checking deployment availability..."
  for namespace in "${APP_NAMESPACES[@]}"; do
    unavailable="$(
      kubectl get deployment -n "$namespace" -o json | python3 -c '
import json
import sys

payload = json.load(sys.stdin)
for deploy in payload.get("items", []):
    spec = deploy.get("spec", {})
    status = deploy.get("status", {})
    desired = int(spec.get("replicas", 1) or 0)
    available = int(status.get("availableReplicas", 0) or 0)
    if desired > available:
        print("{}/{} desired={} available={}".format(
            deploy["metadata"]["namespace"],
            deploy["metadata"]["name"],
            desired,
            available,
        ))
'
    )"
    if [[ -n "$unavailable" ]]; then
      echo "$unavailable" >&2
      fail "unavailable deployments in ${namespace}"
    fi
  done
}

check_image_pull_failures() {
  local blocked
  echo "Checking image-pull failures..."
  blocked="$(
    kubectl get pods -A -o json | python3 -c '
import json
import sys

payload = json.load(sys.stdin)
blocked_reasons = {"ErrImagePull", "ImagePullBackOff"}
for pod in payload.get("items", []):
    statuses = pod.get("status", {}).get("containerStatuses", [])
    for status in statuses:
        reason = status.get("state", {}).get("waiting", {}).get("reason")
        if reason in blocked_reasons:
            print("{}/{} {}".format(
                pod["metadata"]["namespace"],
                pod["metadata"]["name"],
                reason,
            ))
'
  )"
  if [[ -n "$blocked" ]]; then
    echo "$blocked" >&2
    fail "pods blocked on image pulls"
  fi
}

check_endpoint() {
  local namespace="$1"
  local service="$2"
  local endpoint_count
  endpoint_count="$(
    kubectl get endpoints "$service" -n "$namespace" -o json | python3 -c '
import json
import sys

payload = json.load(sys.stdin)
count = 0
for subset in payload.get("subsets", []):
    count += len(subset.get("addresses", []))
print(count)
'
  )"
  if [[ "$endpoint_count" -lt 1 ]]; then
    fail "service ${namespace}/${service} has no endpoints"
  else
    echo "endpoint ${namespace}/${service} count=${endpoint_count}"
  fi
}

check_representative_endpoints() {
  echo "Checking representative service endpoints..."
  check_endpoint go-ecommerce go-auth-service
  check_endpoint go-ecommerce go-product-service
  check_endpoint java-tasks gateway-service
  check_endpoint ai-services ingestion
  check_endpoint monitoring grafana
}

check_url() {
  local label="$1"
  local url="$2"
  local expected="$3"
  local code
  code="$(curl -k -sS -o /dev/null -w '%{http_code}' --connect-timeout 8 --max-time 20 "$url" || true)"
  echo "url ${label} code=${code}"
  if [[ "$code" != "$expected" ]]; then
    fail "${label} expected HTTP ${expected}, got ${code}"
  fi
}

check_external_routes() {
  echo "Checking representative external routes..."
  check_url "prod go auth" "https://api.kylebradshaw.dev/go-auth/health" "200"
  check_url "qa go auth" "https://qa-api.kylebradshaw.dev/go-auth/health" "200"
  check_url "prod java gateway" "https://api.kylebradshaw.dev/actuator/health" "200"
  check_url "qa java gateway" "https://qa-api.kylebradshaw.dev/actuator/health" "200"
  check_url "prod ai ingestion" "https://api.kylebradshaw.dev/ingestion/health" "200"
  check_url "qa ai ingestion" "https://qa-api.kylebradshaw.dev/ingestion/health" "200"
  check_url "grafana" "https://grafana.kylebradshaw.dev/" "200"
}

check_node_ready
check_deployments_available
check_image_pull_failures
check_representative_endpoints
check_external_routes

if [[ "$status" -eq 0 ]]; then
  echo "Postboot health check passed."
else
  echo "Postboot health check failed." >&2
fi

exit "$status"
REMOTE
}

if [[ "$(hostname -s)" == "debian" || "${POSTBOOT_RUN_LOCAL:-}" == "1" ]]; then
  run_payload
else
  ssh debian "POSTBOOT_RUN_LOCAL=1 bash -s" < "$0"
fi
