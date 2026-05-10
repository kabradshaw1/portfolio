# Cross-Encoder Re-Ranking For Document QA

- **Date:** 2026-05-10
- **Status:** Accepted

## Context

The document QA chat service used Qdrant hybrid retrieval as its final ranking
stage. That kept retrieval fast and operationally simple, but it also meant the
answer pipeline could only rank candidates by vector and sparse-retrieval
signals. For semantically close chunks, that is often weaker than scoring the
query and candidate text together.

The portfolio goal is production-grade engineering rather than a demo, so the
re-ranking design had to satisfy several constraints:

- keep existing `/search`, `/chat`, and evaluation behavior unchanged by
  default
- avoid paid cloud dependencies
- preserve Qdrant hybrid retrieval as the candidate generator
- expose enough metadata and metrics to debug fallback behavior
- keep RAG evaluation runs able to compare baseline retrieval against re-ranked
  retrieval
- keep Python security auditing explicit about any accepted dependency risk

The original implementation plan expected `sentence-transformers==5.4.1`, but
the available package index resolved only up to `5.1.2` at implementation time.
That version pulled `transformers==4.57.6`, which `pip-audit` reports as
`CVE-2026-1839` with a fix listed in the `5.0.0rc3` pre-release line.

## Decision

Add opt-in in-process cross-encoder re-ranking to the Python chat service using
`sentence-transformers==5.1.2` and the
`cross-encoder/ms-marco-MiniLM-L6-v2` model.

The chat service continues to retrieve candidates from Qdrant first. When a
request sets `rerank=true`, it retrieves a larger candidate pool, scores
`(query, chunk_text)` pairs with a lazily loaded local `CrossEncoder`, and
returns the highest cross-encoder scores in stable order. If re-ranking fails,
the service records fallback metadata and returns the original retrieval order
instead of failing the user request.

The feature is disabled by default:

- `/search` accepts `rerank`, defaulting to `false`.
- `/chat` accepts `rerank`, defaulting to `false`.
- `/config` exposes the active re-ranker settings.
- `services/eval` accepts `rerank` on evaluation start requests and threads it
  through `RAGClient.search()` and `RAGClient.ask()` so evaluation runs can
  compare baseline and re-ranked retrieval.

Operational instrumentation was added for re-ranking duration, candidate count,
and fallback count. Configuration validation rejects candidate limits that are
below the final retrieval `top_k`.

`pip-audit` ignores only `CVE-2026-1839` for the chat service. This is a narrow
exception for the approved `sentence-transformers` transitive dependency, not a
blanket audit suppression. Local `make preflight-security` preserves
`pip-audit` failure status after temporary-environment cleanup so unapproved
future advisories still fail before push.

## Consequences

**Positive:**

- Re-ranking can improve the final context order for ambiguous or semantically
  close chunks without replacing the existing hybrid candidate generator.
- Default behavior remains unchanged for current API consumers and evaluation
  runs.
- The local model avoids paid external ranking APIs and keeps the architecture
  reproducible in Docker Compose and Kubernetes.
- Eval request threading makes retrieval changes measurable through the
  existing first-party RAG evaluation service.
- Metrics and fallback metadata make model-load or scoring failures observable
  without turning them into user-visible outages.
- CI and local preflight now document and apply the same narrow security
  exception for the accepted transitive `transformers` advisory.

**Trade-offs:**

- Re-ranking adds CPU and memory cost on requests that opt in.
- The first request using a given model/device pays model load latency.
- `sentence-transformers` brings a large ML dependency tree into the chat
  service, including `torch` and `transformers`.
- The accepted `transformers==4.57.6` advisory must be revisited when a stable
  non-vulnerable dependency path is available.
- Local and CI security tooling must keep the chat-only `CVE-2026-1839`
  suppression in sync until the dependency graph can be upgraded.

**Future work:**

- Revisit the dependency pin when `sentence-transformers` supports a stable
  `transformers` version with a fixed advisory.
- Add evaluation reports that compare the same dataset with `rerank=false` and
  `rerank=true`.
- Consider a service-level model warmup only if cold-start latency becomes a
  real operational issue.
