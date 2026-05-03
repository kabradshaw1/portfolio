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

- What belongs in the client versus service layer?
- How do you test without calling the real provider?
- What do you log?
- How do you handle provider-specific error formats?

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

- Should you retry 408?
- How do you handle 429?
- What is a retry budget?
- How can retries make an outage worse?

### 3. How do idempotency keys apply to external APIs?

Fast answer:

> If an external call creates a side effect, a client timeout does not tell me
> whether the provider performed the action. An idempotency key lets retries map
> back to the same provider operation instead of creating duplicates. In the
> payment service, the Stripe checkout flow uses an idempotency key derived from
> the order ID, so repeated attempts for the same order do not create multiple
> independent payment sessions.

Follow-ups:

- What should the key be based on?
- How long should the provider retain it?
- What if the same key is reused with different parameters?
- Do refunds need idempotency too?

### 4. How do you handle webhooks safely?

Fast answer:

> I read the raw body, verify the provider signature before trusting the event,
> parse only after verification, and make processing idempotent because providers
> retry webhooks. Then I update local state transactionally and write any
> downstream events through an outbox if more work needs to happen. In this repo,
> the Stripe webhook handler verifies signatures, the service records processed
> event IDs, and the payment flow uses outbox writes for downstream effects.

Follow-ups:

- Why do you need the raw body for signature verification?
- How do you handle duplicate webhook delivery?
- What HTTP status do you return on processing failure?
- How do you replay failed webhooks?

### 5. How do you prevent provider outages from cascading?

Fast answer:

> I put strict timeouts around provider calls, use bounded retries with backoff,
> classify non-retryable errors, and add a circuit breaker so repeated provider
> failures fail fast instead of exhausting goroutines and connection pools. I also
> design degraded behavior where possible. In this repo, external clients use the
> shared resilience package and Prometheus exposes circuit breaker state.

Follow-ups:

- What is the difference between retry and circuit breaker?
- When should a circuit breaker open?
- What do you return to the user while the provider is down?
- How do you alert on provider degradation?

### 6. How do context timeouts and HTTP client timeouts work together?

Fast answer:

> The HTTP client timeout is a coarse safety net for the whole request lifecycle,
> while the request context carries the caller's deadline and cancellation. I set
> both: a client timeout to prevent stuck connections and a per-request context
> so cancellation propagates from the incoming request or agent turn. Retries
> should stay inside the original context deadline instead of creating work after
> the caller is gone.

Follow-ups:

- Where do you create the timeout?
- Do retries get fresh contexts?
- What happens when the client disconnects?
- How do you test timeout behavior?

### 7. How do you translate provider errors into API errors?

Fast answer:

> I keep provider detail in logs and internal error wrapping, but return stable
> API errors to callers. A Stripe or LLM error should not leak secrets, raw
> provider payloads, or implementation-specific stack detail. I map errors into
> categories like validation, unauthorized, rate limited, dependency unavailable,
> and internal failure. Then I preserve enough metadata for debugging through
> request IDs, trace IDs, and structured logs.

Follow-ups:

- Do you expose provider error messages?
- How do you distinguish client error from dependency error?
- What status code should dependency unavailable return?
- How do you preserve root cause for logs?

### 8. How do you test third-party integrations?

Fast answer:

> I test most behavior with interfaces and fake clients at the service layer,
> then use `httptest.Server` for HTTP client behavior such as auth headers,
> status-code handling, JSON decoding, timeouts, and retries. For webhooks, I
> test signature verification and duplicate event handling separately. In this
> repo, payment service tests use mock Stripe clients, AI service client tests use
> test servers, and webhook tests cover invalid signature behavior.

Follow-ups:

- When do you use contract tests?
- Do you hit sandbox APIs in CI?
- How do you simulate rate limits?
- How do you test retries without slow sleeps?

### 9. How do you manage credentials for third-party APIs?

Fast answer:

> Credentials should come from secret management or environment variables, never
> source code. They should be scoped by environment, rotated, and redacted from
> logs. For outbound requests, the client should attach credentials in the narrow
> place they are needed. In this repo, service configs load provider URLs and
> keys from environment variables, and bearer tokens are attached inside typed
> clients rather than scattered through handlers.

Follow-ups:

- How do you rotate a key safely?
- What should be redacted?
- How do you separate prod and QA credentials?
- How do you handle a leaked key?

### 10. How do you integrate multiple LLM providers?

Fast answer:

> I define a provider-neutral interface and normalize inputs and outputs into
> internal types. Provider-specific request formats, headers, model names, tool
> schemas, and error parsing stay inside provider clients. In this repo, the AI
> service has an `llm.Client` abstraction and separate Ollama, OpenAI, and
> Anthropic clients, while the agent loop depends on the interface instead of a
> specific provider.

Follow-ups:

- How do you normalize tool calls?
- How do you handle provider-specific streaming?
- How do you fail over between providers?
- How do you evaluate model/provider quality?

### 11. How do you handle external API rate limits?

Fast answer:

> I treat rate limits as a first-class dependency constraint. The client should
> detect 429 responses, respect `Retry-After` when appropriate, use bounded
> backoff, and avoid retry storms. At the product level, I may queue work,
> degrade features, cache responses, or shed load. I also expose metrics for
> provider rate-limit responses so the team can decide whether to optimize usage
> or raise provider limits.

Follow-ups:

- Do you always retry 429?
- What if requests are user-facing?
- How do you avoid synchronized retries?
- What is jitter?

### 12. How do you keep local state consistent with an external provider?

Fast answer:

> I treat the external provider as eventually consistent with my local database.
> For payments, I store local pending state before or around the provider call,
> update provider IDs when they are known, and use webhooks as the source of
> truth for asynchronous status changes. Duplicate webhooks must be idempotent,
> and downstream events should use an outbox so local state and emitted events do
> not diverge.

Follow-ups:

- What if local DB write succeeds but provider call fails?
- What if provider succeeds but local update fails?
- What is an outbox pattern?
- What is the source of truth for payment status?

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

## Coding Exercises

### Exercise 1: Retryable API Client

Time target: 40 minutes.

Build:

- `Client.GetThing(ctx, id string) (Thing, error)`
- Uses `http.NewRequestWithContext`.
- Adds `Authorization: Bearer <token>`.
- Retries network errors and 5xx.
- Does not retry 4xx.
- Closes response bodies.
- Decodes JSON into a typed struct.

Expected discussion:

- Retry budget and jitter.
- Request context versus client timeout.
- Testing with `httptest.Server`.
- Error classification.

### Exercise 2: Webhook Receiver

Time target: 45 minutes.

Build:

- `POST /webhooks/provider`
- Reads raw body.
- Verifies a fake HMAC signature.
- Extracts `event_id` and `type`.
- Stores processed event IDs.
- Ignores duplicate events.

Expected discussion:

- Why raw body matters.
- Replay protection.
- Idempotent processing.
- Returning 2xx for duplicates.

### Exercise 3: Payment State Machine

Time target: 45 minutes.

Build:

- `pending -> succeeded`
- `pending -> failed`
- `succeeded -> refunded`
- Invalid transitions return an error.

Expected discussion:

- Local state versus provider state.
- Webhooks as asynchronous status updates.
- Transaction boundaries.
- Audit logging.

### Exercise 4: LLM Provider Interface

Time target: 35 minutes.

Build:

- `type Client interface { Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) }`
- Two fake providers with different response shapes.
- Adapter functions normalize both into `ChatResponse`.

Expected discussion:

- Provider-neutral domain types.
- Tool-call normalization.
- Provider-specific error handling.
- Testing agent loop against an interface.

