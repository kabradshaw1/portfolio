# Java Analytics Frontend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `/java/dashboard` analytics page plus an inline analytics strip on the project tasks page, both consuming the existing `projectHealth` GraphQL query.

**Architecture:** One Apollo `PROJECT_HEALTH` query document is reused by two surfaces. A dedicated client page under `app/java/dashboard` hosts the full dashboard; a small `<AnalyticsStrip>` component is mounted above the existing Kanban board. Each chart/section is its own file under `components/java/analytics/` so they can be unit-tested in isolation. Charts use shadcn's `<Chart>` primitive (Recharts under the hood).

**Tech Stack:** Next.js 16 (app router, client components), React 19, Apollo Client, shadcn/ui chart + Recharts, Vitest + React Testing Library, Playwright.

**Spec:** `docs/superpowers/specs/2026-04-09-java-analytics-frontend-design.md`

---

## Pre-flight Context

Read before starting:
- `docs/superpowers/specs/2026-04-09-java-analytics-frontend-design.md` — the design this plan executes.
- `frontend/CLAUDE.md` → `frontend/AGENTS.md` — Next.js in this repo differs from training data; read `node_modules/next/dist/docs/` for anything non-obvious.
- `frontend/src/app/java/tasks/[projectId]/page.tsx` — existing project detail page that will mount the analytics strip.
- `frontend/src/components/java/ProjectList.tsx` — canonical example of Apollo `useQuery` with the `myProjects` query.
- `frontend/src/lib/auth.ts` — `GATEWAY_URL` + token helpers; use these for auth.
- `frontend/src/lib/apollo-client.ts` — Apollo client wiring.

**Pre-commit checks for every commit:** `make preflight-frontend`. If any E2E task is touched: `make preflight-e2e`. Never push — Kyle handles push/merge.

---

## File Structure

**New files:**
```
frontend/src/app/java/dashboard/
  page.tsx                         # thin server shell + auth guard
  dashboard-client.tsx             # useQuery(PROJECT_HEALTH), composes sections

frontend/src/components/java/analytics/
  project-health.graphql.ts        # gql document + TS types
  project-selector.tsx             # dropdown, URL-synced
  stat-card.tsx                    # reusable scalar card
  task-breakdown-charts.tsx        # status + priority bar charts
  velocity-section.tsx             # throughput chart + percentile cards
  member-workload-table.tsx        # table
  activity-section.tsx             # event-type bar + weekly activity chart
  analytics-strip.tsx              # 4-metric summary bar for tasks page

frontend/src/components/java/analytics/__tests__/
  stat-card.test.tsx
  analytics-strip.test.tsx
  project-selector.test.tsx
  task-breakdown-charts.test.tsx
  velocity-section.test.tsx
  activity-section.test.tsx

frontend/e2e/java-dashboard.spec.ts
```

**Modified files:**
```
frontend/src/app/java/tasks/[projectId]/page.tsx   # mount <AnalyticsStrip/>
frontend/src/app/java/page.tsx                     # add CTA + analytics paragraph
frontend/package.json                               # recharts (via shadcn chart)
frontend/src/components/ui/chart.tsx                # added by shadcn CLI
```

---

## Task 1: Add shadcn chart primitive

**Files:**
- Create: `frontend/src/components/ui/chart.tsx` (via CLI)
- Modify: `frontend/package.json` (recharts added as dep)

- [ ] **Step 1: Install shadcn chart primitive**

```bash
cd frontend && npx shadcn@latest add chart
```

Expected: new file at `src/components/ui/chart.tsx`, `recharts` added to `package.json`.

- [ ] **Step 2: Verify install compiles**

```bash
cd frontend && npm run type-check 2>&1 | tail -20
```
(Or `npx tsc --noEmit` if no `type-check` script.)

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/ui/chart.tsx frontend/package.json frontend/package-lock.json
git commit -m "chore(frontend): add shadcn chart primitive"
```

---

## Task 2: GraphQL document + shared types

**Files:**
- Create: `frontend/src/components/java/analytics/project-health.graphql.ts`

- [ ] **Step 1: Write the document and types**

```typescript
// frontend/src/components/java/analytics/project-health.graphql.ts
import { gql } from "@apollo/client";

export const PROJECT_HEALTH = gql`
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
`;

export interface TaskStatusCounts { todo: number; inProgress: number; done: number; }
export interface TaskPriorityCounts { low: number; medium: number; high: number; }
export interface MemberWorkload {
  userId: string;
  name: string;
  assignedCount: number;
  completedCount: number;
}
export interface ProjectStats {
  taskCountByStatus: TaskStatusCounts;
  taskCountByPriority: TaskPriorityCounts;
  overdueCount: number;
  avgCompletionTimeHours: number | null;
  memberWorkload: MemberWorkload[];
}
export interface WeeklyThroughput { week: string; completed: number; created: number; }
export interface Percentiles { p50: number; p75: number; p95: number; }
export interface VelocityMetrics {
  weeklyThroughput: WeeklyThroughput[];
  avgLeadTimeHours: number | null;
  leadTimePercentiles: Percentiles;
}
export interface EventTypeCount { eventType: string; count: number; }
export interface WeeklyActivity { week: string; events: number; comments: number; }
export interface ActivityStats {
  totalEvents: number;
  eventCountByType: EventTypeCount[];
  commentCount: number;
  activeContributors: number;
  weeklyActivity: WeeklyActivity[];
}
export interface ProjectHealth {
  stats: ProjectStats;
  velocity: VelocityMetrics;
  activity: ActivityStats;
}
export interface ProjectHealthData {
  projectHealth: ProjectHealth;
}
```

- [ ] **Step 2: Type-check**

```bash
cd frontend && npx tsc --noEmit
```
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/java/analytics/project-health.graphql.ts
git commit -m "feat(frontend): add projectHealth graphql document and types"
```

---

## Task 3: StatCard component (TDD)

**Files:**
- Create: `frontend/src/components/java/analytics/stat-card.tsx`
- Test: `frontend/src/components/java/analytics/__tests__/stat-card.test.tsx`

- [ ] **Step 1: Write failing test**

```tsx
// stat-card.test.tsx
import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { StatCard } from "../stat-card";

describe("StatCard", () => {
  it("renders label and value", () => {
    render(<StatCard label="Overdue" value={3} />);
    expect(screen.getByText("Overdue")).toBeInTheDocument();
    expect(screen.getByText("3")).toBeInTheDocument();
  });

  it("renders unit when provided", () => {
    render(<StatCard label="Avg lead time" value={24.5} unit="h" />);
    expect(screen.getByText(/24\.5/)).toBeInTheDocument();
    expect(screen.getByText("h")).toBeInTheDocument();
  });

  it("renders em-dash when value is null", () => {
    render(<StatCard label="Avg lead time" value={null} unit="h" />);
    expect(screen.getByText("—")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run and confirm it fails**

```bash
cd frontend && npx vitest run src/components/java/analytics/__tests__/stat-card.test.tsx
```
Expected: FAIL (`Cannot find module '../stat-card'`).

- [ ] **Step 3: Implement**

```tsx
// stat-card.tsx
import { Card, CardContent } from "@/components/ui/card";

interface Props {
  label: string;
  value: number | null;
  unit?: string;
}

export function StatCard({ label, value, unit }: Props) {
  const display = value === null || value === undefined ? "—" : formatNumber(value);
  return (
    <Card>
      <CardContent className="py-4">
        <div className="text-xs text-muted-foreground uppercase tracking-wide">{label}</div>
        <div className="mt-2 flex items-baseline gap-1">
          <span className="text-2xl font-semibold">{display}</span>
          {unit && value !== null && <span className="text-sm text-muted-foreground">{unit}</span>}
        </div>
      </CardContent>
    </Card>
  );
}

function formatNumber(n: number): string {
  if (Number.isInteger(n)) return n.toString();
  return n.toFixed(1);
}
```

- [ ] **Step 4: Tests pass**

```bash
cd frontend && npx vitest run src/components/java/analytics/__tests__/stat-card.test.tsx
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/java/analytics/stat-card.tsx frontend/src/components/java/analytics/__tests__/stat-card.test.tsx
git commit -m "feat(frontend): add analytics StatCard component"
```

---

## Task 4: ProjectSelector component (TDD)

**Files:**
- Create: `frontend/src/components/java/analytics/project-selector.tsx`
- Test: `frontend/src/components/java/analytics/__tests__/project-selector.test.tsx`

- [ ] **Step 1: Write failing test**

```tsx
// project-selector.test.tsx
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { ProjectSelector } from "../project-selector";

const projects = [
  { id: "p1", name: "Alpha" },
  { id: "p2", name: "Beta" },
];

describe("ProjectSelector", () => {
  it("renders options and highlights current", () => {
    render(<ProjectSelector projects={projects} currentId="p1" onChange={() => {}} />);
    const select = screen.getByRole("combobox") as HTMLSelectElement;
    expect(select.value).toBe("p1");
    expect(screen.getByRole("option", { name: "Alpha" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Beta" })).toBeInTheDocument();
  });

  it("calls onChange with new id", () => {
    const onChange = vi.fn();
    render(<ProjectSelector projects={projects} currentId="p1" onChange={onChange} />);
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "p2" } });
    expect(onChange).toHaveBeenCalledWith("p2");
  });
});
```

- [ ] **Step 2: Run, confirm fail**

```bash
cd frontend && npx vitest run src/components/java/analytics/__tests__/project-selector.test.tsx
```
Expected: FAIL.

- [ ] **Step 3: Implement**

```tsx
// project-selector.tsx
"use client";

interface Project { id: string; name: string; }

interface Props {
  projects: Project[];
  currentId: string;
  onChange: (id: string) => void;
}

export function ProjectSelector({ projects, currentId, onChange }: Props) {
  return (
    <select
      className="rounded-md border bg-background px-3 py-2 text-sm"
      value={currentId}
      onChange={(e) => onChange(e.target.value)}
      aria-label="Select project"
    >
      {projects.map((p) => (
        <option key={p.id} value={p.id}>
          {p.name}
        </option>
      ))}
    </select>
  );
}
```

- [ ] **Step 4: Pass**

```bash
cd frontend && npx vitest run src/components/java/analytics/__tests__/project-selector.test.tsx
```

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/java/analytics/project-selector.tsx frontend/src/components/java/analytics/__tests__/project-selector.test.tsx
git commit -m "feat(frontend): add analytics ProjectSelector component"
```

---

## Task 5: TaskBreakdownCharts (status + priority)

**Files:**
- Create: `frontend/src/components/java/analytics/task-breakdown-charts.tsx`
- Test: `frontend/src/components/java/analytics/__tests__/task-breakdown-charts.test.tsx`

- [ ] **Step 1: Failing test**

```tsx
import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { TaskBreakdownCharts } from "../task-breakdown-charts";

const stats = {
  taskCountByStatus: { todo: 5, inProgress: 3, done: 12 },
  taskCountByPriority: { low: 4, medium: 8, high: 8 },
  overdueCount: 2,
  avgCompletionTimeHours: 48.5,
  memberWorkload: [],
};

describe("TaskBreakdownCharts", () => {
  it("renders both chart headings", () => {
    render(<TaskBreakdownCharts stats={stats} />);
    expect(screen.getByText(/By Status/i)).toBeInTheDocument();
    expect(screen.getByText(/By Priority/i)).toBeInTheDocument();
  });

  it("shows empty state when all counts are zero", () => {
    render(
      <TaskBreakdownCharts
        stats={{ ...stats,
          taskCountByStatus: { todo: 0, inProgress: 0, done: 0 },
          taskCountByPriority: { low: 0, medium: 0, high: 0 },
        }}
      />
    );
    expect(screen.getAllByText(/No data yet/i).length).toBeGreaterThan(0);
  });
});
```

- [ ] **Step 2: Run, fail**

```bash
cd frontend && npx vitest run src/components/java/analytics/__tests__/task-breakdown-charts.test.tsx
```

- [ ] **Step 3: Implement**

```tsx
// task-breakdown-charts.tsx
"use client";

import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from "recharts";
import { ChartContainer, ChartTooltip, ChartTooltipContent } from "@/components/ui/chart";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import type { ProjectStats } from "./project-health.graphql";

interface Props { stats: ProjectStats; }

export function TaskBreakdownCharts({ stats }: Props) {
  const statusData = [
    { name: "TODO", count: stats.taskCountByStatus.todo },
    { name: "IN PROGRESS", count: stats.taskCountByStatus.inProgress },
    { name: "DONE", count: stats.taskCountByStatus.done },
  ];
  const priorityData = [
    { name: "LOW", count: stats.taskCountByPriority.low },
    { name: "MEDIUM", count: stats.taskCountByPriority.medium },
    { name: "HIGH", count: stats.taskCountByPriority.high },
  ];
  const statusEmpty = statusData.every((d) => d.count === 0);
  const priorityEmpty = priorityData.every((d) => d.count === 0);

  const config = { count: { label: "Tasks", color: "hsl(var(--chart-1))" } };

  return (
    <div className="grid gap-4 md:grid-cols-2">
      <Card>
        <CardHeader><CardTitle>By Status</CardTitle></CardHeader>
        <CardContent>
          {statusEmpty ? (
            <p className="text-sm text-muted-foreground">No data yet</p>
          ) : (
            <ChartContainer config={config} className="h-48 w-full">
              <BarChart data={statusData}>
                <CartesianGrid vertical={false} />
                <XAxis dataKey="name" tickLine={false} axisLine={false} />
                <YAxis allowDecimals={false} />
                <ChartTooltip content={<ChartTooltipContent />} />
                <Bar dataKey="count" fill="var(--color-count)" radius={4} />
              </BarChart>
            </ChartContainer>
          )}
        </CardContent>
      </Card>
      <Card>
        <CardHeader><CardTitle>By Priority</CardTitle></CardHeader>
        <CardContent>
          {priorityEmpty ? (
            <p className="text-sm text-muted-foreground">No data yet</p>
          ) : (
            <ChartContainer config={config} className="h-48 w-full">
              <BarChart data={priorityData}>
                <CartesianGrid vertical={false} />
                <XAxis dataKey="name" tickLine={false} axisLine={false} />
                <YAxis allowDecimals={false} />
                <ChartTooltip content={<ChartTooltipContent />} />
                <Bar dataKey="count" fill="var(--color-count)" radius={4} />
              </BarChart>
            </ChartContainer>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
```

- [ ] **Step 4: Pass**

```bash
cd frontend && npx vitest run src/components/java/analytics/__tests__/task-breakdown-charts.test.tsx
```

Note: Recharts in JSDOM may warn about `ResponsiveContainer` dimensions. If tests fail on dimension errors, wrap `ChartContainer` inside a fixed-size div in the test (`<div style={{ width: 400, height: 200 }}>...</div>`) and re-run.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/java/analytics/task-breakdown-charts.tsx frontend/src/components/java/analytics/__tests__/task-breakdown-charts.test.tsx
git commit -m "feat(frontend): add TaskBreakdownCharts component"
```

---

## Task 6: VelocitySection component

**Files:**
- Create: `frontend/src/components/java/analytics/velocity-section.tsx`
- Test: `frontend/src/components/java/analytics/__tests__/velocity-section.test.tsx`

- [ ] **Step 1: Failing test**

```tsx
import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { VelocitySection } from "../velocity-section";

const velocity = {
  weeklyThroughput: [
    { week: "2026-W14", completed: 5, created: 8 },
    { week: "2026-W13", completed: 3, created: 4 },
  ],
  avgLeadTimeHours: 36.2,
  leadTimePercentiles: { p50: 24.0, p75: 48.0, p95: 120.0 },
};

describe("VelocitySection", () => {
  it("renders percentile cards", () => {
    render(<VelocitySection velocity={velocity} />);
    expect(screen.getByText("p50")).toBeInTheDocument();
    expect(screen.getByText("p75")).toBeInTheDocument();
    expect(screen.getByText("p95")).toBeInTheDocument();
  });

  it("renders empty state when no throughput data", () => {
    render(<VelocitySection velocity={{ ...velocity, weeklyThroughput: [] }} />);
    expect(screen.getByText(/No data yet/i)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run, fail**

```bash
cd frontend && npx vitest run src/components/java/analytics/__tests__/velocity-section.test.tsx
```

- [ ] **Step 3: Implement**

```tsx
// velocity-section.tsx
"use client";

import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from "recharts";
import { ChartContainer, ChartTooltip, ChartTooltipContent } from "@/components/ui/chart";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { StatCard } from "./stat-card";
import type { VelocityMetrics } from "./project-health.graphql";

interface Props { velocity: VelocityMetrics; }

export function VelocitySection({ velocity }: Props) {
  // Reverse so oldest week is on the left
  const data = [...velocity.weeklyThroughput].reverse();
  const empty = data.length === 0;

  const config = {
    completed: { label: "Completed", color: "hsl(var(--chart-1))" },
    created: { label: "Created", color: "hsl(var(--chart-2))" },
  };

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader><CardTitle>Weekly Throughput</CardTitle></CardHeader>
        <CardContent>
          {empty ? (
            <p className="text-sm text-muted-foreground">No data yet</p>
          ) : (
            <ChartContainer config={config} className="h-56 w-full">
              <AreaChart data={data}>
                <CartesianGrid vertical={false} />
                <XAxis dataKey="week" tickLine={false} axisLine={false} />
                <YAxis allowDecimals={false} />
                <ChartTooltip content={<ChartTooltipContent />} />
                <Area type="monotone" dataKey="completed" stroke="var(--color-completed)" fill="var(--color-completed)" fillOpacity={0.3} />
                <Area type="monotone" dataKey="created" stroke="var(--color-created)" fill="var(--color-created)" fillOpacity={0.2} />
              </AreaChart>
            </ChartContainer>
          )}
        </CardContent>
      </Card>
      <div className="grid gap-3 sm:grid-cols-4">
        <StatCard label="Avg Lead Time" value={velocity.avgLeadTimeHours} unit="h" />
        <StatCard label="p50" value={velocity.leadTimePercentiles.p50} unit="h" />
        <StatCard label="p75" value={velocity.leadTimePercentiles.p75} unit="h" />
        <StatCard label="p95" value={velocity.leadTimePercentiles.p95} unit="h" />
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Pass**

```bash
cd frontend && npx vitest run src/components/java/analytics/__tests__/velocity-section.test.tsx
```

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/java/analytics/velocity-section.tsx frontend/src/components/java/analytics/__tests__/velocity-section.test.tsx
git commit -m "feat(frontend): add VelocitySection component"
```

---

## Task 7: MemberWorkloadTable component

**Files:**
- Create: `frontend/src/components/java/analytics/member-workload-table.tsx`

- [ ] **Step 1: Implement (simple enough to skip dedicated test)**

```tsx
// member-workload-table.tsx
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import type { MemberWorkload } from "./project-health.graphql";

interface Props { members: MemberWorkload[]; }

export function MemberWorkloadTable({ members }: Props) {
  return (
    <Card>
      <CardHeader><CardTitle>Member Workload</CardTitle></CardHeader>
      <CardContent>
        {members.length === 0 ? (
          <p className="text-sm text-muted-foreground">No members assigned</p>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b text-left text-muted-foreground">
                <th className="py-2 font-medium">Name</th>
                <th className="py-2 font-medium text-right">Assigned</th>
                <th className="py-2 font-medium text-right">Completed</th>
              </tr>
            </thead>
            <tbody>
              {members.map((m) => (
                <tr key={m.userId} className="border-b last:border-0">
                  <td className="py-2">{m.name}</td>
                  <td className="py-2 text-right">{m.assignedCount}</td>
                  <td className="py-2 text-right">{m.completedCount}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </CardContent>
    </Card>
  );
}
```

- [ ] **Step 2: Type-check**

```bash
cd frontend && npx tsc --noEmit
```

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/java/analytics/member-workload-table.tsx
git commit -m "feat(frontend): add MemberWorkloadTable component"
```

---

## Task 8: ActivitySection component

**Files:**
- Create: `frontend/src/components/java/analytics/activity-section.tsx`
- Test: `frontend/src/components/java/analytics/__tests__/activity-section.test.tsx`

- [ ] **Step 1: Failing test**

```tsx
import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { ActivitySection } from "../activity-section";

const activity = {
  totalEvents: 142,
  eventCountByType: [
    { eventType: "task.created", count: 20 },
    { eventType: "task.status_changed", count: 85 },
    { eventType: "task.assigned", count: 37 },
  ],
  commentCount: 24,
  activeContributors: 5,
  weeklyActivity: [
    { week: "2026-W14", events: 32, comments: 6 },
    { week: "2026-W13", events: 28, comments: 4 },
  ],
};

describe("ActivitySection", () => {
  it("renders event type counts", () => {
    render(<ActivitySection activity={activity} />);
    expect(screen.getByText("task.created")).toBeInTheDocument();
    expect(screen.getByText("task.status_changed")).toBeInTheDocument();
  });

  it("renders empty state when no events", () => {
    render(
      <ActivitySection
        activity={{ ...activity, eventCountByType: [], weeklyActivity: [] }}
      />
    );
    expect(screen.getAllByText(/No data yet/i).length).toBeGreaterThan(0);
  });
});
```

- [ ] **Step 2: Run, fail**

```bash
cd frontend && npx vitest run src/components/java/analytics/__tests__/activity-section.test.tsx
```

- [ ] **Step 3: Implement**

```tsx
// activity-section.tsx
"use client";

import { Bar, BarChart, CartesianGrid, Line, LineChart, XAxis, YAxis } from "recharts";
import { ChartContainer, ChartTooltip, ChartTooltipContent } from "@/components/ui/chart";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import type { ActivityStats } from "./project-health.graphql";

interface Props { activity: ActivityStats; }

export function ActivitySection({ activity }: Props) {
  const typeEmpty = activity.eventCountByType.length === 0;
  const weeklyData = [...activity.weeklyActivity].reverse();
  const weeklyEmpty = weeklyData.length === 0;

  const typeConfig = { count: { label: "Events", color: "hsl(var(--chart-3))" } };
  const weeklyConfig = {
    events: { label: "Events", color: "hsl(var(--chart-1))" },
    comments: { label: "Comments", color: "hsl(var(--chart-2))" },
  };

  return (
    <div className="grid gap-4 md:grid-cols-2">
      <Card>
        <CardHeader><CardTitle>Events by Type</CardTitle></CardHeader>
        <CardContent>
          {typeEmpty ? (
            <p className="text-sm text-muted-foreground">No data yet</p>
          ) : (
            <ChartContainer config={typeConfig} className="h-56 w-full">
              <BarChart data={activity.eventCountByType} layout="vertical">
                <CartesianGrid horizontal={false} />
                <XAxis type="number" allowDecimals={false} />
                <YAxis dataKey="eventType" type="category" width={140} />
                <ChartTooltip content={<ChartTooltipContent />} />
                <Bar dataKey="count" fill="var(--color-count)" radius={4} />
              </BarChart>
            </ChartContainer>
          )}
        </CardContent>
      </Card>
      <Card>
        <CardHeader><CardTitle>Weekly Activity</CardTitle></CardHeader>
        <CardContent>
          {weeklyEmpty ? (
            <p className="text-sm text-muted-foreground">No data yet</p>
          ) : (
            <ChartContainer config={weeklyConfig} className="h-56 w-full">
              <LineChart data={weeklyData}>
                <CartesianGrid vertical={false} />
                <XAxis dataKey="week" tickLine={false} axisLine={false} />
                <YAxis allowDecimals={false} />
                <ChartTooltip content={<ChartTooltipContent />} />
                <Line type="monotone" dataKey="events" stroke="var(--color-events)" strokeWidth={2} dot={false} />
                <Line type="monotone" dataKey="comments" stroke="var(--color-comments)" strokeWidth={2} dot={false} />
              </LineChart>
            </ChartContainer>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
```

- [ ] **Step 4: Pass**

```bash
cd frontend && npx vitest run src/components/java/analytics/__tests__/activity-section.test.tsx
```

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/java/analytics/activity-section.tsx frontend/src/components/java/analytics/__tests__/activity-section.test.tsx
git commit -m "feat(frontend): add ActivitySection component"
```

---

## Task 9: AnalyticsStrip component (used on tasks page)

**Files:**
- Create: `frontend/src/components/java/analytics/analytics-strip.tsx`
- Test: `frontend/src/components/java/analytics/__tests__/analytics-strip.test.tsx`

The strip owns its own `useQuery(PROJECT_HEALTH)` call so it can be dropped anywhere without threading data through props.

- [ ] **Step 1: Failing test**

```tsx
import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { MockedProvider } from "@apollo/client/testing";
import { AnalyticsStrip } from "../analytics-strip";
import { PROJECT_HEALTH } from "../project-health.graphql";

const mockData = {
  projectHealth: {
    stats: {
      taskCountByStatus: { todo: 0, inProgress: 0, done: 0 },
      taskCountByPriority: { low: 0, medium: 0, high: 0 },
      overdueCount: 2,
      avgCompletionTimeHours: null,
      memberWorkload: [],
    },
    velocity: {
      weeklyThroughput: [{ week: "2026-W14", completed: 7, created: 9 }],
      avgLeadTimeHours: 36.2,
      leadTimePercentiles: { p50: 24, p75: 48, p95: 120 },
    },
    activity: {
      totalEvents: 0,
      eventCountByType: [],
      commentCount: 0,
      activeContributors: 5,
      weeklyActivity: [],
    },
  },
};

const mocks = [
  {
    request: { query: PROJECT_HEALTH, variables: { projectId: "p1" } },
    result: { data: mockData },
  },
];

describe("AnalyticsStrip", () => {
  it("renders four metrics after load", async () => {
    render(
      <MockedProvider mocks={mocks} addTypename={false}>
        <AnalyticsStrip projectId="p1" />
      </MockedProvider>
    );
    expect(await screen.findByText("Overdue")).toBeInTheDocument();
    expect(screen.getByText("2")).toBeInTheDocument();
    expect(screen.getByText("Completed This Week")).toBeInTheDocument();
    expect(screen.getByText("7")).toBeInTheDocument();
    expect(screen.getByText("Active Contributors")).toBeInTheDocument();
    expect(screen.getByText("5")).toBeInTheDocument();
  });

  it("renders nothing on error", async () => {
    const errMocks = [
      {
        request: { query: PROJECT_HEALTH, variables: { projectId: "p1" } },
        error: new Error("boom"),
      },
    ];
    const { container } = render(
      <MockedProvider mocks={errMocks} addTypename={false}>
        <AnalyticsStrip projectId="p1" />
      </MockedProvider>
    );
    // Wait a tick for error state
    await new Promise((r) => setTimeout(r, 0));
    expect(container.querySelector("[data-testid='analytics-strip']")).toBeNull();
  });
});
```

- [ ] **Step 2: Run, fail**

```bash
cd frontend && npx vitest run src/components/java/analytics/__tests__/analytics-strip.test.tsx
```

- [ ] **Step 3: Implement**

```tsx
// analytics-strip.tsx
"use client";

import Link from "next/link";
import { useQuery } from "@apollo/client/react";
import { PROJECT_HEALTH, type ProjectHealthData } from "./project-health.graphql";
import { StatCard } from "./stat-card";

interface Props { projectId: string; }

export function AnalyticsStrip({ projectId }: Props) {
  const { data, loading, error } = useQuery<ProjectHealthData>(PROJECT_HEALTH, {
    variables: { projectId },
  });

  if (loading || error || !data) return null;

  const { stats, velocity, activity } = data.projectHealth;
  const completedThisWeek = velocity.weeklyThroughput[0]?.completed ?? 0;

  return (
    <div
      data-testid="analytics-strip"
      className="mb-6 flex flex-col gap-4 rounded-lg border bg-muted/30 p-4 md:flex-row md:items-center md:justify-between"
    >
      <div className="grid flex-1 grid-cols-2 gap-3 md:grid-cols-4">
        <StatCard label="Overdue" value={stats.overdueCount} />
        <StatCard label="Completed This Week" value={completedThisWeek} />
        <StatCard label="Avg Lead Time" value={velocity.avgLeadTimeHours} unit="h" />
        <StatCard label="Active Contributors" value={activity.activeContributors} />
      </div>
      <Link
        href={`/java/dashboard?projectId=${projectId}`}
        className="text-sm font-medium underline hover:text-foreground"
      >
        View full dashboard →
      </Link>
    </div>
  );
}
```

- [ ] **Step 4: Pass**

```bash
cd frontend && npx vitest run src/components/java/analytics/__tests__/analytics-strip.test.tsx
```

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/java/analytics/analytics-strip.tsx frontend/src/components/java/analytics/__tests__/analytics-strip.test.tsx
git commit -m "feat(frontend): add AnalyticsStrip component"
```

---

## Task 10: Mount AnalyticsStrip on project tasks page

**Files:**
- Modify: `frontend/src/app/java/tasks/[projectId]/page.tsx`

- [ ] **Step 1: Add import + mount above Kanban board**

Find where `<KanbanBoard ... />` is rendered in `page.tsx` and add the strip immediately above it.

```tsx
import { AnalyticsStrip } from "@/components/java/analytics/analytics-strip";

// ...inside the return, immediately before <KanbanBoard ... />
<AnalyticsStrip projectId={projectId} />
<KanbanBoard /* existing props */ />
```

- [ ] **Step 2: Type-check + lint**

```bash
cd frontend && npx tsc --noEmit && npm run lint
```

- [ ] **Step 3: Manual smoke (optional)**

```bash
cd frontend && npm run dev
```
Visit `/java/tasks/<some project id>` with SSH tunnel running; verify strip loads.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/app/java/tasks/[projectId]/page.tsx
git commit -m "feat(frontend): mount AnalyticsStrip above project Kanban board"
```

---

## Task 11: Dashboard page shell

**Files:**
- Create: `frontend/src/app/java/dashboard/page.tsx`
- Create: `frontend/src/app/java/dashboard/dashboard-client.tsx`

The page is a client component because it uses `useSearchParams`, `useQuery`, and the dropdown.

- [ ] **Step 1: Create page.tsx (thin wrapper)**

```tsx
// page.tsx
import { Suspense } from "react";
import { DashboardClient } from "./dashboard-client";

export default function DashboardPage() {
  return (
    <Suspense fallback={<div className="p-6 text-sm text-muted-foreground">Loading…</div>}>
      <DashboardClient />
    </Suspense>
  );
}
```

- [ ] **Step 2: Create dashboard-client.tsx**

```tsx
// dashboard-client.tsx
"use client";

import { useEffect } from "react";
import { useQuery } from "@apollo/client/react";
import { gql } from "@apollo/client";
import { useRouter, useSearchParams } from "next/navigation";
import { PROJECT_HEALTH, type ProjectHealthData } from "@/components/java/analytics/project-health.graphql";
import { ProjectSelector } from "@/components/java/analytics/project-selector";
import { StatCard } from "@/components/java/analytics/stat-card";
import { TaskBreakdownCharts } from "@/components/java/analytics/task-breakdown-charts";
import { VelocitySection } from "@/components/java/analytics/velocity-section";
import { MemberWorkloadTable } from "@/components/java/analytics/member-workload-table";
import { ActivitySection } from "@/components/java/analytics/activity-section";

const MY_PROJECTS = gql`
  query MyProjects { myProjects { id name } }
`;

interface Project { id: string; name: string; }
interface MyProjectsData { myProjects: Project[]; }

export function DashboardClient() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const projectId = searchParams.get("projectId");

  const { data: projectsData, loading: projectsLoading, error: projectsError } =
    useQuery<MyProjectsData>(MY_PROJECTS);

  const projects = projectsData?.myProjects ?? [];

  // Default-select first project if none in URL
  useEffect(() => {
    if (!projectId && projects.length > 0) {
      router.replace(`/java/dashboard?projectId=${projects[0].id}`);
    }
  }, [projectId, projects, router]);

  const { data, loading, error } = useQuery<ProjectHealthData>(PROJECT_HEALTH, {
    variables: { projectId: projectId ?? "" },
    skip: !projectId,
  });

  if (projectsLoading) {
    return <div className="p-6 text-sm text-muted-foreground">Loading projects…</div>;
  }
  if (projectsError) {
    return <ErrorCard message="Failed to load projects." />;
  }
  if (projects.length === 0) {
    return (
      <div className="mx-auto max-w-2xl p-6">
        <h1 className="text-2xl font-semibold">Project Analytics</h1>
        <p className="mt-4 text-muted-foreground">
          You don&rsquo;t have any projects yet.{" "}
          <a className="underline" href="/java/tasks">Create one to get started →</a>
        </p>
      </div>
    );
  }

  const currentId = projectId ?? projects[0].id;

  return (
    <div className="mx-auto max-w-6xl p-6">
      <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
        <h1 className="text-2xl font-semibold">Project Analytics</h1>
        <ProjectSelector
          projects={projects}
          currentId={currentId}
          onChange={(id) => router.replace(`/java/dashboard?projectId=${id}`)}
        />
      </div>

      {loading && <DashboardSkeleton />}
      {error && <ErrorCard message="Failed to load analytics." />}
      {data && (
        <div className="mt-6 space-y-6">
          <div className="grid gap-3 sm:grid-cols-2 md:grid-cols-4">
            <StatCard label="Overdue" value={data.projectHealth.stats.overdueCount} />
            <StatCard label="Avg Completion" value={data.projectHealth.stats.avgCompletionTimeHours} unit="h" />
            <StatCard label="Active Contributors" value={data.projectHealth.activity.activeContributors} />
            <StatCard label="Total Events" value={data.projectHealth.activity.totalEvents} />
          </div>
          <TaskBreakdownCharts stats={data.projectHealth.stats} />
          <VelocitySection velocity={data.projectHealth.velocity} />
          <MemberWorkloadTable members={data.projectHealth.stats.memberWorkload} />
          <ActivitySection activity={data.projectHealth.activity} />
        </div>
      )}
    </div>
  );
}

function DashboardSkeleton() {
  return (
    <div className="mt-6 grid gap-3 sm:grid-cols-2 md:grid-cols-4">
      {Array.from({ length: 4 }).map((_, i) => (
        <div key={i} className="h-24 animate-pulse rounded-lg border bg-muted/40" />
      ))}
    </div>
  );
}

function ErrorCard({ message }: { message: string }) {
  return (
    <div className="mt-6 rounded-lg border border-destructive/40 bg-destructive/10 p-4 text-sm text-destructive">
      {message}
    </div>
  );
}
```

- [ ] **Step 3: Type-check + lint**

```bash
cd frontend && npx tsc --noEmit && npm run lint
```

- [ ] **Step 4: Manual smoke (optional)**

```bash
cd frontend && npm run dev
```
Visit `/java/dashboard` with SSH tunnel running.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/app/java/dashboard
git commit -m "feat(frontend): add /java/dashboard analytics page"
```

---

## Task 12: Add auth guard to dashboard route

The existing `/java/tasks` page pattern pulls `getAccessToken()` from `@/lib/auth` and redirects if missing. Match that pattern.

**Files:**
- Modify: `frontend/src/app/java/dashboard/dashboard-client.tsx`

- [ ] **Step 1: Read the auth pattern used by tasks page**

```bash
grep -n "getAccessToken\|router.push.*login" frontend/src/app/java/tasks/[projectId]/page.tsx frontend/src/components/java/ProjectList.tsx
```

- [ ] **Step 2: Mirror the same pattern in dashboard-client**

At the top of `DashboardClient`, add:

```tsx
import { useRouter } from "next/navigation";
import { getAccessToken } from "@/lib/auth";

// inside DashboardClient, before any query:
useEffect(() => {
  if (!getAccessToken()) {
    router.push("/login");
  }
}, [router]);
```

If the tasks page uses a different auth pattern (e.g., a shared `<RequireAuth>` wrapper), use that instead — do not introduce a new pattern.

- [ ] **Step 3: Type-check**

```bash
cd frontend && npx tsc --noEmit
```

- [ ] **Step 4: Commit**

```bash
git add frontend/src/app/java/dashboard/dashboard-client.tsx
git commit -m "feat(frontend): require auth on /java/dashboard"
```

---

## Task 13: Update `/java` description page

**Files:**
- Modify: `frontend/src/app/java/page.tsx`

- [ ] **Step 1: Add analytics paragraph under the Task Management System section**

Insert after the existing tech stack `<ul>` (before the Architecture section):

```tsx
<h3 className="mt-6 text-lg font-medium">Analytics & Database Optimization</h3>
<p className="mt-2 text-muted-foreground leading-relaxed">
  The task-service exposes an analytics query layer backed by Flyway
  migrations, compound and partial indexes, Redis caching, HikariCP tuning,
  and MongoDB aggregation pipelines in the activity-service. The GraphQL
  gateway composes these into a single <code>projectHealth</code> query. See
  the{" "}
  <Link
    href="/docs/adr/java-task-management/07_analytics_and_optimization.md"
    className="underline"
  >
    ADR
  </Link>{" "}
  for the full rationale.
</p>
```

(If the ADR link doesn't route — the file lives in the repo, not a public route — drop the link or point it at a GitHub URL Kyle provides. Leave the prose.)

- [ ] **Step 2: Add a second CTA button next to "Open Task Manager"**

Replace the existing CTA section with:

```tsx
<section className="mt-12 flex flex-wrap gap-3">
  <Link
    href="/java/tasks"
    className="inline-flex items-center gap-2 rounded-lg bg-primary px-6 py-3 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
  >
    Open Task Manager &rarr;
  </Link>
  <Link
    href="/java/dashboard"
    className="inline-flex items-center gap-2 rounded-lg border px-6 py-3 text-sm font-medium hover:bg-accent transition-colors"
  >
    View Analytics Dashboard &rarr;
  </Link>
</section>
```

- [ ] **Step 3: Type-check + lint**

```bash
cd frontend && npx tsc --noEmit && npm run lint
```

- [ ] **Step 4: Commit**

```bash
git add frontend/src/app/java/page.tsx
git commit -m "feat(frontend): add analytics dashboard CTA and description to /java"
```

---

## Task 14: Playwright E2E for dashboard (mocked)

**Files:**
- Create: `frontend/e2e/java-dashboard.spec.ts`

Look at an existing mocked Playwright spec (e.g., `frontend/e2e/java-tasks.spec.ts` if present, otherwise any `*.spec.ts` in `frontend/e2e/`) to see the exact mocking pattern used in this repo — reuse it.

- [ ] **Step 1: Write the spec**

```typescript
// frontend/e2e/java-dashboard.spec.ts
import { test, expect } from "@playwright/test";

// Match the repo's existing mocked-graphql pattern. This block intercepts
// the GraphQL endpoint and returns canned responses for MyProjects and
// ProjectHealth. If the existing specs use a helper (e.g. mockGraphql), use
// that helper instead of rolling your own route handler.

test.describe("/java/dashboard", () => {
  test.beforeEach(async ({ page }) => {
    await page.route("**/graphql", async (route) => {
      const req = route.request();
      const body = req.postDataJSON() as { operationName?: string };
      if (body.operationName === "MyProjects") {
        return route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            data: { myProjects: [{ id: "p1", name: "Alpha" }, { id: "p2", name: "Beta" }] },
          }),
        });
      }
      if (body.operationName === "ProjectHealth") {
        return route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            data: {
              projectHealth: {
                stats: {
                  taskCountByStatus: { todo: 5, inProgress: 3, done: 12 },
                  taskCountByPriority: { low: 4, medium: 8, high: 8 },
                  overdueCount: 2,
                  avgCompletionTimeHours: 48.5,
                  memberWorkload: [
                    { userId: "u1", name: "Kyle", assignedCount: 4, completedCount: 7 },
                  ],
                },
                velocity: {
                  weeklyThroughput: [
                    { week: "2026-W14", completed: 5, created: 8 },
                    { week: "2026-W13", completed: 3, created: 4 },
                  ],
                  avgLeadTimeHours: 36.2,
                  leadTimePercentiles: { p50: 24, p75: 48, p95: 120 },
                },
                activity: {
                  totalEvents: 142,
                  eventCountByType: [
                    { eventType: "task.created", count: 20 },
                    { eventType: "task.status_changed", count: 85 },
                  ],
                  commentCount: 24,
                  activeContributors: 5,
                  weeklyActivity: [
                    { week: "2026-W14", events: 32, comments: 6 },
                    { week: "2026-W13", events: 28, comments: 4 },
                  ],
                },
              },
            },
          }),
        });
      }
      return route.continue();
    });

    // Set a fake access token so the auth guard allows entry.
    await page.addInitScript(() => {
      window.localStorage.setItem("accessToken", "fake-test-token");
    });
  });

  test("renders all dashboard sections", async ({ page }) => {
    await page.goto("/java/dashboard");
    await expect(page.getByRole("heading", { name: "Project Analytics" })).toBeVisible();
    await expect(page.getByText("By Status")).toBeVisible();
    await expect(page.getByText("By Priority")).toBeVisible();
    await expect(page.getByText("Weekly Throughput")).toBeVisible();
    await expect(page.getByText("Member Workload")).toBeVisible();
    await expect(page.getByText("Events by Type")).toBeVisible();
    await expect(page.getByText("Weekly Activity")).toBeVisible();
  });

  test("switching project updates URL", async ({ page }) => {
    await page.goto("/java/dashboard?projectId=p1");
    await page.getByRole("combobox", { name: /select project/i }).selectOption("p2");
    await expect(page).toHaveURL(/projectId=p2/);
  });
});
```

If the existing Playwright config expects a different token storage key (e.g. cookie, sessionStorage key, `auth_token`), inspect `lib/auth.ts` and adjust the `addInitScript` accordingly before running.

- [ ] **Step 2: Run mocked E2E suite**

```bash
cd frontend && npx playwright test e2e/java-dashboard.spec.ts
```
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add frontend/e2e/java-dashboard.spec.ts
git commit -m "test(frontend): add /java/dashboard mocked e2e spec"
```

---

## Task 15: Final preflight + sweep

- [ ] **Step 1: Run frontend preflight**

```bash
make preflight-frontend
```
Fix any ruff/tsc/lint/build failures before proceeding.

- [ ] **Step 2: Run E2E preflight**

```bash
make preflight-e2e
```

- [ ] **Step 3: Confirm branch status**

```bash
git status && git log --oneline -20
```

- [ ] **Step 4: Hand off to Kyle**

Report the branch name (`java-analytics-frontend`) and commit count. Kyle handles `git push` and PR/merge.

---

## Self-Review Notes

- Spec coverage: `/java/dashboard` page ✓ (tasks 11-12), analytics strip ✓ (tasks 9-10), `/java` CTA + paragraph ✓ (task 13), all four dashboard sections ✓ (tasks 5-8), single `projectHealth` query reuse ✓ (task 2), E2E ✓ (task 14).
- Placeholders: none.
- Type consistency: `ProjectHealthData` / `ProjectHealth` / `ProjectStats` / `VelocityMetrics` / `ActivityStats` defined in Task 2 and referenced consistently afterward.
- Out-of-scope items from spec (backend changes, new queries, time-range pickers) not present — good.
