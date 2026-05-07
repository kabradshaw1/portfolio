---
name: go-grpc-service-reliability
description: Use when creating, modifying, reviewing, or debugging Go gRPC servers or clients, protobuf definitions, buf generation, generated pb packages, grpc-go interceptors, service-to-service mTLS, gRPC Kubernetes ports, or gRPC Docker/CI wiring.
---

# Go gRPC Service Reliability

This skill captures the default standard for Go gRPC interfaces in this repo. Apply it when a service exposes gRPC, calls another service over gRPC, owns protobuf definitions, or imports another service's generated protobuf package.

## Proto And Generated Code

- [ ] Create protobuf definitions under `go/proto/<service>/v1/<service>.proto`.
- [ ] Use the existing `go/buf.gen.yaml` generation flow unless the repo has intentionally changed it.
- [ ] Run generation from `go/`: `buf generate --path proto/<service>`.
- [ ] Generated code belongs under `go/<service>/pb/<service>/v1/`, not `internal/pb/`.
- [ ] If another service imports this proto, add the needed `replace` directive in that service's `go.mod`.
- [ ] If another service imports this proto, update that service's Dockerfile to copy the imported service/proto source needed for the build.

## Server Requirements

- [ ] Add `grpc.StatsHandler(otelgrpc.NewServerHandler())` to every gRPC server.
- [ ] Support mTLS opt-in: when `TLS_CERT_DIR` is set, load server TLS with `tlsconfig.ServerTLS(certDir)`.
- [ ] Keep plaintext local/CI operation available when `TLS_CERT_DIR` is unset.
- [ ] Register gRPC shutdown with the service shutdown manager so in-flight RPCs drain before process exit.
- [ ] Expose the gRPC port in the Dockerfile, K8s Deployment container ports, K8s Service, compose CI overlay, and any smoke-test env that needs it.

## Client Requirements

- [ ] Use `grpc.NewClient` or the repo's current preferred gRPC dial pattern consistently.
- [ ] Add `grpc.WithUnaryInterceptor(grpcmetrics.UnaryClientInterceptor("<target>"))` to every outbound gRPC client.
- [ ] Use mTLS client credentials when the environment supplies the cert directory; keep local/CI plaintext fallback explicit.
- [ ] Wrap outbound gRPC dependencies with the service's existing circuit breaker/retry pattern where calls are made from repositories or clients.
- [ ] Configure target addresses through `cmd/server/config.go`, K8s ConfigMaps, and compose/CI env.

## Kubernetes And Certificates

For gRPC servers:

- [ ] Mount the service's `grpc-tls` secret at `/etc/tls`.
- [ ] Set `TLS_CERT_DIR=/etc/tls` in the service ConfigMap or Deployment env where mTLS is required.
- [ ] Add the gRPC port to the Service as a named port.
- [ ] Add cert-manager Certificates for prod and QA.

Certificate template:

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: <service>-grpc-tls
spec:
  secretName: <service>-grpc-tls
  issuerRef:
    name: grpc-ca-issuer
    kind: Issuer
  dnsNames:
    - go-<service>
    - go-<service>.<namespace>
    - go-<service>.<namespace>.svc.cluster.local
```

Add the certificate to `k8s/cert-manager/certificates.yml` for prod and `k8s/cert-manager/qa-certificates.yml` for QA.

## Verification

- [ ] `cd go && buf generate --path proto/<service>` succeeds with no stale generated output.
- [ ] The owning service's Go tests pass.
- [ ] Any importing service builds after Dockerfile and `go.mod` updates.
- [ ] K8s manifests validate with the gRPC port, TLS env, and certificate present when serving gRPC.
- [ ] The relevant Go preflight passes before commit.
