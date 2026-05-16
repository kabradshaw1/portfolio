# Observability MCP Private API Design

## TL;DR

Keep `grafana.kylebradshaw.dev` public for portfolio dashboard viewing. Add a
separate authenticated host, `observability-api.kylebradshaw.dev`, for MCP and
other machine access. The Observability MCP should use Grafana as the API
gateway to internal Prometheus, Loki, and Jaeger datasources instead of relying
on local ports, SSH, or directly exposed observability backends.

## Goals

- Preserve unauthenticated public access to Grafana dashboards for portfolio
  visitors.
- Restrict programmatic observability API access to authenticated clients.
- Keep Prometheus and Loki cluster-internal.
- Avoid baking SSH behavior into the MCP service.
- Make local Codex usage work from the development machine without manual
  Kubernetes commands for normal diagnostics.

## Non-Goals

- Do not expose Prometheus or Loki directly to the public internet.
- Do not remove the existing `debug-observability` skill; it remains the
  runbook and break-glass fallback.
- Do not add mutating observability operations to the MCP service.
- Do not require a paid observability provider.

## Current State

The current MCP service runs locally over stdio and defaults to:

- `http://localhost:9090` for Prometheus
- `http://localhost:3100` for Loki
- `http://localhost:16686` for Jaeger

Those ports are not normally present on the development machine. The current
debugging skill reaches observability through `ssh debian` and Kubernetes-local
commands, such as `kubectl exec` into Loki or direct `curl` calls to cluster
service DNS/IPs.

Grafana and Jaeger already have public ingress hosts. Prometheus and Loki are
cluster-internal services. Grafana is configured with proxy datasources for
Prometheus, Loki, and Jaeger.

## Proposed Architecture

Use two externally visible Grafana entry points:

- `grafana.kylebradshaw.dev`: public dashboard UI for portfolio visitors.
- `observability-api.kylebradshaw.dev`: authenticated API endpoint for MCP.

Both hosts route to the Grafana service, but they have different access
policies at the edge. Public dashboard access stays available on the Grafana
host. Programmatic API access goes through the API host and requires
authentication before traffic reaches the cluster.

The Observability MCP adds a Grafana-backed mode:

```text
OBS_GRAFANA_URL=https://observability-api.kylebradshaw.dev
OBS_GRAFANA_TOKEN=...
```

In this mode, the MCP queries Grafana's datasource APIs for Prometheus and Loki
instead of connecting directly to the backend services. Jaeger trace lookup
should use Grafana datasource proxy if it supports the required trace query
cleanly; otherwise Jaeger can remain a follow-up item while metrics and logs
move first.

## Authentication Model

Use Cloudflare Access or an equivalent edge policy on
`observability-api.kylebradshaw.dev`.

For MCP automation, prefer a service token over browser login. Store token
material only in a local uncommitted file, for example:

```text
~/.codex/env/observability-mcp.env
```

The Codex MCP registration should source that file before starting the MCP
process. Secrets must not be committed to the repo or added to Kubernetes
ConfigMaps.

## Data Flow

1. Codex starts the local stdio MCP server.
2. The MCP reads `OBS_GRAFANA_URL` and token configuration from local env.
3. The MCP sends authenticated HTTPS requests to
   `observability-api.kylebradshaw.dev`.
4. Cloudflare or the chosen edge control validates the request.
5. Ingress routes accepted traffic to Grafana.
6. Grafana proxies datasource requests to internal Prometheus, Loki, and
   optionally Jaeger.
7. The MCP returns bounded evidence bundles to Codex.

## Components

- Kubernetes ingress: add or update monitoring ingress routing for
  `observability-api.kylebradshaw.dev`.
- Edge access policy: require authentication for the API host while leaving the
  dashboard host public.
- Observability MCP config: support Grafana URL/token mode.
- Grafana clients in MCP: implement Prometheus and Loki access through Grafana
  datasource APIs.
- Local Codex config: source the local token env file and run the MCP service.
- Documentation: record the normal MCP path and the SSH fallback path.

## Safety And Operations

The MCP remains read-only. It should only query metrics, logs, traces, and
runbook resources. It must not roll out deployments, scale workloads, purge
queues, mutate databases, edit secrets, or apply Kubernetes manifests.

SSH remains acceptable for break-glass diagnostics, cluster recovery, and cases
where the observability stack itself is unavailable. It should not be the normal
MCP data path.

Any shared-environment mutation required for ingress, Cloudflare tunnel, or
Grafana configuration must be represented as committed code or a committed ops
procedure before it is applied.

## Error Handling

The MCP should fail with clear source-level errors:

- Missing Grafana URL or token configuration.
- Authentication denied by the edge policy.
- Grafana datasource lookup failure.
- Grafana query failure.
- Backend datasource unavailable.

Existing bounded query behavior remains in force: max windows, max log lines,
max trace spans, allowlisted service names, and read-only tool semantics.

## Testing

- Unit-test Grafana client request construction and response parsing.
- Keep existing Prometheus, Loki, and Jaeger direct-client tests.
- Add workflow tests proving Grafana mode returns the same evidence bundle
  shape as direct mode.
- Add config tests for direct mode, Grafana mode, and invalid mixed config.
- Run `go test ./...` in `go/observability-mcp-service`.
- Before committing implementation changes, run the relevant Go preflight.

## Rollout Plan

1. Add the authenticated API hostname and edge policy.
2. Validate the API host can reach Grafana while the public dashboard host still
   works anonymously.
3. Add Grafana-backed Prometheus and Loki clients to the MCP.
4. Update local Codex MCP config to source the local token env file.
5. Use the MCP for normal observability evidence collection.
6. Keep the existing SSH-based skill path as a documented fallback.

## Open Follow-Up

Jaeger support through Grafana datasource APIs should be validated during
implementation. If Grafana's proxy path is awkward for trace lookup, ship
Prometheus and Loki first, then handle Jaeger with a dedicated protected route
or a later Grafana proxy adapter.
