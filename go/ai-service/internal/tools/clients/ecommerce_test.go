package clients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEcommerceClient_GetProduct(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/products/abc-123" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"abc-123","name":"Waterproof Jacket","price":12999,"stock":4}`))
	}))
	defer server.Close()

	c := NewEcommerceClient(server.URL)
	p, err := c.GetProduct(context.Background(), "abc-123")
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if p.ID != "abc-123" || p.Name != "Waterproof Jacket" || p.Price != 12999 || p.Stock != 4 {
		t.Errorf("unexpected product: %+v", p)
	}
}

func TestEcommerceClient_GetProduct_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	_, err := NewEcommerceClient(server.URL).GetProduct(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error on 404")
	}
}

func TestEcommerceClient_ListProducts_TextSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "jacket" {
			t.Fatalf("expected q=jacket, got %q", r.URL.Query().Get("q"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"products":[
				{"id":"p1","name":"Waterproof Jacket","price":12999,"stock":4},
				{"id":"p2","name":"Rain Jacket","price":8900,"stock":10}
			],
			"total":2,
			"page":1,
			"limit":10
		}`))
	}))
	defer server.Close()

	c := NewEcommerceClient(server.URL)
	ps, err := c.ListProducts(context.Background(), "jacket", 10)
	if err != nil {
		t.Fatalf("ListProducts: %v", err)
	}
	if len(ps) != 2 {
		t.Fatalf("expected 2 results, got %d", len(ps))
	}
	if ps[0].Name != "Waterproof Jacket" {
		t.Errorf("first product wrong: %+v", ps[0])
	}
}

func TestEcommerceClient_ListOrders_ForwardsJWT(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/orders" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("auth header = %q", got)
		}
		_, _ = w.Write([]byte(`{"orders":[
			{"id":"00000000-0000-0000-0000-000000000001","status":"paid","total":12999,"createdAt":"2026-04-01T00:00:00Z"},
			{"id":"00000000-0000-0000-0000-000000000002","status":"pending","total":8900,"createdAt":"2026-04-02T00:00:00Z"}
		]}`))
	}))
	defer server.Close()

	c := NewEcommerceClient(server.URL)
	orders, err := c.ListOrders(context.Background(), "test-token")
	if err != nil {
		t.Fatalf("ListOrders: %v", err)
	}
	if len(orders) != 2 || orders[0].Status != "paid" || orders[0].Total != 12999 {
		t.Errorf("orders = %+v", orders)
	}
}

func TestEcommerceClient_GetOrder_404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	_, err := NewEcommerceClient(server.URL).GetOrder(context.Background(), "t", "id-x")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEcommerceClient_GetCart(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cart" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer t" {
			t.Fatal("missing auth")
		}
		_, _ = w.Write([]byte(`{
			"items":[
				{"id":"i1","productId":"p1","productName":"Jacket","productPrice":12999,"quantity":1}
			],
			"total":12999
		}`))
	}))
	defer server.Close()

	cart, err := NewEcommerceClient(server.URL).GetCart(context.Background(), "t")
	if err != nil {
		t.Fatalf("GetCart: %v", err)
	}
	if cart.Total != 12999 || len(cart.Items) != 1 || cart.Items[0].ProductName != "Jacket" {
		t.Errorf("cart = %+v", cart)
	}
}

func TestEcommerceClient_AddToCart_BodyShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/cart" {
			t.Fatalf("method/path = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer t" {
			t.Fatal("missing auth")
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["productId"] != "p1" || body["quantity"].(float64) != 2 {
			t.Errorf("body = %+v", body)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"i1","productId":"p1","productName":"Jacket","productPrice":12999,"quantity":2}`))
	}))
	defer server.Close()

	item, err := NewEcommerceClient(server.URL).AddToCart(context.Background(), "t", "p1", 2)
	if err != nil {
		t.Fatalf("AddToCart: %v", err)
	}
	if item.Quantity != 2 || item.ProductName != "Jacket" {
		t.Errorf("item = %+v", item)
	}
}

func TestEcommerceClient_InitiateReturn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.URL.Path != "/orders/order-1/returns" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer t" {
			t.Fatal("missing auth")
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["reason"] != "doesn't fit" {
			t.Errorf("body = %+v", body)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"r1","orderId":"order-1","status":"requested","reason":"doesn't fit"}`))
	}))
	defer server.Close()

	ret, err := NewEcommerceClient(server.URL).InitiateReturn(context.Background(), "t", "order-1", []string{"i1"}, "doesn't fit")
	if err != nil {
		t.Fatalf("InitiateReturn: %v", err)
	}
	if ret.Status != "requested" || ret.ID != "r1" {
		t.Errorf("ret = %+v", ret)
	}
}
