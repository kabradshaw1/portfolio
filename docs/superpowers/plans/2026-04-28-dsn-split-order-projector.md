# Phase 4 — DSN component split: order-projector (first Go service)

Spec: [`docs/superpowers/specs/2026-04-28-secrets-management-design.md`](../specs/2026-04-28-secrets-management-design.md), Phase 4.

## Why order-projector first

Spec says "least-to-most central, auth-service last." Order-projector is the read-side projector consuming Kafka events into a separate `projectordb` — if anything is wrong with the new DSN-assembly pattern, the only user-visible effect is "the projection lags," not "the saga breaks" or "users can't log in." Best place to land the pattern, then repeat it five more times.

## The pattern this PR establishes

For each Go service:

1. **ConfigMap holds connection geometry only.** Replace `DATABASE_URL` and `DATABASE_URL_DIRECT` with:
   - `DB_HOST` — pgbouncer host
   - `DB_PORT` — pgbouncer port (`6432`)
   - `DB_NAME` — application database
   - `DB_OPTIONS` — `sslmode=disable&application_name=<service>` and any other URL-query options
   - `DB_HOST_DIRECT` — postgres host (PgBouncer bypass)
   - `DB_PORT_DIRECT` — postgres port (`5432`)
   - `DB_OPTIONS_DIRECT` — same shape with `application_name=<service>-migrate`
2. **Per-service `<service>-db` SealedSecret carries credentials.** Two keys: `DB_USER`, `DB_PASSWORD`. Same Sealed Secrets workflow as Phase 2/3 — patch live, annotate `managed: true`, run `seal-from-cluster.sh`, commit.
3. **App config builds the DSN at startup** from the components. Existing `pgxpool.ParseConfig` consumer doesn't change; it just gets a DSN string assembled from component pieces instead of read whole.
4. **Migration Job builds the DSN in its shell command** from the same components, using the `_DIRECT` ones for PgBouncer bypass.
5. **QA overlay overrides only `DB_NAME`** (e.g., `projectordb` → `projectordb_qa`). Every other component is shared with prod (we use the same Postgres). Credentials live in QA's own per-service Secret in `go-ecommerce-qa`.
6. **R3 allowlist shrinks** as each service migrates — remove the file from `scripts/k8s-policy-check.sh`'s `R3_ALLOWLIST`.

Subsequent Phase 4 PRs (cart, order, payment, product, then auth-service last) re-run this recipe.

## Out of scope

- Per-service Postgres roles (least-privilege). All services still authenticate as `taskuser`. The component split is what unblocks per-user creds later — once `DB_USER`/`DB_PASSWORD` come from a per-service Secret, swapping in `projector_user` is a one-Secret change.
- RabbitMQ DSN split (`MQ_HOST`, `MQ_USER`, etc.). Separate roll-up; the spec mentions it but order-projector doesn't use RabbitMQ today (it consumes Kafka).
- Redis DSN split. Redis has no password today; defer until it grows one.

## Implementation steps

### 1. Bring up a per-service live Secret (operator workstation)

```bash
# prod
ssh debian "kubectl create secret generic order-projector-db -n go-ecommerce \
  --from-literal=DB_USER=taskuser \
  --from-literal=DB_PASSWORD=taskpass \
  --dry-run=client -o yaml | kubectl apply -f -"
ssh debian "kubectl annotate secret order-projector-db -n go-ecommerce sealedsecrets.bitnami.com/managed=true --overwrite"

# qa (separate sealed file because Strict scope)
ssh debian "kubectl create secret generic order-projector-db -n go-ecommerce-qa \
  --from-literal=DB_USER=taskuser \
  --from-literal=DB_PASSWORD=taskpass \
  --dry-run=client -o yaml | kubectl apply -f -"
ssh debian "kubectl annotate secret order-projector-db -n go-ecommerce-qa sealedsecrets.bitnami.com/managed=true --overwrite"
```

### 2. Add to seal script + seal both

```
SECRETS+=(
  "go-ecommerce/order-projector-db"
  "go-ecommerce-qa/order-projector-db"
)
scripts/seal-from-cluster.sh order-projector-db
```

This writes:
- `k8s/secrets/go-ecommerce/order-projector-db.sealed.yml`
- `k8s/secrets/go-ecommerce-qa/order-projector-db.sealed.yml`

If `k8s/secrets/go-ecommerce-qa/` doesn't exist yet, add a `kustomization.yaml` for it and add `go-ecommerce-qa` to `k8s/secrets/kustomization.yaml`'s `resources`. (Phase 3 already did the `java-tasks-qa` analogue — same shape.)

### 3. Replace ConfigMap DSN with components

`go/k8s/configmaps/order-projector-config.yml`:

```yaml
data:
  DB_HOST: pgbouncer.java-tasks.svc.cluster.local
  DB_PORT: "6432"
  DB_NAME: projectordb
  DB_OPTIONS: sslmode=disable&application_name=order-projector
  DB_HOST_DIRECT: postgres.java-tasks.svc.cluster.local
  DB_PORT_DIRECT: "5432"
  DB_OPTIONS_DIRECT: sslmode=disable&application_name=order-projector-migrate
  KAFKA_BROKERS: kafka.go-ecommerce.svc.cluster.local:9092
  PORT: "8097"
  ALLOWED_ORIGINS: http://localhost:3000,https://kylebradshaw.dev
  OTEL_EXPORTER_OTLP_ENDPOINT: jaeger.monitoring.svc.cluster.local:4317
```

### 4. Assemble DSN in `cmd/server/config.go`

Read the six components, `fmt.Sprintf` the `postgres://...` string, return it through the existing `Config.DatabaseURL` field so `main.go`'s `pgxpool.ParseConfig` consumer doesn't change.

`url.QueryEscape` the user/password to be safe — base64 passwords don't have URL-special characters today, but the assembly should be defensive.

### 5. Update Deployment env

`go/k8s/deployments/order-projector.yml`:

```yaml
envFrom:
  - configMapRef:
      name: order-projector-config
  - secretRef:
      name: order-projector-db
```

`envFrom` exposes every ConfigMap key (DB_HOST, DB_PORT, etc.) and Secret key (DB_USER, DB_PASSWORD) as individual env vars. Cleaner than enumerating each.

### 6. Update Migration Job

`go/k8s/jobs/order-projector-migrate.yml`:

```yaml
envFrom:
  - configMapRef:
      name: order-projector-config
  - secretRef:
      name: order-projector-db
command:
  - /bin/sh
  - -c
args:
  - |
    set -e
    DATABASE_URL="postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST_DIRECT}:${DB_PORT_DIRECT}/${DB_NAME}?${DB_OPTIONS_DIRECT}"
    echo "Running order-projector migrations..."
    /usr/local/bin/migrate -path=/migrations -database="${DATABASE_URL}&x-migrations-table=projector_schema_migrations" up
    echo "Done."
```

### 7. Update QA overlay

In `k8s/overlays/qa-go/kustomization.yaml`, replace the `DATABASE_URL` / `DATABASE_URL_DIRECT` patches for `order-projector-config` with one `replace` op on `/data/DB_NAME` → `projectordb_qa`. Other component paths are shared with prod and stay untouched.

### 8. Remove from R3 allowlist

`scripts/k8s-policy-check.sh`: drop `go/k8s/configmaps/order-projector-config.yml` from `R3_ALLOWLIST`. Run `bash scripts/k8s-policy-check.sh` to confirm the remaining allowlist still tolerates the other five services.

## Verification

- `make preflight-go` (lint + tests).
- `bash scripts/test-k8s-policy-check.sh` (the 8 fixture tests still pass).
- `bash scripts/k8s-policy-check.sh` (R3 reports no new violations; the projector ConfigMap is now policy-clean).
- Apply the new SealedSecret(s) directly to the cluster (the controller picks up the new resources without a full deploy).
- After PR merges to qa: confirm the migration Job runs to Complete on the next deploy, and `kubectl logs deploy/order-projector` shows a clean startup with `pgxpool` connecting.

## Risks

- **DSN with special-char password.** `taskpass` is alphanumeric, but the assembly should use `url.QueryEscape` so a future password rotation with `+/=` works without breaking parsing. Done in step 4.
- **Forgot `_DIRECT` for migration Job.** Bypassing PgBouncer is required for golang-migrate's session-level features (CLAUDE.md note). The migration Job manifest in step 6 explicitly uses `DB_HOST_DIRECT`/`DB_PORT_DIRECT`/`DB_OPTIONS_DIRECT`.
- **Strict-scope SealedSecret applied to wrong namespace.** Each ns has its own sealed file. Don't kustomize-rewrite the namespace on a sealed secret — it will refuse to decrypt.
- **QA pointing at prod's DB_NAME.** The single QA overlay patch on `/data/DB_NAME` is the load-bearing line; if it's missing or typo'd, QA writes into prod's `projectordb`. Mitigated by a manual `kubectl describe configmap order-projector-config -n go-ecommerce-qa` after the QA deploy.
