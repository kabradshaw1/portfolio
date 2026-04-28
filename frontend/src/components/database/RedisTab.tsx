import { PillarSection } from "@/components/database/PillarSection";
import {
  StickyTocChips,
  StickyTocSidebar,
  type StickyTocItem,
} from "@/components/database/StickyToc";

const REPO = "https://github.com/kabradshaw1/portfolio";

const tocItems: StickyTocItem[] = [
  { id: "caching", label: "Read-Side Caching" },
  { id: "rate-limiting", label: "Rate Limiting" },
  { id: "idempotency", label: "HTTP Idempotency" },
  { id: "denylist", label: "JWT Denylist" },
  { id: "analytics", label: "Time-Windowed Analytics" },
];

export function RedisTab() {
  return (
    <div data-testid="redis-tab" className="md:grid md:grid-cols-[1fr_220px] md:gap-10">
      {/* Mobile chip TOC: top of tab content, hidden at md+. */}
      <div className="md:hidden">
        <StickyTocChips items={tocItems} />
      </div>

      <div className="space-y-16 min-w-0">
        <PillarSection
          id="caching"
          title="Read-Side Caching — Two Languages, One Discipline"
          narrative={
            <>
              <p>
                Caching is layered on top of a healthy primary store, never as a
                load-bearing dependency. Both the Go services and the Java
                <code> task-service</code> talk to the same Redis instance, but
                they reach it through idiomatic abstractions in each language —
                a small Go interface that keeps the cache transport-agnostic,
                and Spring&apos;s <code>@Cacheable</code> for declarative
                method-level caching.
              </p>
              <p>
                The shared rule across both sides: every cache call goes
                through a circuit breaker that fails open. When Redis is
                unavailable, reads become misses and writes become no-ops —
                the request path keeps working.
              </p>
            </>
          }
          bullets={[
            <>
              Go <code>Cache</code> interface (<code>go/ai-service/internal/cache/cache.go</code>):
              bytes in, bytes out. Callers handle their own serialization, so
              the cache stays transport-agnostic. <code>RedisCache</code> for
              prod, <code>NopCache</code> for dev or when Redis is disabled.
            </>,
            <>
              Per-prefix namespacing (e.g.{" "}
              <code>ai:tools:&lt;tool&gt;:&lt;args-hash&gt;</code>,{" "}
              <code>product:catalog:&lt;id&gt;</code>) — key inspection in{" "}
              <code>redis-cli</code> stays trivial and a single prefix can be
              invalidated without affecting unrelated entries.
            </>,
            <>
              AI tool-result cache (<code>go/ai-service/internal/tools/cached.go</code>)
              memoizes deterministic tool calls so an agent loop doesn&apos;t
              pay repeated Redis or upstream latency on identical sub-queries
              within the same conversation.
            </>,
            <>
              Product catalog cache (<code>go/product-service/internal/service/product.go</code>):
              short-TTL read-through for hot product reads — keeps Postgres
              free for writes during catalog browsing spikes.
            </>,
            <>
              Java: Spring <code>@Cacheable</code> on <code>project-stats</code>{" "}
              and <code>project-velocity</code> in{" "}
              <code>task-service/AnalyticsService.java</code>. Declarative,
              swappable via <code>CacheConfig</code>; the same Redis instance
              backs Spring&apos;s cache abstraction.
            </>,
            <>
              Cross-cutting: every Redis call wrapped in{" "}
              <code>gobreaker</code>; on trip, reads fail open (cache miss),
              writes are skipped. OpenTelemetry Redis spans via{" "}
              <code>tracing.RedisSpan</code> render the cache hit/miss in
              Jaeger between the HTTP handler and the Postgres query.
            </>,
          ]}
          links={[
            {
              label: "Go Cache interface",
              href: `${REPO}/blob/main/go/ai-service/internal/cache/cache.go`,
            },
            {
              label: "Spring CacheConfig",
              href: `${REPO}/blob/main/java/task-service/src/main/java/dev/kylebradshaw/task/config/CacheConfig.java`,
            },
          ]}
        />

        <PillarSection
          id="rate-limiting"
          title="Per-IP Rate Limiting — Fixed Window, Fail-Open"
          narrative={
            <>
              <p>
                A fixed-window limiter is the simplest correct thing: <code>INCR</code>
                {" "}plus <code>EXPIRE</code>, one Redis round-trip per request,
                no Lua, no token bucket, no surprises. The same pattern lives
                in <code>auth-service</code>, <code>order-service</code>,{" "}
                <code>cart-service</code>, and <code>ai-service</code> — each
                with its own key prefix so a single Redis instance can serve
                all four without collision.
              </p>
              <p>
                The trade-off is documented in the file: fixed windows have a
                known burst-at-boundary edge (a client could double-spend at
                the second a window flips). For the protection profile here —
                abuse blunting, not strict QoS — that&apos;s acceptable.
              </p>
            </>
          }
          bullets={[
            <>
              Canonical implementation:{" "}
              <code>go/ai-service/internal/guardrails/ratelimit.go</code>;
              mirrored as middleware in <code>auth</code>, <code>order</code>,
              and <code>cart</code>.
            </>,
            <>
              Pattern: <code>INCR key</code>; if value is 1,{" "}
              <code>EXPIRE key window</code>; reject when value exceeds{" "}
              <code>max</code>. Returns{" "}
              <code>Retry-After: &lt;seconds-until-window-end&gt;</code> so
              well-behaved clients back off cleanly.
            </>,
            <>
              Per-service prefixes (<code>ai:ratelimit:&lt;ip&gt;</code>,{" "}
              <code>auth:ratelimit:&lt;ip&gt;</code>, etc.) — one Redis serves
              every service without key collision or accidental cross-service
              throttling.
            </>,
            <>
              Fail-open: if the Redis breaker is open, the limiter allows
              traffic. Skipping rate limiting briefly is better than 503&apos;ing
              every request — the upstream load balancer and the Postgres
              connection pool ceiling are the next-layer brakes.
            </>,
            <>
              Per-route ceilings tuned per service (e.g.{" "}
              <code>POST /agent/chat</code> is much tighter than{" "}
              <code>GET /products</code>) so expensive endpoints can&apos;t be
              spammed even when the cheap ones aren&apos;t.
            </>,
          ]}
          links={[
            {
              label: "AI rate limiter",
              href: `${REPO}/blob/main/go/ai-service/internal/guardrails/ratelimit.go`,
            },
          ]}
        />

        <PillarSection
          id="idempotency"
          title="HTTP Idempotency-Key — Safe Retries"
          narrative={
            <>
              <p>
                <code>POST /orders</code> and <code>POST /cart/checkout</code>
                {" "}have to be retry-safe. Flaky networks, saga compensations,
                and the user smashing the buy button shouldn&apos;t create
                duplicate orders. RFC-style{" "}
                <code>Idempotency-Key</code> middleware backs both endpoints,
                using a two-phase Redis entry per key.
              </p>
              <p>
                Why this matters in this portfolio specifically: the saga
                orchestrator retries on transient gRPC failures, and idempotency
                is the only thing standing between a network blip and a
                duplicate charge.
              </p>
            </>
          }
          bullets={[
            <>
              Middleware:{" "}
              <code>go/order-service/internal/middleware/idempotency.go</code>,
              mirrored in <code>cart-service</code>.
            </>,
            <>
              <strong>Phase 1 — in-flight marker:</strong> when a key first
              arrives, write <code>PROCESSING</code> with a 30 s TTL.
              Concurrent duplicates with the same key get a 409 instead of
              racing to write the same order twice.
            </>,
            <>
              <strong>Phase 2 — completed-response cache:</strong> when the
              handler returns, write the captured status code + body with a
              24 h TTL. Subsequent retries with the same key replay the
              original response byte-for-byte — no &ldquo;was that order
              actually created?&rdquo; round-trip back to Postgres.
            </>,
            <>
              Key shape:{" "}
              <code>idempotency:&lt;route&gt;:&lt;idempotency-key-header&gt;</code>
              . The per-route prefix prevents accidental key reuse across
              endpoints from replaying the wrong response.
            </>,
            <>
              <code>responseCapture</code> wraps <code>gin.ResponseWriter</code>{" "}
              so the handler&apos;s body is buffered for caching without
              changing handler signatures or duplicating serialization logic.
            </>,
            <>
              Fail-open: if Redis is unavailable, the middleware logs and lets
              the request through. The downstream guard is Postgres&apos; own
              uniqueness constraints (idempotency keys are also persisted on
              the order row), so retries can&apos;t create duplicates even with
              the cache disabled.
            </>,
          ]}
          links={[
            {
              label: "order-service idempotency",
              href: `${REPO}/blob/main/go/order-service/internal/middleware/idempotency.go`,
            },
          ]}
        />

        <PillarSection
          id="denylist"
          title="JWT Token Denylist — Cheap Revocation Without Sessions"
          narrative={
            <>
              <p>
                Stateless JWTs are great for read-side performance but terrible
                at &ldquo;the user just clicked logout.&rdquo; A small Redis
                denylist closes the gap without sacrificing the
                stateless-verification win — verification adds one{" "}
                <code>EXISTS</code> check; revocation is a single{" "}
                <code>SET … EX &lt;remaining-ttl&gt;</code>.
              </p>
            </>
          }
          bullets={[
            <>
              Implementation:{" "}
              <code>go/auth-service/internal/service/token_denylist.go</code>.
            </>,
            <>
              Key shape: <code>auth:denied:&lt;sha256-of-token&gt;</code>.
              Hashing avoids leaking the token in Redis itself or in any logs
              that capture key names.
            </>,
            <>
              TTL aligned to the token&apos;s remaining lifetime — once the
              token would have expired anyway, the denylist entry self-cleans.
              Avoids unbounded denylist growth without a sweep job.
            </>,
            <>
              Logout handler calls <code>denylist.Revoke(token, ttl)</code>;
              every authenticated request hits the deny check before the
              handler runs.
            </>,
            <>
              Graceful degradation: a <code>nil</code> Redis client is a valid
              construction. Revocation becomes a no-op and tokens expire
              naturally — used in dev environments and unit tests.
            </>,
            <>
              Trade-off note: introduces a per-request Redis hop on every
              authenticated call. Acceptable for current RPS; a future
              optimization would be a bloom filter in front of the denylist if
              throughput climbs.
            </>,
          ]}
          links={[
            {
              label: "Token denylist",
              href: `${REPO}/blob/main/go/auth-service/internal/service/token_denylist.go`,
            },
          ]}
        />

        <PillarSection
          id="analytics"
          title="Time-Windowed Analytics — Bucketed Counters with TTLs"
          narrative={
            <>
              <p>
                The analytics service runs a Kafka consumer over{" "}
                <code>ecommerce.orders</code>, <code>ecommerce.cart</code>, and{" "}
                <code>ecommerce.views</code>, then writes time-bucketed counters
                into Redis. Each metric has an explicit retention window — no
                unbounded growth, no hand-rolled cleanup cron, just per-key TTLs
                that match the dashboard query window.
              </p>
            </>
          }
          bullets={[
            <>
              Store: <code>go/analytics-service/internal/store/redis.go</code>.
            </>,
            <>
              <strong>Revenue</strong> (
              <code>analytics:revenue:&lt;hour&gt;</code>) — 48 h TTL, hourly
              granularity. Daily revenue charts read 24 keys, weekly reads 168.
            </>,
            <>
              <strong>Trending products</strong> (
              <code>analytics:trending:&lt;hour&gt;</code>) — 2 h TTL,
              sorted-set scored by view/order signals. The short window is
              deliberate: trending is a near-real-time concept, stale data is
              worse than no data.
            </>,
            <>
              <strong>Cart abandonment</strong> (
              <code>analytics:abandonment:&lt;hour&gt;</code> +
              {" "}<code>analytics:abandonment:users:&lt;user&gt;</code>) — 24 h
              TTL. Hash for aggregate counters, per-user marker for dedup so a
              single abandoned cart counts once.
            </>,
            <>
              Hourly bucket keys use the Go time layout{" "}
              <code>2006-01-02T15</code> — range queries become a deterministic
              key sweep over the last N hours, no <code>SCAN</code> cursor or
              sorted-set range needed for the common case.
            </>,
            <>
              Sub-hour granularity isn&apos;t supported; deliberate choice to
              keep keys legible and dashboard math obvious. If finer resolution
              is needed later, the bucket layout is a one-line change.
            </>,
            <>
              Standard Redis breaker / OTel span / fail-open story applies —
              analytics is best-effort and never blocks the write path.
            </>,
          ]}
          links={[
            {
              label: "Analytics Redis store",
              href: `${REPO}/blob/main/go/analytics-service/internal/store/redis.go`,
            },
          ]}
        />

        {/* Closing operational note */}
        <section className="rounded-lg border border-foreground/10 bg-card p-6">
          <h3 className="text-base font-semibold">
            Operational discipline (the boring part that keeps it boring)
          </h3>
          <p className="mt-3 text-sm text-muted-foreground leading-relaxed">
            Every Redis call in this portfolio is wrapped in a circuit breaker
            that fails open, every key is namespaced and TTL&apos;d, and every
            operation gets an OpenTelemetry span via{" "}
            <code>tracing.RedisSpan</code> so a single trace shows the cache
            hit between the HTTP handler and the Postgres query. The
            discipline isn&apos;t visible in any single use case — it&apos;s the
            reason none of them have ever taken down the request path.
          </p>
        </section>
      </div>

      {/* Desktop sidebar TOC: right column at md+. */}
      <aside className="hidden md:block">
        <StickyTocSidebar items={tocItems} />
      </aside>
    </div>
  );
}
