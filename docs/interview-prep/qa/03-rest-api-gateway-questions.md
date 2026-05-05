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

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: When do you use POST versus PUT?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you version an API?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you avoid breaking clients?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

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

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you order middleware?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: What happens if auth middleware writes a response early?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you test middleware?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

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

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How long should the key live?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: What if the first request succeeds but the response is lost?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: What if the same key is reused with a different body?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

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

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: Per-user versus global limits?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you avoid unbounded memory growth?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: What do you return to clients?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

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

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How much internal detail do you expose?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do clients handle validation errors?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you preserve request IDs?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

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

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you sort consistently?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: What goes in the response metadata?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you paginate by created time safely?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

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

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you retire old versions?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you version event schemas?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you test backward compatibility?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

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

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: What are gateway failure modes?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you avoid a gateway bottleneck?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: Where do you enforce authorization?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

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

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you handle errors after headers are sent?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you detect client disconnects?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: What server timeouts need to change?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

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

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: JWT versus session?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you prevent cross-tenant access?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: What should never appear in logs?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

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

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you handle rate limits?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you test without the real API?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: What do you cache?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

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

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you test idempotency?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you test auth?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

#### Follow-up: How do you test streaming responses?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

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
