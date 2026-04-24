package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/cache"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/metrics"
)

// Cached returns a Tool that wraps inner with a response cache.
// Cache key is sha256(toolName + canonical(args) + userID).
// Only Result.Content is cached — Display is regenerated on every hit (nil on hits).
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
				metrics.CacheEvents.WithLabelValues("tool", "hit").Inc()
				slog.DebugContext(ctx, "tool cache hit",
					"tool", t.inner.Name(),
					"key_prefix", key[:min(len(key), 16)],
				)
				return Result{Content: content}, nil
			}
		}
	}

	metrics.CacheEvents.WithLabelValues("tool", "miss").Inc()
	res, callErr := t.inner.Call(ctx, args, userID)
	if callErr != nil {
		return res, callErr
	}
	if key != "" && res.Content != nil {
		if raw, merr := json.Marshal(res.Content); merr == nil {
			_ = t.cache.Set(ctx, key, raw, t.ttl)
		}
	}
	return res, nil
}

// cacheKey canonicalizes args by round-tripping through Go's json package,
// which sorts map[string]any keys alphabetically when encoding.
func cacheKey(name string, args json.RawMessage, userID string) (string, error) {
	var v any
	if err := json.Unmarshal(args, &v); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s", name, canonical, userID)
	return hex.EncodeToString(h.Sum(nil)), nil
}
