# Eval MCP Service

Local-only MCP stdio server for agent-driven RAG evaluation experiments.

The Python eval API is the source of truth for datasets, evaluations,
experiments, labels, decisions, conclusions, and evidence. This MCP service is
a local stdio workflow adapter over that API.

## User Experience

Register the MCP server with Codex once, then restart Codex. In normal use,
Codex starts this stdio server for the session; you do not keep a separate
`go run ./cmd/eval-mcp` process open.

After registration, ask for eval work in plain language:

```text
Use eval MCP to list datasets.
Start an eval experiment comparing baseline vs rerank.
Summarize the eval experiment and show the worst cases.
```

The server exposes:

- Prompt: `eval`
- Resource: `eval://workflow`
- Tools listed in [Tools](#tools)

Agents should start with the `eval` prompt or the `eval://workflow` resource
when available. The workflow is: list datasets, create or resume an experiment,
start baseline and candidate runs, wait for completion, compare runs, inspect
worst cases, summarize evidence, and only record a conclusion after the user
approves it.

## Model Ladder Experiments

Model ladder experiments vary the answer model while keeping the eval judge
model fixed across candidates.

- `answer_tier`: local, efficient, or premium.
- `answer_provider`: ollama, openai, or anthropic.
- `answer_base_url`: provider endpoint when required.
- `answer_model`: model name.
- `answer_api_key_secret`: environment variable name containing the provider
  key.

For model ladder experiments, keep the judge model fixed and vary
`answer_tier`, `answer_provider`, `answer_model`, `retrieval_config`, and
`rerank`. Start one run at a time, wait for completion, compare completed or
`completed_with_failures` runs, inspect worst cases, and record a conclusion
only after the user approves it.

## Run Directly

This is mainly useful for smoke testing. In normal use, Codex or another MCP
client starts the process and communicates with it over stdio.

```bash
go run ./cmd/eval-mcp
```

The service logs to stderr and reserves stdout for MCP protocol messages.

## Register With Codex

Local development defaults:

```bash
export EVAL_MCP_AUTH_EMAIL='you@example.com'
export EVAL_MCP_AUTH_PASSWORD='use-a-local-secret-source'
export AUTH_SERVICE_URL='http://localhost:8091/auth'

codex mcp add eval -- zsh -lc 'cd /Users/kylebradshaw/repos/gen_ai_engineer/go/eval-mcp-service && exec go run ./cmd/eval-mcp'
```

Production portfolio API:

```bash
mkdir -p ~/.codex/env
chmod 700 ~/.codex/env

cat > ~/.codex/env/eval-mcp.env <<'EOF'
export EVAL_API_URL='https://api.kylebradshaw.dev/eval'
export AUTH_SERVICE_URL='https://api.kylebradshaw.dev/go-auth/auth'
export EVAL_MCP_AUTH_EMAIL='smoke@kylebradshaw.dev'
export EVAL_MCP_AUTH_PASSWORD='use-a-local-secret-source'
EOF
chmod 600 ~/.codex/env/eval-mcp.env

codex mcp add eval -- zsh -lc 'source ~/.codex/env/eval-mcp.env && cd /Users/kylebradshaw/repos/gen_ai_engineer/go/eval-mcp-service && exec go run ./cmd/eval-mcp'
```

Verify registration:

```bash
codex mcp list
codex mcp get eval
```

Restart Codex after adding the MCP server or changing the env file.

## Configuration

- `EVAL_API_URL`: eval API base URL, defaults to `http://localhost:8000/eval`.
- `RAG_TRIAGE_API_URL`: RAG triage API base URL, defaults to
  `http://localhost:8000/rag-triage`.
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
- `EVAL_MCP_MAX_BACKOFF`: maximum delay after repeated eval API `429`
  responses, defaults to `30s`.
- `EVAL_MCP_DATASET_FIXTURE_ROOTS`: path-list of curated eval fixture roots,
  defaults to the repo `docs/product-catalog` directory.

When eval API polling receives `429`, the client respects `Retry-After` before
falling back to deterministic exponential backoff capped by
`EVAL_MCP_MAX_BACKOFF`.

## Eval Run Evidence

Use `get_eval_run_evidence` when a run is still running, failed, or timed out
from `wait_for_eval_run`. It returns the durable eval API state, captured RAG
configuration, result counts, item counts when available, stale-running
detection, and next-step guidance. For runtime evidence, follow its guidance
with the observability MCP
`investigate_eval_run` tool, which queries eval metrics and eval-id-scoped
logs.

## Auth Notes

For the production portfolio API, the external Go auth route is
`https://api.kylebradshaw.dev/go-auth/auth`, not `/auth`. The MCP auth client
appends `/login` and `/refresh` to `AUTH_SERVICE_URL`.

The smoke-test email is `smoke@kylebradshaw.dev`. Store the password in a local
secret source such as `~/.codex/env/eval-mcp.env`, or use `EVAL_API_TOKEN`
instead. GitHub Actions secret values such as `SMOKE_GO_PASSWORD` are
write-only; they can be rotated but not retrieved.

DLQ replay tools are operator-gated by the eval API. Use operator credentials
or an operator-scoped `EVAL_API_TOKEN` before calling `list_eval_item_dlq` or
`replay_eval_item_dlq`. `replay_eval_item_dlq` is mutating and should only be
run after explicit approval.

CORS is normally irrelevant for this MCP server because it makes server-side
HTTP requests from the local stdio process, not browser fetches. If MCP calls
fail, check DNS, route paths, credentials, JWT validity, and eval API health
before changing CORS.

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
- `get_eval_run_evidence`
- `compare_eval_runs`
- `get_worst_eval_cases`
- `list_eval_item_dlq`
- `replay_eval_item_dlq`
- `triage_rag_regression`
- `summarize_eval_experiment`
- `record_eval_experiment_conclusion`

Eval datasets and RAG collections are separate concepts. Datasets contain
golden questions and expected answers. Collections are Qdrant retrieval corpora.
Use curated fixture tools to create missing datasets, then validate the chosen
RAG collection before starting baseline and rerank runs.

Use `triage_rag_regression` after a completed or `completed_with_failures` eval
run when worst cases need a single triage packet. Provide `baseline_eval_id` to
compare a candidate against the baseline, and set `include_observability` when
runtime evidence is needed.
