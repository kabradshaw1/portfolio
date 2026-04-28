# ADR: WAL Archiving + Point-in-Time Recovery (2026-04-27)

## Status
Accepted

## Context

The existing data-integrity work (`docs/adr/infrastructure/2026-04-24-postgres-data-integrity.md`) gave the shared Postgres 17 instance daily `pg_dump` snapshots with 7-day retention. RPO under that posture is up to 24 hours: a disaster at 23:59 loses an entire day's writes.

Phase 2 of that ADR explicitly listed WAL-based continuous archiving as the next step. This ADR documents what landed.

## Decisions

### Native `archive_command` rather than pgBackRest or Wal-G

Postgres ships `archive_command` and `pg_basebackup` in the core image. A POSIX-`sh` wrapper script that does `cp + sync + atomic rename` is ~15 lines and has no operational dependencies beyond the base image. pgBackRest is a more capable tool — incremental backups, encryption, parallel archive, S3 first-class — but it adds a binary, a config file, a separate daemon, and a different mental model from the surrounding pg_dump pipeline.

For the portfolio, the native primitives are the right choice: they teach the underlying mechanism, the failure modes are the ones a textbook covers, and the interview narrative ("I wrote the archive_command myself") is more interesting than "I configured pgBackRest." When we outgrow the script — multi-region, off-host storage, incremental — pgBackRest is the natural next step.

### Two retained base backups, not one

The basebackup script keeps the four most recent weekly base backups, and prunes WAL only down to the *second-most-recent* base backup's `START WAL LOCATION`. This is defence in depth: if the most recent base backup is corrupt or unreadable, the previous backup plus retained WAL still gives a recoverable point.

Cost: roughly an extra week of WAL on disk (~50–200 MB compressed). Trivial against the 10 GB PV.

### `archive_timeout = 300`

Without `archive_timeout`, an idle Postgres might never fill a WAL segment, leaving the most recent commits unarchived for arbitrary durations. `archive_timeout = 300` (5 minutes) forces a WAL switch every five minutes during idle periods, capping the gap between commit and archive at ~5 minutes plus the few seconds the wrapper needs.

The cost is one mostly-empty WAL segment file every 5 idle minutes. At 16 MB per WAL segment, that's ~4.6 GB/day of zero-padded archive in the worst case — pruning catches up at the next weekly base backup, so steady-state WAL on disk is bounded by the inter-basebackup interval, not the timeout.

### `--wal-method=fetch` (WAL bundled into the base backup)

`pg_basebackup` defaults to `stream` WAL mode, which uses a second connection to stream WAL while the base backup runs and writes a small `pg_wal/` directory referencing the live archive. With `fetch`, the WAL needed to make the base backup self-consistent is bundled into the tarball.

The downside (slightly larger tarball) is acceptable. The upside is significant: a `fetch` tarball is restorable on its own — even if the WAL archive directory is later wiped or corrupted, the base backup can still bootstrap a cluster up to its checkpoint. This matters for the no-paid-services constraint where archive and base-backup live on the same physical disk.

### `replicator` role, not reusing `taskuser`

`pg_basebackup` only needs the `REPLICATION` attribute. The application's `taskuser` already has full DDL/DML on every database; granting it `REPLICATION` would be a principle-of-least-privilege violation. A dedicated `replicator` role with only `REPLICATION LOGIN` keeps the base-backup credential's blast radius tiny — even if leaked, it cannot read application data.

### Three new alerts use existing Prometheus metrics, not a custom exporter

`pg_stat_archiver_archived_count`, `pg_stat_archiver_failed_count`, `pg_stat_archiver_last_archived_time` are scraped by the existing `postgres_exporter` sidecar from the built-in `pg_stat_archiver` view. `kube_cronjob_status_last_successful_time` is exposed by the existing `kube-state-metrics` deployment. No new exporter, scrape config, or textfile collector is introduced.

### Three panels appended to the existing PostgreSQL dashboard

The query-observability work already has its own dashboard (`pg-query-performance`). Archive health is operational, not query-performance, so these panels belong on the existing `postgresql` dashboard alongside connection utilization, cache hit ratio, and pg_dump backup freshness. The on-call engineer sees archive health on the same screen as everything else they care about.

### Same physical disk as data — accepted constraint

The `wal-archive` and `basebackup` PVs are hostPath PVs on the same Debian node where the data PVC lives. A whole-disk failure loses both data and backups simultaneously. This is an explicit constraint of the no-paid-services posture and is acknowledged in the data-integrity ADR's trade-offs section. The mitigation (off-host archive on S3) is a separate roadmap item awaiting relaxation of that constraint.

## Consequences

**Positive:**
- RPO drops from ≤ 24 hours to ≤ 5 minutes (one `archive_timeout` window plus pending in-flight WAL).
- An operator can recover to any timestamp within the WAL retention window using the runbook's Scenario 4 — the most powerful Postgres recovery primitive.
- Archive failures and base-backup staleness are alerted, not silent. A broken `archive_command` would otherwise stall WAL recycling and silently fill the data PVC.
- The `replicator` role pattern generalizes: streaming replication (roadmap item #161) reuses the same role.

**Trade-offs:**
- `archive_mode = on` requires a Postgres restart. Acceptable given the existing `Recreate` deployment strategy.
- Disk usage grows by ~10 GB (two new hostPath PVs) on the Minikube node.
- The wrapper script's loud-fail behaviour on existing-archive-file means an operator running disaster-recovery exercises may see legitimate alert noise; bypass requires consciously deleting the offending archive file.
- WAL archive lives on the same physical disk as the data PVC. Disk failure remains unmitigated.

**Phase 3 (future, separate roadmap items):**
- Streaming replication / hot standby (#161) — same WAL stream, different consumer.
- Off-host archive (S3-compatible) once the no-paid-services constraint relaxes.
- Automated weekly PITR drill (#158) — periodic restore-and-verify using the artifacts produced here.

## Operator action required when applying

The live `java-secrets` Secret in the cluster needs a new `replicator-password` key. Generate a new password, base64-encode, and patch the existing Secret:

```bash
NEW_PW=$(openssl rand -hex 24)
ssh debian "kubectl get secret java-secrets -n java-tasks -o json | \
  jq --arg pw \"\$(echo -n $NEW_PW | base64)\" \
  '.data[\"replicator-password\"] = \$pw' | kubectl apply -f -"
```

Then kick the bootstrap Job:

```bash
ssh debian "kubectl delete job postgres-replicator-bootstrap -n java-tasks --ignore-not-found"
cat java/k8s/jobs/postgres-replicator-bootstrap.yml | ssh debian "kubectl apply -f -"
```

The first base backup runs naturally on the next Sunday 03:00 UTC. To validate sooner, run it manually:

```bash
ssh debian "kubectl create job --from=cronjob/postgres-basebackup postgres-basebackup-manual -n java-tasks"
ssh debian "kubectl logs job/postgres-basebackup-manual -n java-tasks -f"
```
