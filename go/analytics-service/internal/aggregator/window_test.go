package aggregator

import (
	"sync"
	"testing"
	"time"
)

type counter struct {
	N int
}

func newCounter() counter { return counter{} }

func TestWindow_BasicUpdate(t *testing.T) {
	w := NewWindow(time.Hour, newCounter)

	w.Update(func(c *counter) { c.N++ })
	w.Update(func(c *counter) { c.N++ })

	entries := w.Get()
	if len(entries) != 1 {
		t.Fatalf("expected 1 slot, got %d", len(entries))
	}
	if entries[0].Value.N != 2 {
		t.Errorf("expected N=2, got %d", entries[0].Value.N)
	}
}

func TestWindow_Eviction(t *testing.T) {
	w := NewWindow(2*time.Minute, newCounter)

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	// Add at 12:00
	w.now = func() time.Time { return base }
	w.Update(func(c *counter) { c.N = 10 })

	// Add at 12:01
	w.now = func() time.Time { return base.Add(1 * time.Minute) }
	w.Update(func(c *counter) { c.N = 20 })

	// Add at 12:03 — 12:00 slot should be evicted (>2m old)
	w.now = func() time.Time { return base.Add(3 * time.Minute) }
	w.Update(func(c *counter) { c.N = 30 })

	entries := w.Get()
	// 12:01 and 12:03 should remain
	if len(entries) != 2 {
		t.Fatalf("expected 2 slots after eviction, got %d", len(entries))
	}
}

func TestWindow_EmptyGet(t *testing.T) {
	w := NewWindow(time.Hour, newCounter)
	entries := w.Get()
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestWindow_ConcurrentAccess(t *testing.T) {
	w := NewWindow(time.Hour, newCounter)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.Update(func(c *counter) { c.N++ })
			_ = w.Get()
		}()
	}
	wg.Wait()

	entries := w.Get()
	if len(entries) != 1 {
		t.Fatalf("expected 1 slot, got %d", len(entries))
	}
	if entries[0].Value.N != 100 {
		t.Errorf("expected N=100, got %d", entries[0].Value.N)
	}
}
