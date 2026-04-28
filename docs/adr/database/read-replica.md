# ADR: PostgreSQL Streaming Read Replica for Reporting

- **Date:** 2026-04-28
- **Status:** Accepted
- **Roadmap position:** Item 7 of 10 in the `db-roadmap` GitHub label
- **GitHub issue:** [#161 — Streaming read replica for reporting](https://github.com/kabradshaw1/portfolio/issues/161)
- **Spec:** [`docs/superpowers/specs/2026-04-27-read-replica-design.md`](../../superpowers/specs/2026-04-27-read-replica-design.md)
- **Builds on:** [`wal-archiving-pitr.md`](./wal-archiving-pitr.md) — the `replicator` role and primary's WAL configuration

## Context

The order-service exposes a `/reporting/*` endpoint group backed by three
materialized views (`mv_daily_revenue`, `mv_product_performance`,
`mv_customer_summary`) and a handful of CTE-based queries. Until this work
landed, those reads competed with OLTP traffic on the same Postgres pod —
same I/O queue, same CPU, same `shared_buffers`.

This is fine at portfolio scale, but it's the opposite of how production
teams scale Postgres reads. The standard playbook is *primary serves
writes + latency-sensitive reads, async replicas serve read-heavy traffic.*
Implementing it here is both an operational improvement (reporting reads
no longer compete with OLTP) and a portfolio demonstration of vocabulary
that comes up in every database-adjacent interview ("read/write split,"
"physical vs. logical replication," "replication slots").

## Decision

Stand up a single async streaming read replica as a `StatefulSet` in
`java-tasks`, bootstrapped from the primary via `pg_basebackup` using a
named physical replication slot (`replica_1`). Wire the order-service to
use two `pgxpool` instances — one against the primary, one against the
replica — and route the existing reporting handler's reads to the replica
pool. Reporting writes (materialized-view refresh) stay on the primary.

## Considered alternatives

### Logical replication (`wal_level=logical`, publication/subscription)

**Rejected for this use case.** Logical replication ships per-row CDC
events, not raw WAL bytes; subscribers can be on different schemas, can
filter tables, and can target a different major version. That flexibility
costs (a) replay throughput, (b) handling of DDL (logical replication
*doesn't* replicate DDL — schema changes have to be coordinated by hand),
and (c) materialized views, which logical replication doesn't replicate at
all.

We want a *byte-identical* copy of the primary because the consumers are
reading the same materialized views that already exist on the primary;
physical streaming gives that for free. Logical replication is on the
roadmap (issue #163) for genuinely-different needs — feeding analytics
warehouses, downstream consumers — where the trade-off flips.

### Connection-string-level read/write split

`pgx` and pooled drivers can be configured with multiple hosts and a
`target_session_attrs=read-only` hint. The driver picks a read-only host
when the application opens a transaction it has marked read-only.

**Rejected** because it pushes routing into per-call ergonomics: every
reporting query becomes "remember to set the session attr" or "remember
to pass the right context tag." The two-pool approach makes the
distinction structural — the reporting repository is *constructed* with a
replica pool and physically cannot accidentally write through it. The
trade-off is one extra config knob; the upside is that the routing rule
is enforced by the type system, not by reviewer vigilance.

### Sync replication (`synchronous_commit = on` + `synchronous_standby_names`)

Sync replication trades write latency for zero-data-loss HA. We don't have
the latency budget on the primary's commit path for that — and reporting
reads tolerate sub-second staleness easily because the materialized views
they query are *already* refreshed every 15 minutes. Sync replication is
the right answer for a different problem (durability), not this one
(read scaling).

### Multi-replica fan-out (replicas: 2+)

A single replica is sufficient for current reporting load and doubles as
passive HA (Scenario 5 in the postgres recovery runbook). Going to two
replicas is a one-line StatefulSet change when load justifies it; doing
it now would be premature.

## Why a replication slot — and the trade-off

Streaming replication can run with or without a named slot. Without one,
primary prunes WAL on its normal recycle schedule; if the replica falls
behind by more than that, it's lost (and `pg_basebackup` re-bootstrap is
the only recovery). With a slot, primary keeps WAL until *the slot* says
the replica acknowledged it.

The cost is that an offline replica forces the primary's `pg_wal/`
directory to grow. Without a guard, a long-dead replica would fill the
primary's data volume and take it down. The mitigation is
`max_slot_wal_keep_size = 4GB` on the primary: once the slot's retention
crosses 4GB, Postgres invalidates it rather than keep retaining WAL. The
replica then needs `pg_basebackup` again — but the primary stays up, which
is the right priority.

The `Postgres Replication Slot Lag High` alert fires before invalidation
hits, so a healthy operator response is "go fix the replica" rather than
"go promote a fallback."

## Async replication is fine here

The materialized views the reporting handler reads are already 15-minute
stale by design (the refresh interval). Layering sub-second async-replica
lag on top is invisible to consumers — staleness was already accepted at
a much coarser granularity. The `Postgres Replication Lag High` alert
fires at 30s, which is well inside that envelope.

## Scope: order-service reporting only, not all reads

Decisions about which reads to route through the replica are case-by-case:
some reads are latency-sensitive (auth lookups, cart reads) and should
stay on the primary even though they could technically run on the
replica. The reporting handler is the obvious candidate because it's
already read-only, already non-latency-sensitive, and already serving
stale-by-design data. Future routing decisions for other handlers should
be made when those handlers' workloads warrant it, not as part of this
ADR.

## Promotion = manual

`pg_promote()` is a one-line failover (see Scenario 5 in the postgres
recovery runbook), and at portfolio scale that's appropriate. Patroni or
repmgr would automate this — and would also bring leader election,
fencing, and consensus, which are appropriate when the cost of being
*wrong* about who the primary is exceeds the cost of running a consensus
layer. We're not there yet.

## Consequences

**Positive:**
- The OLTP path on primary is no longer competing with reporting I/O.
- Read scaling is now horizontal — additional replicas are a one-line
  StatefulSet change.
- Replica is also passive HA — manual promotion is a documented one-step
  procedure.
- "How do you scale Postgres reads?" stops being a theoretical
  interview answer and becomes a live demo.

**Trade-offs:**
- One more pod, one more PVC (10Gi), one more exporter to scrape. Modest.
- An offline replica still puts WAL pressure on the primary up to the
  `max_slot_wal_keep_size` ceiling — observable, alerted, but real.
- Replica is on the same Minikube node as primary, so a node-level
  failure takes both down. That's a Phase 3 (multi-node) problem.
- Reporting queries that *would* benefit from replica-side write caching
  (e.g., a future write-back caching layer) won't get it; reporting reads
  are uncached on the replica end.

**Phase 2 follow-ups:**
- Replica-aware PgBouncer pools (`*_ro` routing).
- Second replica for true HA.
- Patroni or repmgr for automated failover.
- Logical replication (`wal_level = logical`) as a separate feature for
  CDC / downstream consumers (roadmap #163).
