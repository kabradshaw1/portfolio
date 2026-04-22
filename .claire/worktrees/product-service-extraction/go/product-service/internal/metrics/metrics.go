package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ProductViews = promauto.NewCounter(prometheus.CounterOpts{
		Name: "product_views_total",
		Help: "Total individual product page views.",
	})

	CacheOps = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "product_cache_operations_total",
		Help: "Redis cache operations.",
	}, []string{"operation", "result"})
)
