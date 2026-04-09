## Java Analytics Frontend Design

**Date:** 2026-04-09
**Status:** Draft
**Author:** Kyle Bradshaw + Claude
**Related:** `docs/superpowers/specs/2026-04-05-java-analytics-and-optimization-design.md`

## Context

The Java analytics + optimization work (Flyway, indexes, Redis caching, HikariCP tuning, MongoDB aggregation, GraphQL `projectStats`/`projectVelocity`/`projectHealth` queries) is fully built and deployed, but the original spec explicitly listed the frontend as out of scope. The GraphQL schema now exposes a composite `projectHealth` query that is purpose-built for a dashboard view and is currently unused by the UI.

This spec adds the frontend surface needed to showcase that work for a portfolio/recruiter audience without rewriting the existing Java pages.

## Goals

1. Give the analytics work a visible home so a recruiter can land on it in one click.
2. Surface a lightweight analytics summary inline where the data lives (the project task view), so the feature is discoverable from the app side too.
3. Exercise the `projectHealth` composite query end-to-end (gateway cache included).
4. Do not rewrite `/java` or `/java/tasks`; only additive changes.

## Non-Goals

- No new GraphQL queries or backend changes. The existing schema is sufficient.
- No drill-down / filtering UI beyond project selection and the fixed `weeks=8` velocity window.
- No CSV export, printable reports, or time-range pickers.
- No changes to Go or Python stacks.

## User-Facing Surfaces

### 1. `/java/dashboard` — full analytics dashboard (new page)

A dedicated authenticated page that renders everything `projectHealth` returns for a selected project.

**Layout (top to bottom):**

1. **Header row** — page title ("Project Analytics"), project dropdown selector on the right.
2. **Stat cards strip** — four summary cards: Overdue, Avg Completion Time, Active Contributors, Total Events.
3. **Task breakdown section** — two charts side by side:
   - Bar chart: task count by status (TODO / IN_PROGRESS / DONE)
   - Bar chart: task count by priority (LOW / MEDIUM / HIGH)
4. **Velocity section**:
   - Line or area chart: weekly throughput (completed vs created) over the last 8 weeks
   - Three stat cards inline: p50 / p75 / p95 lead time (hours)
5. **Member workload section** — simple table (name, assigned, completed) from `stats.memberWorkload`.
6. **Activity section**:
   - Horizontal bar chart: event count by type
   - Line chart: weekly activity (events + comments) over the last 8 weeks
   - Stat card: comment count

All data comes from a **single** `projectHealth(projectId)` query. No per-section queries — this deliberately exercises the gateway's Redis-cached composite resolver.

**Project selection:**

- Dropdown at top, populated by the existing `myProjects` query.
- Current selection persisted in URL as `?projectId=...` for shareable links.
- On initial load with no query param: redirect to the first project returned by `myProjects`.
- If the user has zero projects: render an empty state with a CTA to `/java/tasks` to create one.

**Auth:**

- Page is protected by the same auth wrapper used by `/java/tasks`. Unauthenticated users are redirected to login.

**Loading / error states:**

- Skeleton placeholders for each section while `projectHealth` is in flight.
- On error: inline error card with a retry button. Do not crash the page.
- Empty project (no tasks, no activity): each chart renders an "No data yet" placeholder rather than an empty axis.

### 2. `/java/tasks/[projectId]` — inline analytics strip (additive)

A compact 4-metric summary row added above the existing task list for a given project.

**Metrics shown:**

- Overdue count (from `stats.overdueCount`)
- Completed this week (from `velocity.weeklyThroughput[0].completed`)
- Avg lead time hours (from `velocity.avgLeadTimeHours`)
- Active contributors (from `activity.activeContributors`)

**Right side of the strip:** a small "View full dashboard →" link to `/java/dashboard?projectId=...`.

Data source: the same `projectHealth` query. Because the gateway caches it in Redis, hitting it from both the tasks page and the dashboard is cheap.

The strip is a self-contained component; if the query fails it collapses silently rather than blocking the task list.

### 3. `/java` — description page (minimal change)

Add a second CTA button next to the existing "Open Task Manager" link:

- "View Analytics Dashboard →" → `/java/dashboard`

Also add one short paragraph under the existing "Task Management System" section describing the analytics layer (Flyway migrations, partial indexes, Redis caching, HikariCP tuning, MongoDB aggregation) with a link to the ADR at `docs/adr/java-task-management/07_analytics_and_optimization.md`. Keep it under 4 sentences.

No other changes to `/java`.

## Data Contract

All three surfaces consume the existing gateway GraphQL schema. One new Apollo query document is introduced:

```graphql
query ProjectHealth($projectId: ID!) {
  projectHealth(projectId: $projectId) {
    stats {
      taskCountByStatus { todo inProgress done }
      taskCountByPriority { low medium high }
      overdueCount
      avgCompletionTimeHours
      memberWorkload { userId name assignedCount completedCount }
    }
    velocity {
      weeklyThroughput { week completed created }
      avgLeadTimeHours
      leadTimePercentiles { p50 p75 p95 }
    }
    activity {
      totalEvents
      eventCountByType { eventType count }
      commentCount
      activeContributors
      weeklyActivity { week events comments }
    }
  }
}
```

The existing `myProjects` query is reused for the dropdown on the dashboard page.

## Charting

**Library:** Recharts via shadcn/ui's `<Chart>` component family (`npx shadcn@latest add chart`).

**Why:** Drops directly into the existing shadcn aesthetic, themed via CSS variables so it picks up light/dark mode automatically, no styling clash, and handles all four chart types we need (vertical bar, horizontal bar, line, area).

**Chart types used:**

- Vertical bar: status breakdown, priority breakdown
- Area (or line): weekly throughput, weekly activity
- Horizontal bar: event count by type
- Cards (no chart): scalar stats and percentiles

Stat cards use the existing shadcn `Card` primitive — no new component needed.

## Component Structure

New files under `frontend/src/`:

```
app/java/dashboard/
  page.tsx                    # server component shell, auth guard, reads ?projectId
  dashboard-client.tsx        # client component: dropdown, query, renders sections

components/java/analytics/
  project-selector.tsx        # dropdown bound to URL param
  stat-card.tsx               # shared scalar card (label, value, optional unit)
  task-breakdown-charts.tsx   # status + priority bar charts
  velocity-section.tsx        # throughput chart + percentile cards
  member-workload-table.tsx   # table
  activity-section.tsx        # event-type bar + weekly activity chart
  analytics-strip.tsx         # 4-metric summary strip used on tasks page
  project-health.graphql.ts   # gql document + generated types entry

lib/graphql/
  # existing Apollo client reused; no new files
```

Each section component takes a typed slice of the `ProjectHealth` response as its only prop and is independently renderable — this keeps the dashboard page thin and makes each chart easy to test in isolation.

The `/java/tasks/[projectId]` page imports `analytics-strip.tsx` directly; it does **not** share state with the dashboard.

## Routing & Auth

- New route: `app/java/dashboard/page.tsx` with the same auth guard pattern used by `app/java/tasks/*`.
- URL param: `?projectId=<uuid>`. If missing, the client component fetches `myProjects`, picks the first, and replaces the URL.
- No new middleware. No new env vars.

## Error & Empty States

| State | Behavior |
|---|---|
| Not authenticated | Redirect to login (existing pattern) |
| `myProjects` returns empty | Render empty state card with CTA to `/java/tasks` |
| `projectHealth` in flight | Skeleton placeholders per section |
| `projectHealth` errors | Error card with retry button at the top; sections hidden |
| Project has no tasks | Charts render "No data yet" placeholder; strip shows zeros |
| Strip query fails on tasks page | Strip renders nothing; task list unaffected |

## Testing Strategy

**Unit / component (Vitest + React Testing Library):**

- `stat-card.tsx` — renders label + value, handles null/undefined.
- `analytics-strip.tsx` — renders 4 metrics from mock data; collapses on error.
- `project-selector.tsx` — syncs selection to URL param.
- Each chart section component — renders with mock data, renders empty state with empty arrays.

**E2E (Playwright, mocked):**

- New spec `frontend/e2e/java-dashboard.spec.ts`:
  - Mock `myProjects` + `projectHealth` responses.
  - Visit `/java/dashboard`, assert all four sections render.
  - Change project in dropdown, assert URL updates and new data fetched.
  - Visit `/java/dashboard?projectId=<invalid>`, assert error state.
- Extend existing `java-tasks` E2E to assert the analytics strip renders.

**Production smoke (post-deploy):**

- Add one check: `GET /java/dashboard` returns 200 and contains expected page heading.

No new backend tests — the analytics backend already has integration test coverage from the prior spec.

## Preflight

Before commit: `make preflight-frontend` and `make preflight-e2e`. Any new `NEXT_PUBLIC_*` env var (none expected for this work) would need to be added to Vercel before merge per `CLAUDE.md`.

## Files Summary

### New
- `frontend/src/app/java/dashboard/page.tsx`
- `frontend/src/app/java/dashboard/dashboard-client.tsx`
- `frontend/src/components/java/analytics/project-selector.tsx`
- `frontend/src/components/java/analytics/stat-card.tsx`
- `frontend/src/components/java/analytics/task-breakdown-charts.tsx`
- `frontend/src/components/java/analytics/velocity-section.tsx`
- `frontend/src/components/java/analytics/member-workload-table.tsx`
- `frontend/src/components/java/analytics/activity-section.tsx`
- `frontend/src/components/java/analytics/analytics-strip.tsx`
- `frontend/src/components/java/analytics/project-health.graphql.ts`
- `frontend/e2e/java-dashboard.spec.ts`
- shadcn chart primitive files added by `npx shadcn@latest add chart`

### Modified
- `frontend/src/app/java/page.tsx` — add "View Analytics Dashboard" CTA + short paragraph about analytics layer with ADR link
- `frontend/src/app/java/tasks/[projectId]/...` — mount `analytics-strip.tsx` above the task list (exact file determined during implementation)
- `frontend/package.json` — adds `recharts` (pulled in by shadcn chart)

## Out of Scope

- Backend changes of any kind
- New analytics queries or fields
- Drill-down pages, time-range pickers, CSV export
- Rewriting `/java` or `/java/tasks`
- Notification-service or Go/Python surfaces
