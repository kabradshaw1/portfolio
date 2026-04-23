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
  );
}
