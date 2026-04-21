"use client";

import { useState } from "react";
import Link from "next/link";
import { MermaidDiagram } from "@/components/MermaidDiagram";

type Tab = "microservices" | "original";

export default function GoPage() {
  const [activeTab, setActiveTab] = useState<Tab>("microservices");

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
          All six Go services expose Prometheus metrics to a live{" "}
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
          <button
            onClick={() => setActiveTab("microservices")}
            className={`px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px ${
              activeTab === "microservices"
                ? "border-primary text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground"
            }`}
          >
            Microservices
          </button>
          <button
            onClick={() => setActiveTab("original")}
            className={`px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px ${
              activeTab === "original"
                ? "border-primary text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground"
            }`}
          >
            Original
          </button>
        </div>

        {/* Microservices Tab */}
        {activeTab === "microservices" && (
          <div className="mt-8">
            {/* Why Decompose */}
            <section>
              <h3 className="text-lg font-medium">Why Decompose?</h3>
              <p className="mt-4 text-muted-foreground leading-relaxed">
                I decomposed the monolithic ecommerce-service into three
                independent services to demonstrate{" "}
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
                  "6 Go microservices",
                  "gRPC + Protobuf",
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
                Six services communicate via REST (frontend-facing) and gRPC
                (inter-service). The checkout saga coordinates cart reservation,
                stock validation, and order completion through RabbitMQ
                command/event queues. Kafka streams analytics events to the
                analytics-service for real-time aggregation.
              </p>
              <div className="mt-6">
                <MermaidDiagram
                  chart={`flowchart LR
  FE[Next.js Frontend]
  AUTH[auth-service<br/>REST :8091]
  PROD[product-service<br/>REST :8095 / gRPC :9095]
  CART[cart-service<br/>REST :8096 / gRPC :9096]
  ORD[order-service<br/>REST :8092]
  AI[ai-service<br/>REST :8093]
  ANA[analytics-service<br/>REST :8094]
  PG_A[(authdb)]
  PG_P[(productdb)]
  PG_C[(cartdb)]
  PG_O[(ecommercedb)]
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
  CART -->|gRPC| PROD
  ORD -->|gRPC| CART
  ORD -->|gRPC| PROD
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
                creates a pending order, then asynchronously publishes commands
                to cart-service via RabbitMQ, validates stock via gRPC to
                product-service, and clears the cart on success. The client gets
                an immediate 201 response while the saga runs in the background.
              </p>
              <div className="mt-6">
                <MermaidDiagram
                  chart={`sequenceDiagram
  participant U as User
  participant FE as Frontend
  participant ORD as order-service
  participant CART as cart-service
  participant PROD as product-service
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
                to unreserve cart items and marks the order as FAILED. On
                startup, crash recovery queries incomplete sagas and resumes each
                from its last known step.
              </p>
            </section>

            {/* What Changed */}
            <section className="mt-12">
              <h3 className="text-xl font-semibold">What Changed</h3>
              <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2">
                {[
                  {
                    title: "Database-per-Service",
                    desc: "Each service owns its database (productdb, cartdb, ecommercedb). No shared tables.",
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
          </div>
        )}

        {/* Original Tab */}
        {activeTab === "original" && (
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
              <li>
                4 Go microservices (auth, ecommerce, ai-service, analytics)
              </li>
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
                      <th className="pb-2 pr-4 font-medium text-foreground">
                        Issue
                      </th>
                      <th className="pb-2 pr-4 font-medium text-foreground">
                        Before
                      </th>
                      <th className="pb-2 pr-4 font-medium text-foreground">
                        After
                      </th>
                    </tr>
                  </thead>
                  <tbody className="divide-y">
                    <tr>
                      <td className="py-2 pr-4">
                        Stock overselling (race condition)
                      </td>
                      <td className="py-2 pr-4">296 orders on stock=50</td>
                      <td className="py-2 pr-4">
                        0 oversells (SELECT FOR UPDATE)
                      </td>
                    </tr>
                    <tr>
                      <td className="py-2 pr-4">Auth service under load</td>
                      <td className="py-2 pr-4">57% error rate at 20 req/s</td>
                      <td className="py-2 pr-4">
                        0% errors (HPA scales to 3 replicas)
                      </td>
                    </tr>
                    <tr>
                      <td className="py-2 pr-4">Checkout throughput</td>
                      <td className="py-2 pr-4">34 req/s</td>
                      <td className="py-2 pr-4">
                        113 req/s (3.3x improvement)
                      </td>
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
                  <span className="text-foreground font-medium">
                    Stock race condition
                  </span>{" "}
                  &mdash; replaced bare UPDATE with SELECT FOR UPDATE in a
                  transaction
                </li>
                <li>
                  <span className="text-foreground font-medium">
                    HPA autoscaling
                  </span>{" "}
                  &mdash; CPU-based autoscaling (70% target, 1-3 replicas) for
                  auth and ecommerce services
                </li>
                <li>
                  <span className="text-foreground font-medium">
                    Connection pool tuning
                  </span>{" "}
                  &mdash; explicit pgxpool config (25 max, 5 min conns, health
                  checks)
                </li>
                <li>
                  <span className="text-foreground font-medium">
                    Server timeouts
                  </span>{" "}
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
        )}

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
        </div>
      </section>

      {/* AI Shopping Assistant — standalone section */}
      <section className="mt-12">
        <h2 className="text-2xl font-semibold">AI Shopping Assistant</h2>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          An LLM-powered shopping assistant that wraps a tool-calling agent
          loop around the ecommerce backend. Users ask natural language
          questions &mdash; the agent decides which tools to invoke, calls the
          ecommerce API, and synthesizes a streamed response. Built in Go with
          Ollama (Qwen 2.5 14B).
        </p>

        <h3 className="mt-10 text-xl font-semibold">Tool Catalog</h3>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          The agent has access to nine tools organized into three domains.
          Catalog tools are public; order, cart, and return tools require JWT
          authentication. Checkout is deliberately excluded &mdash; the agent
          can advise but not transact.
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
      </section>
    </div>
  );
}
