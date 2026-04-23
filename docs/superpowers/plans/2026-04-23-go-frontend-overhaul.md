# Go Frontend Overhaul Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restructure the /go page into 5 tabs, add error boundaries, rework HealthGate for graceful degradation, clean up analytics, and expand seed data.

**Architecture:** Extract each tab's content into standalone components under `frontend/src/components/go/tabs/`. Add Next.js `error.tsx` boundaries at route segments. Convert HealthGate from blocking to banner mode. Fix seed.sql idempotency and expand catalog.

**Tech Stack:** Next.js, TypeScript, React, Mermaid diagrams, PostgreSQL seed SQL

---

### Task 1: Extract Microservices Tab Component

**Files:**
- Create: `frontend/src/components/go/tabs/MicroservicesTab.tsx`
- Modify: `frontend/src/app/go/page.tsx`

- [ ] **Step 1: Create MicroservicesTab.tsx with updated content**

Cut the entire `{activeTab === "microservices" && (...)}` block from `page.tsx` (lines 67-380) into a new component. Add the origin story paragraph at the top before "Why Decompose". Update the architecture diagram to include payment-service. Update the service count from 6 to 7.

```tsx
// frontend/src/components/go/tabs/MicroservicesTab.tsx
"use client";

import { MermaidDiagram } from "@/components/MermaidDiagram";

export function MicroservicesTab() {
  return (
    <div className="mt-8">
      {/* Origin Story */}
      <section>
        <h3 className="text-lg font-medium">From Monolith to Microservices</h3>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          The cart, product, and order services were originally a single{" "}
          <code className="text-xs bg-muted px-1 py-0.5 rounded">
            ecommerce-service
          </code>{" "}
          handling all ecommerce logic in one Go binary with a shared PostgreSQL
          database. See the{" "}
          <button
            className="underline hover:text-foreground transition-colors"
            onClick={() => {
              // Parent passes setActiveTab via context or prop — handled in page.tsx
              window.dispatchEvent(new CustomEvent("go-tab-switch", { detail: "original" }));
            }}
          >
            Original
          </button>{" "}
          tab for that architecture.
        </p>
      </section>

      {/* Why Decompose */}
      <section className="mt-8">
        <h3 className="text-lg font-medium">Why Decompose?</h3>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          I decomposed the monolithic ecommerce-service into independent
          services to demonstrate{" "}
          <span className="text-foreground font-medium">
            service extraction patterns
          </span>
          ,{" "}
          <span className="text-foreground font-medium">
            gRPC inter-service communication
          </span>
          , and{" "}
          <span className="text-foreground font-medium">
            saga-based distributed transactions
          </span>{" "}
          &mdash; skills relevant to teams managing growing microservice
          architectures. Each service owns its own database, scales
          independently, and communicates through well-defined contracts.
        </p>
      </section>

      {/* Tech Stack */}
      <section className="mt-8">
        <h3 className="text-lg font-medium">Tech Stack</h3>
        <div className="mt-3 flex flex-wrap gap-2">
          {[
            "7 Go microservices",
            "gRPC + Protobuf",
            "Stripe Payments",
            "RabbitMQ Saga",
            "PostgreSQL (per-service)",
            "Redis",
            "Kafka Analytics",
            "Kubernetes + HPA",
            "Prometheus + Jaeger",
          ].map((tech) => (
            <span
              key={tech}
              className="rounded-full bg-primary/10 px-3 py-1 text-xs font-medium text-primary"
            >
              {tech}
            </span>
          ))}
        </div>
      </section>

      {/* Architecture Diagram */}
      <section className="mt-12">
        <h3 className="text-xl font-semibold">Architecture</h3>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          Seven services communicate via REST (frontend-facing) and gRPC
          (inter-service). The checkout saga coordinates cart reservation,
          stock validation, payment processing, and order completion through
          RabbitMQ command/event queues. Kafka streams analytics events to
          the analytics-service for real-time aggregation.
        </p>
        <div className="mt-6">
          <MermaidDiagram
            chart={`flowchart LR
  FE[Next.js Frontend]
  AUTH[auth-service<br/>REST :8091]
  PROD[product-service<br/>REST :8095 / gRPC :9095]
  CART[cart-service<br/>REST :8096 / gRPC :9096]
  ORD[order-service<br/>REST :8092]
  PAY[payment-service<br/>REST :8098 / gRPC :9098]
  AI[ai-service<br/>REST :8093]
  ANA[analytics-service<br/>REST :8094]
  PG_A[(authdb)]
  PG_P[(productdb)]
  PG_C[(cartdb)]
  PG_O[(orderdb)]
  PG_PAY[(paymentdb)]
  RD[(Redis)]
  MQ{{RabbitMQ<br/>Saga Exchange}}
  KF{{Kafka}}
  FE -->|REST /go-auth| AUTH
  FE -->|REST /go-products| PROD
  FE -->|REST /go-cart| CART
  FE -->|REST /go-orders| ORD
  FE -->|REST /ai-api| AI
  AUTH --> PG_A
  PROD --> PG_P
  CART --> PG_C
  ORD --> PG_O
  PAY --> PG_PAY
  CART -->|gRPC| PROD
  ORD -->|gRPC| CART
  ORD -->|gRPC| PROD
  ORD -->|gRPC| PAY
  ORD -->|saga commands| MQ
  MQ -->|saga events| ORD
  MQ -->|saga commands| CART
  ORD -->|order events| KF
  CART -->|cart events| KF
  KF --> ANA
  PROD --> RD
  CART --> RD`}
          />
        </div>
      </section>

      {/* Checkout Saga Flow */}
      <section className="mt-12">
        <h3 className="text-xl font-semibold">
          Request flow: Checkout saga
        </h3>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          Checkout uses a saga orchestrator pattern: the order-service
          creates a pending order, reserves cart items via RabbitMQ,
          validates stock via gRPC to product-service, creates a Stripe
          payment session via gRPC to payment-service, and clears the cart
          on payment confirmation. The client gets an immediate 201
          response with a checkout URL while the saga runs in the
          background.
        </p>
        <div className="mt-6">
          <MermaidDiagram
            chart={`sequenceDiagram
  participant U as User
  participant FE as Frontend
  participant ORD as order-service
  participant CART as cart-service
  participant PROD as product-service
  participant PAY as payment-service
  participant MQ as RabbitMQ
  participant KF as Kafka
  U->>FE: Click "Checkout"
  FE->>ORD: POST /orders (Bearer JWT)
  ORD->>CART: gRPC GetCart(userId)
  CART->>PROD: gRPC GetProduct (price enrichment)
  CART-->>ORD: cart items + prices
  ORD->>ORD: INSERT order (status=pending, saga_step=CREATED)
  ORD-->>FE: 201 order (status=pending)
  Note over ORD,MQ: Saga begins asynchronously
  ORD->>MQ: publish reserve.items
  MQ->>CART: reserve.items command
  CART->>CART: SET reserved=true
  CART->>MQ: items.reserved event
  MQ->>ORD: items.reserved
  ORD->>PROD: gRPC CheckAvailability
  PROD-->>ORD: available=true
  ORD->>PAY: gRPC CreatePayment(order)
  PAY->>PAY: Stripe Checkout Session
  PAY-->>ORD: checkoutUrl
  ORD->>ORD: saga_step=PAYMENT_CREATED
  Note over U,PAY: User completes Stripe payment
  PAY->>ORD: gRPC PaymentConfirmed(webhook)
  ORD->>MQ: publish clear.cart
  MQ->>CART: clear.cart command
  CART->>CART: DELETE cart items
  CART->>MQ: cart.cleared event
  MQ->>ORD: cart.cleared
  ORD->>ORD: status=completed, saga_step=COMPLETED
  ORD->>KF: order.completed event
  KF->>ANA: analytics aggregation`}
          />
        </div>
        <p className="mt-4 text-sm text-muted-foreground leading-relaxed">
          If stock is insufficient at the CheckAvailability step, the saga
          compensates: order-service publishes{" "}
          <code className="text-xs bg-muted px-1 py-0.5 rounded">
            release.items
          </code>{" "}
          to unreserve cart items and marks the order as FAILED. If payment
          fails, the saga refunds via gRPC and compensates the reservation.
          On startup, crash recovery queries incomplete sagas and resumes
          each from its last known step.
        </p>
      </section>

      {/* What Changed */}
      <section className="mt-12">
        <h3 className="text-xl font-semibold">What Changed</h3>
        <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2">
          {[
            {
              title: "Database-per-Service",
              desc: "Each service owns its database (productdb, cartdb, orderdb, paymentdb). No shared tables.",
            },
            {
              title: "gRPC Contracts",
              desc: "Protobuf-defined service contracts with buf toolchain. Type-safe cross-service calls.",
            },
            {
              title: "Saga Orchestration",
              desc: "RabbitMQ-based saga with compensation flows, DLQ, and crash recovery.",
            },
            {
              title: "Independent Scaling",
              desc: "Each service has its own HPA, PDB, and resource limits. Scale what needs scaling.",
            },
          ].map((card) => (
            <div
              key={card.title}
              className="rounded-lg border border-foreground/10 p-4"
            >
              <h4 className="text-sm font-semibold">{card.title}</h4>
              <p className="mt-1 text-xs text-muted-foreground leading-relaxed">
                {card.desc}
              </p>
            </div>
          ))}
        </div>
      </section>

      {/* Database Optimization */}
      <section className="mt-12">
        <h3 className="text-xl font-semibold">Database Optimization</h3>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          Benchmarked all three database services with{" "}
          <span className="text-foreground font-medium">
            real PostgreSQL via testcontainers
          </span>{" "}
          (not mocks), identified anti-patterns, and applied targeted
          optimizations. The full analysis is documented in a{" "}
          <a
            href="https://github.com/kabradshaw1/portfolio/blob/main/docs/adr/go-database-optimization.md"
            target="_blank"
            rel="noopener noreferrer"
            className="underline hover:text-foreground transition-colors"
          >
            database optimization ADR
          </a>
          .
        </p>

        <h4 className="mt-8 text-lg font-medium">Benchmark Results</h4>
        <div className="mt-4 overflow-x-auto">
          <table className="w-full text-sm text-muted-foreground">
            <thead>
              <tr className="border-b text-left">
                <th className="pb-2 pr-4 font-medium text-foreground">
                  Optimization
                </th>
                <th className="pb-2 pr-4 font-medium text-foreground">
                  Before
                </th>
                <th className="pb-2 pr-4 font-medium text-foreground">
                  After
                </th>
                <th className="pb-2 font-medium text-foreground">
                  Speedup
                </th>
              </tr>
            </thead>
            <tbody className="divide-y">
              <tr>
                <td className="py-2 pr-4">Order creation (20 items)</td>
                <td className="py-2 pr-4">4.5 ms</td>
                <td className="py-2 pr-4">1.3 ms</td>
                <td className="py-2 font-medium text-foreground">3.5x</td>
              </tr>
              <tr>
                <td className="py-2 pr-4">Product search</td>
                <td className="py-2 pr-4">1.0 ms</td>
                <td className="py-2 pr-4">0.55 ms</td>
                <td className="py-2 font-medium text-foreground">1.9x</td>
              </tr>
              <tr>
                <td className="py-2 pr-4">Order creation (5 items)</td>
                <td className="py-2 pr-4">1.5 ms</td>
                <td className="py-2 pr-4">0.8 ms</td>
                <td className="py-2 font-medium text-foreground">1.8x</td>
              </tr>
              <tr>
                <td className="py-2 pr-4">Category filter</td>
                <td className="py-2 pr-4">430 &micro;s</td>
                <td className="py-2 pr-4">327 &micro;s</td>
                <td className="py-2 font-medium text-foreground">1.3x</td>
              </tr>
            </tbody>
          </table>
        </div>

        <h4 className="mt-8 text-lg font-medium">What was optimized</h4>
        <ul className="mt-2 list-disc pl-6 text-muted-foreground space-y-1">
          <li>
            <span className="text-foreground font-medium">Batch INSERT</span>{" "}
            &mdash; replaced N+1 order item inserts with single multi-row INSERT
          </li>
          <li>
            <span className="text-foreground font-medium">Window function</span>{" "}
            &mdash; eliminated COUNT+data double query with COUNT(*) OVER()
          </li>
          <li>
            <span className="text-foreground font-medium">
              CTE conflict resolution
            </span>{" "}
            &mdash; single atomic query replaces two-query cart update pattern
          </li>
          <li>
            <span className="text-foreground font-medium">
              Schema hardening
            </span>{" "}
            &mdash; CHECK constraints, targeted indexes (saga_step, composite
            cart, partial low-stock)
          </li>
          <li>
            <span className="text-foreground font-medium">
              Prepared statement cache
            </span>{" "}
            &mdash; pgx QueryExecModeCacheDescribe across all services
          </li>
        </ul>
      </section>
    </div>
  );
}
```

- [ ] **Step 2: Verify the file compiles**

Run: `cd frontend && npx tsc --noEmit src/components/go/tabs/MicroservicesTab.tsx 2>&1 | head -20`
Expected: No errors (or only unrelated errors from other files)

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/go/tabs/MicroservicesTab.tsx
git commit -m "feat: extract MicroservicesTab component with origin story and payment-service"
```

---

### Task 2: Extract Original Tab Component

**Files:**
- Create: `frontend/src/components/go/tabs/OriginalTab.tsx`

- [ ] **Step 1: Create OriginalTab.tsx**

Cut the entire `{activeTab === "original" && (...)}` block from `page.tsx` (lines 383-602) into a new component. Content is unchanged.

```tsx
// frontend/src/components/go/tabs/OriginalTab.tsx
"use client";

import { MermaidDiagram } from "@/components/MermaidDiagram";

export function OriginalTab() {
  return (
    <div className="mt-8">
      <p className="mt-4 text-muted-foreground leading-relaxed">
        Microservices ecommerce platform built with Go, demonstrating
        RESTful API design, JWT authentication, PostgreSQL, Redis caching,
        asynchronous order processing with RabbitMQ, and an LLM-powered
        shopping assistant with tool-calling agent loop. Deployed using
        Docker and Kubernetes.
      </p>

      <h3 className="mt-6 text-lg font-medium">Tech Stack</h3>
      <ul className="mt-2 list-disc pl-6 text-muted-foreground space-y-1">
        <li>4 Go microservices (auth, ecommerce, ai-service, analytics)</li>
        <li>Gin HTTP framework with JWT authentication</li>
        <li>Ollama (Qwen 2.5 14B) tool-calling agent with 9 tools</li>
        <li>PostgreSQL (users, products, carts, orders)</li>
        <li>Redis caching + rate limiting</li>
        <li>RabbitMQ asynchronous order processing</li>
        <li>Apache Kafka streaming analytics pipeline</li>
        <li>Prometheus metrics instrumentation</li>
        <li>Next.js + TypeScript frontend</li>
        <li>Docker Compose (local dev), Kubernetes (production)</li>
      </ul>

      <section className="mt-12">
        <h3 className="text-xl font-semibold">Architecture</h3>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          Two Go services &mdash; auth and ecommerce &mdash; sharing
          Postgres. The ecommerce service caches product reads in Redis
          and offloads order finalization to a RabbitMQ-driven goroutine
          worker pool.
        </p>
        <div className="mt-6">
          <MermaidDiagram
            chart={`flowchart LR
  FE[Next.js Frontend]
  AUTH[auth-service<br/>Go + JWT]
  EC[ecommerce-service<br/>Go]
  PG[(PostgreSQL)]
  RD[(Redis cache)]
  MQ{{RabbitMQ}}
  WP[Worker pool<br/>goroutines]
  FE -->|REST /go-auth| AUTH
  FE -->|REST /go-orders| EC
  AUTH --> PG
  EC --> PG
  EC --> RD
  EC -->|publish order.events| MQ
  MQ --> WP
  WP --> PG
  WP -->|invalidate products| RD`}
          />
        </div>

        <h3 className="mt-10 text-xl font-semibold">
          Request flow: Checkout order
        </h3>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          The HTTP handler reads the cart from Postgres, inserts a pending
          order, clears the cart, and publishes to RabbitMQ &mdash; all
          synchronously &mdash; then returns 201. A 3-goroutine worker
          pool consumes the event and drives the order through processing
          to completed, decrementing product stock and invalidating the
          Redis product cache along the way.
        </p>
        <div className="mt-6">
          <MermaidDiagram
            chart={`sequenceDiagram
  participant U as User
  participant FE as Frontend
  participant AUTH as auth-service
  participant EC as ecommerce-service
  participant PG as Postgres
  participant MQ as RabbitMQ
  participant WP as Worker pool
  participant RD as Redis
  U->>FE: Click "Checkout"
  FE->>AUTH: POST /login
  AUTH->>PG: verify creds
  AUTH-->>FE: JWT
  FE->>EC: POST /orders (Bearer JWT)
  EC->>EC: validate JWT (middleware)
  EC->>PG: SELECT cart_items for userId
  PG-->>EC: cart items
  EC->>PG: INSERT order (status=pending)
  EC->>PG: DELETE cart_items for userId
  EC->>MQ: publish order.created
  EC-->>FE: 201 order (status=pending)
  MQ->>WP: order.created
  WP->>PG: UPDATE order status=processing
  WP->>PG: UPDATE products stock (per item)
  WP->>PG: UPDATE order status=completed
  WP->>RD: DEL ecom:products:* cache
  FE->>EC: GET /orders/{id}
  EC-->>FE: status=completed`}
          />
        </div>
      </section>

      <section className="mt-12">
        <h3 className="text-xl font-semibold">
          Stress Testing &amp; Scalability
        </h3>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          The ecommerce platform was stress-tested using{" "}
          <a
            href="https://k6.io"
            target="_blank"
            rel="noopener noreferrer"
            className="underline hover:text-foreground transition-colors"
          >
            k6
          </a>{" "}
          across all three services to find bottlenecks, fix them, and
          measure the improvement. The full analysis is documented in a{" "}
          <a
            href="https://github.com/kabradshaw1/portfolio/blob/main/docs/adr/go-stress-testing.md"
            target="_blank"
            rel="noopener noreferrer"
            className="underline hover:text-foreground transition-colors"
          >
            stress testing ADR
          </a>
          .
        </p>

        <h4 className="mt-8 text-lg font-medium">
          What we found and fixed
        </h4>
        <div className="mt-4 overflow-x-auto">
          <table className="w-full text-sm text-muted-foreground">
            <thead>
              <tr className="border-b text-left">
                <th className="pb-2 pr-4 font-medium text-foreground">Issue</th>
                <th className="pb-2 pr-4 font-medium text-foreground">Before</th>
                <th className="pb-2 pr-4 font-medium text-foreground">After</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              <tr>
                <td className="py-2 pr-4">Stock overselling (race condition)</td>
                <td className="py-2 pr-4">296 orders on stock=50</td>
                <td className="py-2 pr-4">0 oversells (SELECT FOR UPDATE)</td>
              </tr>
              <tr>
                <td className="py-2 pr-4">Auth service under load</td>
                <td className="py-2 pr-4">57% error rate at 20 req/s</td>
                <td className="py-2 pr-4">0% errors (HPA scales to 3 replicas)</td>
              </tr>
              <tr>
                <td className="py-2 pr-4">Checkout throughput</td>
                <td className="py-2 pr-4">34 req/s</td>
                <td className="py-2 pr-4">113 req/s (3.3x improvement)</td>
              </tr>
              <tr>
                <td className="py-2 pr-4">Product browse (50 VUs)</td>
                <td className="py-2 pr-4" colSpan={2}>
                  195 req/s at p95=27ms, 0% errors &mdash; no fix needed
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <h4 className="mt-8 text-lg font-medium">Fixes applied</h4>
        <ul className="mt-2 list-disc pl-6 text-muted-foreground space-y-1">
          <li>
            <span className="text-foreground font-medium">Stock race condition</span>{" "}
            &mdash; replaced bare UPDATE with SELECT FOR UPDATE in a transaction
          </li>
          <li>
            <span className="text-foreground font-medium">HPA autoscaling</span>{" "}
            &mdash; CPU-based autoscaling (70% target, 1-3 replicas) for auth and
            ecommerce services
          </li>
          <li>
            <span className="text-foreground font-medium">Connection pool tuning</span>{" "}
            &mdash; explicit pgxpool config (25 max, 5 min conns, health checks)
          </li>
          <li>
            <span className="text-foreground font-medium">Server timeouts</span>{" "}
            &mdash; read/write/idle timeouts on the HTTP server
          </li>
        </ul>

        <p className="mt-4 text-sm text-muted-foreground leading-relaxed">
          k6 metrics are pushed to Prometheus via remote-write and
          correlated with service-side metrics in a dedicated Grafana
          dashboard, showing both the load generator and service
          perspective side-by-side.
        </p>
      </section>
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/components/go/tabs/OriginalTab.tsx
git commit -m "feat: extract OriginalTab component"
```

---

### Task 3: Extract AI Assistant Tab Component

**Files:**
- Create: `frontend/src/components/go/tabs/AiAssistantTab.tsx`

- [ ] **Step 1: Create AiAssistantTab.tsx**

Cut the entire AI Shopping Assistant section from `page.tsx` (lines 622-777) into a new component. Content is unchanged.

```tsx
// frontend/src/components/go/tabs/AiAssistantTab.tsx
"use client";

import { MermaidDiagram } from "@/components/MermaidDiagram";

export function AiAssistantTab() {
  return (
    <div className="mt-8">
      <p className="mt-4 text-muted-foreground leading-relaxed">
        An LLM-powered shopping assistant that wraps a tool-calling agent
        loop around the ecommerce backend and a RAG knowledge base. Users
        ask natural language questions &mdash; the agent decides which tools
        to invoke, calls Go microservices or the Python RAG pipeline, and
        synthesizes a streamed response. Built in Go with Ollama (Qwen 2.5
        14B). The RAG bridge connects Go &rarr; Python chat service &rarr;
        Qdrant vector DB, with circuit breakers and OTel trace propagation
        across the stack boundary.
      </p>

      <h3 className="mt-10 text-xl font-semibold">Tool Catalog</h3>
      <p className="mt-4 text-muted-foreground leading-relaxed">
        The agent has access to twelve tools organized into four domains.
        Catalog tools are public; order, cart, and return tools require JWT
        authentication; knowledge base tools are public and hit the Python
        RAG pipeline via a circuit-breaker HTTP bridge with 30-second
        timeout. Checkout is deliberately excluded &mdash; the agent can
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
  subgraph Knowledge ["Knowledge Base (public, RAG)"]
    T10[search_documents<br/>semantic search + sources]
    T11[ask_document<br/>natural-language Q&A]
    T12[list_collections<br/>vector store inventory]
  end
  AGENT --> Catalog
  AGENT --> Orders
  AGENT --> CartReturns
  AGENT --> Knowledge
  X[place_order<br/>deliberately excluded]:::disabled
  AGENT -.-x X
  classDef disabled stroke-dasharray: 5 5,opacity:0.5`}
        />
      </div>

      <h3 className="mt-10 text-xl font-semibold">Agent Loop</h3>
      <p className="mt-4 text-muted-foreground leading-relaxed">
        The agent runs a synchronous ReAct-style loop &mdash; call the LLM,
        dispatch any requested tools, feed results back into the
        conversation, and repeat until the LLM produces a final answer.
        Bounded by 8 steps and a 30-second wall-clock timeout. Tool errors
        become conversation context for the LLM to handle, not hard failures.
      </p>
      <div className="mt-6">
        <MermaidDiagram
          chart={`flowchart TD
  START([Receive user message])
  LLM[Call Ollama<br/>history + tool schemas]
  DECIDE{Tool calls<br/>in response?}
  DISPATCH[Dispatch tool to<br/>ecommerce API or RAG pipeline]
  APPEND[Append result to<br/>conversation history]
  GUARD{Max 8 steps<br/>or 90s?}
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

      <h3 className="mt-10 text-xl font-semibold">
        Request flow: Product search
      </h3>
      <p className="mt-4 text-muted-foreground leading-relaxed">
        A concrete example: the user asks &ldquo;find me a waterproof jacket
        under $150.&rdquo; The frontend streams Server-Sent Events from the
        AI service, which orchestrates between Ollama and the ecommerce API.
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

      <h3 className="mt-10 text-xl font-semibold">
        Request flow: Product knowledge query
      </h3>
      <p className="mt-4 text-muted-foreground leading-relaxed">
        A cross-stack example: the user asks &ldquo;what&rsquo;s the warranty
        on the Storm Jacket?&rdquo; The Go AI service calls the Python RAG
        pipeline, which searches Qdrant for relevant document chunks and
        generates an answer with source citations.
      </p>
      <div className="mt-6">
        <MermaidDiagram
          chart={`sequenceDiagram
  participant U as User
  participant FE as Frontend
  participant AI as AI Service (Go)
  participant OL as Ollama
  participant PY as Python Chat Svc
  participant QD as Qdrant
  U->>FE: "what's the warranty on the Storm Jacket?"
  FE->>AI: POST /chat (SSE stream, Bearer JWT)
  AI->>OL: Chat(messages, tool_schemas)
  OL-->>AI: tool_call: ask_document
  AI-->>FE: SSE: tool_call {name, args}
  AI->>PY: POST /chat {question, collection}
  PY->>QD: vector search (embedded question)
  QD-->>PY: ranked chunks + scores
  PY->>OL: RAG prompt + retrieved context
  OL-->>PY: generated answer
  PY-->>AI: {answer, sources: [{file, page}]}
  AI->>OL: Chat(messages + tool_result)
  OL-->>AI: final text
  AI-->>FE: SSE: final {text}
  FE-->>U: answer with source citations`}
        />
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/components/go/tabs/AiAssistantTab.tsx
git commit -m "feat: extract AiAssistantTab component"
```

---

### Task 4: Create Analytics and Admin Tab Components

**Files:**
- Create: `frontend/src/components/go/tabs/AnalyticsTab.tsx`
- Create: `frontend/src/components/go/tabs/AdminTab.tsx`

- [ ] **Step 1: Create AnalyticsTab.tsx**

```tsx
// frontend/src/components/go/tabs/AnalyticsTab.tsx
"use client";

import { MermaidDiagram } from "@/components/MermaidDiagram";

export function AnalyticsTab() {
  return (
    <div className="mt-8">
      <section>
        <h3 className="text-lg font-medium">Why Streaming Analytics?</h3>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          Batch analytics &mdash; polling a database on a schedule &mdash;
          introduces latency between an event and its visibility. For an
          ecommerce platform, that means revenue spikes, cart abandonment
          trends, and trending products only appear minutes or hours after
          they happen. Streaming analytics processes events as they occur,
          giving real-time visibility into business metrics without
          scheduled ETL jobs or materialized view refreshes.
        </p>
      </section>

      <section className="mt-8">
        <h3 className="text-lg font-medium">Architecture</h3>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          The order-service and cart-service publish domain events to Kafka
          topics as part of their normal operations. The analytics-service
          consumes these events using a dedicated consumer group and
          aggregates them into sliding-window metrics held in memory. The
          frontend polls the analytics REST API every 30 seconds for
          updated dashboards.
        </p>
        <div className="mt-6">
          <MermaidDiagram
            chart={`flowchart LR
  ORD[order-service]
  CART[cart-service]
  KF{{Kafka}}
  ANA[analytics-service<br/>consumer group]
  MEM[(In-memory<br/>sliding windows)]
  FE[Frontend<br/>30s polling]
  ORD -->|ecommerce.orders| KF
  CART -->|ecommerce.cart| KF
  KF --> ANA
  ANA --> MEM
  MEM -->|REST API| FE`}
          />
        </div>
      </section>

      <section className="mt-8">
        <h3 className="text-lg font-medium">Tech Stack</h3>
        <div className="mt-3 flex flex-wrap gap-2">
          {[
            "Apache Kafka 3.7 (KRaft)",
            "segmentio/kafka-go",
            "Sliding window aggregation",
            "Prometheus metrics",
            "OTel trace propagation",
          ].map((tech) => (
            <span
              key={tech}
              className="rounded-full bg-primary/10 px-3 py-1 text-xs font-medium text-primary"
            >
              {tech}
            </span>
          ))}
        </div>
      </section>

      <section className="mt-8">
        <h3 className="text-lg font-medium">What It Surfaces</h3>
        <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-3">
          {[
            {
              title: "Revenue per Hour",
              desc: "Total revenue, order count, and average order value aggregated into hourly windows.",
            },
            {
              title: "Trending Products",
              desc: "Scored by views and cart adds in a sliding window. Surfaces what customers are looking at right now.",
            },
            {
              title: "Cart Abandonment",
              desc: "Tracks carts started vs. converted to measure checkout friction in real time.",
            },
          ].map((card) => (
            <div
              key={card.title}
              className="rounded-lg border border-foreground/10 p-4"
            >
              <h4 className="text-sm font-semibold">{card.title}</h4>
              <p className="mt-1 text-xs text-muted-foreground leading-relaxed">
                {card.desc}
              </p>
            </div>
          ))}
        </div>
      </section>
    </div>
  );
}
```

- [ ] **Step 2: Create AdminTab.tsx**

```tsx
// frontend/src/components/go/tabs/AdminTab.tsx
"use client";

import { MermaidDiagram } from "@/components/MermaidDiagram";

export function AdminTab() {
  return (
    <div className="mt-8">
      <section>
        <h3 className="text-lg font-medium">Why a DLQ Admin Panel?</h3>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          Distributed sagas fail. A payment provider times out, a stock
          check returns an unexpected error, a database constraint
          violation slips through &mdash; and the message gets nacked to a
          dead-letter queue. Without visibility into the DLQ, these
          failures are invisible. You&apos;d need to SSH into a pod and
          query RabbitMQ manually to find out why an order silently
          disappeared.
        </p>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          The admin panel solves this by exposing DLQ messages through a
          REST API with a web UI. Operators can see which saga step
          failed, inspect the routing key and retry count, and replay
          messages with a single click to re-process them through the
          saga.
        </p>
      </section>

      <section className="mt-8">
        <h3 className="text-lg font-medium">How Replay Works</h3>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          When a message is replayed, it&apos;s re-published to the saga
          exchange with its original routing key. The saga orchestrator
          picks it up and advances the order from its last known step.
          This is safe because each saga step is idempotent &mdash;
          re-processing a step that already succeeded is a no-op.
        </p>
        <div className="mt-6">
          <MermaidDiagram
            chart={`flowchart LR
  DLQ[Dead Letter Queue]
  ADMIN[Admin Panel]
  EX{{Saga Exchange}}
  ORCH[Saga Orchestrator]
  DLQ -->|inspect| ADMIN
  ADMIN -->|replay: re-publish| EX
  EX -->|original routing key| ORCH
  ORCH -->|resume from last step| ORCH`}
          />
        </div>
      </section>

      <section className="mt-8">
        <h3 className="text-lg font-medium">What It Shows</h3>
        <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2">
          {[
            {
              title: "Message Inspection",
              desc: "Routing key, timestamp, retry count, and message body for each dead-lettered message.",
            },
            {
              title: "One-Click Replay",
              desc: "Re-publish a message to the saga exchange. Idempotent steps make this safe to retry.",
            },
            {
              title: "DLQ Count",
              desc: "At-a-glance count of unprocessed messages. Prometheus metric for alerting.",
            },
            {
              title: "Operational Awareness",
              desc: "Demonstrates that building distributed systems means building the tools to operate them.",
            },
          ].map((card) => (
            <div
              key={card.title}
              className="rounded-lg border border-foreground/10 p-4"
            >
              <h4 className="text-sm font-semibold">{card.title}</h4>
              <p className="mt-1 text-xs text-muted-foreground leading-relaxed">
                {card.desc}
              </p>
            </div>
          ))}
        </div>
      </section>
    </div>
  );
}
```

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/go/tabs/AnalyticsTab.tsx frontend/src/components/go/tabs/AdminTab.tsx
git commit -m "feat: add Analytics and Admin documentation tab components"
```

---

### Task 5: Rewrite page.tsx as Thin Shell

**Files:**
- Modify: `frontend/src/app/go/page.tsx`

- [ ] **Step 1: Replace page.tsx with the thin shell**

Replace the entire file content with:

```tsx
// frontend/src/app/go/page.tsx
"use client";

import { useState, useEffect } from "react";
import Link from "next/link";
import { MicroservicesTab } from "@/components/go/tabs/MicroservicesTab";
import { OriginalTab } from "@/components/go/tabs/OriginalTab";
import { AiAssistantTab } from "@/components/go/tabs/AiAssistantTab";
import { AnalyticsTab } from "@/components/go/tabs/AnalyticsTab";
import { AdminTab } from "@/components/go/tabs/AdminTab";

type Tab = "microservices" | "original" | "ai-assistant" | "analytics" | "admin";

const tabs: { key: Tab; label: string }[] = [
  { key: "microservices", label: "Microservices" },
  { key: "original", label: "Original" },
  { key: "ai-assistant", label: "AI Assistant" },
  { key: "analytics", label: "Analytics" },
  { key: "admin", label: "Admin" },
];

export default function GoPage() {
  const [activeTab, setActiveTab] = useState<Tab>("microservices");

  // Listen for tab switch events from child components (e.g., MicroservicesTab origin story link)
  useEffect(() => {
    function handleTabSwitch(e: Event) {
      const tab = (e as CustomEvent).detail as Tab;
      if (tabs.some((t) => t.key === tab)) {
        setActiveTab(tab);
      }
    }
    window.addEventListener("go-tab-switch", handleTabSwitch);
    return () => window.removeEventListener("go-tab-switch", handleTabSwitch);
  }, []);

  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <h1 className="mt-8 text-3xl font-bold">Go Backend Developer</h1>

      {/* Bio Section */}
      <section className="mt-8">
        <p className="text-muted-foreground leading-relaxed">
          Go is my preferred language due to its readability, simplicity, and
          strong performance. It&apos;s my first choice for many backend tasks,
          and I&apos;ve used it to build microservices, automation scripts, and
          command-line tools with a focus on clean, efficient design.
        </p>
        <p className="mt-4 text-sm text-muted-foreground leading-relaxed">
          All seven Go services expose Prometheus metrics to a live{" "}
          <a
            href="https://grafana.kylebradshaw.dev/d/system-overview/system-overview?orgId=1&from=now-1h&to=now&timezone=browser"
            target="_blank"
            rel="noopener noreferrer"
            className="underline hover:text-foreground transition-colors"
          >
            Grafana dashboard
          </a>
          .
        </p>
      </section>

      {/* Project Section with Tabs */}
      <section className="mt-12">
        <h2 className="text-2xl font-semibold">Ecommerce Platform</h2>

        {/* Tab Bar */}
        <div className="mt-4 flex gap-0 border-b border-foreground/10">
          {tabs.map((tab) => (
            <button
              key={tab.key}
              onClick={() => setActiveTab(tab.key)}
              className={`px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px ${
                activeTab === tab.key
                  ? "border-primary text-foreground"
                  : "border-transparent text-muted-foreground hover:text-foreground"
              }`}
            >
              {tab.label}
            </button>
          ))}
        </div>

        {/* Tab Content */}
        {activeTab === "microservices" && <MicroservicesTab />}
        {activeTab === "original" && <OriginalTab />}
        {activeTab === "ai-assistant" && <AiAssistantTab />}
        {activeTab === "analytics" && <AnalyticsTab />}
        {activeTab === "admin" && <AdminTab />}

        {/* CTA Buttons */}
        <div className="mt-8 flex gap-3">
          <Link
            href="/go/ecommerce"
            className="inline-flex items-center gap-2 rounded-lg bg-primary px-6 py-3 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
          >
            View Store &rarr;
          </Link>
          <Link
            href="/go/analytics"
            className="inline-flex items-center gap-2 rounded-lg border px-6 py-3 text-sm font-medium hover:bg-accent transition-colors"
          >
            Streaming Analytics &rarr;
          </Link>
          <Link
            href="/go/admin"
            className="inline-flex items-center gap-2 rounded-lg border px-6 py-3 text-sm font-medium hover:bg-accent transition-colors"
          >
            Admin Panel &rarr;
          </Link>
        </div>
      </section>
    </div>
  );
}
```

- [ ] **Step 2: Run type check**

Run: `cd frontend && npx tsc --noEmit 2>&1 | head -30`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add frontend/src/app/go/page.tsx
git commit -m "feat: rewrite /go page as thin shell with 5-tab bar"
```

---

### Task 6: Add Error Boundaries

**Files:**
- Create: `frontend/src/app/go/error.tsx`
- Create: `frontend/src/app/go/ecommerce/error.tsx`
- Create: `frontend/src/app/go/analytics/error.tsx`
- Create: `frontend/src/app/go/admin/error.tsx`

- [ ] **Step 1: Create the shared error boundary pattern**

All four files use the same pattern. Create each one:

```tsx
// frontend/src/app/go/error.tsx
"use client";

import { useEffect } from "react";

export default function GoError({
  error,
  unstable_retry,
}: {
  error: Error & { digest?: string };
  unstable_retry: () => void;
}) {
  useEffect(() => {
    console.error(error);
  }, [error]);

  return (
    <div className="mx-auto max-w-3xl px-6 py-12 text-center">
      <h2 className="text-xl font-bold">Something went wrong</h2>
      <p className="mt-2 text-muted-foreground">
        {error.message || "An unexpected error occurred."}
      </p>
      <button
        onClick={() => unstable_retry()}
        className="mt-4 rounded-lg bg-primary px-6 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
      >
        Try again
      </button>
    </div>
  );
}
```

```tsx
// frontend/src/app/go/ecommerce/error.tsx
"use client";

import { useEffect } from "react";
import Link from "next/link";

export default function EcommerceError({
  error,
  unstable_retry,
}: {
  error: Error & { digest?: string };
  unstable_retry: () => void;
}) {
  useEffect(() => {
    console.error(error);
  }, [error]);

  return (
    <div className="mx-auto max-w-3xl px-6 py-12 text-center">
      <h2 className="text-xl font-bold">Could not load this page</h2>
      <p className="mt-2 text-muted-foreground">
        {error.message || "The ecommerce service may be temporarily unavailable."}
      </p>
      <div className="mt-4 flex justify-center gap-3">
        <button
          onClick={() => unstable_retry()}
          className="rounded-lg bg-primary px-6 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
        >
          Try again
        </button>
        <Link
          href="/go/ecommerce"
          className="rounded-lg border px-6 py-2 text-sm font-medium hover:bg-accent transition-colors"
        >
          Back to store
        </Link>
      </div>
    </div>
  );
}
```

```tsx
// frontend/src/app/go/analytics/error.tsx
"use client";

import { useEffect } from "react";

export default function AnalyticsError({
  error,
  unstable_retry,
}: {
  error: Error & { digest?: string };
  unstable_retry: () => void;
}) {
  useEffect(() => {
    console.error(error);
  }, [error]);

  return (
    <div className="mx-auto max-w-3xl px-6 py-12 text-center">
      <h2 className="text-xl font-bold">Could not load analytics</h2>
      <p className="mt-2 text-muted-foreground">
        {error.message || "The analytics service may be temporarily unavailable."}
      </p>
      <button
        onClick={() => unstable_retry()}
        className="mt-4 rounded-lg bg-primary px-6 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
      >
        Try again
      </button>
    </div>
  );
}
```

```tsx
// frontend/src/app/go/admin/error.tsx
"use client";

import { useEffect } from "react";

export default function AdminError({
  error,
  unstable_retry,
}: {
  error: Error & { digest?: string };
  unstable_retry: () => void;
}) {
  useEffect(() => {
    console.error(error);
  }, [error]);

  return (
    <div className="mx-auto max-w-3xl px-6 py-12 text-center">
      <h2 className="text-xl font-bold">Could not load admin panel</h2>
      <p className="mt-2 text-muted-foreground">
        {error.message || "The admin service may be temporarily unavailable."}
      </p>
      <button
        onClick={() => unstable_retry()}
        className="mt-4 rounded-lg bg-primary px-6 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
      >
        Try again
      </button>
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/app/go/error.tsx frontend/src/app/go/ecommerce/error.tsx frontend/src/app/go/analytics/error.tsx frontend/src/app/go/admin/error.tsx
git commit -m "feat: add error boundaries for /go route segments"
```

---

### Task 7: Rework HealthGate for Degraded Mode

**Files:**
- Modify: `frontend/src/components/HealthGate.tsx`
- Modify: `frontend/src/app/go/ecommerce/layout.tsx`

- [ ] **Step 1: Add degraded mode to HealthGate**

Add a `degraded` prop that switches from blocking to banner mode:

```tsx
// frontend/src/components/HealthGate.tsx
"use client";

import { useEffect, useState } from "react";
import Link from "next/link";

interface HealthGateProps {
  endpoint: string;
  stack: string;
  docsHref: string;
  children: React.ReactNode;
  /** When true, show a warning banner instead of blocking the page */
  degraded?: boolean;
}

export function HealthGate({ endpoint, stack, docsHref, children, degraded }: HealthGateProps) {
  const [status, setStatus] = useState<"checking" | "healthy" | "unhealthy">("checking");

  useEffect(() => {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 3000);

    fetch(endpoint, { signal: controller.signal })
      .then((res) => {
        setStatus(res.ok ? "healthy" : "unhealthy");
      })
      .catch(() => {
        setStatus("unhealthy");
      })
      .finally(() => {
        clearTimeout(timeout);
      });

    return () => {
      controller.abort();
      clearTimeout(timeout);
    };
  }, [endpoint]);

  if (status === "checking") {
    return (
      <div className="flex min-h-[60vh] items-center justify-center bg-background">
        <div className="h-6 w-6 animate-spin rounded-full border-2 border-muted-foreground border-t-transparent" />
      </div>
    );
  }

  if (status === "unhealthy" && !degraded) {
    const message =
      process.env.NEXT_PUBLIC_MAINTENANCE_MESSAGE ||
      "The backend services are currently offline for maintenance. Please check back later.";

    return (
      <div className="flex min-h-[60vh] items-center justify-center bg-background px-6">
        <div className="max-w-md text-center">
          <div className="text-5xl">🔧</div>
          <h2 className="mt-4 text-2xl font-bold text-foreground">
            Server Maintenance
          </h2>
          <p className="mt-3 text-muted-foreground">{message}</p>
          <div className="mt-6 rounded-lg border border-border bg-muted/50 px-4 py-3 text-sm text-muted-foreground">
            <strong className="text-foreground">{stack}</strong> is currently
            unavailable.
          </div>
          <Link
            href={docsHref}
            className="mt-6 inline-block text-sm text-primary hover:underline"
          >
            &larr; View documentation instead
          </Link>
        </div>
      </div>
    );
  }

  return (
    <>
      {status === "unhealthy" && degraded && (
        <div className="border-b border-amber-500/30 bg-amber-500/10 px-4 py-2 text-center text-sm text-amber-700 dark:text-amber-400">
          <strong>{stack}</strong> is currently unavailable &mdash; some features may not work.
        </div>
      )}
      {children}
    </>
  );
}
```

- [ ] **Step 2: Update ecommerce layout to use degraded mode**

```tsx
// frontend/src/app/go/ecommerce/layout.tsx
"use client";

import { AiAssistantDrawer } from "@/components/go/AiAssistantDrawer";
import { HealthGate } from "@/components/HealthGate";

const goOrderUrl =
  process.env.NEXT_PUBLIC_GO_ORDER_URL || "http://localhost:8092";

export default function GoEcommerceLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <HealthGate
      endpoint={`${goOrderUrl}/health`}
      stack="Go Ecommerce"
      docsHref="/go"
      degraded
    >
      {children}
      <AiAssistantDrawer />
    </HealthGate>
  );
}
```

- [ ] **Step 3: Run type check**

Run: `cd frontend && npx tsc --noEmit 2>&1 | head -20`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/HealthGate.tsx frontend/src/app/go/ecommerce/layout.tsx
git commit -m "feat: add degraded mode to HealthGate, use in ecommerce layout"
```

---

### Task 8: Clean Up Analytics Page

**Files:**
- Modify: `frontend/src/app/go/analytics/page.tsx`

- [ ] **Step 1: Remove the historical reporting section and unused imports/state**

Remove from `analytics/page.tsx`:
- The `AreaChart`, `Area`, `Legend` imports from recharts
- The `goOrderFetch` import
- The `SalesTrend` and `ProductPerf` interfaces
- The `salesTrends`, `productPerf`, and `reportingError` state variables
- The `useEffect` that calls `fetchReporting()` (lines 129-149)
- The entire "Historical Reporting" `<div>` (lines 346-458)

The result keeps the Kafka streaming section (revenue, trending, abandonment) and adds a brief intro paragraph.

Replace the file with:

```tsx
// frontend/src/app/go/analytics/page.tsx
"use client";

import { useEffect, useState, useCallback } from "react";
import Link from "next/link";
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from "recharts";

const ANALYTICS_URL =
  process.env.NEXT_PUBLIC_GO_ANALYTICS_URL || "http://localhost:8094";

const POLL_INTERVAL = 30_000; // 30 seconds

interface RevenueWindow {
  window_start: string;
  window_end: string;
  total_cents: number;
  order_count: number;
  avg_order_value_cents: number;
}

interface TrendingProduct {
  product_id: string;
  product_name: string;
  score: number;
  views: number;
  cart_adds: number;
}

interface TrendingData {
  window_end: string;
  products: TrendingProduct[];
  stale: boolean;
}

interface AbandonmentWindow {
  window_start: string;
  window_end: string;
  carts_started: number;
  carts_converted: number;
  carts_abandoned: number;
  abandonment_rate: number;
}

export default function AnalyticsPage() {
  const [revenue, setRevenue] = useState<RevenueWindow[]>([]);
  const [trending, setTrending] = useState<TrendingData | null>(null);
  const [abandonment, setAbandonment] = useState<AbandonmentWindow[]>([]);
  const [stale, setStale] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchAll = useCallback(async () => {
    try {
      const [revRes, trendRes, abandRes] = await Promise.all([
        fetch(`${ANALYTICS_URL}/analytics/revenue?hours=24`),
        fetch(`${ANALYTICS_URL}/analytics/trending?limit=10`),
        fetch(`${ANALYTICS_URL}/analytics/cart-abandonment?hours=12`),
      ]);

      if (revRes.ok) {
        const data = await revRes.json();
        setRevenue(data.windows ?? []);
        if (data.stale) setStale(true);
      }
      if (trendRes.ok) {
        const data = await trendRes.json();
        setTrending(data);
        if (data.stale) setStale(true);
      }
      if (abandRes.ok) {
        const data = await abandRes.json();
        setAbandonment(data.windows ?? []);
        if (data.stale) setStale(true);
      }
      setError(null);
    } catch {
      setError("Unable to reach analytics service");
    }
  }, []);

  useEffect(() => {
    let cancelled = false;

    async function poll() {
      if (!cancelled) await fetchAll();
    }

    poll();
    const interval = setInterval(poll, POLL_INTERVAL);
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [fetchAll]);

  const totalRevenue = revenue.reduce((sum, w) => sum + w.total_cents, 0);
  const totalOrders = revenue.reduce((sum, w) => sum + w.order_count, 0);
  const avgOrderValue =
    totalOrders > 0 ? totalRevenue / totalOrders : 0;

  const latestAbandonment =
    abandonment.length > 0 ? abandonment[abandonment.length - 1] : null;

  const revenueChartData = revenue.map((w) => ({
    hour: w.window_start,
    revenue: w.total_cents / 100,
  }));

  const abandonmentChartData = abandonment.map((w) => ({
    slot: w.window_start,
    rate: w.abandonment_rate * 100,
  }));

  return (
    <div className="mx-auto max-w-5xl px-6 py-8">
      <h1 className="mb-2 text-2xl font-bold">Kafka Streaming Analytics</h1>
      <p className="mb-6 text-sm text-muted-foreground">
        Real-time ecommerce metrics powered by Apache Kafka consumer groups and
        in-memory sliding window aggregation. Events are published by the
        order-service and cart-service as part of normal operations, consumed by
        the analytics-service, and aggregated into the dashboards below. Data
        refreshes every 30 seconds.
      </p>

      {stale && (
        <div className="mb-4 rounded border border-muted-foreground/20 bg-muted px-4 py-3 text-sm text-muted-foreground">
          No recent activity. Place orders in the{" "}
          <Link href="/go/ecommerce" className="underline hover:text-foreground">Store</Link>{" "}
          to see live metrics appear here.
        </div>
      )}

      {error && (
        <div className="mb-4 rounded border border-red-500/30 bg-red-500/10 px-4 py-2 text-sm text-red-600 dark:text-red-400">
          {error}
        </div>
      )}

      {/* Revenue per Hour */}
      <div className="mb-8">
        <h2 className="mb-3 text-lg font-semibold">Revenue per Hour</h2>
        <div className="mb-4 grid grid-cols-3 gap-4">
          <StatCard
            label="Total Revenue (24h)"
            value={`$${(totalRevenue / 100).toFixed(2)}`}
          />
          <StatCard
            label="Total Orders (24h)"
            value={totalOrders.toString()}
          />
          <StatCard
            label="Avg Order Value"
            value={`$${(avgOrderValue / 100).toFixed(2)}`}
          />
        </div>
        <div className="rounded border bg-card p-4">
          {revenueChartData.length > 0 ? (
            <ResponsiveContainer width="100%" height={250}>
              <BarChart data={revenueChartData}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis
                  dataKey="hour"
                  tickFormatter={(v: string) =>
                    new Date(v).toLocaleTimeString([], {
                      hour: "2-digit",
                      minute: "2-digit",
                    })
                  }
                  fontSize={12}
                />
                <YAxis
                  tickFormatter={(v: number) => `$${v.toFixed(0)}`}
                  fontSize={12}
                />
                <Tooltip
                  labelFormatter={(v) => new Date(String(v)).toLocaleString()}
                  formatter={(value) => [`$${Number(value).toFixed(2)}`, "Revenue"]}
                />
                <Bar
                  dataKey="revenue"
                  fill="hsl(var(--primary))"
                />
              </BarChart>
            </ResponsiveContainer>
          ) : (
            <p className="py-8 text-center text-muted-foreground">
              No revenue data yet
            </p>
          )}
        </div>
      </div>

      {/* Trending Products */}
      <div className="mb-8">
        <h2 className="mb-3 text-lg font-semibold">Trending Products</h2>
        <div className="rounded border bg-card">
          {trending?.products && trending.products.length > 0 ? (
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b text-left text-muted-foreground">
                  <th className="px-4 py-2">#</th>
                  <th className="px-4 py-2">Product</th>
                  <th className="px-4 py-2 text-right">Score</th>
                  <th className="px-4 py-2 text-right">Views</th>
                  <th className="px-4 py-2 text-right">Cart Adds</th>
                </tr>
              </thead>
              <tbody>
                {trending.products.map((p, i) => (
                  <tr key={p.product_id} className="border-b last:border-0">
                    <td className="px-4 py-2 text-muted-foreground">
                      {i + 1}
                    </td>
                    <td className="px-4 py-2 font-medium">
                      {p.product_name || p.product_id}
                    </td>
                    <td className="px-4 py-2 text-right font-semibold">
                      {p.score}
                    </td>
                    <td className="px-4 py-2 text-right">{p.views}</td>
                    <td className="px-4 py-2 text-right">{p.cart_adds}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          ) : (
            <p className="px-4 py-8 text-center text-muted-foreground">
              No trending products yet
            </p>
          )}
        </div>
      </div>

      {/* Cart Abandonment */}
      <div className="mb-8">
        <h2 className="mb-3 text-lg font-semibold">Cart Abandonment</h2>
        <div className="mb-4 grid grid-cols-3 gap-4">
          <StatCard
            label="Abandonment Rate"
            value={
              latestAbandonment
                ? `${(latestAbandonment.abandonment_rate * 100).toFixed(1)}%`
                : "---"
            }
            className="text-amber-500"
          />
          <StatCard
            label="Carts Started"
            value={latestAbandonment?.carts_started.toString() ?? "---"}
          />
          <StatCard
            label="Carts Converted"
            value={latestAbandonment?.carts_converted.toString() ?? "---"}
          />
        </div>
        <div className="rounded border bg-card p-4">
          {abandonmentChartData.length > 0 ? (
            <ResponsiveContainer width="100%" height={250}>
              <BarChart data={abandonmentChartData}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis
                  dataKey="slot"
                  tickFormatter={(v: string) =>
                    new Date(v).toLocaleTimeString([], {
                      hour: "2-digit",
                      minute: "2-digit",
                    })
                  }
                  fontSize={12}
                />
                <YAxis
                  tickFormatter={(v: number) => `${v.toFixed(0)}%`}
                  fontSize={12}
                />
                <Tooltip
                  labelFormatter={(v) => new Date(String(v)).toLocaleString()}
                  formatter={(value) => [`${Number(value).toFixed(1)}%`, "Abandonment Rate"]}
                />
                <Bar
                  dataKey="rate"
                  fill="#f59e0b"
                />
              </BarChart>
            </ResponsiveContainer>
          ) : (
            <p className="py-8 text-center text-muted-foreground">
              No cart abandonment data yet
            </p>
          )}
        </div>
      </div>
    </div>
  );
}

function StatCard({
  label,
  value,
  className,
}: {
  label: string;
  value: string;
  className?: string;
}) {
  return (
    <div className="rounded border bg-card px-4 py-3">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className={`mt-1 text-2xl font-bold ${className ?? ""}`}>{value}</p>
    </div>
  );
}
```

- [ ] **Step 2: Run type check**

Run: `cd frontend && npx tsc --noEmit 2>&1 | head -20`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add frontend/src/app/go/analytics/page.tsx
git commit -m "fix: remove stale historical reporting section from analytics page"
```

---

### Task 9: Fix Seed Data Idempotency and Expand Catalog

**Files:**
- Modify: `go/product-service/seed.sql`

- [ ] **Step 1: Rewrite seed.sql with per-product idempotency and expanded catalog**

Replace the entire file. The key change is using individual `INSERT ... WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = '...')` statements instead of one bulk insert guarded by table emptiness.

```sql
-- Seed data for productdb. Run by go/k8s/jobs/product-service-migrate.yml
-- after `migrate up` succeeds. Every INSERT is guarded by name so the Job
-- can re-run on every deploy without creating duplicates or blocking new products.

-- Electronics (8 items)
INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Wireless Bluetooth Headphones', 'Noise-canceling over-ear headphones with 30hr battery', 7999, 'Electronics', '', 50
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Wireless Bluetooth Headphones');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'USB-C Fast Charger', 'GaN 65W charger with 3 ports', 3499, 'Electronics', '', 100
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'USB-C Fast Charger');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Mechanical Keyboard', 'RGB backlit with Cherry MX switches', 12999, 'Electronics', '', 30
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Mechanical Keyboard');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Portable SSD 1TB', 'NVMe external drive, USB-C, 1050MB/s', 8999, 'Electronics', '', 40
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Portable SSD 1TB');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT '4K Webcam', '4K autofocus webcam with built-in microphone and privacy shutter', 9999, 'Electronics', '', 45
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = '4K Webcam');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT '27" Monitor', '27-inch IPS 4K display, USB-C power delivery, 60Hz', 34999, 'Electronics', '', 20
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = '27" Monitor');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Smartwatch', 'Fitness tracking, heart rate monitor, 5-day battery, water resistant', 19999, 'Electronics', '', 35
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Smartwatch');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Wireless Earbuds', 'True wireless with ANC, 8hr battery, IPX5 sweat resistant', 12999, 'Electronics', '', 60
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Wireless Earbuds');

-- Clothing (8 items)
INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Classic Cotton T-Shirt', 'Heavyweight premium cotton, unisex fit', 2499, 'Clothing', '', 200
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Classic Cotton T-Shirt');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Slim Fit Chinos', 'Stretch cotton blend, multiple colors available', 4999, 'Clothing', '', 80
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Slim Fit Chinos');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Lightweight Rain Jacket', 'Packable waterproof shell with sealed seams', 6999, 'Clothing', '', 60
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Lightweight Rain Jacket');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Merino Wool Beanie', 'Breathable and temperature regulating', 1999, 'Clothing', '', 150
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Merino Wool Beanie');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Running Shoes', 'Lightweight mesh upper with responsive foam sole', 8999, 'Clothing', '', 70
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Running Shoes');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Denim Jacket', 'Classic wash, 100% cotton denim with button front', 5999, 'Clothing', '', 50
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Denim Jacket');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Performance Polo', 'Moisture-wicking stretch fabric, UPF 30+', 3499, 'Clothing', '', 90
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Performance Polo');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Zip-Up Hoodie', 'French terry cotton, full zip, kangaroo pocket', 4499, 'Clothing', '', 100
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Zip-Up Hoodie');

-- Home (8 items)
INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Pour-Over Coffee Maker', 'Borosilicate glass with stainless steel filter', 3999, 'Home', '', 70
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Pour-Over Coffee Maker');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Cast Iron Skillet 12"', 'Pre-seasoned, oven safe to 500F', 4499, 'Home', '', 45
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Cast Iron Skillet 12"');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'LED Desk Lamp', 'Adjustable brightness and color temperature', 5999, 'Home', '', 55
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'LED Desk Lamp');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Ceramic Planter Set', 'Set of 3, drainage holes included', 2999, 'Home', '', 90
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Ceramic Planter Set');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Robot Vacuum', 'LiDAR navigation, auto-empty base, 150 min runtime', 29999, 'Home', '', 15
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Robot Vacuum');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Air Purifier', 'HEPA 13 filter, covers 500 sq ft, whisper-quiet night mode', 14999, 'Home', '', 25
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Air Purifier');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Smart Thermostat', 'Wi-Fi enabled, learning schedule, energy reports', 12999, 'Home', '', 30
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Smart Thermostat');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Chef Knife Set', '5-piece forged stainless steel with magnetic block', 7999, 'Home', '', 40
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Chef Knife Set');

-- Books (8 items)
INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'The Go Programming Language', 'Donovan & Kernighan — comprehensive Go guide', 3499, 'Books', '', 120
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'The Go Programming Language');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Designing Data-Intensive Applications', 'Martin Kleppmann — distributed systems bible', 3999, 'Books', '', 100
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Designing Data-Intensive Applications');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Clean Architecture', 'Robert C. Martin — software design principles', 2999, 'Books', '', 80
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Clean Architecture');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'System Design Interview', 'Alex Xu — practical system design guide', 3499, 'Books', '', 95
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'System Design Interview');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Database Internals', 'Alex Petrov — storage engines and distributed data', 4499, 'Books', '', 60
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Database Internals');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Observability Engineering', 'Majors, Fong-Jones, Miranda — modern telemetry', 3999, 'Books', '', 55
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Observability Engineering');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Kubernetes in Action', 'Marko Luksa — practical K8s guide, 2nd edition', 4999, 'Books', '', 70
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Kubernetes in Action');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Networking and Kubernetes', 'James Strong & Vallery Lancey — K8s networking deep dive', 3999, 'Books', '', 50
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Networking and Kubernetes');

-- Sports (8 items)
INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Yoga Mat 6mm', 'Non-slip TPE material with carrying strap', 2999, 'Sports', '', 110
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Yoga Mat 6mm');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Adjustable Dumbbells', 'Quick-change weight from 5-52.5 lbs per hand', 29999, 'Sports', '', 20
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Adjustable Dumbbells');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Resistance Band Set', '5 bands with handles, door anchor, and bag', 1999, 'Sports', '', 130
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Resistance Band Set');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Water Bottle 32oz', 'Insulated stainless steel, keeps cold 24hrs', 2499, 'Sports', '', 200
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Water Bottle 32oz');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Running Watch', 'GPS, heart rate, VO2 max, 14-day battery', 24999, 'Sports', '', 25
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Running Watch');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Foam Roller', 'High-density EVA foam, 18-inch, textured surface', 2499, 'Sports', '', 80
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Foam Roller');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Pull-Up Bar', 'Doorframe mount, padded grips, 300 lb capacity', 3499, 'Sports', '', 65
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Pull-Up Bar');

INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Hiking Backpack 40L', 'Ventilated back panel, rain cover, hydration compatible', 8999, 'Sports', '', 35
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Hiking Backpack 40L');

-- Smoke-test product with effectively unlimited stock so automated tests
-- never deplete inventory or affect the demo catalog.
INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Smoke Test Widget', 'Reserved for automated smoke tests', 100, 'Electronics', '', 999999
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Smoke Test Widget');
```

- [ ] **Step 2: Run migration preflight**

Run: `make preflight-go-migrations`
Expected: All migrations pass, tables verified

- [ ] **Step 3: Commit**

```bash
git add go/product-service/seed.sql
git commit -m "fix: per-product idempotency guard and expand catalog to 40 products"
```

---

### Task 10: Run Full Preflight and Verify

**Files:** None (verification only)

- [ ] **Step 1: Run frontend preflight**

Run: `make preflight-frontend`
Expected: tsc passes, Next.js build succeeds

- [ ] **Step 2: Run Go preflight (for seed.sql validation)**

Run: `make preflight-go`
Expected: lint + tests pass

- [ ] **Step 3: Verify tab components render**

Run: `cd frontend && npm run dev`

Open `http://localhost:3000/go` in a browser and verify:
- All 5 tabs render and switch correctly
- Microservices tab shows origin story, payment-service in diagram, "7 Go microservices" badge
- Original tab content unchanged
- AI Assistant tab shows all diagrams
- Analytics tab shows new problem-solution content
- Admin tab shows new problem-solution content
- Three CTA buttons at bottom (Store, Streaming Analytics, Admin Panel)

- [ ] **Step 4: Verify error boundaries**

Open `http://localhost:3000/go/ecommerce/orders` — should show degraded banner + orders page (or inline error if not logged in), NOT a full-page crash.

- [ ] **Step 5: Verify analytics page**

Open `http://localhost:3000/go/analytics` — should show only Kafka streaming sections, no historical reporting section.
