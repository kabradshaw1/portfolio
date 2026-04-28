# Design: `/database` Page Expansion — PITR, Verified Backups, Query Observability

- **Date:** 2026-04-27
- **Status:** Approved — pending implementation
- **Roadmap position:** Portfolio surface area for the database/SQL track. Extends the original `/database` page (`docs/superpowers/specs/2026-04-27-database-portfolio-page-design.md`) with three additional pieces of work that have shipped since.
- **Builds on:**
  - `docs/superpowers/specs/2026-04-27-database-portfolio-page-design.md` — the original `/database` page spec
  - `docs/adr/database/wal-archiving-pitr.md` — WAL archiving + Point-in-Time Recovery (merged in PR #174)
  - `docs/adr/observability/2026-04-27-pg-query-observability.md` — `pg_stat_statements` + `auto_explain` (merged in PR #165 / #169)
  - `docs/adr/infrastructure/2026-04-24-postgres-data-integrity.md` — daily `pg_dump`, dashboard, alerts (merged earlier)
  - Backup-verification ADR + design (PR #175 — open to qa, expected to merge before this lands)

## Context

The original `/database` spec landed four PostgreSQL pillars: Query Optimization, Schema Design (partitioning + MVs), Migration Safety (`migration-lint`), and a Reliability & Recovery pillar that was deliberately broad ("backups, monitored health, written runbook"). Since then three substantial pieces of operational and observability work have shipped or are about to ship:

1. **WAL archiving + Point-in-Time Recovery** — `archive_command` wrapper, weekly `pg_basebackup` CronJob, `replicator` role, three new alerts on `pg_stat_archiver`, Scenario 4 in the recovery runbook.
2. **Verified backups** — `pg_restore` of yesterday's dump against a throwaway database, success/failure pushed to a Pushgateway sidecar, Prometheus alerts on stale or failed verification.
3. **Query observability** — `pg_stat_statements` + `auto_explain` with a dedicated query-performance dashboard, regression-detection alert, plan capture into Loki.

None of these are surfaced on `/database`. The query-observability story is on `/observability` already (a Prometheus/Loki/Grafana framing), but a recruiter scanning `/database` for "how does this engineer find a slow query?" finds nothing — even though that's a baseline interview question. The PITR and verified-backup work — both of which would land on most production teams' roadmaps — are similarly invisible.

The fix is additive, not a rewrite. Two new pillars (Query Observability, expanded Reliability with PITR + verified backups) join the existing four, the order is reshuffled to put the most widely applied capabilities at the top, and a small piece of database content currently misplaced under `/go` migrates back to `/database` where the keyword discovery is.

## Goals

- Add the two missing capabilities (Query Observability, PITR + verified backups) to the page so a recruiter scanning the page sees the full breadth of SQL/operational depth.
- Reorder pillars so the section a recruiter encounters first is the one most likely to apply to the role they're hiring for.
- Move the standalone "Database Optimization" benchmark-results table currently on `/go` MicroservicesTab to `/database`, leaving a one-line breadcrumb on `/go`.
- Keep cross-linking to `/observability` for the deep query-observability story so the two pages don't drift.
- No new components, no architectural change — reuse `PillarSection` and `StickyToc` as-is.

## Non-goals

- Rewriting any of the existing pillar content beyond the Reliability section (which is the natural home for the PITR + verified-backup additions).
- Filling in the NoSQL or Vector tabs — still stubs, separate roadmap item.
- URL-synced tab state (`?tab=…`) — out of scope as in the original spec.
- Live or interactive content (live `pg_stat_statements` snapshots, embedded `psql`, animated charts).
- Reorganizing `/observability` — that page already has detailed query-observability content; this work cross-links into it without modifying it.
- Reorganizing the rest of `/go` — only the standalone "Database Optimization" sub-section moves; the surrounding service-architecture content stays put.

## Pillar list and ordering rationale

The pillars are ordered so the section a recruiter encounters first is the one most likely to be relevant to *any* PostgreSQL role, with breadth narrowing as the page progresses.

| # | Pillar | Anchor | Why this rank |
|---|---|---|---|
| 1 | Query Optimization & Benchmarking | `optimization` | Every Postgres role asks "show me a slow query you fixed." Most universally applicable; recruiters expect to see it first. |
| 2 | Query Observability — `pg_stat_statements` + `auto_explain` | `observability` | The first tool any Postgres engineer reaches for when triaging a slow query. Standard interview vocabulary. **NEW.** |
| 3 | Reliability & Backups — `pg_dump`, WAL/PITR, verified backups | `reliability` | Every production DB needs backups; PITR is a baseline interview answer. **EXPANDED.** |
| 4 | Migration Safety — `migration-lint` | `migrations` | Every team shipping schema changes. Specialized but widely needed. |
| 5 | Schema Design — Partitioning & Materialized Views | `schema` | Narrow — applies once reporting workloads or large tables force the issue. Strongest depth signal but smallest audience. |

Compared to the current page (`#optimization` → `#schema` → `#migrations` → `#reliability`), this shifts `#schema` to the bottom and inserts `#observability` at position 2. The existing content for pillars 1, 4, 5 is unchanged in copy; only their position changes.

## Per-pillar content changes

### Pillar 1: Query Optimization & Benchmarking — content addition

**Anchor:** `optimization` (unchanged)

The current pillar has good narrative + bullets but no numerical results. The current `/go` page has a benchmark-results table with four before/after rows that demonstrates the value of the optimizations:

| Optimization | Before | After | Speedup |
|---|---|---|---|
| Order creation (20 items) | 4.5 ms | 1.3 ms | **3.5×** |
| Product search | 1.0 ms | 0.55 ms | 1.9× |
| Order creation (5 items) | 1.5 ms | 0.8 ms | 1.8× |
| Category filter | 430 µs | 327 µs | 1.3× |

Move this table to the bottom of the optimization pillar's body (after the bullet list, before the link row). The shape stays a plain `<table>` with the same Tailwind classes the `/go` table uses; it slots into the `narrative` prop or as an additional section before `bullets` — implementation choice. No new component required.

On `/go` `frontend/src/components/go/tabs/MicroservicesTab.tsx`, the existing `{/* Database Optimization */}` `<section>` (currently lines ~236-353) is replaced with a one-line breadcrumb:

> "Benchmark methodology and the full before/after results live on the [Database](/database#optimization) page."

The breadcrumb keeps `/go` focused on service architecture and lifts SQL detail to where the keyword search lands.

### Pillar 2: Query Observability — `pg_stat_statements` + `auto_explain` — NEW

**Anchor:** `observability` (NEW)

**Narrative** (2-3 sentences): Slow queries don't fix themselves; they have to be found first. Postgres ships two extensions that do exactly this — `pg_stat_statements` aggregates per-query latency, call counts, and IO; `auto_explain` captures full execution plans for any query that crosses a duration threshold. Both are wired into the portfolio so the slow query a hiring manager would normally have to take on faith is instead visible in Grafana.

**Bullets** (5-6, focused on the SQL primitives, not the dashboard):

- `shared_preload_libraries='pg_stat_statements,auto_explain'` set on the Postgres deployment — server-wide enablement; restart picked up by the existing `Recreate` strategy.
- Per-database `CREATE EXTENSION IF NOT EXISTS pg_stat_statements` for all 7 prod databases, bootstrapped by an idempotent K8s Job (`postgres-extensions-bootstrap`).
- `auto_explain.log_min_duration=500ms`, `log_analyze=true`, `log_format=json` — every query over 500ms writes a JSON plan to Postgres logs, which Promtail ships to Loki keyed by `query_id`.
- Custom `postgres_exporter` queries surface the top-50 by mean latency and the top-50 by IO, with `query_text` truncated to 200 chars to bound label cardinality.
- Three Prometheus alerts: hard ceiling on per-query mean (> 1s for 10m), regression detection (mean > 2× the 7-day baseline for 15m), and an `auto_explain`-stalled canary that fires when no plan log lines arrive in 24h.
- Read-only `grafana_reader` role (`pg_monitor` predefined role) lets a Grafana PostgreSQL data source render live "top slow queries" tables without leaking write access.

**Links:**
- "Read the ADR →" `docs/adr/observability/2026-04-27-pg-query-observability.md`
- "Detailed observability story →" `/observability` (in-portfolio cross-link)

The cross-link is important: it makes clear the duplication is intentional (database-engineer framing here, observability-engineer framing on `/observability`) and prevents future drift between the two pages.

### Pillar 3: Reliability & Backups — EXPANDED

**Anchor:** `reliability` (unchanged), title changes from "Reliability & Recovery" to "Reliability & Backups" (better matches the content).

**Narrative** (replaced): Production-grade SQL isn't only about queries. Postgres needs scheduled backups, continuous WAL archiving, monitored health, and a written runbook for the day someone has to restore. The portfolio's Postgres deployment ships all four — and verifies the backups are actually restorable, because a backup that hasn't been restored is a hope, not a guarantee.

**Bullets**, organized into three implicit groups (no sub-headers, just ordering — keeps the bullet shape consistent with the other pillars):

*Daily snapshots:*
- Daily `pg_dump --format=custom` per database (7 prod DBs), 7-day retention; backups land on a hostPath PV (`/backups/postgres`) separate from the Postgres data PVC so PVC corruption doesn't affect backups.
- Postgres deployment uses `Recreate` strategy + `terminationGracePeriodSeconds: 90` + `preStop: pg_ctl stop -m fast` — the combination that prevents the WAL-corruption incident the data-integrity ADR documents.
- `PodDisruptionBudget` with `maxUnavailable: 0` on the single-replica DB so node drains don't take it out involuntarily.

*Continuous archiving:*
- `archive_mode=on` + custom `archive_command` wrapper script (`pg-archive-wal.sh`, atomic via temp + rename) ships every WAL segment to a 10Gi `wal-archive` PV.
- `archive_timeout=300` forces a WAL switch every 5 min during idle periods, so RPO drops from ≤ 24h to ≤ 5m.
- Weekly `pg_basebackup` CronJob (Sundays 03:00 UTC) writes `--format=tar --gzip --wal-method=fetch` tarballs; retains 4 weeklies + WAL back to the second-newest base backup. Uses a dedicated `replicator` role with only `REPLICATION LOGIN` (not `taskuser`).
- Three `pg_stat_archiver`-based alerts: archive command failing, WAL archive stale, base backup stale.

*Verified backups:*
- Daily `postgres-backup-verify` CronJob restores yesterday's dump into a throwaway database, runs `pg_restore --list | wc -l` and a row-count smoke check, pushes success/failure to Pushgateway as a Prometheus metric.
- Two alerts: verification *failed* (immediate, severity critical) and verification *stale* (no successful verify in 26h, severity warning).
- The metric is on the existing PostgreSQL dashboard alongside the pg_dump-stale and basebackup-stale panels — three operational signals on one screen.

*Runbook:*
- Four-scenario runbook (`docs/runbooks/postgres-recovery.md`): fresh PVC reset, full restore from `pg_dump`, partial restore (single database), point-in-time recovery to a specific timestamp.

**Links** (3 ADRs + 1 runbook, button row):
- "Read the data-integrity ADR →" `docs/adr/infrastructure/2026-04-24-postgres-data-integrity.md`
- "Read the WAL/PITR ADR →" `docs/adr/database/wal-archiving-pitr.md`
- "Read the backup-verification ADR →" `docs/adr/database/backup-verification.md` (path follows the convention used by the WAL/PITR ADR; confirm exact filename when implementing — backup-verification PR #175 settles it)
- "Read the recovery runbook →" `docs/runbooks/postgres-recovery.md`

### Pillar 4: Migration Safety — `migration-lint` (unchanged content, repositioned)

**Anchor:** `migrations` (unchanged)

No copy changes. Section moves up (was position 3, now position 4 — neutral, and the *relative* shape vs. its neighbours is still "between Reliability and Schema").

### Pillar 5: Schema Design — Partitioning & Materialized Views (unchanged content, repositioned)

**Anchor:** `schema` (unchanged)

No copy changes. Section moves to last (was position 2). It's the deepest content and the narrowest audience — the bottom is the right place.

## TOC update

Sticky TOC (both desktop sidebar and mobile chip row) is regenerated from the new ordering:

```ts
const tocItems: StickyTocItem[] = [
  { id: "optimization",  label: "Query Optimization" },
  { id: "observability", label: "Query Observability" },
  { id: "reliability",   label: "Reliability & Backups" },
  { id: "migrations",    label: "Migration Safety" },
  { id: "schema",        label: "Schema Design" },
];
```

`StickyToc.tsx` is unchanged — it already takes `items` as a prop. The `IntersectionObserver` re-binds to whichever sections render.

## Bio paragraph touch-up

The `<p>` directly under `<h1>` on `/database` currently reads:

> "Production-grade PostgreSQL is one of the load-bearing skills behind this portfolio: real-database benchmarks, range partitioning with materialized views, a custom AST-based migration linter, and an operational track with backups and recovery runbooks."

Replace with:

> "Production-grade PostgreSQL: real-database benchmarks (with measured 3.5× wins), slow-query observability via `pg_stat_statements` + `auto_explain`, point-in-time recovery with verified backups, a custom AST-based migration linter, and range partitioning with materialized views for reporting."

The lead changes from depth signal ("load-bearing skills") to capability list (5 specifics). The "with measured 3.5× wins" is the same number that appears in the optimization pillar's table — recruiter scans see it twice without it feeling repetitive.

## Component impact

| File | Change |
|---|---|
| `frontend/src/components/database/PostgresTab.tsx` | Reorder existing four `PillarSection`s, add a fifth (Observability), expand the Reliability section's narrative + bullets + links, update `tocItems`. |
| `frontend/src/app/database/page.tsx` | Update bio paragraph copy. |
| `frontend/src/components/go/tabs/MicroservicesTab.tsx` | Replace the `{/* Database Optimization */}` `<section>` (~118 lines) with a one-line breadcrumb. |
| `frontend/e2e/database.spec.ts` | Add anchor assertions for `#observability` and the new TOC ordering. Existing anchor assertions for `#optimization`, `#migrations`, `#reliability`, `#schema` stay. |

No new files, no new components, no router changes.

## Testing

- **Playwright smoke** (`frontend/e2e/database.spec.ts` — extend existing test):
  - The TOC labels render in the new order: "Query Optimization", "Query Observability", "Reliability & Backups", "Migration Safety", "Schema Design".
  - All five `<h2>` anchors exist with the IDs above.
  - Clicking the new "Query Observability" TOC chip scrolls the matching `<h2>` into view.
  - The optimization pillar contains a `<table>` with at least one `<td>` containing the text "3.5x" or "3.5×" (proves the moved benchmark table is present).
  - On `/go`, the breadcrumb to `/database#optimization` is present where the old "Database Optimization" `<section>` was.
- **Type/lint** — `npm run typecheck` and `npm run lint` (already in `make preflight-frontend`).
- **Visual** — manual: load locally, scroll the new pillar, verify the `IntersectionObserver` highlights "Query Observability" when its `<h2>` enters the viewport, verify the moved table renders correctly, verify the `/go` breadcrumb link navigates correctly.

## Rollout

This is a single PR — no staged steps. Roughly:

1. Author the new Observability `PillarSection` content + the expanded Reliability section in `PostgresTab.tsx`.
2. Reorder pillars + update `tocItems`.
3. Move the benchmark table from `/go` MicroservicesTab into the optimization pillar.
4. Replace the old `/go` section with the breadcrumb.
5. Update the bio paragraph.
6. Update the Playwright spec.
7. `make preflight-frontend` (must pass) → `make preflight-e2e` (must pass).
8. PR to `qa`.

Sequence dependency: the Playwright assertion that the new "Query Observability" chip exists must not be added before the implementation, or `qa`'s smoke suite goes red on the same commit. Bundle them in the same PR (TDD-style: failing test → implementation → green) so this stays green commit-by-commit.

## Open questions

- **Backup-verification ADR exact filename.** PR #175 will settle it. Confirm at implementation time and update the link if needed (currently assumed `docs/adr/database/backup-verification.md`).
- **Whether the optimization-pillar table goes inside the existing `narrative` prop or as a new optional `extras` prop on `PillarSection`.** Implementation-time call. Both work; the simpler path is to render the table as part of `narrative` (which already accepts `ReactNode`), avoiding a component prop change.

## Consequences

**Positive:**
- The five pillars on `/database` now match the actual depth of database work in the portfolio. A recruiter or hiring manager scanning the page sees PostgreSQL operational depth that wasn't visible before.
- The pillar order leads with the section most likely to be relevant to any PostgreSQL role; narrower content lives at the bottom.
- `/go` and `/database` stop competing for the same content. SQL-engineering keywords are concentrated where the keyword search expects them.
- Cross-linking between `/database` and `/observability` makes the duplication intentional and prevents future drift.

**Trade-offs:**
- The page gets longer. The sticky TOC is the mitigation; it already exists and handles the longer page without restructuring.
- `/go` MicroservicesTab loses a table that demonstrated database wins. The breadcrumb keeps the link discoverable; the depth lives one click away on `/database`.
- The Reliability pillar bullet list grows from ~5 to ~13 bullets. That's the right shape for this content (three operational primitives, each with 3-5 bullets) — a denser pillar at the page midpoint. If it reads as too long during implementation, splitting "verified backups" into its own pillar is a reversible change.
