# ADR: PostgreSQL Data Integrity & Observability (2026-04-24)

## Status
Accepted

## Context

The shared PostgreSQL 17 instance in `java-tasks` namespace hosts 9 databases (7 prod, 7 QA, plus `taskdb`) serving all Go and Java services. On 2026-04-23, a WAL checkpoint corruption (`PANIC: could not locate a valid checkpoint record`) took down every Go and Java service. The same corruption recurred on 2026-04-24 during a deployment that added a postgres_exporter sidecar.

Investigation revealed:

1. **No backups existed.** Recovery required a full PVC reset and reseed — acceptable only because all data was recreatable from migrations and seeds. A production system with user-generated data would have suffered total data loss.
2. **No Postgres-specific monitoring.** The only indication was cascading 503s from dependent services. There were no connection pool, cache, or disk metrics.
3. **No PodDisruptionBudget.** Postgres could be voluntarily evicted during node drains.
4. **The default `RollingUpdate` deployment strategy caused the WAL corruption.** With a single-replica `ReadWriteOnce` PVC, `RollingUpdate` starts the new pod before terminating the old one. Since only one pod can mount an RWO PVC, Kubernetes forcibly kills the old pod — bypassing the `preStop` hook's graceful `pg_ctl stop`. This corrupted the WAL on both occasions.
5. **Alert rules defaulted `noDataState` to `NoData`**, which fires alerts when Prometheus rate queries return empty results after pod restarts. This produced 20+ false-positive Telegram notifications after each incident, making it impossible to distinguish real problems from noise.

Spec: `docs/superpowers/specs/2026-04-23-postgres-data-integrity-design.md`. Plan: `docs/superpowers/plans/2026-04-24-postgres-data-integrity.md`.

## Decisions

### Recreate deployment strategy for Postgres

Changed `strategy.type` from `RollingUpdate` (Kubernetes default) to `Recreate`. This ensures the old pod terminates completely — including running the `preStop` hook (`pg_ctl stop -m fast`) — before the new pod starts. The trade-off is a brief window of unavailability during deploys (typically 10-15 seconds), but for a single-replica stateful workload with no redundancy, a clean shutdown is worth more than zero-downtime rollouts.

The `terminationGracePeriodSeconds: 90` and `preStop` hook were already in place; the problem was that `RollingUpdate` bypassed them by force-killing the old pod when the new pod needed the PVC.

### Daily pg_dump backups with 7-day retention

A CronJob (`postgres-backup`) runs at 02:00 UTC, dumping each prod database individually using `pg_dump --format=custom`. Custom format was chosen over plain SQL because it supports parallel restore and selective table restore. Individual per-database dumps (not `pg_dumpall`) allow restoring a single database without touching others.

Backups write to a hostPath PV at `/backups/postgres` on the Minikube node. This is separate from the Postgres data PVC — a PVC corruption or deletion doesn't affect backups. The hostPath sits on the Minikube node's persistent disk (`/dev/nvme1n1p2`), so it survives pod restarts and Minikube reboots.

QA databases are not backed up — they're fully recreatable from migrations and seeds.

**Trade-off vs. WAL archiving:** pg_dump provides daily snapshots (RPO ≤ 24 hours). WAL-based continuous archiving would provide point-in-time recovery (RPO ≈ 0), but requires `archive_command` configuration, a WAL archive volume, and `pg_basebackup` for base backups — significantly more complexity. pg_dump is the right first step; WAL archiving is planned for Phase 2.

**Trade-off vs. cloud storage:** A production system would upload backups to S3/GCS for off-host durability. The hostPath volume lives on the same physical disk as the data — it protects against PVC corruption and accidental deletion, but not disk failure. This is the "no paid services" constraint accepted for the portfolio.

### postgres_exporter sidecar

Added `prometheuscommunity/postgres-exporter:v0.16.0` as a sidecar container in the Postgres pod. It connects via `localhost:5432` (same pod, no network hop) and exposes metrics on port 9187. Prometheus discovers it via the existing `kubernetes-pods` scrape config using pod annotations.

Sidecar was chosen over a separate Deployment because: (a) it shares the pod lifecycle with Postgres — no stale metrics from a running exporter pointing at a dead database, (b) localhost connection eliminates network config and auth complexity, (c) it's the standard pattern for database exporters in Kubernetes.

### Grafana PostgreSQL dashboard

Six panels covering the operational metrics that matter for a shared database:

| Panel | Why |
|-------|-----|
| Connection utilization (gauge) | The #1 operational risk for a shared Postgres — if connections saturate, all services fail simultaneously |
| Cache hit ratio (stat) | Should be >99%; a drop means queries are hitting disk and shared_buffers needs tuning |
| Last successful backup (stat) | Time since the most recent CronJob completion — turns the backup into something visible, not just a silent cron |
| Database sizes (bar gauge) | Early warning for disk pressure on the 2Gi PVC |
| Transaction rate (time series) | Commits + rollbacks per database — shows load distribution and detects anomalies |
| Deadlocks (time series) | Should be zero; any spike indicates a query pattern problem |

### Four alert rules with noDataState: OK

| Alert | Threshold | Why |
|-------|-----------|-----|
| Connection utilization > 80% | 5m | Gives time to investigate before saturation |
| Cache hit ratio < 95% | 10m | Longer window to avoid transient dips after restarts |
| Deadlocks > 0 | immediate | Any deadlock warrants investigation |
| Backup stale > 26h | immediate | 2h buffer past the 24h schedule to account for transient failures |

All rules use `noDataState: OK`. The earlier incident proved that `NoData` (Grafana's default) generates unacceptable false-positive noise after pod restarts. For rate-based and event-based metrics, no data means no activity, which means no problem. This was also applied retroactively to all 22 existing alert rules that had the same issue.

### PodDisruptionBudget (maxUnavailable: 0)

Prevents voluntary eviction of the Postgres pod during `kubectl drain`, node upgrades, or cluster rebalancing. This does NOT prevent the `Recreate` deployment strategy from terminating the pod (that's a controller-initiated change, not a voluntary disruption). It protects against the scenario where a cluster maintenance operation kills Postgres without the operator intending to.

`maxUnavailable: 0` was chosen over `minAvailable: 1` because the Go service PDBs use `maxUnavailable: 1` (as noted in `go/CLAUDE.md`) — using the same field style but with a stricter value makes the intent clear: this workload must never be disrupted.

### Recovery runbook

Three scenarios documented in `docs/runbooks/postgres-recovery.md`:

1. **Corrupted WAL / fresh PVC reset** — for when Postgres won't start and either no backups exist or data is all seeded. This is the procedure we executed twice on 2026-04-23/24.
2. **Full restore from pg_dump** — for when backups exist and data preservation matters.
3. **Partial restore (single database)** — for when one service's database is corrupted but others are healthy, minimizing blast radius.

Each scenario includes symptoms, prerequisites, step-by-step commands, and verification steps. The runbook references Grafana alert names so an on-call engineer knows which alert maps to which procedure.

### Postgres init scripts updated

Added `ecommercedb` and `projectordb` to `java/k8s/configmaps/postgres-initdb.yml`. These databases were previously created dynamically during CI deploy but not by the init scripts — so a PVC reset required manual `CREATE DATABASE` commands. Now all 7 prod databases are created automatically on fresh initialization.

QA databases (`*_qa`) are still created dynamically during deploy because they follow a naming convention (suffix) rather than being fixed names in the init scripts.

### Auth-service smoke user seed

Moved the smoke test user (`smoke@kylebradshaw.dev`) from `go/order-service/seed.sql` (where it referenced a `users` table that no longer exists in `orderdb` post-decomposition) to a new `go/auth-service/seed.sql`. Updated the auth-service Dockerfile and migration job to include a seed step. This ensures the smoke user is automatically seeded into `authdb` on every deploy — including after a full PVC reset.

## Consequences

**Positive:**
- Automated daily backups with a tested restore procedure — proven to work when the second WAL corruption hit 30 minutes after the first backup ran.
- Postgres-specific observability that didn't exist before — connection saturation, cache health, and backup freshness are now visible and alerted.
- `Recreate` strategy eliminates the root cause of both WAL corruptions.
- `noDataState: OK` eliminates the false-positive alert storms that made incident response harder.
- Recovery time for a full PVC reset dropped from ~30 minutes (manual investigation + ad hoc commands) to ~5 minutes (follow the runbook).

**Trade-offs:**
- `Recreate` introduces 10-15 seconds of Postgres unavailability during deploys. All dependent services have circuit breakers and retry logic, so this manifests as brief connection errors, not data loss.
- Backups are on the same physical disk as data — protects against logical corruption and PVC deletion, but not disk failure.
- pg_dump snapshots have up to 24h RPO. WAL archiving (Phase 2) would close this gap.
- The postgres_exporter sidecar adds ~32-64Mi memory to the Postgres pod. Acceptable given the 768Mi limit and the value of the metrics.

**Phase 2 (future):**
- WAL-based continuous archiving with point-in-time recovery
- Automated backup verification (periodic restore to a temp database)
- Connection pooling via PgBouncer if connection count grows
