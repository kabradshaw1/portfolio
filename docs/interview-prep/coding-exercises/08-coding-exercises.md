# Timed Coding Exercises

Use these for 20-45 minute practice. For each exercise, rehearse the design out
loud for 60 seconds before coding: data structures, concurrency, cancellation,
error handling, tests, and edge cases.

## Repo Anchors

- `go/ai-service/internal/agent/agent.go`: bounded loop, context timeout, tool
  calls, tool errors, and max-step handling.
- `go/ai-service/internal/tools/registry.go`: simple interface plus map-backed
  registry.
- `go/cart-service/internal/middleware/idempotency.go`: idempotent mutating
  request pattern.
- `go/payment-service/internal/outbox/poller.go`: outbox polling and publish
  loop.
- `go/order-service/internal/saga`: saga state transitions, recovery, and
  compensation.
- `go/pkg/resilience`: retries, retry classification, and circuit breakers.
- `go/pkg/tracing`: context propagation across HTTP, AMQP, Kafka, and Redis.

## Exercise Set

### 1. Concurrent map with race prevention

Prompt:

> Build a thread-safe counter map that supports `Inc(key)`, `Get(key)`, and
> `Snapshot()` with 10,000 goroutines incrementing keys.

Fast design:

> A native map is not safe for concurrent writes. The simplest answer is a
> struct with `sync.RWMutex` and a map. For high contention, shard by hash into
> N maps, each with its own lock.

Follow-ups:

- Why does a normal map race?
- `sync.Map` versus mutex map?
- How would you shard it?
- How do you test with `-race`?

### 2. Sharded cache

Prompt:

> Implement a cache with 32 shards, `Get`, `Set`, `Delete`, and TTL expiration.

Fast design:

> Hash the key to choose a shard. Each shard owns a map and lock. Store value
> plus expiration time. On `Get`, delete expired entries lazily. Use a cleanup
> goroutine only if cancellation and shutdown are explicit.

Follow-ups:

- Why shard?
- How do you avoid leaked cleanup goroutines?
- How do you size shards?
- What happens during `Snapshot`?

### 3. Worker pool with cancellation

Prompt:

> Write a worker pool that processes jobs from a channel, stops on context
> cancellation, returns results, and does not leak goroutines.

Fast design:

> Use a parent context, bounded job channel, result channel, and `sync.WaitGroup`.
> Workers `select` on `ctx.Done()` and jobs. A closer goroutine closes results
> after all workers exit.

Follow-ups:

- Who closes the jobs channel?
- How do you propagate the first error?
- How do you bound memory?
- How do you prove no goroutine leaks?

### 4. Producer/consumer with backpressure

Prompt:

> Design a producer that reads events and consumers that process them without
> unbounded memory growth.

Fast design:

> Use a bounded channel. If consumers are slower, producers block, shed load, or
> return a retryable error depending on business priority. Add context
> cancellation and metrics for queue depth, processing latency, and dropped
> events.

Follow-ups:

- Block, drop, or reject?
- How many workers?
- How do you handle poison jobs?
- How do you shut down cleanly?

### 5. Retryable HTTP client

Prompt:

> Implement `DoWithRetry(ctx, req, maxAttempts)` for retryable HTTP failures.

Fast design:

> Clone the request per attempt, respect context, retry network errors, 429, and
> 5xx, use exponential backoff with jitter, and do not retry unsafe side effects
> unless the caller supplies idempotency.

Follow-ups:

- Which statuses are retryable?
- How do you retry a request body?
- How do you honor `Retry-After`?
- How do you avoid retry storms?

### 6. Circuit breaker wrapper

Prompt:

> Build a minimal circuit breaker with closed, open, and half-open states.

Fast design:

> Track consecutive failures. Open after a threshold. While open, fail fast until
> a cooldown expires. Allow a limited half-open probe; close on success, reopen
> on failure.

Follow-ups:

- Retry versus circuit breaker?
- What should be counted as a failure?
- How do you make it concurrency-safe?
- What metrics should it expose?

### 7. Idempotent POST endpoint

Prompt:

> Implement a POST handler that uses an `Idempotency-Key` to prevent duplicate
> side effects.

Fast design:

> Require a key for mutating requests. Store pending/completed state keyed by
> user, route, and idempotency key. If completed, return the stored response. If
> pending, return conflict or retry-later. Include request-body fingerprinting
> to reject key reuse with a different body.

Follow-ups:

- How long do keys live?
- What if the first response is lost?
- What if processing crashes while pending?
- What storage do you use across replicas?

### 8. Webhook receiver

Prompt:

> Build a webhook endpoint that verifies signatures, handles duplicates, and
> writes a domain event.

Fast design:

> Read the raw body, verify timestamp and HMAC/signature before JSON parsing,
> reject replays outside a time window, store provider event IDs in a unique
> processed-events table, and write domain changes in one transaction.

Follow-ups:

- Why verify before parsing?
- How do you handle duplicate webhooks?
- What do you return on unknown event types?
- Where does the outbox fit?

### 9. Outbox poller

Prompt:

> Implement a poller that publishes unpublished outbox rows and marks them
> published.

Fast design:

> Fetch a small batch ordered by creation time, publish each message with
> context, mark published only after publish succeeds, and retry later on
> failure. Use locking or `SKIP LOCKED` if multiple pollers run.

Follow-ups:

- Does this guarantee exactly-once?
- How do you avoid duplicate publishes?
- What metrics matter?
- How do you stop the poller?

### 10. Saga state machine

Prompt:

> Model checkout states and valid transitions for reserve cart, create payment,
> confirm order, release cart, and fail order.

Fast design:

> Represent states as constants and transitions as a table or switch. Validate
> every transition, make handlers idempotent for repeated events, and define
> compensation states for failures after partial success.

Follow-ups:

- Where do you store state?
- What is compensation?
- How do you recover stuck sagas?
- How do you handle out-of-order events?

### 11. Token bucket rate limiter

Prompt:

> Implement an in-memory token bucket limiter with `Allow(key)` for many users.

Fast design:

> Store tokens and last-refill time per key behind a mutex. Refill based on
> elapsed time, cap at burst size, consume one token per allowed request, and
> periodically evict idle keys.

Follow-ups:

- Fixed window versus token bucket?
- How do you make it distributed?
- Fail open or fail closed?
- How do you avoid unbounded key growth?

### 12. Cursor pagination

Prompt:

> Implement cursor pagination for `created_at DESC, id DESC`.

Fast design:

> Fetch `limit + 1` rows. The next cursor encodes the last returned row's
> `created_at` and `id`. The next query filters with
> `(created_at, id) < ($cursorTime, $cursorID)` and uses the same order.

Follow-ups:

- Why include `id` in the cursor?
- Why fetch `limit + 1`?
- Offset versus cursor?
- How do you encode the cursor safely?

### 13. Simple AI tool registry

Prompt:

> Implement a tool registry with `Register`, `Get`, `Schemas`, and duplicate
> name validation.

Fast design:

> Define a `Tool` interface with name, description, schema, and call method.
> Store implementations in a map by name. Return schemas as a slice for the LLM.
> Tests should cover unknown tools, duplicate names, and schema output.

Follow-ups:

- How do you version tools?
- What if schema JSON is invalid?
- How do you isolate tool panics?
- What should a tool return?

### 14. ReAct-style loop with max steps

Prompt:

> Implement an agent loop against a fake LLM and fake tools. Stop on final
> answer, feed tool results back into messages, and return max-step error.

Fast design:

> Loop up to N steps. Call the LLM with messages and schemas. If no tool calls,
> emit final. For each tool call, resolve the tool, execute it with context,
> append a tool result message, and continue. Tool errors become model-visible
> results; context errors stop the turn.

Follow-ups:

- What is the event stream?
- What errors stop the loop?
- How do you test deterministically?
- How do you prevent infinite loops?

### 15. LRU cache

Prompt:

> Implement an LRU cache with `Get` and `Put` in O(1).

Fast design:

> Use a hash map from key to list node and a doubly linked list ordered by
> recency. `Get` moves the node to the front. `Put` updates or inserts at the
> front. If capacity is exceeded, remove the tail and delete from the map.

Follow-ups:

- Why map plus linked list?
- What happens at capacity zero?
- How do you make it thread-safe?
- How would TTL change the design?

## 60-Second Coding Opening

Use this template before writing code:

> I will define the contract first, then pick the simplest data structure that
> meets the concurrency and latency requirements. I will pass `context.Context`
> through blocking or external calls, bound queues or loops, and make ownership
> of channel closing explicit. For tests, I will cover the happy path, edge
> cases, cancellation, and race-prone behavior where concurrency is involved.

## Common Mistakes To Avoid

- Writing to a Go map from many goroutines without a lock.
- Starting background goroutines without a stop path.
- Retrying POST side effects without idempotency.
- Forgetting to close result channels after workers exit.
- Ignoring context cancellation inside loops.
- Returning raw internal errors to clients.
- Building unbounded queues.
- Claiming exactly-once delivery when the design is at-least-once plus
  idempotency.
