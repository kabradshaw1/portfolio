# Observability MCP Grafana Alert Discovery Design

## Summary

Issue 250 adds read-only Grafana alert and rule metadata to the Observability
MCP evidence bundles. The change keeps the MCP diagnostic-only: it discovers
active alerts and provisioned rule metadata, reports bounded evidence, and never
silences alerts, restarts workloads, edits rules, or performs any other
management action.

Issue 251 is intentionally out of scope. Gated management actions need a
separate ops-as-code design before any implementation work.

## Goals

- Surface currently active Grafana alerts in `get_system_health` evidence.
- Include enough rule metadata to identify the owning rule without requiring a
  separate Grafana lookup: rule UID, title, folder or namespace, labels, state,
  and relevant timestamps when Grafana provides them.
- Reuse the existing Grafana gateway configuration and authentication headers.
- Preserve partial evidence behavior when Grafana alerting APIs are unavailable.
- Keep the service read-only and maintain the README safety contract.

## Non-Goals

- No alert silencing, acknowledgement, pausing, deletion, rule updates, rollout
  requests, queue purges, database writes, or Kubernetes mutations.
- No new public HTTP listener for the MCP service.
- No broad alert correlation engine. This first pass exposes compact alert and
  rule evidence; deeper correlation can be added after the metadata shape is
  stable.
- No change to Grafana provisioning manifests or alert rule definitions.

## Current Context

`go/observability-mcp-service` already supports Prometheus, Loki, Jaeger, and
Grafana gateway mode. Grafana mode currently proxies Prometheus and Loki through
datasource routes while retaining direct Jaeger access. Evidence bundles contain
signals, logs, traces, findings, source statuses, and source errors.

The service README states that v1 is read-only. That remains true after this
work.

## Design

### Grafana Alerting Client

Add read-only alerting methods to the Grafana client layer. The client should
use the same base URL, bearer token, and Cloudflare Access service-token headers
as the existing Grafana datasource proxy client.

The implementation should call Grafana alerting APIs directly, not through the
Prometheus or Loki datasource proxy. The implementation plan should validate the
exact API paths against the live or locally stubbed Grafana version before
coding the final client methods. Expected capabilities are:

- List active alert instances from Grafana Alertmanager.
- List provisioned alert rules, or fetch rule metadata by UID when active alert
  payloads expose rule identifiers.

The client should normalize Grafana responses into small internal types instead
of leaking raw API payloads into workflow results.

### Evidence Model

Extend `workflows.EvidenceBundle` with a dedicated alerting section. A suggested
shape is:

```go
type AlertSummary struct {
    ActiveAlerts []observability.AlertInstance `json:"active_alerts,omitempty"`
    Rules        []observability.AlertRule     `json:"rules,omitempty"`
    Truncated    bool                          `json:"truncated,omitempty"`
}
```

The final names can change during implementation, but the bundle should keep
alert data distinct from Prometheus `Signal` values. Alerts are operational
state, not scalar query samples.

Alert instance records should include:

- alert name or title
- state or status
- labels and annotations, bounded to avoid unbounded output
- starts-at and ends-at timestamps when present
- generator URL or dashboard URL when present
- rule UID when Grafana provides it

Rule records should include:

- UID
- title
- folder or namespace
- condition or summary metadata when available in compact form
- labels
- provenance or source when Grafana provides it

### Workflow Behavior

`GetSystemHealth` should collect Grafana alert evidence after Prometheus
signals. This keeps the primary health entry point useful when an operator asks,
"what is firing right now?"

Other investigation workflows should not include alert discovery in this first
pass. That avoids surprising extra network calls and keeps issue 250 focused.
After system health has stable alert metadata, later work can decide whether
checkout, AI pipeline, eval run, or streaming analytics investigations should
include alert context by default.

If one or more active alerts are returned, `GetSystemHealth` should add a
warning or critical finding based on the alert state. Unknown or non-firing
states should be included as evidence without escalating bundle status.

### Source Status And Errors

Add a `grafana_alerting` source status when alert discovery is configured.

- If alert discovery succeeds, mark `grafana_alerting` as `ok`.
- If Grafana is not configured, mark `grafana_alerting` as `skipped`.
- If Grafana alerting calls fail, call `AddError("grafana_alerting", ...)`,
  mark the bundle partial, and keep Prometheus/Loki/Jaeger evidence.

Tool handlers should not return `IsError` for alert discovery failures inside a
workflow. The failure is source-level partial evidence.

### Configuration

Reuse these existing environment variables:

- `OBS_GRAFANA_URL`
- `OBS_GRAFANA_TOKEN`
- `OBS_GRAFANA_ACCESS_CLIENT_ID`
- `OBS_GRAFANA_ACCESS_CLIENT_SECRET`

No write-capable token requirement should be introduced. Documentation should
state that the token, when used, only needs read access to alerting metadata.

If implementation discovers that Grafana needs a separate alerting API feature
flag or path for the deployed version, add the smallest read-only configuration
needed and document the default.

### Safety

The MCP remains read-only. The design must not introduce methods whose names,
types, or tests imply future mutation such as `SilenceAlert`, `PauseRule`, or
`RestartDeployment`.

Issue 251 will handle any management-action design separately under ops-as-code.
That later design must require committed procedures before shared-environment
mutations run.

## Testing

Add unit tests covering:

- Grafana alerting client request paths and authentication headers.
- Parsing active-alert responses into bounded internal records.
- Parsing or fetching rule metadata into bounded internal records.
- `GetSystemHealth` includes alert evidence when Grafana alerting succeeds.
- Active firing alerts create an appropriate finding.
- Grafana alerting failure returns a partial bundle while preserving available
  Prometheus evidence.
- Missing Grafana configuration skips alert discovery cleanly.
- README/config documentation continues to describe the MCP as read-only.

Run the Go preflight before implementation is committed:

```bash
make preflight-go
```

## Acceptance Criteria

- `get_system_health` can return compact active alert and rule metadata.
- Alerting API failures degrade to partial evidence instead of failing the MCP
  tool call.
- The implementation uses existing Grafana auth and gateway settings.
- No management action, mutation API, Kubernetes write, Grafana write, or
  ops-command execution path is added.
- Tests cover the client parsing, workflow integration, and partial-failure
  behavior.
- Documentation remains explicit that the Observability MCP is read-only.
