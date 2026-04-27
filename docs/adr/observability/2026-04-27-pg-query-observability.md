# ADR: PostgreSQL Query Observability (2026-04-27)

## Status
Accepted

## Context
The shared PostgreSQL 17 instance had system-level observability (connections, cache hit, deadlocks, backup freshness) but no view into individual query performance. This is the measurement layer needed for further optimization or replication work, and a baseline backend-engineer skill ("how do you find slow queries?").

Spec: `docs/superpowers/specs/2026-04-27-pg-query-observability-design.md`. Plan: `docs/superpowers/plans/2026-04-27-pg-query-observability.md`.

## Decisions

### `pg_stat_statements` + `auto_explain` preloaded via `args:`
The vanilla `postgres:17-alpine` image is left intact. Startup `-c` flags via `args:` overlay defaults rather than replacing `postgresql.conf`. This avoids drift from upstream defaults and keeps the diff minimal.

### Custom queries in `postgres_exporter`
Two custom queries (latency + IO) export the top-50 entries from `pg_stat_statements`. Cardinality is bounded by `LIMIT 50` and the `WHERE calls > 10` filter; `query_text` is truncated to 200 chars to keep label storage manageable.

### `pg_monitor` predefined role for the Grafana data source
Preferred over hand-rolled GRANTs because it tracks upstream when new monitoring views ship. The `grafana_reader` role gets `pg_monitor` plus per-DB `CONNECT`.

### Per-database Grafana data sources
`pg_stat_statements` is per-database. At three high-traffic DBs the per-DB datasource pattern is simpler than a `monitoring` DB with `postgres_fdw`. The dashboard's `Database` template variable handles switching.

### `auto_explain.log_format = json` to Loki
Plans flow through the existing Postgres → Promtail → Loki path. The JSON format makes them parseable by the Promtail pipeline and renders cleanly in Grafana logs panels.

### Regression alert against 7-day baseline
Hard latency thresholds miss the realistic failure mode — a query that quietly drifts from 50ms to 200ms after a planner change. The regression rule (`current / 7d-avg > 2`) catches that, while the hard `> 1s` rule catches genuinely terrible queries.

### `noDataState: OK`
Applied per the project-wide pattern (see `2026-04-24-postgres-data-integrity.md`). For rate-based and event-based metrics, no data means no activity, which means no problem.

## Consequences

**Positive:**
- Production query performance is observable for the first time — both real-time tables and longitudinal trend lines.
- Plan regressions become visible as alerts, not user complaints.
- The remaining nine `db-roadmap` items (#155–#163) all depend on this measurement layer.

**Trade-offs:**
- `shared_preload_libraries` change requires a Postgres restart. Acceptable given the existing `Recreate` posture.
- `auto_explain.log_analyze = true` adds modest planner overhead. Negligible at portfolio scale; sample rate would be lowered at higher load.
- Per-DB Grafana datasources don't give a single cluster-wide view. Acceptable — the dashboard variable handles switching.

**Phase 2 (future):**
- Trim `pg_stat_statements` periodically (`pg_stat_statements_reset()`) on a CronJob so old query plans don't crowd out current ones.
- Track plan stability via `pg_stat_statements.toplevel` once we have a year of data to compare.
