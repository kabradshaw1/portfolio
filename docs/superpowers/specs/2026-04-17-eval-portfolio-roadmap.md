# RAG Evaluation & CI/CD Portfolio Roadmap

## Context

The RAG evaluation service (`services/eval`) and CI/CD pipeline optimizations are complete and deployed to QA. The next phase is making this work visible in the portfolio frontend and building the tooling to systematically track RAG quality improvements over time.

This document defines the sequenced work items, each to be specced and implemented independently.

## Roadmap

### Spec 1: CI/CD Pipeline Optimization Story Page
**Depends on:** Nothing — can start immediately
**Priority:** High — simplest item, immediately visible in portfolio

Add a narrative section to the existing `/cicd` route documenting the problem-solving story: how adding the eval service exposed CI/CD performance issues (20-minute pip installs, broken QA deploys from immutable Jobs, missing image tags) and how each was systematically diagnosed and fixed. Includes before/after timing metrics at each step.

Static content — no backend needed. Draws from `docs/adr/cicd-performance-optimizations.md`.

**Key deliverables:**
- New section on `/cicd` page with problem → investigation → fix → result narrative
- Before/after timing table (pipeline: 30+ min → ~5 min)
- Mermaid diagrams showing the optimization points in the pipeline

---

### Spec 2: Eval Service UI
**Depends on:** Eval service deployed (done)
**Priority:** High — makes the eval service usable and demonstrable

New frontend route at `/ai/eval` with interactive forms:
- Upload golden datasets (query + expected answer + expected source triples)
- Trigger evaluation runs against a dataset
- Poll for completion and display scorecards
- Per-query score breakdowns with retrieved contexts
- HealthGate pattern for graceful degradation when services are down

New API client (`frontend/src/lib/eval-api.ts`) for eval service endpoints.

**Key deliverables:**
- `/ai/eval` page with dataset management and evaluation runner
- Scorecard display with per-metric gauges and per-query detail
- Integration with eval service API (`/datasets`, `/evaluations`)

---

### Spec 3: Eval Service Enhancements (Comparison & History)
**Depends on:** Spec 2 (UI exists to consume the data)
**Priority:** Medium — enables the tracking narrative

Backend additions to the eval service:
- `GET /evaluations/compare?ids=a,b` — side-by-side score comparison between two runs
- `notes` field on evaluation creation — record what changed (e.g., "increased chunk overlap from 200 to 300")
- `GET /evaluations/history?dataset_id=x` — score trends across all runs for a dataset

Small changes: 2-3 new endpoints, 1 new DB column, corresponding Pydantic models.

**Key deliverables:**
- Comparison endpoint returning delta scores
- History endpoint returning time-series scores
- Notes field for annotation

---

### Spec 4: RAG Improvement Tracking Dashboard
**Depends on:** Specs 2 + 3
**Priority:** Medium — the culmination of the eval work

Extend the eval UI with a dashboard view showing:
- Score trend line charts over time (faithfulness, relevancy, precision, recall)
- Side-by-side run comparison with delta highlighting
- Annotated change log: what was modified, when, and the resulting quality impact
- Links from the timeline to detailed scorecard views

This is the "proof of systematic RAG improvement" page — shows interviewers that Kyle doesn't just build RAG pipelines, he measures and improves them.

**Key deliverables:**
- Time-series charts (likely using a lightweight charting library)
- Comparison view with score deltas
- Change log with annotations linked to evaluation runs

## Execution Order

```
Spec 1 (CI/CD story)     ──→ can start now
Spec 2 (Eval UI)          ──→ can start now (parallel with Spec 1)
Spec 3 (Eval enhancements) ──→ after Spec 2
Spec 4 (Tracking dashboard) ──→ after Specs 2 + 3
```

Specs 1 and 2 are independent and can be worked in parallel. Specs 3 and 4 are sequential and build on Spec 2.
