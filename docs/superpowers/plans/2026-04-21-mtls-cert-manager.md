# mTLS via cert-manager Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add mutual TLS for inter-service gRPC communication using cert-manager with hot-reloadable certificates.

**Architecture:** cert-manager provisions per-service certs from a self-signed CA. A shared `go/pkg/tlsconfig` package provides hot-reloadable server/client TLS configs via `fsnotify` + `atomic.Pointer`. Services opt in via `TLS_CERT_DIR` env var — unset means plaintext (local dev, CI).

**Tech Stack:** Go, gRPC, cert-manager, fsnotify, crypto/x509, Kubernetes

**Spec:** `docs/superpowers/specs/2026-04-21-mtls-cert-manager-design.md`

---

## File Structure

**New files:**
- `go/pkg/tlsconfig/tlsconfig.go` — ServerTLS, ClientTLS, Watch functions
- `go/pkg/tlsconfig/tlsconfig_test.go` — unit tests with generated test certs
- `go/pkg/tlsconfig/testutil_test.go` — test helper: self-signed CA + leaf cert generation
- `k8s/cert-manager/cluster-issuer.yml` — self-signed ClusterIssuer
- `k8s/cert-manager/ca-certificate.yml` — CA Certificate in go-ecommerce namespace
- `k8s/cert-manager/issuer.yml` — CA Issuer in go-ecommerce namespace
- `k8s/cert-manager/certificates.yml` — per-service Certificate resources
- `k8s/cert-manager/qa-certificates.yml` — per-service Certificates for QA namespace

**Modified files:**
- `go/pkg/go.mod` — add fsnotify dependency
- `go/auth-service/cmd/server/main.go` — TLS opt-in for gRPC server
- `go/product-service/cmd/server/main.go` — TLS opt-in for gRPC server
- `go/cart-service/cmd/server/main.go` — TLS opt-in for gRPC server + client
- `go/cart-service/internal/productclient/client.go` — accept TransportCredentials option
- `go/order-service/cmd/server/main.go` — TLS opt-in for gRPC clients
- `go/order-service/internal/productclient/client.go` — accept TransportCredentials option
- `go/order-service/internal/cartclient/client.go` — accept TransportCredentials option
- `go/k8s/deployments/auth-service.yml` — add TLS volume mount
- `go/k8s/deployments/product-service.yml` — add TLS volume mount
- `go/k8s/deployments/cart-service.yml` — add TLS volume mount
- `go/k8s/deployments/order-service.yml` — add TLS volume mount
- `go/k8s/configmaps/auth-service-config.yml` — add TLS_CERT_DIR
- `go/k8s/configmaps/product-service-config.yml` — add TLS_CERT_DIR
- `go/k8s/configmaps/cart-service-config.yml` — add TLS_CERT_DIR
- `go/k8s/configmaps/order-service-config.yml` — add TLS_CERT_DIR
- `k8s/overlays/qa-go/kustomization.yaml` — add TLS_CERT_DIR to QA patches
- `k8s/deploy.sh` — add cert-manager install step
- `.github/workflows/ci.yml` — add cert-manager install in deploy steps

---

### Task 1: TLS Config Package — Test Helper (`go/pkg/tlsconfig`)

**Files:**
- Create: `go/pkg/tlsconfig/testutil_test.go`

- [ ] **Step 1: Add fsnotify dependency**

```bash
cd go/pkg && go get github.com/fsnotify/fsnotify && go mod tidy
```

Then tidy all services since pkg changed:
```bash
for svc in auth-service product-service cart-service order-service ai-service analytics-service; do
  cd /path/to/worktree/go/$svc && go mod tidy
done
```

- [ ] **Step 2: Write the test cert generator**

Create `go/pkg/tlsconfig/testutil_test.go`:

```go
package tlsconfig_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// generateTestCerts creates a self-signed CA and a leaf certificate in dir.
// Files written: ca.crt, tls.crt, tls.key
func generateTestCerts(t *testing.T, dir string) {
	t.Helper()

	// CA key + cert
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, err := x509.ParseCertificate(caCertDER)
	if err != nil {
		t.Fatal(err)
	}

	// Leaf key + cert
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "test-service"},
		DNSNames:     []string{"localhost", "test-service.default.svc.cluster.local"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	leafCertDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}

	// Write ca.crt
	writePEM(t, filepath.Join(dir, "ca.crt"), "CERTIFICATE", caCertDER)
	// Write tls.crt
	writePEM(t, filepath.Join(dir, "tls.crt"), "CERTIFICATE", leafCertDER)
	// Write tls.key
	leafKeyDER, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		t.Fatal(err)
	}
	writePEM(t, filepath.Join(dir, "tls.key"), "EC PRIVATE KEY", leafKeyDER)
}

func writePEM(t *testing.T, path, blockType string, data []byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := pem.Encode(f, &pem.Block{Type: blockType, Bytes: data}); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 3: Commit**

```bash
git add go/pkg/tlsconfig/testutil_test.go go/pkg/go.mod go/pkg/go.sum go/*/go.mod go/*/go.sum
git commit -m "feat(pkg): add test cert generator for tlsconfig package"
```

---

### Task 2: TLS Config Package — ServerTLS and ClientTLS

**Files:**
- Create: `go/pkg/tlsconfig/tlsconfig.go`
- Create: `go/pkg/tlsconfig/tlsconfig_test.go`

- [ ] **Step 1: Write the failing tests**

Create `go/pkg/tlsconfig/tlsconfig_test.go`:

```go
package tlsconfig_test

import (
	"crypto/tls"
	"testing"

	"github.com/kabradshaw1/portfolio/go/pkg/tlsconfig"
)

func TestServerTLS_LoadsCerts(t *testing.T) {
	dir := t.TempDir()
	generateTestCerts(t, dir)

	cfg, err := tlsconfig.ServerTLS(dir)
	if err != nil {
		t.Fatalf("ServerTLS: %v", err)
	}
	if cfg.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Fatalf("expected RequireAndVerifyClientCert, got %v", cfg.ClientAuth)
	}
	// GetCertificate should return a cert
	cert, err := cfg.GetCertificate(nil)
	if err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
	if cert == nil {
		t.Fatal("GetCertificate returned nil")
	}
}

func TestClientTLS_LoadsCerts(t *testing.T) {
	dir := t.TempDir()
	generateTestCerts(t, dir)

	creds, err := tlsconfig.ClientTLS(dir)
	if err != nil {
		t.Fatalf("ClientTLS: %v", err)
	}
	if creds == nil {
		t.Fatal("ClientTLS returned nil credentials")
	}
	// Verify the credentials report "tls" as the security protocol
	if info := creds.Info(); info.SecurityProtocol != "tls" {
		t.Fatalf("expected security protocol 'tls', got %q", info.SecurityProtocol)
	}
}

func TestServerTLS_MissingDir_ReturnsError(t *testing.T) {
	_, err := tlsconfig.ServerTLS("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for missing cert dir")
	}
}

func TestClientTLS_MissingDir_ReturnsError(t *testing.T) {
	_, err := tlsconfig.ClientTLS("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for missing cert dir")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd go/pkg && go test ./tlsconfig/ -v -race`
Expected: FAIL — package `tlsconfig` has no non-test files

- [ ] **Step 3: Write the implementation**

Create `go/pkg/tlsconfig/tlsconfig.go`:

```go
package tlsconfig

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"google.golang.org/grpc/credentials"
)

// ServerTLS loads TLS certificates from certDir and returns a *tls.Config
// configured for mTLS (requiring client certificates). The returned config
// uses GetCertificate for hot-reloadable certs via atomic pointer.
func ServerTLS(certDir string) (*tls.Config, error) {
	certPtr, caPool, err := Load(certDir)
	if err != nil {
		return nil, fmt.Errorf("server tls: %w", err)
	}
	return &tls.Config{
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			return certPtr.Load(), nil
		},
		ClientCAs:  caPool,
		ClientAuth: tls.RequireAndVerifyClientCert,
		MinVersion: tls.VersionTLS13,
	}, nil
}

// ClientTLS loads TLS certificates from certDir and returns gRPC
// TransportCredentials that present the client cert and verify the server.
func ClientTLS(certDir string) (credentials.TransportCredentials, error) {
	certPtr, caPool, err := Load(certDir)
	if err != nil {
		return nil, fmt.Errorf("client tls: %w", err)
	}
	cfg := &tls.Config{
		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return certPtr.Load(), nil
		},
		RootCAs:    caPool,
		MinVersion: tls.VersionTLS13,
	}
	return credentials.NewTLS(cfg), nil
}

// CertPointer returns the atomic pointer for a certDir, allowing Watch
// to swap in new certs. Must be called with the same certDir as ServerTLS/ClientTLS.
func CertPointer(certDir string) (*atomic.Pointer[tls.Certificate], error) {
	certPtr, _, err := load(certDir)
	if err != nil {
		return nil, err
	}
	return certPtr, nil
}

// Load reads cert/key/CA from certDir and returns an atomic pointer to the
// certificate (for hot reload) and a CA pool. Exported for use with Watch.
func Load(certDir string) (*atomic.Pointer[tls.Certificate], *x509.CertPool, error) {
	certFile := filepath.Join(certDir, "tls.crt")
	keyFile := filepath.Join(certDir, "tls.key")
	caFile := filepath.Join(certDir, "ca.crt")

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("load cert/key: %w", err)
	}

	caData, err := os.ReadFile(caFile)
	if err != nil {
		return nil, nil, fmt.Errorf("read ca cert: %w", err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caData) {
		return nil, nil, fmt.Errorf("failed to parse CA cert")
	}

	var ptr atomic.Pointer[tls.Certificate]
	ptr.Store(&cert)

	return &ptr, caPool, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd go/pkg && go test ./tlsconfig/ -v -race`
Expected: PASS (all 4 tests)

- [ ] **Step 5: Run linter**

Run: `cd go/pkg && golangci-lint run ./tlsconfig/`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
git add go/pkg/tlsconfig/tlsconfig.go go/pkg/tlsconfig/tlsconfig_test.go
git commit -m "feat(pkg): add ServerTLS and ClientTLS with hot-reload support"
```

---

### Task 3: TLS Config Package — File Watcher

**Files:**
- Modify: `go/pkg/tlsconfig/tlsconfig.go` — add Watch function
- Modify: `go/pkg/tlsconfig/tlsconfig_test.go` — add watch test

- [ ] **Step 1: Write the failing test**

Add to `go/pkg/tlsconfig/tlsconfig_test.go`:

```go
func TestWatch_ReloadsOnChange(t *testing.T) {
	dir := t.TempDir()
	generateTestCerts(t, dir)

	certPtr, _, err := tlsconfig.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	originalCert := certPtr.Load()

	stop, err := tlsconfig.Watch(dir, certPtr)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer stop()

	// Overwrite certs with new ones
	generateTestCerts(t, dir)

	// Wait for the watcher to pick up the change (debounce is 500ms)
	time.Sleep(1 * time.Second)

	newCert := certPtr.Load()
	if newCert == originalCert {
		t.Fatal("expected cert pointer to change after file write")
	}
}
```

Add `"time"` to the imports of the test file.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go/pkg && go test ./tlsconfig/ -run TestWatch -v -race`
Expected: FAIL — `Watch` undefined

- [ ] **Step 3: Write the Watch implementation**

Add to `go/pkg/tlsconfig/tlsconfig.go`:

```go
import (
	// ... existing imports ...
	"log/slog"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watch monitors certDir for file changes and reloads the certificate into
// certPtr. Returns a stop function. Debounces with a 500ms timer since
// cert-manager writes multiple files in quick succession.
func Watch(certDir string, certPtr *atomic.Pointer[tls.Certificate]) (stop func(), err error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create watcher: %w", err)
	}
	if err := watcher.Add(certDir); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("watch %s: %w", certDir, err)
	}

	done := make(chan struct{})
	go func() {
		defer watcher.Close()
		var debounce *time.Timer
		for {
			select {
			case <-done:
				if debounce != nil {
					debounce.Stop()
				}
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
					if debounce != nil {
						debounce.Stop()
					}
					debounce = time.AfterFunc(500*time.Millisecond, func() {
						reload(certDir, certPtr)
					})
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				slog.Error("tls watcher error", "error", err)
			}
		}
	}()

	return func() { close(done) }, nil
}

func reload(certDir string, certPtr *atomic.Pointer[tls.Certificate]) {
	certFile := filepath.Join(certDir, "tls.crt")
	keyFile := filepath.Join(certDir, "tls.key")
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		slog.Error("tls reload failed", "error", err)
		return
	}
	certPtr.Store(&cert)
	slog.Info("tls certificate reloaded", "certDir", certDir)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd go/pkg && go test ./tlsconfig/ -v -race -timeout 30s`
Expected: PASS (all 5 tests)

- [ ] **Step 5: Commit**

```bash
git add go/pkg/tlsconfig/
git commit -m "feat(pkg): add fsnotify file watcher for TLS cert hot reload"
```

---

### Task 4: gRPC Client Libraries — Accept TransportCredentials

The gRPC client libraries in order-service and cart-service currently hardcode `insecure.NewCredentials()`. They need to accept an optional `TransportCredentials` parameter so the caller in `main.go` can pass TLS or insecure credentials.

**Files:**
- Modify: `go/order-service/internal/productclient/client.go`
- Modify: `go/order-service/internal/cartclient/client.go`
- Modify: `go/cart-service/internal/productclient/client.go`

- [ ] **Step 1: Update order-service productclient**

In `go/order-service/internal/productclient/client.go`, change the `New` function to accept a `grpc.DialOption`:

Replace the current `New` function (lines 23-34):
```go
// New dials the product-service gRPC endpoint and returns a ready client.
func New(addr string, creds credentials.TransportCredentials) (*GRPCClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to product-service: %w", err)
	}
	return &GRPCClient{
		client: pb.NewProductServiceClient(conn),
		conn:   conn,
	}, nil
}
```

Remove `"google.golang.org/grpc/credentials/insecure"` import, add `"google.golang.org/grpc/credentials"`.

- [ ] **Step 2: Update order-service cartclient**

In `go/order-service/internal/cartclient/client.go`, change `New` (lines 26-48):

```go
// New dials both cart-service and product-service gRPC endpoints.
func New(cartAddr, productAddr string, creds credentials.TransportCredentials) (*GRPCClient, error) {
	cartConn, err := grpc.NewClient(cartAddr,
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to cart-service: %w", err)
	}

	productConn, err := grpc.NewClient(productAddr,
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		cartConn.Close()
		return nil, fmt.Errorf("connect to product-service: %w", err)
	}

	return &GRPCClient{
		cartClient:    cart.NewCartServiceClient(cartConn),
		productClient: pb.NewProductServiceClient(productConn),
		cartConn:      cartConn,
		productConn:   productConn,
	}, nil
}
```

Remove `"google.golang.org/grpc/credentials/insecure"` import, add `"google.golang.org/grpc/credentials"`.

- [ ] **Step 3: Update cart-service productclient**

In `go/cart-service/internal/productclient/client.go`, change `New` (lines 22-33):

```go
func New(addr string, creds credentials.TransportCredentials) (*GRPCClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to product-service: %w", err)
	}
	return &GRPCClient{
		client: pb.NewProductServiceClient(conn),
		conn:   conn,
	}, nil
}
```

Remove `"google.golang.org/grpc/credentials/insecure"` import, add `"google.golang.org/grpc/credentials"`.

- [ ] **Step 4: Update callers in main.go files**

Each `main.go` that calls these `New` functions needs to pass credentials. For now, keep passing `insecure.NewCredentials()` — the TLS opt-in logic comes in the next tasks.

In `go/order-service/cmd/server/main.go`, update the calls:
```go
productClient, err := productclient.New(cfg.ProductGRPCAddr, insecure.NewCredentials())
```
```go
cartClient, err := cartclient.New(cfg.CartGRPCAddr, cfg.ProductGRPCAddr, insecure.NewCredentials())
```

In `go/cart-service/cmd/server/main.go`, update:
```go
productClient, err := productclient.New(cfg.ProductGRPCAddr, insecure.NewCredentials())
```

- [ ] **Step 5: Tidy and test**

```bash
for svc in order-service cart-service; do
  cd /path/to/worktree/go/$svc && go mod tidy && go test ./... -race
done
```
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add go/order-service/internal/productclient/ go/order-service/internal/cartclient/ \
  go/cart-service/internal/productclient/ \
  go/order-service/cmd/server/main.go go/cart-service/cmd/server/main.go
git commit -m "refactor(grpc): accept TransportCredentials in gRPC client constructors"
```

---

### Task 5: Service Code — TLS Opt-in for gRPC Servers

**Files:**
- Modify: `go/auth-service/cmd/server/main.go`
- Modify: `go/product-service/cmd/server/main.go`
- Modify: `go/cart-service/cmd/server/main.go`

Each gRPC server gets the TLS opt-in pattern. The pattern is identical for all three, differing only in variable names.

- [ ] **Step 1: Update auth-service**

In `go/auth-service/cmd/server/main.go`, replace the gRPC server creation block (lines 79-82):

```go
// gRPC server — mTLS if TLS_CERT_DIR is set, plaintext otherwise
var grpcServer *grpc.Server
var tlsWatchStop func()
if certDir := os.Getenv("TLS_CERT_DIR"); certDir != "" {
	serverTLS, err := tlsconfig.ServerTLS(certDir)
	if err != nil {
		log.Fatalf("tls config: %v", err)
	}
	grpcServer = grpc.NewServer(
		grpc.Creds(credentials.NewTLS(serverTLS)),
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	)
	certPtr, err := tlsconfig.CertPointer(certDir)
	if err != nil {
		log.Fatalf("tls cert pointer: %v", err)
	}
	tlsWatchStop, err = tlsconfig.Watch(certDir, certPtr)
	if err != nil {
		log.Fatalf("tls watcher: %v", err)
	}
	slog.Info("mTLS enabled for gRPC server", "certDir", certDir)
} else {
	grpcServer = grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	)
}
```

Add imports:
```go
"github.com/kabradshaw1/portfolio/go/pkg/tlsconfig"
"google.golang.org/grpc/credentials"
```

Register the watcher stop in the shutdown manager (add before the existing hooks):
```go
if tlsWatchStop != nil {
	sm.Register("tls-watcher", 0, func(_ context.Context) error {
		tlsWatchStop()
		return nil
	})
}
```

- [ ] **Step 2: Update product-service**

Same pattern in `go/product-service/cmd/server/main.go` (lines 76-79). Replace:
```go
grpcServer := grpc.NewServer(
	grpc.StatsHandler(otelgrpc.NewServerHandler()),
)
```

With the identical TLS opt-in block from Step 1. Add the same imports and shutdown manager registration.

- [ ] **Step 3: Update cart-service**

Same pattern in `go/cart-service/cmd/server/main.go` (lines 132-134). Replace the grpc.NewServer block with the TLS opt-in pattern. Add imports and shutdown registration.

- [ ] **Step 4: Test all three services**

```bash
for svc in auth-service product-service cart-service; do
  cd /path/to/worktree/go/$svc && go build ./... && go test ./... -race
done
```
Expected: BUILD OK, tests PASS (TLS_CERT_DIR is unset, so plaintext path runs)

- [ ] **Step 5: Commit**

```bash
git add go/auth-service/cmd/server/main.go \
  go/product-service/cmd/server/main.go \
  go/cart-service/cmd/server/main.go
git commit -m "feat(grpc): add mTLS opt-in for gRPC servers via TLS_CERT_DIR"
```

---

### Task 6: Service Code — TLS Opt-in for gRPC Clients

**Files:**
- Modify: `go/order-service/cmd/server/main.go`
- Modify: `go/cart-service/cmd/server/main.go`

- [ ] **Step 1: Update order-service main.go**

In `go/order-service/cmd/server/main.go`, add TLS opt-in for all gRPC client connections. Before the existing gRPC client creation code, add:

```go
// Resolve gRPC transport credentials — mTLS if TLS_CERT_DIR is set
var grpcCreds credentials.TransportCredentials
var tlsWatchStop func()
if certDir := os.Getenv("TLS_CERT_DIR"); certDir != "" {
	var err error
	grpcCreds, err = tlsconfig.ClientTLS(certDir)
	if err != nil {
		log.Fatalf("tls config: %v", err)
	}
	certPtr, err := tlsconfig.CertPointer(certDir)
	if err != nil {
		log.Fatalf("tls cert pointer: %v", err)
	}
	tlsWatchStop, err = tlsconfig.Watch(certDir, certPtr)
	if err != nil {
		log.Fatalf("tls watcher: %v", err)
	}
	slog.Info("mTLS enabled for gRPC clients", "certDir", certDir)
} else {
	grpcCreds = insecure.NewCredentials()
}
```

Then update all client creation calls to use `grpcCreds`:
```go
productClient, err := productclient.New(cfg.ProductGRPCAddr, grpcCreds)
cartClient, err := cartclient.New(cfg.CartGRPCAddr, cfg.ProductGRPCAddr, grpcCreds)
```

For the auth-service gRPC client (authmiddleware), also use grpcCreds:
```go
authConn, err := grpc.NewClient(cfg.AuthGRPCURL,
	grpc.WithTransportCredentials(grpcCreds),
)
```

Register the watcher stop in the shutdown manager:
```go
if tlsWatchStop != nil {
	sm.Register("tls-watcher", 0, func(_ context.Context) error {
		tlsWatchStop()
		return nil
	})
}
```

Add imports: `tlsconfig`, `credentials`.

- [ ] **Step 2: Update cart-service main.go**

Cart-service is both a gRPC server (handled in Task 5) and a gRPC client to product-service and auth-service. Add client-side TLS opt-in.

The TLS watcher was already created in Task 5 for the server side. Reuse the same `certDir` env var for client credentials:

```go
// After the server TLS block, create client credentials from the same certDir
var grpcClientCreds credentials.TransportCredentials
if certDir := os.Getenv("TLS_CERT_DIR"); certDir != "" {
	var err error
	grpcClientCreds, err = tlsconfig.ClientTLS(certDir)
	if err != nil {
		log.Fatalf("client tls config: %v", err)
	}
} else {
	grpcClientCreds = insecure.NewCredentials()
}
```

Update the product-service client call:
```go
productClient, err := productclient.New(cfg.ProductGRPCAddr, grpcClientCreds)
```

Update the auth-service gRPC client:
```go
authConn, err := grpc.NewClient(cfg.AuthGRPCURL,
	grpc.WithTransportCredentials(grpcClientCreds),
)
```

- [ ] **Step 3: Test both services**

```bash
for svc in order-service cart-service; do
  cd /path/to/worktree/go/$svc && go build ./... && go test ./... -race
done
```
Expected: BUILD OK, tests PASS

- [ ] **Step 4: Commit**

```bash
git add go/order-service/cmd/server/main.go go/cart-service/cmd/server/main.go
git commit -m "feat(grpc): add mTLS opt-in for gRPC clients via TLS_CERT_DIR"
```

---

### Task 7: cert-manager Kubernetes Manifests

**Files:**
- Create: `k8s/cert-manager/cluster-issuer.yml`
- Create: `k8s/cert-manager/ca-certificate.yml`
- Create: `k8s/cert-manager/issuer.yml`
- Create: `k8s/cert-manager/certificates.yml`
- Create: `k8s/cert-manager/qa-certificates.yml`

- [ ] **Step 1: Create ClusterIssuer**

Create `k8s/cert-manager/cluster-issuer.yml`:

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: selfsigned-issuer
spec:
  selfSigned: {}
```

- [ ] **Step 2: Create CA Certificate**

Create `k8s/cert-manager/ca-certificate.yml`:

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: grpc-ca
  namespace: go-ecommerce
spec:
  isCA: true
  commonName: grpc-internal-ca
  secretName: grpc-ca-keypair
  duration: 8760h    # 1 year
  renewBefore: 720h  # 30 days before expiry
  privateKey:
    algorithm: ECDSA
    size: 256
  issuerRef:
    name: selfsigned-issuer
    kind: ClusterIssuer
```

- [ ] **Step 3: Create CA Issuer**

Create `k8s/cert-manager/issuer.yml`:

```yaml
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: grpc-ca-issuer
  namespace: go-ecommerce
spec:
  ca:
    secretName: grpc-ca-keypair
```

- [ ] **Step 4: Create per-service Certificates (prod)**

Create `k8s/cert-manager/certificates.yml`:

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: auth-grpc-tls
  namespace: go-ecommerce
spec:
  secretName: auth-grpc-tls
  duration: 720h
  renewBefore: 480h
  commonName: go-auth-service.go-ecommerce.svc.cluster.local
  dnsNames:
    - go-auth-service
    - go-auth-service.go-ecommerce
    - go-auth-service.go-ecommerce.svc.cluster.local
  privateKey:
    algorithm: ECDSA
    size: 256
  usages:
    - server auth
    - client auth
  issuerRef:
    name: grpc-ca-issuer
    kind: Issuer
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: product-grpc-tls
  namespace: go-ecommerce
spec:
  secretName: product-grpc-tls
  duration: 720h
  renewBefore: 480h
  commonName: go-product-service.go-ecommerce.svc.cluster.local
  dnsNames:
    - go-product-service
    - go-product-service.go-ecommerce
    - go-product-service.go-ecommerce.svc.cluster.local
  privateKey:
    algorithm: ECDSA
    size: 256
  usages:
    - server auth
    - client auth
  issuerRef:
    name: grpc-ca-issuer
    kind: Issuer
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: cart-grpc-tls
  namespace: go-ecommerce
spec:
  secretName: cart-grpc-tls
  duration: 720h
  renewBefore: 480h
  commonName: go-cart-service.go-ecommerce.svc.cluster.local
  dnsNames:
    - go-cart-service
    - go-cart-service.go-ecommerce
    - go-cart-service.go-ecommerce.svc.cluster.local
  privateKey:
    algorithm: ECDSA
    size: 256
  usages:
    - server auth
    - client auth
  issuerRef:
    name: grpc-ca-issuer
    kind: Issuer
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: order-grpc-tls
  namespace: go-ecommerce
spec:
  secretName: order-grpc-tls
  duration: 720h
  renewBefore: 480h
  commonName: go-order-service.go-ecommerce.svc.cluster.local
  dnsNames:
    - go-order-service
    - go-order-service.go-ecommerce
    - go-order-service.go-ecommerce.svc.cluster.local
  privateKey:
    algorithm: ECDSA
    size: 256
  usages:
    - client auth
  issuerRef:
    name: grpc-ca-issuer
    kind: Issuer
```

- [ ] **Step 5: Create QA Certificates**

Create `k8s/cert-manager/qa-certificates.yml` — same as `certificates.yml` but:
- All `namespace:` values changed to `go-ecommerce-qa`
- All DNS names use `go-ecommerce-qa` namespace
- Issuer reference points to a QA issuer (need a QA CA certificate + issuer too)

Actually, the QA namespace needs its own CA cert and Issuer. Add to the file:

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: grpc-ca
  namespace: go-ecommerce-qa
spec:
  isCA: true
  commonName: grpc-internal-ca-qa
  secretName: grpc-ca-keypair
  duration: 8760h
  renewBefore: 720h
  privateKey:
    algorithm: ECDSA
    size: 256
  issuerRef:
    name: selfsigned-issuer
    kind: ClusterIssuer
---
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: grpc-ca-issuer
  namespace: go-ecommerce-qa
spec:
  ca:
    secretName: grpc-ca-keypair
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: auth-grpc-tls
  namespace: go-ecommerce-qa
spec:
  secretName: auth-grpc-tls
  duration: 720h
  renewBefore: 480h
  commonName: go-auth-service.go-ecommerce-qa.svc.cluster.local
  dnsNames:
    - go-auth-service
    - go-auth-service.go-ecommerce-qa
    - go-auth-service.go-ecommerce-qa.svc.cluster.local
  privateKey:
    algorithm: ECDSA
    size: 256
  usages:
    - server auth
    - client auth
  issuerRef:
    name: grpc-ca-issuer
    kind: Issuer
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: product-grpc-tls
  namespace: go-ecommerce-qa
spec:
  secretName: product-grpc-tls
  duration: 720h
  renewBefore: 480h
  commonName: go-product-service.go-ecommerce-qa.svc.cluster.local
  dnsNames:
    - go-product-service
    - go-product-service.go-ecommerce-qa
    - go-product-service.go-ecommerce-qa.svc.cluster.local
  privateKey:
    algorithm: ECDSA
    size: 256
  usages:
    - server auth
    - client auth
  issuerRef:
    name: grpc-ca-issuer
    kind: Issuer
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: cart-grpc-tls
  namespace: go-ecommerce-qa
spec:
  secretName: cart-grpc-tls
  duration: 720h
  renewBefore: 480h
  commonName: go-cart-service.go-ecommerce-qa.svc.cluster.local
  dnsNames:
    - go-cart-service
    - go-cart-service.go-ecommerce-qa
    - go-cart-service.go-ecommerce-qa.svc.cluster.local
  privateKey:
    algorithm: ECDSA
    size: 256
  usages:
    - server auth
    - client auth
  issuerRef:
    name: grpc-ca-issuer
    kind: Issuer
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: order-grpc-tls
  namespace: go-ecommerce-qa
spec:
  secretName: order-grpc-tls
  duration: 720h
  renewBefore: 480h
  commonName: go-order-service.go-ecommerce-qa.svc.cluster.local
  dnsNames:
    - go-order-service
    - go-order-service.go-ecommerce-qa
    - go-order-service.go-ecommerce-qa.svc.cluster.local
  privateKey:
    algorithm: ECDSA
    size: 256
  usages:
    - client auth
  issuerRef:
    name: grpc-ca-issuer
    kind: Issuer
```

- [ ] **Step 6: Commit**

```bash
git add k8s/cert-manager/
git commit -m "feat(k8s): add cert-manager CA chain and per-service certificates"
```

---

### Task 8: K8s Deployment Volume Mounts and ConfigMaps

**Files:**
- Modify: `go/k8s/deployments/auth-service.yml`
- Modify: `go/k8s/deployments/product-service.yml`
- Modify: `go/k8s/deployments/cart-service.yml`
- Modify: `go/k8s/deployments/order-service.yml`
- Modify: `go/k8s/configmaps/auth-service-config.yml`
- Modify: `go/k8s/configmaps/product-service-config.yml`
- Modify: `go/k8s/configmaps/cart-service-config.yml`
- Modify: `go/k8s/configmaps/order-service-config.yml`
- Modify: `k8s/overlays/qa-go/kustomization.yaml`

- [ ] **Step 1: Add volume and volumeMount to auth-service deployment**

In `go/k8s/deployments/auth-service.yml`, add under `spec.template.spec` (same level as `containers:`):
```yaml
      volumes:
        - name: grpc-tls
          secret:
            secretName: auth-grpc-tls
```

Add under the container's existing entries (after resources/probes):
```yaml
          volumeMounts:
            - name: grpc-tls
              mountPath: /etc/tls
              readOnly: true
```

- [ ] **Step 2: Add volume and volumeMount to product-service deployment**

Same pattern. Secret name: `product-grpc-tls`.

- [ ] **Step 3: Add volume and volumeMount to cart-service deployment**

Same pattern. Secret name: `cart-grpc-tls`.

- [ ] **Step 4: Add volume and volumeMount to order-service deployment**

Same pattern. Secret name: `order-grpc-tls`.

- [ ] **Step 5: Add TLS_CERT_DIR to all four ConfigMaps**

Add to each service's ConfigMap in `go/k8s/configmaps/`:
```yaml
  TLS_CERT_DIR: /etc/tls
```

Files: `auth-service-config.yml`, `product-service-config.yml`, `cart-service-config.yml`, `order-service-config.yml`

- [ ] **Step 6: Add TLS_CERT_DIR to QA overlay**

In `k8s/overlays/qa-go/kustomization.yaml`, add `TLS_CERT_DIR: /etc/tls` to the ConfigMap patches for auth, product, cart, and order services.

- [ ] **Step 7: Commit**

```bash
git add go/k8s/deployments/ go/k8s/configmaps/ k8s/overlays/qa-go/
git commit -m "feat(k8s): add TLS volume mounts and TLS_CERT_DIR to all gRPC services"
```

---

### Task 9: deploy.sh and CI Updates

**Files:**
- Modify: `k8s/deploy.sh`
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Add cert-manager install to deploy.sh**

In `k8s/deploy.sh`, add cert-manager installation before the Go service deployment. For the QA block (before line 41), and for the prod block (before line 112):

For both QA and prod sections, add:
```bash
echo "==> Installing cert-manager (if not already present)..."
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.17.2/cert-manager.yaml 2>/dev/null || true
echo "==> Waiting for cert-manager..."
kubectl wait --for=condition=available --timeout=120s deployment/cert-manager -n cert-manager
kubectl wait --for=condition=available --timeout=120s deployment/cert-manager-webhook -n cert-manager

echo "==> Applying cert-manager resources..."
kubectl apply -f "$SCRIPT_DIR/cert-manager/cluster-issuer.yml"
```

For the QA section, also apply QA certificates:
```bash
kubectl apply -f "$SCRIPT_DIR/cert-manager/qa-certificates.yml"
```

For the prod section, apply prod certificates:
```bash
kubectl apply -f "$SCRIPT_DIR/cert-manager/ca-certificate.yml"
kubectl apply -f "$SCRIPT_DIR/cert-manager/issuer.yml"
kubectl apply -f "$SCRIPT_DIR/cert-manager/certificates.yml"
```

- [ ] **Step 2: Add cert-manager install to CI deploy steps**

In `.github/workflows/ci.yml`, in both the QA deploy and prod deploy SSH steps, add the same cert-manager installation commands before the `kubectl apply -k` for Go services. Follow the same pattern as deploy.sh.

- [ ] **Step 3: Commit**

```bash
git add k8s/deploy.sh .github/workflows/ci.yml
git commit -m "ci: add cert-manager install to deploy pipeline and deploy.sh"
```

---

### Task 10: Final Verification

- [ ] **Step 1: Run full Go preflight**

```bash
for svc in auth-service product-service cart-service order-service ai-service analytics-service; do
  cd /path/to/worktree/go/$svc && golangci-lint run ./... && go test ./... -race
done
```
Expected: All pass

- [ ] **Step 2: Run pkg tlsconfig tests specifically**

```bash
cd go/pkg && go test ./tlsconfig/ -v -race -count=1
```
Expected: All 5 tests pass

- [ ] **Step 3: Verify all services build**

```bash
for svc in auth-service product-service cart-service order-service ai-service analytics-service; do
  cd /path/to/worktree/go/$svc && go build ./...
done
```
Expected: All build OK

- [ ] **Step 4: Final commit if any fixes needed**

```bash
git add -A && git commit -m "fix: address verification issues"
```
