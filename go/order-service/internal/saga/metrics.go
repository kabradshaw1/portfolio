package saga

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	SagaStepsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "saga_steps_total",
		Help: "Total saga step transitions.",
	}, []string{"step", "outcome"})

	SagaDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "saga_duration_seconds",
		Help:    "Total saga duration from CREATED to terminal state.",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
	})

	SagaStepDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "saga_step_duration_seconds",
		Help:    "Duration of each saga step handler.",
		Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5, 10, 30},
	}, []string{"step", "outcome"})

	SagaDLQTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "saga_dlq_messages_total",
		Help: "Messages sent to the saga dead letter queue.",
	})

	SagaDLQReplayed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "saga_dlq_replayed_total",
		Help: "Messages replayed from the saga dead letter queue.",
	}, []string{"routing_key", "outcome"})
)
