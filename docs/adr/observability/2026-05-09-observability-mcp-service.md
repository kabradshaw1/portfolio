# Local Read-Only Observability MCP Service

- **Date:** 2026-05-09
- **Status:** Accepted

## Context

The portfolio has Prometheus, Loki, Jaeger, Grafana dashboards, Go service
metrics, Kafka/RabbitMQ telemetry, and AI/RAG metrics. Debugging still requires
manual navigation across tools. A local MCP service can expose bounded,
read-only evidence bundles to Codex without adding a public operational
surface.

## Decision

Create `go/observability-mcp-service` as a local stdio MCP service. It queries
Prometheus, Loki, and Jaeger through configurable endpoints and returns
structured evidence bundles for system health, checkout, AI pipeline,
streaming analytics, single-service evidence, logs, and traces.

The v1 service is read-only. It does not deploy into Kubernetes, expose remote
transport, mutate infrastructure, read secrets, or run an LLM internally.

## Consequences

This gives local operators and portfolio reviewers a concrete debugging
interface over the existing observability stack. Keeping transport and
workflow logic separate makes future HTTP/Kubernetes deployment possible.

The trade-off is that v1 requires reachable telemetry endpoints, usually by
local services or port-forwarding. Remote access, auth, dashboard UX, and
management actions remain separate follow-up work.
