# Eval MCP Auth Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let `go/eval-mcp-service` authenticate through `go/auth-service`, cache tokens locally, and call the authenticated Python eval API without manually supplied bearer tokens.

**Architecture:** Auth-service keeps cookie behavior by default and adds opt-in token fields for machine clients. Eval MCP gains a focused auth client, token store, and auth provider that supplies bearer tokens to the existing eval API client. The eval API client retries one request after a `401` by asking the provider to invalidate and recover token state.

**Tech Stack:** Go, Gin, `httptest`, JWT tokens from `go/auth-service`, local JSON token cache, existing MCP stdio service, existing Python eval bearer-auth dependency.

---

## File Structure

- Modify `go/auth-service/internal/model/user.go`: add `IncludeTokens` to login and refresh request models, plus `ExpiresInSeconds` to auth responses.
- Modify `go/auth-service/internal/service/auth.go`: populate access-token TTL seconds in generated auth responses.
- Modify `go/auth-service/internal/handler/auth.go`: return token fields only when `includeTokens` is true for login and refresh.
- Modify `go/auth-service/internal/handler/auth_test.go`: cover default no-token body and opt-in token body.
- Modify `go/eval-mcp-service/internal/config/config.go`: add auth-service URL, auth email, auth password, token cache path, and refresh skew.
- Modify `go/eval-mcp-service/internal/config/config_test.go`: cover defaults, overrides, static-token bypass, and missing auth config.
- Create `go/eval-mcp-service/internal/authclient/client.go`: typed login and refresh client for auth-service.
- Create `go/eval-mcp-service/internal/authclient/client_test.go`: auth-client request and error tests.
- Create `go/eval-mcp-service/internal/tokenstore/file.go`: local JSON token cache with permission checks.
- Create `go/eval-mcp-service/internal/tokenstore/file_test.go`: cache read, write, expiry, and unsafe-permission tests.
- Create `go/eval-mcp-service/internal/authprovider/provider.go`: token provider that loads cache, refreshes, logs in, invalidates, and hides secrets.
- Create `go/eval-mcp-service/internal/authprovider/provider_test.go`: provider behavior tests with fake auth client and fake store.
- Modify `go/eval-mcp-service/internal/evalapi/client.go`: accept a token provider and retry once on `401`.
- Modify `go/eval-mcp-service/internal/evalapi/client_test.go`: keep static token coverage and add provider retry coverage.
- Modify `go/eval-mcp-service/cmd/eval-mcp/main.go`: wire static token override or auth provider into the eval API client.
- Modify `go/eval-mcp-service/cmd/eval-mcp/main_test.go`: verify static-token bypass and auth-provider config.
- Modify `go/eval-mcp-service/README.md`: document auth-service configuration and secret handling.

## Task 0: Create Feature Worktree

**Files:**
- No source files changed in this task.

- [ ] **Step 1: Create the feature worktree**

Run from `/Users/kylebradshaw/repos/gen_ai_engineer`:

```bash
git fetch origin
git worktree add .codex/worktrees/eval-mcp-auth-integration -b feature/eval-mcp-auth-integration main
```

Expected: worktree is created at `.codex/worktrees/eval-mcp-auth-integration`.

- [ ] **Step 2: Confirm all work is inside the feature worktree**

Run:

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/.codex/worktrees/eval-mcp-auth-integration
pwd
git branch --show-current
git rev-parse --show-toplevel
```

Expected:

```text
/Users/kylebradshaw/repos/gen_ai_engineer/.codex/worktrees/eval-mcp-auth-integration
feature/eval-mcp-auth-integration
/Users/kylebradshaw/repos/gen_ai_engineer/.codex/worktrees/eval-mcp-auth-integration
```

- [ ] **Step 3: Commit boundary**

Do not commit in this task. All later commands run with workdir:

```text
/Users/kylebradshaw/repos/gen_ai_engineer/.codex/worktrees/eval-mcp-auth-integration
```

## Task 1: Auth-Service Opt-In Token Response

**Files:**
- Modify: `go/auth-service/internal/model/user.go`
- Modify: `go/auth-service/internal/service/auth.go`
- Modify: `go/auth-service/internal/handler/auth.go`
- Test: `go/auth-service/internal/handler/auth_test.go`

- [ ] **Step 1: Add failing login tests for opt-in tokens**

Append these tests to `go/auth-service/internal/handler/auth_test.go`:

```go
func TestLoginHandler_DefaultResponseOmitsTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := newMockUserRepo()
	svc := service.NewAuthService(repo, "test-secret", 900000, 604800000)
	_, err := svc.Register(context.Background(), "smoke@example.com", "password123456", "Smoke")
	if err != nil {
		t.Fatalf("register fixture: %v", err)
	}
	h := handler.NewAuthHandler(svc, nil, service.NewTokenDenylist(nil), 15*time.Minute, 7*24*time.Hour, defaultCookieCfg())

	router := testRouter()
	router.POST("/auth/login", h.Login)

	body := strings.NewReader(`{"email":"smoke@example.com","password":"password123456"}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := resp["accessToken"]; ok {
		t.Fatal("accessToken must not appear without includeTokens")
	}
	if _, ok := resp["refreshToken"]; ok {
		t.Fatal("refreshToken must not appear without includeTokens")
	}
	if _, ok := resp["expiresInSeconds"]; ok {
		t.Fatal("expiresInSeconds must not appear without includeTokens")
	}
	if !hasCookie(w, "access_token") || !hasCookie(w, "refresh_token") {
		t.Fatal("expected auth cookies")
	}
}

func TestLoginHandler_IncludeTokensReturnsTokenFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := newMockUserRepo()
	svc := service.NewAuthService(repo, "test-secret", 900000, 604800000)
	_, err := svc.Register(context.Background(), "smoke@example.com", "password123456", "Smoke")
	if err != nil {
		t.Fatalf("register fixture: %v", err)
	}
	h := handler.NewAuthHandler(svc, nil, service.NewTokenDenylist(nil), 15*time.Minute, 7*24*time.Hour, defaultCookieCfg())

	router := testRouter()
	router.POST("/auth/login", h.Login)

	body := strings.NewReader(`{"email":"smoke@example.com","password":"password123456","includeTokens":true}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["accessToken"] == "" || resp["refreshToken"] == "" {
		t.Fatalf("expected token fields, got %#v", resp)
	}
	if resp["expiresInSeconds"] != float64(900) {
		t.Fatalf("expiresInSeconds = %#v", resp["expiresInSeconds"])
	}
	if !hasCookie(w, "access_token") || !hasCookie(w, "refresh_token") {
		t.Fatal("expected auth cookies")
	}
}
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```bash
cd go/auth-service
go test ./internal/handler -run 'TestLoginHandler_(DefaultResponseOmitsTokens|IncludeTokensReturnsTokenFields)' -count=1
```

Expected: `TestLoginHandler_IncludeTokensReturnsTokenFields` fails because `includeTokens` is not modeled and token fields are not returned.

- [ ] **Step 3: Add request and response fields**

Update `go/auth-service/internal/model/user.go`:

```go
type LoginRequest struct {
	Email         string `json:"email" binding:"required,email"`
	Password      string `json:"password" binding:"required"`
	IncludeTokens bool   `json:"includeTokens"`
}

type RefreshRequest struct {
	RefreshToken   string `json:"refreshToken" binding:"required"`
	IncludeTokens  bool   `json:"includeTokens"`
}

type AuthResponse struct {
	AccessToken      string `json:"accessToken"`
	RefreshToken     string `json:"refreshToken"`
	UserID           string `json:"userId"`
	Email            string `json:"email"`
	Name             string `json:"name"`
	AvatarURL        string `json:"avatarUrl,omitempty"`
	ExpiresInSeconds int64  `json:"expiresInSeconds"`
}
```

- [ ] **Step 4: Populate access token TTL**

In `go/auth-service/internal/service/auth.go`, update `generateTokens` to set `ExpiresInSeconds`:

```go
	return &model.AuthResponse{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		UserID:           user.ID.String(),
		Email:            user.Email,
		Name:             user.Name,
		AvatarURL:        avatar,
		ExpiresInSeconds: int64(s.accessTokenTTL.Seconds()),
	}, nil
```

- [ ] **Step 5: Add token-aware response helper**

In `go/auth-service/internal/handler/auth.go`, add this helper near `clearAuthCookies`:

```go
func authJSON(resp *model.AuthResponse, includeTokens bool) gin.H {
	body := gin.H{
		"userId":    resp.UserID,
		"email":     resp.Email,
		"name":      resp.Name,
		"avatarUrl": resp.AvatarURL,
	}
	if includeTokens {
		body["accessToken"] = resp.AccessToken
		body["refreshToken"] = resp.RefreshToken
		body["expiresInSeconds"] = resp.ExpiresInSeconds
	}
	return body
}
```

- [ ] **Step 6: Use the helper in login and refresh**

In `go/auth-service/internal/handler/auth.go`, replace the response bodies in `Login` and `Refresh`:

```go
	h.setAuthCookies(c, resp)
	c.JSON(http.StatusOK, authJSON(resp, req.IncludeTokens))
```

For `Refresh`, keep the cookie fallback. When refresh token comes from cookie and no JSON body was parsed, `req.IncludeTokens` stays false.

- [ ] **Step 7: Run auth handler tests**

Run:

```bash
cd go/auth-service
go test ./internal/handler -count=1
```

Expected: all handler tests pass.

- [ ] **Step 8: Commit auth-service contract**

Run:

```bash
git add go/auth-service/internal/model/user.go go/auth-service/internal/service/auth.go go/auth-service/internal/handler/auth.go go/auth-service/internal/handler/auth_test.go
git commit -m "feat: expose auth tokens for machine clients"
```

Expected: commit succeeds.

## Task 2: Eval MCP Auth Client And Token Store

**Files:**
- Create: `go/eval-mcp-service/internal/authclient/client.go`
- Create: `go/eval-mcp-service/internal/authclient/client_test.go`
- Create: `go/eval-mcp-service/internal/tokenstore/file.go`
- Create: `go/eval-mcp-service/internal/tokenstore/file_test.go`

- [ ] **Step 1: Add failing auth client tests**

Create `go/eval-mcp-service/internal/authclient/client_test.go`:

```go
package authclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginSendsIncludeTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/login" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		var body loginRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Email != "smoke@example.com" || body.Password != "secret" || !body.IncludeTokens {
			t.Fatalf("body = %#v", body)
		}
		_ = json.NewEncoder(w).Encode(TokenResponse{
			AccessToken:      "access",
			RefreshToken:     "refresh",
			ExpiresInSeconds: 900,
		})
	}))
	t.Cleanup(server.Close)

	client := New(server.URL+"/auth", server.Client())
	got, err := client.Login(context.Background(), "smoke@example.com", "secret")
	if err != nil {
		t.Fatalf("Login error: %v", err)
	}
	if got.AccessToken != "access" || got.RefreshToken != "refresh" || got.ExpiresInSeconds != 900 {
		t.Fatalf("response = %#v", got)
	}
}

func TestRefreshSendsIncludeTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/refresh" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		var body refreshRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.RefreshToken != "refresh-old" || !body.IncludeTokens {
			t.Fatalf("body = %#v", body)
		}
		_ = json.NewEncoder(w).Encode(TokenResponse{
			AccessToken:      "access-new",
			RefreshToken:     "refresh-new",
			ExpiresInSeconds: 900,
		})
	}))
	t.Cleanup(server.Close)

	client := New(server.URL+"/auth", server.Client())
	got, err := client.Refresh(context.Background(), "refresh-old")
	if err != nil {
		t.Fatalf("Refresh error: %v", err)
	}
	if got.AccessToken != "access-new" || got.RefreshToken != "refresh-new" {
		t.Fatalf("response = %#v", got)
	}
}

func TestHTTPErrorIncludesStatusAndExcerpt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, strings.Repeat("x", 300), http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	client := New(server.URL+"/auth", server.Client())
	_, err := client.Login(context.Background(), "smoke@example.com", "bad")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "status 401") {
		t.Fatalf("error = %v", err)
	}
}
```

- [ ] **Step 2: Run auth client tests and verify failure**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/authclient -count=1
```

Expected: package does not compile because `internal/authclient` does not exist.

- [ ] **Step 3: Implement auth client**

Create `go/eval-mcp-service/internal/authclient/client.go`:

```go
package authclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultHTTPTimeout = 30 * time.Second
	errorExcerptLimit  = 256
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

type TokenResponse struct {
	AccessToken      string `json:"accessToken"`
	RefreshToken     string `json:"refreshToken"`
	ExpiresInSeconds int64  `json:"expiresInSeconds"`
}

type loginRequest struct {
	Email         string `json:"email"`
	Password      string `json:"password"`
	IncludeTokens bool   `json:"includeTokens"`
}

type refreshRequest struct {
	RefreshToken  string `json:"refreshToken"`
	IncludeTokens bool   `json:"includeTokens"`
}

func New(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), httpClient: httpClient}
}

func (c *Client) Login(ctx context.Context, email, password string) (TokenResponse, error) {
	return c.do(ctx, http.MethodPost, "/login", loginRequest{
		Email: email, Password: password, IncludeTokens: true,
	})
}

func (c *Client) Refresh(ctx context.Context, refreshToken string) (TokenResponse, error) {
	return c.do(ctx, http.MethodPost, "/refresh", refreshRequest{
		RefreshToken: refreshToken, IncludeTokens: true,
	})
}

func (c *Client) do(ctx context.Context, method, path string, body any) (TokenResponse, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return TokenResponse{}, fmt.Errorf("%s %s: encode request: %w", method, path, err)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, &buf)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("%s %s: create request: %w", method, path, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		excerpt, _ := io.ReadAll(io.LimitReader(resp.Body, errorExcerptLimit))
		return TokenResponse{}, fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(excerpt)))
	}
	var out TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return TokenResponse{}, fmt.Errorf("%s %s: decode response: %w", method, path, err)
	}
	if out.AccessToken == "" || out.RefreshToken == "" || out.ExpiresInSeconds <= 0 {
		return TokenResponse{}, fmt.Errorf("%s %s: response missing token fields", method, path)
	}
	return out, nil
}
```

- [ ] **Step 4: Add failing token store tests**

Create `go/eval-mcp-service/internal/tokenstore/file_test.go`:

```go
package tokenstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileStoreSaveLoadAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	store := NewFileStore(path)
	now := time.Now().UTC()
	state := State{
		AccessToken:    "access",
		RefreshToken:   "refresh",
		AccessTokenExp: now.Add(15 * time.Minute),
		AuthEmail:      "smoke@example.com",
		AuthServiceURL: "http://auth/auth",
		WrittenAt:      now,
	}
	if err := store.Save(context.Background(), state); err != nil {
		t.Fatalf("Save error: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o", got)
	}
	loaded, ok, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if !ok {
		t.Fatal("expected cached state")
	}
	if loaded.AccessToken != state.AccessToken || loaded.RefreshToken != state.RefreshToken || loaded.AuthEmail != state.AuthEmail {
		t.Fatalf("loaded = %#v", loaded)
	}
}

func TestFileStoreMissingFile(t *testing.T) {
	store := NewFileStore(filepath.Join(t.TempDir(), "missing.json"))
	_, ok, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if ok {
		t.Fatal("expected no cached state")
	}
}

func TestFileStoreRejectsUnsafePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(path, []byte(`{"accessToken":"access"}`), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	store := NewFileStore(path)
	_, _, err := store.Load(context.Background())
	if err == nil {
		t.Fatal("expected unsafe permissions error")
	}
}
```

- [ ] **Step 5: Run token store tests and verify failure**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/tokenstore -count=1
```

Expected: package does not compile because `internal/tokenstore` does not exist.

- [ ] **Step 6: Implement token store**

Create `go/eval-mcp-service/internal/tokenstore/file.go`:

```go
package tokenstore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type State struct {
	AccessToken    string    `json:"accessToken"`
	RefreshToken   string    `json:"refreshToken"`
	AccessTokenExp time.Time `json:"accessTokenExp"`
	AuthEmail      string    `json:"authEmail"`
	AuthServiceURL string    `json:"authServiceUrl"`
	WrittenAt      time.Time `json:"writtenAt"`
}

type FileStore struct {
	path string
}

func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

func (s *FileStore) Load(_ context.Context) (State, bool, error) {
	info, err := os.Stat(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, false, nil
		}
		return State{}, false, fmt.Errorf("stat token cache: %w", err)
	}
	if info.Mode().Perm() != 0o600 {
		return State{}, false, fmt.Errorf("token cache %s has permissions %o, want 600", s.path, info.Mode().Perm())
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return State{}, false, fmt.Errorf("read token cache: %w", err)
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, false, fmt.Errorf("decode token cache: %w", err)
	}
	return state, true, nil
}

func (s *FileStore) Save(_ context.Context, state State) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create token cache directory: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode token cache: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write token cache temp file: %w", err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return fmt.Errorf("chmod token cache temp file: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("replace token cache: %w", err)
	}
	return nil
}
```

- [ ] **Step 7: Run auth client and token store tests**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/authclient ./internal/tokenstore -count=1
```

Expected: both packages pass.

- [ ] **Step 8: Commit auth client and token store**

Run:

```bash
git add go/eval-mcp-service/internal/authclient go/eval-mcp-service/internal/tokenstore
git commit -m "feat: add eval mcp auth token plumbing"
```

Expected: commit succeeds.

## Task 3: Eval MCP Auth Provider And Config

**Files:**
- Modify: `go/eval-mcp-service/internal/config/config.go`
- Modify: `go/eval-mcp-service/internal/config/config_test.go`
- Create: `go/eval-mcp-service/internal/authprovider/provider.go`
- Create: `go/eval-mcp-service/internal/authprovider/provider_test.go`

- [ ] **Step 1: Add failing config tests**

Update `go/eval-mcp-service/internal/config/config_test.go` default and override tests to assert these fields:

```go
if cfg.AuthServiceURL != "http://localhost:8091/auth" {
	t.Fatalf("AuthServiceURL = %q", cfg.AuthServiceURL)
}
if cfg.AuthEmail != "" || cfg.AuthPassword != "" {
	t.Fatalf("expected empty auth credentials by default")
}
if cfg.TokenCachePath != "data/eval-mcp-auth.json" {
	t.Fatalf("TokenCachePath = %q", cfg.TokenCachePath)
}
if cfg.TokenRefreshSkew != time.Minute {
	t.Fatalf("TokenRefreshSkew = %s", cfg.TokenRefreshSkew)
}
```

Add this test:

```go
func TestFromEnvRequiresAuthCredentialsWhenNoStaticToken(t *testing.T) {
	t.Setenv("EVAL_API_TOKEN", "")
	t.Setenv("EVAL_MCP_AUTH_EMAIL", "")
	t.Setenv("EVAL_MCP_AUTH_PASSWORD", "")

	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected missing auth credentials error")
	}
}
```

In `TestFromEnvDefaults`, set `EVAL_API_TOKEN` to `token-123` so defaults can be tested through the static-token bypass.

- [ ] **Step 2: Run config tests and verify failure**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/config -count=1
```

Expected: compile failure for missing config fields.

- [ ] **Step 3: Implement config fields and validation**

Update `go/eval-mcp-service/internal/config/config.go`:

```go
const (
	defaultDBPath           = "data/eval-mcp.db"
	defaultEvalAPIURL       = "http://localhost:8000/eval"
	defaultAuthServiceURL   = "http://localhost:8091/auth"
	defaultTokenCachePath   = "data/eval-mcp-auth.json"
	defaultPollInterval     = time.Second
	defaultWaitTimeout      = 5 * time.Minute
	defaultTokenRefreshSkew = time.Minute
)

type Config struct {
	DBPath           string
	EvalAPIURL       string
	APIToken         string
	AuthServiceURL   string
	AuthEmail        string
	AuthPassword     string
	TokenCachePath   string
	TokenRefreshSkew time.Duration
	PollInterval     time.Duration
	WaitTimeout      time.Duration
}
```

In `FromEnv`, parse `EVAL_MCP_TOKEN_REFRESH_SKEW` with `durationEnv`, reject non-positive values, and return:

```go
	cfg := Config{
		DBPath:           getenv("EVAL_MCP_DB_PATH", defaultDBPath),
		EvalAPIURL:       getenv("EVAL_API_URL", defaultEvalAPIURL),
		APIToken:         os.Getenv("EVAL_API_TOKEN"),
		AuthServiceURL:   getenv("AUTH_SERVICE_URL", defaultAuthServiceURL),
		AuthEmail:        os.Getenv("EVAL_MCP_AUTH_EMAIL"),
		AuthPassword:     os.Getenv("EVAL_MCP_AUTH_PASSWORD"),
		TokenCachePath:   getenv("EVAL_MCP_TOKEN_CACHE_PATH", defaultTokenCachePath),
		TokenRefreshSkew: tokenRefreshSkew,
		PollInterval:     pollInterval,
		WaitTimeout:      waitTimeout,
	}
	if cfg.APIToken == "" && (cfg.AuthEmail == "" || cfg.AuthPassword == "") {
		return Config{}, fmt.Errorf("EVAL_MCP_AUTH_EMAIL and EVAL_MCP_AUTH_PASSWORD are required when EVAL_API_TOKEN is not set")
	}
	return cfg, nil
```

- [ ] **Step 4: Add failing auth provider tests**

Create `go/eval-mcp-service/internal/authprovider/provider_test.go`:

```go
package authprovider

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/authclient"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/tokenstore"
)

type fakeAuthClient struct {
	loginCalls   int
	refreshCalls int
	refreshErr   error
	loginResp    authclient.TokenResponse
	refreshResp  authclient.TokenResponse
}

func (f *fakeAuthClient) Login(_ context.Context, _, _ string) (authclient.TokenResponse, error) {
	f.loginCalls++
	return f.loginResp, nil
}

func (f *fakeAuthClient) Refresh(_ context.Context, _ string) (authclient.TokenResponse, error) {
	f.refreshCalls++
	if f.refreshErr != nil {
		return authclient.TokenResponse{}, f.refreshErr
	}
	return f.refreshResp, nil
}

type fakeStore struct {
	state tokenstore.State
	ok    bool
	saved tokenstore.State
}

func (f *fakeStore) Load(context.Context) (tokenstore.State, bool, error) {
	return f.state, f.ok, nil
}

func (f *fakeStore) Save(_ context.Context, state tokenstore.State) error {
	f.saved = state
	f.state = state
	f.ok = true
	return nil
}

func TestProviderUsesValidCachedAccessToken(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{ok: true, state: tokenstore.State{
		AccessToken: "cached", RefreshToken: "refresh", AccessTokenExp: now.Add(10 * time.Minute),
		AuthEmail: "smoke@example.com", AuthServiceURL: "http://auth/auth",
	}}
	client := &fakeAuthClient{}
	provider := New(client, store, Config{
		Email: "smoke@example.com", Password: "secret", AuthServiceURL: "http://auth/auth", RefreshSkew: time.Minute, Now: func() time.Time { return now },
	})
	token, err := provider.Token(context.Background())
	if err != nil {
		t.Fatalf("Token error: %v", err)
	}
	if token != "cached" || client.loginCalls != 0 || client.refreshCalls != 0 {
		t.Fatalf("token=%q login=%d refresh=%d", token, client.loginCalls, client.refreshCalls)
	}
}

func TestProviderRefreshesNearExpiryToken(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{ok: true, state: tokenstore.State{
		AccessToken: "old", RefreshToken: "refresh-old", AccessTokenExp: now.Add(30 * time.Second),
		AuthEmail: "smoke@example.com", AuthServiceURL: "http://auth/auth",
	}}
	client := &fakeAuthClient{refreshResp: authclient.TokenResponse{AccessToken: "new", RefreshToken: "refresh-new", ExpiresInSeconds: 900}}
	provider := New(client, store, Config{
		Email: "smoke@example.com", Password: "secret", AuthServiceURL: "http://auth/auth", RefreshSkew: time.Minute, Now: func() time.Time { return now },
	})
	token, err := provider.Token(context.Background())
	if err != nil {
		t.Fatalf("Token error: %v", err)
	}
	if token != "new" || client.refreshCalls != 1 || client.loginCalls != 0 {
		t.Fatalf("token=%q login=%d refresh=%d", token, client.loginCalls, client.refreshCalls)
	}
	if store.saved.AccessTokenExp != now.Add(900*time.Second) {
		t.Fatalf("saved expiry = %s", store.saved.AccessTokenExp)
	}
}

func TestProviderFallsBackToLoginAfterRefreshFailure(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{ok: true, state: tokenstore.State{
		AccessToken: "old", RefreshToken: "refresh-old", AccessTokenExp: now.Add(-time.Second),
		AuthEmail: "smoke@example.com", AuthServiceURL: "http://auth/auth",
	}}
	client := &fakeAuthClient{
		refreshErr: errors.New("refresh failed"),
		loginResp: authclient.TokenResponse{AccessToken: "login-access", RefreshToken: "login-refresh", ExpiresInSeconds: 900},
	}
	provider := New(client, store, Config{
		Email: "smoke@example.com", Password: "secret", AuthServiceURL: "http://auth/auth", RefreshSkew: time.Minute, Now: func() time.Time { return now },
	})
	token, err := provider.Token(context.Background())
	if err != nil {
		t.Fatalf("Token error: %v", err)
	}
	if token != "login-access" || client.refreshCalls != 1 || client.loginCalls != 1 {
		t.Fatalf("token=%q login=%d refresh=%d", token, client.loginCalls, client.refreshCalls)
	}
}

func TestProviderInvalidateForcesRefresh(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{ok: true, state: tokenstore.State{
		AccessToken: "cached", RefreshToken: "refresh-old", AccessTokenExp: now.Add(10 * time.Minute),
		AuthEmail: "smoke@example.com", AuthServiceURL: "http://auth/auth",
	}}
	client := &fakeAuthClient{refreshResp: authclient.TokenResponse{AccessToken: "new", RefreshToken: "refresh-new", ExpiresInSeconds: 900}}
	provider := New(client, store, Config{
		Email: "smoke@example.com", Password: "secret", AuthServiceURL: "http://auth/auth", RefreshSkew: time.Minute, Now: func() time.Time { return now },
	})
	provider.Invalidate()
	token, err := provider.Token(context.Background())
	if err != nil {
		t.Fatalf("Token error: %v", err)
	}
	if token != "new" || client.refreshCalls != 1 || client.loginCalls != 0 {
		t.Fatalf("token=%q login=%d refresh=%d", token, client.loginCalls, client.refreshCalls)
	}
}
```

- [ ] **Step 5: Run auth provider tests and verify failure**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/authprovider -count=1
```

Expected: package does not compile because `internal/authprovider` does not exist.

- [ ] **Step 6: Implement auth provider**

Create `go/eval-mcp-service/internal/authprovider/provider.go`:

```go
package authprovider

import (
	"context"
	"sync"
	"time"

	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/authclient"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/tokenstore"
)

type AuthClient interface {
	Login(context.Context, string, string) (authclient.TokenResponse, error)
	Refresh(context.Context, string) (authclient.TokenResponse, error)
}

type Store interface {
	Load(context.Context) (tokenstore.State, bool, error)
	Save(context.Context, tokenstore.State) error
}

type Config struct {
	Email          string
	Password       string
	AuthServiceURL string
	RefreshSkew    time.Duration
	Now            func() time.Time
}

type Provider struct {
	client      AuthClient
	store       Store
	cfg         Config
	mu          sync.Mutex
	invalidated bool
}

func New(client AuthClient, store Store, cfg Config) *Provider {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Provider{client: client, store: store, cfg: cfg}
}

func (p *Provider) Token(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	state, ok, err := p.store.Load(ctx)
	if err != nil {
		return "", err
	}
	if ok && !p.invalidated && state.AuthEmail == p.cfg.Email && state.AuthServiceURL == p.cfg.AuthServiceURL && state.AccessToken != "" && p.cfg.Now().Add(p.cfg.RefreshSkew).Before(state.AccessTokenExp) {
		return state.AccessToken, nil
	}
	p.invalidated = false
	if ok && state.RefreshToken != "" {
		resp, err := p.client.Refresh(ctx, state.RefreshToken)
		if err == nil {
			return p.save(ctx, resp)
		}
	}
	resp, err := p.client.Login(ctx, p.cfg.Email, p.cfg.Password)
	if err != nil {
		return "", err
	}
	return p.save(ctx, resp)
}

func (p *Provider) Invalidate() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.invalidated = true
}

func (p *Provider) save(ctx context.Context, resp authclient.TokenResponse) (string, error) {
	now := p.cfg.Now().UTC()
	state := tokenstore.State{
		AccessToken:    resp.AccessToken,
		RefreshToken:   resp.RefreshToken,
		AccessTokenExp: now.Add(time.Duration(resp.ExpiresInSeconds) * time.Second),
		AuthEmail:      p.cfg.Email,
		AuthServiceURL: p.cfg.AuthServiceURL,
		WrittenAt:      now,
	}
	if err := p.store.Save(ctx, state); err != nil {
		return "", err
	}
	return resp.AccessToken, nil
}
```

- [ ] **Step 7: Run config and provider tests**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/config ./internal/authprovider -count=1
```

Expected: both packages pass.

- [ ] **Step 8: Commit provider and config**

Run:

```bash
git add go/eval-mcp-service/internal/config go/eval-mcp-service/internal/authprovider
git commit -m "feat: add eval mcp auth provider"
```

Expected: commit succeeds.

## Task 4: Eval API Retry Wiring And Command Startup

**Files:**
- Modify: `go/eval-mcp-service/internal/evalapi/client.go`
- Modify: `go/eval-mcp-service/internal/evalapi/client_test.go`
- Modify: `go/eval-mcp-service/cmd/eval-mcp/main.go`
- Modify: `go/eval-mcp-service/cmd/eval-mcp/main_test.go`
- Modify: `go/eval-mcp-service/README.md`

- [ ] **Step 1: Add failing eval API retry test**

Append to `go/eval-mcp-service/internal/evalapi/client_test.go`:

```go
type fakeTokenProvider struct {
	tokens      []string
	calls       int
	invalidated int
}

func (f *fakeTokenProvider) Token(context.Context) (string, error) {
	token := f.tokens[f.calls]
	f.calls++
	return token, nil
}

func (f *fakeTokenProvider) Invalidate() {
	f.invalidated++
}

func TestListDatasetsRetriesOnceAfterUnauthorized(t *testing.T) {
	provider := &fakeTokenProvider{tokens: []string{"expired", "fresh"}}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch requests {
		case 1:
			if got := r.Header.Get("Authorization"); got != "Bearer expired" {
				t.Fatalf("first Authorization = %q", got)
			}
			http.Error(w, "expired", http.StatusUnauthorized)
		case 2:
			if got := r.Header.Get("Authorization"); got != "Bearer fresh" {
				t.Fatalf("second Authorization = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"datasets": []Dataset{{ID: "ds-1", Name: "rag", ItemCount: 2}}})
		default:
			t.Fatalf("unexpected request %d", requests)
		}
	}))
	t.Cleanup(server.Close)

	client := NewWithTokenProvider(server.URL, provider, server.Client())
	got, err := client.ListDatasets(context.Background())
	if err != nil {
		t.Fatalf("ListDatasets error: %v", err)
	}
	if len(got) != 1 || got[0].ID != "ds-1" {
		t.Fatalf("datasets = %#v", got)
	}
	if provider.invalidated != 1 || provider.calls != 2 {
		t.Fatalf("invalidated=%d calls=%d", provider.invalidated, provider.calls)
	}
}
```

- [ ] **Step 2: Run eval API tests and verify failure**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/evalapi -run TestListDatasetsRetriesOnceAfterUnauthorized -count=1
```

Expected: compile failure for missing `NewWithTokenProvider`.

- [ ] **Step 3: Implement token provider and retry**

Update `go/eval-mcp-service/internal/evalapi/client.go`:

```go
type TokenProvider interface {
	Token(context.Context) (string, error)
	Invalidate()
}

type staticTokenProvider struct {
	token string
}

func (p staticTokenProvider) Token(context.Context) (string, error) {
	return p.token, nil
}

func (p staticTokenProvider) Invalidate() {
}
```

Change `Client` fields:

```go
type Client struct {
	baseURL       string
	tokenProvider TokenProvider
	httpClient    *http.Client
}
```

Keep the current constructor and add a provider constructor:

```go
func New(baseURL, token string, httpClient *http.Client) *Client {
	var provider TokenProvider
	if token != "" {
		provider = staticTokenProvider{token: token}
	}
	return NewWithTokenProvider(baseURL, provider, httpClient)
}

func NewWithTokenProvider(baseURL string, tokenProvider TokenProvider, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return &Client{
		baseURL:       strings.TrimRight(baseURL, "/"),
		tokenProvider: tokenProvider,
		httpClient:    httpClient,
	}
}
```

Split request execution so `401` retries once:

```go
func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	err := c.doOnce(ctx, method, path, body, out)
	if statusCode(err) == http.StatusUnauthorized && c.tokenProvider != nil {
		c.tokenProvider.Invalidate()
		return c.doOnce(ctx, method, path, body, out)
	}
	return err
}
```

Represent HTTP errors with a typed error:

```go
type HTTPError struct {
	Method     string
	Path       string
	StatusCode int
	Excerpt    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s %s: status %d: %s", e.Method, e.Path, e.StatusCode, e.Excerpt)
}

func statusCode(err error) int {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode
	}
	return 0
}
```

In `doOnce`, before sending the request:

```go
if c.tokenProvider != nil {
	token, err := c.tokenProvider.Token(ctx)
	if err != nil {
		return fmt.Errorf("%s %s: get auth token: %w", method, path, err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}
```

On non-2xx:

```go
return &HTTPError{Method: method, Path: path, StatusCode: resp.StatusCode, Excerpt: strings.TrimSpace(string(excerpt))}
```

- [ ] **Step 4: Wire provider in main**

Update `go/eval-mcp-service/cmd/eval-mcp/main.go` imports:

```go
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/authclient"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/authprovider"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/tokenstore"
```

Replace eval API construction:

```go
	httpClient := &http.Client{Timeout: cfg.WaitTimeout}
	var api *evalapi.Client
	if cfg.APIToken != "" {
		api = evalapi.New(cfg.EvalAPIURL, cfg.APIToken, httpClient)
	} else {
		authClient := authclient.New(cfg.AuthServiceURL, httpClient)
		cache := tokenstore.NewFileStore(cfg.TokenCachePath)
		provider := authprovider.New(authClient, cache, authprovider.Config{
			Email:          cfg.AuthEmail,
			Password:       cfg.AuthPassword,
			AuthServiceURL: cfg.AuthServiceURL,
			RefreshSkew:    cfg.TokenRefreshSkew,
		})
		api = evalapi.NewWithTokenProvider(cfg.EvalAPIURL, provider, httpClient)
	}
```

- [ ] **Step 5: Update command tests**

In `go/eval-mcp-service/cmd/eval-mcp/main_test.go`, keep `EVAL_API_TOKEN` in `TestRunWiresDependenciesAndCallsServer`. Add:

```go
func TestRunAcceptsAuthServiceConfigWhenStaticTokenMissing(t *testing.T) {
	apiServer := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(apiServer.Close)

	t.Setenv("EVAL_MCP_DB_PATH", filepath.Join(t.TempDir(), "eval-mcp.db"))
	t.Setenv("EVAL_API_URL", apiServer.URL)
	t.Setenv("EVAL_API_TOKEN", "")
	t.Setenv("AUTH_SERVICE_URL", "http://auth.local/auth")
	t.Setenv("EVAL_MCP_AUTH_EMAIL", "smoke@example.com")
	t.Setenv("EVAL_MCP_AUTH_PASSWORD", "secret")
	t.Setenv("EVAL_MCP_TOKEN_CACHE_PATH", filepath.Join(t.TempDir(), "auth.json"))
	t.Setenv("EVAL_MCP_POLL_INTERVAL", "10ms")
	t.Setenv("EVAL_MCP_WAIT_TIMEOUT", "2s")

	called := false
	err := run(context.Background(), log.New(&bytes.Buffer{}, "", 0), func(_ context.Context, application *app) error {
		called = true
		if application.service == nil {
			t.Fatal("expected service")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if !called {
		t.Fatal("expected runner to be called")
	}
}
```

- [ ] **Step 6: Update README configuration**

In `go/eval-mcp-service/README.md`, replace the configuration list with:

```markdown
- `EVAL_MCP_DB_PATH`: SQLite path, defaults to `data/eval-mcp.db`.
- `EVAL_API_URL`: eval API base URL, defaults to `http://localhost:8000/eval`.
- `EVAL_API_TOKEN`: optional bearer token override. When set, auth-service login is skipped.
- `AUTH_SERVICE_URL`: auth-service base URL, defaults to `http://localhost:8091/auth`.
- `EVAL_MCP_AUTH_EMAIL`: auth-service login email when `EVAL_API_TOKEN` is not set.
- `EVAL_MCP_AUTH_PASSWORD`: auth-service login password when `EVAL_API_TOKEN` is not set.
- `EVAL_MCP_TOKEN_CACHE_PATH`: auth token cache path, defaults to `data/eval-mcp-auth.json`.
- `EVAL_MCP_TOKEN_REFRESH_SKEW`: token refresh-ahead duration, defaults to `60s`.
- `EVAL_MCP_POLL_INTERVAL`: polling interval, defaults to `1s`.
- `EVAL_MCP_WAIT_TIMEOUT`: wait timeout, defaults to `5m`.
```

Add this registration example:

```bash
EVAL_MCP_AUTH_EMAIL=smoke@kylebradshaw.dev \
EVAL_MCP_AUTH_PASSWORD="$SMOKE_GO_PASSWORD" \
AUTH_SERVICE_URL=https://api.kylebradshaw.dev/go-auth/auth \
EVAL_API_URL=https://api.kylebradshaw.dev/eval \
codex mcp add eval -- zsh -lc 'cd /Users/kylebradshaw/repos/gen_ai_engineer/go/eval-mcp-service && exec go run ./cmd/eval-mcp'
```

- [ ] **Step 7: Run eval MCP tests**

Run:

```bash
cd go/eval-mcp-service
go test ./... -count=1
```

Expected: all eval MCP service tests pass.

- [ ] **Step 8: Commit eval MCP wiring**

Run:

```bash
git add go/eval-mcp-service
git commit -m "feat: authenticate eval mcp service with auth-service"
```

Expected: commit succeeds.

## Task 5: Verification And Pull Request

**Files:**
- No source files changed unless verification exposes a defect.

- [ ] **Step 1: Run auth-service tests**

Run:

```bash
cd go/auth-service
go test ./... -count=1
```

Expected: all auth-service tests pass.

- [ ] **Step 2: Run eval MCP tests**

Run:

```bash
cd go/eval-mcp-service
go test ./... -count=1
```

Expected: all eval MCP tests pass.

- [ ] **Step 3: Run Go preflight**

Run from repo root:

```bash
make preflight-go
```

Expected: Go preflight passes. If it fails because of an unrelated existing issue, record the failing command and exact package, then fix only failures caused by this branch.

- [ ] **Step 4: Inspect final diff**

Run:

```bash
git status --short
git diff --stat origin/main...HEAD
git log --oneline --decorate -5
```

Expected: only auth-service, eval-mcp-service, and README files from this plan changed.

- [ ] **Step 5: Push feature branch**

Run:

```bash
git push -u origin feature/eval-mcp-auth-integration
```

Expected: branch pushes successfully.

- [ ] **Step 6: Open PR to qa**

Run:

```bash
gh pr create --base qa --head feature/eval-mcp-auth-integration --title "Authenticate eval MCP service with auth-service" --body "## Summary
- add opt-in auth-service token responses for machine clients
- add eval MCP auth client, token cache, and refresh/login provider
- retry eval API requests once after token recovery

## Verification
- go test ./... in go/auth-service
- go test ./... in go/eval-mcp-service
- make preflight-go"
```

Expected: PR is created targeting `qa`. Do not watch CI unless Kyle asks.
