# CI Quality Gate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a GitHub Actions quality gate so image builds, QA deploys, and production deploys cannot proceed unless the existing deploy-relevant checks succeed.

**Architecture:** Add one lightweight aggregate `quality-gate` job to `.github/workflows/ci.yml`. Wire image builds and production deploys to require that gate, while preserving the existing environment-specific deploy and smoke-test flow.

**Tech Stack:** GitHub Actions YAML, Bash, existing repo Makefile and CI workflow.

---

## File Structure

- Modify: `.github/workflows/ci.yml`
  - Add the `quality-gate` job after `security-cors-check` and before `build-images`.
  - Change `build-images.needs` to depend on `quality-gate`.
  - Change `deploy-prod.needs` and `deploy-prod.if` so production deploys require successful gate and image build results.
- Create: `docs/superpowers/plans/2026-05-20-ci-quality-gate.md`
  - This implementation plan.

No tests or helper scripts should be added. The change is workflow orchestration only.

## Execution Preconditions

Because this implementation changes CI/CD behavior, do not work directly on
`qa`. Before editing `.github/workflows/ci.yml`, create or select a feature
worktree under `.codex/worktrees/<branch-name>/` using the
`superpowers:using-git-worktrees` skill. After the worktree exists, run all
commands from that worktree.

Use this branch name unless a newer branch already exists for this exact task:

```bash
ci-quality-gate
```

---

### Task 1: Create The Feature Worktree

**Files:**
- Modify: none

- [ ] **Step 1: Invoke the required worktree skill**

Use `superpowers:using-git-worktrees` before creating or selecting the feature
worktree.

- [ ] **Step 2: Create the feature branch and worktree**

Run from the original repo root:

```bash
mkdir -p .codex/worktrees
git worktree add -b ci-quality-gate .codex/worktrees/ci-quality-gate qa
```

Expected: Git creates `.codex/worktrees/ci-quality-gate` on branch
`ci-quality-gate`.

- [ ] **Step 3: Confirm all following work runs inside the worktree**

Run:

```bash
pwd
git branch --show-current
git rev-parse --show-toplevel
git status --short
```

Expected:

```text
/Users/kylebradshaw/repos/gen_ai_engineer/.codex/worktrees/ci-quality-gate
ci-quality-gate
/Users/kylebradshaw/repos/gen_ai_engineer/.codex/worktrees/ci-quality-gate
```

`git status --short` should not show task-related changes yet. If unrelated
changes are present, do not modify or revert them.

---

### Task 2: Add The Quality Gate Job

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Locate the insertion point**

Run:

```bash
rg -n "security-cors-check:|# ---------- Build Images ----------" .github/workflows/ci.yml
```

Expected: output shows `security-cors-check` before the `Build Images` section.

- [ ] **Step 2: Insert the gate job**

In `.github/workflows/ci.yml`, insert this job after `security-cors-check` and
before the `# ---------- Build Images ----------` comment:

```yaml
  quality-gate:
    name: Quality Gate
    runs-on: ubuntu-latest
    if: always()
    needs:
      - grafana-dashboard-sync
      - python-lint
      - python-tests
      - java-lint
      - java-unit-tests
      - java-integration-tests
      - go-lint
      - go-tests
      - buf-breaking
      - go-migration-test
      - frontend-checks
      - e2e-mocked
      - k8s-manifest-validation
      - compose-smoke
      - compose-smoke-go
      - compose-smoke-java
      - security-bandit
      - security-pip-audit
      - security-npm-audit
      - security-gitleaks
      - security-hadolint
      - security-cors-check
    steps:
      - name: Summarize required checks
        env:
          EVENT_NAME: ${{ github.event_name }}
          REF_NAME: ${{ github.ref_name }}
          GRAFANA_DASHBOARD_SYNC: ${{ needs.grafana-dashboard-sync.result }}
          PYTHON_LINT: ${{ needs.python-lint.result }}
          PYTHON_TESTS: ${{ needs.python-tests.result }}
          JAVA_LINT: ${{ needs.java-lint.result }}
          JAVA_UNIT_TESTS: ${{ needs.java-unit-tests.result }}
          JAVA_INTEGRATION_TESTS: ${{ needs.java-integration-tests.result }}
          GO_LINT: ${{ needs.go-lint.result }}
          GO_TESTS: ${{ needs.go-tests.result }}
          BUF_BREAKING: ${{ needs.buf-breaking.result }}
          GO_MIGRATION_TEST: ${{ needs.go-migration-test.result }}
          FRONTEND_CHECKS: ${{ needs.frontend-checks.result }}
          E2E_MOCKED: ${{ needs.e2e-mocked.result }}
          K8S_MANIFEST_VALIDATION: ${{ needs.k8s-manifest-validation.result }}
          COMPOSE_SMOKE: ${{ needs.compose-smoke.result }}
          COMPOSE_SMOKE_GO: ${{ needs.compose-smoke-go.result }}
          COMPOSE_SMOKE_JAVA: ${{ needs.compose-smoke-java.result }}
          SECURITY_BANDIT: ${{ needs.security-bandit.result }}
          SECURITY_PIP_AUDIT: ${{ needs.security-pip-audit.result }}
          SECURITY_NPM_AUDIT: ${{ needs.security-npm-audit.result }}
          SECURITY_GITLEAKS: ${{ needs.security-gitleaks.result }}
          SECURITY_HADOLINT: ${{ needs.security-hadolint.result }}
          SECURITY_CORS_CHECK: ${{ needs.security-cors-check.result }}
        run: |
          set -euo pipefail

          failed=0

          require_success() {
            local name="$1"
            local result="$2"

            printf '%-32s %s\n' "$name" "$result"
            if [ "$result" != "success" ]; then
              echo "::error title=Quality gate blocked::${name} finished with result '${result}'"
              failed=1
            fi
          }

          allow_event_skip() {
            local name="$1"
            local result="$2"
            local allowed_reason="$3"

            printf '%-32s %s\n' "$name" "$result"
            case "$result" in
              success)
                ;;
              skipped)
                echo "::notice title=Quality gate skip::${name} skipped (${allowed_reason})"
                ;;
              *)
                echo "::error title=Quality gate blocked::${name} finished with result '${result}'"
                failed=1
                ;;
            esac
          }

          echo "Quality gate context: event=${EVENT_NAME}, ref=${REF_NAME}"
          echo "Required check results:"
          require_success "grafana-dashboard-sync" "$GRAFANA_DASHBOARD_SYNC"
          require_success "python-lint" "$PYTHON_LINT"
          require_success "python-tests" "$PYTHON_TESTS"
          require_success "java-lint" "$JAVA_LINT"
          require_success "java-unit-tests" "$JAVA_UNIT_TESTS"
          require_success "java-integration-tests" "$JAVA_INTEGRATION_TESTS"
          require_success "go-lint" "$GO_LINT"
          require_success "go-tests" "$GO_TESTS"
          allow_event_skip "buf-breaking" "$BUF_BREAKING" "runs only on pull_request"
          require_success "go-migration-test" "$GO_MIGRATION_TEST"
          require_success "frontend-checks" "$FRONTEND_CHECKS"
          allow_event_skip "e2e-mocked" "$E2E_MOCKED" "runs on pull_request and qa pushes"
          require_success "k8s-manifest-validation" "$K8S_MANIFEST_VALIDATION"
          require_success "compose-smoke" "$COMPOSE_SMOKE"
          require_success "compose-smoke-go" "$COMPOSE_SMOKE_GO"
          require_success "compose-smoke-java" "$COMPOSE_SMOKE_JAVA"
          require_success "security-bandit" "$SECURITY_BANDIT"
          require_success "security-pip-audit" "$SECURITY_PIP_AUDIT"
          require_success "security-npm-audit" "$SECURITY_NPM_AUDIT"
          require_success "security-gitleaks" "$SECURITY_GITLEAKS"
          require_success "security-hadolint" "$SECURITY_HADOLINT"
          require_success "security-cors-check" "$SECURITY_CORS_CHECK"

          if [ "$failed" -ne 0 ]; then
            echo "One or more required quality checks did not succeed. Blocking image builds and deploys."
            exit 1
          fi

          echo "All required quality checks passed."
```

- [ ] **Step 3: Confirm the new job is present once**

Run:

```bash
rg -n "quality-gate:|name: Quality Gate|Quality gate blocked" .github/workflows/ci.yml
```

Expected: output includes exactly one `quality-gate:` line, one
`name: Quality Gate` line, and the gate error messages inside the new job.

---

### Task 3: Put Image Builds Behind The Gate

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Replace the `build-images.needs` list**

In `.github/workflows/ci.yml`, change the `build-images` job dependency block
from the current long list to:

```yaml
    needs:
      - quality-gate
```

The surrounding job header should read:

```yaml
  build-images:
    name: Build Image (${{ matrix.service }})
    runs-on: ubuntu-latest
    if: github.event_name == 'push'
    needs:
      - quality-gate
```

- [ ] **Step 2: Confirm the image build dependency**

Run:

```bash
sed -n '1170,1195p' .github/workflows/ci.yml
```

Expected: the `build-images` job has only `quality-gate` in its `needs` block.

---

### Task 4: Tighten Production Deploy Dependencies

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Add `quality-gate` to `deploy-prod.needs`**

In `.github/workflows/ci.yml`, change the production deploy dependency block
to include `quality-gate`:

```yaml
    needs:
      - changes
      - quality-gate
      - build-images
      - security-gitleaks
      - security-hadolint
```

Keep `changes` because production still uses its outputs for selective restart
behavior. Keep the existing direct security dependencies for now to minimize
behavior churn in the production deploy job.

- [ ] **Step 2: Replace the production deploy condition**

Replace the existing `deploy-prod.if` block with:

```yaml
    if: |
      github.ref == 'refs/heads/main' && github.event_name == 'push' &&
      needs.quality-gate.result == 'success' &&
      needs.build-images.result == 'success'
```

This removes the broad `always()` condition from production deploy and avoids
deploying after cancelled or unexpectedly skipped required gates.

- [ ] **Step 3: Confirm the production deploy gate**

Run:

```bash
sed -n '1600,1625p' .github/workflows/ci.yml
```

Expected: `deploy-prod` lists `quality-gate` in `needs`, and the `if` block
requires both `needs.quality-gate.result == 'success'` and
`needs.build-images.result == 'success'`.

---

### Task 5: Validate Workflow Syntax And Dependency Shape

**Files:**
- Modify: none

- [ ] **Step 1: Parse workflow YAML**

Run:

```bash
ruby -e 'require "yaml"; YAML.load_file(".github/workflows/ci.yml"); puts "workflow yaml parses"'
```

Expected:

```text
workflow yaml parses
```

- [ ] **Step 2: Run actionlint when available**

Run:

```bash
if command -v actionlint >/dev/null 2>&1; then
  actionlint .github/workflows/ci.yml
else
  echo "actionlint not installed; YAML parse check completed"
fi
```

Expected if `actionlint` is installed: no output and exit code `0`.

Expected if `actionlint` is not installed:

```text
actionlint not installed; YAML parse check completed
```

- [ ] **Step 3: Inspect the relevant workflow graph**

Run:

```bash
rg -n "quality-gate:|build-images:|deploy-qa:|deploy-prod:|needs:|needs\\.quality-gate|needs\\.build-images" .github/workflows/ci.yml
```

Expected:

- `quality-gate` appears before `build-images`.
- `build-images` depends on `quality-gate`.
- `deploy-qa` still depends on `build-images` and `e2e-mocked`.
- `deploy-prod` depends on `quality-gate` and `build-images`.
- `deploy-prod.if` requires successful `quality-gate` and `build-images`.

- [ ] **Step 4: Review the focused diff**

Run:

```bash
git diff -- .github/workflows/ci.yml
```

Expected: the diff only adds the `quality-gate` job and updates the
`build-images` and `deploy-prod` dependency/condition blocks.

---

### Task 6: Commit The Implementation

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Confirm only intended implementation files changed**

Run:

```bash
git status --short
```

Expected: `.github/workflows/ci.yml` is modified. The plan document may also be
present if this plan was created in the same worktree. Do not stage unrelated
files.

- [ ] **Step 2: Commit the workflow change**

Run:

```bash
git add .github/workflows/ci.yml
git commit -m "ci: gate deploys on required checks"
```

Expected: commit succeeds. Pre-commit hooks may skip non-matching file types.

- [ ] **Step 3: Confirm clean implementation state**

Run:

```bash
git status --short
```

Expected: no task-related changes remain. If unrelated pre-existing changes
remain, leave them untouched and mention them in the handoff.

---

### Task 7: Push And Open The Pull Request

**Files:**
- Modify: none

- [ ] **Step 1: Push the feature branch**

Run:

```bash
git push -u origin ci-quality-gate
```

Expected: branch pushes successfully.

- [ ] **Step 2: Create a PR to `qa`**

Run:

```bash
gh pr create \
  --base qa \
  --head ci-quality-gate \
  --title "ci: gate deploys on required checks" \
  --body "Adds a Quality Gate job that blocks image builds and deploys unless deploy-relevant checks succeed. QA now uses the same gate before deployment, reducing production surprises."
```

Expected: GitHub CLI prints the new PR URL.

- [ ] **Step 3: Do not watch CI**

Per repo branch workflow, stop after PR creation unless Kyle explicitly asks to
inspect a failed run.
