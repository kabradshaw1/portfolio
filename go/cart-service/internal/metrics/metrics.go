package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	CartItemsAdded = promauto.NewCounter(prometheus.CounterOpts{
		Name: "cart_items_added_total",
		Help: "Total number of items added to carts.",
	})

	CartItemsRemoved = promauto.NewCounter(prometheus.CounterOpts{
		Name: "cart_items_removed_total",
		Help: "Total number of items removed from carts.",
	})

	ProductValidation = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cart_product_validation_total",
		Help: "Product validation via gRPC.",
	}, []string{"result"})
)
