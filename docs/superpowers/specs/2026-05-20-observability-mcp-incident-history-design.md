# Observability MCP Incident History Design

## Summary

Issue 252 adds durable local incident history to `go/observability-mcp-service`.
The MCP remains read-only toward shared infrastructure: it continues to query
Prometheus, Loki, Jaeger, and Grafana, and only writes to a local SQLite history
database. The design stores incident timelines, immutable evidence snapshots,
operator notes, status changes, and evidence-window comparisons.

This should ship before issue 251. Issue 252 creates the audit and timeline
substrate that later gated management actions can attach to without inventing a
separate persistence model.

## Goals

- Persist bounded evidence bundles collected by the observability MCP.
- Group evidence snapshots and notes into durable incidents.
- Support incident lifecycle status updates for investigation review.
- Compare saved evidence windows, or compare saved evidence against fresh
  evidence collected with the same tool shape.
- Preserve the ops-as-code boundary for future issue 251 management actions.

## Non-Goals

- Do not mutate Kubernetes, databases, queues, alerts, rollouts, or secrets.
- Do not implement issue 251 management actions.
- Do not build a full incident-management system with owners, paging, SLAs, or
  alert ingestion.
- Do not add a frontend dashboard in this issue.
- Do not perform semantic log diffing in the first comparison implementation.

## Architecture

Add a local SQLite-backed store package under `go/observability-mcp-service`.
This follows the local MCP persistence pattern already used by the QA and coding
exercise MCP services.

The workflow service remains responsible for collecting evidence and building
`EvidenceBundle` values. The MCP server layer remains responsible for input
validation and schema handling. SQL stays behind a store interface so workflow
and MCP handler tests can use fakes.

Persistence is best-effort for existing investigation tools. If evidence
collection succeeds but storing the snapshot fails, the tool still returns the
evidence bundle and includes a persistence warning in the response metadata or
findings. Explicit history tools return tool errors when the store is disabled
or unavailable because persistence is their primary behavior.

## Data Model

`incidents`

- `id`
- `incident_key`, unique stable identifier supplied by the caller
- `title`
- `status`: `investigating`, `mitigated`, or `resolved`
- `severity`
- `service`
- `created_at`
- `updated_at`

`timeline_events`

- `id`
- `incident_id`
- `event_type`: initially `evidence_snapshot`, `note_added`, `status_changed`
- `summary`
- `details_json`
- `created_at`

Reserve the timeline model for future issue 251 event types such as
`action_requested`, `action_approved`, `action_executed`, and
`post_action_evidence`. Issue 252 should not implement those actions.

`evidence_snapshots`

- `id`
- `incident_id`, nullable for standalone snapshots if needed
- `timeline_event_id`
- `tool`
- `window_from`
- `window_to`
- `window_duration`
- `status`
- `partial`
- `critical_findings`
- `warning_findings`
- `signal_count`
- `log_count`
- `trace_count`
- `source_statuses_json`
- `bundle_json`, the immutable full `EvidenceBundle`
- `created_at`

Evidence snapshots are immutable after insert. Incident metadata can change, but
status changes and notes are preserved as timeline events.

## MCP Tools

Add new tools:

- `record_evidence_snapshot`: persist an evidence bundle explicitly and
  optionally attach it to an incident.
- `list_incidents`: return incident summaries newest first, filterable by
  status, service, and severity.
- `get_incident_history`: return incident metadata plus ordered timeline events
  and compact evidence snapshot summaries.
- `add_incident_note`: append a note and optionally transition incident status.
- `compare_evidence_windows`: compare two saved snapshots, or compare a saved
  snapshot with freshly collected evidence.

Extend existing investigation tool inputs with optional persistence metadata:

- `incident_key`
- `incident_title`
- `severity`
- `persist`

When `incident_key` is present, the tool stores the evidence snapshot and
creates or updates the incident. A configuration flag may enable broader
auto-capture for investigation calls that do not supply an incident key.

## Comparison Behavior

The first implementation compares practical operational fields:

- bundle status changes
- partial/error changes
- source availability changes
- signal values matched by signal name
- finding count and severity changes
- log count changes
- trace count changes

The comparison result should call out improvements, regressions, and unchanged
signals. It should retain enough source snapshot metadata for auditability
without returning full raw bundles unless requested.

## Configuration

Add configuration fields:

- `OBS_HISTORY_ENABLED`, default `true`
- `OBS_HISTORY_DB_PATH`, default user-writable local path
- `OBS_HISTORY_AUTO_CAPTURE`, default `false`

The default DB path should be local to the developer machine, for example under
`~/.codex/data/observability-mcp/history.db`. Tests can override this path with
temporary directories or in-memory SQLite.

Startup opens and migrates the DB only when history is enabled. If history is
disabled, existing evidence tools continue to work and history-specific tools
return clear tool errors.

## Error Handling

- Evidence collection failures keep the current partial-bundle behavior.
- Persistence failures during investigation calls do not mask successful
  evidence collection.
- Explicit history tools fail when required incident keys, snapshot IDs, or
  malformed status values are missing.
- SQLite migration/open failures should fail startup when history is enabled.
- Invalid history config should produce clear startup errors.

## Safety Boundary

Issue 252 writes only to local SQLite. It does not execute operational actions.
Future issue 251 management actions must remain under ops-as-code: any action
request should point to committed repo artifacts, such as `scripts/ops/...` or
Kubernetes job manifests, rather than storing or running arbitrary shell
commands.

## Testing

Store tests:

- migrations are idempotent
- incident upsert preserves stable identity
- evidence snapshots are inserted immutably
- timeline events are ordered by creation time and ID
- list filters work for status, service, and severity
- note and status updates append timeline events

Workflow tests:

- investigation methods save snapshots when `incident_key` is supplied
- auto-capture obeys config
- persistence failure does not turn a successful investigation into a tool error
- comparison reports status, source, signal, finding, log, and trace deltas

MCP server tests:

- new schemas reject malformed inputs
- new handlers call the expected service methods
- existing investigation handlers decode optional persistence metadata
- history tools report clear tool errors when persistence is disabled

Config tests:

- history defaults are loaded
- DB path override is honored
- history can be disabled
- invalid values are rejected

## Implementation Notes

- Add `modernc.org/sqlite` to `go/observability-mcp-service` rather than using a
  CGO-dependent driver.
- Keep SQL in a dedicated store package.
- Keep comparison logic in workflow/service code instead of SQL-heavy ad hoc
  queries.
- Store full evidence JSON for audit and replay, but index only the summary
  fields needed for list, filter, and comparison.
