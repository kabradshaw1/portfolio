# Third-Party Integrations Coding Exercises

### 1. Retryable API Client

Prompt:

> Retryable API Client


Time target: 40 minutes.

Build:

- `Client.GetThing(ctx, id string) (Thing, error)`
- Uses `http.NewRequestWithContext`.
- Adds `Authorization: Bearer <token>`.
- Retries network errors and 5xx.
- Does not retry 4xx.
- Closes response bodies.
- Decodes JSON into a typed struct.

Fast design:

- Retry budget and jitter.
- Request context versus client timeout.
- Testing with `httptest.Server`.
- Error classification.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

### 2. Webhook Receiver

Prompt:

> Webhook Receiver


Time target: 45 minutes.

Build:

- `POST /webhooks/provider`
- Reads raw body.
- Verifies a fake HMAC signature.
- Extracts `event_id` and `type`.
- Stores processed event IDs.
- Ignores duplicate events.

Fast design:

- Why raw body matters.
- Replay protection.
- Idempotent processing.
- Returning 2xx for duplicates.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

### 3. Payment State Machine

Prompt:

> Payment State Machine


Time target: 45 minutes.

Build:

- `pending -> succeeded`
- `pending -> failed`
- `succeeded -> refunded`
- Invalid transitions return an error.

Fast design:

- Local state versus provider state.
- Webhooks as asynchronous status updates.
- Transaction boundaries.
- Audit logging.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund

### 4. LLM Provider Interface

Prompt:

> LLM Provider Interface


Time target: 35 minutes.

Build:

- `type Client interface { Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) }`
- Two fake providers with different response shapes.
- Adapter functions normalize both into `ChatResponse`.

Fast design:

- Provider-neutral domain types.
- Tool-call normalization.
- Provider-specific error handling.
- Testing agent loop against an interface.

Repo anchors:
- `go/payment-service/internal/stripe/client.go` - Stripe Checkout and refund
