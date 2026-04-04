#!/usr/bin/env bash
set -euo pipefail

# Deploy all services to Minikube
# Prerequisites: minikube running, kubectl configured
# Usage: ./k8s/deploy.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "==> Enabling NGINX Ingress Controller..."
minikube addons enable ingress 2>/dev/null || true

echo "==> Creating namespaces..."
kubectl apply -f "$SCRIPT_DIR/ai-services/namespace.yml"
kubectl apply -f "$REPO_DIR/java/k8s/namespace.yml"
kubectl apply -f "$SCRIPT_DIR/monitoring/namespace.yml"

echo "==> Applying secrets..."
kubectl apply -f "$REPO_DIR/java/k8s/secrets/java-secrets.yml"

echo "==> Applying ConfigMaps..."
kubectl apply -f "$SCRIPT_DIR/ai-services/configmaps/"
kubectl apply -f "$REPO_DIR/java/k8s/configmaps/"
kubectl apply -f "$SCRIPT_DIR/monitoring/configmaps/"

echo "==> Deploying ai-services (Qdrant + Ollama)..."
kubectl apply -f "$SCRIPT_DIR/ai-services/services/ollama.yml"
kubectl apply -f "$SCRIPT_DIR/ai-services/deployments/qdrant.yml"
kubectl apply -f "$SCRIPT_DIR/ai-services/services/qdrant.yml"

echo "==> Waiting for Qdrant..."
kubectl wait --for=condition=available --timeout=120s deployment/qdrant -n ai-services

echo "==> Deploying ai-services (Python services)..."
kubectl apply -f "$SCRIPT_DIR/ai-services/deployments/ingestion.yml"
kubectl apply -f "$SCRIPT_DIR/ai-services/deployments/chat.yml"
kubectl apply -f "$SCRIPT_DIR/ai-services/deployments/debug.yml"
kubectl apply -f "$SCRIPT_DIR/ai-services/services/ingestion.yml"
kubectl apply -f "$SCRIPT_DIR/ai-services/services/chat.yml"
kubectl apply -f "$SCRIPT_DIR/ai-services/services/debug.yml"

echo "==> Deploying java-tasks infrastructure..."
kubectl apply -f "$REPO_DIR/java/k8s/deployments/postgres.yml"
kubectl apply -f "$REPO_DIR/java/k8s/deployments/mongodb.yml"
kubectl apply -f "$REPO_DIR/java/k8s/deployments/redis.yml"
kubectl apply -f "$REPO_DIR/java/k8s/deployments/rabbitmq.yml"
kubectl apply -f "$REPO_DIR/java/k8s/services/postgres.yml"
kubectl apply -f "$REPO_DIR/java/k8s/services/mongodb.yml"
kubectl apply -f "$REPO_DIR/java/k8s/services/redis.yml"
kubectl apply -f "$REPO_DIR/java/k8s/services/rabbitmq.yml"

echo "==> Waiting for java-tasks infrastructure..."
kubectl wait --for=condition=available --timeout=120s deployment/postgres -n java-tasks
kubectl wait --for=condition=available --timeout=120s deployment/mongodb -n java-tasks
kubectl wait --for=condition=available --timeout=120s deployment/redis -n java-tasks
kubectl wait --for=condition=available --timeout=120s deployment/rabbitmq -n java-tasks

echo "==> Deploying java-tasks application services..."
kubectl apply -f "$REPO_DIR/java/k8s/deployments/task-service.yml"
kubectl apply -f "$REPO_DIR/java/k8s/deployments/activity-service.yml"
kubectl apply -f "$REPO_DIR/java/k8s/deployments/notification-service.yml"
kubectl apply -f "$REPO_DIR/java/k8s/deployments/gateway-service.yml"
kubectl apply -f "$REPO_DIR/java/k8s/services/task-service.yml"
kubectl apply -f "$REPO_DIR/java/k8s/services/activity-service.yml"
kubectl apply -f "$REPO_DIR/java/k8s/services/notification-service.yml"
kubectl apply -f "$REPO_DIR/java/k8s/services/gateway-service.yml"

echo "==> Deploying monitoring..."
kubectl apply -f "$SCRIPT_DIR/monitoring/deployments/prometheus.yml"
kubectl apply -f "$SCRIPT_DIR/monitoring/services/prometheus.yml"
kubectl apply -f "$SCRIPT_DIR/monitoring/deployments/grafana.yml"
kubectl apply -f "$SCRIPT_DIR/monitoring/services/grafana.yml"

echo "==> Applying Ingress resources..."
kubectl apply -f "$SCRIPT_DIR/ai-services/ingress.yml"
kubectl apply -f "$REPO_DIR/java/k8s/ingress.yml"
kubectl apply -f "$REPO_DIR/java/k8s/ingress-rabbitmq.yml"
kubectl apply -f "$SCRIPT_DIR/monitoring/ingress.yml"

echo "==> Waiting for all application services..."
kubectl wait --for=condition=available --timeout=180s deployment/ingestion -n ai-services
kubectl wait --for=condition=available --timeout=180s deployment/chat -n ai-services
kubectl wait --for=condition=available --timeout=180s deployment/debug -n ai-services
kubectl wait --for=condition=available --timeout=180s deployment/task-service -n java-tasks
kubectl wait --for=condition=available --timeout=180s deployment/activity-service -n java-tasks
kubectl wait --for=condition=available --timeout=180s deployment/notification-service -n java-tasks
kubectl wait --for=condition=available --timeout=180s deployment/gateway-service -n java-tasks
kubectl wait --for=condition=available --timeout=120s deployment/prometheus -n monitoring
kubectl wait --for=condition=available --timeout=120s deployment/grafana -n monitoring

echo ""
echo "==> All services deployed!"
echo ""
echo "    Namespaces:"
echo "      ai-services  — Python AI services + Qdrant"
echo "      java-tasks   — Java microservices + databases"
echo "      monitoring   — Prometheus + Grafana"
echo ""
echo "    Next steps:"
echo "      1. Run 'minikube tunnel' in a separate terminal (requires sudo)"
echo "      2. Access services at http://localhost/"
echo ""
echo "    Endpoints (via Ingress):"
echo "      /ingestion/*    — Document ingestion API"
echo "      /chat/*         — RAG chat API"
echo "      /debug/*        — Debug assistant API"
echo "      /graphql        — Java GraphQL API"
echo "      /graphiql       — GraphQL IDE"
echo "      /api/auth/*     — OAuth authentication"
echo "      /grafana/       — Monitoring dashboards"
echo "      /rabbitmq/      — Message broker UI"
echo ""
echo "    Verify: kubectl get ingress --all-namespaces"
