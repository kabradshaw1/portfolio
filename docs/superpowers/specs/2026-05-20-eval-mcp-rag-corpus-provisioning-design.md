# Eval MCP RAG Corpus Provisioning Design

## Purpose

Give the local eval MCP a controlled way to prepare RAG vector collections for
evaluation without making eval runs mutate the corpus they are measuring.

Today the eval MCP can list Qdrant collections and read collection metadata
through ingestion, while the eval service calls chat for retrieval and answer
generation. That separation is correct: eval should measure the same RAG path
users hit. This design adds a pre-run corpus provisioning workflow so an agent
can create a known collection from curated repo fixtures before starting an
experiment.

## Goals

- Let the eval MCP create or refresh eval-specific RAG collections from curated
  document fixtures.
- Keep eval runs reproducible by freezing the selected collection before
  `start_eval_run`.
- Record enough corpus provenance to explain what documents were ingested into
  a collection.
- Prevent broad or accidental vector database mutation through the MCP.
- Reuse the existing ingestion service as the only writer to Qdrant.

## Non-Goals

- Do not let `start_eval_run` upload documents or modify Qdrant.
- Do not add direct Qdrant access to the eval MCP.
- Do not support arbitrary local filesystem upload paths from agent input.
- Do not delete or overwrite production/shared collections such as `documents`
  through convenience defaults.
- Do not build a full corpus management UI.
- Do not redesign chunking, embeddings, hybrid search, or eval scoring.

## Existing System

- `services/ingestion` owns writes to Qdrant through `POST /ingest` and
  collection deletion through `DELETE /collections/{collection_name}`.
- `services/chat` reads Qdrant using `QDRANT_HOST`, `QDRANT_PORT`, and the
  requested collection.
- `services/eval` does not connect to Qdrant. It validates collections through
  ingestion, snapshots chat and ingestion config, then calls chat.
- `go/eval-mcp-service` has read-only ingestion access today:
  `list_rag_collections` and `get_rag_collection_config`.
- Curated product catalog PDFs already exist under `docs/product-catalog/`.

## Recommended Approach

Add a curated, manifest-driven corpus provisioning layer to the eval MCP.

The MCP should expose tools that create named eval collections from repo-known
fixtures. It should not accept arbitrary absolute file paths or upload documents
inside an eval run. The workflow stays explicit:

1. Choose or create an eval dataset.
2. Choose or provision a RAG corpus collection.
3. Inspect the collection and its config.
4. Start baseline and candidate eval runs against that frozen collection.
5. Compare results and record the experiment conclusion.

This gives agents enough autonomy to prepare missing vector data while keeping
comparisons defensible.

## Alternatives Considered

### Inline Ingestion During Eval Run

`start_eval_run` could accept document references and ingest them before
evaluation.

This is rejected because it combines corpus mutation with measurement. Failures
become ambiguous: a regression might come from ingestion, retrieval, reranking,
or generation. It also makes reruns hard to compare if ingestion behavior or
document state changes between candidates.

### Direct Qdrant MCP Tools

The MCP could write to Qdrant directly.

This is rejected because ingestion already owns parsing, chunking, embeddings,
sparse vectors, and collection metadata. Direct Qdrant writes would duplicate
that logic and could create collections chat cannot search correctly.

### Curated Corpus Provisioning

The MCP provisions collections only through ingestion and only from fixture
manifests committed to the repo.

This is the chosen approach. It preserves service ownership, keeps state
auditable, and fits the current eval workflow.

## Corpus Fixtures

Add a small fixture catalog for RAG corpora, separate from eval question
datasets.

Each fixture should define:

- `id`: stable fixture identifier, for example `product_catalog_v1`
- `name`: human-readable name
- `description`: short purpose statement
- `documents`: repo-relative PDF paths
- `expected_collection_prefix`: default prefix for generated collection names
- optional `notes`: caveats about intended eval use

The fixture catalog should compute `source_hash` from the fixture fields and
the bytes of the listed documents. The hash should not be stored as an input
field in the manifest itself.

Initial fixture:

- `product_catalog_v1`
- Documents: the existing product catalog PDFs under `docs/product-catalog/`
- Intended for the current product-spec RAG eval datasets

The fixture catalog should be read-only from the MCP perspective. Adding new
fixtures is a code/doc change reviewed through normal git workflow.

## Collection Naming

Provisioned collections should use a constrained eval namespace:

```text
eval_<fixture_id>_<short_hash>
```

Example:

```text
eval_product_catalog_v1_a1b2c3d4
```

Rules:

- Only collections matching `^eval_[a-z0-9_]+_[a-f0-9]{8,16}$` are managed by
  the new MCP mutation tools.
- Shared collections such as `documents` are never deleted by these tools.
- Re-provisioning the same fixture should be idempotent when the target
  collection already exists with matching manifest metadata.
- If the target collection exists with conflicting metadata, the tool should
  fail and require an explicit `force_recreate` flag.

## MCP Tools

### `list_rag_corpus_fixtures`

Return available curated corpus fixtures.

Output should include fixture id, name, description, document count,
source hash, and default collection name.

### `provision_rag_corpus`

Create a Qdrant collection from a curated fixture through the ingestion API.

Input:

- `fixture`: required fixture id
- `collection`: optional collection override, constrained to the eval namespace
- `force_recreate`: optional boolean, default false

Behavior:

1. Load the fixture manifest.
2. Resolve each document path relative to the repo root.
3. Compute the fixture source hash.
4. Determine the target collection.
5. If the collection exists:
   - return success if metadata matches the same fixture hash
   - fail on mismatch unless `force_recreate` is true
6. If `force_recreate` is true, delete only the managed eval collection.
7. Upload each PDF to ingestion with `POST /ingest?collection=<target>`.
8. Write corpus manifest metadata through ingestion.
9. Return collection name, document count, chunks created, fixture hash, and
   any per-document failures.

Partial upload failure should return an error with enough detail to diagnose
which document failed. A failed provisioning attempt should not automatically
delete the collection, because deletion could remove useful forensic evidence.
Manual retry with `force_recreate` handles cleanup.

### `get_rag_corpus_manifest`

Fetch corpus-level metadata for a managed eval collection.

Output should include fixture id, fixture hash, documents, provisioning time,
ingestion chunk config, embedding model, sparse model, and point count when
available.

### `delete_rag_corpus`

Delete a managed eval corpus collection.

Input:

- `collection`: required

Behavior:

- Reject any collection outside the eval namespace.
- Delete through ingestion, not direct Qdrant.
- Return the deleted collection name.

This tool is mutating and should be described as cleanup-only. It should not be
part of the normal eval run workflow.

## Ingestion API Changes

The existing ingestion API can upload PDFs into named collections, but it only
stores collection-level chunk and model config. Add minimal corpus manifest
support so provisioning is auditable.

Recommended endpoints:

- `PUT /collections/{collection}/manifest`
- `GET /collections/{collection}/manifest`

The manifest should be small JSON metadata stored beside the existing
collection config SQLite database. It should not contain raw document text.

Manifest shape:

```json
{
  "collection": "eval_product_catalog_v1_a1b2c3d4",
  "fixture_id": "product_catalog_v1",
  "fixture_hash": "a1b2c3d4...",
  "documents": [
    {
      "path": "docs/product-catalog/laptop-pro-15-specs.pdf",
      "sha256": "..."
    }
  ],
  "provisioned_at": "2026-05-20T00:00:00Z",
  "provisioned_by": "eval-mcp-service"
}
```

The existing `GET /collections/{collection}/config` should remain focused on
chunking and model config. The manifest endpoint records corpus identity.

## Eval Workflow Changes

The eval MCP prompt and workflow resource should be updated:

- Use `list_rag_corpus_fixtures` and `provision_rag_corpus` when the desired
  collection does not exist.
- Always call `list_rag_collections`, `get_rag_collection_config`, and
  `get_rag_corpus_manifest` before starting an experiment when the collection
  is managed by the new workflow.
- Never infer a collection from a dataset name.
- Never mutate a collection after baseline starts.
- Use the same collection for all candidates in one experiment.

The eval API does not need to change for this feature. It already stores the
selected collection and captured run config. Future work may copy manifest
metadata into the eval run config snapshot, but the first implementation can
keep that provenance accessible through the MCP.

## Safety Model

The MCP can mutate vector data only through ingestion and only for managed eval
collections.

Required guardrails:

- Restrict fixture documents to repo-relative paths from a manifest.
- Reject absolute paths and parent-directory escapes.
- Restrict mutating collection names to the eval namespace.
- Require `force_recreate` for destructive reprovisioning.
- Never delete `documents` or any collection without the eval namespace.
- Use existing auth token behavior for ingestion API calls.
- Keep rate limits in mind: provisioning multiple PDFs may need bounded retry
  or clear failure messages when ingestion returns `429`.

This is repo/runtime-local mutation, not a Kubernetes or production ops action
by itself. If provisioning is run against a shared QA or production ingestion
URL, it becomes shared-environment mutation and should be treated with the same
care as other operator actions.

## Error Handling

Provisioning should fail clearly for:

- unknown fixture id
- missing fixture document
- invalid collection override
- target collection exists with mismatched manifest
- ingestion upload failure
- ingestion manifest write failure
- auth failure
- ingestion rate limiting

Errors should include the fixture id, target collection, and failed document
path when applicable. They should not include secrets or raw PDF contents.

## Testing

Go eval MCP tests:

- fixture catalog lists corpus fixtures
- collection name generation is deterministic
- invalid fixture id fails
- absolute or escaping document paths fail
- provisioning uploads all fixture PDFs to ingestion
- existing matching manifest is idempotent
- existing mismatched manifest fails without `force_recreate`
- `force_recreate` deletes only managed eval collections
- deletion rejects `documents` and non-eval collections
- MCP tool schemas and prompt text include the new workflow

Python ingestion tests:

- manifest can be stored and fetched
- invalid collection names are rejected
- missing manifest returns 404
- manifest write does not change existing ingest behavior

Integration smoke coverage:

- provision the product catalog fixture into an eval collection
- list the collection
- fetch config and manifest
- run a small eval dataset against that collection

## Rollout

Implement in a feature worktree targeting `qa` because it changes application
behavior and may affect deployable service images.

Recommended order:

1. Add ingestion manifest storage and tests.
2. Add eval MCP corpus fixture catalog.
3. Add ingestion client upload, manifest, and delete methods.
4. Add eval workflow methods and MCP tools.
5. Update README and workflow prompt text.
6. Run Go and Python preflights.

## Success Criteria

- An agent can provision `product_catalog_v1` into a deterministic eval
  collection without direct Qdrant access.
- Baseline and candidate eval runs can target that collection without any
  corpus mutation during the runs.
- Corpus provenance is visible through MCP tools.
- Destructive operations are constrained to managed eval collections.
- Existing `documents` collection workflows continue unchanged.
