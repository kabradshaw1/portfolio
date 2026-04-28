# PostgreSQL Recovery Runbook

## Overview

The shared PostgreSQL instance (`postgres` deployment in `java-tasks` namespace) hosts all Go and Java service databases. This runbook covers four recovery scenarios ordered by severity.

**Related alerts:**
- `Postgres Backup Stale` — no successful pg_dump in 26h → Scenario 2
- `Postgres Connection Utilization High` — connections > 80%
- `Postgres Cache Hit Ratio Low` — cache ratio < 95%
- `Postgres Deadlocks Detected` — deadlocks in 5m window
- `Postgres Archive Command Failing` — `archive_command` exiting non-zero → Scenario 4 (preventive — not a recovery trigger by itself)
- `Postgres WAL Archive Stale` — no new WAL archived in 10+ min → Scenario 4 (preventive)
- `Postgres Base Backup Stale` — no successful weekly base backup in 8d → Scenario 4 (preventive)

**Backup locations on the Debian host:**
- `/backups/postgres/` — daily `pg_dump` per database (Scenario 2)
- `/backups/wal-archive/` — continuous WAL archive (Scenario 4)
- `/backups/basebackup/` — weekly `pg_basebackup` tarballs (Scenario 4)

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

---

## Scenario 4: Point-in-Time Recovery (PITR) to a Specific Timestamp

### Symptoms

- A specific destructive event has been identified — bad migration, accidental
  `DELETE`/`UPDATE` without `WHERE`, malicious write — and rolling the cluster
  back to a point *just before* the event will recover the data.
- A pg_dump restore (Scenario 2) would lose too much: the most recent dump is
  hours older than the event's timestamp, and the daily snapshot granularity
  is unacceptable for the affected data.
- Postgres itself is healthy — this scenario is for *logical* recovery, not
  physical-corruption recovery.

### Prerequisites

- The event's timestamp is known (Grafana, application logs, audit trail).
- That timestamp falls within the WAL archive retention window: at minimum,
  later than the `START WAL LOCATION` of the second-most-recent base backup.
  (The basebackup script retains WAL only that far back.)
- A base backup taken *before* the target timestamp exists in
  `/backups/basebackup/` on the Debian host.
- The `replicator-password` key in the live `java-secrets` Secret is set.
  (Check: `kubectl get secret java-secrets -n java-tasks -o jsonpath='{.data.replicator-password}' | base64 -d`
  should print a non-empty value.)

**Estimated time:** 15-30 minutes (dominated by base-backup unpack + WAL replay).

### Steps

1. **Find a usable base backup taken *before* the target timestamp:**
   ```bash
   ssh debian "ls -1 /backups/basebackup/"
   ```
   Pick the most recent directory whose timestamp (`YYYYMMDDTHHMMSSZ` format)
   is earlier than the target timestamp. Set:
   ```bash
   BACKUP_STAMP=20260426T030000Z   # adjust
   TARGET_TIME='2026-04-27 14:23:00 UTC'   # adjust
   ```

2. **Stop Postgres so the data PVC is unmounted:**
   ```bash
   ssh debian "kubectl scale deployment postgres -n java-tasks --replicas=0"
   ssh debian "kubectl wait --for=delete pod -l app=postgres -n java-tasks --timeout=120s"
   ```

3. **Wipe the data PVC (it will be re-seeded from the base backup):**
   ```bash
   ssh debian "kubectl delete pvc postgres-data -n java-tasks"
   ssh debian "kubectl get pv | awk '/postgres-data/ {print \$1}' | xargs -r kubectl delete pv"
   cat java/k8s/volumes/postgres-pvc.yml | ssh debian "kubectl apply -f -"
   ```

4. **Unpack the base backup tarball into the new data PVC.** Run a one-shot
   pod with both PVCs mounted:
   ```bash
   ssh debian "kubectl run pitr-restore -n java-tasks --restart=Never \
     --image=postgres:17-alpine \
     --overrides='{\"spec\":{\"containers\":[{\"name\":\"pitr-restore\",\"image\":\"postgres:17-alpine\",\"command\":[\"sh\",\"-c\",\"sleep 3600\"],\"volumeMounts\":[{\"name\":\"data\",\"mountPath\":\"/var/lib/postgresql/data\"},{\"name\":\"basebackup\",\"mountPath\":\"/backups/basebackup\",\"readOnly\":true}]}],\"volumes\":[{\"name\":\"data\",\"persistentVolumeClaim\":{\"claimName\":\"postgres-data\"}},{\"name\":\"basebackup\",\"persistentVolumeClaim\":{\"claimName\":\"postgres-basebackup\"}}]}}' \
     --command -- sh -c 'sleep 3600'"

   ssh debian "kubectl wait --for=condition=ready pod/pitr-restore -n java-tasks --timeout=60s"
   ssh debian "kubectl exec -n java-tasks pitr-restore -- sh -c '
     mkdir -p /var/lib/postgresql/data/pgdata &&
     cd /var/lib/postgresql/data/pgdata &&
     tar -xzf /backups/basebackup/${BACKUP_STAMP}/base.tar.gz &&
     tar -xzf /backups/basebackup/${BACKUP_STAMP}/pg_wal.tar.gz -C pg_wal/ &&
     chown -R postgres:postgres /var/lib/postgresql/data
   '"
   ```

5. **Configure recovery target.** Append to `postgresql.auto.conf` and drop a
   `recovery.signal` file:
   ```bash
   ssh debian "kubectl exec -n java-tasks pitr-restore -- sh -c \"
     cat >> /var/lib/postgresql/data/pgdata/postgresql.auto.conf <<EOF
   restore_command = 'cp /var/lib/postgresql/wal-archive/%f %p'
   recovery_target_time = '${TARGET_TIME}'
   recovery_target_action = 'promote'
   EOF
     touch /var/lib/postgresql/data/pgdata/recovery.signal
     chown postgres:postgres /var/lib/postgresql/data/pgdata/postgresql.auto.conf
     chown postgres:postgres /var/lib/postgresql/data/pgdata/recovery.signal
   \""

   ssh debian "kubectl delete pod pitr-restore -n java-tasks"
   ```

   Note: `restore_command` reads from `/var/lib/postgresql/wal-archive/`,
   which is mounted into the postgres pod from the `postgres-wal-archive`
   PVC. Postgres replays WAL from there in order until `recovery_target_time`
   is reached, then promotes (becomes a normal read-write database).

6. **Scale Postgres back up:**
   ```bash
   ssh debian "kubectl scale deployment postgres -n java-tasks --replicas=1"
   ssh debian "kubectl wait --for=condition=ready pod -l app=postgres -n java-tasks --timeout=300s"
   ```

   Watch logs to see WAL replay progress:
   ```bash
   ssh debian "kubectl logs -n java-tasks deployment/postgres --tail=200 -f"
   ```
   Look for `redo done at <LSN>` and `recovery target time reached at <ts>` —
   these confirm the recovery target was honored. The line
   `archive recovery complete` indicates promotion.

7. **Verify:** query for the absence of the corruption-causing row, or the
   presence of the row that was deleted:
   ```bash
   ssh debian "kubectl exec deployment/postgres -n java-tasks -- \
     psql -U taskuser -d <affected-db> -c \"<verification query>\""
   ```

8. **Restart all services to drop stale connections:**
   ```bash
   ssh debian "kubectl rollout restart deployment -n go-ecommerce"
   ssh debian "kubectl rollout restart deployment -n go-ecommerce-qa"
   ssh debian "kubectl rollout restart deployment task-service -n java-tasks"
   ```

### After-action

- File a follow-up to take a fresh `pg_dump` ASAP (the daily backup at 02:00 UTC
  may not run before the next incident) — the cluster is now on a new timeline
  but the backup pipeline is not aware of that until the next dump.
- The next weekly `postgres-basebackup` (Sunday 03:00 UTC) will pick up the
  new timeline automatically — no manual intervention required.
- If the target timestamp turned out to be wrong, you can re-run this entire
  procedure from the same base backup tarball; the WAL archive is unchanged
  by the restore.
