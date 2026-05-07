package kafkametrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	producerMessages = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kafka_producer_messages_total",
		Help: "Kafka producer publish attempts by service, topic, and outcome.",
	}, []string{"service", "topic", "outcome"})

	producerWriteDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "kafka_producer_write_duration_seconds",
		Help:    "Kafka producer write duration by service and topic.",
		Buckets: prometheus.DefBuckets,
	}, []string{"service", "topic"})

	producerAsyncMode = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kafka_producer_async_mode",
		Help: "Whether the service Kafka producer is configured in async best-effort mode.",
	}, []string{"service"})
)

// Recorder captures Kafka producer outcome metrics.
type Recorder interface {
	ObserveMessage(service, topic, outcome string)
	ObserveWriteDuration(service, topic string, duration time.Duration)
	SetAsyncMode(service string, enabled bool)
}

// PrometheusRecorder records producer metrics to package-level collectors.
type PrometheusRecorder struct{}

func (PrometheusRecorder) ObserveMessage(service, topic, outcome string) {
	producerMessages.WithLabelValues(service, topic, outcome).Inc()
}

func (PrometheusRecorder) ObserveWriteDuration(service, topic string, duration time.Duration) {
	producerWriteDuration.WithLabelValues(service, topic).Observe(duration.Seconds())
}

func (PrometheusRecorder) SetAsyncMode(service string, enabled bool) {
	if enabled {
		producerAsyncMode.WithLabelValues(service).Set(1)
		return
	}
	producerAsyncMode.WithLabelValues(service).Set(0)
}
