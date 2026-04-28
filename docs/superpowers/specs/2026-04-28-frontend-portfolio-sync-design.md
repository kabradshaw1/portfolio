# Frontend portfolio sync — April 2026 batch

## Context

A run of PRs (#142–#187) has shipped through `qa` to `main` over the past week, adding production-grade Postgres infrastructure, secret management, observability work, and developer-experience improvements. The frontend portfolio surfaces (`/database`, `/observability`, `/cicd`, `/security`, `/ai`, `/go`, `/java`, `/aws`) lag the actual capabilities of the system. This spec brings the frontend back in sync in a single change and reorders top-level navigation to match the importance Kyle wants reviewers to perceive.

## Goals

1. Reorder both the site header and home-page cards to: **go → database → ai → observability → cicd → aws → security → java**.
2. Surface the two highest-value undocumented Postgres capabilities (PgBouncer connection pooling, streaming read replica + graceful fallback).
3. Document Sealed Secrets as an in-progress migration (4 of 6 phases) on `/security`.
4. Add a CQRS / order-projector sub-section to `/observability` so the existing event-sourcing narrative is complete.
5. Tighten Tier-3 wording (backup verification name-check on `/database`, broader smoke-test wording on `/cicd`).

## Non-goals

- Pre-commit hook backfill (#167) — too narrow to surface.
- New frontend pages — every change lands in an existing page or component.
- Backend changes — this is a documentation/UX-only batch.
- Visual redesign — content lands inside the existing pillar/section components.

## Architecture

Frontend-only. No new routes, no new env vars, no API surface. Edits are scoped to:

- `frontend/src/components/SiteHeader.tsx` — nav reorder + new `/database` link.
- `frontend/src/app/page.tsx` — home-card reorder + new `/observability` card + bio paragraph touch-up.
- `frontend/src/components/database/PostgresTab.tsx` — two new `PillarSection` entries and matching `tocItems`.
- `frontend/src/app/security/page.tsx` — new "Secrets Management" section + posture-matrix row update.
- `frontend/src/app/observability/page.tsx` — new CQRS / order-projector sub-section nested into the existing event-sourcing narrative.
- `frontend/src/app/cicd/page.tsx` — copy edits to the smoke-test paragraph.

## Detailed sections

### 1. Navigation & home reorder

**Header (`SiteHeader.tsx`).** Reorder existing `<Link>` items and insert a `/database` link styled identically to siblings. Final order: go, database, ai, observability, cicd, aws, security, java. Resume button and Grafana link unaffected.

**Home (`page.tsx`).** Reorder the seven existing cards and insert a new card for `/observability` that mirrors the visual shape of the others (icon, title, 1–2 sentence description). Update the bio paragraph above the cards so it briefly mentions the new content (PgBouncer + read replica) and removes any wording that contradicts the new ordering.

**E2E risk.** Before finalizing, grep `frontend/e2e/` for selectors that depend on link order (e.g., `nth-child`, `getByRole('link').nth(N)`). Adjust selectors to match-by-text where order-dependent.

### 2. `/database` PostgresTab — two new pillars

Add two `PillarSection` entries and corresponding `tocItems` IDs in `PostgresTab.tsx`. Final TOC order:

```
optimization → observability → pooling → replica → reliability → migrations → schema
```

Pooling and replica precede reliability because connection pooling and read scaling are hotter scale topics; reliability/migrations/schema sit underneath as foundational discipline.

**`id="pooling"` — "Connection Pooling — PgBouncer"** covers:

- Transaction-mode pooling between every Go service and Postgres.
- `auth_query` for credential delegation (no synced password files).
- Per-service ConfigMap split: `DATABASE_URL` (through pooler) vs `DATABASE_URL_DIRECT` (direct, used by `golang-migrate` Jobs and any session-level work).
- Why migrations bypass the pooler — session features (advisory locks, transaction wrapping) that transaction-pool mode strips.
- Observability hook: pooler stats are scraped if `pgbouncer_exporter` is wired; otherwise flag as a near-term roadmap item.

**`id="replica"` — "Read Replica & Reporting Pool"** covers:

- Async streaming replica (`postgres-replica.java-tasks`) fed by WAL.
- Two-pool design in `order-service`: `Primary` for OLTP, `Reporting` for `/reporting/*` endpoints.
- `application_name` runtime parameter set per pool so `pg_stat_activity` shows primary vs reporting traffic without guessing.
- Graceful fallback (PR #187): if the replica is unreachable at startup, the reporting pool is aliased to primary with a warning log; the service stays up. Concrete production-engineering moment worth telling.
- QA shares the prod replica via `ExternalName` (cross-namespace shared infra pattern); call out the deployment-ordering lesson learned.

### 3. `/security` — Sealed Secrets section

New section on `security/page.tsx` titled **"Secrets Management — Sealed Secrets (in progress, 4 of 6 phases)"**. Content:

- Bitnami sealed-secrets controller installed cluster-wide (Phase 1).
- Four live Kubernetes Secrets converted to `SealedSecret` resources committed in-repo (Phase 2). Encrypted with the cluster's controller public key; only the in-cluster controller can decrypt.
- Why this matters: enables GitOps for secrets without committing plaintext, removes "live `kubectl` edits" as the source of truth.
- In-progress badge: 2 of 6 phases shipped, remaining phases queued (rotation, audit trail, prod cutover, retire-templates).

If the existing security posture matrix has a row for "Secrets management," update its `notes` to reference the migration; otherwise add a new row.

### 4. `/observability` — CQRS / order-projector sub-section

Add a single sub-section nested under the existing event-sourcing narrative (the same one that already covers Kafka headers, the `saga-order-stalled` alert, etc.). Content:

- The same `ecommerce.orders` Kafka topic that drives saga state also feeds an `order-projector` consumer that writes a denormalized read model to `projectordb`.
- Reads against the projection are independent of the OLTP write path — the read schema can evolve without touching `order-service`.
- Why CQRS here: reporting and dashboard reads on the projection won't compete with checkout writes for primary-pool connections, and the projection's shape can be tuned to query patterns rather than transactional invariants.
- Operational hooks: `analytics_aggregation_latency_seconds` and `kafka_consumer_lag` already exist for the analytics consumer; the projector reuses the same metric vocabulary.

One paragraph plus a single architecture note. No new diagrams.

### 5. Tier-3 polish

- **`/database` Reliability pillar:** confirm "automated backup verification" is named explicitly. If only generic "verified backups" wording exists, add the specific term + the CronJob name (`postgres-backup-verify`).
- **`/cicd` smoke tests:** existing copy already mentions compose-smoke and post-deploy Playwright; update wording to reflect that smoke coverage now extends across Go and Java stacks per PR #151.

### 6. Bio paragraph & cross-page consistency

The home-page bio currently mentions "migration linter, range partitioning with materialized views." Extend it to also reference connection pooling and read-replica work, since both now have prominent surfaces and the home page should match the actual top of `/database`.

## Testing

- `make preflight-frontend` (tsc + lint + Next build).
- `make preflight-e2e` (Playwright). Inspect any link-order-dependent specs and update selectors.
- Manual: load `/`, `/database`, `/security`, `/observability`, `/cicd` in the dev server. Confirm header order, home-card order, new pillars render, TOC chips/sidebar include `pooling` and `replica`, and Sealed Secrets section reads as in-progress (not finished).

## Delivery

- Branch: `agent/feat-frontend-portfolio-sync`, off `main`.
- Worktree: `.claude/worktrees/agent-feat-frontend-portfolio-sync/`.
- One PR to `qa`. Single review surface; small enough because every change is content/copy/order, no new components.
- No Vercel env-var changes needed (verified — no new `NEXT_PUBLIC_*` vars introduced).

## Risks & mitigations

- **E2E breakage from link reorder.** Mitigation: prefer match-by-text selectors; adjust positional ones.
- **Bio drift.** Bio paragraph tends to lag content; mitigation is one passes-everything edit in this spec, no separate follow-up.
- **Sealed Secrets framing.** Risk of overclaiming on a 4-of-6 migration; mitigation is the explicit phase counter in the section title.
- **CQRS placement.** Could equally live on `/go`. Choosing `/observability` because the saga + Kafka story already lives there; revisit if the section reads awkwardly during review.
