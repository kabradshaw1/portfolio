package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	CartItemsAdded = promauto.NewCounter(prometheus.CounterOpts{
		Name: "ecommerce_cart_items_added_total",
		Help: "Total number of items added to carts.",
	})

	OrdersPlaced = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ecommerce_orders_placed_total",
		Help: "Total orders placed.",
	}, []string{"status"})

	OrderValue = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "ecommerce_order_value_dollars",
		Help:    "Order values in dollars (cents / 100).",
		Buckets: []float64{5, 10, 25, 50, 100, 250, 500, 1000},
	})

	ProductViews = promauto.NewCounter(prometheus.CounterOpts{
		Name: "ecommerce_product_views_total",
		Help: "Total individual product page views.",
	})

	CacheOps = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ecommerce_cache_operations_total",
		Help: "Redis cache operations.",
	}, []string{"operation", "result"})

	RabbitMQPublish = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ecommerce_rabbitmq_publish_total",
		Help: "RabbitMQ publish attempts.",
	}, []string{"queue", "result"})

	IdempotencyOps = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ecommerce_idempotency_operations_total",
		Help: "Idempotency key operations.",
	}, []string{"result"}) // hit, miss, conflict, error
)
