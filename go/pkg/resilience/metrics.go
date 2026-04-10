package resilience

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sony/gobreaker/v2"
)

// BreakerState is a Prometheus gauge that tracks circuit breaker state.
// Values: 0 = closed, 1 = half-open, 2 = open.
var BreakerState = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "circuit_breaker_state",
	Help: "Circuit breaker state: 0=closed, 1=half-open, 2=open",
}, []string{"name"})

// ObserveStateChange updates the Prometheus gauge for the given breaker.
func ObserveStateChange(name string, _ gobreaker.State, to gobreaker.State) {
	var val float64
	switch to {
	case gobreaker.StateClosed:
		val = 0
	case gobreaker.StateHalfOpen:
		val = 1
	case gobreaker.StateOpen:
		val = 2
	}
	BreakerState.WithLabelValues(name).Set(val)
}
