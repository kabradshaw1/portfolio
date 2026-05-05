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

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you test without calling the real provider?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: What do you log?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you handle provider-specific error formats?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

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

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you handle 429?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: What is a retry budget?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How can retries make an outage worse?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

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

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How long should the provider retain it?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: What if the same key is reused with different parameters?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: Do refunds need idempotency too?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

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

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you handle duplicate webhook delivery?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: What HTTP status do you return on processing failure?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you replay failed webhooks?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

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

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: When should a circuit breaker open?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: What do you return to the user while the provider is down?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you alert on provider degradation?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

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

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: Do retries get fresh contexts?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: What happens when the client disconnects?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you test timeout behavior?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

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

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you distinguish client error from dependency error?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: What status code should dependency unavailable return?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you preserve root cause for logs?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

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

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: Do you hit sandbox APIs in CI?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you simulate rate limits?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you test retries without slow sleeps?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

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

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: What should be redacted?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you separate prod and QA credentials?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you handle a leaked key?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

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

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you handle provider-specific streaming?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you fail over between providers?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you evaluate model/provider quality?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

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

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: What if requests are user-facing?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: How do you avoid synchronized retries?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: What is jitter?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

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

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: What if provider succeeds but local update fails?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: What is an outbox pattern?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

#### Follow-up: What is the source of truth for payment status?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

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
