package resources

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/jwtctx"
)

type fakeUserClient struct {
	orders map[string]string
	cart   map[string]string
	err    error
}

func (f fakeUserClient) Orders(ctx context.Context, userID string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	v, ok := f.orders[userID]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}
func (f fakeUserClient) Cart(ctx context.Context, userID string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	v, ok := f.cart[userID]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}

func TestUserOrdersReadsAuthenticatedUser(t *testing.T) {
	c := fakeUserClient{orders: map[string]string{"u1": `[{"id":"o1"}]`}}
	ctx := jwtctx.WithUserID(context.Background(), "u1")
	r := NewUserOrdersResource(c)
	got, err := r.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Text, `"id":"o1"`) {
		t.Fatalf("got %s", got.Text)
	}
	if r.URI() != "user://orders" {
		t.Fatalf("uri: %s", r.URI())
	}
}

func TestUserOrdersAnonymousReturns404(t *testing.T) {
	c := fakeUserClient{orders: map[string]string{"u1": "[]"}}
	r := NewUserOrdersResource(c)
	_, err := r.Read(context.Background())
	if !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("expected ErrResourceNotFound for anonymous, got %v", err)
	}
}

func TestUserCartReadsAuthenticatedUser(t *testing.T) {
	c := fakeUserClient{cart: map[string]string{"u1": `{"items":[]}`}}
	ctx := jwtctx.WithUserID(context.Background(), "u1")
	r := NewUserCartResource(c)
	got, err := r.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.URI != "user://cart" {
		t.Fatalf("URI: %s", got.URI)
	}
}

func TestUserCartAnonymousReturns404(t *testing.T) {
	c := fakeUserClient{cart: map[string]string{"u1": "{}"}}
	r := NewUserCartResource(c)
	_, err := r.Read(context.Background())
	if !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("expected ErrResourceNotFound, got %v", err)
	}
}

func TestUserOrdersPropagatesClientError(t *testing.T) {
	c := fakeUserClient{err: errors.New("downstream")}
	ctx := jwtctx.WithUserID(context.Background(), "u1")
	r := NewUserOrdersResource(c)
	_, err := r.Read(ctx)
	if err == nil {
		t.Fatalf("expected error from client")
	}
}
