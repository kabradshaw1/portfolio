package authclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLoginSendsIncludeTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.URL.Path != "/auth/login" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q", got)
		}

		var body struct {
			Email         string `json:"email"`
			Password      string `json:"password"`
			IncludeTokens bool   `json:"includeTokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.Email != "user@example.test" || body.Password != "secret" || !body.IncludeTokens {
			t.Fatalf("body = %#v", body)
		}

		_ = json.NewEncoder(w).Encode(TokenResponse{
			AccessToken:      "access-1",
			RefreshToken:     "refresh-1",
			ExpiresInSeconds: 3600,
		})
	}))
	defer server.Close()

	client := New(server.URL+"/auth/", server.Client())
	got, err := client.Login(context.Background(), "user@example.test", "secret")
	if err != nil {
		t.Fatalf("Login error: %v", err)
	}
	if got.AccessToken != "access-1" || got.RefreshToken != "refresh-1" || got.ExpiresInSeconds != 3600 {
		t.Fatalf("response = %#v", got)
	}
}

func TestRefreshSendsIncludeTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.URL.Path != "/auth/refresh" {
			t.Fatalf("path = %s", r.URL.Path)
		}

		var body struct {
			RefreshToken  string `json:"refreshToken"`
			IncludeTokens bool   `json:"includeTokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.RefreshToken != "refresh-old" || !body.IncludeTokens {
			t.Fatalf("body = %#v", body)
		}

		_ = json.NewEncoder(w).Encode(TokenResponse{
			AccessToken:      "access-new",
			RefreshToken:     "refresh-new",
			ExpiresInSeconds: 1800,
		})
	}))
	defer server.Close()

	client := New(server.URL+"/auth", server.Client())
	got, err := client.Refresh(context.Background(), "refresh-old")
	if err != nil {
		t.Fatalf("Refresh error: %v", err)
	}
	if got.AccessToken != "access-new" || got.RefreshToken != "refresh-new" || got.ExpiresInSeconds != 1800 {
		t.Fatalf("response = %#v", got)
	}
}

func TestHTTPErrorIncludesStatusAndBoundedExcerpt(t *testing.T) {
	body := strings.Repeat("x", 300)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, body, http.StatusUnauthorized)
	}))
	defer server.Close()

	client := New(server.URL+"/auth", server.Client())
	_, err := client.Login(context.Background(), "user@example.test", "bad-password")
	if err == nil {
		t.Fatal("expected error")
	}
	got := err.Error()
	for _, want := range []string{"POST", "/login", "status 401"} {
		if !strings.Contains(got, want) {
			t.Fatalf("error = %q, want %q", got, want)
		}
	}
	if strings.Contains(got, strings.Repeat("x", 257)) {
		t.Fatalf("error excerpt was not bounded: %q", got)
	}
	if !strings.Contains(got, strings.Repeat("x", 256)) {
		t.Fatalf("error = %q, want 256-byte excerpt", got)
	}
}

func TestRejectsInvalidTokenResponse(t *testing.T) {
	tests := []struct {
		name     string
		response TokenResponse
		want     string
	}{
		{name: "missing access token", response: TokenResponse{RefreshToken: "refresh", ExpiresInSeconds: 1}, want: "missing access token"},
		{name: "missing refresh token", response: TokenResponse{AccessToken: "access", ExpiresInSeconds: 1}, want: "missing refresh token"},
		{name: "non-positive expiry", response: TokenResponse{AccessToken: "access", RefreshToken: "refresh"}, want: "expiresInSeconds must be positive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(tt.response)
			}))
			defer server.Close()

			client := New(server.URL, server.Client())
			_, err := client.Login(context.Background(), "user@example.test", "secret")
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestNewUsesDefaultHTTPClient(t *testing.T) {
	client := New("http://example.test/auth/", nil)
	if client.baseURL != "http://example.test/auth" {
		t.Fatalf("baseURL = %q", client.baseURL)
	}
	if client.httpClient == nil {
		t.Fatal("httpClient is nil")
	}
	if client.httpClient.Timeout != 30*time.Second {
		t.Fatalf("timeout = %s", client.httpClient.Timeout)
	}
}
