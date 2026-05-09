# Hybrid Search With BM25 And Semantic Vectors

- **Date:** 2026-05-09
- **Status:** Accepted
- **Related PR:** #254
- **Spec:** [docs/superpowers/specs/2026-05-08-hybrid-search-design.md](../../superpowers/specs/2026-05-08-hybrid-search-design.md)
- **Plan:** [docs/superpowers/plans/2026-05-08-hybrid-search.md](../../superpowers/plans/2026-05-08-hybrid-search.md)

## Context

The document QA pipeline used dense semantic retrieval only. That works for
conceptual questions, but it can miss exact identifiers such as RFC numbers,
section numbers, product IDs, and short domain terms. Those misses are visible
in evaluation because context precision and recall depend on the retriever
finding the right chunks before the LLM answers.

The system already used Qdrant for vector storage, and the evaluation service
already captured RAG configuration snapshots. The decision needed to improve
retrieval quality without adding another search service or breaking existing
dense-only collections.

## Decision

Use Qdrant-native hybrid retrieval for newly ingested RAG collections:

- Ingestion stores two named vectors per chunk:
  - `dense` for the existing semantic embedding.
  - `sparse` for a BM25 sparse vector generated with FastEmbed.
- Chat defaults to hybrid retrieval with Qdrant Query API prefetches and RRF
  fusion.
- Semantic retrieval remains available and uses the named `dense` vector for
  new collections.
- Legacy dense-only collections continue to work through fallback to the
  unnamed vector.
- `/chat`, `/search`, and `/config` expose retrieval metadata so eval runs can
  compare semantic-only and hybrid behavior.
- Eval config capture preserves the full chat config, including retrieval mode,
  vector names, sparse model, prefetch limit, and fusion strategy.

The implementation pins Qdrant and `qdrant-client[fastembed]` to compatible
versions instead of relying on `latest` images or an older client API.

## Consequences

**Positive:**

- Exact-match queries have a keyword signal in addition to dense semantic
  similarity.
- Qdrant remains the only retrieval store; no Elasticsearch/OpenSearch service
  is needed for BM25.
- Hybrid behavior is measurable through existing eval runs because retrieval
  metadata is captured with each run.
- Existing `/chat` and `/search` clients remain compatible; retrieval metadata
  is additive.
- Existing dense-only collections can still answer while new collections use
  named dense and sparse vectors.

**Trade-offs:**

- Sparse vector generation adds CPU work and model dependencies to ingestion
  and query-time chat retrieval.
- New collections use named vectors, so fallback logic must distinguish named
  dense semantic fallback from legacy unnamed vector fallback.
- The sparse model and Qdrant client versions are now part of the retrieval
  contract and must stay aligned across ingestion, chat, debug, and shared
  Python packages.
- There is no migration or backfill for existing collections; they become
  hybrid-capable only after explicit reingestion.

**Future work:**

- Add an evaluation comparison run that demonstrates context precision and
  recall changes between semantic-only and hybrid retrieval.
- Consider a backfill job if preserving existing collection content becomes
  more important than keeping the current scope small.
- Promote ingestion collection metadata from ephemeral SQLite storage if long
  term collection config history becomes required.
