# QA Follow-Up Answer Handoffs

## TL;DR

The QA files under `docs/interview-prep/qa` contain repeated placeholder
follow-up answers:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Replace those placeholders with real interview-ready answers. Do not rewrite
the already-good parent answers unless a follow-up answer needs a small
alignment fix.

## Answer Standard

Each follow-up answer should be a quoted `Fast answer:` block, usually 2-5
sentences and suitable to say out loud in 20-45 seconds.

Use the role-map pattern:

1. Define or directly answer the follow-up.
2. Explain the production tradeoff.
3. Name a failure mode.
4. Tie it to this repo with a specific anchor.
5. Mention how to test, observe, or verify it when useful.

Avoid generic filler. A good answer should mention concrete mechanics such as
`context.Context`, pgx pools, RabbitMQ, Kafka, idempotency keys, circuit
breakers, OpenTelemetry, structured errors, tool schemas, or `go test -race`
when those are actually relevant.

## Global Rules For Agents

- Keep the existing markdown structure.
- Replace only placeholder follow-up answer text unless the nearby anchor is
  clearly wrong.
- Keep answers concise and spoken, not textbook essays.
- Use repo-specific references from the top `Repo Anchors` list in each file.
- Do not invent implementation details that are not present in the repo.
- If unsure about a repo anchor, search narrowly with `rg` before writing it.
- After editing a file, run:

```bash
rg -n "A strong answer should extend the parent answer" docs/interview-prep/qa/<file>
```

The command should return no matches for the file assigned to that handoff.

## Handoff 02: Go Language Fundamentals

Source file: `docs/interview-prep/qa/02-go-language-fundamentals.md`

Goal: Replace 48 placeholder follow-up answers with concrete Go fundamentals
answers tied to this repo's Go services and shared packages.

Primary anchors:

- `go/pkg/apperror`
- `go/pkg/resilience`
- `go/pkg/tracing`
- `go/ai-service/internal/agent/agent.go`
- `go/ai-service/internal/tools/registry.go`
- `go/*-service/internal/handler`
- `go/*-service/internal/repository`

Sections to complete:

- `What do you like about Go for backend services?`: cover Go weaknesses
  versus Java/Python, explicit error/concurrency/dependency handling, service
  package structure, and avoiding interface overuse.
- `Arrays versus slices?`: cover `len`/`cap`, `append` allocation, retained
  backing arrays, and array copy cost.
- `How do maps work under concurrency?`: cover `sync.Map`, sharding, nil maps,
  and defensive copies before returning maps.
- `How do interfaces work in Go?`: cover pointer/value receivers, nil
  interface traps, consumer-side interfaces, and tests with fakes.
- `How should errors be handled in Go?`: cover `errors.Is` versus `errors.As`,
  wrapping, panic boundaries, and secret-safe error responses.
- `How do you use context.Context?`: cover context values, deadline-aware
  retries, cancellation leaks, and first-parameter convention.
- `Goroutines versus channels versus mutexes?`: cover ownership decisions,
  goroutine leaks, buffered/unbuffered channels, and channel close semantics.
- `What should you know about Go memory and GC?`: cover escape analysis,
  stack/heap, pointer scan cost, and memory growth investigation.
- `How do you write tests in Go?`: cover table tests, mocks versus fakes,
  middleware tests, and race tests.
- `How do generics fit into Go?`: cover generics versus interfaces, overkill,
  constraints, and readability.
- `How do you structure a Go service?`: cover `internal`, `cmd`, circular
  dependencies, and shared code boundaries.
- `What makes a Go API production-grade?`: cover server timeouts, logs, health
  checks, and easy-to-miss failure paths.

Verification:

```bash
rg -n "A strong answer should extend the parent answer" docs/interview-prep/qa/02-go-language-fundamentals.md
```

## Handoff 03: REST API And Gateway Questions

Source file: `docs/interview-prep/qa/03-rest-api-gateway-questions.md`

Goal: Replace 48 placeholder follow-up answers with practical REST, gateway,
streaming, security, integration, and handler-testing answers.

Primary anchors:

- Go cart/order/payment service handlers and middleware.
- Shared `go/pkg/apperror`, tracing, resilience, auth, metrics, TLS packages.
- API gateway behavior in frontend-facing routes.
- pgx-backed repositories and idempotency patterns.

Sections to complete:

- `How do you design a good REST API?`: status codes, POST versus PUT,
  versioning, and client compatibility.
- `What belongs in API gateway middleware?`: middleware boundaries, ordering,
  early response writes, and middleware tests.
- `How do you make POST requests safe under retries?`: idempotency key storage,
  TTLs, lost responses, and body mismatch handling.
- `How do you design rate limiting?`: token bucket versus fixed window,
  per-user/global limits, memory bounds, and client response headers.
- `How should API errors be shaped?`: 400 versus 422, safe details, validation
  field errors, and request IDs.
- `How do you design pagination for large collections?`: offset versus cursor,
  stable sorting, metadata, and created-time pagination.
- `How do you handle API versioning?`: URI/header tradeoffs, retirement,
  event schema versions, and backward-compatibility tests.
- `What is the role of an API gateway in microservices?`: gateway versus BFF,
  failure modes, bottleneck prevention, and authorization placement.
- `How do you stream responses from an API?`: SSE/WebSocket, post-header
  errors, client disconnects, and streaming timeouts.
- `How do you secure a REST API?`: authn/authz, JWT/session tradeoffs,
  tenant isolation, and log redaction.
- `How do you integrate a third-party API behind REST endpoints?`: retryable
  errors, rate limits, fake-provider tests, and caching.
- `How do you test REST handlers and middleware?`: unit/integration boundary,
  idempotency tests, auth tests, and streaming tests.

Verification:

```bash
rg -n "A strong answer should extend the parent answer" docs/interview-prep/qa/03-rest-api-gateway-questions.md
```

## Handoff 04: Third-Party Integrations

Source file: `docs/interview-prep/qa/04-third-party-integrations.md`

Goal: Replace 48 placeholder follow-up answers with integration answers that
sound operational: timeouts, retries, idempotency, webhooks, credentials,
provider abstraction, and local consistency.

Primary anchors:

- `go/payment-service` for Stripe-like payment flows and idempotency.
- `go/pkg/resilience` for retries and circuit breakers.
- `go/ai-service/internal/tools/clients` and provider abstractions.
- RabbitMQ/Kafka and saga/outbox patterns where local/external consistency is
  discussed.

Sections to complete:

- `How do you design a robust third-party API client?`: client/service split,
  fake-provider tests, logs, and provider error parsing.
- `Which third-party API errors should you retry?`: 408, 429, retry budgets,
  and retry amplification.
- `How do idempotency keys apply to external APIs?`: key derivation, provider
  retention, parameter mismatch, and refunds.
- `How do you handle webhooks safely?`: raw-body signatures, duplicates,
  status codes on failure, and replay.
- `How do you prevent provider outages from cascading?`: retry versus breaker,
  breaker thresholds, user-facing degraded responses, and alerts.
- `How do context timeouts and HTTP client timeouts work together?`: timeout
  creation, retry contexts, client disconnects, and timeout tests.
- `How do you translate provider errors into API errors?`: safe exposure,
  client versus dependency errors, unavailable status, and log root cause.
- `How do you test third-party integrations?`: contract tests, sandbox use,
  rate-limit simulation, and no-sleep retry tests.
- `How do you manage credentials for third-party APIs?`: rotation, redaction,
  environment separation, and leak response.
- `How do you integrate multiple LLM providers?`: tool-call normalization,
  streaming differences, failover, and provider/model evals.
- `How do you handle external API rate limits?`: 429 retry decisions,
  user-facing requests, synchronized retry avoidance, and jitter.
- `How do you keep local state consistent with an external provider?`: local
  success/provider failure, provider success/local failure, outbox, and source
  of truth.

Verification:

```bash
rg -n "A strong answer should extend the parent answer" docs/interview-prep/qa/04-third-party-integrations.md
```

## Handoff 05: Distributed Systems And Scalability

Source file: `docs/interview-prep/qa/05-distributed-systems-scalability.md`

Goal: Replace 48 placeholder follow-up answers with grounded distributed
systems answers for sagas, consistency, messages, outbox, retries, breakers,
backpressure, scaling, tracing, late events, and shutdown.

Primary anchors:

- Checkout/order saga flow.
- RabbitMQ consumers and duplicate message handling.
- Kafka analytics pipeline.
- `go/pkg/resilience`, tracing, metrics, and Kubernetes readiness/shutdown
  behavior.

Sections to complete:

- `How do you handle transactions across multiple microservices?`: 2PC
  avoidance, compensation, saga state, and recovery.
- `What is eventual consistency, and when is it acceptable?`: pending states,
  stuck workflow detection, out-of-order events, and non-convergence.
- `How do you make message consumers reliable?`: ack timing, poison messages,
  duplicate side effects, and horizontal scaling.
- `What is the outbox pattern?`: exactly-once limits, duplicate publishes,
  ordering, and poller metrics.
- `How do retries and idempotency work together?`: retryable operations,
  non-retryable work, retry limits, and retry storms.
- `How do circuit breakers help distributed systems?`: open causes, half-open
  state, placement, and user-facing behavior.
- `How do you design for backpressure?`: producer/consumer mismatch, worker
  pool sizing, Kafka lag, and drop/delay/reject decisions.
- `How do you scale a Go service horizontally?`: non-memory state,
  zero-downtime deployments, readiness/liveness, and autoscaling metrics.
- `How do you debug high latency in a distributed system?`: missing traces,
  queue latency, retries hiding root causes, and p99 interpretation.
- `How do you preserve trace context across async boundaries?`: async tracing
  difficulty, propagated headers, missing context, and log fields.
- `How do you handle late or out-of-order events?`: event time, watermarks,
  double-count prevention, and late-event dropping.
- `How do you design graceful shutdown?`: in-flight requests, consumers,
  timeout fallback, and Kubernetes readiness before termination.

Verification:

```bash
rg -n "A strong answer should extend the parent answer" docs/interview-prep/qa/05-distributed-systems-scalability.md
```

## Handoff 06: AI Agent Systems

Source file: `docs/interview-prep/qa/06-ai-agent-systems.md`

Goal: Replace 40 placeholder follow-up answers with backend-focused AI agent
answers: contracts, bounded loops, tool registry behavior, RAG, streaming,
errors, guardrails, evals, observability, and provider abstraction.

Primary anchors:

- `go/ai-service/internal/agent/agent.go`
- `go/ai-service/internal/tools/registry.go`
- RAG bridge/service integration.
- Streaming events, safe tool execution, provider abstraction, and eval tests.

Sections to complete:

- `How does a tool-calling agent work?`: model state versus backend state,
  schema importance, multiple tool calls, and client/model return differences.
- `How do you prevent runaway agent loops?`: max step behavior, choosing step
  count, disconnect handling, and tool versus LLM failure.
- `What is a tool registry?`: tool contract versioning, new-tool tests,
  unknown tools, and compact tool output.
- `How do you integrate RAG into an agent?`: model context, citations, bad RAG
  retries, and retrieval downtime.
- `How do you stream agent responses?`: SSE/WebSocket, tool errors after
  streaming starts, disconnect detection, and write timeout choices.
- `How do you handle tool errors?`: turn-stopping errors, model-visible detail,
  secret leakage prevention, and panic recovery tests.
- `What guardrails belong in an AI gateway?`: pre-LLM enforcement, post-LLM
  enforcement, anonymous users, and sensitive prompt logging.
- `How do you evaluate an agent?`: unit tests versus evals, quality metrics,
  tool-choice regressions, and tests without Ollama.
- `How do you observe an agent in production?`: alerts, high p99 latency,
  flaky tool detection, and frontend-to-backend trace correlation.
- `Why use a provider abstraction?`: provider interface scope, token metrics,
  model switching, and timeout tests.

Verification:

```bash
rg -n "A strong answer should extend the parent answer" docs/interview-prep/qa/06-ai-agent-systems.md
```

## Handoff 07: Database, Observability, And Security

Source file: `docs/interview-prep/qa/07-database-observability-security.md`

Goal: Replace 40 placeholder follow-up answers with concrete answers around
schema design, SQL optimization, migrations, pgx/PgBouncer, retryable DB
errors, access security, health checks, structured errors, telemetry, and
backup/recovery.

Primary anchors:

- pgx repositories and pool configuration in Go services.
- PostgreSQL migrations and migration roles.
- PgBouncer/Kubernetes database runtime behavior.
- `go/pkg/apperror`, tracing, metrics, and health checks.

Sections to complete:

- `How do you design a database schema for a backend service?`: primary keys,
  unique constraints, denormalization, and workflow state.
- `How do you optimize a slow SQL query?`: `EXPLAIN ANALYZE`, unused indexes,
  write cost, and `ILIKE` search.
- `How do you handle migrations safely?`: risky index creation, rollback,
  expand-contract migrations, and direct migration DB URLs.
- `How do pgx pools and PgBouncer fit together?`: pool sizing, exhaustion,
  transaction/session pooling, and `application_name`.
- `What database errors are retryable?`: serialization failures, transaction
  retry scope, retry storms, and retry-pressure metrics.
- `How do you secure database access?`: SQL injection prevention, unsafe logs,
  app versus migration roles, and credential rotation.
- `What belongs in a health check?`: readiness failure, liveness DB checks,
  rolling deploy impact, and probe herd avoidance.
- `How do structured errors help security and operations?`: 422 responses,
  request IDs, safe client details, and middleware tests.
- `How do traces, logs, and metrics work together?`: span attributes, metric
  versus log decisions, async tracing, and sampled-out traces.
- `How do you design backups and WAL recovery?`: RPO/RTO, point-in-time
  recovery, restore testing, and backup access control.

Verification:

```bash
rg -n "A strong answer should extend the parent answer" docs/interview-prep/qa/07-database-observability-security.md
```

## Handoff 09: Go Performance And Concurrency Drills

Source file: `docs/interview-prep/qa/09-go-performance-and-concurrency-drills.md`

Goal: Replace 35 placeholder follow-up answers with detailed spoken answers for
the reported micro1-style performance scenario: maps, sharding, pipelines, GC,
leaks, rate limiting, deadlines, and retries.

Primary anchors:

- `go/pkg/resilience`
- concurrent Go service handlers and consumers.
- analytics/Kafka consumers.
- HTTP streaming and external-call clients.
- `go test -race`, benchmarks, pprof, and metrics.

Sections to complete:

- `10,000 goroutines need to read and write a shared map`: `sync.Map`
  drawbacks, shard count, hot shards, panic while holding lock, and clean
  shutdown.
- `Design a producer/consumer pipeline in Go with no race conditions`: channel
  ownership, worker errors, leak prevention, slow consumers, and metrics.
- `How would you shard a concurrent map?`: resizing, hot keys, consistent
  hashing, shard cleanup, and concurrent correctness tests.
- `How does Go garbage collection matter for backend latency?`: escape
  analysis, retained arrays, `sync.Pool`, pointer-heavy data, and proof via
  pprof/metrics.
- `How do you prevent goroutine leaks?`: leak tests, HTTP client resource
  leaks, context cancellation versus channel close, streaming shutdown, and
  worker errors.
- `How would you implement a safe rate limiter?`: token bucket versus fixed
  window, key growth, Redis outage behavior, gateway placement, and tests.
- `How do context deadlines affect external API calls?`: timeout placement,
  per-retry contexts, trace context, retry classification, and idempotency.

Verification:

```bash
rg -n "A strong answer should extend the parent answer" docs/interview-prep/qa/09-go-performance-and-concurrency-drills.md
```

## Handoff 10: Mock Interview Drills

Source file: `docs/interview-prep/qa/10-mock-interview-drills.md`

Goal: Replace 40 placeholder follow-up answers with polished spoken responses
that sound like a candidate answering live. These should be less textbook than
the topic files and more portfolio-specific.

Primary anchors:

- The decomposed Go ecommerce services.
- Shared packages for errors, tracing, resilience, TLS, middleware, metrics,
  and migrations.
- RabbitMQ checkout saga, Kafka analytics, AI agent gateway, pgx/PostgreSQL,
  and Kubernetes deployment behavior.

Sections to complete:

- `Mock 1: Backend System Walkthrough`: service split reasoning,
  production-like components, 10x traffic changes, and service-boundary tests.
- `Mock 2: Heavy-Traffic REST API`: POST idempotency, monitored metrics, DB
  pool exhaustion, and API versioning.
- `Mock 3: 10,000 Goroutines Accessing A Map`: mutex versus `sync.Map`,
  sharding, leak prevention, and producer/consumer backpressure.
- `Mock 4: Distributed Checkout`: payment success/order failure, duplicate
  messages, saga state, and stuck-saga alerts.
- `Mock 5: AI Agent Gateway`: runaway loops, tool failures, agent evaluation,
  and gateway guardrails.
- `Mock 6: SQL Optimization`: index write cost, pagination optimization, pool
  exhaustion detection, and safe new-index migration.
- `Mock 7: Difficult Bug`: root-cause discovery, test changes, monitoring
  changes, and what to do differently.
- `Mock 8: Remote Independent Work`: vague requirements, progress updates,
  asking for help, and avoiding overengineering.
- `Mock 9: Technical Tradeoff Communication`: acceptable tradeoff evidence,
  risk callouts, eventual consistency explanation, and choosing simplicity.
- `Mock 10: Go GC And Arrays`: array versus slice, memory retention detection,
  escape analysis, and allocation reduction.

Verification:

```bash
rg -n "A strong answer should extend the parent answer" docs/interview-prep/qa/10-mock-interview-drills.md
```

## Suggested Parallelization

These handoffs are independent and can be assigned to separate agents:

- Agent A: `02-go-language-fundamentals.md`
- Agent B: `03-rest-api-gateway-questions.md`
- Agent C: `04-third-party-integrations.md`
- Agent D: `05-distributed-systems-scalability.md`
- Agent E: `06-ai-agent-systems.md`
- Agent F: `07-database-observability-security.md`
- Agent G: `09-go-performance-and-concurrency-drills.md`
- Agent H: `10-mock-interview-drills.md`

If fewer agents are available, start with `10`, `02`, `09`, and `03`; those are
the highest-leverage files for a Go backend interview.

## Final Verification

After all handoffs are complete, run:

```bash
rg -n "A strong answer should extend the parent answer" docs/interview-prep/qa
```

Expected result: no matches in `docs/interview-prep/qa`.
