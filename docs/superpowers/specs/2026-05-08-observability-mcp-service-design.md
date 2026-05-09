# Observability MCP Service Design

## Summary

Build a local, read-only Go MCP service for operator-style debugging of this
portfolio system. The service runs over stdio for Codex/local MCP clients and
returns structured evidence bundles from the existing observability stack:
Prometheus, Loki, Jaeger, and local runbook metadata.

The first version is not an LLM agent and does not mutate infrastructure. It
collects bounded operational evidence that Codex, a future local LLM workflow,
or a future dashboard can summarize.

## Goals

- Provide useful local operator workflows for debugging the portfolio system.
- Reuse existing observability investment: RED metrics, Go runtime metrics,
  RabbitMQ, Kafka, AI/RAG, Kubernetes, Loki logs, and Jaeger traces.
- Keep v1 lightweight enough to run on the developer Mac with low idle memory.
- Make later Kubernetes deployment possible without rewriting the core clients
  or workflow logic.
- Demonstrate production-grade operational thinking in a portfolio setting.

## Non-Goals

- No Kubernetes manifests, HTTP MCP transport, or remote deployment in v1.
- No authenticated remote access in v1.
- No rollouts, restarts, scale changes, queue purges, alert silences, secret
  reads, database writes, or other mutating operations.
- No LLM-powered diagnosis inside the MCP service in v1.
- No browser dashboard in v1.
- No persistent incident-history database in v1.

## Architecture

Create a new Go service at `go/observability-mcp-service`, modeled closer to
`go/qa-mcp-service` than `go/ai-service`. It starts as a local stdio MCP server
and exposes read-only tools and runbook resources.

Proposed package layout:

- `cmd/observability-mcp`: process entrypoint, config loading, stdio server
  startup.
- `internal/mcpserver`: MCP tool and resource registration.
- `internal/config`: environment defaults, duration parsing, validation.
- `internal/observability`: Prometheus, Loki, Jaeger, and static metadata
  clients.
- `internal/workflows`: higher-level evidence bundle builders.
- `resources/`: concise runbooks for interpreting each investigation workflow.

The transport boundary should stay thin. Core clients and workflows should not
know whether the caller is stdio MCP, future HTTP MCP, or a future dashboard API.

## Configuration

Use environment variables with local-friendly defaults:

- `OBS_PROMETHEUS_URL`: defaults to `http://localhost:9090`.
- `OBS_LOKI_URL`: defaults to `http://localhost:3100`.
- `OBS_JAEGER_URL`: defaults to `http://localhost:16686`.
- `OBS_QUERY_TIMEOUT`: defaults to `5s`.
- `OBS_DEFAULT_WINDOW`: defaults to `15m`.
- `OBS_MAX_WINDOW`: defaults to `1h`.
- `OBS_MAX_LOG_LINES`: defaults to `100`.
- `OBS_MAX_TRACE_SPANS`: defaults to `100`.

The service should work with port-forwarded Kubernetes services, local Docker
services, or any equivalent endpoint reachable from the Mac.

## Tool Set

### `get_system_health`

Returns a compact system-wide snapshot:

- Service request rate, error rate, and p95 latency for Go services.
- Pod readiness and restart signals when Kubernetes metrics are available.
- Kafka lag and consumer errors.
- RabbitMQ saga DLQ depth.
- Circuit breaker state.
- Certificate expiry when cert-manager metrics are available.
- Source-level errors for unavailable telemetry backends.

### `investigate_checkout`

Builds an evidence bundle for checkout and saga failures across:

- `go-order-service`
- `go-cart-service`
- `go-payment-service`
- `go-product-service`

Signals include RED metrics, saga step p95 latency, circuit breaker state,
RabbitMQ publish success/error rate, saga DLQ depth, payment webhook outcomes,
and recent error/warn logs.

### `investigate_ai_pipeline`

Builds an evidence bundle for AI/RAG issues across:

- `go-ai-service`
- Python chat, ingestion, and debug services when metrics are available.
- Ollama metrics.
- Embedding, Qdrant, and RAG pipeline metrics.

Signals include agent turns by outcome, agent duration, tool call rates, tool
latency, cache hit/miss, RAG stage latency, RAG errors, token throughput,
Ollama latency, and recent AI/RAG logs.

### `investigate_streaming_analytics`

Builds an evidence bundle for Kafka and analytics issues:

- Kafka consumer lag.
- Analytics events consumed by topic.
- Consumer error counts.
- Analytics-service RED metrics when available.
- Recent analytics-service error/warn logs.

### `get_service_evidence`

Returns bounded evidence for a single allowlisted service:

- RED metrics.
- Go runtime metrics when present.
- Kubernetes readiness and restarts when metrics are available.
- Recent logs.
- Optional trace summary when a trace ID is supplied.

### `search_logs`

Safe Loki wrapper:

- Requires an allowlisted service label.
- Defaults to error/warn/exception filtering.
- Enforces max time window and max line count.
- Returns truncation metadata.

### `get_trace`

Fetches a Jaeger trace by ID and returns:

- Trace ID.
- Span count.
- Longest spans.
- Operation names.
- Service names when present.
- Error tags when present.
- Truncation metadata if span count exceeds the configured cap.

### Runbook Resources

Expose small MCP resources or equivalent tools:

- `observability://runbooks/system-health`
- `observability://runbooks/checkout`
- `observability://runbooks/ai-pipeline`
- `observability://runbooks/streaming-analytics`

Runbooks should explain what signals are checked, common interpretations, and
which follow-up tool is useful next.

## Evidence Bundle Format

Workflow tools return JSON text content with a stable top-level shape:

```json
{
  "status": "ok|warning|critical|unknown",
  "window": {
    "from": "RFC3339 timestamp",
    "to": "RFC3339 timestamp",
    "duration": "15m"
  },
  "sources": [
    {
      "name": "prometheus",
      "status": "ok|error|skipped",
      "endpoint": "http://localhost:9090"
    }
  ],
  "signals": [],
  "logs": [],
  "traces": [],
  "findings": [],
  "partial": false,
  "errors": []
}
```

`signals`, `logs`, `traces`, and `findings` should use typed structs in Go.
The JSON should stay compact, deterministic, and easy for Codex or a dashboard
to consume.

## Data Flow

1. MCP client calls a workflow tool with input such as `window`, `service`, or
   `trace_id`.
2. Tool validates duration bounds, service allowlists, required fields, and
   endpoint configuration.
3. Workflow runs Prometheus, Loki, and Jaeger requests with a shared context
   timeout.
4. Each source returns data or a source-level error.
5. Partial source failures mark `partial: true` and populate structured errors.
6. Workflow applies deterministic rules to produce findings.
7. MCP returns the evidence bundle as JSON text content.

## Safety Boundaries

- V1 is read-only.
- Log search is restricted to allowlisted service labels.
- Query windows are capped.
- Log lines and trace spans are capped.
- No direct Kubernetes API calls are required in v1; Kubernetes state comes
  from Prometheus/kube-state-metrics.
- Missing metrics are treated as unknown data, not proof of health.
- Remote deployment is blocked until auth, network exposure, and access policy
  are designed.

## Error Handling

- Invalid input returns an MCP tool error.
- Downstream source errors are included in the evidence bundle unless every
  required source for a tool fails.
- Empty query results are not errors; they produce `unknown` or low-confidence
  findings.
- HTTP clients use configured timeouts and context cancellation.
- JSON decode errors identify the source and operation.

## Testing

Use focused Go tests:

- Config default and env override tests.
- Duration and max-window validation tests.
- Prometheus query construction and response parsing tests.
- Loki query construction, allowlist enforcement, and truncation tests.
- Jaeger trace parsing and longest-span extraction tests.
- Workflow tests with fake HTTP servers covering full success, partial failure,
  empty data, and bad responses.
- MCP server tests for tool/resource registration and representative calls.

Before commit of an implementation, run `make preflight-go`.

## Future Enhancement Issues

Create follow-up GitHub issues for:

- Deployable HTTP MCP transport with Dockerfile, Kubernetes manifests, Service,
  and local/cluster configuration.
- Authentication and authorization for remote MCP clients.
- Browser dashboard/demo UI backed by this evidence API and a locally hosted
  LLM.
- Local Ollama incident narrative generation on top of evidence bundles.
- Grafana alert and rule discovery for richer active-alert summaries.
- Optional gated management actions such as alert silencing or rollout
  requests, designed under `ops-as-code`.
- Persistent incident history and comparison across time windows.

## Acceptance Criteria

- A local MCP client can start the service over stdio.
- The service exposes the v1 tools and runbook resources.
- Tools return bounded structured evidence bundles.
- Source failures are visible without hiding successful evidence from other
  sources.
- No v1 tool mutates infrastructure or reads secrets.
- Tests cover config, clients, workflow aggregation, and MCP registration.
