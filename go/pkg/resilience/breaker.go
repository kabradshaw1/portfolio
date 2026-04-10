package resilience

import (
	"log/slog"
	"time"

	"github.com/sony/gobreaker/v2"
)

// BreakerConfig configures a circuit breaker.
type BreakerConfig struct {
	Name           string
	MaxFailures    uint32        // consecutive failures to trip (default 5)
	HalfOpenWindow time.Duration // time before half-open retry (default 10s)
	OnStateChange  func(name string, from, to gobreaker.State) // optional extra callback
}

// NewBreaker creates a gobreaker.CircuitBreaker with sensible defaults.
func NewBreaker(cfg BreakerConfig) *gobreaker.CircuitBreaker[any] {
	if cfg.MaxFailures == 0 {
		cfg.MaxFailures = 5
	}
	if cfg.HalfOpenWindow == 0 {
		cfg.HalfOpenWindow = 10 * time.Second
	}

	settings := gobreaker.Settings{
		Name:    cfg.Name,
		Timeout: cfg.HalfOpenWindow,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= cfg.MaxFailures
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			slog.Warn("circuit breaker state change",
				"breaker", name,
				"from", from.String(),
				"to", to.String(),
			)
			if cfg.OnStateChange != nil {
				cfg.OnStateChange(name, from, to)
			}
		},
	}

	return gobreaker.NewCircuitBreaker[any](settings)
}
