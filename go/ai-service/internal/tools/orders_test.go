package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/jwtctx"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools/clients"
)

type fakeOrdersAPI struct {
	listOut []clients.Order
	listErr error
	getOut  clients.Order
	getErr  error
	seenJWT string
	seenID  string
}

func (f *fakeOrdersAPI) ListOrders(ctx context.Context, jwt string) ([]clients.Order, error) {
	f.seenJWT = jwt
	return f.listOut, f.listErr
}

func (f *fakeOrdersAPI) GetOrder(ctx context.Context, jwt, id string) (clients.Order, error) {
	f.seenJWT = jwt
	f.seenID = id
	return f.getOut, f.getErr
}

func ctxWithJWT(jwt string) context.Context {
	return jwtctx.WithJWT(context.Background(), jwt)
}

func TestListOrdersTool_BoundedAndForwardsJWT(t *testing.T) {
	fake := &fakeOrdersAPI{listOut: make([]clients.Order, 50)}
	for i := range fake.listOut {
		fake.listOut[i] = clients.Order{
			ID:        "o" + string(rune('a'+i%26)),
			Status:    "paid",
			Total:     100 + i,
			CreatedAt: time.Now(),
		}
	}
	tool := NewListOrdersTool(fake)

	res, err := tool.Call(ctxWithJWT("tok"), json.RawMessage(`{}`), "user-1")
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if fake.seenJWT != "tok" {
		t.Errorf("jwt forwarded = %q", fake.seenJWT)
	}
	items := res.Content.([]map[string]any)
	if len(items) > 20 {
		t.Errorf("expected bound of 20, got %d", len(items))
	}
}

func TestListOrdersTool_RequiresUserID(t *testing.T) {
	tool := NewListOrdersTool(&fakeOrdersAPI{})
	_, err := tool.Call(ctxWithJWT("tok"), json.RawMessage(`{}`), "")
	if err == nil {
		t.Fatal("expected error when userID empty")
	}
}

func TestGetOrderTool_Success(t *testing.T) {
	fake := &fakeOrdersAPI{getOut: clients.Order{ID: "order-1", Status: "paid", Total: 12999}}
	tool := NewGetOrderTool(fake)

	res, err := tool.Call(ctxWithJWT("tok"), json.RawMessage(`{"order_id":"order-1"}`), "user-1")
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if fake.seenID != "order-1" || fake.seenJWT != "tok" {
		t.Errorf("seen id=%q jwt=%q", fake.seenID, fake.seenJWT)
	}
	m := res.Content.(map[string]any)
	if m["id"] != "order-1" || m["status"] != "paid" {
		t.Errorf("content = %+v", m)
	}
}

func TestGetOrderTool_MissingID(t *testing.T) {
	tool := NewGetOrderTool(&fakeOrdersAPI{})
	_, err := tool.Call(ctxWithJWT("tok"), json.RawMessage(`{}`), "user-1")
	if err == nil {
		t.Fatal("expected error for missing order_id")
	}
}

func TestGetOrderTool_RequiresUserID(t *testing.T) {
	tool := NewGetOrderTool(&fakeOrdersAPI{})
	_, err := tool.Call(ctxWithJWT("tok"), json.RawMessage(`{"order_id":"x"}`), "")
	if err == nil {
		t.Fatal("expected error for empty userID")
	}
}
