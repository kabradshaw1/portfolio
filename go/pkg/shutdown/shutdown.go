package shutdown

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"
)

type hook struct {
	name     string
	priority int
	fn       func(ctx context.Context) error
}

// Manager orchestrates graceful shutdown in priority order.
type Manager struct {
	timeout time.Duration
	hooks   []hook
}

// New creates a Manager with the given overall shutdown timeout.
func New(timeout time.Duration) *Manager {
	return &Manager{timeout: timeout}
}

// Register adds a shutdown function. Lower priority values run first.
// Functions at the same priority run concurrently.
func (m *Manager) Register(name string, priority int, fn func(ctx context.Context) error) {
	m.hooks = append(m.hooks, hook{name: name, priority: priority, fn: fn})
}

// Wait blocks until SIGINT or SIGTERM is received, then runs all
// registered hooks in priority order within the timeout.
func (m *Manager) Wait() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutdown signal received")
	m.runAll()
	slog.Info("shutdown complete")
}

func (m *Manager) runAll() {
	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()

	sort.Slice(m.hooks, func(i, j int) bool {
		return m.hooks[i].priority < m.hooks[j].priority
	})

	groups := m.groupByPriority()
	for _, group := range groups {
		if ctx.Err() != nil {
			slog.Warn("shutdown timeout reached, skipping remaining hooks")
			return
		}
		m.runGroup(ctx, group)
	}
}

func (m *Manager) groupByPriority() [][]hook {
	if len(m.hooks) == 0 {
		return nil
	}
	var groups [][]hook
	currentPriority := m.hooks[0].priority
	var current []hook
	for _, h := range m.hooks {
		if h.priority != currentPriority {
			groups = append(groups, current)
			current = nil
			currentPriority = h.priority
		}
		current = append(current, h)
	}
	groups = append(groups, current)
	return groups
}

func (m *Manager) runGroup(ctx context.Context, group []hook) {
	var wg sync.WaitGroup
	for _, h := range group {
		wg.Add(1)
		go func(h hook) {
			defer wg.Done()
			if err := h.fn(ctx); err != nil {
				slog.Error("shutdown hook failed", "name", h.name, "error", err)
			} else {
				slog.Info("shutdown hook completed", "name", h.name)
			}
		}(h)
	}
	wg.Wait()
}
