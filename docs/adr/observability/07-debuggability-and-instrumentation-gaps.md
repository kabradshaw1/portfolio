# Debuggability and Instrumentation Gaps

- **Date:** 2026-04-22
- **Status:** Accepted

## Context

After deploying the decomposed ecommerce services (order, cart, payment, product) with the monitoring stack (ADRs 01-06), a checkout failure in QA exposed several instrumentation gaps. The failure — a gRPC mTLS handshake error between order-service and payment-service — was ultimately diagnosable, but the debugging process required extensive manual `kubectl exec`, `kubectl logs | grep`, and `openssl` commands that should have been answerable from Grafana and Loki.

This ADR documents the gaps found, the decisions made to close them, and the CI/CD issues discovered along the way.

### The debugging timeline

1. **Initial symptom:** "Order failed. Please try again." on the QA frontend.
2. **Loki query** (worked on third attempt — first two failed due to port-forwarding issues): Found `"create payment failed, compensating"` with `"transport: authentication handshake failed"` in order-service logs.
3. **Root cause 1 — mTLS server code missing:** The payment-service gRPC server wasn't using `tlsconfig.ServerTLS()` when `TLS_CERT_DIR` was set. Fixed by adding mTLS opt-in to the server (same pattern as cart, product, auth services).
4. **Root cause 2 — stale image:** After fixing the code, the error persisted because CI's change detection (`git diff HEAD~1`) only checked the most recent commit. The mTLS fix was in an earlier commit that CI never diffed. The payment-service image was never rebuilt.
5. **Root cause 3 — unique constraint:** Once the image deployed, Stripe checkout sessions succeeded but `UpdateStripeIDs` failed on a `UNIQUE` constraint for `stripe_payment_intent_id`. The blanket unique constraint didn't allow multiple pending payments (needed for saga retries).
6. **Root cause 4 — no gRPC call timeout:** The order-service's gRPC call to payment-service had no context deadline. When the TLS handshake hung, the saga blocked forever with zero logging.

Each root cause was only discoverable through manual investigation. The observability stack had the data but lacked the instrumentation to surface it without `kubectl`.

## Decision

### 1. Loki access method: kubectl exec over port-forwarding

Port-forwarding Loki from Mac through SSH was unreliable (double-hop drops). Using `kubectl exec -n monitoring loki-0 -- wget -qO-` to query Loki directly from inside the pod works consistently. All CLAUDE.md Loki recipes were updated to use this pattern.

**Trade-off:** Longer command lines with URL-encoded LogQL. Mitigated by documenting a URL-encoding reference table and a reusable Python JSON parser.

### 2. CRI-wrapped JSON parser

Loki returns logs in CRI format where the actual service JSON is nested inside a `log` field:
```json
{"log": "{\"time\":\"...\",\"level\":\"ERROR\",\"msg\":\"...\"}\\n", "stream": "stdout"}
```

The parser must `json.loads(outer['log'])` to get structured fields. This caused multiple failed parsing attempts before being identified and documented. The correct parser is now codified in the `/debug-observability` skill.

### 3. Service-layer logging for cart and product services

Cart-service and product-service service layers had zero `slog` calls — operations succeeded or failed silently. Added logging at decision points:

- **Cart:** AddItem (success + validation failure), UpdateQuantity, RemoveItem — all with userID/productID/itemID context
- **Product:** Cache miss paths (List, GetByID), cache invalidation with key count

**Principle:** Log writes and decisions, not reads. Include IDs needed for saga correlation.

### 4. Payment-service Prometheus metrics

Payment-service had zero custom metrics. Added:
- `payment_webhook_events_total` (event_type, outcome)
- `payment_created_total` (status)
- `payment_outbox_publish_total` (outcome)
- `payment_outbox_lag_seconds` (histogram)

### 5. AMQP trace injection in payment outbox

The payment-service outbox poller was the only RabbitMQ producer that didn't call `tracing.InjectAMQP()`. Added it to maintain the trace chain through payment saga events.

### 6. gRPC client interceptor

Created a shared `go/pkg/grpcmetrics/` package with a unary client interceptor that records:
- `grpc_client_requests_total` (target, method, status code)
- `grpc_client_request_duration_seconds` (target, method)
- `slog.ErrorContext` on every non-OK result

Wired into all outbound gRPC clients in order-service (4 clients) and cart-service (2 clients). This would have immediately shown "order→payment calls are all status=Unavailable, duration=30s" during the original debugging.

**Alternative considered:** Using `otelgrpc.NewClientHandler()` for automatic instrumentation. Rejected because it doesn't log errors to slog (only traces) and doesn't label by target service name.

### 7. Saga step duration metric

Added `saga_step_duration_seconds` histogram (labels: step, outcome) to the saga orchestrator's `Advance()` method. Timing wraps the switch-case handler calls; waiting/terminal states return early without being timed.

This would have shown "STOCK_VALIDATED step is taking 30s+" immediately in the dashboard.

### 8. Startup build version logging

Created `go/pkg/buildinfo/` with `Version` and `GitSHA` variables set via `-ldflags` at Docker build time. All 7 Go services call `buildinfo.Log()` at startup, making build identity queryable in Loki:

```
{app="go-payment-service"} |= "service started" | json | gitSHA="abc1234"
```

This replaces the manual `kubectl get pods -o jsonpath` image digest check.

### 9. Stripe API call logging

Added before/after logging with duration around `CreateCheckoutSession` and `Refund` calls in payment-service. No new metrics — the gRPC interceptor captures overall call duration from the caller's side. This logging captures Stripe's specific contribution.

### 10. CI change detection: HEAD~5 over HEAD~1

CI's image build decision used `git diff HEAD~1` to check for changes. Multi-commit pushes (common when batching doc + code changes) caused CI to skip image rebuilds because only the final commit was diffed. Changed to `HEAD~5` with `fetch-depth: 10` to catch changes across recent commits.

**Trade-off:** Slightly more image rebuilds (rebuilds when any of the last 5 commits touched the service, even if only the oldest one did). This is acceptable — unnecessary rebuilds are cheap, stale images are expensive.

### 11. Payment idempotency and constraint fix

- Changed payment `CREATE` to `INSERT ... ON CONFLICT (order_id) DO UPDATE` so saga retries reuse the existing payment record.
- Replaced the blanket `UNIQUE` constraint on `stripe_payment_intent_id` with a partial unique index: `WHERE stripe_payment_intent_id IS NOT NULL AND stripe_payment_intent_id != ''`. This allows multiple pending payments with empty intent IDs while preventing duplicate real Stripe intents.

### 12. Grafana dashboard updates

Added panels to the existing go-services dashboard:
- **Decomposed Services RED:** Per-service request rate, error rate, p95 latency for order/cart/payment/product
- **Saga & Payment Health:** Order status, circuit breaker state, webhook throughput, outbox publish rate
- **gRPC Client Health:** Outbound request rate, error rate, latency by target service
- **Saga Step Duration:** p95 duration per saga step
- **Certificate Expiry:** Days until cert-manager certificates expire

### 13. Alert rules for decomposed services

Extended Grafana alert rules: error rate and p95 latency alerts for cart, payment, product, and auth services (same thresholds as existing order-service alerts). Added circuit breaker operational alert (fires when any breaker is OPEN for 2+ minutes).

### 14. Skills extraction

Extracted debugging recipes and service scaffolding checklist from CLAUDE.md into project-local skills:
- `/debug-observability` — Loki/Jaeger/Grafana debugging recipes with correct CRI JSON parser
- `/scaffold-go-service` — Full 15-item checklist with observability boilerplate, K8s manifests, CI integration

This reduced CLAUDE.md by ~135 lines while making the knowledge invocable on-demand.

## Consequences

**Positive:**
- Every gRPC call between services is now metered and logged on failure — silent hangs are impossible with the 30s timeout + interceptor
- Build identity is queryable from Loki — "is my fix deployed?" no longer requires kubectl
- Saga step timing reveals bottlenecks without manual log timestamp comparison
- CI catches image staleness across multi-commit pushes
- Dashboard panels surface all new metrics without requiring manual PromQL
- Skills package operational knowledge for consistent reuse

**Trade-offs:**
- Additional log volume from service-layer logging and gRPC interceptor (mitigated by logging decisions/errors only, not reads)
- More Prometheus time series from per-target gRPC metrics (bounded by number of services, not requests)
- `HEAD~5` may trigger unnecessary image rebuilds when only docs changed in the window (cheap compared to stale images)
- Two new shared packages (`grpcmetrics`, `buildinfo`) that all services depend on — changes to these packages trigger rebuilds across all services

**Remaining gaps:**
- No PostgreSQL query tracing (manual spans) — useful but significant effort for incremental value
- No RabbitMQ queue depth metrics — requires management API scraping
- Saga DLQ depth alerts — deferred until DLQ metrics are scraped
- Promtail `trace_id` field fix for Python services — deployed but not yet verified end-to-end
