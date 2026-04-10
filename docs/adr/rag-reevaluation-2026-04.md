# RAG Reevaluation — 2026-04

**Date:** 2026-04-09
**Status:** Accepted
**Context:** Portfolio AI work, Gen AI Engineer job applications (Go-focused roles)

## Decision

Pivot the portfolio's AI roadmap away from "more RAG" and toward **agents with tool use in a Go microservice**. Keep the existing Python Doc Q&A and Debug Assistant as RAG evidence, but do not invest further in them. Build a new Go service (`go/ai-service`) whose center of gravity is an LLM agent loop orchestrating typed tool calls against real ecommerce backends.

## Why

### 1. RAG is commodity in 2026

Large context windows (1M tokens) have eroded naive RAG's differentiation for small corpora. The "stuff the docs in the prompt" approach beats a naive RAG pipeline on accuracy, dev speed, and complexity for most portfolio-sized use cases. RAG still wins for corpora that don't fit, cost at scale, freshness, and citation — all real — but the portfolio's Doc Q&A already demonstrates RAG competence. Doubling down on more RAG is diminishing returns.

### 2. Agents and tool use are the scarce skill

The shift toward agentic AI (tool calling, MCP, multi-step reasoning) is the single biggest capability change since the portfolio was started. Job postings for AI-focused Go roles increasingly probe for structured output, function calling, and "AI as a system citizen" (evals, observability, cost/latency awareness). These are underrepresented in candidate portfolios relative to RAG demos.

### 3. The portfolio's center of gravity should match the job search

Kyle applies primarily to Go roles. The existing AI work lives in Python, and the previous roadmap's integration plan leaned on the Java stack. A Go hiring manager skimming the repo should see **Go services doing interesting AI-adjacent work**, not Python services reaching into Java. The `go/ai-service` plan puts Go at the center: agent loop, tool registry, HTTP/SSE handler, structured-output validation, evals, caching, metrics — all in Go. Python stays as the model-serving / embedding layer (realistic — nobody writes embedding pipelines in Go).

## Consequences

- The roadmap doc `docs/superpowers/specs/2026-04-07-ai-enhancements-roadmap.md` is superseded by `2026-04-09-go-ai-service-agent-design.md`.
- Tracks A, B-Go, and C from the old roadmap are collapsed into the new Go service. Java Track B integration is dropped.
- The nine-tool catalog that ships with `go/ai-service` deliberately includes `summarize_orders` (a sub-LLM call over structured rows) as the one "LLM over non-doc structured data" capability the RAG pair didn't already show.
- The tool registry is designed so a future MCP adapter can be added without touching any existing code. MCP is the 2026 hype topic; this lets the portfolio pick it up later without rewrite.
- `place_order` / checkout is deliberately **not** a tool. "I drew a boundary here and here's why" is a stronger interview answer than a flashier demo.

## Alternatives considered

- **Keep building more RAG.** Would produce more work, less signal. Rejected.
- **Build an MCP server from scratch as the headline.** Tempting — MCP is the hottest topic. But the Go MCP ecosystem is young and an MCP server could eat the whole project and leave a thin agent behind. The tool registry's interface-based design lets us add MCP as a one-file follow-up once the agent itself is solid.
- **Put the agent work in Python.** Faster to build (existing FastAPI scaffolding, mature libraries), but mismatches the job search and loses the "Go + LLM" talking point that's the scarcest part of the pitch.

## Evidence this was right

- `go/ai-service` ships nine tools, JWT auth, a cache, metrics, guardrails, an eval harness, and a frontend drawer — all in Go, all testable offline, all deployed to Minikube.
- The existing Python Doc Q&A and Debug Assistant still demonstrate RAG.
- The tool registry lives behind a single-file interface that an MCP adapter can implement without changes to any consumer.
