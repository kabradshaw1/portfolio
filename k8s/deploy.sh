#!/usr/bin/env bash
set -euo pipefail

# Deploy all services to Kubernetes
# Usage: ./k8s/deploy.sh [minikube|aws]
#   minikube (default) — deploy to local Minikube cluster
#   aws               — deploy to AWS EKS cluster

ENV="${1:-minikube}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

if [ "$ENV" != "minikube" ] && [ "$ENV" != "aws" ] && [ "$ENV" != "qa" ]; then
  echo "Usage: $0 [minikube|aws|qa]"
  exit 1
fi

echo "==> Deploying to: $ENV"

# --- Minikube-specific setup ---
if [ "$ENV" = "minikube" ]; then
  echo "==> Enabling NGINX Ingress Controller..."
  minikube addons enable ingress 2>/dev/null || true
fi

# --- QA-specific deploy ---
if [ "$ENV" = "qa" ]; then
  echo "==> Creating QA database (ecommercedb_qa) if not exists..."
  kubectl exec deployment/postgres -n java-tasks -- \
    psql -U taskuser -d taskdb -c "SELECT 1 FROM pg_database WHERE datname='ecommercedb_qa'" | grep -q 1 || \
    kubectl exec deployment/postgres -n java-tasks -- \
      psql -U taskuser -d taskdb -c "CREATE DATABASE ecommercedb_qa;"

  echo "==> Deploying ai-services-qa..."
  kubectl apply -k "$SCRIPT_DIR/overlays/qa"

  echo "==> Deploying java-tasks-qa..."
  kubectl apply -k "$SCRIPT_DIR/overlays/qa-java"

  echo "==> Installing cert-manager (if not already present)..."
  kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.17.2/cert-manager.yaml 2>/dev/null || true
  echo "==> Waiting for cert-manager..."
  kubectl wait --for=condition=available --timeout=120s deployment/cert-manager -n cert-manager 2>/dev/null || true
  kubectl wait --for=condition=available --timeout=120s deployment/cert-manager-webhook -n cert-manager 2>/dev/null || true

  echo "==> Applying cert-manager resources..."
  kubectl apply -f "$SCRIPT_DIR/cert-manager/cluster-issuer.yml"
  kubectl apply -f "$SCRIPT_DIR/cert-manager/qa-certificates.yml"

  echo "==> Deploying go-ecommerce-qa..."
  kubectl apply -k "$SCRIPT_DIR/overlays/qa-go"

  echo "==> Waiting for QA application services..."
  kubectl wait --for=condition=available --timeout=180s deployment/ingestion -n ai-services-qa
  kubectl wait --for=condition=available --timeout=180s deployment/chat -n ai-services-qa
  kubectl wait --for=condition=available --timeout=180s deployment/debug -n ai-services-qa
  kubectl wait --for=condition=available --timeout=180s deployment/task-service -n java-tasks-qa
  kubectl wait --for=condition=available --timeout=180s deployment/activity-service -n java-tasks-qa
  kubectl wait --for=condition=available --timeout=180s deployment/notification-service -n java-tasks-qa
  kubectl wait --for=condition=available --timeout=180s deployment/gateway-service -n java-tasks-qa
  kubectl wait --for=condition=available --timeout=180s deployment/go-auth-service -n go-ecommerce-qa
  kubectl wait --for=condition=available --timeout=180s deployment/go-order-service -n go-ecommerce-qa
  kubectl wait --for=condition=available --timeout=180s deployment/go-ai-service -n go-ecommerce-qa
  kubectl wait --for=condition=available --timeout=180s deployment/go-product-service -n go-ecommerce-qa
  kubectl wait --for=condition=available --timeout=180s deployment/go-cart-service -n go-ecommerce-qa
  kubectl wait --for=condition=available --timeout=180s deployment/go-payment-service -n go-ecommerce-qa

  echo "==> Seeding product documents for RAG (QA)..."
  "$REPO_DIR/scripts/seed-product-docs.sh" "http://$(minikube ip):80/qa/ingestion" || echo "WARN: Product doc seeding failed (non-fatal)"

  echo ""
  echo "==> QA environment deployed!"
  echo "    Backend: qa-api.kylebradshaw.dev"
  echo "    Frontend: qa.kylebradshaw.dev"
  exit 0
fi

# --- Secrets (applied directly — not managed by kustomize) ---
echo "==> Applying secrets..."
if [ -f "$REPO_DIR/java/k8s/secrets/java-secrets.yml" ]; then
  kubectl apply -f "$REPO_DIR/java/k8s/secrets/java-secrets.yml"
else
  echo "    WARN: java-secrets.yml not found — copy java-secrets.yml.template and fill in values"
fi

if [ -f "$REPO_DIR/go/k8s/secrets/go-secrets.yml" ]; then
  kubectl apply -f "$REPO_DIR/go/k8s/secrets/go-secrets.yml"
else
  echo "    WARN: go-secrets.yml not found — create go/k8s/secrets/go-secrets.yml with jwt-secret"
fi

# --- Deploy ai-services ---
echo "==> Deploying ai-services (Python)..."
kubectl apply -k "$SCRIPT_DIR/overlays/$ENV"

echo "==> Waiting for Qdrant..."
kubectl wait --for=condition=available --timeout=120s deployment/qdrant -n ai-services

# --- Deploy monitoring (same for both environments) ---
echo "==> Deploying monitoring..."
if [ -f "$SCRIPT_DIR/monitoring/secrets/grafana-secrets.yml" ]; then
  kubectl apply -f "$SCRIPT_DIR/monitoring/secrets/grafana-secrets.yml"
else
  echo "    WARN: grafana-secrets.yml not found — Telegram alerts will not work"
fi
kubectl apply -k "$SCRIPT_DIR/monitoring"

# --- Deploy java-tasks ---
echo "==> Deploying java-tasks..."
kubectl apply -k "$REPO_DIR/java/k8s/overlays/$ENV"

if [ "$ENV" = "minikube" ]; then
  echo "==> Waiting for java-tasks infrastructure..."
  kubectl wait --for=condition=available --timeout=120s deployment/postgres -n java-tasks
  kubectl wait --for=condition=available --timeout=120s deployment/mongodb -n java-tasks
  kubectl wait --for=condition=available --timeout=120s deployment/redis -n java-tasks
  kubectl wait --for=condition=available --timeout=120s deployment/rabbitmq -n java-tasks
fi

# --- Deploy go-ecommerce ---
echo "==> Deploying go-ecommerce..."
echo "==> Installing cert-manager (if not already present)..."
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.17.2/cert-manager.yaml 2>/dev/null || true
echo "==> Waiting for cert-manager..."
kubectl wait --for=condition=available --timeout=120s deployment/cert-manager -n cert-manager 2>/dev/null || true
kubectl wait --for=condition=available --timeout=120s deployment/cert-manager-webhook -n cert-manager 2>/dev/null || true

echo "==> Applying cert-manager resources..."
kubectl apply -f "$SCRIPT_DIR/cert-manager/cluster-issuer.yml"
kubectl apply -f "$SCRIPT_DIR/cert-manager/ca-certificate.yml"
kubectl apply -f "$SCRIPT_DIR/cert-manager/issuer.yml"
kubectl apply -f "$SCRIPT_DIR/cert-manager/certificates.yml"

kubectl apply -k "$REPO_DIR/go/k8s/overlays/$ENV"

echo "==> Waiting for all application services..."
kubectl wait --for=condition=available --timeout=180s deployment/ingestion -n ai-services
kubectl wait --for=condition=available --timeout=180s deployment/chat -n ai-services
kubectl wait --for=condition=available --timeout=180s deployment/debug -n ai-services
kubectl wait --for=condition=available --timeout=180s deployment/task-service -n java-tasks
kubectl wait --for=condition=available --timeout=180s deployment/activity-service -n java-tasks
kubectl wait --for=condition=available --timeout=180s deployment/notification-service -n java-tasks
kubectl wait --for=condition=available --timeout=180s deployment/gateway-service -n java-tasks
kubectl wait --for=condition=available --timeout=180s deployment/go-auth-service -n go-ecommerce
kubectl wait --for=condition=available --timeout=180s deployment/go-order-service -n go-ecommerce
kubectl wait --for=condition=available --timeout=180s deployment/go-ai-service -n go-ecommerce
kubectl wait --for=condition=available --timeout=180s deployment/go-product-service -n go-ecommerce
kubectl wait --for=condition=available --timeout=180s deployment/go-cart-service -n go-ecommerce
kubectl wait --for=condition=available --timeout=180s deployment/go-payment-service -n go-ecommerce
kubectl wait --for=condition=available --timeout=120s deployment/prometheus -n monitoring
kubectl wait --for=condition=available --timeout=120s deployment/kube-state-metrics -n monitoring
kubectl wait --for=condition=available --timeout=120s deployment/grafana -n monitoring

echo "==> Seeding product documents for RAG..."
"$REPO_DIR/scripts/seed-product-docs.sh" "http://$(minikube ip):80/ingestion" || echo "WARN: Product doc seeding failed (non-fatal)"

echo ""
echo "==> All services deployed! (env: $ENV)"
echo ""
echo "    Namespaces:"
echo "      ai-services    — Python AI services + Qdrant"
echo "      java-tasks     — Java microservices + databases"
echo "      go-ecommerce   — Go auth + ecommerce + AI agent services"
echo "      monitoring     — Prometheus + Grafana"
echo ""
if [ "$ENV" = "minikube" ]; then
  echo "    Next steps:"
  echo "      1. Run 'minikube tunnel' in a separate terminal (requires sudo)"
  echo "      2. Access services at http://localhost/"
else
  echo "    Next steps:"
  echo "      1. Point api.kylebradshaw.dev to the ALB hostname"
  echo "      2. Access services at https://api.kylebradshaw.dev/"
fi
echo ""
echo "    Endpoints (via Ingress):"
echo "      /ingestion/*    — Document ingestion API"
echo "      /chat/*         — RAG chat API"
echo "      /debug/*        — Debug assistant API"
echo "      /graphql        — Java GraphQL API"
echo "      /graphiql       — GraphQL IDE"
echo "      /auth/*         — OAuth authentication"
echo "      /go-auth/*      — Go auth API"
echo "      /go-orders/*       — Go ecommerce API"
echo "      /ai-api/*       — Go AI agent API"
echo "      /grafana/       — Monitoring dashboards"
echo "      /rabbitmq/      — Message broker UI"
echo ""
echo "    Verify: kubectl get ingress --all-namespaces"
