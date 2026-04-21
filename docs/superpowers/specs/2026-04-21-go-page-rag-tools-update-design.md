# Go Page AI Assistant Section — RAG Tools Update

## Context

The Go ai-service recently gained 3 RAG tools (`search_documents`, `ask_document`, `list_collections`) that bridge to the Python chat/ingestion services and Qdrant vector DB. The frontend `AiAssistantDrawer` already renders RAG results (green-bordered cards with source citations), but the `/go` portfolio page's AI Shopping Assistant section still only documents the original 9 ecommerce tools. This update brings the page content in sync with the actual 12-tool agent.

## Approach

Expand existing content in-place (Approach A) — add a 4th tool domain to the catalog diagram, tweak the agent loop, and add a second request flow. No new components, no structural changes to the page layout.

## Changes

All edits are in `frontend/src/app/go/page.tsx`, within the `{/* AI Shopping Assistant — standalone section */}` block (lines 502–611).

### 1. Update intro paragraph (lines 505–511)

Current text describes the agent as wrapping "the ecommerce backend" with no mention of RAG.

Replace with text that mentions both the ecommerce backend and RAG knowledge base, the cross-stack bridge (Go → Python chat service → Qdrant), circuit breakers, and OTel trace propagation across the stack boundary.

### 2. Update Tool Catalog description + diagram (lines 513–546)

**Description text:**
- "nine tools" → "twelve tools"
- "three domains" → "four domains"
- Add sentence: Knowledge Base tools are public and hit the Python RAG pipeline via a circuit-breaker HTTP bridge with 30-second timeout.

**Mermaid flowchart:** Add a 4th subgraph `Knowledge ["Knowledge Base (public, RAG)"]` containing:
- `search_documents` — semantic search, ranked chunks
- `ask_document` — natural-language Q&A with sources
- `list_collections` — list vector store collections

Add `AGENT --> Knowledge` edge. Keep the existing `place_order` excluded node.

### 3. Update Agent Loop diagram (lines 548–579)

Two label changes in the existing flowchart:
- `DISPATCH` box: "Dispatch tool to ecommerce API" → "Dispatch tool to ecommerce API or RAG pipeline"
- `GUARD` box: "Max 8 steps or 30s?" → "Max 8 steps or 90s?" (matches actual `agent.go` config)

No structural changes to the flowchart.

### 4. Add second request flow — RAG query (after line 610)

New `<h3>` + `<p>` + `<MermaidDiagram>` block titled "Request flow: Product knowledge query".

Example scenario: user asks "What's the warranty on the Storm Jacket?"

Sequence diagram participants:
- User, Frontend, AI Service, Ollama, Python Chat Svc, Qdrant

Flow:
1. User → FE: natural language question
2. FE → AI Service: POST /chat (SSE stream, Bearer JWT)
3. AI Service → Ollama: Chat(messages, tool_schemas)
4. Ollama → AI Service: tool_call: ask_document
5. AI Service → FE: SSE: tool_call event
6. AI Service → Python Chat Svc: POST /chat {question, collection}
7. Python Chat Svc → Qdrant: vector search
8. Qdrant → Python Chat Svc: ranked chunks
9. Python Chat Svc → Ollama: RAG prompt + context
10. Ollama → Python Chat Svc: generated answer
11. Python Chat Svc → AI Service: {answer, sources}
12. AI Service → Ollama: Chat(messages + tool_result)
13. Ollama → AI Service: final text
14. AI Service → FE: SSE: final event
15. FE → User: rendered answer with source citations

## Files Modified

- `frontend/src/app/go/page.tsx` — all 4 changes above

## Verification

1. `npx tsc --noEmit` — type check passes
2. `npm run lint` — no lint errors
3. Visual check — both tabs still render, all 4 Mermaid diagrams load, Knowledge Base subgraph visible in tool catalog, RAG sequence diagram renders below ecommerce one
4. `make preflight-frontend` — full frontend preflight passes
