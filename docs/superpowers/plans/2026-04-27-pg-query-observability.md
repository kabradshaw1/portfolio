# PostgreSQL Query Observability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add query-level observability to the shared PostgreSQL instance via `pg_stat_statements` (latency metrics + live SQL inspection) and `auto_explain` (full plans for slow queries in Loki), with four regression-aware alerts and a dedicated Grafana dashboard.

**Architecture:** Vanilla Postgres image gains startup `-c` flags via `args:` (no custom postgresql.conf — overlays defaults). `postgres_exporter` sidecar gets a custom-queries ConfigMap. A new `grafana_reader` role with `pg_monitor` powers per-DB Grafana PostgreSQL data sources. `auto_explain` writes JSON plans to Postgres logs → Promtail → Loki. New dashboard + alerts in the existing Grafana provisioning ConfigMaps.

**Tech Stack:** PostgreSQL 17, postgres_exporter v0.16.0, Grafana provisioning (datasources, dashboards, alerts), Promtail JSON pipeline, Kubernetes Jobs for one-shot bootstrap, testcontainers-go for integration test.

**Spec:** `docs/superpowers/specs/2026-04-27-pg-query-observability-design.md`

---

## File Structure

| File | Status | Purpose |
|---|---|---|
| `go/pkg/db/extensions_integration_test.go` | create | TDD anchor — testcontainers-driven verification that preload → CREATE EXTENSION → slow query → `pg_stat_statements` row + `auto_explain` JSON log all work |
| `java/k8s/configmaps/postgres-exporter-queries.yml` | create | `postgres-exporter-queries` ConfigMap with two custom queries |
| `java/k8s/configmaps/postgres-initdb.yml` | modify | Add `08-create-extensions.sql` so fresh PVCs get `pg_stat_statements` extension |
| `java/k8s/deployments/postgres.yml` | modify | Add startup `args` (`-c shared_preload_libraries=...` etc.); mount queries ConfigMap on exporter; set `PG_EXPORTER_EXTEND_QUERY_PATH` |
| `java/k8s/jobs/postgres-extensions-bootstrap.yml` | create | One-shot Job: `CREATE EXTENSION IF NOT EXISTS pg_stat_statements` on each existing DB |
| `java/k8s/jobs/postgres-grafana-reader.yml` | create | One-shot Job: create `grafana_reader` role + GRANT `pg_monitor` |
| `java/k8s/secrets/java-secrets.yml.template` | modify | Add `grafana-reader-password` key (template) |
| `java/k8s/kustomization.yaml` | modify | Register the two new ConfigMaps and two new Jobs |
| `k8s/monitoring/configmaps/grafana-datasource.yml` | modify | Add `postgres-productdb`, `postgres-orderdb`, `postgres-paymentdb` data sources |
| `k8s/monitoring/configmaps/promtail-config.yml` | modify | Append a pipeline stage that parses `auto_explain` JSON and exposes `database`/`duration_ms`/`query_id` |
| `k8s/monitoring/configmaps/grafana-dashboards.yml` | modify | Add `pg-query-performance.json` (top-N tables, p95 trend, plan viewer) |
| `k8s/monitoring/configmaps/grafana-alerting.yml` | modify | Append 4 new rules to the `PostgreSQL` alert group |
| `docs/adr/observability/2026-04-27-pg-query-observability.md` | create | Companion ADR documenting decisions + cardinality math + screenshot of a real captured plan |

---

## Task 1: Integration test (TDD anchor)

**Files:**
- Create: `go/pkg/db/extensions_integration_test.go`

This test launches a Postgres testcontainer with the same `shared_preload_libraries` and `auto_explain` settings the K8s deployment will use, exercises the full pipeline, and asserts the observable behaviors. Independent of K8s — runs anywhere with Docker.

- [ ] **Step 1: Write the failing test**

```go
//go:build integration

package db_test

import (
	"context"
	"database/sql"
	"io"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestPgStatStatementsAndAutoExplain(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:17-alpine",
		postgres.WithDatabase("appdb"),
		postgres.WithUsername("appuser"),
		postgres.WithPassword("apppass"),
		testcontainers.WithCmdArgs(
			"-c", "shared_preload_libraries=pg_stat_statements,auto_explain",
			"-c", "pg_stat_statements.max=5000",
			"-c", "pg_stat_statements.track=top",
			"-c", "pg_stat_statements.track_utility=off",
			"-c", "auto_explain.log_min_duration=500ms",
			"-c", "auto_explain.log_analyze=true",
			"-c", "auto_explain.log_buffers=true",
			"-c", "auto_explain.log_timing=true",
			"-c", "auto_explain.log_format=json",
			"-c", "auto_explain.sample_rate=1.0",
		),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = pgContainer.Terminate(ctx) })

	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("dsn: %v", err)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	// Bootstrap step: app must explicitly create the extension per DB
	if _, err := db.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS pg_stat_statements`); err != nil {
		t.Fatalf("create extension: %v", err)
	}
	if _, err := db.ExecContext(ctx, `SELECT pg_stat_statements_reset()`); err != nil {
		t.Fatalf("reset stats: %v", err)
	}

	// Run a deliberately slow query — must exceed auto_explain.log_min_duration
	for i := 0; i < 3; i++ {
		if _, err := db.ExecContext(ctx, `SELECT pg_sleep(0.6)`); err != nil {
			t.Fatalf("slow query: %v", err)
		}
	}

	// Assert it appears in pg_stat_statements
	var calls int64
	var meanMs float64
	err = db.QueryRowContext(ctx, `
		SELECT calls, mean_exec_time
		FROM pg_stat_statements
		WHERE query LIKE '%pg_sleep(0.6)%'
		ORDER BY calls DESC LIMIT 1
	`).Scan(&calls, &meanMs)
	if err != nil {
		t.Fatalf("query pg_stat_statements: %v", err)
	}
	if calls < 3 {
		t.Errorf("expected calls >= 3, got %d", calls)
	}
	if meanMs < 500 {
		t.Errorf("expected mean_exec_time >= 500ms, got %.1fms", meanMs)
	}

	// Assert auto_explain wrote a JSON plan to container logs
	logs, err := pgContainer.Logs(ctx)
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	defer logs.Close()
	raw, err := io.ReadAll(logs)
	if err != nil {
		t.Fatalf("read logs: %v", err)
	}
	logText := string(raw)
	if !strings.Contains(logText, "duration:") || !strings.Contains(logText, "plan:") {
		t.Errorf("expected auto_explain duration/plan markers in logs")
	}
	if !strings.Contains(logText, `"Plan":`) {
		t.Errorf("expected JSON plan body in logs")
	}
}
```

- [ ] **Step 2: Run the test and verify it fails**

`pgx/v5/stdlib` and the testcontainers postgres module are likely not yet in `go/pkg/go.mod`.

```bash
cd go/pkg
go test -tags integration -run TestPgStatStatementsAndAutoExplain ./db/...
```

Expected: build error, missing package `github.com/testcontainers/testcontainers-go/modules/postgres` or similar.

- [ ] **Step 3: Add dependencies**

```bash
cd go/pkg
go get github.com/testcontainers/testcontainers-go/modules/postgres
go get github.com/jackc/pgx/v5/stdlib
go mod tidy
```

- [ ] **Step 4: Run the test and verify it passes**

```bash
cd go/pkg
go test -tags integration -run TestPgStatStatementsAndAutoExplain ./db/...
```

Expected: PASS in ~10–20s (container startup dominates). If it fails because `db/` directory doesn't exist yet, create the file at the path above (it doesn't need a non-test file alongside).

- [ ] **Step 5: Commit**

```bash
git add go/pkg/db/extensions_integration_test.go go/pkg/go.mod go/pkg/go.sum
git commit -m "test: integration test for pg_stat_statements + auto_explain pipeline"
```

---

## Task 2: postgres_exporter custom queries ConfigMap

**Files:**
- Create: `java/k8s/configmaps/postgres-exporter-queries.yml`

- [ ] **Step 1: Write the file**

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: postgres-exporter-queries
  namespace: java-tasks
data:
  queries.yml: |
    pg_stat_statements:
      query: |
        SELECT
          pss.queryid::text AS queryid,
          LEFT(pss.query, 200) AS query_text,
          pss.calls,
          pss.total_exec_time,
          pss.mean_exec_time,
          pss.stddev_exec_time,
          pss.rows
        FROM pg_stat_statements pss
        WHERE pss.calls > 10
        ORDER BY pss.mean_exec_time DESC
        LIMIT 50
      master: true
      metrics:
        - queryid:
            usage: "LABEL"
            description: "pg_stat_statements query ID"
        - query_text:
            usage: "LABEL"
            description: "Truncated query text (200 chars)"
        - calls:
            usage: "COUNTER"
            description: "Number of times executed"
        - total_exec_time:
            usage: "COUNTER"
            description: "Total time in milliseconds"
        - mean_exec_time:
            usage: "GAUGE"
            description: "Mean execution time in milliseconds"
        - stddev_exec_time:
            usage: "GAUGE"
            description: "Standard deviation of execution time"
        - rows:
            usage: "COUNTER"
            description: "Total rows returned"

    pg_stat_statements_io:
      query: |
        SELECT
          pss.queryid::text AS queryid,
          pss.shared_blks_hit,
          pss.shared_blks_read,
          pss.shared_blks_dirtied
        FROM pg_stat_statements pss
        WHERE pss.calls > 10
        ORDER BY pss.shared_blks_read DESC
        LIMIT 50
      master: true
      metrics:
        - queryid:
            usage: "LABEL"
            description: "pg_stat_statements query ID"
        - shared_blks_hit:
            usage: "COUNTER"
            description: "Buffer hits per query"
        - shared_blks_read:
            usage: "COUNTER"
            description: "Disk reads per query"
        - shared_blks_dirtied:
            usage: "COUNTER"
            description: "Buffers dirtied per query"
```

- [ ] **Step 2: Validate YAML**

```bash
kubectl apply --dry-run=client -f java/k8s/configmaps/postgres-exporter-queries.yml
```

Expected: `configmap/postgres-exporter-queries created (dry run)`.

- [ ] **Step 3: Commit**

```bash
git add java/k8s/configmaps/postgres-exporter-queries.yml
git commit -m "k8s(postgres): add postgres_exporter custom queries for pg_stat_statements"
```

---

## Task 3: Update postgres-initdb.yml for fresh-PVC extension creation

**Files:**
- Modify: `java/k8s/configmaps/postgres-initdb.yml`

The init scripts run only on first boot against a fresh PVC. Adds a script that creates `pg_stat_statements` in every database after the `CREATE DATABASE` scripts run. (Alphabetical order matters — `08-` runs after `01-` through `07-`.)

- [ ] **Step 1: Append the new init script**

Append at the end of the `data:` block, after the existing `07-create-projectordb.sql`:

```yaml
  08-create-extensions.sql: |
    -- pg_stat_statements is per-database; auto_explain is server-wide via
    -- shared_preload_libraries (no CREATE EXTENSION needed).
    \connect taskdb
    CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
    \connect authdb
    CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
    \connect orderdb
    CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
    \connect productdb
    CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
    \connect cartdb
    CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
    \connect paymentdb
    CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
    \connect ecommercedb
    CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
    \connect projectordb
    CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
```

- [ ] **Step 2: Validate**

```bash
kubectl apply --dry-run=client -f java/k8s/configmaps/postgres-initdb.yml
```

Expected: dry-run success.

- [ ] **Step 3: Commit**

```bash
git add java/k8s/configmaps/postgres-initdb.yml
git commit -m "k8s(postgres): create pg_stat_statements extension in init scripts"
```

---

## Task 4: Postgres deployment — startup args + exporter custom queries

**Files:**
- Modify: `java/k8s/deployments/postgres.yml`

Two changes: (a) add `args:` to the `postgres` container so `shared_preload_libraries` and `auto_explain.*` are passed at startup; (b) mount the new queries ConfigMap on the `postgres-exporter` sidecar and set `PG_EXPORTER_EXTEND_QUERY_PATH`.

- [ ] **Step 1: Add `args` to the postgres container**

Insert after line 27 (after the `ports:` block), before `lifecycle:`:

```yaml
          args:
            - "-c"
            - "shared_preload_libraries=pg_stat_statements,auto_explain"
            - "-c"
            - "pg_stat_statements.max=5000"
            - "-c"
            - "pg_stat_statements.track=top"
            - "-c"
            - "pg_stat_statements.track_utility=off"
            - "-c"
            - "auto_explain.log_min_duration=500ms"
            - "-c"
            - "auto_explain.log_analyze=true"
            - "-c"
            - "auto_explain.log_buffers=true"
            - "-c"
            - "auto_explain.log_timing=true"
            - "-c"
            - "auto_explain.log_format=json"
            - "-c"
            - "auto_explain.sample_rate=1.0"
```

- [ ] **Step 2: Mount custom queries on the exporter container**

In the `postgres-exporter` container block, add a `volumeMounts` section after the `env` block:

```yaml
          volumeMounts:
            - name: exporter-queries
              mountPath: /etc/postgres-exporter/queries.yml
              subPath: queries.yml
              readOnly: true
```

And add an env var inside the existing `env:` list of the exporter:

```yaml
            - name: PG_EXPORTER_EXTEND_QUERY_PATH
              value: /etc/postgres-exporter/queries.yml
```

- [ ] **Step 3: Add the queries volume to `volumes`**

Append to the `volumes:` block at the bottom of the spec:

```yaml
        - name: exporter-queries
          configMap:
            name: postgres-exporter-queries
```

- [ ] **Step 4: Validate the full manifest**

```bash
kubectl apply --dry-run=server -f java/k8s/deployments/postgres.yml
```

Expected: `deployment.apps/postgres configured (server dry run)`.

- [ ] **Step 5: Commit**

```bash
git add java/k8s/deployments/postgres.yml
git commit -m "k8s(postgres): preload pg_stat_statements + auto_explain; wire exporter custom queries"
```

---

## Task 5: One-shot Job — `CREATE EXTENSION` on existing DBs

**Files:**
- Create: `java/k8s/jobs/postgres-extensions-bootstrap.yml`

For DBs that already exist on the running PVC, `08-create-extensions.sql` from Task 3 won't fire (init scripts only run on fresh PVCs). This Job runs `CREATE EXTENSION IF NOT EXISTS pg_stat_statements` against each prod DB. Idempotent.

- [ ] **Step 1: Write the Job manifest**

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: postgres-extensions-bootstrap
  namespace: java-tasks
spec:
  ttlSecondsAfterFinished: 600
  backoffLimit: 3
  template:
    spec:
      restartPolicy: OnFailure
      containers:
        - name: psql
          image: postgres:17-alpine
          env:
            - name: PGPASSWORD
              valueFrom:
                secretKeyRef:
                  name: java-secrets
                  key: postgres-password
          command:
            - /bin/sh
            - -c
            - |
              set -eu
              for db in taskdb authdb orderdb productdb cartdb paymentdb ecommercedb projectordb; do
                echo "Enabling pg_stat_statements on $db"
                psql -h postgres.java-tasks.svc.cluster.local -U taskuser -d "$db" \
                  -v ON_ERROR_STOP=1 \
                  -c "CREATE EXTENSION IF NOT EXISTS pg_stat_statements;"
              done
              echo "All extensions ready"
          resources:
            requests: { cpu: "10m", memory: "32Mi" }
            limits:   { cpu: "100m", memory: "64Mi" }
```

- [ ] **Step 2: Validate**

```bash
kubectl apply --dry-run=client -f java/k8s/jobs/postgres-extensions-bootstrap.yml
```

- [ ] **Step 3: Commit**

```bash
git add java/k8s/jobs/postgres-extensions-bootstrap.yml
git commit -m "k8s(postgres): bootstrap Job to create pg_stat_statements on existing DBs"
```

---

## Task 6: Add `grafana-reader-password` to secret template

**Files:**
- Modify: `java/k8s/secrets/java-secrets.yml.template`

- [ ] **Step 1: Append the new key**

Add a new line under `data:`:

```yaml
  grafana-reader-password: Z3JhZmFuYS1yZWFkZXItc2VjcmV0   # grafana-reader-secret
```

(`echo -n 'grafana-reader-secret' | base64` → `Z3JhZmFuYS1yZWFkZXItc2VjcmV0`. The real cluster secret should use a strong random value — this template uses a placeholder.)

- [ ] **Step 2: Document in plan README**

In the same commit, add a single-line note above the existing `# echo -n 'value' | base64` comment in the template that points to this Job for the role creation:

```yaml
  # grafana_reader role created by jobs/postgres-grafana-reader.yml using this key
```

- [ ] **Step 3: Commit**

```bash
git add java/k8s/secrets/java-secrets.yml.template
git commit -m "k8s(secrets): add grafana-reader-password key to template"
```

---

## Task 7: One-shot Job — create `grafana_reader` role

**Files:**
- Create: `java/k8s/jobs/postgres-grafana-reader.yml`

- [ ] **Step 1: Write the Job manifest**

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: postgres-grafana-reader
  namespace: java-tasks
spec:
  ttlSecondsAfterFinished: 600
  backoffLimit: 3
  template:
    spec:
      restartPolicy: OnFailure
      containers:
        - name: psql
          image: postgres:17-alpine
          env:
            - name: PGPASSWORD
              valueFrom:
                secretKeyRef:
                  name: java-secrets
                  key: postgres-password
            - name: GRAFANA_READER_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: java-secrets
                  key: grafana-reader-password
          command:
            - /bin/sh
            - -c
            - |
              set -eu
              psql -h postgres.java-tasks.svc.cluster.local -U taskuser -d postgres \
                -v ON_ERROR_STOP=1 \
                -v reader_pw="$GRAFANA_READER_PASSWORD" <<'SQL'
              DO $$
              BEGIN
                IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'grafana_reader') THEN
                  EXECUTE format('CREATE ROLE grafana_reader LOGIN PASSWORD %L', :'reader_pw');
                ELSE
                  EXECUTE format('ALTER ROLE grafana_reader WITH PASSWORD %L', :'reader_pw');
                END IF;
              END
              $$;
              GRANT pg_monitor TO grafana_reader;
              GRANT CONNECT ON DATABASE productdb TO grafana_reader;
              GRANT CONNECT ON DATABASE orderdb   TO grafana_reader;
              GRANT CONNECT ON DATABASE paymentdb TO grafana_reader;
              GRANT CONNECT ON DATABASE cartdb    TO grafana_reader;
              GRANT CONNECT ON DATABASE authdb    TO grafana_reader;
              SQL
              echo "grafana_reader ready"
          resources:
            requests: { cpu: "10m", memory: "32Mi" }
            limits:   { cpu: "100m", memory: "64Mi" }
```

- [ ] **Step 2: Validate**

```bash
kubectl apply --dry-run=client -f java/k8s/jobs/postgres-grafana-reader.yml
```

- [ ] **Step 3: Commit**

```bash
git add java/k8s/jobs/postgres-grafana-reader.yml
git commit -m "k8s(postgres): bootstrap Job for grafana_reader role with pg_monitor"
```

---

## Task 8: Register new resources in kustomization

**Files:**
- Modify: `java/k8s/kustomization.yaml`

- [ ] **Step 1: Add new ConfigMap and Job entries**

Insert in the `resources:` list, alphabetically grouped:

```yaml
  - configmaps/postgres-exporter-queries.yml
```
…right after `- configmaps/postgres-initdb.yml`, and:

```yaml
  - jobs/postgres-extensions-bootstrap.yml
  - jobs/postgres-grafana-reader.yml
```
…right after `- jobs/postgres-backup.yml`.

- [ ] **Step 2: Validate kustomize build**

```bash
kustomize build java/k8s | head -50
```

Expected: produces YAML, no errors. Expect "Job" and "ConfigMap" mentions for the new resources in the output.

- [ ] **Step 3: Commit**

```bash
git add java/k8s/kustomization.yaml
git commit -m "k8s(java): register postgres exporter queries + bootstrap jobs"
```

---

## Task 9: Add three Grafana PostgreSQL data sources

**Files:**
- Modify: `k8s/monitoring/configmaps/grafana-datasource.yml`

- [ ] **Step 1: Append three datasources**

Append to the `datasources:` list (after the existing Jaeger entry, line 35):

```yaml
      - name: postgres-productdb
        type: postgres
        access: proxy
        url: postgres.java-tasks.svc.cluster.local:5432
        user: grafana_reader
        editable: false
        uid: postgres-productdb
        jsonData:
          database: productdb
          sslmode: disable
          postgresVersion: 1700
          timescaledb: false
        secureJsonData:
          password: "$__env{GRAFANA_READER_PASSWORD}"

      - name: postgres-orderdb
        type: postgres
        access: proxy
        url: postgres.java-tasks.svc.cluster.local:5432
        user: grafana_reader
        editable: false
        uid: postgres-orderdb
        jsonData:
          database: orderdb
          sslmode: disable
          postgresVersion: 1700
          timescaledb: false
        secureJsonData:
          password: "$__env{GRAFANA_READER_PASSWORD}"

      - name: postgres-paymentdb
        type: postgres
        access: proxy
        url: postgres.java-tasks.svc.cluster.local:5432
        user: grafana_reader
        editable: false
        uid: postgres-paymentdb
        jsonData:
          database: paymentdb
          sslmode: disable
          postgresVersion: 1700
          timescaledb: false
        secureJsonData:
          password: "$__env{GRAFANA_READER_PASSWORD}"
```

- [ ] **Step 2: Wire `GRAFANA_READER_PASSWORD` into the Grafana deployment**

Locate the Grafana deployment manifest:

```bash
ls k8s/monitoring/deployments/grafana.yml 2>/dev/null || find k8s/monitoring -name "grafana*.yml" -path "*deployment*"
```

Add to the Grafana container's `env:` block (next to other `$__env{...}` references like `TELEGRAM_BOT_TOKEN`):

```yaml
            - name: GRAFANA_READER_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: grafana-env
                  key: grafana-reader-password
```

If the secret doesn't exist yet, also add the key to the existing `grafana-env` Secret (or whichever secret already holds `TELEGRAM_BOT_TOKEN`). Check:

```bash
grep -rn "TELEGRAM_BOT_TOKEN" k8s/monitoring/secrets/ 2>/dev/null
```

Then add the matching template entry the same way Telegram is added.

- [ ] **Step 3: Validate**

```bash
kubectl apply --dry-run=client -f k8s/monitoring/configmaps/grafana-datasource.yml
```

- [ ] **Step 4: Commit**

```bash
git add k8s/monitoring/configmaps/grafana-datasource.yml k8s/monitoring/deployments k8s/monitoring/secrets
git commit -m "k8s(grafana): add per-DB Postgres datasources (productdb, orderdb, paymentdb)"
```

---

## Task 10: Promtail pipeline for `auto_explain` JSON

**Files:**
- Modify: `k8s/monitoring/configmaps/promtail-config.yml`

`auto_explain.log_format = json` writes plans as JSON. The existing pipeline already does a `cri:` + generic JSON parse step. Add a Postgres-specific stage that extracts `database`, `duration_ms`, and `query_id` as derived fields (not labels — high cardinality). The plan body stays in `msg` for inline rendering.

- [ ] **Step 1: Append a Postgres-specific match block**

After the closing of the existing `pipeline_stages:` (after the last `labels:` block at line 55), append:

```yaml
          - match:
              selector: '{app="postgres"}'
              stages:
                - regex:
                    expression: 'duration: (?P<duration_ms>[0-9.]+) ms.*\bplan:\s*(?P<plan_json>\{.*\})'
                - json:
                    source: plan_json
                    expressions:
                      database: '"Database"'
                      query_id: '"Query Identifier"'
                - labels:
                    database:
                - structured_metadata:
                    duration_ms:
                    query_id:
```

(The `\bplan:` alternation matches the standard `auto_explain` log format. Postgres logs the plan as `duration: <ms> ms  plan: <json>`.)

- [ ] **Step 2: Validate the YAML**

```bash
kubectl apply --dry-run=client -f k8s/monitoring/configmaps/promtail-config.yml
```

- [ ] **Step 3: Commit**

```bash
git add k8s/monitoring/configmaps/promtail-config.yml
git commit -m "k8s(promtail): extract auto_explain database + duration + query_id"
```

---

## Task 11: Add `pg-query-performance.json` dashboard

**Files:**
- Modify: `k8s/monitoring/configmaps/grafana-dashboards.yml`

Append a new dashboard key. The dashboard uses the three new PostgreSQL data sources and the existing Prometheus + Loki sources.

- [ ] **Step 1: Append the dashboard JSON**

At the end of the `data:` block (after `postgresql.json:` ends — find the last `}` followed by indentation), append:

```yaml
  pg-query-performance.json: |
    {
      "annotations": { "list": [] },
      "editable": true,
      "graphTooltip": 1,
      "title": "PostgreSQL Query Performance",
      "uid": "pg-query-performance",
      "schemaVersion": 39,
      "tags": ["postgres", "queries"],
      "time": { "from": "now-6h", "to": "now" },
      "timepicker": {},
      "templating": {
        "list": [
          {
            "name": "database",
            "type": "datasource",
            "query": "postgres",
            "current": { "text": "postgres-productdb", "value": "postgres-productdb" },
            "regex": "/^postgres-/",
            "label": "Database"
          },
          {
            "name": "queryid",
            "type": "query",
            "datasource": { "type": "postgres", "uid": "${database}" },
            "query": "SELECT queryid::text FROM pg_stat_statements WHERE calls > 10 ORDER BY mean_exec_time DESC LIMIT 25",
            "multi": true,
            "includeAll": true
          }
        ]
      },
      "panels": [
        {
          "type": "table",
          "title": "Top 10 slowest queries (mean exec time)",
          "datasource": { "type": "postgres", "uid": "${database}" },
          "gridPos": { "x": 0, "y": 0, "w": 24, "h": 8 },
          "targets": [{
            "rawSql": "SELECT LEFT(query, 200) AS query, calls, ROUND(mean_exec_time::numeric, 2) AS mean_ms, ROUND(stddev_exec_time::numeric, 2) AS stddev_ms, rows FROM pg_stat_statements WHERE calls > 10 ORDER BY mean_exec_time DESC LIMIT 10",
            "format": "table",
            "refId": "A"
          }]
        },
        {
          "type": "table",
          "title": "Top 10 by total exec time",
          "datasource": { "type": "postgres", "uid": "${database}" },
          "gridPos": { "x": 0, "y": 8, "w": 24, "h": 8 },
          "targets": [{
            "rawSql": "SELECT LEFT(query, 200) AS query, calls, ROUND(total_exec_time::numeric / 1000, 2) AS total_seconds, ROUND(mean_exec_time::numeric, 2) AS mean_ms FROM pg_stat_statements WHERE calls > 10 ORDER BY total_exec_time DESC LIMIT 10",
            "format": "table",
            "refId": "A"
          }]
        },
        {
          "type": "timeseries",
          "title": "Mean exec time per queryid (top 5)",
          "datasource": { "type": "prometheus", "uid": "PBFA97CFB590B2093" },
          "gridPos": { "x": 0, "y": 16, "w": 12, "h": 8 },
          "targets": [{
            "expr": "topk(5, pg_stat_statements_mean_exec_time)",
            "legendFormat": "{{queryid}} {{query_text}}",
            "refId": "A"
          }]
        },
        {
          "type": "timeseries",
          "title": "Slow-query call rate (mean > 500ms)",
          "datasource": { "type": "prometheus", "uid": "PBFA97CFB590B2093" },
          "gridPos": { "x": 12, "y": 16, "w": 12, "h": 8 },
          "targets": [{
            "expr": "sum by (queryid) (rate(pg_stat_statements_calls_total{queryid=~\".+\"}[5m]) and on(queryid) pg_stat_statements_mean_exec_time > 500)",
            "legendFormat": "{{queryid}}",
            "refId": "A"
          }]
        },
        {
          "type": "table",
          "title": "Cache hit ratio per top query",
          "datasource": { "type": "postgres", "uid": "${database}" },
          "gridPos": { "x": 0, "y": 24, "w": 24, "h": 8 },
          "targets": [{
            "rawSql": "SELECT LEFT(query, 200) AS query, shared_blks_hit, shared_blks_read, CASE WHEN (shared_blks_hit + shared_blks_read) > 0 THEN ROUND((shared_blks_hit::numeric / (shared_blks_hit + shared_blks_read)) * 100, 2) ELSE NULL END AS hit_pct FROM pg_stat_statements WHERE calls > 10 ORDER BY shared_blks_read DESC LIMIT 10",
            "format": "table",
            "refId": "A"
          }]
        },
        {
          "type": "logs",
          "title": "Recent slow plans (auto_explain, last 1h)",
          "datasource": { "type": "loki", "uid": "loki" },
          "gridPos": { "x": 0, "y": 32, "w": 24, "h": 10 },
          "targets": [{
            "expr": "{namespace=\"java-tasks\", app=\"postgres\"} |= \"auto_explain\" |= \"plan:\"",
            "refId": "A"
          }]
        },
        {
          "type": "logs",
          "title": "Plan viewer (filter by queryid)",
          "datasource": { "type": "loki", "uid": "loki" },
          "gridPos": { "x": 0, "y": 42, "w": 24, "h": 10 },
          "targets": [{
            "expr": "{namespace=\"java-tasks\", app=\"postgres\"} |= \"auto_explain\" | json | query_id=~\"$queryid\"",
            "refId": "A"
          }]
        }
      ]
    }
```

- [ ] **Step 2: Validate JSON inside the YAML**

```bash
python3 -c '
import yaml, json, sys
with open("k8s/monitoring/configmaps/grafana-dashboards.yml") as f:
    cm = yaml.safe_load(f)
for k, v in cm["data"].items():
    json.loads(v)
    print(f"  ok: {k}")
'
```

Expected: every dashboard key prints `ok:`.

- [ ] **Step 3: Validate the ConfigMap**

```bash
kubectl apply --dry-run=client -f k8s/monitoring/configmaps/grafana-dashboards.yml
```

- [ ] **Step 4: Commit**

```bash
git add k8s/monitoring/configmaps/grafana-dashboards.yml
git commit -m "grafana: add PostgreSQL Query Performance dashboard"
```

---

## Task 12: Append four alert rules to the PostgreSQL group

**Files:**
- Modify: `k8s/monitoring/configmaps/grafana-alerting.yml`

Append at the end of the existing `PostgreSQL` group (after `pg-backup-stale` finishes, before the closing of the `groups:` list — locate by searching for the rule following `pg-backup-stale`).

- [ ] **Step 1: Append the four new rules**

Find the last rule in the `PostgreSQL` group (the alerting file's `name: PostgreSQL` block, around line 1391+). Append these four rules at the same indentation level as the existing rules in that group:

```yaml
          - uid: pg-query-mean-exec-time-high
            title: Postgres Query Mean Exec Time High
            noDataState: OK
            condition: C
            data:
              - refId: A
                relativeTimeRange: { from: 600, to: 0 }
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: max(pg_stat_statements_mean_exec_time) by (queryid, query_text)
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
                    - evaluator: { type: gt, params: [1000] }
                  refId: C
            for: 10m
            labels:
              severity: warning
            annotations:
              summary: "Postgres query mean exec time > 1s — {{ $labels.query_text }}"

          - uid: pg-query-mean-regression
            title: Postgres Query Regression vs 7-day Baseline
            noDataState: OK
            condition: C
            data:
              - refId: A
                relativeTimeRange: { from: 900, to: 0 }
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: >-
                    (max(pg_stat_statements_mean_exec_time) by (queryid))
                    /
                    (max(avg_over_time(pg_stat_statements_mean_exec_time[7d])) by (queryid))
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange: { from: 900, to: 0 }
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange: { from: 900, to: 0 }
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator: { type: gt, params: [2] }
                  refId: C
            for: 15m
            labels:
              severity: warning
            annotations:
              summary: "Postgres query mean is >2× its 7-day baseline (queryid {{ $labels.queryid }})"

          - uid: pg-slow-query-rate-spike
            title: Postgres Slow Query Rate Spike
            noDataState: OK
            condition: C
            data:
              - refId: A
                relativeTimeRange: { from: 600, to: 0 }
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: >-
                    sum(rate(pg_stat_statements_calls_total[5m])
                      and on(queryid) pg_stat_statements_mean_exec_time > 500)
                    /
                    sum(avg_over_time(rate(pg_stat_statements_calls_total[5m])[1h:5m])
                      and on(queryid) pg_stat_statements_mean_exec_time > 500)
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
                    - evaluator: { type: gt, params: [3] }
                  refId: C
            for: 10m
            labels:
              severity: warning
            annotations:
              summary: "Slow-query call rate is >3× the 1h baseline"

          - uid: pg-auto-explain-stalled
            title: Postgres auto_explain Plans Not Flowing
            noDataState: OK
            condition: C
            data:
              - refId: A
                relativeTimeRange: { from: 86400, to: 0 }
                datasourceUid: loki
                model:
                  expr: 'sum(count_over_time({namespace="java-tasks", app="postgres"} |= "auto_explain" [24h]))'
                  refId: A
              - refId: B
                relativeTimeRange: { from: 86400, to: 0 }
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange: { from: 86400, to: 0 }
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator: { type: lt, params: [1] }
                  refId: C
            for: 5m
            labels:
              severity: warning
            annotations:
              summary: "No auto_explain log lines in 24h — query observability is silently broken"
```

- [ ] **Step 2: Validate the YAML**

```bash
python3 -c '
import yaml
with open("k8s/monitoring/configmaps/grafana-alerting.yml") as f:
    cm = yaml.safe_load(f)
y = yaml.safe_load(cm["data"]["alerting.yml"])
groups = y["groups"]
pg = next(g for g in groups if g["name"] == "PostgreSQL")
new = {"pg-query-mean-exec-time-high","pg-query-mean-regression","pg-slow-query-rate-spike","pg-auto-explain-stalled"}
have = {r["uid"] for r in pg["rules"]} & new
assert have == new, f"missing: {new - have}"
print("ok: 4 new rules present")
'
```

Expected: `ok: 4 new rules present`.

- [ ] **Step 3: Validate the ConfigMap**

```bash
kubectl apply --dry-run=client -f k8s/monitoring/configmaps/grafana-alerting.yml
```

- [ ] **Step 4: Commit**

```bash
git add k8s/monitoring/configmaps/grafana-alerting.yml
git commit -m "grafana(alerts): add 4 query-observability rules (latency, regression, spike, stalled)"
```

---

## Task 13: Companion ADR

**Files:**
- Create: `docs/adr/observability/2026-04-27-pg-query-observability.md`

- [ ] **Step 1: Write the ADR**

```markdown
# ADR: PostgreSQL Query Observability (2026-04-27)

## Status
Accepted

## Context
The shared PostgreSQL 17 instance had system-level observability (connections, cache hit, deadlocks, backup freshness) but no view into individual query performance. This is the measurement layer needed for further optimization or replication work, and a baseline backend-engineer skill ("how do you find slow queries?").

Spec: `docs/superpowers/specs/2026-04-27-pg-query-observability-design.md`. Plan: `docs/superpowers/plans/2026-04-27-pg-query-observability.md`.

## Decisions

### `pg_stat_statements` + `auto_explain` preloaded via `args:`
The vanilla `postgres:17-alpine` image is left intact. Startup `-c` flags via `args:` overlay defaults rather than replacing `postgresql.conf`. This avoids drift from upstream defaults and keeps the diff minimal.

### Custom queries in `postgres_exporter`
Two custom queries (latency + IO) export the top-50 entries from `pg_stat_statements`. Cardinality is bounded by `LIMIT 50` and the `WHERE calls > 10` filter; `query_text` is truncated to 200 chars to keep label storage manageable.

### `pg_monitor` predefined role for the Grafana data source
Preferred over hand-rolled GRANTs because it tracks upstream when new monitoring views ship. The `grafana_reader` role gets `pg_monitor` plus per-DB `CONNECT`.

### Per-database Grafana data sources
`pg_stat_statements` is per-database. At three high-traffic DBs the per-DB datasource pattern is simpler than a `monitoring` DB with `postgres_fdw`. The dashboard's `Database` template variable handles switching.

### `auto_explain.log_format = json` to Loki
Plans flow through the existing Postgres → Promtail → Loki path. The JSON format makes them parseable by the Promtail pipeline and renders cleanly in Grafana logs panels.

### Regression alert against 7-day baseline
Hard latency thresholds miss the realistic failure mode — a query that quietly drifts from 50ms to 200ms after a planner change. The regression rule (`current / 7d-avg > 2`) catches that, while the hard `> 1s` rule catches genuinely terrible queries.

### `noDataState: OK`
Applied per the project-wide pattern (see `2026-04-24-postgres-data-integrity.md`). For rate-based and event-based metrics, no data means no activity, which means no problem.

## Consequences

**Positive:**
- Production query performance is observable for the first time — both real-time tables and longitudinal trend lines.
- Plan regressions become visible as alerts, not user complaints.
- The remaining nine `db-roadmap` items (#155–#163) all depend on this measurement layer.

**Trade-offs:**
- `shared_preload_libraries` change requires a Postgres restart. Acceptable given the existing `Recreate` posture.
- `auto_explain.log_analyze = true` adds modest planner overhead. Negligible at portfolio scale; sample rate would be lowered at higher load.
- Per-DB Grafana datasources don't give a single cluster-wide view. Acceptable — the dashboard variable handles switching.

**Phase 2 (future):**
- Trim `pg_stat_statements` periodically (`pg_stat_statements_reset()`) on a CronJob so old query plans don't crowd out current ones.
- Track plan stability via `pg_stat_statements.toplevel` once we have a year of data to compare.
```

- [ ] **Step 2: Commit**

```bash
git add docs/adr/observability/2026-04-27-pg-query-observability.md
git commit -m "docs: ADR for postgres query observability (pg_stat_statements + auto_explain)"
```

---

## Task 14: Open the PR

After all prior tasks are committed:

- [ ] **Step 1: Push the branch**

```bash
git push -u origin agent/feat-pg-query-observability
```

- [ ] **Step 2: Open PR to `qa`**

```bash
gh pr create --base qa --title "feat: pg_stat_statements + auto_explain query observability (db-roadmap 1/10)" --body "$(cat <<'EOF'
## Summary
- Enable `pg_stat_statements` and `auto_explain` on the shared Postgres
- Export top-50 query metrics via `postgres_exporter` custom queries
- Add `grafana_reader` (`pg_monitor`) role + per-DB Grafana PostgreSQL datasources for live SQL inspection
- Promtail pipeline parses `auto_explain` JSON plans into Loki
- New `PostgreSQL Query Performance` Grafana dashboard
- 4 new alerts: latency ceiling, 7-day regression, slow-query rate spike, auto_explain stalled
- Companion ADR + integration test (testcontainers)

First item on the **db-roadmap** label (issues #155–#163). Spec at `docs/superpowers/specs/2026-04-27-pg-query-observability-design.md`. Plan at `docs/superpowers/plans/2026-04-27-pg-query-observability.md`.

## Test plan
- [ ] `make preflight-go` passes
- [ ] `cd go/pkg && go test -tags integration -run TestPgStatStatementsAndAutoExplain ./db/...` passes locally
- [ ] `kustomize build java/k8s` succeeds
- [ ] `kustomize build k8s/monitoring` succeeds
- [ ] In QA, run a slow product search; within 5 min the query appears in the top-N table panel
- [ ] In QA, the `Recent slow plans` Loki panel shows JSON `auto_explain` output

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 3: Stop here**

Per project rules: do NOT watch CI. Notify Kyle that the PR is open with its URL and stop.

---

## Self-Review Notes

**Spec coverage:** Every section of the spec maps to a task —
- *PostgreSQL configuration* → Task 4
- *Extensions per database* → Tasks 3 (init script) + 5 (bootstrap Job)
- *Metrics layer* → Task 2
- *Inspection layer* → Tasks 6, 7, 9
- *Plan capture* → Task 10
- *Dashboard* → Task 11
- *Alerts* → Task 12
- *Testing* → Task 1
- *Rollout* → kustomize wiring in Task 8 + PR in Task 14
- *ADR* → Task 13

**Type / property consistency:** All references to `grafana_reader`, `grafana-reader-password`, `GRAFANA_READER_PASSWORD`, `pg_monitor`, and the three datasource UIDs (`postgres-productdb`, `postgres-orderdb`, `postgres-paymentdb`) are consistent across tasks 6, 7, 9, 11.

**Open items handed to the executor:**
- The Grafana deployment's existing secret name for `TELEGRAM_BOT_TOKEN` must be inspected at Task 9 step 2 to wire `GRAFANA_READER_PASSWORD` into the same secret. The plan acknowledges this and provides the discovery command.
