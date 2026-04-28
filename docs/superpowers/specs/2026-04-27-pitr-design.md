# Design: WAL Archiving + Point-in-Time Recovery

- **Date:** 2026-04-27
- **Status:** Draft — pending implementation
- **Roadmap position:** Item 3 of 10 in the `db-roadmap` GitHub label
- **GitHub issue:** [#157 — WAL archiving + Point-in-Time Recovery](https://github.com/kabradshaw1/portfolio/issues/157)
- **Builds on:**
  - `docs/adr/infrastructure/2026-04-24-postgres-data-integrity.md` — daily `pg_dump`, recovery runbook (this is the explicit Phase 2 follow-up)
  - `docs/runbooks/postgres-recovery.md` — existing recovery scenarios (extended here)
  - `docs/superpowers/specs/2026-04-27-pg-query-observability-design.md` — postgres_exporter, dashboard, alert patterns

## Context

The shared PostgreSQL 17 instance currently has daily `pg_dump` snapshots with 7-day retention. RPO is up to 24 hours — a disaster at 23:59 loses the entire day's data. The existing data-integrity ADR explicitly lists WAL-based continuous archiving + PITR as Phase 2 — that is what this spec delivers.

A production system would treat 24h RPO as unacceptable for any data the business cares about. WAL archiving narrows RPO to seconds (every committed transaction is in a WAL segment that gets archived as soon as the segment is full or `archive_timeout` elapses), and base backups + the WAL archive together let an operator recover to any specific timestamp.

This is also baseline interview vocabulary: RPO, RTO, and "how do you do PITR in Postgres?" are standard backend questions.

## Goals

- Continuously archive WAL segments to durable storage as they fill.
- Take a weekly `pg_basebackup` so any restore has a base to apply WAL on top of.
- Enable point-in-time recovery: given a target timestamp, an operator can restore the cluster to its state at that moment.
- Detect archiving failures fast (silent archive failure stalls WAL recycling and eventually fills the data PVC).
- Document the PITR procedure in the existing recovery runbook so an on-call engineer can execute it under pressure.

## Non-goals

- Streaming replication / read replica (separate roadmap item #161).
- Off-host backup storage (S3/GCS) — bound by the no-paid-services constraint.
- Automated weekly PITR drills (separate roadmap item #158, "automated backup verification").
- WAL-based logical replication (`wal_level = logical`) — separate roadmap item #163.
- Multi-region / DR-to-another-site posture.

## Architecture

```
Postgres pod (java-tasks namespace, single replica, Recreate strategy)
│
├── volume: postgres-data           PVC, existing                /var/lib/postgresql/data
├── volume: postgres-backup         hostPath PV, existing        /backups/postgres        (pg_dump output)
├── volume: postgres-wal-archive    hostPath PV, NEW             /backups/wal-archive
└── volume: postgres-basebackup     hostPath PV, NEW             /backups/basebackup
                                     │
   args (extends existing -c flags from query-observability spec):
     -c archive_mode=on
     -c archive_command='/usr/local/bin/pg-archive-wal.sh %p %f'
     -c archive_timeout=300         (force WAL switch every 5 min when idle)
     -c wal_level=replica           (default; explicit for clarity)

   ConfigMap: postgres-wal-scripts (mounted at /usr/local/bin)
     - pg-archive-wal.sh            atomic, idempotent archiver
     - pg-basebackup-and-prune.sh   weekly base backup + WAL retention

CronJobs (java-tasks namespace):
   - postgres-basebackup            Sundays 03:00 UTC, runs pg-basebackup-and-prune.sh
                                     in a postgres-client image with both backup PVs
                                     and the WAL archive PV mounted

postgres_exporter sidecar           existing — gains scrape of pg_stat_archiver
                                     view (built-in, no custom query needed)

Grafana                             extends existing PostgreSQL dashboard with
                                     three new panels; appends three new alerts to
                                     the PostgreSQL alert group
```

Three independent data flows: (1) WAL archive — every committed transaction lands in a WAL segment that the wrapper script copies to the archive PV; (2) base backups — weekly tarball lands in the basebackup PV; (3) observability — `pg_stat_archiver` view is scraped, dashboard and alerts derive from it.

## PostgreSQL configuration

Adds three new `args` flags to the postgres container (extending the args from the query-observability spec):

```yaml
args:
  # ...existing flags from pg-query-observability spec...
  - "-c"
  - "archive_mode=on"
  - "-c"
  - "archive_command=/usr/local/bin/pg-archive-wal.sh %p %f"
  - "-c"
  - "archive_timeout=300"
  - "-c"
  - "wal_level=replica"
```

`wal_level=replica` is the default in PG 17, but setting it explicitly documents intent and protects against future config drift.

`archive_timeout=300` forces a WAL switch every 5 minutes during idle periods. Without it, an idle database might never fill a WAL segment, leaving recent commits unarchived for arbitrary durations.

## The archive script

`pg-archive-wal.sh` (mounted from `postgres-wal-scripts` ConfigMap into `/usr/local/bin/` in the postgres container):

```bash
#!/bin/sh
# Postgres archive_command target.
#   $1 = %p — absolute path to the WAL segment in pg_wal/
#   $2 = %f — segment filename
# Contract: exit 0 on success. Anything non-zero retries forever.
set -eu

SRC="$1"
FN="$2"
DST_DIR="/var/lib/postgresql/wal-archive"
DST="$DST_DIR/$FN"
TMP="$DST.tmp"

# Refuse to overwrite — would corrupt the archive timeline.
if [ -e "$DST" ]; then
  echo "pg-archive-wal: $FN already archived; refusing overwrite" >&2
  exit 1
fi

cp "$SRC" "$TMP"
sync "$TMP"
mv "$TMP" "$DST"
```

Notes:
- Atomic via temp + rename (mv on the same filesystem is atomic).
- `sync` after `cp` ensures the segment is durable before Postgres considers it archived and recycles the source.
- Refuses to overwrite. An overwrite would silently break the timeline; failing loud is correct.
- Runs as the `postgres` user (the image entrypoint already drops privileges before running `archive_command`).

## Base backup + WAL retention

A weekly CronJob runs `pg-basebackup-and-prune.sh` in a postgres-client container. The script does two things atomically:

```bash
#!/bin/sh
set -eu

STAMP=$(date -u +%Y%m%dT%H%M%SZ)
BACKUP_DIR="/backups/basebackup/$STAMP"
ARCHIVE_DIR="/backups/wal-archive"
RETAIN=4   # keep 4 most recent weekly base backups

mkdir -p "$BACKUP_DIR"

# 1) Take the base backup as a tarball, with WAL inline so this backup is
#    self-restorable even if the archive is lost.
pg_basebackup \
  --host=postgres.java-tasks.svc.cluster.local \
  --username=replicator \
  --pgdata="$BACKUP_DIR" \
  --format=tar \
  --gzip --compress=6 \
  --wal-method=fetch \
  --label="weekly-$STAMP" \
  --checkpoint=fast \
  --progress

# 2) Prune base backups older than the retention window.
ls -1d /backups/basebackup/*/ 2>/dev/null \
  | sort | head -n -"$RETAIN" | xargs -r rm -rf

# 3) Find the SECOND-most-recent surviving base backup. Retain WAL >= the
#    earliest WAL needed by that backup, so we always have recovery from
#    at least the previous-but-one weekly point. Use pg_archivecleanup.
SECOND_NEWEST=$(ls -1d /backups/basebackup/*/ | sort | tail -n 2 | head -n 1)
if [ -n "$SECOND_NEWEST" ] && [ -f "$SECOND_NEWEST/backup_label.gz" ]; then
  START_WAL=$(zcat "$SECOND_NEWEST/backup_label.gz" \
              | awk '/^START WAL LOCATION:/ {print $6; exit}' | tr -d '()')
  if [ -n "$START_WAL" ]; then
    pg_archivecleanup "$ARCHIVE_DIR" "$START_WAL"
  fi
fi
```

Why `--wal-method=fetch`? It pulls the WAL into the base backup tarball directly, so a single backup tarball is restorable on its own — even if the WAL archive is lost. The downside (a slightly larger tarball) is acceptable.

Why retain WAL from the *second*-most-recent base backup? Defence in depth: if the most recent base backup is corrupt, the previous backup + WAL gives a fallback path.

A new database role `replicator` with `REPLICATION` privilege is created (separate from `taskuser`) so the base-backup process has only the privileges it needs. Created by a one-shot Job analogous to the `grafana_reader` Job in the query-observability spec.

## CronJob manifest

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: postgres-basebackup
  namespace: java-tasks
spec:
  schedule: "0 3 * * 0"   # Sundays 03:00 UTC, one hour after the daily pg_dump
  concurrencyPolicy: Forbid
  successfulJobsHistoryLimit: 3
  failedJobsHistoryLimit: 3
  jobTemplate:
    spec:
      backoffLimit: 1
      template:
        spec:
          restartPolicy: OnFailure
          containers:
            - name: basebackup
              image: postgres:17-alpine
              env:
                - name: PGPASSWORD
                  valueFrom:
                    secretKeyRef:
                      name: java-secrets
                      key: replicator-password
              command: ["/usr/local/bin/pg-basebackup-and-prune.sh"]
              volumeMounts:
                - name: scripts
                  mountPath: /usr/local/bin/pg-basebackup-and-prune.sh
                  subPath: pg-basebackup-and-prune.sh
                - name: basebackup
                  mountPath: /backups/basebackup
                - name: wal-archive
                  mountPath: /backups/wal-archive
          volumes:
            - name: scripts
              configMap:
                name: postgres-wal-scripts
                defaultMode: 0755
            - name: basebackup
              persistentVolumeClaim:
                claimName: postgres-basebackup
            - name: wal-archive
              persistentVolumeClaim:
                claimName: postgres-wal-archive
```

## Storage layout

Two new hostPath PVs and PVCs follow the existing `postgres-backup-pv.yml` shape:

| PV / PVC | Size | hostPath on Minikube node | Mount in postgres pod | Mount in CronJob |
|---|---|---|---|---|
| postgres-data (existing) | 5Gi | `/data/postgres` | `/var/lib/postgresql/data` | — |
| postgres-backup (existing) | 5Gi | `/backups/postgres` | — | — |
| postgres-wal-archive (NEW) | 10Gi | `/backups/wal-archive` | `/var/lib/postgresql/wal-archive` | `/backups/wal-archive` |
| postgres-basebackup (NEW) | 10Gi | `/backups/basebackup` | — | `/backups/basebackup` |

Sizing: at portfolio traffic, daily WAL volume is ~50–200 MB compressed. 10 Gi covers months of archive even if pruning failed. Base backup tarballs are ~200–500 MB compressed each; 4 retained × 500 MB = 2 GB worst case, with margin.

## Observability

`pg_stat_archiver` is a Postgres-built-in view exposing archiver health. `postgres_exporter` v0.16.0 already scrapes it; no custom query needed. Three metrics matter:

- `pg_stat_archiver_archived_count` — total successful archives (counter)
- `pg_stat_archiver_failed_count` — total failures (counter)
- `pg_stat_archiver_last_archived_time` — timestamp of last success

### Dashboard panels

Append three panels to the existing PostgreSQL health dashboard (the one shipped in the data-integrity ADR), not the new query-performance dashboard:

| Panel | Type | Source | Purpose |
|---|---|---|---|
| Time since last WAL archive | Stat | Prometheus | At-a-glance freshness — should be < `archive_timeout` (5 min) under normal load |
| Archive failures (5m rate) | Time series | Prometheus | Detects archive_command flapping |
| Time since last base backup | Stat | Prometheus (kube-state-metrics) | Should be < 8 days |

The "time since last base backup" panel sources from `kube_cronjob_status_last_successful_time{cronjob="postgres-basebackup"}`, exposed by the existing kube-state-metrics deployment. No new exporter or textfile collector is needed.

### Alerts

Append three rules to the existing `PostgreSQL` group in `grafana-alerting.yml`. All use `noDataState: OK` per project convention.

| Alert | Threshold | Why |
|---|---|---|
| `PgArchiveCommandFailing` | `rate(pg_stat_archiver_failed_count[10m]) > 0` for 5m | Archive failures stall WAL recycling and silently break PITR |
| `PgWalArchiveStale` | `time() - pg_stat_archiver_last_archived_time > 600` for 5m | No archive in 10 min when `archive_timeout=300` indicates archive_command is broken |
| `PgBasebackupStale` | `time() - kube_cronjob_status_last_successful_time{cronjob="postgres-basebackup"} > 691200` (8 days) | One-day buffer past the 7-day schedule for transient failures |

## PITR runbook

A new section is appended to the existing `docs/runbooks/postgres-recovery.md`. Structure mirrors the existing three scenarios (symptoms, prerequisites, step-by-step, verification):

> **Scenario 4: Point-in-time recovery to a specific timestamp**
>
> *Symptoms:* an operator-induced data corruption (bad migration, accidental DELETE, malicious write) that needs to be undone without taking the database back further than necessary.
>
> *Prerequisites:* the target timestamp falls within the WAL archive retention window; at least one base backup exists from before the target timestamp.
>
> *Steps:*
> 1. Stop Postgres (`kubectl scale deployment postgres -n java-tasks --replicas=0`).
> 2. Choose the most recent base backup taken *before* the target timestamp. List candidates with `kubectl exec` into a debug pod with the basebackup PV mounted.
> 3. Restore that base backup tarball to a fresh data dir (the `postgres-data` PVC is wiped or replaced).
> 4. Drop a `recovery.signal` file in the data dir.
> 5. Add `restore_command='cp /var/lib/postgresql/wal-archive/%f %p'` and `recovery_target_time='YYYY-MM-DD HH:MM:SS UTC'` and `recovery_target_action='promote'` to `postgresql.auto.conf`.
> 6. Scale Postgres back up. It enters recovery, replays WAL until the target time is reached, and promotes.
> 7. Verify by querying for the corruption-causing row's absence (or the missing row's presence, depending on direction).
>
> Each step has the exact `kubectl` / `psql` commands inline in the runbook.

The cross-link from the existing recovery scenarios is updated so on-call sees PITR as a peer of the existing options.

## Testing

**Integration test** (Go, testcontainers, build-tagged) at `go/pkg/db/wal_archive_integration_test.go`:

1. Start Postgres in a container with `archive_mode=on`, `archive_timeout=2`, and `archive_command` pointing at a directory mounted from a host tmpdir.
2. Insert a row, force a WAL switch (`SELECT pg_switch_wal()`), wait briefly.
3. Assert at least one file appears in the archive directory.
4. Verify the archive script's idempotency: re-run with the same target should fail loudly (exit non-zero), confirming the no-overwrite contract.

**QA smoke test** (manual, in QA cluster):

1. After deploy, exec into the postgres pod and run `SELECT * FROM pg_stat_archiver;` — verify `archived_count > 0` and `last_archived_time` is recent.
2. Verify a WAL file is visible on the host: `ls /backups/wal-archive/` on the Minikube node.
3. Trigger a manual base backup: `kubectl create job --from=cronjob/postgres-basebackup postgres-basebackup-manual -n java-tasks-qa`. Verify a new tarball appears.
4. Run a PITR drill in QA per the runbook — restore to a 5-minutes-ago timestamp on a parallel temporary deployment, query a known marker.

The continuous PITR-restore verification is delegated to the next roadmap item (#158, "automated backup verification") so this spec stays focused.

## Rollout

Each step is independently mergeable.

1. Land the wrapper scripts ConfigMap.
2. Add the two new PVs/PVCs.
3. Create the `replicator` role via a one-shot Job (analogous to the `grafana_reader` Job).
4. Update the postgres deployment with the new `args` and the wal-archive volume mount. Deploy → `Recreate` restart picks it up.
5. Add the basebackup CronJob.
6. Run the basebackup manually once to seed the basebackup PV.
7. Add dashboard panels.
8. Add alerts.
9. Run the manual QA PITR drill and append findings to the runbook.

## ADR

Companion ADR at `docs/adr/database/wal-archiving-pitr.md` (the `database/` directory is created by either this work or the migration-lint work, whichever lands first). The ADR documents:

- Why native `archive_command` over pgBackRest (interview narrative + matches existing pg_dump pattern)
- Why two retained base backups (defence-in-depth against backup corruption)
- Why hostPath PVs over off-host storage (no-paid-services constraint, with the explicit acknowledgement that disk failure is unmitigated)
- The `archive_timeout = 300` choice and what it costs in WAL volume
- The decision to bundle WAL into the base backup tarball (`--wal-method=fetch`)

## Consequences

**Positive:**
- RPO drops from ≤ 24h to seconds (one `archive_timeout` window plus pending in-flight WAL).
- An operator can recover to any timestamp within the archive retention window — the most powerful Postgres recovery primitive.
- Archive failures and base-backup staleness are alerted, not silent.
- The runbook turns "we have backups" into "we know how to restore to a specific second."

**Trade-offs:**
- `archive_mode=on` requires a Postgres restart. Acceptable given the existing `Recreate` strategy.
- Disk usage grows by ~10 GB (two hostPath PVs).
- WAL archiving is on the same physical disk as the data PVC. Disk failure remains unmitigated — that's a Phase 3 problem (off-host or replica).
- The wrapper script's failure mode (refuses to overwrite) is loud-fail; an operator might see legitimate alert noise during disaster-recovery exercises and need to consciously bypass.

**Phase 2 (future, separate roadmap items):**
- Automated weekly PITR drill (#158 — backup verification)
- Read replica via streaming replication (#161) — the same WAL stream that feeds the archive can feed a hot standby
- Off-host archive (S3-compatible) once the no-paid-services constraint relaxes
