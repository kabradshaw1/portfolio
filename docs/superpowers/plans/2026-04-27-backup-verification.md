# Automated Backup Verification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an automated weekly verification CronJob that restores every `pg_dump` backup into an ephemeral local Postgres, smoke-checks it, and pushes per-database Prometheus metrics so dashboards and alerts can prove "we know our backups work."

**Architecture:** A new Pushgateway in the `monitoring` namespace receives metrics from a weekly CronJob in `java-tasks`. The CronJob spawns its own Postgres on a Unix socket inside an emptyDir, restores each `pg_dump` file from a read-only mount of the existing backup hostPath PV, runs a smoke check, and pushes per-DB metrics. Two new alerts and three dashboard panels make staleness and failures actionable.

**Tech Stack:** Kubernetes (CronJob, ConfigMap, hostPath PV/PVC), Prometheus + Pushgateway (`prom/pushgateway:v1.9.0`), Grafana (provisioned alerts and dashboards), `postgres:17-alpine` for the verify pod, Go + testcontainers for the integration test.

**Phase split:**
- **Phase 1 (Tasks 1–13):** ships now — `pg_dump` verification end-to-end.
- **Phase 2 (Tasks 14–17):** ships after roadmap item #157 (WAL archiving + PITR) merges. Each Phase 2 task is clearly marked **Phase 2 only — implement after #157**.

**Important deviations from spec:**

1. **Prometheus scrape mechanism.** The spec assumes Prometheus scrapes annotated `Service`s via a `kubernetes-services` job. The actual `prometheus-config.yml` only has a `k8s-pods` scrape job that uses **pod**-level annotations. We therefore put `prometheus.io/scrape: "true"` and `prometheus.io/port: "9091"` on the **pod template** of the Pushgateway Deployment, not on the Service. Functionally equivalent; matches existing pattern.
2. **Read-only backup access.** The existing `postgres-backup-pv` is `ReadWriteOnce` and bound to the daily-dump CronJob's PVC. Rather than reuse that PVC (which would require both pods to coexist on the same node and mount it RWO), we create a parallel `postgres-backup-readonly-pv` pointing at the same hostPath, with `ReadOnlyMany` access mode, and a separate PVC for the verify pod.

---

## File Structure

### Phase 1 files

| File | Status | Responsibility |
|---|---|---|
| `k8s/monitoring/pvc/pushgateway-data.yml` | **NEW** | 1Gi PVC backing Pushgateway's `--persistence.file` |
| `k8s/monitoring/deployments/pushgateway.yml` | **NEW** | Single-replica Deployment with pod-level scrape annotations |
| `k8s/monitoring/services/pushgateway.yml` | **NEW** | ClusterIP Service exposing `:9091` |
| `k8s/monitoring/kustomization.yaml` | **MOD** | Wire the 3 new resources into the monitoring kustomization |
| `k8s/monitoring/configmaps/grafana-dashboards.yml` | **MOD** | Append 3 panels to the `postgresql` dashboard |
| `k8s/monitoring/configmaps/grafana-alerting.yml` | **MOD** | Append 2 rules to the `PostgreSQL` alert group |
| `java/k8s/configmaps/postgres-verify-scripts.yml` | **NEW** | ConfigMap holding `pg-verify-backups.sh` |
| `java/k8s/volumes/postgres-backup-readonly-pv.yml` | **NEW** | RO PV/PVC pointing at the existing `/backups/postgres` hostPath |
| `java/k8s/jobs/postgres-backup-verify.yml` | **NEW** | Weekly CronJob (Mondays 04:00 UTC) running the script |
| `java/k8s/kustomization.yaml` | **MOD** | Wire the 3 new java-tasks resources |
| `go/pkg/db/backup_verification_integration_test.go` | **NEW** | testcontainers integration test for the script |
| `docs/adr/database/backup-verification.md` | **NEW** | ADR documenting the design decisions |
| `docs/runbooks/postgres-recovery.md` | **MOD** | Add a "Verifying backups manually" section + new alerts list |

### Phase 2 files (implement after #157)

| File | Status | Responsibility |
|---|---|---|
| `java/k8s/configmaps/postgres-verify-scripts.yml` | **MOD** | Add `pg-verify-pitr.sh` next to the Phase 1 script |
| `java/k8s/jobs/postgres-verify-sentinel.yml` | **NEW** | Weekly CronJob writing the sentinel row to a meta DB |
| `java/k8s/jobs/postgres-backup-verify.yml` | **MOD** | Mount WAL + basebackup volumes; run both scripts |
| `docs/adr/database/backup-verification.md` | **MOD** | Append Phase 2 section explaining sentinel design |
| `go/pkg/db/backup_verification_integration_test.go` | **MOD** | Add `TestPITRVerification` |

---

## Pre-flight: Setup

- [ ] **Step 0.1: Confirm worktree and branch**

Run: `git branch --show-current`
Expected: `agent/feat-backup-verification`
Run: `pwd`
Expected: ends with `.claude/worktrees/agent+feat-backup-verification`

- [ ] **Step 0.2: Confirm baseline preflight is green**

Run: `make preflight-go`
Expected: passes (we touch `go/pkg/db/` so this must work before we change it)

If this fails before any of our changes land, stop and investigate — the integration test will be impossible to validate against a broken baseline.

---

## Phase 1: pg_dump verification (ships now)

### Task 1: Pushgateway PVC

**Files:**
- Create: `k8s/monitoring/pvc/pushgateway-data.yml`

- [ ] **Step 1.1: Create the PVC manifest**

Create `k8s/monitoring/pvc/pushgateway-data.yml` with:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: pushgateway-data
  namespace: monitoring
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
```

We rely on Minikube's default dynamic provisioner — no hostPath PV needed. 1Gi is overkill for a single metrics snapshot file but cheap.

- [ ] **Step 1.2: Validate YAML**

Run: `kubectl apply --dry-run=client -f k8s/monitoring/pvc/pushgateway-data.yml`
Expected: `persistentvolumeclaim/pushgateway-data created (dry run)`

- [ ] **Step 1.3: Commit**

```bash
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification add k8s/monitoring/pvc/pushgateway-data.yml
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification commit -m "feat(monitoring): add Pushgateway data PVC"
```

---

### Task 2: Pushgateway Deployment

**Files:**
- Create: `k8s/monitoring/deployments/pushgateway.yml`

- [ ] **Step 2.1: Create the Deployment manifest**

Create `k8s/monitoring/deployments/pushgateway.yml` with:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: pushgateway
  namespace: monitoring
  labels:
    app: pushgateway
spec:
  replicas: 1
  strategy:
    type: Recreate # PVC is RWO; rolling would deadlock
  selector:
    matchLabels:
      app: pushgateway
  template:
    metadata:
      labels:
        app: pushgateway
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "9091"
        prometheus.io/path: "/metrics"
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 65534
        fsGroup: 65534
      containers:
        - name: pushgateway
          image: prom/pushgateway:v1.9.0
          args:
            - --persistence.file=/data/metrics
            - --persistence.interval=5m
            - --web.listen-address=:9091
          ports:
            - name: http
              containerPort: 9091
          readinessProbe:
            httpGet:
              path: /-/ready
              port: 9091
            initialDelaySeconds: 5
            periodSeconds: 10
          livenessProbe:
            httpGet:
              path: /-/healthy
              port: 9091
            initialDelaySeconds: 10
            periodSeconds: 30
          resources:
            requests:
              cpu: "20m"
              memory: "32Mi"
            limits:
              cpu: "200m"
              memory: "128Mi"
          securityContext:
            readOnlyRootFilesystem: true
            allowPrivilegeEscalation: false
            capabilities:
              drop: [ALL]
          volumeMounts:
            - name: data
              mountPath: /data
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: pushgateway-data
```

The pod-level annotations (`prometheus.io/scrape`, `prometheus.io/port`) are how the existing `k8s-pods` Prometheus scrape job discovers it — see `k8s/monitoring/configmaps/prometheus-config.yml`.

- [ ] **Step 2.2: Validate YAML**

Run: `kubectl apply --dry-run=client -f k8s/monitoring/deployments/pushgateway.yml`
Expected: `deployment.apps/pushgateway created (dry run)`

- [ ] **Step 2.3: Commit**

```bash
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification add k8s/monitoring/deployments/pushgateway.yml
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification commit -m "feat(monitoring): add Pushgateway deployment with persistence"
```

---

### Task 3: Pushgateway Service

**Files:**
- Create: `k8s/monitoring/services/pushgateway.yml`

- [ ] **Step 3.1: Create the Service manifest**

Create `k8s/monitoring/services/pushgateway.yml` with:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: pushgateway
  namespace: monitoring
  labels:
    app: pushgateway
spec:
  type: ClusterIP
  selector:
    app: pushgateway
  ports:
    - name: http
      port: 9091
      targetPort: 9091
      protocol: TCP
```

The verify CronJob will reach this at `http://pushgateway.monitoring.svc.cluster.local:9091`.

- [ ] **Step 3.2: Validate YAML**

Run: `kubectl apply --dry-run=client -f k8s/monitoring/services/pushgateway.yml`
Expected: `service/pushgateway created (dry run)`

- [ ] **Step 3.3: Commit**

```bash
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification add k8s/monitoring/services/pushgateway.yml
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification commit -m "feat(monitoring): add Pushgateway service"
```

---

### Task 4: Wire Pushgateway into the monitoring kustomization

**Files:**
- Modify: `k8s/monitoring/kustomization.yaml`

- [ ] **Step 4.1: Add the three new resources**

Edit `k8s/monitoring/kustomization.yaml`. Find the line `  - pvc/loki-data.yml` and add immediately after it:

```yaml
  - pvc/pushgateway-data.yml
```

Find the line `  - deployments/kube-event-exporter.yml` and add immediately after it:

```yaml
  - deployments/pushgateway.yml
```

Find the line `  - services/prometheus.yml` and add immediately after it:

```yaml
  - services/pushgateway.yml
```

- [ ] **Step 4.2: Validate the kustomization renders**

Run: `kubectl kustomize k8s/monitoring | head -100`
Expected: output includes `kind: Deployment` with `name: pushgateway`, `kind: Service` with `name: pushgateway`, and `kind: PersistentVolumeClaim` with `name: pushgateway-data`. No errors.

Run: `kubectl kustomize k8s/monitoring | grep -E "name: pushgateway|name: pushgateway-data" | sort -u`
Expected: 3 lines.

- [ ] **Step 4.3: Commit**

```bash
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification add k8s/monitoring/kustomization.yaml
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification commit -m "feat(monitoring): wire Pushgateway resources into kustomization"
```

---

### Task 5: Verification script ConfigMap

**Files:**
- Create: `java/k8s/configmaps/postgres-verify-scripts.yml`

- [ ] **Step 5.1: Create the ConfigMap with the script**

Create `java/k8s/configmaps/postgres-verify-scripts.yml` with:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: postgres-verify-scripts
  namespace: java-tasks
data:
  pg-verify-backups.sh: |
    #!/bin/sh
    # Restore each pg_dump file to an ephemeral local Postgres,
    # smoke-check it, and push per-DB metrics to Pushgateway.
    set -eu

    DBS="${VERIFY_DBS:-authdb productdb orderdb cartdb paymentdb ecommercedb projectordb}"
    PUSHGATEWAY="${PUSHGATEWAY_URL:-http://pushgateway.monitoring.svc.cluster.local:9091}"
    DUMPS_DIR="${DUMPS_DIR:-/backups/postgres}"
    DATA_DIR="${PG_DATA_DIR:-/var/lib/postgresql/data}"
    SOCKET_DIR="${PG_SOCKET_DIR:-/tmp/pg-verify}"

    mkdir -p "$SOCKET_DIR"
    chmod 700 "$SOCKET_DIR"

    push_success() {
      db="$1"; rows="$2"; dump="$3"
      now="$(date +%s)"
      dump_age_sec="$(( now - $(stat -c %Y "$dump") ))"
      cat <<EOF | curl -fsS --data-binary @- "$PUSHGATEWAY/metrics/job/postgres_backup_verify/instance/$db"
    # TYPE backup_verification_last_success_timestamp gauge
    backup_verification_last_success_timestamp $now
    # TYPE backup_verification_restored_rows gauge
    backup_verification_restored_rows $rows
    # TYPE backup_verification_dump_age_seconds gauge
    backup_verification_dump_age_seconds $dump_age_sec
    EOF
    }

    push_failure() {
      db="$1"; reason="$2"
      now="$(date +%s)"
      cat <<EOF | curl -fsS --data-binary @- "$PUSHGATEWAY/metrics/job/postgres_backup_verify/instance/$db"
    # TYPE backup_verification_last_failure_timestamp gauge
    backup_verification_last_failure_timestamp $now
    # TYPE backup_verification_last_failure_reason gauge
    backup_verification_last_failure_reason{reason="$reason"} 1
    EOF
    }

    push_overall() {
      ok="$1"
      cat <<EOF | curl -fsS --data-binary @- "$PUSHGATEWAY/metrics/job/postgres_backup_verify"
    # TYPE backup_verification_run_success gauge
    backup_verification_run_success $ok
    # TYPE backup_verification_run_timestamp gauge
    backup_verification_run_timestamp $(date +%s)
    EOF
    }

    if [ ! -f "$DATA_DIR/PG_VERSION" ]; then
      echo "Initializing local Postgres data dir at $DATA_DIR"
      initdb -D "$DATA_DIR" -U verify --auth=trust --no-locale --encoding=UTF8
    fi

    echo "Starting local Postgres on socket $SOCKET_DIR"
    pg_ctl -D "$DATA_DIR" -l /tmp/pg-verify.log -o "-k $SOCKET_DIR -h ''" -w start

    trap 'pg_ctl -D "$DATA_DIR" -m fast stop || true' EXIT

    OVERALL_OK=1

    for DB in $DBS; do
      echo "Verifying $DB..."
      DUMP="$(ls -t "$DUMPS_DIR/$DB"-*.dump 2>/dev/null | head -1 || true)"
      if [ -z "$DUMP" ]; then
        echo "  FAIL: no dump file found for $DB in $DUMPS_DIR"
        push_failure "$DB" "no_dump_file"
        OVERALL_OK=0
        continue
      fi

      TARGET="${DB}_verify"
      psql -h "$SOCKET_DIR" -U verify -d postgres -c "DROP DATABASE IF EXISTS $TARGET;" >/dev/null
      psql -h "$SOCKET_DIR" -U verify -d postgres -c "CREATE DATABASE $TARGET;" >/dev/null

      if ! pg_restore -h "$SOCKET_DIR" -U verify -d "$TARGET" --no-owner --no-acl "$DUMP" 2>&1 | tail -20; then
        echo "  FAIL: pg_restore exited non-zero for $DB"
        push_failure "$DB" "pg_restore_failed"
        OVERALL_OK=0
        psql -h "$SOCKET_DIR" -U verify -d postgres -c "DROP DATABASE $TARGET;" >/dev/null || true
        continue
      fi

      ROWS="$(psql -h "$SOCKET_DIR" -U verify -d "$TARGET" -t -A -c "
        SELECT COALESCE(SUM(reltuples)::bigint, 0)
        FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE c.relkind = 'r' AND n.nspname = 'public';
      ")"
      ROWS="${ROWS:-0}"

      if [ "$ROWS" -lt 1 ]; then
        echo "  FAIL: restored $DB has zero rows in public schema"
        push_failure "$DB" "empty_after_restore"
        OVERALL_OK=0
      else
        echo "  OK: $DB has $ROWS rows"
        push_success "$DB" "$ROWS" "$DUMP"
      fi

      psql -h "$SOCKET_DIR" -U verify -d postgres -c "DROP DATABASE $TARGET;" >/dev/null
    done

    push_overall "$OVERALL_OK"

    if [ "$OVERALL_OK" -eq 1 ]; then
      echo "All databases verified."
      exit 0
    else
      echo "One or more databases failed verification."
      exit 1
    fi
```

Notes:
- `pg_restore` errors are non-fatal at the shell level (`set -e` is bypassed for `if !` constructs) so we report failures per DB and still verify the rest.
- `--auth=trust` on `initdb` is safe because the cluster is reachable only via Unix socket inside the pod.
- `set -eu` does NOT use `pipefail` because alpine `sh` doesn't support it; the `if !` patterns handle the per-DB exit codes.
- The DB list defaults to the same 7 prod DBs as the daily backup CronJob (`java/k8s/jobs/postgres-backup.yml`) but is overridable via `VERIFY_DBS` for the integration test.

- [ ] **Step 5.2: Validate YAML**

Run: `kubectl apply --dry-run=client -f java/k8s/configmaps/postgres-verify-scripts.yml`
Expected: `configmap/postgres-verify-scripts created (dry run)`

- [ ] **Step 5.3: Commit**

```bash
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification add java/k8s/configmaps/postgres-verify-scripts.yml
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification commit -m "feat(java-tasks): add backup verification script ConfigMap"
```

---

### Task 6: Read-only PV/PVC for the backup volume

**Files:**
- Create: `java/k8s/volumes/postgres-backup-readonly-pv.yml`

- [ ] **Step 6.1: Create the read-only PV/PVC manifest**

Create `java/k8s/volumes/postgres-backup-readonly-pv.yml` with:

```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: postgres-backup-readonly-pv
  labels:
    type: backup-readonly
spec:
  capacity:
    storage: 5Gi
  accessModes:
    - ReadOnlyMany
  persistentVolumeReclaimPolicy: Retain
  storageClassName: manual-readonly
  hostPath:
    path: /backups/postgres
    type: Directory
  claimRef:
    namespace: java-tasks
    name: postgres-backup-readonly
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: postgres-backup-readonly
  namespace: java-tasks
spec:
  accessModes:
    - ReadOnlyMany
  storageClassName: manual-readonly
  resources:
    requests:
      storage: 5Gi
  selector:
    matchLabels:
      type: backup-readonly
```

The `claimRef` + label selector pin the PVC to this exact PV (no risk of binding to the existing RWO `postgres-backup-pv`). `storageClassName: manual-readonly` is a sentinel name that does not match the existing `manual` class — prevents accidental cross-binding.

`hostPath.type: Directory` (not `DirectoryOrCreate`) — the directory is created by the daily backup PV. If it doesn't exist, fail loudly rather than creating a fresh empty one.

- [ ] **Step 6.2: Validate YAML**

Run: `kubectl apply --dry-run=client -f java/k8s/volumes/postgres-backup-readonly-pv.yml`
Expected: `persistentvolume/postgres-backup-readonly-pv created (dry run)` and `persistentvolumeclaim/postgres-backup-readonly created (dry run)`

- [ ] **Step 6.3: Commit**

```bash
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification add java/k8s/volumes/postgres-backup-readonly-pv.yml
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification commit -m "feat(java-tasks): add read-only backup PV/PVC for verify pod"
```

---

### Task 7: Verify CronJob (Phase 1)

**Files:**
- Create: `java/k8s/jobs/postgres-backup-verify.yml`

- [ ] **Step 7.1: Create the CronJob manifest**

Create `java/k8s/jobs/postgres-backup-verify.yml` with:

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: postgres-backup-verify
  namespace: java-tasks
spec:
  schedule: "0 4 * * 1" # Mondays 04:00 UTC — after Sunday's basebackup (#157, future) and Monday's pg_dump (02:00)
  concurrencyPolicy: Forbid
  successfulJobsHistoryLimit: 3
  failedJobsHistoryLimit: 3
  jobTemplate:
    spec:
      backoffLimit: 1
      activeDeadlineSeconds: 1800 # 30 min hard cap
      template:
        spec:
          restartPolicy: Never
          securityContext:
            runAsUser: 70 # postgres user in postgres:17-alpine
            runAsGroup: 70
            fsGroup: 70
          containers:
            - name: pg-verify
              image: postgres:17-alpine
              command: ["/bin/sh", "-c"]
              args:
                - |
                  set -eu
                  apk add --no-cache curl >/dev/null
                  exec /scripts/pg-verify-backups.sh
              env:
                - name: PUSHGATEWAY_URL
                  value: "http://pushgateway.monitoring.svc.cluster.local:9091"
                - name: VERIFY_DBS
                  value: "authdb productdb orderdb cartdb paymentdb ecommercedb projectordb"
              volumeMounts:
                - name: scripts
                  mountPath: /scripts
                  readOnly: true
                - name: backups
                  mountPath: /backups/postgres
                  readOnly: true
                - name: pgdata
                  mountPath: /var/lib/postgresql/data
                - name: tmp
                  mountPath: /tmp
              resources:
                requests:
                  cpu: "200m"
                  memory: "512Mi"
                  ephemeral-storage: "5Gi"
                limits:
                  cpu: "1"
                  memory: "2Gi"
                  ephemeral-storage: "5Gi"
          volumes:
            - name: scripts
              configMap:
                name: postgres-verify-scripts
                defaultMode: 0555
            - name: backups
              persistentVolumeClaim:
                claimName: postgres-backup-readonly
                readOnly: true
            - name: pgdata
              emptyDir:
                sizeLimit: 5Gi
            - name: tmp
              emptyDir: {}
```

Notes:
- `apk add --no-cache curl` — alpine doesn't ship curl by default, and the `push_*` helpers use it.
- `runAsUser: 70` — the `postgres` user inside the alpine image. `initdb` and `pg_ctl` refuse to run as root.
- `pgdata` is an `emptyDir` with `sizeLimit: 5Gi` — the verified data lives only inside the pod; nothing leaks to the node.
- Schedule is **Monday** 04:00 UTC. The daily backup runs daily at 02:00 UTC, so the latest dump is two hours old when verify runs.
- `activeDeadlineSeconds: 1800` matches the spec's "2–5 min" expectation with a generous safety margin.

- [ ] **Step 7.2: Validate YAML**

Run: `kubectl apply --dry-run=client -f java/k8s/jobs/postgres-backup-verify.yml`
Expected: `cronjob.batch/postgres-backup-verify created (dry run)`

- [ ] **Step 7.3: Commit**

```bash
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification add java/k8s/jobs/postgres-backup-verify.yml
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification commit -m "feat(java-tasks): add postgres-backup-verify CronJob (phase 1)"
```

---

### Task 8: Wire java-tasks resources into kustomization

**Files:**
- Modify: `java/k8s/kustomization.yaml`

- [ ] **Step 8.1: Add the three new java-tasks resources**

Edit `java/k8s/kustomization.yaml`. Find `  - configmaps/task-service-config.yml` and add immediately after it:

```yaml
  - configmaps/postgres-verify-scripts.yml
```

Find `  - volumes/postgres-backup-pv.yml` and add immediately after it:

```yaml
  - volumes/postgres-backup-readonly-pv.yml
```

Find `  - jobs/postgres-grafana-reader.yml` and add immediately after it:

```yaml
  - jobs/postgres-backup-verify.yml
```

- [ ] **Step 8.2: Validate the kustomization renders**

Run: `kubectl kustomize java/k8s | grep -E "name: postgres-verify-scripts|name: postgres-backup-readonly|name: postgres-backup-verify" | sort -u`
Expected: at least 4 lines (script ConfigMap, PV, PVC, CronJob).

Run: `kubectl kustomize java/k8s > /tmp/java-rendered.yaml && wc -l /tmp/java-rendered.yaml`
Expected: completes without error, line count is larger than before.

- [ ] **Step 8.3: Commit**

```bash
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification add java/k8s/kustomization.yaml
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification commit -m "feat(java-tasks): wire backup verification resources into kustomization"
```

---

### Task 9: Integration test for the verification script

The verification script is the single most failure-prone component (shell scripts silently misbehave). We test it end-to-end in a build-tagged integration test using testcontainers.

**Test design:**
1. Start a "source" Postgres testcontainer, seed a small schema with rows.
2. Use `Exec` inside the source container to run `pg_dump` and write a `.dump` file into a host bind directory.
3. Start an `httptest.Server` on the test process to mock Pushgateway. Capture every POST body keyed by URL path.
4. Start a "verify" testcontainer (`postgres:17-alpine`) that mounts both the host dump dir and the script (read from the project file). Set `PUSHGATEWAY_URL` to a host-reachable URL (testcontainers' `host.docker.internal` mapping).
5. The verify container runs `apk add curl && /scripts/pg-verify-backups.sh` and exits.
6. Assert: exit code 0, mock recorded a success metric body with `backup_verification_last_success_timestamp` for `instance=appdb`, restored row count >= seeded row count, dump file age >= 0.
7. Failure-mode subtest: truncate the dump file before running verify, assert exit code 1 and a failure metric.

**Files:**
- Create: `go/pkg/db/backup_verification_integration_test.go`
- Reference: `go/pkg/db/extensions_integration_test.go` (existing testcontainers pattern in the same package)
- Reference: `java/k8s/configmaps/postgres-verify-scripts.yml` (script source)

- [ ] **Step 9.1: Add the bind-mount helper file**

The test will need to read the verify script out of the YAML ConfigMap. We add a small helper that extracts the `pg-verify-backups.sh` body from the YAML.

Create `go/pkg/db/backup_verification_integration_test.go` with the test skeleton (this step writes the file with all imports + the helper, then we fill in tests in subsequent steps):

```go
//go:build integration

package db_test

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gopkg.in/yaml.v3"
)

// loadVerifyScript reads pg-verify-backups.sh out of the ConfigMap YAML and
// writes it to a temp file the test can bind-mount into the verify container.
func loadVerifyScript(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// go/pkg/db -> repo root is three levels up
	repoRoot := filepath.Join(wd, "..", "..", "..")
	cmPath := filepath.Join(repoRoot, "java", "k8s", "configmaps", "postgres-verify-scripts.yml")
	raw, err := os.ReadFile(cmPath)
	if err != nil {
		t.Fatalf("read configmap: %v", err)
	}
	var doc struct {
		Data map[string]string `yaml:"data"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal configmap: %v", err)
	}
	body, ok := doc.Data["pg-verify-backups.sh"]
	if !ok {
		t.Fatalf("pg-verify-backups.sh not found in configmap")
	}
	tmp := filepath.Join(t.TempDir(), "pg-verify-backups.sh")
	if err := os.WriteFile(tmp, []byte(body), 0o555); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return tmp
}

// pushgatewayMock records POST bodies keyed by request path.
type pushgatewayMock struct {
	mu     sync.Mutex
	bodies map[string]string
	server *httptest.Server
}

func newPushgatewayMock() *pushgatewayMock {
	m := &pushgatewayMock{bodies: map[string]string{}}
	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		m.mu.Lock()
		// Append — multiple POSTs to the same path replace, but we keep all
		// for assertion clarity.
		m.bodies[r.URL.Path] = m.bodies[r.URL.Path] + string(body)
		m.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	return m
}

func (m *pushgatewayMock) Close() { m.server.Close() }

func (m *pushgatewayMock) URLForContainer(t *testing.T) string {
	// host.docker.internal is automatically mapped by testcontainers on macOS
	// and Linux (with --add-host=host.docker.internal:host-gateway).
	t.Helper()
	addr := m.server.Listener.Addr().String()
	parts := strings.Split(addr, ":")
	port := parts[len(parts)-1]
	return "http://host.docker.internal:" + port
}

func (m *pushgatewayMock) BodyFor(path string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bodies[path]
}
```

The `host.docker.internal` mapping is automatic on Docker Desktop and Colima. For Linux runners (CI Debian), testcontainers-go automatically adds `--add-host=host.docker.internal:host-gateway`.

- [ ] **Step 9.2: Run the test file to verify it compiles**

Run: `cd go/pkg && go vet -tags=integration ./db/...`
Expected: no errors.

If `gopkg.in/yaml.v3` isn't already a dependency, add it:

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification/go/pkg
go get gopkg.in/yaml.v3
go mod tidy
```

Then re-run vet. Expected: clean.

- [ ] **Step 9.3: Add the success-path test**

Append to `go/pkg/db/backup_verification_integration_test.go`:

```go
func TestBackupVerification_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	ctx := context.Background()

	// 1. Seed a source Postgres container.
	src, err := postgres.Run(ctx,
		"postgres:17-alpine",
		postgres.WithDatabase("appdb"),
		postgres.WithUsername("appuser"),
		postgres.WithPassword("apppass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start source postgres: %v", err)
	}
	t.Cleanup(func() { _ = src.Terminate(ctx) })

	dsn, err := src.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("dsn: %v", err)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, `
		CREATE TABLE widgets (id SERIAL PRIMARY KEY, name TEXT NOT NULL);
		INSERT INTO widgets (name)
		SELECT 'widget-' || g FROM generate_series(1, 25) g;
	`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// 2. Dump appdb into a host temp dir following <db>-YYYY-MM-DD.dump naming.
	dumpHostDir := t.TempDir()
	dumpName := "appdb-" + time.Now().UTC().Format("2006-01-02") + ".dump"
	dumpInContainer := "/tmp/" + dumpName
	rc, _, err := src.Exec(ctx, []string{
		"pg_dump", "--format=custom",
		"-U", "appuser", "-d", "appdb",
		"-f", dumpInContainer,
	})
	if err != nil || rc != 0 {
		t.Fatalf("pg_dump exec rc=%d err=%v", rc, err)
	}
	r, err := src.CopyFileFromContainer(ctx, dumpInContainer)
	if err != nil {
		t.Fatalf("copy dump out: %v", err)
	}
	dumpPath := filepath.Join(dumpHostDir, dumpName)
	out, err := os.Create(dumpPath)
	if err != nil {
		t.Fatalf("create dump file: %v", err)
	}
	if _, err := io.Copy(out, r); err != nil {
		t.Fatalf("copy dump: %v", err)
	}
	out.Close()
	r.Close()

	// 3. Mock Pushgateway.
	pg := newPushgatewayMock()
	t.Cleanup(pg.Close)

	// 4. Run the verify container.
	scriptPath := loadVerifyScript(t)
	verifyReq := testcontainers.ContainerRequest{
		Image: "postgres:17-alpine",
		Cmd: []string{"sh", "-c",
			"apk add --no-cache curl >/dev/null && /scripts/pg-verify-backups.sh"},
		Env: map[string]string{
			"PUSHGATEWAY_URL": pg.URLForContainer(t),
			"VERIFY_DBS":      "appdb",
			"DUMPS_DIR":       "/backups/postgres",
		},
		Files: []testcontainers.ContainerFile{
			{HostFilePath: scriptPath, ContainerFilePath: "/scripts/pg-verify-backups.sh", FileMode: 0o555},
			{HostFilePath: dumpPath, ContainerFilePath: "/backups/postgres/" + dumpName, FileMode: 0o444},
		},
		HostConfigModifier: func(cfg *container.HostConfig) {
			cfg.ExtraHosts = append(cfg.ExtraHosts, "host.docker.internal:host-gateway")
		},
		User:       "70:70",
		WaitingFor: wait.ForExit().WithExitTimeout(3 * time.Minute),
	}
	verify, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: verifyReq,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("start verify container: %v", err)
	}
	t.Cleanup(func() { _ = verify.Terminate(ctx) })

	state, err := verify.State(ctx)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	if state.ExitCode != 0 {
		logs, _ := verify.Logs(ctx)
		buf, _ := io.ReadAll(logs)
		t.Fatalf("verify container exit=%d, logs:\n%s", state.ExitCode, string(buf))
	}

	// 5. Assert the mock received per-DB success metrics.
	got := pg.BodyFor("/metrics/job/postgres_backup_verify/instance/appdb")
	if !strings.Contains(got, "backup_verification_last_success_timestamp") {
		t.Errorf("missing last_success_timestamp in pushed body: %q", got)
	}
	if !strings.Contains(got, "backup_verification_restored_rows") {
		t.Errorf("missing restored_rows in pushed body: %q", got)
	}
	overall := pg.BodyFor("/metrics/job/postgres_backup_verify")
	if !strings.Contains(overall, "backup_verification_run_success 1") {
		t.Errorf("missing run_success=1 in overall body: %q", overall)
	}
}
```

Add this import to the existing import block at the top of the file:

```go
"github.com/docker/docker/api/types/container"
```

- [ ] **Step 9.4: Run the success test**

Run: `cd go/pkg && go test -tags=integration -run TestBackupVerification_Success ./db/... -v`
Expected: PASS within ~2–3 minutes (testcontainers cold start dominates).

If Docker isn't running locally (Colima not started), the test will fail with a clear "cannot connect to docker daemon" error — start Colima (`colima start`) and re-run.

- [ ] **Step 9.5: Add the failure-path test**

Append to `go/pkg/db/backup_verification_integration_test.go`:

```go
func TestBackupVerification_FailureOnCorruptDump(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	ctx := context.Background()

	dumpHostDir := t.TempDir()
	dumpName := "appdb-" + time.Now().UTC().Format("2006-01-02") + ".dump"
	dumpPath := filepath.Join(dumpHostDir, dumpName)
	// A truncated/garbage file — pg_restore will reject it.
	if err := os.WriteFile(dumpPath, []byte("not a real pg_dump file"), 0o644); err != nil {
		t.Fatalf("write corrupt dump: %v", err)
	}

	pg := newPushgatewayMock()
	t.Cleanup(pg.Close)

	scriptPath := loadVerifyScript(t)
	verify, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "postgres:17-alpine",
			Cmd: []string{"sh", "-c",
				"apk add --no-cache curl >/dev/null && /scripts/pg-verify-backups.sh"},
			Env: map[string]string{
				"PUSHGATEWAY_URL": pg.URLForContainer(t),
				"VERIFY_DBS":      "appdb",
				"DUMPS_DIR":       "/backups/postgres",
			},
			Files: []testcontainers.ContainerFile{
				{HostFilePath: scriptPath, ContainerFilePath: "/scripts/pg-verify-backups.sh", FileMode: 0o555},
				{HostFilePath: dumpPath, ContainerFilePath: "/backups/postgres/" + dumpName, FileMode: 0o444},
			},
			HostConfigModifier: func(cfg *container.HostConfig) {
				cfg.ExtraHosts = append(cfg.ExtraHosts, "host.docker.internal:host-gateway")
			},
			User:       "70:70",
			WaitingFor: wait.ForExit().WithExitTimeout(2 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start verify: %v", err)
	}
	t.Cleanup(func() { _ = verify.Terminate(ctx) })

	state, err := verify.State(ctx)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	if state.ExitCode == 0 {
		t.Fatalf("expected non-zero exit on corrupt dump, got 0")
	}

	got := pg.BodyFor("/metrics/job/postgres_backup_verify/instance/appdb")
	if !strings.Contains(got, "backup_verification_last_failure_timestamp") {
		t.Errorf("missing failure_timestamp metric: %q", got)
	}
	if !strings.Contains(got, `reason="pg_restore_failed"`) {
		t.Errorf("expected pg_restore_failed reason in body: %q", got)
	}
	overall := pg.BodyFor("/metrics/job/postgres_backup_verify")
	if !strings.Contains(overall, "backup_verification_run_success 0") {
		t.Errorf("expected run_success=0 in overall body: %q", overall)
	}
}
```

Use the existing `bytes` import in your file's import block (added in step 9.1) — these tests don't need any new imports beyond what's already there.

- [ ] **Step 9.6: Run the failure test**

Run: `cd go/pkg && go test -tags=integration -run TestBackupVerification_FailureOnCorruptDump ./db/... -v`
Expected: PASS.

- [ ] **Step 9.7: Run both tests**

Run: `cd go/pkg && go test -tags=integration -run TestBackupVerification ./db/... -v`
Expected: both PASS.

- [ ] **Step 9.8: Run go preflight on the affected service**

Run: `make preflight-go`
Expected: lint + non-integration tests pass. The integration test is gated by `//go:build integration` so it doesn't run here — that's intentional, matches the existing `extensions_integration_test.go` pattern.

- [ ] **Step 9.9: Commit**

```bash
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification add go/pkg/db/backup_verification_integration_test.go go/pkg/go.mod go/pkg/go.sum
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification commit -m "test(db): integration test for backup verification script (testcontainers)"
```

---

### Task 10: Grafana dashboard panels

We append three panels to the **existing** `postgresql` dashboard JSON inside the `grafana-dashboards` ConfigMap. The spec is explicit: do NOT touch the `pg-query-performance` dashboard.

**Files:**
- Modify: `k8s/monitoring/configmaps/grafana-dashboards.yml` — append panels inside the `postgresql.json` panels array.

The existing dashboard's last panel is `Connections by Service` (id 7) with `gridPos: { h: 8, w: 12, x: 0, y: 16 }`. Our new panels go at `y: 24` (3 panels of `h: 8`).

- [ ] **Step 10.1: Locate the insertion point**

Run: `grep -n '"title": "Connections by Service"' k8s/monitoring/configmaps/grafana-dashboards.yml`
Expected: a single line number (around 3471).

Run: `grep -n '"title": "PostgreSQL"' k8s/monitoring/configmaps/grafana-dashboards.yml`
Expected: a single line near the end of the postgresql dashboard block (~3508).

The `Connections by Service` panel ends at the `}` followed by `]` (close panels array) followed by `"schemaVersion": 39,` for the postgresql dashboard. We insert before the `]` that closes the panels array.

- [ ] **Step 10.2: Append the three panels**

Locate this exact block in `k8s/monitoring/configmaps/grafana-dashboards.yml` (the closing of the `Connections by Service` panel):

```
          "options": {
            "orientation": "horizontal",
            "displayMode": "gradient"
          }
        }
      ],
      "schemaVersion": 39,
      "tags": ["postgresql", "database"],
```

(Note: this exact `]` + `"schemaVersion": 39,` + `"tags": ["postgresql", "database"]` sequence is unique to the postgresql dashboard — the pg-query-performance dashboard has its own.)

Replace with:

```
          "options": {
            "orientation": "horizontal",
            "displayMode": "gradient"
          }
        },
        {
          "title": "Backup Verification — Time Since Last Success",
          "type": "stat",
          "gridPos": { "h": 8, "w": 8, "x": 0, "y": 24 },
          "id": 8,
          "datasource": { "type": "prometheus", "uid": "" },
          "targets": [
            {
              "expr": "time() - backup_verification_last_success_timestamp",
              "legendFormat": "{{instance}}",
              "refId": "A"
            }
          ],
          "fieldConfig": {
            "defaults": {
              "unit": "s",
              "thresholds": {
                "steps": [
                  { "color": "green", "value": null },
                  { "color": "yellow", "value": 604800 },
                  { "color": "red", "value": 691200 }
                ]
              }
            },
            "overrides": []
          },
          "options": {
            "reduceOptions": { "calcs": ["lastNotNull"], "fields": "", "values": false },
            "textMode": "value_and_name",
            "colorMode": "value",
            "orientation": "horizontal"
          }
        },
        {
          "title": "Backup Verification Failures (last 30d)",
          "type": "timeseries",
          "gridPos": { "h": 8, "w": 8, "x": 8, "y": 24 },
          "id": 9,
          "datasource": { "type": "prometheus", "uid": "" },
          "targets": [
            {
              "expr": "increase(backup_verification_last_failure_timestamp[30d])",
              "legendFormat": "{{instance}}",
              "refId": "A"
            }
          ],
          "fieldConfig": {
            "defaults": {
              "unit": "short",
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
          "title": "Backup Verification — Restored Row Counts",
          "type": "bargauge",
          "gridPos": { "h": 8, "w": 8, "x": 16, "y": 24 },
          "id": 10,
          "datasource": { "type": "prometheus", "uid": "" },
          "targets": [
            {
              "expr": "backup_verification_restored_rows",
              "legendFormat": "{{instance}}",
              "refId": "A"
            }
          ],
          "fieldConfig": {
            "defaults": {
              "unit": "short",
              "thresholds": {
                "steps": [
                  { "color": "red", "value": null },
                  { "color": "yellow", "value": 1 },
                  { "color": "green", "value": 100 }
                ]
              }
            },
            "overrides": []
          },
          "options": {
            "orientation": "horizontal",
            "displayMode": "gradient"
          }
        }
      ],
      "schemaVersion": 39,
      "tags": ["postgresql", "database"],
```

The 3 panels lay out at `y: 24` in three columns (`x: 0/8/16`, each `w: 8`).

- [ ] **Step 10.3: Validate the JSON parses**

Run:
```bash
python3 -c "
import yaml, json
with open('k8s/monitoring/configmaps/grafana-dashboards.yml') as f:
    cm = yaml.safe_load(f)
for k, v in cm['data'].items():
    json.loads(v)  # raises if any dashboard JSON is malformed
    print(k, 'OK')
"
```
Expected: every dashboard prints `OK`. If any raises a `JSONDecodeError`, fix the surrounding indentation/braces.

- [ ] **Step 10.4: Validate kustomization renders**

Run: `kubectl kustomize k8s/monitoring | grep -c "Backup Verification — Time Since Last Success"`
Expected: `1` (panel title appears once in the rendered ConfigMap).

- [ ] **Step 10.5: Commit**

```bash
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification add k8s/monitoring/configmaps/grafana-dashboards.yml
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification commit -m "feat(monitoring): add backup verification panels to PostgreSQL dashboard"
```

---

### Task 11: Grafana alerts

We append two alerts to the **existing** `PostgreSQL` group in `grafana-alerting.yml`. Both follow the existing per-rule pattern: A=metric, B=reduce, C=threshold.

**Files:**
- Modify: `k8s/monitoring/configmaps/grafana-alerting.yml`

- [ ] **Step 11.1: Locate the insertion point**

Run: `grep -n 'pg-auto-explain-stalled' k8s/monitoring/configmaps/grafana-alerting.yml`
Expected: one line (the last existing rule in the PostgreSQL group, around line 1694).

Run: `tail -10 k8s/monitoring/configmaps/grafana-alerting.yml`
Expected: the file ends after the `pg-auto-explain-stalled` block — confirm by inspecting the last 30 lines.

- [ ] **Step 11.2: Append the two new rules**

Locate the very last block of the file:

```
          - uid: pg-auto-explain-stalled
            ...
            for: 5m
            labels:
              severity: warning
            annotations:
              summary: "No auto_explain log lines in 24h — query observability is silently broken"
```

Append (preserving the 10-space indent that aligns with sibling rules — same as the rules above it):

```yaml

          - uid: pg-backup-verification-failed
            title: Postgres Backup Verification Failed
            noDataState: OK
            condition: C
            data:
              - refId: A
                relativeTimeRange: { from: 600, to: 0 }
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: >-
                    max by (instance) (backup_verification_last_failure_timestamp)
                    - max by (instance) (backup_verification_last_success_timestamp)
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange: { from: 600, to: 0 }
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange: { from: 600, to: 0 }
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator: { type: gt, params: [0] }
                  refId: C
            for: 5m
            labels:
              severity: critical
            annotations:
              summary: "Backup verification FAILED for {{ $labels.instance }} — most recent verification did not restore"

          - uid: pg-backup-verification-stale
            title: Postgres Backup Verification Stale
            noDataState: OK
            condition: C
            data:
              - refId: A
                relativeTimeRange: { from: 600, to: 0 }
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: time() - max by (instance) (backup_verification_last_success_timestamp)
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange: { from: 600, to: 0 }
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange: { from: 600, to: 0 }
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator: { type: gt, params: [691200] }
                  refId: C
            for: 5m
            labels:
              severity: warning
            annotations:
              summary: "No successful backup verification for {{ $labels.instance }} in over 8 days"
```

The thresholds:
- `pg-backup-verification-failed`: failure timestamp > success timestamp (per instance) — newer failure than success means the latest run is bad.
- `pg-backup-verification-stale`: time since success > 691200s (8 days). Schedule is weekly, so 8 days = one missed run.

- [ ] **Step 11.3: Validate YAML parses**

Run:
```bash
python3 -c "
import yaml
with open('k8s/monitoring/configmaps/grafana-alerting.yml') as f:
    cm = yaml.safe_load(f)
# Ensure the embedded alerting.yml string is also valid YAML
yaml.safe_load(cm['data']['alerting.yml'])
print('OK')
"
```
Expected: `OK`.

- [ ] **Step 11.4: Validate kustomization renders**

Run: `kubectl kustomize k8s/monitoring | grep -c "pg-backup-verification-"`
Expected: `2` (the two new uids appear in the rendered ConfigMap).

- [ ] **Step 11.5: Commit**

```bash
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification add k8s/monitoring/configmaps/grafana-alerting.yml
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification commit -m "feat(monitoring): add backup verification alerts (failed + stale)"
```

---

### Task 12: ADR

**Files:**
- Create: `docs/adr/database/backup-verification.md`

- [ ] **Step 12.1: Create the ADR**

Note: the `docs/adr/database/` directory does not yet exist — `Write` will create it.

Create `docs/adr/database/backup-verification.md` with:

```markdown
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

This ADR records the design of an automated weekly verification job that actually restores each backup, runs a smoke check, and emits per-DB Prometheus metrics.

## Decision

A new CronJob (`postgres-backup-verify` in `java-tasks`, Mondays 04:00 UTC) restores every prod DB's latest dump into an ephemeral local Postgres inside its pod, runs a row-count smoke check, and pushes per-DB metrics to a new Pushgateway in the `monitoring` namespace. Two alerts (failure + staleness) fire if anything regresses.

### Why Pushgateway over kube-state-metrics-only

`kube_job_status_succeeded{job_name=~"postgres-backup-verify-.*"}` tells us *the cron ran*. It does not tell us *which database* failed. The verify pod restores 7 databases; if `paymentdb` silently truncates while `authdb` is fine, kube-state-metrics shows "job succeeded" even though one DB is broken (or worse, the script exits 1 and we know nothing about which DB caused it).

Pushgateway lets the script emit `backup_verification_last_success_timestamp{instance="<db>"}` per DB, so the alert message says *exactly* which DB to investigate. Single-source-of-truth for "is each backup verified?" without reaching into job logs.

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
```

- [ ] **Step 12.2: Verify the ADR renders**

Run: `wc -l docs/adr/database/backup-verification.md`
Expected: ~80–100 lines, no errors.

- [ ] **Step 12.3: Commit (LOCAL only — doc-only change, do not push yet)**

```bash
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification add docs/adr/database/backup-verification.md
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification commit -m "docs(adr): backup verification design and trade-offs"
```

This commit will be pushed together with the runbook update and the code changes — see Task 13 and the final push step.

---

### Task 13: Runbook update

**Files:**
- Modify: `docs/runbooks/postgres-recovery.md`

- [ ] **Step 13.1: Add the alerts list entry and a verification section**

Open `docs/runbooks/postgres-recovery.md` and find the **Related alerts** block:

```markdown
**Related alerts:**
- `Postgres Backup Stale` — no successful backup in 26h
- `Postgres Connection Utilization High` — connections > 80%
- `Postgres Cache Hit Ratio Low` — cache ratio < 95%
- `Postgres Deadlocks Detected` — deadlocks in 5m window
```

Append two new bullet points:

```markdown
- `Postgres Backup Verification Failed` — most recent restore-from-dump failed for at least one DB
- `Postgres Backup Verification Stale` — no successful verification in over 8 days
```

Then find the **Backup location** line:

```markdown
**Backup location:** `/backups/postgres/` on the Debian host (outside Minikube PVC)
```

After it, insert a new section before `## Scenario 1`:

```markdown
**Verification:** A weekly CronJob (`postgres-backup-verify` in `java-tasks`, Mondays 04:00 UTC) restores every dump into an ephemeral local Postgres and pushes per-DB metrics to Pushgateway. Check the most recent run with:

```bash
ssh debian "kubectl get jobs -n java-tasks -l job-name --sort-by=.status.completionTime | grep postgres-backup-verify | tail -3"
ssh debian "kubectl logs -n java-tasks job/<latest-verify-job-name>"
```

To trigger an ad-hoc verification (e.g., after restoring backups manually):

```bash
ssh debian "kubectl create job --from=cronjob/postgres-backup-verify postgres-backup-verify-manual-$(date +%s) -n java-tasks"
```

The Grafana **PostgreSQL** dashboard's "Backup Verification — Time Since Last Success" panel shows per-DB freshness; green = within a week, red = over 8 days.

---
```

(That trailing `---` separates the new section from the existing `## Scenario 1` header.)

- [ ] **Step 13.2: Verify the runbook is well-formed**

Run: `grep -n "Postgres Backup Verification" docs/runbooks/postgres-recovery.md`
Expected: at least 2 lines (alerts list entries) plus the verification section header.

- [ ] **Step 13.3: Commit (LOCAL only — doc-only)**

```bash
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification add docs/runbooks/postgres-recovery.md
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification commit -m "docs(runbook): add backup verification alerts and verification section"
```

---

### Task 14: Final preflight + push (Phase 1 close-out)

- [ ] **Step 14.1: Run the full preflight sweep**

Run: `make preflight-go`
Expected: pass.

Run: `kubectl kustomize k8s/monitoring > /dev/null && echo OK`
Expected: `OK`.

Run: `kubectl kustomize java/k8s > /dev/null && echo OK`
Expected: `OK`.

Run: `python3 -c "import yaml,json; cm=yaml.safe_load(open('k8s/monitoring/configmaps/grafana-dashboards.yml')); [json.loads(v) for v in cm['data'].values()]; print('dashboards OK')"`
Expected: `dashboards OK`.

- [ ] **Step 14.2: Push the feature branch**

Run: `git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification push -u origin agent/feat-backup-verification`
Expected: branch created on remote.

- [ ] **Step 14.3: Open a PR to qa**

Run:
```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification
gh pr create --base qa --title "feat: automated backup verification (db-roadmap 4/10, phase 1)" --body "$(cat <<'EOF'
## Summary

- Adds Pushgateway to the `monitoring` namespace (with persistent storage)
- Adds a weekly `postgres-backup-verify` CronJob in `java-tasks` that restores every `pg_dump` file into an ephemeral local Postgres, smoke-checks it, and pushes per-DB metrics to Pushgateway
- Adds 3 new panels to the PostgreSQL dashboard and 2 new alerts (`PgBackupVerificationFailed`, `PgBackupVerificationStale`)
- Adds an integration test (`go/pkg/db/backup_verification_integration_test.go`) using testcontainers
- Documents the design in `docs/adr/database/backup-verification.md` and updates the recovery runbook

## Phase split

This PR ships **Phase 1** of the [backup-verification spec](docs/superpowers/specs/2026-04-27-backup-verification-design.md) — `pg_dump` verification only. **Phase 2** (PITR verification with sentinel rows) is gated on roadmap item #157 (WAL archiving + PITR) and will ship as a follow-up PR using the same CronJob.

## Closes
- #158 (partial — phase 1 only; phase 2 stays open until #157 merges)

## Test plan
- [ ] CI quality checks pass (lint, test, k8s validation)
- [ ] After deploy to QA: `kubectl get cronjob/postgres-backup-verify -n java-tasks` shows the cron with next scheduled run
- [ ] After deploy to QA: `kubectl get deployment/pushgateway -n monitoring` shows 1/1 ready
- [ ] After deploy to QA: trigger ad-hoc run via `kubectl create job --from=cronjob/postgres-backup-verify postgres-backup-verify-test-$(date +%s) -n java-tasks` and confirm exit code 0 and Pushgateway metrics appear
- [ ] Grafana PostgreSQL dashboard shows the 3 new panels
- [ ] Grafana alerts list shows `Postgres Backup Verification Failed` and `Postgres Backup Verification Stale`
EOF
)"
```

Expected: PR URL is printed.

- [ ] **Step 14.4: Notify Kyle**

Print the PR URL. Do NOT watch CI.

---

## Phase 2: PITR verification (implement after #157 merges)

> **Phase 2 only — implement after roadmap item #157 (WAL archiving + PITR) has merged to `main`.**
>
> Before starting Phase 2: confirm `/backups/wal-archive` and `/backups/basebackup` hostPath PVs exist in the cluster (these are added by #157). If they don't, stop — #157 has not landed yet.

### Task 15: Sentinel-row seed CronJob (Phase 2 only — implement after #157)

The PITR verifier needs a known-state value to assert against post-restore. We add a small weekly CronJob that writes a deterministic sentinel into a meta DB.

**Files:**
- Create: `java/k8s/jobs/postgres-verify-sentinel.yml`
- Modify: `java/k8s/configmaps/postgres-initdb.yml` (add `CREATE DATABASE backup_verify_meta;` if not present)

- [ ] **Step 15.1: Verify the meta DB exists in the seed config**

Run: `grep -n "backup_verify_meta" java/k8s/configmaps/postgres-initdb.yml`
Expected: at least one line. If zero lines, add `CREATE DATABASE backup_verify_meta;` to the initdb script in the existing ConfigMap (preserving the existing `CREATE DATABASE` style).

- [ ] **Step 15.2: Create the sentinel seed CronJob**

Create `java/k8s/jobs/postgres-verify-sentinel.yml` with:

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: postgres-verify-sentinel
  namespace: java-tasks
spec:
  schedule: "0 3 * * 0" # Sundays 03:00 UTC, before Monday's verify
  concurrencyPolicy: Forbid
  successfulJobsHistoryLimit: 1
  failedJobsHistoryLimit: 1
  jobTemplate:
    spec:
      backoffLimit: 1
      activeDeadlineSeconds: 120
      template:
        spec:
          restartPolicy: Never
          containers:
            - name: seed
              image: postgres:17-alpine
              command: ["/bin/sh", "-c"]
              args:
                - |
                  set -eu
                  TS="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
                  psql -h postgres.java-tasks.svc.cluster.local -U taskuser -d backup_verify_meta <<SQL
                  CREATE TABLE IF NOT EXISTS sentinel (
                    id INT PRIMARY KEY,
                    app_state TEXT NOT NULL,
                    written_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
                  );
                  INSERT INTO sentinel (id, app_state)
                  VALUES (1, 'verified-${TS}')
                  ON CONFLICT (id) DO UPDATE
                    SET app_state = EXCLUDED.app_state, written_at = NOW();
                  SQL
              env:
                - name: PGPASSWORD
                  valueFrom:
                    secretKeyRef:
                      name: java-secrets
                      key: postgres-password
              resources:
                requests: { cpu: "50m", memory: "32Mi" }
                limits:   { cpu: "200m", memory: "128Mi" }
```

The verify script (next task) reads the most recent expected `app_state` for the chosen `recovery_target_time` from this table.

- [ ] **Step 15.3: Validate and commit**

Run: `kubectl apply --dry-run=client -f java/k8s/jobs/postgres-verify-sentinel.yml`
Expected: dry-run success.

```bash
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification add java/k8s/jobs/postgres-verify-sentinel.yml java/k8s/configmaps/postgres-initdb.yml
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification commit -m "feat(java-tasks): add sentinel-row seed CronJob (phase 2)"
```

---

### Task 16: PITR verification script (Phase 2 only — implement after #157)

**Files:**
- Modify: `java/k8s/configmaps/postgres-verify-scripts.yml` — add a second key `pg-verify-pitr.sh` next to the existing `pg-verify-backups.sh`.

- [ ] **Step 16.1: Append the PITR script to the ConfigMap**

Edit `java/k8s/configmaps/postgres-verify-scripts.yml`. After the `pg-verify-backups.sh: |` block (the entire script body), add at the same indentation level:

```yaml
  pg-verify-pitr.sh: |
    #!/bin/sh
    # Restore the latest base backup, replay WAL to a random recent target
    # time, promote, and assert the sentinel row matches the expected value
    # at that target.
    set -eu

    PUSHGATEWAY="${PUSHGATEWAY_URL:-http://pushgateway.monitoring.svc.cluster.local:9091}"
    BASEBACKUP_DIR="${BASEBACKUP_DIR:-/backups/basebackup}"
    WAL_ARCHIVE_DIR="${WAL_ARCHIVE_DIR:-/backups/wal-archive}"
    DATA_DIR="${PG_DATA_DIR:-/var/lib/postgresql/data}"
    SOCKET_DIR="${PG_SOCKET_DIR:-/tmp/pg-verify-pitr}"
    INSTANCE="${INSTANCE:-pitr}"

    push_pitr_success() {
      target="$1"; sentinel="$2"
      cat <<EOF | curl -fsS --data-binary @- "$PUSHGATEWAY/metrics/job/postgres_backup_verify/instance/$INSTANCE"
    # TYPE backup_verification_pitr_last_success_timestamp gauge
    backup_verification_pitr_last_success_timestamp $(date +%s)
    # TYPE backup_verification_pitr_target_time gauge
    backup_verification_pitr_target_time $target
    backup_verification_pitr_sentinel_match{sentinel="$sentinel"} 1
    EOF
    }

    push_pitr_failure() {
      reason="$1"
      cat <<EOF | curl -fsS --data-binary @- "$PUSHGATEWAY/metrics/job/postgres_backup_verify/instance/$INSTANCE"
    # TYPE backup_verification_pitr_last_failure_timestamp gauge
    backup_verification_pitr_last_failure_timestamp $(date +%s)
    backup_verification_pitr_last_failure_reason{reason="$reason"} 1
    EOF
    }

    rm -rf "$DATA_DIR" "$SOCKET_DIR"
    mkdir -p "$DATA_DIR" "$SOCKET_DIR"
    chmod 700 "$DATA_DIR" "$SOCKET_DIR"

    LATEST_BASE="$(ls -dt "$BASEBACKUP_DIR"/*/ 2>/dev/null | head -1 || true)"
    if [ -z "$LATEST_BASE" ]; then
      echo "FAIL: no base backups in $BASEBACKUP_DIR"
      push_pitr_failure "no_basebackup"
      exit 1
    fi

    cp -a "$LATEST_BASE/." "$DATA_DIR/"

    NOW="$(date +%s)"
    SIX_DAYS_AGO=$(( NOW - 6 * 86400 ))
    BASE_AGE=$(stat -c %Y "$LATEST_BASE")
    WINDOW_START=$(( BASE_AGE > SIX_DAYS_AGO ? BASE_AGE : SIX_DAYS_AGO ))
    WINDOW_LEN=$(( NOW - WINDOW_START ))
    if [ "$WINDOW_LEN" -le 60 ]; then
      echo "FAIL: PITR window is too small ($WINDOW_LEN s)"
      push_pitr_failure "window_too_small"
      exit 1
    fi
    OFFSET=$(( $(od -An -N4 -tu4 /dev/urandom | tr -d ' ') % WINDOW_LEN ))
    TARGET_EPOCH=$(( WINDOW_START + OFFSET ))
    TARGET_ISO="$(date -u -d @"$TARGET_EPOCH" +%Y-%m-%dT%H:%M:%SZ)"
    echo "PITR target: $TARGET_ISO"

    cat > "$DATA_DIR/postgresql.auto.conf" <<EOF
    restore_command = 'cp $WAL_ARCHIVE_DIR/%f %p'
    recovery_target_time = '$TARGET_ISO'
    recovery_target_action = 'promote'
    EOF
    touch "$DATA_DIR/recovery.signal"

    pg_ctl -D "$DATA_DIR" -l /tmp/pg-verify-pitr.log -o "-k $SOCKET_DIR -h ''" -w start
    trap 'pg_ctl -D "$DATA_DIR" -m fast stop || true' EXIT

    # Wait for promotion (recovery.signal disappears).
    for i in $(seq 1 60); do
      if [ ! -f "$DATA_DIR/recovery.signal" ]; then break; fi
      sleep 2
    done

    EXPECTED="$(psql -h "$SOCKET_DIR" -U postgres -d backup_verify_meta -t -A -c "
      SELECT app_state FROM sentinel
      WHERE written_at <= '$TARGET_ISO'::timestamptz
      ORDER BY written_at DESC LIMIT 1;
    " 2>/dev/null || true)"

    if [ -z "$EXPECTED" ]; then
      echo "FAIL: no sentinel row at or before $TARGET_ISO"
      push_pitr_failure "no_sentinel_at_target"
      exit 1
    fi
    echo "Sentinel at target: $EXPECTED"
    push_pitr_success "$TARGET_EPOCH" "$EXPECTED"
    exit 0
```

Notes:
- The script restores the basebackup tarball-equivalent (the daily basebackup CronJob from #157 is expected to write directories under `/backups/basebackup/<timestamp>/`, not tarballs — confirm this when #157 lands and adjust if it ships tarballs instead).
- `od -An -N4 -tu4 /dev/urandom` picks a random 32-bit number portably across busybox `sh` / dash / bash.
- `recovery_target_action = 'promote'` ensures Postgres exits recovery mode automatically.
- The sentinel assertion finds the most recent sentinel row written *before or at* the target time and asserts that's what the restored DB shows. (Schema implied by Task 15.)

- [ ] **Step 16.2: Validate YAML and commit**

Run: `kubectl apply --dry-run=client -f java/k8s/configmaps/postgres-verify-scripts.yml`
Expected: dry-run success.

```bash
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification add java/k8s/configmaps/postgres-verify-scripts.yml
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification commit -m "feat(java-tasks): add PITR verification script (phase 2)"
```

---

### Task 17: Wire PITR into the verify CronJob (Phase 2 only — implement after #157)

**Files:**
- Modify: `java/k8s/jobs/postgres-backup-verify.yml`
- Modify: `java/k8s/kustomization.yaml`

- [ ] **Step 17.1: Add PITR volumes and run both scripts**

Edit `java/k8s/jobs/postgres-backup-verify.yml`. Replace the `args:` block:

```yaml
              args:
                - |
                  set -eu
                  apk add --no-cache curl >/dev/null
                  exec /scripts/pg-verify-backups.sh
```

with:

```yaml
              args:
                - |
                  set -eu
                  apk add --no-cache curl >/dev/null
                  /scripts/pg-verify-backups.sh
                  RC1=$?
                  /scripts/pg-verify-pitr.sh
                  RC2=$?
                  if [ "$RC1" -ne 0 ] || [ "$RC2" -ne 0 ]; then exit 1; fi
                  exit 0
```

Add two new entries to the `volumeMounts:` block, after the existing `backups` mount:

```yaml
                - name: wal-archive
                  mountPath: /backups/wal-archive
                  readOnly: true
                - name: basebackup
                  mountPath: /backups/basebackup
                  readOnly: true
```

Add two new entries to the `volumes:` block, after the existing `backups` volume:

```yaml
            - name: wal-archive
              persistentVolumeClaim:
                claimName: postgres-wal-archive-readonly
                readOnly: true
            - name: basebackup
              persistentVolumeClaim:
                claimName: postgres-basebackup-readonly
                readOnly: true
```

(These PVCs are introduced by #157 — confirm their exact names before this step lands.)

- [ ] **Step 17.2: Wire the sentinel CronJob into the kustomization**

Edit `java/k8s/kustomization.yaml`. After `  - jobs/postgres-backup-verify.yml`, add:

```yaml
  - jobs/postgres-verify-sentinel.yml
```

- [ ] **Step 17.3: Validate, commit, push, PR**

Run: `kubectl kustomize java/k8s | grep -E "postgres-verify-sentinel|wal-archive|basebackup"`
Expected: lines confirming all are present.

```bash
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification add java/k8s/jobs/postgres-backup-verify.yml java/k8s/kustomization.yaml
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification commit -m "feat(java-tasks): wire PITR verification into verify CronJob (phase 2)"
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification push
```

Open a follow-up PR (Phase 2):

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent+feat-backup-verification
gh pr create --base qa --title "feat: backup verification phase 2 — PITR + sentinel" --body "Phase 2 follow-up to #158. Restores latest base backup, replays WAL to a random target time, asserts sentinel row matches expected value. Depends on #157 (already merged)."
```

---

## Self-review checklist

After all tasks are complete (or as far as Phase 1):

- **Spec coverage:** every section of the spec is mapped to at least one task.
  - Pushgateway → Tasks 1–4 ✓
  - Verification script → Task 5 ✓
  - CronJob → Task 7 ✓
  - PV/PVC isolation → Task 6 ✓
  - Metrics + dashboard panels → Task 10 ✓
  - Alerts → Task 11 ✓
  - Integration tests (success + failure) → Task 9 ✓
  - ADR → Task 12 ✓
  - Runbook update → Task 13 ✓
  - Phase 2 (PITR + sentinel) → Tasks 15–17 ✓
- **No placeholders:** every step contains the actual content. No "TBD", no "implement later", no "similar to Task N" without repeating the code.
- **Type/name consistency:** the metric names (`backup_verification_last_success_timestamp` etc.), Pushgateway URL, ConfigMap keys, and label values used in the script match the alert expressions and dashboard queries.
