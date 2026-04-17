# CI/CD Pipeline Optimization Story Page

## What

Add a "Pipeline Optimization" section to the existing `/cicd` page telling the story of how adding the eval service exposed CI/CD performance issues and how each was systematically diagnosed and fixed.

## Why

Makes the CI/CD optimization work visible in the portfolio. Shows interviewers that Kyle diagnoses and fixes infrastructure problems systematically, not just builds features.

## Source Material

All content comes from `docs/adr/cicd-performance-optimizations.md`.

## Deliverables

### 1. Narrative Section

Four subsections, one per optimization, each following the pattern:
- **Problem** — what was slow/broken and why
- **Investigation** — what was discovered
- **Fix** — what was changed (with a code snippet where relevant)
- **Result** — before/after timing

The four optimizations:
1. **Virtualenv Caching** — eval service pip install 20 min → 20 sec
2. **Conditional Image Builds** — skip unchanged services, rebuild only affected ones
3. **Compose-Smoke: Pull Instead of Build** — ~15 min → 95 sec
4. **QA Deploy: Job Immutability Fix** — failing → 85 sec

### 2. Before/After Timing Table

Reproduces the combined impact table from the ADR:

| Stage | Before | After |
|-------|--------|-------|
| Python Tests (eval) | 20 min | 20 sec |
| pip-audit (eval) | 20 min | 9 sec |
| Compose Smoke | ~15 min | 95 sec |
| Image Builds (no change) | ~3 min each | ~20 sec (skipped) |
| Deploy QA | failing | 85 sec |
| **Total pipeline** | **30+ min** | **~5 min** |

### 3. Mermaid Diagram

A pipeline-stage diagram annotating where each optimization applies. Shows the flow from PR → quality checks → build → deploy → smoke, with callouts at each optimization point.

## Implementation

- All changes in `frontend/src/app/cicd/page.tsx`
- New section inserted after "Agent Automation" and before "Smoke Tests" (the optimization story flows naturally before the smoke test description)
- Uses existing `MermaidDiagram` component
- No new components, no new dependencies, no backend changes
- Follows the existing page's prose style and Tailwind classes

## Out of Scope

- No interactive elements
- No data fetching
- No new routes
