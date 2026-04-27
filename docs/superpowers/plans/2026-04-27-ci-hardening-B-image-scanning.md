# CI Hardening — Initiative B: Container Image Vulnerability Scanning (Plan)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Trivy image scan job between `build-and-push-images` and the deploy jobs. Block deploys when CRITICAL/HIGH CVEs are found in any rebuilt image, with `.trivyignore` as the only escape hatch. Upload findings to the GitHub Security tab via SARIF.

**Architecture:** New `image-scan` matrix job with one entry per service (mirroring `build-and-push-images`). Reuses the `./.github/actions/check-changes` composite action so unchanged services skip the scan. SARIF output uploaded via `github/codeql-action/upload-sarif`. Deploy jobs add `image-scan` to their `needs:` array.

**Tech Stack:** `aquasecurity/trivy-action@master`, `github/codeql-action/upload-sarif@v3`, the existing `./.github/actions/check-changes` composite action.

**Spec:** `docs/superpowers/specs/2026-04-27-ci-pipeline-hardening-design.md` — Initiative B.

**File structure:**

| File | Status | Responsibility |
| --- | --- | --- |
| `.github/workflows/ci.yml` | Modify | Add `image-scan` job; extend `deploy-qa` and `deploy-prod` `needs:` |
| `.trivyignore` | Create | Empty allowlist file with header comment explaining the format |

**Verification per task:** YAML syntax check + a local Trivy scan against the latest QA image to surface findings before they block CI on first run.

**Risk-handling note:** The first run after merge is likely to surface previously-invisible CVEs in built images. Task 4 runs Trivy locally against an existing GHCR image *before* the PR opens so any unavoidable findings can be added to `.trivyignore` in the same PR.

---

### Task 1: Create the `.trivyignore` allowlist file

**Files:**
- Create: `.trivyignore`

- [ ] **Step 1: Create the file**

```bash
cat > .trivyignore <<'EOF'
# .trivyignore — known-acknowledged CVEs for image vulnerability scanning.
#
# Format: one CVE ID per line. Use comment lines starting with `#` to
# explain why each entry is here (false positive, accepted risk, awaiting
# upstream fix, etc.) — entries without justification will be reverted
# during code review.
#
# CRITICAL and HIGH severity CVEs fail the build by default; adding an
# entry here is the only way to dismiss a finding without fixing it.
# MEDIUM/LOW are reported but don't fail.
#
# Trivy reference: https://aquasecurity.github.io/trivy/latest/docs/configuration/filtering/
EOF
```

- [ ] **Step 2: Commit**

```bash
git add .trivyignore
git -c commit.gpgsign=false commit -m "ci: add empty .trivyignore allowlist for image scanning"
```

---

### Task 2: Add the `image-scan` job

**Files:**
- Modify: `.github/workflows/ci.yml` (add new job after `build-and-push-images`)

- [ ] **Step 1: Locate the end of `build-and-push-images`**

Find the existing `build-and-push-images:` job (currently around line 896). Identify its last step. The new `image-scan:` job will be inserted as a new top-level job entry after it.

- [ ] **Step 2: Insert the image-scan job**

Add this new job at the same indentation level as `build-and-push-images` (i.e. as a sibling job under `jobs:`), positioned immediately after `build-and-push-images`:

```yaml
  image-scan:
    name: Image Scan (${{ matrix.service }})
    runs-on: ubuntu-latest
    needs: build-and-push-images
    permissions:
      contents: read
      packages: read
      security-events: write
    strategy:
      fail-fast: false
      matrix:
        include:
          - service: ingestion
            image: ingestion
            paths: services/ingestion services/shared .github/workflows/ci.yml .github/actions/check-changes/action.yml .trivyignore
          - service: chat
            image: chat
            paths: services/chat services/shared .github/workflows/ci.yml .github/actions/check-changes/action.yml .trivyignore
          - service: debug
            image: debug
            paths: services/debug services/shared .github/workflows/ci.yml .github/actions/check-changes/action.yml .trivyignore
          - service: eval
            image: eval
            paths: services/eval services/shared .github/workflows/ci.yml .github/actions/check-changes/action.yml .trivyignore
          - service: java-task-service
            image: java-task-service
            paths: java/task-service java/build.gradle java/settings.gradle .github/workflows/ci.yml .github/actions/check-changes/action.yml .trivyignore
          - service: java-activity-service
            image: java-activity-service
            paths: java/activity-service java/build.gradle java/settings.gradle .github/workflows/ci.yml .github/actions/check-changes/action.yml .trivyignore
          - service: java-notification-service
            image: java-notification-service
            paths: java/notification-service java/build.gradle java/settings.gradle .github/workflows/ci.yml .github/actions/check-changes/action.yml .trivyignore
          - service: java-gateway-service
            image: java-gateway-service
            paths: java/gateway-service java/build.gradle java/settings.gradle .github/workflows/ci.yml .github/actions/check-changes/action.yml .trivyignore
          - service: go-auth-service
            image: go-auth-service
            paths: go/auth-service go/pkg go/go.work .github/workflows/ci.yml .github/actions/check-changes/action.yml .trivyignore
          - service: go-order-service
            image: go-order-service
            paths: go/order-service go/product-service go/cart-service go/pkg go/go.work .github/workflows/ci.yml .github/actions/check-changes/action.yml .trivyignore
          - service: go-ai-service
            image: go-ai-service
            paths: go/ai-service go/pkg go/go.work .github/workflows/ci.yml .github/actions/check-changes/action.yml .trivyignore
          - service: go-analytics-service
            image: go-analytics-service
            paths: go/analytics-service go/pkg go/go.work .github/workflows/ci.yml .github/actions/check-changes/action.yml .trivyignore
          - service: go-product-service
            image: go-product-service
            paths: go/product-service go/pkg go/go.work .github/workflows/ci.yml .github/actions/check-changes/action.yml .trivyignore
          - service: go-cart-service
            image: go-cart-service
            paths: go/cart-service go/product-service go/pkg go/go.work .github/workflows/ci.yml .github/actions/check-changes/action.yml .trivyignore
          - service: go-payment-service
            image: go-payment-service
            paths: go/payment-service go/pkg go/go.work .github/workflows/ci.yml .github/actions/check-changes/action.yml .trivyignore
          - service: go-order-projector
            image: go-order-projector
            paths: go/order-projector go/pkg go/go.work .github/workflows/ci.yml .github/actions/check-changes/action.yml .trivyignore
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
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Run Trivy vulnerability scanner
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

      - name: Upload SARIF to GitHub Security
        if: always() && steps.changes.outputs.changed == 'true'
        uses: github/codeql-action/upload-sarif@v3
        with:
          sarif_file: trivy-${{ matrix.service }}.sarif
          category: trivy-${{ matrix.service }}
```

The `paths:` per entry intentionally mirror the equivalent entries in `build-and-push-images`. If those drift in the future (e.g., a new path is added to a Go service in `build-and-push-images`), it must also be added here.

- [ ] **Step 3: Validate YAML**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"
```

Expected: no output (success).

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/ci.yml
git -c commit.gpgsign=false commit -m "ci: add Trivy image-scan job with SARIF upload"
```

---

### Task 3: Wire `image-scan` into deploy jobs' `needs:`

**Files:**
- Modify: `.github/workflows/ci.yml` (`deploy-qa` and `deploy-prod` jobs)

- [ ] **Step 1: Update `deploy-qa` needs**

Find the `deploy-qa:` job (currently around line 1100). Locate its `needs:` block and add `image-scan` to the list (preserve all existing entries).

Example (the existing `needs:` block plus the new entry — your actual existing entries may differ; preserve them all):

```yaml
  deploy-qa:
    name: Deploy QA
    ...
    needs:
      - build-and-push-images
      - image-scan      # NEW
      # ... preserve all other existing needs entries
```

- [ ] **Step 2: Update `deploy-prod` needs**

Find the `deploy-prod:` job (currently around line 1291). Same change — add `image-scan` to its `needs:` list, preserving everything else.

- [ ] **Step 3: Validate YAML and commit**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"
git add .github/workflows/ci.yml
git -c commit.gpgsign=false commit -m "ci: gate deploys on image-scan job completion"
```

---

### Task 4: Local pre-merge scan to surface findings

**Files:** No file changes — risk mitigation only.

- [ ] **Step 1: Confirm Trivy is installed locally**

```bash
which trivy || brew install trivy
trivy --version
```

If Trivy isn't installed, run the brew install. If on Linux, follow https://aquasecurity.github.io/trivy/latest/getting-started/installation/.

- [ ] **Step 2: Authenticate with GHCR**

```bash
echo "$GITHUB_TOKEN" | docker login ghcr.io -u kabradshaw1 --password-stdin
```

(`GITHUB_TOKEN` here is a personal access token with `read:packages` scope — generate one if needed at https://github.com/settings/tokens.)

- [ ] **Step 3: Pick a recent QA image to scan**

```bash
# Use the most recent image tag for any single service, e.g.:
LATEST_TAG=$(gh api /users/kabradshaw1/packages/container/go-auth-service/versions --jq '.[0].metadata.container.tags[0]')
echo "Latest tag: $LATEST_TAG"
```

- [ ] **Step 4: Scan it with the same flags CI will use**

```bash
trivy image \
  --severity HIGH,CRITICAL \
  --ignore-unfixed \
  --ignorefile .trivyignore \
  --exit-code 1 \
  ghcr.io/kabradshaw1/go-auth-service:$LATEST_TAG
```

Expected outcomes:
- **Exit 0:** No HIGH/CRITICAL findings. CI will pass on first run for this image.
- **Exit 1 with findings listed:** Each finding is either a real CVE (open an issue or fix the dependency) or a false-positive / accepted-risk (add the CVE ID to `.trivyignore` with a justification comment).

- [ ] **Step 5: Repeat for representative images from each language stack**

Scan one Python service (e.g., `chat`), one Java service (e.g., `java-task-service`), and one Go service if the previous scans passed. The goal is to surface any baseline findings before merge so CI doesn't block the first deploy.

- [ ] **Step 6: If findings exist, update `.trivyignore` with justifications**

For each finding kept as accepted risk:

```
# CVE-XXXX-YYYY — <one-line justification, e.g., "openssl detected version is wrong on alpine 3.19, fixed on 3.20 — base-image upgrade tracked in #123">
CVE-XXXX-YYYY
```

Commit any `.trivyignore` updates as part of this PR before merging:

```bash
git add .trivyignore
git -c commit.gpgsign=false commit -m "ci: pre-populate .trivyignore with acknowledged baseline CVEs"
```

If no findings exist, the `.trivyignore` stays empty — this task is satisfied.

---

### Task 5: Push and PR

**Files:** No file changes.

- [ ] **Step 1: Push the branch**

```bash
git push -u origin agent/feat-ci-image-scanning
```

(Or whichever branch name the worktree uses.)

- [ ] **Step 2: Open the PR against qa**

```bash
gh pr create --base qa --title "ci: add Trivy image vulnerability scanning" --body "$(cat <<'EOF'
## Summary
Adds a `image-scan` job that scans every freshly-built GHCR image with Trivy, blocking deploys when HIGH or CRITICAL CVEs are found. Findings upload to the GitHub Security tab as SARIF. The same `.github/actions/check-changes` composite action gates the matrix so unchanged services skip the scan.

Spec: docs/superpowers/specs/2026-04-27-ci-pipeline-hardening-design.md (Initiative B)
Plan: docs/superpowers/plans/2026-04-27-ci-hardening-B-image-scanning.md

## Configuration
- Severity threshold: HIGH, CRITICAL fail the build; MEDIUM/LOW reported only
- `ignore-unfixed: true` — CVEs without an upstream fix don't block (can't be acted on)
- `.trivyignore` is the only escape hatch for acknowledged findings; entries require justification comments

## Pre-merge verification
A local Trivy scan was run against representative images from each language stack to surface baseline findings before this PR opens. Any unavoidable acknowledged-risk CVEs are pre-populated in `.trivyignore` with justifications.

## Test plan
- [ ] After merge, push a Go-service code change and verify only that service's image-scan runs
- [ ] Check the Security tab on the qa branch for SARIF results from the run
- [ ] Verify deploy-qa shows `image-scan` as a dependency in the workflow graph
- [ ] If a deliberate test CVE is introduced (e.g., pin a vulnerable base image), the deploy is blocked
EOF
)"
```

- [ ] **Step 3: Notify Kyle**

Tell Kyle the PR is open. Note in the comment any pre-populated `.trivyignore` entries so they aren't silently merged. Do not watch CI.

## Self-Review

### Spec coverage check (Initiative B acceptance criteria)

| Spec criterion | Plan task |
| --- | --- |
| New high-severity CVE fails the deploy on next build | Task 2 (`exit-code: '1'` on HIGH/CRITICAL) + Task 3 (deploy `needs:` extended) |
| Findings appear under repository Security tab | Task 2 (SARIF upload step) |
| `.trivyignore` is the only dismissal path | Task 1 (file) + Task 2 (`trivyignores:` flag points at it) |
| Unchanged services skip the scan | Task 2 (composite action gating, mirrored paths) |

No gaps detected.

### Placeholder scan

No "TBD"/"TODO" content. Task 4's "if findings exist, update `.trivyignore`" is a documented escape hatch with explicit justification format, not a placeholder.

### Type / identifier consistency

- Service names match `build-and-push-images` matrix exactly (cross-checked entry by entry).
- `image-scan` referenced consistently in deploy `needs:` updates.
- `trivyignores:` (plural) is the trivy-action input name — verified against the action's documentation.
