# Local Eval MCP Adapter Design

## Summary

Add a local-only stdio MCP service for agent-driven RAG evaluation iteration.
The MCP service will orchestrate experiments over the existing Python eval API,
store lightweight local experiment metadata, and return agent-friendly run
summaries, comparisons, and worst-case slices.

The Python eval service remains the source of truth for datasets, evaluation
runs, scores, config snapshots, and per-query results. The MCP service must not
duplicate scoring logic or persist full eval results.

## Goals

- Make iterative RAG eval work fast from Codex or another MCP client.
- Avoid manually copying run data from the browser UI into an agent session.
- Preserve the existing eval API as the durable backend.
- Track local experiment context: hypothesis, focus metric, baseline, candidate
  labels, notes, and conclusion.
- Match the existing local stdio MCP pattern used by `qa-mcp-service` and
  `coding-exercises-mcp-service`.

## Non-Goals

- Do not replace `/ai/eval` or the Python eval API.
- Do not create a deployable shared MCP gateway in the first version.
- Do not store full eval results, datasets, retrieved contexts, generated
  answers, or scores in the MCP SQLite database.
- Do not add new RAG scoring metrics as part of this design.
- Do not mutate Kubernetes, production, or QA infrastructure.

## Architecture

Create a new Go service under `go/eval-mcp-service`.

Suggested package layout:

- `cmd/eval-mcp`: stdio entrypoint.
- `internal/config`: local configuration for eval API URL, token source, DB path,
  polling interval, and timeout.
- `internal/evalapi`: typed HTTP client for the Python eval API.
- `internal/store`: SQLite store for experiment metadata.
- `internal/evalworkflow`: orchestration over eval API calls plus local
  experiment state.
- `internal/mcpserver`: MCP prompt, workflow resource, tool schemas, and tool
  handlers.

Runtime is local-only. The process logs to stderr and reserves stdout for MCP
protocol messages.

## Data Ownership

The Python eval service owns:

- datasets
- evaluation runs
- run status
- aggregate scores
- per-query results
- score reasons
- config snapshots
- built-in run comparison responses

The MCP SQLite store owns only:

- experiment ID and name
- dataset ID
- collection
- baseline eval ID
- focus metric, such as `context_precision`
- run labels mapped to eval IDs
- hypothesis and free-form notes
- conclusion
- timestamps

When an MCP tool needs scores or result details, it fetches them from the eval
API at call time.

## MCP Surface

Expose one prompt:

- `eval`: starts an agent-led eval experiment workflow.

Expose one resource:

- `eval://workflow`: concise workflow instructions for agents.

Initial tools:

- `start_eval_experiment`
- `list_eval_experiments`
- `get_eval_experiment`
- `list_eval_datasets`
- `start_eval_run`
- `wait_for_eval_run`
- `attach_eval_run`
- `get_eval_run`
- `compare_eval_runs`
- `get_worst_eval_cases`
- `summarize_eval_experiment`
- `record_eval_experiment_conclusion`

The tools should support both session-aware and direct usage. For example,
`compare_eval_runs` can accept explicit eval IDs, but if an experiment ID is
provided it can resolve run labels such as `baseline` and `rerank_candidate`.

## Workflow

Expected agent flow:

1. Start or resume a named experiment.
2. Confirm dataset, collection, focus metric, and hypothesis.
3. Attach or create a baseline run.
4. Start candidate runs with descriptive labels.
5. Wait for completion when requested.
6. Compare candidates against baseline.
7. Retrieve worst cases for the focus metric.
8. Summarize likely causes and suggest the next experiment.
9. Record the final conclusion when the user approves it.

Example user request:

```text
Use eval MCP. Start experiment "precision tuning" for dataset rag-baseline,
focus context_precision. Compare rerank against baseline and show the worst
precision failures.
```

## API Client Behavior

The MCP service should call the existing eval endpoints rather than reaching
into the eval database directly.

Required eval API operations:

- list datasets
- start evaluation
- get evaluation
- list evaluations when needed for lookup
- compare evaluations

If the Python eval API lacks a direct endpoint needed for worst-case slicing,
the MCP can derive the slice from `get evaluation` by sorting returned
per-query results locally. It should return bounded output by default so agent
context stays small.

## Authentication And Configuration

The first version is local-only but still needs to call an authenticated eval
API when auth is enabled.

Configuration:

- `EVAL_MCP_DB_PATH`: SQLite path, default `data/eval-mcp.db`.
- `EVAL_API_URL`: eval API base URL, default `http://localhost:8000/eval`.
- `EVAL_API_TOKEN`: optional bearer token for local authenticated calls.
- `EVAL_MCP_POLL_INTERVAL`: default polling interval for run waits.
- `EVAL_MCP_WAIT_TIMEOUT`: maximum wait time for a single wait operation.

The service should fail clearly when auth is required and no token is supplied.

## Error Handling

- Tool argument validation errors return MCP tool errors, not process failures.
- HTTP failures include status code and a short response excerpt.
- Polling timeout returns the latest known run status and does not mark the run
  failed locally.
- Missing local experiment IDs or labels return actionable errors listing known
  labels when possible.
- Stale local run references are tolerated; the MCP reports that the eval API
  no longer knows the referenced run.

## Testing

Follow the existing Go MCP service pattern:

- unit tests for config parsing
- eval API client tests with `httptest`
- SQLite store tests
- workflow service tests for session metadata and label resolution
- MCP handler tests for valid calls, validation errors, and JSON responses
- command startup smoke test verifying stderr logging and stdio runner wiring

Relevant verification before commit of implementation:

```bash
make preflight-go
```

## UI Relationship

The browser UI remains useful for human review, portfolio demonstration, and
dashboard-style trend inspection. It should no longer be the primary loop for
agent-driven eval iteration. The MCP service becomes the agent workflow
surface; the eval API remains the backend; the UI remains the human dashboard.
