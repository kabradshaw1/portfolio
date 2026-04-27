# CI/CD Pipeline Hardening

- **Date:** 2026-04-27
- **Status:** Approved
- **Spec marker:** `ci-pipeline-hardening-design`

## Context

The CI/CD pipeline (`.github/workflows/ci.yml`, currently 1,567 lines) handles all quality gates and deployment for the portfolio. PR #164 introduced a composite action `./.github/actions/check-changes` that resolves an exact compare base from `github.event.before` (push) or `github.event.pull_request.base.sha` (PR), with `HEAD~5` as a fallback. That action was wired into three jobs: `build-and-push-images`, `go-tests`, and `go-lint`.

The same overshoot still affects ten other matrix or always-on jobs. Three independent improvements close the remaining gaps:

1. **Extend the composite action** to the rest of the matrix and always-on jobs that should be path-gated.
2. **Add container image vulnerability scanning** between build and deploy. The pipeline currently lints Dockerfiles (`hadolint`) and audits source dependencies (`pip-audit`, `npm audit`) but never scans the *built* images for CVEs in OS packages or transitive dependencies that don't appear in lockfiles.
3. **Backfill pre-commit hooks** so the local "shift-left" framework catches the issues CI catches today, not a strict subset of them. Currently pre-commit lints Python via ruff, lints partial Go (auth + order only), runs Java checkstyle, and runs frontend tsc/lint/build — but misses bandit, hadolint, and 5 of 7 Go services.

These three initiatives are thematically related (CI hardening) but independently shippable. This spec covers the design for all three; implementation will land as three separate PRs.

## Goals

- A docs-only or single-stack push completes CI faster by skipping unrelated matrix entries.
- Built images are vulnerability-scanned before they reach QA or production.
- Local pre-commit hooks cover the same surface as CI for the cheap, fast checks (lint, SAST, style).

## Non-goals

- No new orchestration platforms or CI providers.
- No reusable workflow refactor for `compose-smoke-*` or `build-and-push-images` (mentioned as "lower ROI" in the audit; out of scope here).
- No additional security tooling beyond image scanning (no Snyk, no CodeQL, no SBOM generation, no license scanning).
- No changes to deploy steps, smoke tests, or Cloudflare/Vercel integration.
- No changes to which services exist in which matrix (`payment-service` not being in `go-tests` / `go-lint` is flagged elsewhere as a separate item).

## Initiative A — Extend change-detection composite action

### Scope

Apply `./.github/actions/check-changes` (already shipped in PR #164) to eleven more jobs that currently run unconditionally on every push:

| Job | Per-entry path strategy |
| --- | --- |
| `python-tests` (matrix of 4) | `services/<name>` + `services/shared` |
| `security-pip-audit` (matrix of 4) | `services/<name>` + `services/shared` |
| `java-unit-tests` (matrix of 4) | `java/<service>` + `java/build.gradle` + `java/settings.gradle` |
| `java-integration-tests` (single job) | `java/**` |
| `frontend-checks` | `frontend/**` |
| `compose-smoke-python` | `services/**` + `docker-compose.yml` + nginx routing |
| `compose-smoke-go` | `go/**` + Go compose file (if separate) |
| `compose-smoke-java` | `java/**` + Java compose file |
| `k8s-validation` | `k8s/**` |
| `go-migration-test` | `go/*/migrations/**` + relevant migration tooling |
| `security-hadolint` (matrix per Dockerfile) | the specific Dockerfile path |

`grafana-dashboard-sync`, `python-lint`, `java-lint`, `buf-breaking`, `security-bandit`, `security-npm-audit`, `security-gitleaks`, `security-cors-guardrail` are deliberately left **always-on** — they are either fast (sub-30s), specifically gated by content type already (e.g. `buf-breaking` runs `git diff` for proto changes inline), or perform repo-wide checks where path-gating would create a false sense of safety (e.g. gitleaks must scan whatever is being pushed regardless of which subtree it touches).

### Pattern

Each gated job converts its matrix to use `include:` with explicit `paths:` per entry, mirroring the existing `build-and-push-images` style:

```yaml
strategy:
  matrix:
    include:
      - service: ingestion
        paths: services/ingestion services/shared
      - service: chat
        paths: services/chat services/shared
      ...
```

Each gated job adds a `Check for changes` step using the composite action and conditions all subsequent steps on `if: steps.changes.outputs.changed == 'true'`.

`fetch-depth: 50` is required on the checkout so the PR base / push before SHA is reachable in the shallow clone.

### Workflow-file safeguard

A change to `.github/workflows/ci.yml` or anything under `.github/actions/**` should trigger **every** matrix entry to run, not skip them. Without this safeguard a refactor to the workflow could ship without exercising the matrix paths it altered.

The `paths` for every gated entry is therefore extended to include `.github/workflows/ci.yml .github/actions/**`. This is a small, mechanical addition per entry and makes pipeline-validation runs explicit.

### Path-set discipline

For each gated job, the `paths` value must encompass:
1. The service's own source tree
2. Any shared libraries the service imports (`services/shared`, `go/pkg`, `java/build.gradle`, etc.)
3. The workflow safeguard paths (`.github/workflows/ci.yml`, `.github/actions/**`)

Every entry in `python-tests` and `security-pip-audit` for the same service uses the **identical** paths. Drift between sibling matrices is the failure mode that ADR 07 was originally trying to prevent — same path-set per service across all gated jobs avoids it.

### Acceptance criteria

- Pushing a docs-only commit shows all ten gated jobs as green with their main steps skipped (matrix instances themselves stay green; downstream `needs:` consumers are not blocked).
- Pushing a Python-only commit (e.g. `services/chat/...`) skips Java unit tests, frontend checks, k8s validation, Go migration test, compose-smoke-go, and compose-smoke-java.
- Editing `.github/workflows/ci.yml` triggers every matrix entry, regardless of which application paths were touched.
- A change to `services/shared` triggers every Python service's tests, pip-audit, and the Python compose smoke (because every service entry includes `services/shared`).
- Composite action invocation is identical to PR #164 — same inputs, same outputs.

## Initiative B — Container image vulnerability scanning

### Tool selection

**Trivy** via `aquasecurity/trivy-action`. Reasons:

- Most-adopted OSS scanner; broad CVE database (NVD, GitHub Advisory, OS-specific feeds).
- First-party GHA action; mature, actively maintained.
- Native SARIF output that uploads to GitHub Security tab via `github/codeql-action/upload-sarif`, so findings appear on PRs without separate UI.
- Faster than Grype on equivalent scans; comparable accuracy.
- No vendor account, no licence, no API key — runs entirely in-action.

**Rejected alternatives:**

- **Grype** (Anchore): comparable feature set, slightly slower, smaller community.
- **Snyk Container**: requires account + token, free tier rate-limited, vendor lock-in for advisory data.
- **Docker Scout**: tied to Docker Hub authentication patterns, less ergonomic for GHCR.

### Pipeline placement

A new job `image-scan` runs **after** `build-and-push-images` and **before** `deploy-qa` / `deploy-prod`. It scans the freshly-pushed GHCR image (not the source tree), gated by the same `check-changes` composite action used by `build-and-push-images` so unchanged services skip the scan too.

```yaml
image-scan:
  name: Image Scan (${{ matrix.service }})
  runs-on: ubuntu-latest
  needs: build-and-push-images
  permissions:
    contents: read
    packages: read
    security-events: write   # required for SARIF upload
  strategy:
    fail-fast: false
    matrix:
      include:
        # Same matrix as build-and-push-images; reuse via YAML anchor or duplicate.
  steps:
    - uses: actions/checkout@v4
      with:
        fetch-depth: 50
    - name: Check for changes
      id: changes
      uses: ./.github/actions/check-changes
      with:
        paths: ${{ matrix.paths }}
    - name: Log in to GHCR
      if: steps.changes.outputs.changed == 'true'
      ...
    - name: Run Trivy
      if: steps.changes.outputs.changed == 'true'
      uses: aquasecurity/trivy-action@master
      with:
        image-ref: ghcr.io/${{ github.repository_owner }}/${{ matrix.image }}:${{ github.sha }}
        format: sarif
        output: trivy-${{ matrix.service }}.sarif
        severity: HIGH,CRITICAL
        exit-code: '1'
        ignore-unfixed: true
        trivyignores: .trivyignore
    - name: Upload SARIF
      if: always() && steps.changes.outputs.changed == 'true'
      uses: github/codeql-action/upload-sarif@v3
      with:
        sarif_file: trivy-${{ matrix.service }}.sarif
        category: trivy-${{ matrix.service }}
```

The deploy jobs (`deploy-qa`, `deploy-prod`) extend their `needs:` array to include `image-scan`. If the scan fails, deploys do not run.

### Severity policy

- **CRITICAL, HIGH** → fail the build (`exit-code: '1'`)
- **MEDIUM, LOW, UNKNOWN** → reported in SARIF output but do not fail
- **`ignore-unfixed: true`** → CVEs without an upstream patch are noise; an unfixable HIGH in a base-image library can't be acted on, so don't block on them

### Allowlist mechanism

A `.trivyignore` file at the repo root holds known acknowledged CVEs. Each entry is one CVE ID with a comment explaining why it's ignored:

```
# .trivyignore
# CVE-XXXX-YYYY — false positive, openssl version detected by trivy is wrong (#issue-link)
CVE-XXXX-YYYY
# CVE-AAAA-BBBB — accepted risk, only triggered by configurable feature we don't use
CVE-AAAA-BBBB
```

Adding a CVE to `.trivyignore` is a code change, reviewed in PR. There is no other path to dismissing a finding.

### Concurrency / cost

The matrix runs in parallel; per-image scan time is 30-90s. Total wall-clock added to the pipeline is bounded by the slowest single image scan, not the cumulative time. Skipped (unchanged) services don't add anything.

### Acceptance criteria

- A new high-severity CVE in any built image fails the deploy on the next build.
- Findings appear under the repository's Security tab, scoped per service.
- Adding a CVE to `.trivyignore` with a justification comment unblocks the build without code changes elsewhere.
- Running on an unchanged service shows the scan job as green-with-skipped, not failed.

## Initiative C — Pre-commit hook backfill

### Current state

`.pre-commit-config.yaml` defines:
- `gitleaks` (secrets) — repo-wide
- `ruff` + `ruff-format` — `services/**`
- `java-checkstyle` — `java/**/*.java`
- `frontend-typecheck` (`tsc --noEmit`) — `frontend/**/*.{ts,tsx}`
- `frontend-lint` (`npm run lint`) — `frontend/**/*.{ts,tsx,js,jsx}`
- `frontend-build` (`next build`) — pre-push only
- `go-lint` — only `auth-service` and `order-service` (5 services missing)

### Add

#### bandit (Python SAST)

```yaml
- repo: https://github.com/PyCQA/bandit
  rev: 1.7.9
  hooks:
    - id: bandit
      args: ["-c", "pyproject.toml"]
      additional_dependencies: ["bandit[toml]"]
      files: ^services/
```

Mirrors the CI `security-bandit` job. Bandit is fast (sub-5s on this repo's surface) and catches Python anti-patterns before CI does.

#### hadolint (Dockerfile lint)

```yaml
- repo: https://github.com/hadolint/hadolint
  rev: v2.12.0
  hooks:
    - id: hadolint
      files: Dockerfile(\..+)?$
```

Mirrors the CI `security-hadolint` matrix. Catches Dockerfile issues (latest tag, missing USER, etc.) at commit time.

#### Full go-lint coverage

Replace the existing hardcoded `auth-service + order-service` entry with one entry per Go service. Loop pattern in the local hook:

```yaml
- id: go-lint
  name: Go Lint (all services)
  entry: bash -c 'set -e; for svc in auth-service order-service ai-service analytics-service product-service cart-service payment-service order-projector; do
    echo "--- $svc ---"
    (cd "go/$svc" && ~/go/bin/golangci-lint run ./...)
  done'
  language: system
  files: ^go/.*\.go$
  pass_filenames: false
```

Note `payment-service` is included even though it's missing from CI's `go-tests` / `go-lint` matrices — covering it locally costs nothing and surfaces issues earlier.

### Don't add

- **pip-audit / npm audit** — network-dependent, slow (10-60s), gated to CI.
- **trivy** — scans built images, not source. CI-only.
- **prettier** for frontend — `eslint-config-next` already covers formatting; adding prettier would create dual-source-of-truth.

### Onboarding

Add a Make target so new contributors install the hooks once:

```makefile
.PHONY: install-pre-commit
install-pre-commit:
	@command -v pre-commit >/dev/null 2>&1 || { echo "Install pre-commit first: pip install pre-commit"; exit 1; }
	pre-commit install --install-hooks
	pre-commit install --hook-type pre-push --install-hooks
	@echo "✅ pre-commit hooks installed (commit + pre-push stages)"
```

Document in `CLAUDE.md` under "Pre-commit Requirements" section.

### Acceptance criteria

- `pre-commit run --all-files` passes on a clean main branch.
- Adding a hardcoded password to a Python file is caught locally, not in CI.
- A Dockerfile change with a `latest` tag fails locally.
- A lint error in `go/payment-service` is caught locally.
- `make install-pre-commit` is a one-liner for a new clone.

## Implementation phasing

Three independent PRs against `qa`. Order doesn't matter; each is shippable on its own.

### PR 1 — Initiative A (composite action expansion)

- Convert ten matrices to `include:` with `paths:` entries
- Add `Check for changes` step + `if:` gates per job
- Bump `fetch-depth: 50` on each affected checkout
- Touches only `.github/workflows/ci.yml`
- Risk: low (mechanical change, same pattern as PR #164)

### PR 2 — Initiative B (image scanning)

- Add `image-scan` job with Trivy + SARIF upload
- Wire `image-scan` into `needs:` for `deploy-qa` and `deploy-prod`
- Add empty `.trivyignore` with comment header explaining the format
- Touches `.github/workflows/ci.yml` and creates `.trivyignore`
- Risk: medium (new job; first run will surface previously-invisible CVEs and may immediately need entries in `.trivyignore` before merging — implementation will run the scan locally first to surface findings)

### PR 3 — Initiative C (pre-commit backfill)

- Update `.pre-commit-config.yaml` with bandit, hadolint, full go-lint
- Add `make install-pre-commit` target to `Makefile`
- Update `CLAUDE.md` "Pre-commit Requirements" section
- Touches `.pre-commit-config.yaml`, `Makefile`, `CLAUDE.md`
- Risk: low (developer tooling only — CI is unaffected)

## Documentation

Single ADR `docs/adr/ci-cd-pipeline-evolution.md` records the design decisions: change-detection compare-base strategy, image-scan policy, and the local-vs-CI split for security tooling. Written after all three PRs land, so it reflects the final state.

`CLAUDE.md` updated in PR 3 to include:
- The new `image-scan` job in the CI/CD pipeline table
- `make install-pre-commit` in the "Pre-commit Requirements" section
- Note that pushing a docs-only commit will skip most matrix jobs (so the user knows the green-with-skipped state is normal)

## Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| Path-set drift between sibling matrices (e.g. `python-tests` and `security-pip-audit` for the same service) | Identical `paths:` value reused per service across all gated jobs. Spec-level rule. |
| Workflow refactor accidentally breaks change-detection without exercising the changed matrices | Workflow path safeguard: every gated entry includes `.github/workflows/ci.yml .github/actions/**` so a CI change re-runs everything. |
| First Trivy run reveals undismissable CRITICAL CVEs that block deploy | Run scan locally before merging PR 2. Pre-populate `.trivyignore` with any acknowledged-and-justified findings. Open issues for fixable findings before merging. |
| `payment-service` is locally linted by pre-commit but not CI-tested | Documented as known gap (separate spec). Pre-commit catch-rate is best-effort, not a substitute for CI. |
| Hadolint findings on existing Dockerfiles fail pre-commit on first install | Run `pre-commit run hadolint --all-files` locally as part of PR 3, fix findings inline. |

## Out of scope (deliberately)

- Reusable workflows for `compose-smoke-*` (lower-ROI cleanup, not blocking anything).
- `payment-service` addition to `go-tests` / `go-lint` matrices (correctness, not speed; separate spec).
- Path filtering for `python-lint`, `java-lint`, `security-bandit`, `security-npm-audit`, `security-gitleaks`, `security-cors-guardrail`, `grafana-dashboard-sync`, `buf-breaking` — fast or repo-wide, gating buys little.
- SBOM generation, license scanning, supply-chain attestation (Sigstore / cosign).
- CodeQL or other deep SAST.
- Per-PR vulnerability comments (Trivy's SARIF output already surfaces in the Security tab and PR checks).

## Acceptance criteria summary (cross-cutting)

- A docs-only push on `qa` finishes CI in noticeably less time (target: most matrix jobs green-with-skipped, only repo-wide checks running).
- A Python-only PR doesn't run Java unit tests, Go test/lint, k8s validation, or compose-smoke-go/-java.
- New CVEs in built images fail the deploy with allowlist as the only escape hatch.
- A new contributor can clone, run `make install-pre-commit`, and commit with full local lint coverage.
