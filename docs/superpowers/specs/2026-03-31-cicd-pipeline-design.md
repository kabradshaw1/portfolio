# CI/CD Pipeline Design

## Context

This project has no CI/CD pipeline. Tests exist (36 pytest tests across two Python services) but only run locally. The frontend builds and lints but has no automated checks. Deployment is manual — Vercel for the frontend, SSH + docker compose on a Windows PC for the backend.

The goal is to automate testing on every push (so PR reviews show passing checks) and automate deployment when main is updated.

## Architecture

Single GitHub Actions workflow file (`.github/workflows/ci.yml`) with parallel CI jobs and a gated deploy job.

### Triggers

- **`push` to any branch** — runs all CI check jobs
- **`pull_request` to `main`** — runs all CI check jobs
- **Deploy job** — only on `push` to `main`, requires all CI jobs to pass

### CI Jobs (parallel, every push)

#### 1. `backend-lint`
- **Runner:** `ubuntu-latest`
- **Steps:** Install ruff, run `ruff check services/` and `ruff format --check services/`
- **Purpose:** Catch style issues, unused imports, common Python bugs
- **New dependency:** `ruff` (installed via pip in CI, not added to service requirements.txt — it's a dev/CI-only tool)

#### 2. `backend-tests`
- **Runner:** `ubuntu-latest`
- **Strategy:** Matrix `service: [ingestion, chat]` — runs both in parallel
- **Steps:** Set up Python 3.11, cache pip deps, install `requirements.txt`, run `pytest services/${{ matrix.service }}/tests/ -v`
- **Caching:** `actions/setup-python` with `cache: 'pip'` (keyed on requirements.txt hash)
- **No external services needed:** All tests use mocks (unittest.mock), no Qdrant or Ollama required

#### 3. `frontend-checks`
- **Runner:** `ubuntu-latest`
- **Steps:** Set up Node 20, cache npm, install deps, run:
  - `npm run lint` (ESLint)
  - `npx tsc --noEmit` (TypeScript type checking)
  - `npm run build` (Next.js build — catches SSR/import issues)
- **Caching:** `actions/setup-node` with `cache: 'npm'`

#### 4. `docker-build`
- **Runner:** `ubuntu-latest`
- **Strategy:** Matrix `service: [ingestion, chat]`
- **Steps:** Build Docker images for both services (build-only, no push)
- **Caching:** Docker layer caching via `docker/build-push-action` with GitHub Actions cache backend
- **Purpose:** Verify Dockerfiles and dependency installation work before deploy

### Deploy Job (main branch only)

#### 5. `deploy`
- **Runner:** `ubuntu-latest`
- **Condition:** `if: github.ref == 'refs/heads/main' && github.event_name == 'push'`
- **Depends on:** All 4 CI jobs must pass (`needs: [backend-lint, backend-tests, frontend-checks, docker-build]`)

**Frontend deployment:**
- Handled by Vercel's GitHub integration (auto-deploys on push to main)
- No workflow step needed — Vercel watches the repo directly
- Preview deploys on PRs are also automatic via Vercel

**Backend deployment:**
1. **Tailscale:** `tailscale/github-action` joins the runner to the tailnet
2. **SSH:** Connect to `PC@100.79.113.84` using key-based auth
3. **Deploy commands:**
   ```bash
   cd $DEPLOY_PATH
   git pull origin main
   docker compose down
   docker compose up -d --build
   ```

### GitHub Secrets Required

| Secret | Purpose |
|--------|---------|
| `TAILSCALE_AUTHKEY` | Ephemeral, reusable Tailscale auth key for runner to join tailnet |
| `SSH_PRIVATE_KEY` | SSH private key for `PC@100.79.113.84` |
| `DEPLOY_PATH` | Absolute path to project directory on Windows PC |

### New Files

| File | Purpose |
|------|---------|
| `.github/workflows/ci.yml` | Main CI/CD workflow |
| `ruff.toml` | Ruff linter configuration (line length, target Python version, rule selection) |

### Ruff Configuration

Minimal `ruff.toml` at project root:
- Target Python 3.11
- Line length 88 (black-compatible)
- Enable: `E` (pycodestyle errors), `F` (pyflakes), `I` (isort), `UP` (pyupgrade)
- Scope: `services/` directory only

### Dependency Caching Strategy

- **pip:** Built-in to `actions/setup-python` via `cache: 'pip'` — caches based on `requirements.txt` hash
- **npm:** Built-in to `actions/setup-node` via `cache: 'npm'` — caches based on `package-lock.json` hash
- **Docker layers:** `docker/build-push-action` with `cache-from: type=gha` and `cache-to: type=gha,mode=max`

### PR Status Checks

All 4 CI jobs report independently in PR status checks:
- `backend-lint` — green/red
- `backend-tests (ingestion)` — green/red
- `backend-tests (chat)` — green/red
- `frontend-checks` — green/red
- `docker-build (ingestion)` — green/red
- `docker-build (chat)` — green/red

Reviewers see at a glance which checks pass or fail.

## Verification

1. **CI checks:** Push to a non-main branch, verify all 4 job types run and report status on the PR
2. **Caching:** Second push should be faster (check "Cache restored" messages in logs)
3. **Deploy gate:** Merge to main, verify deploy job runs only after all CI jobs pass
4. **Backend deploy:** Confirm Docker containers restart on Windows PC after main merge
5. **Frontend deploy:** Confirm Vercel deploys automatically (this should already work once the Vercel GitHub integration is connected)
6. **Failure case:** Push code that breaks a test, verify the PR shows a failing check and deploy does not run

## GitHub Secrets Setup

Before the deploy job will work, add these secrets in GitHub (Settings > Secrets and variables > Actions):

### `TAILSCALE_AUTHKEY`
1. Go to https://login.tailscale.com/admin/settings/keys
2. Generate a new auth key with **Ephemeral** and **Reusable** checked
3. Copy the key and add it as a GitHub secret

### `SSH_PRIVATE_KEY`
1. Generate a new SSH key pair: `ssh-keygen -t ed25519 -f ~/.ssh/github_deploy -N ""`
2. Add the public key to your Windows PC: append `~/.ssh/github_deploy.pub` to `C:\Users\PC\.ssh\authorized_keys`
3. Copy the **private** key contents and add as a GitHub secret

### `DEPLOY_PATH`
The absolute path to the project on your Windows PC (e.g., `C:\Users\PC\repos\gen_ai_engineer` or wherever docker-compose.yml lives).

## Out of Scope

- Container registry (images built and used locally on the PC, not pushed to a registry)
- Frontend testing (no test framework configured — could be a follow-up)
- Secrets scanning / SAST tools (good addition later, not needed for v1)
- Branch protection rules (recommended to set up manually in GitHub settings after pipeline works)
