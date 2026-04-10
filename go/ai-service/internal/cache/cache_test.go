package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
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
	c := NewRedisCache(client, "ai", resilience.NewBreaker(resilience.BreakerConfig{Name: "test"}))

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
	c := NewRedisCache(client, "ai", resilience.NewBreaker(resilience.BreakerConfig{Name: "test"}))
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
	c := NewRedisCache(client, "ai", resilience.NewBreaker(resilience.BreakerConfig{Name: "test"}))

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
