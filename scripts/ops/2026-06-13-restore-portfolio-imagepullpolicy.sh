#!/usr/bin/env bash
# Emergency recovery: patch portfolio deployments to imagePullPolicy=IfNotPresent
# so they use the cached :latest images on the minikube node instead of
# attempting a fresh ghcr.io pull through the broken Docker DNS forwarder
# (Tailscale MagicDNS @ 100.100.100.100 returning SERVFAIL after the
#  2026-06-13 ~08:23 EDT docker+tailscale restart on debian).
#
# Mirrors story/scripts/ops/restore-story-imagepullpolicy.sh for this repo's
# namespaces. All target :latest images verified cached on the minikube node
# at the time of writing.
#
# Durable follow-ups:
#   - update manifests under k8s/ai-services/deployments, k8s/overlays/*,
#     k8s/monitoring so imagePullPolicy=IfNotPresent matches cluster state
#   - real host DNS fix (Tailscale admin global nameservers, or
#     /etc/docker/daemon.json dns)
#
# Idempotent: safe to re-run.

set -euo pipefail

SSH_HOST="${SSH_HOST:-debian}"

# namespace:deployment pairs
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

echo "=== Pre-state (non-Running pods) ==="
ssh "$SSH_HOST" "kubectl get pods -A | grep -vE 'Running|Completed' || true"

for t in "${TARGETS[@]}"; do
  ns="${t%%:*}"
  d="${t##*:}"
  echo "=== Patching ${ns}/${d} → imagePullPolicy=IfNotPresent ==="
  ssh "$SSH_HOST" "kubectl -n ${ns} patch deploy/${d} --type=json \
    -p='[{\"op\":\"replace\",\"path\":\"/spec/template/spec/containers/0/imagePullPolicy\",\"value\":\"IfNotPresent\"}]'"
done

echo "=== Waiting for rollouts ==="
for t in "${TARGETS[@]}"; do
  ns="${t%%:*}"
  d="${t##*:}"
  ssh "$SSH_HOST" "kubectl -n ${ns} rollout status deploy/${d} --timeout=120s" || true
done

echo "=== Post-state (non-Running pods) ==="
ssh "$SSH_HOST" "kubectl get pods -A | grep -vE 'Running|Completed' || echo 'all pods Running/Completed'"
