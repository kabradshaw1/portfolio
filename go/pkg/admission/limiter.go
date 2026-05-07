package admission

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

// Observer receives admission lifecycle signals. Implementations should avoid
// high-cardinality labels because admission metrics are usually dashboard inputs.
type Observer interface {
	ObserveInFlight(n int)
	ObserveAdmitted()
	ObserveRejected()
	ObserveReleased()
}

// Option customizes a Limiter.
type Option func(*Limiter)

// WithObserver installs an observer for admission metrics.
func WithObserver(observer Observer) Option {
	return func(l *Limiter) {
		l.observer = observer
	}
}

// Limiter bounds concurrent in-flight work.
type Limiter struct {
	sem      chan struct{}
	observer Observer
	inFlight atomic.Int64
}

// NewLimiter creates a limiter with max concurrent permits.
func NewLimiter(max int, opts ...Option) (*Limiter, error) {
	if max <= 0 {
		return nil, fmt.Errorf("admission limiter max must be positive, got %d", max)
	}
	l := &Limiter{sem: make(chan struct{}, max)}
	for _, opt := range opts {
		opt(l)
	}
	l.observeInFlight()
	return l, nil
}

// TryAcquire admits work without waiting. The returned permit must be released.
func (l *Limiter) TryAcquire(ctx context.Context) (*Permit, bool) {
	if l == nil {
		return &Permit{}, true
	}
	if err := ctx.Err(); err != nil {
		l.observeRejected()
		return nil, false
	}
	select {
	case l.sem <- struct{}{}:
		return l.admit(), true
	default:
		l.observeRejected()
		return nil, false
	}
}

// Acquire waits until a permit is available or the context is cancelled.
func (l *Limiter) Acquire(ctx context.Context) (*Permit, error) {
	if l == nil {
		return &Permit{}, nil
	}
	if err := ctx.Err(); err != nil {
		l.observeRejected()
		return nil, err
	}
	select {
	case <-ctx.Done():
		l.observeRejected()
		return nil, ctx.Err()
	case l.sem <- struct{}{}:
		return l.admit(), nil
	}
}

// InFlight returns the current admitted work count.
func (l *Limiter) InFlight() int {
	if l == nil {
		return 0
	}
	return int(l.inFlight.Load())
}

func (l *Limiter) admit() *Permit {
	l.inFlight.Add(1)
	l.observeAdmitted()
	l.observeInFlight()
	return &Permit{limiter: l}
}

func (l *Limiter) release() {
	select {
	case <-l.sem:
	default:
		return
	}
	l.inFlight.Add(-1)
	l.observeReleased()
	l.observeInFlight()
}

func (l *Limiter) observeInFlight() {
	if l.observer != nil {
		l.observer.ObserveInFlight(l.InFlight())
	}
}

func (l *Limiter) observeAdmitted() {
	if l.observer != nil {
		l.observer.ObserveAdmitted()
	}
}

func (l *Limiter) observeRejected() {
	if l.observer != nil {
		l.observer.ObserveRejected()
		l.observeInFlight()
	}
}

func (l *Limiter) observeReleased() {
	if l.observer != nil {
		l.observer.ObserveReleased()
	}
}

// Permit releases one admitted unit of work. Release is idempotent so callers
// can safely defer it across complex control flow.
type Permit struct {
	limiter *Limiter
	once    sync.Once
}

// Release returns the permit to its limiter.
func (p *Permit) Release() {
	if p == nil {
		return
	}
	p.once.Do(func() {
		if p.limiter != nil {
			p.limiter.release()
		}
	})
}
