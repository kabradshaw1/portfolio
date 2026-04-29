package clients

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/jwtctx"
)

func TestUserResourceClientOrdersForwardsJWT(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/orders" {
			t.Fatalf("path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token-1" {
			t.Fatalf("authorization: %s", got)
		}
		fmt.Fprint(w, `{"orders":[{"id":"o1"}]}`)
	}))
	defer srv.Close()

	ctx := jwtctx.WithJWT(context.Background(), "token-1")
	got, err := NewUserResourceClient(srv.URL).Orders(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if got != `{"orders":[{"id":"o1"}]}` {
		t.Fatalf("body: %s", got)
	}
}

func TestUserResourceClientCartForwardsJWT(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cart" {
			t.Fatalf("path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token-1" {
			t.Fatalf("authorization: %s", got)
		}
		fmt.Fprint(w, `{"items":[]}`)
	}))
	defer srv.Close()

	ctx := jwtctx.WithJWT(context.Background(), "token-1")
	got, err := NewUserResourceClient(srv.URL).Cart(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if got != `{"items":[]}` {
		t.Fatalf("body: %s", got)
	}
}
