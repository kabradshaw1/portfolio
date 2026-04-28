# Design: PgBouncer Connection Pooling Layer

- **Date:** 2026-04-27
- **Status:** Draft — pending implementation
- **Roadmap position:** Item 6 of 10 in the `db-roadmap` GitHub label
- **GitHub issue:** [#160 — PgBouncer connection pooling layer](https://github.com/kabradshaw1/portfolio/issues/160)
- **Builds on:**
  - `go/CLAUDE.md` — existing `pgxpool` tuning per service
  - `docs/adr/infrastructure/2026-04-24-postgres-data-integrity.md` — Phase 2 todo

## Context

Every Go service today connects directly to the shared Postgres instance with its own `pgxpool`. That works at portfolio scale, but it's wrong for any system that grows. Postgres is *bad at idle connections* — each backend connection allocates significant memory (~10 MB), eats a backend slot, and competes for `max_connections` (default 100). Multiplying out:

- 5 Go services × 1 replica each × `pgxpool.MaxConns = 25` (typical default) = 125 potential connections
- Plus Java services, plus migration jobs, plus ad-hoc psql sessions
- Plus QA copies of every service

Even at one-replica-per-service, this saturates `max_connections` quickly. Bumping `max_connections` is the wrong fix — Postgres performance degrades because each backend has overhead. The right fix is a **connection pooler** between the apps and Postgres: PgBouncer in transaction-pooling mode lets apps keep many cheap client-side connections while PgBouncer holds a small, stable server-side pool to Postgres.

This is table stakes at any company with more than one service. "Why do you need a connection pooler in front of Postgres?" is asked in nearly every backend interview that touches scale.

## Goals

- Drop a single PgBouncer between every app's `DATABASE_URL` and the Postgres pod.
- Keep Postgres `max_connections` low (default 100) by aggressively fanning in via transaction-mode pooling.
- Preserve `pgx` prepared-statement caching (`QueryExecModeCacheDescribe` is set on every Go service).
- Observe pool wait times and saturation as first-class signals.
- Don't introduce new auth secrets — reuse existing Postgres credentials via `auth_query`.

## Non-goals

- PgBouncer HA via multiple replicas. A single replica is sufficient at portfolio scale; HA is straightforward (`replicas: 2`, ClusterIP Service in front) and noted in trade-offs.
- pgcat / Supavisor / Odyssey alternatives — PgBouncer is the established standard.
- Routing reads to a separate replica (separate roadmap item #161; this spec leaves a hook for it).
- Eliminating per-service `pgxpool.MaxConns`. The local pool is still useful for connection warmth and per-process behavior — it just gets *smaller*.

## Architecture

```
java-tasks namespace
├── postgres (existing)
│   └── port 5432, accepts connections from PgBouncer's server pool
│
├── pgbouncer (NEW)
│   ├── Deployment, single replica
│   ├── Image: edoburu/pgbouncer:1.23.1 (or community)
│   ├── pgbouncer.ini       per-DB pool, transaction mode, prepared statements
│   ├── userlist.txt        empty (auth_query handles it)
│   ├── ConfigMap-mounted; secret password injected via env
│   └── Service: pgbouncer.java-tasks.svc.cluster.local:6432
│
└── pgbouncer-exporter (NEW)
    ├── sidecar in pgbouncer pod (or separate Deployment)
    ├── Image: prometheuscommunity/pgbouncer-exporter:v0.10.2
    └── Scraped at :9127 by Prometheus

Go services
└── DATABASE_URL changes:
       was:  postgres://user@postgres.java-tasks.svc.cluster.local:5432/<db>
       now:  postgres://user@pgbouncer.java-tasks.svc.cluster.local:6432/<db>
    pgxpool.MaxConns lowered (math below).

Java services
└── Same DATABASE_URL change for any service that connects to Postgres.

Migration jobs (golang-migrate)
└── Bypass PgBouncer — connect directly to Postgres on 5432.
    Migrations need session-level features (transaction wrapping, advisory locks)
    that transaction-pooling mode doesn't preserve.
```

## Pool sizing math

```
Per-app:
  pgxpool.MaxConns = 8     (down from typical 25 default)

Across the cluster:
  5 Go services × 1 pod × 8 conn  = 40 client conns
  3 Java services × 1 pod × 10    = 30 client conns
  TOTAL client side                = ~70

PgBouncer config:
  default_pool_size      = 25     (server conns per (user, db) pair)
  max_client_conn        = 200    (room for QA + ad-hoc + future growth)
  max_db_connections     = 80     (cap toward Postgres regardless of pools)

Postgres:
  max_connections = 100   (unchanged)
```

The fan-in: 70+ client connections compress into ~25 server-side. Idle apps hold open client connections cheaply (PgBouncer keeps them in memory, not on Postgres). The pool only checks out a server connection while a transaction is in flight.

`max_db_connections = 80` is a hard ceiling that protects Postgres regardless of pool config drift — even if every per-pool limit were misconfigured, Postgres would never see more than 80 PgBouncer connections, leaving headroom under the 100 default `max_connections`.

## Pool mode — transaction

`pool_mode = transaction` is chosen because:

- **Session mode** holds a server connection for the entire client session — no fan-in, defeats the point.
- **Statement mode** is finer-grained but breaks any multi-statement transaction. Every Go service uses transactions; this would break the ecommerce saga.
- **Transaction mode** holds a server connection only for the duration of a `BEGIN`...`COMMIT`. Best fan-in compatible with our workload.

Trade-offs of transaction mode:
- `LISTEN` / `NOTIFY` doesn't work (we don't use it).
- Session-level `SET` (e.g., `SET LOCAL` is fine, `SET` is not) doesn't persist. We don't depend on this either.
- Prepared statements need explicit support (next section).

## Prepared statements (the gotcha)

`pgx` defaults to `QueryExecModeCacheDescribe`, which uses prepared statements. PgBouncer in `transaction` mode historically broke this — a prepared statement is bound to a server connection, but the next query might land on a different server connection that doesn't have it.

PgBouncer 1.21+ added **protocol-level prepared statement support** in transaction mode (`max_prepared_statements > 0`). Each PgBouncer client connection caches its prepared statements; PgBouncer transparently re-prepares them on whatever server connection it routes to.

Configuration:

```ini
max_prepared_statements = 200
```

This adds memory per client connection (~few KB per cached statement). Acceptable.

Verification: an integration test asserts that the same query, sent twice through PgBouncer, parses to the same plan id (`pg_stat_statements.queryid` is stable) — proof that prepared-statement reuse is working.

## Authentication via `auth_query`

`auth_query` lets PgBouncer ask Postgres for the password hash of a connecting user, rather than maintaining a static `userlist.txt`. New users are picked up automatically — no PgBouncer restart on user rotation.

```ini
auth_user = pgbouncer_auth
auth_query = SELECT usename, passwd FROM pg_shadow WHERE usename=$1
```

A new role `pgbouncer_auth` is created with `pg_read_server_files`-equivalent access to `pg_shadow`:

```sql
CREATE ROLE pgbouncer_auth LOGIN PASSWORD :'pw';
CREATE FUNCTION pg_temp.pgbouncer_auth(text) RETURNS TABLE(uname TEXT, phash TEXT) AS $$
  SELECT usename, passwd FROM pg_shadow WHERE usename = $1
$$ LANGUAGE SQL SECURITY DEFINER;
GRANT EXECUTE ON FUNCTION pg_temp.pgbouncer_auth(text) TO pgbouncer_auth;
```

(The exact `auth_query` is finalized in the plan; the SECURITY DEFINER wrapper avoids granting `pg_shadow` access broadly.)

The `pgbouncer_auth` password is stored in `java-secrets` as a new key.

## Migrations bypass PgBouncer

`golang-migrate` Jobs and any other migration tool need session-level semantics. The migration K8s Jobs keep the existing direct `DATABASE_URL` to `postgres.java-tasks.svc.cluster.local:5432`. This is documented in the migration playbook (#156) and called out explicitly in the manifests.

## Observability

### `prometheus-pgbouncer-exporter`

Sidecar in the pgbouncer pod (or separate small Deployment — sidecar is simpler). Exposes:

- `pgbouncer_pools_client_active_connections{database, user}`
- `pgbouncer_pools_client_waiting_connections{database, user}`
- `pgbouncer_pools_server_active_connections`
- `pgbouncer_pools_server_idle_connections`
- `pgbouncer_pools_server_used_connections`
- `pgbouncer_stats_avg_query_time_seconds`
- `pgbouncer_stats_avg_wait_time_seconds`

### Dashboard

A new dashboard `pgbouncer.json` in `grafana-dashboards.yml` (separate from the PostgreSQL health dashboard, but cross-linked):

| Panel | Type | Source | Purpose |
|---|---|---|---|
| Client connections (active vs waiting) | Time series | Prometheus | Saturation early warning |
| Server connection usage | Time series | Prometheus | Capacity planning |
| Avg wait time per pool | Time series | Prometheus | The key SLI for PgBouncer |
| Avg query time per pool | Time series | Prometheus | Compare to direct-Postgres baseline |
| Top pools by waiting clients | Bar gauge | Prometheus | Hotspot identification |
| Total Postgres backends (before/after) | Stat | Prometheus | The "did this work?" answer |

### Alerts (appended to `PostgreSQL` group, all `noDataState: OK`)

| Alert | Threshold | Why |
|---|---|---|
| `PgBouncerPoolWaitTimeHigh` | `pgbouncer_stats_avg_wait_time_seconds > 0.1` for 5m, per pool | Wait > 100ms means the pool is too small or the DB is slow |
| `PgBouncerClientsWaiting` | `pgbouncer_pools_client_waiting_connections > 0` for 10m | Sustained queueing, not just bursts |
| `PgBouncerServerConnectionFailures` | `rate(pgbouncer_stats_total_xact_count[5m]) drops to 0` while client conns active | PgBouncer can't reach Postgres |

## Per-service config changes

The Go services that need touching (one-line `DATABASE_URL` change in each Deployment env, plus a `pgxpool.MaxConns` retune):

| Service | Current `MaxConns` | New `MaxConns` | Notes |
|---|---|---|---|
| auth-service | 25 | 8 | |
| product-service | 25 | 8 | |
| order-service | 25 | 8 | Reporting pool to follow once #161 lands |
| cart-service | 25 | 8 | |
| payment-service | 25 | 8 | |
| ai-service | n/a (no Postgres) | n/a | |

Java services (Spring HikariCP):
| Service | `maximumPoolSize` was | New | Notes |
|---|---|---|---|
| task-service | 10 | 5 | |
| activity-service | 10 | 5 | |

The exact current values are confirmed during plan-writing by reading each service's config.

## Testing

**Integration test** at `go/pkg/db/pgbouncer_integration_test.go` (testcontainers, build-tagged):

1. Spin up a Postgres container.
2. Spin up a PgBouncer container linked to it, with `pool_mode=transaction`, `max_prepared_statements=200`.
3. Open a `pgxpool` against PgBouncer using `QueryExecModeCacheDescribe`.
4. Run the same parameterized query 100× across 20 goroutines.
5. Assert: no errors, all queries return correct results, `pg_stat_statements.calls = 100` for that queryid (proving prepared statements were reused).
6. Assert: `pg_stat_database.numbackends` on Postgres stays ≤ pool_size (proving fan-in worked).

**QA smoke test:**
1. After deploy, exec into a Go service pod and verify `psql $DATABASE_URL -c '\conninfo'` shows port 6432.
2. Run a load test (Apache Bench or hey) against an endpoint that hits the database.
3. Watch `kubectl exec postgres -- psql -c "SELECT count(*) FROM pg_stat_activity WHERE application_name LIKE 'pgbouncer%'"` — should stay flat as load increases.
4. Verify the dashboard's "Total Postgres backends" panel drops dramatically post-deploy.

## Rollout

This is a blast-radius-y change — every service's connection path moves. Mitigation: deploy in QA first, observe for 24h, then prod.

1. Land the PgBouncer Deployment, Service, ConfigMap, sidecar exporter, and the `pgbouncer_auth` Job. PgBouncer is up but unused.
2. Verify QA: connect manually with `psql -h pgbouncer.java-tasks-qa -p 6432 -U taskuser -d productdb`. Confirm prepared statements work via the integration test in QA.
3. Switch *one* QA service's `DATABASE_URL` (start with auth-service — smallest surface). Watch logs and the new dashboard for 1h.
4. Switch the rest of QA services in a single PR, observe overnight.
5. Repeat in prod.
6. Land observability (dashboard + alerts) early — they're harmless without PgBouncer running.
7. Migration Jobs explicitly keep the direct URL — verified by reading the Job manifests during plan execution.

## ADR

Companion ADR at `docs/adr/database/pgbouncer.md` covering:

- Why transaction mode (with the LISTEN/NOTIFY caveat)
- Why `auth_query` over `userlist.txt`
- Pool sizing math (the headline numbers above) and the `max_db_connections` safety rail
- Why migrations bypass PgBouncer
- Why `max_prepared_statements` is non-negotiable in our setup
- The single-replica HA trade-off and the "two replicas behind a Service" upgrade path

## Consequences

**Positive:**
- Postgres `max_connections = 100` becomes safe headroom even as services scale.
- Idle apps cost nothing on Postgres (server-side conns are recycled at end of transaction).
- Pool wait time becomes a first-class SLI — alerting on it catches real saturation before user-facing timeouts.
- Adds a layered architecture story: pgx pool (per-process) + PgBouncer (cluster) is exactly how production shops do it.

**Trade-offs:**
- PgBouncer is now a SPOF for every Postgres-dependent service. Single-replica is acceptable at portfolio scale; HA upgrade is small. The PDB and `nodeAffinity` keep it from being voluntarily evicted.
- Transaction-mode pooling forecloses LISTEN/NOTIFY and session-level state. We don't use these; future work that wants them must connect directly.
- Migrations carve out an exception. Documented in the playbook (#156) and the PgBouncer ADR.
- `max_prepared_statements > 0` adds modest memory per client connection. Not material.

**Phase 2:**
- Multi-replica PgBouncer (`replicas: 2`, headless service, client-side connection retry).
- Replica-aware PgBouncer pools once #161 (read replica) lands — separate `pgbouncer.ini` `[databases]` entries `productdb_ro` etc. routing reads to the replica.
