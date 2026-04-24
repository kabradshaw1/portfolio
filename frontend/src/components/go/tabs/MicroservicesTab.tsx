import { MermaidDiagram } from "@/components/MermaidDiagram";

export function MicroservicesTab() {
  return (
    <div className="mt-8">
      {/* Origin Story */}
      <section>
        <h3 className="text-lg font-medium">From Monolith to Microservices</h3>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          This platform started as a single <code className="text-xs bg-muted px-1 py-0.5 rounded">ecommerce-service</code> handling
          products, cart, and orders in one Go binary — a deliberate starting
          point to demonstrate the full decomposition journey. Over three phases,
          cart, product, and order responsibilities were extracted into
          independent services with their own databases, each communicating
          through well-defined gRPC contracts.{" "}
          <button
            onClick={() =>
              window.dispatchEvent(
                new CustomEvent("go-tab-switch", { detail: "original" })
              )
            }
            className="underline hover:text-foreground transition-colors"
          >
            See the original architecture &rarr;
          </button>
        </p>
      </section>

      {/* Why Decompose */}
      <section className="mt-8">
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
            "8 Go microservices",
            "gRPC + Protobuf",
            "RabbitMQ Saga",
            "PostgreSQL (per-service)",
            "Redis",
            "Kafka Analytics",
            "Stripe Payments",
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
          RabbitMQ command/event queues. Kafka streams analytics events to the
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
  PAY[payment-service<br/>REST :8098 / gRPC :9098]
  AI[ai-service<br/>REST :8093]
  ANA[analytics-service<br/>REST :8094]
  PROJ[order-projector<br/>REST :8097]
  PG_A[(authdb)]
  PG_P[(productdb)]
  PG_C[(cartdb)]
  PG_O[(orderdb)]
  PG_PAY[(paymentdb)]
  PG_PROJ[(projectordb)]
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
  KF --> PROJ
  PROJ --> PG_PROJ
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
          product-service, processes payment via gRPC to payment-service,
          and clears the cart on success. The client gets an immediate 201
          response while the saga runs in the background.
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
  ORD->>PAY: gRPC CreatePayment (Stripe)
  PAY-->>ORD: payment_intent_id
  Note over PAY,ORD: Stripe webhook → PaymentConfirmed
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
          to unreserve cart items, issues a payment refund if payment was
          already created, and marks the order as FAILED. On startup, crash
          recovery queries incomplete sagas and resumes each from its last
          known step.
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
                <td className="py-2 pr-4">
                  Order creation (20 items)
                </td>
                <td className="py-2 pr-4">4.5 ms</td>
                <td className="py-2 pr-4">1.3 ms</td>
                <td className="py-2 font-medium text-foreground">
                  3.5x
                </td>
              </tr>
              <tr>
                <td className="py-2 pr-4">Product search</td>
                <td className="py-2 pr-4">1.0 ms</td>
                <td className="py-2 pr-4">0.55 ms</td>
                <td className="py-2 font-medium text-foreground">
                  1.9x
                </td>
              </tr>
              <tr>
                <td className="py-2 pr-4">
                  Order creation (5 items)
                </td>
                <td className="py-2 pr-4">1.5 ms</td>
                <td className="py-2 pr-4">0.8 ms</td>
                <td className="py-2 font-medium text-foreground">
                  1.8x
                </td>
              </tr>
              <tr>
                <td className="py-2 pr-4">Category filter</td>
                <td className="py-2 pr-4">430 &micro;s</td>
                <td className="py-2 pr-4">327 &micro;s</td>
                <td className="py-2 font-medium text-foreground">
                  1.3x
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <h4 className="mt-8 text-lg font-medium">What was optimized</h4>
        <ul className="mt-2 list-disc pl-6 text-muted-foreground space-y-1">
          <li>
            <span className="text-foreground font-medium">
              Batch INSERT
            </span>{" "}
            &mdash; replaced N+1 order item inserts with single multi-row
            INSERT
          </li>
          <li>
            <span className="text-foreground font-medium">
              Window function
            </span>{" "}
            &mdash; eliminated COUNT+data double query with COUNT(*) OVER()
          </li>
          <li>
            <span className="text-foreground font-medium">
              CTE conflict resolution
            </span>{" "}
            &mdash; single atomic query replaces two-query cart update
            pattern
          </li>
          <li>
            <span className="text-foreground font-medium">
              Schema hardening
            </span>{" "}
            &mdash; CHECK constraints, targeted indexes (saga_step,
            composite cart, partial low-stock)
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
