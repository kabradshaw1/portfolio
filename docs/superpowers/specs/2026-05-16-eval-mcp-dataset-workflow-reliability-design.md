# Eval MCP Dataset And Workflow Reliability Design

## Purpose

Make the local eval MCP service reliable for repeatable baseline-versus-rerank
experiments. Agents should be able to discover curated eval dataset fixtures,
create eval API datasets from those fixtures, validate Qdrant retrieval
collections, and avoid misleading comparisons from failed or incomplete runs.

## Background

The current eval MCP workflow can list existing eval datasets and start eval
runs, but it does not distinguish golden eval datasets from Qdrant retrieval
collections strongly enough. A failed experiment used `product-docs-rag-v1` as
the retrieval collection because that was the eval dataset name. Qdrant did not
have that collection, so chat failed with collection-not-found errors and eval
comparisons later appeared zero-like rather than clearly invalid.

The first curated fixture already exists at
`docs/product-catalog/rag-eval-dataset-product-docs.json`. It references source
PDFs under `docs/product-catalog/`. Future experiments should add more curated
fixture files instead of asking agents to generate golden answers directly from
PDFs at runtime.

## Scope

Implement this in `go/eval-mcp-service` and its README/workflow prompt.

Include:

- curated dataset fixture discovery and validation
- eval API dataset creation from a curated fixture
- ingestion collection discovery and config lookup
- collection validation before experiment or run creation
- MCP prompt and workflow instructions that separate dataset and collection
  concepts
- comparison safeguards for non-completed runs

Exclude:

- Python eval/chat readiness and runtime load hardening
- generated golden-answer creation from PDFs or Markdown
- Kubernetes manifest changes
- changes to the eval API data model beyond using its existing `POST /datasets`
  contract

## Architecture

Add focused clients and workflow methods inside the MCP service:

- `evalapi.Client` remains the source for eval API calls and gains
  `CreateDataset`.
- A new ingestion API client calls ingestion `/collections` and
  `/collections/{name}/config`.
- A new fixture catalog component scans configured fixture roots, parses JSON
  fixtures, validates item shape and source references, and returns stable
  fixture metadata.
- `evalworkflow.Service` coordinates fixture import, collection validation,
  experiment creation, run creation, comparison, and experiment summaries.
- `mcpserver` exposes the new tools and updates the `eval` prompt plus
  `eval://workflow` resource.

These boundaries keep file IO, HTTP calls, workflow policy, and MCP schema
handling separately testable.

## Dataset Fixture Model

Curated fixture JSON uses the existing eval API request shape:

```json
{
  "name": "product-docs-rag-v1",
  "items": [
    {
      "query": "How long does the Laptop Pro 15 battery last?",
      "expected_answer": "The Laptop Pro 15 has a 72 Wh battery rated for up to 10 hours of mixed use.",
      "expected_sources": ["laptop-pro-15-specs.pdf"]
    }
  ]
}
```

Validation rules:

- `name` must match the eval API dataset naming rules:
  `^[a-zA-Z0-9_-]+$`, length 1 to 100.
- `items` must contain 1 to 100 entries.
- every item must have non-empty `query` and `expected_answer`.
- `query` must be at most 2000 characters.
- `expected_answer` must be at most 5000 characters.
- `expected_sources` is optional but, when present, must contain only relative
  basenames or relative paths under the fixture document root.
- each referenced source must exist and must be a `.pdf` or `.md` file.
- source paths must not escape the configured document root.

The initial fixture root is `docs/product-catalog/`, which contains both the
fixture file and the referenced product documents. The catalog should support
additional fixture files in that root without code changes.

## Configuration

Add MCP configuration fields:

- `EVAL_MCP_DATASET_FIXTURE_ROOTS`: optional path-list of fixture roots. Default
  resolves to the repo `docs/product-catalog` directory from the
  `go/eval-mcp-service` working directory.
- `EVAL_MCP_INGESTION_URL`: optional ingestion API base URL. Default should
  match local development, such as `http://localhost:8000/ingestion`, following
  existing project URL conventions.

Keep existing eval API and auth configuration unchanged.

## MCP Tools

Add `list_eval_dataset_fixtures`.

Input: none for the first version.

Output for each fixture:

- fixture ID or stable relative path
- dataset name
- fixture path
- document root
- item count
- referenced source files
- validation status and validation errors, if any

Add `create_eval_dataset`.

Input:

- `fixture`: fixture ID or relative path from `list_eval_dataset_fixtures`

Behavior:

- validate the selected fixture
- fail without calling eval API if validation fails
- call eval API `POST /datasets`
- return the created dataset ID and dataset name
- surface duplicate dataset names clearly from the eval API `409`

Add `list_rag_collections`.

Input: none.

Behavior:

- call ingestion `/collections`
- return collection names and point counts where provided
- surface ingestion connectivity or auth failures as MCP tool errors

Add `get_rag_collection_config`.

Input:

- `name`: collection name

Behavior:

- call ingestion `/collections/{name}/config`
- return metadata when present
- return a clear not-found response when ingestion reports missing metadata or
  missing collection

Update `start_eval_experiment` and `start_eval_run`.

Behavior:

- require or default collection as today
- validate the collection exists by calling ingestion before eval API calls
- reject missing collections with a message that names the requested collection
- do not infer collection from dataset name

Update `compare_eval_runs`.

Behavior:

- resolve explicit IDs and experiment labels as today
- fetch each run before comparing
- reject any run whose status is not `completed`
- name the offending run IDs and statuses in the error
- call eval API compare only after all runs are completed

Update `summarize_eval_experiment`.

Behavior:

- include baseline and candidate run details as today
- calculate worst cases only for completed runs with results
- compare only completed runs
- if any attached run is not completed, return an explicit summary warning or
  tool error rather than silently producing misleading deltas

## Workflow Prompt

Revise the `eval` prompt and `eval://workflow` resource to state:

- A dataset is the golden question and expected-answer set.
- A collection is the Qdrant retrieval corpus.
- Never infer collection from dataset name.
- Start by listing existing eval datasets and curated dataset fixtures.
- Use `create_eval_dataset` when a curated fixture exists locally but the eval
  API dataset is missing.
- List and validate RAG collections before creating experiments or runs.
- Run baseline first and wait for completion.
- Run rerank candidate after the baseline completes until runtime hardening is
  in place.
- Compare only completed runs.
- Record conclusions only after user approval.

## Error Handling

MCP tool errors should be actionable and should avoid exposing irrelevant stack
details. Important cases:

- invalid fixture JSON names the fixture path and invalid field
- missing source file names the missing `expected_sources` value
- path traversal source references are rejected before filesystem access
- duplicate dataset names report that the eval API already has the dataset
- missing Qdrant collection reports the collection name and asks the agent to
  choose from `list_rag_collections`
- non-completed comparison reports run IDs and statuses

## Testing

Add Go unit tests for:

- fixture discovery with the product-catalog fixture
- valid fixture parsing and metadata output
- invalid fixture name
- empty items
- missing `query` or `expected_answer`
- missing expected source file
- path traversal in expected source
- eval API client `POST /datasets`
- ingestion client `/collections`
- ingestion client `/collections/{name}/config`
- workflow collection validation before experiment creation
- workflow collection validation before run creation
- comparison rejection for running or failed runs
- summary behavior with incomplete attached runs
- MCP tool registration and schema list
- MCP handlers for new tools
- prompt and workflow text mention dataset fixtures, collection validation, and
  completed-only comparison

Verification target:

```bash
make preflight-go
```

If full preflight is blocked, run focused tests from `go/eval-mcp-service`:

```bash
go test ./...
```

## Implementation Notes

The implementation branch should be a feature worktree because this changes a
deployable Go service. Target the PR to `qa`.

The existing `.gitignore` rule for `go/eval-mcp-service/data/` should remain in
place so local token cache files are not tracked.
