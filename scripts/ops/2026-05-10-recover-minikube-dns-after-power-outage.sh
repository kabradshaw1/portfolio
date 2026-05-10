#!/usr/bin/env bash
# Repair Minikube node DNS after the production host restarted and Docker's
# gateway resolver began returning SERVFAIL to image pulls.
# Idempotent: yes.

set -euo pipefail

ssh debian bash <<'REMOTE'
set -euo pipefail

echo "Checking current Minikube DNS state..."
minikube ssh -- 'cat /etc/resolv.conf' </dev/null

echo "Repairing Minikube node resolver configuration..."
minikube ssh -- \
  "printf '%s\n' 'nameserver 100.100.100.100' 'nameserver 1.1.1.1' 'nameserver 8.8.8.8' 'options ndots:0' | sudo tee /etc/resolv.conf >/dev/null" \
  </dev/null

echo "Verifying registry DNS from inside Minikube..."
minikube ssh -- 'nslookup ghcr.io >/dev/null' </dev/null
minikube ssh -- 'nslookup registry-1.docker.io >/dev/null' </dev/null

echo "Finding pods blocked on image pulls..."
blocked_pods=$(
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
)

if [[ -z "${blocked_pods}" ]]; then
  echo "No pods are currently blocked on image pulls."
else
  echo "${blocked_pods}" | while read -r namespace pod; do
    [[ -n "${namespace}" && -n "${pod}" ]] || continue
    echo "Deleting ${namespace}/${pod} so its controller recreates it with working DNS..."
    kubectl delete pod -n "${namespace}" "${pod}"
  done
fi

echo "Waiting for application deployments to become available..."
for namespace in ai-services ai-services-qa go-ecommerce go-ecommerce-qa java-tasks java-tasks-qa monitoring; do
  kubectl wait --for=condition=available deployment --all -n "${namespace}" --timeout=10m
done

echo "Current non-running pods:"
kubectl get pods -A --field-selector=status.phase!=Running,status.phase!=Succeeded -o wide

echo "Recovery complete."
REMOTE
