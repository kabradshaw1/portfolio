# Redis tab on /database

## Context

The `/database` portfolio page currently has three tabs — PostgreSQL, NoSQL, Vector. The NoSQL tab is a one-paragraph stub for MongoDB that already references Redis as a side note. Redis is actually used in eight distinct patterns across the codebase (caching, rate limiting, idempotency, JWT denylist, analytics windowing, Spring `@Cacheable`, etc.) and deserves first-class treatment alongside the other database technologies. This spec adds a Redis tab and tightens up the surrounding tab structure.

## Goals

1. Add a first-class **Redis** tab to `/database`, positioned between PostgreSQL and the document-store tab.
2. Rename the existing **NoSQL** tab to **MongoDB**, since the content has only ever covered MongoDB.
3. Tell the full Redis story across five pillars: caching, rate limiting, HTTP idempotency, JWT denylist, time-windowed analytics — plus a closing operational-discipline paragraph that ties them together.
4. Keep the new tab visually and structurally consistent with the existing PostgresTab (sticky TOC, `PillarSection` components, ADR/spec links).

## Non-goals

- New backend code. This is a frontend documentation change against an existing architecture.
- Changes to MongoDB/Vector tab content (beyond the rename).
- Benchmarks or performance numbers — none currently exist for the Redis flows, and we're not going to fabricate them.

## Architecture

Frontend-only. Edits scoped to:

- `frontend/src/components/database/tabs.ts` — extend `DatabaseTab` type, reorder/rename entries.
- `frontend/src/app/database/page.tsx` — add a render branch for the new tab key.
- `frontend/src/components/database/NoSqlTab.tsx` → `MongoDbTab.tsx` — rename file + component + `data-testid`. Content-light; just a name and prose touch-up to remove the "Redis as a side note" sentence (Redis now has its own tab).
- `frontend/src/components/database/RedisTab.tsx` — **new** component, modeled after `PostgresTab.tsx` (TOC items, `PillarSection`s, mobile chip / desktop sidebar layout).
- `frontend/e2e/mocked/database-page.spec.ts` — update tab-label assertions; add Redis tab activation + pillar/anchor coverage.

The Redis tab follows the same shape as `PostgresTab`: a TOC list at the top of the file, a `<div data-testid="redis-tab">` with mobile chips, a `space-y-16` column of `PillarSection`s, and a sticky sidebar for desktop.

## Tab structure

Final tab order: **PostgreSQL · Redis · MongoDB · Vector** (`postgres → redis → mongodb → vector`).

`DatabaseTab` becomes `"postgres" | "redis" | "mongodb" | "vector"`. The `databaseTabs` array reorders to match. The MongoDB tab keeps the same "stub points at /java" shape it has today; only the visible label and `data-testid` change.

## Pillars (RedisTab)

Sticky TOC items, in order:

```
caching → rate-limiting → idempotency → denylist → analytics
```

### 1. `id="caching"` — Read-Side Caching

Narrative: caching layered on top of a healthy primary store, never as a load-bearing dependency. Two languages, two styles.

Bullets:

- Go `Cache` interface (`go/ai-service/internal/cache/cache.go`): bytes in, bytes out — callers handle their own serialization, so the cache stays transport-agnostic. `RedisCache` for prod, `NopCache` for dev / when Redis is unavailable.
- Per-prefix namespacing (e.g., `ai:tools:<tool>:<args-hash>`, `product:catalog:<id>`); makes key inspection in `redis-cli` trivial and bounds blast radius if a prefix needs invalidation.
- AI tool-result cache (`go/ai-service/internal/tools/cached.go`) memoizes deterministic tool calls so an agent loop doesn't pay Redis latency on identical sub-queries.
- Product catalog cache (`go/product-service/internal/service/product.go`) — short-TTL read-through for hot product reads.
- Java: Spring `@Cacheable` on `project-stats` and `project-velocity` queries in `task-service` (`AnalyticsService.java`). Declarative, swappable via `CacheConfig`. Same Redis instance backs Spring's cache abstraction.
- Cross-cutting: every Redis call goes through a circuit breaker (`gobreaker`); on trip, calls fail open (cache miss for reads, skip-write for writes). OTel Redis spans (`tracing.RedisSpan`) capture every operation for end-to-end traces.

### 2. `id="rate-limiting"` — Per-IP Rate Limiting

Narrative: a fixed-window limiter is the simplest correct thing. `INCR` + `EXPIRE` per IP per window — no token bucket, no Lua, no surprises. Used wherever the gateway-level CDN rules don't reach (auth, ecommerce, AI agent).

Bullets:

- Implementation: `go/ai-service/internal/guardrails/ratelimit.go` (canonical) and per-service middleware in `auth`, `order`, `cart`.
- Pattern: `INCR key`; if value is 1, `EXPIRE key window`; reject when value exceeds `max`. Returns `Retry-After: <seconds-until-window-end>`.
- Per-service prefixes (`ai:ratelimit:<ip>`, `auth:ratelimit:<ip>`, etc.) so a single Redis instance serves multiple services without collision.
- Fail-open: if the Redis breaker is open, the limiter allows traffic. Better to skip rate limiting briefly than to 503 every request — the upstream load balancer + the Postgres connection pool ceiling are the next-layer brakes.
- Trade-off note: fixed-window has a known burst-at-boundary edge case. Documented in the file; acceptable given the protection profile.

### 3. `id="idempotency"` — HTTP Idempotency-Key

Narrative: POST `/orders` and POST `/cart` need to be retry-safe — flaky networks, saga compensations, and the user smashing the buy button shouldn't create duplicate orders. RFC-style `Idempotency-Key` middleware backs both.

Bullets:

- Middleware: `go/order-service/internal/middleware/idempotency.go`, mirrored in `cart-service`.
- Two-phase Redis entry per key:
  - **In-flight marker** (`PROCESSING`, 30 s TTL) set on the first request — concurrent duplicates with the same key get a 409 instead of racing.
  - **Completed-response cache** (status code + body, 24 h TTL) written when the handler returns — subsequent retries with the same key replay the original response byte-for-byte.
- Key shape: `idempotency:<route>:<idempotency-key-header>` keyed under a per-route prefix so accidental key reuse across routes can't replay the wrong response.
- `responseCapture` wraps `gin.ResponseWriter` so the handler's body is buffered for caching without changing the handler signature.
- Why this matters in this portfolio specifically: the saga orchestrator retries on transient gRPC failures, and idempotency is the only thing standing between a network blip and a duplicate charge.

### 4. `id="denylist"` — JWT Token Denylist

Narrative: stateless JWTs are great for read-side performance but terrible at "the user just clicked logout." A small Redis denylist closes the gap without sacrificing the stateless-verification win — verification adds one `EXISTS` check, revocation is one `SET … EX <remaining-ttl>`.

Bullets:

- Implementation: `go/auth-service/internal/service/token_denylist.go`.
- Key shape: `auth:denied:<sha256-of-token>` with TTL matching the token's remaining lifetime. Hashing avoids leaking the token; aligned TTL avoids unbounded growth.
- Logout flow: handler calls `denylist.Revoke(token, ttl)`; subsequent requests presenting the same token hit the deny check first and 401.
- Graceful degradation: a `nil` Redis client is a valid construction — revocation becomes a no-op and tokens expire naturally. Used in dev / unit tests.
- Trade-off note: introduces a per-request Redis hop on every authenticated call. Acceptable for the use case; a future optimization is a bloom filter in front of the denylist if RPS climbs.

### 5. `id="analytics"` — Time-Windowed Analytics

Narrative: the analytics service runs a Kafka consumer over `ecommerce.orders` / `ecommerce.cart` / `ecommerce.views` and writes time-bucketed counters into Redis. Each metric has an explicit TTL — no unbounded growth, no hand-rolled cleanup cron.

Bullets:

- Store: `go/analytics-service/internal/store/redis.go`.
- Three metrics, three retention windows:
  - **Revenue** (`analytics:revenue:<hour>`) — 48 h TTL, hourly granularity.
  - **Trending products** (`analytics:trending:<hour>`) — 2 h TTL, sorted-set scored by view/order signals.
  - **Cart abandonment** (`analytics:abandonment:<hour>` + `analytics:abandonment:users:<user>`) — 24 h TTL, hash + per-user marker for dedup.
- Hourly bucket keys (`2006-01-02T15` Go time layout) make range queries a simple key sweep over the last N hours; no SCAN cursor or sorted-set range needed for the common case.
- Trade-off note: sub-hour granularity isn't supported; deliberate choice to keep keys legible and the dashboard math obvious. If finer resolution is needed later, the bucket layout is a one-line change.
- Operational signals: standard Redis breaker / OTel span / fail-open story applies — analytics is best-effort, never blocks the write path.

## Closing operational note

A short paragraph at the bottom of the tab (under the last pillar, no separate `PillarSection`) ties the patterns together:

> Every Redis call in this portfolio is wrapped in a circuit breaker that fails open, every key is namespaced and TTL'd, and every operation gets an OpenTelemetry span via `tracing.RedisSpan` so a single trace shows the cache hit between the HTTP handler and the Postgres query. The discipline isn't visible in any single use case — it's the reason none of them have ever taken down the request path.

## Testing

- `npx tsc --noEmit` clean.
- `npx eslint src/` — no new errors (existing warnings allowed).
- `npx next build` — succeeds.
- E2E (`frontend/e2e/mocked/database-page.spec.ts`):
  - Tab labels: PostgreSQL · Redis · MongoDB · Vector.
  - Click "Redis" → `[data-testid="redis-tab"]` visible; pillar headings render; anchor IDs `caching`, `rate-limiting`, `idempotency`, `denylist`, `analytics` are present; sidebar TOC labels are in order.
  - Click "MongoDB" → existing `[data-testid="mongodb-tab"]` (renamed) visible, `/java` link still points correctly.
- Manual: load `/database` in the dev server, click through all four tabs, verify TOC chip + sidebar render correctly on the Redis tab.

## Delivery

- Branch: `agent/feat-redis-database-tab`, off `qa` (not `main` — recent merges including the Redis-using infrastructure docs are not yet on main and this PR is intentionally not main-bound until other blockers clear).
- Worktree: `.claude/worktrees/agent-feat-redis-database-tab/`.
- Single PR to `qa`. Frontend-only diff; small enough for a one-shot review.
- No Vercel env-var changes — no new `NEXT_PUBLIC_*` vars introduced.

## Risks & mitigations

- **MongoDB tab rename surface area.** Tab key change ripples through the renderer, the test IDs, and the Playwright spec. Mitigation: do all three in the same commit; verify with `grep` for any other consumer of `"nosql"` before pushing.
- **Pillar copy drift.** The bullets reference specific file paths (`cached.go`, `idempotency.go`, etc.). Mitigation: every reference is verified against the worktree at write time, and links use stable file paths rather than line ranges.
- **Tab strip overflow.** Four tabs at typical viewport widths still fit comfortably (the existing tab buttons are short). Mitigation: visual check during dev-server testing; the existing tab-bar component is responsive.
