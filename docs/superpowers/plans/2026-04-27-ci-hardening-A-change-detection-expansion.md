# CI Hardening — Initiative A: Change-Detection Expansion (Plan)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Apply the existing `./.github/actions/check-changes` composite action to eleven more matrix and always-on jobs in `.github/workflows/ci.yml` so that unrelated subsystems skip work on pushes that don't touch them.

**Architecture:** Pure CI workflow edit. Each gated job converts its matrix to `include:` with explicit `paths:` per entry, adds a `Check for changes` step using the composite action, and conditions all subsequent steps on `if: steps.changes.outputs.changed == 'true'`. The composite action itself (shipped in PR #164) is unchanged.

**Tech Stack:** GitHub Actions YAML, the existing `aquasecurity`-style `actions/checkout@v4` + composite action invocation pattern.

**Spec:** `docs/superpowers/specs/2026-04-27-ci-pipeline-hardening-design.md` — Initiative A.

**File structure:**

| File | Status | Responsibility |
| --- | --- | --- |
| `.github/workflows/ci.yml` | Modify | Eleven jobs converted to use composite action + path-gated matrices |

No other files change in this PR.

**Verification per task:** YAML syntax check (`python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"`). Final verification: push to a feature branch and observe matrix instances skipping correctly on a docs-only follow-up commit (covered in Task 13).

**Workflow safeguard rule:** Every gated entry's `paths` value must include `.github/workflows/ci.yml .github/actions/check-changes/action.yml` so that pipeline edits re-run all matrices.

---

### Task 1: python-tests — gate by per-service paths

**Files:**
- Modify: `.github/workflows/ci.yml` (the `python-tests:` job)

- [ ] **Step 1: Replace the python-tests job**

Find the existing `python-tests:` job (currently around line 52-96). Replace its body with:

```yaml
  python-tests:
    name: Python Tests (${{ matrix.service }})
    runs-on: ubuntu-latest
    strategy:
      matrix:
        include:
          - service: ingestion
            paths: services/ingestion services/shared .github/workflows/ci.yml .github/actions/check-changes/action.yml
          - service: chat
            paths: services/chat services/shared .github/workflows/ci.yml .github/actions/check-changes/action.yml
          - service: debug
            paths: services/debug services/shared .github/workflows/ci.yml .github/actions/check-changes/action.yml
          - service: eval
            paths: services/eval services/shared .github/workflows/ci.yml .github/actions/check-changes/action.yml
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 50

      - name: Check for changes
        id: changes
        uses: ./.github/actions/check-changes
        with:
          paths: ${{ matrix.paths }}

      - name: Set up Python
        if: steps.changes.outputs.changed == 'true'
        uses: actions/setup-python@v5
        with:
          python-version: "3.11"

      - name: Cache virtualenv
        if: steps.changes.outputs.changed == 'true'
        uses: actions/cache@v4
        id: venv-cache
        with:
          path: .venv
          key: venv-${{ matrix.service }}-${{ hashFiles(format('services/{0}/requirements.txt', matrix.service), 'services/shared/pyproject.toml') }}

      - name: Install dependencies
        if: steps.changes.outputs.changed == 'true' && steps.venv-cache.outputs.cache-hit != 'true'
        run: |
          python -m venv .venv
          source .venv/bin/activate
          pip install services/shared/
          pip install -r services/${{ matrix.service }}/requirements.txt

      - name: Run tests with coverage
        if: steps.changes.outputs.changed == 'true'
        env:
          PYTHONPATH: services
        run: |
          source .venv/bin/activate
          pytest services/${{ matrix.service }}/tests/ -v \
            --cov=services/${{ matrix.service }}/app \
            --cov-report=term-missing \
            --cov-report=xml:coverage-${{ matrix.service }}.xml

      - name: Upload coverage report
        if: steps.changes.outputs.changed == 'true' && always()
        uses: actions/upload-artifact@v4
        with:
          name: coverage-${{ matrix.service }}
          path: coverage-${{ matrix.service }}.xml
```

- [ ] **Step 2: Validate YAML**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"`
Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git -c commit.gpgsign=false commit -m "ci: gate python-tests matrix on per-service path changes"
```

---

### Task 2: java-unit-tests — gate by per-service paths

**Files:**
- Modify: `.github/workflows/ci.yml` (the `java-unit-tests:` job)

- [ ] **Step 1: Replace the java-unit-tests job**

Find the existing `java-unit-tests:` job (currently around line 115-144). Replace its body with:

```yaml
  java-unit-tests:
    name: Java Unit Tests (${{ matrix.service }})
    runs-on: ubuntu-latest
    strategy:
      matrix:
        include:
          - service: task-service
            paths: java/task-service java/build.gradle java/settings.gradle java/checkstyle.xml .github/workflows/ci.yml .github/actions/check-changes/action.yml
          - service: activity-service
            paths: java/activity-service java/build.gradle java/settings.gradle java/checkstyle.xml .github/workflows/ci.yml .github/actions/check-changes/action.yml
          - service: notification-service
            paths: java/notification-service java/build.gradle java/settings.gradle java/checkstyle.xml .github/workflows/ci.yml .github/actions/check-changes/action.yml
          - service: gateway-service
            paths: java/gateway-service java/build.gradle java/settings.gradle java/checkstyle.xml .github/workflows/ci.yml .github/actions/check-changes/action.yml
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 50

      - name: Check for changes
        id: changes
        uses: ./.github/actions/check-changes
        with:
          paths: ${{ matrix.paths }}

      - name: Set up JDK 21
        if: steps.changes.outputs.changed == 'true'
        uses: actions/setup-java@v4
        with:
          distribution: temurin
          java-version: "21"
          cache: gradle

      - name: Run unit tests
        if: steps.changes.outputs.changed == 'true'
        working-directory: java
        run: ./gradlew :${{ matrix.service }}:test --no-daemon --stacktrace

      - name: Upload test report
        if: steps.changes.outputs.changed == 'true' && always()
        uses: actions/upload-artifact@v4
        with:
          name: java-test-report-${{ matrix.service }}
          path: java/${{ matrix.service }}/build/reports/tests/
```

Note: if `java/checkstyle.xml` doesn't exist at the listed path, omit it from the `paths` value. The path is included defensively because checkstyle config affects test compilation.

- [ ] **Step 2: Validate YAML**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"`
Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git -c commit.gpgsign=false commit -m "ci: gate java-unit-tests matrix on per-service path changes"
```

---

### Task 3: java-integration-tests — gate on java/**

**Files:**
- Modify: `.github/workflows/ci.yml` (the `java-integration-tests:` job)

- [ ] **Step 1: Add change detection to java-integration-tests**

Find the existing `java-integration-tests:` job (currently around line 146-168). It is a single job (no matrix). Add the change-detection step and condition the subsequent steps. The new body:

```yaml
  java-integration-tests:
    name: Java Integration Tests
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 50

      - name: Check for changes
        id: changes
        uses: ./.github/actions/check-changes
        with:
          paths: java .github/workflows/ci.yml .github/actions/check-changes/action.yml

      - name: Set up JDK 21
        if: steps.changes.outputs.changed == 'true'
        uses: actions/setup-java@v4
        with:
          distribution: temurin
          java-version: "21"
          cache: gradle

      - name: Run integration tests
        if: steps.changes.outputs.changed == 'true'
        working-directory: java
        run: ./gradlew :task-service:integrationTest --no-daemon --stacktrace

      - name: Upload test report
        if: steps.changes.outputs.changed == 'true' && always()
        uses: actions/upload-artifact@v4
        with:
          name: java-test-report-integration
          path: java/task-service/build/reports/tests/
```

- [ ] **Step 2: Validate YAML and commit**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"
git add .github/workflows/ci.yml
git -c commit.gpgsign=false commit -m "ci: gate java-integration-tests on java/** changes"
```

---

### Task 4: frontend-checks — gate on frontend/**

**Files:**
- Modify: `.github/workflows/ci.yml` (the `frontend-checks:` job)

- [ ] **Step 1: Add change detection**

Find the `frontend-checks:` job (currently around line 336). Read its full body, then add a `Check for changes` step **after** the checkout but **before** the setup-node step. Add `if: steps.changes.outputs.changed == 'true'` to every subsequent step that uses Node, runs lint, runs tsc, or runs the build.

The pattern to apply (modify in place — preserve all existing flags and commands):

```yaml
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 50

      - name: Check for changes
        id: changes
        uses: ./.github/actions/check-changes
        with:
          paths: frontend .github/workflows/ci.yml .github/actions/check-changes/action.yml

      # ALL existing subsequent steps below get: if: steps.changes.outputs.changed == 'true'
      # (combined with any existing if: condition using &&)
      - name: Set up Node
        if: steps.changes.outputs.changed == 'true'
        uses: actions/setup-node@v4
        ... (preserve rest verbatim)
```

Apply the `if: steps.changes.outputs.changed == 'true'` gate to **every** step in the job after the change-detection step. If a step already has an `if:` (e.g., `if: always()`), combine: `if: steps.changes.outputs.changed == 'true' && always()`.

- [ ] **Step 2: Validate YAML and commit**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"
git add .github/workflows/ci.yml
git -c commit.gpgsign=false commit -m "ci: gate frontend-checks on frontend/** changes"
```

---

### Task 5: compose-smoke (Python stack) — gate on services/** + compose

**Files:**
- Modify: `.github/workflows/ci.yml` (the `compose-smoke:` job — Python smoke)

- [ ] **Step 1: Add change detection**

Find `compose-smoke:` (currently around line 459). Add a `Check for changes` step right after the checkout, with `paths: services docker-compose.yml nginx .github/workflows/ci.yml .github/actions/check-changes/action.yml`. Gate every subsequent step on `if: steps.changes.outputs.changed == 'true'`.

Apply the same `if:` combination rule for steps that already have `if: always()` etc.

The change-detection step block to insert:

```yaml
      - name: Check for changes
        id: changes
        uses: ./.github/actions/check-changes
        with:
          paths: services docker-compose.yml nginx .github/workflows/ci.yml .github/actions/check-changes/action.yml
```

Set `fetch-depth: 50` on the checkout.

- [ ] **Step 2: Validate YAML and commit**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"
git add .github/workflows/ci.yml
git -c commit.gpgsign=false commit -m "ci: gate compose-smoke (python) on services/** + compose changes"
```

---

### Task 6: compose-smoke-go — gate on go/** + compose

**Files:**
- Modify: `.github/workflows/ci.yml` (the `compose-smoke-go:` job)

- [ ] **Step 1: Add change detection**

Find `compose-smoke-go:` (currently around line 532). Look near the top of the job for the docker-compose file path it references (commonly `docker-compose.go.yml` or similar — observe the actual filename when reading the job).

Apply the same pattern as Task 5: checkout with fetch-depth 50, `Check for changes` step with `paths: go <go-compose-file> .github/workflows/ci.yml .github/actions/check-changes/action.yml`, gate all subsequent steps on `if: steps.changes.outputs.changed == 'true'`.

The change-detection step block (replace `<go-compose-file>` with the actual path the job uses; if it's `docker-compose.go.yml`, use that):

```yaml
      - name: Check for changes
        id: changes
        uses: ./.github/actions/check-changes
        with:
          paths: go docker-compose.go.yml .github/workflows/ci.yml .github/actions/check-changes/action.yml
```

If the job uses a different compose file name, substitute it. If there is no separate compose file (the job uses `docker-compose.yml`), use that instead.

- [ ] **Step 2: Validate YAML and commit**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"
git add .github/workflows/ci.yml
git -c commit.gpgsign=false commit -m "ci: gate compose-smoke-go on go/** + compose changes"
```

---

### Task 7: compose-smoke-java — gate on java/** + compose

**Files:**
- Modify: `.github/workflows/ci.yml` (the `compose-smoke-java:` job)

- [ ] **Step 1: Add change detection**

Find `compose-smoke-java:` (currently around line 648). Same pattern as Task 6.

Insert this block right after the checkout (set `fetch-depth: 50`):

```yaml
      - name: Check for changes
        id: changes
        uses: ./.github/actions/check-changes
        with:
          paths: java docker-compose.java.yml .github/workflows/ci.yml .github/actions/check-changes/action.yml
```

(Substitute the actual Java compose file name if different.)

Gate all subsequent steps on `if: steps.changes.outputs.changed == 'true'`.

- [ ] **Step 2: Validate YAML and commit**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"
git add .github/workflows/ci.yml
git -c commit.gpgsign=false commit -m "ci: gate compose-smoke-java on java/** + compose changes"
```

---

### Task 8: security-pip-audit — gate by per-service paths

**Files:**
- Modify: `.github/workflows/ci.yml` (the `security-pip-audit:` job)

- [ ] **Step 1: Replace the security-pip-audit job**

Find `security-pip-audit:` (currently around line 748). Read its body to capture the exact pip-audit command. Then replace the matrix and add path gating using the same pattern as Task 1 (`python-tests`) — same per-service `paths:` value (`services/<name> services/shared .github/workflows/ci.yml .github/actions/check-changes/action.yml`).

Convert the matrix to `include:` form:

```yaml
    strategy:
      matrix:
        include:
          - service: ingestion
            paths: services/ingestion services/shared .github/workflows/ci.yml .github/actions/check-changes/action.yml
          - service: chat
            paths: services/chat services/shared .github/workflows/ci.yml .github/actions/check-changes/action.yml
          - service: debug
            paths: services/debug services/shared .github/workflows/ci.yml .github/actions/check-changes/action.yml
          - service: eval
            paths: services/eval services/shared .github/workflows/ci.yml .github/actions/check-changes/action.yml
```

Add checkout with `fetch-depth: 50`, the `Check for changes` step, and gate every other step on `if: steps.changes.outputs.changed == 'true'`. Preserve the existing pip-audit command verbatim.

- [ ] **Step 2: Validate YAML and commit**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"
git add .github/workflows/ci.yml
git -c commit.gpgsign=false commit -m "ci: gate security-pip-audit matrix on per-service path changes"
```

---

### Task 9: security-hadolint — gate per Dockerfile path

**Files:**
- Modify: `.github/workflows/ci.yml` (the `security-hadolint:` job)

- [ ] **Step 1: Convert hadolint matrix to per-Dockerfile path entries**

Find `security-hadolint:` (currently around line 850). It uses a matrix of `dockerfile` paths. Convert each entry to `include:` form with a `paths:` key equal to the Dockerfile path itself, plus the workflow safeguard:

```yaml
    strategy:
      matrix:
        include:
          - dockerfile: <path/to/Dockerfile>
            paths: <path/to/Dockerfile> .github/workflows/ci.yml .github/actions/check-changes/action.yml
          # ... one entry per Dockerfile in the existing matrix
```

Read the current matrix to enumerate all Dockerfile paths. For each existing entry like `dockerfile: services/ingestion/Dockerfile`, the new entry becomes:

```yaml
          - dockerfile: services/ingestion/Dockerfile
            paths: services/ingestion/Dockerfile .github/workflows/ci.yml .github/actions/check-changes/action.yml
```

Add checkout with `fetch-depth: 50` and the `Check for changes` step. Gate the hadolint-action invocation on `if: steps.changes.outputs.changed == 'true'`.

- [ ] **Step 2: Validate YAML and commit**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"
git add .github/workflows/ci.yml
git -c commit.gpgsign=false commit -m "ci: gate security-hadolint matrix on per-Dockerfile changes"
```

---

### Task 10: k8s-validation — gate on k8s/**

**Files:**
- Modify: `.github/workflows/ci.yml` (the `k8s-validation` job — find by name "K8s Manifest Validation")

- [ ] **Step 1: Add change detection**

Find the K8s validation job (currently around line 399). Add `Check for changes` after the checkout with:

```yaml
      - name: Check for changes
        id: changes
        uses: ./.github/actions/check-changes
        with:
          paths: k8s .github/workflows/ci.yml .github/actions/check-changes/action.yml
```

Set `fetch-depth: 50` on the checkout. Gate every subsequent step on `if: steps.changes.outputs.changed == 'true'`.

- [ ] **Step 2: Validate YAML and commit**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"
git add .github/workflows/ci.yml
git -c commit.gpgsign=false commit -m "ci: gate k8s-validation on k8s/** changes"
```

---

### Task 11: go-migration-test — gate on go/*/migrations/**

**Files:**
- Modify: `.github/workflows/ci.yml` (the `go-migration-test:` job)

- [ ] **Step 1: Add change detection**

Find `go-migration-test:` (currently around line 235). The job sets up Postgres and runs migrations for several services. Insert the `Check for changes` step right after the checkout, gating on migration paths:

```yaml
      - name: Check for changes
        id: changes
        uses: ./.github/actions/check-changes
        with:
          paths: go/auth-service/migrations go/auth-service/seed.sql go/order-service/migrations go/order-service/seed.sql go/product-service/migrations go/product-service/seed.sql go/order-projector/migrations .github/workflows/ci.yml .github/actions/check-changes/action.yml
```

Set `fetch-depth: 50` on the checkout. Gate every subsequent step on `if: steps.changes.outputs.changed == 'true'`. The Postgres `services:` block is part of the job spec (not a step) so it remains; the `services` block only spins up the Postgres container when at least one step actually runs (GitHub Actions does this implicitly when all gated steps skip — verify by observing the job runs as success-with-skipped on a no-migration-change push).

- [ ] **Step 2: Validate YAML and commit**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"
git add .github/workflows/ci.yml
git -c commit.gpgsign=false commit -m "ci: gate go-migration-test on migration path changes"
```

Note: GitHub Actions will still spin up the Postgres `services:` container even when all steps skip — this is a known limitation. The waste is small (5-10s pod startup) compared to the ~60-90s the job's actual steps take. If observed cost becomes meaningful, follow-up PR could move the Postgres setup behind the gate as the first conditional step.

---

### Task 12: Verify YAML still parses end-to-end

**Files:** No file changes — verification only.

- [ ] **Step 1: Final YAML parse**

```bash
python3 -c "import yaml; data = yaml.safe_load(open('.github/workflows/ci.yml')); print('jobs defined:', len(data['jobs']))"
```

Expected: prints the same total number of jobs that existed before this PR (no jobs accidentally deleted).

- [ ] **Step 2: Inspect the diff against main**

```bash
git diff main..HEAD -- .github/workflows/ci.yml | grep -cE '^\+\s+- name: Check for changes'
```

Expected: at least 11 (one Check-for-changes step added per gated job).

---

### Task 13: Push, observe, PR

**Files:** No file changes.

- [ ] **Step 1: Push the feature branch**

```bash
git push -u origin agent/feat-ci-change-detection-expansion
```

(Or whichever branch name you create the worktree with.)

- [ ] **Step 2: Open the PR against qa**

```bash
gh pr create --base qa --title "ci: extend change detection to remaining matrix jobs" --body "$(cat <<'EOF'
## Summary
Extends the `./.github/actions/check-changes` composite action (introduced in PR #164) to the remaining matrix and always-on jobs: python-tests, java-unit-tests, java-integration-tests, frontend-checks, compose-smoke (3 stacks), security-pip-audit, security-hadolint, k8s-validation, go-migration-test.

Spec: docs/superpowers/specs/2026-04-27-ci-pipeline-hardening-design.md (Initiative A)
Plan: docs/superpowers/plans/2026-04-27-ci-hardening-A-change-detection-expansion.md

## Net effect
A docs-only push or single-stack push skips unrelated matrix entries entirely. Workflow file edits trigger every matrix entry (safeguard against silent pipeline regressions).

## Test plan
- [ ] After merge, push a docs-only commit and verify these jobs show as success-with-skipped: python-tests, java-unit-tests, java-integration-tests, frontend-checks, compose-smoke (all 3), security-pip-audit, security-hadolint, k8s-validation, go-migration-test
- [ ] Push a Python-only commit and verify Java + frontend + Go matrices skip
- [ ] Edit ci.yml and verify all matrices re-run regardless of which subtree the test commit modifies
EOF
)"
```

- [ ] **Step 3: Notify Kyle**

Tell Kyle the PR is open. Do not watch CI per CLAUDE.md feature-branch rules.

## Self-Review

### Spec coverage check (Initiative A acceptance criteria)

| Spec criterion | Plan task |
| --- | --- |
| Pushing docs-only commit shows ten gated jobs as green-with-skipped | Tasks 1-11 + verification in Task 13 |
| Python-only commit skips Java tests, frontend, k8s, Go migration, compose-go/-java | Tasks 2, 3, 4, 6, 7, 10, 11 |
| Workflow edit triggers every matrix entry | Workflow safeguard included in every `paths:` value |
| `services/shared` change triggers every Python service | All Python entries (Tasks 1, 8) include `services/shared` |
| Composite action invocation is identical to PR #164 | Same `uses: ./.github/actions/check-changes` everywhere |

No gaps detected.

### Placeholder scan

No "TBD"/"TODO"/"implement later" content. The notes about "if the compose file is named differently, substitute" are deliberate flexibility — the engineer reads the actual filename and applies it. The hadolint matrix instruction tells the engineer to enumerate from the existing entries; this is reasonable because the current set of Dockerfiles is enumerable.

### Type / identifier consistency

- `steps.changes.outputs.changed` referenced consistently in all `if:` conditions.
- `id: changes` used on every Check-for-changes step.
- `fetch-depth: 50` used on every gated checkout.
- `paths:` key used consistently in matrix entries.

All consistent.
