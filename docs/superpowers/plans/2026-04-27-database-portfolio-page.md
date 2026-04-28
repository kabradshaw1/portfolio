# `/database` Portfolio Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a recruiter-facing `/database` portfolio page that surfaces the existing PostgreSQL work (query optimization, schema design, migration safety, reliability) under a single discoverable URL, with stub tabs reserving IA for NoSQL and Vector content later.

**Architecture:** Next.js App Router page at `frontend/src/app/database/page.tsx` with a three-tab state machine that exactly mirrors `/go` page tab pattern. The PostgreSQL tab is a two-column layout (pillar sections + sticky table-of-contents) using a reusable `PillarSection` component and an `IntersectionObserver`-driven `StickyToc`. NoSQL and Vector tabs are short stub components that point users at `/java` and `/ai` respectively. The homepage gets a new card slotted between `/cicd` and `/security`.

**Tech Stack:**
- Next.js (App Router) + React + TypeScript — match the existing repo conventions
- Tailwind CSS — match existing classnames (`text-muted-foreground`, `border-foreground/10`, etc.)
- shadcn/ui primitives already in repo: `Card`, `CardHeader`, `CardTitle`, `CardDescription`, `CardContent` (`frontend/src/components/ui/card.tsx`)
- Playwright (`frontend/e2e/mocked/` directory, `npm run e2e`) — only test framework wired up for the frontend
- Native browser `IntersectionObserver` for the scroll-spy TOC; no extra deps

---

## Pre-flight notes for the executing engineer

- **Read `frontend/AGENTS.md`.** The repo says "This is NOT the Next.js you know — APIs, conventions, and file structure may all differ from your training data." Use the existing `/go` page (`frontend/src/app/go/page.tsx`) as the canonical reference for component shape and tab state, and read `node_modules/next/dist/docs/` if you need any Next API beyond what `/go` already uses.
- **No new deps.** Everything in this plan uses existing components, Tailwind classes, and the standard browser API (`IntersectionObserver`).
- **Test strategy.** Frontend tests are Playwright-only (`frontend/e2e/mocked/*.spec.ts`). There is no Vitest / Jest. Component-level "tests" are Playwright assertions on rendered DOM. Run with `npm run e2e -- <spec.ts>` from `frontend/`.
- **Dev server.** `cd frontend && npm run dev` serves on `http://localhost:3000`. Playwright auto-starts it via `webServer` config when not already running.
- **Preflight.** Frontend changes require `make preflight-frontend` and `make preflight-e2e` per `CLAUDE.md`. Run them before pushing.
- **Vercel-bake gotcha.** This page does NOT add any `NEXT_PUBLIC_*` env vars, so the Vercel localhost-fallback rule from `CLAUDE.md` doesn't apply. If you find yourself wanting one, stop and reconsider — none of the content on this page is dynamic.

---

## File Structure

**New files:**

```
frontend/src/app/database/
├── page.tsx                            # Three-tab page, mirrors /go pattern
└── error.tsx                           # Standard error boundary

frontend/src/components/database/
├── tabs.ts                             # Tab union type + tab labels
├── PillarSection.tsx                   # Reusable: header + narrative + bullets + ADR links
├── StickyToc.tsx                       # IntersectionObserver active-section tracker
├── PostgresTab.tsx                     # Two-column layout w/ four pillars + sticky TOC
├── NoSqlTab.tsx                        # Stub component → /java
└── VectorTab.tsx                       # Stub component → /ai

frontend/e2e/mocked/
└── database-page.spec.ts               # Playwright e2e for the new page
```

**Modified files:**

```
frontend/src/app/page.tsx               # Add Database Engineering card between /cicd and /security
```

**Decomposition rationale:**

- `PillarSection` and `StickyToc` are isolated components with narrow inputs (props arrays). They're easy to reason about independently and trivial to reuse if a future pillar lands on a different page.
- `tabs.ts` holds the single source of truth for tab keys and labels — same shape `/go` uses inline today, but extracted because three different files need to reference the labels (page.tsx, e2e tests, future stubs).
- `PostgresTab` owns the four-pillar narrative and the wiring of `PillarSection` + `StickyToc`. Splitting each pillar into its own file would be over-engineering: each pillar is mostly a data array, not behavior.
- Stubs are full files (not inline JSX in `page.tsx`) so swapping them for real content later is a single-file change.

---

## Task 1: `/database` route skeleton with three tabs

**Files:**
- Create: `frontend/src/app/database/page.tsx`
- Create: `frontend/src/app/database/error.tsx`
- Create: `frontend/src/components/database/tabs.ts`
- Create: `frontend/src/components/database/PostgresTab.tsx` (placeholder)
- Create: `frontend/src/components/database/NoSqlTab.tsx` (placeholder)
- Create: `frontend/src/components/database/VectorTab.tsx` (placeholder)
- Create: `frontend/e2e/mocked/database-page.spec.ts`

This task ships the page skeleton and a passing-and-failing test pair: the page loads, all three tabs are present, switching tabs swaps the visible content. Pillar content lands in later tasks.

- [ ] **Step 1: Create the tabs data module**

`frontend/src/components/database/tabs.ts`:

```ts
export type DatabaseTab = "postgres" | "nosql" | "vector";

export const databaseTabs: { key: DatabaseTab; label: string }[] = [
  { key: "postgres", label: "PostgreSQL" },
  { key: "nosql", label: "NoSQL" },
  { key: "vector", label: "Vector" },
];
```

- [ ] **Step 2: Create placeholder tab components**

`frontend/src/components/database/PostgresTab.tsx`:

```tsx
export function PostgresTab() {
  return (
    <div data-testid="postgres-tab">
      <p className="text-muted-foreground">PostgreSQL content — coming in Task 5.</p>
    </div>
  );
}
```

`frontend/src/components/database/NoSqlTab.tsx`:

```tsx
export function NoSqlTab() {
  return (
    <div data-testid="nosql-tab">
      <p className="text-muted-foreground">NoSQL content — coming in Task 6.</p>
    </div>
  );
}
```

`frontend/src/components/database/VectorTab.tsx`:

```tsx
export function VectorTab() {
  return (
    <div data-testid="vector-tab">
      <p className="text-muted-foreground">Vector content — coming in Task 6.</p>
    </div>
  );
}
```

- [ ] **Step 3: Create the page (mirrors `/go` page tab pattern exactly)**

`frontend/src/app/database/page.tsx`:

```tsx
"use client";

import { useState } from "react";
import { databaseTabs, type DatabaseTab } from "@/components/database/tabs";
import { PostgresTab } from "@/components/database/PostgresTab";
import { NoSqlTab } from "@/components/database/NoSqlTab";
import { VectorTab } from "@/components/database/VectorTab";

export default function DatabasePage() {
  const [activeTab, setActiveTab] = useState<DatabaseTab>("postgres");

  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <h1 className="mt-8 text-3xl font-bold">Database Engineering</h1>

      {/* Bio Section */}
      <section className="mt-8">
        <p className="text-muted-foreground leading-relaxed">
          Production-grade PostgreSQL is one of the load-bearing skills behind
          this portfolio: real-database benchmarks, range partitioning with
          materialized views, a custom AST-based migration linter, and an
          operational track with backups and recovery runbooks. MongoDB and
          Qdrant are also in use elsewhere in the portfolio — dedicated tabs
          for each are coming.
        </p>
      </section>

      {/* Project Section with Tabs */}
      <section className="mt-12">
        <h2 className="text-2xl font-semibold">Database Tracks</h2>

        {/* Tab Bar */}
        <div className="mt-4 flex gap-0 border-b border-foreground/10">
          {databaseTabs.map((tab) => (
            <button
              key={tab.key}
              onClick={() => setActiveTab(tab.key)}
              className={`px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px ${
                activeTab === tab.key
                  ? "border-primary text-foreground"
                  : "border-transparent text-muted-foreground hover:text-foreground"
              }`}
            >
              {tab.label}
            </button>
          ))}
        </div>

        {/* Tab Content */}
        <div className="mt-8">
          {activeTab === "postgres" && <PostgresTab />}
          {activeTab === "nosql" && <NoSqlTab />}
          {activeTab === "vector" && <VectorTab />}
        </div>
      </section>
    </div>
  );
}
```

- [ ] **Step 4: Create the standard error boundary**

`frontend/src/app/database/error.tsx`:

```tsx
"use client";

export default function DatabaseError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <h1 className="text-2xl font-bold">Something went wrong on /database</h1>
      <p className="mt-4 text-muted-foreground">{error.message}</p>
      <button
        onClick={reset}
        className="mt-6 rounded-lg border px-4 py-2 text-sm hover:bg-accent transition-colors"
      >
        Try again
      </button>
    </div>
  );
}
```

- [ ] **Step 5: Write the failing Playwright test**

`frontend/e2e/mocked/database-page.spec.ts`:

```ts
import { test, expect } from "./fixtures";

test.describe("/database page", () => {
  test("renders the page heading", async ({ page }) => {
    await page.goto("/database");
    await expect(page.getByRole("heading", { name: "Database Engineering", level: 1 })).toBeVisible();
  });

  test("renders all three tab labels", async ({ page }) => {
    await page.goto("/database");
    await expect(page.getByRole("button", { name: "PostgreSQL", exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "NoSQL", exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "Vector", exact: true })).toBeVisible();
  });

  test("PostgreSQL tab is active by default", async ({ page }) => {
    await page.goto("/database");
    await expect(page.getByTestId("postgres-tab")).toBeVisible();
    await expect(page.getByTestId("nosql-tab")).not.toBeVisible();
    await expect(page.getByTestId("vector-tab")).not.toBeVisible();
  });

  test("clicking NoSQL switches to the NoSQL tab", async ({ page }) => {
    await page.goto("/database");
    await page.getByRole("button", { name: "NoSQL", exact: true }).click();
    await expect(page.getByTestId("nosql-tab")).toBeVisible();
    await expect(page.getByTestId("postgres-tab")).not.toBeVisible();
  });

  test("clicking Vector switches to the Vector tab", async ({ page }) => {
    await page.goto("/database");
    await page.getByRole("button", { name: "Vector", exact: true }).click();
    await expect(page.getByTestId("vector-tab")).toBeVisible();
  });
});
```

- [ ] **Step 6: Run the e2e test, verify it passes**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent/feat-database-page/frontend
npm run e2e -- database-page.spec.ts
```

Expected: 5 tests pass. (If the dev server isn't running, Playwright starts it automatically per `playwright.config.ts`.)

- [ ] **Step 7: Commit**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent/feat-database-page
git add frontend/src/app/database frontend/src/components/database frontend/e2e/mocked/database-page.spec.ts
git commit -m "feat(database): scaffold /database page with three-tab skeleton"
```

---

## Task 2: NoSQL and Vector stub content

**Files:**
- Modify: `frontend/src/components/database/NoSqlTab.tsx`
- Modify: `frontend/src/components/database/VectorTab.tsx`
- Modify: `frontend/e2e/mocked/database-page.spec.ts`

The stubs do real work: they tell recruiters that NoSQL and Vector experience exists in the portfolio and point at where the running code lives.

- [ ] **Step 1: Replace `NoSqlTab` placeholder with stub content**

`frontend/src/components/database/NoSqlTab.tsx`:

```tsx
import Link from "next/link";

export function NoSqlTab() {
  return (
    <div data-testid="nosql-tab" className="space-y-6">
      <p className="text-muted-foreground leading-relaxed">
        MongoDB powers the activity feed and analytics aggregations in the
        Java task-management portfolio — document-shaped activity events,
        time-bucketed aggregations, and a Redis read-cache layered on top.
        A dedicated NoSQL section is on the way; for now, the running code
        and its supporting docs live on the Java page.
      </p>
      <div>
        <Link
          href="/java"
          className="inline-flex items-center gap-2 rounded-lg border px-6 py-3 text-sm font-medium hover:bg-accent transition-colors"
        >
          View MongoDB usage in /java &rarr;
        </Link>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Replace `VectorTab` placeholder with stub content**

`frontend/src/components/database/VectorTab.tsx`:

```tsx
import Link from "next/link";

export function VectorTab() {
  return (
    <div data-testid="vector-tab" className="space-y-6">
      <p className="text-muted-foreground leading-relaxed">
        Qdrant backs the retrieval layer behind the Document Q&amp;A
        assistant and the code-aware Debug Assistant — chunked PDF
        embeddings, code-aware splitting, and approximate-nearest-neighbor
        search feeding RAG prompts. A dedicated vector-database section is
        on the way; for now, the running code and its supporting docs live
        on the AI page.
      </p>
      <div>
        <Link
          href="/ai"
          className="inline-flex items-center gap-2 rounded-lg border px-6 py-3 text-sm font-medium hover:bg-accent transition-colors"
        >
          View vector DB usage in /ai &rarr;
        </Link>
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Add stub-content assertions to the existing spec**

Append to `frontend/e2e/mocked/database-page.spec.ts` inside the same `test.describe` block:

```ts
  test("NoSQL stub points to /java", async ({ page }) => {
    await page.goto("/database");
    await page.getByRole("button", { name: "NoSQL", exact: true }).click();
    await expect(page.getByText("MongoDB powers the activity feed", { exact: false })).toBeVisible();
    const link = page.getByRole("link", { name: /View MongoDB usage in \/java/ });
    await expect(link).toHaveAttribute("href", "/java");
  });

  test("Vector stub points to /ai", async ({ page }) => {
    await page.goto("/database");
    await page.getByRole("button", { name: "Vector", exact: true }).click();
    await expect(page.getByText("Qdrant backs the retrieval layer", { exact: false })).toBeVisible();
    const link = page.getByRole("link", { name: /View vector DB usage in \/ai/ });
    await expect(link).toHaveAttribute("href", "/ai");
  });
```

- [ ] **Step 4: Run the spec and verify all 7 tests pass**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent/feat-database-page/frontend
npm run e2e -- database-page.spec.ts
```

Expected: 7 tests pass.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/database/NoSqlTab.tsx frontend/src/components/database/VectorTab.tsx frontend/e2e/mocked/database-page.spec.ts
git commit -m "feat(database): NoSQL and Vector stub content with cross-links"
```

---

## Task 3: `PillarSection` reusable component

**Files:**
- Create: `frontend/src/components/database/PillarSection.tsx`

`PillarSection` is the unit the PostgreSQL tab repeats four times. Defining it before the tab keeps each pillar's data declarative.

- [ ] **Step 1: Create `PillarSection.tsx`**

```tsx
import type { ReactNode } from "react";

export type PillarLink = {
  label: string;
  href: string;
};

export type PillarSectionProps = {
  /** Anchor id, used by the sticky TOC and as the `<h2>`'s id. */
  id: string;
  /** Section heading, e.g. "Query Optimization & Benchmarking". */
  title: string;
  /** Two- to four-sentence narrative paragraph(s). */
  narrative: ReactNode;
  /** 4–6 concrete-claim bullets. Strings or ReactNodes. */
  bullets: ReactNode[];
  /** ADR / runbook links rendered as a muted button row. */
  links: PillarLink[];
};

export function PillarSection({ id, title, narrative, bullets, links }: PillarSectionProps) {
  return (
    <section id={id} className="scroll-mt-24" data-testid={`pillar-${id}`}>
      <h2 className="text-2xl font-semibold">{title}</h2>
      <div className="mt-4 text-muted-foreground leading-relaxed space-y-3">{narrative}</div>
      <ul className="mt-4 space-y-2 list-disc pl-5 text-sm text-muted-foreground">
        {bullets.map((bullet, i) => (
          <li key={i} className="leading-relaxed">{bullet}</li>
        ))}
      </ul>
      {links.length > 0 && (
        <div className="mt-5 flex flex-wrap gap-3">
          {links.map((link) => (
            <a
              key={link.href}
              href={link.href}
              className="inline-flex items-center gap-2 rounded-lg border px-4 py-2 text-sm font-medium hover:bg-accent transition-colors"
              target={link.href.startsWith("http") ? "_blank" : undefined}
              rel={link.href.startsWith("http") ? "noopener noreferrer" : undefined}
            >
              {link.label} &rarr;
            </a>
          ))}
        </div>
      )}
    </section>
  );
}
```

- [ ] **Step 2: Type-check the component by running the existing build**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent/feat-database-page/frontend
npm run build 2>&1 | tail -20
```

Expected: build succeeds. (`PillarSection` isn't imported anywhere yet, so it just has to compile cleanly.)

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/database/PillarSection.tsx
git commit -m "feat(database): add PillarSection reusable component"
```

---

## Task 4: `StickyToc` with IntersectionObserver

**Files:**
- Create: `frontend/src/components/database/StickyToc.tsx`

The sticky TOC tracks which pillar is currently in view and highlights the corresponding entry. On mobile (< md), it collapses to a horizontal chip row pinned at the top of the tab content. On desktop, it sits as a sticky right-column.

- [ ] **Step 1: Create `StickyToc.tsx`**

```tsx
"use client";

import { useEffect, useState } from "react";

export type StickyTocItem = {
  id: string;
  label: string;
};

export type StickyTocProps = {
  items: StickyTocItem[];
};

export function StickyToc({ items }: StickyTocProps) {
  const [activeId, setActiveId] = useState<string>(items[0]?.id ?? "");

  useEffect(() => {
    if (typeof window === "undefined" || items.length === 0) return;

    const sections = items
      .map((item) => document.getElementById(item.id))
      .filter((el): el is HTMLElement => el !== null);

    if (sections.length === 0) return;

    // Pick the section closest to the top of the viewport that is still
    // intersecting. The 0.25 threshold avoids flicker when sections are
    // long enough that two are partially in view.
    const observer = new IntersectionObserver(
      (entries) => {
        const visible = entries
          .filter((entry) => entry.isIntersecting)
          .sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top);
        if (visible.length > 0) {
          setActiveId(visible[0].target.id);
        }
      },
      {
        // Trigger when a section's top crosses ~25% of the viewport.
        rootMargin: "-25% 0px -65% 0px",
        threshold: 0,
      },
    );

    sections.forEach((section) => observer.observe(section));
    return () => observer.disconnect();
  }, [items]);

  return (
    <>
      {/* Mobile: horizontal chip row */}
      <nav
        aria-label="Section navigation"
        className="md:hidden -mx-6 mb-6 overflow-x-auto px-6"
        data-testid="sticky-toc-mobile"
      >
        <ul className="flex gap-2 whitespace-nowrap">
          {items.map((item) => (
            <li key={item.id}>
              <a
                href={`#${item.id}`}
                className={`inline-flex items-center rounded-full border px-3 py-1 text-xs font-medium transition-colors ${
                  activeId === item.id
                    ? "border-primary text-foreground bg-accent"
                    : "border-foreground/10 text-muted-foreground hover:text-foreground"
                }`}
              >
                {item.label}
              </a>
            </li>
          ))}
        </ul>
      </nav>

      {/* Desktop: sticky right-column TOC */}
      <nav
        aria-label="Section navigation"
        className="hidden md:block sticky top-24 self-start"
        data-testid="sticky-toc-desktop"
      >
        <p className="text-xs uppercase tracking-wide text-muted-foreground mb-3">On this page</p>
        <ul className="space-y-2 border-l border-foreground/10">
          {items.map((item) => (
            <li key={item.id}>
              <a
                href={`#${item.id}`}
                className={`block border-l-2 pl-3 -ml-px text-sm transition-colors ${
                  activeId === item.id
                    ? "border-primary text-foreground"
                    : "border-transparent text-muted-foreground hover:text-foreground"
                }`}
              >
                {item.label}
              </a>
            </li>
          ))}
        </ul>
      </nav>
    </>
  );
}
```

- [ ] **Step 2: Type-check the component**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent/feat-database-page/frontend
npm run build 2>&1 | tail -10
```

Expected: build succeeds.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/database/StickyToc.tsx
git commit -m "feat(database): add StickyToc with IntersectionObserver scroll-spy"
```

---

## Task 5: PostgreSQL tab — four pillars + sticky TOC

**Files:**
- Modify: `frontend/src/components/database/PostgresTab.tsx`
- Modify: `frontend/e2e/mocked/database-page.spec.ts`

Replace the placeholder `PostgresTab` with the real two-column layout: pillar sections in the main column, `StickyToc` in the right column on desktop.

- [ ] **Step 1: Replace `PostgresTab.tsx`**

```tsx
import { PillarSection } from "@/components/database/PillarSection";
import { StickyToc, type StickyTocItem } from "@/components/database/StickyToc";

const tocItems: StickyTocItem[] = [
  { id: "optimization", label: "Query Optimization" },
  { id: "schema", label: "Schema Design" },
  { id: "migrations", label: "Migration Safety" },
  { id: "reliability", label: "Reliability & Recovery" },
];

export function PostgresTab() {
  return (
    <div data-testid="postgres-tab" className="md:grid md:grid-cols-[1fr_220px] md:gap-10">
      <div className="space-y-16 min-w-0">
        <PillarSection
          id="optimization"
          title="Query Optimization & Benchmarking"
          narrative={
            <>
              <p>
                Functional ORM-style code is a starting point, not the finish line.
                The Go services were re-benchmarked against a real PostgreSQL 16
                container and rewritten where the data showed it.
              </p>
              <p>
                <code>testcontainers-go</code> made the benchmarks runnable on any
                Docker-equipped machine; results were captured to{" "}
                <code>go/benchdata/</code> for interview-ready evidence.
              </p>
            </>
          }
          bullets={[
            <>Real-DB benchmarks via <code>testcontainers-go</code> against PostgreSQL 16; results in <code>go/benchdata/baseline-results.txt</code> and <code>optimized-results.txt</code></>,
            <><strong>Batch INSERT for order items: 3.5× speedup on 20-item orders</strong> (4.5ms → 1.3ms), single round trip instead of N</>,
            <><code>COUNT(*) OVER()</code> window function replaces COUNT-then-data double query</>,
            <>CTE-based atomic conflict resolution in cart updates (<code>WITH updated AS (UPDATE … RETURNING) SELECT EXISTS(...)</code>)</>,
            <>pgx prepared-statement cache enabled (<code>QueryExecModeCacheDescribe</code>)</>,
            <>Targeted indexes: <code>idx_orders_saga_step</code>, partial <code>idx_products_low_stock WHERE stock &lt; 10</code>, composite <code>idx_cart_items_user_reserved</code></>,
            <>Typed pgx error checks: <code>errors.Is(err, pgx.ErrNoRows)</code>, <code>errors.As(err, &amp;pgconn.PgError)</code> for code <code>23505</code></>,
          ]}
          links={[
            { label: "Read the ADR", href: "https://github.com/kabradshaw1/portfolio/blob/main/docs/adr/ecommerce/go-database-optimization.md" },
          ]}
        />

        <PillarSection
          id="schema"
          title="Schema Design — Partitioning & Materialized Views"
          narrative={
            <>
              <p>
                Reporting workloads on a monotonically growing <code>orders</code>{" "}
                table forced a schema-design pass. Range partitioning by{" "}
                <code>created_at</code> prunes scan scope; three materialized views
                give constant-time reads for dashboard queries; CTE + window
                functions express the rolling-average business logic without
                application-side aggregation.
              </p>
            </>
          }
          bullets={[
            <>Range partitioning on <code>orders.created_at</code> (monthly), 18 months pre-provisioned with a default catch-all partition</>,
            <>Background goroutine creates partitions 3 months ahead daily; idempotent <code>CREATE TABLE IF NOT EXISTS</code></>,
            <>Three materialized views (<code>mv_daily_revenue</code>, <code>mv_product_performance</code>, <code>mv_customer_summary</code>) refreshed <code>CONCURRENTLY</code> on a 15-min cadence</>,
            <>Unique indexes per MV to support <code>REFRESH CONCURRENTLY</code></>,
            <>CTE-driven reporting with <code>SUM(...) OVER (ORDER BY day ROWS BETWEEN 6 PRECEDING AND CURRENT ROW)</code> for rolling 7/30-day averages</>,
            <><code>DENSE_RANK()</code> for tie-aware top-N (turnover, top customers)</>,
            <>Composite primary key trade-off documented (<code>(id, created_at)</code> removes single-column FK target — referential integrity moves to the saga)</>,
          ]}
          links={[
            { label: "Read the ADR", href: "https://github.com/kabradshaw1/portfolio/blob/main/docs/adr/ecommerce/go-sql-optimization-reporting.md" },
          ]}
        />

        <PillarSection
          id="migrations"
          title="Migration Safety — migration-lint"
          narrative={
            <>
              <p>
                <code>golang-migrate</code> catches syntactic errors when the
                migration runs against Docker; it doesn&apos;t catch operationally
                unsafe DDL that&apos;s syntactically valid. A custom Go linter
                (<code>migration-lint</code>) walks each <code>.up.sql</code> AST
                via <code>libpg_query</code> and flags eight common foot-guns at
                lint time, before any container starts.
              </p>
              <p>
                Each rule pairs with a recipe in a checked-in safe-migration
                runbook.
              </p>
            </>
          }
          bullets={[
            <>Custom Go CLI built on <code>pganalyze/pg_query_go</code> (CGO wrapper around <code>libpg_query</code>, the upstream PG parser)</>,
            <>Eight rules: CREATE INDEX without CONCURRENTLY (MIG001), NOT NULL ADD COLUMN with volatile default (MIG002), table-rewrite ALTER COLUMN TYPE (MIG003), CHECK without NOT VALID (MIG004), DROP COLUMN (MIG005), RENAME COLUMN (MIG006), CONCURRENTLY mixed with other DDL (MIG007), LOCK TABLE (MIG008)</>,
            <>Per-statement opt-out: <code>{`-- migration-lint: ignore=MIGNNN reason="..."`}</code> with mandatory <code>reason=&quot;…&quot;</code></>,
            <>Wired into <code>make preflight-go-migrations</code> and the CI matrix as a hard prerequisite to the runtime migration pipeline</>,
            <>Companion 8-recipe runbook</>,
            <>Worked example: <code>CREATE INDEX CONCURRENTLY</code> in its own migration file (<code>go/product-service/migrations/005_add_product_search_index.up.sql</code>)</>,
          ]}
          links={[
            { label: "Read the ADR", href: "https://github.com/kabradshaw1/portfolio/blob/main/docs/adr/database/migration-lint.md" },
            { label: "Read the runbook", href: "https://github.com/kabradshaw1/portfolio/blob/main/docs/runbooks/postgres-migrations.md" },
          ]}
        />

        <PillarSection
          id="reliability"
          title="Reliability & Recovery"
          narrative={
            <>
              <p>
                Production-grade SQL isn&apos;t only about queries. Postgres needs
                scheduled backups, monitored health, and a written runbook for
                the day someone has to restore from one. The portfolio&apos;s
                Postgres deployment ships with all three.
              </p>
            </>
          }
          bullets={[
            <>Automated <code>pg_dump</code> CronJob writing to a persistent volume on the Minikube node; retention policy in the manifest</>,
            <>Pod Disruption Budget on the StatefulSet (<code>maxUnavailable: 1</code>) so node drains don&apos;t block on a single-replica DB</>,
            <><code>postgres_exporter</code> sidecar feeding Prometheus; Grafana dashboard surfaces connection counts, replication lag, table sizes, and slow queries</>,
            <>Alert rules: backup-job failure, replication-lag-too-high, disk-full, long-running-transaction</>,
            <>Written recovery runbook: step-by-step from <code>pg_dump</code> artifact to a restored database</>,
          ]}
          links={[
            { label: "Read the runbook", href: "https://github.com/kabradshaw1/portfolio/blob/main/docs/runbooks/postgres-recovery.md" },
          ]}
        />
      </div>

      {/* TOC column */}
      <aside className="hidden md:block">
        <StickyToc items={tocItems} />
      </aside>

      {/* Mobile TOC (above content) — rendered inside StickyToc with md:hidden,
          but it lives at the top of the tab DOM so it appears first on mobile. */}
      <div className="md:hidden order-first">
        <StickyToc items={tocItems} />
      </div>
    </div>
  );
}
```

> Note on the mobile TOC: the `StickyToc` component renders both the mobile chip row and the desktop sticky list, gating each with `md:hidden` / `hidden md:block`. We render `<StickyToc>` twice in this layout — once inside the desktop `<aside>` (which is `hidden md:block`) and once at the top in a `md:hidden` wrapper for mobile. Each render only displays the variant that matches the current breakpoint, so the user sees one TOC at a time. Keeping both renders in `PostgresTab` (rather than relying on a single `StickyToc` placement) makes the desktop sticky positioning work without grid-row gymnastics.

- [ ] **Step 2: Add e2e assertions for all four pillars and TOC**

Append these tests to `frontend/e2e/mocked/database-page.spec.ts` inside the same `test.describe`:

```ts
  test("PostgreSQL tab renders all four pillar headings", async ({ page }) => {
    await page.goto("/database");
    await expect(page.getByRole("heading", { name: "Query Optimization & Benchmarking", level: 2 })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Schema Design — Partitioning & Materialized Views", level: 2 })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Migration Safety — migration-lint", level: 2 })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Reliability & Recovery", level: 2 })).toBeVisible();
  });

  test("each pillar has a stable anchor id", async ({ page }) => {
    await page.goto("/database");
    for (const id of ["optimization", "schema", "migrations", "reliability"]) {
      await expect(page.locator(`#${id}`)).toBeVisible();
    }
  });

  test("PostgreSQL tab includes recruiter keywords inline", async ({ page }) => {
    await page.goto("/database");
    // A few high-signal keywords spread across pillars — each is a recruiter ATS hit.
    await expect(page.getByText("PostgreSQL 16", { exact: false })).toBeVisible();
    await expect(page.getByText("Range partitioning", { exact: false })).toBeVisible();
    await expect(page.getByText("CREATE INDEX CONCURRENTLY", { exact: false }).first()).toBeVisible();
    await expect(page.getByText("postgres_exporter", { exact: false })).toBeVisible();
  });

  test("clicking a TOC link scrolls to the corresponding pillar", async ({ page }) => {
    await page.goto("/database");
    await page.locator('a[href="#migrations"]').first().click();
    // After the hash navigation the migrations heading should be in view.
    await expect(page.getByRole("heading", { name: "Migration Safety — migration-lint", level: 2 })).toBeInViewport();
  });
```

- [ ] **Step 3: Run the spec**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent/feat-database-page/frontend
npm run e2e -- database-page.spec.ts
```

Expected: 11 tests pass.

- [ ] **Step 4: Manually verify the layout in a browser**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent/feat-database-page/frontend
npm run dev
# Browse: http://localhost:3000/database
# Verify: page renders, all four pillars visible, sticky TOC tracks the active section as you scroll, mobile chip row appears at narrow viewports.
```

Stop the dev server (Ctrl-C) before continuing.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/database/PostgresTab.tsx frontend/e2e/mocked/database-page.spec.ts
git commit -m "feat(database): PostgreSQL tab with four pillars and sticky TOC"
```

---

## Task 6: Homepage card

**Files:**
- Modify: `frontend/src/app/page.tsx`
- Modify: `frontend/e2e/mocked/database-page.spec.ts`

The card slots between `/cicd` (around lines 112–131 of `page.tsx`) and `/security` (lines 132+) per the spec.

- [ ] **Step 1: Add the card to `frontend/src/app/page.tsx`**

Insert the new `<Link>` block immediately after the closing `</Link>` of the `/cicd` card and before the `<Link href="/security"…>` block. The exact insertion point in the current file is the line containing `</Link>` immediately followed by `<Link href="/security" className="block">` (around line 131–132).

The card to insert:

```tsx
          <Link href="/database" className="block">
            <Card className="hover:ring-foreground/20 transition-all">
              <CardHeader>
                <CardTitle>Database Engineering</CardTitle>
                <CardDescription>
                  Production PostgreSQL — optimization, partitioning, migration
                  safety, and reliability
                </CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-muted-foreground text-sm">
                  Real benchmarks against PostgreSQL 16, range partitioning with
                  materialized views, a custom AST-based migration linter, and
                  an operational track with backups and recovery runbooks.
                </p>
              </CardContent>
            </Card>
          </Link>
```

- [ ] **Step 2: Add a homepage assertion**

Append to `frontend/e2e/mocked/database-page.spec.ts` (still inside the same `test.describe`):

```ts
  test("homepage links to /database", async ({ page }) => {
    await page.goto("/");
    const link = page.getByRole("link", { name: /Database Engineering/ });
    await expect(link).toBeVisible();
    await expect(link).toHaveAttribute("href", "/database");
  });
```

- [ ] **Step 3: Run the spec**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent/feat-database-page/frontend
npm run e2e -- database-page.spec.ts
```

Expected: 12 tests pass.

- [ ] **Step 4: Verify the homepage looks right manually**

```bash
npm run dev
# http://localhost:3000 — confirm the Database card shows between CI/CD and Security, with hover ring matching the others.
```

Stop the dev server.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/app/page.tsx frontend/e2e/mocked/database-page.spec.ts
git commit -m "feat(home): add Database Engineering card linking to /database"
```

---

## Task 7: Final preflight, push, and PR

**Files:** none (verification + push)

- [ ] **Step 1: Run the frontend preflight gates**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent/feat-database-page
make preflight-frontend
```

Expected: tsc + eslint + `next build` pass clean.

- [ ] **Step 2: Run the full mocked-e2e suite (catches regressions in other pages)**

```bash
make preflight-e2e
```

Expected: every spec passes, including the 12 new `database-page.spec.ts` tests.

> If the e2e run is slow on Mac and you only changed the database page, you can run the new spec in isolation with `cd frontend && npm run e2e -- database-page.spec.ts`. The full sweep is what CI runs, though, so do it before pushing.

- [ ] **Step 3: Confirm the branch state**

```bash
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent/feat-database-page status
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent/feat-database-page log --oneline main..HEAD
```

You should see ~6 commits and a clean working tree.

- [ ] **Step 4: Push the branch**

```bash
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent/feat-database-page push -u origin agent/feat-database-page
```

- [ ] **Step 5: Open the PR to `qa`**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent/feat-database-page
gh pr create --base qa --title "feat: /database portfolio page (PostgreSQL focus, NoSQL/Vector stubs)" --body "$(cat <<'EOF'
## Summary

- New top-level `/database` page surfaces existing PostgreSQL work under a single recruiter-discoverable URL.
- Three-tab layout: PostgreSQL (full content), NoSQL (stub → /java), Vector (stub → /ai).
- PostgreSQL tab is single-scroll across four pillars (Query Optimization, Schema Design, Migration Safety, Reliability & Recovery), each with a narrative paragraph, 4–7 keyword-rich bullets, and links to the matching ADR / runbook.
- `IntersectionObserver`-driven sticky TOC on the right column (desktop) / chip row (mobile).
- Homepage gets a new "Database Engineering" card slotted between CI/CD and Security.

## Closes / refs

- Spec: `docs/superpowers/specs/2026-04-27-database-portfolio-page-design.md`
- Plan: `docs/superpowers/plans/2026-04-27-database-portfolio-page.md`
- Surfaces work from `docs/adr/ecommerce/go-database-optimization.md`, `docs/adr/ecommerce/go-sql-optimization-reporting.md`, `docs/adr/database/migration-lint.md`, and `docs/runbooks/postgres-recovery.md`.

## Test plan

- [x] Local: `npm run e2e -- database-page.spec.ts` — 12 tests pass
- [x] Local: `make preflight-frontend` — tsc + eslint + next build clean
- [x] Local: `make preflight-e2e` — full mocked suite passes
- [x] Manual: page renders correctly at `localhost:3000/database`, sticky TOC tracks active pillar on scroll, mobile chip TOC appears below the bio at narrow viewports
- [ ] CI: Frontend Checks job passes
- [ ] CI: e2e job passes

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 6: Notify Kyle and stop**

Per `CLAUDE.md` feature-branch flow: do NOT watch CI. Kyle will check results.

---

## Self-review checklist (executed by the plan author)

**Spec coverage map:**
- ✅ Top-level `/database` page → Task 1
- ✅ Three tabs (PostgreSQL / NoSQL / Vector) with shared tab pattern → Task 1
- ✅ Tab state local to page (no URL routing) → Task 1
- ✅ NoSQL stub → /java + Vector stub → /ai → Task 2
- ✅ `PillarSection` reusable component → Task 3
- ✅ `StickyToc` with IntersectionObserver, desktop + mobile variants → Task 4
- ✅ Four pillars (Optimization, Schema, Migrations, Reliability) with narratives + bullets + ADR links → Task 5
- ✅ Two-column layout w/ sticky TOC (desktop) + chip row (mobile) → Task 5
- ✅ Homepage card between /cicd and /security → Task 6
- ✅ Playwright e2e covering page load, tab switching, stub links, pillar headings, recruiter keywords, TOC scroll behavior, homepage card → Tasks 1, 2, 5, 6
- ✅ Out-of-scope (NoSQL/Vector full content, URL-synced tab state, charts, sandbox) → not implemented, matches spec

**Placeholder scan:** none. Each step has the actual code or command. Tasks that say "verify in browser" describe specifically what to check.

**Type / name consistency check:**
- `DatabaseTab` union and `databaseTabs` array defined in Task 1 (`tabs.ts`), imported unchanged by `page.tsx`.
- `PillarSection` props (`PillarSectionProps`, `PillarLink`) defined in Task 3, used unchanged in Task 5.
- `StickyToc` props (`StickyTocProps`, `StickyTocItem`) defined in Task 4, used unchanged in Task 5.
- `data-testid="postgres-tab" | "nosql-tab" | "vector-tab"` placeholder names from Task 1 carry through to the real components in Tasks 2 and 5 — Playwright assertions remain valid.
- Pillar anchor ids (`optimization`, `schema`, `migrations`, `reliability`) declared in Task 5 (`tocItems` and each `PillarSection.id`) and referenced in the e2e tests by exact match.
