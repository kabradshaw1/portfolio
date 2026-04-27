# Observability Gaps: Deploy Annotations, Kubernetes Events, Connection Tracking

- **Date:** 2026-04-24
- **Status:** Accepted

## Context

During the Postgres WAL corruption incident on 2026-04-24, three observability gaps made diagnosis harder than necessary:

1. **No deploy-to-metric correlation.** When metrics changed, we had to manually piece together timestamps from `kubectl get events` to figure out which deploy triggered the change. Every production observability stack has deploy markers on dashboards — ours didn't.

2. **Kubernetes Warning events invisible.** OOM kills, probe failures, evictions, and back-off restarts only appeared in `kubectl get events`, which is ephemeral (events expire after 1 hour by default) and not queryable from Grafana. During the incident, a probe failure cascade happened before anyone ran `kubectl get events`, and the events had already expired by the time we looked.

3. **No per-service connection attribution.** When Postgres connections spiked, the `pg_stat_activity` view showed connections from the same user (`taskuser`) with no way to tell which service owned them. All six Go services share the same Postgres credentials, so a connection leak in one service is indistinguishable from normal load across all services.

## Decision

### 1. Grafana Deploy Annotations via CI

Added `GF_SECURITY_ADMIN_PASSWORD` from a K8s secret to the Grafana deployment, re-enabling the login form (anonymous Viewer access remains for public dashboard viewing). This allows creating a Grafana service account with Editor role, which generates an API key stored as the `GRAFANA_API_KEY` GitHub Actions secret.

After every rollout restart in CI (both QA and prod), a curl posts an annotation to Grafana's `/api/annotations` endpoint with the namespace and short SHA. Tags distinguish QA deploys (`qa-deploy`) from prod (`deploy`), and per-namespace tags (`ai-services`, `java-tasks`, `go-ecommerce`) allow filtering annotations per dashboard.

The annotation block is guarded by `if [ -n "${GRAFANA_API_KEY:-}" ]` so deploys succeed even before the API key is configured. This is a one-time manual setup step after the first deploy creates the Grafana pod with admin access.

**Why not Grafana provisioning for annotations?** Provisioning handles datasources, dashboards, and alert rules — not runtime annotations. Annotations are time-stamped events created at deploy time, so they must come from CI, not from static config.

### 2. Kubernetes Event Exporter to Loki

Deployed `ghcr.io/resmoio/kubernetes-event-exporter:v1.7` (the actively maintained fork of `opsgenie/kubernetes-event-exporter`, which is archived) as a single-replica Deployment in the `monitoring` namespace.

The exporter watches all Kubernetes events cluster-wide via a ClusterRole with `get`, `watch`, `list` on `events`. A route filter restricts forwarding to Warning events only — Normal events (pod started, image pulled, volume attached) are excluded to avoid storage bloat in Loki.

Each Warning event is pushed to Loki's `/loki/api/v1/push` endpoint as a structured log entry with labels: `namespace`, `reason`, `kind`, `name`. This makes Warning events queryable via `{job="kube-event-exporter"}` in Grafana Explore or the `loki-query` script, with full label-based filtering.

**Why Warning only?** Normal events fire constantly (every pod start, every image pull, every volume mount) and provide no diagnostic value for incident triage. Warning events (OOM kills, probe failures, evictions, scheduling failures) are the ones that matter during incidents and are rare enough to store without concern.

**Why push to Loki instead of Prometheus metrics?** Kubernetes events are unstructured text with context (object name, namespace, message). Prometheus metrics would lose the message content and collapse events into counters. Loki preserves the full event message and makes it searchable alongside application logs.

### 3. Per-Service Postgres Connection Tracking

Appended `&application_name=<service-name>` to the `DATABASE_URL` in all six Go service configmaps (auth-service, order-service, product-service, cart-service, payment-service, order-projector) for both prod and QA environments. The `pgxpool` driver reads this parameter from the connection string and sets it in `pg_stat_activity` automatically — no code changes required.

Added a "Connections by Service" bar gauge panel to the PostgreSQL Grafana dashboard. The panel queries `pg_stat_activity_count{datname!~"template.*|postgres"}` grouped by `application_name`, showing which service owns how many connections at a glance.

**Why `application_name` in the connection string instead of `pgxpool.Config`?** The connection string approach requires zero code changes — it's a configmap-only edit. The `pgxpool` driver and the underlying `pgx` library both honor the `application_name` parameter from the DSN. This also means migration jobs (which use `golang-migrate`'s `pq` driver, not `pgx`) automatically get the label too.

## Consequences

**Positive:**
- Deploy annotations make it trivial to correlate metric changes with specific deploys — no more cross-referencing `kubectl` event timestamps
- Kubernetes Warning events are now persisted in Loki (not ephemeral) and queryable alongside application logs
- Connection leaks can be attributed to a specific service within seconds via the dashboard or `pg_stat_activity`
- All three changes are config-only — no application code was modified, no services need rebuilding

**Trade-offs:**
- The `GRAFANA_API_KEY` GitHub secret requires a one-time manual setup (create service account, generate token, add secret) after the first deploy with admin access enabled
- The kube-event-exporter adds one more pod to the monitoring namespace (~32Mi memory)
- `application_name` appears in `pg_stat_activity` which is visible to any user who can query the system catalog — acceptable since the service names are not sensitive
