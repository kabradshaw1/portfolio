# Docker Compose to Kubernetes Migration

- **Date:** 2026-04-04
- **Status:** Accepted

## Context

The project started with Docker Compose as the production runtime. All Python AI services (ingestion, chat, debug), Qdrant, nginx gateway, and monitoring (Prometheus, Grafana) ran via a single `docker-compose.yml`. When the Java microservices were added (task-service, activity-service, notification-service, gateway-service, plus PostgreSQL, MongoDB, Redis, and RabbitMQ), they joined with a second Docker Compose file.

This worked, but several limitations became apparent:

- **No namespace isolation.** AI services, Java services, and databases all ran in the same flat network. Any container could talk to any other container with no boundaries.
- **No health-based routing.** The nginx container required manual configuration for each new service. Adding a path meant editing `nginx.conf` and restarting.
- **No resource limits enforcement.** Docker Compose `deploy.resources` only works with Docker Swarm. Without it, a runaway service could starve the host.
- **Manual restart on failure.** `restart: unless-stopped` restarts crashed containers but doesn't do health-check-based restarts or rolling updates.
- **Port collision management.** Each service needed a unique host port or an nginx route. Adding services meant careful port planning.
- **No declarative rollout.** Deploying meant `docker compose down && docker compose up`, which caused downtime. There was no rolling update strategy.

Additionally, this is a portfolio project for a Gen AI Engineer role. The job description lists Kubernetes familiarity as a requirement. Demonstrating real K8s operational knowledge — manifests, namespaces, Ingress, resource limits, health probes — is a goal.

**Constraint:** Ollama must run natively on the Windows host for GPU access (RTX 3090). Minikube's Docker driver on Windows does not support GPU passthrough.

## Decision

Migrate the production runtime from Docker Compose to **Minikube** on the Windows PC:

- **Three namespaces** for isolation: `ai-services` (Python services + Qdrant), `java-tasks` (Java microservices + databases), `monitoring` (Prometheus + Grafana).
- **NGINX Ingress Controller** (Minikube addon) replaces the custom nginx container. Path-based routing is declared in Ingress resources, one per namespace.
- **ExternalName service** for Ollama bridges the host-native GPU to Kubernetes. Pods reach `http://ollama:11434`, which resolves to `host.minikube.internal`.
- **`minikube tunnel`** exposes the Ingress LoadBalancer on `localhost:80`, replacing the direct port mappings.
- **Resource requests and limits** are set per deployment (e.g., 128Mi-512Mi memory for Python services, 256Mi-512Mi for Java services).
- **Readiness probes** on every deployment ensure traffic only routes to healthy pods.
- **CI/CD switches** from `docker compose pull && up -d` to `kubectl apply --recursive && kubectl rollout restart`.
- **Docker Compose is kept for local development.** It's simpler for the "edit code, restart, test" workflow and doesn't require Minikube or tunnel setup.
- **`nvidia-gpu-exporter`** remains as a standalone Docker container outside Kubernetes because it needs direct GPU access via `--gpus all`.

## Consequences

**Positive:**

- Namespace isolation separates AI services, Java services, and monitoring. Network policies can be added later to restrict cross-namespace traffic.
- Declarative infrastructure: all manifests are in git, `kubectl apply` is idempotent, and the cluster state matches the repo.
- Built-in rolling updates: `kubectl rollout restart` replaces pods one at a time with zero downtime.
- Resource requests/limits prevent one service from starving others and give the scheduler information for placement decisions.
- Readiness probes stop traffic to unhealthy pods automatically.
- Ingress resources are declarative and namespace-scoped — adding a new service route is a YAML change, not an nginx.conf edit.
- The portfolio demonstrates real Kubernetes operational knowledge: manifests, namespaces, Ingress, ExternalName services, resource management.

**Trade-offs:**

- Higher complexity. The production stack now requires Minikube + Docker Desktop + `minikube tunnel` + scheduled tasks, compared to a single `docker-compose up`.
- `minikube tunnel` must run persistently with Administrator privileges. This is solved with a Windows scheduled task, but it's an additional moving part.
- The Ollama ExternalName service is a workaround. If Minikube supported GPU passthrough on Windows, Ollama could run as a pod with proper health checks and resource limits.
- Debugging requires `kubectl logs` and `kubectl exec` instead of `docker compose logs` — a steeper learning curve.
- Memory overhead: Minikube's control plane (etcd, API server, scheduler, controller manager, CoreDNS) uses ~2 GB before any workloads start.
- Docker Compose and Kubernetes manifests must be kept in sync when adding services. Changes to environment variables, ports, or images need to be reflected in both.
