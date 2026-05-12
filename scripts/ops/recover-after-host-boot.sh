#!/usr/bin/env bash
# Recover the Debian-hosted Minikube portfolio stack after a host reboot.
# Idempotent: yes.

set -euo pipefail

SYSTEMD_SERVICES=(docker tailscaled minikube minikube-tunnel cloudflared)
APP_NAMESPACES=(ai-services ai-services-qa go-ecommerce go-ecommerce-qa java-tasks java-tasks-qa monitoring)
WAIT_SECONDS="${WAIT_SECONDS:-600}"

run_payload() {
  bash -s -- "$WAIT_SECONDS" "${SYSTEMD_SERVICES[@]}" -- "${APP_NAMESPACES[@]}" <<'REMOTE'
set -euo pipefail

WAIT_SECONDS="$1"
shift

SYSTEMD_SERVICES=()
while [[ "$1" != "--" ]]; do
  SYSTEMD_SERVICES+=("$1")
  shift
done
shift

APP_NAMESPACES=("$@")
REGISTRY_HOSTS=(ghcr.io registry-1.docker.io)

deadline_epoch() {
  python3 -c 'import sys, time; print(int(time.time()) + int(sys.argv[1]))' "$1"
}

wait_until() {
  local label="$1"
  local command="$2"
  local deadline
  deadline="$(deadline_epoch "$WAIT_SECONDS")"

  echo "Waiting for ${label}..."
  until bash -c "$command"; do
    if [[ "$(date +%s)" -ge "$deadline" ]]; then
      echo "ERROR: timed out waiting for ${label}" >&2
      return 1
    fi
    sleep 5
  done
}

wait_for_systemd_service() {
  local service="$1"
  wait_until "systemd service ${service}" "systemctl is-active --quiet '${service}'"
}

wait_for_node_ready() {
  wait_until "Kubernetes node readiness" "kubectl get nodes --no-headers 2>/dev/null | awk '{ok = ok || \$2 == \"Ready\"} END {exit ok ? 0 : 1}'"
}

minikube_dns_ok() {
  local host
  for host in "${REGISTRY_HOSTS[@]}"; do
    minikube ssh -- "nslookup '${host}' >/dev/null" </dev/null >/dev/null
  done
}

repair_minikube_dns() {
  echo "Repairing Minikube node DNS resolver..."
  minikube ssh -- \
    "printf '%s\n' 'nameserver 100.100.100.100' 'nameserver 1.1.1.1' 'nameserver 8.8.8.8' 'options ndots:0' | sudo tee /etc/resolv.conf >/dev/null" \
    </dev/null
}

ensure_minikube_dns() {
  echo "Checking Minikube registry DNS..."
  if minikube_dns_ok; then
    echo "Minikube registry DNS is healthy."
    return 0
  fi

  repair_minikube_dns
  wait_until "Minikube registry DNS" "minikube ssh -- 'nslookup ghcr.io >/dev/null && nslookup registry-1.docker.io >/dev/null' </dev/null"
}

list_image_pull_blocked_pods() {
  kubectl get pods -A -o json | python3 -c '
import json
import sys

payload = json.load(sys.stdin)
blocked_reasons = {"ErrImagePull", "ImagePullBackOff"}
for pod in payload.get("items", []):
    statuses = pod.get("status", {}).get("containerStatuses", [])
    if any(
        status.get("state", {}).get("waiting", {}).get("reason") in blocked_reasons
        for status in statuses
    ):
        metadata = pod["metadata"]
        print("{} {}".format(metadata["namespace"], metadata["name"]))
'
}

recycle_image_pull_blocked_pods() {
  local blocked_pods namespace pod
  blocked_pods="$(list_image_pull_blocked_pods)"

  if [[ -z "$blocked_pods" ]]; then
    echo "No pods are currently blocked on image pulls."
    return 0
  fi

  echo "$blocked_pods" | while read -r namespace pod; do
    [[ -n "$namespace" && -n "$pod" ]] || continue
    echo "Deleting ${namespace}/${pod} so its controller recreates it..."
    kubectl delete pod -n "$namespace" "$pod"
  done
}

wait_for_deployments() {
  local namespace
  for namespace in "${APP_NAMESPACES[@]}"; do
    echo "Waiting for deployments in ${namespace}..."
    kubectl wait --for=condition=available deployment --all -n "$namespace" --timeout="${WAIT_SECONDS}s"
  done
}

for service in "${SYSTEMD_SERVICES[@]}"; do
  wait_for_systemd_service "$service"
done

wait_for_node_ready
ensure_minikube_dns
recycle_image_pull_blocked_pods
wait_for_deployments

echo "Remaining non-running pods:"
kubectl get pods -A --field-selector=status.phase!=Running,status.phase!=Succeeded -o wide

echo "Postboot recovery complete."
REMOTE
}

if [[ "$(hostname -s)" == "debian" || "${POSTBOOT_RUN_LOCAL:-}" == "1" ]]; then
  run_payload
else
  ssh debian POSTBOOT_RUN_LOCAL=1 WAIT_SECONDS="$WAIT_SECONDS" bash -s < "$0"
fi
