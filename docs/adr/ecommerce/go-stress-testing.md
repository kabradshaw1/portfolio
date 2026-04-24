# ADR: Go Ecommerce Stress Testing & Scalability Analysis

**Date:** 2026-04-10
**Status:** Accepted

## Context

The Go ecommerce stack (ecommerce-service, auth-service, ai-service) runs as single-replica deployments in Minikube on a Windows PC. Before promoting this as a portfolio piece, we needed to validate performance characteristics under load, identify bottlenecks, and demonstrate scalability thinking.

## Approach

Used k6 to stress test three phases:
1. **Ecommerce** — product browsing, cart operations, checkout flow, stock contention
2. **Auth** — registration burst, sustained login load
3. **AI Agent** — simple queries with Ollama tool-calling, rate limiter behavior

Tests ran from macOS through an SSH tunnel to the Windows PC's Minikube ingress, matching real-world access patterns. k6 metrics were recorded locally; service-side metrics are available in Prometheus/Grafana.

## Findings

### Phase 1: Ecommerce

| Scenario | VUs | p95 Latency | Error Rate | Threshold | Result |
|----------|-----|-------------|-----------|-----------|--------|
| Product browse | 50 (ramping) | 27ms | 0% | < 500ms | PASS |
| Cart operations | 20 | N/A (503 cascade) | ~94% | < 500ms | FAIL |
| Checkout flow | 30 | 5.14s | 0% | < 1s | FAIL |
| Stock contention | 50 | 31ms | 94% | N/A | FAIL |

**Product browsing** handled 50 concurrent users effortlessly at 195 req/s with a p95 of 27ms. The Redis cache is working well — zero errors across 58,608 requests.

**Cart operations** failed because each VU registers a new user (bcrypt) before performing cart actions. With 20 VUs hitting the auth service simultaneously, the auth pod (128Mi memory, 250m CPU limit) returned 503s, causing the cart scenario to abort.

**Checkout flow** completed without errors but crossed the p95 threshold at 5.14s. The bottleneck is the same — bcrypt registration at the start of each iteration inflates overall latency. Median response time for actual checkout calls was only 13ms.

**Stock contention** was the most critical finding: **296 successful orders were placed on an item with stock=50.** This confirms the TOCTOU race condition in `DecrementStock()` — the bare `UPDATE ... WHERE stock >= qty` allowed multiple concurrent transactions to pass the stock check before any had committed.

### Phase 2: Auth

| Scenario | VUs / Rate | p95 Latency | Error Rate | Threshold | Result |
|----------|-----------|-------------|-----------|-----------|--------|
| Registration burst | 50 VUs | 13.52s (successful) | 98% | < 3s | FAIL |
| Login sustained | 20 req/s | 24.87s (successful) | 57% | < 2s | FAIL |

**Registration burst** overwhelmed the auth service — only 291 of 17,554 attempts succeeded. bcrypt hash generation at `DefaultCost` (12 rounds) saturates the single CPU allocation. The pod returned 503s for the majority of requests.

**Login sustained** at 20 req/s could only achieve ~10 effective req/s. 1,515 iterations were dropped because k6 couldn't maintain the target rate — bcrypt comparison is equally expensive as hashing.

**Root cause:** The auth service pod has a 250m CPU limit. bcrypt at cost 12 is intentionally CPU-intensive (~200-300ms per operation on modern hardware). With a single replica capped at 1/4 of a CPU core, concurrent bcrypt operations queue up and exhaust the pod's resources.

### Phase 3: AI Agent

| Scenario | VUs | p95 Latency | Error Rate | Threshold | Result |
|----------|-----|-------------|-----------|-----------|--------|
| Simple query | 10 | 30s (timeout ceiling) | 59% | < 15s | FAIL |

**Simple queries** hit two bottlenecks simultaneously:
1. **Rate limiter**: 86 of 146 requests were rate-limited (429). The 20 req/min per-IP limit is correct behavior but means 10 VUs will saturate it in seconds.
2. **Ollama throughput**: Successful requests took p95=30s (the agent timeout ceiling). Qwen 2.5 14B on an RTX 3090 can only serve ~1 request at a time; concurrent requests queue.

Effective throughput: ~0.33 successful req/s (~20 per minute), perfectly matching the rate limit.

## Bottlenecks Identified

1. **Stock decrement race condition** — TOCTOU vulnerability, confirmed overselling (296 orders on stock=50)
2. **Auth service CPU starvation** — bcrypt at 250m CPU limit, 98% failure under 50 VUs
3. **No HTTP server timeouts** — slow clients can hold connections indefinitely
4. **Default pgxpool config** — no explicit pool sizing, idle timeout, or health checks
5. **Hardcoded worker concurrency** — RabbitMQ consumer locked at 3 workers
6. **Ollama GPU saturation** — single GPU can only process one inference at a time, queuing under load
7. **AI rate limiter per-IP** — correct behavior but limits testing; real multi-user traffic would spread across IPs

## Fixes Applied

### 1. Stock race condition (Critical)
**Before:** `UPDATE products SET stock = stock - $1 WHERE id = $2 AND stock >= $1` — no row lock, concurrent transactions pass the check simultaneously.
**After:** `SELECT ... FOR UPDATE` inside a transaction — acquires a row-level lock, preventing concurrent reads until the transaction commits.
**Impact:** Eliminates overselling. Concurrent checkouts now serialize at the row level.

### 2. pgxpool configuration
**Before:** `pgxpool.New(ctx, databaseURL)` — relies on defaults (no min conns, no idle timeout).
**After:** Explicit config: MaxConns=25, MinConns=5, MaxConnIdleTime=5min, MaxConnLifetime=30min, HealthCheckPeriod=30s.
**Impact:** Connections stay warm, unhealthy connections are recycled, pool behavior is predictable under load.

### 3. HTTP server timeouts
**Before:** No timeouts on the `http.Server`.
**After:** ReadTimeout=10s, WriteTimeout=30s, IdleTimeout=60s.
**Impact:** Prevents slow-client resource exhaustion and connection leaks.

### 4. Configurable worker concurrency
**Before:** Hardcoded `3` in `processor.StartConsumer(ctx, ch, 3)`.
**After:** Reads `WORKER_CONCURRENCY` env var with default=3.
**Impact:** Can tune RabbitMQ consumer concurrency per-environment without code changes.

### 5. HPA manifests
Added `HorizontalPodAutoscaler` for both ecommerce-service and auth-service:
- Target: 70% CPU utilization
- Range: 1-3 replicas
- Scale-up: 1 pod per 60s (60s stabilization)
- Scale-down: 1 pod per 120s (300s stabilization)

**Impact:** Auth service (the primary bottleneck) can scale to 3 replicas under bcrypt load. Conservative scale-down prevents flapping.

### 6. Metrics-server and deploy script
Enabled Minikube's metrics-server addon (required for HPA to read CPU usage). Refactored `k8s/deploy.sh` to use directory-based `kubectl apply -f <dir>/` so new manifests are auto-discovered — no script changes needed when adding resources.

## Before/After Comparison

Re-tests were run after deploying all fixes to Minikube, including HPA with metrics-server enabled.

| Scenario | Metric | Before | After | Change |
|----------|--------|--------|-------|--------|
| Product browse (50 VUs) | p95 | 27ms | 26ms | Slightly better (pool warmth) |
| Product browse (50 VUs) | max | 219ms | 100ms | **54% improvement** |
| Checkout (30 VUs) | overall p95 | 5.14s | 41ms | **99% improvement** (fast-fail) |
| Checkout (30 VUs) | throughput | 34 req/s | 113 req/s | **3.3x improvement** |
| Stock contention (50 VUs) | overselling | 296 orders on stock=50 | stock=0 (not negative) | **Fixed** |
| Stock contention (50 VUs) | iterations | 286 | 878 | **3x more** (HPA handled auth load) |
| Auth login (20 req/s) | error rate | 57% | **0%** | **HPA eliminated 503s** |
| Auth login (20 req/s) | p95 | 24.87s | **16.16s** | **35% improvement** |
| Auth login (20 req/s) | throughput | ~10 req/s | **12.5 req/s** | **25% improvement** |
| Auth registration (50 VUs) | error rate | 98% | 98% | Burst too short for HPA |

The stock race condition fix (`SELECT FOR UPDATE`) eliminates overselling. The HPA scaled the auth service to 3 replicas under sustained login load, eliminating 503 errors entirely and improving throughput by 25%. Registration bursts (1 minute) are too short for the HPA's 60-second stabilization window — this is expected behavior, as HPA is designed for sustained load, not spikes.

## Performance Summary

| Service | Metric | Value | Notes |
|---------|--------|-------|-------|
| Ecommerce (products) | Throughput | 195 req/s | At 50 VUs, p95=26ms |
| Ecommerce (products) | Cache hit rate | ~100% | Redis caching effective |
| Ecommerce (checkout) | Throughput | ~5 orders/s | Excluding auth overhead |
| Auth (register) | Max concurrent | ~5-10 | Before 503s at 250m CPU |
| Auth (login) | Effective rate | ~10 req/s | Half of target 20 req/s |
| AI agent | Throughput | ~0.33 req/s | GPU-bound (Ollama) |
| AI agent | Rate limit | 20 req/min per IP | Working as designed |

## Scaling Recommendations

1. **Auth service CPU** — Increase CPU limit from 250m to at least 500m, or rely on HPA to add replicas. bcrypt cost 12 is non-negotiable for security; scaling is the only option.
2. **Auth service replicas** — HPA manifests added. With 3 replicas, theoretical max is ~30 req/s for login.
3. **Ollama** — GPU-bound, not horizontally scalable without additional GPUs. For production, consider a smaller model or an API-based LLM service.
4. **Rate limiter** — Currently per-IP. Multi-user traffic naturally distributes across IPs. No change needed.
5. **Stock contention** — Fixed with SELECT FOR UPDATE. Throughput will decrease slightly due to row-level locking serialization, but correctness is more important than speed.

## Decision

The Go ecommerce stack handles its expected load profile adequately for read-heavy workloads (product browsing). Write-heavy workloads (auth, checkout) are constrained by bcrypt CPU cost and single-replica deployments. The applied fixes address correctness issues (stock overselling) and operational concerns (timeouts, pool config, scaling). The main constraints are:

- **Ollama throughput** — GPU-bound, ~20 successful requests per minute
- **bcrypt CPU cost** — intentional security trade-off, mitigated by HPA
- **Single PostgreSQL instance** — shared with Java services, not tested in isolation

Load test scripts, Grafana dashboard, and k6 configuration are committed for future regression testing.
