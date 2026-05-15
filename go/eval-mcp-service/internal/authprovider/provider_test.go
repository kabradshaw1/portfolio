package authprovider

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/authclient"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/tokenstore"
)

func TestTokenReusesValidCachedAccessToken(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{
		state: tokenstore.State{
			AccessToken:    "access-cached",
			RefreshToken:   "refresh-cached",
			AccessTokenExp: now.Add(2 * time.Minute),
			AuthEmail:      "user@example.test",
			AuthServiceURL: "http://auth.test/auth",
		},
		ok: true,
	}
	client := &fakeClient{}
	provider := New(client, store, testConfig(now))

	got, err := provider.Token(context.Background())
	if err != nil {
		t.Fatalf("Token error: %v", err)
	}
	if got != "access-cached" {
		t.Fatalf("token = %q", got)
	}
	if client.loginCalls != 0 || client.refreshCalls != 0 {
		t.Fatalf("client calls: login=%d refresh=%d", client.loginCalls, client.refreshCalls)
	}
	if len(store.saves) != 0 {
		t.Fatalf("saves = %#v", store.saves)
	}
}

func TestTokenRefreshesNearExpiryAndSavesNewExpiry(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{
		state: tokenstore.State{
			AccessToken:    "access-old",
			RefreshToken:   "refresh-old",
			AccessTokenExp: now.Add(30 * time.Second),
			AuthEmail:      "user@example.test",
			AuthServiceURL: "http://auth.test/auth",
		},
		ok: true,
	}
	client := &fakeClient{
		refreshResponse: authclient.TokenResponse{
			AccessToken:      "access-new",
			RefreshToken:     "refresh-new",
			ExpiresInSeconds: 120,
		},
	}
	provider := New(client, store, testConfig(now))

	got, err := provider.Token(context.Background())
	if err != nil {
		t.Fatalf("Token error: %v", err)
	}
	if got != "access-new" {
		t.Fatalf("token = %q", got)
	}
	if client.refreshCalls != 1 || client.loginCalls != 0 {
		t.Fatalf("client calls: login=%d refresh=%d", client.loginCalls, client.refreshCalls)
	}
	if client.refreshToken != "refresh-old" {
		t.Fatalf("refresh token = %q", client.refreshToken)
	}
	assertSavedState(t, store, tokenstore.State{
		AccessToken:    "access-new",
		RefreshToken:   "refresh-new",
		AccessTokenExp: now.Add(120 * time.Second),
		AuthEmail:      "user@example.test",
		AuthServiceURL: "http://auth.test/auth",
		WrittenAt:      now,
	})
}

func TestTokenFallsBackToLoginWhenRefreshFails(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{
		state: tokenstore.State{
			AccessToken:    "access-old",
			RefreshToken:   "refresh-old",
			AccessTokenExp: now.Add(-time.Second),
			AuthEmail:      "user@example.test",
			AuthServiceURL: "http://auth.test/auth",
		},
		ok: true,
	}
	client := &fakeClient{
		refreshErr: errors.New("refresh failed"),
		loginResponse: authclient.TokenResponse{
			AccessToken:      "access-login",
			RefreshToken:     "refresh-login",
			ExpiresInSeconds: 300,
		},
	}
	provider := New(client, store, testConfig(now))

	got, err := provider.Token(context.Background())
	if err != nil {
		t.Fatalf("Token error: %v", err)
	}
	if got != "access-login" {
		t.Fatalf("token = %q", got)
	}
	if client.refreshCalls != 1 || client.loginCalls != 1 {
		t.Fatalf("client calls: login=%d refresh=%d", client.loginCalls, client.refreshCalls)
	}
	if client.loginEmail != "user@example.test" || client.loginPassword != "secret" {
		t.Fatalf("login credentials: email=%q password=%q", client.loginEmail, client.loginPassword)
	}
	assertSavedState(t, store, tokenstore.State{
		AccessToken:    "access-login",
		RefreshToken:   "refresh-login",
		AccessTokenExp: now.Add(300 * time.Second),
		AuthEmail:      "user@example.test",
		AuthServiceURL: "http://auth.test/auth",
		WrittenAt:      now,
	})
}

func TestInvalidateForcesRefreshForValidCachedAccessToken(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{
		state: tokenstore.State{
			AccessToken:    "access-cached",
			RefreshToken:   "refresh-cached",
			AccessTokenExp: now.Add(2 * time.Minute),
			AuthEmail:      "user@example.test",
			AuthServiceURL: "http://auth.test/auth",
		},
		ok: true,
	}
	client := &fakeClient{
		refreshResponse: authclient.TokenResponse{
			AccessToken:      "access-refreshed",
			RefreshToken:     "refresh-refreshed",
			ExpiresInSeconds: 180,
		},
	}
	provider := New(client, store, testConfig(now))

	provider.Invalidate()
	got, err := provider.Token(context.Background())
	if err != nil {
		t.Fatalf("Token error: %v", err)
	}
	if got != "access-refreshed" {
		t.Fatalf("token = %q", got)
	}
	if client.refreshCalls != 1 || client.loginCalls != 0 {
		t.Fatalf("client calls: login=%d refresh=%d", client.loginCalls, client.refreshCalls)
	}
	assertSavedState(t, store, tokenstore.State{
		AccessToken:    "access-refreshed",
		RefreshToken:   "refresh-refreshed",
		AccessTokenExp: now.Add(180 * time.Second),
		AuthEmail:      "user@example.test",
		AuthServiceURL: "http://auth.test/auth",
		WrittenAt:      now,
	})
}

func testConfig(now time.Time) Config {
	return Config{
		Email:          "user@example.test",
		Password:       "secret",
		AuthServiceURL: "http://auth.test/auth",
		RefreshSkew:    time.Minute,
		Now: func() time.Time {
			return now
		},
	}
}

func assertSavedState(t *testing.T, store *fakeStore, want tokenstore.State) {
	t.Helper()

	if len(store.saves) != 1 {
		t.Fatalf("save count = %d", len(store.saves))
	}
	got := store.saves[0]
	if got.AccessToken != want.AccessToken ||
		got.RefreshToken != want.RefreshToken ||
		!got.AccessTokenExp.Equal(want.AccessTokenExp) ||
		got.AuthEmail != want.AuthEmail ||
		got.AuthServiceURL != want.AuthServiceURL ||
		!got.WrittenAt.Equal(want.WrittenAt) {
		t.Fatalf("saved state = %#v, want %#v", got, want)
	}
}

type fakeClient struct {
	loginResponse   authclient.TokenResponse
	loginErr        error
	loginCalls      int
	loginEmail      string
	loginPassword   string
	refreshResponse authclient.TokenResponse
	refreshErr      error
	refreshCalls    int
	refreshToken    string
}

func (c *fakeClient) Login(_ context.Context, email, password string) (authclient.TokenResponse, error) {
	c.loginCalls++
	c.loginEmail = email
	c.loginPassword = password
	if c.loginErr != nil {
		return authclient.TokenResponse{}, c.loginErr
	}
	return c.loginResponse, nil
}

func (c *fakeClient) Refresh(_ context.Context, refreshToken string) (authclient.TokenResponse, error) {
	c.refreshCalls++
	c.refreshToken = refreshToken
	if c.refreshErr != nil {
		return authclient.TokenResponse{}, c.refreshErr
	}
	return c.refreshResponse, nil
}

type fakeStore struct {
	state tokenstore.State
	ok    bool
	err   error
	saves []tokenstore.State
}

func (s *fakeStore) Load(context.Context) (tokenstore.State, bool, error) {
	if s.err != nil {
		return tokenstore.State{}, false, s.err
	}
	return s.state, s.ok, nil
}

func (s *fakeStore) Save(_ context.Context, state tokenstore.State) error {
	s.saves = append(s.saves, state)
	s.state = state
	s.ok = true
	return nil
}
