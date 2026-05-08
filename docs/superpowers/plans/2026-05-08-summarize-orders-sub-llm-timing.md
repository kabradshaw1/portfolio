# summarize_orders Sub-LLM Timing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add logs, a targeted Prometheus histogram, and an AI Pipeline dashboard panel for `summarize_orders` nested LLM call latency.

**Architecture:** Keep total tool latency recording in the agent loop unchanged. Add a low-cardinality `ai_tool_sub_llm_duration_seconds{tool="summarize_orders"}` histogram behind the existing metrics recorder boundary, then inject that recorder only into the production `summarize_orders` tool registration path. The tool records sub-LLM duration only when the nested `llm.Chat` call actually runs.

**Tech Stack:** Go, `log/slog`, Prometheus client, Prometheus `testutil`, Grafana dashboard JSON, repo `make grafana-sync` and `make preflight-go`.

---

## File Structure

- Modify `go/ai-service/internal/metrics/metrics.go`
  - Add `ToolSubLLMDuration`.
  - Extend `Recorder` and `NopRecorder`.
  - Implement `PromRecorder.RecordToolSubLLM`.
- Modify `go/ai-service/internal/metrics/metrics_test.go`
  - Add a focused test proving the new recorder observes the histogram.
- Modify `go/ai-service/internal/tools/orders.go`
  - Add a local `toolSubLLMRecorder` interface.
  - Add `rec` to `summarizeOrdersTool`.
  - Add `NewSummarizeOrdersToolWithRecorder`.
  - Time `t.llm.Chat`, log `sub_llm_duration_ms`, and record the metric on success and sub-LLM failure.
- Modify `go/ai-service/internal/tools/orders_test.go`
  - Add a fake sub-LLM recorder.
  - Add a local slog capture helper.
  - Assert success, sub-LLM error, and no-orders metric/log behavior.
- Modify `go/ai-service/cmd/server/main.go`
  - Register `summarize_orders` with `metrics.PromRecorder{}`.
- Modify `monitoring/grafana/dashboards/ai-pipeline.json`
  - Add the `summarize_orders Sub-LLM Latency` panel near `Tool Latency`.
- Modify `k8s/monitoring/configmaps/grafana-dashboards.yml`
  - Regenerate with `make grafana-sync`.

---

### Task 1: Add The Metrics Recorder Surface

**Files:**
- Modify: `go/ai-service/internal/metrics/metrics.go`
- Modify: `go/ai-service/internal/metrics/metrics_test.go`

- [ ] **Step 1: Write the failing metrics test**

Update the imports in `go/ai-service/internal/metrics/metrics_test.go` to include:

```go
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
```

Add this test to the same file:

```go
func TestPromRecorder_RecordToolSubLLM(t *testing.T) {
	ToolSubLLMDuration.Reset()
	r := PromRecorder{}
	r.RecordToolSubLLM("summarize_orders", 250*time.Millisecond)
	observer := ToolSubLLMDuration.WithLabelValues("summarize_orders")
	metric, ok := observer.(prometheus.Metric)
	if !ok {
		t.Fatalf("histogram observer does not implement prometheus.Metric")
	}
	dtoMetric := &dto.Metric{}
	if err := metric.Write(dtoMetric); err != nil {
		t.Fatalf("write metric: %v", err)
	}
	if got := dtoMetric.GetHistogram().GetSampleCount(); got != 1 {
		t.Errorf("sub-LLM histogram count = %d", got)
	}
}
```

- [ ] **Step 2: Run the failing metrics test**

Run:

```bash
cd go/ai-service && go test ./internal/metrics -run TestPromRecorder_RecordToolSubLLM -count=1
```

Expected: FAIL with errors that `ToolSubLLMDuration` and `RecordToolSubLLM` are undefined.

- [ ] **Step 3: Add the histogram and recorder method**

In `go/ai-service/internal/metrics/metrics.go`, add the histogram after `ToolDuration`:

```go
	ToolSubLLMDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "ai_tool_sub_llm_duration_seconds",
		Help:    "Nested LLM call latency within AI tools.",
		Buckets: prometheus.DefBuckets,
	}, []string{"tool"})
```

Extend `Recorder`:

```go
type Recorder interface {
	RecordTurn(outcome string, steps int, dur time.Duration)
	RecordTool(name, outcome string, dur time.Duration)
	RecordToolSubLLM(tool string, dur time.Duration)
	RecordOllamaCall(model, operation string, dur time.Duration, promptTokens, completionTokens int, evalDurNs int)
}
```

Add the Prometheus implementation after `RecordTool`:

```go
func (PromRecorder) RecordToolSubLLM(tool string, dur time.Duration) {
	ToolSubLLMDuration.WithLabelValues(tool).Observe(dur.Seconds())
}
```

Add the no-op implementation beside the other `NopRecorder` methods:

```go
func (NopRecorder) RecordToolSubLLM(string, time.Duration) {}
```

- [ ] **Step 4: Run metrics tests**

Run:

```bash
cd go/ai-service && go test ./internal/metrics -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit metrics surface**

```bash
git add go/ai-service/internal/metrics/metrics.go go/ai-service/internal/metrics/metrics_test.go
git commit -m "Add AI tool sub-LLM duration metric"
```

---

### Task 2: Instrument summarize_orders And Test Behavior

**Files:**
- Modify: `go/ai-service/internal/tools/orders.go`
- Modify: `go/ai-service/internal/tools/orders_test.go`

- [ ] **Step 1: Add test imports and helpers**

Update the imports in `go/ai-service/internal/tools/orders_test.go` to include:

```go
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
```

Add these helpers after `summarizerLLM`:

```go
type fakeSubLLMRecorder struct {
	calls []recordedSubLLMCall
}

type recordedSubLLMCall struct {
	tool string
	dur  time.Duration
}

func (f *fakeSubLLMRecorder) RecordToolSubLLM(tool string, dur time.Duration) {
	f.calls = append(f.calls, recordedSubLLMCall{tool: tool, dur: dur})
}

type capturedLog struct {
	message string
	attrs   map[string]any
}

type captureHandler struct {
	mu      sync.Mutex
	records []capturedLog
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *captureHandler) Handle(ctx context.Context, r slog.Record) error {
	attrs := map[string]any{}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, capturedLog{message: r.Message, attrs: attrs})
	return nil
}

func (h *captureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *captureHandler) WithGroup(name string) slog.Handler {
	return h
}

func captureLogs(t *testing.T) *captureHandler {
	t.Helper()
	handler := &captureHandler{}
	prev := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() {
		slog.SetDefault(prev)
	})
	return handler
}

func (h *captureHandler) hasRecord(message string, fields ...string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.ContainsFunc(h.records, func(r capturedLog) bool {
		if r.message != message {
			return false
		}
		for _, field := range fields {
			if _, ok := r.attrs[field]; !ok {
				return false
			}
		}
		return true
	})
}
```

- [ ] **Step 2: Add the failing success metric/log assertions**

In `TestSummarizeOrdersTool_Success`, replace the tool construction with a recorder-enabled constructor and add assertions:

```go
	rec := &fakeSubLLMRecorder{}
	logs := captureLogs(t)
	tool := NewSummarizeOrdersToolWithRecorder(fakeAPI, fakeLLM, rec)
```

Add after the existing `seenMsg` assertion:

```go
	if len(rec.calls) != 1 {
		t.Fatalf("sub-LLM metric calls = %d, want 1", len(rec.calls))
	}
	if rec.calls[0].tool != "summarize_orders" {
		t.Errorf("sub-LLM metric tool = %q", rec.calls[0].tool)
	}
	if rec.calls[0].dur <= 0 {
		t.Errorf("sub-LLM metric duration = %v, want positive", rec.calls[0].dur)
	}
	if !logs.hasRecord("tool result", "duration_ms", "sub_llm_duration_ms") {
		t.Errorf("expected success log to include duration_ms and sub_llm_duration_ms")
	}
```

- [ ] **Step 3: Add failing sub-LLM error and no-orders tests**

Add these tests to `go/ai-service/internal/tools/orders_test.go`:

```go
func TestSummarizeOrdersTool_SubLLMErrorRecordsDuration(t *testing.T) {
	fakeAPI := &fakeOrdersAPI{listOut: []clients.Order{
		{ID: "o1", Status: "paid", Total: 12999, CreatedAt: time.Now().Add(-48 * time.Hour)},
	}}
	fakeLLM := &summarizerLLM{err: errors.New("model unavailable")}
	rec := &fakeSubLLMRecorder{}
	logs := captureLogs(t)
	tool := NewSummarizeOrdersToolWithRecorder(fakeAPI, fakeLLM, rec)

	_, err := tool.Call(ctxWithJWT("tok"), json.RawMessage(`{}`), "user-1")
	if err == nil {
		t.Fatal("expected sub-LLM error")
	}
	if !strings.Contains(err.Error(), "summarize_orders: sub-llm: model unavailable") {
		t.Fatalf("error = %v", err)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("sub-LLM metric calls = %d, want 1", len(rec.calls))
	}
	if rec.calls[0].tool != "summarize_orders" {
		t.Errorf("sub-LLM metric tool = %q", rec.calls[0].tool)
	}
	if rec.calls[0].dur <= 0 {
		t.Errorf("sub-LLM metric duration = %v, want positive", rec.calls[0].dur)
	}
	if !logs.hasRecord("tool error", "error", "sub_llm_duration_ms") {
		t.Errorf("expected error log to include error and sub_llm_duration_ms")
	}
}

func TestSummarizeOrdersTool_NoOrdersSkipsSubLLMMetric(t *testing.T) {
	fakeAPI := &fakeOrdersAPI{listOut: nil}
	fakeLLM := &summarizerLLM{reply: "should not be called"}
	rec := &fakeSubLLMRecorder{}
	tool := NewSummarizeOrdersToolWithRecorder(fakeAPI, fakeLLM, rec)

	res, err := tool.Call(ctxWithJWT("tok"), json.RawMessage(`{}`), "user-1")
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	m := res.Content.(map[string]any)
	if m["summary"] != "You have no orders yet." {
		t.Errorf("summary = %+v", m)
	}
	if fakeLLM.seenMsg != nil {
		t.Error("expected sub-LLM to be skipped on empty order list")
	}
	if len(rec.calls) != 0 {
		t.Fatalf("sub-LLM metric calls = %d, want 0", len(rec.calls))
	}
}
```

- [ ] **Step 4: Run failing tool tests**

Run:

```bash
cd go/ai-service && go test ./internal/tools -run 'TestSummarizeOrdersTool_(Success|SubLLMErrorRecordsDuration|NoOrdersSkipsSubLLMMetric)$' -count=1
```

Expected: FAIL with `NewSummarizeOrdersToolWithRecorder` undefined.

- [ ] **Step 5: Add the recorder dependency and constructor**

In `go/ai-service/internal/tools/orders.go`, update `summarizeOrdersTool` and constructors:

```go
type toolSubLLMRecorder interface {
	RecordToolSubLLM(tool string, dur time.Duration)
}

type summarizeOrdersTool struct {
	api ordersAPI
	llm llm.Client
	rec toolSubLLMRecorder
}
```

Replace the existing constructor with:

```go
// NewSummarizeOrdersTool builds a tool that lists the user's recent orders and
// asks a small sub-LLM call to summarize them. It reuses the parent turn's
// context so the agent's wall-clock timeout still covers the sub-call.
func NewSummarizeOrdersTool(api ordersAPI, llmc llm.Client) Tool {
	return NewSummarizeOrdersToolWithRecorder(api, llmc, nil)
}

func NewSummarizeOrdersToolWithRecorder(api ordersAPI, llmc llm.Client, rec toolSubLLMRecorder) Tool {
	return &summarizeOrdersTool{api: api, llm: llmc, rec: rec}
}
```

- [ ] **Step 6: Time, log, and record the nested LLM call**

Replace the `t.llm.Chat` block in `summarizeOrdersTool.Call` with:

```go
	llmStart := time.Now()
	resp, err := t.llm.Chat(ctx, []llm.Message{
		{Role: llm.RoleUser, Content: prompt},
	}, nil)
	subLLMDuration := time.Since(llmStart)
	if t.rec != nil {
		t.rec.RecordToolSubLLM("summarize_orders", subLLMDuration)
	}
	if err != nil {
		slog.WarnContext(ctx,
			"tool error",
			"tool", "summarize_orders",
			"error", err.Error(),
			"sub_llm_duration_ms", subLLMDuration.Milliseconds(),
		)
		return Result{}, fmt.Errorf("summarize_orders: sub-llm: %w", err)
	}
	slog.InfoContext(ctx,
		"tool result",
		"tool", "summarize_orders",
		"order_count", len(orders),
		"duration_ms", time.Since(start).Milliseconds(),
		"sub_llm_duration_ms", subLLMDuration.Milliseconds(),
	)
```

Keep the existing output construction immediately after this block:

```go
	out := map[string]any{"summary": resp.Content, "order_count": len(orders)}
	return Result{Content: out, Display: out}, nil
```

- [ ] **Step 7: Run focused tool tests**

Run:

```bash
cd go/ai-service && go test ./internal/tools -run 'TestSummarizeOrdersTool' -count=1
```

Expected: PASS.

- [ ] **Step 8: Run all tool tests**

Run:

```bash
cd go/ai-service && go test ./internal/tools -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit tool instrumentation**

```bash
git add go/ai-service/internal/tools/orders.go go/ai-service/internal/tools/orders_test.go
git commit -m "Instrument summarize orders sub-LLM timing"
```

---

### Task 3: Wire Production Registration

**Files:**
- Modify: `go/ai-service/cmd/server/main.go`

- [ ] **Step 1: Update production tool registration**

In `go/ai-service/cmd/server/main.go`, replace:

```go
	registry.Register(tools.NewSummarizeOrdersTool(ecomClient, llmc))
```

with:

```go
	registry.Register(tools.NewSummarizeOrdersToolWithRecorder(ecomClient, llmc, metrics.PromRecorder{}))
```

The file already imports `github.com/kabradshaw1/portfolio/go/ai-service/internal/metrics`, so no import change is needed.

- [ ] **Step 2: Run server package tests**

Run:

```bash
cd go/ai-service && go test ./cmd/server -count=1
```

Expected: PASS.

- [ ] **Step 3: Run compile check across ai-service packages**

Run:

```bash
cd go/ai-service && go test ./... -run '^$' -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit production wiring**

```bash
git add go/ai-service/cmd/server/main.go
git commit -m "Wire summarize orders sub-LLM metrics"
```

---

### Task 4: Add Dashboard Panel And Sync ConfigMap

**Files:**
- Modify: `monitoring/grafana/dashboards/ai-pipeline.json`
- Modify: `k8s/monitoring/configmaps/grafana-dashboards.yml`

- [ ] **Step 1: Add the dashboard panel**

Edit `monitoring/grafana/dashboards/ai-pipeline.json`.

Find the panel with:

```json
"title": "Tool Latency"
```

Insert this panel immediately after the `Tool Latency` panel and before `Agent Steps/Turn`:

```json
{
  "title": "summarize_orders Sub-LLM Latency",
  "type": "timeseries",
  "gridPos": {
    "h": 6,
    "w": 8,
    "x": 16,
    "y": 28
  },
  "id": 18,
  "datasource": {
    "type": "prometheus",
    "uid": ""
  },
  "targets": [
    {
      "expr": "histogram_quantile(0.5, rate(ai_tool_sub_llm_duration_seconds_bucket{tool=\"summarize_orders\"}[5m]))",
      "legendFormat": "sub-LLM p50",
      "refId": "A"
    },
    {
      "expr": "histogram_quantile(0.95, rate(ai_tool_sub_llm_duration_seconds_bucket{tool=\"summarize_orders\"}[5m]))",
      "legendFormat": "sub-LLM p95",
      "refId": "B"
    }
  ],
  "fieldConfig": {
    "defaults": {
      "unit": "s",
      "custom": {
        "drawStyle": "line",
        "lineWidth": 1,
        "fillOpacity": 10,
        "showPoints": "never"
      }
    },
    "overrides": []
  },
  "options": {
    "tooltip": {
      "mode": "multi"
    },
    "legend": {
      "displayMode": "list",
      "placement": "bottom"
    }
  }
}
```

Move the existing `Agent Steps/Turn` panel to the next row by changing its `gridPos` to:

```json
"gridPos": {
  "h": 6,
  "w": 8,
  "x": 0,
  "y": 34
}
```

Use `id: 18`; the current dashboard's highest panel ID is `17`.

- [ ] **Step 2: Validate dashboard JSON**

Run:

```bash
python3 -m json.tool monitoring/grafana/dashboards/ai-pipeline.json >/tmp/ai-pipeline.json
```

Expected: exits 0 with no output.

- [ ] **Step 3: Sync Grafana ConfigMap**

Run:

```bash
make grafana-sync
```

Expected output includes:

```text
=== Grafana: regenerating ConfigMap from dashboard JSON ===
regenerated
```

- [ ] **Step 4: Check dashboard sync**

Run:

```bash
make grafana-sync-check
```

Expected output includes:

```text
grafana dashboards in sync
```

- [ ] **Step 5: Commit dashboard assets**

```bash
git add monitoring/grafana/dashboards/ai-pipeline.json k8s/monitoring/configmaps/grafana-dashboards.yml
git commit -m "Add summarize orders sub-LLM dashboard panel"
```

---

### Task 5: Final Verification

**Files:**
- No new files.
- Verify all files changed in Tasks 1 through 4.

- [ ] **Step 1: Run Go preflight**

Run:

```bash
make preflight-go
```

Expected: PASS.

- [ ] **Step 2: Run dashboard sync check**

Run:

```bash
make grafana-sync-check
```

Expected: PASS with `grafana dashboards in sync`.

- [ ] **Step 3: Inspect final diff**

Run:

```bash
git status --short
git log --oneline -5
```

Expected:

- Only unrelated pre-existing user changes, if any, remain unstaged.
- The latest commits include the metrics, tool instrumentation, production wiring, and dashboard panel commits.

- [ ] **Step 4: Report verification results**

Include:

- `make preflight-go` result.
- `make grafana-sync-check` result.
- Any files intentionally left untouched because they were unrelated user changes.
