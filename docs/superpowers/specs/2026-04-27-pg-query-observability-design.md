# Design: PostgreSQL Query Observability (`pg_stat_statements` + `auto_explain`)

- **Date:** 2026-04-27
- **Status:** Draft — pending implementation
- **Roadmap position:** Item 1 of 10 in the `db-roadmap` GitHub label
- **Related issues:** #155–#163 (db-roadmap items 2–10)
- **Builds on:**
  - `docs/adr/infrastructure/2026-04-24-postgres-data-integrity.md` (postgres_exporter, dashboards, alerts)
  - `docs/adr/ecommerce/go-database-optimization.md` (existing benchmarks)
  - `docs/adr/ecommerce/go-sql-optimization-reporting.md` (CTEs, materialized views)

## Context

The shared PostgreSQL 17 instance hosts databases for every Go and Java service. The existing observability stack covers *system-level* health (connections, cache hit ratio overall, deadlocks, backup freshness) but has no view into individual *query* performance:

- We don't know which queries are slow, called most often, or consume the most CPU.
- We can't detect plan regressions when data shape changes.
- We have no way to inspect actual execution plans for production traffic.
- After running optimization work (batch INSERTs, CTEs, partitioning), we have no production-side proof that those queries are still fast.

This is the measurement layer that any further database optimization or replication work needs as a foundation. It is also a baseline backend-engineer skill: "How do you find slow queries?" is one of the most commonly asked Postgres interview questions.

## Goals

- Identify the slowest queries (by mean and total exec time) in a live, browseable view.
- Track per-query latency over time so plan regressions show up as Prometheus alerts.
- Capture full `EXPLAIN ANALYZE` output for any query that exceeds 500ms, with the plan readable inline in Grafana.
- Provide alerting that catches both hard latency ceilings and slow drifts.
- Add no new infrastructure components — extend the existing observability stack.

## Non-goals

- Cluster-wide query views via `postgres_fdw` / `dblink` — per-database datasource is sufficient at current scale.
- Statement-level CPU/IO accounting via `pg_stat_kcache` — out of scope.
- Query rewriting recommendations (pgBadger, pganalyze) — paid/heavyweight tools, not justified here.
- Production sampling tuning — `auto_explain.sample_rate = 1.0` is fine at portfolio scale.
- Authoring queries that *use* the data (this is the measurement layer; optimization work it enables is tracked separately).

## Architecture

```
Postgres (java-tasks namespace, single replica, Recreate strategy)
├── pg_stat_statements    extension, in shared_preload_libraries
├── auto_explain          extension, in shared_preload_libraries
└── grafana_reader role   pg_monitor predefined role

       │ (5432, in-pod)
       ├──▶ postgres_exporter (existing sidecar) ──▶ Prometheus
       │      • new pg_stat_statements queries in custom-queries ConfigMap
       │      • exports top-50 queries: calls, mean, stddev, total, rows
       │      • exports top-50 by IO: shared_blks_hit/read/dirtied
       │
       ├──▶ Grafana PostgreSQL data source ──▶ Grafana
       │      • read-only via grafana_reader
       │      • powers live "top slow queries" table panels
       │      • per-DB datasource: productdb, orderdb, paymentdb
       │
       └──▶ Postgres logs ─▶ Promtail ─▶ Loki
              • auto_explain writes JSON plans for queries > 500ms
              • Grafana logs panel renders plans inline by queryid
```

Data flows through three independent paths: time-series metrics (Prometheus), live query inspection (Grafana PostgreSQL data source), and execution plans (Loki). Each is reversible on its own.

## PostgreSQL configuration

Set in the Postgres ConfigMap (`java/k8s/configmaps/postgres-config.yml`):

```ini
shared_preload_libraries = 'pg_stat_statements,auto_explain'

# pg_stat_statements
pg_stat_statements.max           = 5000
pg_stat_statements.track         = top
pg_stat_statements.track_utility = off

# auto_explain
auto_explain.log_min_duration = 500ms
auto_explain.log_analyze      = true
auto_explain.log_buffers      = true
auto_explain.log_timing       = true
auto_explain.log_format       = json
auto_explain.sample_rate      = 1.0
```

**Restart implication:** changing `shared_preload_libraries` requires restarting Postgres. The existing `Recreate` deployment strategy handles this — the next deploy that ships this config picks up the change cleanly.

**Tradeoffs:**
- `auto_explain.log_analyze = true` adds modest overhead because Postgres has to instrument every plan to capture actual row counts. At portfolio request volume this is negligible; the tradeoff would be revisited at scale.
- `pg_stat_statements.track = top` (not `all`) means nested statement calls inside functions/triggers don't show up as separate entries. We have very little stored-procedure code, so `top` is correct.
- `track_utility = off` skips `CREATE`/`ALTER`/`VACUUM`-style statements, which would otherwise crowd out application queries in the top-N.

## Extensions per database

Postgres extensions are per-database. Two paths:

1. **For new databases:** add `CREATE EXTENSION IF NOT EXISTS pg_stat_statements;` to `java/k8s/configmaps/postgres-initdb.yml`, executed during fresh PVC initialization.
2. **For existing databases:** a one-shot K8s Job (`postgres-extensions-bootstrap`) that connects to each prod DB and runs `CREATE EXTENSION IF NOT EXISTS pg_stat_statements;`. Idempotent; safe to re-run.

`auto_explain` is a server-side extension loaded via `shared_preload_libraries` and does not require `CREATE EXTENSION` per database.

Databases targeted: `authdb`, `productdb`, `orderdb`, `cartdb`, `paymentdb`, `ecommercedb`, `taskdb` (and the `_qa` equivalents).

## Metrics layer (postgres_exporter)

Two custom queries appended to the postgres_exporter custom-queries ConfigMap.

### `pg_stat_statements` (latency)

```yaml
pg_stat_statements:
  query: |
    SELECT
      pss.queryid,
      LEFT(pss.query, 200) AS query_text,
      pss.calls,
      pss.total_exec_time,
      pss.mean_exec_time,
      pss.stddev_exec_time,
      pss.rows
    FROM pg_stat_statements pss
    WHERE pss.calls > 10
    ORDER BY pss.mean_exec_time DESC
    LIMIT 50
  master: true
  metrics:
    - queryid:           { usage: LABEL,   description: "pg_stat_statements query ID" }
    - query_text:        { usage: LABEL,   description: "Truncated query text (200 chars)" }
    - calls:             { usage: COUNTER, description: "Number of times executed" }
    - total_exec_time:   { usage: COUNTER, description: "Total time in milliseconds" }
    - mean_exec_time:    { usage: GAUGE,   description: "Mean execution time in milliseconds" }
    - stddev_exec_time:  { usage: GAUGE,   description: "Std dev of execution time" }
    - rows:              { usage: COUNTER, description: "Total rows returned" }
```

### `pg_stat_statements_io` (cache behavior)

```yaml
pg_stat_statements_io:
  query: |
    SELECT
      pss.queryid,
      pss.shared_blks_hit,
      pss.shared_blks_read,
      pss.shared_blks_dirtied
    FROM pg_stat_statements pss
    WHERE pss.calls > 10
    ORDER BY pss.shared_blks_read DESC
    LIMIT 50
  master: true
  metrics:
    - queryid:             { usage: LABEL }
    - shared_blks_hit:     { usage: COUNTER, description: "Buffer hits per query" }
    - shared_blks_read:    { usage: COUNTER, description: "Disk reads per query" }
    - shared_blks_dirtied: { usage: COUNTER, description: "Buffers dirtied per query" }
```

**Cardinality budget:** 50 queries × 8 metrics × 30s scrape ≈ 13 samples/sec ingest. Well below any concern. The `query_text` label is bounded to 200 chars to keep label storage manageable. The `WHERE calls > 10` filter excludes one-off queries (test fixtures, ad-hoc queries) that would otherwise churn the top-N.

## Inspection layer (Grafana PostgreSQL data source)

### Read-only role

Migration applied to the Postgres instance:

```sql
CREATE ROLE grafana_reader LOGIN PASSWORD :'GRAFANA_READER_PASSWORD';
GRANT pg_monitor TO grafana_reader;
GRANT CONNECT ON DATABASE productdb, orderdb, paymentdb, cartdb, authdb TO grafana_reader;
```

`pg_monitor` is a predefined Postgres role that grants `SELECT` on monitoring views including `pg_stat_statements`, `pg_stat_activity`, `pg_stat_user_tables`, `pg_stat_replication`. Preferred over hand-rolled GRANTs because it tracks upstream when new views are added.

The password is stored in the existing `postgres-secret` Kubernetes Secret as a new key `grafana-reader-password`. Grafana reads it via secret reference in the data source provisioning manifest.

### Data sources

Three Grafana data sources (one per high-traffic database), defined in `k8s/monitoring/configmaps/grafana-datasources.yml`:

- `postgres-productdb`
- `postgres-orderdb`
- `postgres-paymentdb`

The dashboard uses a `Database` template variable to switch between them. Adding `cartdb`, `authdb`, etc. is additive and one-line if needed later.

**Why per-DB rather than a `monitoring` DB with `postgres_fdw`?** At three high-traffic DBs, three datasources are simpler than maintaining foreign tables. The cluster-wide pattern is justified once you have ten or more DBs.

## Plan capture (auto_explain → Loki)

`auto_explain` writes plans to standard Postgres logs. Postgres logs already flow to Loki via the existing Promtail config. Because `auto_explain.log_format = json`, plan log lines are parseable JSON.

A new Promtail pipeline stage extracts:
- `query_id` (as a structured field, not a label — high cardinality)
- `duration_ms` (as a structured field)
- `database` (as a label — bounded set)

The dashboard's plan-viewer panel queries Loki:

```
{namespace="java-tasks", app="postgres"} |= "auto_explain" | json | duration_ms > 500
```

A second panel filters by a `queryid` template variable so the user can drill from the metrics top-N table into the plan that matches.

## Dashboard

New dashboard `pg-query-performance.json` deployed via the existing Grafana provisioning ConfigMap pattern. Cross-linked from the existing Postgres health dashboard via a "Drill into queries →" link.

| Panel | Type | Source | Purpose |
|---|---|---|---|
| Top 10 slowest queries (mean) | Table | Grafana PostgreSQL DS | Live triage |
| Top 10 slowest queries (total time) | Table | Grafana PostgreSQL DS | "What's eating CPU" |
| p95 latency per queryid (top 5) | Time series | Prometheus | Regression detection |
| Slow-query rate (calls with mean > 500ms) | Time series | Prometheus | Volume trend |
| Cache hit ratio per query | Table | Grafana PostgreSQL DS | I/O hotspots |
| Recent slow plans (last 1h) | Logs | Loki | Plan deep-dive |
| Plan viewer (filter by queryid) | Logs | Loki, var-driven | Interview demo |

Template variables:
- `Database` — switches between productdb / orderdb / paymentdb
- `queryid` — multi-select, drives the plan-viewer filter

## Alerts

Four new alert rules, all `noDataState: OK` (per the project-wide pattern from the data-integrity ADR).

| Alert | Threshold | Why |
|---|---|---|
| `PgQueryMeanExecTimeHigh` | tracked queryid mean_exec_time > 1000ms for 10m | Hard ceiling on individual query latency |
| `PgQueryMeanRegression` | tracked queryid mean > 2× its 7-day moving baseline for 15m | Catches plan flips and data-driven regressions that don't trip the hard ceiling |
| `PgSlowQueryRateSpike` | rate of slow-query calls > 3× the 1h baseline for 10m | Volume-based incident signal |
| `PgAutoExplainStalled` | no auto_explain log line received in Loki for 24h | Misconfig protection — a silent failure here would hide regressions |

The regression alert is the differentiator: hard thresholds miss the realistic failure mode, which is a query that quietly drifts from 50ms to 200ms after a planner change.

## Testing

**Integration test** (testcontainers, build-tagged `//go:build integration`):
- Start Postgres with `pg_stat_statements` and `auto_explain` preloaded
- Run a deliberately slow query: `SELECT pg_sleep(0.6)`
- Assert it appears in `pg_stat_statements`
- Assert the Postgres log contains a JSON `auto_explain` plan for it

**Existing benchmark suite** gets a follow-up assertion: after the slow benchmarks run (e.g., `BenchmarkOrderCreate_20Items`), `pg_stat_statements` must return non-zero rows for the underlying SQL.

**Alert smoke test** in QA:
- Temporarily lower `PgQueryMeanExecTimeHigh` to 1ms
- Run any production-style request
- Verify Telegram alert fires within 10 minutes
- Restore threshold

## Rollout

Each step is an independently mergeable PR. Steps 1–2 are the only ones that touch the running Postgres pod; the rest are observability-only and safe.

1. Add extensions to `postgres-initdb.yml` + ship the one-shot bootstrap Job for existing DBs.
2. Update the Postgres ConfigMap (`shared_preload_libraries` + `auto_explain.*` settings). Deploy → `Recreate` restart picks it up.
3. Append the two custom queries to the postgres_exporter ConfigMap; redeploy exporter.
4. Add `grafana_reader` role + secret + Grafana data source provisioning manifests.
5. Add the dashboard ConfigMap.
6. Add the alert rules ConfigMap.
7. Verify in QA → promote to prod.

## ADR

A companion ADR will be written at `docs/adr/observability/2026-04-27-pg-query-observability.md` after rollout, documenting the cardinality math, the regression-alert design, the per-DB datasource decision, and one screenshot of a real production plan caught by `auto_explain`.

## Consequences

**Positive:**
- Production query performance is observable for the first time — both real-time (table panels) and longitudinally (Prometheus).
- Plan regressions become detectable as alerts, not just user complaints.
- The roadmap's remaining nine items (replication, retention, vacuum tuning, full-text search, etc.) all depend on a measurement layer; this is that layer.
- Strong interview demo: walk into a Grafana panel, point at a real slow query, click into its plan, explain what `auto_explain` showed.

**Trade-offs:**
- `shared_preload_libraries` change requires a Postgres restart. Acceptable given the existing `Recreate` posture.
- `auto_explain.log_analyze = true` has overhead. Negligible at portfolio scale; would be sampled at higher load.
- Per-DB Grafana data sources don't give a single cluster-wide view. Acceptable — the dashboard variable handles switching, and adding a `monitoring` DB with `postgres_fdw` is reversible if scale demands it.
- `query_text` as a Prometheus label is bounded to 200 chars. Long queries get truncated; the full text remains visible in the Grafana PostgreSQL data source panels.
