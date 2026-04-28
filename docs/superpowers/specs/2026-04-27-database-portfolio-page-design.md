# Design: `/database` Portfolio Page

- **Date:** 2026-04-27
- **Status:** Approved — pending implementation
- **Roadmap position:** Portfolio surface area for the database/SQL track of `db-roadmap`. Not itself a `db-roadmap` item — it's the recruiter-facing index for them.
- **Builds on:**
  - `docs/adr/ecommerce/go-database-optimization.md`
  - `docs/adr/ecommerce/go-sql-optimization-reporting.md`
  - `docs/adr/database/migration-lint.md`
  - `docs/superpowers/specs/2026-04-23-postgres-data-integrity-design.md`

## Context

The portfolio has accumulated four substantial pieces of production-grade
PostgreSQL work — query optimization with real benchmarks, range partitioning
with materialized views, a custom safe-migration linter, and a backup +
recovery operational track. None of it is currently surfaced anywhere a
recruiter would find it without clicking deep into `/go` and reading ADRs.

Most of the roles Kyle is applying to list "PostgreSQL" or "SQL" as a primary
requirement. ATS systems and recruiter scans key on those terms. The current
homepage cards (`/go`, `/aws`, `/java`, `/ai`, plus `/observability`,
`/security`, `/cicd`) demonstrate that cross-cutting specialty pages already
have an established home in the IA — `/observability` and `/security` aren't
languages, they're capabilities. Database engineering belongs in that family.

The portfolio also already touches MongoDB (Java task-management activity feed
and read models) and Qdrant (Document Q&A and Debug Assistant). Those are
worth surfacing in the same place over time, but they aren't the current
priority — the page should reserve space for them without delaying the
PostgreSQL story.

## Goals

- Make the existing PostgreSQL work discoverable from the homepage in one
  click.
- Surface specific recruiter keywords (PostgreSQL, partitioning, materialized
  views, CONCURRENTLY, NOT VALID, libpg_query, pg_dump, postgres_exporter)
  inline in the page text, not buried in linked PDFs.
- Provide a "scan then dig" reading experience: a recruiter scrolling sees the
  breadth in 30 seconds; an interested hiring manager has links into every
  ADR for the depth.
- Reserve IA for NoSQL (MongoDB) and Vector (Qdrant) so adding them later is
  additive, not restructuring.

## Non-goals

- Filling in the NoSQL and Vector tabs with full content — they ship as
  stubs that point at `/java` and `/ai` respectively.
- Live query playgrounds, embedded `psql` shells, or interactive benchmark
  charts. Static numbers and a code snippet or two are sufficient.
- Reorganizing the existing `/go` tabs to remove database content from
  there. Both pages can reference the same work; the `/database` page is
  the indexed entry point, the `/go` content remains as service-level
  context.

## Architecture & IA placement

New top-level page at `frontend/src/app/database/page.tsx`. Surfaced as the
seventh card on the homepage (`frontend/src/app/page.tsx`), placed after `/cicd`
and before `/security` (so the order reads: portfolio → infra → quality
specialties).

**Homepage card:**

- Title: "Database Engineering"
- Description: "Production PostgreSQL — optimization, partitioning,
  migration safety, and reliability"
- Body (one sentence): "Real benchmarks against PostgreSQL 16, range
  partitioning with materialized views, a custom AST-based migration
  linter, and an operational track with backups and recovery runbooks."

The card matches the existing `<Card>` / `<CardHeader>` / `<CardContent>`
pattern used for every other portfolio entry, no new components.

## Page structure

Three-tab layout using the same `border-b` tab pattern `/go` already uses.
Tab state managed locally in the page (mirrors `/go` page.tsx).

| Tab | Slug | Content |
|---|---|---|
| **PostgreSQL** | default | Full content — four pillars, single-scroll, sticky TOC |
| **NoSQL** | `?tab=nosql` | Stub: short blurb + link to `/java` |
| **Vector** | `?tab=vector` | Stub: short blurb + link to `/ai` |

Tabs are visual only — no URL routing required for the MVP (matches `/go`'s
pattern). If we want shareable deep-links later, add `?tab=` query-param
sync, but that's not in scope here.

### NoSQL stub copy

> "MongoDB powers the activity feed and analytics aggregations in the Java
> task-management portfolio. A dedicated NoSQL section is on the way — for
> now, the working code lives in `/java`."

With a button-style link: "View MongoDB usage in /java →".

### Vector stub copy

> "Qdrant backs the RAG pipeline behind the Document Q&A assistant and the
> code-aware Debug Assistant. A dedicated vector-database section is on the
> way — for now, the working code lives in `/ai`."

With a button-style link: "View vector DB usage in /ai →".

## PostgreSQL tab — single scroll, sticky TOC, four pillars

The tab body has a two-column layout on desktop (≥ md): the four pillar
sections take the main column, the sticky TOC takes a narrow right column
that stays in view as the user scrolls, with the active section highlighted
via `IntersectionObserver`. On mobile (< md), the TOC collapses to a
horizontal scrollable chip row pinned below the bio paragraph.

Each pillar follows the same shape (one reusable component, see Components
below):

1. **Section header** — pillar name as `<h2>` with `id` matching the TOC
   anchor.
2. **Narrative paragraph** — 2–4 sentences setting up the why and the
   high-level approach.
3. **Bullet list** — 4–6 concrete bullets with specific techniques,
   measured results, and recruiter keywords. Bullets are factual claims
   matched to the linked ADRs.
4. **ADR / runbook link row** — one or two "Read the ADR →" / "Read the
   runbook →" links rendered as a muted button row.

The four pillars in narrative order:

### 1. Query Optimization & Benchmarking

**Anchor:** `#optimization`

**Narrative:** Functional ORM-style code is a starting point, not the
finish line. The Go services were re-benchmarked against a real PostgreSQL
16 container and rewritten where the data showed it. testcontainers-go made
the benchmarks runnable on any Docker-equipped machine; results were
captured to `go/benchdata/` for interview-ready evidence.

**Bullets:**

- Real-DB benchmarks via `testcontainers-go` against PostgreSQL 16; results
  in `go/benchdata/baseline-results.txt` and `optimized-results.txt`
- Batch INSERT for order items: **3.5× speedup on 20-item orders**
  (4.5ms → 1.3ms), single round trip instead of N
- `COUNT(*) OVER()` window function replaces COUNT-then-data double query
- CTE-based atomic conflict resolution in cart updates
  (`WITH updated AS (UPDATE ... RETURNING) SELECT EXISTS(...)`)
- pgx prepared-statement cache enabled (`QueryExecModeCacheDescribe`)
- Targeted indexes: `idx_orders_saga_step`, partial
  `idx_products_low_stock WHERE stock < 10`, composite
  `idx_cart_items_user_reserved`
- Typed pgx error checks: `errors.Is(err, pgx.ErrNoRows)`,
  `errors.As(err, &pgconn.PgError)` for code `23505`

**Links:** Read the ADR → `docs/adr/ecommerce/go-database-optimization.md`

### 2. Schema Design — Partitioning & Materialized Views

**Anchor:** `#schema`

**Narrative:** Reporting workloads on a monotonically growing `orders`
table forced a schema-design pass. Range partitioning by `created_at`
prunes scan scope; three materialized views give constant-time reads for
the dashboard queries; CTE + window functions express the rolling-average
business logic without application-side aggregation.

**Bullets:**

- Range partitioning on `orders.created_at` (monthly), with 18 months
  pre-provisioned and a default catch-all partition
- Background goroutine creates partitions 3 months ahead daily; idempotent
  `CREATE TABLE IF NOT EXISTS`
- Three materialized views (`mv_daily_revenue`, `mv_product_performance`,
  `mv_customer_summary`) refreshed `CONCURRENTLY` on a 15-min cadence
- Unique indexes per MV to support `REFRESH CONCURRENTLY`
- CTE-driven reporting queries with `SUM(...) OVER (ORDER BY day ROWS
  BETWEEN 6 PRECEDING AND CURRENT ROW)` for rolling 7/30-day averages
- `DENSE_RANK()` for tie-aware top-N (turnover, top customers)
- Composite primary key trade-off documented (`(id, created_at)` removes
  FK target on `id` alone — referential integrity moves to the saga)

**Links:** Read the ADR → `docs/adr/ecommerce/go-sql-optimization-reporting.md`

### 3. Migration Safety — `migration-lint`

**Anchor:** `#migrations`

**Narrative:** `golang-migrate` catches syntactic errors when the migration
runs against Docker; it doesn't catch operationally unsafe DDL that's
syntactically valid. A custom Go linter (`migration-lint`) walks each
`.up.sql` AST via `libpg_query` and flags eight common foot-guns at lint
time, before any container starts. Each rule pairs with a recipe in a
checked-in safe-migration runbook.

**Bullets:**

- Custom Go CLI built on `pganalyze/pg_query_go` (CGO wrapper around
  `libpg_query`, the upstream PG parser)
- Eight rules: CREATE INDEX without CONCURRENTLY (MIG001), NOT NULL ADD
  COLUMN with volatile default (MIG002), table-rewrite ALTER COLUMN TYPE
  (MIG003), CHECK without NOT VALID (MIG004), DROP COLUMN (MIG005),
  RENAME COLUMN (MIG006), CONCURRENTLY mixed with other DDL (MIG007),
  LOCK TABLE (MIG008)
- Per-statement opt-out: `-- migration-lint: ignore=MIGNNN reason="..."`
  with mandatory `reason="..."`
- Wired into `make preflight-go-migrations` and the CI matrix as a hard
  prerequisite to the runtime migration pipeline
- Companion 8-recipe runbook: `docs/runbooks/postgres-migrations.md`
- Worked example: `CREATE INDEX CONCURRENTLY` in its own migration file
  (`go/product-service/migrations/005_add_product_search_index.up.sql`)

**Links:** Read the ADR → `docs/adr/database/migration-lint.md` · Read the
runbook → `docs/runbooks/postgres-migrations.md`

### 4. Reliability & Recovery

**Anchor:** `#reliability`

**Narrative:** Production-grade SQL isn't only about queries. Postgres
needs scheduled backups, monitored health, and a written runbook for the
day someone has to restore from one. The portfolio's Postgres deployment
ships with all three.

**Bullets:**

- Automated `pg_dump` CronJob writing to a persistent volume on the
  Minikube node; retention policy in the manifest
- Pod Disruption Budget on the StatefulSet (`maxUnavailable: 1`) so node
  drains don't block on a single-replica DB
- `postgres_exporter` sidecar feeding Prometheus; Grafana dashboard
  surfaces connection counts, replication lag, table sizes, and slow
  queries
- Alert rules: backup-job failure, replication-lag-too-high, disk-full,
  long-running-transaction
- Written recovery runbook: `docs/runbooks/postgres-recovery.md` (step by
  step from `pg_dump` artifact to a hot standby)

**Links:** Read the runbook → `docs/runbooks/postgres-recovery.md` · Read
the spec → `docs/superpowers/specs/2026-04-23-postgres-data-integrity-design.md`

## Components

```
frontend/src/app/database/
├── page.tsx                            # Three-tab page, mirrors /go pattern
└── error.tsx                           # Standard error boundary

frontend/src/components/database/
├── PostgresTab.tsx                     # Two-column layout: pillars + sticky TOC
├── NoSqlTab.tsx                        # Stub component
├── VectorTab.tsx                       # Stub component
├── PillarSection.tsx                   # Reusable: header + narrative + bullets + links
├── StickyToc.tsx                       # IntersectionObserver-based active highlight
└── tabs.ts                             # `Tab` union, tab labels (matches /go style)
```

`PillarSection` props (informal):

- `id: string` — anchor id
- `title: string`
- `narrative: ReactNode` — paragraph(s)
- `bullets: ReactNode[]`
- `links: { label: string; href: string }[]`

The sticky TOC takes a list of `{ id, label }` and tracks active section via
`IntersectionObserver` on the section headings. The "active" item gets a
left border accent matching the existing tab pattern.

## Homepage card

Edit `frontend/src/app/page.tsx`, add a new `<Link href="/database">` block
matching the existing card structure. Place it between the `/cicd` and
`/security` cards.

## Testing

- **Playwright smoke** at `frontend/e2e/database.spec.ts`:
  - Loads `/database`, asserts `<h1>` contains "Database Engineering"
  - Asserts all three tab labels render (PostgreSQL, NoSQL, Vector)
  - Clicks the NoSQL tab, asserts the stub copy and the "View MongoDB
    usage in /java" link render
  - Clicks the Vector tab, asserts the stub copy renders
  - Switches back to PostgreSQL, clicks each TOC anchor, asserts the
    corresponding `<h2>` is in the visible viewport
- **Type/lint sweep** — `npm run typecheck` and `npm run lint` (already
  in `make preflight-frontend`)
- **Visual** — manual: load locally, scroll the PostgreSQL tab, verify
  the sticky TOC highlights the right pillar at each scroll position

## Out of scope (Phase 2)

- Filling NoSQL and Vector tabs with content
- URL-synced tab state (`?tab=nosql`)
- Animated benchmark charts or live `pg_stat_statements` snapshots
- A "Try the linter" embedded sandbox

## Open questions

- None blocking. Rendering polish (exact spacing of the sticky TOC,
  whether bullet keywords get a subtle highlight color) is best handled
  during implementation against the live page.
