# CI/CD Pipeline Rebuild & QA Environment

## Context

The current CI/CD pipeline has three separate GitHub Actions workflows (`ci.yml`, `java-ci.yml`, `go-ci.yml`) and a staging branch that re-runs the same checks already passed on feature branches. For a solo developer, these intermediate stops add no value — if checks pass on the feature branch, the code is identical when merged.

This redesign consolidates everything into a single unified workflow, introduces a QA environment for pre-production validation, and automates the spec-to-QA pipeline using Claude Code agents with git worktrees.

## Goals

- Single workflow file handling all CI/CD for all stacks
- QA environment that is functionally identical to production
- Automated agent workflow: brainstorm spec → implementation → QA, with minimal manual steps
- Kyle only needs to: review PR diffs, merge PRs to `qa`, inspect QA, merge `qa` to `main`

## Branching Model

**Branches:**
- `main` — production. Kyle pushes directly after reviewing QA. Triggers prod deploy.
- `qa` — pre-production. Agents create PRs targeting this branch. Kyle merges PRs and pushes tweaks here.
- Feature branches (`agent/feat-*`) — created by agents, short-lived, deleted after PR merge.
- `staging` — retired. QA replaces it.

**Per-branch rules for Claude Code (CLAUDE.md):**
- **On a feature branch:** implement, commit, push, create PR to `qa`.
- **On `qa`:** commit and push when Kyle asks. Watch CI after pushing and debug failures. For CI fixes: lint errors, formatting, type errors, and config issues are fine to fix autonomously. For anything that changes application behavior (logic, API contracts, data flow), stop and check with Kyle before fixing.
- **On `main`:** never push autonomously. When Kyle explicitly says to merge/ship to main, handle the full flow: merge `qa` into `main`, push, watch CI, debug minor failures, clean up worktree, delete feature branch (local + remote).

Claude Code determines the current branch via `git branch --show-current` and follows the rules for that branch automatically. No special mode or prompt needed.

## Unified Workflow (`.github/workflows/ci.yml`)

Replaces `ci.yml`, `java-ci.yml`, and `go-ci.yml`. The `aws-deploy.yml` is unrelated (manual trigger) and stays unchanged.

### Triggers

```yaml
on:
  pull_request:
    branches: [qa]
  push:
    branches: [qa, main]
```

### Job Matrix

| Job | PR to `qa` | Push to `qa` | Push to `main` |
|-----|------------|--------------|-----------------|
| Quality checks (lint, test, security, k8s validation) | Yes | Yes | Yes |
| Build images (all services, push to GHCR) | No | Yes | Yes |
| Deploy QA (QA namespaces) | No | Yes | No |
| Deploy prod (prod namespaces) | No | No | Yes |
| Smoke QA (`qa-api.kylebradshaw.dev`) | No | Yes | No |
| Smoke prod (`api.kylebradshaw.dev`) | No | No | Yes |

### Quality Checks (all run in parallel)

- `python-lint` — ruff check + format
- `python-tests` — pytest matrix (ingestion, chat, debug) with coverage
- `java-lint` — checkstyle
- `java-tests` — unit test matrix (task, activity, notification, gateway)
- `go-lint` — golangci-lint matrix (auth, ecommerce, ai-service)
- `go-tests` — go test -race matrix
- `go-migration-test` — PostgreSQL + golang-migrate
- `frontend-checks` — npm lint, tsc, next build
- `security` — bandit, pip-audit, npm-audit, gitleaks, hadolint, CORS guardrail
- `k8s-validation` — kubeconform + kind cluster dry-run + policy checks
- `grafana-sync` — dashboard JSON / ConfigMap sync check

### Image Build

Matrix builds all services, pushes to GHCR:
- **QA tag:** `:qa-<short-sha>`
- **Prod tag:** `:latest`

Services: ingestion, chat, debug, java-task-service, java-activity-service, java-notification-service, java-gateway-service, go-auth-service, go-ecommerce-service, go-ai-service.

### Deploy

Both QA and prod deploy via SSH to Windows PC (`PC@100.79.113.84` via Tailscale), running `kubectl apply` with the appropriate Kustomize overlay. The existing deploy logic (SSH setup, Tailscale auth, kubectl apply, rollout restart, migration jobs) is preserved from the current `ci.yml` deploy job.

- QA: `kubectl apply -k k8s/overlays/qa` (and java/go equivalents)
- Prod: `kubectl apply -k k8s/overlays/minikube` (unchanged)

### Conditionals

```yaml
build-images:
  if: github.event_name == 'push'

deploy-qa:
  if: github.event_name == 'push' && github.ref == 'refs/heads/qa'

deploy-prod:
  if: github.event_name == 'push' && github.ref == 'refs/heads/main'
```

## QA Environment Architecture

### Namespaces

Separate namespaces in the same Minikube cluster on the Windows PC:

| Production | QA |
|---|---|
| `ai-services` | `ai-services-qa` |
| `java-tasks` | `java-tasks-qa` |
| `go-ecommerce` | `go-ecommerce-qa` |
| `monitoring` | shared (no QA copy) |

### Database Strategy

QA shares database instances with production but uses separate databases:

| Service | Prod DB | QA DB |
|---|---|---|
| Java task-service | `taskdb` | `taskdb_qa` |
| Go ecommerce-service | `ecommercedb` | `ecommercedb_qa` |
| Go auth-service | `ecommercedb` (users table) | `ecommercedb_qa` |
| Python services (Qdrant) | `documents` collection | `documents_qa` collection |
| Java activity-service (MongoDB) | `activitydb` | `activitydb_qa` |

QA services connect to prod-namespace database pods via cross-namespace DNS (e.g., `postgres.java-tasks.svc.cluster.local`). This avoids duplicating PostgreSQL, MongoDB, Redis, RabbitMQ, and Qdrant pods (~1.3GB saved).

### Networking

- **QA backend:** `qa-api.kylebradshaw.dev` → Cloudflare Tunnel → Windows PC `localhost:80` → NGINX Ingress routes by host header to QA namespaces
- **QA frontend:** `qa.kylebradshaw.dev` → Vercel branch domain for the `qa` branch (DNS in Cloudflare pointing to Vercel)
- **Prod:** unchanged (`api.kylebradshaw.dev`, `kylebradshaw.dev`)

The NGINX Ingress Controller handles both prod and QA via host-based routing. QA ingress manifests are identical to prod but with `host: qa-api.kylebradshaw.dev`.

### CORS

- QA ConfigMaps: `ALLOWED_ORIGINS: https://qa.kylebradshaw.dev,http://localhost:3000`
- Prod ConfigMaps: unchanged (`https://kylebradshaw.dev,http://localhost:3000`)

### Kustomize Overlays

New QA overlays for each stack: `k8s/overlays/qa/`, `java/k8s/overlays/qa/`, `go/k8s/overlays/qa/`. Each patches:
- Namespace (e.g., `ai-services` → `ai-services-qa`)
- `ALLOWED_ORIGINS` in ConfigMaps
- Database names in ConfigMaps
- Ingress host (`qa-api.kylebradshaw.dev`)
- Image tags (set during deploy via kustomize edit or kubectl set image)

Base manifests are unchanged — QA overlays patch them.

### Frontend

No frontend Docker image. Vercel handles both environments:
- `qa.kylebradshaw.dev` → Vercel branch domain for `qa` branch
- `kylebradshaw.dev` → Vercel production (unchanged)

QA env vars in Vercel: `NEXT_PUBLIC_*_URL` values pointing to `qa-api.kylebradshaw.dev`.

### Shared Resources

- **Ollama:** QA services share the same Ollama instance as prod (ExternalName service → `host.minikube.internal:11434`)
- **Redis:** Shared instance. QA services use Redis DB 1 (`?db=1` in connection string) while prod uses the default DB 0. Set via QA ConfigMap `REDIS_URL` override.
- **RabbitMQ:** Shared instance. QA services use a `qa_` prefix on queue/exchange names. Set via QA ConfigMap environment variable (e.g., `QUEUE_PREFIX: qa_`).

## Agent Automation Workflow

### Full Flow: Spec to QA

1. **Brainstorm skill** produces a design spec (this document pattern)
2. **Writing-plans skill** produces an implementation plan
3. Agent creates a feature branch from `main` (e.g., `agent/feat-xyz`)
4. Agent creates a worktree in `.claude/worktrees/feat-xyz/`
5. Agent implements in the worktree, runs preflight checks, commits
6. Agent pushes the feature branch to origin
7. Agent creates PR targeting `qa` via `gh pr create --base qa`
8. Agent watches CI via `gh run watch --exit-status` (background, notified on completion)
9. If CI fails: agent reads logs via `gh run view --log-failed`. Minor fixes (lint, formatting, type errors, config) are fixed autonomously. Changes that affect application behavior require Kyle's approval before fixing.
10. If CI passes: agent notifies Kyle — "PR to `qa` is ready, CI passed. Review at [PR link]"

### Kyle's Review and QA Inspection

11. Kyle reviews PR diff on GitHub
12. Kyle merges PR on GitHub (push to `qa` triggers build + deploy + smoke)
13. Kyle inspects at `qa.kylebradshaw.dev`
14. If satisfied: tell Claude to ship it. Claude merges `qa` into `main`, pushes, watches CI, debugs minor failures, cleans up worktree, deletes feature branch.
15. If tweaks needed:
    - `git checkout qa && git pull`
    - For frontend changes: `npm run dev` in `frontend/` with QA env vars
    - Tell Claude what to fix — it sees it's on `qa`, commits
    - Ask Claude to push — Claude pushes and watches CI via `gh run watch`, debugs any failures
    - QA redeploys, inspect again
    - Repeat until satisfied, then merge to main

### Worktree Lifecycle

- **Created:** when agent starts a feature (step 4)
- **Active:** through all iteration
- **Cleaned up:** automatically by Claude as part of the "ship it" flow (merge to main → clean up worktree → delete feature branch). Also cleaned up on explicit rejection.
- **Fallback cleanup:** `make worktree-cleanup` target removes worktrees for merged/deleted branches; periodic `git worktree prune` for anything missed.

## Documentation Deliverables

### ADR

Create `docs/adr/cicd-rebuild-qa-environment.md` documenting:
- Why the three workflows were consolidated into one
- Why staging was retired in favor of QA
- The shared database decision and its trade-offs
- Why agents push (not Kyle) during the feature→QA flow
- Why Kyle pushes directly to main (solo developer, code already reviewed in QA)

### Workflow Guide

Create `docs/qa-workflow-guide.md` — a step-by-step reference for Kyle explaining his role in the pipeline:

1. Agent notifies you: "PR to `qa` is ready, CI passed. Review at [PR link]"
2. Review the PR diff on GitHub
3. Merge the PR on GitHub — this triggers the QA build + deploy
4. Wait for the deploy to complete (watch the Actions tab or wait for Claude to confirm)
5. Inspect at `qa.kylebradshaw.dev`
6. If it looks good: tell Claude to ship it — Claude merges `qa` into `main`, pushes, watches prod CI, debugs minor failures, cleans up the worktree, and deletes the feature branch
7. If tweaks are needed:
   - `git checkout qa && git pull`
   - For frontend changes: `npm run dev` in `frontend/` with QA env vars
   - Tell Claude what to fix
   - Ask Claude to push — it will watch CI and debug minor failures (lint, formatting, config). It will stop and ask you before changing anything that affects app behavior.
   - Inspect again at `qa.kylebradshaw.dev`
   - Repeat until satisfied, then merge to main

## Files Changed

| Action | File | Purpose |
|---|---|---|
| Rewrite | `.github/workflows/ci.yml` | Single unified workflow |
| Delete | `.github/workflows/java-ci.yml` | Replaced by unified ci.yml |
| Delete | `.github/workflows/go-ci.yml` | Replaced by unified ci.yml |
| Create | `k8s/overlays/qa/kustomization.yaml` | QA overlay for Python services |
| Create | `java/k8s/overlays/qa/kustomization.yaml` | QA overlay for Java services |
| Create | `go/k8s/overlays/qa/kustomization.yaml` | QA overlay for Go services |
| Create | `k8s/overlays/qa/namespace.yml` | `ai-services-qa` namespace |
| Create | `java/k8s/overlays/qa/namespace.yml` | `java-tasks-qa` namespace |
| Create | `go/k8s/overlays/qa/namespace.yml` | `go-ecommerce-qa` namespace |
| Modify | `k8s/deploy.sh` | Accept `qa` as third environment |
| Create | `docs/adr/cicd-rebuild-qa-environment.md` | ADR documenting design decisions |
| Create | `docs/qa-workflow-guide.md` | Step-by-step reference for Kyle's role in the pipeline |
| Modify | `CLAUDE.md` | New branching model, agent workflow, per-branch push rules |
| Modify | `.gitignore` | Exclude `.claude/worktrees/` |
| Modify | `Makefile` | Add `worktree-cleanup` target |
| Keep | `.github/workflows/aws-deploy.yml` | Unrelated manual trigger |

## Verification

1. **Workflow syntax:** `act` or push to a test branch to validate the workflow YAML
2. **QA overlay:** `kubectl apply -k k8s/overlays/qa --dry-run=client` for each stack
3. **QA deploy:** manually trigger by creating a test PR to `qa`, merge it, verify services come up in QA namespaces
4. **Networking:** hit `qa-api.kylebradshaw.dev` health endpoints from browser
5. **CORS:** verify browser requests from `qa.kylebradshaw.dev` are accepted, other origins rejected
6. **Agent workflow:** run a small test change through the full agent flow (worktree → PR → CI → QA)
7. **Prod deploy:** merge `qa` to `main`, push, verify prod is unchanged or correctly updated

## Follow-up Spec (Out of Scope)

After this goes live, create a separate spec for:
1. **CI/CD pipeline documentation page** — update the UI to explain the pipeline and why it's designed for a solo developer
2. **Document the shared database decision** — why QA shares database instances with prod (resource efficiency on a single Minikube node)
3. **Document why Kyle pushes directly to main** — solo developer, code already reviewed in QA, branch protection is unnecessary ceremony
