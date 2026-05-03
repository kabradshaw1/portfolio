# Mock Interview Drills

Use these as full spoken practice loops. Keep the first answer under 60 seconds,
then expect follow-ups.

## Mock 1: Backend System Walkthrough

Prompt:

> Tell me about a backend system you built.

60-second answer:

> I built a portfolio backend with decomposed Go services for auth, product,
> cart, order, payment, analytics, and AI functionality. The services use REST
> for frontend traffic, gRPC where service-to-service contracts make sense,
> PostgreSQL for persistence, RabbitMQ for checkout saga workflows, Kafka for
> analytics events, and shared Go packages for errors, tracing, resilience, and
> TLS. The production focus was the important part: migrations, connection
> pools, timeouts, retries, circuit breakers, idempotency, health checks,
> metrics, traces, Dockerfiles, and Kubernetes manifests.

Follow-ups:

- Why split those services?
- Which part is most production-like?
- What would you change if traffic increased 10x?
- How did you test the service boundaries?

## Mock 2: Heavy-Traffic REST API

Prompt:

> Design a REST API that handles high traffic safely.

60-second answer:

> I would start with resource-oriented endpoints, validation, auth, stable error
> envelopes, pagination, and explicit rate limits. For high traffic, the main
> concerns are bounding work per request, using connection pools correctly,
> avoiding unbounded result sets, caching safe reads, and making mutating
> endpoints idempotent under retries. In this repo, cart and order services use
> middleware for errors, metrics, auth, rate limits, and idempotency, while DB
> access goes through tuned pgx pools.

Follow-ups:

- How do you make POST idempotent?
- What metrics would you monitor?
- How do you handle database pool exhaustion?
- How do you version the API?

## Mock 3: 10,000 Goroutines Accessing A Map

Prompt:

> What happens if 10,000 goroutines access a Go map?

60-second answer:

> Concurrent reads can be okay only if there are no writes, but concurrent writes
> or read-write access to a normal map is unsafe and can panic or race. The
> first fix is a map protected by `sync.RWMutex`. If contention is high, I would
> shard the map by key hash so different keys use different locks. I would also
> bound goroutine creation with a worker pool if the goroutines represent work,
> pass context for cancellation, and run tests with `go test -race`.

Follow-ups:

- Mutex map versus `sync.Map`?
- How would sharding work?
- How do you prevent goroutine leaks?
- How would you design producer/consumer backpressure?

## Mock 4: Distributed Checkout

Prompt:

> How would you design checkout across cart, order, and payment services?

60-second answer:

> I would not use a distributed transaction by default. I would use a saga:
> create an order in a pending state, reserve cart items, create or confirm
> payment, then mark the order complete. Each step needs idempotent handlers,
> retry rules, and compensation, such as releasing cart reservation if payment
> fails. The repo has checkout saga components, RabbitMQ commands/events,
> payment webhooks, and an outbox so database changes and emitted events do not
> get separated by a crash.

Follow-ups:

- What if payment succeeds but order update fails?
- How do you handle duplicate messages?
- Where is saga state stored?
- What alerts catch stuck sagas?

## Mock 5: AI Agent Gateway

Prompt:

> Explain the AI agent system in this repo.

60-second answer:

> The Go AI service acts as a gateway for AI functionality. It runs a bounded
> ReAct-style loop: send messages and tool schemas to the LLM, execute requested
> tools, append tool results, and continue until a final answer or max-step
> error. It streams events over SSE so clients see tool calls, results, errors,
> and final output. Tools cover ecommerce and RAG use cases, and the RAG client
> calls Python services through typed HTTP clients with timeouts, retries,
> circuit breakers, and tracing.

Follow-ups:

- How do you stop runaway tool loops?
- What happens when a tool fails?
- How do you evaluate the agent?
- What guardrails are in the gateway?

## Mock 6: SQL Optimization

Prompt:

> A database query is slow. How do you debug it?

60-second answer:

> I start with evidence: query latency metrics, slow query logs, and
> `EXPLAIN ANALYZE` for the exact query. I check whether it scans too many rows,
> sorts inefficiently, misses an index, waits on locks, or suffers from pool
> exhaustion. Then I match indexes to access patterns and verify the plan
> changed. In this repo, examples include pagination indexes, saga-step indexes,
> constraints, and a trigram GIN index for product-name search.

Follow-ups:

- When can an index hurt?
- How do you optimize pagination?
- How do you detect pool exhaustion?
- What is a safe migration path for a new index?

## Mock 7: Difficult Bug

Prompt:

> Tell me about a difficult bug.

60-second answer:

> I would frame a distributed or concurrency bug: the symptom, the signal, the
> root cause, the fix, and the prevention. For example, in a saga or webhook
> flow, the hard bug is usually duplicate or out-of-order delivery. The fix is
> not just retrying; it is idempotency, explicit state transitions, processed
> event tracking, and observability so duplicates become expected behavior. I
> would mention adding tests and metrics so the same class of bug is visible
> earlier next time.

Follow-ups:

- How did you find the root cause?
- What did you change in tests?
- What did you change in monitoring?
- What would you do differently now?

## Mock 8: Remote Independent Work

Prompt:

> How do you work independently in a remote environment?

60-second answer:

> I work best by making decisions visible early: write down assumptions, keep
> scope focused, push small reviewable changes, and include tests and operational
> notes with the code. For ambiguous work, I clarify the success criteria and
> then move with a conservative implementation. This repo reflects that style:
> area instructions, service boundaries, migrations, CI preflights, observability
> conventions, and docs that make the system easier for another engineer to
> operate.

Follow-ups:

- How do you handle vague requirements?
- How often do you communicate progress?
- How do you ask for help?
- How do you avoid overengineering?

## Mock 9: Technical Tradeoff Communication

Prompt:

> Explain a technical tradeoff to a non-specialist.

60-second answer:

> I start with the user or business impact, then explain the engineering options
> in terms of risk, cost, and reliability. For example, with checkout consistency,
> a distributed transaction sounds simple but can reduce availability and couple
> services tightly. A saga accepts temporary pending states but is more resilient
> if each step is idempotent and observable. I would recommend the saga when the
> business can tolerate short-lived pending states and needs services to fail
> independently.

Follow-ups:

- How do you know the tradeoff is acceptable?
- What risks would you call out?
- How do you explain eventual consistency?
- When would you choose the simpler option?

## Mock 10: Go GC And Arrays

Prompt:

> What should a Go developer know about GC with arrays and slices?

60-second answer:

> Arrays are values, and slices are small descriptors pointing to an underlying
> array. The GC cares about what remains reachable. A small slice can keep a
> large backing array alive, so if I take a tiny slice from a huge buffer and
> store it long-term, I may retain much more memory than expected. The fix is to
> copy the needed bytes or elements into a new smaller allocation when retaining
> them beyond the short-lived operation.

Follow-ups:

- Array versus slice?
- How do you detect memory retention?
- What does escape analysis tell you?
- How do you reduce allocations?
