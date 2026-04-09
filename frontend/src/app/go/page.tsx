import Link from "next/link";
import { MermaidDiagram } from "@/components/MermaidDiagram";

export default function GoPage() {
  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <h1 className="mt-8 text-3xl font-bold">Go Backend Developer</h1>

      {/* Bio Section */}
      <section className="mt-8">
        <p className="text-muted-foreground leading-relaxed">
          Go is my preferred language due to its readability, simplicity, and
          strong performance. It’s my first choice for many backend tasks, and
          I’ve used it to build microservices, automation scripts, and
          command-line tools with a focus on clean, efficient design.
        </p>
        <p className="mt-4 text-sm text-muted-foreground leading-relaxed">
          Both Go services expose Prometheus metrics to a live{" "}
          <a
            href="https://api.kylebradshaw.dev/grafana/"
            target="_blank"
            rel="noopener noreferrer"
            className="underline hover:text-foreground transition-colors"
          >
            Grafana dashboard
          </a>
          .
        </p>
      </section>

      {/* Project Section */}
      <section className="mt-12">
        <h2 className="text-2xl font-semibold">Ecommerce Platform</h2>

        <p className="mt-4 text-muted-foreground leading-relaxed">
          Microservices ecommerce platform built with Go, demonstrating
          RESTful API design, JWT authentication, PostgreSQL, Redis caching,
          and asynchronous order processing with RabbitMQ. Deployed using
          Docker and Kubernetes.
        </p>

        <h3 className="mt-6 text-lg font-medium">Tech Stack</h3>
        <ul className="mt-2 list-disc pl-6 text-muted-foreground space-y-1">
          <li>2 Go microservices (auth-service, ecommerce-service)</li>
          <li>Gin HTTP framework with JWT authentication</li>
          <li>PostgreSQL (users, products, carts, orders)</li>
          <li>Redis product caching for fast reads</li>
          <li>RabbitMQ asynchronous order processing</li>
          <li>Prometheus metrics instrumentation</li>
          <li>Next.js + TypeScript frontend</li>
          <li>Docker Compose (local dev), Kubernetes (production)</li>
        </ul>

        <section className="mt-12">
          <h2 className="text-2xl font-semibold">Architecture</h2>
          <p className="mt-4 text-muted-foreground leading-relaxed">
            Two Go services — auth and ecommerce — sharing Postgres. The
            ecommerce service caches product reads in Redis and offloads order
            finalization to a RabbitMQ-driven goroutine worker pool.
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
  FE -->|REST /go-api| EC
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
            order, clears the cart, and publishes to RabbitMQ — all
            synchronously — then returns 201. A 3-goroutine worker pool
            consumes the event and drives the order through processing to
            completed, decrementing product stock and invalidating the Redis
            product cache along the way.
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

        <div className="mt-6">
          <Link
            href="/go/ecommerce"
            className="inline-flex items-center gap-2 rounded-lg bg-primary px-6 py-3 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
          >
            View Project &rarr;
          </Link>
        </div>
      </section>
    </div>
  );
}