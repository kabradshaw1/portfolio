# Eval MCP Service

Local-only MCP stdio server for agent-driven RAG evaluation experiments.

The Python eval API is the source of truth for datasets, evaluations,
experiments, labels, decisions, conclusions, and evidence. This MCP service is
a local stdio workflow adapter over that API.

## Run Directly

```bash
go run ./cmd/eval-mcp
```

The service logs to stderr and reserves stdout for MCP protocol messages.

## Register With Codex

```bash
codex mcp add eval -- zsh -lc 'cd /Users/kylebradshaw/repos/gen_ai_engineer/go/eval-mcp-service && exec go run ./cmd/eval-mcp'
```

## Configuration

- `EVAL_API_URL`: eval API base URL, defaults to `http://localhost:8000/eval`.
- `EVAL_API_TOKEN`: optional bearer token for authenticated eval API calls.
- `EVAL_MCP_POLL_INTERVAL`: polling interval, defaults to `1s`.
- `EVAL_MCP_WAIT_TIMEOUT`: wait timeout, defaults to `5m`.

## Tools

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
