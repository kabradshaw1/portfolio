# Observability Gaps: Deploy Annotations, Kubernetes Events, Connection Tracking

**Date:** 2026-04-24
**Status:** Draft
**Scope:** Three observability improvements identified during the 2026-04-24 Postgres incident

## Context

During the Postgres WAL corruption incident on 2026-04-24, three observability gaps made diagnosis harder than necessary:

1. No way to correlate metric changes with deployments — had to manually piece together timestamps from kubectl events
2. Kubernetes Warning events (OOM kills, probe failures, evictions) only visible via `kubectl get events` — not in Loki or Grafana
3. No visibility into which service owns which Postgres connections — if one service leaks connections, can't tell from the dashboard

## 1. Grafana Deploy Annotations

### Setup
- Set `GF_SECURITY_ADMIN_PASSWORD` in `grafana-secrets.yml` (new secret key alongside existing `TELEGRAM_BOT_TOKEN`)
- Keep anonymous Viewer access enabled — dashboard viewing still works without login
- After deploy, create a Grafana service account via the API: `POST /api/serviceaccounts` with `{"name":"ci-deploy","role":"Editor"}`, then generate an API key: `POST /api/serviceaccounts/{id}/tokens`. This is a one-time manual step during initial setup (or after a Grafana PVC reset).
- Store the API key as GitHub Actions secret `GRAFANA_API_KEY`

### CI Integration
After each rollout restart in the CI deploy steps (prod and QA), post an annotation:

```bash
curl -s -X POST "http://192.168.49.2:80/grafana/api/annotations" \
  -H "Authorization: Bearer $GRAFANA_API_KEY" \
  -H "Content-Type: application/json" \
  -d "{\"text\":\"Deploy: <namespace> (sha:$SHA)\",\"tags\":[\"deploy\",\"<namespace>\"]}"
```

Tags per namespace (`ai-services`, `java-tasks`, `go-ecommerce`) allow filtering annotations per dashboard. QA deploys use `qa-deploy` tag to distinguish from prod.

### Files Changed
| File | Change |
|------|--------|
| `k8s/monitoring/deployments/grafana.yml` | Add `GF_SECURITY_ADMIN_PASSWORD` env from secret |
| `k8s/monitoring/secrets/grafana-secrets.yml.template` | Add `grafana-admin-password` key |
| `.github/workflows/ci.yml` | Add annotation curl after each rollout restart (prod + QA sections) |

## 2. Kubernetes Event Exporter

### Tool
`opsgenie/kubernetes-event-exporter` — a single Deployment in the `monitoring` namespace.

### Filtering
Warning events only. Captures: OOM kills, failed probes, evictions, back-off restarts, failed scheduling. Excludes Normal events (pod started, image pulled) to avoid noise and storage bloat.

### Loki Integration
Pushes directly to Loki's `/loki/api/v1/push` endpoint (`http://loki.monitoring.svc.cluster.local:3100`). Each event becomes a log entry with labels: `namespace`, `reason`, `kind`, `name`. Queryable via the existing `loki-query` script and Grafana Explore.

### RBAC
ClusterRole with `get`, `watch`, `list` on `events` (core API group). ServiceAccount and ClusterRoleBinding in `monitoring` namespace. Same pattern as kube-state-metrics.

### Files Changed
| File | Change |
|------|--------|
| `k8s/monitoring/deployments/kube-event-exporter.yml` | New — Deployment |
| `k8s/monitoring/configmaps/kube-event-exporter-config.yml` | New — route config with Warning filter + Loki sink |
| `k8s/monitoring/rbac/kube-event-exporter-clusterrole.yml` | New |
| `k8s/monitoring/rbac/kube-event-exporter-clusterrolebinding.yml` | New |
| `k8s/monitoring/rbac/kube-event-exporter-serviceaccount.yml` | New |
| `k8s/monitoring/kustomization.yaml` | Register new resources |

## 3. Per-Service Postgres Connection Tracking

### Config Change
Add `&application_name=<service-name>` to the `DATABASE_URL` in each Go service's configmap. The `pgxpool` driver reads this parameter from the connection string and sets it in `pg_stat_activity` automatically. No code changes needed.

### Services
| Service | Configmap | application_name |
|---------|-----------|-----------------|
| auth-service | `go/k8s/configmaps/auth-service-config.yml` | `auth-service` |
| order-service | `go/k8s/configmaps/order-service-config.yml` | `order-service` |
| product-service | `go/k8s/configmaps/product-service-config.yml` | `product-service` |
| cart-service | `go/k8s/configmaps/cart-service-config.yml` | `cart-service` |
| payment-service | `go/k8s/configmaps/payment-service-config.yml` | `payment-service` |
| order-projector | `go/k8s/configmaps/order-projector-config.yml` | `order-projector` |

QA Kustomize overlay (`k8s/overlays/qa-go/kustomization.yaml`) patches also need `&application_name=<service>` appended to each DATABASE_URL.

### Dashboard Panel
Add a "Connections by Service" bar gauge panel to the PostgreSQL dashboard (`k8s/monitoring/configmaps/grafana-dashboards.yml`). Query: `pg_stat_activity_count{datname!~"template.*|postgres"}` grouped by `application_name`.

### Files Changed
| File | Change |
|------|--------|
| `go/k8s/configmaps/{auth,order,product,cart,payment}-service-config.yml` | Add `&application_name=<service>` to DATABASE_URL |
| `go/k8s/configmaps/order-projector-config.yml` | Add `&application_name=order-projector` to DATABASE_URL |
| `k8s/overlays/qa-go/kustomization.yaml` | Append `&application_name=<service>` to QA DATABASE_URL patches |
| `k8s/monitoring/configmaps/grafana-dashboards.yml` | Add "Connections by Service" panel to PostgreSQL dashboard |

## All Files Changed (Summary)

| File | Section |
|------|---------|
| `k8s/monitoring/deployments/grafana.yml` | 1 |
| `k8s/monitoring/secrets/grafana-secrets.yml.template` | 1 |
| `.github/workflows/ci.yml` | 1 |
| `k8s/monitoring/deployments/kube-event-exporter.yml` | 2 |
| `k8s/monitoring/configmaps/kube-event-exporter-config.yml` | 2 |
| `k8s/monitoring/rbac/kube-event-exporter-*.yml` (3 files) | 2 |
| `k8s/monitoring/kustomization.yaml` | 2 |
| `go/k8s/configmaps/*-config.yml` (6 files) | 3 |
| `k8s/overlays/qa-go/kustomization.yaml` | 3 |
| `k8s/monitoring/configmaps/grafana-dashboards.yml` | 3 |
