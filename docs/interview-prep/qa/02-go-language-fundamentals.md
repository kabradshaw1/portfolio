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

> Go is weaker when you want a large batteries-included framework like Spring,
> or very fast iteration around data-heavy scripting like Python. The tradeoff
> is that Go gives fewer architectural rails, so service boundaries, validation,
> dependency injection, and error contracts have to be designed explicitly. In
> this repo that shows up in the shared `go/pkg` packages: errors, tracing, and
> resilience are deliberate libraries, not framework magic.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: What does Go force you to be explicit about?

Fast answer:

> Go forces explicit error checks, dependency wiring, context propagation, and
> concurrency ownership. That is good for production services because hidden
> control flow is rare, but it means boilerplate can creep in if shared patterns
> are not clean. In this repo, handlers pass context into repositories and
> clients, and `apperror` makes expected failure modes explicit at API
> boundaries.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How do you structure a medium-sized Go service?

Fast answer:

> I keep startup and dependency wiring in `cmd`, service-specific code under
> `internal`, and reusable cross-service code under a small shared package with
> a stable contract. Handlers should translate HTTP, services should hold
> business behavior, and repositories should isolate database access. The Go
> services in this repo follow that shape with handlers, repositories,
> migrations, and shared packages for errors, tracing, TLS, and resilience.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How do you avoid overengineering interfaces?

Fast answer:

> I introduce interfaces at the consumer boundary when they make testing or
> substitution real, not just because a type exists. A one-method dependency
> used by a handler is a good candidate; a speculative provider-wide interface
> is usually noise. In this repo, the AI agent benefits from small interfaces
> for the LLM client and tool registry, while plain data structs stay concrete.

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

> `len` is how many elements are currently in the slice; `cap` is how many
> elements fit before `append` must allocate a new backing array. The production
> issue is avoiding accidental allocations in hot paths and avoiding oversized
> retained buffers. For handler responses or agent message slices, I would
> preallocate when the expected size is known, but I would not micro-optimize
> until a profile shows allocation pressure.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: When does `append` allocate?

Fast answer:

> `append` allocates when the existing slice capacity cannot hold the new
> elements, or when the compiler/runtime must move to a new backing array. That
> matters because any other slice pointing at the old array will not see the new
> array, and extra allocations can show up in p99 latency. In repo code that
> builds response lists or agent message histories, I would treat append as
> safe but not free, and test behavior instead of relying on shared backing
> arrays.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How can a small slice retain a large array?

Fast answer:

> A slice points into a backing array, so keeping a tiny subslice alive can keep
> the entire original array reachable to the GC. The failure mode is memory
> growth that looks like a leak even though references are technically valid. If
> a handler or client parses a large payload and needs to keep only a small
> field, I would copy the field into a new slice or string-sized buffer.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: Why can passing arrays be expensive?

Fast answer:

> Arrays are values in Go, so passing an array copies the whole array unless you
> pass a pointer. That can be expensive for large buffers and can surprise
> people coming from slice-heavy code. In backend services I normally pass
> slices for variable data and reserve arrays for fixed-size values like hashes
> or small protocol fields where value semantics are intentional.

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

> `sync.Map` is appropriate for specialized patterns: many reads, write-once
> keys, or disjoint keys updated by many goroutines. It is not a general
> replacement for `map` plus `RWMutex` because it gives up type safety and can
> be harder to reason about. For a service-local cache or registry in this repo,
> I would start with a normal map protected by ownership or a mutex unless the
> access pattern proved `sync.Map` was better.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How do you shard a map?

Fast answer:

> I hash the key to pick one of several shards, and each shard owns its own map
> and lock. That reduces contention compared with one global lock, but it adds
> complexity around shard count, hot keys, and iteration across all data. I
> would use it only for a measured high-contention path, then verify with the
> race detector and benchmarks under realistic key distribution.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: What is a map zero value?

Fast answer:

> The zero value of a map is `nil`. You can read from it and get zero values,
> but writing to it panics. That matters in constructors and request decoding:
> if a repository or registry expects to add entries, it should initialize the
> map with `make`, while a nil map can still be useful to represent no optional
> metadata in a response.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: Why copy a map before returning it?

Fast answer:

> A map is a reference type, so returning an internal map gives the caller the
> ability to mutate your state and create data races. Copying preserves
> ownership at the boundary. For something like a tool registry or cached
> metadata map, I would return a snapshot so callers can inspect it without
> bypassing the registry's synchronization or invariants.

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

> A value receiver copies the value and is fine for small immutable structs. A
> pointer receiver avoids copying, can mutate the receiver, and is required when
> methods need shared state. I keep the method set consistent for a type; for
> services, clients, repositories, and registries in this repo, pointer
> receivers are usually the right default because they hold dependencies or
> state.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: What is a nil interface trap?

Fast answer:

> An interface value is only nil when both its dynamic type and dynamic value
> are nil. If you store a nil pointer of a concrete type in an interface, the
> interface itself is non-nil, which can make checks like `if err != nil`
> misleading when custom error types are involved. In this repo's error
> handling, constructors should return a plain nil `error` on success and avoid
> wrapping typed nils.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: Where should interfaces live?

Fast answer:

> Interfaces should usually live with the consumer because the consumer knows
> the small behavior it needs. The provider can stay concrete and not predict
> every possible abstraction. In this repo, a handler or agent package should
> define a narrow dependency interface when it needs a fake in tests, while the
> actual client or repository implementation can remain a concrete type.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How do interfaces help tests?

Fast answer:

> Interfaces let tests replace slow or nondeterministic dependencies with small
> fakes. That keeps handler, service, and agent tests focused on behavior rather
> than a live database, LLM, queue, or third-party API. In this repo, the agent
> can be tested with a scripted LLM client and fake tools, while HTTP handlers
> can exercise validation and error mapping without constructing the whole
> service graph.

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

> `errors.Is` answers whether an error chain contains a specific sentinel or
> equivalent error. `errors.As` extracts a specific error type from the chain.
> In this repo, `errors.As` is useful for finding an `apperror.AppError` and
> mapping it to a stable HTTP response, while `errors.Is` is better for checks
> like context cancellation or a named domain sentinel.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: When do you wrap an error?

Fast answer:

> I wrap unexpected lower-level errors when crossing a boundary and the added
> context helps debugging: for example, which dependency call or repository
> operation failed. I avoid wrapping when the caller needs an exact public
> contract or when the error is already a typed application error. The pattern
> in this repo is to preserve root cause for logs and traces while returning a
> safe `apperror` response to clients.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: Panic versus error?

Fast answer:

> Errors are for expected failure paths: bad input, unavailable dependencies,
> timeouts, and domain conflicts. Panics are for programmer bugs or impossible
> states, and they should be recovered only at a boundary that can safely
> isolate the failure. The AI agent's tool execution is a good example: a tool
> panic should become a tool error for that turn, not crash the whole service.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How do you avoid exposing secrets in errors?

Fast answer:

> I separate internal error detail from client-facing messages. Logs can include
> safe structured context like request ID, dependency name, and error class, but
> not tokens, passwords, raw prompts, payment data, or connection strings. In
> this repo, `apperror` gives clients stable codes and messages while internal
> causes can stay in logs and traces behind access controls.

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

> Context values should be request-scoped metadata that crosses API boundaries,
> like trace IDs, request IDs, auth claims, or logger fields. They should not be
> used for optional function parameters or long-lived dependencies. In this
> repo, tracing propagation belongs in context; database pools, clients, and
> service configuration belong on structs injected at startup.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How do deadlines interact with retries?

Fast answer:

> Retries should fit inside the original context deadline, not reset the user's
> budget on every attempt. Each attempt and backoff should stop when
> `ctx.Done()` fires, otherwise retries keep working after the caller has gone
> away and can amplify an outage. That is why the repo's resilience wrappers
> need retry classification plus context-aware waits.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How do you avoid context leaks?

Fast answer:

> I call the cancel function for derived contexts, make goroutines select on
> `ctx.Done()`, and avoid storing request contexts for later background work.
> The failure mode is leaked goroutines, open network calls, or database work
> continuing after a request is over. For agent turns, streams, and external
> calls in this repo, cancellation has to propagate all the way to the LLM,
> tools, RAG, and repository calls.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: Why should context be the first parameter?

Fast answer:

> Making context the first parameter is Go convention and makes cancellation
> visible at every blocking boundary. It also keeps APIs consistent: `Do(ctx,
> req)`, `Find(ctx, id)`, or `Call(ctx, input)` immediately tells the caller
> that the operation can time out or be canceled. This matters across the repo's
> handlers, repositories, queue consumers, tracing helpers, and external
> clients.

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

> A channel is better when one goroutine should own state or when you are
> coordinating work, cancellation, or fan-out/fan-in. A mutex is better when
> multiple goroutines need simple protected access to shared memory. For a queue
> consumer pipeline in this repo, channels are natural for jobs and shutdown
> signals; for a small in-memory registry, a mutex is usually clearer.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How do channels leak goroutines?

Fast answer:

> A goroutine leaks when it waits forever on a send or receive that no one will
> complete. Common causes are unclosed job channels, missing context
> cancellation, unbounded producer work, or returning early without draining
> results. In repo code that streams agent events or runs workers, I would
> include `ctx.Done()` in selects and make ownership of channel closing
> explicit.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: Buffered versus unbuffered channels?

Fast answer:

> An unbuffered channel synchronizes sender and receiver at the handoff. A
> buffered channel decouples them up to a fixed capacity, which can absorb
> bursts but can also hide backpressure until the buffer fills. For backend
> workers, I treat the buffer size as part of the resource limit and expose
> queue depth or lag as a metric when the path matters.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: What does closing a channel mean?

Fast answer:

> Closing a channel means no more values will be sent; it is a sender-owned
> signal, not a receiver cleanup tool. Receivers can range until the channel is
> drained, and sends after close panic. In a producer/consumer path, the
> producer closes the jobs channel when input is complete, while cancellation
> should normally flow through context.

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

> Escape analysis is the compiler's decision about whether a value can stay on
> the stack or must move to the heap. Heap allocation is not automatically bad,
> but high allocation rates increase GC work and can affect tail latency. If a
> hot handler or agent loop showed allocation pressure, I would inspect compiler
> escape output and confirm the impact with benchmarks or pprof.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: Stack versus heap allocation?

Fast answer:

> Stack allocation is cheap and tied to a goroutine's call lifetime. Heap
> allocation is needed when data outlives the call or must be shared, but then
> the GC has to track it. In service code, I care less about forcing everything
> onto the stack and more about avoiding accidental long-lived allocations in
> hot paths like request handling, streaming, and large JSON processing.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How do pointers affect GC scan work?

Fast answer:

> The GC has to scan live pointers to find more live objects. Pointer-heavy
> graphs, maps of pointers, and retained object chains can increase scan work
> even when total bytes are not huge. In backend code I try to keep hot data
> structures simple, avoid unnecessary pointer fields, and validate with pprof
> before changing layouts for performance.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How do you investigate memory growth?

Fast answer:

> I first distinguish expected cache growth from an actual leak, then look at
> heap profiles, allocation profiles, goroutine profiles, and object retention.
> I would check for retained slices, unbounded maps, missed response body
> closes, and leaked goroutines. In this repo, long-running consumers, agent
> streams, and service-local caches are the places I would inspect first.

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

> Table tests are good when the setup and assertion shape are the same across
> cases, like validation, error mapping, or retry classification. Separate tests
> are clearer when behavior has different setup, concurrency, or multi-step
> state. In this repo, I would use table tests for `apperror` mapping and
> handler validation, but separate tests for saga flows, streaming, or
> cancellation behavior.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: Mock versus fake?

Fast answer:

> A mock usually asserts specific calls; a fake is a small working
> implementation with controlled behavior. I prefer fakes when the behavior is
> what matters because tests are less brittle. For this repo, a fake LLM client,
> fake tool registry, or fake repository can drive success, timeout, and error
> paths without tying the test to incidental call order.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How do you test HTTP middleware?

Fast answer:

> I test middleware with `httptest`, a minimal handler after the middleware, and
> cases for both pass-through and short-circuit behavior. For error middleware,
> I assert the status, response envelope, request ID, and that internal detail
> is not exposed. In this repo that is important for auth, rate limits,
> structured errors, tracing, and metrics middleware.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How do you test races?

Fast answer:

> I design the test so multiple goroutines actually hit the shared path, use
> timeouts so failures do not hang, and run it with `go test -race`. The race
> detector finds unsafe memory access, but it does not prove the logic is
> correct, so I still assert invariants like final counts, no duplicate side
> effects, and clean shutdown. That is relevant for maps, workers, streams, and
> queue consumers.

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

> Generics are for algorithms or wrappers that are the same across types while
> preserving concrete type safety. Interfaces are for behavior and substitution.
> In this repo, a generic resilience function can call any dependency and
> return a typed result, while an `llm.Client` interface describes behavior the
> agent needs.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: When would generics be overkill?

Fast answer:

> Generics are overkill when there are only one or two call sites, when the type
> parameter makes the code harder to read, or when the variation is actually
> business behavior. In service code, clarity usually beats clever reuse. I
> would not make handlers, repositories, or domain services generic unless
> there is real repeated type-agnostic logic like retrying a typed operation.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: What constraints would you define?

Fast answer:

> I would define the narrowest constraint that supports the operation. If a
> function only needs any result type, `any` may be enough; if it needs ordering
> or string behavior, the constraint should say exactly that. For shared repo
> helpers like resilience wrappers, broad constraints keep the API useful, while
> domain-specific constraints usually signal the abstraction is drifting into
> business logic.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How do generics affect readability?

Fast answer:

> Generics improve readability when they remove duplicated boilerplate without
> hiding the control flow. They hurt readability when every call requires
> decoding type parameters, constraints, and helper layers. In a portfolio repo,
> I would rather show a small, obvious generic helper in `go/pkg/resilience`
> than spread clever generic abstractions through business services.

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

> `internal` is for code that belongs to a service or module and should not be
> imported as a public library by unrelated packages. That includes handlers,
> services, repositories, clients, and domain logic specific to that service.
> In this repo, service-local `internal` packages keep auth, cart, order, AI,
> and analytics implementation details from becoming accidental shared APIs.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: What belongs in `cmd`?

Fast answer:

> `cmd` should be thin executable wiring: load config, construct dependencies,
> register routes or consumers, start servers, and handle shutdown. Business
> logic should not live there because it becomes hard to test without running
> the process. In this repo, `cmd/server` should compose handlers,
> repositories, tracing, metrics, and clients, then delegate behavior to
> internal packages.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How do you avoid circular dependencies?

Fast answer:

> I keep dependencies pointing inward: handlers depend on services, services
> depend on repositories or clients through narrow interfaces, and shared
> packages do not import service-specific code. If two packages need each other,
> that usually means a boundary is wrong or a small type should move to a lower
> package. In this repo, shared concerns like errors and tracing belong in
> `go/pkg`, while service behavior stays under each service's `internal`.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: How do you share code between services?

Fast answer:

> I share code only when the contract is stable and genuinely cross-cutting:
> structured errors, tracing, resilience, TLS, metrics, or generated protobuf
> types. I avoid sharing business logic just to reduce duplication because it
> couples service evolution. This repo's `go/pkg` is the right place for
> operational primitives; order-specific or payment-specific behavior should
> stay in those services.

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

> I set read header timeouts, read timeouts, write timeouts, idle timeouts, and
> per-request context deadlines based on endpoint behavior. Streaming endpoints
> need special care because a normal write timeout can kill a valid long stream,
> so they need heartbeat or stream-specific settings. In this repo, REST
> handlers, AI streaming, and external dependency calls should all have bounded
> time budgets.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: What do you include in logs?

Fast answer:

> Logs should include fields that help correlate and debug: request ID, trace
> ID, route, status, latency, dependency name, error class, and safe domain
> identifiers. They should not include secrets, raw credentials, full tokens,
> or sensitive prompt/payment data. In this repo, logs should line up with
> traces and metrics so an API error can be followed into a repository,
> provider call, or tool execution.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: What should health checks cover?

Fast answer:

> Liveness should usually answer whether the process is alive and not wedged.
> Readiness should answer whether it can serve traffic, which may include
> database pool availability, required dependencies, migrations, or queue
> consumer readiness depending on the service. In Kubernetes, failing readiness
> during startup or shutdown prevents routing traffic before the service is
> actually safe to use.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

#### Follow-up: What failure path is easiest to miss?

Fast answer:

> The easy path to miss is partial failure after work has already started:
> client disconnects, lost responses after a successful write, timeouts during
> retries, or a dependency succeeding while the local update fails. Those paths
> need context cancellation, idempotency, transaction boundaries, and observable
> error handling. In this repo, checkout, external payment calls, agent tools,
> and streaming responses are the places I would test hardest.

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
