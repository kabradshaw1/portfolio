# PostgreSQL Data Integrity & Observability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add automated daily backups, a PodDisruptionBudget, postgres_exporter metrics, a Grafana dashboard with alerts, and a recovery runbook to the shared PostgreSQL instance.

**Architecture:** A CronJob dumps each prod database daily to a hostPath volume on the Debian host. A postgres_exporter sidecar exposes metrics to the existing Prometheus scrape pipeline. Four new Grafana alerts catch connection saturation, cache misses, deadlocks, and stale backups. A recovery runbook documents three restore scenarios.

**Tech Stack:** PostgreSQL 17, postgres_exporter, Kubernetes CronJob, Grafana provisioning API, Prometheus

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `java/k8s/volumes/postgres-backup-pv.yml` | Create | hostPath PV + PVC for backup storage |
| `java/k8s/jobs/postgres-backup.yml` | Create | CronJob — daily pg_dump of 7 prod databases |
| `java/k8s/pdb/postgres-pdb.yml` | Create | PDB preventing voluntary Postgres eviction |
| `java/k8s/deployments/postgres.yml` | Modify | Add postgres_exporter sidecar + Prometheus annotations |
| `java/k8s/kustomization.yaml` | Modify | Register new resources |
| `k8s/monitoring/configmaps/grafana-dashboards.yml` | Modify | Add PostgreSQL dashboard |
| `k8s/monitoring/configmaps/grafana-alerting.yml` | Modify | Add 4 Postgres alert rules |
| `.github/workflows/ci.yml` | Modify | Create backup dir on Debian host before apply |
| `docs/runbooks/postgres-recovery.md` | Create | Recovery runbook (3 scenarios) |

---

### Task 1: Backup Storage — hostPath PV and PVC

**Files:**
- Create: `java/k8s/volumes/postgres-backup-pv.yml`

- [ ] **Step 1: Create the PV and PVC manifest**

```yaml
# java/k8s/volumes/postgres-backup-pv.yml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: postgres-backup-pv
spec:
  capacity:
    storage: 5Gi
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Retain
  storageClassName: manual
  hostPath:
    path: /backups/postgres
    type: DirectoryOrCreate
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: postgres-backup
  namespace: java-tasks
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: manual
  resources:
    requests:
      storage: 5Gi
```

- [ ] **Step 2: Register in kustomization.yaml**

Add to `java/k8s/kustomization.yaml` in the resources list, after `volumes/postgres-pvc.yml`:

```yaml
  - volumes/postgres-backup-pv.yml
```

- [ ] **Step 3: Verify the manifest applies cleanly**

Run: `ssh debian "cat <<'EOF' | kubectl apply --dry-run=server -f -
$(cat java/k8s/volumes/postgres-backup-pv.yml)
EOF"`

Expected: `persistentvolume/postgres-backup-pv created (server dry run)` and `persistentvolumeclaim/postgres-backup created (server dry run)`

- [ ] **Step 4: Commit**

```bash
git add java/k8s/volumes/postgres-backup-pv.yml java/k8s/kustomization.yaml
git commit -m "feat(k8s): add hostPath PV/PVC for postgres backup storage"
```

---

### Task 2: pg_dump Backup CronJob

**Files:**
- Create: `java/k8s/jobs/postgres-backup.yml`
- Modify: `java/k8s/kustomization.yaml`

- [ ] **Step 1: Create the CronJob manifest**

```yaml
# java/k8s/jobs/postgres-backup.yml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: postgres-backup
  namespace: java-tasks
spec:
  schedule: "0 2 * * *"
  concurrencyPolicy: Forbid
  successfulJobsHistoryLimit: 3
  failedJobsHistoryLimit: 3
  jobTemplate:
    spec:
      backoffLimit: 1
      activeDeadlineSeconds: 600
      template:
        spec:
          restartPolicy: Never
          containers:
            - name: pg-dump
              image: postgres:17-alpine
              command: ["/bin/sh", "-c"]
              args:
                - |
                  set -e
                  DATE=$(date +%Y-%m-%d)
                  BACKUP_DIR=/backups

                  for DB in authdb orderdb productdb cartdb paymentdb ecommercedb projectordb; do
                    echo "Dumping $DB..."
                    pg_dump --format=custom \
                      --host=postgres.java-tasks.svc.cluster.local \
                      --username=taskuser \
                      --dbname="$DB" \
                      --file="$BACKUP_DIR/${DB}-${DATE}.dump"
                    echo "  -> $BACKUP_DIR/${DB}-${DATE}.dump ($(du -h "$BACKUP_DIR/${DB}-${DATE}.dump" | cut -f1))"
                  done

                  echo "Cleaning up backups older than 7 days..."
                  find "$BACKUP_DIR" -name '*.dump' -mtime +7 -delete -print

                  echo "Backup complete. Current backups:"
                  ls -lh "$BACKUP_DIR"/*.dump
              env:
                - name: PGPASSWORD
                  valueFrom:
                    secretKeyRef:
                      name: java-secrets
                      key: postgres-password
              volumeMounts:
                - name: backup-storage
                  mountPath: /backups
              resources:
                requests:
                  memory: "64Mi"
                  cpu: "100m"
                limits:
                  memory: "256Mi"
                  cpu: "500m"
          volumes:
            - name: backup-storage
              persistentVolumeClaim:
                claimName: postgres-backup
```

- [ ] **Step 2: Register in kustomization.yaml**

Add to `java/k8s/kustomization.yaml` in the resources list, after the PDB entry (added in Task 3) or after `volumes/postgres-backup-pv.yml`:

```yaml
  - jobs/postgres-backup.yml
```

- [ ] **Step 3: Verify the manifest applies cleanly**

Run: `cat java/k8s/jobs/postgres-backup.yml | ssh debian "kubectl apply --dry-run=server -f -"`

Expected: `cronjob.batch/postgres-backup created (server dry run)`

- [ ] **Step 4: Commit**

```bash
git add java/k8s/jobs/postgres-backup.yml java/k8s/kustomization.yaml
git commit -m "feat(k8s): add daily pg_dump CronJob with 7-day retention"
```

---

### Task 3: PodDisruptionBudget for Postgres

**Files:**
- Create: `java/k8s/pdb/postgres-pdb.yml`
- Modify: `java/k8s/kustomization.yaml`

- [ ] **Step 1: Create the PDB manifest**

```yaml
# java/k8s/pdb/postgres-pdb.yml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: postgres-pdb
  namespace: java-tasks
spec:
  maxUnavailable: 0
  selector:
    matchLabels:
      app: postgres
```

- [ ] **Step 2: Register in kustomization.yaml**

Add to `java/k8s/kustomization.yaml` in the resources list, after `volumes/postgres-backup-pv.yml`:

```yaml
  - pdb/postgres-pdb.yml
```

- [ ] **Step 3: Verify the manifest applies cleanly**

Run: `cat java/k8s/pdb/postgres-pdb.yml | ssh debian "kubectl apply --dry-run=server -f -"`

Expected: `poddisruptionbudget.policy/postgres-pdb created (server dry run)`

- [ ] **Step 4: Commit**

```bash
git add java/k8s/pdb/postgres-pdb.yml java/k8s/kustomization.yaml
git commit -m "feat(k8s): add PodDisruptionBudget for postgres (maxUnavailable: 0)"
```

---

### Task 4: postgres_exporter Sidecar

**Files:**
- Modify: `java/k8s/deployments/postgres.yml`

- [ ] **Step 1: Add Prometheus scrape annotations to the pod template**

In `java/k8s/deployments/postgres.yml`, add annotations to `spec.template.metadata`:

```yaml
    metadata:
      labels:
        app: postgres
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "9187"
        prometheus.io/path: "/metrics"
```

- [ ] **Step 2: Add the postgres_exporter sidecar container**

Add a second container to `spec.template.spec.containers`, after the existing `postgres` container:

```yaml
        - name: postgres-exporter
          image: prometheuscommunity/postgres-exporter:v0.16.0
          ports:
            - containerPort: 9187
          env:
            - name: DATA_SOURCE_USER
              value: taskuser
            - name: DATA_SOURCE_PASS
              valueFrom:
                secretKeyRef:
                  name: java-secrets
                  key: postgres-password
            - name: DATA_SOURCE_URI
              value: "localhost:5432/taskdb?sslmode=disable"
          resources:
            requests:
              memory: "32Mi"
              cpu: "25m"
            limits:
              memory: "64Mi"
              cpu: "100m"
```

- [ ] **Step 3: Verify the full deployment applies cleanly**

Run: `cat java/k8s/deployments/postgres.yml | ssh debian "kubectl apply --dry-run=server -f -"`

Expected: `deployment.apps/postgres configured (server dry run)`

- [ ] **Step 4: Commit**

```bash
git add java/k8s/deployments/postgres.yml
git commit -m "feat(monitoring): add postgres_exporter sidecar to postgres deployment"
```

---

### Task 5: Grafana PostgreSQL Dashboard

**Files:**
- Modify: `k8s/monitoring/configmaps/grafana-dashboards.yml`

- [ ] **Step 1: Add a new dashboard JSON key**

Add a new key `postgresql.json` to the `data:` section of `k8s/monitoring/configmaps/grafana-dashboards.yml`, after the last existing dashboard. The dashboard contains 6 panels:

```json
  postgresql.json: |
    {
      "annotations": { "list": [] },
      "editable": true,
      "fiscalYearStartMonth": 0,
      "graphTooltip": 1,
      "id": null,
      "links": [],
      "panels": [
        {
          "title": "Connection Utilization",
          "type": "gauge",
          "gridPos": { "h": 8, "w": 6, "x": 0, "y": 0 },
          "id": 1,
          "datasource": { "type": "prometheus", "uid": "" },
          "targets": [
            {
              "expr": "sum(pg_stat_activity_count) / pg_settings_max_connections * 100",
              "legendFormat": "utilization %",
              "refId": "A"
            }
          ],
          "fieldConfig": {
            "defaults": {
              "unit": "percent",
              "min": 0,
              "max": 100,
              "thresholds": {
                "steps": [
                  { "color": "green", "value": null },
                  { "color": "yellow", "value": 60 },
                  { "color": "red", "value": 80 }
                ]
              }
            },
            "overrides": []
          }
        },
        {
          "title": "Cache Hit Ratio",
          "type": "stat",
          "gridPos": { "h": 8, "w": 6, "x": 6, "y": 0 },
          "id": 2,
          "datasource": { "type": "prometheus", "uid": "" },
          "targets": [
            {
              "expr": "sum(pg_stat_database_blks_hit) / (sum(pg_stat_database_blks_hit) + sum(pg_stat_database_blks_read)) * 100",
              "legendFormat": "hit ratio %",
              "refId": "A"
            }
          ],
          "fieldConfig": {
            "defaults": {
              "unit": "percent",
              "decimals": 2,
              "thresholds": {
                "steps": [
                  { "color": "red", "value": null },
                  { "color": "yellow", "value": 90 },
                  { "color": "green", "value": 99 }
                ]
              }
            },
            "overrides": []
          }
        },
        {
          "title": "Last Successful Backup",
          "type": "stat",
          "gridPos": { "h": 8, "w": 6, "x": 12, "y": 0 },
          "id": 3,
          "datasource": { "type": "prometheus", "uid": "" },
          "targets": [
            {
              "expr": "time() - max(kube_job_status_completion_time{job_name=~\"postgres-backup.*\", namespace=\"java-tasks\"})",
              "legendFormat": "age",
              "refId": "A"
            }
          ],
          "fieldConfig": {
            "defaults": {
              "unit": "s",
              "thresholds": {
                "steps": [
                  { "color": "green", "value": null },
                  { "color": "yellow", "value": 86400 },
                  { "color": "red", "value": 93600 }
                ]
              }
            },
            "overrides": []
          }
        },
        {
          "title": "Database Sizes",
          "type": "bargauge",
          "gridPos": { "h": 8, "w": 6, "x": 18, "y": 0 },
          "id": 4,
          "datasource": { "type": "prometheus", "uid": "" },
          "targets": [
            {
              "expr": "pg_database_size_bytes{datname!~\"template.*|postgres\"}",
              "legendFormat": "{{datname}}",
              "refId": "A"
            }
          ],
          "fieldConfig": {
            "defaults": {
              "unit": "bytes",
              "thresholds": {
                "steps": [
                  { "color": "green", "value": null },
                  { "color": "yellow", "value": 500000000 },
                  { "color": "red", "value": 1500000000 }
                ]
              }
            },
            "overrides": []
          },
          "options": {
            "orientation": "horizontal",
            "displayMode": "gradient"
          }
        },
        {
          "title": "Transaction Rate",
          "type": "timeseries",
          "gridPos": { "h": 8, "w": 12, "x": 0, "y": 8 },
          "id": 5,
          "datasource": { "type": "prometheus", "uid": "" },
          "targets": [
            {
              "expr": "sum by (datname) (rate(pg_stat_database_xact_commit{datname!~\"template.*|postgres\"}[5m]))",
              "legendFormat": "{{datname}} commits",
              "refId": "A"
            },
            {
              "expr": "sum by (datname) (rate(pg_stat_database_xact_rollback{datname!~\"template.*|postgres\"}[5m]))",
              "legendFormat": "{{datname}} rollbacks",
              "refId": "B"
            }
          ],
          "fieldConfig": {
            "defaults": {
              "unit": "ops",
              "custom": {
                "drawStyle": "line",
                "lineWidth": 1,
                "fillOpacity": 10,
                "showPoints": "never"
              }
            },
            "overrides": []
          },
          "options": {
            "tooltip": { "mode": "multi" },
            "legend": { "displayMode": "list", "placement": "bottom" }
          }
        },
        {
          "title": "Deadlocks",
          "type": "timeseries",
          "gridPos": { "h": 8, "w": 12, "x": 12, "y": 8 },
          "id": 6,
          "datasource": { "type": "prometheus", "uid": "" },
          "targets": [
            {
              "expr": "sum by (datname) (rate(pg_stat_database_deadlocks{datname!~\"template.*|postgres\"}[5m]))",
              "legendFormat": "{{datname}}",
              "refId": "A"
            }
          ],
          "fieldConfig": {
            "defaults": {
              "unit": "ops",
              "custom": {
                "drawStyle": "line",
                "lineWidth": 1,
                "fillOpacity": 10,
                "showPoints": "never"
              }
            },
            "overrides": []
          },
          "options": {
            "tooltip": { "mode": "multi" },
            "legend": { "displayMode": "list", "placement": "bottom" }
          }
        }
      ],
      "schemaVersion": 39,
      "tags": ["postgresql", "database"],
      "templating": { "list": [] },
      "time": { "from": "now-1h", "to": "now" },
      "timepicker": {},
      "timezone": "",
      "title": "PostgreSQL",
      "uid": "postgresql",
      "version": 1
    }
```

- [ ] **Step 2: Commit**

```bash
git add k8s/monitoring/configmaps/grafana-dashboards.yml
git commit -m "feat(monitoring): add PostgreSQL Grafana dashboard"
```

---

### Task 6: Grafana PostgreSQL Alert Rules

**Files:**
- Modify: `k8s/monitoring/configmaps/grafana-alerting.yml`

- [ ] **Step 1: Add a new "PostgreSQL" alert group**

Add the following group to `k8s/monitoring/configmaps/grafana-alerting.yml`, after the existing `Operational` group (after the `circuit-breaker-flapping` rule, before the end of the `groups:` list):

```yaml
      - orgId: 1
        name: PostgreSQL
        folder: Infrastructure Alerts
        interval: 1m
        rules:
          - uid: pg-connection-utilization-high
            title: Postgres Connection Utilization High
            noDataState: OK
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: >-
                    sum(pg_stat_activity_count)
                    / pg_settings_max_connections
                    * 100
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator:
                        type: gt
                        params:
                          - 80
                  refId: C
            for: 5m
            labels:
              severity: warning
            annotations:
              summary: "Postgres connection utilization is above 80%"

          - uid: pg-cache-hit-ratio-low
            title: Postgres Cache Hit Ratio Low
            noDataState: OK
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 600
                  to: 0
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: >-
                    sum(pg_stat_database_blks_hit)
                    / (sum(pg_stat_database_blks_hit) + sum(pg_stat_database_blks_read))
                    * 100
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange:
                  from: 600
                  to: 0
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange:
                  from: 600
                  to: 0
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator:
                        type: lt
                        params:
                          - 95
                  refId: C
            for: 10m
            labels:
              severity: warning
            annotations:
              summary: "Postgres cache hit ratio is below 95% — consider increasing shared_buffers"

          - uid: pg-deadlocks-detected
            title: Postgres Deadlocks Detected
            noDataState: OK
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: sum(increase(pg_stat_database_deadlocks[5m]))
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator:
                        type: gt
                        params:
                          - 0
                  refId: C
            for: 0s
            labels:
              severity: warning
            annotations:
              summary: "Postgres deadlocks detected — check application query patterns"

          - uid: pg-backup-stale
            title: Postgres Backup Stale
            noDataState: OK
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 86400
                  to: 0
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: >-
                    time() - max(kube_job_status_completion_time{
                      job_name=~"postgres-backup.*",
                      namespace="java-tasks"
                    })
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange:
                  from: 86400
                  to: 0
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange:
                  from: 86400
                  to: 0
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator:
                        type: gt
                        params:
                          - 93600
                  refId: C
            for: 0s
            labels:
              severity: critical
            annotations:
              summary: "No successful postgres backup in 26 hours — check CronJob postgres-backup in java-tasks namespace"
```

- [ ] **Step 2: Commit**

```bash
git add k8s/monitoring/configmaps/grafana-alerting.yml
git commit -m "feat(monitoring): add postgres alert rules (connections, cache, deadlocks, backup staleness)"
```

---

### Task 7: CI Pipeline — Create Backup Directory

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Add mkdir before java-tasks manifest apply**

In `.github/workflows/ci.yml`, find the production deploy section where java-tasks manifests are applied. The line is:

```bash
for f in $(find java/k8s -name '*.yml' -not -name 'namespace.yml' -not -path '*/secrets/*'); do echo '---'; cat "$f"; done | $SSH "kubectl apply -f -"
```

Add this line immediately before it:

```bash
$SSH "mkdir -p /backups/postgres"
```

Also find the QA deploy section and add the same line before the QA java-tasks apply. Search for the `qa` section that applies java manifests. Since QA uses the same Postgres, the backup directory only needs to be created once, but adding it to both paths ensures idempotency.

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: ensure /backups/postgres directory exists before deploy"
```

---

### Task 8: Recovery Runbook

**Files:**
- Create: `docs/runbooks/postgres-recovery.md`

- [ ] **Step 1: Create the runbook**

```markdown
# PostgreSQL Recovery Runbook

## Overview

The shared PostgreSQL instance (`postgres` deployment in `java-tasks` namespace) hosts all Go and Java service databases. This runbook covers three recovery scenarios ordered by severity.

**Related alerts:**
- `Postgres Backup Stale` — no successful backup in 26h
- `Postgres Connection Utilization High` — connections > 80%
- `Postgres Cache Hit Ratio Low` — cache ratio < 95%
- `Postgres Deadlocks Detected` — deadlocks in 5m window

**Backup location:** `/backups/postgres/` on the Debian host (outside Minikube PVC)

---

## Scenario 1: Corrupted WAL / Postgres Won't Start

### Symptoms
- Postgres pod in `CrashLoopBackOff`
- Logs show `PANIC: could not locate a valid checkpoint record`
- All Go services returning 503 (CrashLoopBackOff — can't connect to database)
- Java task-service not ready

### Prerequisites
- SSH access to the Debian server (`ssh debian`)
- Check if backups exist: `ls -lh /backups/postgres/` on the Debian host

### If backups exist — use Scenario 2 instead
Restoring from backup preserves data created after the last seed run.

### If no backups or data is all seeded — fresh PVC reset

**Estimated time: 5 minutes**

1. **Scale down Postgres:**
   ```bash
   ssh debian "kubectl scale deployment postgres -n java-tasks --replicas=0"
   ```

2. **Delete the corrupted PVC and any leftover PVs:**
   ```bash
   ssh debian "kubectl delete pvc postgres-data -n java-tasks"
   ssh debian "kubectl get pv | grep java-tasks | grep postgres | awk '{print \$1}' | xargs -r kubectl delete pv"
   ```

3. **Recreate the PVC:**
   ```bash
   cat java/k8s/volumes/postgres-pvc.yml | ssh debian "kubectl apply -f -"
   ```

4. **Scale up Postgres:**
   ```bash
   ssh debian "kubectl scale deployment postgres -n java-tasks --replicas=1"
   ssh debian "kubectl wait --for=condition=ready pod -l app=postgres -n java-tasks --timeout=120s"
   ```

5. **Verify all databases were created by init scripts:**
   ```bash
   ssh debian "kubectl exec deployment/postgres -n java-tasks -- psql -U taskuser -d taskdb -c '\l'"
   ```
   Expected: `authdb`, `orderdb`, `productdb`, `cartdb`, `paymentdb`, `ecommercedb`, `projectordb` (and QA variants).

6. **Create QA databases (not in init scripts):**
   ```bash
   ssh debian "for db in authdb_qa orderdb_qa productdb_qa cartdb_qa paymentdb_qa ecommercedb_qa projectordb_qa; do
     kubectl exec deployment/postgres -n java-tasks -- psql -U taskuser -d taskdb -c \"CREATE DATABASE \$db OWNER taskuser;\"
   done"
   ```

7. **Re-run migration jobs (prod):**
   ```bash
   ssh debian "kubectl delete job go-auth-migrate go-order-migrate go-product-migrate go-cart-migrate go-payment-migrate go-projector-migrate -n go-ecommerce --ignore-not-found --wait=true"
   ```
   Then re-apply from the local manifests:
   ```bash
   for f in go/k8s/jobs/*.yml; do cat "$f" | ssh debian "kubectl apply -f -"; done
   ssh debian "kubectl wait --for=condition=complete job/go-auth-migrate -n go-ecommerce --timeout=120s"
   ssh debian "kubectl wait --for=condition=complete job --all -n go-ecommerce --timeout=120s"
   ```

8. **Re-run migration jobs (QA):**
   ```bash
   ssh debian "kubectl delete job --all -n go-ecommerce-qa --ignore-not-found --wait=true"
   for f in go/k8s/jobs/*.yml; do
     sed 's/namespace: go-ecommerce$/namespace: go-ecommerce-qa/' "$f" | ssh debian "kubectl apply -f -"
   done
   ssh debian "kubectl wait --for=condition=complete job/go-auth-migrate -n go-ecommerce-qa --timeout=120s"
   ssh debian "kubectl wait --for=condition=complete job --all -n go-ecommerce-qa --timeout=120s"
   ```

9. **Restart all dependent services:**
   ```bash
   ssh debian "kubectl rollout restart deployment -n go-ecommerce"
   ssh debian "kubectl rollout restart deployment -n go-ecommerce-qa"
   ssh debian "kubectl rollout restart deployment task-service -n java-tasks"
   ```

10. **Verify services are healthy:**
    ```bash
    ssh debian "curl -s -o /dev/null -w '%{http_code}' http://192.168.49.2/go-auth/health"
    ssh debian "curl -s -o /dev/null -w '%{http_code}' http://192.168.49.2/go-products/products"
    ```
    Expected: `200` for both.

---

## Scenario 2: Restore from pg_dump Backup

### Symptoms
- Data corruption or accidental deletion
- Need to roll back to a known-good state
- Postgres is running but data is wrong

### Prerequisites
- Backups exist at `/backups/postgres/` on the Debian host
- Postgres pod is running and accepting connections

**Estimated time: 10 minutes**

1. **List available backups:**
   ```bash
   ssh debian "ls -lh /backups/postgres/"
   ```

2. **Pick the backup date** (most recent, or a specific date):
   ```bash
   DATE=2026-04-24  # adjust to desired date
   ```

3. **Copy backup files into the Postgres pod** (or use the backup PVC mount):
   ```bash
   for DB in authdb orderdb productdb cartdb paymentdb ecommercedb projectordb; do
     ssh debian "kubectl cp /backups/postgres/${DB}-${DATE}.dump java-tasks/\$(kubectl get pod -l app=postgres -n java-tasks -o jsonpath='{.items[0].metadata.name}'):/tmp/${DB}.dump"
   done
   ```

4. **Restore each database:**
   ```bash
   for DB in authdb orderdb productdb cartdb paymentdb ecommercedb projectordb; do
     echo "Restoring $DB..."
     ssh debian "kubectl exec deployment/postgres -n java-tasks -- pg_restore \
       --clean --if-exists \
       --dbname=$DB \
       --username=taskuser \
       --no-owner \
       /tmp/${DB}.dump"
   done
   ```

5. **Restart dependent services** to clear connection pools:
   ```bash
   ssh debian "kubectl rollout restart deployment -n go-ecommerce"
   ssh debian "kubectl rollout restart deployment task-service -n java-tasks"
   ```

6. **Verify:**
   ```bash
   ssh debian "curl -s -o /dev/null -w '%{http_code}' http://192.168.49.2/go-auth/health"
   ssh debian "curl -s -o /dev/null -w '%{http_code}' http://192.168.49.2/go-products/products"
   ```

---

## Scenario 3: Partial Restore (Single Database)

### Symptoms
- One service is failing but others are healthy
- Only one database needs recovery

### Prerequisites
- Identify which database is affected (check service logs for the failing service)
- Backups exist for that database

**Estimated time: 5 minutes**

1. **Identify the affected database** from service logs:
   | Service | Database |
   |---------|----------|
   | auth-service | authdb |
   | order-service | orderdb |
   | product-service | productdb |
   | cart-service | cartdb |
   | payment-service | paymentdb |
   | ecommerce-service | ecommercedb |
   | order-projector | projectordb |

2. **Restore from backup** (replace `$DB` and `$DATE`):
   ```bash
   ssh debian "kubectl cp /backups/postgres/${DB}-${DATE}.dump java-tasks/\$(kubectl get pod -l app=postgres -n java-tasks -o jsonpath='{.items[0].metadata.name}'):/tmp/${DB}.dump"
   ssh debian "kubectl exec deployment/postgres -n java-tasks -- pg_restore \
     --clean --if-exists \
     --dbname=$DB \
     --username=taskuser \
     --no-owner \
     /tmp/${DB}.dump"
   ```

3. **Or restore from scratch** (if no backup — loses non-seeded data):
   ```bash
   ssh debian "kubectl exec deployment/postgres -n java-tasks -- psql -U taskuser -d taskdb -c 'DROP DATABASE $DB;'"
   ssh debian "kubectl exec deployment/postgres -n java-tasks -- psql -U taskuser -d taskdb -c 'CREATE DATABASE $DB OWNER taskuser;'"
   # Re-run that service's migration job:
   ssh debian "kubectl delete job go-${SERVICE}-migrate -n go-ecommerce --ignore-not-found"
   cat go/k8s/jobs/${SERVICE}-service-migrate.yml | ssh debian "kubectl apply -f -"
   ssh debian "kubectl wait --for=condition=complete job/go-${SERVICE}-migrate -n go-ecommerce --timeout=120s"
   ```

4. **Restart only the affected service:**
   ```bash
   ssh debian "kubectl rollout restart deployment go-${SERVICE}-service -n go-ecommerce"
   ```

5. **Verify** the service is healthy via its health endpoint.
```

- [ ] **Step 2: Commit**

```bash
git add docs/runbooks/postgres-recovery.md
git commit -m "docs: add PostgreSQL recovery runbook (3 scenarios)"
```

---

### Task 9: Apply to Live Cluster and Verify

**Files:** None — operational verification only.

- [ ] **Step 1: Create the backup directory on the Debian host**

```bash
ssh debian "mkdir -p /backups/postgres"
```

- [ ] **Step 2: Apply the PV, PVC, and PDB**

```bash
cat java/k8s/volumes/postgres-backup-pv.yml | ssh debian "kubectl apply -f -"
cat java/k8s/pdb/postgres-pdb.yml | ssh debian "kubectl apply -f -"
```

- [ ] **Step 3: Apply the updated Postgres deployment (with exporter sidecar)**

```bash
cat java/k8s/deployments/postgres.yml | ssh debian "kubectl apply -f -"
ssh debian "kubectl wait --for=condition=ready pod -l app=postgres -n java-tasks --timeout=120s"
```

- [ ] **Step 4: Verify postgres_exporter is running and serving metrics**

```bash
ssh debian "kubectl get pod -l app=postgres -n java-tasks -o jsonpath='{.items[0].status.containerStatuses[*].name}'"
```
Expected: `postgres postgres-exporter`

```bash
ssh debian "kubectl exec deployment/postgres -n java-tasks -c postgres-exporter -- wget -qO- http://localhost:9187/metrics | head -20"
```
Expected: Lines starting with `pg_` metric names.

- [ ] **Step 5: Apply the CronJob**

```bash
cat java/k8s/jobs/postgres-backup.yml | ssh debian "kubectl apply -f -"
ssh debian "kubectl get cronjob -n java-tasks"
```
Expected: `postgres-backup` with schedule `0 2 * * *`.

- [ ] **Step 6: Trigger a manual backup run to verify it works**

```bash
ssh debian "kubectl create job postgres-backup-manual --from=cronjob/postgres-backup -n java-tasks"
ssh debian "kubectl wait --for=condition=complete job/postgres-backup-manual -n java-tasks --timeout=120s"
ssh debian "kubectl logs job/postgres-backup-manual -n java-tasks"
```
Expected: `Dumping authdb...` through all 7 databases, then `Backup complete.`

```bash
ssh debian "ls -lh /backups/postgres/"
```
Expected: 7 `.dump` files with today's date.

- [ ] **Step 7: Apply Grafana dashboard and alert rules**

```bash
cat k8s/monitoring/configmaps/grafana-dashboards.yml | ssh debian "kubectl apply -f -"
cat k8s/monitoring/configmaps/grafana-alerting.yml | ssh debian "kubectl apply -f -"
ssh debian "kubectl rollout restart deployment grafana -n monitoring"
ssh debian "kubectl wait --for=condition=ready pod -l app=grafana -n monitoring --timeout=60s"
```

- [ ] **Step 8: Verify dashboard and alerts loaded**

Check that the PostgreSQL dashboard is accessible:
```bash
ssh debian "curl -s 'http://10.100.246.150:3000/api/dashboards/uid/postgresql' | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d[\"dashboard\"][\"title\"])'"
```
Expected: `PostgreSQL`

Check that alerts are provisioned:
```bash
ssh debian "curl -s 'http://10.100.246.150:3000/api/v1/provisioning/alert-rules' | python3 -c 'import json,sys; rules=json.load(sys.stdin); [print(r[\"uid\"],r[\"title\"]) for r in rules if r[\"uid\"].startswith(\"pg-\")]'"
```
Expected: 4 rules starting with `pg-`.

- [ ] **Step 9: Clean up the manual test job**

```bash
ssh debian "kubectl delete job postgres-backup-manual -n java-tasks"
```

- [ ] **Step 10: Commit any adjustments from verification**

If any manifests needed tweaking during verification, commit the fixes.
