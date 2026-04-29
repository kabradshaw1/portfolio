# Cart Service DSN Split Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move cart-service PostgreSQL and RabbitMQ credentials out of ConfigMaps by assembling URLs from ConfigMap geometry plus Secret credentials.

**Architecture:** Reuse the order-projector Phase 4 pattern for Postgres and apply the same component split to RabbitMQ. `cart-service-config` owns host, port, database name, URL options, and RabbitMQ host/port/vhost; new `cart-service-db` and `cart-service-mq` Secrets own credentials; app startup and the migrate Job assemble final URLs.

**Tech Stack:** Go, pgxpool, Kubernetes ConfigMaps/Deployments/Jobs, Sealed Secrets, Kustomize QA overlay, shell policy checks.

---

### Task 1: Cart Config DSN Builder

**Files:**
- Create: `go/cart-service/cmd/server/config_test.go`
- Modify: `go/cart-service/cmd/server/config.go`

- [ ] **Step 1: Write the failing test**

Add tests for `buildDatabaseURL()` and `buildRabbitMQURL()` covering populated components, omitted options/vhost, URL-special password characters, and missing required values.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go/cart-service && go test ./cmd/server`

Expected: FAIL because `buildDatabaseURL` and `buildRabbitMQURL` are undefined.

- [ ] **Step 3: Write minimal implementation**

Add `buildDatabaseURL()` and `buildRabbitMQURL()` to `config.go`, using URL escaping for user/password fields. Change `loadConfig()` to call them and keep returning `Config.DatabaseURL` and `Config.RabbitmqURL`.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd go/cart-service && go test ./cmd/server`

Expected: PASS.

### Task 2: Kubernetes Runtime Manifests

**Files:**
- Modify: `go/k8s/configmaps/cart-service-config.yml`
- Modify: `go/k8s/deployments/cart-service.yml`
- Modify: `go/k8s/jobs/cart-service-migrate.yml`

- [ ] **Step 1: Replace credential URLs with DB components**

In `cart-service-config`, remove `DATABASE_URL`, `DATABASE_URL_DIRECT`, and `RABBITMQ_URL`; add `DB_HOST`, `DB_PORT`, `DB_NAME`, `DB_OPTIONS`, `DB_HOST_DIRECT`, `DB_PORT_DIRECT`, `DB_OPTIONS_DIRECT`, `MQ_HOST`, `MQ_PORT`, and `MQ_VHOST`.

- [ ] **Step 2: Wire the runtime Secret**

Add `cart-service-db` and `cart-service-mq` to the deployment `envFrom` list after `cart-service-config`.

- [ ] **Step 3: Wire migration Job assembly**

Change the migrate Job to read `cart-service-config` and `cart-service-db` via `envFrom`, then assemble `DATABASE_URL` inside the shell command with the `_DIRECT` host/port/options values.

### Task 3: QA Overlay And Policy

**Files:**
- Modify: `k8s/overlays/qa-go/kustomization.yaml`
- Modify: `scripts/k8s-policy-check.sh`

- [ ] **Step 1: Narrow QA database patch**

Replace the cart-service QA `DATABASE_URL` and `DATABASE_URL_DIRECT` patches with one patch for `/data/DB_NAME` set to `cartdb_qa`.

- [ ] **Step 2: Keep QA RabbitMQ patch in place**

Replace the cart-service QA `RABBITMQ_URL` patch with one patch for `/data/MQ_VHOST` set to `qa`.

- [ ] **Step 3: Remove cart-service from R3 allowlist**

Delete `go/k8s/configmaps/cart-service-config.yml` from `R3_ALLOWLIST`.

### Task 4: Verification

**Files:**
- No edits.

- [ ] **Step 1: Run focused Go tests**

Run: `cd go/cart-service && go test ./cmd/server`

- [ ] **Step 2: Run policy tests**

Run: `bash scripts/test-k8s-policy-check.sh`

- [ ] **Step 3: Run policy check**

Run: `bash scripts/k8s-policy-check.sh`

- [ ] **Step 4: Check diff**

Run: `git diff --check` and `git status --short`.

### Deployment Note

Before this can deploy, create and seal `cart-service-db` and `cart-service-mq` in both `go-ecommerce` and `go-ecommerce-qa`, matching the order-projector secret pattern:

- `k8s/secrets/go-ecommerce/cart-service-db.sealed.yml`
- `k8s/secrets/go-ecommerce-qa/cart-service-db.sealed.yml`
- `k8s/secrets/go-ecommerce/cart-service-mq.sealed.yml`
- `k8s/secrets/go-ecommerce-qa/cart-service-mq.sealed.yml`
