# Unified Kubernetes Deployment with NGINX Ingress

## Context

The portfolio project runs two backend stacks on the Windows PC, currently both on Docker Compose:

- **Python stack** (root docker-compose): ingestion, chat, debug services + Qdrant + monitoring
- **Java stack** (java/docker-compose): gateway-service (GraphQL), task-service, activity-service, notification-service + Postgres, MongoDB, Redis, RabbitMQ

For a distributed systems role, running everything on Kubernetes (Minikube) is a much stronger signal than Docker Compose. Rather than bridging two separate networks (Docker Compose ↔ K8s), we move all services into Kubernetes with an NGINX Ingress Controller as the unified entry point.

Docker Compose files remain for local development (industry-standard pattern: Compose for dev, K8s for deploy).

## Design

### Architecture

```
Cloudflare Tunnel → Windows PC localhost:80 (via minikube tunnel)
│
NGINX Ingress Controller (Minikube addon)
│
├── ai-services namespace
│   ├── /ingestion/*    → ingestion service (Python/FastAPI)
│   ├── /chat/*         → chat service (Python/FastAPI)
│   ├── /debug/*        → debug service (Python/FastAPI)
│   └── qdrant          → vector database (internal only)
│
├── java-tasks namespace (existing manifests)
│   ├── /graphql        → gateway-service (Spring Boot GraphQL)
│   ├── /graphiql/**    → gateway-service (GraphQL IDE)
│   ├── /api/auth/*     → task-service (OAuth, token refresh)
│   ├── /rabbitmq/      → rabbitmq management UI
│   ├── task-service    → (internal, reached via gateway)
│   ├── activity-service → (internal, reached via gateway)
│   ├── notification-service → (internal, reached via gateway)
│   ├── postgres, mongodb, redis, rabbitmq → (internal)
│   └── /actuator/health → gateway-service health check
│
└── monitoring namespace
    ├── prometheus       → metrics collection
    ├── grafana          → dashboards (exposed via Ingress)
    └── (scrapes services across all namespaces)
```

### Namespaces

Three namespaces demonstrate multi-team cluster organization:

- **ai-services** — Python AI services (RAG, debug assistant) + Qdrant
- **java-tasks** — Java microservices (already exists) + infrastructure databases
- **monitoring** — Prometheus, Grafana

### Ingress Routing

A single NGINX Ingress resource routes all external traffic by path:

```yaml
# Ingress rules (across namespaces via ExternalName or cross-namespace backends)
/ingestion/*    → ingestion.ai-services:8000
/chat/*         → chat.ai-services:8000
/debug/*        → debug.ai-services:8000
/graphql        → gateway-service.java-tasks:8080
/graphiql       → gateway-service.java-tasks:8080
/api/auth/      → task-service.java-tasks:8081
/rabbitmq/      → rabbitmq.java-tasks:15672
/java/health    → gateway-service.java-tasks:8080  (rewrite to /actuator/health)
/grafana/       → grafana.monitoring:3000
```

Note: NGINX Ingress doesn't natively support cross-namespace backends. Each namespace gets its own Ingress resource, but all share the same Ingress controller and host.

### Ollama (External Service)

Ollama runs on the Windows host with the RTX 3090 — GPU passthrough into Minikube is impractical. Python services reach Ollama via a K8s ExternalName or Endpoints service:

```yaml
# In ai-services namespace
apiVersion: v1
kind: Service
metadata:
  name: ollama
  namespace: ai-services
spec:
  type: ExternalName
  externalName: host.minikube.internal  # Minikube's equivalent of host.docker.internal
```

Python service configs use `OLLAMA_BASE_URL=http://ollama:11434` — K8s DNS resolves to the host. This is a realistic pattern (external GPU inference service).

### Minikube Tunnel

`minikube tunnel` makes the Ingress LoadBalancer accessible on `localhost:80`. Cloudflare Tunnel config changes from `localhost:8000` to `localhost:80`.

`minikube tunnel` must run as a persistent background process. On the Windows PC, this can be a startup script or Windows service.

### CORS Configuration

All services that receive browser requests need `ALLOWED_ORIGINS` to include `https://kylebradshaw.dev`:
- gateway-service (GraphQL)
- task-service (OAuth auth)
- Python services (ingestion, chat, debug)

Each service manages its own CORS — the Ingress does NOT add CORS headers.

### GraphiQL

Enable GraphiQL on gateway-service (`GRAPHIQL_ENABLED=true`) so recruiters can explore the GraphQL API interactively at `https://api.kylebradshaw.dev/graphiql`.

### Frontend Environment

In production (Vercel), both env vars point to the same domain:
- `NEXT_PUBLIC_API_URL=https://api.kylebradshaw.dev`
- `NEXT_PUBLIC_GATEWAY_URL=https://api.kylebradshaw.dev`

The Ingress differentiates by path.

## New K8s Manifests Needed

### ai-services namespace

```
k8s/
├── ai-services/
│   ├── namespace.yml
│   ├── configmaps/
│   │   ├── ingestion-config.yml     # OLLAMA_BASE_URL, QDRANT_HOST, etc.
│   │   ├── chat-config.yml
│   │   └── debug-config.yml
│   ├── deployments/
│   │   ├── ingestion.yml
│   │   ├── chat.yml
│   │   ├── debug.yml
│   │   └── qdrant.yml
│   ├── services/
│   │   ├── ingestion.yml
│   │   ├── chat.yml
│   │   ├── debug.yml
│   │   ├── qdrant.yml
│   │   └── ollama.yml              # ExternalName → host
│   └── ingress.yml                 # /ingestion/*, /chat/*, /debug/*
```

### monitoring namespace

```
k8s/
├── monitoring/
│   ├── namespace.yml
│   ├── configmaps/
│   │   └── prometheus-config.yml    # Scrape configs for all namespaces
│   ├── deployments/
│   │   ├── prometheus.yml
│   │   └── grafana.yml
│   ├── services/
│   │   ├── prometheus.yml
│   │   └── grafana.yml
│   └── ingress.yml                 # /grafana/*
```

### java-tasks namespace (modifications)

```
java/k8s/
├── ingress.yml                      # NEW: /graphql, /graphiql, /api/auth/*, /rabbitmq/, /actuator/health
├── configmaps/
│   ├── gateway-service-config.yml   # UPDATE: ALLOWED_ORIGINS, GRAPHIQL_ENABLED
│   └── task-service-config.yml      # UPDATE: ALLOWED_ORIGINS
```

### Shared

```
k8s/
├── deploy.sh                        # Updated: deploys all 3 namespaces + enables ingress addon
```

## CI/CD Pipeline Changes

The current `ci.yml` deploy job SSHes to the Windows PC and runs `docker compose` commands. This needs to change to `kubectl` commands.

### Deploy job update (`.github/workflows/ci.yml`)

Replace the Docker Compose deploy script with kubectl-based deployment:

```yaml
script: |
  cd ${{ secrets.DEPLOY_PATH }}
  git pull origin main

  # Deploy Python AI services to K8s
  kubectl apply -f k8s/ai-services/ --recursive
  kubectl rollout restart deployment -n ai-services

  # Deploy Java services to K8s
  kubectl apply -f java/k8s/ --recursive
  kubectl rollout restart deployment -n java-tasks

  # Deploy monitoring
  kubectl apply -f k8s/monitoring/ --recursive

  # Wait for rollouts
  kubectl rollout status deployment -n ai-services --timeout=180s
  kubectl rollout status deployment -n java-tasks --timeout=180s
```

### Java CI (`java-ci.yml`) docker-build job

No change needed — it already builds and pushes Docker images to GHCR. K8s deployments pull from GHCR, same as before.

### Python CI docker-build

No change needed — same pattern. Images pushed to GHCR, K8s pulls them.

### Hadolint

No change — still lints all Dockerfiles (Python + Java). Docker images are still built, just deployed to K8s instead of Compose.

### What changes in the deploy job's `needs`

The deploy job currently depends on all Python CI jobs. It should also wait for Java CI jobs if they're in the same workflow. Since `java-ci.yml` is a separate workflow with path filtering, the deploy in `ci.yml` only handles Python services. Java images are built and pushed by `java-ci.yml`'s docker-build job independently.

However, the deploy step now applies ALL K8s manifests (including Java). This is fine — `kubectl apply` is idempotent, and `kubectl rollout restart` will pull the latest image regardless of which CI workflow built it.

### Smoke tests

The `smoke-production` job in `ci.yml` should add Java endpoint checks:

```yaml
# Add to smoke test environment
SMOKE_GRAPHQL_URL: https://api.kylebradshaw.dev/graphql
```

And add a smoke test that hits the GraphQL endpoint to verify Java services are running after deploy.

## Changes to Existing Files

| File | Change |
|------|--------|
| `.github/workflows/ci.yml` | Replace docker compose deploy with kubectl apply/rollout; add Java smoke test |
| `java/k8s/configmaps/gateway-service-config.yml` | Add ALLOWED_ORIGINS with kylebradshaw.dev, add GRAPHIQL_ENABLED=true |
| `java/k8s/configmaps/task-service-config.yml` | Add ALLOWED_ORIGINS with kylebradshaw.dev |
| `frontend/.env.example` | Add NEXT_PUBLIC_GATEWAY_URL and NEXT_PUBLIC_GOOGLE_CLIENT_ID |
| `CLAUDE.md` | Document K8s architecture, minikube tunnel, Cloudflare Tunnel port change |

## What NOT to Change

- **Don't remove Docker Compose files** — they're for local development
- **Don't put Ollama in K8s** — GPU passthrough is impractical; ExternalName service is the right pattern
- **Don't add CORS in Ingress annotations** — each service manages its own CORS
- **Don't use Helm** — plain manifests are clearer for a portfolio project
- **Don't create persistent volumes for demo databases** — ephemeral storage is fine for portfolio demo (data loss on restart is acceptable)

## Deployment

### Prerequisites
- Minikube installed and running (`minikube start`)
- `minikube addons enable ingress` for NGINX Ingress Controller
- `minikube tunnel` running as background process
- Cloudflare Tunnel config updated: `localhost:80` instead of `localhost:8000`

### Deploy Script
```bash
#!/usr/bin/env bash
set -euo pipefail

# Enable ingress addon
minikube addons enable ingress

# Deploy all namespaces
kubectl apply -f k8s/ai-services/namespace.yml
kubectl apply -f java/k8s/namespace.yml
kubectl apply -f k8s/monitoring/namespace.yml

# Deploy secrets, configmaps, then services in dependency order
# ... (full deploy script in implementation plan)
```

## Verification

1. **Ingress is working:**
   - `kubectl get ingress --all-namespaces` — all ingresses have an ADDRESS
   - `curl http://localhost/chat/health` — Python service reachable

2. **Python services:**
   - `curl https://api.kylebradshaw.dev/ingestion/health`
   - `curl https://api.kylebradshaw.dev/chat/health`

3. **Java GraphQL:**
   - `curl https://api.kylebradshaw.dev/graphql -X POST -H "Content-Type: application/json" -d '{"query":"{ __typename }"}'`

4. **GraphiQL:** Visit `https://api.kylebradshaw.dev/graphiql`

5. **Auth endpoint:**
   - `curl -X POST https://api.kylebradshaw.dev/api/auth/google -H "Content-Type: application/json" -d '{"code":"test","redirectUri":"test"}' -w "%{http_code}"` (expect 400/401, not 502)

6. **RabbitMQ UI:** Visit `https://api.kylebradshaw.dev/rabbitmq/`

7. **Grafana:** Visit `https://api.kylebradshaw.dev/grafana/`

8. **Frontend:** Visit `https://kylebradshaw.dev/java/tasks` — reaches GraphQL backend

9. **Ollama connectivity:**
   - `kubectl exec -n ai-services deploy/chat -- curl http://ollama:11434/api/tags` — can reach host Ollama
