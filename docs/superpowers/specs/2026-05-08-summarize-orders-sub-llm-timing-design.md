# summarize_orders Sub-LLM Timing Design

Date: 2026-05-08

## Goal

Split `summarize_orders` sub-LLM latency from total tool latency so the AI
Pipeline dashboard can show whether slow `summarize_orders` calls are dominated
by ecommerce order retrieval or by the nested LLM call.

This implements Phase 4 from
`docs/handoffs/observability-remaining-gaps-roadmap.md`.

## Scope

In scope:

- Time only the nested `t.llm.Chat(...)` call inside `summarizeOrdersTool.Call`.
- Add `sub_llm_duration_ms` to `summarize_orders` success logs.
- Add `sub_llm_duration_ms` to `summarize_orders` sub-LLM failure logs.
- Add a targeted Prometheus histogram named
  `ai_tool_sub_llm_duration_seconds`.
- Record that histogram with `tool="summarize_orders"` after the sub-LLM call
  returns, including error cases.
- Add one AI Pipeline dashboard panel for sub-LLM latency.
- Keep existing `ai_tool_duration_seconds` behavior unchanged.

Out of scope:

- Timing ecommerce API calls separately.
- Adding labels for model, user, period, outcome, error type, or prompt shape.
- Adding generic nested-operation metrics for arbitrary tool internals.
- Changing agent-loop tool duration recording.
- Changing the `summarize_orders` user-facing response.

## Current Behavior

`summarize_orders` currently records total wall-clock tool duration through the
agent loop's existing `ai_tool_duration_seconds` metric. Inside the tool, the
success log also includes `duration_ms`.

The sub-LLM call duration is folded into the total tool duration. When the tool
is slow, dashboard users can see that `summarize_orders` is slow but cannot tell
whether the nested LLM call is the main contributor without reading logs or
inferring from provider-level LLM metrics.

## Design

### Tool Logging

Inside `go/ai-service/internal/tools/orders.go`, measure the nested LLM call:

```go
llmStart := time.Now()
resp, err := t.llm.Chat(ctx, []llm.Message{
	{Role: llm.RoleUser, Content: prompt},
}, nil)
subLLMDuration := time.Since(llmStart)
```

On sub-LLM error, keep the existing warning shape and add
`sub_llm_duration_ms`.

On success, keep the existing `"tool result"` log and add
`sub_llm_duration_ms` beside `duration_ms`.

The empty-order path does not call the sub-LLM and should not log
`sub_llm_duration_ms`.

### Metric

Add a targeted histogram in `go/ai-service/internal/metrics/metrics.go`:

```go
ToolSubLLMDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "ai_tool_sub_llm_duration_seconds",
	Help:    "Nested LLM call latency within AI tools.",
	Buckets: prometheus.DefBuckets,
}, []string{"tool"})
```

Expose it through the existing recorder boundary with:

```go
RecordToolSubLLM(tool string, dur time.Duration)
```

`PromRecorder.RecordToolSubLLM` observes
`ToolSubLLMDuration.WithLabelValues(tool).Observe(dur.Seconds())`.

Use only `tool="summarize_orders"` for this phase. This keeps cardinality
bounded and avoids committing to a reusable operation taxonomy before another
tool needs it.

### Wiring

`summarizeOrdersTool` currently depends only on `ordersAPI` and `llm.Client`.
Add a small dependency for recording sub-LLM timing so tests can verify calls
without touching Prometheus globals.

The preferred shape is a local interface in `orders.go`:

```go
type toolSubLLMRecorder interface {
	RecordToolSubLLM(tool string, dur time.Duration)
}
```

Then extend `summarizeOrdersTool` with an optional recorder field. Keep the
existing `NewSummarizeOrdersTool(api, llmc)` constructor and have it delegate to
a new metrics-enabled constructor:

```go
func NewSummarizeOrdersToolWithRecorder(api ordersAPI, llmc llm.Client, rec toolSubLLMRecorder) Tool
```

Update `go/ai-service/cmd/server/main.go` so the production registration path
uses:

```go
registry.Register(tools.NewSummarizeOrdersToolWithRecorder(ecomClient, llmc, metrics.PromRecorder{}))
```

Tests that do not assert metrics can continue using `NewSummarizeOrdersTool`.

When the recorder is nil, skip metric recording. This preserves test simplicity
and prevents a nil dependency from changing tool behavior.

### Dashboard

Update `monitoring/grafana/dashboards/ai-pipeline.json` with one panel for
sub-LLM latency:

```promql
histogram_quantile(
  0.5,
  rate(ai_tool_sub_llm_duration_seconds_bucket{tool="summarize_orders"}[5m])
)
```

and the matching p95 query.

The panel title should be `summarize_orders Sub-LLM Latency`, use seconds as
the unit, and sit near the existing `Tool Latency` panel. A standalone panel is
preferred over combining total and sub-LLM latency in one panel because the
existing `Tool Latency` panel is generic across tools and should stay simple.

Place the new panel immediately after the existing `Tool Latency` panel and
move following panels down without changing their queries.

## Error Handling

Metric recording must not affect tool outcomes. The recorder implementation
uses Prometheus collectors and does not return errors.

Sub-LLM failures keep the existing returned error:

```text
summarize_orders: sub-llm: <cause>
```

The warning log for that failure includes both the error string and
`sub_llm_duration_ms`.

## Testing

Unit tests should cover:

- Successful `summarize_orders` records one sub-LLM duration.
- Sub-LLM error records one sub-LLM duration before returning the wrapped error.
- Empty order list records no sub-LLM duration.
- Existing behavior still skips the LLM on empty order lists.

Add log-field assertions with a local slog test handler for the success and
sub-LLM error paths. Keep the assertions focused on field presence so the tests
do not depend on exact duration values.

Dashboard changes are verified structurally by existing preflight checks.

## Verification

Run:

```bash
make preflight-go
```

Also run the existing monitoring/dashboard validation path if it is available in
the repo. Do not use Debian for this verification.

## Acceptance Criteria

- Success log includes both `duration_ms` and `sub_llm_duration_ms`.
- Sub-LLM failure log includes `sub_llm_duration_ms`.
- `ai_tool_sub_llm_duration_seconds_bucket{tool="summarize_orders"}` is emitted
  after successful and failed sub-LLM calls.
- No sub-LLM metric is emitted when there are no orders.
- Existing `ai_tool_duration_seconds` behavior remains unchanged.
- AI Pipeline includes a sub-LLM latency panel for `summarize_orders` p50 and
  p95.
