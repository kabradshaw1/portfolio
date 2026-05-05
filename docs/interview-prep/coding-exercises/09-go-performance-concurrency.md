# Go Performance And Concurrency Coding Exercises

### 1. Concurrent Counter Store

Prompt:

> Concurrent Counter Store


Time target: 25 minutes.

Build:

- `Increment(key string)`
- `Get(key string) int`
- `Snapshot() map[string]int`

Constraints:

- Safe under concurrent access.
- Include tests with `t.Parallel()` or goroutines.
- Mention how you would benchmark global lock versus sharded lock.

Fast design:

- `RWMutex` versus sharding.
- Copying snapshot to avoid exposing internal map.
- Race detector.

Repo anchors:
- `go/` - Relevant implementation anchor for this topic.

### 2. Worker Pool With Cancellation

Prompt:

> Worker Pool With Cancellation


Time target: 35 minutes.

Build:

- A worker pool that accepts jobs.
- Stops on context cancellation.
- Returns results and errors.
- Does not leak goroutines when producers or consumers stop early.

Fast design:

- Bounded channels.
- Single owner for channel close.
- `WaitGroup` or `errgroup`.
- Backpressure and timeout behavior.

Repo anchors:
- `go/` - Relevant implementation anchor for this topic.

### 3. Retryable HTTP Client

Prompt:

> Retryable HTTP Client


Time target: 40 minutes.

Build:

- `Do(ctx context.Context, req *http.Request) (*http.Response, error)`
- Retry on 429 and 5xx.
- Do not retry 400/401/403/404.
- Use exponential backoff with jitter.
- Stop when context is canceled.

Fast design:

- Request body replay.
- Idempotency keys.
- Retry budget.
- Metrics and tracing.

Repo anchors:
- `go/` - Relevant implementation anchor for this topic.

### 4. Sharded Idempotency Store

Prompt:

> Sharded Idempotency Store


Time target: 45 minutes.

Build:

- `Start(key string) (started bool)`
- `Finish(key string, result Result)`
- `Get(key string) (Result, bool)`
- TTL cleanup can be described instead of fully implemented.

Fast design:

- Duplicate POST protection.
- Pending versus completed states.
- Lock granularity.
- Memory cleanup.

Repo anchors:
- `go/` - Relevant implementation anchor for this topic.
