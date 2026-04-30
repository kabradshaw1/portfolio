# Codex Context Hygiene Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor root `AGENTS.md` into a compact router while preserving detailed project instructions in narrower directory `AGENTS.md` files.

**Architecture:** Root `AGENTS.md` keeps invariant safety, quality, search, autonomy, and routing rules. Stack-specific and situational guidance moves to `go/`, `frontend/`, `services/`, `java/`, `k8s/`, `.github/`, and `docs/` instruction files so agents load context pull-first.

**Tech Stack:** Markdown project instruction files, Git worktree workflow, shell verification with `wc`, `rg`, `git diff`, and targeted file reads.

---

## File Structure

- Modify: `AGENTS.md` - compact router and invariant rule set, target 80-120 lines.
- Modify: `go/AGENTS.md` - preserve and update Go service, gRPC/proto, migrations, ecommerce, Kafka, and Go verification details.
- Modify: `frontend/AGENTS.md` - preserve Next.js rule and add Vercel, frontend dev, env, and verification guidance.
- Modify: `services/AGENTS.md` - preserve Python/FastAPI guidance and add AI platform/RAG service details.
- Create: `java/AGENTS.md` - Java service structure, JDK 21 limitation, heap sizing, Spring/JPA ownership, Java verification.
- Create: `k8s/AGENTS.md` - Minikube runtime, namespaces, ingress, sealed secrets, cert-manager, shared-environment mutation gates, runtime locality.
- Create: `.github/AGENTS.md` - CI/CD trigger matrix, log handling, compose-smoke realism, Tailscale auth key maintenance.
- Create: `docs/AGENTS.md` - documentation loading policy, ADR/runbook pull rules, Superpowers generated artifact rules, product-catalog fixture warning.

### Task 1: Create Narrow Instruction Files

**Files:**
- Create: `java/AGENTS.md`
- Create: `k8s/AGENTS.md`
- Create: `.github/AGENTS.md`
- Create: `docs/AGENTS.md`

- [ ] **Step 1: Create `java/AGENTS.md`**

Add Java-specific guidance moved from root:

```md
# Java Services

Spring Boot microservices live under `java/`:

- `task-service` - task/project CRUD, PostgreSQL, JPA
- `activity-service` - activity feed, MongoDB, Redis caching, analytics aggregation
- `notification-service` - event-driven notifications, RabbitMQ consumer
- `gateway-service` - GraphQL gateway
- `k8s/` - Java-specific Kubernetes manifests

## Schema Ownership

Java services own schema through Spring/JPA at startup. Do not add a separate
migration framework unless the service design changes explicitly.

## Resource Limits

Java services use `-Xmx512m` heap cap with 768Mi container memory limits. New
Java service Dockerfiles must include the heap cap in `ENTRYPOINT`; otherwise
JVM auto-sizing can cause OOM kills.

## Verification

`make preflight-java` is the expected Java check. If local execution is blocked
by missing JDK 21, report the blocker and leave Java verification to CI. Do not
run Java tests on Debian as a workaround unless Kyle explicitly authorizes that
specific exception.
```

- [ ] **Step 2: Create `k8s/AGENTS.md`**

Add infrastructure guidance moved from root:

```md
# Kubernetes And Runtime Infrastructure

Debian 13 (`ssh debian`) hosts Ollama, Minikube, observability, and runtime
services. It is runtime infrastructure, not a general build or test worker.

## Namespaces

- `ai-services` - Python AI services and Qdrant
- `java-tasks` - Java services and databases
- `go-ecommerce` - Go auth and ecommerce services
- `monitoring` - Prometheus, Grafana, Loki, Promtail, Jaeger, exporters
- `ai-services-qa`, `java-tasks-qa`, `go-ecommerce-qa` - QA service copies

## Local And Production Routing

Local frontend development uses an SSH tunnel:

```bash
ssh -f -N -L 8000:localhost:8000 debian
```

Production traffic reaches Debian Minikube through Cloudflare Tunnel:
`https://api.kylebradshaw.dev` routes to Minikube Ingress.

## Shared Environment Safety

Before any mutating shared-environment action, use the `ops-as-code` skill.
This includes `kubectl apply/exec/rollout/scale/delete`, database mutations,
secret edits, queue purges, or one-off production fixes.

Application secrets are committed as `SealedSecret` resources in
`k8s/secrets/<namespace>/<name>.sealed.yml`. Do not edit live Kubernetes
Secrets, do not create app Secrets imperatively in CI, and do not put
credentials in ConfigMap data.

Use `scripts/seal-from-cluster.sh` when a sealed secret must be regenerated
from cluster state.

## Cert-Manager And gRPC mTLS

cert-manager resources live under `k8s/cert-manager/`. Go gRPC services use
per-service TLS secrets mounted at `/etc/tls/` and switch to mTLS when
`TLS_CERT_DIR` is set.

If a gRPC call fails with an authentication handshake error, check:

```bash
kubectl get pods -n cert-manager
kubectl get certificates -n <namespace>
```

Then inspect service logs for `mTLS enabled` to catch stale images.

## Observability

Critical datasource UIDs:

- Prometheus: `PBFA97CFB590B2093`
- Loki: `loki`
- Jaeger: `jaeger`

For alerts, running pods with errors, circuit breakers, saga issues, gRPC
failures, or post-incident verification, use `debug-observability` before ad
hoc log inspection.

Pods down or CrashLoopBackOff are the exception: start with `kubectl get pods`
and `kubectl logs`, then use observability after the target is running.
```

- [ ] **Step 3: Create `.github/AGENTS.md`**

Add CI/CD guidance moved from root:

```md
# GitHub Actions And CI/CD

The unified workflow is `.github/workflows/ci.yml`.

## Trigger Matrix

- PR to `qa` - quality checks
- Push to `qa` - quality checks, image builds, QA deploy, QA smoke tests
- Push to `main` - quality checks, image builds, production deploy, production smoke tests

Quality checks include Python lint/tests, frontend type/build checks, Java
checkstyle/tests, Go lint/tests, security scans, Kubernetes validation, and
CORS guardrails.

## CI Log Handling

Identify the failed job first:

```bash
gh pr view <pr> --json statusCheckRollup
```

Then inspect only the failed job with a targeted filter:

```bash
gh run view <run-id> --job <job-id> --log \
  | rg -n "##\\[error\\]|FAIL|failed|Error|panic|Exception|required|unhealthy"
```

Only fetch broader logs after filtered output identifies the failing step or
proves insufficient. Prefer redirecting full logs to `/tmp` and searching
locally instead of streaming large logs into the conversation.

## Compose Smoke Realism

The `compose-smoke` job runs the Python AI stack through `docker-compose.yml`
with mocked Ollama. Python service configuration changes must update both
`docker-compose.yml` and the matching Kubernetes manifests under
`k8s/ai-services/`.

## Tailscale Auth Key

The free-plan Tailscale auth key expires every 90 days. Regenerate it in the
Tailscale admin console and update the `TAILSCALE_AUTHKEY` GitHub repo secret.
```

- [ ] **Step 4: Create `docs/AGENTS.md`**

Add documentation loading rules:

```md
# Documentation

Docs are pull-based reference material. Do not read or summarize `docs/`
broadly during routine project discovery.

## Loading Policy

- Load ADRs only for architecture decisions or when explicitly referenced.
- Load runbooks only for matching operational tasks.
- `docs/product-catalog/` contains RAG/test fixture content, not engineering
  onboarding context.
- `docs/superpowers/` contains generated specs and plans. Keep it ignored
  except when the user or a skill explicitly asks for a spec or plan.
- Prefer targeted `rg` searches that respect `.ignore` over `find` or broad
  directory scans.

## Adding Documentation

Use `docs/adr/template-adr.md` for new ADRs. Add deep decisions and references
to `docs/`, repeated procedures to a skill, and directory-scoped rules to the
nearest `AGENTS.md`.
```

- [ ] **Step 5: Verify created files are discoverable**

Run:

```bash
rg --files -g 'AGENTS.md' -g '!docs/superpowers/**' -g '!docs/adr/**'
```

Expected: output includes `java/AGENTS.md`, `k8s/AGENTS.md`, `.github/AGENTS.md`, and `docs/AGENTS.md`.

### Task 2: Update Existing Child Instruction Files

**Files:**
- Modify: `go/AGENTS.md`
- Modify: `frontend/AGENTS.md`
- Modify: `services/AGENTS.md`

- [ ] **Step 1: Update `go/AGENTS.md`**

Preserve existing content and add sections for current decomposed ecommerce, migrations, Kafka, AI service gateway, and service verification. Include:

```md
## Go Service Inventory

Current decomposed services include `auth-service`, `product-service`,
`cart-service`, `order-service`, `payment-service`, `ai-service`, and
`analytics-service`.
```

Add migration rules requiring `DATABASE_URL_DIRECT` for migration Jobs and
`sslmode=disable`, plus the Kafka topics and `make preflight-go` /
`make preflight-go-migrations` verification commands.

- [ ] **Step 2: Update `frontend/AGENTS.md`**

Preserve the Next.js warning and add frontend dev, Vercel, and verification guidance:

```md
## Verification

Before committing frontend changes, run:

```bash
make preflight-frontend
make preflight-e2e
```
```

Include the critical `NEXT_PUBLIC_*` Vercel env var rule and command hints
`vercel env ls production`, `vercel env add`, and `vercel redeploy`.

- [ ] **Step 3: Update `services/AGENTS.md`**

Preserve existing Python package guidance and add AI service topology:

```md
## AI Service Topology

- `ingestion` - PDF upload, parse, chunk, embed, store, delete
- `chat` - question embed, search, RAG prompt, stream
- `debug` - code indexing, agent loop, tool execution, debug streaming
```

Include Ollama model context, Qdrant, shared LLM factory, and Python verification
commands `make preflight-python` and `make preflight-security`.

- [ ] **Step 4: Verify child files contain moved anchors**

Run:

```bash
rg -n "DATABASE_URL_DIRECT|NEXT_PUBLIC|Tailscale|SealedSecret|compose-smoke|JDK 21|product-catalog|Qwen|Kafka|sslmode=disable" go/AGENTS.md frontend/AGENTS.md services/AGENTS.md java/AGENTS.md k8s/AGENTS.md .github/AGENTS.md docs/AGENTS.md
```

Expected: each phrase appears in the appropriate child instruction file.

### Task 3: Rewrite Root Router

**Files:**
- Modify: `AGENTS.md`

- [ ] **Step 1: Replace root with compact router**

Rewrite `AGENTS.md` to include only:

- project intent and quality bar
- search scope and context discipline
- pull-based docs loading policy
- compact project map
- child `AGENTS.md` routing index
- safety gates and skills
- branch/autonomy rules
- verification expectations
- root growth guardrail

The root must explicitly state that detailed architecture, service internals,
environment inventories, and troubleshooting runbooks belong in the narrowest
directory `AGENTS.md`, a skill, or a triggered doc.

- [ ] **Step 2: Verify root line count**

Run:

```bash
wc -l AGENTS.md
```

Expected: root is roughly 80-120 lines.

- [ ] **Step 3: Verify routing index mentions every child**

Run:

```bash
rg -n "go/AGENTS.md|frontend/AGENTS.md|services/AGENTS.md|java/AGENTS.md|k8s/AGENTS.md|\\.github/AGENTS.md|docs/AGENTS.md" AGENTS.md
```

Expected: all seven child instruction files are mentioned.

### Task 4: Final Verification And Local Commit

**Files:**
- Verify: all changed `AGENTS.md` files

- [ ] **Step 1: Verify docs search still respects ignore rules**

Run:

```bash
rg --files docs | head -50
```

Expected: command exits successfully and does not require broad loading of ignored docs.

- [ ] **Step 2: Review diff shape**

Run:

```bash
git diff --stat
git diff -- AGENTS.md go/AGENTS.md frontend/AGENTS.md services/AGENTS.md java/AGENTS.md k8s/AGENTS.md .github/AGENTS.md docs/AGENTS.md
```

Expected: diff is a docs-only reorganization with no application behavior changes.

- [ ] **Step 3: Commit locally**

Run:

```bash
git add AGENTS.md go/AGENTS.md frontend/AGENTS.md services/AGENTS.md java/AGENTS.md k8s/AGENTS.md .github/AGENTS.md docs/AGENTS.md docs/superpowers/plans/2026-04-29-codex-context-hygiene.md
git commit -m "docs: refactor agent context instructions"
```

Expected: commit succeeds. Do not push because this is a docs-only change unless Kyle explicitly asks.

## Self-Review

- Spec coverage: root shrink, pull-based docs policy, child routing, root growth guardrail, autonomy rules, verification checks, and local docs-only commit are covered.
- Placeholder scan: no `TBD`, `TODO`, `implement later`, or unspecified test steps.
- Type consistency: all referenced paths are concrete repo paths and all verification commands match the changed files.
