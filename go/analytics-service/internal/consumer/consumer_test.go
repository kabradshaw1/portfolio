package consumer

import (
	"encoding/json"
	"testing"

	"github.com/segmentio/kafka-go"

	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/aggregator"
)

func TestRoute_OrderCreated(t *testing.T) {
	orders := aggregator.NewOrderAggregator()
	trending := aggregator.NewTrendingAggregator()
	carts := aggregator.NewCartAggregator()

	c := &Consumer{orders: orders, trending: trending, carts: carts}

	body, _ := json.Marshal(event{
		Type: "order.created",
		Data: json.RawMessage(`{"orderID":"abc","userID":"u1","totalCents":5000}`),
	})

	c.route(kafka.Message{Topic: TopicOrders, Value: body})

	stats := orders.Stats()
	if stats.StatusBreakdown.Created != 1 {
		t.Errorf("expected 1 created, got %d", stats.StatusBreakdown.Created)
	}
}

func TestRoute_CartItemAdded(t *testing.T) {
	orders := aggregator.NewOrderAggregator()
	trending := aggregator.NewTrendingAggregator()
	carts := aggregator.NewCartAggregator()

	c := &Consumer{orders: orders, trending: trending, carts: carts}

	body, _ := json.Marshal(event{
		Type: "cart.item_added",
		Data: json.RawMessage(`{"productID":"p1"}`),
	})

	c.route(kafka.Message{Topic: TopicCart, Value: body})

	cartStats := carts.Stats()
	if cartStats.ActiveCarts != 1 {
		t.Errorf("expected 1 active cart, got %d", cartStats.ActiveCarts)
	}
}

func TestRoute_ProductViewed(t *testing.T) {
	orders := aggregator.NewOrderAggregator()
	trending := aggregator.NewTrendingAggregator()
	carts := aggregator.NewCartAggregator()

	c := &Consumer{orders: orders, trending: trending, carts: carts}

	body, _ := json.Marshal(event{
		Type: "product.viewed",
		Data: json.RawMessage(`{"productID":"p1","productName":"Widget"}`),
	})

	c.route(kafka.Message{Topic: TopicViews, Value: body})

	top := trending.TopProducts()
	if len(top) != 1 {
		t.Fatalf("expected 1 trending product, got %d", len(top))
	}
	if top[0].Name != "Widget" {
		t.Errorf("expected name Widget, got %s", top[0].Name)
	}
}

func TestRoute_InvalidJSON(t *testing.T) {
	orders := aggregator.NewOrderAggregator()
	trending := aggregator.NewTrendingAggregator()
	carts := aggregator.NewCartAggregator()

	c := &Consumer{orders: orders, trending: trending, carts: carts}

	// Should not panic on invalid JSON.
	c.route(kafka.Message{Topic: TopicOrders, Value: []byte("not json")})
}
