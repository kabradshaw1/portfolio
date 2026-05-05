# REST API And API Gateway Rehearsal

Use this file for fast spoken answers. The target is 30-90 seconds per answer,
with one repo-backed example.

## Repo Anchors

- `go/cart-service/cmd/server/routes.go`: route composition with recovery,
  security headers, OpenTelemetry, logging, structured errors, metrics, CORS,
  auth, rate limiting, and idempotency.
- `go/cart-service/internal/middleware/idempotency.go`: Redis-backed
  `Idempotency-Key` handling for mutating requests.
- `go/cart-service/internal/middleware/ratelimit.go`: Redis-backed rate limit
  middleware.
- `go/pkg/apperror`: shared structured error envelope and validation errors.
- `go/order-service/internal/handler/product.go`: pagination, filtering, and
  cursor-style product list behavior.
- `go/ai-service/internal/http/chat.go`: SSE event stream for agent turns.
- `go/ai-service/internal/tools/clients/ecommerce.go`: gateway-style typed
  client calls to ecommerce services with resilience wrappers.

## High-Frequency Questions

### 1. How do you design a good REST API?

Fast answer:

> I start with resource-oriented routes, clear HTTP methods, predictable status
> codes, validation, consistent error bodies, pagination for collections, auth at
> the boundary, and observability on every request. I avoid exposing internal
> service topology to clients. In this repo, cart and order services expose REST
> endpoints through handler/service/repository layers, and shared middleware
> handles cross-cutting concerns like errors, metrics, auth, CORS, rate limiting,
> and idempotency.

Follow-ups:

#### Follow-up: How do you choose status codes?

Fast answer:

> I choose status codes by separating syntax, authorization, resource state,
> and dependency failures. Bad JSON or a missing `Idempotency-Key` is a 400,
> validation with field errors is a 422, duplicates in flight are 409, and
> rate limiting is 429 with retry guidance. The failure mode is returning
> everything as 500 or 200-with-error, because clients cannot make safe retry
> decisions. In this repo, `go/pkg/apperror` centralizes that envelope so
> handlers can return precise codes consistently.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: When do you use POST versus PUT?

Fast answer:

> I use POST when the server creates a subordinate resource or triggers an
> operation where the server owns the final identity, and PUT when the client
> is replacing a known resource at a stable URI. The tradeoff is idempotency:
> PUT is naturally retryable if the representation is complete, while POST
> needs an `Idempotency-Key` for safe retries. In this repo, mutating cart and
> order POST paths use Redis-backed idempotency middleware because
> checkout-style operations can have side effects.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you version an API?

Fast answer:

> I prefer additive evolution first, then explicit versioning when the
> response contract truly breaks. URI versions are easy for clients and
> gateways to reason about, while header versions keep URLs cleaner but are
> easier to miss in caches and tooling. The failure mode is changing field
> meaning under the same contract. In this repo, I would keep REST changes
> backward-compatible and apply the same discipline to protobuf and event
> schemas.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you avoid breaking clients?

Fast answer:

> I avoid breaking clients by making responses tolerant: add optional fields,
> keep old fields stable, and never repurpose a code or enum value silently.
> The production tradeoff is carrying old behavior longer, but that is usually
> cheaper than forcing synchronized deploys. I would lock that down with
> handler tests that assert status codes and `go/pkg/apperror` JSON shapes.
> For generated clients or protobuf contracts, I would add compatibility tests
> before removing anything.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

### 2. What belongs in API gateway middleware?

Fast answer:

> Gateway middleware should handle concerns that apply across routes: auth,
> request IDs, logging, tracing, metrics, rate limiting, CORS, security headers,
> panic recovery, and sometimes request/response shaping. Business logic should
> stay behind the gateway in handlers or services. In the cart service route
> setup, the middleware chain includes recovery, security headers, OTel, logging,
> structured errors, metrics, CORS, auth, rate limiting, and idempotency for
> mutating cart requests.

Follow-ups:

#### Follow-up: What should not be in middleware?

Fast answer:

> Middleware should not contain route-specific business decisions like cart
> quantity rules, payment state transitions, or product lookup semantics. Its
> job is cross-cutting policy: auth, tracing, metrics, CORS, rate limiting,
> idempotency, and error shaping. The failure mode is a hidden business
> workflow in the gateway that bypasses service tests. In this repo, route
> setup composes middleware while handlers and services own cart/order
> behavior.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you order middleware?

Fast answer:

> I order middleware from outer safety and observability toward route-specific
> controls: recovery and security headers first, then tracing/logging/metrics,
> then CORS/auth/rate limiting, then idempotency around mutating handlers. The
> tradeoff is that early middleware sees more failures and can attach request
> IDs or spans before later middleware rejects. If the order is wrong, aborted
> auth requests may miss metrics or panics may bypass structured errors. The
> cart and order route composition is the anchor I would inspect for this.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: What happens if auth middleware writes a response early?

Fast answer:

> Once auth writes a response and aborts the request, downstream handlers must
> not run and later middleware must not try to write a second JSON body. The
> failure mode is partial responses, duplicate writes, or side effects after a
> rejected request. In Gin, that means using `c.Abort()` after attaching a
> structured `apperror` or writing the response. Tests should assert that the
> downstream handler counter stays at zero on missing or invalid auth.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you test middleware?

Fast answer:

> I test middleware with a tiny in-memory router and a downstream handler that
> records whether it ran. Each test should cover pass-through, rejection,
> malformed inputs, headers, and the final JSON/status shape after
> `apperror.ErrorHandler` runs. The production tradeoff is catching ordering
> bugs without needing a full service stack. This repo already uses that style
> for idempotency, auth, and shared error middleware.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

### 3. How do you make POST requests safe under retries?

Fast answer:

> POST is not naturally idempotent, so a timeout followed by a retry can create
> duplicate side effects. I use an `Idempotency-Key` for client-generated request
> identity, store a processing or completed state, and return the original result
> for duplicate completed requests. While a request is still processing, I return
> a conflict or retry-later style response. This repo has Redis-backed
> idempotency middleware in cart and order flows.

Follow-ups:

#### Follow-up: What do you store for an idempotency key?

Fast answer:

> I store enough to prove request identity and safely replay the result: key
> scope, status, response status code, and response body, plus a short
> processing marker while the first request is still running. The production
> detail I would add for stricter APIs is a request-body hash to reject key
> reuse with different parameters. In this repo, cart and order idempotency
> stores `processing` and `done` entries in Redis under a user-scoped key.
> That prevents a retry from creating duplicate side effects after a timeout.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How long should the key live?

Fast answer:

> The TTL should cover the realistic retry window for clients and
> infrastructure, not become permanent state. In this repo, the processing
> marker is short, around 30 seconds, and completed responses live for 24
> hours. The tradeoff is storage cost versus protection from delayed retries.
> If the TTL is too short, a mobile client retry after a lost response can
> repeat the side effect; if it is too long, Redis fills with stale keys.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: What if the first request succeeds but the response is lost?

Fast answer:

> If the first request succeeds but the response is lost, the retry should
> return the cached original response, not run the operation again. That is
> the main reason to store the completed status code and body, not just a
> boolean. The failure mode is duplicate checkout, duplicate cart mutation, or
> duplicate external payment. In this repo, the idempotency middleware replays
> the cached `done` response for the same user and key.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: What if the same key is reused with a different body?

Fast answer:

> Reusing the same key with a different body should be rejected as a conflict
> or bad request, because otherwise the server might replay a response for the
> wrong operation. The usual implementation is storing a normalized request
> hash with the idempotency record. The current repo validates UUID keys and
> scopes them by user, and it caches status/body; for a stricter production
> API, I would add that body-hash check. The test would send the same key with
> two different payloads and expect no second side effect.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

### 4. How do you design rate limiting?

Fast answer:

> Rate limiting starts with the key: user ID, API key, IP, route, or tenant. For
> one process, an in-memory token bucket can work. For multiple replicas, I use a
> shared atomic store like Redis and make the failure policy explicit. Low-risk
> endpoints may fail open; sensitive endpoints may fail closed. The repo has
> Redis-backed rate limit middleware and AI guardrail rate limiting that fails
> open when Redis or the breaker is unavailable.

Follow-ups:

#### Follow-up: Token bucket versus fixed window?

Fast answer:

> Fixed window is simple and cheap: increment a Redis key and expire it after
> the window. Token bucket is smoother because it allows bursts while
> enforcing an average rate. The failure mode with fixed windows is boundary
> bursts where a client gets two windows back to back. This repo's cart and
> order rate limiter uses the simple Redis fixed-window pattern, which is fine
> for portfolio ecommerce endpoints but worth calling out as a tradeoff.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: Per-user versus global limits?

Fast answer:

> I use per-user limits for authenticated business APIs because they match
> abuse and fairness better than IP alone. I still keep global or per-IP
> limits for anonymous routes, login, and dependency protection. The failure
> mode is one tenant or NATed network starving everyone else. In this repo,
> the middleware currently keys by client IP, while a stricter authenticated
> cart API could key by user ID plus route.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you avoid unbounded memory growth?

Fast answer:

> To avoid unbounded growth, rate-limit state needs bounded keys and
> expirations. Redis keys should have TTLs, and in-memory limiters need
> eviction or a maximum cardinality policy. The failure mode is accepting
> attacker-controlled identifiers and keeping a bucket forever. In this repo,
> the Redis fixed-window limiter expires each key after the window, which
> bounds growth for normal traffic.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: What do you return to clients?

Fast answer:

> I return 429 with a stable error code and retry metadata, usually
> `Retry-After` and optionally remaining/reset headers. The body should use
> the same error envelope as the rest of the API so clients do not need
> special parsing. The failure mode is just dropping the connection or
> returning a vague 500, which causes clients to retry harder. This repo's
> limiter returns `RATE_LIMITED` and sets `Retry-After` from the Redis key
> TTL.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

### 5. How should API errors be shaped?

Fast answer:

> Errors should be consistent, machine-readable, and safe. I want a stable code,
> human-readable message, request ID, and field-level validation errors for 422s.
> Internal errors should be logged with detail but returned with a safe message.
> In this repo, `go/pkg/apperror` gives shared `AppError` types and middleware
> that converts handler errors into structured JSON responses.

Follow-ups:

#### Follow-up: 400 versus 422?

Fast answer:

> I use 400 when the request cannot be parsed or is structurally invalid, like
> malformed JSON or a missing required header. I use 422 when the JSON is
> syntactically valid but violates business validation, like an invalid
> quantity or field constraint. The tradeoff is client ergonomics: 422 can
> include field-level errors that forms can render directly. In this repo,
> `apperror.BadRequest` and `apperror.Validation` make that split explicit.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How much internal detail do you expose?

Fast answer:

> I expose stable error codes, safe messages, field names, and request IDs; I
> do not expose stack traces, SQL, provider secrets, or internal topology. The
> server log can keep the wrapped cause with the request ID. The failure mode
> is helping attackers or binding clients to internals. In this repo,
> `apperror.ErrorHandler` logs unknown errors and returns `INTERNAL_ERROR`
> with a safe message.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do clients handle validation errors?

Fast answer:

> Clients should treat validation errors as structured data, not scrape
> message strings. A good envelope has a stable top-level code plus `fields`
> entries with field names and messages. The production tradeoff is keeping
> those field identifiers stable because clients may bind UI behavior to them.
> In this repo, `go/pkg/apperror` has a dedicated validation response shape
> with field-level details.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you preserve request IDs?

Fast answer:

> I preserve request IDs by creating or accepting one at the edge, putting it
> in context, returning it in the response header, and including it in error
> bodies and logs. The failure mode is losing correlation exactly when a
> request fails across gateway, service, and database layers. In this repo,
> logging middleware sets `X-Request-ID`, and `apperror.ErrorHandler` includes
> the same request ID in the JSON envelope. Tracing then carries the deeper
> distributed path.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

### 6. How do you design pagination for large collections?

Fast answer:

> I avoid returning unbounded collections. Offset pagination is simple but gets
> expensive and unstable at high offsets. Cursor pagination is better for large
> or frequently changing data because the cursor encodes the next position. I
> also include filtering, sorting, limits, and a maximum limit. The order service
> product listing path has validation around page/limit/sort and cursor-related
> behavior in the repository and pagination package.

Follow-ups:

#### Follow-up: Offset versus cursor pagination?

Fast answer:

> Offset pagination is easy for humans and admin screens, but it becomes
> expensive and unstable as rows are inserted or deleted. Cursor pagination
> uses the last seen sort value and ID, so it is better for large changing
> collections. The failure mode is duplicates or skipped rows between pages.
> In this repo, product listing supports cursor-style behavior through
> `go/order-service/internal/pagination` and handler response metadata.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you sort consistently?

Fast answer:

> Consistent sorting needs a deterministic tie-breaker, usually a unique ID
> after the primary sort key. Sorting only by `created_at` or price can
> produce unstable pages when many rows share the same value. The repository
> should use the same sort fields that the cursor encodes. In this repo,
> product cursors encode values such as price, name, or created time plus the
> product ID.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: What goes in the response metadata?

Fast answer:

> The response metadata should include the page size actually used, the next
> cursor when more data exists, and sometimes total count if it is cheap and
> meaningful. I avoid promising expensive totals on hot paths unless the
> product needs them. The failure mode is clients guessing whether to fetch
> more or relying on stale counts. In this repo, product responses include
> `limit` and `nextCursor`, with page/total support where offset-style listing
> is used.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you paginate by created time safely?

Fast answer:

> Created-time pagination is safe only if the order is stable and ties are
> handled. I use `(created_at, id)` as the cursor boundary, encode it
> opaquely, and query with the same composite ordering. The failure mode is
> missing rows created at the same timestamp or duplicating rows when new
> items arrive. In this repo, the product handler formats created time with
> `time.RFC3339Nano` and pairs it with the product ID in the cursor.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

### 7. How do you handle API versioning?

Fast answer:

> The main rule is compatibility first. I prefer additive changes: new optional
> fields, new routes, and tolerant readers. For breaking changes, I use URI or
> header versioning, publish a deprecation window, and keep tests around both
> versions. Internally, protobuf/gRPC contracts need the same discipline:
> reserved fields, additive changes, and generated client compatibility.

Follow-ups:

#### Follow-up: URI versioning versus headers?

Fast answer:

> URI versioning is explicit and easy to route, document, and cache, which is
> useful for public REST APIs. Header versioning keeps resources conceptually
> cleaner but can hide behavior in clients, proxies, and logs. The production
> tradeoff is discoverability versus URL churn. For this repo, I would use URI
> versions for external ecommerce REST routes and reserve header negotiation
> for narrow compatibility cases.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you retire old versions?

Fast answer:

> I retire old versions with telemetry, published dates, compatibility tests,
> and a staged removal plan. First I measure usage, then mark deprecated, then
> stop new clients, then remove after the window. The failure mode is deleting
> a version that an active mobile or partner client still uses. In this repo,
> request metrics and route-level tests would be the guardrails before
> removing an old REST contract.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you version event schemas?

Fast answer:

> Event schemas need explicit versions because producers and consumers deploy
> independently. I prefer additive fields, tolerant consumers, and reserved or
> never-reused field names for protobuf-style contracts. The failure mode is a
> consumer crashing or silently misreading an event after a producer deploy.
> This repo's Kafka and RabbitMQ paths already carry structured messages, so I
> would version event payloads with the same backward-compatibility mindset as
> the REST API.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you test backward compatibility?

Fast answer:

> I test backward compatibility with contract fixtures: old request bodies,
> old response assertions, and generated client or consumer tests where
> applicable. The goal is proving old clients still parse the new server
> response. The failure mode is a refactor that changes JSON field names,
> status codes, or error shapes without noticing. In this repo, handler tests
> around `apperror` envelopes and product pagination responses are the right
> place to pin those contracts.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

### 8. What is the role of an API gateway in microservices?

Fast answer:

> The gateway is the stable client-facing edge. It routes requests, authenticates
> callers, applies rate limits, collects telemetry, and hides internal service
> layout. It can also aggregate responses or translate protocols, but I try to
> avoid turning it into a business-logic monolith. In this repo, the AI service
> acts like a gateway for AI functionality: it fronts ecommerce tools, RAG tools,
> LLM calls, streaming responses, and guardrails behind one client-facing API.

Follow-ups:

#### Follow-up: Gateway versus backend-for-frontend?

Fast answer:

> A gateway is a general edge for routing, auth, limits, telemetry, and
> protocol translation across clients. A BFF is client-specific and may shape
> responses for a web or mobile experience. The tradeoff is avoiding
> duplicated logic while still giving clients ergonomic APIs. In this repo,
> the AI HTTP chat endpoint behaves closer to a BFF for agent workflows, while
> ecommerce service routes are more conventional gateway-style REST edges.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: What are gateway failure modes?

Fast answer:

> Gateway failure modes include becoming a single bottleneck, hiding
> downstream failures, retry amplification, bad auth propagation, and
> inconsistent error translation. The production risk is that every client
> path depends on the gateway, so a small bug has broad blast radius. I
> mitigate that with timeouts, circuit breakers, metrics, and structured
> errors. In this repo, `go/pkg/resilience` wrappers and OpenTelemetry
> middleware are the mechanisms I would point to.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you avoid a gateway bottleneck?

Fast answer:

> I avoid a gateway bottleneck by keeping business logic out of it, setting
> deadlines, bounding payload sizes, streaming long work, and avoiding
> unbounded fan-out. Aggregation should be measured and cached only when it is
> safe. The failure mode is one client request triggering slow serial calls
> across many services. In this repo, the AI service wraps ecommerce calls
> with resilience and streams agent progress instead of making the client wait
> blindly.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: Where do you enforce authorization?

Fast answer:

> I enforce authentication at the gateway or edge, but authorization must also
> be checked at the service/resource boundary. The gateway can validate the
> token and forward user context, but the cart or order service still needs to
> verify ownership. The failure mode is trusting an upstream hop too much and
> allowing cross-user access through an internal route. In this repo, AI tool
> clients forward the bearer token to ecommerce services so user-scoped checks
> stay with the data-owning service.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

### 9. How do you stream responses from an API?

Fast answer:

> I use streaming when the client benefits from partial progress or long-running
> work. For browser-friendly one-way streams, SSE is simpler than WebSockets. The
> handler must set streaming headers, flush events, respect context cancellation,
> and switch from normal JSON errors to stream-level error events once the stream
> starts. In the AI service chat handler, the response emits events like
> `tool_call`, `tool_result`, `tool_error`, `final`, and `error`.

Follow-ups:

#### Follow-up: SSE versus WebSocket?

Fast answer:

> SSE is best for one-way server-to-browser progress: it is simple HTTP,
> auto-reconnects, and works well for text events. WebSockets are better for
> bidirectional, low-latency interaction where the client also streams
> frequent messages. The tradeoff is operational complexity. In this repo, the
> AI chat handler uses SSE because agent turns emit progress events like
> `tool_call`, `tool_result`, and `final` back to the browser.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you handle errors after headers are sent?

Fast answer:

> After headers are sent, I cannot switch back to a normal JSON error response
> or change the status code. I need to emit a stream-level `error` event,
> flush it, and end the stream cleanly. The failure mode is logging an error
> while the client waits forever or receives malformed data. In this repo, the
> chat handler explicitly comments that after the SSE boundary, errors go
> through emitted events rather than `c.Error()`.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you detect client disconnects?

Fast answer:

> Client disconnects show up through the request context being canceled or
> writes failing. The handler and any downstream tool calls should select on
> `ctx.Done()` and stop work quickly. The failure mode is continuing an
> expensive LLM or ecommerce call after the browser tab closed. In this repo,
> the chat handler passes `c.Request.Context()` into the agent runner and tool
> clients use context-aware HTTP requests.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: What server timeouts need to change?

Fast answer:

> Streaming endpoints need longer read or idle policies for the response path,
> while still keeping bounded total work with context deadlines. A normal
> write timeout can kill a valid long SSE response if no bytes are written for
> a while, so the handler should flush heartbeats or progress events. The
> failure mode is either premature disconnects or unbounded hung streams. In
> this repo, the SSE handler flushes events and the agent turn should remain
> capped by context and guardrail limits.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

### 10. How do you secure a REST API?

Fast answer:

> I secure the transport with HTTPS, authenticate requests, authorize by resource
> ownership and role, validate all inputs, use security headers, apply rate
> limits, avoid leaking internals in errors, and log enough for audit without
> exposing secrets. In this repo, cart/order middleware validates JWTs, security
> headers are applied centrally, CORS is explicit, and structured errors keep
> internal detail out of client responses.

Follow-ups:

#### Follow-up: Authentication versus authorization?

Fast answer:

> Authentication proves who the caller is; authorization decides whether that
> caller can perform this action on this resource. The production failure mode
> is validating a JWT and then forgetting to check ownership. In this repo,
> middleware validates bearer tokens, and user-scoped ecommerce calls forward
> the JWT so cart and order services can enforce resource access. Tests should
> cover both missing auth and authenticated access to the wrong user's
> resource.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: JWT versus session?

Fast answer:

> JWTs are stateless and work well across services, but revocation and
> short-lived claims need careful handling. Sessions are easier to revoke
> centrally but require a shared store and add a dependency on every request.
> The tradeoff is operational simplicity versus control. In this repo, JWTs
> fit the microservice shape because the AI service can forward the bearer
> token to ecommerce services without sharing session state.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you prevent cross-tenant access?

Fast answer:

> Cross-tenant access is prevented by deriving tenant or user scope from
> trusted auth context, not from client-submitted path or body fields alone.
> Every repository query should include that scope where data is user-owned.
> The failure mode is an IDOR bug: a valid user changes an ID and reads
> someone else's data. In this repo, cart and order paths should use the
> authenticated `userId`, and AI ecommerce clients forward the JWT rather than
> letting the agent invent identity.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: What should never appear in logs?

Fast answer:

> Secrets, bearer tokens, cookies, raw payment data, passwords, private
> prompts, and provider keys should never appear in logs. I also avoid logging
> full request bodies on auth, checkout, or LLM endpoints because they can
> contain user or business-sensitive data. The failure mode is turning
> observability into a data leak. In this repo, structured logs should keep
> request IDs, status, latency, and stable codes while redacting authorization
> headers and secrets.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

### 11. How do you integrate a third-party API behind REST endpoints?

Fast answer:

> I isolate the integration behind a typed client, pass context deadlines, wrap
> retryable calls with bounded backoff, avoid retrying unsafe side effects unless
> idempotency exists, and translate dependency errors into API-safe responses.
> I also expose dependency latency and error metrics. The AI service ecommerce
> client and LLM/RAG clients are examples of typed clients behind agent-facing
> and HTTP-facing APIs.

Follow-ups:

#### Follow-up: Which errors are retryable?

Fast answer:

> Retryable errors are usually transient transport failures, timeouts,
> connection resets, 408s, 429s when allowed by policy, and 5xx responses from
> dependencies. I do not retry validation errors, auth failures, or unsafe
> side effects unless idempotency exists. The failure mode is retry
> amplification during an outage. In this repo, `go/pkg/resilience` lets
> clients define `IsRetryable`, and ecommerce/RAG clients wrap calls with
> bounded retry and circuit breaker behavior.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you handle rate limits?

Fast answer:

> I treat external rate limits as a dependency contract, not just an error.
> For 429s, I respect `Retry-After` when it is safe, apply jitter, cap retry
> budgets, and translate sustained limits into a client-safe 429 or 503. The
> failure mode is synchronizing retries across replicas and making the
> provider block you harder. In this repo, the same retry and breaker patterns
> used by AI tool clients are where I would enforce that policy.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you test without the real API?

Fast answer:

> I test behind a fake provider using `httptest.Server` or a small fake
> client, not the real API in unit tests. The fake should assert request
> method, path, headers like `Authorization`, timeout behavior, and
> representative error responses. The production tradeoff is speed and
> determinism versus occasional contract coverage. In this repo, ecommerce
> client tests already use fake HTTP servers to verify JWT forwarding and
> response handling.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: What do you cache?

Fast answer:

> I cache stable, non-user-sensitive reads where freshness requirements allow
> it, such as product list responses without cursor parameters. I avoid
> caching personalized, authorization-sensitive, or side-effect responses
> unless the cache key includes the full user and request scope. The failure
> mode is serving one user's data to another or keeping stale inventory
> decisions. In this repo, product service caching skips cursor queries
> because cursor values are unique per request and would waste cache space.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

### 12. How do you test REST handlers and middleware?

Fast answer:

> I test handlers with an in-memory router, fake services, representative
> requests, and assertions on status code and JSON shape. Middleware tests should
> cover success, rejection, missing headers, malformed inputs, and downstream
> behavior. In this repo, handler tests include the shared `apperror.ErrorHandler`
> so tests verify the same error envelope clients see.

Follow-ups:

#### Follow-up: Unit test versus integration test?

Fast answer:

> Unit tests cover handler logic with fake services and in-memory routers;
> integration tests cover real middleware, Redis, database, migrations, and
> repository behavior. The tradeoff is speed versus confidence. I want most
> edge cases in unit tests and a smaller set of integration tests for wiring
> and persistence. In this repo, handler tests assert JSON/status behavior,
> while idempotency integration tests verify Redis-backed replay semantics.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you test idempotency?

Fast answer:

> I test idempotency by sending the same mutating request twice with the same
> valid UUID key and asserting the handler side effect only happens once and
> the second response matches the first. I also test missing keys, invalid
> UUIDs, in-flight conflicts, and different keys creating separate work. The
> failure mode is a retry after a lost response creating duplicate state. This
> repo has middleware and integration tests around those Redis-backed cases.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you test auth?

Fast answer:

> I test auth with missing tokens, malformed tokens, expired or invalid
> signatures, valid tokens, and valid tokens that lack ownership or role
> permission. The assertion should include both status code and that the
> downstream handler did not run on rejection. The failure mode is an auth
> middleware that rejects correctly but still allows side effects. In this
> repo, cart auth middleware and AI chat JWT tests are the patterns I would
> follow.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you test streaming responses?

Fast answer:

> I test streaming with an in-memory server or recorder that can observe
> headers, event framing, flush behavior, and final/error events. The test
> should verify `Content-Type: text/event-stream`, expected event names, and
> behavior when the runner returns an error after streaming starts. The
> failure mode is passing normal JSON handler tests while the browser receives
> malformed SSE. In this repo, `go/ai-service/internal/http/chat_test.go`
> verifies the chat handler streams SSE events.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

## Scenario Drills

### Scenario 1: Heavy Traffic Cart API

Prompt:

> Design a cart API that can handle heavy traffic and safe retries from mobile
> clients on unstable networks.

Strong answer checklist:

- `GET /cart`, `POST /cart/items`, `PATCH /cart/items/{id}`, `DELETE /cart/items/{id}`.
- JWT auth and user ownership checks.
- Request validation with bounded quantity.
- `Idempotency-Key` for mutating requests.
- Rate limit per user and possibly per IP.
- Context deadlines on product validation dependency.
- Structured errors with request IDs.
- Metrics for latency, status, rate-limit hits, idempotency hits/conflicts.
- Tests for duplicate POST, missing auth, invalid product ID, dependency failure.

Repo tie-in:

- Cart service already has auth, rate limiting, idempotency, validation,
  product validation via gRPC, and structured errors.

### Scenario 2: API Gateway For AI Agent Tools

Prompt:

> Design an API gateway for an AI agent that can call ecommerce and RAG tools and
> stream progress back to the client.

Strong answer checklist:

- `POST /chat` or `/agent/turn` accepts conversation messages.
- Authenticated user context becomes tool-call context.
- Tool registry controls which tools are callable.
- Context timeout caps the full agent turn.
- SSE streams `tool_call`, `tool_result`, `tool_error`, `final`, and `error`.
- Tool failures are fed back as tool errors; infrastructure failures return API
  or stream errors.
- Rate limits and history limits prevent abuse.
- Metrics count turns, steps, tool calls, duration, and outcomes.

Repo tie-in:

- AI service implements an agent loop, SSE chat handler, ecommerce/RAG clients,
  rate limiting, history guardrails, and metrics.

### Scenario 3: Gateway Is Causing High Latency

Prompt:

> A gateway endpoint has p99 latency spikes. How do you debug it?

Strong answer checklist:

- Check whether latency is at gateway, service, database, or dependency layer.
- Use traces to identify slow spans.
- Use metrics for request duration, status codes, dependency latency, retries,
  circuit breaker state, and rate-limit/store latency.
- Check connection pools, goroutine count, lock contention, queue lag, and retry
  amplification.
- Look for expensive response aggregation or unbounded payloads.
- Add deadlines and shed load if the dependency is saturated.

Repo tie-in:

- The repo has OpenTelemetry middleware, tracing helpers, Prometheus metrics,
  and resilience wrappers that make this kind of debugging explainable.
