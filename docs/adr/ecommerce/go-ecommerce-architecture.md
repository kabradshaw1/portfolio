# Go Ecommerce Platform Architecture

- **Date:** 2026-04-06
- **Status:** Accepted

## Context

The portfolio needed a Go backend project to demonstrate skills for a Go backend developer role. The job description emphasizes: idiomatic Go, RESTful API design, SQL databases, concurrency patterns, message brokers, caching, observability, and microservices architecture.

The existing portfolio already demonstrates Python (FastAPI, RAG, LLM workflows) and Java (Spring Boot, GraphQL, microservices). The Go project needed to complement these without duplicating functionality, while showcasing Go-specific strengths: simplicity, performance, and concurrency.

An ecommerce platform was chosen because it naturally exercises all the required skills: product catalog (REST + SQL), shopping cart (stateful user sessions), order processing (async workflows), and the full operational stack (caching, observability, CI/CD).

## Decision

### Two Microservices, Not One

The platform is split into **auth-service** and **ecommerce-service**:

| Service | Owns | Port |
|---------|------|------|
| auth-service | Users, registration, login, JWT tokens | 8091 |
| ecommerce-service | Products, cart, orders, async processing | 8092 |

**Why not a monolith?** The Java section already demonstrates monolithic-ish service design (gateway proxies to backend services, but each service is substantial). Splitting Go into two services demonstrates microservice decomposition within a single language — service boundaries, shared infrastructure, and independent deployability.

**Why not more services?** Two is the minimum to demonstrate the pattern. Three or more would add deployment complexity without teaching anything new. The auth/ecommerce boundary is a natural seam: auth is a stable, rarely-changing service; ecommerce evolves frequently.

### Shared JWT Secret, No Inter-Service Calls

Both services share the same JWT signing secret. Auth-service issues tokens; ecommerce-service validates them locally using the same secret. No service-to-service HTTP call is needed for authentication.

```
User → auth-service: POST /auth/login → JWT token
User → ecommerce-service: GET /cart (Bearer token) → validated locally
```

**Why not token validation via auth-service?** Adding an HTTP call on every authenticated request adds latency, creates a runtime dependency (ecommerce-service can't function if auth-service is down), and complicates the deployment. Stateless JWT validation is the standard pattern for microservices — the token is self-contained.

**Trade-off:** If a user is deleted from auth-service, their existing tokens remain valid until expiry (15 minutes). For a portfolio project, this is acceptable. A production system might add token revocation via a shared blacklist in Redis.

### Gin Framework

The job description lists Gin first among Go frameworks. Gin was chosen over alternatives:

| Framework | Pros | Cons | Verdict |
|-----------|------|------|---------|
| **Gin** | Most popular, large ecosystem, fast | More "magic" than stdlib | **Chosen** — matches job listing |
| Chi | Idiomatic, composes with net/http | Smaller ecosystem | Good alternative |
| Echo | Similar to Gin | Less popular | No advantage |
| net/http | Zero dependencies, most idiomatic | More boilerplate | Underkill for this scope |

### pgx Over database/sql

pgx is the Go Postgres driver that talks the PostgreSQL wire protocol directly, bypassing `database/sql`. Benefits:

- **Connection pooling** via `pgxpool` — built-in, no separate pool library needed
- **PostgreSQL-native types** — UUID, JSONB, arrays without custom scanners
- **Better performance** — avoids the `database/sql` abstraction overhead
- **Context-aware** — all operations accept `context.Context` for cancellation

**Why not GORM/sqlx?** The job description emphasizes "optimizing queries and data models." Hand-written SQL with pgx demonstrates SQL proficiency better than an ORM. The repository pattern with explicit queries is idiomatic Go — no struct tag magic, no query builder DSL, just SQL.

### Prices as Integers (Cents)

All prices are stored as integers representing cents (`7999` = $79.99). This avoids floating-point arithmetic issues that plague `float64` and `DECIMAL` types in currency calculations. The frontend formats cents to dollars for display.

### RabbitMQ Worker Pool for Order Processing

When a user checks out, the order is created immediately (status: `pending`) and the response is returned. A message is published to RabbitMQ, and a pool of worker goroutines processes orders asynchronously:

```
POST /orders → create order (pending) → publish to RabbitMQ → return 201
                                              ↓
                                     worker goroutine picks up
                                              ↓
                                     validate stock → decrement inventory
                                              ↓
                                     status: completed (or failed)
```

**Why not process synchronously?** The checkout endpoint would block while validating stock and updating inventory. Under load, this creates contention on the products table. Async processing decouples the user-facing latency from the backend work.

**Why workers in the same process?** A separate worker binary would be the "pure" microservice approach, but adds a third deployment, a third Dockerfile, and a third K8s manifest for ~50 lines of code. Workers in the same process still demonstrate goroutines, channels, context cancellation, and graceful shutdown — all the concurrency patterns the job asks about.

**Worker pool pattern:**
- Configurable concurrency (default 3 goroutines)
- Each worker blocks on channel receive, processes one message at a time
- Ack on success, nack with requeue on transient failure
- Graceful shutdown via `context.WithCancel` — workers drain on SIGTERM

### Redis Caching Strategy

| What | Key Pattern | TTL | Invalidation |
|------|-------------|-----|-------------|
| Product listings | `ecom:products:list:<params>` | 5min | On stock change (order worker) |
| Single product | `ecom:product:<id>` | 5min | On stock change |
| Categories | `ecom:categories` | 30min | Rarely changes |

**Cache invalidation approach:** The order processor worker calls `InvalidateCache()` after decrementing stock. This is a simple "bust everything" approach — scan and delete all `ecom:products:*` keys. For 20 products this is fine. A production system with millions of products would use targeted invalidation or event-driven cache updates.

**Redis is optional:** The service starts and operates without Redis (cache operations are nil-checked). This simplifies testing and local development.

### Observability

Prometheus metrics are exposed via Gin middleware on both services:

- `http_requests_total{method, path, status}` — request counter
- `http_request_duration_seconds{method, path}` — latency histogram
- `orders_total{status}` — business metric (completed/failed)
- `rabbitmq_messages_processed_total{result}` — worker health (success/error)

Structured logging via Go's `slog` (stdlib) in JSON format. Each request gets a UUID request ID propagated via `X-Request-ID` header.

### Shared Infrastructure

Both Go services share existing infrastructure from the Java stack rather than deploying duplicates:

| Resource | Shared From | Access Method |
|----------|------------|---------------|
| PostgreSQL | `java-tasks` namespace | Cross-namespace DNS (`postgres.java-tasks.svc.cluster.local`) |
| Redis | `java-tasks` namespace | Cross-namespace DNS |
| RabbitMQ | `java-tasks` namespace | Cross-namespace DNS |
| Prometheus | `monitoring` namespace | New scrape target |

The Go services use a separate database (`ecommercedb`) on the same PostgreSQL instance, and prefix Redis keys with `ecom:` to avoid collisions with Java services.

### Testing Strategy

Three levels, matching what the job description asks for:

- **Unit tests** — Service layer with mock repository interfaces. Handler tests with `httptest` + Gin test context. 16 tests total across both services.
- **Integration tests** — Repository layer against real Postgres (future, using testcontainers).
- **Benchmark tests** — `go test -bench` on auth (bcrypt-dominated: ~45ms/op) and product listing (~150ns/op). Demonstrates Go benchmark proficiency.

## Consequences

**Positive:**
- Demonstrates all key skills from the job description in a single cohesive project
- Two Go services show microservice decomposition without over-engineering
- Shared JWT secret keeps auth simple while maintaining service independence
- RabbitMQ worker pool demonstrates real concurrency patterns (goroutines, channels, graceful shutdown)
- Redis caching is optional — service works without it, reducing deployment friction
- Reuses existing infrastructure — no new Postgres/Redis/RabbitMQ instances to manage

**Trade-offs:**
- Shared JWT secret means token revocation requires a separate mechanism (Redis blacklist)
- Workers in the same process means they scale together — can't independently scale order processing
- golangci-lint needed locally to catch lint errors before pushing (added to pre-commit hooks)
- Cross-namespace K8s DNS for shared infrastructure couples the Go namespace to `java-tasks` — if the Java stack is removed, Go services need their own infra
