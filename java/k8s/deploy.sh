#!/usr/bin/env bash
set -euo pipefail

# Deploy Java Task Management to Minikube
# Prerequisites: minikube running, kubectl configured

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Creating namespace..."
kubectl apply -f "$SCRIPT_DIR/namespace.yml"

# java-secrets (secrets/java-secrets.yml, NOT committed) must include these keys,
# in addition to postgres-password:
#   mongo-story-chat-password  — password for the Mongo story_chat app user
#                                (avoid " \ $ — injected unescaped into a JS string)
#   rabbit-story-password      — password for the RabbitMQ story user
#                                (must NOT contain a literal '|' — it is the sed delimiter)
# Values must match the corresponding Story credentials in the GalaxyVoyagers
# story-secrets Secret. Coordinate cross-repo; do not commit real values here.
echo "==> Applying secrets..."
kubectl apply -f "$SCRIPT_DIR/secrets/java-secrets.yml"

echo "==> Applying ConfigMaps..."
kubectl apply -f "$SCRIPT_DIR/configmaps/"

echo "==> Deploying infrastructure..."
kubectl apply -f "$SCRIPT_DIR/deployments/postgres.yml"
kubectl apply -f "$SCRIPT_DIR/deployments/mongodb.yml"
kubectl apply -f "$SCRIPT_DIR/deployments/redis.yml"
kubectl apply -f "$SCRIPT_DIR/deployments/rabbitmq.yml"
kubectl apply -f "$SCRIPT_DIR/services/postgres.yml"
kubectl apply -f "$SCRIPT_DIR/services/mongodb.yml"
kubectl apply -f "$SCRIPT_DIR/services/redis.yml"
kubectl apply -f "$SCRIPT_DIR/services/rabbitmq.yml"

echo "==> Waiting for infrastructure to be ready..."
kubectl wait --for=condition=available --timeout=120s deployment/postgres -n java-tasks
kubectl wait --for=condition=available --timeout=120s deployment/mongodb -n java-tasks
kubectl wait --for=condition=available --timeout=120s deployment/redis -n java-tasks
kubectl wait --for=condition=available --timeout=120s deployment/rabbitmq -n java-tasks

echo "==> Deploying application services..."
kubectl apply -f "$SCRIPT_DIR/deployments/task-service.yml"
kubectl apply -f "$SCRIPT_DIR/deployments/activity-service.yml"
kubectl apply -f "$SCRIPT_DIR/deployments/notification-service.yml"
kubectl apply -f "$SCRIPT_DIR/deployments/gateway-service.yml"
kubectl apply -f "$SCRIPT_DIR/services/task-service.yml"
kubectl apply -f "$SCRIPT_DIR/services/activity-service.yml"
kubectl apply -f "$SCRIPT_DIR/services/notification-service.yml"
kubectl apply -f "$SCRIPT_DIR/services/gateway-service.yml"

echo "==> Waiting for application services..."
kubectl wait --for=condition=available --timeout=180s deployment/task-service -n java-tasks
kubectl wait --for=condition=available --timeout=180s deployment/activity-service -n java-tasks
kubectl wait --for=condition=available --timeout=180s deployment/notification-service -n java-tasks
kubectl wait --for=condition=available --timeout=180s deployment/gateway-service -n java-tasks

echo ""
echo "==> All services deployed!"
echo "    Gateway URL: $(minikube service gateway-service -n java-tasks --url 2>/dev/null || echo 'Run: minikube service gateway-service -n java-tasks --url')"
echo "    GraphiQL:    <gateway-url>/graphiql"
echo "    RabbitMQ UI: kubectl port-forward svc/rabbitmq 15672:15672 -n java-tasks"
