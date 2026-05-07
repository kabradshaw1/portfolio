package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// EventsConsumed counts Kafka events processed by topic.
	EventsConsumed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "analytics_events_consumed_total",
		Help: "Total Kafka events consumed by topic.",
	}, []string{"topic"})

	// AggregationLatency tracks time to update aggregators.
	AggregationLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "analytics_aggregation_latency_seconds",
		Help:    "Histogram of aggregation update latency.",
		Buckets: prometheus.DefBuckets,
	}, []string{"aggregator"})

	// ConsumerLag tracks how far behind the consumer is.
	ConsumerLag = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "kafka_consumer_lag",
		Help: "Number of messages the consumer is behind.",
	})

	// ConsumerErrors counts Kafka consumer read errors.
	ConsumerErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "kafka_consumer_errors_total",
		Help: "Total Kafka consumer read errors.",
	})

	// WindowFlushesTotal counts window flush operations per aggregator.
	WindowFlushesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "analytics_window_flushes_total",
		Help: "Total window flushes to storage.",
	}, []string{"aggregator"})

	// WindowFlushLatency tracks flush duration per aggregator.
	WindowFlushLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "analytics_window_flush_latency_seconds",
		Help:    "Histogram of window flush latency.",
		Buckets: prometheus.DefBuckets,
	}, []string{"aggregator"})

	// LateEventsDropped counts events that arrived past the grace period.
	LateEventsDropped = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "analytics_late_events_dropped_total",
		Help: "Total events dropped due to late arrival past grace period.",
	}, []string{"aggregator"})

	// ActiveWindows tracks currently open windows per aggregator.
	ActiveWindows = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "analytics_active_windows",
		Help: "Number of currently open windows.",
	}, []string{"aggregator"})

	CommitErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "analytics_kafka_commit_errors_total",
		Help: "Kafka commit errors by group and topic.",
	}, []string{"group", "topic"})

	InvalidEvents = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "analytics_invalid_events_total",
		Help: "Invalid Kafka events by source topic.",
	}, []string{"topic"})

	DLQPublished = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "analytics_kafka_dlq_published_total",
		Help: "Kafka records published to DLQ by group and source topic.",
	}, []string{"group", "topic"})

	DLQPublishErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "analytics_kafka_dlq_publish_errors_total",
		Help: "Kafka DLQ publish errors by group and source topic.",
	}, []string{"group", "topic"})

	LastSuccessfulFlushTimestamp = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "analytics_last_successful_flush_timestamp",
		Help: "Unix timestamp of the last successful window flush by aggregator.",
	}, []string{"aggregator"})
)
