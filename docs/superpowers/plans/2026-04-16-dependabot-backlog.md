# Dependabot Backlog — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Merge the 11 open Dependabot PRs (#48–#58) currently targeting `qa` through safely-ordered batches, and delete one `pip-audit` ignore from CI as part of the pytest bump.

**Architecture:** Work sequentially in batches per the spec. Each batch merges one or more Dependabot PRs to `qa` (autonomous per CLAUDE.md), waits for QA CI to go green, then stops and awaits Kyle's explicit "ship it" before merging `qa` → `main`. Do not push `main` autonomously.

**Tech Stack:** `gh` CLI (PR ops + check monitoring), direct commits to `qa` (for the one follow-up Python change in Batch A), local preflight targets (`make preflight-python`, `make preflight-security`, `make preflight-frontend`, `make preflight-e2e`).

## Context Loaded from Spec

- **Spec:** `docs/superpowers/specs/2026-04-16-dependabot-backlog-design.md`
- **Batches:** A (pytest + CVE cleanup) → B (React pair) → C (individual review: uvicorn, python-multipart, dompurify) → D (low-risk pairs).
- **Out of scope:** LangChain 0.2→0.3 migration (tracked separately via issue #62).

## Critical Findings from Pre-Plan Investigation

These are things the spec flagged as preconditions — I checked and they have concrete answers now.

### 1. `pytest==8.2.0` and `pytest-asyncio==0.25.3` are pinned in all three Python services

```
services/chat/requirements.txt:8:pytest==8.2.0
services/chat/requirements.txt:9:pytest-asyncio==0.25.3
services/debug/requirements.txt:9:pytest==8.2.0
services/debug/requirements.txt:10:pytest-asyncio==0.25.3
services/ingestion/requirements.txt:10:pytest==8.2.0
services/ingestion/requirements.txt:11:pytest-asyncio==0.25.3
```

Dependabot only opened PRs for `services/chat` (#48, #49). The spec called this out in Batch A preconditions: "either (a) wait for Dependabot to propose it separately, or (b) bump manually in the same PR."

**Decision:** option (b) for ingestion + debug, via a direct commit to `qa` that also deletes the CVE ignore. Reasons:
- `services/debug` is **not listed** in `.github/dependabot.yml` (only `ingestion` and `chat` are) — Dependabot will **never** propose its pytest bump. Waiting is not an option for debug.
- `services/ingestion` is watched by Dependabot but has 2 open PRs (under the limit of 5); the pytest PR may just not have been produced yet. Waiting would stall Batch A behind Dependabot's weekly schedule with no guarantee.
- The spec's verification step (`pip-audit` green on chat + ingestion + debug **without** `CVE-2025-71176` in the ignore list) **cannot pass** unless all three services bump pytest. So doing it all in one coherent Batch A change is the only way the verification signal works.

### 2. PR branches are stale on an old ci.yml that's missing 3 pip-audit ignores

PR #48's `pip-audit (debug)` check was `FAILURE` (not canceled). From the run log, the actual command that ran was:

```
pip-audit --ignore-vuln CVE-2025-6984 --ignore-vuln CVE-2025-65106 --ignore-vuln CVE-2025-68664 --ignore-vuln CVE-2026-26013 --ignore-vuln CVE-2025-6985 --ignore-vuln GHSA-926x-3r5x-gfhw
```

That's **only 6** `--ignore-vuln` args — missing `CVE-2025-71176`, `GHSA-rr7j-v2q5-chgv`, and `CVE-2026-34070`. The current `ci.yml` on qa (verified at lines 488–498) has all 9. Root cause: these PRs were opened against `staging` before 3 ignores were added to qa. The retarget to qa didn't rebase the branches, so each PR's CI runs against its own stale `ci.yml`.

The chat/ingestion pip-audit runs on PR #48 show `CANCELLED` — likely concurrency-group cancellation during a slower run. Debug finished first and hit the real "CVE not in ignore list" failure.

**Fix:** rebase each PR onto qa before merging via `@dependabot rebase` (lighter than `recreate`). This pushes a new commit onto the PR branch that picks up qa's current `ci.yml` and re-triggers CI with the full 9-ignore list. All pip-audit checks should then pass cleanly.

Fallback if rebase fails: merge despite `UNSTABLE` status (the underlying PR payload — a pytest bump — is fine; only the PR's own CI signal is wrong, and the post-merge qa CI will use the correct ci.yml). But try rebase first for clean history.

### 3. Repo merge strategy

`gh repo view` shows: `mergeCommitAllowed: true`, `squashMergeAllowed: true`, `rebaseMergeAllowed: true`, `viewerDefaultMergeMethod: MERGE`, `deleteBranchOnMerge: false`.

**Decision:** use `gh pr merge --merge` (explicit merge commit) for Dependabot PRs. Preserves the Dependabot bump as an identifiable commit in history, making `git revert <merge-commit>` the clean rollback path the spec calls for. Pass `--delete-branch` to clean up the Dependabot branch (they stack up otherwise since `deleteBranchOnMerge` is false).

### 4. Follow-up (out of scope for this plan, but worth flagging at the end)

- `services/debug` is audited by CI (`Security - pip-audit (debug)`) but not watched by Dependabot. That's a gap — add it to `.github/dependabot.yml` in a future spec.

---

## Task 1: Prep — Set Spec Marker, Sync qa

**Files:** None modified.

- [ ] **Step 1.1: Update current-spec marker so Kyle can see which spec is active**

Run:
```bash
echo "dependabot-backlog-design" > ~/.claude/current-spec.txt
```

- [ ] **Step 1.2: Confirm current branch is `main`, then fetch + switch to `qa`**

Run:
```bash
git branch --show-current
git fetch origin
git checkout qa
git pull --ff-only origin qa
```

Expected: `qa` branch checked out, clean working tree.

- [ ] **Step 1.3: Confirm qa CI baseline is green (no pre-existing failures to disentangle from our work)**

Run:
```bash
gh run list --branch qa --limit 3 --workflow ci.yml
```

Expected: the most recent completed run on `qa` shows `completed success`. If it doesn't, **stop** and ask Kyle before proceeding — we don't want to merge into a broken baseline.

---

## Task 2: Batch A — Python dev-deps + CVE ignore cleanup

**Files:**
- Merge (via `gh`): PR #48, PR #49
- Modify: `services/ingestion/requirements.txt`, `services/debug/requirements.txt`, `.github/workflows/ci.yml`

**Batch summary:** Merge #48 (chat pytest) and #49 (chat pytest-asyncio). Then one direct commit to `qa` bumps the same two deps in `services/ingestion` and `services/debug` and deletes the `CVE-2025-71176` ignore from `ci.yml`. End result: all three Python services synced to `pytest==8.4.2` + `pytest-asyncio==0.26.0`, pip-audit ignore list shorter by one line, one line of issue #62 closed.

- [ ] **Step 2.1: Request fresh CI on PR #48 (the canceled run isn't trustworthy)**

Run:
```bash
gh pr comment 48 --body "@dependabot rebase"
```

Expected: Dependabot posts a confirmation comment and re-creates the branch in 1–3 min, which triggers a new CI run.

- [ ] **Step 2.2: Wait for PR #48 fresh CI to complete**

Run (poll until green):
```bash
gh pr checks 48 --watch
```

Expected: all required checks pass. If `pip-audit (chat)` fails on this fresh run, **stop** — that would indicate a new CVE has surfaced in the chat dep graph that isn't the one we're about to close. Investigate before merging.

- [ ] **Step 2.3: Merge PR #48 to qa**

Run:
```bash
gh pr merge 48 --merge --delete-branch
```

Expected: PR marked merged, branch deleted, qa CI kicks off.

- [ ] **Step 2.4: Wait for qa CI to go green after #48 merge**

Run:
```bash
gh run list --branch qa --limit 1 --workflow ci.yml
# grab the run id from the output, then:
gh run watch <run-id>
```

Expected: the post-merge qa run completes `success` — including QA Smoke Tests.

- [ ] **Step 2.5: Request fresh CI on PR #49 and merge it**

Run:
```bash
gh pr comment 49 --body "@dependabot rebase"
gh pr checks 49 --watch
gh pr merge 49 --merge --delete-branch
```

Expected: fresh run green → merge succeeds. Then:

```bash
git pull --ff-only origin qa
gh run list --branch qa --limit 1 --workflow ci.yml
gh run watch <run-id>
```

Expected: post-merge qa run completes `success`.

- [ ] **Step 2.6: Create the follow-up commit — bump ingestion + debug pytest + pytest-asyncio, delete CVE ignore**

Ensure qa is up to date locally:
```bash
git checkout qa
git pull --ff-only origin qa
```

Edit `services/ingestion/requirements.txt` — change:
```
pytest==8.2.0
pytest-asyncio==0.25.3
```
to:
```
pytest==8.4.2
pytest-asyncio==0.26.0
```

Edit `services/debug/requirements.txt` — change the same two pins (lines 9 and 10) to `8.4.2` and `0.26.0`.

Edit `.github/workflows/ci.yml` — delete the single line:
```
          --ignore-vuln CVE-2025-71176
```
(around line 496; leave the other 8 `--ignore-vuln` lines in place — those are langchain-family and out of scope).

- [ ] **Step 2.7: Run Python preflight checks locally**

Run:
```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer
make preflight-python
make preflight-security
```

Expected: both green. If `preflight-security` (pip-audit) fails because a NEW pytest CVE has surfaced at 8.4.2, **stop** and tell Kyle — we'd need to either re-add the ignore or bump further. If `preflight-python` fails with a pytest compatibility issue (a test breaks under 8.4.2), fix the test in the same commit.

- [ ] **Step 2.8: Commit and push to qa**

Run:
```bash
git add services/ingestion/requirements.txt services/debug/requirements.txt .github/workflows/ci.yml
git commit -m "$(cat <<'EOF'
chore(deps): sync ingestion/debug to pytest 8.4.2 + drop CVE-2025-71176 ignore

- Bump services/ingestion pytest 8.2.0→8.4.2 and pytest-asyncio 0.25.3→0.26.0
  to match PR #48/#49 for services/chat. Dependabot didn't propose these —
  ingestion's PR hadn't surfaced yet and services/debug isn't watched at all.
- Drop --ignore-vuln CVE-2025-71176 from pip-audit args now that all three
  Python services ship pytest 8.4.2 which resolves the advisory. Closes one
  line of issue #62.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>
EOF
)"
git push origin qa
```

Expected: push succeeds (qa allows autonomous commits per CLAUDE.md).

- [ ] **Step 2.9: Watch qa CI; verify all three pip-audit jobs pass without the CVE ignore**

Run:
```bash
gh run list --branch qa --limit 1 --workflow ci.yml
gh run watch <run-id>
```

Then verify specifically:
```bash
gh run view <run-id> --json jobs -q '.jobs[] | select(.name | startswith("Security - pip-audit")) | {name, conclusion}'
```

Expected: all three entries (chat, ingestion, debug) report `"conclusion": "success"`. This is the spec's required verification for Batch A.

If any pip-audit fails: revert just the `ci.yml` change (`git revert` the last commit, then cherry-pick back the requirements.txt edits from that same commit, then push) — the pytest bumps stay, the ignore comes back, and we escalate to Kyle with whatever new CVE surfaced.

- [ ] **Step 2.10: Mark Batch A as ready to ship to main — STOP and wait for Kyle**

Post a status to Kyle summarizing:
- Which PRs merged (#48, #49)
- The follow-up commit SHA and what it did
- qa CI run URL showing green with all three pip-audit jobs passing without the ignore

Do **not** merge to `main`. Per CLAUDE.md: "On `main`: never push autonomously. When Kyle explicitly says to merge/ship to main, handle the full flow."

When Kyle says "ship it" for Batch A, execute:
```bash
git checkout main
git pull --ff-only origin main
git merge --no-ff qa -m "Merge qa: Batch A — pytest sync + CVE-2025-71176 ignore removed"
git push origin main
gh run list --branch main --limit 1 --workflow ci.yml
gh run watch <run-id>
```

Expected: main CI + Production Smoke Tests pass.

---

## Task 3: Batch B — React patch pair (PRs #56 + #57)

**Files:** merges only — `frontend/package.json`, `frontend/package-lock.json` via Dependabot.

**Batch summary:** react and react-dom must move together. Merge #56 (react), then immediately #57 (react-dom). Frontend-only. Verify via frontend checks + mocked E2E.

- [ ] **Step 3.1: Sync qa locally**

```bash
git checkout qa
git pull --ff-only origin qa
```

- [ ] **Step 3.2: Request fresh CI on PR #56 (react) and wait**

```bash
gh pr comment 56 --body "@dependabot rebase"
gh pr checks 56 --watch
```

Expected: Frontend Checks (tsc, eslint, next build) pass; E2E Tests (Staging) pass.

- [ ] **Step 3.3: Merge PR #56**

```bash
gh pr merge 56 --merge --delete-branch
```

- [ ] **Step 3.4: Immediately refresh and merge PR #57 (react-dom)**

Because react and react-dom must be in lockstep, do NOT wait for the qa post-merge CI between #56 and #57 — merging them in one sitting keeps them from ever being skewed on `qa`.

```bash
gh pr comment 57 --body "@dependabot rebase"
gh pr checks 57 --watch
gh pr merge 57 --merge --delete-branch
```

Expected: both merged within a few minutes of each other.

- [ ] **Step 3.5: Watch the post-merge qa CI run that includes both bumps**

```bash
git pull --ff-only origin qa
gh run list --branch qa --limit 1 --workflow ci.yml
gh run watch <run-id>
```

Expected: Frontend Checks + QA Smoke Tests green.

- [ ] **Step 3.6: STOP — await Kyle's ship-it for Batch B**

Same pattern as Task 2.10. Summarize to Kyle and wait.

When Kyle says ship: merge qa → main as in Task 2.10.

---

## Task 4: Batch C.1 — uvicorn (PR #51) individually

**Files:** merge only — `services/chat/requirements.txt` via Dependabot.

**Batch summary:** 14 minor versions skipped (0.30 → 0.44). Highest-blast-radius single PR in this plan because uvicorn is the ASGI runtime for the chat service. Review CHANGELOG before merging.

- [ ] **Step 4.1: Read uvicorn CHANGELOG for behavior changes in 0.31–0.44**

Open https://github.com/encode/uvicorn/blob/master/CHANGELOG.md in a browser and scan entries between 0.31 and 0.44 for:
- Lifespan protocol changes
- HTTP/2 defaults
- Signal handling changes
- WebSocket behavior changes
- Any `BREAKING CHANGE` markers or `Removed`/`Deprecated` sections

Record any concerns in the PR as a comment. If anything looks like it would break the chat service's specific usage pattern (streaming SSE for chat responses), **stop and ask Kyle**.

- [ ] **Step 4.2: Sync qa and request fresh CI on PR #51**

```bash
git checkout qa
git pull --ff-only origin qa
gh pr comment 51 --body "@dependabot rebase"
gh pr checks 51 --watch
```

Expected: Backend Tests (chat), Docker Build (chat), Compose Smoke (Python stack), and pip-audit (chat) all pass.

- [ ] **Step 4.3: Merge PR #51**

```bash
gh pr merge 51 --merge --delete-branch
```

- [ ] **Step 4.4: Watch qa CI, with extra attention to Compose Smoke + QA Smoke Tests**

```bash
git pull --ff-only origin qa
gh run list --branch qa --limit 1 --workflow ci.yml
gh run watch <run-id>
```

Expected: both `Compose Smoke (Python stack)` and `QA Smoke Tests` are green. These are the best integration signals we have for "uvicorn still serves the chat service correctly."

- [ ] **Step 4.5: STOP — await Kyle's ship-it for Batch C.1**

Summarize. Ship when Kyle says. After prod merge, monitor `Production Smoke Tests` with extra care — this is the PR most likely to surface a subtle runtime regression.

---

## Task 5: Batch C.2 — python-multipart (PR #50) individually

**Files:** merge only — `services/ingestion/requirements.txt` via Dependabot.

**Batch summary:** Small jump (0.0.22 → 0.0.26) but python-multipart has had recent CVE activity. Low blast radius (only ingestion's PDF upload path).

- [ ] **Step 5.1: Quick check of python-multipart 0.0.26 release notes**

Visit https://github.com/Kludex/python-multipart/releases and scan 0.0.23 through 0.0.26 for any CVE references or parser-behavior changes.

- [ ] **Step 5.2: Sync qa, fresh CI, merge**

```bash
git checkout qa
git pull --ff-only origin qa
gh pr comment 50 --body "@dependabot rebase"
gh pr checks 50 --watch
gh pr merge 50 --merge --delete-branch
```

- [ ] **Step 5.3: Watch qa CI**

```bash
git pull --ff-only origin qa
gh run list --branch qa --limit 1 --workflow ci.yml
gh run watch <run-id>
```

Expected: Backend Tests (ingestion), Docker Build (ingestion), and QA Smoke Tests green.

- [ ] **Step 5.4: STOP — await Kyle's ship-it for Batch C.2**

Summarize and wait.

---

## Task 6: Batch C.3 — dompurify (PR #55) individually

**Files:** merge only — `frontend/package.json`, `frontend/package-lock.json` via Dependabot.

**Batch summary:** XSS sanitization library; always take security-relevant bumps. Verify that sanitization behavior on the chat message-rendering path is still correct.

- [ ] **Step 6.1: Read dompurify changelog/releases between the old and new pinned versions**

Visit https://github.com/cure53/DOMPurify/releases. Look for any notes about changed default config or removed allowances — these would be the ones that could break our existing calls.

Then find our frontend dompurify call sites with the Grep tool using the pattern `DOMPurify\.sanitize|from ['"]dompurify['"]` and glob `frontend/src/**/*.{ts,tsx}`. Note which files use it; those are the visual paths to eyeball on the Vercel preview.

- [ ] **Step 6.2: Sync qa, fresh CI, merge**

```bash
git checkout qa
git pull --ff-only origin qa
gh pr comment 55 --body "@dependabot rebase"
gh pr checks 55 --watch
gh pr merge 55 --merge --delete-branch
```

- [ ] **Step 6.3: Watch qa CI + eyeball Vercel preview of any dompurify call site**

```bash
git pull --ff-only origin qa
gh run list --branch qa --limit 1 --workflow ci.yml
gh run watch <run-id>
```

Open the Vercel preview URL (from the PR comments / Vercel dashboard) and visit at least one page that renders untrusted content through dompurify — confirm markdown / HTML still renders as expected.

- [ ] **Step 6.4: STOP — await Kyle's ship-it for Batch C.3**

Summarize, note any visual differences from the Vercel preview, and wait.

---

## Task 7: Batch D.1 — prometheus-fastapi-instrumentator pair (PRs #52 + #53)

**Files:** merges only — `services/ingestion/requirements.txt`, `services/chat/requirements.txt` via Dependabot.

**Batch summary:** Same package, same version jump, on two services. Batch them together for consistency.

- [ ] **Step 7.1: Sync qa, fresh CI on both, merge both**

```bash
git checkout qa
git pull --ff-only origin qa

gh pr comment 52 --body "@dependabot rebase"
gh pr comment 53 --body "@dependabot rebase"
gh pr checks 52 --watch
gh pr merge 52 --merge --delete-branch

gh pr checks 53 --watch
gh pr merge 53 --merge --delete-branch
```

- [ ] **Step 7.2: Watch qa CI**

```bash
git pull --ff-only origin qa
gh run list --branch qa --limit 1 --workflow ci.yml
gh run watch <run-id>
```

Expected: Backend Tests (chat + ingestion), Docker Builds, and QA Smoke Tests green. Also spot-check that `/metrics` endpoints still return Prometheus-format output in the QA smoke run logs if possible.

- [ ] **Step 7.3: STOP — await Kyle's ship-it for Batch D.1**

---

## Task 8: Batch D.2 — eslint-config-next + @base-ui/react (PRs #54 + #58)

**Files:** merges only — `frontend/package.json`, `frontend/package-lock.json` via Dependabot.

**Batch summary:** Both frontend. Dev-tool (#54) and runtime-component-lib (#58). Safe patch-level risk either way.

- [ ] **Step 8.1: Sync qa, fresh CI on both, merge both**

```bash
git checkout qa
git pull --ff-only origin qa

gh pr comment 54 --body "@dependabot rebase"
gh pr comment 58 --body "@dependabot rebase"

gh pr checks 54 --watch
gh pr merge 54 --merge --delete-branch

gh pr checks 58 --watch
gh pr merge 58 --merge --delete-branch
```

- [ ] **Step 8.2: Watch qa CI**

```bash
git pull --ff-only origin qa
gh run list --branch qa --limit 1 --workflow ci.yml
gh run watch <run-id>
```

Expected: Frontend Checks + QA Smoke Tests green.

- [ ] **Step 8.3: STOP — await Kyle's ship-it for Batch D.2**

---

## Verification checklist (run at the end of each batch)

- [ ] `gh pr checks <N>` — all checks green on the PR before merging (after `@dependabot rebase`)
- [ ] Post-merge qa CI run: completes `success` including `QA Smoke Tests`
- [ ] Post-main CI run (after Kyle ships): `Production Smoke Tests` green
- [ ] `gh run view <run-id>` on prod: no new warnings/flakes
- [ ] **Batch A only:** `Security - pip-audit (chat|ingestion|debug)` all report `conclusion: success` without `CVE-2025-71176` in the ignore list

## Rollback strategy

All changes are isolated to `requirements.txt` / `package.json` / `package-lock.json`, plus one `ci.yml` line in Batch A. For any batch:

```bash
git checkout main
git revert <merge-commit>         # a standard merge commit revert
git push origin main
```

Dependabot re-proposes the PR in the next cycle (for the merged-PR bumps). For the Batch A follow-up commit, revert that commit specifically — the PR #48 / #49 merges stay in place.

## Out of scope — follow-ups to flag

- **LangChain 0.2.x → 0.3.x migration.** Closes 8 of 9 lines in issue #62. Needs its own design/plan spec. Blocked on deciding 0.3.x vs 1.x.
- **`services/debug` isn't in `.github/dependabot.yml`.** It's pip-audited in CI but never gets dep-update PRs. Worth adding a 4th `pip` entry after this spec wraps.
- **Revisit the `langchain*` semver-minor ignore in `dependabot.yml`** once the langchain migration lands.
