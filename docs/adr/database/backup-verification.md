# Automated Backup Verification

- **Date:** 2026-04-27
- **Status:** Accepted
- **Spec:** [`docs/superpowers/specs/2026-04-27-backup-verification-design.md`](../../superpowers/specs/2026-04-27-backup-verification-design.md)
- **Issue:** [#158 — Automated backup verification](https://github.com/kabradshaw1/portfolio/issues/158)
- **Builds on:**
  - [Postgres data integrity ADR](../infrastructure/2026-04-24-postgres-data-integrity.md) (the daily `pg_dump` CronJob this verifies)
  - [Postgres recovery runbook](../../runbooks/postgres-recovery.md)

## Context

The shared PostgreSQL instance has an automated daily `pg_dump` CronJob writing to a hostPath PV. **The dumps are never restored.** Backups silently corrupt; scripts silently break. A senior engineer treats untested backups as no backups.

This ADR records the design of an automated weekly verification job that actually restores each backup, runs a smoke check, and emits per-DB Prometheus metrics so dashboards and alerts can prove "we know our backups work."

## Decision

A new CronJob (`postgres-backup-verify` in the `java-tasks` namespace, Mondays 04:00 UTC) restores every prod DB's latest dump into an ephemeral local Postgres inside its pod, runs a row-count smoke check, and pushes per-DB metrics to a new Pushgateway in the `monitoring` namespace. Two alerts (failure + staleness) fire if anything regresses.

### Why Pushgateway over kube-state-metrics-only

`kube_job_status_succeeded{job_name=~"postgres-backup-verify-.*"}` tells us *the cron ran*. It does not tell us *which database* failed. The verify pod restores 7 databases; if `paymentdb` silently truncates while `authdb` is fine, kube-state-metrics shows "job succeeded" even though one DB is broken (or worse, the script exits 1 and we know nothing about which DB caused it).

Pushgateway lets the script emit `backup_verification_last_success_timestamp{instance="<db>"}` per DB, so the alert message says *exactly* which DB to investigate. Single-source-of-truth for "is each backup verified?" without having to grep through job logs.

### Why ephemeral Postgres in the verify pod

Alternatives we rejected:

1. **Restore into the prod Postgres under a temp DB name** — risks running expensive `pg_restore` against the live cluster, eats prod connection slots, and leaves a footprint if the verify pod is killed mid-run.
2. **Restore into a sidecar Postgres container** — same node, same kernel, same storage; spins up roughly the same complexity as initdb in our pod with no isolation benefit.
3. **Run in a fresh Postgres testcontainer in CI** — would require shipping prod backups to CI infrastructure. Cost, security, and freshness all wrong.

The chosen approach (initdb into emptyDir, postgres on a Unix socket inside the pod, no TCP exposure) is fully self-contained: zero impact on prod, zero outbound traffic except metric pushes, complete teardown at exit.

### Why "rows > 0 in public schema" rather than per-table assertions

Per-table row-count assertions are brittle: any migration that drops a table or moves it out of `public` will fail verification even though the backup is fine. Schema-evolution-aware verification is a separate concern (Phase 3).

A `pg_restore` that completes but produces an empty database catches the most common silent failure mode (`pg_dump` ran but the source connection was dead). It's a low-effort, high-signal smoke check.

### Why Pushgateway persistence is mandatory

Without `--persistence.file`, a Pushgateway pod restart wipes every metric. With our 8-day staleness threshold, a single restart would falsely fire `PgBackupVerificationStale` for every DB. The 1Gi PVC and `--persistence.interval=5m` cap the loss window at 5 minutes of metric updates — irrelevant for a weekly job.

### Why `ReadOnlyMany` PV for the backup volume

The existing `postgres-backup-pv` is `ReadWriteOnce`, bound to the daily-dump CronJob. Sharing it would either require the verify pod to coexist on the same node and share the PVC RWO (fragile, depends on node scheduling), or require gymnastics to detach/reattach. The clean solution is a separate `ReadOnlyMany` PV pointing at the same hostPath, with a label selector + `claimRef` to ensure binding stability. The hostPath is the same physical directory on the Debian host; the kernel handles the read-only constraint.

### Why the verify pod runs as root and drops to postgres via gosu

`initdb` and `pg_ctl` refuse to run as root by design. But we also need to `apk add --no-cache curl` so the script can POST to Pushgateway, and `apk` requires root. The pod therefore starts as root, installs curl, then `gosu postgres` switches to the postgres user (UID 70) for the rest of the script. This pattern matches the upstream `postgres:17-alpine` image's own init flow.

## Phase 2 (planned, gated on roadmap item #157)

Once WAL archiving + base backups land, a second script `pg-verify-pitr.sh` runs alongside Phase 1 in the same Job. It restores the latest base backup, replays WAL to a randomly chosen `recovery_target_time` within the last 6 days, promotes the cluster, and asserts a sentinel row's value at that timestamp.

The sentinel: a meta DB (`backup_verify_meta`) gets a row updated weekly by a separate seed CronJob with a known timestamped value. The verify script knows what value to expect at any given time and asserts it post-restore. This catches "PITR restored, but to the wrong moment" — a class of bugs the row-count smoke check cannot detect.

## Consequences

**Positive:**
- Backups go from "we have files" to "we know they restore."
- Per-DB metrics turn a backup failure into actionable signal — alert says exactly which DB.
- Verification runs in complete isolation; never touches prod connections.
- Pushgateway is a one-time addition that any future batch/cron job can reuse.

**Trade-offs:**
- Pushgateway is now stateful monitoring infrastructure. Mitigation: PVC persistence + a Pushgateway pod-up alert (covered by the existing `Deployment Replicas Unavailable` rule).
- Verification CPU/memory burns weekly on the Minikube node (~2 GiB peak during `pg_restore`). At our scale, fine; at scale we'd move to a dedicated node pool.
- The smoke check is shallow — a `pg_restore` that quietly drops half the rows would still pass. Phase 2 (PITR + sentinel) raises the bar; Phase 3 (schema diff) raises it further.

**Phase 2 / 3 follow-ups:**
- PITR-restore verification (Phase 2 of the linked spec)
- Schema diff between prod and verified DB (would need a snapshot of expected schema)
- Cross-region or off-host backup verification once the no-paid-services constraint relaxes
