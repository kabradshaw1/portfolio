# Design: PostgreSQL Streaming Read Replica

- **Date:** 2026-04-27
- **Status:** Draft — pending implementation
- **Roadmap position:** Item 7 of 10 in the `db-roadmap` GitHub label
- **GitHub issue:** [#161 — Streaming read replica for reporting](https://github.com/kabradshaw1/portfolio/issues/161)
- **Builds on:**
  - `docs/superpowers/specs/2026-04-27-pitr-design.md` — the `replicator` role and the WAL stream this replica taps into
  - `docs/adr/ecommerce/go-sql-optimization-reporting.md` — the reporting handler whose reads move to the replica
  - `docs/superpowers/specs/2026-04-27-pgbouncer-design.md` — PgBouncer as the primary front; this spec leaves a hook for replica routing

## Context

The order-service exposes a `/reporting/*` endpoint group backed by the materialized views `mv_daily_revenue`, `mv_product_performance`, and `mv_customer_summary`, plus CTE-based reporting queries. These reads compete with OLTP traffic on the same Postgres pod — the same I/O queue, the same CPU, the same `shared_buffers`.

That's fine at portfolio scale, but it's the opposite of how production teams scale Postgres. The standard playbook is:

- **Primary** handles the OLTP workload (writes, latency-sensitive reads).
- **One or more async replicas** receive WAL via streaming replication and serve read-heavy traffic.
- **Application code** routes reads to the replica when staleness is acceptable.

This spec adds one async streaming replica and routes the reporting handler's reads to it. The same WAL stream that already feeds the archive (from the PITR work in roadmap item #157) feeds the replica — no duplicated infrastructure. "Async streaming replica" / "read/write split" / "physical vs logical replication" are foundational interview vocabulary.

## Goals

- Stand up a second Postgres pod as a hot standby of the primary.
- Replicate via physical streaming replication using a replication slot (so WAL isn't pruned before the replica catches up).
- Route the order-service reporting reads to the replica.
- Observe replica lag as a first-class metric, alert on it.
- Document a manual promotion procedure (the runbook scenario for "primary is dead").

## Non-goals

- Synchronous replication (`synchronous_commit = on` w/ `synchronous_standby_names`). Async is the right default for read-scaling; sync replication is a separate concern (durability vs. throughput trade-off).
- Automated failover / Patroni / repmgr. Manual promotion runbook only.
- Connection-string-level read/write splitting (e.g., a `?target_session_attrs=read-only` hack on a single URL). The Go services use *separate pools* for primary vs replica — explicit beats clever.
- Multi-replica fan-out. One replica is sufficient for the current reporting load and the HA story. Two-replica is a one-line StatefulSet change.

## Architecture

```
java-tasks namespace
├── postgres (primary) — existing
│   ├── wal_level = replica  (already set in PITR spec)
│   ├── max_wal_senders = 10
│   ├── max_replication_slots = 5
│   ├── replicator role (created in PITR spec)
│   └── physical replication slot: replica_1
│
├── postgres-replica (NEW)
│   ├── StatefulSet, single replica
│   ├── Image: postgres:17-alpine (same as primary)
│   ├── Bootstrapped via pg_basebackup at first start
│   ├── standby.signal in data dir → starts in recovery
│   ├── primary_conninfo points at postgres.java-tasks.svc.cluster.local:5432
│   ├── primary_slot_name = replica_1
│   ├── hot_standby = on (default)
│   └── Service: postgres-replica.java-tasks.svc.cluster.local:5432 (read-only)
│
└── postgres-replica-bootstrap (NEW one-shot Job)
    └── Runs pg_basebackup against primary into the replica's PVC on first boot

go-ecommerce namespace
└── order-service
    ├── DATABASE_URL          (unchanged) → primary
    ├── DATABASE_URL_REPLICA  (NEW) → replica
    ├── two pgxpool instances: Pool (primary), ReportingPool (replica)
    └── ReportingRepository takes *pgxpool.Pool of replica
        Materialized-view refresh keeps writing to primary (replicas are RO)

monitoring namespace
└── Grafana extends existing PostgreSQL dashboard (3 new panels)
└── Grafana extends PostgreSQL alert group (3 new rules)
```

Replication is **physical** (byte-level WAL replay), not **logical** (per-table change feed). Physical replication means the replica is an exact copy of the primary, including all tables, indexes, materialized views, and roles.

## Replica setup

The replica is a `StatefulSet` (not Deployment) because it has identity — the data dir is keyed to one specific replica, and you can't replace it without re-running `pg_basebackup`.

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: postgres-replica
  namespace: java-tasks
spec:
  serviceName: postgres-replica
  replicas: 1
  selector:
    matchLabels: { app: postgres-replica }
  template:
    metadata:
      labels: { app: postgres-replica }
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "9187"
    spec:
      initContainers:
        - name: bootstrap
          image: postgres:17-alpine
          command: ["/scripts/bootstrap-replica.sh"]
          env:
            - name: PRIMARY_HOST
              value: postgres.java-tasks.svc.cluster.local
            - name: PGPASSWORD
              valueFrom: { secretKeyRef: { name: java-secrets, key: replicator-password } }
          volumeMounts:
            - { name: data, mountPath: /var/lib/postgresql/data }
            - { name: scripts, mountPath: /scripts }
      containers:
        - name: postgres
          image: postgres:17-alpine
          # same args/lifecycle/probes as primary
          # plus: data dir contains standby.signal so it starts in recovery
          volumeMounts:
            - { name: data, mountPath: /var/lib/postgresql/data, subPath: pgdata }
        - name: postgres-exporter
          # same as primary's exporter
      volumes:
        - name: scripts
          configMap: { name: postgres-replica-scripts, defaultMode: 0755 }
  volumeClaimTemplates:
    - metadata: { name: data }
      spec:
        accessModes: ["ReadWriteOnce"]
        resources: { requests: { storage: 10Gi } }
```

### `bootstrap-replica.sh`

```bash
#!/bin/sh
set -eu
DATA_DIR="/var/lib/postgresql/data/pgdata"

if [ -f "$DATA_DIR/PG_VERSION" ]; then
  echo "Replica already initialized — skipping pg_basebackup."
  exit 0
fi

echo "Bootstrapping replica from $PRIMARY_HOST..."
mkdir -p "$DATA_DIR"
pg_basebackup \
  --host="$PRIMARY_HOST" \
  --username=replicator \
  --pgdata="$DATA_DIR" \
  --format=plain \
  --wal-method=stream \
  --slot=replica_1 \
  --create-slot \
  --write-recovery-conf \
  --progress \
  --verbose

# pg_basebackup with --write-recovery-conf creates standby.signal automatically
# and writes primary_conninfo + primary_slot_name into postgresql.auto.conf.

chmod 0700 "$DATA_DIR"
echo "Replica bootstrapped."
```

Notes:
- `--create-slot --slot=replica_1` creates the physical replication slot on the primary at bootstrap time. If the slot exists, `pg_basebackup` errors — use `IF NOT EXISTS`-style pre-check or just let it fail loudly on re-bootstrap (a re-bootstrap is itself an unusual event that warrants attention).
- `--write-recovery-conf` is the modern (PG 12+) way to set up the recovery file — it writes `standby.signal` and `postgresql.auto.conf` entries.

### Why a replication slot (and the trade-off)

The slot guarantees primary keeps WAL until the replica acknowledges it. Without a slot, primary can prune WAL the replica still needs → replica falls off → manual re-bootstrap required.

The cost: if the replica goes offline for an extended period, primary's data dir grows because WAL accumulates. Mitigation: the `PgWalArchiveStale` alert from #157 catches "primary's WAL retention is unhappy"; a new alert (`PgReplicationSlotLagHigh`) catches "the slot is the cause."

`max_slot_wal_keep_size = 4GB` is set on the primary as a safety rail — beyond 4 GB of slot retention, the slot is invalidated rather than filling the disk. This trades correctness (replica needs re-bootstrap) for safety (primary stays up).

## Application-side routing

The order-service is the only consumer right now. Two pools live side by side:

```go
// internal/db/pools.go
type Pools struct {
    Primary   *pgxpool.Pool
    Reporting *pgxpool.Pool
}

func New(ctx context.Context, primaryDSN, replicaDSN string) (*Pools, error) {
    primary, err := newPool(ctx, primaryDSN, "primary")
    if err != nil { return nil, err }
    reporting, err := newPool(ctx, replicaDSN, "reporting")
    if err != nil {
        primary.Close()
        return nil, err
    }
    return &Pools{Primary: primary, Reporting: reporting}, nil
}
```

`newPool` sets `pgxpool.Config.ConnConfig.RuntimeParams["application_name"] = name` so primary vs. replica traffic is distinguishable in `pg_stat_activity`.

The reporting repository constructor takes the replica pool:

```go
// internal/reporting/repository.go
func NewRepository(pool *pgxpool.Pool) *Repository {
    return &Repository{pool: pool}
}
```

Wiring in `cmd/server/main.go`:

```go
pools, err := db.New(ctx, cfg.DatabaseURL, cfg.DatabaseURLReplica)
// ...
reportingRepo := reporting.NewRepository(pools.Reporting)
```

`cfg.DatabaseURLReplica` falls back to `cfg.DatabaseURL` if unset — so local development without a replica still works.

### What writes still go to primary

- All non-reporting endpoints (orders CRUD, saga step transitions, etc.) — `pools.Primary`.
- Materialized view refreshes — `pools.Primary`. Materialized views can't be refreshed on a hot standby (writes only). The existing 15-min refresh interval is unchanged; the *reads* of those views move to the replica.
- Migrations — direct to primary on port 5432 (already the case from the PgBouncer spec's exception).

### Staleness expectations

The reporting endpoints are *already* serving 15-minute-stale data (materialized views refresh every 15 min). Adding sub-second replica lag on top is invisible to the consumer. This is exactly the workload that should run on a replica — staleness was already accepted.

## Observability

Replica lag is the central new metric. Two complementary views:

- **From primary:** `pg_stat_replication.replay_lag` (interval), `.write_lag`, `.flush_lag`. Reports how far each replica is behind.
- **From replica:** `pg_last_xact_replay_timestamp()`, `pg_last_wal_replay_lsn()`. Reports "what time was the last replayed transaction."

Both are exposed by `postgres_exporter` out of the box (the replica gets its own exporter sidecar). Metrics:

- `pg_stat_replication_replay_lag_seconds{slot_name="replica_1"}` (from primary)
- `pg_last_xact_replay_timestamp` (from replica)

### Dashboard panels (extend existing PostgreSQL health dashboard)

| Panel | Type | Source | Purpose |
|---|---|---|---|
| Replica lag (seconds) | Time series | Prometheus, primary view | Single most important replica SLI |
| Replication slot retention bytes | Time series | Prometheus | "Is primary about to invalidate the slot?" |
| Replica replay LSN vs primary LSN | Stat | Prometheus | Visual confidence the stream is live |

### Alerts (appended to `PostgreSQL` group, all `noDataState: OK`)

| Alert | Threshold | Why |
|---|---|---|
| `PgReplicationLagHigh` | `pg_stat_replication_replay_lag_seconds > 30` for 5m | 30s lag is the SLA for reporting reads |
| `PgReplicationSlotLagHigh` | `pg_replication_slots_active = 0` OR `slot retention bytes > 2GB` for 5m | Slot is unhealthy or about to be invalidated |
| `PgReplicaDown` | `up{job="postgres-replica-exporter"} = 0` for 2m | Replica pod is gone — reporting reads will start hitting the fallback (primary) |

## Promotion runbook

A new section in `docs/runbooks/postgres-recovery.md` (peer with the existing scenarios + the PITR scenario from #157):

> **Scenario 5: Promote the replica (primary is unrecoverable)**
>
> *Symptoms:* primary pod won't start, data is corrupt beyond `pg_dump` recovery, or a failover drill.
>
> *Steps:*
> 1. Stop the primary's Service so apps can't reach it: `kubectl scale deployment postgres -n java-tasks --replicas=0`.
> 2. Verify replica replay LSN is current: `kubectl exec postgres-replica-0 -- psql -c "SELECT pg_last_wal_replay_lsn();"`.
> 3. Promote the replica: `kubectl exec postgres-replica-0 -- psql -c "SELECT pg_promote();"`. Replica becomes primary.
> 4. Update the `postgres` Service selector to point at `postgres-replica`. Apps reconnect.
> 5. Bring up a NEW replica pointed at the now-promoted instance. The old primary's data is left as-is for forensics.

The runbook flags this as a manual-only procedure and references the future Patroni/repmgr work for automation.

## Testing

**Integration test** at `go/pkg/db/replica_integration_test.go` (testcontainers, build-tagged):

1. Spin up a primary container with `wal_level=replica`, `max_wal_senders=4`, plus a `replicator` role + a physical replication slot.
2. Spin up a second container that runs `pg_basebackup` from the primary, sets `standby.signal`, and starts in recovery.
3. Wait for replica to report `pg_last_xact_replay_timestamp()`.
4. Insert a row on primary, wait briefly, query the replica.
5. Assert the row is visible on the replica.
6. Kill the replica, write more rows on primary, restart the replica.
7. Assert the replica catches up via the slot (no `pg_basebackup` re-run needed).

**QA smoke test:**
1. After deploy, verify replica pod is in `RUNNING` state and `pg_is_in_recovery()` returns `true`.
2. Hit the order-service `/reporting/sales-trends` endpoint; verify in the primary's `pg_stat_activity` that the query did NOT execute there.
3. In replica's `pg_stat_activity`, verify the connection has `application_name = 'reporting'`.
4. Insert a test row on primary; query for it on the replica within 1s — should be visible.

## Rollout

1. Land the StatefulSet, bootstrap script, replica Service, exporter sidecar config — replica boots, replicates, but no app uses it yet.
2. Add `DATABASE_URL_REPLICA` env var to order-service Deployment, defaulted to the replica Service. The Go code's fallback to `DATABASE_URL` means the change is no-op until the replica DSN is actually set.
3. Wire the replica pool through `db.New` and pass it to `reporting.NewRepository`. Reporting reads now hit the replica.
4. Land dashboard panels and alerts.
5. QA verify; observe overnight for slot/lag behavior.
6. Repeat in prod.

## ADR

Companion ADR at `docs/adr/database/read-replica.md` covering:

- Why physical (streaming) replication over logical for this use case
- Why a replication slot, with the WAL-retention failure mode and the `max_slot_wal_keep_size` mitigation
- Why two pools instead of a connection-string-level read/write split
- The async (not sync) durability trade-off and what reporting consumers can expect
- Why this spec is scoped to the order-service reporting handler, not all reads

## Consequences

**Positive:**
- The OLTP path on primary is no longer competing with reporting I/O.
- Read scaling is now horizontal — additional replicas are a one-line StatefulSet change.
- Replica is also passive HA — manual promotion is a documented one-command failover.
- "How do you scale Postgres reads?" becomes a real demo, not a theoretical answer.

**Trade-offs:**
- Async replication means the replica is *always* slightly stale. Acceptable for the materialized-view-backed reporting workload (already 15-min stale by design).
- The replication slot can fill primary's WAL volume if the replica is down for an extended period. `max_slot_wal_keep_size` limits the damage at the cost of replica re-bootstrap.
- Replica is on the same Minikube node as primary. A node-level failure takes both down. That's a Phase 3 (multi-node) problem.
- One more pod, one more PVC (10Gi), one more exporter to monitor. Modest.

**Phase 2:**
- Replica-aware PgBouncer pools (`productdb_ro` etc. routing to the replica) so apps connect through PgBouncer for both primary and replica reads.
- Second replica for true HA — the StatefulSet `replicas: 2` change.
- Patroni or repmgr for automated failover.
- Logical replication (`wal_level = logical`) — different feature for a different need (CDC, downstream consumers); roadmap item #163.
