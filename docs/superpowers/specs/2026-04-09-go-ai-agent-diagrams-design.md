# Go AI Agent Diagrams — UI Design Spec

## Summary

Add a new "AI Shopping Assistant" section to the `/go` landing page (`frontend/src/app/go/page.tsx`) with three Mermaid diagrams explaining the AI agent's architecture, loop mechanics, and request flow. Positioned as a new `<h2>` section after the existing checkout sequence diagram and before the "View Project" button.

## Goals

- Help portfolio reviewers understand the AI agent system at a glance
- Demonstrate architectural depth: tool-calling agent loop, auth boundaries, deliberate design constraints
- Maintain visual consistency with the existing page (same `MermaidDiagram` component, same prose-then-diagram pattern)

## Approach

Pure Mermaid — all three diagrams use the existing `MermaidDiagram` component. No new components needed.

## Page Structure (after changes)

```
h1: Go Backend Developer
  Bio section (existing)

h2: Ecommerce Platform (existing)
  Tech stack list (existing)

h2: Architecture (existing)
  Ecommerce architecture flowchart (existing)
  h3: Request flow: Checkout order (existing)
    Checkout sequence diagram (existing)

h2: AI Shopping Assistant (NEW)
  Intro paragraph
  h3: Tool Catalog (NEW)
    Intro paragraph + Mermaid flowchart with subgraphs
  h3: Agent Loop (NEW)
    Intro paragraph + Mermaid flowchart
  h3: Request Flow: Product Search (NEW)
    Intro paragraph + Mermaid sequence diagram

"View Project" button (existing, stays at bottom)
```

## Section Details

### H2 Intro

> An LLM-powered shopping assistant that wraps a tool-calling agent loop around the ecommerce backend. Users ask natural language questions — the agent decides which tools to invoke, calls the ecommerce API, and synthesizes a streamed response. Built in Go with Ollama (Qwen 2.5 14B).

### H3: Tool Catalog

**Intro text:**
> The agent has access to nine tools organized into three domains. Catalog tools are public; order, cart, and return tools require JWT authentication. Checkout is deliberately excluded — the agent can advise but not transact.

**Diagram:** Mermaid `flowchart LR` with subgraphs:

- **Catalog (public):** search_products, get_product, check_inventory
- **Orders (auth-scoped):** list_orders, get_order, summarize_orders
- **Cart & Returns (auth-scoped):** view_cart, add_to_cart, initiate_return
- Central `Agent` node connecting to all three subgraphs
- A `place_order` node with a dashed/dotted border or `:::disabled` class style, labeled "deliberately excluded" to highlight the design boundary

### H3: Agent Loop

**Intro text:**
> The agent runs a synchronous ReAct-style loop — call the LLM, dispatch any requested tools, feed results back into the conversation, and repeat until the LLM produces a final answer. Bounded by 8 steps and a 30-second wall-clock timeout. Tool errors become conversation context for the LLM to handle, not hard failures.

**Diagram:** Mermaid `flowchart TD`:

```
Receive user message
  → Call Ollama (history + tool schemas)
  → Decision: tool calls in response?
    → Yes: Dispatch tool to ecommerce API
      → Append result to conversation history
      → Loop back (max 8 steps / 30s)
    → No: Check for refusal
      → Stream final answer via SSE
```

### H3: Request Flow: Product Search

**Intro text:**
> A concrete example: the user asks "find me a waterproof jacket under $150." The frontend streams Server-Sent Events from the AI service, which orchestrates between Ollama and the ecommerce API.

**Diagram:** Mermaid `sequenceDiagram` with participants:

```
User → Frontend: "find waterproof jackets under $150"
Frontend → AI Service: POST /chat (SSE stream, Bearer JWT)
AI Service → Ollama: Chat(messages, tool_schemas)
Ollama → AI Service: tool_call: search_products({query, max_price})
AI Service → Frontend: SSE: tool_call {name, args}
AI Service → Ecommerce: GET /products?q=waterproof+jacket&max_price=15000
Ecommerce → AI Service: [{name:"Storm Jacket", price:12999, ...}]
AI Service → Ollama: Chat(messages + tool_result)
Ollama → AI Service: final text
AI Service → Frontend: SSE: final {text}
Frontend → User: "I found 3 waterproof jackets under $150..."
```

## File Changes

| File | Change |
|------|--------|
| `frontend/src/app/go/page.tsx` | Add new AI Shopping Assistant section with 3 MermaidDiagram components |

## Out of Scope

- No new React components
- No changes to the AI service backend
- No changes to the ecommerce pages or AI chat drawer
- No interactive elements (these are static explanatory diagrams)

## Preflight

- `make preflight-frontend` (tsc + Next.js build)
- `make preflight-e2e` (Playwright tests)
