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
codex mcp add observability -- zsh -lc 'source ~/.codex/env/observability-mcp.env && cd /Users/kylebradshaw/repos/gen_ai_engineer/go/observability-mcp-service && exec go run ./cmd/observability-mcp'
```

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `OBS_PROMETHEUS_URL` | `http://localhost:9090` | Prometheus HTTP API endpoint. |
| `OBS_LOKI_URL` | `http://localhost:3100` | Loki HTTP API endpoint. |
| `OBS_JAEGER_URL` | `http://localhost:16686` | Jaeger query API endpoint. |
| `OBS_GRAFANA_URL` | unset | Grafana gateway endpoint for Prometheus and Loki datasource proxy mode. |
| `OBS_GRAFANA_TOKEN` | unset | Optional Grafana service account token. When alert discovery is enabled, this token only needs read access to alerting metadata. |
| `OBS_GRAFANA_ACCESS_CLIENT_ID` | unset | Cloudflare Access service-token client ID for Grafana gateway mode. |
| `OBS_GRAFANA_ACCESS_CLIENT_SECRET` | unset | Cloudflare Access service-token client secret for Grafana gateway mode. |
| `OBS_GRAFANA_PROMETHEUS_DS_UID` | `PBFA97CFB590B2093` | Grafana Prometheus datasource UID for gateway mode. |
| `OBS_GRAFANA_LOKI_DS_UID` | `loki` | Grafana Loki datasource UID for gateway mode. |
| `OBS_QUERY_TIMEOUT` | `5s` | Shared HTTP client timeout. |
| `OBS_DEFAULT_WINDOW` | `15m` | Default investigation window. |
| `OBS_MAX_WINDOW` | `1h` | Maximum accepted investigation window. |
| `OBS_MAX_LOG_LINES` | `100` | Maximum log lines per Loki query. |
| `OBS_MAX_TRACE_SPANS` | `100` | Maximum spans returned per trace. |
| `OBS_HISTORY_ENABLED` | `true` | Enables local SQLite incident history. |
| `OBS_HISTORY_DB_PATH` | `~/.codex/data/observability-mcp/history.db` | Local SQLite history database path. |
| `OBS_HISTORY_AUTO_CAPTURE` | `false` | Persist investigation evidence even without `incident_key`. |
| `OBS_MANAGEMENT_ACTIONS_ENABLED` | `false` | Enables cataloged management action execution. Read-only evidence tools work without this. |
| `OBS_MANAGEMENT_ALLOW_HIGH_RISK` | `false` | Allows high-risk cataloged actions to execute instead of preview-only. Keep false for normal use. |
| `OBS_MANAGEMENT_ACTION_TIMEOUT` | `45m` | Default maximum action execution timeout unless a catalog entry is lower. |
| `OBS_MANAGEMENT_MAX_OUTPUT_BYTES` | `32768` | Maximum stdout/stderr bytes returned and stored for each action. |

## Grafana Gateway Mode

For normal development-machine usage, prefer Grafana gateway mode for
Prometheus and Loki instead of direct local ports. Jaeger trace lookup still
uses `OBS_JAEGER_URL` until Grafana trace proxy support is validated.

When `OBS_GRAFANA_URL` is configured, `get_system_health` also performs
read-only Grafana alert discovery. It queries active alert instances and
provisioned alert rule metadata, then returns compact metadata in the evidence
bundle. The MCP does not silence alerts, edit rules, restart workloads, or run
ops commands.

Create an uncommitted local env file:

```bash
mkdir -p ~/.codex/env
$EDITOR ~/.codex/env/observability-mcp.env
```

Expected shape:

```bash
export OBS_GRAFANA_URL="https://observability-api.kylebradshaw.dev"
export OBS_GRAFANA_ACCESS_CLIENT_ID="<cloudflare-access-client-id>"
export OBS_GRAFANA_ACCESS_CLIENT_SECRET="<cloudflare-access-client-secret>"
```

Optional, if Grafana itself requires a service account token in addition to
Cloudflare Access:

```bash
export OBS_GRAFANA_TOKEN="<grafana-service-account-token>"
```

The MCP remains read-only. SSH-based diagnostics remain available through the
`debug-observability` skill for break-glass cases and when the observability
gateway is unavailable.

## Eval Evidence Workflow

For RAG eval investigations, start with the eval MCP `get_eval_run_evidence`
tool to read durable run state and captured config from the eval API. Then use
`investigate_eval_run` here with the same `eval_id` to collect bounded runtime
signals: eval run counters, item-stage latency, upstream chat/search failures,
stale-running gauges, and eval service logs filtered by that run ID.

## Tools

- `get_system_health`
- `investigate_checkout`
- `investigate_ai_pipeline`
- `investigate_eval_run`
- `investigate_streaming_analytics`
- `get_service_evidence`
- `search_logs`
- `get_trace`
- `list_incidents`
- `get_incident_history`
- `add_incident_note`
- `compare_evidence_windows`
- `list_management_actions`
- `preview_management_action`
- `execute_management_action`
- `get_management_action_history`

Runbook resources are exposed under:

- `observability://runbooks/system-health`
- `observability://runbooks/checkout`
- `observability://runbooks/ai-pipeline`
- `observability://runbooks/streaming-analytics`

## Safety

By default, external observability and runtime systems remain read-only. When
`OBS_MANAGEMENT_ACTIONS_ENABLED=true`, the MCP can execute cataloged management
actions only. Those actions must map to committed repo scripts, use fixed
command shapes, bounded inputs, timeouts, output redaction, and incident-history
logging. The MCP still does not accept free-form shell, kubectl, SSH commands,
Grafana mutation payloads, arbitrary URLs, or arbitrary script paths.

When incident history is enabled, the service may write investigation snapshots
and `add_incident_note` entries to the configured local SQLite history database.

Example prompts:

- `Use the observability MCP to check system health.`
- `Investigate checkout over the last 15m.`
- `Search go-payment-service error logs over the last 10m.`
