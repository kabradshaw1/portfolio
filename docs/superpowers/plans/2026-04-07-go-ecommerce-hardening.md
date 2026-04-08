# Go ecommerce hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Harden the Go ecommerce stack so that postgres data persists across restarts, schema changes use a proper versioned migration runner (`golang-migrate`) in Kubernetes Jobs, and CI smoke tests catch broken Go deploys before they ship.

**Architecture:** Add a PersistentVolumeClaim to the shared `java-tasks` postgres. Introduce per-service Kubernetes Jobs that run `golang-migrate` over each Go service's `migrations/` directory, baked into the service image. Extract seed data (including a smoke-test user) into a separate `seed.sql` run after the ecommerce-service migration Job. Extend the existing Playwright production smoke suite with Go store assertions: products load, smoke user can log in.

**Tech Stack:** Kubernetes (Minikube), postgres 17, `golang-migrate/migrate` v4, Go 1.26, Alpine-based Docker images, GitHub Actions, Playwright smoke tests.

**Spec:** `docs/superpowers/specs/2026-04-07-go-ecommerce-hardening-design.md`

**Branch:** `go-ecommerce-hardening` (already checked out)

---

## Pre-flight: context the implementer needs

Before touching files, understand the current state:

- Postgres runs in `java-tasks` namespace on an implicit emptyDir (no `volumes` for data). Data is wiped on every pod restart. See `java/k8s/deployments/postgres.yml`.
- Both Go services (`go-auth-service`, `go-ecommerce-service`) run in the `go-ecommerce` namespace and get their `DATABASE_URL` from per-service ConfigMaps (`go/k8s/configmaps/auth-service-config.yml`, `go/k8s/configmaps/ecommerce-service-config.yml`). Both point at `postgres://taskuser:taskpass@postgres.java-tasks.svc.cluster.local:5432/ecommercedb`.
- The `postgres-initdb` ConfigMap (`java/k8s/configmaps/postgres-initdb.yml`) currently contains schema blocks AND seed data for the Go services as a workaround — that's what we're replacing.
- Go auth-service uses `golang.org/x/crypto/bcrypt` with `bcrypt.DefaultCost` (cost 10). Bcrypt hashes for the smoke user must be generated at this cost.
- Migrations directory today:
  - `go/auth-service/migrations/001_create_users.sql`
  - `go/auth-service/migrations/002_google_oauth.sql`
  - `go/ecommerce-service/migrations/001_create_products.sql` (contains seed INSERT — needs extraction)
  - `go/ecommerce-service/migrations/002_create_cart_items.sql`
  - `go/ecommerce-service/migrations/003_create_orders.sql`
- CI deploy step lives in `.github/workflows/ci.yml` around line 485. Production smoke tests are Playwright at `frontend/e2e/smoke.spec.ts`, run in the `smoke-production` job around line 526.

**One-time manual prerequisite (Kyle does this before merging):**

1. Generate a bcrypt hash (cost 10) for a smoke-test password:
   ```bash
   python3 -c "import bcrypt; print(bcrypt.hashpw(b'REPLACE_WITH_SMOKE_PASSWORD', bcrypt.gensalt(10)).decode())"
   ```
   Example output: `$2b$10$K9...`
2. Add `SMOKE_GO_PASSWORD` to GitHub Actions repo secrets with the plaintext password.
3. Hand the hash string to the implementer for Task 8 (it gets baked into `seed.sql`).

Until Kyle provides the hash, Task 8 uses the literal placeholder `__SMOKE_BCRYPT_HASH__`. Grep for that placeholder before merging to main.

---

## Task 1: Create the postgres PVC manifest

**Files:**
- Create: `java/k8s/volumes/postgres-pvc.yml`

- [ ] **Step 1: Create the volumes directory and PVC manifest**

```bash
mkdir -p java/k8s/volumes
```

Create `java/k8s/volumes/postgres-pvc.yml`:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: postgres-data
  namespace: java-tasks
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: standard
  resources:
    requests:
      storage: 2Gi
```

- [ ] **Step 2: Verify the manifest parses**

Run: `kubectl apply --dry-run=client -f java/k8s/volumes/postgres-pvc.yml`
Expected: `persistentvolumeclaim/postgres-data created (dry run)` with no validation errors.

- [ ] **Step 3: Commit**

```bash
git add java/k8s/volumes/postgres-pvc.yml
git commit -m "feat(k8s): add postgres PVC manifest"
```

---

## Task 2: Mount the PVC in the postgres deployment

**Files:**
- Modify: `java/k8s/deployments/postgres.yml`

- [ ] **Step 1: Add the volume mount and volume**

Edit `java/k8s/deployments/postgres.yml`. Under `containers[0].volumeMounts` (which already has `initdb`), add a second entry for the data volume. Under `spec.template.spec.volumes`, add the PVC-backed volume.

The resulting `volumeMounts` block:

```yaml
          volumeMounts:
            - name: initdb
              mountPath: /docker-entrypoint-initdb.d
            - name: postgres-data
              mountPath: /var/lib/postgresql/data
              subPath: pgdata
```

The resulting `volumes` block:

```yaml
      volumes:
        - name: initdb
          configMap:
            name: postgres-initdb
        - name: postgres-data
          persistentVolumeClaim:
            claimName: postgres-data
```

The `subPath: pgdata` avoids postgres's "directory is not empty" complaint when the PVC filesystem includes a `lost+found` directory (common with provisioned volumes).

- [ ] **Step 2: Verify the manifest parses**

Run: `kubectl apply --dry-run=client -f java/k8s/deployments/postgres.yml`
Expected: `deployment.apps/postgres configured (dry run)` with no errors.

- [ ] **Step 3: Commit**

```bash
git add java/k8s/deployments/postgres.yml
git commit -m "feat(k8s): mount postgres PVC at pgdata subpath"
```

---

## Task 3: Shrink the postgres-initdb ConfigMap to CREATE DATABASE only

**Files:**
- Modify: `java/k8s/configmaps/postgres-initdb.yml`

Migration Jobs now own schemas and seed data. The ConfigMap's only remaining job is to create the `ecommercedb` database on the first (and only) postgres boot against a fresh PVC.

- [ ] **Step 1: Replace the ConfigMap contents**

Overwrite `java/k8s/configmaps/postgres-initdb.yml` with:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: postgres-initdb
  namespace: java-tasks
data:
  # Runs once, on first boot against a fresh PVC. Creates the database used
  # by the Go auth/ecommerce services. Schemas and seed data are owned by
  # per-service Kubernetes Jobs (golang-migrate) — see go/k8s/jobs/.
  01-create-ecommercedb.sql: |
    CREATE DATABASE ecommercedb OWNER taskuser;
```

- [ ] **Step 2: Verify the manifest parses**

Run: `kubectl apply --dry-run=client -f java/k8s/configmaps/postgres-initdb.yml`
Expected: `configmap/postgres-initdb configured (dry run)`.

- [ ] **Step 3: Commit**

```bash
git add java/k8s/configmaps/postgres-initdb.yml
git commit -m "refactor(k8s): shrink postgres-initdb to CREATE DATABASE only"
```

---

## Task 4: Rename auth-service migrations to `golang-migrate` format

**Files:**
- Rename: `go/auth-service/migrations/001_create_users.sql` → `001_create_users.up.sql`
- Rename: `go/auth-service/migrations/002_google_oauth.sql` → `002_google_oauth.up.sql`
- Create: `go/auth-service/migrations/001_create_users.down.sql`
- Create: `go/auth-service/migrations/002_google_oauth.down.sql`

`golang-migrate` requires the strict `NNN_name.up.sql` / `NNN_name.down.sql` naming. File contents of the up files stay identical — only the filename changes.

- [ ] **Step 1: Rename the up files (preserving git history)**

```bash
git mv go/auth-service/migrations/001_create_users.sql \
       go/auth-service/migrations/001_create_users.up.sql

git mv go/auth-service/migrations/002_google_oauth.sql \
       go/auth-service/migrations/002_google_oauth.up.sql
```

- [ ] **Step 2: Create the 001 down migration**

Create `go/auth-service/migrations/001_create_users.down.sql`:

```sql
DROP INDEX IF EXISTS idx_users_email;
DROP TABLE IF EXISTS users;
-- pgcrypto is shared; leave it in place.
```

- [ ] **Step 3: Create the 002 down migration**

Create `go/auth-service/migrations/002_google_oauth.down.sql`:

```sql
-- Reverse: 002_google_oauth.up.sql
-- Note: setting password_hash NOT NULL will fail if any Google-only users
-- exist. This is the correct behavior for a real downgrade — drop the
-- Google-only users first if you need to apply this.
ALTER TABLE users DROP COLUMN IF EXISTS avatar_url;
ALTER TABLE users ALTER COLUMN password_hash SET NOT NULL;
```

- [ ] **Step 4: Commit**

```bash
git add go/auth-service/migrations/
git commit -m "refactor(auth): rename migrations to golang-migrate format"
```

---

## Task 5: Rename ecommerce-service schema migrations and extract the seed

**Files:**
- Rename: `go/ecommerce-service/migrations/001_create_products.sql` → `001_create_products.up.sql`
- Rename: `go/ecommerce-service/migrations/002_create_cart_items.sql` → `002_create_cart_items.up.sql`
- Rename: `go/ecommerce-service/migrations/003_create_orders.sql` → `003_create_orders.up.sql`
- Create: `go/ecommerce-service/migrations/001_create_products.down.sql`
- Create: `go/ecommerce-service/migrations/002_create_cart_items.down.sql`
- Create: `go/ecommerce-service/migrations/003_create_orders.down.sql`
- Modify: `go/ecommerce-service/migrations/001_create_products.up.sql` (remove the seed INSERT — it moves to `seed.sql` in Task 8)

- [ ] **Step 1: Rename the up files**

```bash
git mv go/ecommerce-service/migrations/001_create_products.sql \
       go/ecommerce-service/migrations/001_create_products.up.sql
git mv go/ecommerce-service/migrations/002_create_cart_items.sql \
       go/ecommerce-service/migrations/002_create_cart_items.up.sql
git mv go/ecommerce-service/migrations/003_create_orders.sql \
       go/ecommerce-service/migrations/003_create_orders.up.sql
```

- [ ] **Step 2: Extract the seed INSERT from 001**

Edit `go/ecommerce-service/migrations/001_create_products.up.sql`. Remove the entire `INSERT INTO products ... VALUES (...);` block at the end of the file. The file should now contain ONLY:

```sql
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    price INTEGER NOT NULL,
    category VARCHAR(100) NOT NULL,
    image_url VARCHAR(500),
    stock INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_products_category ON products (category);
CREATE INDEX idx_products_price ON products (price);
```

- [ ] **Step 3: Create the 001 down migration**

Create `go/ecommerce-service/migrations/001_create_products.down.sql`:

```sql
DROP INDEX IF EXISTS idx_products_price;
DROP INDEX IF EXISTS idx_products_category;
DROP TABLE IF EXISTS products;
```

- [ ] **Step 4: Create the 002 down migration**

Create `go/ecommerce-service/migrations/002_create_cart_items.down.sql`:

```sql
DROP INDEX IF EXISTS idx_cart_items_user;
DROP TABLE IF EXISTS cart_items;
```

- [ ] **Step 5: Create the 003 down migration**

Create `go/ecommerce-service/migrations/003_create_orders.down.sql`:

```sql
DROP INDEX IF EXISTS idx_order_items_order;
DROP TABLE IF EXISTS order_items;
DROP INDEX IF EXISTS idx_orders_status;
DROP INDEX IF EXISTS idx_orders_user;
DROP TABLE IF EXISTS orders;
```

- [ ] **Step 6: Commit**

```bash
git add go/ecommerce-service/migrations/
git commit -m "refactor(ecommerce): rename migrations and split seed"
```

---

## Task 6: Add `migrate` binary and migrations to auth-service Dockerfile

**Files:**
- Modify: `go/auth-service/Dockerfile`

- [ ] **Step 1: Rewrite the Dockerfile**

Replace `go/auth-service/Dockerfile` with:

```dockerfile
FROM migrate/migrate:v4.17.0 AS migrate

FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /auth-service ./cmd/server

FROM alpine:3.19

RUN apk add --no-cache postgresql-client
RUN adduser -D -u 1001 appuser

COPY --from=builder /auth-service /auth-service
COPY --from=migrate /usr/local/bin/migrate /usr/local/bin/migrate
COPY migrations/ /migrations/

USER appuser

EXPOSE 8091
ENTRYPOINT ["/auth-service"]
```

- [ ] **Step 2: Build the image locally and verify the migrate binary is present**

Run (from repo root, with Colima/Docker running):
```bash
docker build -t go-auth-service:local go/auth-service
docker run --rm --entrypoint /usr/local/bin/migrate go-auth-service:local -version
```
Expected: `golang-migrate` prints its version (e.g., `4.17.0`) and exits 0.

- [ ] **Step 3: Verify migrations are present inside the image**

Run: `docker run --rm --entrypoint ls go-auth-service:local /migrations`
Expected: four files — `001_create_users.up.sql`, `001_create_users.down.sql`, `002_google_oauth.up.sql`, `002_google_oauth.down.sql`.

- [ ] **Step 4: Commit**

```bash
git add go/auth-service/Dockerfile
git commit -m "feat(auth): bake migrate binary and migrations into image"
```

---

## Task 7: Add `migrate` binary, migrations, and seed to ecommerce-service Dockerfile

**Files:**
- Modify: `go/ecommerce-service/Dockerfile`

- [ ] **Step 1: Rewrite the Dockerfile**

Replace `go/ecommerce-service/Dockerfile` with:

```dockerfile
FROM migrate/migrate:v4.17.0 AS migrate

FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /ecommerce-service ./cmd/server

FROM alpine:3.19

RUN apk add --no-cache postgresql-client
RUN adduser -D -u 1001 appuser

COPY --from=builder /ecommerce-service /ecommerce-service
COPY --from=migrate /usr/local/bin/migrate /usr/local/bin/migrate
COPY migrations/ /migrations/
COPY seed.sql /seed.sql

USER appuser

EXPOSE 8092
ENTRYPOINT ["/ecommerce-service"]
```

- [ ] **Step 2: Note — do not build yet**

The build will fail until Task 8 creates `seed.sql`. The commit for this Task 7 is still safe because the Dockerfile only *declares* the COPY; the build itself runs in Task 8's verification step.

- [ ] **Step 3: Commit**

```bash
git add go/ecommerce-service/Dockerfile
git commit -m "feat(ecommerce): bake migrate, migrations, and seed into image"
```

---

## Task 8: Create `go/ecommerce-service/seed.sql`

**Files:**
- Create: `go/ecommerce-service/seed.sql`

This file is run by the ecommerce-service migration Job after `migrate up` succeeds. Both operations (product catalog and smoke user) are guarded so re-runs are idempotent.

- [ ] **Step 1: Create the seed file**

Create `go/ecommerce-service/seed.sql`:

```sql
-- Seed data for ecommercedb. Run by go/k8s/jobs/ecommerce-service-migrate.yml
-- after `migrate up` succeeds. Every operation is idempotent so the Job can
-- re-run on every deploy without creating duplicates.

-- Product catalog (20 rows). image_url is '' (not NULL) because the Go
-- model.Product.ImageURL is a plain string and would fail to scan NULL.
INSERT INTO products (name, description, price, category, image_url, stock)
SELECT * FROM (VALUES
    ('Wireless Bluetooth Headphones', 'Noise-canceling over-ear headphones with 30hr battery', 7999, 'Electronics', '', 50),
    ('USB-C Fast Charger', 'GaN 65W charger with 3 ports', 3499, 'Electronics', '', 100),
    ('Mechanical Keyboard', 'RGB backlit with Cherry MX switches', 12999, 'Electronics', '', 30),
    ('Portable SSD 1TB', 'NVMe external drive, USB-C, 1050MB/s', 8999, 'Electronics', '', 40),
    ('Classic Cotton T-Shirt', 'Heavyweight premium cotton, unisex fit', 2499, 'Clothing', '', 200),
    ('Slim Fit Chinos', 'Stretch cotton blend, multiple colors available', 4999, 'Clothing', '', 80),
    ('Lightweight Rain Jacket', 'Packable waterproof shell with sealed seams', 6999, 'Clothing', '', 60),
    ('Merino Wool Beanie', 'Breathable and temperature regulating', 1999, 'Clothing', '', 150),
    ('Pour-Over Coffee Maker', 'Borosilicate glass with stainless steel filter', 3999, 'Home', '', 70),
    ('Cast Iron Skillet 12"', 'Pre-seasoned, oven safe to 500F', 4499, 'Home', '', 45),
    ('LED Desk Lamp', 'Adjustable brightness and color temperature', 5999, 'Home', '', 55),
    ('Ceramic Planter Set', 'Set of 3, drainage holes included', 2999, 'Home', '', 90),
    ('The Go Programming Language', 'Donovan & Kernighan — comprehensive Go guide', 3499, 'Books', '', 120),
    ('Designing Data-Intensive Applications', 'Martin Kleppmann — distributed systems bible', 3999, 'Books', '', 100),
    ('Clean Architecture', 'Robert C. Martin — software design principles', 2999, 'Books', '', 80),
    ('System Design Interview', 'Alex Xu — practical system design guide', 3499, 'Books', '', 95),
    ('Yoga Mat 6mm', 'Non-slip TPE material with carrying strap', 2999, 'Sports', '', 110),
    ('Adjustable Dumbbells', 'Quick-change weight from 5-52.5 lbs per hand', 29999, 'Sports', '', 20),
    ('Resistance Band Set', '5 bands with handles, door anchor, and bag', 1999, 'Sports', '', 130),
    ('Water Bottle 32oz', 'Insulated stainless steel, keeps cold 24hrs', 2499, 'Sports', '', 200)
) AS v(name, description, price, category, image_url, stock)
WHERE NOT EXISTS (SELECT 1 FROM products);

-- Smoke-test user. The password is stored in GitHub Actions secret
-- SMOKE_GO_PASSWORD; this hash must match it (bcrypt cost 10, the same
-- cost the auth-service uses via bcrypt.DefaultCost).
--
-- To regenerate the hash after rotating the password:
--   python3 -c "import bcrypt; print(bcrypt.hashpw(b'<new-password>', bcrypt.gensalt(10)).decode())"
INSERT INTO users (email, password_hash, name)
SELECT 'smoke@kylebradshaw.dev', '__SMOKE_BCRYPT_HASH__', 'Smoke Test'
WHERE NOT EXISTS (SELECT 1 FROM users WHERE email = 'smoke@kylebradshaw.dev');
```

- [ ] **Step 2: Replace the bcrypt placeholder with the real hash**

Kyle must provide the bcrypt hash before this step. If he hasn't, leave `__SMOKE_BCRYPT_HASH__` in place and flag it for the final pre-merge check. Otherwise, replace `__SMOKE_BCRYPT_HASH__` with the literal hash string (including the leading `$2b$10$`).

- [ ] **Step 3: Build the ecommerce-service image now that seed.sql exists**

Run:
```bash
docker build -t go-ecommerce-service:local go/ecommerce-service
docker run --rm --entrypoint ls go-ecommerce-service:local /migrations
docker run --rm --entrypoint ls go-ecommerce-service:local /seed.sql
```
Expected: six migration files, and `/seed.sql` listed.

- [ ] **Step 4: Commit**

```bash
git add go/ecommerce-service/seed.sql
git commit -m "feat(ecommerce): add idempotent seed data with smoke user"
```

---

## Task 9: Write the auth-service migration Job manifest

**Files:**
- Create: `go/k8s/jobs/auth-service-migrate.yml`

- [ ] **Step 1: Create the jobs directory and manifest**

```bash
mkdir -p go/k8s/jobs
```

Create `go/k8s/jobs/auth-service-migrate.yml`:

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: go-auth-migrate
  namespace: go-ecommerce
spec:
  backoffLimit: 2
  activeDeadlineSeconds: 120
  ttlSecondsAfterFinished: 600
  template:
    spec:
      restartPolicy: Never
      imagePullSecrets:
        - name: ghcr-secret
      containers:
        - name: migrate
          image: ghcr.io/kabradshaw1/portfolio/go-auth-service:latest
          imagePullPolicy: Always
          command: ["/usr/local/bin/migrate"]
          args:
            - "-path=/migrations"
            - "-database=$(DATABASE_URL)"
            - "up"
          env:
            - name: DATABASE_URL
              valueFrom:
                configMapKeyRef:
                  name: auth-service-config
                  key: DATABASE_URL
          resources:
            requests:
              memory: "32Mi"
              cpu: "50m"
            limits:
              memory: "128Mi"
              cpu: "200m"
```

- [ ] **Step 2: Verify the manifest parses**

Run: `kubectl apply --dry-run=client -f go/k8s/jobs/auth-service-migrate.yml`
Expected: `job.batch/go-auth-migrate created (dry run)`.

- [ ] **Step 3: Commit**

```bash
git add go/k8s/jobs/auth-service-migrate.yml
git commit -m "feat(k8s): add auth-service migration Job"
```

---

## Task 10: Write the ecommerce-service migration Job manifest

**Files:**
- Create: `go/k8s/jobs/ecommerce-service-migrate.yml`

The ecommerce Job runs `migrate up` first, then `psql -f /seed.sql`. Both steps in a single container via `sh -c`.

- [ ] **Step 1: Create the manifest**

Create `go/k8s/jobs/ecommerce-service-migrate.yml`:

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: go-ecommerce-migrate
  namespace: go-ecommerce
spec:
  backoffLimit: 2
  activeDeadlineSeconds: 120
  ttlSecondsAfterFinished: 600
  template:
    spec:
      restartPolicy: Never
      imagePullSecrets:
        - name: ghcr-secret
      containers:
        - name: migrate-and-seed
          image: ghcr.io/kabradshaw1/portfolio/go-ecommerce-service:latest
          imagePullPolicy: Always
          command: ["/bin/sh", "-c"]
          args:
            - |
              set -e
              echo "Running migrations..."
              /usr/local/bin/migrate -path=/migrations -database="$DATABASE_URL" up
              echo "Applying seed data..."
              psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f /seed.sql
              echo "Done."
          env:
            - name: DATABASE_URL
              valueFrom:
                configMapKeyRef:
                  name: ecommerce-service-config
                  key: DATABASE_URL
          resources:
            requests:
              memory: "32Mi"
              cpu: "50m"
            limits:
              memory: "128Mi"
              cpu: "200m"
```

- [ ] **Step 2: Verify the manifest parses**

Run: `kubectl apply --dry-run=client -f go/k8s/jobs/ecommerce-service-migrate.yml`
Expected: `job.batch/go-ecommerce-migrate created (dry run)`.

- [ ] **Step 3: Commit**

```bash
git add go/k8s/jobs/ecommerce-service-migrate.yml
git commit -m "feat(k8s): add ecommerce-service migrate+seed Job"
```

---

## Task 11: Wire migration Jobs into the CI deploy step

**Files:**
- Modify: `.github/workflows/ci.yml`

The existing deploy step applies manifests from `go/k8s/` recursively and then restarts deployments. Problem: Jobs are immutable once created, so `kubectl apply` of an unchanged Job name is a no-op — the migrations won't re-run on future deploys. Fix: delete the Jobs explicitly before applying, then wait for completion before rolling deployments.

- [ ] **Step 1: Modify the Go restart block**

In `.github/workflows/ci.yml`, find the block starting around line 518:

```bash
          if [ "$GO_CHANGED" = "true" ] || [ "$K8S_CHANGED" = "true" ]; then
            echo "Restarting go-ecommerce..."
            $SSH "kubectl rollout restart deployment -n go-ecommerce"
            $SSH "kubectl rollout status deployment -n go-ecommerce --timeout=180s"
          fi
```

Replace it with:

```bash
          if [ "$GO_CHANGED" = "true" ] || [ "$K8S_CHANGED" = "true" ]; then
            echo "Running Go migration Jobs..."
            # Jobs are immutable; delete before re-applying so they re-run on every deploy.
            $SSH "kubectl delete job go-auth-migrate go-ecommerce-migrate -n go-ecommerce --ignore-not-found --wait=true"
            $SSH "kubectl apply -f -" < go/k8s/jobs/auth-service-migrate.yml
            $SSH "kubectl apply -f -" < go/k8s/jobs/ecommerce-service-migrate.yml

            # Wait for both Jobs to complete. If either fails, dump logs and fail the deploy.
            if ! $SSH "kubectl wait --for=condition=complete --timeout=120s job/go-auth-migrate job/go-ecommerce-migrate -n go-ecommerce"; then
              echo "Migration Job failed. Logs follow:"
              $SSH "kubectl logs job/go-auth-migrate -n go-ecommerce" || true
              $SSH "kubectl logs job/go-ecommerce-migrate -n go-ecommerce" || true
              exit 1
            fi

            echo "Restarting go-ecommerce deployments..."
            $SSH "kubectl rollout restart deployment -n go-ecommerce"
            $SSH "kubectl rollout status deployment -n go-ecommerce --timeout=180s"
          fi
```

- [ ] **Step 2: Prevent the earlier blanket-apply from creating stale Jobs**

Scroll up to the blanket `find go/k8s -name '*.yml'` apply on line ~503:

```bash
          for f in $(find go/k8s -name '*.yml' -not -name 'namespace.yml' -not -path '*/secrets/*'); do echo '---'; cat "$f"; done | $SSH "kubectl apply -f -"
```

Exclude the new `jobs/` directory from this blanket apply (Jobs are handled explicitly in the Go restart block):

```bash
          for f in $(find go/k8s -name '*.yml' -not -name 'namespace.yml' -not -path '*/secrets/*' -not -path '*/jobs/*'); do echo '---'; cat "$f"; done | $SSH "kubectl apply -f -"
```

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: run Go migration Jobs before rolling deployments"
```

---

## Task 12: Add Go smoke tests to the Playwright suite

**Files:**
- Modify: `frontend/e2e/smoke.spec.ts`

Existing smoke tests cover Python AI and Java task management. Add a new `describe` block for the Go store that asserts:
1. Products endpoint returns 200 with a non-empty `products` array.
2. Categories endpoint returns 200 with a non-empty `categories` array.
3. Smoke user can log in and receive an `accessToken`.

Using Playwright's `request` fixture matches the existing style and keeps everything in one workflow job.

- [ ] **Step 1: Append the new describe block at the end of the file**

Add to the end of `frontend/e2e/smoke.spec.ts` (after the Java describe block, after the final `});`):

```typescript
test.describe("Go ecommerce smoke tests", () => {
  const SMOKE_EMAIL = "smoke@kylebradshaw.dev";
  const SMOKE_PASSWORD = process.env.SMOKE_GO_PASSWORD;

  test("products endpoint returns a non-empty catalog", async ({ request }) => {
    const res = await request.get(`${API_URL}/go-api/products`);
    expect(res.ok()).toBeTruthy();
    const body = await res.json();
    expect(Array.isArray(body.products)).toBe(true);
    expect(body.products.length).toBeGreaterThan(0);
  });

  test("categories endpoint returns a non-empty list", async ({ request }) => {
    const res = await request.get(`${API_URL}/go-api/categories`);
    expect(res.ok()).toBeTruthy();
    const body = await res.json();
    expect(Array.isArray(body.categories)).toBe(true);
    expect(body.categories.length).toBeGreaterThan(0);
  });

  test("smoke user can log in to the Go auth service", async ({ request }) => {
    expect(
      SMOKE_PASSWORD,
      "SMOKE_GO_PASSWORD env var must be set for this test"
    ).toBeTruthy();

    const res = await request.post(`${API_URL}/go-auth/auth/login`, {
      data: { email: SMOKE_EMAIL, password: SMOKE_PASSWORD },
    });
    expect(res.ok()).toBeTruthy();
    const body = await res.json();
    expect(typeof body.accessToken).toBe("string");
    expect(body.accessToken.length).toBeGreaterThan(0);
  });
});
```

- [ ] **Step 2: Pass SMOKE_GO_PASSWORD through the workflow env**

In `.github/workflows/ci.yml`, find the `smoke-production` job's `Run smoke tests` step (around line 553):

```yaml
      - name: Run smoke tests
        env:
          SMOKE_FRONTEND_URL: https://kylebradshaw.dev
          SMOKE_API_URL: https://api.kylebradshaw.dev
          SMOKE_GRAPHQL_URL: https://api.kylebradshaw.dev/graphql
        run: npx playwright test e2e/smoke.spec.ts --config=playwright.smoke.config.ts
```

Add `SMOKE_GO_PASSWORD`:

```yaml
      - name: Run smoke tests
        env:
          SMOKE_FRONTEND_URL: https://kylebradshaw.dev
          SMOKE_API_URL: https://api.kylebradshaw.dev
          SMOKE_GRAPHQL_URL: https://api.kylebradshaw.dev/graphql
          SMOKE_GO_PASSWORD: ${{ secrets.SMOKE_GO_PASSWORD }}
        run: npx playwright test e2e/smoke.spec.ts --config=playwright.smoke.config.ts
```

- [ ] **Step 3: Typecheck the frontend**

Run:
```bash
cd frontend && npx tsc --noEmit
```
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/e2e/smoke.spec.ts .github/workflows/ci.yml
git commit -m "test(smoke): add Go store production smoke tests"
```

---

## Task 13: Document the migration workflow in CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Add a new "Migrations" subsection**

In `CLAUDE.md`, after the "Vercel CLI" subsection (under Infrastructure), add:

```markdown
### Migrations

- **Go services (`go/auth-service`, `go/ecommerce-service`):** schema changes use `golang-migrate`. Migration files live in `go/<service>/migrations/` and use the strict `NNN_name.up.sql` / `NNN_name.down.sql` pair format. The `migrate` binary is baked into each service image; a Kubernetes `Job` per service (`go/k8s/jobs/*-migrate.yml`) runs `migrate up` on every deploy before the deployments are rolled.
- **To add a schema change:** create a new `NNN_name.up.sql` + matching `.down.sql` in the right `migrations/` directory. Commit. The next deploy runs it automatically.
- **Seed data (ecommerce only):** lives in `go/ecommerce-service/seed.sql`, applied by the ecommerce Job after `migrate up`. Must be idempotent (guard every INSERT with `WHERE NOT EXISTS`).
- **Java services:** schema is owned by Spring/JPA at service startup. No separate migration step.
- **Python AI services:** no relational schema (Qdrant is schemaless).
- **`postgres-initdb` ConfigMap:** only creates the `ecommercedb` database on first boot of a fresh PVC. It does NOT own any schemas — those are owned by the per-service migration Jobs.
```

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs(claude): document Go migration workflow"
```

---

## Task 14: End-to-end local verification (no push)

Before handing the branch to Kyle for push, verify the full flow locally against the Minikube cluster on the Windows PC via SSH. This does NOT push images to GHCR — it tests the manifests and the local image builds.

- [ ] **Step 1: Confirm the bcrypt placeholder was replaced**

Run: `grep -r __SMOKE_BCRYPT_HASH__ go/ecommerce-service/seed.sql || echo "OK"`
Expected: `OK` (no matches).

If the placeholder is still there, STOP and ask Kyle for the hash before continuing. The smoke tests will fail on main without it.

- [ ] **Step 2: Apply the PVC and new postgres manifest to Minikube**

```bash
cat java/k8s/volumes/postgres-pvc.yml | ssh PC@100.79.113.84 "kubectl apply -f -"
cat java/k8s/configmaps/postgres-initdb.yml | ssh PC@100.79.113.84 "kubectl apply -f -"
cat java/k8s/deployments/postgres.yml | ssh PC@100.79.113.84 "kubectl apply -f -"
ssh PC@100.79.113.84 "kubectl rollout status deployment/postgres -n java-tasks --timeout=120s"
```
Expected: rollout finishes. PVC bound (`kubectl get pvc -n java-tasks` shows `Bound`).

- [ ] **Step 3: Verify postgres can start against the new PVC and ecommercedb exists**

```bash
ssh PC@100.79.113.84 "kubectl exec -n java-tasks deploy/postgres -- psql -U taskuser -d postgres -c '\\l'"
```
Expected: output lists `taskdb` AND `ecommercedb`.

- [ ] **Step 4: Confirm ecommercedb is empty (no schemas yet)**

```bash
ssh PC@100.79.113.84 "kubectl exec -n java-tasks deploy/postgres -- psql -U taskuser -d ecommercedb -c '\\dt'"
```
Expected: `Did not find any relations.`

- [ ] **Step 5: Apply the Job manifests and run them**

These Jobs pull the `:latest` image from GHCR, which is the pre-change image. For a clean local test, skip to Step 6 — the real validation is the post-merge CI deploy. If you want to test manifests anyway:

```bash
cat go/k8s/jobs/auth-service-migrate.yml | ssh PC@100.79.113.84 "kubectl apply -f -"
cat go/k8s/jobs/ecommerce-service-migrate.yml | ssh PC@100.79.113.84 "kubectl apply -f -"
ssh PC@100.79.113.84 "kubectl wait --for=condition=complete --timeout=120s job/go-auth-migrate job/go-ecommerce-migrate -n go-ecommerce"
```

These jobs will FAIL against `:latest` because that image doesn't have `migrate` or `seed.sql`. That's expected — the real validation happens post-merge when CI builds and pushes new images. Delete the failed Jobs after:

```bash
ssh PC@100.79.113.84 "kubectl delete job go-auth-migrate go-ecommerce-migrate -n go-ecommerce --ignore-not-found"
```

- [ ] **Step 6: Re-seed ecommercedb by hand so the site keeps working until CI deploys the new images**

The ecommercedb is empty right now (we wiped it in Step 2 by applying the new PVC). Apply the old seed via a one-liner so production stays up until the first post-merge deploy:

```bash
ssh PC@100.79.113.84 "kubectl exec -n java-tasks deploy/postgres -i -- psql -U taskuser -d ecommercedb" < go/ecommerce-service/migrations/001_create_products.up.sql
ssh PC@100.79.113.84 "kubectl exec -n java-tasks deploy/postgres -i -- psql -U taskuser -d ecommercedb" < go/ecommerce-service/migrations/002_create_cart_items.up.sql
ssh PC@100.79.113.84 "kubectl exec -n java-tasks deploy/postgres -i -- psql -U taskuser -d ecommercedb" < go/ecommerce-service/migrations/003_create_orders.up.sql
ssh PC@100.79.113.84 "kubectl exec -n java-tasks deploy/postgres -i -- psql -U taskuser -d ecommercedb" < go/auth-service/migrations/001_create_users.up.sql
ssh PC@100.79.113.84 "kubectl exec -n java-tasks deploy/postgres -i -- psql -U taskuser -d ecommercedb" < go/auth-service/migrations/002_google_oauth.up.sql
ssh PC@100.79.113.84 "kubectl exec -n java-tasks deploy/postgres -i -- psql -U taskuser -d ecommercedb" < go/ecommerce-service/seed.sql
```

Expected: all commands exit 0. Site stays up.

- [ ] **Step 7: Verify the live site still works**

```bash
curl -sf https://api.kylebradshaw.dev/go-api/products | jq -e '.products | length > 0' > /dev/null && echo "products OK"
curl -sf https://api.kylebradshaw.dev/go-api/categories | jq -e '.categories | length > 0' > /dev/null && echo "categories OK"
```
Expected: `products OK` and `categories OK`.

- [ ] **Step 8: Confirm pre-push checks pass**

Run: `make preflight-frontend`
Expected: passes (tsc + lint + build).

---

## Task 15: Hand off to Kyle for push + merge

The branch is not pushed from this session per the CLAUDE.md convention ("Kyle handles all git push and merge operations").

- [ ] **Step 1: Show the branch state**

Run:
```bash
git log --oneline main..HEAD
git status
```
Expected: clean working tree, ~13 commits on `go-ecommerce-hardening` ahead of `main`.

- [ ] **Step 2: Write a handoff message**

Print a message for Kyle summarizing:
- What the branch does (3 bullet points: PVC, migration Jobs, smoke tests)
- The manual prerequisites before push: confirm `SMOKE_GO_PASSWORD` is set in GitHub secrets, confirm the bcrypt hash in `seed.sql` matches
- The expected post-merge behavior: CI deploy runs the Jobs, smoke tests verify the site, Grafana/logs available if anything fails
- What to watch for on the first deploy: the migration Job logs will be the first place to look if things break

---

## Self-review notes

**Spec coverage:**
- ✅ PVC → Tasks 1, 2, 14
- ✅ Shrunk initdb ConfigMap → Task 3
- ✅ Renamed migrations + down files (both services) → Tasks 4, 5
- ✅ Dockerfile changes (migrate binary + migrations + seed) → Tasks 6, 7
- ✅ seed.sql with products + smoke user → Task 8
- ✅ Migration Job manifests → Tasks 9, 10
- ✅ CI deploy orchestration → Task 11
- ✅ Go smoke tests → Task 12
- ✅ CLAUDE.md documentation → Task 13
- ✅ Manual bcrypt generation called out in pre-flight and Task 8
- ✅ `SMOKE_GO_PASSWORD` secret added to smoke workflow env in Task 12 Step 2

**Known risks / things to watch:**
- **Step 14.6 bridge seeding:** applying the new PVC deployment *before* the CI-built images exist will leave production in a temporarily broken state. The bridge seeding commands in Step 14.6 avoid this by manually re-seeding the DB, keeping the site up until the first post-merge CI deploy brings the real migration Jobs. If you'd rather avoid any production disruption, defer Task 14 Steps 2-6 entirely and let the first CI deploy handle everything — but then the first deploy is also the first time the PVC is tested, which is riskier.
- **Jobs using `:latest`:** the manifests reference `ghcr.io/kabradshaw1/portfolio/go-{auth,ecommerce}-service:latest`. The CI build job must have pushed updated `:latest` tags by the time the deploy step runs. This is already how the existing deployments work, so the pattern is proven — but note the coupling.
- **Bridge seeding uses the old in-service auth table schema:** `go/auth-service/migrations/001_create_users.up.sql` has `password_hash NOT NULL` and `002` loosens it. Make sure both apply in order or the smoke user seed will fail.
