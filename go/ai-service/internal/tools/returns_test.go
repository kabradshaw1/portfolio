package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools/clients"
)

type fakeReturnsAPI struct {
	ret     clients.Return
	err     error
	seenJWT string
	seenOrd string
	seenIDs []string
	seenRsn string
}

func (f *fakeReturnsAPI) InitiateReturn(ctx context.Context, jwt, orderID string, itemIDs []string, reason string) (clients.Return, error) {
	f.seenJWT = jwt
	f.seenOrd = orderID
	f.seenIDs = itemIDs
	f.seenRsn = reason
	return f.ret, f.err
}

func TestInitiateReturnTool_Success(t *testing.T) {
	fake := &fakeReturnsAPI{ret: clients.Return{ID: "r1", OrderID: "o1", Status: "requested", Reason: "doesn't fit"}}
	tool := NewInitiateReturnTool(fake)

	args := json.RawMessage(`{"order_id":"o1","item_ids":["i1"],"reason":"doesn't fit"}`)
	res, err := tool.Call(ctxWithJWT("tok"), args, "user-1")
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if fake.seenJWT != "tok" || fake.seenOrd != "o1" || fake.seenRsn != "doesn't fit" {
		t.Errorf("seen = %+v", fake)
	}
	m := res.Content.(map[string]any)
	if m["status"] != "requested" {
		t.Errorf("content = %+v", m)
	}
}

func TestInitiateReturnTool_MissingFields(t *testing.T) {
	tool := NewInitiateReturnTool(&fakeReturnsAPI{})
	for _, args := range []string{
		`{}`,
		`{"order_id":"o1"}`,
		`{"order_id":"o1","item_ids":[]}`,
		`{"order_id":"o1","item_ids":["i1"]}`,
		`{"item_ids":["i1"],"reason":"r"}`,
	} {
		if _, err := tool.Call(ctxWithJWT("tok"), json.RawMessage(args), "user-1"); err == nil {
			t.Errorf("expected error for args %s", args)
		}
	}
}

func TestInitiateReturnTool_RequiresUserID(t *testing.T) {
	tool := NewInitiateReturnTool(&fakeReturnsAPI{})
	args := json.RawMessage(`{"order_id":"o1","item_ids":["i1"],"reason":"r"}`)
	_, err := tool.Call(ctxWithJWT("tok"), args, "")
	if err == nil {
		t.Fatal("expected error")
	}
}
