# PostgreSQL Data Integrity & Observability

**Date:** 2026-04-23
**Status:** Draft
**Scope:** Phase 1 (breadth) — automated backups, monitoring, PDB, recovery runbook

## Context

The shared PostgreSQL instance in `java-tasks` namespace hosts 9 databases (7 prod, 2 QA) serving all Go and Java services. A WAL checkpoint corruption on 2026-04-23 took down every Go service and required a full PVC reset. There were no backups, no Postgres-specific monitoring, and no documented recovery procedure. The recovery relied on the fact that all data is seeded — a luxury production systems don't have.

This spec adds the operational tooling a production Postgres deployment requires, within the constraint of a single-node Minikube cluster with no paid cloud services.

## 1. pg_dump Backup CronJob

### Schedule & Retention
- Runs daily at 02:00 UTC via a Kubernetes CronJob in `java-tasks` namespace
- Keeps 7 days of backups, deletes older files after each run

### Implementation
- **Image:** `postgres:17-alpine` (matches the running instance — no version skew)
- **Format:** `pg_dump --format=custom` (`-Fc`) per database — supports parallel restore and selective table restore
- **Databases:** `authdb`, `orderdb`, `productdb`, `cartdb`, `paymentdb`, `ecommercedb`, `projectordb` (prod only, not QA — QA data is fully recreatable from migrations and seeds)
- **Output:** Individual files named `<db>-YYYY-MM-DD.dump` written to `/backups/postgres/` on the host
- **Error handling:** `set -e` fails the job on any single database failure; `backoffLimit: 1` retries once
- **Cleanup:** After successful dump, `find /backups/postgres -name '*.dump' -mtime +7 -delete`

### Storage
- **hostPath PersistentVolume** mounted at `/backups/postgres` on the Debian host
- Lives outside Minikube's dynamic PVC provisioner — survives PVC deletion, pod restarts, and the exact WAL corruption scenario that triggered this work
- Separate PV/PVC pair so the backup storage lifecycle is independent of the database storage

### Prerequisites
- The directory `/backups/postgres` must exist on the Debian host with appropriate permissions. The deploy step (CI or `deploy.sh`) should create it via SSH if it doesn't exist: `mkdir -p /backups/postgres`

### Manifests
- `java/k8s/jobs/postgres-backup.yml` (CronJob)
- `java/k8s/volumes/postgres-backup-pv.yml` (hostPath PV + PVC)

## 2. PodDisruptionBudget

- `maxUnavailable: 0` for the Postgres deployment
- Prevents voluntary eviction during node drains, upgrades, or rebalancing
- Involuntary disruptions (OOM, node crash) still terminate the pod — this only blocks voluntary ones
- Different from Go service PDBs (`maxUnavailable: 1`) because Postgres is a single-replica stateful workload with no redundancy

### Manifest
- `java/k8s/pdb/postgres-pdb.yml`

## 3. postgres_exporter Sidecar

### Deployment
- `prometheuscommunity/postgres-exporter` added as a sidecar container in `java/k8s/deployments/postgres.yml`
- Connects via `localhost:5432` (same pod, no network hop)
- Exposes metrics on port `9187` at `/metrics`
- Prometheus discovers it via existing `kubernetes-pods` scrape config using pod annotations (`prometheus.io/scrape: "true"`, `prometheus.io/port: "9187"`)

### Key Metrics
| Metric | Purpose |
|--------|---------|
| `pg_stat_activity_count` | Active connections by state (active, idle, waiting) |
| `pg_stat_database_blks_hit` / `blks_read` | Cache hit ratio calculation |
| `pg_stat_database_tup_*` | Row throughput (fetched, inserted, updated, deleted) |
| `pg_stat_database_xact_commit` / `xact_rollback` | Transaction rates |
| `pg_stat_database_deadlocks` | Deadlock count |
| `pg_database_size_bytes` | Per-database disk usage |
| `pg_settings_max_connections` | Connection limit for utilization % |

### Configuration
- `DATA_SOURCE_NAME` env var using the existing `java-secrets` secret for the password
- Default collectors only — no custom queries needed

## 4. Grafana Dashboard & Alerts

### Dashboard: "PostgreSQL"
Provisioned via configmap alongside existing dashboards.

| Panel | Type | Query |
|-------|------|-------|
| Connection utilization | Gauge | active connections / max_connections |
| Cache hit ratio | Stat | blks_hit / (blks_hit + blks_read), target >99% |
| Transaction rate | Time series | commits and rollbacks over time |
| Database sizes | Bar chart | pg_database_size_bytes per database |
| Deadlocks | Time series | deadlock count over time |
| Last successful backup | Stat | time since kube_job_status_completion_time for backup job |

### Alert Rules
Added to `k8s/monitoring/configmaps/grafana-alerting.yml`. All rules use `noDataState: OK`.

| Rule | Condition | Severity | For |
|------|-----------|----------|-----|
| Postgres connection utilization high | active connections > 80% of max_connections | warning | 5m |
| Postgres cache hit ratio low | cache hit ratio < 95% | warning | 10m |
| Postgres deadlocks detected | deadlocks > 0 in 5m window | warning | 0s |
| Postgres backup stale | no successful backup job completion in 26h | critical | 0s |

The backup-stale alert uses `kube_job_status_completion_time{job_name=~"postgres-backup.*"}` from kube-state-metrics (already deployed).

## 5. Recovery Runbook

Markdown document at `docs/runbooks/postgres-recovery.md` covering three scenarios:

### Scenario 1: Corrupted WAL / Postgres won't start
- **Symptoms:** Postgres CrashLoopBackOff, logs show `PANIC: could not locate a valid checkpoint record`, all dependent services returning 503
- **Procedure:** Scale down Postgres, delete PVC, recreate PVC, verify init scripts create all databases, restart Postgres, re-run migration jobs (auth first — other services reference users table), verify all services recover
- **Data impact:** Loses all data not in seeds. Use Scenario 2 if backups exist.

### Scenario 2: Restore from pg_dump backup
- **Symptoms:** Data corruption, accidental deletion, or need to roll back to a known-good state
- **Procedure:** Find latest backup on host (`/backups/postgres/`), `pg_restore` per database, verify table counts and smoke test
- **When to use:** When you need to preserve user-created data that isn't in seeds

### Scenario 3: Partial restore (single database)
- **Symptoms:** One service's database is corrupted but others are healthy
- **Procedure:** Drop and restore the affected database only, re-run that service's migration job if restoring from scratch
- **Advantage:** Minimizes blast radius — other services stay online

### Each scenario includes
- Symptoms (logs/alerts that indicate this scenario)
- Prerequisites (what you need before starting)
- Step-by-step commands
- Verification steps
- Time estimate
- Cross-references to Grafana alert names

## Files Changed

| File | Change |
|------|--------|
| `java/k8s/jobs/postgres-backup.yml` | New — CronJob |
| `java/k8s/volumes/postgres-backup-pv.yml` | New — hostPath PV + PVC for backup storage |
| `java/k8s/pdb/postgres-pdb.yml` | New — PDB for Postgres |
| `java/k8s/deployments/postgres.yml` | Modified — add postgres_exporter sidecar + annotations |
| `k8s/monitoring/configmaps/grafana-dashboards.yml` | Modified — add PostgreSQL dashboard |
| `k8s/monitoring/configmaps/grafana-alerting.yml` | Modified — add 4 Postgres alert rules |
| `docs/runbooks/postgres-recovery.md` | New — recovery runbook |

## Future Work (Phase 2 — Depth)

Not in scope for this spec, but natural next steps:
- **WAL archiving with PITR** — continuous archiving to the backup volume, point-in-time recovery
- **Automated recovery testing** — periodic CronJob that restores a backup to a temp database and validates it
- **Connection pooling (PgBouncer)** — if connection count grows with more services
