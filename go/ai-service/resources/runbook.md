# How This Portfolio Works

A multi-language portfolio monorepo demonstrating production-grade engineering across Go, Python, Java, and Next.js.

## Services at a Glance

**Go microservices** (shared `go/pkg/` module):
- `auth-service` (8091) — JWT register/login/refresh, PostgreSQL (`authdb`)
- `product-service` (REST 8095, gRPC 9095) — product catalog CRUD, `productdb`
- `cart-service` (gRPC 9096) — cart CRUD + saga reserve/release/clear, `cartdb`
- `order-service` (REST 8092) — saga orchestrator, `orderdb`
- `payment-service` (REST 8098, gRPC 9098) — Stripe checkout + outbox, `paymentdb`
- `ai-service` (8093) — MCP server: agent loop, tool registry, RAG bridge
- `analytics-service` (8094) — Kafka consumer, streaming order/cart/view metrics

**Python AI services** (FastAPI):
- `ingestion` — PDF upload, parse, chunk with LangChain, embed with nomic-embed-text, store in Qdrant
- `chat` — question embed, Qdrant search, RAG prompt, SSE stream
- `debug` — code indexing + LLM agent loop with tool execution

**Java services** (Spring Boot, Gradle multi-project):
- `task-service`, `activity-service`, `notification-service`, `gateway-service` (GraphQL)

**Frontend:** Next.js + TypeScript + shadcn/ui + Apollo GraphQL. Served on Vercel at `kylebradshaw.dev`.

## Checkout Saga

Order-service is the saga orchestrator. The saga progresses through four steps recorded in `orders.saga_step`:

1. `reserve_stock` — order-service calls product-service (gRPC) to reserve inventory.
2. `reserve_cart` — order-service calls cart-service (gRPC) to lock cart items.
3. `charge_payment` — order-service calls payment-service (gRPC) to create a Stripe charge.
4. `confirm` — all participants succeeded; order status moves to `confirmed`.

On any participant failure the orchestrator publishes a compensating command over RabbitMQ to roll back upstream steps. QA uses a separate `/qa` vhost for queue isolation.

## This AI Service as an MCP Server

`ai-service` exposes three MCP primitive categories:

- **Tools** — 12 tools: `investigate_my_order`, `compare_products`, `recommend_with_rationale`, plus 9 primitive ecommerce/RAG tools (list_products, get_product, search_rag, etc.). Registered in `main.go`, cached via Redis wrapper.
- **Resources** — read-only URIs the LLM can fetch without a tool call: `catalog://categories`, `catalog://featured`, `catalog://product/{id}`, `user://orders`, `user://cart`, `runbook://how-this-portfolio-works`, `schema://ecommerce`.
- **Prompts** — server-provided prompt templates the client can render with arguments.

Agent loop: ReAct pattern, 8 steps max, 90 s timeout. Streams SSE events (`tool_call`, `tool_result`, `tool_error`, `final`, `error`) from `internal/http/chat.go`.

RAG bridge: `go/ai-service/internal/tools/clients/rag.go` calls Python chat at `/search` + `/chat` and ingestion at `/collections`. 30 s timeout, circuit breaker, OTel trace propagation.

## Observability

Three pillars in the `monitoring` namespace (Minikube):
- **Prometheus** — service metrics, alert rules, dashboards in `k8s/monitoring/configmaps/`
- **Loki + Promtail** — structured log aggregation from all pods
- **Jaeger** — distributed traces via OTel/OTLP. RabbitMQ and Kafka messages carry W3C trace context in headers.

## Infrastructure

- **Dev:** Mac (code editing) + Colima Docker
- **Backend:** Debian 13 running Minikube (RTX 3090 for Ollama, Qwen 2.5 14B)
- **Routing:** NGINX Ingress → Cloudflare Tunnel → `api.kylebradshaw.dev`
- **CI/CD:** GitHub Actions builds GHCR images, deploys via SSH to Minikube. QA namespace mirrors prod on every `qa` branch push.
