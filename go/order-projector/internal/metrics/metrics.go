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

	FetchErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "projector_consumer_fetch_errors_total",
		Help: "Kafka fetch errors by group and topic",
	}, []string{"group", "topic"})

	CommitErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "projector_consumer_commit_errors_total",
		Help: "Kafka commit errors by group and topic",
	}, []string{"group", "topic"})

	DLQPublished = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "projector_consumer_dlq_published_total",
		Help: "Kafka records published to DLQ by group and source topic",
	}, []string{"group", "topic"})

	DLQPublishErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "projector_consumer_dlq_publish_errors_total",
		Help: "Kafka DLQ publish errors by group and source topic",
	}, []string{"group", "topic"})

	DuplicateEvents = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "projector_duplicate_events_total",
		Help: "Duplicate projector events skipped by projection",
	}, []string{"projection"})

	ProjectionDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "projector_projection_duration_seconds",
		Help:    "Projection processing duration by projection and event type",
		Buckets: prometheus.DefBuckets,
	}, []string{"projection", "event_type"})
)
