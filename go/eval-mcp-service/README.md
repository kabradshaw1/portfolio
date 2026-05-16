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
export EVAL_MCP_AUTH_EMAIL='you@example.com'
export EVAL_MCP_AUTH_PASSWORD='use-a-local-secret-source'
export AUTH_SERVICE_URL='http://localhost:8091/auth'

codex mcp add eval -- zsh -lc 'cd /Users/kylebradshaw/repos/gen_ai_engineer/go/eval-mcp-service && exec go run ./cmd/eval-mcp'
```

## Configuration

- `EVAL_API_URL`: eval API base URL, defaults to `http://localhost:8000/eval`.
- `EVAL_MCP_INGESTION_URL`: ingestion API base URL for collection discovery,
  defaults to `http://localhost:8000/ingestion`.
- `EVAL_API_TOKEN`: optional bearer token override for eval API calls. When set,
  the service skips auth-service login and token cache refresh.
- `AUTH_SERVICE_URL`: auth-service base URL, defaults to
  `http://localhost:8091/auth`.
- `EVAL_MCP_AUTH_EMAIL`: auth-service email. Required when `EVAL_API_TOKEN` is
  empty.
- `EVAL_MCP_AUTH_PASSWORD`: auth-service password. Required when
  `EVAL_API_TOKEN` is empty.
- `EVAL_MCP_TOKEN_CACHE_PATH`: local token cache path, defaults to
  `data/eval-mcp-auth.json`.
- `EVAL_MCP_TOKEN_REFRESH_SKEW`: refresh lead time before access token expiry,
  defaults to `1m`.
- `EVAL_MCP_POLL_INTERVAL`: polling interval, defaults to `1s`.
- `EVAL_MCP_WAIT_TIMEOUT`: wait timeout, defaults to `5m`.
- `EVAL_MCP_DATASET_FIXTURE_ROOTS`: path-list of curated eval fixture roots,
  defaults to the repo `docs/product-catalog` directory.

## Tools

- `start_eval_experiment`
- `list_eval_experiments`
- `get_eval_experiment`
- `list_eval_datasets`
- `list_eval_dataset_fixtures`
- `create_eval_dataset`
- `list_rag_collections`
- `get_rag_collection_config`
- `start_eval_run`
- `wait_for_eval_run`
- `attach_eval_run`
- `get_eval_run`
- `compare_eval_runs`
- `get_worst_eval_cases`
- `summarize_eval_experiment`
- `record_eval_experiment_conclusion`

Eval datasets and RAG collections are separate concepts. Datasets contain
golden questions and expected answers. Collections are Qdrant retrieval corpora.
Use curated fixture tools to create missing datasets, then validate the chosen
RAG collection before starting baseline and rerank runs.
