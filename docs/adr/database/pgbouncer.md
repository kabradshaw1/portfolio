# PgBouncer Connection Pooling

- **Date:** 2026-04-28
- **Status:** Accepted

## Context

The portfolio runs ~7 Postgres-using microservices against a single self-hosted Postgres 16 pod. Without a pooler, each service holds a long-lived `pgxpool` (or HikariCP) of 5–25 client connections, which the database backs 1:1 with a server process (~10 MB RSS each before any work). Current per-service pool ceilings sum to about 70 connections; Postgres `max_connections` is 100. Headroom for migrations, backup verifies, and ad-hoc psql sessions is uncomfortably thin, and adding any new service would push us over the wall.

Two pillars at risk:
- **Memory pressure on the Postgres pod** — every idle backend is fixed cost.
- **Cold-start latency** — TCP+TLS+startup-packet handshakes per new connection are ~1–5 ms each, paid by every request the service decides to open a new conn for.

A connection pooler in transaction mode lets us fan many short-lived "client" connections into a small number of stable "server" connections, decouples app pool sizing from Postgres backend count, and keeps per-service tuning local.

### Pool sizing math (target after full rollout)

| service           | client pool (`MaxConns`) | shared server backends |
|-------------------|--------------------------|------------------------|
| auth-service      | 8                        | 5 (default_pool_size)  |
| product-service   | 8                        | 5                      |
| order-service     | 8                        | 5                      |
| cart-service      | 8                        | 5                      |
| payment-service   | 8                        | 5                      |
| order-projector   | 10                       | 5                      |
| task-service (Java) | 5                       | 5                      |
| **totals**        | **~55 client → 70 with slack** | **~25 server, capped at 80 by max_db_connections** |

`max_db_connections=80` is the hard rail: even if every per-pool default expanded simultaneously, Postgres backends can't exceed it.

## Decision

Drop **PgBouncer 1.23.1** between every Postgres-using service and the Postgres pod. Specifically:

1. **Single-replica Deployment** in the `java-tasks` namespace (the same namespace as Postgres), reachable as `pgbouncer.java-tasks.svc.cluster.local:6432`. The QA Java overlay creates an `ExternalName` Service so QA workloads share the prod pool (single instance, separate `_qa` databases).
2. **`pool_mode=transaction`** — sessions are released back to the pool at every commit/rollback. Foregoes session-level state (`SET LOCAL`-style settings within a session, advisory locks, `LISTEN/NOTIFY`); we don't use any of these in app code.
3. **`auth_query`** against a dedicated `pgbouncer_auth` role with a `SECURITY DEFINER` wrapper over `pg_shadow`. PgBouncer authenticates new clients dynamically — no `userlist.txt` rebuild + restart on user changes. The wrapper is scoped to `EXECUTE` for `pgbouncer_auth` only.
4. **`max_prepared_statements=200`** — required for transaction-mode pooling with pgx's `QueryExecModeCacheStatement` / `CacheDescribe` defaults. Without it, the second use of any prepared statement would fail because PgBouncer 1.21+ now tracks server-side prepared statements and replays them on a different backend; before that, transaction-mode + prepared statements was simply broken.
5. **Migrations bypass the pooler.** Each Go service ConfigMap defines two keys: `DATABASE_URL` (through PgBouncer) and `DATABASE_URL_DIRECT` (direct Postgres). Migration Jobs reference `DATABASE_URL_DIRECT` because `golang-migrate` uses session-level features (advisory locks, transaction wrapping) that transaction-pool mode doesn't preserve.
6. **Observability sidecar.** A `prometheuscommunity/pgbouncer-exporter` runs alongside PgBouncer in the same Pod, scraping pool stats over the admin console, and is scraped by Prometheus via pod annotations. A dedicated Grafana dashboard (`pgbouncer-overview`) and three alerts (pool wait time, waiting clients, server connection failures) ship with this change.

## Consequences

### Positive

- Postgres backend count is bounded and predictable. Adding a new microservice no longer pushes us toward `max_connections`.
- Cold-start latency for short-lived connections drops to a re-bind against an already-warm backend.
- Pool sizing becomes a per-service knob without coordinating across the cluster.
- Failure modes are observable: waiting clients, wait time, and server connection drops are all metrics now.

### Trade-offs

- **SPOF risk.** Single-replica Deployment with a `Recreate` strategy means a brief outage when the pod restarts. Mitigated by a `minAvailable: 1` PDB. If the failure-domain analysis ever shifts (e.g., we add HA Postgres), scaling to two replicas behind the same ClusterIP is the next step.
- **Transaction-mode forecloses session-level state.** No `LISTEN/NOTIFY`, no session-level `SET`, no advisory locks held across statements. We don't currently use any of these; this is a constraint on future work.
- **Memory cost of `max_prepared_statements`** — a few KB per active server backend per cached statement. With ~25 backends and 200 cached statements, immaterial.

## Alternatives considered

- **pgcat** — newer, pgx-friendly, less battle-tested in production. Promising but the operational story is thinner (fewer dashboards, fewer postmortems on the public web). Revisit if PgBouncer hits a wall.
- **Supavisor** — Supabase's pooler. Coupled to their stack assumptions (e.g., tenancy model). Overkill for a single-tenant deployment.
- **Odyssey** — Yandex's pooler. Sparse English docs, smaller operator pool. Fast but harder to debug.

PgBouncer wins on maturity, observability surface, and the existence of `max_prepared_statements` (added in 1.21, refined in 1.23) which removes the historical "transaction mode breaks prepared statements" footgun.

## Rollout

Two-PR rollout:

1. **This PR** lands the infrastructure end-to-end (PgBouncer pod + exporter, ConfigMap, Service, PDB, bootstrap Job, dashboard, alerts, ADR, integration test) and cuts over **auth-service only** — smallest blast radius. Migration Jobs swap to `DATABASE_URL_DIRECT` (same value as before in this PR; the route flips for them only when their service's `DATABASE_URL` flips).
2. **Follow-up PR** cuts over the remaining Go services (product, order, cart, payment, order-projector) and Java task-service. Each service's `MaxConns` retunes to 8 and Java `task-service` parameterizes its Postgres host/port.

Auth-service has 24h to bake in QA before the bulk cutover ships.

## References

- Spec: [`docs/superpowers/specs/2026-04-27-pgbouncer-design.md`](../../superpowers/specs/2026-04-27-pgbouncer-design.md)
- Plan: [`docs/superpowers/plans/2026-04-28-pgbouncer.md`](../../superpowers/plans/2026-04-28-pgbouncer.md)
- GitHub issue: [#160](https://github.com/kabradshaw1/gen_ai_engineer/issues/160)
- PgBouncer prepared statements changelog: [PgBouncer 1.21 release notes](https://www.pgbouncer.org/changelog.html#pgbouncer-121x)
