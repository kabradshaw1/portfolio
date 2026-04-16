# Security Assessment

An evaluation of the security and DevSecOps controls currently implemented in this portfolio project. Each finding cites the exact file(s) that implement the control so every claim can be independently verified.

**Scope:** the entire repository — Python AI services, Go and Java microservices, Next.js frontend, Kubernetes manifests, CI/CD workflows, and supporting scripts.

**Methodology:** source-code review of `.github/workflows/`, Dockerfiles, Kubernetes manifests, application auth code, and operational scripts. No dynamic testing was performed.

**Originally written:** 2026-04-08
**Last updated:** 2026-04-16 — incorporates the application-layer security audit (`docs/superpowers/specs/2026-04-15-security-audit-hardening-design.md`) and adds a §11 pointer to the new host-OS hardening assessment ([`linux-server-hardening.md`](linux-server-hardening.md)).

## Summary of findings

| Area | Status | Notes |
|---|---|---|
| Shift-left security in CI | **Strong** | Six gating jobs covering SAST, SCA, secret scanning, Dockerfile lint, and a custom CORS guardrail |
| Infrastructure-as-Code validation | **Strong** | `kubeconform`, `kind` server-side dry-run, and a custom policy-as-code script |
| Supply chain | **Adequate for portfolio** | Multi-stage non-root builds and a private registry; no image signing |
| Secrets management | **Strong** | Gitignored files, full-history secret scanning, and k8s-native `secretKeyRef` injection |
| Application AuthN/AuthZ | **Strong** | JWT + bcrypt + Google OAuth 2.0, httpOnly cookies, JWT propagated and independently validated downstream, Redis denylist for token revocation |
| Transport security | **Adequate** | TLS terminated at Cloudflare; no direct internet exposure of the backend |
| Developer guardrails | **Strong** | Pre-commit hooks, preflight Makefile targets, and a structured branch workflow |
| Post-deploy verification | **Strong** | Production Playwright smoke tests plus a new pre-deploy compose-smoke job |
| Kubernetes runtime posture | **Adequate** | Pod security contexts (`runAsNonRoot`, `readOnlyRootFilesystem`, `capabilities.drop:[ALL]`) on every Deployment; `NetworkPolicy` and namespace-level PSS still gaps |
| Observability | **Foundation only** | Metrics and dashboards exist; no security monitoring or alerting |
| Host / OS hardening | **Strong** | Debian 13 host hardened per [`linux-server-hardening.md`](linux-server-hardening.md) — UFW, SSH-Tailscale-only, narrow sudo, auditd, sysctl, lynis baseline 77 |

The overall posture is strong for a portfolio-scale project. The most notable remaining gaps are at the Kubernetes runtime layer (`NetworkPolicy` and namespace-level Pod Security Standards), documented as accepted risks below.

---

## 1. Shift-left security in CI

**Status:** Strong.

Six dedicated security jobs in `.github/workflows/ci.yml` gate every push and block deployment on failure.

| Control | Tool | Evidence |
|---|---|---|
| Python SAST | Bandit `-ll` (high-confidence only) | `.github/workflows/ci.yml:461-489` (`security-bandit`) |
| Python dependency CVE scan | `pip-audit` per service | `.github/workflows/ci.yml:491-521` (`security-pip-audit`) |
| JavaScript dependency CVE scan | `npm audit --audit-level=high` | `.github/workflows/ci.yml:522-542` (`security-npm-audit`) |
| Full-history secret scan | `gitleaks-action@v2` with custom allowlist | `.github/workflows/ci.yml:544-555`, `.gitleaks.toml` |
| Dockerfile lint | Hadolint across 9 Dockerfiles | `.github/workflows/ci.yml:557-578` |
| CORS guardrail | `grep` for `allow_origins=["*"]` | `.github/workflows/ci.yml:580-594` |
| Java dependency tree (manual review) | OWASP Dependency Check (Gradle) | `.github/workflows/java-ci.yml:88-105` |
| Automated dependency PRs | Dependabot (pip / npm / Actions, weekly) | `.github/dependabot.yml` |

**Effectiveness:** the `deploy` job declares `needs:` on gitleaks, Hadolint, and all build jobs (`.github/workflows/ci.yml:630-643`), so a single security failure blocks production deployment. Dependabot feeds into the same pipeline, so proposed dependency updates are subject to the same gates.

**Limitations:** the Java OWASP Dependency Check currently generates a report but does not fail the build; it is effectively an informational control.

---

## 2. Infrastructure-as-Code validation

**Status:** Strong.

This layer was built in response to a real production incident on 2026-04-08, where Kubernetes manifests without a `readinessProbe` on postgres and without `sslmode=disable` on the Go services' `DATABASE_URL` caused cascading migration failures.

- **`kubeconform`** — static Kubernetes OpenAPI schema validation across `k8s/`, `java/k8s/`, and `go/k8s/`. Evidence: `.github/workflows/ci.yml:352-404`.
- **`kind` cluster with server-side `--dry-run=server`** — real API-server admission validation, not only schema validation. Catches CRD references, admission webhooks, and cross-resource validation.
- **Custom policy-as-code script** — `scripts/k8s-policy-check.sh` enforces two rules derived from the 2026-04-08 incident:
  - **R1:** any `Deployment` whose container image is `postgres`, `mongo`, or `redis` must define a `readinessProbe`. Addresses a class of bug where `kubectl rollout status` returns before the database binds its port, allowing downstream Jobs to race startup.
  - **R2:** any `ConfigMap` data key ending in `DATABASE_URL` and starting with `postgres://` must include `sslmode=disable`. Addresses the `lib/pq` driver's default of `sslmode=require`, which fails against a non-SSL postgres.
- **Test harness** at `scripts/test-k8s-policy-check.sh` covers five fixtures, including both positive and negative cases for each rule plus a non-applicable case to prevent overreach.

**Effectiveness:** the policy script has already surfaced real regressions — missing probes on `mongodb` and `redis` Deployments — during the implementation of this assessment, forcing them to be remediated before merge.

---

## 3. Supply chain

**Status:** Adequate for a portfolio project.

- **Multi-stage Docker builds** on every service. The builder stage compiles code; the final image contains only artifacts on a slim or Alpine base, reducing attack surface. Evidence: `go/auth-service/Dockerfile:1-11`, `services/chat/Dockerfile`.
- **Non-root containers** — explicit `USER appuser` (uid 1001 on Go) across all 9 services. Evidence: `services/chat/Dockerfile:13-14`, `go/auth-service/Dockerfile:17,23`.
- **Private registry (GHCR)** with `imagePullSecrets` on every Deployment and `GITHUB_TOKEN`-based push authentication.
- **Pinned tool versions** in CI for reproducibility: `kubeconform v0.6.7`, `golang-migrate v4.17.0`, `yq v4.44.3`, `ruff v0.11.6`, and Hadolint via a pinned action SHA.

**Accepted risks:**
- Images are tagged `:latest` rather than pinned to commit SHAs or digests, which means the tag is mutable. This is acceptable for portfolio use but would not be acceptable in production.
- No image signing (Cosign / SLSA provenance) or attestation verification. The registry is private and push is gated by the CI workflow, which is a partial mitigation.

---

## 4. Secrets management

**Status:** Strong.

- **Gitignored secrets:** both `.env` and `**/k8s/secrets/*.yml` are gitignored (`.gitignore:18-23`). Only `*.yml.template` files are committed, providing a public contract for required secret fields without leaking values.
- **Kubernetes-native secret injection:** all production secrets are stored as `Secret` resources and injected at the Deployment level via `valueFrom.secretKeyRef`. They are never baked into images or ConfigMaps. Evidence: `java/k8s/deployments/task-service.yml:27-46`, `go/k8s/deployments/auth-service.yml:28-42`.
- **GitHub Actions secrets** (`TAILSCALE_AUTHKEY`, `SSH_PRIVATE_KEY`, `SMOKE_GO_PASSWORD`, `GITHUB_TOKEN`) are the only place CI-level credentials are referenced.
- **Full-history gitleaks scan** runs with `fetch-depth: 0`, so the entire repository history is scanned, not only the latest commit.

**Observations:**
- Secret rotation is manual. The `TAILSCALE_AUTHKEY` is documented as requiring rotation every 90 days (`CLAUDE.md:181`), but no automation enforces this.
- There is no envelope encryption (SOPS / sealed-secrets / external-secrets-operator). Kubernetes Secrets are therefore only as well protected as the cluster's etcd backing store, which in a Minikube deployment is not encrypted at rest.

---

## 5. Application AuthN/AuthZ

**Status:** Strong. The Go and Java stacks independently implement the same patterns, which demonstrates the patterns rather than the framework choices. The Python AI services share an auth module so all three independently validate JWTs at the edge.

- **JWT + bcrypt.** 15-minute access tokens and 7-day refresh tokens, HMAC-SHA256 signed, with bcrypt password hashes. Evidence: `go/auth-service/internal/service/auth.go`, `java/task-service/src/main/java/.../SecurityConfig.java`.
- **JWT signing-method validation.** Both Go services explicitly check `t.Method.(*jwt.SigningMethodHMAC)` before validating signatures, defending against the "alg=none" and key-confusion attack class. Evidence: `go/auth-service/internal/service/auth.go:74`, `go/ecommerce-service/internal/middleware/auth.go:27`.
- **Token revocation via Redis denylist.** On `POST /auth/logout`, the auth-service hashes the token (SHA-256) and writes it to Redis under `auth:denied:<hash>` with TTL matching the access-token expiration. Auth middleware checks the denylist before accepting any token. Evidence: `go/auth-service/internal/handler/auth.go:152-165` (`Logout`), `go/auth-service/internal/service/token_denylist.go:25-31` (`Revoke`).
- **httpOnly cookies for both stacks.** `access_token` and `refresh_token` are set with `HttpOnly: true`, `Secure: true`, `SameSite=Lax` on both Go and Java. The frontend never touches the JWT in JavaScript, removing a whole class of XSS-token-theft attacks. Evidence: `go/auth-service/internal/handler/auth.go:50-54` (Go cookies), `java/task-service/.../controller/AuthController.java:56-82` (Java cookies), `java/task-service/.../security/JwtAuthenticationFilter.java:30-60` (Java filter that reads the cookie when the Authorization header is absent).
- **JWT propagation, not header trust.** The Java gateway forwards `Authorization: Bearer <token>` to downstream services; `task-service`, `activity-service`, and `notification-service` each validate the JWT independently using the shared secret. This replaces the earlier "trust the `X-User-Id` header from the gateway" model — a compromised pod in another namespace can no longer forge identity by injecting a header. Evidence: `java/gateway-service/src/main/java/.../SecurityConfig.java`, `docs/adr/java-task-management/03_authentication_and_security.md`.
- **Google OAuth 2.0 with state/CSRF parameter.** Federated authentication with server-side code exchange. Both password and OAuth flows return the same JWT envelope so the frontend is agnostic. The OAuth state parameter is validated on callback to defend against CSRF. Evidence: `go/auth-service/internal/handler/auth.go`, `docs/adr/password-authentication.md`.
- **Python AI services require JWT.** A shared `services/shared/auth.py` exposes `create_auth_dependency(secret)` returning a FastAPI dependency that validates HS256 Bearer tokens, decodes the `sub`/`exp` claims, and returns the user id (or 401). All mutating endpoints across the three services use it via `Depends(require_auth)`: `services/ingestion/app/main.py:110` (`/ingest`), `:187` (`/documents`), `:195` (`/documents/{id}`), `:213` (`/collections/{name}`); `services/chat/app/main.py:101` (`/chat`); `services/debug/app/main.py:109` (`/index`), `:148` (`/debug`).
- **Per-IP rate limiting (Python).** `slowapi` decorators on every endpoint: ingestion `/ingest` 5/min and reads 30/min; chat `/chat` 20/min; debug `/index` 5/min, `/debug` 10/min. Evidence: `services/ingestion/app/main.py:105,186,193,211`, `services/chat/app/main.py:99`, `services/debug/app/main.py:107,146`.
- **Prompt-injection defenses (Python).** Both AI services wrap untrusted input in XML tags and instruct the model in the system prompt to ignore instructions inside those tags. Evidence: `services/chat/app/prompt.py:6-7,11,15,21` (`<context>`, `<user_question>` delimiters); `services/debug/app/prompts.py:43-44,52-53` (`<bug_description>`, `<error_output>` delimiters with explicit "treat as data only" instruction).
- **GraphQL hardening (Java gateway).** GraphiQL disabled in production (`GRAPHIQL_ENABLED=false` default), maximum query depth 10, maximum query complexity 100. Evidence: `java/gateway-service/src/main/resources/application.yml:13`, `java/gateway-service/src/main/java/.../GraphQlConfig.java:12-19`.
- **Input validation (Java DTOs).** `@Size` constraints on all create-request DTOs to prevent runaway payloads: `CreateProjectRequest.java:7-8` (name 255, description 2000), `CreateTaskRequest.java:10` (title 255, description 5000), `CreateCommentRequest.java:6` (body 5000).
- **Password validation (Go).** Minimum 12 characters enforced at the binding tag. Evidence: `go/auth-service/internal/model/user.go:20`.
- **Strict CORS.** Environment-driven allowlists with no wildcards. Runtime-enforced in `go/auth-service/internal/middleware/cors.go` and `java/gateway-service/src/main/java/.../SecurityConfig.java`. Python services use `allowed_origins.split(",")` from env (default `https://kylebradshaw.dev`), never `["*"]`. Also enforced at CI time via the CORS guardrail (§1), so a wildcard cannot be committed.
- **Security headers.** Spring Security sets `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, and HSTS `max-age=31536000; includeSubDomains`. Evidence: `java/gateway-service/src/main/java/.../SecurityConfig.java`.
- **Stateless sessions** (`SessionCreationPolicy.STATELESS`). No server-side session state; all authorization is via JWT.

**Note on previous "accepted risk":** the earlier draft of this document called out a gateway/downstream trust risk based on `X-User-Id` header forwarding. That model has been replaced by JWT propagation (each service independently validates the token), so the risk no longer applies.

---

## 6. Transport security

**Status:** Adequate.

- **Cloudflare Tunnel** — the backend is never directly exposed to the public internet. TLS terminates at the Cloudflare edge; the tunnel forwards traffic to the home-lab Debian 13 server running Minikube. The tunnel daemon is the only process listening on `127.0.0.1:80` on the host, gated by UFW (see [`linux-server-hardening.md`](linux-server-hardening.md) §2). Evidence: `CLAUDE.md:36-40`.
- **NGINX Ingress** with path-based routing inside Minikube.
- **Frontend on Vercel** with managed HTTPS; Cloudflare-managed certificates for the API subdomain.

**Observations:** intra-cluster traffic between pods is unencrypted. For a home-lab deployment this is acceptable; production workloads with sensitive data would require a service mesh (e.g. Linkerd, Istio) for mTLS.

---

## 7. Developer guardrails

**Status:** Strong.

- **Pre-commit hooks** for ruff, Checkstyle, `tsc`, ESLint, and golangci-lint, plus a `pre-push` stage that runs `next build`. Evidence: `.pre-commit-config.yaml`.
- **`make preflight*` targets** mirror CI locally so issues are caught before pushing. Evidence: `Makefile`.
- **Structured branch workflow** — `feature → staging → main`, with distinct CI triggers per branch and a deploy-only-on-`main`-push guard. Staging runs mocked Playwright E2E tests; `main` runs deploy and production smoke tests.

**Observations:** the branch workflow is enforced by convention rather than branch protection rules. A sufficiently motivated contributor (or an errant script) could push directly to `main`. For a single-maintainer portfolio this is acceptable.

---

## 8. Pre-deploy and post-deploy verification

**Status:** Strong.

- **Production smoke tests** — Playwright suite against `https://api.kylebradshaw.dev` after every deploy. Tests cover frontend load, health checks, document upload, chat query, and cleanup. Evidence: `.github/workflows/ci.yml:716-749`.
- **Compose smoke CI job** — stands up the full Python stack via `docker-compose.ci.yml` with a mocked Ollama stub (`services/mock-ollama/`) and runs a RAG happy-path Playwright test (`frontend/e2e/smoke-ci.spec.ts`). Catches contract drift before a staging merge.
- **Go migration pipeline test** — runs the real `golang-migrate` auth and ecommerce migrations plus seed SQL against a postgres service container on every push. Reproduces the 2026-04-08 incident failure modes (sslmode, Job ordering, cross-service schema references) in CI.
- **Readiness probes** on every stateful service (postgres, mongo, redis) — made non-skippable by the policy-as-code rule R1 in §2.

---

## 9. Kubernetes runtime posture

**Status:** Adequate. Pod-level controls are now in place across every Deployment; namespace-level enforcement and network isolation remain gaps.

- **Pod security contexts on every Deployment.** `runAsNonRoot: true`, `readOnlyRootFilesystem: true`, `allowPrivilegeEscalation: false`, and `capabilities.drop: ["ALL"]` are set on the container `securityContext` of every workload. Evidence: `java/k8s/deployments/activity-service.yml:47-53`, `gateway-service.yml:52-59`, `notification-service.yml:46-53`, `task-service.yml:67-74`; `go/k8s/deployments/ai-service.yml:37-44`, `auth-service.yml:47-54`, `ecommerce-service.yml:37-44`.

**Remaining gaps (accepted risk):**

- **No `NetworkPolicy`.** Pods across all namespaces can reach each other without restriction. With JWT propagation in place (§5), this is no longer a confused-deputy auth risk, but lateral movement after a single-pod compromise is still unconstrained.
- **No Pod Security Standards (PSS) profile** enforced at the namespace level — the per-Deployment `securityContext` blocks are unenforced *defaults* rather than namespace-mandated policy. A new Deployment without those fields would not be rejected.

A single commit adding PSS `restricted` labels to the `ai-services`, `java-tasks`, and `go-ecommerce` namespaces plus targeted `NetworkPolicy` resources would close the bulk of this remaining surface. See "Recommended next steps" below.

---

## 10. Observability

**Status:** Foundation only. Not a security monitoring control in its current form.

- **Prometheus** scrapes every service, the host, and the GPU (`k8s/monitoring/configmaps/prometheus-config.yml`).
- **Grafana dashboard** at `https://grafana.kylebradshaw.dev` (public read-only).
- **Health endpoints** on every service, used by readiness probes and production smoke tests.

**Gaps:** there is no alerting on authentication failures, no anomaly detection, no SIEM integration, no centralized log aggregation (Loki / ELK), and no application-layer audit logging of privileged actions. For security purposes the current stack is observational rather than detective. (The host OS does have local `auditd` — see §11.)

---

## 11. Host / OS hardening

**Status:** Strong. Detailed in [`linux-server-hardening.md`](linux-server-hardening.md); summarized here.

The Debian 13 home-lab server that runs Minikube, Ollama, and the Cloudflare Tunnel was hardened on 2026-04-16:

- **SSH locked to Tailscale.** Listener binds only `100.82.52.82:22` (Tailscale IP) and `127.0.0.1:22` — never `0.0.0.0:22`. Public-internet SSH is gone.
- **UFW default-deny firewall** with narrow allow rules. Ollama is firewall-fenced to docker bridges and the tailnet (no LAN exposure).
- **Narrow passwordless sudo allowlist** for routine ops (`systemctl`, `journalctl`, `apt`, `kubectl`, `minikube`, `docker`, `ufw status`, `lynis audit system`). Privilege-changing actions still require password.
- **`auditd`** with immutable baseline rules covering identity files, sudo/sshd configs, and privilege-escalation invocations. Persistent `journald` with retention caps.
- **`sysctl` kernel hardening** drop-in (`kptr_restrict`, `ptrace_scope`, `unprivileged_bpf_disabled`, network-stack hygiene).
- **Patch management fix.** Discovered that `unattended-upgrades` had been silently doing nothing since the migration because `/etc/apt/sources.list` was missing the `security.debian.org` entry. Fixed; 15 stale patches applied including kernel `6.12.73 → 6.12.74`.
- **Lynis hardening index: 77** (target was ≥75).

For implementation rationale and as-built decisions, see the engineering ADR at [`docs/adr/2026-04-16-debian-host-hardening.md`](../adr/2026-04-16-debian-host-hardening.md).

---

## Recommended next steps

Ordered by impact-to-effort ratio:

1. **Add a PSS `restricted` label** to the `ai-services`, `java-tasks`, and `go-ecommerce` namespaces and fix any resulting violations. The per-Deployment `securityContext` blocks (§9) already satisfy `restricted` — labeling the namespaces enforces it on future workloads.
2. **Add minimal `NetworkPolicy` resources** — default-deny ingress in each namespace, plus explicit allow rules for gateway→downstream and ingress-controller→gateway. Limits lateral movement after a single-pod compromise.
3. **Forward host audit logs to a remote sink.** §11's `auditd` and `journald` are local-only — a remote forwarder (Loki via `journald-remote`, or a SIEM) makes the audit trail survive a host compromise. Pairs naturally with §10 alerting work.
4. **Pin production container images by digest** (`@sha256:…`) instead of `:latest`. Eliminates the mutable-tag supply chain risk.
5. **Promote Java OWASP Dependency Check from reporting to gating.** One-line CI change.
6. **Add Prometheus alert rules** for 4xx/5xx rate spikes and authentication failure bursts. Converts observability into detection.
7. **Run a dedicated IaC scanner** (e.g. `kube-linter`, `checkov`) alongside the existing custom policy script. Catches the broader class of rules the homegrown script was intentionally narrow about.
8. **Migrate to envelope-encrypted secrets** (SOPS with age, or external-secrets-operator with a cloud KMS) to remove dependence on Kubernetes Secrets' at-rest model.

---

## Evidence index

Every file path cited in this assessment, for quick navigation:

- `.github/workflows/ci.yml` — primary CI/CD workflow (Python, frontend, security, k8s validation, deploy, smoke)
- `.github/workflows/java-ci.yml` — Java CI workflow
- `.github/workflows/go-ci.yml` — Go CI workflow
- `.github/dependabot.yml` — automated dependency updates
- `.gitleaks.toml` — secret scanner allowlist
- `.gitignore` — secret and environment file exclusions
- `.pre-commit-config.yaml` — pre-commit hooks
- `Makefile` — preflight targets
- `scripts/k8s-policy-check.sh` — custom policy-as-code
- `scripts/test-k8s-policy-check.sh` — policy script test harness
- `services/mock-ollama/` — CI-only Ollama stub for compose smoke tests
- `docker-compose.ci.yml` — compose overlay for CI smoke runs
- `frontend/e2e/smoke.spec.ts` — post-deploy production smoke tests
- `frontend/e2e/smoke-ci.spec.ts` — pre-deploy compose smoke tests
- `go/auth-service/internal/service/auth.go` — JWT, bcrypt, and signing-method validation
- `go/auth-service/internal/handler/auth.go` — login/refresh/logout handlers, httpOnly cookie setup
- `go/auth-service/internal/service/token_denylist.go` — Redis token revocation
- `go/auth-service/internal/middleware/cors.go` — Go CORS enforcement
- `go/auth-service/internal/model/user.go` — password validation (min 12 chars)
- `go/ecommerce-service/internal/middleware/auth.go` — JWT signing-method validation in ecommerce
- `java/gateway-service/src/main/java/.../SecurityConfig.java` — Spring Security configuration
- `java/gateway-service/src/main/java/.../config/GraphQlConfig.java` — GraphQL depth/complexity limits
- `java/gateway-service/src/main/resources/application.yml` — GraphiQL disabled in prod
- `java/task-service/src/main/java/.../SecurityConfig.java` — Spring Security configuration
- `java/task-service/src/main/java/.../controller/AuthController.java` — httpOnly cookie setup
- `java/task-service/src/main/java/.../security/JwtAuthenticationFilter.java` — cookie-based JWT extraction
- `java/k8s/deployments/*.yml` — Deployment manifests with `securityContext` (runAsNonRoot, readOnlyRootFilesystem, capabilities.drop)
- `go/k8s/deployments/*.yml` — Deployment manifests with `securityContext`
- `go/k8s/configmaps/*.yml` — Go service ConfigMaps with `sslmode=disable`
- `services/shared/auth.py` — shared FastAPI JWT dependency (`create_auth_dependency`)
- `services/ingestion/app/main.py`, `services/chat/app/main.py`, `services/debug/app/main.py` — `Depends(require_auth)` on every mutating endpoint, slowapi rate-limit decorators
- `services/chat/app/prompt.py`, `services/debug/app/prompts.py` — XML-delimited prompt templates with injection-defense system instructions
- `docs/adr/password-authentication.md` — auth architecture ADR
- `docs/adr/java-task-management/03_authentication_and_security.md` — Java auth ADR
- `docs/security/linux-server-hardening.md` — companion host-OS assessment
- `docs/adr/2026-04-16-debian-host-hardening.md` — host-hardening engineering ADR
- `docs/superpowers/specs/2026-04-15-security-audit-hardening-design.md` — application-layer audit spec
- `docs/superpowers/specs/2026-04-16-debian-host-hardening-design.md` — host hardening spec
- `docs/security/lynis-baseline-2026-04-16.log` — captured lynis baseline report
