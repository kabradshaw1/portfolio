# CI/CD Performance Optimizations

- **Date:** 2026-04-17
- **Status:** Accepted

## Context

Adding the RAG evaluation service exposed several CI/CD performance issues. The eval service's dependency on RAGAS (200+ transitive packages including langchain) caused the Python test and pip-audit jobs to take ~20 minutes just for `pip install`. The compose-smoke job rebuilt all Docker images from scratch on every run (~15 minutes). And every push rebuilt all 11 service images regardless of what changed.

The full pipeline was taking 30+ minutes on a typical push, with most time spent on redundant work.

## Decision

Four optimizations, each targeting a different bottleneck:

### 1. Virtualenv Caching (Python Tests + pip-audit)

**Problem:** `pip install` ran from scratch on every CI run. For the eval service (RAGAS + langchain), this took ~20 minutes.

**Solution:** Cache the entire `.venv` directory using `actions/cache@v4`, keyed on:
```
venv-{service}-{hash(requirements.txt, shared/pyproject.toml)}
```

On cache hit, the install step is skipped entirely. pip is upgraded during venv creation to avoid stale-pip CVEs in the cached venv. pip-audit is also installed into the cached venv.

**Impact:** Eval tests: 20 min → 20 sec. Other services: marginal improvement (their deps are small).

### 2. Conditional Image Builds

**Problem:** All 11 service images were rebuilt on every push, even when only one service changed.

**Solution:** Each matrix entry has a `paths` field listing the directories that affect its image:
```yaml
- service: chat
  paths: services/chat services/shared
- service: go-auth-service
  paths: go/auth-service go/pkg go/go.work
```

A `git diff HEAD~1` check at the start of each build job skips the entire build+push when none of those paths changed.

**Impact:** A typical single-service change rebuilds 1 image (~3 min) instead of 11 (~15 min total wall time due to parallelism, but saves compute and GHCR storage).

**Caveat:** Skipped services don't get a new `qa-<sha>` tag. The deploy step uses `:latest` (which always exists from the most recent build) to avoid referencing nonexistent tags.

### 3. Compose-Smoke: Pull Instead of Build

**Problem:** `docker compose up --build` rebuilt all Python images from source in CI, spending ~10 minutes per service on pip install with no layer cache (fresh GH Actions runner each time).

**Solution:** Pull pre-built `:latest` images from GHCR and run `docker compose up` without `--build`. The smoke tests verify service configuration (env vars, nginx routing, health checks, inter-service connectivity) — not the code itself. Code correctness is covered by unit tests.

```yaml
- name: Pull pre-built images and start compose stack
  run: |
    for svc in ingestion chat debug; do
      docker pull "ghcr.io/.../\${svc}:latest"
    done
    docker compose up -d qdrant gateway ingestion chat debug mock-ollama
```

**Impact:** Compose-smoke: ~15 min → ~95 sec.

### 4. QA Deploy: Job Immutability Fix

**Problem:** The Go kustomize overlay includes migration Jobs. Kubernetes Jobs are immutable — once created, their `spec.template` cannot be patched. When the kustomize apply tried to update existing Jobs (even just with a new image tag), it failed with `field is immutable`.

**Solution:** Filter Jobs out of the kustomize output using awk, then handle them separately in the sequential migration section (delete → create → wait):

```bash
# Apply overlay without Jobs
kubectl kustomize k8s/overlays/qa-go/ \
  | awk '..filter out kind: Job...' \
  | kubectl apply -f -

# Run migrations sequentially
kubectl delete job go-auth-migrate --ignore-not-found
kubectl apply -f auth-service-migrate.yml
kubectl wait --for=condition=complete job/go-auth-migrate
# then ecommerce-migrate
```

**Impact:** Deploy QA: failing → succeeding in ~85 sec.

### Combined Pipeline Impact

| Stage | Before | After |
|-------|--------|-------|
| Python Tests (eval) | 20 min | 20 sec |
| pip-audit (eval) | 20 min | 9 sec |
| Compose Smoke | ~15 min | 95 sec |
| Image Builds (no change) | ~3 min each | ~20 sec (skipped) |
| Deploy QA | failing | 85 sec |
| **Total pipeline** | **30+ min** | **~5 min** |

## Consequences

**Positive:**
- 6x faster CI pipeline on typical pushes
- Developers get feedback in minutes, not half an hour
- Reduced GHCR storage (unchanged images aren't re-pushed)
- Deploy is reliable (no more Job immutability or missing image tag failures)

**Trade-offs:**
- Venv cache can go stale if deps change outside of requirements.txt (e.g., transitive dep updates). Cache key includes the requirements hash, so explicit changes invalidate correctly.
- Conditional builds mean unchanged services keep their previous `:latest` image. If a shared dependency changes (e.g., a Go `pkg/` change), only services whose `paths` include `go/pkg` will rebuild. This is correct but requires `paths` to be maintained accurately when adding shared dependencies.
- Compose-smoke uses pre-built images, so it won't catch Dockerfile or requirements.txt regressions. Those are caught by the build-images job and unit tests respectively.
- The awk Job filter is fragile to YAML formatting changes. If the kustomize output format changes, the filter may need updating.

**Monitoring:**
- Watch for cache hit rates in the `actions/cache` step output
- If a service starts failing in QA but not in tests, check whether its `paths` field is missing a shared dependency
