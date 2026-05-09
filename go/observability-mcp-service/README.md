# Observability MCP Service

Local, read-only MCP service for collecting bounded evidence from Prometheus,
Loki, Jaeger, and embedded runbooks.

## Run Locally

```bash
cd go/observability-mcp-service
go run ./cmd/observability-mcp
```

The service uses stdio transport for local MCP clients. It does not expose an
HTTP listener in v1.

## Codex Registration

```bash
codex mcp add observability -- go run /Users/kylebradshaw/repos/gen_ai_engineer/go/observability-mcp-service/cmd/observability-mcp
```

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `OBS_PROMETHEUS_URL` | `http://localhost:9090` | Prometheus HTTP API endpoint. |
| `OBS_LOKI_URL` | `http://localhost:3100` | Loki HTTP API endpoint. |
| `OBS_JAEGER_URL` | `http://localhost:16686` | Jaeger query API endpoint. |
| `OBS_QUERY_TIMEOUT` | `5s` | Shared HTTP client timeout. |
| `OBS_DEFAULT_WINDOW` | `15m` | Default investigation window. |
| `OBS_MAX_WINDOW` | `1h` | Maximum accepted investigation window. |
| `OBS_MAX_LOG_LINES` | `100` | Maximum log lines per Loki query. |
| `OBS_MAX_TRACE_SPANS` | `100` | Maximum spans returned per trace. |

## Tools

- `get_system_health`
- `investigate_checkout`
- `investigate_ai_pipeline`
- `investigate_streaming_analytics`
- `get_service_evidence`
- `search_logs`
- `get_trace`

Runbook resources are exposed under:

- `observability://runbooks/system-health`
- `observability://runbooks/checkout`
- `observability://runbooks/ai-pipeline`
- `observability://runbooks/streaming-analytics`

## Safety

V1 is read-only. It queries metrics, logs, traces, and embedded runbook text. It
does not call Kubernetes write APIs, roll out or restart workloads, scale
deployments, purge queues, silence alerts, mutate databases, or read secrets.

Example prompts:

- `Use the observability MCP to check system health.`
- `Investigate checkout over the last 15m.`
- `Search go-payment-service error logs over the last 10m.`
