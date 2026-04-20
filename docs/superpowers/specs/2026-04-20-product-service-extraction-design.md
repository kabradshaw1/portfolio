# Phase 1: Product Service Extraction

## Context

First phase of the ecommerce decomposition roadmap (see `2026-04-20-ecommerce-decomposition-grpc-design.md`). Extracts product and category functionality from the monolithic `ecommerce-service` into a standalone `product-service` with both REST and gRPC interfaces. Establishes the proto toolchain (`buf`), gRPC patterns (reflection, health, OTel interceptors), and dual-server architecture that Phases 2 and 3 will follow.

## Service Architecture

### Dual Server

`product-service` runs two servers from a single binary, sharing the same service layer:

- **REST** on `:8095` — frontend traffic via NGINX ingress (`/go-products/*`)
- **gRPC** on `:9095` — inter-service calls (cart price validation, order stock checks)

### Directory Structure

```
go/product-service/
├── cmd/server/
│   ├── main.go          # Starts REST + gRPC servers, signal handling
│   ├── config.go        # Env var loading
│   ├── deps.go          # Postgres, Redis connection helpers
│   └── routes.go        # Gin router + middleware stack
├── internal/
│   ├── handler/         # REST handlers (List, GetByID, Categories)
│   ├── service/         # Business logic + Redis caching
│   ├── repository/      # PostgreSQL queries (cursor/offset pagination, stock)
│   ├── model/           # Product, ProductListParams, ProductListResponse
│   ├── grpc/            # gRPC server implementation
│   │   └── server.go    # Implements ProductServiceServer
│   ├── metrics/         # Prometheus collectors (views, cache ops, gRPC)
│   └── validate/        # Param validation
├── migrations/
│   ├── 001_create_products.up.sql
│   ├── 001_create_products.down.sql
│   ├── 002_add_pagination_indexes.up.sql
│   └── 002_add_pagination_indexes.down.sql
├── seed.sql             # 20 products + smoke test widget
├── Dockerfile
└── go.mod
```

### REST Endpoints (unchanged from current)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | /products | No | List with pagination, filtering, search, sorting |
| GET | /products/:id | No | Single product by UUID |
| GET | /categories | No | Distinct category list |
| GET | /health | No | Liveness/readiness |
| GET | /metrics | No | Prometheus scrape |

### Middleware Stack (same pattern as ecommerce-service)

1. `gin.Recovery()`
2. `pkgmw.SecurityHeaders()`
3. `otelgin.Middleware("product-service")`
4. `middleware.Logging()`
5. `apperror.ErrorHandler()`
6. `middleware.Metrics()`
7. `middleware.CORS(cfg.AllowedOrigins)`

## Proto Definition

### Toolchain

- `go/buf.yaml` — module config, lint rules
- `go/buf.gen.yaml` — Go code generation config
- `go/proto/product/v1/product.proto` — service definition
- Generated output: `go/product-service/internal/pb/`

### Service Contract

```protobuf
syntax = "proto3";
package product.v1;

import "google/protobuf/timestamp.proto";

service ProductService {
  rpc GetProduct(GetProductRequest) returns (Product);
  rpc GetProducts(GetProductsRequest) returns (GetProductsResponse);
  rpc CheckAvailability(CheckAvailabilityRequest) returns (CheckAvailabilityResponse);
  rpc DecrementStock(DecrementStockRequest) returns (DecrementStockResponse);
  rpc InvalidateCache(InvalidateCacheRequest) returns (InvalidateCacheResponse);
}

message Product {
  string id = 1;
  string name = 2;
  string description = 3;
  int32 price = 4;           // cents
  string category = 5;
  string image_url = 6;
  int32 stock = 7;
  google.protobuf.Timestamp created_at = 8;
  google.protobuf.Timestamp updated_at = 9;
}

message GetProductRequest {
  string id = 1;
}

message GetProductsRequest {
  string category = 1;
  string query = 2;
  string sort = 3;
  int32 page = 4;
  int32 limit = 5;
  string cursor = 6;
}

message GetProductsResponse {
  repeated Product products = 1;
  int32 total = 2;
  int32 page = 3;
  int32 limit = 4;
  string next_cursor = 5;
  bool has_more = 6;
}

message CheckAvailabilityRequest {
  string product_id = 1;
  int32 quantity = 2;
}

message CheckAvailabilityResponse {
  bool available = 1;
  int32 current_stock = 2;
  int32 price = 3;          // current price in cents
}

message DecrementStockRequest {
  string product_id = 1;
  int32 quantity = 2;
}

message DecrementStockResponse {
  int32 remaining_stock = 1;
}

message InvalidateCacheRequest {}
message InvalidateCacheResponse {}
```

### gRPC Server Features

- **Reflection:** `reflection.Register(grpcServer)` — enables `grpcurl` debugging
- **Health:** `grpc_health_v1.RegisterHealthServer()` — K8s gRPC health probes
- **OTel interceptors:** `otelgrpc.UnaryServerInterceptor()`, `otelgrpc.StreamServerInterceptor()` — automatic Jaeger spans
- **User context:** Extract `user_id` from gRPC metadata where needed (DecrementStock, InvalidateCache)

## Database

### Separate Database

New `productdb` on the shared Postgres instance. The `postgres-initdb` ConfigMap creates it on first boot alongside `ecommercedb`.

### Migrations

Renumbered from ecommerce-service originals:
- `001_create_products.up.sql` — products table with `pgcrypto`, indexes on `category` and `price`
- `002_add_pagination_indexes.up.sql` — composite indexes for cursor pagination (`price,id`, `name,id`, `created_at DESC,id DESC`)

K8s Job: `go/k8s/jobs/product-service-migrate.yml` with `x-migrations-table=product_schema_migrations`.

### Seed Data

Same 20 products + smoke test widget from current `seed.sql`. Applied by the migration Job after `migrate up` (same pattern as current ecommerce-service).

## Changes to Ecommerce Service

### Remove

- `internal/handler/product.go` + tests
- `internal/service/product.go` + tests + benchmarks
- `internal/repository/product.go`
- `internal/model/product.go`
- `internal/validate/` product param validation
- Product routes from `cmd/server/routes.go`
- Product handler from `cmd/server/main.go` setup
- Migrations `001` and `005` (moved to product-service)

### Modify

- **Order worker** (`internal/worker/order_processor.go`): Replace direct `ProductRepo.DecrementStock()` and `ProductService.InvalidateCache()` calls with gRPC calls to product-service. Add gRPC client setup in `main.go`.
- **`cmd/server/main.go`**: Remove product repo/service/handler initialization. Add gRPC client connection to product-service for the order worker.
- **`go.mod`**: Add `google.golang.org/grpc` and protobuf dependencies.

### Keep (for now)

- Cart repository's JOIN against products table (products table stays in `ecommercedb` until Phase 2)
- Order repository's JOIN against products table (stays until Phase 3)
- `order_items.product_id` and `cart_items.product_id` foreign keys (removed in later phases)

**Transition strategy:** During Phase 1, the products table exists in BOTH databases:
- `productdb` — authoritative, owned by product-service. All writes (stock decrement) go here via gRPC.
- `ecommercedb` — read-only copy for cart/order JOINs. Populated by seed job, not written to at runtime.

This avoids breaking cart/order JOINs while keeping clean service ownership. The read-only copy is removed in Phases 2 and 3 when cart and order services switch to gRPC calls for product data.

## Kubernetes Manifests

### New Files

- `go/k8s/deployments/product-service.yml` — dual-port container (8095 REST, 9095 gRPC)
- `go/k8s/services/product-service.yml` — two named ports (http, grpc)
- `go/k8s/pdb/product-pdb.yml` — `maxUnavailable: 1`
- `go/k8s/hpa/product-hpa.yml` — 70% CPU, 1-3 replicas
- `go/k8s/jobs/product-service-migrate.yml` — migrations + seed

### Modified Files

- `go/k8s/ingress.yml` — add `/go-products` path rewrite to product-service:8095
- `go/k8s/configmaps/go-config.yml` — add `PRODUCT_GRPC_ADDR: product-service.go-ecommerce.svc.cluster.local:9095`
- `go/k8s/kustomization.yaml` — add new resources

### Security Context (same as all Go services)

```yaml
securityContext:
  runAsNonRoot: true
  runAsUser: 1001
  readOnlyRootFilesystem: true
  allowPrivilegeEscalation: false
  capabilities:
    drop: [ALL]
```

### Probes

- Readiness: HTTP GET `/health` on port 8095
- Liveness: HTTP GET `/health` on port 8095
- Resources: 64Mi/100m request, 256Mi/500m limit

## CI/CD Changes

### `ci.yml` Modifications

- Add `product-service` to Go lint matrix
- Add `product-service` to Go test matrix
- Add `product-service` to Docker build matrix
- Add `buf lint` step (runs on `go/proto/**` changes)
- Add `buf generate` step (ensures generated code is up to date)
- Add product-service migration test (new Postgres DB in CI)

### Docker

- `go/product-service/Dockerfile` — multi-stage build, same pattern as other Go services
- Build context: `go/` (for `../pkg` replace directive)
- Include `golang-migrate` binary for K8s Job

## Frontend Changes

- New env var: `NEXT_PUBLIC_GO_PRODUCT_URL`
  - Local: `http://localhost:8095`
  - Production: `https://api.kylebradshaw.dev/go-products`
- Update product API calls in:
  - `frontend/src/app/go/ecommerce/page.tsx` — product listing
  - `frontend/src/app/go/ecommerce/[productId]/page.tsx` — product detail
- Add to Vercel env vars before merge

## Verification

1. **REST:** `curl http://localhost:8095/products` returns product list
2. **gRPC:** `grpcurl -plaintext localhost:9095 list` shows `product.v1.ProductService`
3. **gRPC call:** `grpcurl -plaintext -d '{"id":"<uuid>"}' localhost:9095 product.v1.ProductService/GetProduct`
4. **gRPC health:** `grpcurl -plaintext localhost:9095 grpc.health.v1.Health/Check`
5. **Frontend:** Product listing and detail pages load from new service
6. **Traces:** Jaeger shows `product-service` spans for both REST and gRPC calls
7. **Order worker:** Place an order, verify stock decrements via gRPC call in Jaeger trace
8. **Metrics:** `/metrics` on :8095 shows product_views, cache_ops, gRPC server metrics
9. **CI:** All matrix jobs pass (lint, test, build, migration, buf lint)
10. **K8s:** `kubectl get pods -n go-ecommerce` shows product-service running
