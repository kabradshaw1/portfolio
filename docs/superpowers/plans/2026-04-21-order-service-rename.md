# Order Service Rename (Phase 3a) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename ecommerce-service to order-service across the entire codebase — no logic changes, everything works identically after.

**Architecture:** Mechanical rename of directory, module path, Docker image, K8s manifests, CI pipelines, frontend URLs, and ingress path (`/go-api` → `/go-orders`). The ecommerce-service currently contains only order/return logic after Phases 1 (product extraction) and 2 (cart extraction).

**Tech Stack:** Go, Kubernetes YAML, GitHub Actions CI, Next.js frontend, Playwright E2E tests

**Spec:** `docs/superpowers/specs/2026-04-21-order-service-saga-design.md` (Sub-phase A section)

**Base branch:** `main` (Phase 2 cart-service extraction is deployed)

**IMPORTANT:** Create a new worktree from `main` before implementing. Run `make preflight-go` and `make preflight-frontend` after the rename to verify nothing broke.

**Rename mapping:**

| Old | New |
|-----|-----|
| `go/ecommerce-service/` | `go/order-service/` |
| `github.com/kabradshaw1/portfolio/go/ecommerce-service` | `github.com/kabradshaw1/portfolio/go/order-service` |
| `go-ecommerce-service` (K8s names, Docker image) | `go-order-service` |
| `ecommerce-service-config` (ConfigMap) | `order-service-config` |
| `go-ecommerce-migrate` (Job) | `go-order-migrate` |
| `ecommerce-service-hpa` / `ecommerce-hpa` | `go-order-hpa` |
| `go-ecommerce-service-pdb` / `ecommerce-pdb` | `go-order-pdb` |
| `/go-api` (ingress path) | `/go-orders` |
| `GO_ECOMMERCE_URL` / `NEXT_PUBLIC_GO_ECOMMERCE_URL` | `GO_ORDER_URL` / `NEXT_PUBLIC_GO_ORDER_URL` |
| `ECOMMERCE_URL` (ai-service config) | `ORDER_URL` |
| `go-api.ts` (frontend) | `go-order-api.ts` |

**What NOT to rename:**
- `ecommercedb` / `ecommercedb_qa` — database names stay (renaming would require data migration)
- `ecommerce_schema_migrations` — migration table name stays
- `go-ecommerce` / `go-ecommerce-qa` — K8s namespace names stay
- `ecommerce.saga`, `ecommerce.orders`, `ecommerce.cart` — RabbitMQ/Kafka topics stay
- `go/ecommerce-service/seed.sql` content — seeds stay, file moves with directory
- ADR docs and historical specs — don't rename references in documentation

---

### Task 1: Rename Go directory and update module path

**Files:**
- Rename: `go/ecommerce-service/` → `go/order-service/`
- Modify: `go/order-service/go.mod` (module path)
- Modify: Every `.go` file in `go/order-service/` (import paths)

- [ ] **Step 1: Create worktree from main**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer
git worktree add .claude/worktrees/agent+feat-order-service-saga -b agent/feat-order-service-saga main
cd .claude/worktrees/agent+feat-order-service-saga
```

- [ ] **Step 2: Move the directory**

```bash
mv go/ecommerce-service go/order-service
```

- [ ] **Step 3: Update go.mod module path**

In `go/order-service/go.mod`, change:
```
module github.com/kabradshaw1/portfolio/go/ecommerce-service
```
to:
```
module github.com/kabradshaw1/portfolio/go/order-service
```

- [ ] **Step 4: Find-and-replace all import paths in go/order-service/**

Replace all occurrences of `github.com/kabradshaw1/portfolio/go/ecommerce-service` with `github.com/kabradshaw1/portfolio/go/order-service` across every `.go` file in `go/order-service/`.

Use:
```bash
find go/order-service -name '*.go' -exec sed -i '' 's|github.com/kabradshaw1/portfolio/go/ecommerce-service|github.com/kabradshaw1/portfolio/go/order-service|g' {} +
```

- [ ] **Step 5: Update OTel service name strings**

In `go/order-service/cmd/server/routes.go`, change `otelgin.Middleware("ecommerce-service")` to `otelgin.Middleware("order-service")`.

In `go/order-service/cmd/server/main.go`, change `tracing.Init(ctx, "ecommerce-service",` to `tracing.Init(ctx, "order-service",`.

- [ ] **Step 6: Update Kafka event source**

In `go/order-service/internal/kafka/producer.go`, change `event.Source = "ecommerce-service"` to `event.Source = "order-service"`.

- [ ] **Step 7: Update circuit breaker name**

In `go/order-service/cmd/server/main.go`, change breaker name `"ecommerce-postgres"` to `"order-postgres"`.

- [ ] **Step 8: Verify go/order-service compiles**

```bash
cd go/order-service && go mod tidy && go build ./cmd/server
```

- [ ] **Step 9: Run order-service tests**

```bash
cd go/order-service && go test ./... -v -race
```

- [ ] **Step 10: Commit**

```bash
git add go/ecommerce-service go/order-service
git commit -m "refactor: rename ecommerce-service directory to order-service"
```

Note: `git add go/ecommerce-service` stages the deletion of the old directory.

---

### Task 2: Update cross-service references (cart-service, ai-service, product-service)

**Files:**
- Modify: `go/cart-service/go.mod` (replace directive if it references ecommerce-service)
- Modify: `go/ai-service/cmd/server/config.go` (ECOMMERCE_URL → ORDER_URL)
- Modify: `go/ai-service/cmd/server/main.go` (ecommerceURL variable)
- Modify: `go/ai-service/internal/tools/clients/ecommerce.go` (or rename file)
- Modify: `go/ai-service/internal/auth/jwt.go` (comments)
- Modify: `go/ai-service/internal/mcp/server.go` (comments)
- Modify: `go/docker-compose.yml` (service name and references)

- [ ] **Step 1: Update ai-service config**

Read `go/ai-service/cmd/server/config.go`. Rename `ECOMMERCE_URL` env var to `ORDER_URL` and the config field to `OrderURL`. Update the default value comment.

- [ ] **Step 2: Update ai-service main.go**

Read `go/ai-service/cmd/server/main.go`. Update the variable name from `ecommerceURL` to `orderURL` and the env var from `ECOMMERCE_URL` to `ORDER_URL`.

- [ ] **Step 3: Update ai-service client**

Read `go/ai-service/internal/tools/clients/ecommerce.go`. Update comments referencing "ecommerce-service" to "order-service". The file can keep its name since it wraps ecommerce domain operations, or rename if you prefer — but the function signatures and behavior don't change.

- [ ] **Step 4: Update ai-service auth and mcp comments**

Update comments in `go/ai-service/internal/auth/jwt.go` and `go/ai-service/internal/mcp/server.go` that reference "ecommerce-service" to "order-service".

- [ ] **Step 5: Update go/docker-compose.yml**

Read `go/docker-compose.yml`. Rename the `ecommerce-service:` service block to `order-service:`. Update:
- Service name
- `dockerfile:` path from `ecommerce-service/Dockerfile` to `order-service/Dockerfile`
- `ECOMMERCE_URL: http://ecommerce-service:8092` → `ORDER_URL: http://order-service:8092` in ai-service env
- `depends_on: ecommerce-service` → `depends_on: order-service`

- [ ] **Step 6: Check cart-service go.mod**

Read `go/cart-service/go.mod`. If it has a replace directive for `ecommerce-service`, update it to `order-service`. (It likely doesn't — cart-service doesn't import ecommerce-service code.)

- [ ] **Step 7: Verify ai-service compiles**

```bash
cd go/ai-service && go mod tidy && go build ./cmd/server
```

- [ ] **Step 8: Commit**

```bash
git add go/ai-service go/docker-compose.yml go/cart-service
git commit -m "refactor: update cross-service references from ecommerce to order"
```

---

### Task 3: Rename Dockerfile and update build references

**Files:**
- Modify: `go/order-service/Dockerfile` (paths and binary name)

- [ ] **Step 1: Update Dockerfile**

Read `go/order-service/Dockerfile`. Replace all `ecommerce-service` references:
- `WORKDIR /app/ecommerce-service` → `WORKDIR /app/order-service`
- `COPY order-service/go.mod order-service/go.sum ./` (already correct after directory rename)
- `COPY order-service/ .` (already correct)
- `go build -o /ecommerce-service` → `go build -o /order-service`
- `COPY --from=builder /ecommerce-service /ecommerce-service` → `COPY --from=builder /order-service /order-service`
- `COPY ecommerce-service/migrations/` → `COPY order-service/migrations/`
- `COPY ecommerce-service/seed.sql` → `COPY order-service/seed.sql`
- `ENTRYPOINT ["/ecommerce-service"]` → `ENTRYPOINT ["/order-service"]`
- Update comment at top: `# Order-service: REST :8092`

- [ ] **Step 2: Commit**

```bash
git add go/order-service/Dockerfile
git commit -m "refactor: update order-service Dockerfile paths and binary name"
```

---

### Task 4: Rename K8s manifests

**Files:**
- Rename: `go/k8s/deployments/ecommerce-service.yml` → `go/k8s/deployments/order-service.yml`
- Rename: `go/k8s/services/ecommerce-service.yml` → `go/k8s/services/order-service.yml`
- Rename: `go/k8s/configmaps/ecommerce-service-config.yml` → `go/k8s/configmaps/order-service-config.yml`
- Rename: `go/k8s/jobs/ecommerce-service-migrate.yml` → `go/k8s/jobs/order-service-migrate.yml`
- Rename: `go/k8s/hpa/ecommerce-hpa.yml` → `go/k8s/hpa/order-hpa.yml`
- Rename: `go/k8s/pdb/ecommerce-pdb.yml` → `go/k8s/pdb/order-pdb.yml`
- Modify: Each renamed file (update internal names)
- Modify: `go/k8s/ingress.yml` (path and service name)
- Modify: `go/k8s/kustomization.yaml` (resource paths)

- [ ] **Step 1: Rename all K8s files**

```bash
mv go/k8s/deployments/ecommerce-service.yml go/k8s/deployments/order-service.yml
mv go/k8s/services/ecommerce-service.yml go/k8s/services/order-service.yml
mv go/k8s/configmaps/ecommerce-service-config.yml go/k8s/configmaps/order-service-config.yml
mv go/k8s/jobs/ecommerce-service-migrate.yml go/k8s/jobs/order-service-migrate.yml
mv go/k8s/hpa/ecommerce-hpa.yml go/k8s/hpa/order-hpa.yml
mv go/k8s/pdb/ecommerce-pdb.yml go/k8s/pdb/order-pdb.yml
```

- [ ] **Step 2: Update deployment**

Read `go/k8s/deployments/order-service.yml`. Replace:
- `name: go-ecommerce-service` → `name: go-order-service`
- `app: go-ecommerce-service` → `app: go-order-service`
- `image: ghcr.io/kabradshaw1/portfolio/go-ecommerce-service:latest` → `ghcr.io/kabradshaw1/portfolio/go-order-service:latest`
- `configMapRef: name: ecommerce-service-config` → `order-service-config`
- Container name: `go-ecommerce-service` → `go-order-service`

- [ ] **Step 3: Update service**

Read `go/k8s/services/order-service.yml`. Replace `go-ecommerce-service` → `go-order-service` in name and selector.

- [ ] **Step 4: Update configmap**

Read `go/k8s/configmaps/order-service-config.yml`. Change `name: ecommerce-service-config` → `name: order-service-config`. Content stays the same.

- [ ] **Step 5: Update migration job**

Read `go/k8s/jobs/order-service-migrate.yml`. Replace:
- `name: go-ecommerce-migrate` → `name: go-order-migrate`
- Image: `go-ecommerce-service` → `go-order-service`
- ConfigMap ref: `ecommerce-service-config` → `order-service-config`
- Keep `x-migrations-table=ecommerce_schema_migrations` (migration table name does NOT change)

- [ ] **Step 6: Update HPA**

Read `go/k8s/hpa/order-hpa.yml`. Replace names: `ecommerce-service-hpa` → `go-order-hpa`, target `go-ecommerce-service` → `go-order-service`.

- [ ] **Step 7: Update PDB**

Read `go/k8s/pdb/order-pdb.yml`. Replace: `go-ecommerce-service-pdb` → `go-order-pdb`, selector `go-ecommerce-service` → `go-order-service`.

- [ ] **Step 8: Update ingress**

Read `go/k8s/ingress.yml`. Change the ecommerce path:
- `/go-api(/|$)(.*)` → `/go-orders(/|$)(.*)`
- Service name: `go-ecommerce-service` → `go-order-service`

- [ ] **Step 9: Update kustomization.yaml**

Read `go/k8s/kustomization.yaml`. Replace all ecommerce file references with order equivalents.

- [ ] **Step 10: Commit**

```bash
git add go/k8s/
git commit -m "refactor: rename ecommerce K8s manifests to order-service"
```

---

### Task 5: Update QA overlay

**Files:**
- Modify: `k8s/overlays/qa-go/kustomization.yaml`

- [ ] **Step 1: Update QA overlay**

Read `k8s/overlays/qa-go/kustomization.yaml`. Replace:
- `name: ecommerce-service-config` → `name: order-service-config`
- `ECOMMERCE_URL` → `ORDER_URL` and `http://go-ecommerce-service:8092` → `http://go-order-service:8092` in ai-service patch

- [ ] **Step 2: Commit**

```bash
git add k8s/overlays/qa-go/
git commit -m "refactor: update QA overlay for order-service rename"
```

---

### Task 6: Update CI pipeline

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `.github/workflows/aws-deploy.yml` (if exists)
- Modify: `Makefile`
- Modify: `k8s/deploy.sh`
- Modify: `.pre-commit-config.yaml`

- [ ] **Step 1: Update ci.yml**

Read `.github/workflows/ci.yml`. Replace all `ecommerce-service` references:
- `go-lint` matrix: `ecommerce-service` → `order-service`
- `go-tests` matrix: `ecommerce-service` → `order-service`
- `go-migration-test` section: paths to `go/ecommerce-service/migrations` → `go/order-service/migrations`, seed path
- `security-hadolint` matrix: `go/ecommerce-service/Dockerfile` → `go/order-service/Dockerfile`
- `build-images` matrix: service name `go-ecommerce-service` → `go-order-service`, file path, image name, paths
- QA deploy: `go-ecommerce-migrate` → `go-order-migrate`, job file path `ecommerce-service-migrate.yml` → `order-service-migrate.yml`
- Prod deploy: same renames
- Comments about ecommerce-service

- [ ] **Step 2: Update aws-deploy.yml**

Read `.github/workflows/aws-deploy.yml`. Replace `ecommerce-service` references in service name, Dockerfile path, ECR repo, and migration job sections.

- [ ] **Step 3: Update Makefile**

Read `Makefile`. Replace `go/ecommerce-service` with `go/order-service` in `preflight-go` and `preflight-go-integration` targets.

- [ ] **Step 4: Update deploy.sh**

Read `k8s/deploy.sh`. Replace `go-ecommerce-service` with `go-order-service` in kubectl wait commands (both QA and prod sections). Update the comment about `/go-api` → `/go-orders`.

- [ ] **Step 5: Update .pre-commit-config.yaml**

Read `.pre-commit-config.yaml`. Replace `ecommerce-service` with `order-service` in the golangci-lint hook path.

- [ ] **Step 6: Commit**

```bash
git add .github/workflows/ Makefile k8s/deploy.sh .pre-commit-config.yaml
git commit -m "ci: rename ecommerce-service to order-service in CI/CD pipeline"
```

---

### Task 7: Update frontend

**Files:**
- Modify: `frontend/.env.local`
- Modify: `frontend/src/lib/go-auth.ts`
- Rename: `frontend/src/lib/go-api.ts` → `frontend/src/lib/go-order-api.ts`
- Modify: All files that import from `go-api.ts`
- Modify: `frontend/src/components/go/GoStoreProvider.tsx`
- Modify: `frontend/src/app/go/ecommerce/layout.tsx`
- Modify: `frontend/src/app/go/page.tsx` (diagrams)

- [ ] **Step 1: Update .env.local**

Change `NEXT_PUBLIC_GO_ECOMMERCE_URL=http://localhost:8092` to `NEXT_PUBLIC_GO_ORDER_URL=http://localhost:8092`.

- [ ] **Step 2: Update go-auth.ts**

Change `GO_ECOMMERCE_URL` export to `GO_ORDER_URL` and `NEXT_PUBLIC_GO_ECOMMERCE_URL` to `NEXT_PUBLIC_GO_ORDER_URL`.

- [ ] **Step 3: Rename go-api.ts to go-order-api.ts**

```bash
mv frontend/src/lib/go-api.ts frontend/src/lib/go-order-api.ts
```

Update the file contents: import `GO_ORDER_URL` instead of `GO_ECOMMERCE_URL`, rename the function from `goApiFetch` to `goOrderFetch`, use `GO_ORDER_URL` in fetch calls.

- [ ] **Step 4: Update all imports of go-api.ts**

Search for all files importing from `@/lib/go-api` and update to `@/lib/go-order-api`. Also rename `goApiFetch` to `goOrderFetch` at each call site. Key files:
- `frontend/src/app/go/ecommerce/cart/page.tsx` (uses `goApiFetch` for `/orders`)
- `frontend/src/app/go/ecommerce/orders/page.tsx`
- Any other files importing `goApiFetch`

- [ ] **Step 5: Update GoStoreProvider.tsx**

Read `frontend/src/components/go/GoStoreProvider.tsx`. Replace `GO_ECOMMERCE_URL` import and usage with `GO_ORDER_URL`. This component fetches products — but products now come from product-service. Check if it's already using `GO_PRODUCT_URL` and only update if it still references `GO_ECOMMERCE_URL`.

- [ ] **Step 6: Update layout.tsx**

Read `frontend/src/app/go/ecommerce/layout.tsx`. Replace `NEXT_PUBLIC_GO_ECOMMERCE_URL` with `NEXT_PUBLIC_GO_ORDER_URL` if referenced.

- [ ] **Step 7: Update architecture diagrams**

Read `frontend/src/app/go/page.tsx`. Update mermaid diagram references from `ecommerce-service` to `order-service` and `/go-api` to `/go-orders`.

- [ ] **Step 8: Run frontend checks**

```bash
cd frontend && npx tsc --noEmit && npm run lint
```

- [ ] **Step 9: Commit**

```bash
git add frontend/
git commit -m "feat(frontend): rename ecommerce URLs to order-service"
```

---

### Task 8: Update smoke tests and load tests

**Files:**
- Modify: `frontend/e2e/smoke-prod/smoke.spec.ts`
- Modify: `loadtest/lib/helpers.js`
- Modify: `loadtest/scripts/phase1-ecommerce.js`

- [ ] **Step 1: Update smoke tests**

Read `frontend/e2e/smoke-prod/smoke.spec.ts`. Replace `/go-api/orders` with `/go-orders/orders`.

- [ ] **Step 2: Update load test helpers**

Read `loadtest/lib/helpers.js`. Replace `ECOMMERCE_URL = \`${BASE_URL}/go-api\`` with `ORDER_URL = \`${BASE_URL}/go-orders\``. Update the comment.

- [ ] **Step 3: Update load test scripts**

Read `loadtest/scripts/phase1-ecommerce.js`. Replace all `ECOMMERCE_URL` imports and usages with `ORDER_URL`.

- [ ] **Step 4: Commit**

```bash
git add frontend/e2e/ loadtest/
git commit -m "test: update smoke and load tests for order-service rename"
```

---

### Task 9: Update monitoring and Grafana alerts

**Files:**
- Modify: `k8s/monitoring/configmaps/grafana-alerting.yml`

- [ ] **Step 1: Update alert queries**

Read `k8s/monitoring/configmaps/grafana-alerting.yml`. Replace `service="go-ecommerce-service"` with `service="go-order-service"` in alert rule queries.

- [ ] **Step 2: Commit**

```bash
git add k8s/monitoring/
git commit -m "fix(monitoring): update Grafana alerts for order-service rename"
```

---

### Task 10: Run preflight and push

- [ ] **Step 1: Run full preflight**

```bash
cd go/order-service && go mod tidy && go build ./cmd/server && go test ./... -v -race
cd go/ai-service && go mod tidy && go build ./cmd/server
cd go/cart-service && go mod tidy && go build ./cmd/server
cd frontend && npx tsc --noEmit && npm run lint
```

- [ ] **Step 2: Fix any issues**

Address lint errors, test failures, or build errors from the rename.

- [ ] **Step 3: Push and create PR to qa**

```bash
git push -u origin agent/feat-order-service-saga
gh pr create --base qa --title "refactor: rename ecommerce-service to order-service (Phase 3a)" --body "$(cat <<'EOF'
## Summary
- Renames ecommerce-service to order-service across the entire codebase
- Ingress path changes from /go-api to /go-orders
- No logic changes — purely mechanical rename
- Database names (ecommercedb) and migration table unchanged

## Test plan
- [ ] All Go services compile (order, cart, product, ai, auth, analytics)
- [ ] Order-service tests pass
- [ ] Frontend tsc + lint clean
- [ ] Smoke tests pass with /go-orders paths
- [ ] CI matrices reference order-service

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 4: Watch CI and debug failures**

- [ ] **Step 5: After QA is green, notify Kyle**

**IMPORTANT:** Add `NEXT_PUBLIC_GO_ORDER_URL` to Vercel (both production and preview) before merging to main. Value: `https://api.kylebradshaw.dev/go-orders`
