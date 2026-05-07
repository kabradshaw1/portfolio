# Third-Party API Integration Rehearsal

Use this section for questions about external APIs, SDKs, webhooks, LLM
providers, payment providers, and service-to-service HTTP clients.

## Repo Anchors

- `go/payment-service/internal/stripe/client.go`: Stripe Checkout and refund
  client wrapper.
- `go/payment-service/internal/stripe/verifier.go`: Stripe webhook signature
  verification.
- `go/payment-service/internal/service/stripe.go`: local payment state around
  Stripe calls, idempotency keys, status transitions, and API duration logging.
- `go/payment-service/internal/service/webhook.go`: webhook processing with
  processed-event idempotency and outbox writes.
- `go/payment-service/internal/repository/processed_event.go`: duplicate event
  protection with an insert-on-conflict pattern.
- `go/ai-service/internal/tools/clients/ecommerce.go`: typed ecommerce HTTP
  client with context, bearer auth, retry classification, and circuit breaker.
- `go/ai-service/internal/tools/clients/rag.go`: typed RAG client for Python
  chat/ingestion services.
- `go/ai-service/internal/llm/ollama.go`, `openai.go`, `anthropic.go`: LLM
  provider clients with HTTP timeouts and structured request/response mapping.
- `go/pkg/resilience`: shared retry and circuit breaker helpers.

## High-Frequency Questions

### 1. How do you design a robust third-party API client?

Fast answer:

> I hide the provider behind a typed interface, pass `context.Context` through
> every call, set client timeouts, close response bodies, classify errors, and
> translate provider responses into domain types. I avoid letting provider SDK
> details leak across the codebase. In this repo, the AI service has typed
> clients for ecommerce, RAG, Ollama, OpenAI, and Anthropic, and the payment
> service wraps Stripe checkout, refunds, and webhook verification.

Follow-ups:

#### Follow-up: What belongs in the client versus service layer?

Fast answer:

> The client layer should know HTTP, provider paths, headers, timeouts, response
> parsing, and provider-specific error formats. The service layer should own
> business state: when to create a payment record, what idempotency key to use,
> and how local status changes after a provider response. The failure mode is
> letting Stripe or LLM SDK details leak into handlers and tests. In this repo,
> `go/payment-service/internal/service/stripe.go` depends on a `StripeClient`
> interface instead of constructing provider calls itself.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you test without calling the real provider?

Fast answer:

> I test without the real provider by using interfaces at the service layer and
> `httptest.Server` at the HTTP client layer. The fake should assert the method,
> path, auth headers, body shape, and status-code handling. The tradeoff is that
> unit tests stay deterministic, while a smaller contract or sandbox suite can
> catch provider drift. In this repo, payment service tests use mock Stripe
> clients, and AI ecommerce/RAG client tests use local HTTP servers.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: What do you log?

Fast answer:

> I log the operation name, local resource ID, provider object ID when safe,
> duration, status class, and stable error category. I do not log API keys,
> bearer tokens, raw webhook bodies, card data, or full provider payloads. The
> failure mode is making an outage debuggable by leaking secrets. In this repo,
> the Stripe service logs `create_checkout` and `refund` durations with order
> IDs and provider IDs, while keeping credentials out of the log fields.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you handle provider-specific error formats?

Fast answer:

> I parse provider-specific error formats at the edge of the client and convert
> them into internal categories like retryable, validation, auth, rate limited,
> or unavailable. The rest of the app should not need to know Stripe's or an LLM
> provider's raw JSON shape. The failure mode is sprinkling string matching
> across services and handlers. In this repo, AI clients classify 4xx responses
> as non-retryable and use `go/pkg/resilience` for the retry decision.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

### 2. Which third-party API errors should you retry?

Fast answer:

> I retry transient failures: network errors, timeouts, some 5xx responses, and
> sometimes 429 if the retry can respect `Retry-After` and the retry budget. I do
> not retry most 4xx errors because they usually mean bad input, auth failure, or
> forbidden access. For side-effecting calls, retries require idempotency keys or
> another duplicate-protection strategy. The AI service clients explicitly avoid
> retrying 4xx responses, and the shared resilience package supports retry
> classification.

Follow-ups:

#### Follow-up: Should you retry 408?

Fast answer:

> I usually treat 408 as retryable because it means the request timed out before
> the provider produced a usable response. The caveat is side effects: for a
> payment or checkout call, I only retry if an idempotency key makes the retry
> map to the same provider operation. The failure mode is turning an ambiguous
> timeout into duplicate external state. In this repo, Stripe checkout uses an
> order-derived idempotency key before retrying would be acceptable.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you handle 429?

Fast answer:

> For 429, I respect `Retry-After` when the call is safe to retry and the
> caller's context still has time left. I cap attempts, add jitter, and surface a
> clean rate-limit or dependency-unavailable response when retrying would only
> make the provider angrier. The failure mode is every replica retrying at the
> same moment. In this repo, `go/pkg/resilience` gives bounded backoff, and the
> provider client decides whether 429 is retryable.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: What is a retry budget?

Fast answer:

> A retry budget is the maximum extra work I allow retries to create, usually
> expressed as attempts, time, or a percentage of normal request volume. It
> keeps a dependency outage from multiplying traffic beyond what the provider or
> my service can handle. The failure mode is a retry storm that exhausts
> goroutines, connection pools, and provider quotas. In this repo, the default
> retry config is three attempts with capped exponential backoff and context
> cancellation.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How can retries make an outage worse?

Fast answer:

> Retries make an outage worse when failing calls are repeated faster than the
> provider can recover. That increases load, queues, timeouts, and tail latency
> for every caller. The production fix is bounded attempts, jitter, context
> deadlines, non-retryable classification, and a circuit breaker that fails fast.
> In this repo, `resilience.Call` stops retrying when the breaker is open.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

### 3. How do idempotency keys apply to external APIs?

Fast answer:

> If an external call creates a side effect, a client timeout does not tell me
> whether the provider performed the action. An idempotency key lets retries map
> back to the same provider operation instead of creating duplicates. In the
> payment service, the Stripe checkout flow uses an idempotency key derived from
> the order ID, so repeated attempts for the same order do not create multiple
> independent payment sessions.

Follow-ups:

#### Follow-up: What should the key be based on?

Fast answer:

> The key should be based on the business operation, not a random attempt ID. For
> checkout, an order-derived key is right because every retry for that order
> should resolve to the same Stripe operation. The failure mode is generating a
> new key per retry and creating duplicate sessions or charges. In this repo,
> `repository.IdempotencyKey(orderID)` creates a stable payment key from the
> order ID.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How long should the provider retain it?

Fast answer:

> The provider should retain idempotency records for at least the maximum client
> and job retry window, including delayed retries from mobile clients and
> queues. I also keep local state so I can resume or inspect the operation after
> provider retention expires. The tradeoff is provider storage limits versus
> duplicate protection. In this repo, local payments keep the order, status, and
> Stripe IDs so webhooks can reconcile later.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: What if the same key is reused with different parameters?

Fast answer:

> Reusing the same key with different parameters should be rejected, because the
> key no longer identifies one operation. Providers often return an idempotency
> mismatch instead of creating a new side effect. The failure mode is replaying a
> checkout response for the wrong amount or currency. In this repo, the stable
> key is tied to the order, so the service must keep the amount and currency
> consistent for that order before calling Stripe.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: Do refunds need idempotency too?

Fast answer:

> Yes, refunds need idempotency because they are side-effecting and retries are
> ambiguous after timeouts. A refund key should be based on the refund business
> operation, not just the payment intent, especially if partial refunds are
> allowed. The failure mode is issuing two refunds for one user request. In this
> repo, `RefundPayment` wraps Stripe refund behind the service layer; a stricter
> production version should pass a refund idempotency key too.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

### 4. How do you handle webhooks safely?

Fast answer:

> I read the raw body, verify the provider signature before trusting the event,
> parse only after verification, and make processing idempotent because providers
> retry webhooks. Then I update local state transactionally and write any
> downstream events through an outbox if more work needs to happen. In this repo,
> the Stripe webhook handler verifies signatures, the service records processed
> event IDs, and the payment flow uses outbox writes for downstream effects.

Follow-ups:

#### Follow-up: Why do you need the raw body for signature verification?

Fast answer:

> Signature verification must run over the exact bytes the provider signed.
> Parsing and re-marshaling JSON can change whitespace, ordering, or escaping,
> which makes a valid signature look invalid. The failure mode is either
> rejecting real provider events or, worse, trusting parsed data before proving
> it came from the provider. In this repo, the webhook handler reads the raw body
> and passes it with `Stripe-Signature` to the verifier.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you handle duplicate webhook delivery?

Fast answer:

> I treat provider event IDs as idempotency keys. The first delivery records the
> event and performs the state transition; later deliveries return success
> without repeating side effects. The failure mode is duplicate outbox messages,
> duplicate refunds, or repeated status transitions. In this repo, the processed
> event repository uses an insert-on-conflict style `TryInsert`, and duplicate
> webhook events become no-ops with metrics.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: What HTTP status do you return on processing failure?

Fast answer:

> If processing fails after a valid webhook, I usually return a 5xx so the
> provider retries. If the event is invalid or the signature fails, I return a
> 400 because retrying the same bad request will not help. For duplicates that
> are already processed, I return 2xx so the provider stops retrying. In this
> repo, verified webhook events return `{"received": true}` after successful or
> duplicate-safe service handling.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you replay failed webhooks?

Fast answer:

> Replays should go through the same verification and idempotent processing path
> as the original webhook. I do not want a special admin path that bypasses
> signature checks, duplicate detection, or outbox writes. The failure mode is a
> replay fixing one record while emitting duplicate downstream events. In this
> repo, replayed Stripe events should still hit the webhook handler and
> processed-event guard.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

### 5. How do you prevent provider outages from cascading?

Fast answer:

> I put strict timeouts around provider calls, use bounded retries with backoff,
> classify non-retryable errors, and add a circuit breaker so repeated provider
> failures fail fast instead of exhausting goroutines and connection pools. I also
> design degraded behavior where possible. In this repo, external clients use the
> shared resilience package and Prometheus exposes circuit breaker state.

Follow-ups:

#### Follow-up: What is the difference between retry and circuit breaker?

Fast answer:

> A retry handles one request that might succeed if tried again; a circuit
> breaker protects the system when many requests are failing. Retries spend a
> small budget on transient errors, while the breaker opens and fails fast after
> repeated failures. The failure mode is retrying every request into a provider
> that is already down. In this repo, `resilience.Call` routes each attempt
> through the breaker and stops retrying on open-breaker errors.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: When should a circuit breaker open?

Fast answer:

> A circuit breaker should open when recent failures show the dependency is not
> healthy enough to keep calling, such as consecutive timeouts or 5xx responses.
> The threshold should avoid opening on one blip but react before queues and
> goroutines pile up. The failure mode is either flapping too aggressively or
> staying closed through an outage. In this repo, `NewBreaker` defaults to
> opening after five consecutive failures and then probes after a half-open
> window.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: What do you return to the user while the provider is down?

Fast answer:

> I return a user-safe degraded response based on the operation. For checkout, I
> would avoid claiming payment succeeded; I would show pending or ask the user
> to retry later with the same order. For AI/RAG tools, I would emit a tool error
> or partial answer instead of failing the whole agent turn when possible. The
> failure mode is hiding dependency failure as success.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you alert on provider degradation?

Fast answer:

> I alert on provider degradation using error rate, timeout rate, p95/p99
> latency, breaker state, and business symptoms like checkout failures or tool
> error events. The alert should distinguish provider failure from local DB or
> validation failures. The failure mode is paging on every isolated 500 instead
> of sustained user impact. In this repo, `go/pkg/resilience` exposes circuit
> breaker state and the payment service logs Stripe API duration.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

### 6. How do context timeouts and HTTP client timeouts work together?

Fast answer:

> The HTTP client timeout is a coarse safety net for the whole request lifecycle,
> while the request context carries the caller's deadline and cancellation. I set
> both: a client timeout to prevent stuck connections and a per-request context
> so cancellation propagates from the incoming request or agent turn. Retries
> should stay inside the original context deadline instead of creating work after
> the caller is gone.

Follow-ups:

#### Follow-up: Where do you create the timeout?

Fast answer:

> I create the timeout at the boundary that owns the work budget. A handler or
> agent turn sets the overall deadline, and the provider client uses that
> context plus an HTTP client timeout as a safety net. The failure mode is each
> lower layer creating a fresh longer timeout and continuing work after the
> caller gave up. In this repo, AI ecommerce and RAG clients use context-aware
> requests with different HTTP timeouts for short ecommerce calls and longer RAG
> generation.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: Do retries get fresh contexts?

Fast answer:

> Retries should use the original operation context, not a brand-new background
> context. Each attempt can create a request from that context, but the total
> retry loop must stop when the caller's deadline or cancellation fires. The
> failure mode is doing provider work after the user disconnected or the saga
> step timed out. In this repo, `resilience.Retry` waits between attempts with
> `ctx.Done()` and returns the context error on cancellation.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: What happens when the client disconnects?

Fast answer:

> When the client disconnects, the request context is canceled and downstream
> provider calls should stop quickly. For non-idempotent side effects, I still
> have to reconcile because the provider may have completed the action before
> cancellation reached my process. The failure mode is leaking goroutines or
> leaving payment state permanently ambiguous. In this repo, payment state and
> Stripe webhooks give a path to reconcile ambiguous checkout results.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you test timeout behavior?

Fast answer:

> I test timeouts with fake servers or fake clients that block until the context
> is canceled, not by sleeping for real provider-scale durations. The assertion
> should prove the call returns a context deadline error, no extra retry happens
> after cancellation, and resources are closed. The failure mode is slow flaky
> tests that still miss cancellation leaks. In this repo, `go/pkg/resilience`
> has retry cancellation tests that use short test durations.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

### 7. How do you translate provider errors into API errors?

Fast answer:

> I keep provider detail in logs and internal error wrapping, but return stable
> API errors to callers. A Stripe or LLM error should not leak secrets, raw
> provider payloads, or implementation-specific stack detail. I map errors into
> categories like validation, unauthorized, rate limited, dependency unavailable,
> and internal failure. Then I preserve enough metadata for debugging through
> request IDs, trace IDs, and structured logs.

Follow-ups:

#### Follow-up: Do you expose provider error messages?

Fast answer:

> I expose provider messages only when they are safe, stable, and actionable for
> the caller. Most raw provider messages should stay in logs because they can
> include implementation details or change without notice. The failure mode is
> leaking secrets or making clients depend on Stripe or LLM-specific wording. In
> this repo, service methods wrap provider errors with operation context, while
> API layers should translate them into stable app errors.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you distinguish client error from dependency error?

Fast answer:

> A client error is caused by our request or the user's input: invalid amount,
> missing auth, unsupported model, or bad parameters. A dependency error is the
> provider timing out, returning 5xx, rate limiting, or being unreachable. The
> tradeoff is not blaming the user for a provider outage and not retrying bad
> input. In this repo, AI clients classify 4xx as non-retryable while resilience
> handles transient dependency failures.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: What status code should dependency unavailable return?

Fast answer:

> Dependency unavailable usually maps to 503, often with retry guidance when the
> operation is safe to retry. A timeout can be 504 at a gateway boundary, but
> internally I still treat it as dependency failure. The failure mode is
> returning 500 for everything and making clients guess whether retrying is safe.
> In this repo, provider and database errors should be translated into stable
> `apperror` or gRPC status categories at the boundary.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you preserve root cause for logs?

Fast answer:

> I preserve root cause by wrapping errors with `%w`, logging structured fields,
> and carrying request IDs or trace context through the call. The public API gets
> a stable safe error, while logs keep the provider operation, local ID, provider
> ID, duration, and wrapped cause. The failure mode is either losing the true
> cause or exposing it to clients. In this repo, Stripe service methods wrap
> errors like `create stripe checkout session` and log operation duration.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

### 8. How do you test third-party integrations?

Fast answer:

> I test most behavior with interfaces and fake clients at the service layer,
> then use `httptest.Server` for HTTP client behavior such as auth headers,
> status-code handling, JSON decoding, timeouts, and retries. For webhooks, I
> test signature verification and duplicate event handling separately. In this
> repo, payment service tests use mock Stripe clients, AI service client tests use
> test servers, and webhook tests cover invalid signature behavior.

Follow-ups:

#### Follow-up: When do you use contract tests?

Fast answer:

> I use contract tests when the provider contract is important enough that a
> fake might drift: webhook signatures, payload fields, status-code semantics,
> or generated client assumptions. They should be narrow and usually run against
> recorded fixtures or an explicit sandbox job, not every unit test. The failure
> mode is a perfect fake that no longer matches the provider. In this repo,
> Stripe webhook verification and LLM request/response mapping are the kinds of
> boundaries worth contract coverage.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: Do you hit sandbox APIs in CI?

Fast answer:

> I do not hit sandbox APIs in normal CI unless the pipeline is explicitly
> designed for external dependencies, secrets, quotas, and provider outages.
> Normal CI should be deterministic with fakes, fixtures, and local test
> servers. The tradeoff is that sandbox tests are useful but belong in a
> scheduled or manually triggered integration suite. This repo's portfolio
> preflights should not depend on paid or flaky cloud APIs.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you simulate rate limits?

Fast answer:

> I simulate rate limits with a fake server that returns 429, optional
> `Retry-After`, and then either success or continued failure. The test should
> assert retry count, delay behavior through a controllable clock or tiny test
> durations, and the final translated error. The failure mode is treating 429 as
> a generic 500 or retrying forever. In this repo, AI HTTP client tests using
> `httptest.Server` are the right pattern.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you test retries without slow sleeps?

Fast answer:

> I avoid slow sleeps by injecting retry configuration with very small delays or
> a controllable clock, and by testing retry decisions separately from real
> elapsed time. The assertion should verify attempts and cancellation, not wait
> seconds per case. The failure mode is a flaky slow suite that engineers stop
> running. In this repo, resilience tests use millisecond delays and context
> cancellation to keep retry behavior fast and deterministic.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

### 9. How do you manage credentials for third-party APIs?

Fast answer:

> Credentials should come from secret management or environment variables, never
> source code. They should be scoped by environment, rotated, and redacted from
> logs. For outbound requests, the client should attach credentials in the narrow
> place they are needed. In this repo, service configs load provider URLs and
> keys from environment variables, and bearer tokens are attached inside typed
> clients rather than scattered through handlers.

Follow-ups:

#### Follow-up: How do you rotate a key safely?

Fast answer:

> I rotate safely by supporting old and new credentials during a short overlap,
> deploying the new secret, verifying traffic, and then revoking the old one.
> For webhook secrets, the verifier may need to accept both signatures during
> the transition. The failure mode is flipping the provider key and breaking all
> in-flight traffic. In this repo, Stripe keys and webhook secrets are loaded
> from service config, so rotation should be a config/secret rollout, not a code
> change.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: What should be redacted?

Fast answer:

> I redact API keys, bearer tokens, cookies, webhook signatures, raw payment
> details, customer PII, provider request bodies, and LLM prompts or responses
> that may contain user secrets. Logs should keep operation, IDs, duration, and
> error category instead. The failure mode is turning debugging data into a
> breach. In this repo, provider credentials are attached inside typed clients
> and should never be emitted by structured logs.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you separate prod and QA credentials?

Fast answer:

> Prod and QA credentials should live in separate secret scopes, point at
> separate provider accounts or modes, and be impossible to mix accidentally
> through naming and deployment boundaries. I also avoid copying production
> webhooks into QA. The failure mode is a QA test charging real users or a prod
> service trusting QA webhook events. In this repo, environment-driven service
> config is the right boundary for Stripe, OpenAI, Anthropic, and provider URLs.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you handle a leaked key?

Fast answer:

> A leaked key gets revoked or rotated immediately, logs and git history are
> checked for exposure, provider audit logs are reviewed, and blast radius is
> assessed by scope and time window. Then I add prevention: redaction,
> least-privilege keys, and alerts on unusual provider usage. The failure mode is
> only deleting the visible secret while the provider credential remains active.
> In this repo, no provider key should be committed; secrets belong in env or
> Kubernetes secret management.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

### 10. How do you integrate multiple LLM providers?

Fast answer:

> I define a provider-neutral interface and normalize inputs and outputs into
> internal types. Provider-specific request formats, headers, model names, tool
> schemas, and error parsing stay inside provider clients. In this repo, the AI
> service has an `llm.Client` abstraction and separate Ollama, OpenAI, and
> Anthropic clients, while the agent loop depends on the interface instead of a
> specific provider.

Follow-ups:

#### Follow-up: How do you normalize tool calls?

Fast answer:

> I normalize tool calls into an internal `ToolCall` shape with an ID, name, and
> raw JSON arguments. Each provider adapter translates its own tool-call format
> into that shape before the agent loop sees it. The failure mode is making the
> agent understand OpenAI, Anthropic, and Ollama differences directly. In this
> repo, `go/ai-service/internal/llm/types.go` defines provider-neutral
> `ToolCall`, `ToolSchema`, and `Message` types.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you handle provider-specific streaming?

Fast answer:

> I hide provider-specific streaming behind a common event interface or normalize
> it before it reaches the HTTP API. Providers differ in chunk framing, tool-call
> deltas, finish reasons, and error events. The failure mode is exposing those
> differences to the frontend and making provider switching a UI change. In this
> repo, the chat HTTP layer already emits repo-defined SSE events like
> `tool_call`, `tool_result`, `final`, and `error`.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you fail over between providers?

Fast answer:

> I fail over only for safe cases where the fallback model can satisfy the same
> contract and the operation will not duplicate side effects. The fallback needs
> its own timeout, budget, and observability label, and it may change quality or
> cost. The failure mode is silently changing model behavior in a way the user or
> evaluator cannot explain. In this repo, the LLM factory supports Ollama,
> OpenAI, and Anthropic behind one interface, which is the seam needed for
> controlled failover.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you evaluate model/provider quality?

Fast answer:

> I evaluate providers with task-specific fixtures, tool-call accuracy,
> groundedness, latency, cost, safety behavior, and production failure rate. For
> agent systems, I care about whether the model calls the right tool with valid
> JSON, not just whether the final prose sounds good. The failure mode is
> choosing a model from a generic benchmark that fails the product workflow. In
> this repo, ecommerce and RAG tool flows give concrete eval scenarios.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

### 11. How do you handle external API rate limits?

Fast answer:

> I treat rate limits as a first-class dependency constraint. The client should
> detect 429 responses, respect `Retry-After` when appropriate, use bounded
> backoff, and avoid retry storms. At the product level, I may queue work,
> degrade features, cache responses, or shed load. I also expose metrics for
> provider rate-limit responses so the team can decide whether to optimize usage
> or raise provider limits.

Follow-ups:

#### Follow-up: Do you always retry 429?

Fast answer:

> No, I retry 429 only when the operation is safe, the provider tells me when to
> retry or my backoff policy is conservative, and the caller's deadline still
> has budget. For user-facing requests, it may be better to return a clear
> rate-limit response than make the user wait through doomed retries. The
> failure mode is burning provider quota and making all replicas retry together.
> In this repo, retry classification belongs inside the typed provider client.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: What if requests are user-facing?

Fast answer:

> For user-facing requests, I keep retries short and bounded because the user is
> waiting and may manually retry. I return a stable response like retry later,
> pending, or degraded answer rather than hiding long provider waits. The failure
> mode is tying up request workers until the browser times out anyway. In this
> repo, checkout can show pending/retry behavior, while AI tool failures can
> become stream-level tool error events.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you avoid synchronized retries?

Fast answer:

> I avoid synchronized retries with jitter, capped exponential backoff, and by
> respecting provider retry headers instead of using fixed sleeps. Jitter spreads
> retry attempts across instances so recovery traffic does not arrive in one
> spike. The failure mode is a thundering herd right when the provider starts to
> recover. In this repo, `go/pkg/resilience` adds jitter to exponential backoff.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: What is jitter?

Fast answer:

> Jitter is random variation added to retry delays. Instead of every instance
> retrying after exactly 100ms, 200ms, and 400ms, each one waits a slightly
> different amount. The tradeoff is a little less predictability for much better
> fleet behavior under failure. In this repo, the retry helper adds random delay
> on top of exponential backoff and caps it at the configured maximum.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

### 12. How do you keep local state consistent with an external provider?

Fast answer:

> I treat the external provider as eventually consistent with my local database.
> For payments, I store local pending state before or around the provider call,
> update provider IDs when they are known, and use webhooks as the source of
> truth for asynchronous status changes. Duplicate webhooks must be idempotent,
> and downstream events should use an outbox so local state and emitted events do
> not diverge.

Follow-ups:

#### Follow-up: What if local DB write succeeds but provider call fails?

Fast answer:

> If the local DB write succeeds but the provider call fails, I keep a local
> record that shows the operation did not complete and return a retryable or
> pending response depending on the workflow. The key is not pretending payment
> succeeded. The failure mode is leaving an order in an irreversible state with
> no way to retry or reconcile. In this repo, `CreatePayment` creates the local
> payment record first and marks it failed when Stripe checkout returns an
> error.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: What if provider succeeds but local update fails?

Fast answer:

> If the provider succeeds but the local update fails, the system is ambiguous
> and must reconcile from provider IDs, idempotency keys, or webhooks. I would
> return a pending/unknown state, log the failure with the order and provider
> IDs, and rely on webhook or repair logic to update local state. The failure
> mode is charging the user while the local order never moves forward. In this
> repo, Stripe webhooks can backfill intent IDs and update payment status.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: What is an outbox pattern?

Fast answer:

> The outbox pattern writes the local state change and the message to publish in
> the same database transaction, then a separate poller publishes the message.
> It avoids the gap where the DB commit succeeds but the process crashes before
> sending RabbitMQ or Kafka events. The failure mode is local state and
> downstream workflows diverging. In this repo, payment webhooks write saga
> confirmation or failure messages through the payment-service outbox.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: What is the source of truth for payment status?

Fast answer:

> For payment status, the provider is the source of truth for whether money
> moved, while the local database is the source of truth the application serves
> after reconciliation. That means local status should be updated from confirmed
> provider responses and signed webhooks, not from hopeful assumptions. The
> failure mode is showing success before Stripe has actually confirmed payment.
> In this repo, webhook processing updates local payment status and emits
> downstream saga events.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

## Scenario Drills

### Scenario 1: Stripe Checkout Is Timing Out

Prompt:

> Users are creating orders, but Stripe checkout session creation intermittently
> times out. How do you design and debug the integration?

Strong answer checklist:

- Confirm idempotency key is used per order.
- Check timeout duration, retry policy, and whether calls are side-effect safe.
- Inspect Stripe latency/error logs and local structured logs.
- Check whether local payment status transitions to failed/pending correctly.
- Avoid retry storms and duplicate checkout sessions.
- Expose metrics for provider latency, errors, and status transitions.
- Consider user-facing response: retry later, resume checkout, or show pending.

Repo tie-in:

- Payment service creates local payment records, calls Stripe checkout, records
  provider IDs, logs API duration, and marks payment failed on Stripe errors.

### Scenario 2: Webhook Delivered Twice

Prompt:

> Stripe sends the same refund webhook twice. What should happen?

Strong answer checklist:

- Verify signature on both deliveries.
- Use provider event ID to detect duplicates.
- First event updates local payment status and writes downstream outbox event.
- Second event should become a no-op or return success without duplicate effects.
- Return 2xx for already-processed events so provider stops retrying.

Repo tie-in:

- Payment service has a processed-events repository using insert-on-conflict and
  a webhook service designed around idempotency and outbox writes.

### Scenario 3: AI Agent Calls A Slow RAG Service

Prompt:

> The AI agent uses a RAG service that sometimes takes 20 seconds or returns 500.
> How should the Go gateway handle it?

Strong answer checklist:

- Set request context deadline inside the agent turn deadline.
- Use a typed RAG client and avoid leaking provider details to the agent API.
- Retry only transient failures within a bounded budget.
- Use a circuit breaker to fail fast during outages.
- Return a tool error event so the agent can recover or explain limitation.
- Emit metrics/traces for tool latency and error class.

Repo tie-in:

- AI service has typed RAG clients, agent tool error events, resilience wrappers,
  and an agent turn timeout.
