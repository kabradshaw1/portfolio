# mTLS Between Services via cert-manager

**Issue:** #101
**Date:** 2026-04-21

## Overview

Add mutual TLS for inter-service gRPC communication using cert-manager for automatic certificate provisioning and rotation. Services load certs from disk with hot reload via `fsnotify` — zero-downtime rotation without restarts. TLS is opt-in via `TLS_CERT_DIR` env var so local dev and Docker Compose stay plaintext.

## 1. cert-manager Infrastructure

### Installation

cert-manager installed via static manifest (`kubectl apply -f`) into its own `cert-manager` namespace. Three pods: controller, webhook, cainjector. ~150Mi total memory, fits within 16Gi Minikube.

### Certificate Chain

Two-level chain following cert-manager best practice:

1. **`ClusterIssuer` (self-signed)** — bootstraps the CA certificate
2. **`Certificate` (CA cert)** — in `go-ecommerce` namespace, uses the self-signed ClusterIssuer, stored in Secret `grpc-ca-keypair`
3. **`Issuer` (CA-type)** — namespace-scoped in `go-ecommerce`, references `grpc-ca-keypair`, issues per-service certs

A single self-signed issuer per service would technically work but defeats identity verification — there'd be no shared CA to trust.

### Per-Service Certificates

One `Certificate` resource per gRPC-participating service:

| Service | Secret Name | Common Name |
|---------|-------------|-------------|
| auth-service | `auth-grpc-tls` | `go-auth-service.go-ecommerce.svc.cluster.local` |
| product-service | `product-grpc-tls` | `go-product-service.go-ecommerce.svc.cluster.local` |
| cart-service | `cart-grpc-tls` | `go-cart-service.go-ecommerce.svc.cluster.local` |
| order-service | `order-grpc-tls` | `go-order-service.go-ecommerce.svc.cluster.local` |

Each Certificate specifies:
- DNS SANs: the service's cluster DNS name
- Duration: `720h` (30 days)
- Renew before: `480h` (20 days remaining)
- Issuer: the namespace-scoped CA Issuer

cert-manager writes `tls.crt`, `tls.key`, and `ca.crt` into each Secret.

Services that are both gRPC server and client (cart-service) reuse the same cert for both roles — the DNS SAN identifies the service regardless of role.

Order-service has no gRPC server but needs a client cert to present during the mTLS handshake.

## 2. Shared TLS Config Package (`go/pkg/tlsconfig`)

New package providing hot-reloadable TLS configuration for both servers and clients.

### API

**`ServerTLS(certDir string) (*tls.Config, error)`**
- Loads `certDir/tls.crt`, `certDir/tls.key`, `certDir/ca.crt`
- Returns `*tls.Config` with `ClientAuth: tls.RequireAndVerifyClientCert`
- `GetCertificate` callback reads from `atomic.Pointer[tls.Certificate]` for hot reload

**`ClientTLS(certDir string) (credentials.TransportCredentials, error)`**
- Loads same files as ServerTLS
- Returns gRPC `TransportCredentials` that presents the client cert and verifies the server cert against the CA
- `GetClientCertificate` callback reads from atomic pointer for hot reload

**`Watch(certDir string, reload func()) error`**
- Starts a goroutine using `fsnotify` to watch `certDir` for writes
- On change, calls `reload` which swaps the atomic pointer to freshly-loaded cert
- Debounces with 500ms timer (cert-manager writes multiple files in quick succession)
- Returns a stop function for cleanup

### Hot Reload Mechanism

```
fsnotify detects write to certDir/
  → 500ms debounce timer
  → reload() called
  → tls.LoadX509KeyPair(certDir/tls.crt, certDir/tls.key)
  → atomic.Pointer.Store(newCert)
  → next TLS handshake uses new cert (via GetCertificate callback)
```

No connection interruption. Existing connections continue with the old cert until they close naturally. New connections use the new cert.

## 3. Service Code Changes

### TLS opt-in pattern

All services use the same pattern. TLS is enabled when `TLS_CERT_DIR` env var is set:

**gRPC servers** (auth, product, cart):
```go
if certDir := os.Getenv("TLS_CERT_DIR"); certDir != "" {
    serverTLS, err := tlsconfig.ServerTLS(certDir)
    // ... error handling
    grpcServer = grpc.NewServer(
        grpc.Creds(credentials.NewTLS(serverTLS)),
        grpc.StatsHandler(otelgrpc.NewServerHandler()),
    )
    slog.Info("mTLS enabled for gRPC server", "certDir", certDir)
} else {
    grpcServer = grpc.NewServer(
        grpc.StatsHandler(otelgrpc.NewServerHandler()),
    )
}
```

**gRPC clients** (order, cart, auth-middleware in order/cart):
```go
if certDir := os.Getenv("TLS_CERT_DIR"); certDir != "" {
    creds, err := tlsconfig.ClientTLS(certDir)
    // ... error handling
    conn, err = grpc.NewClient(addr, grpc.WithTransportCredentials(creds))
} else {
    conn, err = grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
}
```

### Watcher registration

The file watcher is registered with the shutdown manager at priority 0 (stop watching before draining connections):

```go
if certDir != "" {
    stop, err := tlsconfig.Watch(certDir, reloadFn)
    sm.Register("tls-watcher", 0, func(_ context.Context) error { stop(); return nil })
}
```

### Services modified

| Service | Server TLS | Client TLS | Connects to |
|---------|-----------|------------|-------------|
| auth-service | gRPC :9091 | — | — |
| product-service | gRPC :9095 | — | — |
| cart-service | gRPC :9094 | product, auth | product :9095, auth :9091 |
| order-service | — | product, cart, auth | product :9095, cart :9094, auth :9091 |

## 4. Kubernetes Wiring

### Secret volume mounts

Each deployment that participates in gRPC gets a Secret volume mount:

```yaml
spec:
  template:
    spec:
      volumes:
        - name: grpc-tls
          secret:
            secretName: <service>-grpc-tls
      containers:
        - volumeMounts:
            - name: grpc-tls
              mountPath: /etc/tls
              readOnly: true
```

### ConfigMap updates

Add `TLS_CERT_DIR: /etc/tls` to each service's ConfigMap:
- `go/k8s/configmaps/auth-service-config.yml`
- `go/k8s/configmaps/product-service-config.yml`
- `go/k8s/configmaps/cart-service-config.yml`
- `go/k8s/configmaps/order-service-config.yml`

### QA overlay

Add the same `TLS_CERT_DIR` to QA ConfigMap patches in `k8s/overlays/qa-go/kustomization.yaml`. QA uses the same cert-manager setup (separate Certificate resources in `go-ecommerce-qa` namespace).

### cert-manager manifests

New directory `k8s/cert-manager/` containing:
- `install.yml` — cert-manager CRD + deployment manifest (pinned version)
- `cluster-issuer.yml` — self-signed ClusterIssuer
- `ca-certificate.yml` — CA Certificate in `go-ecommerce` namespace
- `issuer.yml` — CA Issuer in `go-ecommerce` namespace
- `certificates.yml` — per-service Certificate resources (auth, product, cart, order)

QA certificates in `k8s/overlays/qa-go/` or a separate `k8s/cert-manager/qa/` directory with Certificate resources targeting `go-ecommerce-qa` namespace.

### Security context

Existing `readOnlyRootFilesystem: true` is compatible with Secret volume mounts (read-only by nature). No security context changes needed.

### OTel compatibility

`grpc.StatsHandler(otelgrpc.NewServerHandler())` instruments at the gRPC layer, not the transport layer. Traces and metrics flow unchanged over TLS.

## 5. What Stays Plaintext

- **Local dev / Docker Compose** — `TLS_CERT_DIR` unset, plaintext gRPC
- **CI compose-smoke tests** — no cert-manager, plaintext
- **REST endpoints** — mTLS covers gRPC only. REST traffic is routed through NGINX Ingress which handles its own TLS termination via Cloudflare Tunnel
- **Kafka, RabbitMQ, Redis, PostgreSQL** — internal infrastructure connections stay plaintext within the cluster (separate concern, out of scope)

## 6. Testing Strategy

### Unit tests (`go/pkg/tlsconfig/`)

- `TestServerTLS_LoadsCerts` — write temp cert files, verify returned config has `RequireAndVerifyClientCert`
- `TestClientTLS_LoadsCerts` — verify valid `TransportCredentials` returned
- `TestWatch_ReloadsOnChange` — start watcher, overwrite cert, verify atomic pointer swapped
- `TestServerTLS_MissingFiles_ReturnsError` — clean error on missing cert dir

Test helper: generates self-signed CA + leaf cert in temp directory using `crypto/x509` and `crypto/ecdsa` from Go stdlib. No `openssl` dependency.

### Existing tests unchanged

Service tests don't set `TLS_CERT_DIR`, so they use plaintext. No test changes needed.

### QA verification (manual)

- Services start with mTLS enabled (check logs for "mTLS enabled" message)
- gRPC health checks pass
- Smoke tests pass (order → product/cart/auth calls work over TLS)
- Plaintext gRPC call from debug pod is rejected (verifies mTLS enforcement)

## 7. deploy.sh and CI Updates

- `k8s/deploy.sh` — add cert-manager install step (idempotent `kubectl apply`) before Go service deployment
- `.github/workflows/ci.yml` — add cert-manager install + Certificate apply in QA and prod deploy steps, after namespace creation but before service deployment
- No changes to compose-smoke or local dev workflows
