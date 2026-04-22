# DLQ Admin Panel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a frontend admin panel at `/go/admin` to inspect and replay dead-lettered saga messages via the order-service admin endpoints.

**Architecture:** New Next.js page with a simple API client (no auth), table with expandable rows, and graceful handling of unreachable backend. No context providers — the page is self-contained with local state.

**Tech Stack:** Next.js (app router), TypeScript, Tailwind CSS, Playwright (mocked E2E)

---

## File Structure

| File | Action | Purpose |
|------|--------|---------|
| `frontend/src/lib/go-admin-api.ts` | Create | API client: fetchDLQMessages, replayDLQMessage |
| `frontend/src/components/go/DemoBanner.tsx` | Create | Reusable portfolio demo callout banner |
| `frontend/src/components/go/DLQMessageRow.tsx` | Create | Expandable table row with replay button |
| `frontend/src/app/go/admin/page.tsx` | Create | Admin page — data fetching, table, states |
| `frontend/src/components/go/GoSubHeader.tsx` | Modify | Add "Admin" nav link |
| `frontend/e2e/mocked/go-dlq-admin.spec.ts` | Create | Playwright mocked E2E test |

---

### Task 1: API Client

**Files:**
- Create: `frontend/src/lib/go-admin-api.ts`

- [ ] **Step 1: Create the API client with types and fetch functions**

```typescript
// frontend/src/lib/go-admin-api.ts

const GO_ORDER_URL =
  process.env.NEXT_PUBLIC_GO_ORDER_URL || "http://localhost:8092";

export interface DLQMessage {
  index: number;
  routing_key: string;
  exchange: string;
  timestamp: string;
  retry_count: number;
  headers: Record<string, unknown>;
  body: unknown;
}

export interface DLQListResponse {
  messages: DLQMessage[];
  count: number;
}

export interface DLQReplayResponse {
  replayed: DLQMessage;
}

export interface DLQResult<T> {
  data: T | null;
  error: string | null;
  connected: boolean;
}

export async function fetchDLQMessages(
  limit = 50,
): Promise<DLQResult<DLQListResponse>> {
  try {
    const res = await fetch(
      `${GO_ORDER_URL}/admin/dlq/messages?limit=${limit}`,
    );
    if (!res.ok) {
      return { data: null, error: `HTTP ${res.status}`, connected: true };
    }
    const data: DLQListResponse = await res.json();
    return { data, error: null, connected: true };
  } catch {
    return { data: null, error: null, connected: false };
  }
}

export async function replayDLQMessage(
  index: number,
): Promise<DLQResult<DLQReplayResponse>> {
  try {
    const res = await fetch(`${GO_ORDER_URL}/admin/dlq/replay`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ index }),
    });
    if (!res.ok) {
      const text = await res.text();
      return { data: null, error: text || `HTTP ${res.status}`, connected: true };
    }
    const data: DLQReplayResponse = await res.json();
    return { data, error: null, connected: true };
  } catch {
    return { data: null, error: null, connected: false };
  }
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit 2>&1 | head -20`
Expected: No errors related to `go-admin-api.ts`

- [ ] **Step 3: Commit**

```bash
git add frontend/src/lib/go-admin-api.ts
git commit -m "feat(frontend): add DLQ admin API client"
```

---

### Task 2: DemoBanner Component

**Files:**
- Create: `frontend/src/components/go/DemoBanner.tsx`

- [ ] **Step 1: Create the demo banner component**

```typescript
// frontend/src/components/go/DemoBanner.tsx
"use client";

export function DemoBanner() {
  return (
    <div className="mb-6 rounded-lg border border-yellow-500/30 bg-yellow-500/10 px-4 py-3">
      <p className="text-sm font-medium text-yellow-700 dark:text-yellow-400">
        Portfolio Demo
      </p>
      <p className="mt-1 text-sm text-yellow-600/80 dark:text-yellow-400/70">
        This panel demonstrates DLQ operational tooling for the checkout saga. In
        a production environment, this would be implemented as a CLI tool with
        role-based access control.
      </p>
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/components/go/DemoBanner.tsx
git commit -m "feat(frontend): add DemoBanner component"
```

---

### Task 3: DLQMessageRow Component

**Files:**
- Create: `frontend/src/components/go/DLQMessageRow.tsx`

- [ ] **Step 1: Create the expandable message row component**

```typescript
// frontend/src/components/go/DLQMessageRow.tsx
"use client";

import { useState } from "react";
import type { DLQMessage } from "@/lib/go-admin-api";
import { replayDLQMessage } from "@/lib/go-admin-api";

function timeAgo(timestamp: string): string {
  const seconds = Math.floor(
    (Date.now() - new Date(timestamp).getTime()) / 1000,
  );
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

interface DLQMessageRowProps {
  message: DLQMessage;
  onReplayed: () => void;
}

export function DLQMessageRow({ message, onReplayed }: DLQMessageRowProps) {
  const [expanded, setExpanded] = useState(false);
  const [replaying, setReplaying] = useState(false);
  const [replayResult, setReplayResult] = useState<
    "success" | "error" | null
  >(null);
  const [replayError, setReplayError] = useState<string | null>(null);

  async function handleReplay(e: React.MouseEvent) {
    e.stopPropagation();
    setReplaying(true);
    setReplayResult(null);
    setReplayError(null);

    const result = await replayDLQMessage(message.index);

    setReplaying(false);
    if (result.data) {
      setReplayResult("success");
      setTimeout(() => onReplayed(), 1000);
    } else {
      setReplayResult("error");
      setReplayError(result.error || "Replay failed");
    }
  }

  return (
    <>
      <tr
        className="cursor-pointer border-b transition-colors hover:bg-muted/50"
        onClick={() => setExpanded(!expanded)}
      >
        <td className="px-4 py-2 text-muted-foreground">#{message.index}</td>
        <td className="px-4 py-2 font-mono text-sm">{message.routing_key}</td>
        <td className="px-4 py-2 text-muted-foreground">
          {message.timestamp ? timeAgo(message.timestamp) : "—"}
        </td>
        <td className="px-4 py-2 text-center">{message.retry_count}</td>
        <td className="px-4 py-2 text-right">
          {replayResult === "success" ? (
            <span className="text-sm text-green-600 dark:text-green-400">
              Replayed
            </span>
          ) : (
            <button
              onClick={handleReplay}
              disabled={replaying}
              className="rounded bg-primary px-3 py-1 text-xs font-medium text-primary-foreground transition-colors hover:bg-primary/80 disabled:opacity-50"
            >
              {replaying ? "..." : "Replay"}
            </button>
          )}
        </td>
      </tr>
      {expanded && (
        <tr className="border-b bg-muted/30">
          <td colSpan={5} className="px-4 py-3">
            <div className="space-y-3 text-sm">
              <div>
                <span className="text-xs font-medium uppercase text-muted-foreground">
                  Exchange
                </span>
                <p className="font-mono">{message.exchange}</p>
              </div>
              <div>
                <span className="text-xs font-medium uppercase text-muted-foreground">
                  Headers
                </span>
                <pre className="mt-1 overflow-x-auto rounded bg-background p-2 text-xs">
                  {JSON.stringify(message.headers, null, 2)}
                </pre>
              </div>
              <div>
                <span className="text-xs font-medium uppercase text-muted-foreground">
                  Body
                </span>
                <pre className="mt-1 overflow-x-auto rounded bg-background p-2 text-xs">
                  {JSON.stringify(message.body, null, 2)}
                </pre>
              </div>
              {replayResult === "error" && replayError && (
                <p className="text-sm text-red-600 dark:text-red-400">
                  {replayError}
                </p>
              )}
            </div>
          </td>
        </tr>
      )}
    </>
  );
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit 2>&1 | head -20`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/go/DLQMessageRow.tsx
git commit -m "feat(frontend): add DLQMessageRow expandable table row"
```

---

### Task 4: Admin Page

**Files:**
- Create: `frontend/src/app/go/admin/page.tsx`

- [ ] **Step 1: Create the admin page**

```typescript
// frontend/src/app/go/admin/page.tsx
"use client";

import { useEffect, useState, useCallback } from "react";
import { DemoBanner } from "@/components/go/DemoBanner";
import { DLQMessageRow } from "@/components/go/DLQMessageRow";
import { fetchDLQMessages } from "@/lib/go-admin-api";
import type { DLQMessage } from "@/lib/go-admin-api";

export default function AdminPage() {
  const [messages, setMessages] = useState<DLQMessage[]>([]);
  const [count, setCount] = useState(0);
  const [connected, setConnected] = useState(true);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    const result = await fetchDLQMessages();
    setLoading(false);
    setConnected(result.connected);

    if (result.data) {
      setMessages(result.data.messages);
      setCount(result.data.count);
    } else if (result.error) {
      setError(result.error);
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  return (
    <div className="mx-auto max-w-5xl px-6 py-8">
      <h1 className="mb-2 text-2xl font-bold">DLQ Admin</h1>
      <p className="mb-6 text-sm text-muted-foreground">
        Inspect and replay dead-lettered messages from the checkout saga.
      </p>

      <DemoBanner />

      {!connected && (
        <div className="mb-6 rounded-lg border border-muted-foreground/20 bg-muted px-4 py-4">
          <p className="font-medium">Admin endpoints are not publicly exposed</p>
          <p className="mt-1 text-sm text-muted-foreground">
            These endpoints are only reachable from within the Kubernetes
            cluster. To use this panel locally:
          </p>
          <pre className="mt-2 overflow-x-auto rounded bg-background px-3 py-2 text-xs">
            ssh -f -N -L 8092:localhost:8092 debian
          </pre>
        </div>
      )}

      {connected && (
        <>
          <div className="mb-6 flex items-center gap-4">
            <div className="rounded border bg-card px-4 py-3">
              <p className="text-xs text-muted-foreground">DLQ Messages</p>
              <p className="mt-1 text-2xl font-bold">{count}</p>
            </div>
            <button
              onClick={refresh}
              disabled={loading}
              className="rounded bg-muted px-4 py-2 text-sm font-medium transition-colors hover:bg-muted/80 disabled:opacity-50"
            >
              {loading ? "Loading..." : "↻ Refresh"}
            </button>
          </div>

          {error && (
            <div className="mb-4 rounded border border-red-500/30 bg-red-500/10 px-4 py-2 text-sm text-red-600 dark:text-red-400">
              {error}
            </div>
          )}

          <div className="rounded border bg-card">
            {messages.length > 0 ? (
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b text-left text-muted-foreground">
                    <th className="px-4 py-2">Index</th>
                    <th className="px-4 py-2">Routing Key</th>
                    <th className="px-4 py-2">Timestamp</th>
                    <th className="px-4 py-2 text-center">Retries</th>
                    <th className="px-4 py-2 text-right">Action</th>
                  </tr>
                </thead>
                <tbody>
                  {messages.map((msg) => (
                    <DLQMessageRow
                      key={msg.index}
                      message={msg}
                      onReplayed={refresh}
                    />
                  ))}
                </tbody>
              </table>
            ) : (
              <p className="px-4 py-8 text-center text-muted-foreground">
                {loading ? "Loading..." : "No messages in the dead-letter queue"}
              </p>
            )}
          </div>
        </>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit 2>&1 | head -20`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add frontend/src/app/go/admin/page.tsx
git commit -m "feat(frontend): add DLQ admin page at /go/admin"
```

---

### Task 5: Update GoSubHeader Navigation

**Files:**
- Modify: `frontend/src/components/go/GoSubHeader.tsx:10-14`

- [ ] **Step 1: Add Admin link and update showNav condition**

In `frontend/src/components/go/GoSubHeader.tsx`, add `inAdmin` detection and update the nav:

Change lines 10-14 from:
```typescript
  const inStore = pathname.startsWith("/go/ecommerce");
  const inAnalytics = pathname.startsWith("/go/analytics");
  const onStoreRoot = pathname === "/go/ecommerce";
  const showNav = inStore || inAnalytics;
```

To:
```typescript
  const inStore = pathname.startsWith("/go/ecommerce");
  const inAnalytics = pathname.startsWith("/go/analytics");
  const inAdmin = pathname.startsWith("/go/admin");
  const onStoreRoot = pathname === "/go/ecommerce";
  const showNav = inStore || inAnalytics || inAdmin;
```

Then add the Admin link after the Analytics link (after line 37):

```tsx
          <Link
            href="/go/admin"
            className={`text-sm font-medium transition-colors ${
              inAdmin ? "text-foreground" : "text-muted-foreground hover:text-foreground"
            }`}
          >
            Admin
          </Link>
```

- [ ] **Step 2: Verify TypeScript compiles and dev server renders**

Run: `cd frontend && npx tsc --noEmit 2>&1 | head -20`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/go/GoSubHeader.tsx
git commit -m "feat(frontend): add Admin link to GoSubHeader navigation"
```

---

### Task 6: Playwright Mocked E2E Test

**Files:**
- Create: `frontend/e2e/mocked/go-dlq-admin.spec.ts`

- [ ] **Step 1: Create the Playwright test**

```typescript
// frontend/e2e/mocked/go-dlq-admin.spec.ts
import { test, expect } from "./fixtures";

const MOCK_DLQ_MESSAGES = {
  messages: [
    {
      index: 0,
      routing_key: "saga.cart.commands",
      exchange: "ecommerce.saga",
      timestamp: new Date().toISOString(),
      retry_count: 0,
      headers: { "x-retry-count": 0 },
      body: { command: "reserve.items", order_id: "test-order-1" },
    },
    {
      index: 1,
      routing_key: "saga.order.events",
      exchange: "ecommerce.saga",
      timestamp: new Date(Date.now() - 900_000).toISOString(),
      retry_count: 1,
      headers: { "x-retry-count": 1 },
      body: { event: "items.reserved", order_id: "test-order-2" },
    },
  ],
  count: 2,
};

test.describe("DLQ Admin Panel", () => {
  test.beforeEach(async ({ page }) => {
    await page.route("**/admin/dlq/messages*", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(MOCK_DLQ_MESSAGES),
      }),
    );
  });

  test("renders the admin page with demo banner", async ({ page }) => {
    await page.goto("/go/admin");
    await expect(page.locator("h1", { hasText: "DLQ Admin" })).toBeVisible();
    await expect(page.getByText("Portfolio Demo")).toBeVisible();
  });

  test("displays DLQ messages in a table", async ({ page }) => {
    await page.goto("/go/admin");
    await expect(page.getByText("saga.cart.commands")).toBeVisible();
    await expect(page.getByText("saga.order.events")).toBeVisible();
  });

  test("shows message count", async ({ page }) => {
    await page.goto("/go/admin");
    await expect(page.getByText("2")).toBeVisible();
  });

  test("expands row to show message body", async ({ page }) => {
    await page.goto("/go/admin");
    await page.getByText("saga.cart.commands").click();
    await expect(page.getByText("reserve.items")).toBeVisible();
    await expect(page.getByText("test-order-1")).toBeVisible();
  });

  test("replay button sends POST request", async ({ page }) => {
    let replayRequested = false;

    await page.route("**/admin/dlq/replay", (route) => {
      replayRequested = true;
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          replayed: { ...MOCK_DLQ_MESSAGES.messages[0], retry_count: 1 },
        }),
      });
    });

    await page.goto("/go/admin");
    const replayButtons = page.getByRole("button", { name: "Replay" });
    await replayButtons.first().click();

    await expect(page.getByText("Replayed")).toBeVisible();
    expect(replayRequested).toBe(true);
  });

  test("shows unreachable state when backend is down", async ({ page }) => {
    // Override the route to simulate network failure
    await page.route("**/admin/dlq/messages*", (route) => route.abort());

    await page.goto("/go/admin");
    await expect(
      page.getByText("Admin endpoints are not publicly exposed"),
    ).toBeVisible();
  });
});
```

- [ ] **Step 2: Run the Playwright tests**

Run: `cd frontend && npx playwright test e2e/mocked/go-dlq-admin.spec.ts --reporter=list 2>&1`
Expected: All 6 tests PASS

- [ ] **Step 3: Commit**

```bash
git add frontend/e2e/mocked/go-dlq-admin.spec.ts
git commit -m "test(frontend): add Playwright mocked tests for DLQ admin panel"
```

---

### Task 7: Preflight Verification

- [ ] **Step 1: Run frontend preflight**

Run: `make preflight-frontend`
Expected: TypeScript check + lint pass

- [ ] **Step 2: Run E2E tests**

Run: `make preflight-e2e`
Expected: All mocked E2E tests pass (including new DLQ admin tests)

---

## Verification Checklist

After all tasks are complete:

1. `make preflight-frontend` — tsc + lint green
2. `make preflight-e2e` — all mocked Playwright tests green
3. `npm run dev` → navigate to `/go/admin` — page renders with demo banner
4. Navigate to `/go/ecommerce` — "Admin" link visible in GoSubHeader nav
5. Without SSH tunnel — admin page shows unreachable state with instructions
6. With SSH tunnel — admin page shows DLQ messages (if any), expand works, replay works
