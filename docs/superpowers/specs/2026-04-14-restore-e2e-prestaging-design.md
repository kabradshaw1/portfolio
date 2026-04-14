# Restore E2E Pre-Staging Checks

**Date:** 2026-04-14
**Status:** Draft

## Context

The CI/CD overhaul (commit `9cec1cc`, 2026-04-13) unified three workflow files into a single `ci.yml` and replaced the old `e2e-staging` Playwright job with deeper infrastructure checks (`compose-smoke`, `k8s-manifest-validation`, `go-migration-test`). However, the `e2e-staging` job — which ran the mocked Playwright E2E suite as a pre-merge gate — was removed entirely rather than being migrated to the new branch model.

**Problem:** With multiple agents creating PRs to `qa`, a frontend regression merged by one agent can block testing for all others. The mocked E2E tests catch these regressions cheaply (no backend needed) and should gate PR merges.

## Design

### 1. New CI job: `e2e-mocked`

Add a single job to `.github/workflows/ci.yml` that restores the mocked Playwright E2E tests as a pre-merge staging check.

**Triggers:**
- `pull_request` to `qa` (pre-merge gate)
- `push` to `qa` (post-merge small tweaks)
- Does NOT run on `push` to `main` (compose-smoke and smoke-prod cover that)

**Dependencies:** `needs: [frontend-checks]` — ensures the build passes before running E2E.

**Condition:** `if: github.event_name == 'pull_request' || github.ref == 'refs/heads/qa'` — skips on push to main.

**Steps:**
1. Checkout code
2. Set up Node 20 with npm cache
3. `npm ci` in `frontend/`
4. `npx playwright install --with-deps chromium`
5. `npx playwright test` (runs default config: `e2e/mocked/` suite)
6. Upload `frontend/playwright-report/` artifact on failure

**Estimated CI time:** ~2-3 minutes (Node install + Playwright install + mocked tests).

### 2. Update CI/CD frontend page (`frontend/src/app/cicd/page.tsx`)

**"Why a Unified Workflow" section:** Rewrite to reflect Kyle's actual experience — started with separate workflows which were helpful for refining individual checks, but as a solo developer rarely stopped between stages, so unified made more practical sense. Also easier for Claude agents to follow and debug a single pipeline. No need to rigorously defend staging since no one else is using it — Kyle pushes minor tweaks directly to qa without feature branches, which is fine because there's no one else to disrupt.

**Add staging checks context:** Update the page to explain that mocked E2E tests now run as a pre-merge staging check on PRs to `qa`, catching frontend regressions before actually deploying to QA.

**Rewrite "Agent Automation" section:**
- Add justification for why agent-driven workflow makes sense: solo developer, no risk of disrupting others' deployments.
- Highlight that Kyle rigorously reviews specs before anything gets implemented — this is a major time investment and a critical human checkpoint.
- Split the current "Spec → Plan" step into two distinct steps:
  1. **Spec:** Kyle and Claude brainstorm the design. Kyle reviews the spec thoroughly before approving.
  2. **Plan:** Once the spec is approved, Claude writes an implementation plan to keep track of what it needs to do during execution.
- Highlight the quality steps that the superpowers plugin adds throughout the workflow:
  - **Spec self-review:** After writing a spec, Claude automatically checks for placeholders, internal contradictions, ambiguity, and scope issues before presenting it to Kyle.
  - **Code review agent:** After completing a major implementation step, a dedicated code-review agent examines the work against the plan and coding standards.
  - **Verification before completion:** Before claiming work is done, Claude runs verification commands and confirms output — evidence before assertions.
- The rest of the workflow (implement, CI watch, QA deploy, ship) stays similar.

**Update trigger matrix:** Add a row for "E2E staging checks" showing it runs on PR to qa and push to qa but not push to main.

**Update pipeline flow diagram:** Add E2E staging checks to the PR subgraph.

### What this does NOT change

- `compose-smoke` continues to run on all pushes (deeper Python stack E2E)
- `smoke-prod` continues to run after production deploys (real Playwright against prod)
- All other quality checks remain unchanged
- No new branches or branch protection rules needed

## Files to Modify

- `.github/workflows/ci.yml` — add `e2e-mocked` job after the `frontend-checks` job
- `frontend/src/app/cicd/page.tsx` — rewrite "Why a Unified Workflow" section, add staging checks info, update trigger matrix and pipeline diagram

## Verification

1. Create a test PR to `qa` and confirm the `e2e-mocked` job appears and runs
2. Verify it depends on `frontend-checks` completing first
3. Verify the Playwright report artifact is uploaded
4. Push to `qa` and confirm the job runs there too
5. Push to `main` and confirm the job is skipped
6. Run `npm run dev` in `frontend/` and verify the CI/CD page renders correctly with updated content
