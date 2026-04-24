# Deployment Architecture

How the portfolio site at kylebradshaw.dev is deployed, networked, and served.

## High-Level Overview

```
                         INTERNET
                            |
              +-------------+-------------+
              |                           |
        Vercel (CDN)             Cloudflare Edge
   kylebradshaw.dev           api.kylebradshaw.dev
              |                           |
              |    HTTPS API calls        | Cloudflare Tunnel
              +-------------------------->|
                                          |
                                   cloudflared service
                                   (Windows PC)
                                          |
                                    localhost:80
                                          |
                                   minikube tunnel
                                          |
                                 NGINX Ingress Controller
                                          |
                   +----------------------+----------------------+
                   |                      |                      |
             ai-services            java-tasks              monitoring
           (Python + Qdrant)    (Java + databases)    (Prometheus + Grafana)
```

The frontend is a Next.js app deployed on **Vercel**. It serves static assets and makes API calls to `api.kylebradshaw.dev`. Those API calls hit **Cloudflare's edge network**, which routes them through a **Cloudflare Tunnel** to a Windows PC running a Minikube Kubernetes cluster. The **NGINX Ingress Controller** inside Minikube routes each request by URL path to the correct service across three namespaces.

The Windows PC has an NVIDIA RTX 3090 GPU. **Ollama** runs natively on the host (not inside Kubernetes) to access the GPU directly. Kubernetes services reach Ollama through an ExternalName service.

## Networking Layer

### Cloudflare Tunnel

Cloudflare Tunnel creates an **outbound-only** encrypted connection from the Windows PC to Cloudflare's edge. This means:

- No ports are opened on the PC's firewall
- No public IP address is exposed
- No port forwarding on the router is needed
- All traffic is encrypted and authenticated by Cloudflare

The `cloudflared` daemon runs as a **Windows service** (auto-starts on boot). It maintains a persistent connection to Cloudflare's edge network. When a request arrives at `api.kylebradshaw.dev`, Cloudflare routes it through this tunnel to the PC.

**Configuration** (`~/.cloudflared/config.yml`):

```yaml
tunnel: 85f54326-...
ingress:
  - hostname: api.kylebradshaw.dev
    service: http://localhost:80       # → Minikube Ingress
  - hostname: grafana.kylebradshaw.dev
    service: http://localhost:80       # → Minikube Ingress
  - service: http_status:404           # catch-all
```

### Tailscale

Tailscale is a **WireGuard-based mesh VPN** that gives the Windows PC a stable private IP (`100.79.113.84`) accessible from other devices on the same Tailscale network. It serves two purposes:

1. **CI/CD deployment** — GitHub Actions joins the Tailscale network during deploys and SSHes to the PC to run `kubectl apply`
2. **Local development** — SSH tunnels from the Mac dev machine forward ports through Tailscale

Cloudflare Tunnel and Tailscale handle different traffic:

```
Public traffic (users)                  Private traffic (operators)
======================                  ===========================

Browser                                 GitHub Actions runner
   |                                       |
   | HTTPS                                 | Tailscale VPN
   v                                       v
Cloudflare Edge                         100.79.113.84 (Windows PC)
   |                                       |
   | Cloudflare Tunnel                     | SSH → kubectl apply
   | (outbound from PC)                    |     → kubectl rollout restart
   v                                       v
localhost:80 → Ingress                  Minikube cluster updated

Mac dev machine
   |
   | SSH tunnel via Tailscale
   | ssh -L 8000:localhost:8000
   v
localhost:8000 → Docker Compose
```

## Kubernetes Cluster Layout

The Minikube cluster runs on the Windows PC using the Docker driver. Three namespaces isolate services by concern:

```
Minikube Cluster (Docker driver, 4 CPU, 8 GB RAM)
+------------------------------------------------------------------+
|                                                                  |
|  ai-services namespace                                           |
|  +-------------------------------------------------------------+ |
|  |  ingestion    chat    debug       (FastAPI, port 8000 each) | |
|  |  qdrant (vector DB, ports 6333/6334)                        | |
|  |  ollama (ExternalName → host.minikube.internal:11434)       | |
|  +-------------------------------------------------------------+ |
|                                                                  |
|  java-tasks namespace                                            |
|  +-------------------------------------------------------------+ |
|  |  gateway-service (8080)    task-service (8081)              | |
|  |  activity-service (8082)   notification-service (8083)      | |
|  |  PostgreSQL (5432)  MongoDB (27017)                         | |
|  |  Redis (6379)       RabbitMQ (5672/15672)                   | |
|  +-------------------------------------------------------------+ |
|                                                                  |
|  monitoring namespace                                            |
|  +-------------------------------------------------------------+ |
|  |  Prometheus (9090)    Grafana (3000)                        | |
|  +-------------------------------------------------------------+ |
|                                                                  |
+------------------------------------------------------------------+
     Host: Ollama (RTX 3090, :11434)  |  nvidia-gpu-exporter (:9835)
```

### Ollama and GPU Access

Ollama runs **natively on the Windows host** because Minikube's Docker driver on Windows cannot pass through the GPU. A Kubernetes `ExternalName` service bridges the gap:

```yaml
# k8s/ai-services/services/ollama.yml
apiVersion: v1
kind: Service
metadata:
  name: ollama
  namespace: ai-services
spec:
  type: ExternalName
  externalName: host.minikube.internal
```

Inside any pod in the `ai-services` namespace, `http://ollama:11434` resolves to `host.minikube.internal:11434`, which reaches the Ollama process on the host. This requires Ollama to bind to `0.0.0.0` (not the default `127.0.0.1`).

Prometheus scrapes GPU metrics from `nvidia-gpu-exporter`, which runs as a standalone Docker container on the host with `--gpus all`.

## Ingress Routing

The NGINX Ingress Controller is the single entry point for all HTTP traffic. It routes requests by URL path to the correct namespace and service:

| Path | Namespace | Service | Port | Match Type | Notes |
|------|-----------|---------|------|------------|-------|
| `/ingestion(/\|$)(.*)` | ai-services | ingestion | 8000 | Regex | Strips prefix via rewrite-target |
| `/chat(/\|$)(.*)` | ai-services | chat | 8000 | Regex | Strips prefix, SSE buffering disabled |
| `/debug(/\|$)(.*)` | ai-services | debug | 8000 | Regex | Strips prefix |
| `/graphql` | java-tasks | gateway-service | 8080 | Exact | GraphQL API endpoint |
| `/graphiql` | java-tasks | gateway-service | 8080 | Prefix | GraphQL IDE |
| `/api/auth/(.*)` | java-tasks | task-service | 8081 | Regex | OAuth authentication flow |
| `/actuator/health` | java-tasks | gateway-service | 8080 | Exact | Java health check |
| `/rabbitmq(/\|$)(.*)` | java-tasks | rabbitmq | 15672 | Regex | Management UI, strips prefix |
| `/grafana` | monitoring | grafana | 3000 | Prefix | Monitoring dashboards |

**Path rewriting:** For regex paths with `rewrite-target: /$2`, the prefix is stripped before forwarding. For example, a request to `/chat/query` is forwarded to the chat service as `/query`.

**AI services annotations:** Proxy buffering is disabled (required for Server-Sent Events streaming), read timeout is 300 seconds (LLM inference can be slow), and max body size is 55 MB (PDF uploads).

## Request Flow Examples

### Example 1: User Asks a Question (AI Service Path)

```
1. User types a question in the chat UI on kylebradshaw.dev
2. Next.js client sends POST to https://api.kylebradshaw.dev/chat/query
3. Cloudflare edge receives the request
4. Cloudflare routes it through the tunnel to cloudflared on the Windows PC
5. cloudflared forwards to localhost:80
6. minikube tunnel routes to the NGINX Ingress Controller
7. Ingress matches /chat(/|$)(.*), rewrites to /query, forwards to chat:8000
8. Chat service embeds the question via Ollama (http://ollama:11434)
9. Chat service searches Qdrant (http://qdrant:6333) for similar document chunks
10. Chat service sends the question + context to Ollama for LLM inference
11. Response streams back as Server-Sent Events through the entire chain
```

### Example 2: Frontend Loads Task List (Java Service Path)

```
1. User navigates to the tasks page on kylebradshaw.dev
2. Apollo Client sends POST to https://api.kylebradshaw.dev/graphql
   with Authorization: Bearer <jwt-token>
3. Cloudflare → tunnel → cloudflared → localhost:80 → Ingress
4. Ingress matches /graphql (exact) → gateway-service:8080
5. Gateway service validates the JWT token
6. Gateway resolves the GraphQL query by calling task-service:8081
7. Task service queries PostgreSQL, returns task data
8. Gateway assembles the GraphQL response and returns it to the frontend
```

## Local Development Setup

For day-to-day development, **Docker Compose** runs the backend on the Windows PC instead of Kubernetes. The Mac dev machine connects via SSH tunnel:

```
Mac (dev machine)                        Windows PC (100.79.113.84)
+---------------------+                  +------------------------------+
|                     |                  |  Docker Compose              |
|  Next.js dev server |   SSH tunnel     |  +------------------------+  |
|  localhost:3000     | ===============> |  | nginx gateway :8000    |  |
|                     | -L 8000:...      |  |   /ingestion → :8000  |  |
|  API calls go to    |                  |  |   /chat → :8000       |  |
|  localhost:8000     |                  |  |   /debug → :8000      |  |
|                     |                  |  +------------------------+  |
+---------------------+                  |  Qdrant, Prometheus, Grafana |
                                         +------------------------------+
                                         |  Ollama (native, RTX 3090)   |
                                         +------------------------------+
```

Docker Compose is simpler for the "edit code, restart, test" loop. Kubernetes is the production runtime. See the [migration ADR](adr/docker-compose-to-kubernetes.md) for the reasoning behind this split.

## Host Services

Two Windows scheduled tasks keep the production stack running across reboots:

**Ollama Serve** (starts 10s after boot):
- Runs `ollama serve` with `OLLAMA_HOST=0.0.0.0` and `OLLAMA_ORIGINS=*`
- `0.0.0.0` is required because Minikube's Docker network reaches the host via `host.minikube.internal`, which is not `127.0.0.1`. Without this, Ollama rejects connections from pods.
- Runs as user `PC` (needs access to the GPU)

**Minikube Tunnel** (starts 30s after boot):
- Runs `minikube tunnel` as SYSTEM with highest privileges
- Creates a network route so the Ingress LoadBalancer is accessible on `localhost:80`
- The 30-second delay gives Docker Desktop time to start (Minikube depends on it)

Both tasks auto-restart up to 3 times on failure. See `k8s/create-scheduled-tasks.ps1` for the task definitions.

## See Also

- [CI/CD Pipeline](cicd-pipeline.md) — how code gets from a git push to production
- [Docker Compose to Kubernetes ADR](adr/docker-compose-to-kubernetes.md) — why we migrated
- [Windows Setup Guide](../k8s/WINDOWS-SETUP.md) — step-by-step setup instructions for the Windows PC
