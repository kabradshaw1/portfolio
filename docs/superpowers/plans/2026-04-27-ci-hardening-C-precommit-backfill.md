# CI Hardening — Initiative C: Pre-commit Hook Backfill (Plan)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend `.pre-commit-config.yaml` so local hooks cover the same surface CI catches today: add bandit (Python SAST), hadolint (Dockerfile lint), and full Go service coverage. Add a `make install-pre-commit` target. Update `CLAUDE.md` to point new contributors at it.

**Architecture:** Three additions to `.pre-commit-config.yaml`. New target in `Makefile`. Documentation update in `CLAUDE.md`. No CI workflow changes.

**Tech Stack:** `pre-commit` framework, `bandit`, `hadolint`, `golangci-lint`.

**Spec:** `docs/superpowers/specs/2026-04-27-ci-pipeline-hardening-design.md` — Initiative C.

**File structure:**

| File | Status | Responsibility |
| --- | --- | --- |
| `.pre-commit-config.yaml` | Modify | Add bandit hook, hadolint hook, full Go service loop |
| `Makefile` | Modify | Add `install-pre-commit` target |
| `CLAUDE.md` | Modify | Document `make install-pre-commit` in Pre-commit Requirements section |

---

### Task 1: Add bandit hook for Python SAST

**Files:**
- Modify: `.pre-commit-config.yaml`

- [ ] **Step 1: Add the bandit hook**

Read `.pre-commit-config.yaml` to see its current shape. Add this block at the top of the `repos:` list, immediately after the `gitleaks` repo entry (so secret detection runs first, then Python SAST, then formatting):

```yaml
  - repo: https://github.com/PyCQA/bandit
    rev: 1.7.9
    hooks:
      - id: bandit
        args: ["-c", "pyproject.toml"]
        additional_dependencies: ["bandit[toml]"]
        files: ^services/
```

- [ ] **Step 2: Verify pyproject.toml exists at the repo root and configures bandit**

```bash
grep -A 5 '\[tool.bandit\]' pyproject.toml 2>/dev/null
```

If the output is empty (no `[tool.bandit]` section), the bandit hook will use defaults — that's fine for now. The hook still works; it just won't honor any per-project skip rules. If the existing CI bandit job uses a config file, mirror its config in `pyproject.toml` so local and CI behave identically. If there's no `pyproject.toml` at the repo root, create one with:

```toml
[tool.bandit]
exclude_dirs = ["tests", "venv", ".venv"]
```

Skip this sub-step if `pyproject.toml` already exists with appropriate config.

- [ ] **Step 3: Test the hook**

```bash
pre-commit run bandit --all-files
```

Expected: passes. If it fails on existing code, the failures are real findings that should be fixed inline (or excluded with `# nosec` comments on specific lines, with justifications). Do not bypass with `--no-verify`.

- [ ] **Step 4: Commit**

```bash
git add .pre-commit-config.yaml pyproject.toml 2>/dev/null
git -c commit.gpgsign=false commit -m "chore(pre-commit): add bandit Python SAST hook"
```

(`pyproject.toml` is included only if it was created/modified — `git add` will silently no-op if it doesn't exist.)

---

### Task 2: Add hadolint hook for Dockerfile lint

**Files:**
- Modify: `.pre-commit-config.yaml`

- [ ] **Step 1: Add the hadolint hook**

Add this block to `.pre-commit-config.yaml` after the bandit entry from Task 1:

```yaml
  - repo: https://github.com/hadolint/hadolint
    rev: v2.12.0
    hooks:
      - id: hadolint
        files: Dockerfile(\..+)?$
```

The regex matches `Dockerfile`, `Dockerfile.dev`, `Dockerfile.prod`, etc.

- [ ] **Step 2: Test the hook**

```bash
pre-commit run hadolint --all-files
```

Expected: passes. If it fails on existing Dockerfiles, the failures are real findings — fix them inline. Common issues:
- `DL3008`: pin apt package versions
- `DL3009`: clean apt cache after install
- `DL3018`: pin alpine package versions
- `DL3025`: use JSON-array form for CMD/ENTRYPOINT

If a finding is genuinely a false positive or unfixable in this context, add an inline ignore comment in the Dockerfile:

```dockerfile
# hadolint ignore=DL3008
RUN apt-get install -y curl
```

Do not bypass globally.

- [ ] **Step 3: Commit**

```bash
git add .pre-commit-config.yaml
git -c commit.gpgsign=false commit -m "chore(pre-commit): add hadolint Dockerfile lint hook"
```

If any Dockerfiles were edited to fix findings, also stage them:

```bash
git add <Dockerfile path>
git -c commit.gpgsign=false commit --amend --no-edit
```

(Amend allowed here only because the previous commit is unpushed and entirely about the same change. If pushed already, instead create a new commit.)

---

### Task 3: Replace the partial go-lint hook with full service coverage

**Files:**
- Modify: `.pre-commit-config.yaml`

- [ ] **Step 1: Read the current go-lint entry**

The existing entry hardcodes `auth-service` and `order-service` only. The full service list is `auth-service order-service ai-service analytics-service product-service cart-service payment-service order-projector`.

- [ ] **Step 2: Replace the existing go-lint local hook**

Find the existing `id: go-lint` entry under the `- repo: local` block. Replace its `entry:` field with a loop over all 8 services:

```yaml
      - id: go-lint
        name: Go Lint (all services)
        entry: bash -c 'set -e; for svc in auth-service order-service ai-service analytics-service product-service cart-service payment-service order-projector; do echo "--- $svc ---"; (cd "go/$svc" && ~/go/bin/golangci-lint run ./...); done'
        language: system
        files: ^go/.*\.go$
        pass_filenames: false
```

Keep the rest of the entry (`name`, `language`, `files`, `pass_filenames`) unchanged.

- [ ] **Step 3: Test the hook**

```bash
pre-commit run go-lint --all-files
```

Expected: passes. If it fails on previously-uncovered services (`ai-service`, `analytics-service`, `product-service`, `cart-service`, `payment-service`, `order-projector`), the failures are real lint findings that should be fixed before this PR merges. The CI Go lint matrix already passes (per PR #164 work), so any failures here would be lint rules that were enforced in CI but not locally — fix them.

- [ ] **Step 4: Commit**

```bash
git add .pre-commit-config.yaml
git -c commit.gpgsign=false commit -m "chore(pre-commit): expand go-lint to all 8 services"
```

If any Go source files were edited to fix lint findings, stage and amend (same caveat as Task 2 — only if previous commit is unpushed):

```bash
git add go/
git -c commit.gpgsign=false commit --amend --no-edit
```

---

### Task 4: Add `make install-pre-commit` target

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Locate the `.PHONY` declaration**

Read `Makefile` and find the existing `.PHONY:` declaration line. Verify the existing target list (the Bash tool's `head` output already showed `preflight preflight-python preflight-frontend preflight-e2e preflight-java preflight-java-integration preflight-go preflight-go-integration preflight-go-migrations preflight-security preflight-compose-config preflight-ai-service preflight-ai-service-evals grafana-sync grafana-sync-check worktree-cleanup`).

- [ ] **Step 2: Add `install-pre-commit` to `.PHONY`**

Append `install-pre-commit` to the `.PHONY:` list (one space-separated word).

- [ ] **Step 3: Add the target body**

Add this target near related developer tooling targets (the file likely has a section for setup/install — if not, append at the end):

```makefile
# --- Developer setup ---
.PHONY: install-pre-commit
install-pre-commit:
	@command -v pre-commit >/dev/null 2>&1 || { echo "Install pre-commit first: pip install pre-commit"; exit 1; }
	pre-commit install --install-hooks
	pre-commit install --hook-type pre-push --install-hooks
	@echo "✅ pre-commit hooks installed (commit + pre-push stages)"
```

(Note: if `.PHONY: install-pre-commit` already appears in the consolidated `.PHONY:` line at the top, you can drop the duplicate inline `.PHONY:` declaration above the target body. Keep one or the other, not both.)

- [ ] **Step 4: Verify the target runs**

```bash
make install-pre-commit
```

Expected: succeeds. If `pre-commit` isn't on `PATH`, the target's first line tells you to install it. The hooks should now be active in `.git/hooks/pre-commit` and `.git/hooks/pre-push`.

- [ ] **Step 5: Commit**

```bash
git add Makefile
git -c commit.gpgsign=false commit -m "chore(make): add install-pre-commit target"
```

---

### Task 5: Update `CLAUDE.md` with the install instruction

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Find the "Pre-commit Requirements" section**

Open `CLAUDE.md`. Locate the heading `## Pre-commit Requirements`. The section currently lists `make preflight-*` commands.

- [ ] **Step 2: Add a new top sub-section about local hook installation**

Insert this block immediately after the `## Pre-commit Requirements` heading (before the existing list):

```markdown
**First-time setup:** New clones must install the pre-commit hook framework once:

```bash
make install-pre-commit
```

This installs both commit-stage hooks (gitleaks, ruff, bandit, hadolint, java-checkstyle, frontend tsc/lint, go-lint) and pre-push-stage hooks (frontend `next build`). After this, every commit triggers the relevant subset based on what files changed.

```

(The trailing blank line + existing content stays.)

- [ ] **Step 3: Sanity-check the markdown renders**

```bash
grep -A 10 "## Pre-commit Requirements" CLAUDE.md | head -15
```

Expected: shows the new "First-time setup" block followed by the original content.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md
git -c commit.gpgsign=false commit -m "docs: document make install-pre-commit in CLAUDE.md"
```

---

### Task 6: Final preflight + push + PR

**Files:** No file changes.

- [ ] **Step 1: Run the full pre-commit suite to confirm everything passes**

```bash
pre-commit run --all-files
```

Expected: every hook either passes or reports "(no files to check) Skipped". No FAILED entries. If anything fails, fix the underlying issue before pushing.

- [ ] **Step 2: Push the branch**

```bash
git push -u origin agent/feat-ci-precommit-backfill
```

(Or whichever branch name the worktree uses.)

- [ ] **Step 3: Open the PR against qa**

```bash
gh pr create --base qa --title "ci: backfill pre-commit hooks to match CI coverage" --body "$(cat <<'EOF'
## Summary
Extends `.pre-commit-config.yaml` so local hooks cover the same surface CI catches today:
- Adds bandit (Python SAST) — mirrors the CI `security-bandit` job
- Adds hadolint (Dockerfile lint) — mirrors the CI `security-hadolint` matrix
- Replaces the partial 2-service go-lint with full 8-service coverage

Adds `make install-pre-commit` for first-time setup. Documents in CLAUDE.md.

Spec: docs/superpowers/specs/2026-04-27-ci-pipeline-hardening-design.md (Initiative C)
Plan: docs/superpowers/plans/2026-04-27-ci-hardening-C-precommit-backfill.md

## Risk
Pre-commit hooks run locally on developer machines — no CI workflow changes. CI continues to act as the safety net. The only failure mode is a developer needing to fix a finding that previously was caught only in CI; that's the desired behavior.

## Test plan
- [ ] On a fresh clone: `make install-pre-commit` succeeds and installs both stages
- [ ] `pre-commit run --all-files` passes on a clean main
- [ ] Modifying a Python file with a hardcoded password fails the bandit hook locally
- [ ] Modifying a Dockerfile with a `latest` tag fails the hadolint hook locally
- [ ] Modifying a Go file in `payment-service` fails locally if it has lint issues (was previously CI-only)
EOF
)"
```

- [ ] **Step 4: Notify Kyle**

Tell Kyle the PR is open. Do not watch CI.

## Self-Review

### Spec coverage check (Initiative C acceptance criteria)

| Spec criterion | Plan task |
| --- | --- |
| `pre-commit run --all-files` passes on a clean main | Task 6 Step 1 (verification) |
| Hardcoded password caught locally | Task 1 (bandit) |
| Dockerfile with `latest` tag fails locally | Task 2 (hadolint) |
| Lint error in `go/payment-service` caught locally | Task 3 (full service loop) |
| `make install-pre-commit` is one-liner for new clone | Task 4 |

No gaps detected.

### Placeholder scan

No "TBD"/"TODO" content. Task 1 Step 2's "skip if `pyproject.toml` exists" is conditional logic, not a placeholder. Task 2's "fix findings inline" lists the actual common rule codes the engineer will see.

### Type / identifier consistency

- Hook IDs (`bandit`, `hadolint`, `go-lint`) match the upstream tool's `id:` exactly.
- Service list in Task 3 matches the spec's stated 8-service list.
- `pre-commit run <hook-id>` calls match registered IDs.
