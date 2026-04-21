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

	SagaDLQTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "saga_dlq_messages_total",
		Help: "Messages sent to the saga dead letter queue.",
	})
)
