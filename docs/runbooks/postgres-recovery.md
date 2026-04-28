# PostgreSQL Recovery Runbook

## Overview

The shared PostgreSQL instance (`postgres` deployment in `java-tasks` namespace) hosts all Go and Java service databases. This runbook covers three recovery scenarios ordered by severity.

**Related alerts:**
- `Postgres Backup Stale` — no successful backup in 26h
- `Postgres Connection Utilization High` — connections > 80%
- `Postgres Cache Hit Ratio Low` — cache ratio < 95%
- `Postgres Deadlocks Detected` — deadlocks in 5m window
- `Postgres Backup Verification Failed` — most recent restore-from-dump failed for at least one DB
- `Postgres Backup Verification Stale` — no successful verification in over 8 days

**Backup location:** `/backups/postgres/` on the Debian host (outside Minikube PVC)

**Verification:** A weekly CronJob (`postgres-backup-verify` in `java-tasks`, Mondays 04:00 UTC) restores every dump into an ephemeral local Postgres and pushes per-DB metrics to Pushgateway. Check the most recent run with:

```bash
ssh debian "kubectl get jobs -n java-tasks --sort-by=.status.completionTime | grep postgres-backup-verify | tail -3"
ssh debian "kubectl logs -n java-tasks job/<latest-verify-job-name>"
```

To trigger an ad-hoc verification (e.g., after restoring backups manually):

```bash
ssh debian "kubectl create job --from=cronjob/postgres-backup-verify postgres-backup-verify-manual-$(date +%s) -n java-tasks"
```

The Grafana **PostgreSQL** dashboard's "Backup Verification — Time Since Last Success" panel shows per-DB freshness; green = within a week, red = over 8 days.

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
