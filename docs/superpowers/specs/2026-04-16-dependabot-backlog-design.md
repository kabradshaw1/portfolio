# Dependabot Backlog — Merge Strategy

## Summary

Eleven open Dependabot PRs (#48–#58) have been retargeted from the retired `staging` branch to `qa`. This spec defines the batching, order, and verification plan to merge them. One of the eleven directly resolves a CVE from issue #62 (the pip-audit ignore-list tracker); the remaining eight of #62's advisories are all in the langchain family and are **out of scope** here — they require a coordinated `0.2.x → 0.3.x` migration that's blocked by Dependabot's own `langchain*` minor-version ignore rule in `.github/dependabot.yml`.

## Context at time of writing (2026-04-16)

Open Dependabot PRs (all against `qa`):

| PR | Bump | Path |
|---|---|---|
| #48 | pytest 8.2.0 → 8.4.2 | services/chat |
| #49 | pytest-asyncio 0.25.3 → 0.26.0 | services/chat |
| #50 | python-multipart 0.0.22 → 0.0.26 | services/ingestion |
| #51 | uvicorn 0.30.0 → 0.44.0 | services/chat |
| #52 | prometheus-fastapi-instrumentator 7.0.2 → 7.1.0 | services/ingestion |
| #53 | prometheus-fastapi-instrumentator 7.0.2 → 7.1.0 | services/chat |
| #54 | eslint-config-next 16.2.1 → 16.2.3 | frontend |
| #55 | dompurify + @types/dompurify | frontend |
| #56 | react 19.2.4 → 19.2.5 | frontend |
| #57 | react-dom 19.2.4 → 19.2.5 | frontend |
| #58 | @base-ui/react 1.3.0 → 1.4.0 | frontend |

Current pip-audit ignore list in `.github/workflows/ci.yml` (around line 490):

```yaml
--ignore-vuln CVE-2025-6984       # langchain
--ignore-vuln CVE-2025-65106      # langchain
--ignore-vuln CVE-2025-68664      # langchain
--ignore-vuln CVE-2026-26013      # langchain
--ignore-vuln CVE-2025-6985       # langchain
--ignore-vuln GHSA-926x-3r5x-gfhw # langchain-core
--ignore-vuln CVE-2025-71176      # pytest (dev-only)  ← PR #48 fixes this
--ignore-vuln GHSA-rr7j-v2q5-chgv # langsmith
--ignore-vuln CVE-2026-34070      # langchain-core
```

## Goals

1. Merge the non-risky Dependabot PRs in small, well-reasoned batches through `qa` → `main`.
2. Delete the `CVE-2025-71176` ignore from CI config as part of the pytest bump (one less line to forget about later).
3. Leave production safe — no dependency surgery that the CI matrix can't validate end-to-end.

## Non-goals

- **The langchain 0.2.x → 0.3.x migration.** The six langchain-family ignores plus langchain-core and langsmith (8 of 9 ignores in issue #62) are blocked on that migration. It's a separate spec; Dependabot's `langchain*` semver-minor ignore rule deliberately prevents partial bumps.
- **The 11 PRs as a single merge train.** Some (#51 uvicorn skipping 14 minor versions) warrant individual scrutiny; batching them all would make rollback harder.

## Batches

### Batch A — Python dev-deps + CVE ignore cleanup

Scope:
- PR #48 pytest 8.2.0 → 8.4.2 (services/chat)
- PR #49 pytest-asyncio 0.25.3 → 0.26.0 (services/chat)
- Delete `--ignore-vuln CVE-2025-71176` from `.github/workflows/ci.yml`

Why batch these: both touch `services/chat/requirements.txt` only, both are dev-only deps (don't ship in the runtime image), and the CI ignore cleanup is directly enabled by #48. Landing them together tells one coherent story in the commit log.

Preconditions before merging:
- Verify `services/chat/requirements.txt` is the only Python requirements file that pins pytest / pytest-asyncio at the old version. If services/ingestion and services/debug also pin `pytest==8.2.0`, Dependabot should have opened separate PRs; if it hasn't, the other services may be silently skipping the bump. Check:
  ```bash
  grep -E "^pytest\b|^pytest-asyncio\b" services/*/requirements.txt
  ```
  If the old pin exists elsewhere, either (a) wait for Dependabot to propose it separately, or (b) bump manually in the same PR.

Execution:
1. Merge PR #48 first (pytest alone). Wait for qa CI to go green.
2. Merge PR #49 (pytest-asyncio). Wait for qa CI.
3. In a small follow-up commit to `qa`, delete the `CVE-2025-71176` line from `ci.yml`. Watch CI — the pip-audit jobs should still pass because the pytest bump closes that vuln. Commit message suggestion: `chore(ci): drop CVE-2025-71176 pip-audit ignore (pytest 8.4.2 resolves)`
4. Once qa is clean: merge qa → main (per CLAUDE.md's ship-to-main flow).

Verification: `pip-audit` jobs (chat, ingestion, debug) all pass **without** the `CVE-2025-71176` ignore line.

Rollback: if pip-audit re-surfaces a new pytest CVE at 8.4.2, revert the ignore-deletion commit. The pytest bump itself stays.

### Batch B — React patch pair

Scope:
- PR #56 react 19.2.4 → 19.2.5
- PR #57 react-dom 19.2.4 → 19.2.5

Why batch these: react and react-dom must move in lockstep — they reference each other's APIs at the same version. Merging one without the other is a footgun.

Execution: merge both in a single sitting. Either enable auto-merge on both so GitHub waits for each to reach a mergeable state, or merge #56 then immediately #57. Frontend-only; no backend impact.

Verification: Frontend Checks (tsc, eslint, next build) and mocked E2E pass.

### Batch C — Individual review PRs

These deserve their own look before merging. Don't batch.

| PR | Why individual review |
|---|---|
| **#51 uvicorn 0.30.0 → 0.44.0** | 14 minor versions skipped. Read the CHANGELOG between 0.30 and 0.44 for ASGI-behavior changes, lifespan protocol changes, HTTP/2 defaults, signal handling. Most likely safe but non-trivial blast radius if something subtle changes. |
| **#50 python-multipart 0.0.22 → 0.0.26** | Not in issue #62's list, but `python-multipart` has had CVE activity recently (see commit 774f3d8 message). Check what fixes 0.0.26 includes; likely worth taking. Low blast radius — only affects multipart upload parsing in ingestion. |
| **#55 dompurify (+ @types/dompurify)** | XSS-sanitization library. Always take security-relevant bumps. Verify via changelog that behavior on untrusted-HTML paths (chat message rendering in the RAG UI, if applicable) hasn't changed in a way that breaks existing sanitization calls. |

Merge each on its own to keep the commit → deploy cycle attributable.

### Batch D — Low-risk boring bumps (whenever)

- PR #52 prometheus-fastapi-instrumentator 7.0.2 → 7.1.0 (services/ingestion)
- PR #53 prometheus-fastapi-instrumentator 7.0.2 → 7.1.0 (services/chat)
- PR #54 eslint-config-next 16.2.1 → 16.2.3
- PR #58 @base-ui/react 1.3.0 → 1.4.0

Batch these two at a time (e.g., 52+53 together since they're the same package on different services; 54+58 together since they're both frontend dev/runtime deps with patch-level risk).

## Order of operations (suggested day-of plan)

1. **Morning (low-risk):** Batch A pytest + ignore cleanup → ship qa → main. Closes one line of issue #62.
2. **Same day:** Batch B react pair → qa → main. Frontend-only, quick to validate by eye on the Vercel preview.
3. **Later session:** Batch C individually — uvicorn first (most friction-prone), then python-multipart, then dompurify. Each gets its own qa cycle.
4. **Whenever:** Batch D in pairs, as filler work between bigger tasks.

Between every batch, wait for qa CI to go green (including QA Smoke Tests), then ship to main and wait for Production Smoke Tests green. Do **not** batch multiple independent-risk bumps into a single main push — if a regression appears, you want one suspect, not four.

## Verification checklist (apply per batch)

- [ ] `gh pr checks <N>` — all checks green on the PR before merging
- [ ] After merge to qa: QA CI passes including QA Smoke Tests
- [ ] After qa → main: Production Smoke Tests pass (full Playwright suite)
- [ ] `gh run view <run-id>` — confirm no new warnings or flakes in the prod smoke
- [ ] For Batch A only: pip-audit jobs (chat, ingestion, debug) are green without `CVE-2025-71176` in the ignore list

## Rollback strategy

All Dependabot bumps are isolated to `requirements.txt` or `package.json`/`package-lock.json`. Rollback is a `git revert <merge-commit>` on main, which Dependabot will then re-propose in a fresh PR when the base branch is back in a state it recognizes. No data or schema changes; no coordination required.

## Out of scope — pointers for follow-up

- **LangChain 0.2.x → 0.3.x (or 1.x) migration** — tracked in [issue #62](https://github.com/kabradshaw1/portfolio/issues/62). Eight of the nine ignores in that issue are langchain-family and can only be resolved by the migration. Needs its own design/plan spec. Blocked on deciding whether to go to 0.3.x (incremental) or 1.x (further jump, check the `+echo.1` suffix on CVE-2026-34070's fix version for provenance).
- **Dependabot's `langchain*` semver-minor ignore rule** in `.github/dependabot.yml` — intentionally protective. Revisit when the langchain migration is actually planned; likely keep it in place until then so stray minor bumps don't create surprise PRs that can't pass CI.
- **The `missing-dependabot-PR` hypothesis in Batch A preconditions.** If services/ingestion and services/debug also pin `pytest==8.2.0` and Dependabot didn't open PRs for them, either dependabot.yml is scoped narrowly or something's silently skipping updates. Worth a 5-minute diagnose once the pytest migration starts.
