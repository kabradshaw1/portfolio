# Plan: Streaming Read Replica (db-roadmap 7/10)

Spec: `docs/superpowers/specs/2026-04-27-read-replica-design.md`
Issue: #161
Branch: `agent/feat-read-replica`

## Step 1 — Primary postgres config (set replication knobs explicitly)
- `java/k8s/deployments/postgres.yml`: append `-c max_wal_senders=10 -c max_replication_slots=10 -c max_slot_wal_keep_size=4GB -c hot_standby=on` to `args`. (The PG17 defaults already set `max_wal_senders=10` and `max_replication_slots=10`, but we make them explicit so the safety rail and intent are self-documenting.)

## Step 2 — Replica StatefulSet, scripts, service, PVC
- `java/k8s/configmaps/postgres-replica-scripts.yml` — bootstrap-replica.sh (idempotent pg_basebackup using slot `replica_1`).
- `java/k8s/statefulsets/postgres-replica.yml` — single-replica StatefulSet, initContainer running bootstrap, postgres container, postgres-exporter sidecar, volumeClaimTemplate.
- `java/k8s/services/postgres-replica.yml` — ClusterIP service on 5432 selecting replica pods.
- `java/k8s/kustomization.yaml` — register the new resources.
- `java/k8s/network-policy.yml` — allow ingress to replica from go-ecommerce + monitoring (review existing rules).

## Step 3 — Order-service: two-pool wiring
- `go/order-service/cmd/server/config.go` — add `DatabaseURLReplica` (env `DATABASE_URL_REPLICA`); fall back to `DatabaseURL` when unset.
- `go/order-service/internal/db/pools.go` — new `Pools` struct (`Primary`, `Reporting`); `newPool` sets `application_name` runtime param.
- `go/order-service/cmd/server/deps.go` — replace `connectPostgres` callsite with `db.New`; existing single-pool callers continue against `Primary`.
- `go/order-service/cmd/server/main.go` — pass `pools.Reporting` to `reporting.NewRepository`; primary pool unchanged everywhere else; refresher stays on primary.
- `go/k8s/configmaps/order-service-config.yml` — add `DATABASE_URL_REPLICA` pointing at `postgres-replica.java-tasks.svc.cluster.local:5432`.

No changes required to existing reporting tests — `NewRepository` signature unchanged; the test still passes any `*pgxpool.Pool`.

## Step 4 — Observability
- `k8s/monitoring/configmaps/grafana-alerting.yml` — append three rules to the `PostgreSQL` group: `PgReplicationLagHigh`, `PgReplicationSlotLagHigh`, `PgReplicaDown`.
- `k8s/monitoring/configmaps/grafana-dashboards.yml` — append three panels to the `postgresql` dashboard: replica lag, slot retention bytes, replica replay LSN vs primary LSN.
- `docs/runbooks/postgres-recovery.md` — append "Scenario 5: Promote the replica".

## Step 5 — ADR
- `docs/adr/database/read-replica.md` — physical vs logical, slot trade-off + `max_slot_wal_keep_size`, two-pool vs URL-level split, async durability, scope.

## Step 6 — Lint + commit + PR
- `make preflight-go` (order-service builds with the new pool wiring).
- Commit by step (k8s primary tweak, replica resources, app wiring + config, observability, runbook + ADR, plan).
- Push to `qa`-targeted PR.

## Out of scope
- Multi-replica fan-out, automated failover (Patroni/repmgr), replica-aware PgBouncer routing, logical replication, integration test for streaming pair (testcontainers two-container chain is heavy and out-of-scope per spec scoping; existing nil-pool unit tests on reporting repo are unchanged).
