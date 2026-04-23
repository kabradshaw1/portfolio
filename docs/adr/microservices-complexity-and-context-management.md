# Microservices Complexity and AI-Assisted Context Management

- **Date:** 2026-04-23
- **Status:** Accepted

## Context

This portfolio project demonstrates a production-grade microservices architecture: 7 Go services, 4 Java services, 3 Python services, a Next.js frontend, and a full observability stack (Prometheus, Loki, Jaeger, Grafana). The system runs on Minikube with 14+ namespaces, mTLS between services, a RabbitMQ-based saga orchestrator, Kafka streaming analytics, and CI/CD deploying to both QA and production environments.

The architecture achieves its goal — it demonstrates the ability to build and operate a highly scalable system. But the complexity cost has been significant, and the experience has surfaced hard-won lessons about what happens when a system outgrows the ability to hold it in one person's head.

### The complexity tax

**Debugging is the dominant time cost.** Over the past week, two checkout bugs required a combined ~6 hours of debugging across 4 services, 3 databases, 2 message brokers, and the observability stack. The root causes were simple (an empty field, a shared queue, a stale FK constraint), but finding them required correlating logs across namespaces, tracing saga state through RabbitMQ, querying databases via kubectl exec, and reasoning about which environment a message was consumed in.

**Observability became a first-class workstream.** What started as "add Prometheus and Grafana" grew into 5 Grafana dashboards, 4 alert rule groups, a structured logging standard, distributed tracing with W3C context propagation through HTTP/gRPC/RabbitMQ/Kafka, a custom gRPC client interceptor, build version logging, and two debugging-specific Claude Code skills. The observability stack now represents a substantial fraction of the project's total effort — but without it, the system would be effectively undebuggable.

**Small changes have large blast radii.** Adding the payment-service required: a new database, migration job, K8s deployment, service + PDB, ConfigMap, cert-manager certificate (prod + QA), CI matrix entries, Kustomize QA overlay patches, Grafana alert rules, dashboard panels, and CLAUDE.md documentation. The `/scaffold-go-service` skill codifies this 15-item checklist, but each item is a potential failure point.

**AI-assisted development burns context rapidly.** With a 446-line CLAUDE.md, 2 project skills, 8 observability ADRs, and a growing spec/plan archive, Claude Code needs substantial context to operate effectively. Debugging sessions that span multiple services consume tokens rapidly as the agent reads source files, K8s manifests, log output, and database state. A single debugging session can easily consume more tokens than the entire implementation it's investigating.

### What worked

**The observability investment paid off.** Despite the debugging friction, the system IS debuggable. Loki queries surface errors in seconds once you know where to look. Jaeger traces show the full request path. Grafana dashboards catch regressions. The problem was never "we can't find the data" — it was "we didn't know we needed to look, because errors were silent."

**Skills encapsulate operational knowledge.** The `/debug-observability` skill packages Loki query recipes, Jaeger trace lookup, saga debugging flows, and circuit breaker diagnosis into an invocable package. The `/scaffold-go-service` skill ensures new services ship with observability boilerplate from day one. These skills reduce the context Claude Code needs to load for common operations.

**ADRs capture decision context.** The observability ADR series (01-08) documents not just what was built but why — the debugging incidents that motivated each improvement, the alternatives considered, and the trade-offs accepted. This is the context that makes future decisions faster.

## Decision

### 1. Observability is a prerequisite, not an afterthought

Every new service, endpoint, or integration must ship with:
- Structured JSON logging at decision points (writes, errors, state transitions)
- Prometheus metrics for request rate, error rate, and latency
- OpenTelemetry trace propagation through all transport layers
- Grafana alert rules for error rate and latency SLOs
- Dashboard panels in the appropriate dashboard

The `/scaffold-go-service` skill enforces this. No service ships without passing the observability checklist.

### 2. Skill maintenance is an ongoing responsibility

Skills are not write-once artifacts. As the system evolves, skills must be updated:

- **`/debug-observability`**: Update when new services are added, new log patterns emerge, or debugging recipes change. The Loki URL-encoding reference and CRI JSON parser must stay current.
- **`/scaffold-go-service`**: Update when the deployment checklist changes (new cert-manager patterns, new CI matrix entries, new shared packages).
- **New skills to consider**: A `/debug-saga` skill focused specifically on saga flow debugging, and a `/verify-deployment` skill for confirming a fix is live (build version check, pod restart time, expected log lines).

### 3. Context injection needs consolidation

Context is currently spread across multiple injection points:

| Source | Purpose | Size |
|--------|---------|------|
| `CLAUDE.md` | Project-wide context, infrastructure, conventions | 446 lines |
| `go/CLAUDE.md` | Go-specific patterns, shared packages, K8s requirements | ~150 lines |
| `frontend/AGENTS.md` | Frontend conventions | 5 lines |
| `.claude/skills/debug-observability/` | Debugging recipes | ~120 lines |
| `.claude/skills/scaffold-go-service/` | Service creation checklist | ~200 lines |
| `docs/adr/observability/01-08` | Observability decision history | ~800 lines |
| Memory files | User preferences, project state | ~15 entries |

**The CLAUDE.md is approaching its practical limit.** At 446 lines, it's the single largest context document and is loaded into every conversation. Sections like the Loki query recipes and saga debugging flows have already been extracted to skills. Further extraction opportunities:

- **Migration patterns** (~30 lines) could move to a `/run-migration` skill
- **Vercel CLI recipes** (~20 lines) could move to a `/deploy-frontend` skill
- **Monitoring section** (~40 lines) could be trimmed now that dashboard/alert details live in the observability ADRs

**Principle:** CLAUDE.md should contain rules and conventions (how to behave), not recipes (how to do specific tasks). Recipes belong in skills that are loaded on demand.

### 4. Token efficiency through targeted context loading

The current approach loads CLAUDE.md into every conversation regardless of task scope. For a frontend-only change, the Go infrastructure details are wasted context. Strategies to reduce token burn:

- **Keep extracting to skills.** Skills are loaded on demand — a debugging session loads `/debug-observability` but doesn't pay for `/scaffold-go-service`.
- **Trim CLAUDE.md aggressively.** If information can be derived from code or git history, it doesn't belong in CLAUDE.md. If it's only needed for one type of task, it belongs in a skill.
- **Use ADRs for historical context, not operational context.** ADRs document why decisions were made. Skills document how to do things now. CLAUDE.md documents rules to follow always. Keeping these distinct prevents context bloat.

### 5. Future observability roadmap

Remaining gaps to address incrementally:

- **PostgreSQL slow query logging**: Enable `log_min_duration_statement` on the shared Postgres. Surface in Loki.
- **RabbitMQ queue depth metrics**: Scrape the management API for queue depth, consumer count, and message rates. Alert on unbounded growth.
- **End-to-end saga tracing**: A single Jaeger trace should span the entire checkout flow from order creation through cart clearing. Currently, trace context breaks at some RabbitMQ hops.
- **Synthetic monitoring**: Periodic health checks that exercise the full checkout flow (without Stripe) to catch regressions before users do.
- **Log-based alerts for error patterns**: Loki ruler alerts for specific error messages that indicate systemic failures (e.g., "foreign key constraint" errors that indicate stale schema references).

## Consequences

**Positive:**
- Observability is treated as infrastructure, not as a feature — it ships with every service by default
- Skills package operational knowledge into reusable, maintainable units
- Context management strategy prevents CLAUDE.md from growing unbounded
- ADR series provides a learning path for anyone reviewing the project

**Trade-offs:**
- Skills require maintenance effort — stale skills are worse than no skills because they provide false confidence
- Extracting from CLAUDE.md to skills adds indirection — a developer must know which skill to invoke
- The observability stack itself adds deployment complexity (Prometheus, Loki, Jaeger, Grafana, Promtail, cert-manager, kube-state-metrics, node-exporter — 8 additional components)

**Lessons learned:**
- Microservices complexity is not linear. Going from 3 services to 7 didn't double the complexity — it roughly quadrupled it, because the interaction surface grows combinatorially.
- The best time to add observability is before the first bug. The second best time is immediately after the first bug. Every debugging session that doesn't result in better instrumentation is a missed opportunity.
- Context management for AI-assisted development is itself an engineering problem. The context window is a resource with a cost, and managing it well is as important as managing memory or disk.
- Documentation that captures "why" (ADRs) ages better than documentation that captures "how" (runbooks). The "how" changes with every deployment; the "why" remains relevant for months or years.
