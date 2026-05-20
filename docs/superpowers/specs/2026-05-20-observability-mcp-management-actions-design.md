# Observability MCP Management Actions Design

## Summary

Issue 251 adds an opt-in management-action layer to the Observability MCP. The
goal is agent-autonomous operations without free-form mutation: agents may run
cataloged, low-risk operational procedures, but every shared-environment write
must still route through committed ops-as-code artifacts.

The MCP should not become a general shell, Kubernetes client, Grafana write
client, or arbitrary script launcher. It should expose typed, auditable actions
whose commands, inputs, risk tier, timeouts, redaction rules, and verification
expectations are reviewed as code.

## Goals

- Allow agents to autonomously execute low-risk, cataloged operational actions.
- Preserve ops-as-code: every mutation must be backed by committed repo code.
- Keep management actions disabled by default so the current read-only contract
  remains the safe default.
- Record previews, blocked decisions, executions, failures, and completions in
  local incident history when history is enabled.
- Return structured, bounded evidence that lets an agent continue triage after
  an action runs or is blocked.
- Start with already-committed low-risk procedures such as Grafana alerting
  reload and manual Postgres backup verification.

## Non-Goals

- No free-form shell, kubectl arguments, Grafana mutation API, SSH command
  strings, arbitrary URLs, or arbitrary script paths.
- No web UI for approving actions.
- No requirement that Kyle approve every low-risk action before execution.
- No high-risk execution by default.
- No persistent server-side queue. The local stdio MCP can execute an action
  synchronously with timeout-bounded output.
- No replacement for existing read-only evidence tools.

## Current Context

`go/observability-mcp-service` is a local stdio MCP. It gathers bounded evidence
from Prometheus, Loki, Jaeger, Grafana alert metadata, embedded runbooks, and
local SQLite incident history.

The README currently states that the service is read-only and does not silence
alerts, restart workloads, run ops commands, or mutate external systems. This
design intentionally changes that only behind explicit opt-in configuration and
only for committed, cataloged actions.

The `ops-as-code` rule still applies: no mutating action runs against a shared
environment unless its exact behavior exists as committed code in this repo.
Existing examples live under `scripts/ops/`, including:

- `scripts/ops/2026-05-09-reload-grafana-alerting.sh`
- `scripts/ops/2026-05-15-run-postgres-backup-verify.sh`
- `scripts/ops/check-postboot-health.sh`

## Design

### Action Catalog

Add a management action catalog. The first implementation should use Go code for
type safety and straightforward unit tests. A file-backed YAML catalog can be
added later if non-Go editing becomes valuable.

Each action declares:

- `id`
- `title`
- `description`
- `risk_tier`
- `script_path`
- `allowed_args`
- `timeout`
- `requires_incident`
- `idempotent`
- `preflight` summary
- `postflight` summary
- `redaction` rules
- `next_steps` or rollback guidance

Initial risk tiers:

- `diagnostic`: read-only or check-only procedures.
- `low_risk_mutation`: bounded, idempotent committed procedures that may run
  autonomously after management actions are enabled.
- `high_risk_mutation`: destructive, broad, data-owning, or public-impacting
  procedures. These are preview-only unless a separate high-risk opt-in flag is
  set.

Initial catalog candidates:

- `reload_grafana_alerting`: low-risk mutation backed by the existing Grafana
  alerting reload script.
- `run_postgres_backup_verify`: low-risk mutation backed by the existing manual
  backup verification script.
- `check_postboot_health`: diagnostic only after implementation verifies that
  the existing script performs no shared-environment mutation. If it mutates
  runtime state, leave it out of the initial catalog.

Catalog validation should reject duplicate IDs, empty descriptions, invalid
risk tiers, missing or absolute script paths, paths outside the repo, timeout
values outside a bounded range, unknown argument types, and catalog entries
whose scripts do not exist.

### Policy

Add a policy evaluator that returns one of:

- `allow`
- `block`
- `preview_only`

Default behavior:

- All management execution is disabled unless
  `OBS_MANAGEMENT_ACTIONS_ENABLED=true`.
- `diagnostic` actions can run when management actions are enabled.
- `low_risk_mutation` actions can run when management actions are enabled.
- `high_risk_mutation` actions return preview-only unless
  `OBS_MANAGEMENT_ALLOW_HIGH_RISK=true`.

A blocked result should not look like an execution failure. It should report the
action ID, risk tier, decision, policy reason, and what configuration would be
required to proceed.

### Runner

Add a runner that executes only catalog entries from the repo root.

Runner constraints:

- No public API accepts shell fragments, kubectl args, SSH commands, or script
  paths.
- The command shape is fixed by the catalog. For no-argument actions, it is
  equivalent to `bash scripts/ops/<name>.sh`.
- Inputs must match catalog-declared arguments and must be converted without
  shell interpolation.
- The runner uses context deadlines and kills timed-out actions.
- Stdout and stderr are bounded before returning or storing.
- Output is redacted before it leaves the runner.
- Missing scripts, path traversal, non-executable paths where required, timeout,
  and nonzero exits produce structured results.

The runner should not try to infer success from arbitrary output. Catalog
entries should state what postflight evidence or verification the script itself
performs. The first cataloged scripts already include their own checks and
nonzero exit behavior.

### MCP Tools

Add these tools:

- `list_management_actions`
- `preview_management_action`
- `execute_management_action`
- `get_management_action_history`

`list_management_actions` returns the catalog, excluding internal implementation
details and sensitive redaction patterns.

`preview_management_action` validates inputs and returns the policy decision,
fixed script path, risk tier, timeout, preflight summary, postflight summary,
and expected result shape. It never executes the script.

`execute_management_action` validates inputs, evaluates policy, records the
decision when possible, runs allowed actions, bounds and redacts output, records
the final result when possible, and returns a structured result.

`get_management_action_history` returns recent action events, optionally filtered
by `incident_key`, `action_id`, and decision/status.

### Result Shape

Action results should include:

- `action_id`
- `risk_tier`
- `decision`
- `status`
- `script_path`
- `incident_key`
- `started_at`
- `completed_at`
- `duration_ms`
- `exit_code`
- bounded `stdout`
- bounded `stderr`
- `output_truncated`
- `redactions_applied`
- `policy_reason`
- `history_event_ids`
- `warning`

The result status vocabulary should distinguish policy and execution outcomes:

- `previewed`
- `blocked`
- `running`
- `succeeded`
- `failed`
- `timed_out`

### History Integration

Extend the existing local SQLite history store with management action events.
Suggested event types:

- `management_action_previewed`
- `management_action_started`
- `management_action_completed`
- `management_action_failed`
- `management_action_blocked`

When `incident_key` is provided, action events should attach to that incident
timeline. If the incident does not exist, the tool should create it only when
`incident_title` is also provided; otherwise it should return a structured
validation error before execution. This avoids orphaned action history with
empty incident metadata.

When history is disabled, action execution can still run. The result should
include a warning that no local audit record was persisted.

### Configuration

Add these environment variables:

- `OBS_MANAGEMENT_ACTIONS_ENABLED`: default `false`.
- `OBS_MANAGEMENT_ALLOW_HIGH_RISK`: default `false`.
- `OBS_MANAGEMENT_ACTION_TIMEOUT`: optional global maximum timeout.
- `OBS_MANAGEMENT_MAX_OUTPUT_BYTES`: bounded output cap with a conservative
  default.

The README should be updated to state that the MCP is read-only by default, and
that management actions are opt-in, cataloged, and ops-as-code only.

## Error Handling

Unknown action IDs, disabled management actions, high-risk actions without
explicit enablement, invalid arguments, missing scripts, invalid catalog
metadata, timeouts, and nonzero exits should all return structured responses.

Blocked actions are policy decisions, not execution failures.

Execution failures should include bounded stdout, bounded stderr, exit code,
duration, and catalog-declared next steps. Secret-looking values and
catalog-declared sensitive fields must be redacted before output is returned or
stored.

If history persistence fails, the action result should include a warning. A
history write failure should not hide the execution result.

## Testing

Add unit tests for:

- Catalog validation: IDs, paths, risk tiers, duplicate IDs, timeout bounds, and
  missing scripts.
- Policy matrix: disabled, diagnostic, low-risk, high-risk with and without
  high-risk opt-in.
- Runner behavior: fixed paths only, unknown args rejected, timeout handling,
  nonzero exits, output bounds, and redaction.
- MCP handlers: list, preview, execute, history, blocked vs failed behavior.
- History persistence: action timeline events attach to incidents and can be
  queried.
- README/config documentation: management actions are opt-in and cataloged, not
  free-form mutation.

Before committing implementation, run:

```bash
make preflight-go
```

## Acceptance Criteria

- Existing read-only tools keep working with management actions disabled by
  default.
- Agents can autonomously run low-risk cataloged ops procedures after opt-in.
- High-risk actions are preview-only unless explicitly enabled.
- No free-form shell, kubectl, Grafana mutation, SSH command, or arbitrary
  script path is exposed.
- Every executed action returns a structured result.
- When history is enabled, management action events are persisted on the
  incident timeline.
- The initial catalog includes only already-committed low-risk or diagnostic
  procedures.
- Documentation clearly explains the agent-autonomous ops-as-code model.
