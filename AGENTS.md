# AGENTS.md

## Project Intent

Portfolio project for Golang Engineer job applications. The quality bar is
production-grade engineering, not working demos. Prefer the approach a serious
code review would accept: focused scope, operational rigor, clear tests, and no
paid cloud dependency unless already part of the project.

## Context Hygiene

Treat context as limited. Start with `rg`, `git diff`, `git status`, and narrow
`sed -n` ranges. Do not read whole large files unless the task truly requires
the full document.

Respect `.ignore` for repo searches. Avoid ignored paths unless Kyle explicitly
asks for them or the task cannot be completed without them. In normal work,
avoid `frontend/node_modules/`, generated build output, caches, Jupyter
checkpoints, `docs/superpowers/`, and `docs/adr/`.

Do not read or summarize `docs/` broadly. Docs are pull-based reference
material. Load only the specific doc needed for the current task. Never scan
`docs/adr/`, `docs/superpowers/`, product PDFs, notebooks, or runbooks unless
the task explicitly requires them.

For handoffs, specs, plans, and CI logs, read the status or failing section
first, then jump only to the referenced lines needed for the immediate decision.
Prefer targeted filters over streaming large files or logs.

## Project Map

- `services/` - Python/FastAPI AI microservices
- `java/` - Spring Boot task/activity/notification/gateway services
- `go/` - Go auth, ecommerce, AI agent, analytics, protobuf, shared package
- `frontend/` - Next.js + TypeScript UI
- `k8s/` - Kubernetes manifests, secrets, monitoring, cert-manager
- `.github/` - CI/CD workflows
- `docs/` - ADRs, runbooks, generated specs/plans, reference material

## Routing Index

Load the narrowest relevant instruction file before touching that area:

- `go/AGENTS.md` - Go services, protobuf/gRPC, migrations, ecommerce, Kafka
- `frontend/AGENTS.md` - Next.js, Vercel, frontend env vars, browser console work
- `services/AGENTS.md` - Python AI services, packages, RAG, Qdrant, Ollama
- `java/AGENTS.md` - Spring services, Java tests, JDK limits, heap sizing
- `k8s/AGENTS.md` - Minikube, namespaces, ingress, sealed secrets, cert-manager
- `.github/AGENTS.md` - CI/CD, failed-run triage, deploy trigger behavior
- `docs/AGENTS.md` - ADR/runbook/spec loading policy and doc placement

Use installed project skills when their trigger conditions match:
`debug-observability`, `ops-as-code`, and `scaffold-go-service`.

## Safety Gates

Debian is runtime infrastructure, not a build or test worker. Run verification
locally on the Mac or in GitHub Actions. Use Debian only for runtime/deployment
operations that belong there: Kubernetes diagnostics, image pulls during
deploys, Ollama checks, observability queries, and read-only service health
inspection.

Use `ops-as-code` before mutating any shared environment: Kubernetes writes,
database mutations, secret edits, queue purges, or one-off production fixes.
Ask before destructive git operations, deleting tracked files, changing
secrets, or touching `main`/production outside branch rules.

Do not ask before repo-local tests, preflights, linters, type checks,
formatters, Playwright, Gradle, pytest, Go tests, file inspection, commits,
branch pushes where branch rules allow them, PR creation where branch rules say
to create a PR, or deleting temporary untracked files Codex created during the
current task under the repo, `/tmp`, or `~/.codex/tmp`.

## Branch Workflow

Determine the current branch with `git branch --show-current`.

- Feature branches: after spec approval, plan and execute autonomously, commit,
  push, create a PR to `qa`, and do not watch CI.
- `qa`: commit and push autonomously. Do not watch CI. For reported CI fixes,
  fix lint, formatting, type, and config issues autonomously; ask before
  behavior changes. Commit doc-only changes locally but do not push them until
  a later code change or explicit request.
- `main`: never push autonomously. When Kyle explicitly says to ship to main,
  merge `qa` into `main`, push, clean up worktrees, and delete the feature
  branch local and remote.

Agents create feature worktrees in `.codex/worktrees/<branch-name>/` when a
separate worktree is needed.

## Verification

Before every commit, run the relevant local preflight and fix failures:

- Python: `make preflight-python` and `make preflight-security`
- Frontend: `make preflight-frontend` and `make preflight-e2e`
- Java: `make preflight-java`
- Go: `make preflight-go`
- Go migrations: `make preflight-go-migrations`
- Full sweep: `make preflight`

If local verification is blocked by missing tools, disk pressure, or platform
limits, report the blocker clearly and leave remaining verification to CI. Do
not move verification to Debian unless Kyle explicitly authorizes it.

## Root Growth Guardrail

Do not add detailed architecture, service internals, environment inventories,
or troubleshooting runbooks to this root file. Add them to the narrowest
directory `AGENTS.md`, a Codex skill, or a triggered doc. Root may only contain
rules that apply to nearly every task.
