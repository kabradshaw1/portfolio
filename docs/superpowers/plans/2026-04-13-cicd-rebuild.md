# CI/CD Pipeline Rebuild & QA Environment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace three GitHub Actions workflows with one unified workflow, add a QA environment with separate Minikube namespaces, and update CLAUDE.md with the new agent-driven workflow.

**Architecture:** Single `ci.yml` handles PR-to-qa (checks only), push-to-qa (checks + build + deploy QA + smoke), and push-to-main (checks + build + deploy prod + smoke). QA uses separate K8s namespaces sharing prod database instances but with separate databases. Kustomize QA overlays patch namespaces, CORS, DB names, and ingress hosts.

**Tech Stack:** GitHub Actions, Kustomize, Kubernetes (Minikube), Cloudflare Tunnel, Vercel, GHCR

**Spec:** `docs/superpowers/specs/2026-04-13-cicd-rebuild-design.md`

---

## File Structure

### New Files

| File | Responsibility |
|------|---------------|
| `.github/workflows/ci.yml` | Unified workflow (rewrite of existing) |
| `k8s/overlays/qa/kustomization.yaml` | QA overlay for Python AI services |
| `k8s/overlays/qa/namespace.yml` | ai-services-qa namespace |
| `k8s/overlays/qa/ollama.yml` | Ollama ExternalName in QA namespace |
| `java/k8s/overlays/qa/kustomization.yaml` | QA overlay for Java services |
| `java/k8s/overlays/qa/namespace.yml` | java-tasks-qa namespace |
| `go/k8s/overlays/qa/kustomization.yaml` | QA overlay for Go services |
| `go/k8s/overlays/qa/namespace.yml` | go-ecommerce-qa namespace |
| `docs/adr/cicd-rebuild-qa-environment.md` | ADR documenting design decisions |
| `docs/qa-workflow-guide.md` | Step-by-step workflow guide for Kyle |

### Modified Files

| File | Change |
|------|--------|
| `CLAUDE.md` | New branching model, agent push rules, worktree conventions |
| `.gitignore` | Add `.claude/worktrees/` |
| `Makefile` | Add `worktree-cleanup` target |
| `k8s/deploy.sh` | Accept `qa` as third environment |
| `docs/adr/cicd-pipeline.md` | Mark as superseded by new ADR |

### Deleted Files

| File | Reason |
|------|--------|
| `.github/workflows/java-ci.yml` | Replaced by unified ci.yml |
| `.github/workflows/go-ci.yml` | Replaced by unified ci.yml |

---

## Task 1: QA Kustomize Overlay — Python AI Services

**Files:**
- Create: `k8s/overlays/qa/kustomization.yaml`
- Create: `k8s/overlays/qa/namespace.yml`
- Create: `k8s/overlays/qa/ollama.yml`

- [ ] **Step 1: Create the QA namespace manifest**

```yaml
# k8s/overlays/qa/namespace.yml
apiVersion: v1
kind: Namespace
metadata:
  name: ai-services-qa
```

- [ ] **Step 2: Create the Ollama ExternalName service for QA namespace**

The prod overlay at `k8s/overlays/minikube/ollama.yml` defines an ExternalName service in `ai-services`. QA needs the same in `ai-services-qa`.

```yaml
# k8s/overlays/qa/ollama.yml
apiVersion: v1
kind: Service
metadata:
  name: ollama
  namespace: ai-services-qa
spec:
  type: ExternalName
  externalName: host.minikube.internal
```

- [ ] **Step 3: Create the QA Kustomize overlay**

This overlay references the base `ai-services` resources, overrides the namespace, patches CORS and Qdrant collection name, and adds the QA host to ingress.

```yaml
# k8s/overlays/qa/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - ../../ai-services
  - namespace.yml
  - ollama.yml

namespace: ai-services-qa

patches:
  # --- CORS: allow QA frontend ---
  - target:
      kind: ConfigMap
      name: ingestion-config
    patch: |
      - op: replace
        path: /data/ALLOWED_ORIGINS
        value: "https://qa.kylebradshaw.dev,http://localhost:3000"
      - op: replace
        path: /data/COLLECTION_NAME
        value: "documents_qa"
      - op: replace
        path: /data/QDRANT_HOST
        value: "qdrant.ai-services.svc.cluster.local"
  - target:
      kind: ConfigMap
      name: chat-config
    patch: |
      - op: replace
        path: /data/ALLOWED_ORIGINS
        value: "https://qa.kylebradshaw.dev,http://localhost:3000"
      - op: replace
        path: /data/COLLECTION_NAME
        value: "documents_qa"
      - op: replace
        path: /data/QDRANT_HOST
        value: "qdrant.ai-services.svc.cluster.local"
  - target:
      kind: ConfigMap
      name: debug-config
    patch: |
      - op: replace
        path: /data/ALLOWED_ORIGINS
        value: "https://qa.kylebradshaw.dev,http://localhost:3000"
      - op: replace
        path: /data/QDRANT_HOST
        value: "qdrant.ai-services.svc.cluster.local"
  # --- Ingress: QA host ---
  - target:
      kind: Ingress
      name: ai-services-ingress
    patch: |
      - op: add
        path: /spec/rules/0/host
        value: "qa-api.kylebradshaw.dev"
  # --- Remove Qdrant deployment (shared with prod) ---
  - target:
      kind: Deployment
      name: qdrant
    patch: |
      $patch: delete
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: qdrant
  - target:
      kind: Service
      name: qdrant
    patch: |
      - op: replace
        path: /spec
        value:
          type: ExternalName
          externalName: qdrant.ai-services.svc.cluster.local
```

- [ ] **Step 4: Validate the overlay builds**

Run: `kubectl kustomize k8s/overlays/qa/ | head -100`

Expected: YAML output with namespace `ai-services-qa`, ConfigMaps showing `ALLOWED_ORIGINS: https://qa.kylebradshaw.dev,http://localhost:3000`, ingress with `host: qa-api.kylebradshaw.dev`.

- [ ] **Step 5: Commit**

```bash
git add k8s/overlays/qa/
git commit -m "k8s: add QA Kustomize overlay for Python AI services"
```

---

## Task 2: QA Kustomize Overlay — Java Services

**Files:**
- Create: `java/k8s/overlays/qa/kustomization.yaml`
- Create: `java/k8s/overlays/qa/namespace.yml`

- [ ] **Step 1: Create the QA namespace manifest**

```yaml
# java/k8s/overlays/qa/namespace.yml
apiVersion: v1
kind: Namespace
metadata:
  name: java-tasks-qa
```

- [ ] **Step 2: Create the QA Kustomize overlay**

Java QA shares the infrastructure pods (postgres, mongodb, redis, rabbitmq) from `java-tasks` namespace via cross-namespace DNS. Only application services are deployed in QA.

```yaml
# java/k8s/overlays/qa/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - ../../
  - namespace.yml

namespace: java-tasks-qa

patches:
  # --- CORS ---
  - target:
      kind: ConfigMap
      name: gateway-service-config
    patch: |
      - op: replace
        path: /data/ALLOWED_ORIGINS
        value: "https://qa.kylebradshaw.dev,http://localhost:3000"
  - target:
      kind: ConfigMap
      name: task-service-config
    patch: |
      - op: replace
        path: /data/ALLOWED_ORIGINS
        value: "https://qa.kylebradshaw.dev,http://localhost:3000"
      - op: replace
        path: /data/POSTGRES_HOST
        value: "postgres.java-tasks.svc.cluster.local"
      - op: replace
        path: /data/RABBITMQ_HOST
        value: "rabbitmq.java-tasks.svc.cluster.local"
      - op: replace
        path: /data/REDIS_HOST
        value: "redis.java-tasks.svc.cluster.local"
  - target:
      kind: ConfigMap
      name: activity-service-config
    patch: |
      - op: replace
        path: /data/MONGODB_HOST
        value: "mongodb.java-tasks.svc.cluster.local"
      - op: replace
        path: /data/RABBITMQ_HOST
        value: "rabbitmq.java-tasks.svc.cluster.local"
  - target:
      kind: ConfigMap
      name: notification-service-config
    patch: |
      - op: replace
        path: /data/REDIS_HOST
        value: "redis.java-tasks.svc.cluster.local"
      - op: replace
        path: /data/RABBITMQ_HOST
        value: "rabbitmq.java-tasks.svc.cluster.local"
  - target:
      kind: ConfigMap
      name: gateway-service-config
    patch: |
      - op: replace
        path: /data/REDIS_HOST
        value: "redis.java-tasks.svc.cluster.local"
  # --- Ingress: QA host ---
  - target:
      kind: Ingress
      name: java-tasks-ingress
    patch: |
      - op: add
        path: /spec/rules/0/host
        value: "qa-api.kylebradshaw.dev"
  # --- Remove infrastructure deployments (shared with prod) ---
  - target:
      kind: Deployment
      name: postgres
    patch: |
      $patch: delete
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: postgres
  - target:
      kind: Deployment
      name: mongodb
    patch: |
      $patch: delete
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: mongodb
  - target:
      kind: Deployment
      name: redis
    patch: |
      $patch: delete
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: redis
  - target:
      kind: Deployment
      name: rabbitmq
    patch: |
      $patch: delete
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: rabbitmq
  # --- Remove infrastructure services (replace with ExternalName) ---
  - target:
      kind: Service
      name: postgres
    patch: |
      - op: replace
        path: /spec
        value:
          type: ExternalName
          externalName: postgres.java-tasks.svc.cluster.local
  - target:
      kind: Service
      name: mongodb
    patch: |
      - op: replace
        path: /spec
        value:
          type: ExternalName
          externalName: mongodb.java-tasks.svc.cluster.local
  - target:
      kind: Service
      name: redis
    patch: |
      - op: replace
        path: /spec
        value:
          type: ExternalName
          externalName: redis.java-tasks.svc.cluster.local
  - target:
      kind: Service
      name: rabbitmq
    patch: |
      - op: replace
        path: /spec
        value:
          type: ExternalName
          externalName: rabbitmq.java-tasks.svc.cluster.local
  # --- Remove PVC (no local storage needed) ---
  - target:
      kind: PersistentVolumeClaim
      name: postgres-pvc
    patch: |
      $patch: delete
      apiVersion: v1
      kind: PersistentVolumeClaim
      metadata:
        name: postgres-pvc
  # --- Remove rabbitmq ingress (not needed for QA) ---
  - target:
      kind: Ingress
      name: rabbitmq-ingress
    patch: |
      $patch: delete
      apiVersion: networking.k8s.io/v1
      kind: Ingress
      metadata:
        name: rabbitmq-ingress
```

- [ ] **Step 3: Validate the overlay builds**

Run: `kubectl kustomize java/k8s/overlays/qa/ | head -100`

Expected: YAML with namespace `java-tasks-qa`, cross-namespace DNS for infrastructure, QA CORS origins.

- [ ] **Step 4: Commit**

```bash
git add java/k8s/overlays/qa/
git commit -m "k8s: add QA Kustomize overlay for Java services"
```

---

## Task 3: QA Kustomize Overlay — Go Services

**Files:**
- Create: `go/k8s/overlays/qa/kustomization.yaml`
- Create: `go/k8s/overlays/qa/namespace.yml`

- [ ] **Step 1: Create the QA namespace manifest**

```yaml
# go/k8s/overlays/qa/namespace.yml
apiVersion: v1
kind: Namespace
metadata:
  name: go-ecommerce-qa
```

- [ ] **Step 2: Create the QA Kustomize overlay**

```yaml
# go/k8s/overlays/qa/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - ../../
  - namespace.yml

namespace: go-ecommerce-qa

patches:
  # --- CORS + QA database ---
  - target:
      kind: ConfigMap
      name: auth-service-config
    patch: |
      - op: replace
        path: /data/ALLOWED_ORIGINS
        value: "https://qa.kylebradshaw.dev,http://localhost:3000"
      - op: replace
        path: /data/DATABASE_URL
        value: "postgres://taskuser:taskpass@postgres.java-tasks.svc.cluster.local:5432/ecommercedb_qa?sslmode=disable"
  - target:
      kind: ConfigMap
      name: ecommerce-service-config
    patch: |
      - op: replace
        path: /data/ALLOWED_ORIGINS
        value: "https://qa.kylebradshaw.dev,http://localhost:3000"
      - op: replace
        path: /data/DATABASE_URL
        value: "postgres://taskuser:taskpass@postgres.java-tasks.svc.cluster.local:5432/ecommercedb_qa?sslmode=disable"
      - op: replace
        path: /data/REDIS_URL
        value: "redis://redis.java-tasks.svc.cluster.local:6379/1"
      - op: replace
        path: /data/RABBITMQ_URL
        value: "amqp://guest:guest@rabbitmq.java-tasks.svc.cluster.local:5672"
  - target:
      kind: ConfigMap
      name: ai-service-config
    patch: |
      - op: replace
        path: /data/ALLOWED_ORIGINS
        value: "https://qa.kylebradshaw.dev,http://localhost:3000"
      - op: replace
        path: /data/OLLAMA_URL
        value: "http://ollama.ai-services.svc.cluster.local:11434"
      - op: replace
        path: /data/ECOMMERCE_URL
        value: "http://go-ecommerce-service:8092"
      - op: replace
        path: /data/REDIS_URL
        value: "redis://redis.java-tasks.svc.cluster.local:6379/1"
  # --- Ingress: QA host ---
  - target:
      kind: Ingress
      name: go-ecommerce-ingress
    patch: |
      - op: add
        path: /spec/rules/0/host
        value: "qa-api.kylebradshaw.dev"
  # --- Migration jobs: point at QA database ---
  - target:
      kind: Job
      name: go-auth-migrate
    patch: |
      - op: replace
        path: /spec/template/spec/containers/0/env/0/value
        value: "postgres://taskuser:taskpass@postgres.java-tasks.svc.cluster.local:5432/ecommercedb_qa?sslmode=disable"
  - target:
      kind: Job
      name: go-ecommerce-migrate
    patch: |
      - op: replace
        path: /spec/template/spec/containers/0/env/0/value
        value: "postgres://taskuser:taskpass@postgres.java-tasks.svc.cluster.local:5432/ecommercedb_qa?sslmode=disable"
```

- [ ] **Step 3: Validate the overlay builds**

Run: `kubectl kustomize go/k8s/overlays/qa/ | head -100`

Expected: YAML with namespace `go-ecommerce-qa`, `ecommercedb_qa` in DATABASE_URLs, Redis DB 1, QA CORS.

- [ ] **Step 4: Commit**

```bash
git add go/k8s/overlays/qa/
git commit -m "k8s: add QA Kustomize overlay for Go services"
```

---

## Task 4: Extend deploy.sh for QA

**Files:**
- Modify: `k8s/deploy.sh`

- [ ] **Step 1: Update deploy.sh to accept `qa` environment**

The script currently accepts `minikube` or `aws`. Add `qa` as a third option. QA deploys to separate namespaces, skips monitoring, and creates the `ecommercedb_qa` database if it doesn't exist.

In `k8s/deploy.sh`, replace the environment validation block and add QA-specific logic:

Replace the existing validation (lines 14-17):
```bash
if [ "$ENV" != "minikube" ] && [ "$ENV" != "aws" ]; then
  echo "Usage: $0 [minikube|aws]"
  exit 1
fi
```

With:
```bash
if [ "$ENV" != "minikube" ] && [ "$ENV" != "aws" ] && [ "$ENV" != "qa" ]; then
  echo "Usage: $0 [minikube|aws|qa]"
  exit 1
fi
```

Then, after the existing Minikube-specific setup block (line 25), add the QA deploy path. When `ENV=qa`, the script should:
1. Apply QA Kustomize overlays (not base manifests)
2. Skip monitoring (shared with prod)
3. Create `ecommercedb_qa` database on the existing PostgreSQL if it doesn't exist
4. Wait for QA application services only

Add this block after the Minikube ingress setup:

```bash
# --- QA-specific deploy ---
if [ "$ENV" = "qa" ]; then
  echo "==> Creating QA database (ecommercedb_qa) if not exists..."
  kubectl exec deployment/postgres -n java-tasks -- \
    psql -U taskuser -d taskdb -c "SELECT 1 FROM pg_database WHERE datname='ecommercedb_qa'" | grep -q 1 || \
    kubectl exec deployment/postgres -n java-tasks -- \
      psql -U taskuser -d taskdb -c "CREATE DATABASE ecommercedb_qa;"

  echo "==> Deploying ai-services-qa..."
  kubectl apply -k "$SCRIPT_DIR/overlays/qa"

  echo "==> Deploying java-tasks-qa..."
  kubectl apply -k "$REPO_DIR/java/k8s/overlays/qa"

  echo "==> Deploying go-ecommerce-qa..."
  kubectl apply -k "$REPO_DIR/go/k8s/overlays/qa"

  echo "==> Waiting for QA application services..."
  kubectl wait --for=condition=available --timeout=180s deployment/ingestion -n ai-services-qa
  kubectl wait --for=condition=available --timeout=180s deployment/chat -n ai-services-qa
  kubectl wait --for=condition=available --timeout=180s deployment/debug -n ai-services-qa
  kubectl wait --for=condition=available --timeout=180s deployment/task-service -n java-tasks-qa
  kubectl wait --for=condition=available --timeout=180s deployment/activity-service -n java-tasks-qa
  kubectl wait --for=condition=available --timeout=180s deployment/notification-service -n java-tasks-qa
  kubectl wait --for=condition=available --timeout=180s deployment/gateway-service -n java-tasks-qa
  kubectl wait --for=condition=available --timeout=180s deployment/go-auth-service -n go-ecommerce-qa
  kubectl wait --for=condition=available --timeout=180s deployment/go-ecommerce-service -n go-ecommerce-qa
  kubectl wait --for=condition=available --timeout=180s deployment/go-ai-service -n go-ecommerce-qa

  echo ""
  echo "==> QA environment deployed!"
  echo "    Backend: qa-api.kylebradshaw.dev"
  echo "    Frontend: qa.kylebradshaw.dev"
  exit 0
fi
```

- [ ] **Step 2: Verify the updated script parses correctly**

Run: `bash -n k8s/deploy.sh`

Expected: No output (no syntax errors).

- [ ] **Step 3: Commit**

```bash
git add k8s/deploy.sh
git commit -m "k8s: extend deploy.sh to support QA environment"
```

---

## Task 5: Unified GitHub Actions Workflow

**Files:**
- Rewrite: `.github/workflows/ci.yml`
- Delete: `.github/workflows/java-ci.yml`
- Delete: `.github/workflows/go-ci.yml`

- [ ] **Step 1: Write the unified workflow**

This is the largest file (~600 lines). It consolidates all jobs from `ci.yml`, `java-ci.yml`, and `go-ci.yml` into a single workflow with conditional deploy jobs. Reference the existing files for exact job definitions — copy them, remove `needs: [changes]` dependencies and `if: needs.changes.outputs.*` conditionals, then add the new deploy/smoke jobs.

Key changes from current `ci.yml`:
- Triggers: `pull_request: [qa]` + `push: [qa, main]` (replaces `push: ["**"]` + `pull_request: [main]`)
- Remove `changes` job and all `if: needs.changes.outputs.*` conditionals — all checks always run
- Remove `e2e-staging` job (staging branch retired)
- Add Go lint/test jobs (from `go-ci.yml`)
- Add Java integration tests (from `java-ci.yml`)
- Consolidate all Docker builds into one matrix job
- Add `deploy-qa` job (conditional on push to qa)
- Rename `deploy` to `deploy-prod` (conditional on push to main)
- Add `smoke-qa` job

Write the complete workflow to `.github/workflows/ci.yml`. The file will be large (~600 lines). Key sections:

**Triggers:**
```yaml
name: CI/CD

on:
  pull_request:
    branches: [qa]
  push:
    branches: [qa, main]
```

**Quality gates** (no `needs: changes` dependency, no `if` conditionals — all run always):
- `python-lint` — ruff check + format
- `python-tests` — matrix pytest (ingestion, chat, debug)
- `java-lint` — checkstyle
- `java-unit-tests` — matrix (task, activity, notification, gateway)
- `java-integration-tests` — Gradle integrationTest
- `go-lint` — matrix golangci-lint (auth, ecommerce, ai-service)
- `go-tests` — matrix go test -race (auth, ecommerce, ai-service)
- `go-migration-test` — PostgreSQL + golang-migrate
- `frontend-checks` — lint, tsc, build
- `grafana-dashboard-sync` — dashboard JSON sync
- `k8s-manifest-validation` — kubeconform + kind dry-run + policy
- `compose-smoke` — Docker Compose smoke with mock-ollama
- `security-bandit`, `security-pip-audit`, `security-npm-audit`, `security-gitleaks`, `security-hadolint`, `security-cors-check`

**Build images** (conditional: push only):
```yaml
build-images:
  name: Build Image (${{ matrix.service }})
  runs-on: ubuntu-latest
  if: github.event_name == 'push'
  needs: [python-lint, python-tests, java-lint, java-unit-tests, go-lint, go-tests, frontend-checks, security-gitleaks, security-hadolint]
  permissions:
    packages: write
  strategy:
    matrix:
      include:
        - service: ingestion
          context: services
          file: services/ingestion/Dockerfile
          image: ingestion
        - service: chat
          context: services
          file: services/chat/Dockerfile
          image: chat
        - service: debug
          context: services
          file: services/debug/Dockerfile
          image: debug
        - service: java-task-service
          context: java/task-service
          file: java/task-service/Dockerfile
          image: java-task-service
          pre-build: cd java && ./gradlew :task-service:bootJar --no-daemon
        - service: java-activity-service
          context: java/activity-service
          file: java/activity-service/Dockerfile
          image: java-activity-service
          pre-build: cd java && ./gradlew :activity-service:bootJar --no-daemon
        - service: java-notification-service
          context: java/notification-service
          file: java/notification-service/Dockerfile
          image: java-notification-service
          pre-build: cd java && ./gradlew :notification-service:bootJar --no-daemon
        - service: java-gateway-service
          context: java/gateway-service
          file: java/gateway-service/Dockerfile
          image: java-gateway-service
          pre-build: cd java && ./gradlew :gateway-service:bootJar --no-daemon
        - service: go-auth-service
          context: go
          file: go/auth-service/Dockerfile
          image: go-auth-service
        - service: go-ecommerce-service
          context: go
          file: go/ecommerce-service/Dockerfile
          image: go-ecommerce-service
        - service: go-ai-service
          context: go
          file: go/ai-service/Dockerfile
          image: go-ai-service
```

Image tags:
- Push to `qa`: `ghcr.io/${{ github.repository }}/${{ matrix.image }}:qa-${{ github.sha }}`
- Push to `main`: `ghcr.io/${{ github.repository }}/${{ matrix.image }}:latest`

**Deploy QA** (conditional: push to qa):
```yaml
deploy-qa:
  name: Deploy QA
  runs-on: ubuntu-latest
  needs: [build-images]
  if: github.event_name == 'push' && github.ref == 'refs/heads/qa'
```

Uses the same SSH + Tailscale pattern as current deploy job, but runs `kubectl apply -k` with the QA overlays and restarts QA namespace deployments.

**Deploy prod** (conditional: push to main):
Preserves the existing `deploy` job logic exactly (SSH, selective restarts based on change detection). Add the `changes` detection job back but ONLY as a dependency of `deploy-prod` — not of quality gates.

**Smoke tests** (one for QA, one for prod):
```yaml
smoke-qa:
  if: github.event_name == 'push' && github.ref == 'refs/heads/qa'
  # Hit qa-api.kylebradshaw.dev health endpoints

smoke-prod:
  if: github.event_name == 'push' && github.ref == 'refs/heads/main'
  # Existing production smoke tests (Playwright)
```

- [ ] **Step 2: Delete the old workflow files**

```bash
git rm .github/workflows/java-ci.yml
git rm .github/workflows/go-ci.yml
```

- [ ] **Step 3: Validate workflow YAML syntax**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"`

Expected: No errors.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: unified workflow replacing ci.yml, java-ci.yml, go-ci.yml

Single workflow handles PR checks (qa), QA deploy (push to qa),
and production deploy (push to main). Removes change detection
for quality gates — all checks always run."
```

---

## Task 6: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Replace the Branching & Workflow section**

Replace the existing "Branching & Workflow" section (lines 140-157) with:

```markdown
## Branching & Workflow

- `main` — production. Pushes trigger deploy + post-deploy smoke tests.
- `qa` — pre-production QA environment. PRs trigger quality checks. Pushes trigger build + deploy to QA + smoke tests.
- Feature branches (`agent/feat-*`) — created by agents from `main`, short-lived, deleted after merge.
- `staging` — retired. Replaced by `qa`.

**Per-branch rules for Claude Code:**

- **On a feature branch:** implement, commit, push, create PR to `qa`.
- **On `qa`:** commit and push when Kyle asks. Watch CI after pushing and debug failures. For CI fixes: lint errors, formatting, type errors, and config issues are fine to fix autonomously. For anything that changes application behavior (logic, API contracts, data flow), stop and check with Kyle before fixing.
- **On `main`:** never push autonomously. When Kyle explicitly says to merge/ship to main, handle the full flow: merge `qa` into `main`, push, watch CI, debug minor failures, clean up worktree, delete feature branch (local + remote).

Claude Code determines the current branch via `git branch --show-current` and follows the rules for that branch. No special mode or prompt needed.

**Agent worktrees:** Agents create worktrees in `.claude/worktrees/<branch-name>/` for feature work. Worktrees are cleaned up as part of the "ship to main" flow.
```

- [ ] **Step 2: Update the CI/CD Pipeline section**

Replace the existing "CI/CD Pipeline" section (lines 172-184) with:

```markdown
## CI/CD Pipeline

Single unified workflow (`.github/workflows/ci.yml`) handles all CI/CD:

| Trigger | What runs |
|---------|-----------|
| PR to `qa` | All quality checks (lint, test, security, k8s validation) |
| Push to `qa` | Quality checks + build all images + deploy to QA + smoke QA |
| Push to `main` | Quality checks + build all images + deploy to prod + smoke prod |

**Quality:** ruff lint/format, pytest + coverage, tsc, Next.js build, checkstyle, golangci-lint, Go tests
**Security:** Bandit (SAST), pip-audit, npm audit, gitleaks, Hadolint, CORS guardrail
**Deploy:** GHCR images built in CI → SSH to Windows PC → kubectl apply Kustomize overlays
**QA:** `qa-api.kylebradshaw.dev` (backend), `qa.kylebradshaw.dev` (frontend on Vercel)

**Compose-smoke realism:** Job 3 (`compose-smoke`) runs the Python AI stack via `docker-compose.yml` with a mocked Ollama. Any change to Python service configuration (env vars, ports, depends_on, env_file references) must be reflected in BOTH `docker-compose.yml` and the corresponding k8s manifests under `k8s/ai-services/`, or compose-smoke will drift from prod and stop catching real regressions.

**Tailscale authkey:** Expires every 90 days (free plan). Regenerate at Tailscale admin → Keys and update `TAILSCALE_AUTHKEY` in GitHub repo secrets.
```

- [ ] **Step 3: Remove the "Do not use git worktrees" line**

Delete line 157: `**Do not use git worktrees or the EnterWorktree/ExitWorktree tools.**`

- [ ] **Step 4: Update the Minikube namespaces list in Infrastructure section**

Add QA namespaces to the Minikube section (after line 26):

```markdown
  - `ai-services-qa` namespace: QA copies of Python AI services (shared Qdrant with prod)
  - `java-tasks-qa` namespace: QA copies of Java services (shared infra with prod)
  - `go-ecommerce-qa` namespace: QA copies of Go services (shared infra with prod)
```

- [ ] **Step 5: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md with new branching model and agent workflow"
```

---

## Task 7: Update .gitignore and Makefile

**Files:**
- Modify: `.gitignore`
- Modify: `Makefile`

- [ ] **Step 1: Add worktree exclusion to .gitignore**

Add after the `.superpowers/` line (line 49):

```
# Agent worktrees
.claude/worktrees/
```

- [ ] **Step 2: Add worktree-cleanup target to Makefile**

Add at the end of the Makefile:

```makefile
# --- Worktree cleanup ---
worktree-cleanup:
	@echo "\n=== Cleaning up merged worktrees ==="
	@for wt in $$(git worktree list --porcelain | grep '^worktree' | awk '{print $$2}' | grep '.claude/worktrees'); do \
		branch=$$(git worktree list --porcelain | grep -A2 "$$wt" | grep '^branch' | sed 's|branch refs/heads/||'); \
		if [ -n "$$branch" ] && ! git rev-parse --verify "$$branch" >/dev/null 2>&1; then \
			echo "  Removing stale worktree: $$wt (branch $$branch deleted)"; \
			git worktree remove "$$wt" --force; \
		fi; \
	done
	@git worktree prune
	@echo "Done"
```

- [ ] **Step 3: Update .PHONY**

Add `worktree-cleanup` to the `.PHONY` declaration at line 1.

- [ ] **Step 4: Commit**

```bash
git add .gitignore Makefile
git commit -m "chore: add worktree exclusion and cleanup target"
```

---

## Task 8: Write the ADR

**Files:**
- Create: `docs/adr/cicd-rebuild-qa-environment.md`
- Modify: `docs/adr/cicd-pipeline.md` (mark as superseded)

- [ ] **Step 1: Write the ADR**

```markdown
# CI/CD Pipeline Rebuild and QA Environment

- **Date:** 2026-04-13
- **Status:** Accepted
- **Supersedes:** [CI/CD Pipeline](cicd-pipeline.md)

## Context

The portfolio project had three separate GitHub Actions workflows (`ci.yml`, `java-ci.yml`, `go-ci.yml`) and a staging branch. For a solo developer, the staging branch added no value — it re-ran the same checks that already passed on the feature branch, with no code review or manual gate in between. The multiple workflow files also made it harder to understand the full CI/CD picture.

The project needed:
1. A pre-production QA environment for visual inspection before shipping
2. Automated agent workflow to reduce manual steps
3. A simpler, unified CI/CD pipeline

## Decision

### Unified Workflow

Consolidated all three workflow files into a single `ci.yml` with three triggers:
- **PR to `qa`:** runs all quality checks (lint, test, security, K8s validation)
- **Push to `qa`:** runs checks + builds all Docker images + deploys to QA namespaces + smoke tests
- **Push to `main`:** runs checks + builds images + deploys to production + smoke tests

The staging branch is retired. The `qa` branch replaces it.

### QA Environment

QA runs in the same Minikube cluster as production using separate namespaces (`ai-services-qa`, `java-tasks-qa`, `go-ecommerce-qa`). QA shares database instances with production but uses separate databases (`ecommercedb_qa`, `taskdb_qa`, `documents_qa` collection). This avoids duplicating infrastructure pods (PostgreSQL, MongoDB, Redis, RabbitMQ, Qdrant) saving ~1.3GB of memory on the single Minikube node.

QA is publicly accessible:
- Backend: `qa-api.kylebradshaw.dev` via Cloudflare Tunnel
- Frontend: `qa.kylebradshaw.dev` via Vercel branch domain

### Agent Workflow

Agents create feature branches, implement changes in git worktrees, push, and create PRs targeting `qa`. After CI passes, Kyle reviews the PR, merges it, inspects the QA deployment, and tells Claude to ship it to main. Claude handles the merge, push, CI watch, and worktree cleanup.

### Why Kyle Pushes Directly to Main

This is a solo developer project. By the time code reaches `main`, it has already passed all quality checks (on the PR), been deployed to QA, and been visually inspected. Branch protection requiring a PR approval would be a single person approving their own PR — ceremony with no value. Kyle pushes directly to main (via Claude when told to "ship it") after reviewing QA.

## Consequences

**Positive:**
- Single workflow file is easier to understand and maintain
- QA environment catches visual and integration issues before production
- Agent automation reduces manual steps from ~8 to ~2 (review PR, say "ship it")
- Shared database instances keep Minikube resource usage manageable

**Trade-offs:**
- All quality checks run on every trigger (no path-based skipping). Slower but simpler and catches cross-stack issues.
- QA and production share database instances. A runaway QA query could theoretically affect production, but this is a portfolio project with no real traffic.
- QA is publicly accessible, which is intentional — it demonstrates the CI/CD pipeline as part of the portfolio.
```

- [ ] **Step 2: Mark the old ADR as superseded**

Add to the top of `docs/adr/cicd-pipeline.md`, after the title line:

```markdown
> **Superseded** by [CI/CD Pipeline Rebuild and QA Environment](cicd-rebuild-qa-environment.md) (2026-04-13)
```

- [ ] **Step 3: Commit**

```bash
git add docs/adr/cicd-rebuild-qa-environment.md docs/adr/cicd-pipeline.md
git commit -m "docs: ADR for CI/CD rebuild and QA environment"
```

---

## Task 9: Write the QA Workflow Guide

**Files:**
- Create: `docs/qa-workflow-guide.md`

- [ ] **Step 1: Write the workflow guide**

```markdown
# QA Workflow Guide

Step-by-step reference for reviewing and shipping changes through the QA pipeline.

## Normal Flow: Agent Delivers a Feature

1. **Agent notifies you:** "PR to `qa` is ready, CI passed. Review at [PR link]"
2. **Review the PR diff** on GitHub — check the code changes make sense
3. **Merge the PR** on GitHub — this triggers the QA build + deploy pipeline
4. **Wait for deploy** — watch the Actions tab or wait for the pipeline to finish (~5-10 min)
5. **Inspect QA** at [qa.kylebradshaw.dev](https://qa.kylebradshaw.dev) — click through the affected pages, verify the change works as expected
6. **Ship to production** — tell Claude "ship it" and Claude will:
   - Merge `qa` into `main`
   - Push `main`
   - Watch the production CI/deploy pipeline
   - Debug any minor failures (lint, config)
   - Clean up the worktree and delete the feature branch
   - Report back when production is live

## Tweaking QA

If something needs adjustment after inspecting QA:

1. `git checkout qa && git pull`
2. For **frontend changes:** run `npm run dev` in `frontend/` with QA backend env vars:
   ```bash
   NEXT_PUBLIC_API_URL=https://qa-api.kylebradshaw.dev \
   NEXT_PUBLIC_GATEWAY_URL=https://qa-api.kylebradshaw.dev \
   NEXT_PUBLIC_INGESTION_API_URL=https://qa-api.kylebradshaw.dev/ingestion \
   NEXT_PUBLIC_CHAT_API_URL=https://qa-api.kylebradshaw.dev/chat \
   NEXT_PUBLIC_GO_AUTH_URL=https://qa-api.kylebradshaw.dev/go-auth \
   NEXT_PUBLIC_GO_ECOMMERCE_URL=https://qa-api.kylebradshaw.dev/go-api \
   NEXT_PUBLIC_AI_SERVICE_URL=https://qa-api.kylebradshaw.dev/ai-api \
   npm run dev
   ```
3. Tell Claude what to fix
4. **Ask Claude to push** — it will:
   - Commit the fix
   - Push to `qa`
   - Watch CI and debug minor failures (lint, formatting, config issues)
   - Stop and ask you before changing anything that affects app behavior
5. Wait for QA to redeploy, inspect again
6. When satisfied, tell Claude to ship it

## Environments

| Environment | Backend URL | Frontend URL |
|-------------|-------------|--------------|
| Production | api.kylebradshaw.dev | kylebradshaw.dev |
| QA | qa-api.kylebradshaw.dev | qa.kylebradshaw.dev |
| Local dev | localhost:8000 (via SSH tunnel) | localhost:3000 |

## What Claude Can Fix Autonomously

When watching CI on the `qa` branch:
- **Go ahead:** lint errors, formatting issues, type errors, import ordering, config typos
- **Stop and ask:** logic changes, API contract changes, data flow changes, new dependencies
```

- [ ] **Step 2: Commit**

```bash
git add docs/qa-workflow-guide.md
git commit -m "docs: add QA workflow guide for Kyle"
```

---

## Task 10: Validation

- [ ] **Step 1: Validate all Kustomize overlays build cleanly**

```bash
kubectl kustomize k8s/overlays/qa/ > /dev/null && echo "AI services QA: OK"
kubectl kustomize java/k8s/overlays/qa/ > /dev/null && echo "Java QA: OK"
kubectl kustomize go/k8s/overlays/qa/ > /dev/null && echo "Go QA: OK"
```

Expected: All three print OK with no errors.

- [ ] **Step 2: Validate workflow YAML**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml')); print('ci.yml: OK')"
```

Expected: `ci.yml: OK`

- [ ] **Step 3: Validate deploy.sh syntax**

```bash
bash -n k8s/deploy.sh && echo "deploy.sh: OK"
```

Expected: `deploy.sh: OK`

- [ ] **Step 4: Run relevant preflight checks**

```bash
make grafana-sync-check
```

Expected: Pass (we didn't change Grafana config).

- [ ] **Step 5: Verify .gitignore excludes worktrees**

```bash
mkdir -p .claude/worktrees/test-branch
git status | grep -q 'worktrees' && echo "FAIL: worktrees showing in git status" || echo "OK: worktrees excluded"
rmdir .claude/worktrees/test-branch
```

Expected: `OK: worktrees excluded`

- [ ] **Step 6: Commit any fixes from validation**

If any validation steps revealed issues, fix and commit before proceeding.
