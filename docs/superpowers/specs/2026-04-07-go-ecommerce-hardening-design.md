# Go ecommerce hardening

**Date:** 2026-04-07
**Branch:** `go-ecommerce-hardening`
**Status:** Approved, ready for implementation plan

## Background

A production outage on `https://kylebradshaw.dev/go/ecommerce` exposed three
weaknesses in the Go stack:

1. The shared `java-tasks` postgres runs on an `emptyDir` volume. Every pod
   restart wipes data, including the `ecommercedb` database used by the Go
   services. A recent fix added an `initdb` ConfigMap to auto-create the
   database on restart, but nothing applies the SQL migration files in
   `go/{auth,ecommerce}-service/migrations/` — they are dead code in the repo.
2. Schema and seed data are currently embedded in the `postgres-initdb`
   ConfigMap as a workaround. That couples the database image to specific
   service schemas, and any schema change requires hand-editing the ConfigMap
   and restarting postgres.
3. CI smoke tests only hit Java and AI endpoints
   (`https://api.kylebradshaw.dev`). Go endpoints receive no coverage, so
   broken deploys ship silently. All three bugs found during the 2026-04-07
   incident (missing ingress `rewrite-target`, empty `ecommercedb`, NULL
   `image_url` scan failure) would have been caught by a trivial product-list
   smoke assertion.

## Goals

- Postgres data persists across pod restarts.
- Schema changes on the Go side use a real, versioned migration tool instead
  of hand-edited ConfigMaps.
- Each Go service's migrations live with its code and version-lock to its
  container image.
- Post-deploy CI fails when the Go store is broken in production.

## Non-goals

- Moving postgres to a dedicated namespace or out of the `java-tasks`
  namespace.
- Adding Flyway to the Java side (Java already uses Spring-managed schemas).
- Adding migrations to the Python AI services.
- Running a full auth round-trip in smoke tests (register → login → logout).
  Login against a seeded smoke user is sufficient coverage for the portfolio.
- Introducing a separate migration-only service account or RBAC isolation.
  The existing `go-secrets` credentials are reused.

## Architecture

Three coordinated changes ship together on one branch. They depend on each
other: once postgres has a PVC, `initdb` scripts only run on the *first*
boot, so the schema-creation blocks currently in `postgres-initdb.yml`
become dead code. The migration Jobs become the authoritative source of
schema, and the ConfigMap shrinks to just `CREATE DATABASE`.

```
postgres (PVC) ──────────┐
                         │
                         ▼
   ┌─── initdb ──> CREATE DATABASE ecommercedb  (runs once, on fresh PVC)
   │
   ├─── go-auth-migrate Job ──> schema + indexes (runs every deploy, idempotent)
   │
   └─── go-ecommerce-migrate Job ──> schema + seed.sql (runs every deploy, idempotent)
                                        │
                                        ▼
                                   products, smoke user
```

## Component 1 — postgres PVC

**Files:**
- `java/k8s/volumes/postgres-pvc.yml` (new)
- `java/k8s/deployments/postgres.yml` (modified)
- `java/k8s/configmaps/postgres-initdb.yml` (modified)

**PVC:** `postgres-data`, namespace `java-tasks`, `ReadWriteOnce`, `2Gi`,
storage class `standard` (Minikube default).

**Mount:** `/var/lib/postgresql/data` in the postgres deployment. The
existing `initdb` ConfigMap mount at `/docker-entrypoint-initdb.d` stays.

**`postgres-initdb.yml` reduction:** strip back to a single file that only
creates the database. The schema blocks for `users`, `products`, `cart_items`,
`orders`, `order_items` move out (they're owned by migration Jobs now) and
so does the seed INSERT (it moves to `go/ecommerce-service/seed.sql`):

```yaml
data:
  01-create-ecommercedb.sql: |
    CREATE DATABASE ecommercedb OWNER taskuser;
```

**Migration caveat:** applying the PVC will wipe the current emptyDir
contents one last time. All current data is seed or dev throwaway, so this
is acceptable. The next deploy's migration Jobs will repopulate.

## Component 2 — migration Jobs (`golang-migrate`)

**Per-service changes (`go/auth-service/`, `go/ecommerce-service/`):**

### Migration files

Rename existing SQL files to `golang-migrate` strict format with matching
`down` migrations:

**auth-service:**
- `migrations/001_create_users.sql` → `001_create_users.up.sql`
- New: `001_create_users.down.sql` (`DROP TABLE users`)
- `migrations/002_google_oauth.sql` → `002_google_oauth.up.sql`
- New: `002_google_oauth.down.sql` (reverse ALTER/ADD)

**ecommerce-service:**
- `migrations/001_create_products.sql` → `001_create_products.up.sql`
  (schema only — seed INSERT extracted)
- New: `001_create_products.down.sql` (`DROP TABLE products`)
- `migrations/002_create_cart_items.sql` → `002_create_cart_items.up.sql`
- New: `002_create_cart_items.down.sql`
- `migrations/003_create_orders.sql` → `003_create_orders.up.sql`
- New: `003_create_orders.down.sql`

### Seed file (ecommerce-service only)

**`go/ecommerce-service/seed.sql`** — separate from migrations, run after
`migrate up` succeeds. Idempotent via `WHERE NOT EXISTS`:

- Product catalog (20 rows, same data as the current `postgres-initdb.yml`
  ConfigMap, with `image_url = ''` not NULL)
- Smoke test user: `smoke@kylebradshaw.dev`, bcrypt hash of
  `$SMOKE_GO_PASSWORD` baked in as a string literal

The smoke user lives in the `users` table owned by the auth-service schema.
The ecommerce-service Job writes into a table the auth-service owns, which
is a minor cross-service coupling. We accept it because splitting the user
seed into its own Job adds complexity without value — both services share
the same database and the same user table.

### Dockerfile changes

Add a `migrate` binary stage to both service Dockerfiles:

```dockerfile
FROM migrate/migrate:v4.17.0 AS migrate

FROM golang:1.26-alpine AS builder
# ... existing build ...

FROM alpine:3.19
RUN apk add --no-cache postgresql-client
COPY --from=builder /ecommerce-service /ecommerce-service
COPY --from=migrate /usr/local/bin/migrate /usr/local/bin/migrate
COPY migrations/ /migrations/
COPY seed.sql /seed.sql   # ecommerce-service only
# ... existing USER, EXPOSE, ENTRYPOINT ...
```

`postgresql-client` is needed for `seed.sql` (ecommerce-service); auth-service
can skip it but we add it to both for symmetry. Image size impact: ~15MB
(migrate binary + psql client).

### Kubernetes Jobs

**`go/k8s/jobs/auth-service-migrate.yml`** and
**`go/k8s/jobs/ecommerce-service-migrate.yml`**:

- Namespace: `go-ecommerce`
- `backoffLimit: 2`
- `activeDeadlineSeconds: 120`
- `ttlSecondsAfterFinished: 600` (auto-cleanup)
- Image: same as the service (`ghcr.io/kabradshaw1/go-{auth,ecommerce}-service:<tag>`)
- `DATABASE_URL` from existing `go-secrets`
- Entrypoint overrides:
  - auth-service: `migrate -path /migrations -database "$DATABASE_URL" up`
  - ecommerce-service:
    `sh -c "migrate -path /migrations -database \"$DATABASE_URL\" up && psql \"$DATABASE_URL\" -f /seed.sql"`

### Deploy orchestration (`ci.yml`)

The deploy step SSHs to the Windows PC and runs `kubectl apply`. Extend it
to handle the migration Jobs. In the SSH block, before applying the Go
deployments:

```bash
# Jobs are immutable once created; delete and recreate per deploy.
kubectl delete job go-auth-migrate go-ecommerce-migrate \
  -n go-ecommerce --ignore-not-found --wait=true

kubectl apply -f go/k8s/jobs/

# Wait for both to complete. Fail the deploy if either fails.
kubectl wait --for=condition=complete --timeout=120s \
  job/go-auth-migrate job/go-ecommerce-migrate -n go-ecommerce || {
    echo "Migration job failed. Logs:"
    kubectl logs job/go-auth-migrate -n go-ecommerce || true
    kubectl logs job/go-ecommerce-migrate -n go-ecommerce || true
    exit 1
  }

# Only after migrations succeed, roll out the services.
kubectl apply -f go/k8s/deployments/ go/k8s/services/ go/k8s/ingress.yml
```

## Component 3 — Go smoke tests

**File:** `.github/workflows/ci.yml` (modified, post-deploy smoke step
around line ~556).

**New secret:** `SMOKE_GO_PASSWORD` (GitHub Actions repo secret). Plaintext
in the workflow, bcrypt hash in `seed.sql`.

**Assertions added to the existing smoke step:**

```bash
# Go product catalog loads and is non-empty
PRODUCTS=$(curl -sf "$SMOKE_API_URL/go-api/products")
echo "$PRODUCTS" | jq -e '.products | length > 0' > /dev/null

# Go categories loads and is non-empty
CATEGORIES=$(curl -sf "$SMOKE_API_URL/go-api/categories")
echo "$CATEGORIES" | jq -e '.categories | length > 0' > /dev/null

# Smoke user can log in and receives an access token
LOGIN=$(curl -sf -X POST "$SMOKE_API_URL/go-auth/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"smoke@kylebradshaw.dev\",\"password\":\"$SMOKE_GO_PASSWORD\"}")
echo "$LOGIN" | jq -e '.accessToken' > /dev/null
```

`curl -sf` fails on non-2xx. `jq -e` fails on missing/null fields. Any
failure fails the step and the workflow.

The assertions cover every bug surfaced during the 2026-04-07 incident:
- Ingress routing broken → products 404 → smoke fail
- `ecommercedb` empty → products 500 → smoke fail
- NULL `image_url` scan error → products 500 → smoke fail
- Missing `users` table → login fail → smoke fail

## Error handling

- **Migration Job failure:** CI deploy step fails, logs surface in GitHub
  Actions output, deployment rollout is skipped (prior version keeps
  serving traffic).
- **PVC provisioning failure:** postgres pod stays in `Pending`. Deploy
  step fails on the existing `rollout status` check.
- **Smoke test failure:** existing smoke step mechanism already marks the
  workflow red; no change needed.
- **Bcrypt hash mismatch:** smoke user can't log in. Caught by the login
  assertion. Fix by regenerating the hash locally and updating `seed.sql`.

## Testing

- **Local sanity check:** build both Go service images locally
  (`docker build`), run them against the docker-compose postgres, exec into
  the container to verify `migrate -path /migrations ...` runs and
  `seed.sql` applies cleanly against a fresh database.
- **Staging CI:** the mocked Playwright E2E tests on `staging` don't hit
  production endpoints, so they won't catch anything here. The validation
  signal is the production smoke tests after merge to `main`.
- **Manual post-deploy verification:**
  - `kubectl exec postgres -- psql -U taskuser -d ecommercedb -c "\dt"`
    — confirm all tables present
  - `curl https://api.kylebradshaw.dev/go-api/products` — confirm 200 +
    non-empty
  - Load `https://kylebradshaw.dev/go/ecommerce` in a browser, log in as
    `smoke@kylebradshaw.dev`, confirm the catalog renders.

## Documentation

Add a short note to `CLAUDE.md` under a new "Migrations" subsection
explaining:
- Go services use `golang-migrate` via a per-service Kubernetes Job.
- Migration files live in `go/{service}/migrations/` and use the strict
  up/down format.
- Schema changes require adding a new `NNN_name.up.sql` +
  `NNN_name.down.sql` pair — the Job runs automatically on deploy.
- The `postgres-initdb` ConfigMap only creates databases, never schemas.

## File-level change summary

**Modified:**
- `java/k8s/deployments/postgres.yml`
- `java/k8s/configmaps/postgres-initdb.yml`
- `go/auth-service/Dockerfile`
- `go/ecommerce-service/Dockerfile`
- `go/auth-service/migrations/001_create_users.sql` → `.up.sql`
- `go/auth-service/migrations/002_google_oauth.sql` → `.up.sql`
- `go/ecommerce-service/migrations/001_create_products.sql` → `.up.sql`
  (seed INSERT removed)
- `go/ecommerce-service/migrations/002_create_cart_items.sql` → `.up.sql`
- `go/ecommerce-service/migrations/003_create_orders.sql` → `.up.sql`
- `.github/workflows/ci.yml`
- `CLAUDE.md`

**New:**
- `java/k8s/volumes/postgres-pvc.yml`
- `go/auth-service/migrations/001_create_users.down.sql`
- `go/auth-service/migrations/002_google_oauth.down.sql`
- `go/ecommerce-service/migrations/001_create_products.down.sql`
- `go/ecommerce-service/migrations/002_create_cart_items.down.sql`
- `go/ecommerce-service/migrations/003_create_orders.down.sql`
- `go/ecommerce-service/seed.sql`
- `go/k8s/jobs/auth-service-migrate.yml`
- `go/k8s/jobs/ecommerce-service-migrate.yml`

**Manual (Kyle):**
- Add `SMOKE_GO_PASSWORD` to GitHub Actions repo secrets
- Generate bcrypt hash for the smoke user password and paste into
  `seed.sql` before first deploy
