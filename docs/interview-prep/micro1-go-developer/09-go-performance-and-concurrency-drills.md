# Go Performance And Concurrency Drills

These are high-priority because role-specific interview reports mention Go GC,
arrays, 10,000 goroutines accessing maps, producer/consumer design, sharding,
context management, and leak prevention.

## Drill Format

For each prompt:

1. Answer out loud in 60-90 seconds.
2. Name the failure mode.
3. Give the production design.
4. Mention how you would test it.
5. Tie it back to this repo.

## Spoken Design Drills

### 1. 10,000 goroutines need to read and write a shared map. What do you do?

Fast answer:

> A plain Go map is not safe for concurrent reads and writes, so I need
> synchronization or a different ownership model. For simple cases I would use a
> `sync.RWMutex` around the map. For high throughput I would consider sharding
> the map across N buckets, each with its own lock, to reduce contention. If the
> workload is write-once/read-many or keys are mostly disjoint, `sync.Map` may be
> reasonable. Another option is a single owner goroutine receiving operations on
> a channel, which simplifies safety but can become a bottleneck. I would run
> `go test -race`, add benchmarks, and measure lock contention and p99 latency.

Follow-ups:

- When is `sync.Map` worse than `map` plus `RWMutex`?
- How do you choose shard count?
- How do you prevent hot shards?
- What happens if one goroutine panics while holding a lock?
- How do you shut this down cleanly?

Repo tie-in:

- Shared resilience helpers and analytics consumers need safe concurrency,
  bounded work, cancellation, and observability.

### 2. Design a producer/consumer pipeline in Go with no race conditions.

Fast answer:

> I would model producers writing jobs into a bounded channel and consumers
> reading from it with a shared `context.Context`. The bounded channel provides
> backpressure. Producers select on `ctx.Done()` and the jobs channel so they do
> not block forever. Workers stop when the context is canceled or the jobs
> channel is closed. Results and errors should go through separate channels or a
> synchronized collector. Shutdown has to be owned by one component so channels
> are not closed by multiple goroutines.

Follow-ups:

- Who closes the jobs channel?
- How do workers report errors?
- How do you avoid goroutine leaks?
- What if consumers are slower than producers?
- What metrics do you collect?

Repo tie-in:

- Analytics event consumption and saga/message processing require backpressure,
  cancellation, retries, and safe shutdown.

### 3. How would you shard a concurrent map?

Fast answer:

> I would hash the key to pick a shard, where each shard owns a regular map and
> its own lock. This lowers contention because unrelated keys do not all compete
> for one global mutex. The shard count should be fixed or carefully migrated,
> usually a power of two for cheap indexing. I would watch for hot keys, uneven
> shard distribution, lock wait time, and memory growth. If shards run worker
> goroutines, each shard needs a context and explicit shutdown path to avoid
> leaking goroutines.

Follow-ups:

- How do you resize shards?
- How do you handle hot keys?
- Would you use consistent hashing?
- How do you prevent shard leaks?
- How do you test concurrent correctness?

Repo tie-in:

- Redis/cache patterns, rate limits, idempotency stores, and analytics windows
  can all use sharding ideas under high contention.

### 4. How does Go garbage collection matter for backend latency?

Fast answer:

> Go has a concurrent, tri-color mark-and-sweep garbage collector. It is designed
> for low pauses, but allocation-heavy hot paths still increase CPU work and can
> hurt tail latency. Arrays and slices matter because slices can retain references
> to larger backing arrays, and arrays containing pointers increase the amount of
> memory the GC must scan. In backend code I reduce unnecessary allocations with
> preallocation, streaming decoders, avoiding accidental escapes, careful buffer
> reuse where appropriate, and profiling with pprof before optimizing.

Follow-ups:

- What is escape analysis?
- How can a small slice keep a large array alive?
- When is `sync.Pool` useful?
- Why can pointer-heavy structures increase GC work?
- How would you prove GC is causing latency?

Repo tie-in:

- REST handlers, analytics aggregation, and AI streaming paths should avoid
  avoidable allocation pressure on hot request paths.

### 5. How do you prevent goroutine leaks?

Fast answer:

> Every goroutine should have a clear owner, a stop condition, and a way to
> observe cancellation. I usually pass `context.Context`, avoid sends or receives
> that can block forever, use bounded channels, and coordinate shutdown with
> `errgroup`, `WaitGroup`, or explicit close semantics. Leaks often come from
> abandoned workers, blocked sends, unclosed response bodies, missing timeouts,
> or background loops that ignore cancellation.

Follow-ups:

- How do you detect leaks in tests?
- How do HTTP clients leak resources?
- What is the difference between canceling context and closing a channel?
- How do you shut down streaming responses?
- How do you handle worker errors?

Repo tie-in:

- Go services use graceful shutdown, HTTP timeouts, streaming AI responses, and
  message consumers where goroutine ownership matters.

### 6. How would you implement a safe rate limiter?

Fast answer:

> For a single process I would use token bucket or leaky bucket semantics keyed
> by user, IP, or API key. The limiter state needs synchronization, TTL cleanup,
> and bounded memory. For a distributed service I would move shared state to
> Redis or another atomic store, accept some approximation, and fail open or
> closed depending on the endpoint risk. I would expose metrics for allowed,
> limited, store failures, and latency.

Follow-ups:

- Token bucket versus fixed window?
- How do you avoid unbounded key growth?
- What happens if Redis is down?
- How does an API gateway change the design?
- How would you test it?

Repo tie-in:

- Cart service middleware and shared resilience patterns are good talking
  points for edge protections and failure behavior.

### 7. How do context deadlines affect external API calls?

Fast answer:

> Context deadlines put an upper bound on work. For third-party HTTP calls I
> combine client-level timeouts with per-request contexts, close response bodies,
> classify errors, and keep retries within the original deadline. I do not let
> retries create work after the caller has gone away. If a dependency is slow,
> the service should fail predictably, record metrics, and maybe trip a circuit
> breaker instead of exhausting workers or connection pools.

Follow-ups:

- Where do you set the timeout?
- Do you create a new context per retry?
- How do you preserve trace context?
- What do you retry?
- How does idempotency affect retries?

Repo tie-in:

- Payment/API clients, AI-service RAG/Ollama bridges, and resilience helpers all
  depend on deadlines and retry boundaries.

## Coding Exercises

### Exercise 1: Concurrent Counter Store

Time target: 25 minutes.

Build:

- `Increment(key string)`
- `Get(key string) int`
- `Snapshot() map[string]int`

Constraints:

- Safe under concurrent access.
- Include tests with `t.Parallel()` or goroutines.
- Mention how you would benchmark global lock versus sharded lock.

Expected discussion:

- `RWMutex` versus sharding.
- Copying snapshot to avoid exposing internal map.
- Race detector.

### Exercise 2: Worker Pool With Cancellation

Time target: 35 minutes.

Build:

- A worker pool that accepts jobs.
- Stops on context cancellation.
- Returns results and errors.
- Does not leak goroutines when producers or consumers stop early.

Expected discussion:

- Bounded channels.
- Single owner for channel close.
- `WaitGroup` or `errgroup`.
- Backpressure and timeout behavior.

### Exercise 3: Retryable HTTP Client

Time target: 40 minutes.

Build:

- `Do(ctx context.Context, req *http.Request) (*http.Response, error)`
- Retry on 429 and 5xx.
- Do not retry 400/401/403/404.
- Use exponential backoff with jitter.
- Stop when context is canceled.

Expected discussion:

- Request body replay.
- Idempotency keys.
- Retry budget.
- Metrics and tracing.

### Exercise 4: Sharded Idempotency Store

Time target: 45 minutes.

Build:

- `Start(key string) (started bool)`
- `Finish(key string, result Result)`
- `Get(key string) (Result, bool)`
- TTL cleanup can be described instead of fully implemented.

Expected discussion:

- Duplicate POST protection.
- Pending versus completed states.
- Lock granularity.
- Memory cleanup.

