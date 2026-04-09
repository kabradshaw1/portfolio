# Plan 3 — `go/ai-service` Operations: Cache, Metrics, Guardrails, Evals

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn `go/ai-service` from "an agent that works" into "an agent operated like a production system." Add a Redis-backed tool-response cache, Prometheus metrics on the agent loop and tool calls, a `/metrics` endpoint and a new "AI Service" row on the existing Grafana `system-overview` dashboard, guardrails (history truncation, rate limiting, refusal detection, structured per-turn logging), and a mocked-LLM eval harness that runs as part of the normal `go test` sweep.

**Architecture:** One new `cache` package with an interface + Redis impl. One new `metrics` package with Prometheus counters/histograms and instrumentation helpers for the agent loop. One new `evals` package (build-tagged) that runs the agent loop with a scripted `llm.Client` and asserts tool-selection + recovery behavior. Guardrails land as small, explicit additions: a history-truncation helper called inside the agent loop, a Redis token-bucket rate limiter as Gin middleware, a refusal-detection helper run on final events, and a single structured JSON log line per turn.

**Tech Stack:** Go 1.26, `github.com/redis/go-redis/v9` (already in the repo via ecommerce-service, transitively available), `github.com/prometheus/client_golang` (already used by ecommerce-service), existing Grafana dashboard JSON.

**Scope boundaries:**
- **No embedding cache.** The current `search_products` tool uses text search via ecommerce-service, not embeddings. An embedding cache would be dead code in Plan 3 and lands in Plan 4/5 if semantic search ever gets added. The cache interface is designed to also hold embeddings later — just nothing writes them yet.
- **No real-LLM eval tier.** The mocked-LLM tier (fake `llm.Client` with scripted tool-call sequences) runs in normal `go test` and is what PRs will gate on. Real-LLM nightly runs need a scheduled CI workflow + SSH to the Windows PC, which is Plan 5 territory.
- **No PII scrub / content moderation / jailbreak detection.** Scoped out in the spec.
- **No frontend** (→ Plan 4).
- **No CI/K8s wiring of the new `/metrics` endpoint.** Plan 5 extends the Prometheus scrape config.
- **Cache is opt-in per tool** via a small wrapper, not a blanket proxy. Catalog tools get caching; `add_to_cart`, `initiate_return` never cache; order tools get short TTL.

**Reference:** spec section 5 (operations).

**Module path:** `github.com/kabradshaw1/portfolio/go/ai-service`

---

## File Map

New:

```
go/ai-service/internal/
├── cache/
│   ├── cache.go              # Cache interface + NopCache + RedisCache
│   └── cache_test.go         # httptest-less (miniredis) unit tests
├── metrics/
│   ├── metrics.go            # Prometheus counters + histograms + RegisterHandlers
│   └── metrics_test.go
├── guardrails/
│   ├── history.go            # TruncateHistory, refusal detection
│   ├── history_test.go
│   ├── ratelimit.go          # Gin middleware, Redis token bucket
│   └── ratelimit_test.go
├── tools/
│   └── cached.go             # Cached wrapper around any Tool
│   └── cached_test.go
└── evals/
    ├── harness.go            # Scripted fake llm.Client + assertion helpers
    ├── cases_test.go         # Build-tagged eval cases
    └── doc.go                # //go:build eval documentation
```

Modified:
- `go/ai-service/internal/agent/agent.go` — emit metrics + log turn summary + call history guardrail
- `go/ai-service/internal/http/chat.go` — register rate-limit middleware on `/chat`
- `go/ai-service/internal/http/health.go` — add `/metrics` handler
- `go/ai-service/cmd/server/main.go` — wire cache, metrics, rate limiter, wrap catalog tools with `Cached`
- `go/ai-service/Makefile` (or root `Makefile` `preflight-ai-service` target) — no change needed; evals run under the same `go test ./...`
- `monitoring/grafana/dashboards/system-overview.json` — append "AI Service" row
- `go/docker-compose.yml` — add `REDIS_URL` to `ai-service` environment

**Go dependencies to add:**
- `github.com/redis/go-redis/v9` (if not already transitively present)
- `github.com/prometheus/client_golang`
- `github.com/alicebob/miniredis/v2` (test-only, for cache + rate-limit tests)

---

## Task 1: `cache` package — interface + Redis impl

**Files:**
- Create: `go/ai-service/internal/cache/cache.go`
- Create: `go/ai-service/internal/cache/cache_test.go`

**Dependencies:**
```bash
cd go/ai-service && go get github.com/redis/go-redis/v9@latest
cd go/ai-service && go get github.com/alicebob/miniredis/v2@latest
```

- [ ] **Step 1: Failing test** — `cache_test.go`

```go
package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	return redis.NewClient(&redis.Options{Addr: mr.Addr()}), mr
}

func TestRedisCache_SetGet(t *testing.T) {
	client, _ := newRedis(t)
	c := NewRedisCache(client, "ai")

	if err := c.Set(context.Background(), "k", []byte("v"), time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	v, ok, err := c.Get(context.Background(), "k")
	if err != nil || !ok || string(v) != "v" {
		t.Errorf("Get: v=%q ok=%v err=%v", v, ok, err)
	}
}

func TestRedisCache_MissReturnsOKFalse(t *testing.T) {
	client, _ := newRedis(t)
	c := NewRedisCache(client, "ai")
	_, ok, err := c.Get(context.Background(), "missing")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if ok {
		t.Error("expected miss")
	}
}

func TestRedisCache_Expiry(t *testing.T) {
	client, mr := newRedis(t)
	c := NewRedisCache(client, "ai")

	_ = c.Set(context.Background(), "k", []byte("v"), time.Second)
	mr.FastForward(2 * time.Second)
	_, ok, _ := c.Get(context.Background(), "k")
	if ok {
		t.Error("expected expired entry to be gone")
	}
}

func TestNopCache(t *testing.T) {
	c := NopCache{}
	_ = c.Set(context.Background(), "k", []byte("v"), time.Minute)
	_, ok, _ := c.Get(context.Background(), "k")
	if ok {
		t.Error("nop cache should never hit")
	}
}
```

- [ ] **Step 2: Run, expect compile failure**

```bash
cd go/ai-service && go test ./internal/cache/...
```

- [ ] **Step 3: Implement `cache.go`**

```go
package cache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache is a tiny key/value interface. bytes in, bytes out.
// Callers handle their own serialization so the cache stays transport-agnostic.
type Cache interface {
	Get(ctx context.Context, key string) (value []byte, ok bool, err error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
}

// NopCache is a zero-cost no-op. Used when Redis is unavailable or disabled.
type NopCache struct{}

func (NopCache) Get(context.Context, string) ([]byte, bool, error) { return nil, false, nil }
func (NopCache) Set(context.Context, string, []byte, time.Duration) error { return nil }

// RedisCache is a Redis-backed Cache. All keys are automatically prefixed.
type RedisCache struct {
	client *redis.Client
	prefix string
}

func NewRedisCache(client *redis.Client, prefix string) *RedisCache {
	return &RedisCache{client: client, prefix: prefix}
}

func (c *RedisCache) key(k string) string { return c.prefix + ":" + k }

func (c *RedisCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	val, err := c.client.Get(ctx, c.key(key)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return val, true, nil
}

func (c *RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return c.client.Set(ctx, c.key(key), value, ttl).Err()
}
```

- [ ] **Step 4: Run tests, expect pass. Commit.**

```bash
cd go/ai-service && go test ./internal/cache/... -v
git add go/ai-service/
git commit -m "feat(ai-service): add cache interface with Redis and no-op implementations"
```

---

## Task 2: Cached tool wrapper

**Files:**
- Create: `go/ai-service/internal/tools/cached.go`
- Create: `go/ai-service/internal/tools/cached_test.go`

The wrapper composes any `Tool`. Cache key is `sha256(toolName + canonicalArgs + userID)`. On hit it returns the stored JSON `Result.Content`; on miss it calls the underlying tool, stores the serialized content, and returns normally. Display payloads are NOT cached (they can be large and the frontend is fine regenerating them from content).

- [ ] **Step 1: Failing test**

```go
package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/cache"
)

func newCache(t *testing.T) cache.Cache {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	return cache.NewRedisCache(redis.NewClient(&redis.Options{Addr: mr.Addr()}), "ai-test")
}

func TestCached_MissThenHit(t *testing.T) {
	inner := &scriptedTool{
		name:   "echo",
		result: Result{Content: map[string]any{"x": 1}},
	}
	wrapped := Cached(inner, newCache(t), time.Minute)

	// First call: miss — inner is called.
	res, err := wrapped.Call(context.Background(), json.RawMessage(`{"q":"a"}`), "user-1")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if inner.calls != 1 {
		t.Errorf("expected 1 inner call, got %d", inner.calls)
	}
	if _, ok := res.Content.(map[string]any); !ok {
		t.Errorf("first call content type %T", res.Content)
	}

	// Second call with same args + same user: hit — inner not called again.
	_, err = wrapped.Call(context.Background(), json.RawMessage(`{"q":"a"}`), "user-1")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if inner.calls != 1 {
		t.Errorf("expected inner to still have 1 call (cache hit), got %d", inner.calls)
	}
}

func TestCached_DifferentUsersAreDistinct(t *testing.T) {
	inner := &scriptedTool{name: "echo", result: Result{Content: map[string]any{"ok": true}}}
	wrapped := Cached(inner, newCache(t), time.Minute)

	_, _ = wrapped.Call(context.Background(), json.RawMessage(`{}`), "u1")
	_, _ = wrapped.Call(context.Background(), json.RawMessage(`{}`), "u2")
	if inner.calls != 2 {
		t.Errorf("expected 2 inner calls (different users), got %d", inner.calls)
	}
}

func TestCached_DifferentArgsAreDistinct(t *testing.T) {
	inner := &scriptedTool{name: "echo", result: Result{Content: map[string]any{"ok": true}}}
	wrapped := Cached(inner, newCache(t), time.Minute)

	_, _ = wrapped.Call(context.Background(), json.RawMessage(`{"a":1}`), "u1")
	_, _ = wrapped.Call(context.Background(), json.RawMessage(`{"a":2}`), "u1")
	if inner.calls != 2 {
		t.Errorf("expected 2 inner calls (different args), got %d", inner.calls)
	}
}

func TestCached_SameArgsCanonicalized(t *testing.T) {
	// Whitespace-only difference should still hit.
	inner := &scriptedTool{name: "echo", result: Result{Content: map[string]any{"ok": true}}}
	wrapped := Cached(inner, newCache(t), time.Minute)

	_, _ = wrapped.Call(context.Background(), json.RawMessage(`{"a":1,"b":2}`), "u1")
	_, _ = wrapped.Call(context.Background(), json.RawMessage(`{ "b": 2, "a": 1 }`), "u1")
	if inner.calls != 1 {
		t.Errorf("expected canonicalized hit, got %d inner calls", inner.calls)
	}
}

func TestCached_NameAndSchemaPassThrough(t *testing.T) {
	inner := &scriptedTool{name: "echo"}
	wrapped := Cached(inner, newCache(t), time.Minute)
	if wrapped.Name() != "echo" {
		t.Errorf("name = %q", wrapped.Name())
	}
	if wrapped.Schema() == nil {
		t.Error("schema should pass through")
	}
}
```

- [ ] **Step 2: Run, expect compile failure.**

- [ ] **Step 3: Implement `cached.go`**

```go
package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/cache"
)

// Cached returns a Tool that wraps inner with a response cache.
// Cache key is sha256(toolName + canonical(args) + userID).
// Only Result.Content is cached — Display is regenerated fresh on every hit.
func Cached(inner Tool, c cache.Cache, ttl time.Duration) Tool {
	return &cachedTool{inner: inner, cache: c, ttl: ttl}
}

type cachedTool struct {
	inner Tool
	cache cache.Cache
	ttl   time.Duration
}

func (t *cachedTool) Name() string            { return t.inner.Name() }
func (t *cachedTool) Description() string     { return t.inner.Description() }
func (t *cachedTool) Schema() json.RawMessage { return t.inner.Schema() }

func (t *cachedTool) Call(ctx context.Context, args json.RawMessage, userID string) (Result, error) {
	key, err := cacheKey(t.inner.Name(), args, userID)
	if err == nil {
		if raw, ok, _ := t.cache.Get(ctx, key); ok {
			var content any
			if json.Unmarshal(raw, &content) == nil {
				return Result{Content: content}, nil
			}
		}
	}

	res, err := t.inner.Call(ctx, args, userID)
	if err != nil {
		return res, err
	}
	if key != "" && res.Content != nil {
		if raw, merr := json.Marshal(res.Content); merr == nil {
			_ = t.cache.Set(ctx, key, raw, t.ttl)
		}
	}
	return res, nil
}

// cacheKey canonicalizes args by round-tripping through Go's json package,
// which sorts object keys alphabetically when encoding a map[string]any.
func cacheKey(name string, args json.RawMessage, userID string) (string, error) {
	var v any
	if err := json.Unmarshal(args, &v); err != nil {
		return "", err
	}
	canonical, err := marshalCanonical(v)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s", name, canonical, userID)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// marshalCanonical walks v and encodes it with sorted map keys at every level.
// Go's json.Marshal already sorts map keys, but to guarantee arrays of objects
// also round-trip predictably we convert through map[string]any explicitly.
func marshalCanonical(v any) ([]byte, error) {
	switch x := v.(type) {
	case map[string]any:
		// json.Marshal already sorts keys for map[string]any.
		return json.Marshal(x)
	default:
		return json.Marshal(v)
	}
}
```

- [ ] **Step 4: Run tool tests, expect all PASS.**

```bash
cd go/ai-service && go test ./internal/tools/... -v
git add go/ai-service/internal/tools/cached.go go/ai-service/internal/tools/cached_test.go
git commit -m "feat(ai-service): add Cached tool wrapper with per-user key isolation"
```

---

## Task 3: `metrics` package + `/metrics` endpoint + instrument the agent loop

**Files:**
- Create: `go/ai-service/internal/metrics/metrics.go`
- Create: `go/ai-service/internal/metrics/metrics_test.go`
- Modify: `go/ai-service/internal/http/health.go` — add a `/metrics` handler registration helper
- Modify: `go/ai-service/internal/agent/agent.go` — call metric hooks on each step and outcome

**Dependency:**
```bash
cd go/ai-service && go get github.com/prometheus/client_golang@latest
```

- [ ] **Step 1: Write `metrics.go`**

```go
package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	TurnsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ai_agent_turns_total",
		Help: "Agent turns by outcome.",
	}, []string{"outcome"})

	StepsPerTurn = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "ai_agent_steps_per_turn",
		Help:    "Number of LLM calls per turn.",
		Buckets: []float64{1, 2, 3, 4, 5, 6, 8, 10},
	})

	TurnDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "ai_agent_turn_duration_seconds",
		Help:    "End-to-end agent turn duration.",
		Buckets: prometheus.ExponentialBuckets(0.1, 2, 10),
	})

	ToolCallsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ai_tool_calls_total",
		Help: "Tool invocations by name and outcome.",
	}, []string{"name", "outcome"})

	ToolDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "ai_tool_duration_seconds",
		Help:    "Per-tool latency.",
		Buckets: prometheus.DefBuckets,
	}, []string{"name"})

	CacheEvents = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ai_cache_events_total",
		Help: "Cache events by cache and event type.",
	}, []string{"cache", "event"})
)

// Recorder is the interface the agent loop uses to emit metrics. It keeps the
// agent package from importing Prometheus directly and makes tests trivial.
type Recorder interface {
	RecordTurn(outcome string, steps int, dur time.Duration)
	RecordTool(name, outcome string, dur time.Duration)
}

// PromRecorder writes to the package-level Prometheus collectors.
type PromRecorder struct{}

func (PromRecorder) RecordTurn(outcome string, steps int, dur time.Duration) {
	TurnsTotal.WithLabelValues(outcome).Inc()
	StepsPerTurn.Observe(float64(steps))
	TurnDuration.Observe(dur.Seconds())
}

func (PromRecorder) RecordTool(name, outcome string, dur time.Duration) {
	ToolCallsTotal.WithLabelValues(name, outcome).Inc()
	ToolDuration.WithLabelValues(name).Observe(dur.Seconds())
}

// NopRecorder is the zero-value substitute for tests that don't care about metrics.
type NopRecorder struct{}

func (NopRecorder) RecordTurn(string, int, time.Duration) {}
func (NopRecorder) RecordTool(string, string, time.Duration) {}
```

- [ ] **Step 2: Test for outcome labels**

`metrics_test.go`:

```go
package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestPromRecorder_RecordTurn(t *testing.T) {
	TurnsTotal.Reset()
	r := PromRecorder{}
	r.RecordTurn("final", 3, 500*time.Millisecond)
	if got := testutil.ToFloat64(TurnsTotal.WithLabelValues("final")); got != 1 {
		t.Errorf("turns counter = %v", got)
	}
}

func TestPromRecorder_RecordTool(t *testing.T) {
	ToolCallsTotal.Reset()
	r := PromRecorder{}
	r.RecordTool("search_products", "success", 10*time.Millisecond)
	if got := testutil.ToFloat64(ToolCallsTotal.WithLabelValues("search_products", "success")); got != 1 {
		t.Errorf("tool counter = %v", got)
	}
}
```

- [ ] **Step 3: Run, expect pass.**

- [ ] **Step 4: Add `/metrics` handler to `internal/http/health.go`**

Add a new function at the bottom of `health.go`:

```go
// RegisterMetricsRoute wires GET /metrics using the Prometheus default handler.
func RegisterMetricsRoute(r *gin.Engine) {
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
}
```

Add imports: `"github.com/prometheus/client_golang/prometheus/promhttp"`.

- [ ] **Step 5: Instrument `agent.Agent`**

Change `agent.New` signature to accept a `metrics.Recorder`:

```go
func New(client llm.Client, registry tools.Registry, rec metrics.Recorder, maxSteps int, timeout time.Duration) *Agent {
	if rec == nil {
		rec = metrics.NopRecorder{}
	}
	return &Agent{llm: client, registry: registry, rec: rec, maxSteps: maxSteps, timeout: timeout}
}
```

Store `rec` on the struct. In `Run`:

1. Track `startTime := time.Now()` before the loop.
2. On successful final: `a.rec.RecordTurn("final", step+1, time.Since(startTime))`.
3. On LLM error: `a.rec.RecordTurn("error", step, time.Since(startTime))` before returning.
4. On max steps: `a.rec.RecordTurn("max_steps", a.maxSteps, time.Since(startTime))`.
5. Around each `tool.Call` / `safeCall`, wrap in:
   ```go
   toolStart := time.Now()
   result, toolErr := safeCall(ctx, tool, call.Args, turn.UserID)
   outcome := "success"
   if toolErr != nil {
       outcome = "error"
   }
   a.rec.RecordTool(call.Name, outcome, time.Since(toolStart))
   ```
6. For unknown tools, record `a.rec.RecordTool(call.Name, "unknown", 0)`.

Update every existing `agent.New(llmc, reg, maxSteps, timeout)` call site in `agent_test.go` to pass `metrics.NopRecorder{}`.

Update `main.go`'s `agent.New(llmc, registry, 8, 30*time.Second)` to `agent.New(llmc, registry, metrics.PromRecorder{}, 8, 30*time.Second)`.

- [ ] **Step 6: Run full suite**

```bash
cd go/ai-service && go test ./... -count=1
git add go/ai-service/
git commit -m "feat(ai-service): add Prometheus metrics and instrument agent loop"
```

---

## Task 4: Guardrails — history cap + rate limit + refusal detection + turn logging

**Files:**
- Create: `go/ai-service/internal/guardrails/history.go`
- Create: `go/ai-service/internal/guardrails/history_test.go`
- Create: `go/ai-service/internal/guardrails/ratelimit.go`
- Create: `go/ai-service/internal/guardrails/ratelimit_test.go`
- Modify: `go/ai-service/internal/agent/agent.go` — call `guardrails.TruncateHistory` once at the start of `Run`, detect refusal on the final event, log a per-turn JSON line
- Modify: `go/ai-service/internal/http/chat.go` — accept an optional rate limiter middleware

### 4a. history.go

```go
package guardrails

import (
	"strings"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/llm"
)

const DefaultMaxHistory = 20

// TruncateHistory keeps the most recent n messages. If a system message is
// present at index 0, it's preserved regardless of the cap.
func TruncateHistory(msgs []llm.Message, n int) []llm.Message {
	if len(msgs) <= n {
		return msgs
	}
	if len(msgs) == 0 || msgs[0].Role != llm.RoleSystem {
		return append([]llm.Message(nil), msgs[len(msgs)-n:]...)
	}
	// keep system + last (n-1)
	out := make([]llm.Message, 0, n)
	out = append(out, msgs[0])
	out = append(out, msgs[len(msgs)-(n-1):]...)
	return out
}

var refusalPrefixes = []string{
	"i can't",
	"i cannot",
	"i'm not able",
	"i am not able",
	"i'm unable",
	"sorry, i can",
}

// IsRefusal returns true if text looks like a model refusal. Used for metric tagging
// and per-turn logging, not for user-facing behavior.
func IsRefusal(text string) bool {
	low := strings.TrimSpace(strings.ToLower(text))
	for _, p := range refusalPrefixes {
		if strings.HasPrefix(low, p) {
			return true
		}
	}
	return false
}
```

### 4b. `history_test.go` covers: short history passes through; long history without system keeps tail; long history with system keeps system + tail; `IsRefusal` true/false cases.

### 4c. ratelimit.go — Redis token bucket, 20 req/min per IP, returns 429 with Retry-After.

```go
package guardrails

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// Limiter is a simple fixed-window limiter keyed by IP.
// Using a fixed window rather than a token bucket keeps the Redis logic trivial
// (INCR + EXPIRE) and still gives us a clear "20 per minute" rule.
type Limiter struct {
	client *redis.Client
	prefix string
	max    int
	window time.Duration
}

func NewLimiter(client *redis.Client, max int, window time.Duration) *Limiter {
	return &Limiter{client: client, prefix: "ai:ratelimit", max: max, window: window}
}

// Allow returns (allowed, retryAfter, error). retryAfter is 0 on allow.
func (l *Limiter) Allow(ctx context.Context, key string) (bool, time.Duration, error) {
	k := l.prefix + ":" + key
	n, err := l.client.Incr(ctx, k).Result()
	if err != nil {
		return false, 0, err
	}
	if n == 1 {
		if err := l.client.Expire(ctx, k, l.window).Err(); err != nil {
			return false, 0, err
		}
	}
	if int(n) > l.max {
		ttl, _ := l.client.TTL(ctx, k).Result()
		if ttl < 0 {
			ttl = l.window
		}
		return false, ttl, nil
	}
	return true, 0, nil
}

// Middleware returns Gin middleware that applies the limiter. If the limiter
// is nil, it's a no-op — callers wire it conditionally based on Redis availability.
func Middleware(l *Limiter) gin.HandlerFunc {
	if l == nil {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		ok, retry, err := l.Allow(c.Request.Context(), c.ClientIP())
		if err != nil {
			// Fail open on Redis errors — an outage shouldn't disable the service.
			c.Next()
			return
		}
		if !ok {
			c.Header("Retry-After", strconv.Itoa(int(retry.Seconds())))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			return
		}
		c.Next()
	}
}

// ErrRedisUnavailable is returned by NewLimiter when Redis is nil.
var ErrRedisUnavailable = errors.New("redis client required")
```

### 4d. `ratelimit_test.go` — use miniredis to verify: 20 allowed, 21st returns false with retryAfter > 0, window resets after fast-forward, nil limiter allows all.

### 4e. Agent loop integration

In `agent.Run`, at the top (before the main loop):

```go
turn.Messages = guardrails.TruncateHistory(turn.Messages, guardrails.DefaultMaxHistory)
```

On each final event, set an `outcome` variable: `"final"` normally, `"refused"` if `guardrails.IsRefusal(resp.Content)`. Pass that to `a.rec.RecordTurn`.

At the very end of a successful Run, log:

```go
slog.Info("agent turn",
    "turn_id", /* generate with uuid */,
    "user_id", turn.UserID,
    "steps", stepsCompleted,
    "tools_called", len(toolsCalled),
    "duration_ms", time.Since(startTime).Milliseconds(),
    "outcome", outcome,
)
```

Add imports: `"log/slog"`, `"github.com/google/uuid"`, `"github.com/kabradshaw1/portfolio/go/ai-service/internal/guardrails"`. Fetch `uuid` if not already present: `go get github.com/google/uuid`.

Track `toolsCalled []string` inside `Run` (append the tool name on each dispatch) and emit it on the final log line.

### 4f. Chat handler integration

Change `RegisterChatRoutes` to accept a `*guardrails.Limiter`:

```go
func RegisterChatRoutes(r *gin.Engine, runner Runner, jwtSecret string, limiter *guardrails.Limiter)
```

Apply `guardrails.Middleware(limiter)` to the `/chat` route group (or directly on the POST route). All existing tests pass `nil` as the limiter argument.

- [ ] **Run full suite, commit**

```bash
cd go/ai-service && go test ./... -count=1
git add go/ai-service/
git commit -m "feat(ai-service): add history cap, rate limit, refusal detection, turn logging"
```

---

## Task 5: Mocked-LLM eval harness

**Files:**
- Create: `go/ai-service/internal/evals/doc.go`
- Create: `go/ai-service/internal/evals/harness.go`
- Create: `go/ai-service/internal/evals/cases_test.go`

The harness is a thin test helper: you construct an `agent.Agent` with a scripted `llm.Client` and an in-memory tool registry, feed it a prompt, and assert which tools were called and what the final text was. Build-tagged with `//go:build eval` so `go test ./...` skips it by default but a one-line preflight target runs it.

### 5a. doc.go

```go
//go:build eval

// Package evals runs scripted agent-loop behavior tests without real LLMs.
// Enable with: go test -tags=eval ./internal/evals/...
package evals
```

### 5b. harness.go

```go
//go:build eval

package evals

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/agent"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/llm"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools"
)

// ScriptedLLM returns canned ChatResponses in order. Used to simulate tool-call
// sequences without touching a real model.
type ScriptedLLM struct {
	Responses []llm.ChatResponse
	calls     int
}

func (s *ScriptedLLM) Chat(ctx context.Context, _ []llm.Message, _ []llm.ToolSchema) (llm.ChatResponse, error) {
	if s.calls >= len(s.Responses) {
		return llm.ChatResponse{}, errors.New("scripted LLM: unexpected extra call")
	}
	r := s.Responses[s.calls]
	s.calls++
	return r, nil
}

// EchoTool is a tool that records what it was called with and returns a canned result.
type EchoTool struct {
	ToolName string
	Calls    int
	SeenArgs []json.RawMessage
	Result   tools.Result
	Err      error
}

func (e *EchoTool) Name() string            { return e.ToolName }
func (e *EchoTool) Description() string     { return "echo " + e.ToolName }
func (e *EchoTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (e *EchoTool) Call(ctx context.Context, args json.RawMessage, userID string) (tools.Result, error) {
	e.Calls++
	e.SeenArgs = append(e.SeenArgs, args)
	return e.Result, e.Err
}

// Run runs the agent with the given LLM script and registry and returns the
// ordered slice of events emitted.
func Run(scripted *ScriptedLLM, reg tools.Registry, turn agent.Turn, maxSteps int) ([]agent.Event, error) {
	a := agent.New(scripted, reg, metrics.NopRecorder{}, maxSteps, 0)
	var events []agent.Event
	err := a.Run(context.Background(), turn, func(e agent.Event) { events = append(events, e) })
	return events, err
}
```

**Note on timeout:** pass `0` for the timeout and either (a) special-case a zero timeout in `agent.Run` to mean "no deadline" or (b) pass a long timeout like `time.Minute`. Option (a) is cleaner but requires a one-line change to `agent.Run`. **Choose (b) in this task** to avoid touching the agent loop; update the harness to pass `time.Minute`.

### 5c. `cases_test.go` — three starter eval cases

```go
//go:build eval

package evals

import (
	"encoding/json"
	"testing"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/agent"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/llm"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools"
)

func TestEval_CallsSearchProductsWithPriceFilter(t *testing.T) {
	searchTool := &EchoTool{ToolName: "search_products", Result: tools.Result{Content: []map[string]any{{"id": "p1"}}}}
	reg := tools.NewMemRegistry()
	reg.Register(searchTool)

	scripted := &ScriptedLLM{Responses: []llm.ChatResponse{
		{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "search_products", Args: json.RawMessage(`{"query":"jacket","max_price":150}`)}}},
		{Content: "Here are some jackets under $150."},
	}}

	events, err := Run(scripted, reg,
		agent.Turn{UserID: "user-1", Messages: []llm.Message{{Role: llm.RoleUser, Content: "find me a jacket under 150"}}},
		8)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if searchTool.Calls != 1 {
		t.Errorf("search_products calls = %d", searchTool.Calls)
	}
	if len(events) == 0 || events[len(events)-1].Final == nil {
		t.Errorf("expected final event, got %+v", events)
	}
}

func TestEval_RecoversFromToolError(t *testing.T) {
	badTool := &EchoTool{ToolName: "get_order", Err: errors.New("upstream 500")}
	reg := tools.NewMemRegistry()
	reg.Register(badTool)

	scripted := &ScriptedLLM{Responses: []llm.ChatResponse{
		{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "get_order", Args: json.RawMessage(`{"order_id":"x"}`)}}},
		{Content: "Sorry, I couldn't fetch that order right now."},
	}}

	events, err := Run(scripted, reg,
		agent.Turn{UserID: "user-1", Messages: []llm.Message{{Role: llm.RoleUser, Content: "where's order x"}}},
		8)
	if err != nil {
		t.Fatalf("run should not bubble tool error: %v", err)
	}
	if events[len(events)-1].Final == nil {
		t.Errorf("expected recovered final: %+v", events)
	}
}

func TestEval_HonorsMaxSteps(t *testing.T) {
	looper := &EchoTool{ToolName: "echo"}
	reg := tools.NewMemRegistry()
	reg.Register(looper)

	call := llm.ToolCall{ID: "c", Name: "echo", Args: json.RawMessage(`{}`)}
	scripted := &ScriptedLLM{Responses: []llm.ChatResponse{
		{ToolCalls: []llm.ToolCall{call}},
		{ToolCalls: []llm.ToolCall{call}},
		{ToolCalls: []llm.ToolCall{call}},
	}}

	_, err := Run(scripted, reg,
		agent.Turn{UserID: "u", Messages: []llm.Message{{Role: llm.RoleUser, Content: "spin"}}},
		3)
	if err == nil {
		t.Fatal("expected ErrMaxSteps")
	}
}
```

Add `"errors"` to the imports and `"time"` if needed.

- [ ] **Run and commit**

```bash
cd go/ai-service && go test -tags=eval ./internal/evals/... -v
git add go/ai-service/internal/evals/
git commit -m "feat(ai-service): add mocked-LLM eval harness with starter cases"
```

Add a Makefile target at the root:

```makefile
preflight-ai-service-evals:
	cd go/ai-service && go test -tags=eval ./internal/evals/... -count=1
```

Do NOT add this to `preflight-ai-service` or `preflight-go` — the eval tag must be explicit. Commit the Makefile change with the above.

---

## Task 6: Wire everything in main.go + update Grafana dashboard + compose

**Files:**
- Modify: `go/ai-service/cmd/server/main.go`
- Modify: `go/docker-compose.yml`
- Modify: `monitoring/grafana/dashboards/system-overview.json`

### 6a. main.go

Add:

```go
redisURL := os.Getenv("REDIS_URL") // optional

// Cache
var toolCache cache.Cache = cache.NopCache{}
var limiter *guardrails.Limiter
if redisURL != "" {
    opts, err := redis.ParseURL(redisURL)
    if err != nil {
        log.Fatalf("bad REDIS_URL: %v", err)
    }
    rc := redis.NewClient(opts)
    if err := rc.Ping(context.Background()).Err(); err != nil {
        slog.Warn("redis unreachable, caching disabled", "error", err)
    } else {
        toolCache = cache.NewRedisCache(rc, "ai")
        limiter = guardrails.NewLimiter(rc, 20, time.Minute)
        slog.Info("redis connected, caching + rate limit enabled")
    }
}

// Registry — catalog tools get cached with 60s TTL. User-scoped tools:
// orders get 10s, cart/returns never cache.
registry := tools.NewMemRegistry()
registry.Register(tools.Cached(tools.NewSearchProductsTool(ecomClient), toolCache, 60*time.Second))
registry.Register(tools.Cached(tools.NewGetProductTool(ecomClient), toolCache, 60*time.Second))
registry.Register(tools.Cached(tools.NewCheckInventoryTool(ecomClient), toolCache, 10*time.Second))
registry.Register(tools.Cached(tools.NewListOrdersTool(ecomClient), toolCache, 10*time.Second))
registry.Register(tools.Cached(tools.NewGetOrderTool(ecomClient), toolCache, 10*time.Second))
registry.Register(tools.NewSummarizeOrdersTool(ecomClient, llmc))
registry.Register(tools.NewViewCartTool(ecomClient))
registry.Register(tools.NewAddToCartTool(ecomClient))
registry.Register(tools.NewInitiateReturnTool(ecomClient))

a := agent.New(llmc, registry, metrics.PromRecorder{}, 8, 30*time.Second)

// HTTP
apphttp.RegisterHealthRoutes(router, ...)
apphttp.RegisterMetricsRoute(router)
apphttp.RegisterChatRoutes(router, a, jwtSecret, limiter)
```

Add imports for `cache`, `metrics`, `guardrails`, `redis`.

`summarize_orders` stays uncached because each of its results bakes in LLM-generated text — caching a stale summary is worse than regenerating.

### 6b. docker-compose.yml

Add to `ai-service`'s `environment:`:

```yaml
      REDIS_URL: redis://redis:6379
```

The Redis service is already defined elsewhere in `go/docker-compose.yml`.

### 6c. Grafana dashboard row

Open `monitoring/grafana/dashboards/system-overview.json`. Find the last panel object in the `"panels"` array (before the closing `]`). Append a new row + five panels after it. Use the existing row title structure (`"type": "row"`, `"title": "AI Service"`).

Five panels:

1. **Turn rate** — `rate(ai_agent_turns_total[5m])` grouped by `outcome`, stat panel or time series.
2. **Turn p95 latency** — `histogram_quantile(0.95, rate(ai_agent_turn_duration_seconds_bucket[5m]))`.
3. **Tool call rate by name** — `sum by (name) (rate(ai_tool_calls_total[5m]))`.
4. **Tool error ratio** — `sum(rate(ai_tool_calls_total{outcome="error"}[5m])) / sum(rate(ai_tool_calls_total[5m]))`.
5. **Cache hit rate** — `sum(rate(ai_cache_events_total{event="hit"}[5m])) / sum(rate(ai_cache_events_total[5m]))`.

Give each panel a sane `gridPos` (y starting after the last existing panel's y+h, w=12 each, h=8). Copy the JSON shape from an existing time-series panel in the same file rather than hand-crafting it.

**Note:** cache event metrics are NOT yet emitted in Plan 3 — Task 2's `Cached` wrapper doesn't call `metrics.CacheEvents.WithLabelValues(...).Inc()`. Add those two calls (hit / miss) to `cached.go` as part of Task 6: one line each where the cache is hit or missed. The panel query in (5) is only meaningful once those counters move. Include that edit in this task's commit.

### 6d. Build + test + commit

```bash
cd go/ai-service && go build ./...
cd go/ai-service && go test ./... -count=1
git add go/ai-service/cmd/server/main.go \
        go/ai-service/internal/tools/cached.go \
        go/docker-compose.yml \
        monitoring/grafana/dashboards/system-overview.json
git commit -m "feat(ai-service): wire cache + metrics + rate limit + AI Service Grafana row"
```

---

## Done criteria for Plan 3

- `go test ./go/ai-service/...` passes fully offline with all new packages included.
- `go test -tags=eval ./go/ai-service/internal/evals/...` passes the three starter cases.
- Agent loop emits `ai_agent_turns_total`, `ai_agent_steps_per_turn`, `ai_agent_turn_duration_seconds`, `ai_tool_calls_total`, `ai_tool_duration_seconds`, and `ai_cache_events_total` metrics.
- `GET /metrics` returns Prometheus text format.
- `POST /chat` is rate-limited per IP when Redis is available, and fails open when Redis is down.
- History is truncated to 20 messages at the top of each turn; refusals are tagged as `outcome="refused"`.
- Grafana `system-overview` dashboard has an "AI Service" row visible to anyone who opens the dashboard.
- No frontend, no real-LLM eval tier, no CI wiring for nightly evals — those stay in Plans 4 and 5.
