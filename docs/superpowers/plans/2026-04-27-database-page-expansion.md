# Plan: `/database` Page Expansion

- **Date:** 2026-04-27
- **Spec:** `docs/superpowers/specs/2026-04-27-database-page-expansion-design.md`
- **Branch:** `agent/feat-database-page-expansion` → PR to `qa`

## Files touched

| File | Change |
|---|---|
| `frontend/src/components/database/PostgresTab.tsx` | Reorder pillars; add Observability pillar; expand Reliability; embed benchmark table in Optimization narrative; update `tocItems` |
| `frontend/src/app/database/page.tsx` | Replace bio paragraph |
| `frontend/src/components/go/tabs/MicroservicesTab.tsx` | Replace `{/* Database Optimization */}` `<section>` with one-line breadcrumb to `/database#optimization` |
| `frontend/e2e/mocked/database-page.spec.ts` | Update pillar-headings list & anchor list to new ordering; add `#observability` assertion + benchmark-table assertion |
| `frontend/e2e/mocked/go-page.spec.ts` (or new) | Assert breadcrumb to `/database#optimization` exists on `/go` Microservices tab |

No new components, no new files outside the plan/spec docs.

## Confirmed paths (verified in working tree)

- `docs/adr/database/backup-verification.md` ✓
- `docs/adr/database/wal-archiving-pitr.md` ✓
- `docs/adr/database/migration-lint.md` ✓
- `docs/adr/observability/2026-04-27-pg-query-observability.md` ✓
- `docs/adr/infrastructure/2026-04-24-postgres-data-integrity.md` ✓
- `docs/runbooks/postgres-recovery.md` ✓
- `docs/runbooks/postgres-migrations.md` ✓

PR #175 has merged — backup-verification ADR is at the spec-assumed path. No TODO needed.

## Implementation steps

1. **PostgresTab.tsx** — write pillars in new order: Optimization (with embedded benchmark table) → Observability (new) → Reliability & Backups (expanded) → Migration Safety → Schema Design. Update `tocItems` and the Reliability section title. Cross-link button uses `/observability` (in-portfolio).
2. **database/page.tsx** — replace the `<p>` under `<h1>` with the new capability-list copy.
3. **MicroservicesTab.tsx** — delete the `{/* Database Optimization */}` `<section>` (lines 236-353) and replace with a single `<section>` containing a `<p>` breadcrumb sentence linking `/database#optimization`.
4. **e2e** — extend `database-page.spec.ts`: update existing "all four pillar headings" → "all five"; update anchor list; add `#observability` heading; add a `<table>` assertion for `3.5×|3.5x` text. Add `/go` breadcrumb assertion (likely in a new test inside `database-page.spec.ts` or a new `go-page-database-breadcrumb.spec.ts`).
5. **`make preflight-frontend`** — must pass before commit.
6. **`make preflight-e2e`** — run after all assertions land.
7. Commit, push, open PR to `qa`.

## Risks & rollback

- The page gets longer (~13-bullet Reliability pillar). Mitigation: sticky TOC already exists. If too dense in review, splitting verified-backups to its own pillar is a reversible follow-up.
- `/go` users lose a benchmark table. Mitigation: breadcrumb keeps the link discoverable.
