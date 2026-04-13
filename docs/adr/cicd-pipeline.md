# CI/CD Pipeline

> **Superseded** by [CI/CD Pipeline Rebuild and QA Environment](cicd-rebuild-qa-environment.md) (2026-04-13)

How code gets from a git push to production.

## Overview

Two GitHub Actions workflows run on every push. All quality and security jobs must pass before deployment. Only pushes to `main` trigger deployment.

```
git push
   |
   v
+---------------------------+     +---------------------------+
|  ci.yml (all branches)    |     |  java-ci.yml              |
|                           |     |  (java/** changes only)   |
|  Lint    Tests    Build   |     |  Lint   Unit   Integration|
|  ruff    pytest   Next.js |     |  checkstyle    Testcontain|
|  Docker Build (GHCR)     |     |  Docker Build (GHCR)      |
|                           |     |                           |
|  Security (6 jobs)        |     |  Security (2 jobs)        |
|  Bandit, pip-audit,       |     |  OWASP, Hadolint          |
|  npm audit, gitleaks,     |     |                           |
|  Hadolint, CORS check    |     |                           |
+---------------------------+     +---------------------------+
   |                                         |
   | all pass + main branch                  |
   v                                         |
+---------------------------+                |
|  Deploy                   |                |
|  Tailscale → SSH → PC    |                |
|  kubectl apply + restart  |                |
+---------------------------+                |
   |                                         |
   v                                         |
+---------------------------+                |
|  Smoke Tests              |                |
|  Playwright vs live URLs  |                |
+---------------------------+                |

+---------------------------+
|  E2E (staging branch)     |
|  Playwright mocked tests  |
+---------------------------+
```

## Python & Frontend CI/CD (`ci.yml`)

Runs on every push to any branch and on pull requests to `main`.

### Quality Jobs

| Job | What it does | Runs on |
|-----|--------------|---------|
| **Backend Lint** | `ruff check` + `ruff format --check` on `services/` | All pushes |
| **Backend Tests** | `pytest` with coverage for ingestion, chat, debug | All pushes |
| **Frontend Checks** | `npm run lint`, `tsc --noEmit`, `npm run build` | All pushes |
| **Docker Build** | Build images for ingestion, chat, debug; push to GHCR on main | All pushes (push only on main) |

### Security Jobs

| Job | What it checks | Tool |
|-----|----------------|------|
| **Bandit** | Python static analysis (SAST) | bandit |
| **pip-audit** | Python dependency vulnerabilities | pip-audit |
| **npm audit** | Frontend dependency vulnerabilities | npm audit |
| **Gitleaks** | Secrets accidentally committed to git | gitleaks |
| **Hadolint** | Dockerfile best practices (Python + Java) | hadolint |
| **CORS Check** | No wildcard `allow_origins=["*"]` in services | grep |

### Gate

All quality and security jobs must pass before the deploy job runs. This is enforced by the `needs:` field on the deploy job:

```yaml
deploy:
  needs:
    - backend-lint
    - backend-tests
    - frontend-checks
    - docker-build
    - security-bandit
    - security-pip-audit
    - security-npm-audit
    - security-gitleaks
    - security-hadolint
    - security-cors-check
  if: github.ref == 'refs/heads/main' && github.event_name == 'push'
```

## Java CI/CD (`java-ci.yml`)

Runs only when files in `java/` are changed. Triggered on all pushes and PRs to `main`.

| Job | What it does |
|-----|--------------|
| **Lint** | Checkstyle on `main` and `test` source sets |
| **Unit Tests** | Per-service tests for task, activity, notification, gateway |
| **Integration Tests** | Full integration tests with Testcontainers (spins up real Postgres, MongoDB, Redis, RabbitMQ) |
| **Docker Build** | Build JARs via Gradle, build images, push to GHCR on main |
| **OWASP** | Dependency vulnerability check |
| **Hadolint** | Dockerfile linting for all 4 Java service Dockerfiles |

## Deploy Mechanism

The deploy job runs only on pushes to `main` after all gates pass.

```
GitHub Actions runner (Ubuntu)
   |
   | 1. Join Tailscale VPN (using TAILSCALE_AUTHKEY secret)
   |    Runner gets a Tailscale IP, can now reach 100.79.113.84
   |
   | 2. SSH to Windows PC (using SSH_PRIVATE_KEY secret)
   v
Windows PC (100.79.113.84)
   |
   | 3. cd $DEPLOY_PATH && git pull origin main
   | 4. kubectl apply -f k8s/ai-services/ --recursive
   |    kubectl apply -f java/k8s/ --recursive
   |    kubectl apply -f k8s/monitoring/ --recursive
   | 5. kubectl rollout restart deployment -n ai-services
   |    kubectl rollout restart deployment -n java-tasks
   | 6. kubectl rollout status --timeout=180s (wait for pods to be ready)
   v
Services updated, new images pulled from GHCR
```

The deploy applies all Kubernetes manifests (picks up any config changes) and restarts deployments (forces new image pulls since we use `:latest` tags). The rollout status wait ensures the deploy job fails if pods don't become healthy within 3 minutes.

## Production Smoke Tests

After a successful deploy, Playwright runs against the live production URLs:

```yaml
env:
  SMOKE_FRONTEND_URL: https://kylebradshaw.dev
  SMOKE_API_URL: https://api.kylebradshaw.dev
  SMOKE_GRAPHQL_URL: https://api.kylebradshaw.dev/graphql
```

The smoke tests verify:
- Frontend loads correctly
- `/chat/health` returns 200
- `/ingestion/health` returns 200
- End-to-end: upload a PDF, ask a question, receive a streamed response, clean up

A 30-second wait before the smoke tests gives the deployment time to stabilize (new pods start, old pods terminate).

## E2E Staging Tests

On pushes to the `staging` branch, Playwright runs mocked E2E tests. These don't hit the real backend — they use mocked API responses to test frontend behavior in isolation.

```yaml
e2e-staging:
  if: github.ref == 'refs/heads/staging'
  needs: [frontend-checks]
```

The staging workflow: feature branches merge into `staging` first, E2E tests validate the frontend, then `staging` merges into `main` for production deploy.

## Required GitHub Secrets

| Secret | Purpose | Rotation |
|--------|---------|----------|
| `TAILSCALE_AUTHKEY` | Lets the CI runner join the Tailscale VPN | Every 90 days (free plan) |
| `SSH_PRIVATE_KEY` | SSH key for `PC@100.79.113.84` | As needed |
| `DEPLOY_PATH` | Repo path on the Windows PC | Static |
| `GITHUB_TOKEN` | Auto-provided; used for GHCR push and Gitleaks | Auto-managed |

**Tailscale key rotation:** The free plan limits auth keys to 90 days. When it expires, CI deploys will fail. Regenerate at Tailscale admin console → Keys, then update the `TAILSCALE_AUTHKEY` secret in the GitHub repo settings.

## See Also

- [Deployment Architecture](deployment-architecture.md) — how the deployed system works at runtime
- [Docker Compose to Kubernetes ADR](adr/docker-compose-to-kubernetes.md) — why we migrated to K8s
