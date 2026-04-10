package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	"github.com/redis/go-redis/v9"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/cache"
)

// scriptedTool is a minimal Tool implementation for testing.
type scriptedTool struct {
	name   string
	result Result
	err    error
	calls  int
}

func (s *scriptedTool) Name() string            { return s.name }
func (s *scriptedTool) Description() string     { return "" }
func (s *scriptedTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (s *scriptedTool) Call(ctx context.Context, args json.RawMessage, userID string) (Result, error) {
	s.calls++
	return s.result, s.err
}

func newCache(t *testing.T) cache.Cache {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	return cache.NewRedisCache(redis.NewClient(&redis.Options{Addr: mr.Addr()}), "ai-test", resilience.NewBreaker(resilience.BreakerConfig{Name: "test"}))
}

func TestCached_MissThenHit(t *testing.T) {
	inner := &scriptedTool{
		name:   "echo",
		result: Result{Content: map[string]any{"x": 1}},
	}
	wrapped := Cached(inner, newCache(t), time.Minute)

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

	_, err = wrapped.Call(context.Background(), json.RawMessage(`{"q":"a"}`), "user-1")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if inner.calls != 1 {
		t.Errorf("expected cache hit (inner calls still 1), got %d", inner.calls)
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
