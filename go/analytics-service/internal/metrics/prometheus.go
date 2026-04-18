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
)
