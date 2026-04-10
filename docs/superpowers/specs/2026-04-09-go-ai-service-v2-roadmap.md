# `go/ai-service` v2 — Proposed Next Steps

**Date:** 2026-04-09
**Status:** Roadmap (not yet a spec — read first, then promote items into individual specs via the brainstorming skill when you're ready to build)
**Context:** v1 (Plans 1–5) shipped a working Go agent service with nine tools, JWT auth, cache, metrics, guardrails, an eval harness, a frontend drawer, K8s deployment, and CI. This document captures the work that was deliberately deferred from v1 and ranks it by portfolio leverage, not by effort.

## The honest framing

The most interesting v2 isn't "more tools" — v1 already has nine. It's the things deferred from v1 because they'd dilute the core agent story. The items below are grouped by what they actually add to the portfolio pitch, not by code complexity.

---

## Tier 1 — Highest portfolio leverage

### 1. MCP adapter (and your own MCP server)

The whole `Tool` interface in `internal/tools/registry.go` was designed for this. v2 ships an `mcp.ClientTool` that proxies `Call` to an MCP server over stdio/HTTP and pulls schemas from `tools/list`. Bonus: also stand up your own MCP **server** that exposes the existing nine tools, so the same registry is reachable from Claude Desktop, Cursor, or any MCP host.

- **Why this matters:** As of 2026 this is the single hottest thing nobody's portfolio has. The pitch becomes: "I built a Go agent with a tool registry that's transport-agnostic, and here's both an MCP client (so my agent can call any MCP server) and an MCP server (so any MCP host can call my tools)."
- **Cost:** ~2 days. Main risk is the Go MCP ecosystem still being young.
- **Pre-requisite:** none. v1's `Tool` interface is the only thing it depends on.

### 2. Hosted-model `llm.Client`

One file: an Anthropic or OpenAI implementation of the existing `llm.Client` interface. Add a `--model anthropic` flag (or `LLM_BACKEND` env var) in `main.go`. Lets you demo the same agent against Claude 4.6 or GPT-4 in an interview, side by side with the local Ollama version.

- **Why this matters:** Demonstrates that the abstraction was the point, not the model. Also unlocks dollar-cost tracking for free (see Tier 4 below).
- **Cost:** ~50 LOC, half a day.
- **Pre-requisite:** none.

### 3. Real-LLM nightly eval tier

The mocked-LLM eval tier already exists at `go/ai-service/internal/evals/` (build-tagged `eval`). v2 adds a scheduled GHA workflow that SSH's to the Windows PC and runs `go test -tags=eval,realllm` against actual Ollama. The "soft" tier checks final-answer embedding similarity to golden answers and warns instead of failing.

- **Why this matters:** Turns "I have an eval harness" into "I have eval data over time." Pass rate as a graph beats a static test count in interviews.
- **Cost:** ~1 day, mostly GHA YAML and SSH plumbing.
- **Pre-requisite:** none. Uses the existing eval harness.

---

## Tier 2 — Real production wins

### 4. Semantic product search via Qdrant

The current `search_products` tool is text search through `ecommerce-service` `GET /products?q=...`. v2 ingests product `name + description` into Qdrant (the existing instance in the `ai-services` namespace), embeds queries via `nomic-embed-text`, and replaces the catalog tool's backend.

- **Why this matters:** The embedding cache I scoped out of v1 finally has a job — it lives in the `cache.Cache` interface I already shipped. This is also where you regain "one canonical RAG pipeline that lives in Go", since semantic search over product rows is RAG-shaped.
- **Cost:** ~1–2 days. Includes a small ingestion job and the Go Qdrant client.
- **Pre-requisite:** none, but pairs naturally with #2 if you want to compare hosted-model embeddings vs. `nomic-embed-text`.

### 5. Streaming intermediate LLM tokens

Right now v1 only streams structured events; the final answer arrives all at once. v2 makes the Ollama client stream token-by-token, and the `final` SSE event becomes a sequence of `final_chunk` events on the wire.

- **Why this matters:** That's the difference between "request/response" and "real-time AI" in a reviewer's gut. Same cost as the change is small but the perceived demo polish doubles.
- **Cost:** Half a day. Frontend changes maybe 20 lines.
- **Pre-requisite:** none.

### 6. Conversation persistence + sessions

Currently each turn is stateless and the frontend holds the history. v2 adds a `sessions` table (Postgres in `go-ecommerce`), keys by user_id, lets the user resume conversations, and lets you do "long-running agent sessions" later.

- **Why this matters:** Required prerequisite for any kind of memory. Also fixes the "refresh nukes the conversation" rough edge in the demo.
- **Cost:** ~1 day. Migration + repo + small handler changes.
- **Pre-requisite:** none.

---

## Tier 3 — Rounds out the pitch

### 7. Parallel tool dispatch

When the model returns multiple tool calls in one step, dispatch them with a goroutine fan-out instead of sequentially. The interview value isn't latency — it's *"I added concurrency once an eval showed it mattered"* as a story.

- **Cost:** ~half a day, careful test additions to verify ordering invariants.
- **Pre-requisite:** worth waiting until the eval harness shows multi-call steps actually happen.

### 8. Conversation compaction

When history exceeds N tokens, summarize the oldest M messages via a sub-LLM call (reuses the `summarize_orders` pattern) and replace them with the summary.

- **Why this matters:** Necessary the moment anyone has a 50-turn conversation. Also avoids the brittle 20-message hard cap from v1's guardrails.
- **Cost:** ~half a day.
- **Pre-requisite:** #6 (sessions) makes the long histories actually exist.

### 9. Rich product cards in the drawer

The drawer currently renders tool `display` payloads as pretty-printed JSON. v2 has a small `<DisplayRenderer kind="...">` switch that turns `{"kind": "product_list", "products": [...]}` into actual product cards with images and click-through.

- **Why this matters:** Pure frontend; no backend touched. Doubles the demo polish.
- **Cost:** ~1 day.
- **Pre-requisite:** none.

### 10. Prompt & system-prompt versioning

Store the current system prompt in a YAML file under `internal/agent/prompts/`, version it, log the version on every turn alongside `turn_id`. Tiny change but makes "I A/B'd this prompt and have data" possible later.

- **Cost:** ~half a day.
- **Pre-requisite:** none. Foundation for prompt eval comparisons that nobody actually does.

---

## Tier 4 — Probably skip unless asked

These all sound impressive but cost more than they pay back for a portfolio:

- **Dollar-cost tracking** — only meaningful with a hosted model. The moment you do #2, it falls out for free, so it's not really a separate item.
- **PII scrub / jailbreak detection / content moderation** — these are full projects on their own and dilute the agent story. Defer to a separate `ai-safety` ADR rather than building them.
- **Multi-agent orchestration / supervisor patterns** — trendy in 2025, but the interview signal is weak unless you can show a use case that genuinely needs it. The shopping + concierge agent doesn't.
- **GraphQL subscription transport for `/chat`** — replacing SSE with GraphQL subscriptions adds zero capability and a lot of complexity. Skip.
- **Per-user cost / quota enforcement** — only matters if real users start hitting it. The rate limiter is enough until then.

---

## What I'd actually build first

Strict ordering by ROI:

1. **#2 hosted-model client** — half a day, unlocks the "two LLMs, one agent" demo immediately
2. **#1 MCP adapter (+ MCP server)** — ~2 days, single biggest portfolio differentiator in 2026
3. **#4 semantic Qdrant search** — 1–2 days, gives you a credible "RAG inside the agent" story and finally puts the embedding cache to work
4. **#5 token streaming** — half a day, biggest demo-feel upgrade for the smallest change

After those four, the pitch becomes: **"Go agent + tool use + MCP + hosted-model swap + semantic search + token streaming."** That's roughly state of the art for a 2026 portfolio in this space. Everything else (sessions, compaction, parallelism, rich cards) is incremental polish.

---

## One meta-point

A v2 ADR documenting the decisions you *didn't* make is worth more than any individual feature. "I considered MCP and shipped it. I considered multi-agent and rejected it. Here's why" is the kind of judgment a hiring manager actually wants to see. The standalone `docs/adr/rag-reevaluation-2026-04.md` from v1 is the model for this — keep doing one of those per major decision point.

---

## How to use this doc

When you're ready to build any of these, run the brainstorming skill against the item:

> "I want to brainstorm v2 item #1 (MCP adapter) from `docs/superpowers/specs/2026-04-09-go-ai-service-v2-roadmap.md`"

That'll produce a per-item spec, then a per-item plan, same flow as v1. Don't try to build multiple items in one branch — each item is small enough on its own that they shouldn't be batched.
