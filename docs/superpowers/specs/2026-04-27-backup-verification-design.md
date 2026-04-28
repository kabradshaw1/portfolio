# Design: Automated Backup Verification

- **Date:** 2026-04-27
- **Status:** Draft — pending implementation
- **Roadmap position:** Item 4 of 10 in the `db-roadmap` GitHub label
- **GitHub issue:** [#158 — Automated backup verification](https://github.com/kabradshaw1/portfolio/issues/158)
- **Builds on:**
  - `docs/adr/infrastructure/2026-04-24-postgres-data-integrity.md` (the `pg_dump` CronJob this verifies)
  - `docs/superpowers/specs/2026-04-27-pitr-design.md` (the WAL archive + base backups verified in Phase 2)
  - `docs/runbooks/postgres-recovery.md` (the recovery scenarios this proves are working)

## Context

The shared PostgreSQL instance has two backup mechanisms once the prior roadmap items land: (1) daily `pg_dump` files and (2) weekly `pg_basebackup` + continuous WAL archive. **Neither is verified.** A backup file that exists on disk says nothing about whether it can be restored — backups silently corrupt, scripts silently break, disk failures masquerade as healthy writes.

The most common cause of catastrophic data loss in production systems is not "we had no backups" but "we had backups that didn't restore." A senior engineer is expected to treat untested backups as no backups, and to wire continuous verification into the same observability stack as the rest of the system.

This spec ships an automated weekly verification CronJob that actually restores each backup type, runs smoke checks, and emits Prometheus metrics that drive a "we know our backups work" alert. The work is split into two phases — Phase 1 (`pg_dump` verification) ships first; Phase 2 (PITR verification) ships once roadmap item #157 has merged.

## Goals

- Restore every weekly `pg_dump` file to a fresh, isolated Postgres and run smoke checks against it.
- Restore the latest `pg_basebackup` + replay WAL to a randomly chosen point-in-time, and verify the resulting state matches an expected sentinel (Phase 2).
- Emit per-database Prometheus metrics so dashboards and alerts can see *which* DB's verification is failing or stale.
- Alert on stale verification (no success in > 8 days) and on outright verification failure.
- Run the verification in complete isolation from prod traffic — restoration happens in an ephemeral Postgres process inside the verify pod.

## Non-goals

- Cross-region or cross-cluster verification.
- Schema-evolution-aware verification (e.g., diffing migration history) — that's a separate concern.
- Verification of `pg_dump` files older than the latest (the most recent file is the only one that matters operationally).
- Java-managed schemas — Spring/JPA owns those at startup; nothing to verify against a backup.

## Architecture

```
monitoring namespace
└── Pushgateway (NEW)              prom/pushgateway:v1.9.0
    ├── Deployment, single replica
    ├── PVC for /metrics persistence
    └── Service exposed on :9091
                ▲
                │ scraped by Prometheus via existing kubernetes-services job
                │
java-tasks namespace
└── postgres-backup-verify (NEW CronJob, Mondays 04:00 UTC)
    └── single Job → single pod → single container running:
        Volumes:
          • emptyDir   at /var/lib/postgresql/data    (ephemeral postgres data dir)
          • PV (RO)    at /backups/postgres           (existing pg_dump output)
          • PV (RO)    at /backups/wal-archive        (Phase 2 only — from #157)
          • PV (RO)    at /backups/basebackup        (Phase 2 only — from #157)
          • ConfigMap  at /usr/local/bin             (the verification scripts)

        Container = postgres:17-alpine + curl + python3
          1. initdb -D /var/lib/postgresql/data
          2. start postgres locally on socket only (no TCP)
          3. Phase 1: for each *.dump file → createdb _verify → pg_restore →
             run smoke checks → drop → push metric to pushgateway
          4. Phase 2: for the latest base backup → restore tarball → set
             recovery_target_time to a random recent timestamp →
             promote → query a sentinel row → tear down → push metric
          5. exit 0 if all DBs verified, exit 1 otherwise

Grafana
├── extends existing PostgreSQL dashboard (3 new panels)
└── extends PostgreSQL alert group (2 new rules)
```

The verification pod is fully self-contained: it spawns its own Postgres, restores into emptyDir storage, and tears down at exit. The only outbound traffic is metric pushes to Pushgateway. Prod Postgres is never touched.

## Pushgateway

Pushgateway is the Prometheus team's blessed solution for batch/cron jobs that don't run long enough to be scraped directly. We add one Pushgateway deployment to the `monitoring` namespace.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: pushgateway
  namespace: monitoring
spec:
  replicas: 1
  template:
    spec:
      containers:
        - name: pushgateway
          image: prom/pushgateway:v1.9.0
          args:
            - --persistence.file=/data/metrics
            - --persistence.interval=5m
          ports:
            - containerPort: 9091
          volumeMounts:
            - name: data
              mountPath: /data
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: pushgateway-data
```

`--persistence.file` ensures metrics survive a Pushgateway pod restart — without it, a Pushgateway crash would wipe verification state and falsely fire the staleness alert.

A `Service` of type ClusterIP exposes the gateway. Prometheus already scrapes services with the appropriate annotations via the existing `kubernetes-services` scrape config — adding `prometheus.io/scrape: "true"` and `prometheus.io/port: "9091"` to the Service is enough.

A 1Gi PVC for persistence is plenty (Pushgateway holds metric snapshots in a single file, ~hundreds of bytes per metric).

## The verification script

Mounted from a new ConfigMap `postgres-verify-scripts` into `/usr/local/bin/` in the verify pod. The Phase 1 script:

```bash
#!/bin/sh
# pg-verify-backups.sh
# Restore each pg_dump file to an ephemeral local Postgres, smoke-check, push metrics.
set -eu

DBS="authdb productdb orderdb cartdb paymentdb ecommercedb"
PUSHGATEWAY="${PUSHGATEWAY_URL:-http://pushgateway.monitoring.svc.cluster.local:9091}"
DUMPS_DIR="/backups/postgres"
DATA_DIR="/var/lib/postgresql/data"
SOCKET_DIR="/tmp/pg-verify"

mkdir -p "$SOCKET_DIR"
chmod 700 "$SOCKET_DIR"

# 1) initdb if empty
if [ ! -f "$DATA_DIR/PG_VERSION" ]; then
  initdb -D "$DATA_DIR" -U verify --auth=trust --no-locale --encoding=UTF8
fi

# 2) start postgres on Unix socket only — no TCP exposure
pg_ctl -D "$DATA_DIR" -l /tmp/pg-verify.log -o "-k $SOCKET_DIR -h ''" -w start

trap 'pg_ctl -D "$DATA_DIR" -m fast stop || true' EXIT

OVERALL_OK=true

for DB in $DBS; do
  echo "Verifying $DB..."
  DUMP=$(ls -t "$DUMPS_DIR/$DB"-*.dump 2>/dev/null | head -1 || true)
  if [ -z "$DUMP" ]; then
    echo "  FAIL: no dump file found for $DB"
    push_failure "$DB" "no_dump_file"
    OVERALL_OK=false
    continue
  fi

  TARGET="${DB}_verify"
  psql -h "$SOCKET_DIR" -U verify -d postgres -c "DROP DATABASE IF EXISTS $TARGET;" >/dev/null
  psql -h "$SOCKET_DIR" -U verify -d postgres -c "CREATE DATABASE $TARGET;" >/dev/null

  if ! pg_restore -h "$SOCKET_DIR" -U verify -d "$TARGET" --no-owner --no-acl "$DUMP"; then
    echo "  FAIL: pg_restore exited non-zero"
    push_failure "$DB" "pg_restore_failed"
    OVERALL_OK=false
    psql -h "$SOCKET_DIR" -U verify -d postgres -c "DROP DATABASE $TARGET;" >/dev/null
    continue
  fi

  # Smoke check: at least one table has rows
  ROWS=$(psql -h "$SOCKET_DIR" -U verify -d "$TARGET" -t -A -c "
    SELECT COALESCE(SUM(reltuples)::bigint, 0)
    FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE c.relkind = 'r' AND n.nspname = 'public';
  ")

  if [ "${ROWS:-0}" -lt 1 ]; then
    echo "  FAIL: restored DB has zero rows in public schema"
    push_failure "$DB" "empty_after_restore"
    OVERALL_OK=false
  else
    echo "  OK: $ROWS rows restored"
    push_success "$DB" "$ROWS" "$DUMP"
  fi

  psql -h "$SOCKET_DIR" -U verify -d postgres -c "DROP DATABASE $TARGET;" >/dev/null
done

# Final overall status
if [ "$OVERALL_OK" = "true" ]; then
  push_overall_success
  exit 0
else
  push_overall_failure
  exit 1
fi
```

The `push_success` / `push_failure` helper functions emit metrics:

```bash
push_success() {
  local db="$1" rows="$2" dump="$3"
  local dump_age_sec
  dump_age_sec=$(( $(date +%s) - $(stat -c %Y "$dump") ))
  cat <<EOF | curl -fsS --data-binary @- "$PUSHGATEWAY/metrics/job/postgres_backup_verify/instance/$db"
# TYPE backup_verification_last_success_timestamp gauge
backup_verification_last_success_timestamp $(date +%s)
# TYPE backup_verification_restored_rows gauge
backup_verification_restored_rows $rows
# TYPE backup_verification_dump_age_seconds gauge
backup_verification_dump_age_seconds $dump_age_sec
EOF
}

push_failure() {
  local db="$1" reason="$2"
  cat <<EOF | curl -fsS --data-binary @- "$PUSHGATEWAY/metrics/job/postgres_backup_verify/instance/$db"
# TYPE backup_verification_last_failure_timestamp gauge
backup_verification_last_failure_timestamp $(date +%s)
# TYPE backup_verification_last_failure_reason gauge
backup_verification_last_failure_reason{reason="$reason"} 1
EOF
}
```

The `instance` label per DB is essential — it's how the alert can say *which* DB failed.

## Phase 2: PITR verification

Once roadmap item #157 (WAL archiving + PITR) lands, a second script `pg-verify-pitr.sh` runs alongside Phase 1 in the same Job. It restores the latest base backup, configures `recovery_target_time` to a random timestamp within the last 6 days (after the second-newest base backup, before now), promotes, and verifies a sentinel row's expected value at that target.

The sentinel: an inserted row with `app_state='verified-2026-04-27T03:00:00Z'` written by the seed and updated by a separate weekly CronJob. The verifier knows what value to expect at any given timestamp and asserts it.

Phase 2 is in the same spec but is conditionally implemented — the plan flags steps as "Phase 2 only; implement after #157 has merged."

## Resource considerations

The verify pod restores all six prod DBs sequentially. At current data sizes (~few hundred MB total dumps), the pod completes in 2–5 minutes. The schedule (Monday 04:00 UTC) deliberately follows the daily `pg_dump` (02:00) and weekly `pg_basebackup` (Sunday 03:00 UTC) so the latest backups exist when verification runs.

CPU/memory: the pod gets `requests: { cpu: "200m", memory: "512Mi" }`, `limits: { cpu: "1", memory: "2Gi" }`. The transient memory spike during `pg_restore` is the limiting factor.

Disk: emptyDir for the data dir is sized via the pod's ephemeral storage limit (`requests.ephemeral-storage: "5Gi"`). For datasets > 5 Gi this would need to switch to a dedicated PVC.

## Observability

### Metrics emitted (per DB, via Pushgateway)

- `backup_verification_last_success_timestamp{instance="<db>"}` — gauge, Unix epoch seconds
- `backup_verification_last_failure_timestamp{instance="<db>"}` — gauge, Unix epoch seconds
- `backup_verification_last_failure_reason{instance="<db>",reason="<reason>"}` — gauge value 1 (set on each failure)
- `backup_verification_restored_rows{instance="<db>"}` — gauge, last successful row count
- `backup_verification_dump_age_seconds{instance="<db>"}` — gauge, age of the verified dump file

### Dashboard panels

Append three panels to the existing PostgreSQL health dashboard:

| Panel | Type | Source | Purpose |
|---|---|---|---|
| Time since last verification (per DB) | Stat (multi-value) | Prometheus | Should be < 8 days for every DB |
| Verification failures (last 30d) | Time series | Prometheus | Spike = trend toward backup unreliability |
| Restored row counts | Bar gauge | Prometheus | Sudden drop = `pg_dump` truncation |

### Alerts (appended to the `PostgreSQL` group, all `noDataState: OK`)

| Alert | Threshold | Why |
|---|---|---|
| `PgBackupVerificationFailed` | `backup_verification_last_failure_timestamp > backup_verification_last_success_timestamp` for 5m, per `instance` | A failure newer than the latest success means the most recent verification is bad |
| `PgBackupVerificationStale` | `time() - backup_verification_last_success_timestamp > 691200` (8 days), per `instance` | Verification hasn't succeeded in over a week — pipeline is silently broken |

Per-instance labels make the alert message say `paymentdb is unverified` rather than `something is unverified`.

## Testing

**Integration test** at `go/pkg/db/backup_verification_integration_test.go` (testcontainers, build-tagged):

1. Spin up a Postgres container, seed a small schema with N rows.
2. Run `pg_dump --format=custom` on it, drop the source DB.
3. Spin up a SECOND Postgres container, run the verification script (mounted from the project) against the dump file.
4. Assert the script exits 0, that the temp `_verify` DB is gone afterward, and that a `curl`-mockable Pushgateway endpoint received the success metric with the right row count.

The Pushgateway interaction can be verified by pointing `PUSHGATEWAY_URL` at a tiny Go test server that records the body, rather than a real Pushgateway. Faster and deterministic.

**Failure-mode test:** corrupt the dump file by truncating, re-run the script, assert it exits 1 and pushes a failure metric.

## Rollout

1. Land Pushgateway (Deployment + PVC + Service + scrape annotations).
2. Land the verification script ConfigMap.
3. Land the CronJob (Phase 1 only).
4. Manually trigger one run (`kubectl create job --from=cronjob/...`) and verify the Pushgateway shows metrics.
5. Land dashboard panels.
6. Land alerts.
7. After #157 has merged: land Phase 2 (PITR verification) as a follow-up PR using the same CronJob.

## ADR

Companion ADR at `docs/adr/database/backup-verification.md` covering:

- Why Pushgateway over kube-state-metrics-only (kube-state-metrics gives "did the cron run" but no per-DB granularity)
- Why ephemeral-Postgres-in-a-pod over restoring into the prod Postgres (isolation; never touches prod connections)
- Why the smoke check is "rows > 0 in public schema" rather than per-table assertions (resilient to schema evolution)
- Why Pushgateway persistence is mandatory (a Pushgateway pod restart without persistence would falsely fire `PgBackupVerificationStale`)
- The Phase 2 sentinel-row design and its assumption that we have a known-state row to verify against

## Consequences

**Positive:**
- Backups go from "we have files" to "we know they restore." Most production teams never close this gap.
- Per-DB metrics turn a backup failure into actionable signal — the alert says exactly which DB to investigate.
- Verification runs in complete isolation from prod, so a slow or failing verification never degrades user-facing latency.

**Trade-offs:**
- Pushgateway is now a piece of stateful monitoring infrastructure. If it goes down without persistence, every verification metric falsely appears stale. The PVC mitigates this; the alert on Pushgateway pod up/down covers the rest.
- Verification CPU/memory burns on the Minikube node weekly. Acceptable at current scale; would be moved to a dedicated node pool at scale.
- The smoke check is shallow ("rows > 0") — a `pg_restore` that quietly drops half the rows would still pass. Strengthening this is Phase 2/3 work (sentinel rows, schema diff).

**Phase 2 / 3:**
- PITR-restore verification (Phase 2 of this spec, after #157 merges)
- Schema diff between prod and verified DB (would need a snapshot of expected schema)
- Cross-region or off-host backup verification once the no-paid-services constraint relaxes
