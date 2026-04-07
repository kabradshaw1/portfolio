# AI Enhancements Roadmap — Ecommerce & Task Manager

**Date:** 2026-04-07
**Status:** Roadmap (index of sub-projects to brainstorm individually)

## Purpose

Extend the existing Ollama + Qdrant + RAG stack into the Go ecommerce and Java task-management services so the portfolio demonstrates AI as a production system concern — not just a standalone demo page. Each track below is its own sub-project with its own design spec and implementation plan.

## Guiding Principle

Each track should surface a capability that isn't already shown by the Document Q&A or Debug Assistant: structured output, event-driven AI, operational maturity, or product-level AI UX.

## Track A — New AI Techniques (domain expansion)

Show AI over non-document data and structured output — techniques not yet in the portfolio.

Candidate features:
- **Semantic product search** (ecommerce): embed product name/description with `nomic-embed-text`, store in Qdrant, natural-language query → ranked products.
- **Natural-language task creation** (tasks): free-text input → LLM returns validated JSON (`{title, assignee, due_date, tags}`) via Qwen JSON mode → existing GraphQL mutation.
- **Order / activity summarization**: LLM over structured rows ("summarize my last 10 orders", "what did I ship this week").

Skills demonstrated: embeddings outside docs, JSON mode / function calling, prompt engineering for extraction, schema validation.

## Track B — Cross-Stack Event-Driven Integration

Put Python AI services on the same RabbitMQ bus as the Go and Java stacks so AI participates in the real architecture, not a side page.

Shape:
- New Python FastAPI + `aio-pika` service (e.g. `services/ai-worker/`) deployed to the `ai-services` namespace.
- Consumes domain events (`OrderPlaced`, `TaskCreated`, etc.) from existing RabbitMQ exchanges.
- Writes enrichment back via Redis, a callback REST endpoint, or a GraphQL mutation.

Candidate features:
- `OrderPlaced` → generate personalized thank-you blurb + "you might also like" list, cache in Redis keyed by order id.
- `TaskCreated` → auto-suggest subtasks/tags, push back via GraphQL mutation on task-service.

Skills demonstrated: polyglot event-driven architecture, async (non-blocking) AI processing, realistic microservice boundaries, K8s deployment of a new Python service.

## Track C — Production-Flavored AI Operations

Show that AI is treated as a system with latency, cost, reliability, and evaluation concerns — not just a working prompt.

Menu (pick the ones that give the best signal):
- **Embedding cache in Redis** — `hash(text) → vector`, avoid re-hitting Ollama for repeat inputs.
- **Streaming everywhere** — SSE for Track A/B user-visible AI output.
- **Eval harness in CI** — pytest suite running a fixed prompt set against the LLM, asserting JSON shape, keyword presence, or embedding similarity to golden answers. Runs in `ci.yml`.
- **Guardrails** — input length limits, PII scrub, refusal detection, timeout fallback.
- **Observability** — Prometheus metrics for tokens, latency p95, cache hit rate; panels on existing Grafana dashboard.

Skills demonstrated: AI-as-system thinking, eval-driven development, operational maturity.

## Track D — Portfolio UX Polish (follow-up)

Once A/B/C land, make the AI features visible to a reviewer within seconds of landing on `/go/ecommerce` or `/java/tasks`. Deliberately deferred — cosmetic layer on top of the real features.

Candidate surfaces:
- Ecommerce AI search bar (Track A semantic search) at top of product list, with streaming rerank.
- "Explain this product" button on product cards with persona toggle.
- Tasks "Quick add" input streaming parsed fields into the form (Track A natural-language task creation).
- Tasks weekly digest card generated from activity-service data.

## Suggested Sequence

1. **Track A — Natural-language task creation** (smallest, highest-signal, reuses Java GraphQL).
2. **Track A — Semantic product search** (adds embeddings-over-non-docs; reuses Qdrant).
3. **Track B — `ai-worker` service + first consumer** (e.g. `TaskCreated` enrichment).
4. **Track C — Eval harness + embedding cache + Grafana panels** (retrofits onto A and B).
5. **Track D — UX polish** across ecommerce and tasks pages.

Each step above becomes its own `YYYY-MM-DD-<topic>-design.md` spec via the brainstorming skill, then its own implementation plan.

## Open Questions (to resolve per-track during brainstorming)

- Which Ollama model handles JSON mode best for Track A — Qwen 2.5 14B or a smaller dedicated extractor?
- For Track B, do we introduce a new exchange or reuse existing ones? What's the contract for AI-generated enrichment writes?
- For Track C evals, what's the golden-answer source and how do we keep it from being flaky in CI?
- How do we keep the `ai-worker` off the critical path so Ollama latency never blocks order placement or task creation?
