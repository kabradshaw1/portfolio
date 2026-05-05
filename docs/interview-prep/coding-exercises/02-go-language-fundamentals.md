# Go Language Fundamentals Coding Exercises

### 1. Slice retention fix

Prompt:

> Given a function that reads a 10 MB buffer and returns `buf[:100]`, change it
> so retaining the result does not retain the full buffer.

Fast design:

- Allocate `out := make([]byte, 100)`.
- Copy the needed bytes.
- Return `out`.
- Explain backing array retention.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

### 2. Typed error mapping

Prompt:

> Write a function that maps repository errors to not-found, conflict, or
> internal application errors.

Fast design:

- Use sentinel errors or wrapped errors.
- Use `errors.Is` or `errors.As`.
- Return stable codes at the handler boundary.
- Log internal causes separately from client messages.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

### 3. Context-aware retry helper

Prompt:

> Implement a retry helper that retries a function up to N times with backoff,
> stops on non-retryable errors, and returns immediately when context is done.

Fast design:

- Accept `context.Context`, attempts, delay, and `isRetryable`.
- Call the function with context.
- Use `time.NewTimer` plus `select` on `ctx.Done()`.
- Return the last error with context.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

### 4. Interface-backed service test

Prompt:

> Define a small repository interface for a service method and test the service
> with a fake repository.

Fast design:

- Put the interface at the consumer.
- Fake only the methods the service needs.
- Test success and repository error.
- Keep the fake deterministic.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP

### 5. JSON HTTP handler

Prompt:

> Write a handler that decodes JSON, validates required fields, calls a service,
> and returns structured errors.

Fast design:

- Limit request body size.
- Decode and validate explicitly.
- Pass request context to the service.
- Return stable error codes.

Repo anchors:
- `go/pkg/apperror` - custom error types, wrapping, `errors.As`, structured HTTP
