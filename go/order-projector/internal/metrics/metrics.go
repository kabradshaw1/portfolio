package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// EventsConsumed counts order events consumed and projected, labeled by event type.
	EventsConsumed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "projector_events_consumed_total",
		Help: "Total order events consumed and projected",
	}, []string{"event_type"})

	// ProjectionLag tracks seconds between the latest event timestamp and now.
	ProjectionLag = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "projector_projection_lag_seconds",
		Help: "Seconds between latest event timestamp and now",
	})

	// ReplayInProgress is 1 if a replay is running, 0 otherwise.
	ReplayInProgress = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "projector_replay_in_progress",
		Help: "1 if replay is in progress, 0 otherwise",
	})

	// ConsumerErrors counts total Kafka consumer errors (fetch, deserialization).
	ConsumerErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "projector_consumer_errors_total",
		Help: "Total consumer errors",
	})

	// ProjectionErrors counts errors applying projections, labeled by projection and event type.
	ProjectionErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "projector_projection_errors_total",
		Help: "Errors applying projections",
	}, []string{"projection", "event_type"})
)
