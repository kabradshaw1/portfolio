# Portfolio Recall Matrix

Use this file to turn generic interview questions into concrete evidence from
the repo.

## Fast Pitch

I built a portfolio backend with decomposed Go services, REST and gRPC
boundaries, event-driven ecommerce flows, resilience helpers, observability,
containerized deployment, and an AI-service gateway that exposes agent-style
tool calling over ecommerce and RAG tools.

## Requirement Mapping

| Job Requirement | Repo Evidence | Fast Answer |
| --- | --- | --- |
| Architect backend systems in Go | `go/auth-service`, `go/cart-service`, `go/order-service`, `go/payment-service`, `go/analytics-service`, `go/ai-service` | I have built decomposed Go services with clear service boundaries, HTTP/gRPC interfaces, database persistence, tests, Dockerfiles, and Kubernetes manifests. |
| REST API design | Cart/order/payment REST handlers and middleware | I design APIs around resources, validation, consistent error envelopes, auth, rate limiting, idempotency where retries can duplicate work, and observability at the handler boundary. |
| API gateway / edge concerns | `go/ai-service` gateway, middleware, ingress/K8s manifests | I think of the gateway as the place for routing, auth, request shaping, streaming, rate limits, cross-cutting telemetry, and isolating clients from internal service topology. |
| Third-party API integration | Payment service, Stripe-style checkout/webhook flow, AI service client bridges | I wrap external APIs with typed clients, context timeouts, retry classification, idempotency, structured errors, and webhook verification where applicable. |
| Distributed systems | Order saga, RabbitMQ checkout flow, Kafka analytics events | I use asynchronous messaging when a workflow crosses service boundaries, and I design for retries, idempotency, compensation, and eventual consistency. |
| Scalability | Kafka analytics consumer, Redis-backed analytics store, rate limiting, DB pool configuration | I scale by reducing synchronous coupling, using queues/streams, controlling concurrency, tuning pools, adding caching where useful, and measuring p95/p99 latency. |
| Reliability | `go/pkg/resilience`, health checks, readiness/liveness probes, PDBs | I use timeouts, retries, circuit breakers, graceful shutdown, health checks, and deployment safety mechanisms so failures degrade instead of cascading. |
| Observability | `go/pkg/tracing`, Prometheus metrics, Jaeger/Loki/Grafana manifests | I instrument services with structured logs, metrics, traces, request IDs, and dependency-specific spans so debugging is based on evidence. |
| AI agent exposure | `go/ai-service` ReAct-style agent loop, Ollama tool calling, RAG bridge | I have backend exposure to agent-style systems: tool calls, streaming events, ecommerce tools, RAG clients, and integration boundaries between Go and Python AI services. |
| Remote collaboration / documentation | AGENTS files, CI workflows, K8s manifests, docs discipline | I write code and docs so another engineer can operate the system: clear boundaries, tests, deployment manifests, and operational conventions. |

## Rehearsal Answers

### Tell me about a Go backend system you built.

Fast answer:

> I built a decomposed Go ecommerce backend with auth, cart, order, payment,
> analytics, and AI-service components. The services use REST for frontend
> traffic, gRPC for service-to-service calls where useful, PostgreSQL for
> persistence, RabbitMQ/Kafka for asynchronous flows, and shared packages for
> resilience, tracing, TLS, and error handling. I focused on production concerns:
> timeouts, health checks, migrations, metrics, tests, and Kubernetes manifests.

Likely follow-ups:

- Why split the services?
- Where did you use async messaging?
- How did you avoid cascading failures?
- How did you test service boundaries?

### Tell me about a distributed workflow you designed.

Fast answer:

> The ecommerce checkout flow is the best example. Ordering, cart reservation,
> payment, and projection concerns are separated, so the design has to handle
> partial failure. I would describe it as saga-oriented: each step needs clear
> state, retry behavior, and compensation where the workflow cannot complete.
> The important design point is that retries must be idempotent and observable,
> because in distributed systems duplicate messages and delayed responses are
> normal conditions, not edge cases.

Likely follow-ups:

- Why not use a distributed transaction?
- How do you handle duplicate messages?
- What happens if payment succeeds but the order update fails?
- What metrics would you alert on?

### Tell me about your AI agent experience.

Fast answer:

> The repo includes a Go AI-service gateway that fronts ecommerce tools and RAG
> tools through a ReAct-style agent loop. It streams events such as tool calls,
> tool results, final answers, and errors, and it bridges to Python AI services
> for RAG. My main backend focus is making the agent integration reliable:
> typed tool contracts, timeouts, clear error states, trace propagation, and
> guardrails around external calls.

Likely follow-ups:

- How do you prevent runaway tool loops?
- How do you represent tool errors to the model and the client?
- How would you evaluate an agent in production?
- What belongs in Go versus Python?

### How do you handle third-party API failures?

Fast answer:

> I start with context deadlines and clear typed errors. Then I classify failures
> into retryable and non-retryable categories. Retryable calls get bounded
> exponential backoff, and side-effecting calls need idempotency keys or a
> transaction/outbox pattern. I also expose metrics for latency, error class,
> retry count, and circuit-breaker state so an integration failure is visible
> before it becomes a user-facing outage.

Likely follow-ups:

- Which HTTP status codes would you retry?
- How do you handle rate limits?
- Where do webhook verification and replay protection fit?
- What do you return to clients when the dependency is down?

### How do you debug high latency across services?

Fast answer:

> I start by separating client latency, gateway latency, service latency,
> database latency, and external dependency latency. I use traces to find the
> slow span, metrics to see whether it is systemic or isolated, and logs with
> correlation IDs to inspect representative requests. Then I check saturation:
> connection pools, goroutine counts, queue lag, lock contention, retry storms,
> and downstream error rates.

Likely follow-ups:

- What if traces are missing?
- How do you know whether retries are making it worse?
- How do you distinguish DB slowness from pool exhaustion?
- What would you change first?

