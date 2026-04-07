package google

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExchangeCode_Success(t *testing.T) {
	var tokenHits, userinfoHits int

	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		tokenHits++
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if got := r.Form.Get("code"); got != "auth-code-123" {
			t.Errorf("code = %q, want auth-code-123", got)
		}
		if got := r.Form.Get("client_id"); got != "test-client" {
			t.Errorf("client_id = %q, want test-client", got)
		}
		if got := r.Form.Get("client_secret"); got != "test-secret" {
			t.Errorf("client_secret = %q, want test-secret", got)
		}
		if got := r.Form.Get("grant_type"); got != "authorization_code" {
			t.Errorf("grant_type = %q, want authorization_code", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "g-access-token"})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		userinfoHits++
		if got := r.Header.Get("Authorization"); got != "Bearer g-access-token" {
			t.Errorf("authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"email":   "alice@example.com",
			"name":    "Alice",
			"picture": "https://example.com/alice.png",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewClient("test-client", "test-secret", srv.URL+"/token", srv.URL+"/userinfo")
	info, err := c.ExchangeCode(context.Background(), "auth-code-123", "http://localhost:3000/go/login")
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if info.Email != "alice@example.com" || info.Name != "Alice" || info.Picture != "https://example.com/alice.png" {
		t.Errorf("unexpected UserInfo: %+v", info)
	}
	if tokenHits != 1 || userinfoHits != 1 {
		t.Errorf("hits: token=%d userinfo=%d", tokenHits, userinfoHits)
	}
}

func TestExchangeCode_TokenEndpointError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer srv.Close()

	c := NewClient("id", "secret", srv.URL, srv.URL)
	_, err := c.ExchangeCode(context.Background(), "bad", "http://localhost:3000/go/login")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "token") {
		t.Errorf("error should mention token: %v", err)
	}
}

func TestExchangeCode_UserinfoEndpointError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "ok"})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewClient("id", "secret", srv.URL+"/token", srv.URL+"/userinfo")
	_, err := c.ExchangeCode(context.Background(), "code", "http://localhost:3000/go/login")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "userinfo") {
		t.Errorf("error should mention userinfo: %v", err)
	}
}

func TestExchangeCode_MalformedTokenJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	c := NewClient("id", "secret", srv.URL, srv.URL)
	_, err := c.ExchangeCode(context.Background(), "code", "http://localhost:3000/go/login")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
