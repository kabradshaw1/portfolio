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

Project skills live under `.codex/skills/`. Use them when their trigger
conditions match, especially `scaffold-go-service`,
`go-grpc-service-reliability`, `go-kafka-consumer-reliability`,
`go-rabbitmq-reliability`, and `java-rabbitmq-reliability`. Installed local
skills such as `debug-observability` and `ops-as-code` still apply when their
trigger conditions match.

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
- Default work location:
  - Do not do feature, behavior, application-code, infrastructure-code, or
    image-rebuild-triggering work directly on `qa` or `main` unless Kyle
    explicitly instructs otherwise.
  - If work would require rebuilding a Docker image, changing
    application/runtime behavior, changing CI/CD behavior, or modifying
    deployable manifests, create or use a feature worktree under
    `.codex/worktrees/<branch-name>/` and target a PR to `qa`.
  - `qa` and `main` may be used directly only for pressing hotfixes to issues
    already live in QA/production, or for documentation-only changes.
  - Shell scripts that are written for Kyle to inspect or run manually count as
    documentation-only unless they are wired into CI/CD, image builds,
    deployments, cron/systemd, Kubernetes Jobs, or another automated runtime
    path.
- `qa`: use directly only for pressing fixes to issues already live on QA,
  CI/config fixes for reported failures, or documentation-only changes. For new
  feature work or changes that trigger image rebuilds, create/use a feature
  worktree and PR to `qa`. Commit doc-only changes locally but do not push them
  until a later code change or explicit request.
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
