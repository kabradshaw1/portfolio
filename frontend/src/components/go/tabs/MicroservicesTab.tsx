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

      {/* Database Optimization — moved to /database#optimization */}
      <section className="mt-12" data-testid="database-optimization-breadcrumb">
        <h3 className="text-xl font-semibold">Database Optimization</h3>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          Benchmark methodology and the full before/after results live on the{" "}
          <a
            href="/database#optimization"
            className="underline hover:text-foreground transition-colors"
          >
            Database
          </a>{" "}
          page.
        </p>
      </section>
    </div>
  );
}
