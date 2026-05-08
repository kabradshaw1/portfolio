# ADR: Loki Alert Query Windows and Log Markers (2026-05-08)

## Status
Accepted

## Context
Grafana fired `Postgres auto_explain Plans Not Flowing` even though Postgres
query-plan logging was still functional. The alert annotation showed the real
failure:

`query time range exceeds the limit (query length: 47h59m54s, limit: 1d)`.

The rule intended to count `auto_explain` plan logs over 23 hours:

```logql
sum(count_over_time({namespace="java-tasks", app="postgres"} |= "auto_explain" [23h]))
```

Two assumptions were wrong:

- Postgres `auto_explain` logs do not include the literal string
  `auto_explain`; the emitted line starts with `LOG: duration: ... plan:`.
- Grafana's Loki alert evaluation converted the rule into an effective
  roughly 48-hour query, which exceeded Loki's configured one-day query cap.

Manual verification showed that `auto_explain` itself was working. A harmless
slow query, `SELECT pg_sleep(0.6);`, produced a Postgres plan log and Loki
returned it when queried for `duration:` and `plan:`.

During the same alert triage, `kube-event-exporter` was also logging RBAC
denials while enriching warning events. Its ClusterRole only allowed `events`,
but the exporter attempted to read involved pods, jobs, deployments,
replicasets, and HPAs.

## Decision
Loki-backed Grafana alert rules must query the log markers that are actually
emitted, not the component or feature name we expect conceptually.

For the Postgres query-plan signal, the stalled-plan alert now counts logs that
include both `duration:` and `plan:`:

```logql
sum(count_over_time({namespace="java-tasks", app="postgres"} |= "duration:" |= "plan:" [12h]))
```

The rule is evaluated as an instant Loki query with a short Grafana
`relativeTimeRange`. The LogQL range selector carries the lookback window. The
lookback is 12 hours rather than 23 hours because this deployment accepts the
corrected query at 12 hours and rejects longer windows near 18-23 hours.

The alert summary now names the real signal:

`No Postgres auto_explain plan log lines in 12h`.

`kube-event-exporter` also gets read-only RBAC for the Kubernetes resources it
uses for event metadata enrichment:

- core `pods`
- apps `deployments` and `replicasets`
- batch `jobs`
- autoscaling `horizontalpodautoscalers`

## Consequences
The Postgres plan-flow alert now verifies the actual observability path:
Postgres stderr logs -> Promtail labels -> Loki query -> Grafana alert.

The alert no longer fails due to Loki's query-length cap, and it no longer
fires because it searches for a string Postgres never emits.

The shorter 12-hour window means the alert can fire sooner if the environment
has no qualifying slow query for half a day. That is acceptable because this is
a warning about query-observability freshness, not a page for user impact.

Granting `kube-event-exporter` read access to involved object metadata reduces
monitoring noise and improves warning-event context. The added permissions are
read-only and limited to resource types the exporter already attempted to read.

Future Loki-backed alerts should be tested directly against Loki with the exact
LogQL expression before being provisioned, including the expected alert
evaluation mode and lookback window.
