#!/usr/bin/env bash
# Apply the prod eval RabbitMQ SealedSecret and wait for eval workloads.
# This addresses the 2026-05-20 ai-services eval-worker unavailable alert.
# Idempotent: yes; re-applying the same SealedSecret is a no-op.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SEALED_SECRET="${REPO_ROOT}/k8s/secrets/ai-services/eval-rabbitmq.sealed.yml"

if [[ ! -s "${SEALED_SECRET}" ]]; then
  echo "ERROR: missing ${SEALED_SECRET}" >&2
  exit 1
fi

MANIFEST_B64="$(base64 < "${SEALED_SECRET}" | tr -d '\n')"

ssh debian 'bash -s' -- "${MANIFEST_B64}" <<'REMOTE'
set -euo pipefail
MANIFEST_B64="$1"

printf '%s' "${MANIFEST_B64}" | base64 -d | kubectl apply -f -
for _ in $(seq 1 60); do
  if kubectl get secret eval-rabbitmq -n ai-services >/dev/null 2>&1; then
    break
  fi
  sleep 2
done
kubectl get secret eval-rabbitmq -n ai-services >/dev/null
kubectl wait --for=condition=available --timeout=180s deployment/eval-worker -n ai-services
kubectl wait --for=condition=available --timeout=180s deployment/eval -n ai-services
REMOTE
