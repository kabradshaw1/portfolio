# Ecommerce Service Decomposition with gRPC & Saga

## Context

Kyle is applying for Go microservice developer positions. Job listings emphasize async operations (RabbitMQ), observability, CI/CD, security, gRPC, REST, and Kubernetes. The current `ecommerce-service` handles products, cart, and orders in a single service — three distinct bounded contexts with different scaling profiles. Decomposing into separate services with gRPC inter-service communication and a saga-based checkout flow strengthens the portfolio across all target areas simultaneously.

## Architecture

### Service Decomposition

The monolithic `ecommerce-service` splits into three services. Each exposes REST for frontend traffic (via NGINX ingress) and a gRPC server for inter-service communication. Cross-cutting concerns (auth, rate limiting, CORS, metrics, logging) remain in shared middleware (`go/pkg/`), applied per-service.

```
            NGINX Ingress (path-based routing)
           /            |              \
       /products     /cart          /orders
          |            |               |
    ┌──────────┐  ┌──────────┐  ┌──────────┐
    │ Product  │  │   Cart   │  │  Order   │
    │ Service  │  │ Service  │  │ Service  │
    │ REST+gRPC│  │ REST+gRPC│  │ REST+gRPC│
    └──────────┘  └────┬─────┘  └──┬───┬───┘
                  gRPC │            │   │
                       ▼            │   │
                  Product Svc   gRPC│  RabbitMQ
                 (price check)      ▼   (saga)
                                Cart Svc
                               (reserve/release)
```

### Data Layer

Database-per-service using separate databases on the shared Postgres instance:
- `productdb` — products, categories
- `cartdb` — cart_items
- `orderdb` — orders, order_items

Same logical isolation as enterprise separate clusters, pragmatic for a portfolio project's infra constraints.

### gRPC Contracts

Proto files in `go/proto/`, code generated into each service's `internal/pb/`. Toolchain: `buf`.

**`product.proto`**
```protobuf
service ProductService {
  rpc GetProduct(GetProductRequest) returns (Product);
  rpc GetProducts(GetProductsRequest) returns (GetProductsResponse);
  rpc CheckAvailability(CheckAvailabilityRequest) returns (CheckAvailabilityResponse);
}
```

**`cart.proto`**
```protobuf
service CartService {
  rpc GetCart(GetCartRequest) returns (Cart);
  rpc ReserveItems(ReserveItemsRequest) returns (ReserveItemsResponse);
  rpc ReleaseItems(ReleaseItemsRequest) returns (ReleaseItemsResponse);
  rpc ClearCart(ClearCartRequest) returns (ClearCartResponse);
}
```

**`order.proto`**
```protobuf
service OrderService {
  rpc GetOrder(GetOrderRequest) returns (Order);
  rpc ListOrders(ListOrdersRequest) returns (ListOrdersResponse);
}
```

All gRPC services include:
- Reflection (debugging with grpcurl)
- gRPC health checking (K8s gRPC health probes)
- OTel interceptors (client + server) for automatic Jaeger tracing
- User context propagation via gRPC metadata (`user_id` from JWT, trusted internally)

### Checkout Saga (Orchestration)

The order-service orchestrates checkout via RabbitMQ commands/events.

**Happy path:**
1. Client `POST /orders/checkout`
2. Order service creates order (status: `PENDING`)
3. → RabbitMQ: `reserve.items` command
4. Cart service reserves items → replies: `items.reserved`
5. → gRPC: `product-service.CheckAvailability` (stock validation)
6. Order confirms (status: `CONFIRMED`)
7. → RabbitMQ: `clear.cart` command
8. Cart service clears cart → replies: `cart.cleared`
9. Order finalizes (status: `COMPLETED`)

**Compensation (failure at step 5 or 6):**
1. Order marked `FAILED`
2. → RabbitMQ: `release.items` command
3. Cart service releases reserved items → replies: `items.released`

**RabbitMQ topology:**
- Exchange: `ecommerce.saga` (topic, durable)
- Command queues: `saga.cart.commands`, `saga.order.events`
- Dead letter exchange: `ecommerce.saga.dlx`
- Dead letter queue: `ecommerce.saga.dlq`
- Retry: 3 attempts, exponential backoff (1s, 5s, 25s), per-message TTL with retry exchange

**Saga state machine** stored in the orders table (current step + status). On service restart, queries pending orders and resumes from last known state.

**Kafka integration** unchanged — order completion publishes to `ecommerce.orders` for analytics-service. This gives the portfolio both saga orchestration (critical flows) and event choreography (non-critical flows).

### Observability Additions

- gRPC spans via OTel interceptors (all inter-service calls in Jaeger)
- Saga metrics: `saga_steps_total` (counter: step, outcome), `saga_duration_seconds` (histogram), `saga_dlq_messages_total`
- Structured logs at each saga state transition: `traceID`, `orderID`, `sagaStep`
- Full checkout shows as a Jaeger waterfall: REST → order creation → RabbitMQ → cart reservation → gRPC stock check → confirmation

## Phased Implementation

### Phase 1: Product Service Extraction
- Set up `buf` toolchain, define `product.proto`
- Create `go/product-service/` — extract product/category handlers, repository, service layers
- Dual server: REST (`:8095`) + gRPC (`:9095`) + health + metrics
- gRPC reflection, health checking, OTel interceptors
- Own database (`productdb`) with golang-migrate migrations
- K8s manifests: deployment, service, ingress path, HPA, PDB
- CI pipeline: add product-service to lint/test/build matrix
- Remove product logic from ecommerce-service (ecommerce-service continues running with cart + order logic)
- Update frontend routing: product paths → new product-service, cart/order paths → ecommerce-service (unchanged)
- **Exit criteria:** frontend product pages work, gRPC calls work via grpcurl, traces visible in Jaeger

### Phase 2: Cart Service Extraction
- Define `cart.proto`
- Create `go/cart-service/` — extract cart handlers, repository, service
- gRPC client to product-service for price validation
- Own database (`cartdb`) with migrations
- K8s manifests, CI pipeline updates
- Remove cart logic from ecommerce-service (ecommerce-service continues running with order logic only)
- Update frontend routing: cart paths → new cart-service
- **Exit criteria:** cart operations work, Jaeger traces show cart→product gRPC call

### Phase 3: Order Service + Saga
- Define `order.proto`
- Create `go/order-service/` with saga orchestrator
- RabbitMQ saga topology (exchanges, queues, DLX, DLQ, retry)
- Saga state machine (resumable on crash)
- Cart service saga command handlers (reserve, release, clear)
- gRPC to product-service for stock validation
- Compensation flow
- Saga Prometheus metrics
- Kafka publish on order completion
- K8s manifests, CI pipeline updates
- Retire old ecommerce-service
- **Exit criteria:** full checkout works, Jaeger shows saga waterfall, DLQ catches failures

## GitHub Issues (Future Enhancements)

1. **Auth-service gRPC + token revocation** — `CheckToken` gRPC endpoint, called by other services
2. **RabbitMQ DLQ replay tooling** — admin endpoint to inspect and replay dead-lettered messages
3. **Proto contract testing** — `buf breaking` in CI to catch backwards-incompatible changes
4. **Integration tests for async flows** — RabbitMQ saga and Kafka consumer test coverage
5. **Graceful shutdown orchestration** — drain gRPC connections, finish in-flight saga steps
6. **mTLS between services** — cert-manager for zero-trust internal communication

## Enterprise vs. Portfolio Justifications

| Decision | Portfolio | Enterprise | Why it's fine |
|----------|-----------|------------|---------------|
| Shared Postgres instance (3 DBs) | Single instance, separate databases | Separate DB clusters per service | Same logical isolation, different operational scale |
| Single replica defaults | 1 replica + HPA | Multi-replica baseline | HPA is configured, demonstrates the scaling pattern |
| Shared JWT secret | `JWT_SECRET` env var | Service mesh / mTLS + OIDC | Perimeter auth is valid; mTLS noted as future enhancement |
| `buf` local toolchain | Local `buf generate` | Buf Schema Registry + CI enforcement | Same workflow, registry is team-scale tooling |
| Manual DLQ inspection | Logs + queue browsing | Dedicated DLQ dashboard + auto-alerting | Alerting exists via Grafana; replay tooling is a future issue |

## Verification

After each phase:
1. Frontend E2E: product browsing, cart operations, checkout flow work end-to-end
2. gRPC: `grpcurl` against each service's gRPC port confirms reflection and method invocation
3. Traces: Jaeger shows inter-service gRPC calls and saga step waterfall
4. Metrics: `/metrics` endpoints expose new counters/histograms, Grafana dashboards updated
5. CI: all quality checks pass (lint, test, build) with new services in the matrix
6. K8s: `kubectl get pods -n go-ecommerce` shows all services healthy
7. DLQ: intentionally fail a checkout step, confirm message lands in DLQ
