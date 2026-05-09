# Hybrid Search Design

## Context

Issue #80 adds keyword matching alongside semantic vector search for the RAG
pipeline. The goal is to catch exact-match queries such as RFC numbers, section
numbers, product IDs, and other terms that dense embeddings can miss.

Phase 4a is already available under `services/eval/`, so this work should use
the existing evaluation harness to compare semantic-only retrieval with hybrid
retrieval.

Current state:

- `services/ingestion` chunks PDFs, embeds chunk text, and writes one dense
  vector per point to Qdrant.
- `services/chat` embeds each query and uses Qdrant search for semantic
  retrieval.
- `services/eval` calls chat `/search` and `/chat`, then scores the run.
- Existing Qdrant collections are dense-only and should continue to work.

## Goals

- Store dense and BM25 sparse vectors for newly ingested RAG collections.
- Use native Qdrant hybrid search with reciprocal rank fusion for default
  retrieval.
- Preserve existing `/chat` and `/search` callers.
- Fall back to semantic-only retrieval for legacy dense-only collections.
- Capture enough metadata for eval runs to compare semantic-only and hybrid
  behavior.

## Non-Goals

- No migration or backfill job for existing collections.
- No mutation of QA or production Qdrant data during implementation.
- No UI redesign or new evaluation UI work.
- No separate keyword index outside Qdrant.

## Selected Approach

Use Qdrant native hybrid search with client-side BM25 sparse vector generation.

Ingestion will create new collections with two named vectors per point:

- `dense`: the existing 768-dimensional embedding from `nomic-embed-text`.
- `sparse`: a BM25 sparse vector generated from the same chunk text.

Chat will query both vectors with Qdrant's Query API and fuse candidates with
reciprocal rank fusion. Qdrant remains the single retrieval store, which avoids
adding a second BM25 service or local index.

The implementation must pin compatible Qdrant and `qdrant-client` versions.
Qdrant's hybrid Query API is available in Qdrant 1.10 and newer, and the repo
currently pins `qdrant-client==1.9.0`, so this feature requires an explicit
client upgrade and a non-`latest` Qdrant image.

## Architecture

### Ingestion

Add a small sparse-vector module in `services/ingestion` that converts chunk
text into BM25 sparse vectors. The module should hide the concrete FastEmbed or
Qdrant-client API so store code only receives normalized sparse-vector data.

Update `QdrantStore._ensure_collection()` to create hybrid-capable collections
for new collections:

- named dense vector config for the existing 768-dimensional cosine vector
- named sparse vector config for BM25 sparse vectors

Update `QdrantStore.upsert()` to write each point with both named vectors and
the existing payload fields:

- `text`
- `page_number`
- `chunk_index`
- `document_id`
- `filename`

If sparse vector generation fails during ingest, ingestion should fail before
writing points. Partial hybrid collection writes make later retrieval behavior
hard to reason about.

Collection metadata should record hybrid capability and sparse model/config
alongside existing chunk and embedding metadata. This allows eval snapshots to
explain how a collection was indexed.

### Chat

Replace the single-vector retriever path with a retriever that supports two
modes:

- `semantic`: dense-vector search only, using named `dense` vectors for new
  collections and the legacy unnamed vector for old collections
- `hybrid`: dense and sparse prefetches fused by Qdrant RRF

`hybrid` is the default for new collections. For legacy dense-only collections,
chat should detect unsupported collection shape or Qdrant errors that indicate
missing named vectors, log the reason, and retry semantic-only retrieval.

The public `/chat` and `/search` request shapes remain compatible. Existing
clients do not need to pass a new field. `/search` must return top-level
retrieval metadata. `/chat` should include the same metadata in JSON responses
and in the final SSE `done` event. Metadata fields are:

- `retrieval_mode`: `hybrid` or `semantic`
- `retrieval_fallback`: boolean
- `fusion`: `rrf` when hybrid is used

`/config` should include retrieval defaults and hybrid settings so eval runs
can snapshot the active behavior.

### Evaluation

Use the Phase 4a evaluation harness in `services/eval`.

The evaluation service should be able to compare:

- semantic-only baseline
- hybrid retrieval using the same dataset and equivalent collection content

Eval config capture should include retrieval mode, fusion strategy, sparse
model/config, and whether a run used fallback semantic retrieval. Context
precision and context recall are the primary metrics for demonstrating
retrieval improvement.

## Error Handling

- If a collection lacks named dense or sparse vectors, chat falls back to
  semantic-only retrieval and marks the response metadata.
- If query-time sparse vector generation fails and semantic retrieval is
  possible, chat falls back to semantic-only retrieval.
- If query-time sparse vector generation fails and semantic fallback is not
  possible, chat returns a controlled `503`.
- If Qdrant is unavailable, existing service-unavailable behavior is preserved.
- If ingestion cannot produce sparse vectors, ingestion returns an error before
  upserting points.

## Compatibility

- Existing `/chat` and `/search` callers keep working.
- Existing dense-only collections keep working through semantic fallback.
- Freshly ingested collections are hybrid-capable.
- Existing collections become hybrid-capable only after explicit reingestion or
  a future backfill/migration issue.
- Docker Compose and Kubernetes Qdrant images should be pinned to a compatible
  version instead of `qdrant/qdrant:latest`.

## Testing

Unit tests should cover:

- ingestion creates hybrid-capable collection configs
- ingestion upserts points with both named vectors and existing payload fields
- sparse-vector conversion handles empty and normal text batches
- chat hybrid search calls Qdrant Query API with dense and sparse prefetches
  and RRF fusion
- chat semantic fallback works for legacy dense-only collections
- `/search` remains backward compatible while returning retrieval metadata
- `/chat` JSON and SSE responses expose retrieval metadata without changing the
  existing answer/token/source fields
- eval config capture includes retrieval mode and hybrid settings

Verification before commit should include:

- `make preflight-python`
- `make preflight-security`

A local compose smoke test against a fresh Qdrant collection is useful if local
resources allow it, but the core acceptance gate is covered by focused tests and
the Python/security preflights.

## Acceptance Criteria

- Freshly ingested RAG collections store dense and sparse vectors for every
  chunk.
- Hybrid retrieval is the default for compatible collections.
- Legacy dense-only collections still answer through semantic fallback.
- `/chat` and `/search` remain compatible with existing callers.
- Eval runs can compare semantic-only and hybrid retrieval and capture the
  retrieval configuration used.
- Relevant Python and security preflights pass, or any blocker is documented.
