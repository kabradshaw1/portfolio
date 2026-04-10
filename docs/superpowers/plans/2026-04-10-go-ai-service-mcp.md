# Go AI-Service MCP Adapter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add MCP server (exposing 9 tools via stdio + streamable HTTP) and MCP client (consuming external MCP servers) to the existing ai-service, using the official `modelcontextprotocol/go-sdk`.

**Architecture:** Embedded in the existing binary with two runtime modes — `serve` (HTTP + `/mcp` endpoint) and `mcp` (stdio). The `tools.Registry` is the single source of truth; the MCP server adapts it outward, the MCP client adapts external tools inward.

**Tech Stack:** Go, `github.com/modelcontextprotocol/go-sdk` v1.5.0+, Gin, existing `tools.Tool` / `tools.Registry` interfaces.

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `go/ai-service/internal/tools/registry.go` | Modify | Add `All() []Tool` to Registry interface + MemRegistry |
| `go/ai-service/internal/tools/registry_test.go` | Modify | Test `All()` |
| `go/ai-service/internal/mcp/server.go` | Create | MCP server adapter: wraps Registry tools as MCP tools |
| `go/ai-service/internal/mcp/server_test.go` | Create | Server adapter unit tests |
| `go/ai-service/internal/mcp/auth.go` | Create | Optional JWT middleware for streamable HTTP, context helpers |
| `go/ai-service/internal/mcp/auth_test.go` | Create | Auth middleware tests |
| `go/ai-service/internal/mcp/client.go` | Create | MCP client adapter: wraps MCP tools as tools.Tool |
| `go/ai-service/internal/mcp/client_test.go` | Create | Client adapter unit tests |
| `go/ai-service/cmd/server/main.go` | Modify | Subcommand routing (serve/mcp), wire MCP server + client |
| `go/ai-service/internal/evals/cases_test.go` | Modify | Add MCP round-trip eval case |
| `go/ai-service/go.mod` | Modify | Add go-sdk dependency |
| `docs/adr/go-ai-service-mcp.md` | Create | ADR documenting MCP decisions |

---

### Task 1: Add `All()` to Registry Interface

**Files:**
- Modify: `go/ai-service/internal/tools/registry.go`
- Modify: `go/ai-service/internal/tools/registry_test.go`

- [ ] **Step 1: Write the failing test**

Add to `go/ai-service/internal/tools/registry_test.go`:

```go
func TestMemRegistry_All(t *testing.T) {
	reg := NewMemRegistry()
	reg.Register(&fakeTool{name: "a", schema: json.RawMessage(`{"type":"object"}`)})
	reg.Register(&fakeTool{name: "b", schema: json.RawMessage(`{"type":"object"}`)})

	all := reg.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(all))
	}
	names := map[string]bool{}
	for _, tool := range all {
		names[tool.Name()] = true
	}
	if !names["a"] || !names["b"] {
		t.Errorf("missing tools: %v", names)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go/ai-service && go test ./internal/tools/ -run TestMemRegistry_All -v`
Expected: Compilation error — `reg.All undefined`

- [ ] **Step 3: Add `All()` to interface and implementation**

In `go/ai-service/internal/tools/registry.go`, add `All() []Tool` to the `Registry` interface:

```go
// Registry holds tool implementations keyed by name.
type Registry interface {
	Register(Tool)
	Get(name string) (Tool, bool)
	Schemas() []llm.ToolSchema
	All() []Tool
}
```

Add the implementation on `MemRegistry`:

```go
// All returns every registered tool.
func (r *MemRegistry) All() []Tool {
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd go/ai-service && go test ./internal/tools/ -run TestMemRegistry_All -v`
Expected: PASS

- [ ] **Step 5: Run full tools test suite**

Run: `cd go/ai-service && go test ./internal/tools/... -v`
Expected: All pass

- [ ] **Step 6: Commit**

```bash
cd go/ai-service && git add internal/tools/registry.go internal/tools/registry_test.go
git commit -m "feat(ai-service): add All() to Registry interface for MCP adapter"
```

---

### Task 2: Add go-sdk Dependency

**Files:**
- Modify: `go/ai-service/go.mod`

- [ ] **Step 1: Add the dependency**

Run: `cd go/ai-service && go get github.com/modelcontextprotocol/go-sdk@latest`

- [ ] **Step 2: Tidy**

Run: `cd go/ai-service && go mod tidy`

- [ ] **Step 3: Verify build**

Run: `cd go/ai-service && go build ./...`
Expected: Clean build

- [ ] **Step 4: Commit**

```bash
cd go/ai-service && git add go.mod go.sum
git commit -m "chore(ai-service): add modelcontextprotocol/go-sdk dependency"
```

---

### Task 3: MCP Server Adapter

**Files:**
- Create: `go/ai-service/internal/mcp/server.go`
- Create: `go/ai-service/internal/mcp/server_test.go`

- [ ] **Step 1: Write the failing test — tool discovery**

Create `go/ai-service/internal/mcp/server_test.go`:

```go
package mcp

import (
	"context"
	"encoding/json"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools"
)

// fakeTool is a minimal tools.Tool for testing.
type fakeTool struct {
	name   string
	desc   string
	schema json.RawMessage
	result tools.Result
	err    error
	calls  int
	seenID string
}

func (f *fakeTool) Name() string            { return f.name }
func (f *fakeTool) Description() string     { return f.desc }
func (f *fakeTool) Schema() json.RawMessage { return f.schema }
func (f *fakeTool) Call(ctx context.Context, args json.RawMessage, userID string) (tools.Result, error) {
	f.calls++
	f.seenID = userID
	return f.result, f.err
}

func TestNewServer_RegistersAllTools(t *testing.T) {
	reg := tools.NewMemRegistry()
	reg.Register(&fakeTool{
		name:   "search_products",
		desc:   "Search products",
		schema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
	})
	reg.Register(&fakeTool{
		name:   "get_product",
		desc:   "Get one product",
		schema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}}}`),
	})

	srv := NewServer(reg, Defaults{})

	// Use an in-process client to verify tools are registered.
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	ctx := context.Background()
	session, err := client.Connect(ctx, srv.InProcessTransport(), nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(result.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(result.Tools))
	}
	names := map[string]bool{}
	for _, tool := range result.Tools {
		names[tool.Name] = true
	}
	if !names["search_products"] || !names["get_product"] {
		t.Errorf("unexpected tools: %v", names)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go/ai-service && go test ./internal/mcp/ -run TestNewServer_RegistersAllTools -v`
Expected: Compilation error — package `mcp` not found

- [ ] **Step 3: Write the server adapter**

Create `go/ai-service/internal/mcp/server.go`:

```go
// Package mcp provides adapters between the ai-service's tools.Registry and
// the MCP protocol, using the official modelcontextprotocol/go-sdk.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/jwtctx"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools"
)

// Defaults holds fallback auth values for environments without per-request
// auth (stdio mode). In HTTP mode these are empty — the middleware injects
// per-request values instead.
type Defaults struct {
	UserID string // from AI_SERVICE_TOKEN JWT sub claim
	JWT    string // raw AI_SERVICE_TOKEN value, forwarded to ecommerce-service
}

// NewServer creates an MCP server that exposes every tool in reg.
func NewServer(reg tools.Registry, defaults Defaults) *sdkmcp.Server {
	srv := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    "ai-service",
		Version: "1.0.0",
	}, nil)

	for _, t := range reg.All() {
		registerTool(srv, t, defaults)
	}
	return srv
}

func registerTool(srv *sdkmcp.Server, t tools.Tool, defaults Defaults) {
	var inputSchema any
	_ = json.Unmarshal(t.Schema(), &inputSchema)

	srv.AddTool(
		&sdkmcp.Tool{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: inputSchema,
		},
		makeHandler(t, defaults),
	)
}

func makeHandler(t tools.Tool, defaults Defaults) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		args, err := json.Marshal(req.Arguments)
		if err != nil {
			return errorResult("bad arguments: " + err.Error()), nil
		}

		userID := UserIDFromContext(ctx)
		if userID == "" && defaults.UserID != "" {
			userID = defaults.UserID
			ctx = jwtctx.WithJWT(ctx, defaults.JWT)
		}

		result, err := t.Call(ctx, json.RawMessage(args), userID)
		if err != nil {
			return errorResult(err.Error()), nil
		}

		content, err := json.Marshal(result.Content)
		if err != nil {
			return errorResult("result not serializable: " + err.Error()), nil
		}

		return &sdkmcp.CallToolResult{
			Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: string(content)}},
		}, nil
	}
}

func errorResult(msg string) *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: msg}},
		IsError: true,
	}
}

// --- context helpers ---

type contextKey string

const userIDCtxKey contextKey = "mcp_user_id"

// WithUserID returns a context carrying the authenticated user's ID.
func WithUserID(ctx context.Context, uid string) context.Context {
	return context.WithValue(ctx, userIDCtxKey, uid)
}

// UserIDFromContext extracts the user ID set by WithUserID.
func UserIDFromContext(ctx context.Context) string {
	uid, _ := ctx.Value(userIDCtxKey).(string)
	return uid
}

// --- TextContent helper ---
// The SDK's Content is an interface. TextContent implements it.
// If the SDK provides a constructor, use it; otherwise the struct literal works
// because TextContent has an exported Text field and the required interface method.
// The exact shape may need minor adjustment when the dependency is first compiled.
```

**Note:** The `TextContent` struct literal (`&sdkmcp.TextContent{Text: ...}`) may need adjustment based on the exact SDK API. If the SDK uses a different constructor pattern (e.g., `sdkmcp.NewTextContent("...")`), update accordingly when the dependency compiles. The test in Step 4 will catch this.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd go/ai-service && go test ./internal/mcp/ -run TestNewServer_RegistersAllTools -v`
Expected: PASS

If compilation fails due to `TextContent` struct shape, check `go doc github.com/modelcontextprotocol/go-sdk/mcp.TextContent` and adjust the struct literal or use the correct constructor.

- [ ] **Step 5: Write test — tool call dispatch**

Add to `go/ai-service/internal/mcp/server_test.go`:

```go
func TestServer_CallTool_Success(t *testing.T) {
	ft := &fakeTool{
		name:   "get_product",
		desc:   "Get a product",
		schema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}}}`),
		result: tools.Result{Content: map[string]any{"id": "p1", "name": "Widget"}},
	}
	reg := tools.NewMemRegistry()
	reg.Register(ft)

	srv := NewServer(reg, Defaults{})
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	ctx := context.Background()
	session, err := client.Connect(ctx, srv.InProcessTransport(), nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "get_product",
		Arguments: map[string]any{"id": "p1"},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result")
	}
	if ft.calls != 1 {
		t.Errorf("expected 1 call, got %d", ft.calls)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content")
	}
}

func TestServer_CallTool_ToolError(t *testing.T) {
	ft := &fakeTool{
		name:   "get_order",
		desc:   "Get order",
		schema: json.RawMessage(`{"type":"object"}`),
		err:    fmt.Errorf("upstream 500"),
	}
	reg := tools.NewMemRegistry()
	reg.Register(ft)

	srv := NewServer(reg, Defaults{})
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	ctx := context.Background()
	session, err := client.Connect(ctx, srv.InProcessTransport(), nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "get_order",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result")
	}
}
```

- [ ] **Step 6: Run tests**

Run: `cd go/ai-service && go test ./internal/mcp/ -v`
Expected: All pass

- [ ] **Step 7: Write test — default userID (stdio mode)**

Add to `go/ai-service/internal/mcp/server_test.go`:

```go
func TestServer_DefaultUserID(t *testing.T) {
	ft := &fakeTool{
		name:   "list_orders",
		desc:   "List orders",
		schema: json.RawMessage(`{"type":"object"}`),
		result: tools.Result{Content: []string{"order1"}},
	}
	reg := tools.NewMemRegistry()
	reg.Register(ft)

	srv := NewServer(reg, Defaults{UserID: "user-42"})
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	ctx := context.Background()
	session, err := client.Connect(ctx, srv.InProcessTransport(), nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	_, err = session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "list_orders",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if ft.seenID != "user-42" {
		t.Errorf("expected userID 'user-42', got %q", ft.seenID)
	}
}
```

- [ ] **Step 8: Run tests**

Run: `cd go/ai-service && go test ./internal/mcp/ -v`
Expected: All pass

- [ ] **Step 9: Commit**

```bash
cd go/ai-service && git add internal/mcp/server.go internal/mcp/server_test.go
git commit -m "feat(ai-service): add MCP server adapter over tools.Registry"
```

---

### Task 4: MCP Server Auth Middleware

**Files:**
- Create: `go/ai-service/internal/mcp/auth.go`
- Create: `go/ai-service/internal/mcp/auth_test.go`

- [ ] **Step 1: Write the failing test**

Create `go/ai-service/internal/mcp/auth_test.go`:

```go
package mcp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/jwtctx"
)

func TestOptionalJWTMiddleware_WithValidToken(t *testing.T) {
	// Generate a valid JWT for testing.
	secret := "test-secret"
	token := testJWT(t, secret, "user-99")

	var capturedUserID string
	var capturedJWT string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUserID = UserIDFromContext(r.Context())
		capturedJWT = jwtctx.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := OptionalJWTMiddleware(secret)(inner)

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if capturedUserID != "user-99" {
		t.Errorf("expected userID 'user-99', got %q", capturedUserID)
	}
	if capturedJWT != token {
		t.Error("expected JWT in context")
	}
}

func TestOptionalJWTMiddleware_WithoutToken(t *testing.T) {
	var capturedUserID string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUserID = UserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := OptionalJWTMiddleware("secret")(inner)

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if capturedUserID != "" {
		t.Errorf("expected empty userID, got %q", capturedUserID)
	}
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestOptionalJWTMiddleware_WithInvalidToken(t *testing.T) {
	var capturedUserID string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUserID = UserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := OptionalJWTMiddleware("secret")(inner)

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Invalid token is silently ignored — request proceeds without auth.
	if capturedUserID != "" {
		t.Errorf("expected empty userID for invalid token, got %q", capturedUserID)
	}
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// testJWT creates a valid HS256 JWT with the given sub claim.
func testJWT(t *testing.T, secret, sub string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": sub})
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign JWT: %v", err)
	}
	return signed
}
```

Add the `jwt` import to the import block:
```go
import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v5"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/jwtctx"
)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go/ai-service && go test ./internal/mcp/ -run TestOptionalJWT -v`
Expected: Compilation error — `OptionalJWTMiddleware` undefined

- [ ] **Step 3: Implement the middleware**

Create `go/ai-service/internal/mcp/auth.go`:

```go
package mcp

import (
	"net/http"
	"strings"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/auth"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/jwtctx"
)

// OptionalJWTMiddleware extracts a Bearer token from the Authorization header,
// validates it, and injects the userID + raw JWT into the request context.
// If no token is present or the token is invalid, the request proceeds without
// auth — individual tools enforce their own auth requirements.
func OptionalJWTMiddleware(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authHeader := r.Header.Get("Authorization"); authHeader != "" {
				uid, err := auth.ParseBearer(authHeader, jwtSecret)
				if err == nil {
					ctx := WithUserID(r.Context(), uid)
					ctx = jwtctx.WithJWT(ctx, strings.TrimPrefix(authHeader, "Bearer "))
					r = r.WithContext(ctx)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
```

- [ ] **Step 4: Run tests**

Run: `cd go/ai-service && go test ./internal/mcp/ -run TestOptionalJWT -v`
Expected: All pass

- [ ] **Step 5: Commit**

```bash
cd go/ai-service && git add internal/mcp/auth.go internal/mcp/auth_test.go
git commit -m "feat(ai-service): add optional JWT middleware for MCP HTTP transport"
```

---

### Task 5: MCP Client Adapter

**Files:**
- Create: `go/ai-service/internal/mcp/client.go`
- Create: `go/ai-service/internal/mcp/client_test.go`

- [ ] **Step 1: Write the failing test — tool discovery and wrapping**

Create `go/ai-service/internal/mcp/client_test.go`:

```go
package mcp

import (
	"context"
	"encoding/json"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools"
)

func TestDiscoverTools_WrapsRemoteTools(t *testing.T) {
	// Stand up an in-process MCP server with one tool.
	reg := tools.NewMemRegistry()
	reg.Register(&fakeTool{
		name:   "search_products",
		desc:   "Search products",
		schema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
		result: tools.Result{Content: map[string]any{"products": []string{"p1"}}},
	})
	srv := NewServer(reg, Defaults{})

	// Connect a client to it.
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	ctx := context.Background()
	session, err := client.Connect(ctx, srv.InProcessTransport(), nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	// Discover tools with prefix "remote".
	discovered, err := DiscoverTools(ctx, session, "remote")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(discovered) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(discovered))
	}

	tool := discovered[0]
	if tool.Name() != "remote.search_products" {
		t.Errorf("expected 'remote.search_products', got %q", tool.Name())
	}
	if tool.Description() != "Search products" {
		t.Errorf("unexpected description: %q", tool.Description())
	}
	if len(tool.Schema()) == 0 {
		t.Error("expected non-empty schema")
	}
}

func TestMCPClientTool_Call_Success(t *testing.T) {
	reg := tools.NewMemRegistry()
	reg.Register(&fakeTool{
		name:   "get_product",
		desc:   "Get product",
		schema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}}}`),
		result: tools.Result{Content: map[string]any{"id": "p1", "name": "Widget"}},
	})
	srv := NewServer(reg, Defaults{})

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	ctx := context.Background()
	session, err := client.Connect(ctx, srv.InProcessTransport(), nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	discovered, err := DiscoverTools(ctx, session, "remote")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}

	result, err := discovered[0].Call(ctx, json.RawMessage(`{"id":"p1"}`), "")
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if result.Content == nil {
		t.Error("expected non-nil content")
	}
}

func TestMCPClientTool_Call_ToolError(t *testing.T) {
	reg := tools.NewMemRegistry()
	reg.Register(&fakeTool{
		name:   "bad_tool",
		desc:   "Always errors",
		schema: json.RawMessage(`{"type":"object"}`),
		err:    fmt.Errorf("boom"),
	})
	srv := NewServer(reg, Defaults{})

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	ctx := context.Background()
	session, err := client.Connect(ctx, srv.InProcessTransport(), nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	discovered, err := DiscoverTools(ctx, session, "remote")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}

	_, err = discovered[0].Call(ctx, json.RawMessage(`{}`), "")
	if err == nil {
		t.Fatal("expected error from failing tool")
	}
}
```

Add `"fmt"` to the import block.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go/ai-service && go test ./internal/mcp/ -run TestDiscoverTools -v`
Expected: Compilation error — `DiscoverTools` undefined

- [ ] **Step 3: Implement the client adapter**

Create `go/ai-service/internal/mcp/client.go`:

```go
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools"
)

// MCPClientTool wraps a tool discovered from an MCP server as a tools.Tool.
type MCPClientTool struct {
	prefixedName string
	description  string
	schema       json.RawMessage
	session      *sdkmcp.ClientSession
	remoteName   string
}

func (t *MCPClientTool) Name() string            { return t.prefixedName }
func (t *MCPClientTool) Description() string     { return t.description }
func (t *MCPClientTool) Schema() json.RawMessage { return t.schema }

func (t *MCPClientTool) Call(ctx context.Context, args json.RawMessage, userID string) (tools.Result, error) {
	var arguments map[string]any
	if err := json.Unmarshal(args, &arguments); err != nil {
		return tools.Result{}, fmt.Errorf("mcp client: bad args: %w", err)
	}

	result, err := t.session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      t.remoteName,
		Arguments: arguments,
	})
	if err != nil {
		return tools.Result{}, fmt.Errorf("mcp client: call %s: %w", t.remoteName, err)
	}

	if result.IsError {
		text := extractText(result)
		return tools.Result{}, fmt.Errorf("mcp tool error: %s", text)
	}

	text := extractText(result)
	var content any
	if err := json.Unmarshal([]byte(text), &content); err != nil {
		// Not JSON — return as plain string.
		return tools.Result{Content: text}, nil
	}
	return tools.Result{Content: content}, nil
}

// extractText returns the concatenated text content from a CallToolResult.
func extractText(r *sdkmcp.CallToolResult) string {
	var parts []string
	for _, c := range r.Content {
		if tc, ok := c.(*sdkmcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "")
}

// DiscoverTools connects to an MCP server session, lists all available tools,
// and returns them wrapped as tools.Tool. Each tool name is prefixed with
// serverName + "." to avoid collisions with local tools.
func DiscoverTools(ctx context.Context, session *sdkmcp.ClientSession, serverName string) ([]tools.Tool, error) {
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp discover %s: list tools: %w", serverName, err)
	}

	out := make([]tools.Tool, 0, len(result.Tools))
	for _, t := range result.Tools {
		schema, _ := json.Marshal(t.InputSchema)
		out = append(out, &MCPClientTool{
			prefixedName: serverName + "." + t.Name,
			description:  t.Description,
			schema:       schema,
			session:      session,
			remoteName:   t.Name,
		})
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests**

Run: `cd go/ai-service && go test ./internal/mcp/ -v`
Expected: All pass

- [ ] **Step 5: Commit**

```bash
cd go/ai-service && git add internal/mcp/client.go internal/mcp/client_test.go
git commit -m "feat(ai-service): add MCP client adapter wrapping remote tools as tools.Tool"
```

---

### Task 6: Subcommand Routing + MCP Wiring in main.go

**Files:**
- Modify: `go/ai-service/cmd/server/main.go`

- [ ] **Step 1: Refactor main() into subcommands**

Restructure `go/ai-service/cmd/server/main.go`. The current `main()` becomes `runServe()`. A new `runMCP()` function handles stdio mode. The new `main()` dispatches based on `os.Args`.

Replace the `func main()` definition (lines 30–31) and add subcommand dispatch at the top of main:

```go
func main() {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "serve":
		runServe()
	case "mcp":
		runMCP()
	default:
		log.Fatalf("unknown command: %s (use 'serve' or 'mcp')", cmd)
	}
}
```

Move the entire body of the current `main()` into a new `func runServe()`. Keep the `getenv` helper as-is.

- [ ] **Step 2: Add MCP server to runServe (streamable HTTP)**

At the end of `runServe()`, after the tool registry is built and before the HTTP server starts, add the MCP streamable HTTP endpoint:

```go
	// MCP streamable HTTP endpoint
	mcpSrv := mcpadapter.NewServer(registry, mcpadapter.Defaults{})
	mcpHandler := sdkmcp.NewStreamableHTTPHandler(func(_ *http.Request) *sdkmcp.Server {
		return mcpSrv
	}, &sdkmcp.StreamableHTTPOptions{Stateless: true})
	authedMCPHandler := mcpadapter.OptionalJWTMiddleware(jwtSecret)(mcpHandler)
	router.Any("/mcp", gin.WrapH(authedMCPHandler))
```

Add imports:

```go
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	mcpadapter "github.com/kabradshaw1/portfolio/go/ai-service/internal/mcp"
```

- [ ] **Step 3: Add MCP client wiring to runServe**

After mounting the MCP HTTP endpoint and before creating the Agent, add MCP client tool discovery:

```go
	// MCP client: discover and register tools from configured MCP servers.
	if mcpServersJSON := os.Getenv("MCP_SERVERS"); mcpServersJSON != "" {
		var servers []struct {
			Name      string `json:"name"`
			Transport string `json:"transport"`
			URL       string `json:"url"`
		}
		if err := json.Unmarshal([]byte(mcpServersJSON), &servers); err != nil {
			log.Fatalf("bad MCP_SERVERS: %v", err)
		}
		for _, s := range servers {
			if s.Transport != "http" {
				log.Fatalf("unsupported MCP transport %q for server %q (only 'http' supported in serve mode)", s.Transport, s.Name)
			}
			mcpClient := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "ai-service", Version: "1.0.0"}, nil)
			session, err := mcpClient.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: s.URL}, nil)
			if err != nil {
				slog.Warn("mcp server unreachable, skipping", "name", s.Name, "url", s.URL, "error", err)
				continue
			}
			discovered, err := mcpadapter.DiscoverTools(ctx, session, s.Name)
			if err != nil {
				slog.Warn("mcp tool discovery failed, skipping", "name", s.Name, "error", err)
				continue
			}
			for _, t := range discovered {
				registry.Register(t)
			}
			slog.Info("mcp tools registered", "server", s.Name, "count", len(discovered))
		}
	}
```

Add `"encoding/json"` to imports if not already present.

- [ ] **Step 4: Implement runMCP (stdio mode)**

Add to `go/ai-service/cmd/server/main.go`:

```go
func runMCP() {
	jwtSecret := os.Getenv("JWT_SECRET")
	ecommerceURL := getenv("ECOMMERCE_URL", "http://ecommerce-service:8092")

	ecomBreaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name:          "ai-ecommerce",
		OnStateChange: resilience.ObserveStateChange,
	})
	ecomClient := clients.NewEcommerceClient(ecommerceURL, ecomBreaker)

	registry := tools.NewMemRegistry()
	registry.Register(tools.NewSearchProductsTool(ecomClient))
	registry.Register(tools.NewGetProductTool(ecomClient))
	registry.Register(tools.NewCheckInventoryTool(ecomClient))
	registry.Register(tools.NewListOrdersTool(ecomClient))
	registry.Register(tools.NewGetOrderTool(ecomClient))
	registry.Register(tools.NewViewCartTool(ecomClient))
	registry.Register(tools.NewAddToCartTool(ecomClient))
	registry.Register(tools.NewInitiateReturnTool(ecomClient))
	// Note: summarize_orders is excluded — it requires an LLM client which
	// is not available in stdio mode. The 8 remaining tools are sufficient.

	// Parse JWT from env for user-scoped tools.
	var defaults mcpadapter.Defaults
	if token := os.Getenv("AI_SERVICE_TOKEN"); token != "" && jwtSecret != "" {
		uid, err := auth.ParseBearer("Bearer "+token, jwtSecret)
		if err != nil {
			log.Fatalf("AI_SERVICE_TOKEN invalid: %v", err)
		}
		defaults = mcpadapter.Defaults{UserID: uid, JWT: token}
		slog.Info("stdio mode: authenticated", "user_id", uid)
	}

	mcpSrv := mcpadapter.NewServer(registry, defaults)
	slog.Info("ai-service MCP server starting (stdio)")
	if err := mcpSrv.Run(context.Background(), &sdkmcp.StdioTransport{}); err != nil {
		log.Fatalf("mcp server: %v", err)
	}
}
```

- [ ] **Step 5: Verify build**

Run: `cd go/ai-service && go build ./cmd/server/`
Expected: Clean build

- [ ] **Step 6: Run all tests**

Run: `cd go/ai-service && go test ./... -v`
Expected: All pass

- [ ] **Step 7: Commit**

```bash
cd go/ai-service && git add cmd/server/main.go
git commit -m "feat(ai-service): add serve/mcp subcommands with MCP server + client wiring"
```

---

### Task 7: MCP Round-Trip Eval Case

**Files:**
- Modify: `go/ai-service/internal/evals/cases_test.go`

- [ ] **Step 1: Write the eval test**

Add to `go/ai-service/internal/evals/cases_test.go`:

```go
func TestEval_MCPRoundTrip(t *testing.T) {
	// Set up a local registry with one tool.
	innerTool := &EchoTool{
		ToolName: "search_products",
		Result:   tools.Result{Content: []map[string]any{{"id": "p1", "name": "Jacket"}}},
	}
	innerReg := tools.NewMemRegistry()
	innerReg.Register(innerTool)

	// Stand up an MCP server over this registry.
	mcpSrv := mcpadapter.NewServer(innerReg, mcpadapter.Defaults{UserID: "test-user"})

	// Connect an MCP client and discover tools.
	ctx := context.Background()
	sdkClient := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "eval", Version: "1.0.0"}, nil)
	session, err := sdkClient.Connect(ctx, mcpSrv.InProcessTransport(), nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	discovered, err := mcpadapter.DiscoverTools(ctx, session, "mcp")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}

	// Register MCP-wrapped tools into a fresh registry for the agent.
	agentReg := tools.NewMemRegistry()
	for _, d := range discovered {
		agentReg.Register(d)
	}

	// Script the LLM to call the MCP-prefixed tool, then give a final answer.
	scripted := &ScriptedLLM{Responses: []llm.ChatResponse{
		{ToolCalls: []llm.ToolCall{{
			ID:   "c1",
			Name: "mcp.search_products",
			Args: json.RawMessage(`{"query":"jacket"}`),
		}}},
		{Content: "Found a jacket for you."},
	}}

	events, err := Run(scripted, agentReg,
		agent.Turn{UserID: "user-1", Messages: []llm.Message{{Role: llm.RoleUser, Content: "find me a jacket"}}},
		8)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// The tool should have been called through the MCP round-trip.
	if innerTool.Calls != 1 {
		t.Errorf("expected 1 call to inner tool, got %d", innerTool.Calls)
	}
	if len(events) == 0 || events[len(events)-1].Final == nil {
		t.Errorf("expected final event, got %+v", events)
	}
}
```

Add imports to the import block:

```go
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	mcpadapter "github.com/kabradshaw1/portfolio/go/ai-service/internal/mcp"
```

- [ ] **Step 2: Run the eval test**

Run: `cd go/ai-service && go test -tags=eval ./internal/evals/ -run TestEval_MCPRoundTrip -v`
Expected: PASS

- [ ] **Step 3: Run all eval tests**

Run: `cd go/ai-service && go test -tags=eval ./internal/evals/ -v`
Expected: All pass

- [ ] **Step 4: Commit**

```bash
cd go/ai-service && git add internal/evals/cases_test.go
git commit -m "test(ai-service): add MCP round-trip eval case"
```

---

### Task 8: Preflight Check

- [ ] **Step 1: Run full preflight**

Run: `make preflight-go`
Expected: All lint + tests pass

- [ ] **Step 2: Fix any lint issues**

Address any golangci-lint findings. Common issues:
- Unused imports from restructuring
- Error return values not checked
- Line length

- [ ] **Step 3: Re-run preflight after fixes**

Run: `make preflight-go`
Expected: Clean

- [ ] **Step 4: Commit any lint fixes**

```bash
git add -A go/ai-service/
git commit -m "fix(ai-service): address lint findings from MCP adapter"
```

---

### Task 9: ADR Document

**Files:**
- Create: `docs/adr/go-ai-service-mcp.md`

- [ ] **Step 1: Write the ADR**

Create `docs/adr/go-ai-service-mcp.md`:

```markdown
# ADR: MCP Support in go/ai-service

**Date:** 2026-04-10
**Status:** Accepted

## Context

The `go/ai-service` v1 shipped with a `tools.Tool` interface and `tools.Registry` designed to be transport-agnostic. The comment in `registry.go` — "Tool is the only interface a future MCP adapter needs to satisfy" — made the intent explicit. MCP (Model Context Protocol) has become the emerging standard for tool interop between AI agents and tool providers.

## Decision

Add both an MCP server (exposing the 9 tools) and an MCP client (consuming external MCP servers) to the existing ai-service binary, using the official `modelcontextprotocol/go-sdk`.

### Why the official Go SDK

- v1.x with semver stability guarantees
- Maintained by the MCP org and Google
- GitHub's own MCP Server migrated from `mark3labs/mcp-go` to this SDK
- Single `mcp` package, idiomatic Go API

### Why embedded (not a separate binary)

One registry, one deploy, one test surface. The "transport-agnostic" story is literal when it's the same binary serving tools through both SSE (`/chat`) and MCP (`/mcp`).

### Why both transports (stdio + streamable HTTP)

- **Stdio** — required for Claude Desktop, Cursor, and most MCP hosts
- **Streamable HTTP** — enables network-accessible MCP server, needed for the agent's own MCP client to connect (K8s pod-to-pod)

### Why not full OAuth 2.1

The project already has JWT auth via auth-service. Adding a full OAuth 2.1 authorization server would be significant scope for zero portfolio signal. The MCP server accepts Bearer JWTs using the same validation logic as `/chat`.

## Alternatives Rejected

- **Separate MCP server binary** — doubles the deployment surface for no demo value
- **MCP-as-middleware** (all tool calls go through MCP) — over-engineered; adds JSON-RPC overhead to every local tool call
- **Python or TypeScript** — breaks the "same registry, two transports" Go portfolio story
- **`mark3labs/mcp-go`** — community library, pre-v1 (v0.47), no stability guarantees

## Consequences

- The ai-service binary gains a `mcp` subcommand for stdio mode
- The `/mcp` endpoint is automatically available on the existing HTTP port
- MCP client support is opt-in via `MCP_SERVERS` environment variable
- `summarize_orders` tool is excluded from stdio mode (requires LLM client)
```

- [ ] **Step 2: Commit**

```bash
git add docs/adr/go-ai-service-mcp.md
git commit -m "docs: add ADR for MCP support in go/ai-service"
```

---

## Verification

After all tasks are complete:

1. **Unit tests:** `cd go/ai-service && go test ./... -v` — all pass
2. **Eval tests:** `cd go/ai-service && go test -tags=eval ./internal/evals/ -v` — all pass (including MCP round-trip)
3. **Lint:** `make preflight-go` — clean
4. **Build:** `cd go/ai-service && go build ./cmd/server/` — produces binary
5. **Manual stdio test:** `echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}' | ./server mcp` — should return capabilities with tool list
6. **Manual HTTP test:** Start `./server serve`, then `curl -X POST http://localhost:8093/mcp` with MCP initialize request — should return capabilities
