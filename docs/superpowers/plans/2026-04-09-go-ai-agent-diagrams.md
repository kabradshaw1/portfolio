# Go AI Agent Diagrams Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add three Mermaid diagrams to the /go landing page explaining the AI Shopping Assistant's tool catalog, agent loop, and request flow.

**Architecture:** Single file edit to `frontend/src/app/go/page.tsx`. New `<section>` with three `MermaidDiagram` components inserted after the checkout sequence diagram and before the "View Project" button. Follows the existing prose-then-diagram pattern.

**Tech Stack:** Next.js, TypeScript, MermaidDiagram component (already in use on the page)

---

### Task 1: Add AI Shopping Assistant section with tool catalog diagram

**Files:**
- Modify: `frontend/src/app/go/page.tsx:126` (insert new section before the closing `</section>` tag, above the "View Project" button)

- [ ] **Step 1: Add the AI Shopping Assistant h2, intro paragraph, and tool catalog diagram**

Insert this JSX after line 125 (the closing `</div>` of the checkout diagram) and before line 127 (the "View Project" `<div>`):

```tsx
        <section className="mt-12">
          <h2 className="text-2xl font-semibold">AI Shopping Assistant</h2>
          <p className="mt-4 text-muted-foreground leading-relaxed">
            An LLM-powered shopping assistant that wraps a tool-calling agent
            loop around the ecommerce backend. Users ask natural language
            questions — the agent decides which tools to invoke, calls the
            ecommerce API, and synthesizes a streamed response. Built in Go
            with Ollama (Qwen 2.5 14B).
          </p>

          <h3 className="mt-10 text-xl font-semibold">Tool Catalog</h3>
          <p className="mt-4 text-muted-foreground leading-relaxed">
            The agent has access to nine tools organized into three domains.
            Catalog tools are public; order, cart, and return tools require JWT
            authentication. Checkout is deliberately excluded — the agent can
            advise but not transact.
          </p>
          <div className="mt-6">
            <MermaidDiagram
              chart={`flowchart LR
  AGENT((Agent<br/>Qwen 2.5 14B))
  subgraph Catalog ["Catalog (public)"]
    T1[search_products<br/>query + max_price]
    T2[get_product<br/>full details by ID]
    T3[check_inventory<br/>stock count]
  end
  subgraph Orders ["Orders (auth-scoped)"]
    T4[list_orders<br/>last 20 orders]
    T5[get_order<br/>single order detail]
    T6[summarize_orders<br/>LLM-generated summary]
  end
  subgraph CartReturns ["Cart & Returns (auth-scoped)"]
    T7[view_cart<br/>items + total]
    T8[add_to_cart<br/>product + quantity]
    T9[initiate_return<br/>order item + reason]
  end
  AGENT --> Catalog
  AGENT --> Orders
  AGENT --> CartReturns
  X[place_order<br/>deliberately excluded]:::disabled
  AGENT -.-x X
  classDef disabled stroke-dasharray: 5 5,opacity:0.5`}
            />
          </div>
        </section>
```

- [ ] **Step 2: Verify the page builds**

Run: `cd frontend && npx next build 2>&1 | tail -5`
Expected: Build succeeds with no TypeScript errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/app/go/page.tsx
git commit -m "feat(frontend): add AI Shopping Assistant section with tool catalog diagram"
```

---

### Task 2: Add agent loop flowchart

**Files:**
- Modify: `frontend/src/app/go/page.tsx` (append inside the AI Shopping Assistant section added in Task 1)

- [ ] **Step 1: Add the agent loop heading, intro, and flowchart**

Insert this JSX after the closing `</div>` of the tool catalog `MermaidDiagram`, inside the AI Shopping Assistant `<section>`:

```tsx
          <h3 className="mt-10 text-xl font-semibold">Agent Loop</h3>
          <p className="mt-4 text-muted-foreground leading-relaxed">
            The agent runs a synchronous ReAct-style loop — call the LLM,
            dispatch any requested tools, feed results back into the
            conversation, and repeat until the LLM produces a final answer.
            Bounded by 8 steps and a 30-second wall-clock timeout. Tool errors
            become conversation context for the LLM to handle, not hard
            failures.
          </p>
          <div className="mt-6">
            <MermaidDiagram
              chart={`flowchart TD
  START([Receive user message])
  LLM[Call Ollama<br/>history + tool schemas]
  DECIDE{Tool calls<br/>in response?}
  DISPATCH[Dispatch tool to<br/>ecommerce API]
  APPEND[Append result to<br/>conversation history]
  GUARD{Max 8 steps<br/>or 30s?}
  REFUSAL{Refusal<br/>detected?}
  TAG[Tag outcome as refused]
  STREAM([Stream final answer<br/>via SSE])
  START --> LLM
  LLM --> DECIDE
  DECIDE -->|Yes| DISPATCH
  DISPATCH --> APPEND
  APPEND --> GUARD
  GUARD -->|No| LLM
  GUARD -->|Yes| STREAM
  DECIDE -->|No| REFUSAL
  REFUSAL -->|Yes| TAG
  TAG --> STREAM
  REFUSAL -->|No| STREAM`}
            />
          </div>
```

- [ ] **Step 2: Verify the page builds**

Run: `cd frontend && npx next build 2>&1 | tail -5`
Expected: Build succeeds with no TypeScript errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/app/go/page.tsx
git commit -m "feat(frontend): add agent loop flowchart to /go page"
```

---

### Task 3: Add request flow sequence diagram

**Files:**
- Modify: `frontend/src/app/go/page.tsx` (append inside the AI Shopping Assistant section after the agent loop diagram)

- [ ] **Step 1: Add the request flow heading, intro, and sequence diagram**

Insert this JSX after the closing `</div>` of the agent loop `MermaidDiagram`, still inside the AI Shopping Assistant `<section>`:

```tsx
          <h3 className="mt-10 text-xl font-semibold">
            Request flow: Product search
          </h3>
          <p className="mt-4 text-muted-foreground leading-relaxed">
            A concrete example: the user asks &ldquo;find me a waterproof
            jacket under $150.&rdquo; The frontend streams Server-Sent Events
            from the AI service, which orchestrates between Ollama and the
            ecommerce API.
          </p>
          <div className="mt-6">
            <MermaidDiagram
              chart={`sequenceDiagram
  participant U as User
  participant FE as Frontend
  participant AI as AI Service
  participant OL as Ollama
  participant EC as Ecommerce API
  U->>FE: "find waterproof jackets under $150"
  FE->>AI: POST /chat (SSE stream, Bearer JWT)
  AI->>OL: Chat(messages, tool_schemas)
  OL-->>AI: tool_call: search_products
  AI-->>FE: SSE: tool_call {name, args}
  AI->>EC: GET /products?q=waterproof+jacket&max_price=15000
  EC-->>AI: [{name:"Storm Jacket", price:12999}]
  AI->>OL: Chat(messages + tool_result)
  OL-->>AI: final text
  AI-->>FE: SSE: final {text}
  FE-->>U: "I found 3 waterproof jackets under $150..."`}
            />
          </div>
```

- [ ] **Step 2: Verify the page builds**

Run: `cd frontend && npx next build 2>&1 | tail -5`
Expected: Build succeeds with no TypeScript errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/app/go/page.tsx
git commit -m "feat(frontend): add AI request flow sequence diagram to /go page"
```

---

### Task 4: Run preflight checks

- [ ] **Step 1: Run frontend preflight**

Run: `make preflight-frontend`
Expected: tsc and Next.js build both pass.

- [ ] **Step 2: Run E2E preflight**

Run: `make preflight-e2e`
Expected: All Playwright tests pass.

- [ ] **Step 3: Fix any failures and re-commit if needed**
