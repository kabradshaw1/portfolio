# QA Environment Deployment Design

## Context

The production environment was migrated from Windows to Debian 13 earlier today. The QA namespaces (`ai-services-qa`, `java-tasks-qa`, `go-ecommerce-qa`) exist with secrets in place, but no workloads are deployed. Additionally, a Kustomize v5.7.1 cycle detection bug prevents `kubectl apply -k` from working with the Java and Go overlay directories, because overlay directories live inside the base directory that they reference.

**Goal:** Fix the Kustomize directory structure, deploy QA workloads, and verify the full QA pipeline works end-to-end.

## Part 1: Fix Kustomize Directory Structure

### Problem

`java/k8s/overlays/minikube/kustomization.yaml` references `../../` as its base. Since `overlays/` is inside `java/k8s/`, Kustomize v5.7.1 detects this as a cycle. Same issue with `go/k8s/overlays/*/`.

The `k8s/ai-services/` structure doesn't have this problem because overlays live at `k8s/overlays/` (sibling, not child).

### Solution

Restructure Java and Go K8s directories to use the standard `base/` + `overlays/` pattern:

**Before:**
```
java/k8s/
  kustomization.yaml      # base
  configmaps/
  deployments/
  services/
  ...
  overlays/               # inside base = cycle
    minikube/
    aws/
    qa/
```

**After:**
```
java/k8s/
  base/                   # new directory
    kustomization.yaml
    configmaps/
    deployments/
    services/
    ...
  overlays/               # now sibling of base, not child
    minikube/
    aws/
    qa/
```

Same restructure for `go/k8s/`.

### Files to Move

**Java (`java/k8s/`):**
- Move into `java/k8s/base/`: `kustomization.yaml`, `namespace.yml`, `ingress.yml`, `ingress-rabbitmq.yml`, `configmaps/`, `deployments/`, `services/`, `volumes/`, `secrets/`
- Keep at `java/k8s/`: `overlays/`
- Update overlay references from `../../` to `../base`

**Go (`go/k8s/`):**
- Move into `go/k8s/base/`: `kustomization.yaml`, `namespace.yml`, `ingress.yml`, `configmaps/`, `deployments/`, `services/`, `hpa/`, `pdb/`, `jobs/`, `secrets/`
- Keep at `go/k8s/`: `overlays/`
- Update overlay references from `../../` to `../base`

### CI/CD References to Update

In `.github/workflows/ci.yml`:
- Line 342: `find java/k8s/` → `find java/k8s/base/` (K8s validation)
- Line 367: `for dir in k8s java/k8s go/k8s` → `for dir in k8s java/k8s/base go/k8s/base` (dry-run validation)
- Line 706: `kubectl kustomize java/k8s/overlays/qa/` — no change needed (overlays stay in same place)
- Line 799: `find java/k8s -name 'namespace.yml'` → `find java/k8s/base` (prod deploy)
- Line 801: `find java/k8s -name '*.yml'` → `find java/k8s/base` (prod deploy)
- Line 803: `find go/k8s -name '*.yml'` → `find go/k8s/base` (prod deploy)
- Lines 826, 833: `go/k8s/jobs/` → `go/k8s/base/jobs/` (migration jobs)

In `k8s/deploy.sh`:
- Secret file paths: `java/k8s/secrets/` → `java/k8s/base/secrets/`
- Go secrets: `go/k8s/secrets/` → `go/k8s/base/secrets/`
- Overlay paths stay the same

## Part 2: Deploy QA Workloads

### Prerequisites (already done)
- QA namespaces exist
- GHCR pull secrets exist in all QA namespaces
- Java and Go application secrets exist in QA namespaces
- Cloudflare tunnel has `qa-api.kylebradshaw.dev` DNS route

### Steps

1. **Create QA database:** `CREATE DATABASE ecommercedb_qa` in the shared PostgreSQL (java-tasks namespace)

2. **Apply QA overlays:** Using the fixed Kustomize structure:
   ```bash
   kubectl apply -k k8s/overlays/qa/
   kubectl apply -k java/k8s/overlays/qa/
   kubectl apply -k go/k8s/overlays/qa/
   ```

3. **Run Go migrations** in QA namespace:
   - Delete any existing migration jobs
   - Apply migration jobs from the QA overlay
   - Wait for completion

4. **Verify all QA pods are running:**
   ```bash
   kubectl get pods -n ai-services-qa
   kubectl get pods -n java-tasks-qa
   kubectl get pods -n go-ecommerce-qa
   ```

5. **Test QA endpoints via Cloudflare:**
   - `https://qa-api.kylebradshaw.dev/chat/health`
   - `https://qa-api.kylebradshaw.dev/ingestion/health`
   - `https://qa-api.kylebradshaw.dev/debug/health`
   - `https://qa-api.kylebradshaw.dev/go-auth/health`
   - `https://qa-api.kylebradshaw.dev/go-api/health`

6. **Test CI/CD pipeline:** Push to `qa` branch and verify the Deploy QA job succeeds.

## Verification

1. `kubectl apply -k java/k8s/overlays/minikube/` works without cycle error
2. `kubectl apply -k go/k8s/overlays/minikube/` works without cycle error
3. All QA pods are Running (1/1)
4. All QA health endpoints return 200 via `qa-api.kylebradshaw.dev`
5. Push to `qa` triggers CI, Deploy QA succeeds, QA Smoke Tests pass
6. `make preflight` passes locally (no broken references)
