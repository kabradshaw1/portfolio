# gRPC Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add auth-service gRPC with token revocation checking, enforce proto contract governance in CI, and standardize graceful shutdown across all Go services.

**Architecture:** Auth-service gets a gRPC server alongside its REST server. A shared `go/pkg/authmiddleware` calls `CheckToken` via gRPC with local caching. A shared `go/pkg/shutdown` manager replaces ad-hoc signal handling in all 6 services. `buf breaking` enforces proto governance in CI.

**Tech Stack:** Go, gRPC, protobuf/buf, Redis, OTel, Gin, Kubernetes

**Spec:** `docs/superpowers/specs/2026-04-21-grpc-hardening-design.md`

---

## File Structure

**New files:**
- `go/proto/auth/v1/auth.proto` — AuthService proto definition
- `go/buf.gen.auth.yaml` — buf generation config for auth-service
- `go/auth-service/pb/auth/v1/auth.pb.go` — generated (buf)
- `go/auth-service/pb/auth/v1/auth_grpc.pb.go` — generated (buf)
- `go/auth-service/internal/grpc/server.go` — gRPC handler for CheckToken
- `go/auth-service/internal/grpc/server_test.go` — unit tests
- `go/auth-service/authmiddleware/middleware.go` — shared Gin middleware with gRPC denylist check (lives in auth-service as a client package, not in go/pkg, to avoid circular deps)
- `go/auth-service/authmiddleware/middleware_test.go` — unit tests
- `go/pkg/shutdown/shutdown.go` — shared graceful shutdown manager
- `go/pkg/shutdown/shutdown_test.go` — unit tests

**Modified files:**
- `go/auth-service/cmd/server/main.go` — add gRPC server, use shutdown manager
- `go/auth-service/cmd/server/config.go` — add GRPC_PORT, REDIS_URL config
- `go/auth-service/go.mod` — add gRPC dependencies
- `go/auth-service/Dockerfile` — expose gRPC port 9091
- `go/product-service/cmd/server/main.go` — use shutdown manager
- `go/cart-service/cmd/server/main.go` — use shutdown manager
- `go/order-service/cmd/server/main.go` — use shutdown manager, shared authmiddleware
- `go/ai-service/cmd/server/main.go` — use shutdown manager
- `go/analytics-service/cmd/server/main.go` — use shutdown manager
- `go/order-service/cmd/server/routes.go` — shared authmiddleware
- `go/order-service/go.mod` — add replace for auth-service (proto + authmiddleware)
- `go/order-service/Dockerfile` — COPY auth-service for cross-module import
- `go/cart-service/cmd/server/routes.go` — shared authmiddleware
- `go/cart-service/go.mod` — add replace for auth-service (proto + authmiddleware)
- `go/cart-service/Dockerfile` — COPY auth-service for cross-module import
- `go/k8s/services/auth-service.yml` — add gRPC port
- `go/k8s/deployments/auth-service.yml` — add gRPC port, terminationGracePeriodSeconds
- `go/k8s/configmaps/auth-service-config.yml` — add GRPC_PORT, REDIS_URL
- All Go service deployments — add terminationGracePeriodSeconds: 20
- `.github/workflows/ci.yml` — add buf-breaking job

**Dependency note:** The authmiddleware lives at `go/auth-service/authmiddleware/` (public path, not `internal/`) so other services can import it. Services that use it (order-service, cart-service) add `replace github.com/kabradshaw1/portfolio/go/auth-service => ../auth-service` to their go.mod and `COPY auth-service/ /app/auth-service/` to their Dockerfile. This follows the same cross-module pattern already used for product-service proto imports. `go/pkg` remains free of auth-service dependencies.

---

### Task 1: Shutdown Manager (`go/pkg/shutdown`)

**Files:**
- Create: `go/pkg/shutdown/shutdown.go`
- Create: `go/pkg/shutdown/shutdown_test.go`

- [ ] **Step 1: Write the failing tests**

Create `go/pkg/shutdown/shutdown_test.go`:

```go
package shutdown

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunsInPriorityOrder(t *testing.T) {
	m := New(5 * time.Second)

	var order []int
	m.Register("third", 20, func(ctx context.Context) error {
		order = append(order, 20)
		return nil
	})
	m.Register("first", 0, func(ctx context.Context) error {
		order = append(order, 0)
		return nil
	})
	m.Register("second", 10, func(ctx context.Context) error {
		order = append(order, 10)
		return nil
	})

	m.runAll()

	if len(order) != 3 || order[0] != 0 || order[1] != 10 || order[2] != 20 {
		t.Fatalf("expected [0 10 20], got %v", order)
	}
}

func TestSamePriorityRunsConcurrently(t *testing.T) {
	m := New(5 * time.Second)

	var running atomic.Int32
	var maxConcurrent atomic.Int32

	for i := 0; i < 3; i++ {
		m.Register("concurrent", 10, func(ctx context.Context) error {
			n := running.Add(1)
			for {
				old := maxConcurrent.Load()
				if n <= old || maxConcurrent.CompareAndSwap(old, n) {
					break
				}
			}
			time.Sleep(50 * time.Millisecond)
			running.Add(-1)
			return nil
		})
	}

	m.runAll()

	if maxConcurrent.Load() < 2 {
		t.Fatalf("expected concurrent execution, max was %d", maxConcurrent.Load())
	}
}

func TestTimeoutCancelsContext(t *testing.T) {
	m := New(100 * time.Millisecond)

	var cancelled bool
	m.Register("slow", 0, func(ctx context.Context) error {
		<-ctx.Done()
		cancelled = true
		return nil
	})

	m.runAll()

	if !cancelled {
		t.Fatal("expected context to be cancelled after timeout")
	}
}

func TestErrorsAreLoggedNotFatal(t *testing.T) {
	m := New(5 * time.Second)

	m.Register("failing", 0, func(ctx context.Context) error {
		return context.DeadlineExceeded
	})
	m.Register("after-fail", 10, func(ctx context.Context) error {
		return nil
	})

	// Should not panic
	m.runAll()
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/pkg && go test ./shutdown/ -v -race`
Expected: FAIL — package does not exist

- [ ] **Step 3: Write the implementation**

Create `go/pkg/shutdown/shutdown.go`:

```go
package shutdown

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"
)

type hook struct {
	name     string
	priority int
	fn       func(ctx context.Context) error
}

// Manager orchestrates graceful shutdown in priority order.
type Manager struct {
	timeout time.Duration
	hooks   []hook
}

// New creates a Manager with the given overall shutdown timeout.
func New(timeout time.Duration) *Manager {
	return &Manager{timeout: timeout}
}

// Register adds a shutdown function. Lower priority values run first.
// Functions at the same priority run concurrently.
func (m *Manager) Register(name string, priority int, fn func(ctx context.Context) error) {
	m.hooks = append(m.hooks, hook{name: name, priority: priority, fn: fn})
}

// Wait blocks until SIGINT or SIGTERM is received, then runs all
// registered hooks in priority order within the timeout.
func (m *Manager) Wait() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutdown signal received")
	m.runAll()
	slog.Info("shutdown complete")
}

func (m *Manager) runAll() {
	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()

	sort.Slice(m.hooks, func(i, j int) bool {
		return m.hooks[i].priority < m.hooks[j].priority
	})

	groups := m.groupByPriority()
	for _, group := range groups {
		if ctx.Err() != nil {
			slog.Warn("shutdown timeout reached, skipping remaining hooks")
			return
		}
		m.runGroup(ctx, group)
	}
}

func (m *Manager) groupByPriority() [][]hook {
	if len(m.hooks) == 0 {
		return nil
	}
	var groups [][]hook
	currentPriority := m.hooks[0].priority
	var current []hook
	for _, h := range m.hooks {
		if h.priority != currentPriority {
			groups = append(groups, current)
			current = nil
			currentPriority = h.priority
		}
		current = append(current, h)
	}
	groups = append(groups, current)
	return groups
}

func (m *Manager) runGroup(ctx context.Context, group []hook) {
	var wg sync.WaitGroup
	for _, h := range group {
		wg.Add(1)
		go func(h hook) {
			defer wg.Done()
			if err := h.fn(ctx); err != nil {
				slog.Error("shutdown hook failed", "name", h.name, "error", err)
			} else {
				slog.Info("shutdown hook completed", "name", h.name)
			}
		}(h)
	}
	wg.Wait()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/pkg && go test ./shutdown/ -v -race`
Expected: PASS (all 4 tests)

- [ ] **Step 5: Commit**

```bash
git add go/pkg/shutdown/
git commit -m "feat(pkg): add shared graceful shutdown manager"
```

---

### Task 2: Auth Proto + Code Generation

**Files:**
- Create: `go/proto/auth/v1/auth.proto`
- Create: `go/buf.gen.auth.yaml`
- Generated: `go/auth-service/pb/auth/v1/auth.pb.go`
- Generated: `go/auth-service/pb/auth/v1/auth_grpc.pb.go`

- [ ] **Step 1: Create the proto definition**

Create `go/proto/auth/v1/auth.proto`:

```protobuf
syntax = "proto3";

package auth.v1;

option go_package = "github.com/kabradshaw1/portfolio/go/auth-service/pb/auth/v1";

service AuthService {
  rpc CheckToken(CheckTokenRequest) returns (CheckTokenResponse);
}

message CheckTokenRequest {
  string token = 1;
}

message CheckTokenResponse {
  bool valid = 1;
  string user_id = 2;
  string reason = 3;
}
```

- [ ] **Step 2: Create the buf generation config**

Create `go/buf.gen.auth.yaml`:

```yaml
version: v2
plugins:
  - remote: buf.build/protocolbuffers/go
    out: auth-service/pb
    opt: paths=source_relative
  - remote: buf.build/grpc/go
    out: auth-service/pb
    opt: paths=source_relative
```

- [ ] **Step 3: Lint the proto**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go && buf lint`
Expected: No errors

- [ ] **Step 4: Generate Go code**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go && buf generate --path proto/auth --template buf.gen.auth.yaml`
Expected: Creates `auth-service/pb/auth/v1/auth.pb.go` and `auth-service/pb/auth/v1/auth_grpc.pb.go`

- [ ] **Step 5: Verify generated files exist**

Run: `ls /Users/kylebradshaw/repos/gen_ai_engineer/go/auth-service/pb/auth/v1/`
Expected: `auth.pb.go  auth_grpc.pb.go`

- [ ] **Step 6: Commit**

```bash
git add go/proto/auth/v1/auth.proto go/buf.gen.auth.yaml go/auth-service/pb/
git commit -m "feat(auth): add AuthService proto with CheckToken RPC"
```

---

### Task 3: Auth gRPC Server Implementation

**Files:**
- Create: `go/auth-service/internal/grpc/server.go`
- Create: `go/auth-service/internal/grpc/server_test.go`
- Modify: `go/auth-service/go.mod`

- [ ] **Step 1: Add gRPC dependencies to auth-service**

Run:
```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/go/auth-service && \
go get google.golang.org/grpc && \
go get go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc && \
go get google.golang.org/grpc/health/grpc_health_v1 && \
go get google.golang.org/grpc/reflection && \
go mod tidy
```

- [ ] **Step 2: Write the failing tests**

Create `go/auth-service/internal/grpc/server_test.go`:

```go
package grpc

import (
	"context"
	"testing"

	pb "github.com/kabradshaw1/portfolio/go/auth-service/pb/auth/v1"
)

type fakeDenylist struct {
	revoked map[string]bool
}

func (f *fakeDenylist) IsRevoked(_ context.Context, token string) bool {
	return f.revoked[token]
}

func TestCheckToken_Valid(t *testing.T) {
	secret := "test-secret"
	token := createTestToken(t, secret, "user-123", false)
	srv := NewAuthGRPCServer(secret, &fakeDenylist{revoked: map[string]bool{}})

	resp, err := srv.CheckToken(context.Background(), &pb.CheckTokenRequest{Token: token})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Valid {
		t.Fatalf("expected valid=true, got false, reason=%s", resp.Reason)
	}
	if resp.UserId != "user-123" {
		t.Fatalf("expected user-123, got %s", resp.UserId)
	}
}

func TestCheckToken_Expired(t *testing.T) {
	secret := "test-secret"
	token := createTestToken(t, secret, "user-123", true)
	srv := NewAuthGRPCServer(secret, &fakeDenylist{revoked: map[string]bool{}})

	resp, err := srv.CheckToken(context.Background(), &pb.CheckTokenRequest{Token: token})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Valid {
		t.Fatal("expected valid=false for expired token")
	}
	if resp.Reason != "expired" {
		t.Fatalf("expected reason=expired, got %s", resp.Reason)
	}
}

func TestCheckToken_Revoked(t *testing.T) {
	secret := "test-secret"
	token := createTestToken(t, secret, "user-456", false)
	srv := NewAuthGRPCServer(secret, &fakeDenylist{revoked: map[string]bool{token: true}})

	resp, err := srv.CheckToken(context.Background(), &pb.CheckTokenRequest{Token: token})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Valid {
		t.Fatal("expected valid=false for revoked token")
	}
	if resp.Reason != "revoked" {
		t.Fatalf("expected reason=revoked, got %s", resp.Reason)
	}
}

func TestCheckToken_Malformed(t *testing.T) {
	srv := NewAuthGRPCServer("test-secret", &fakeDenylist{revoked: map[string]bool{}})

	resp, err := srv.CheckToken(context.Background(), &pb.CheckTokenRequest{Token: "not-a-jwt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Valid {
		t.Fatal("expected valid=false for malformed token")
	}
	if resp.Reason != "malformed" {
		t.Fatalf("expected reason=malformed, got %s", resp.Reason)
	}
}

func TestCheckToken_WrongSecret(t *testing.T) {
	token := createTestToken(t, "secret-A", "user-789", false)
	srv := NewAuthGRPCServer("secret-B", &fakeDenylist{revoked: map[string]bool{}})

	resp, err := srv.CheckToken(context.Background(), &pb.CheckTokenRequest{Token: token})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Valid {
		t.Fatal("expected valid=false for wrong-secret token")
	}
	if resp.Reason != "malformed" {
		t.Fatalf("expected reason=malformed, got %s", resp.Reason)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/auth-service && go test ./internal/grpc/ -v -race`
Expected: FAIL — `NewAuthGRPCServer` and `createTestToken` undefined

- [ ] **Step 4: Write the test helper**

Add to the bottom of `go/auth-service/internal/grpc/server_test.go`:

```go
// createTestToken mints a HS256 JWT for testing. If expired is true,
// the token's exp is set 1 hour in the past.
func createTestToken(t *testing.T, secret, userID string, expired bool) string {
	t.Helper()
	claims := jwtlib.MapClaims{
		"sub": userID,
		"iat": time.Now().Unix(),
	}
	if expired {
		claims["exp"] = time.Now().Add(-1 * time.Hour).Unix()
	} else {
		claims["exp"] = time.Now().Add(1 * time.Hour).Unix()
	}
	token := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}
```

Add imports to the test file:

```go
import (
	"context"
	"testing"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	pb "github.com/kabradshaw1/portfolio/go/auth-service/pb/auth/v1"
)
```

- [ ] **Step 5: Write the implementation**

Create `go/auth-service/internal/grpc/server.go`:

```go
package grpc

import (
	"context"

	jwtlib "github.com/golang-jwt/jwt/v5"
	pb "github.com/kabradshaw1/portfolio/go/auth-service/pb/auth/v1"
)

// Denylist checks whether a raw token string has been revoked.
type Denylist interface {
	IsRevoked(ctx context.Context, token string) bool
}

// AuthGRPCServer implements auth.v1.AuthService.
type AuthGRPCServer struct {
	pb.UnimplementedAuthServiceServer
	jwtSecret []byte
	denylist  Denylist
}

// NewAuthGRPCServer creates an AuthGRPCServer.
func NewAuthGRPCServer(jwtSecret string, denylist Denylist) *AuthGRPCServer {
	return &AuthGRPCServer{
		jwtSecret: []byte(jwtSecret),
		denylist:  denylist,
	}
}

// CheckToken validates a JWT and checks the Redis denylist.
func (s *AuthGRPCServer) CheckToken(ctx context.Context, req *pb.CheckTokenRequest) (*pb.CheckTokenResponse, error) {
	claims := jwtlib.MapClaims{}
	_, err := jwtlib.ParseWithClaims(req.Token, claims, func(t *jwtlib.Token) (any, error) {
		if _, ok := t.Method.(*jwtlib.SigningMethodHMAC); !ok {
			return nil, jwtlib.ErrSignatureInvalid
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		reason := "malformed"
		if isExpiredError(err) {
			reason = "expired"
		}
		return &pb.CheckTokenResponse{Valid: false, Reason: reason}, nil
	}

	sub, ok := claims["sub"].(string)
	if !ok || sub == "" {
		return &pb.CheckTokenResponse{Valid: false, Reason: "malformed"}, nil
	}

	if s.denylist.IsRevoked(ctx, req.Token) {
		return &pb.CheckTokenResponse{Valid: false, UserId: sub, Reason: "revoked"}, nil
	}

	return &pb.CheckTokenResponse{Valid: true, UserId: sub}, nil
}

func isExpiredError(err error) bool {
	// jwt/v5 wraps expiry as a validation error containing ErrTokenExpired.
	return jwtlib.ErrTokenExpired != nil && err.Error() != "" &&
		contains(err.Error(), "token is expired")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
```

Wait — the `isExpiredError` helper is overly manual. Use `errors.Is` from jwt/v5 instead. Replace with:

```go
package grpc

import (
	"context"
	"errors"

	jwtlib "github.com/golang-jwt/jwt/v5"
	pb "github.com/kabradshaw1/portfolio/go/auth-service/pb/auth/v1"
)

// Denylist checks whether a raw token string has been revoked.
type Denylist interface {
	IsRevoked(ctx context.Context, token string) bool
}

// AuthGRPCServer implements auth.v1.AuthService.
type AuthGRPCServer struct {
	pb.UnimplementedAuthServiceServer
	jwtSecret []byte
	denylist  Denylist
}

// NewAuthGRPCServer creates an AuthGRPCServer.
func NewAuthGRPCServer(jwtSecret string, denylist Denylist) *AuthGRPCServer {
	return &AuthGRPCServer{
		jwtSecret: []byte(jwtSecret),
		denylist:  denylist,
	}
}

// CheckToken validates a JWT and checks the Redis denylist.
func (s *AuthGRPCServer) CheckToken(ctx context.Context, req *pb.CheckTokenRequest) (*pb.CheckTokenResponse, error) {
	claims := jwtlib.MapClaims{}
	_, err := jwtlib.ParseWithClaims(req.Token, claims, func(t *jwtlib.Token) (any, error) {
		if _, ok := t.Method.(*jwtlib.SigningMethodHMAC); !ok {
			return nil, jwtlib.ErrSignatureInvalid
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		reason := "malformed"
		if errors.Is(err, jwtlib.ErrTokenExpired) {
			reason = "expired"
		}
		return &pb.CheckTokenResponse{Valid: false, Reason: reason}, nil
	}

	sub, ok := claims["sub"].(string)
	if !ok || sub == "" {
		return &pb.CheckTokenResponse{Valid: false, Reason: "malformed"}, nil
	}

	if s.denylist.IsRevoked(ctx, req.Token) {
		return &pb.CheckTokenResponse{Valid: false, UserId: sub, Reason: "revoked"}, nil
	}

	return &pb.CheckTokenResponse{Valid: true, UserId: sub}, nil
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/auth-service && go test ./internal/grpc/ -v -race`
Expected: PASS (all 5 tests)

- [ ] **Step 7: Commit**

```bash
git add go/auth-service/internal/grpc/ go/auth-service/go.mod go/auth-service/go.sum
git commit -m "feat(auth): implement CheckToken gRPC handler with denylist"
```

---

### Task 4: Wire gRPC Server into auth-service main.go

**Files:**
- Modify: `go/auth-service/cmd/server/main.go`
- Modify: `go/auth-service/cmd/server/config.go`
- Modify: `go/auth-service/Dockerfile`

- [ ] **Step 1: Add GRPC_PORT and REDIS_URL to config**

In `go/auth-service/cmd/server/config.go`, add `GRPCPort` and `RedisURL` fields to the `Config` struct and `loadConfig()` function. `GRPCPort` defaults to `"9091"`. `RedisURL` should already exist in the struct — verify it's loaded and used.

Add to the Config struct:
```go
GRPCPort string
```

Add to `loadConfig()`:
```go
GRPCPort: getEnv("GRPC_PORT", "9091"),
```

- [ ] **Step 2: Wire gRPC server into main.go**

Replace the signal handling block in `go/auth-service/cmd/server/main.go` (lines 66-82) with the shutdown manager. Add the gRPC server startup. The main.go should now:

1. Import the new packages:
```go
import (
	"net"

	authgrpc "github.com/kabradshaw1/portfolio/go/auth-service/internal/grpc"
	pb "github.com/kabradshaw1/portfolio/go/auth-service/pb/auth/v1"
	"github.com/kabradshaw1/portfolio/go/pkg/shutdown"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
)
```

2. Create the gRPC server after the existing router setup (after line ~57):
```go
// gRPC server
grpcServer := grpc.NewServer(
	grpc.StatsHandler(otelgrpc.NewServerHandler()),
)
pb.RegisterAuthServiceServer(grpcServer, authgrpc.NewAuthGRPCServer(cfg.JWTSecret, denylist))

healthSrv := health.NewServer()
healthpb.RegisterHealthServer(grpcServer, healthSrv)
healthSrv.SetServingStatus("auth.v1.AuthService", healthpb.HealthCheckResponse_SERVING)

reflection.Register(grpcServer)

lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
if err != nil {
	log.Fatalf("gRPC listen: %v", err)
}

go func() {
	slog.Info("gRPC server starting", "port", cfg.GRPCPort)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("gRPC server failed: %v", err)
	}
}()
```

3. Replace the signal handling (lines 66-82) with the shutdown manager:
```go
sm := shutdown.New(15 * time.Second)
sm.Register("grpc-drain", 10, func(ctx context.Context) error {
	grpcServer.GracefulStop()
	return nil
})
sm.Register("http", 20, func(ctx context.Context) error {
	return srv.Shutdown(ctx)
})
sm.Register("redis", 20, func(ctx context.Context) error {
	if redisClient != nil {
		return redisClient.Close()
	}
	return nil
})
sm.Register("otel", 30, func(ctx context.Context) error {
	return shutdownTracer(ctx)
})
sm.Wait()
```

Remove the existing `defer func() { _ = shutdownTracer(ctx) }()` since the shutdown manager handles it now.

- [ ] **Step 3: Update Dockerfile to expose gRPC port**

In `go/auth-service/Dockerfile`, change line 26 from:
```dockerfile
EXPOSE 8091
```
to:
```dockerfile
EXPOSE 8091 9091
```

- [ ] **Step 4: Run the existing auth-service tests**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/auth-service && go test ./... -v -race`
Expected: PASS

- [ ] **Step 5: Run linter**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/auth-service && golangci-lint run ./...`
Expected: No new errors

- [ ] **Step 6: Commit**

```bash
git add go/auth-service/cmd/server/ go/auth-service/Dockerfile go/auth-service/go.mod go/auth-service/go.sum
git commit -m "feat(auth): wire gRPC server with CheckToken into main"
```

---

### Task 5: Shared Auth Middleware (`go/auth-service/authmiddleware`)

The middleware lives in `go/auth-service/authmiddleware/` (public, not `internal/`) so other services can import it alongside the proto package. This avoids circular deps through `go/pkg`.

**Files:**
- Create: `go/auth-service/authmiddleware/middleware.go`
- Create: `go/auth-service/authmiddleware/middleware_test.go`

- [ ] **Step 1: Write the failing tests**

Create `go/auth-service/authmiddleware/middleware_test.go`:

```go
package authmiddleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	jwtlib "github.com/golang-jwt/jwt/v5"
	pb "github.com/kabradshaw1/portfolio/go/auth-service/pb/auth/v1"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	"google.golang.org/grpc"
)

// Note: this package is at go/auth-service/authmiddleware/ (not internal/),
// so it can import sibling packages within the auth-service module directly.

func init() { gin.SetMode(gin.TestMode) }

type fakeAuthClient struct {
	resp *pb.CheckTokenResponse
	err  error
}

func (f *fakeAuthClient) CheckToken(_ context.Context, req *pb.CheckTokenRequest, _ ...grpc.CallOption) (*pb.CheckTokenResponse, error) {
	return f.resp, f.err
}

func signToken(t *testing.T, secret, userID string) string {
	t.Helper()
	claims := jwtlib.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	tok := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func newTestRouter(mw gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	r.Use(apperror.ErrorHandler())
	r.Use(mw)
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"userId": c.GetString("userId")})
	})
	return r
}

func TestValidToken_Allowed(t *testing.T) {
	secret := "test-secret"
	token := signToken(t, secret, "user-1")
	client := &fakeAuthClient{resp: &pb.CheckTokenResponse{Valid: true, UserId: "user-1"}}
	mw := New(secret, client)
	r := newTestRouter(mw)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMissingToken_Returns401(t *testing.T) {
	client := &fakeAuthClient{}
	mw := New("secret", client)
	r := newTestRouter(mw)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestRevokedToken_Returns401(t *testing.T) {
	secret := "test-secret"
	token := signToken(t, secret, "user-2")
	client := &fakeAuthClient{resp: &pb.CheckTokenResponse{Valid: false, Reason: "revoked"}}
	mw := New(secret, client)
	r := newTestRouter(mw)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCookieToken_Allowed(t *testing.T) {
	secret := "test-secret"
	token := signToken(t, secret, "user-3")
	client := &fakeAuthClient{resp: &pb.CheckTokenResponse{Valid: true, UserId: "user-3"}}
	mw := New(secret, client)
	r := newTestRouter(mw)

	req := httptest.NewRequest("GET", "/test", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: token})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCacheHit_SkipsDenylistCall(t *testing.T) {
	secret := "test-secret"
	token := signToken(t, secret, "user-4")

	callCount := 0
	client := &fakeAuthClient{resp: &pb.CheckTokenResponse{Valid: true, UserId: "user-4"}}
	// Override CheckToken to count calls
	countingClient := &countingAuthClient{inner: client, count: &callCount}
	mw := New(secret, countingClient, WithCacheTTL(5*time.Second))
	r := newTestRouter(mw)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("request %d: expected 200, got %d", i, w.Code)
		}
	}

	if callCount != 1 {
		t.Fatalf("expected 1 denylist call (cached), got %d", callCount)
	}
}

func TestSkipPaths(t *testing.T) {
	client := &fakeAuthClient{}
	mw := New("secret", client, WithSkipPaths("/health", "/metrics"))

	r := gin.New()
	r.Use(mw)
	r.GET("/health", func(c *gin.Context) { c.String(200, "ok") })

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200 for skipped path, got %d", w.Code)
	}
}

type countingAuthClient struct {
	inner AuthChecker
	count *int
}

func (c *countingAuthClient) CheckToken(ctx context.Context, req *pb.CheckTokenRequest, opts ...grpc.CallOption) (*pb.CheckTokenResponse, error) {
	*c.count++
	return c.inner.CheckToken(ctx, req, opts...)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/auth-service && go test ./authmiddleware/ -v -race`
Expected: FAIL — package does not exist

- [ ] **Step 3: Write the implementation**

Create `go/auth-service/authmiddleware/middleware.go`:

```go
package authmiddleware

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	jwtlib "github.com/golang-jwt/jwt/v5"
	pb "github.com/kabradshaw1/portfolio/go/auth-service/pb/auth/v1"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	"google.golang.org/grpc"
)

// AuthChecker is the subset of the generated AuthServiceClient we need.
type AuthChecker interface {
	CheckToken(ctx context.Context, in *pb.CheckTokenRequest, opts ...grpc.CallOption) (*pb.CheckTokenResponse, error)
}

type cacheEntry struct {
	resp      *pb.CheckTokenResponse
	expiresAt time.Time
}

type options struct {
	cacheTTL  time.Duration
	skipPaths map[string]bool
}

// Option configures the middleware.
type Option func(*options)

// WithCacheTTL sets how long denylist responses are cached per token.
func WithCacheTTL(d time.Duration) Option {
	return func(o *options) { o.cacheTTL = d }
}

// WithSkipPaths sets paths that bypass auth entirely.
func WithSkipPaths(paths ...string) Option {
	return func(o *options) {
		for _, p := range paths {
			o.skipPaths[p] = true
		}
	}
}

// New creates Gin middleware that validates JWTs locally and checks the
// auth-service denylist via gRPC, caching results.
func New(jwtSecret string, checker AuthChecker, opts ...Option) gin.HandlerFunc {
	cfg := &options{
		cacheTTL:  30 * time.Second,
		skipPaths: make(map[string]bool),
	}
	for _, o := range opts {
		o(cfg)
	}

	var mu sync.RWMutex
	cache := make(map[string]cacheEntry)

	return func(c *gin.Context) {
		if cfg.skipPaths[c.Request.URL.Path] {
			c.Next()
			return
		}

		tokenStr := extractToken(c)
		if tokenStr == "" {
			_ = c.Error(apperror.Unauthorized("MISSING_AUTH", "missing authorization"))
			c.Abort()
			return
		}

		// Fast path: validate signature + expiry locally.
		claims := jwtlib.MapClaims{}
		_, err := jwtlib.ParseWithClaims(tokenStr, claims, func(t *jwtlib.Token) (any, error) {
			if _, ok := t.Method.(*jwtlib.SigningMethodHMAC); !ok {
				return nil, jwtlib.ErrSignatureInvalid
			}
			return []byte(jwtSecret), nil
		})
		if err != nil {
			_ = c.Error(apperror.Unauthorized("INVALID_TOKEN", "invalid or expired token"))
			c.Abort()
			return
		}

		sub, ok := claims["sub"].(string)
		if !ok || sub == "" {
			_ = c.Error(apperror.Unauthorized("INVALID_TOKEN", "invalid token claims"))
			c.Abort()
			return
		}

		// Check denylist with cache.
		if resp, hit := getCached(&mu, cache, tokenStr); hit {
			if !resp.Valid {
				_ = c.Error(apperror.Unauthorized("TOKEN_REVOKED", resp.Reason))
				c.Abort()
				return
			}
		} else {
			resp, err := checker.CheckToken(c.Request.Context(), &pb.CheckTokenRequest{Token: tokenStr})
			if err == nil {
				putCached(&mu, cache, tokenStr, resp, cfg.cacheTTL)
				if !resp.Valid {
					_ = c.Error(apperror.Unauthorized("TOKEN_REVOKED", resp.Reason))
					c.Abort()
					return
				}
			}
			// On gRPC error, fail open — local validation already passed.
		}

		c.Set("userId", sub)
		c.Next()
	}
}

func extractToken(c *gin.Context) string {
	if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	if cookie, err := c.Cookie("access_token"); err == nil && cookie != "" {
		return cookie
	}
	return ""
}

func getCached(mu *sync.RWMutex, cache map[string]cacheEntry, key string) (*pb.CheckTokenResponse, bool) {
	mu.RLock()
	defer mu.RUnlock()
	entry, ok := cache[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.resp, true
}

func putCached(mu *sync.RWMutex, cache map[string]cacheEntry, key string, resp *pb.CheckTokenResponse, ttl time.Duration) {
	mu.Lock()
	defer mu.Unlock()
	cache[key] = cacheEntry{resp: resp, expiresAt: time.Now().Add(ttl)}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/auth-service && go test ./authmiddleware/ -v -race`
Expected: PASS (all 6 tests)

- [ ] **Step 5: Run linter**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/auth-service && golangci-lint run ./...`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
git add go/auth-service/authmiddleware/
git commit -m "feat(auth): add shared auth middleware with gRPC denylist check and cache"
```

---

### Task 6: Adopt Shutdown Manager in All Services

**Files:**
- Modify: `go/product-service/cmd/server/main.go`
- Modify: `go/cart-service/cmd/server/main.go`
- Modify: `go/order-service/cmd/server/main.go`
- Modify: `go/ai-service/cmd/server/main.go`
- Modify: `go/analytics-service/cmd/server/main.go`

Each service replaces its ad-hoc signal handling with the shutdown manager. Auth-service was already done in Task 4.

- [ ] **Step 1: Update product-service**

In `go/product-service/cmd/server/main.go`, replace the shutdown block (lines 106-121) with:

```go
sm := shutdown.New(15 * time.Second)
sm.Register("grpc-drain", 10, func(ctx context.Context) error {
	grpcServer.GracefulStop()
	return nil
})
sm.Register("http", 20, func(ctx context.Context) error {
	return httpSrv.Shutdown(ctx)
})
sm.Register("otel", 30, func(ctx context.Context) error {
	return shutdownTracer(ctx)
})
sm.Wait()
```

Remove the existing signal handling code (`quit` channel, `signal.Notify`, etc.) and the deferred `shutdownTracer` call. Keep the `cancel()` call in the shutdown — add it as priority 0:

```go
sm.Register("cancel-ctx", 0, func(_ context.Context) error {
	cancel()
	return nil
})
```

Add import: `"github.com/kabradshaw1/portfolio/go/pkg/shutdown"`

- [ ] **Step 2: Update cart-service**

In `go/cart-service/cmd/server/main.go`, replace the shutdown block (lines 144-160) with:

```go
sm := shutdown.New(15 * time.Second)
sm.Register("cancel-ctx", 0, func(_ context.Context) error {
	cancel()
	return nil
})
sm.Register("grpc-drain", 10, func(ctx context.Context) error {
	grpcServer.GracefulStop()
	return nil
})
sm.Register("http", 20, func(ctx context.Context) error {
	return httpSrv.Shutdown(ctx)
})
sm.Register("otel", 30, func(ctx context.Context) error {
	return shutdownTracer(ctx)
})
sm.Wait()
```

Remove existing signal handling and deferred `shutdownTracer`.

- [ ] **Step 3: Update order-service**

In `go/order-service/cmd/server/main.go`, replace the shutdown block (lines 125-138) with:

```go
sm := shutdown.New(15 * time.Second)
sm.Register("cancel-ctx", 0, func(_ context.Context) error {
	cancel()
	return nil
})
sm.Register("http", 20, func(ctx context.Context) error {
	return srv.Shutdown(ctx)
})
sm.Register("otel", 30, func(ctx context.Context) error {
	return shutdownTracer(ctx)
})
sm.Wait()
```

Remove existing signal handling and deferred `shutdownTracer`.

- [ ] **Step 4: Update ai-service**

In `go/ai-service/cmd/server/main.go`, in the `runServe()` function, replace the shutdown block (lines 165-175) with:

```go
sm := shutdown.New(15 * time.Second)
sm.Register("http", 20, func(ctx context.Context) error {
	return srv.Shutdown(ctx)
})
sm.Register("otel", 30, func(sctx context.Context) error {
	return shutdownTracer(sctx)
})
sm.Wait()
```

Remove existing signal handling. Note: ai-service's `runServe()` creates its own context — verify the variable names match. The `shutdownTracer` variable comes from the tracing init earlier in the function.

- [ ] **Step 5: Update analytics-service**

In `go/analytics-service/cmd/server/main.go`, replace the shutdown block (lines 66-84) with:

```go
sm := shutdown.New(15 * time.Second)
sm.Register("cancel-ctx", 0, func(_ context.Context) error {
	cancel()
	return nil
})
sm.Register("kafka-close", 10, func(_ context.Context) error {
	return cons.Close()
})
sm.Register("http", 20, func(ctx context.Context) error {
	return srv.Shutdown(ctx)
})
sm.Register("otel", 30, func(ctx context.Context) error {
	return shutdownTracer(ctx)
})
sm.Wait()
```

Remove existing signal handling and deferred `shutdownTracer`.

- [ ] **Step 6: Tidy all modules**

Run:
```bash
for svc in auth-service product-service cart-service order-service ai-service analytics-service; do
  cd /Users/kylebradshaw/repos/gen_ai_engineer/go/$svc && go mod tidy
done
```

- [ ] **Step 7: Run all Go tests**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer && make preflight-go`
Expected: All services lint and test pass

- [ ] **Step 8: Commit**

```bash
git add go/product-service/cmd/server/main.go \
  go/cart-service/cmd/server/main.go \
  go/order-service/cmd/server/main.go \
  go/ai-service/cmd/server/main.go \
  go/analytics-service/cmd/server/main.go \
  go/*/go.mod go/*/go.sum
git commit -m "refactor(go): adopt shared shutdown manager in all services"
```

---

### Task 7: Replace Service Auth Middleware with Shared Middleware

**Files:**
- Modify: `go/order-service/cmd/server/routes.go`
- Modify: `go/order-service/cmd/server/main.go`
- Modify: `go/order-service/cmd/server/config.go`
- Modify: `go/cart-service/cmd/server/routes.go`
- Modify: `go/cart-service/cmd/server/main.go`
- Modify: `go/cart-service/cmd/server/config.go`

- [ ] **Step 1: Update order-service main.go to create gRPC client**

In `go/order-service/cmd/server/main.go`, add after the existing dependency wiring:

```go
// Auth-service gRPC connection for denylist checks.
authConn, err := grpc.NewClient(cfg.AuthGRPCURL,
	grpc.WithTransportCredentials(insecure.NewCredentials()),
)
if err != nil {
	log.Fatalf("auth gRPC dial: %v", err)
}
defer authConn.Close()
authClient := authpb.NewAuthServiceClient(authConn)
```

Add imports:
```go
authpb "github.com/kabradshaw1/portfolio/go/auth-service/pb/auth/v1"
"github.com/kabradshaw1/portfolio/go/auth-service/authmiddleware"
"google.golang.org/grpc"
"google.golang.org/grpc/credentials/insecure"
```

Add `AuthGRPCURL` to order-service config:
```go
AuthGRPCURL string
```
In `loadConfig()`:
```go
AuthGRPCURL: getEnv("AUTH_GRPC_URL", "localhost:9091"),
```

- [ ] **Step 2: Update order-service routes.go**

In `go/order-service/cmd/server/routes.go`, change the `setupRouter` function signature to accept `authMw gin.HandlerFunc` instead of using the internal middleware. Replace:

```go
auth.Use(middleware.Auth(cfg.JWTSecret))
```

with:

```go
auth.Use(authMw)
```

Pass the middleware from main.go:
```go
authMw := authmiddleware.New(cfg.JWTSecret, authClient)
router := setupRouter(cfg, orderHandler, returnHandler, healthHandler, ecomLimiter, redisClient, authMw)
```

Update `setupRouter` signature accordingly.

- [ ] **Step 3: Update cart-service identically**

Same changes as order-service:
1. Add `AuthGRPCURL` to cart-service config (default `"localhost:9091"`)
2. Create gRPC client connection in main.go
3. Create `authmiddleware.New(cfg.JWTSecret, authClient)` 
4. Update `setupRouter` to accept and use the shared middleware
5. Replace `middleware.Auth(cfg.JWTSecret)` with the shared middleware

- [ ] **Step 4: Add replace directives for auth-service pb import**

Each service that uses the shared authmiddleware transitively imports the auth-service pb package through `go/pkg`. Add replace directive to both service go.mod files:

```
replace github.com/kabradshaw1/portfolio/go/auth-service => ../auth-service
```

Then copy the auth-service directory in each service's Dockerfile:
```dockerfile
COPY auth-service/ /app/auth-service/
```

Add this COPY line after the existing `COPY pkg/ /app/pkg/` line in both order-service and cart-service Dockerfiles.

- [ ] **Step 5: Tidy modules**

Run:
```bash
for svc in order-service cart-service; do
  cd /Users/kylebradshaw/repos/gen_ai_engineer/go/$svc && go mod tidy
done
```

- [ ] **Step 6: Run tests**

Run:
```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/go/order-service && go test ./... -v -race
cd /Users/kylebradshaw/repos/gen_ai_engineer/go/cart-service && go test ./... -v -race
```
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add go/order-service/ go/cart-service/
git commit -m "refactor(order,cart): replace local auth middleware with shared authmiddleware"
```

---

### Task 8: Kubernetes Manifests

**Files:**
- Modify: `go/k8s/services/auth-service.yml`
- Modify: `go/k8s/deployments/auth-service.yml`
- Modify: `go/k8s/configmaps/auth-service-config.yml`
- Modify: All Go service deployments (terminationGracePeriodSeconds)
- Modify: `go/k8s/configmaps/order-service-config.yml`
- Modify: `go/k8s/configmaps/cart-service-config.yml`

- [ ] **Step 1: Update auth-service K8s service**

Replace `go/k8s/services/auth-service.yml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: go-auth-service
  namespace: go-ecommerce
spec:
  selector:
    app: go-auth-service
  ports:
    - name: http
      port: 8091
      targetPort: 8091
    - name: grpc
      port: 9091
      targetPort: 9091
```

- [ ] **Step 2: Update auth-service deployment**

In `go/k8s/deployments/auth-service.yml`, add the gRPC port and terminationGracePeriodSeconds:

Add under `ports:`:
```yaml
          ports:
            - containerPort: 8091
              name: http
            - containerPort: 9091
              name: grpc
```

Add under `spec:` (at the pod spec level, same level as `containers:`):
```yaml
      terminationGracePeriodSeconds: 20
```

- [ ] **Step 3: Update auth-service configmap**

Add to `go/k8s/configmaps/auth-service-config.yml`:
```yaml
  GRPC_PORT: "9091"
  REDIS_URL: redis://redis.java-tasks.svc.cluster.local:6379
```

- [ ] **Step 4: Add AUTH_GRPC_URL to order-service and cart-service configmaps**

In `go/k8s/configmaps/order-service-config.yml`, add:
```yaml
  AUTH_GRPC_URL: go-auth-service.go-ecommerce.svc.cluster.local:9091
```

In `go/k8s/configmaps/cart-service-config.yml`, add:
```yaml
  AUTH_GRPC_URL: go-auth-service.go-ecommerce.svc.cluster.local:9091
```

- [ ] **Step 5: Add terminationGracePeriodSeconds to all Go deployments**

Add `terminationGracePeriodSeconds: 20` to the pod spec in each deployment file:
- `go/k8s/deployments/product-service.yml`
- `go/k8s/deployments/cart-service.yml`
- `go/k8s/deployments/order-service.yml`
- `go/k8s/deployments/ai-service.yml`
- `go/k8s/deployments/analytics-service.yml`

Add at the same level as `containers:` under `spec.template.spec:`:
```yaml
      terminationGracePeriodSeconds: 20
```

- [ ] **Step 6: Commit**

```bash
git add go/k8s/
git commit -m "feat(k8s): add auth gRPC port, AUTH_GRPC_URL, terminationGracePeriodSeconds"
```

---

### Task 9: `buf breaking` in CI

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Add buf-breaking job**

In `.github/workflows/ci.yml`, add a new job after the `go-tests` job (after line ~207):

```yaml
  buf-breaking:
    name: Proto Breaking Change Check
    runs-on: ubuntu-latest
    if: github.event_name == 'pull_request'
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Check for proto changes
        id: proto-changes
        run: |
          if git diff --name-only origin/${{ github.base_ref }}...HEAD | grep -q '^go/proto/'; then
            echo "changed=true" >> "$GITHUB_OUTPUT"
          else
            echo "changed=false" >> "$GITHUB_OUTPUT"
          fi

      - uses: bufbuild/buf-setup-action@v1
        if: steps.proto-changes.outputs.changed == 'true'

      - name: Check for breaking changes
        if: steps.proto-changes.outputs.changed == 'true'
        working-directory: go
        run: buf breaking proto --against '../.git#branch=origin/${{ github.base_ref }},subdir=go/proto'
```

- [ ] **Step 2: Add buf-breaking to the required checks**

If there's a `needs:` list for the deploy or gate job, add `buf-breaking` to it. Check the existing job dependency chain — the quality gate for PRs to qa likely lists `go-lint` and `go-tests`. Add `buf-breaking` alongside them.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add buf breaking change detection for proto files"
```

---

### Task 10: QA Kustomize Overlay Updates

**Files:**
- Modify: `k8s/overlays/qa-go/kustomization.yaml` (or equivalent QA overlay)

- [ ] **Step 1: Update QA configmap patches**

Add `AUTH_GRPC_URL` pointing to the QA auth-service in the QA overlay ConfigMap patches for order-service and cart-service. The QA services use the same namespace so the URL is the same.

Check the QA overlay file to verify the exact format, then add:
```yaml
  AUTH_GRPC_URL: go-auth-service.go-ecommerce-qa.svc.cluster.local:9091
```

Also add `REDIS_URL` and `GRPC_PORT` to the QA auth-service ConfigMap patch.

- [ ] **Step 2: Commit**

```bash
git add k8s/overlays/
git commit -m "feat(k8s): update QA overlays for auth gRPC"
```

---

### Task 11: Final Verification

- [ ] **Step 1: Run full Go preflight**

Run: `make preflight-go`
Expected: All lint + tests pass across all 6 services

- [ ] **Step 2: Run security preflight**

Run: `make preflight-security`
Expected: No new security issues

- [ ] **Step 3: Verify proto generation is clean**

Run:
```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/go && \
buf lint && \
buf generate --path proto/auth --template buf.gen.auth.yaml && \
buf generate --path proto/product --template buf.gen.product.yaml && \
buf generate --path proto/cart --template buf.gen.cart.yaml
```

Verify no diff in generated files:
```bash
git diff go/*/pb/
```
Expected: No changes (generated files already committed match)

- [ ] **Step 4: Verify Docker builds**

Run:
```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/go && \
docker build -f auth-service/Dockerfile -t test-auth . && \
docker build -f order-service/Dockerfile -t test-order . && \
docker build -f cart-service/Dockerfile -t test-cart .
```
Expected: All three build successfully

- [ ] **Step 5: Final commit if any fixes needed**

If any step above required fixes, commit them:
```bash
git add -A && git commit -m "fix: address verification issues from final checks"
```
