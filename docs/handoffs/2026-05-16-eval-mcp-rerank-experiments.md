# Eval MCP Rerank Experiment Handoff

Date: 2026-05-16

## TL;DR

The eval MCP is wired to `services/eval` for listing datasets, creating
experiments, starting runs, polling runs, comparing runs, summarizing
experiments, and recording conclusions. It is not yet robust enough for
repeatable rerank experiments because it does not distinguish eval datasets
from retrieval collections, cannot discover/validate Qdrant collections, cannot
create datasets, and can produce misleading comparisons for failed runs.

Also, `go/eval-mcp-service/data/eval-mcp-auth.json` is a local token cache and
must not be tracked. `.gitignore` now ignores `go/eval-mcp-service/data/`.

## Current State

- MCP dataset available:
  - ID: `19d3ee69-6544-44b8-94bd-0fac110f06f5`
  - Name: `product-docs-rag-v1`
  - Items: `15`
- Runtime Qdrant collections observed in `ai-services`:
  - `documents`
  - `documents_qa`
- Ingestion metadata exists for `documents`.
- Ingestion metadata does not exist for `product-docs-rag-v1`.
- `product-docs-rag-v1` is an eval dataset name, not a Qdrant collection name.

## What Happened

Initial experiment:

- Experiment ID: `c6054294-7d39-43e8-8742-450b883a735b`
- Baseline run: `c14cf8b6-0c14-4135-b215-70bb78a88cea`
- Rerank run: `25925a05-8949-465d-9eb4-08456d2df15e`
- Collection used: `product-docs-rag-v1`
- Result: both runs failed.

Root cause:

- `services/eval` called `chat /search` with collection
  `product-docs-rag-v1`.
- `chat` queried Qdrant collection `product-docs-rag-v1`.
- Qdrant returned `404 Collection doesn't exist`.
- `chat /search` surfaced the failure as HTTP 500.
- Eval config capture also recorded `404` from
  `ingestion /collections/product-docs-rag-v1/config`.

Corrected experiment attempt:

- Experiment ID: `6751a5b8-8b77-48aa-bbde-f59d98afb82b`
- Baseline run: `124aeb82-8ba4-4d6e-9533-59c44446e216`
- Rerank run: `ca53e587-25e3-4f8c-bda6-7d4a3519848b`
- Collection used: `documents`
- Result: runs started, but external polling began returning nginx 503 because
  the eval pod became NotReady under load.

Runtime evidence:

- `ai-services` pod state showed `eval` as `0/1 Running` while the corrected
  runs were active.
- Kubernetes events showed readiness probe failures for `eval` and one
  readiness timeout for `chat`.
- Chat logs showed one rerank `/search` request taking about `38.7s`.
- Chat and eval deployments each have one replica; CPU limits are `500m`.

## Concerns Found

1. Dataset versus collection ambiguity.
   The MCP workflow says to list datasets, choose data, and start runs. It does
   not tell the agent that eval dataset names and Qdrant collection names are
   separate concepts.

2. Missing collection tools.
   The MCP has no tool to call ingestion `/collections` or
   `/collections/{name}/config`, so the agent cannot validate the retrieval
   collection before starting an eval.

3. Missing dataset creation.
   `services/eval` supports `POST /datasets`, but the MCP does not expose a
   dataset creation tool. From MCP, datasets can only be listed.

4. Failed run comparison is misleading.
   `compare_eval_runs` and `summarize_eval_experiment` can include failed runs,
   which results in empty cases and zero-like deltas. The workflow should only
   compare completed runs.

5. Rerank config snapshot is ambiguous.
   `chat /config` reports global `rerank_enabled: true`, even for baseline
   runs. That only means reranking is available. Eval run config should also
   store the per-run requested rerank flag.

6. Readiness and load behavior need hardening.
   `eval /health` calls `chat /health`; under long-running eval/rerank load,
   readiness can fail and the external API can return nginx 503 while work is
   still in progress.

7. Local token cache location was unsafe.
   The default token cache path is under the repo:
   `go/eval-mcp-service/data/eval-mcp-auth.json`. It should be ignored or moved
   outside the repo.

## Recommended Next Changes

1. Add MCP tools:
   - `list_rag_collections`
   - `get_rag_collection_config`
   - optionally `create_eval_dataset`

2. Update MCP prompt/workflow instructions:
   - Dataset selects golden questions.
   - Collection selects Qdrant retrieval corpus.
   - Never infer collection from dataset name.
   - Validate collection existence before `start_eval_experiment` or
     `start_eval_run`.
   - Run baseline and rerank sequentially until readiness/load issues are fixed.
   - Compare only `status == completed` runs.

3. Update `services/eval` run config capture:
   - Include `requested_rerank`.
   - Possibly include `effective_collection`.
   - Treat missing collection config as warning metadata, but make missing
     Qdrant collection fail clearly before starting the background task.

4. Harden eval/chat runtime for experiments:
   - Decouple readiness from downstream chat dependency, or make it less prone
     to transient timeout under active jobs.
   - Consider background worker isolation for eval jobs.
   - Consider sequential experiment orchestration in MCP.
   - Review CPU limits and worker counts before running concurrent rerank evals.

5. Security cleanup:
   - Keep `go/eval-mcp-service/data/` ignored.
   - If a token cache was ever committed, rotate the token and remove it from
     git history as appropriate.

## Useful Files

- `go/eval-mcp-service/README.md`
- `go/eval-mcp-service/internal/mcpserver/server.go`
- `go/eval-mcp-service/internal/evalworkflow/service.go`
- `go/eval-mcp-service/internal/evalapi/client.go`
- `services/eval/app/main.py`
- `services/eval/app/config_capture.py`
- `services/ingestion/app/main.py`
- `services/chat/app/main.py`
- `services/chat/app/chain.py`

## Safe Resume Prompt

Use this handoff:
`docs/handoffs/2026-05-16-eval-mcp-rerank-experiments.md`.

Goal: make eval MCP experiments reliable for baseline versus rerank. First add
collection discovery/validation to the MCP and update workflow instructions.
Then decide whether to add dataset creation and whether to harden
`services/eval` readiness/config capture in the same branch or a follow-up.
