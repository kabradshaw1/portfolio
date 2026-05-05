# Go Language Fundamentals Rehearsal

Use this for Go basics, language mechanics, standard library judgment, and
short coding follow-ups. Keep answers concrete and tie them back to backend
code in this repo.

## Repo Anchors

- `go/pkg/apperror`: custom error types, wrapping, `errors.As`, structured HTTP
  responses, and safe internal error handling.
- `go/pkg/resilience`: generic retry/circuit-breaker wrappers, retry
  classification, and context-aware calls.
- `go/pkg/tracing`: `context.Context` propagation through HTTP, AMQP, Kafka,
  Redis, and logs.
- `go/ai-service/internal/agent/agent.go`: interfaces, context timeouts,
  slices of messages, JSON handling, safe panic recovery, and named errors.
- `go/ai-service/internal/tools/registry.go`: interface design, map-backed
  registry, JSON schemas, and result structs.
- `go/*-service/internal/handler`: Gin handlers, request validation, dependency
  injection, and typed responses.
- `go/*-service/internal/repository`: pgx usage, context-aware database calls,
  transactions, and error mapping.

## High-Frequency Questions

### 1. What do you like about Go for backend services?

Fast answer:

> Go is simple, explicit, and strong for network services. Goroutines and
> channels make concurrency accessible, the standard library has solid HTTP,
> JSON, context, and testing support, and static binaries fit container
> deployments well. The tradeoff is that Go pushes architecture discipline onto
> the developer: clear interfaces, explicit error handling, bounded concurrency,
> and avoiding overuse of globals. This repo uses Go for decomposed REST/gRPC
> services, shared resilience, tracing, and an AI-service gateway.

Follow-ups:

#### Follow-up: Where is Go weaker than Java or Python?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: What does Go force you to be explicit about?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How do you structure a medium-sized Go service?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How do you avoid overengineering interfaces?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

### 2. Arrays versus slices?

Fast answer:

> An array has a fixed length and is a value. A slice is a small descriptor with
> pointer, length, and capacity pointing at an underlying array. Appending may
> reuse the backing array or allocate a new one. The GC impact is important: a
> small retained slice can keep a large backing array alive. If I need to keep a
> small part of a large buffer long-term, I copy it into a new smaller slice.

Follow-ups:

#### Follow-up: What does `len` versus `cap` mean?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: When does `append` allocate?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How can a small slice retain a large array?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: Why can passing arrays be expensive?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

### 3. How do maps work under concurrency?

Fast answer:

> A Go map is not safe for concurrent writes, or concurrent read/write access,
> without synchronization. I use `sync.RWMutex` for a normal map, sharding for
> high-contention key spaces, `sync.Map` for specialized cases like mostly
> read-heavy or write-once patterns, or a single owner goroutine for serialized
> access. I verify with `go test -race`.

Follow-ups:

#### Follow-up: When is `sync.Map` appropriate?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How do you shard a map?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: What is a map zero value?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: Why copy a map before returning it?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

### 4. How do interfaces work in Go?

Fast answer:

> Interfaces are satisfied implicitly by method set. I prefer small interfaces
> at the consumer boundary, not huge provider-side abstractions. That keeps code
> easy to test and swap. In this repo, the AI agent depends on an `llm.Client`
> interface and a `tools.Registry`, and HTTP handlers accept narrow runner or
> service dependencies instead of constructing everything internally.

Follow-ups:

#### Follow-up: Pointer receiver versus value receiver?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: What is a nil interface trap?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: Where should interfaces live?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How do interfaces help tests?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

### 5. How should errors be handled in Go?

Fast answer:

> Errors are values, so I handle them explicitly at each boundary. I wrap
> unexpected lower-level errors with context, map expected conditions to typed
> application errors, and avoid leaking internals to clients. In this repo,
> `apperror.AppError` gives stable codes and HTTP statuses, and middleware
> returns safe JSON while logging unknown internal errors.

Follow-ups:

#### Follow-up: `errors.Is` versus `errors.As`?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: When do you wrap an error?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: Panic versus error?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How do you avoid exposing secrets in errors?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

### 6. How do you use `context.Context`?

Fast answer:

> Context carries cancellation, deadlines, and request-scoped values across API
> boundaries. I pass it as the first argument to work that can block: HTTP,
> database, queues, external calls, and long loops. I do not store context in a
> struct for later use. In this repo, agent turns, RAG calls, database calls,
> tracing, and graceful shutdown all rely on context propagation.

Follow-ups:

#### Follow-up: What belongs in context values?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How do deadlines interact with retries?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How do you avoid context leaks?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: Why should context be the first parameter?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

### 7. Goroutines versus channels versus mutexes?

Fast answer:

> Goroutines are execution. Channels are communication and coordination. Mutexes
> protect shared memory. I choose based on ownership: if one goroutine should own
> state, channels can serialize commands; if many goroutines need direct access
> to shared data, a mutex is usually simpler. The interview trap is using
> channels for everything when a lock is clearer.

Follow-ups:

#### Follow-up: When is a channel better than a mutex?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How do channels leak goroutines?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: Buffered versus unbuffered channels?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: What does closing a channel mean?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

### 8. What should you know about Go memory and GC?

Fast answer:

> Go has a concurrent garbage collector optimized for low pauses, but allocation
> rate and pointer-heavy structures still affect CPU and tail latency. I profile
> before optimizing, reduce avoidable allocations in hot paths, preallocate
> slices when sizes are known, avoid retaining large backing arrays, and use
> `sync.Pool` only for proven hot reusable buffers.

Follow-ups:

#### Follow-up: What is escape analysis?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: Stack versus heap allocation?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How do pointers affect GC scan work?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How do you investigate memory growth?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

### 9. How do you write tests in Go?

Fast answer:

> I use table-driven tests for input/output behavior, small fakes for
> dependencies, `httptest` for handlers, and integration tests only where the
> real dependency matters. For concurrency, I use cancellation, timeouts, and the
> race detector. In this repo, service tests use fake clients and handlers,
> agent evals use a scripted LLM, and database behavior is covered with targeted
> integration tests.

Follow-ups:

#### Follow-up: Table tests versus separate tests?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: Mock versus fake?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How do you test HTTP middleware?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How do you test races?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

### 10. How do generics fit into Go?

Fast answer:

> Generics are useful when the operation is truly type-agnostic and avoids
> repetitive boilerplate, but I do not reach for them for business logic. In
> backend code, a good use is a shared resilience wrapper that can call a
> dependency and return a typed result without losing type safety. I still keep
> interfaces small and concrete code readable.

Follow-ups:

#### Follow-up: Generics versus interfaces?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: When would generics be overkill?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: What constraints would you define?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How do generics affect readability?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

### 11. How do you structure a Go service?

Fast answer:

> I keep the executable wiring in `cmd/server`, business logic in internal
> packages, transport handlers separate from services and repositories, and
> shared reusable code in a package that has a clear contract. Dependencies are
> injected through constructors so tests can swap fakes. This repo follows that
> pattern across Go services with handlers, repositories, migrations, shared
> `go/pkg` packages, and service-local `internal` code.

Follow-ups:

#### Follow-up: What belongs in `internal`?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: What belongs in `cmd`?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How do you avoid circular dependencies?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How do you share code between services?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

### 12. What makes a Go API production-grade?

Fast answer:

> A production-grade API has explicit timeouts, validation, auth, structured
> errors, pagination, idempotency for unsafe retries, rate limits, metrics,
> traces, logs, health checks, graceful shutdown, and tests around failure paths.
> Go makes these straightforward, but you still have to wire them consistently.
> The repo's Go services use shared packages for errors, resilience, tracing,
> TLS, and operational behavior.

Follow-ups:

#### Follow-up: What server timeouts do you set?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: What do you include in logs?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: What should health checks cover?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: What failure path is easiest to miss?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

## Scenario Drills

### Scenario: A handler returns a slice from an internal cache.

Answer outline:

> I would avoid returning a mutable internal slice directly because callers can
> change shared state and a small slice may retain a large backing array. Return
> a copy if ownership crosses a boundary, especially for cached or long-lived
> data. If performance matters, benchmark and document ownership clearly.

### Scenario: A dependency function sometimes panics.

Answer outline:

> Panics should not be normal control flow. At service boundaries, recover to
> keep the process alive, log the panic, and return a safe error. Then fix the
> root cause with validation or nil checks. The AI agent's `safeCall` is a good
> repo anchor: tool panics become tool errors instead of process crashes.

### Scenario: A retry loop ignores context cancellation.

Answer outline:

> That can keep work running after the client is gone and amplify load during
> dependency failures. Each attempt and backoff wait should select on
> `ctx.Done()`, and all retries should fit within the original deadline. The
> repo's resilience and client patterns are good anchors for bounded external
> calls.

## Quick Flashcards

- Slice: descriptor over backing array: pointer, length, capacity.
- Map: not safe for concurrent writes without synchronization.
- Interface: implicit contract based on method set.
- Context: cancellation, deadline, request scope; first parameter.
- Error wrapping: preserve cause while adding boundary context.
- Panic: exceptional; recover at process or plugin/tool boundary.
- Channel close: sender-owned signal that no more values will arrive.
- Race detector: use `go test -race` for concurrency confidence.
- GC risk: high allocation rate, pointer-heavy data, retained backing arrays.
- Production API: timeouts, validation, auth, errors, observability, shutdown.
