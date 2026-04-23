package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	WebhookEvents = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "payment_webhook_events_total",
		Help: "Stripe webhook events received.",
	}, []string{"event_type", "outcome"}) // outcome: processed, duplicate, error

	PaymentsCreated = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "payment_created_total",
		Help: "Payments created via gRPC.",
	}, []string{"status"}) // status: succeeded, failed

	OutboxPublish = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "payment_outbox_publish_total",
		Help: "Outbox message publish attempts.",
	}, []string{"outcome"}) // outcome: success, error

	OutboxLag = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "payment_outbox_lag_seconds",
		Help:    "Time from outbox insert to publish.",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30},
	})
)
