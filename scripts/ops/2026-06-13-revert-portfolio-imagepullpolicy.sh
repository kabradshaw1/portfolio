#!/usr/bin/env bash
# Reverts the 2026-06-13 emergency IPP=IfNotPresent patch back to Always
# on the 38 portfolio deployments now that CoreDNS external forwarding has
# been restored (see 2026-05-15-recover-coredns-external-forwarding.sh).
#
# Manifests declare imagePullPolicy=Always for :latest tag semantics — CI
# pushes :latest and kubelet auto-pulls on the next pod start. Keeping the
# emergency patch in place would break that contract. Reverting here makes
# the cluster match the manifests.
#
# Idempotent: safe to re-run.

set -euo pipefail

SSH_HOST="${SSH_HOST:-debian}"

TARGETS=(
  "ai-services:chat"
  "ai-services:debug"
  "ai-services:eval"
  "ai-services:eval-worker"
  "ai-services:ingestion"
  "ai-services:qdrant"
  "ai-services-qa:chat"
  "ai-services-qa:debug"
  "ai-services-qa:eval"
  "ai-services-qa:eval-worker"
  "ai-services-qa:ingestion"
  "go-ecommerce:go-ai-service"
  "go-ecommerce:go-analytics-service"
  "go-ecommerce:go-auth-service"
  "go-ecommerce:go-cart-service"
  "go-ecommerce:go-ecommerce-service"
  "go-ecommerce:go-order-projector"
  "go-ecommerce:go-order-service"
  "go-ecommerce:go-payment-service"
  "go-ecommerce:go-product-service"
  "go-ecommerce-qa:go-ai-service"
  "go-ecommerce-qa:go-analytics-service"
  "go-ecommerce-qa:go-auth-service"
  "go-ecommerce-qa:go-cart-service"
  "go-ecommerce-qa:go-ecommerce-service"
  "go-ecommerce-qa:go-order-projector"
  "go-ecommerce-qa:go-order-service"
  "go-ecommerce-qa:go-payment-service"
  "go-ecommerce-qa:go-product-service"
  "java-tasks:activity-service"
  "java-tasks:gateway-service"
  "java-tasks:notification-service"
  "java-tasks:task-service"
  "java-tasks-qa:activity-service"
  "java-tasks-qa:gateway-service"
  "java-tasks-qa:notification-service"
  "java-tasks-qa:task-service"
  "monitoring:grafana"
)

for t in "${TARGETS[@]}"; do
  ns="${t%%:*}"
  d="${t##*:}"
  echo "=== Patching ${ns}/${d} → imagePullPolicy=Always ==="
  ssh "$SSH_HOST" "kubectl -n ${ns} patch deploy/${d} --type=json \
    -p='[{\"op\":\"replace\",\"path\":\"/spec/template/spec/containers/0/imagePullPolicy\",\"value\":\"Always\"}]'"
done

echo "=== Waiting for rollouts ==="
for t in "${TARGETS[@]}"; do
  ns="${t%%:*}"
  d="${t##*:}"
  ssh "$SSH_HOST" "kubectl -n ${ns} rollout status deploy/${d} --timeout=180s" || true
done

echo "=== Post-state (non-Running pods) ==="
ssh "$SSH_HOST" "kubectl get pods -A | grep -vE 'Running|Completed' || echo 'all pods Running/Completed'"
