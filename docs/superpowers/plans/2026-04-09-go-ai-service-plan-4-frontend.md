# Plan 4 — `go/ai-service` Frontend Assistant Drawer

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **⚠ Frontend Next.js note:** `frontend/AGENTS.md` says "This is NOT the Next.js you know — read `node_modules/next/dist/docs/` before writing any code." Every task in this plan that touches the frontend must start by consulting the local Next docs for anything non-obvious.

**Goal:** Expose the `go/ai-service` agent to users via a floating assistant drawer on `/go/ecommerce/*`. The drawer shows the conversation, renders each tool call as a visible trace (name + args) with the rendered result card, streams the final answer token-by-token, and lets an authenticated user have a real agent conversation over SSE.

**Architecture:** New `lib/ai-service.ts` module provides a typed SSE client that POSTs to `/chat` with the current JWT, parses `event: tool_call | tool_result | tool_error | final | error` events, and yields them via an async iterator. New `AiAssistantDrawer.tsx` component renders the drawer UI (message list, tool-call cards, streaming final) and owns the message history in React state. The drawer mounts in `app/go/ecommerce/layout.tsx` as a floating button + slide-in panel.

**Tech Stack:** Next.js + TypeScript + Tailwind + existing shadcn/ui primitives (Button, Input/Textarea, Card, ScrollArea). No new shadcn components — a plain Tailwind fixed-position panel is enough for v1 and avoids adding `sheet.tsx`.

**Scope boundaries:**
- **No server-side rendering of the drawer.** Client-only component with `"use client"`.
- **No conversation persistence.** State lives in React; refresh wipes it. Matches the ai-service v1 contract (stateless sessions).
- **No custom product-card click handlers beyond navigating to `/go/ecommerce/[productId]`.** "Add to cart from the agent" is left to `add_to_cart` tool calls that actually hit the backend — no extra UI affordance.
- **No GraphQL / Apollo changes.** The drawer speaks plain fetch + SSE.
- **No K8s or CI changes** (→ Plan 5).
- **Mocked E2E only.** Production smoke adds in Plan 5.

**Reference:** spec section 6.2 (frontend integration).

---

## File Map

New:
```
frontend/src/
├── lib/
│   ├── ai-service.ts              # AI_SERVICE_URL, sendChat(messages, jwt) async generator
│   └── ai-service.test.ts         # (optional) unit test for SSE parsing
└── components/go/
    ├── AiAssistantDrawer.tsx      # client component, floating button + panel
    └── AiToolCallCard.tsx         # sub-component for one tool call + its result
frontend/e2e/mocked/
└── go-ai-assistant.spec.ts        # Playwright mocked E2E
```

Modified:
- `frontend/src/app/go/ecommerce/layout.tsx` — mount `<AiAssistantDrawer />`
- `frontend/.env.example` (if it exists) or `CLAUDE.md` docs — document the new `NEXT_PUBLIC_AI_SERVICE_URL` env var
- `CLAUDE.md` (root) — add `NEXT_PUBLIC_AI_SERVICE_URL` to the list of Vercel env vars

---

## Task 1: SSE client helper in `lib/ai-service.ts`

**Files:**
- Create: `frontend/src/lib/ai-service.ts`

**Before writing:** read `node_modules/next/dist/docs/` for any recent guidance on client-side fetch + ReadableStream in the Next version this repo uses. The code below uses plain browser APIs so it should be portable, but verify before deviating.

### Contents

```ts
// frontend/src/lib/ai-service.ts
// SSE client for the go/ai-service /chat endpoint.
// Parses event: / data: lines and yields typed events.

export const AI_SERVICE_URL =
  process.env.NEXT_PUBLIC_AI_SERVICE_URL || "http://localhost:8093";

export type ChatMessage = {
  role: "user" | "assistant" | "system";
  content: string;
};

export type ToolCallEvent = { kind: "tool_call"; name: string; args: unknown };
export type ToolResultEvent = {
  kind: "tool_result";
  name: string;
  display?: unknown;
};
export type ToolErrorEvent = { kind: "tool_error"; name: string; error: string };
export type FinalEvent = { kind: "final"; text: string };
export type ErrorEvent = { kind: "error"; reason: string };

export type AiEvent =
  | ToolCallEvent
  | ToolResultEvent
  | ToolErrorEvent
  | FinalEvent
  | ErrorEvent;

export type SendChatArgs = {
  messages: ChatMessage[];
  jwt?: string | null;
  signal?: AbortSignal;
};

/**
 * sendChat POSTs to /chat and yields parsed SSE events in order.
 * Caller is responsible for accumulating state (message history, final text).
 */
export async function* sendChat(args: SendChatArgs): AsyncGenerator<AiEvent> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    Accept: "text/event-stream",
  };
  if (args.jwt) {
    headers["Authorization"] = `Bearer ${args.jwt}`;
  }

  const res = await fetch(`${AI_SERVICE_URL}/chat`, {
    method: "POST",
    headers,
    body: JSON.stringify({ messages: args.messages }),
    signal: args.signal,
  });

  if (!res.ok || !res.body) {
    const text = await res.text().catch(() => "");
    throw new Error(`ai-service /chat: ${res.status} ${text}`);
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });

    // SSE events are separated by a blank line (\n\n).
    let sep: number;
    while ((sep = buffer.indexOf("\n\n")) !== -1) {
      const chunk = buffer.slice(0, sep);
      buffer = buffer.slice(sep + 2);

      const parsed = parseSseChunk(chunk);
      if (parsed) yield parsed;
    }
  }
}

function parseSseChunk(chunk: string): AiEvent | null {
  let eventName = "";
  let dataLines: string[] = [];
  for (const line of chunk.split("\n")) {
    if (line.startsWith("event:")) {
      eventName = line.slice("event:".length).trim();
    } else if (line.startsWith("data:")) {
      dataLines.push(line.slice("data:".length).trim());
    }
  }
  if (!eventName || dataLines.length === 0) return null;

  let data: unknown;
  try {
    data = JSON.parse(dataLines.join("\n"));
  } catch {
    return null;
  }

  switch (eventName) {
    case "tool_call":
      return { kind: "tool_call", name: (data as { name: string }).name, args: (data as { args: unknown }).args };
    case "tool_result":
      return { kind: "tool_result", name: (data as { name: string }).name, display: (data as { display?: unknown }).display };
    case "tool_error":
      return { kind: "tool_error", name: (data as { name: string }).name, error: (data as { error: string }).error };
    case "final":
      return { kind: "final", text: (data as { text: string }).text };
    case "error":
      return { kind: "error", reason: (data as { reason: string }).reason };
    default:
      return null;
  }
}
```

### Steps

- [ ] **Step 1: Create the file with the content above.**

- [ ] **Step 2: Type-check**

```bash
cd frontend && npx tsc --noEmit
```

Expected: clean.

- [ ] **Step 3: Update `CLAUDE.md` (root)**

In the Vercel CLI section that lists env vars (around the "Frontend env vars currently set in Vercel production" bullet list), add:

```
- `NEXT_PUBLIC_AI_SERVICE_URL=https://api.kylebradshaw.dev/ai-api` (add in Vercel before merge — localhost fallback will otherwise bake into the production bundle)
```

- [ ] **Step 4: Commit**

```bash
git add frontend/src/lib/ai-service.ts CLAUDE.md
git commit -m "feat(frontend): add ai-service SSE client helper"
```

---

## Task 2: `AiAssistantDrawer` component + mount in ecommerce layout

**Files:**
- Create: `frontend/src/components/go/AiAssistantDrawer.tsx`
- Create: `frontend/src/components/go/AiToolCallCard.tsx`
- Modify: `frontend/src/app/go/ecommerce/layout.tsx`

**Before writing:** confirm the local Next version's client-component syntax by reading `node_modules/next/dist/docs/` if anything feels unfamiliar.

### 2a. `AiToolCallCard.tsx`

Small presentational component that renders one tool call with its args and, if available, its result display payload. It doesn't own any state.

```tsx
"use client";

import { Card } from "@/components/ui/card";

export type ToolCallView = {
  id: string;
  name: string;
  args: unknown;
  status: "running" | "success" | "error";
  display?: unknown;
  error?: string;
};

export function AiToolCallCard({ call }: { call: ToolCallView }) {
  const statusColor =
    call.status === "error"
      ? "text-red-600"
      : call.status === "running"
        ? "text-muted-foreground"
        : "text-green-600";

  return (
    <Card className="my-2 border-dashed p-3 text-sm">
      <div className="flex items-center gap-2 font-mono">
        <span aria-hidden>🔧</span>
        <span className="font-semibold">{call.name}</span>
        <span className={`ml-auto text-xs ${statusColor}`}>{call.status}</span>
      </div>
      <pre className="mt-1 overflow-x-auto text-xs text-muted-foreground">
        {JSON.stringify(call.args, null, 2)}
      </pre>
      {call.error && (
        <div className="mt-2 text-xs text-red-600">error: {call.error}</div>
      )}
      {call.display !== undefined && call.status === "success" && (
        <pre className="mt-2 max-h-48 overflow-auto rounded bg-muted p-2 text-xs">
          {JSON.stringify(call.display, null, 2)}
        </pre>
      )}
    </Card>
  );
}
```

**Note on rendering rich display payloads:** v1 renders every `display` as pretty-printed JSON. Rich product cards / order cards are deliberately left to Plan 5 polish; what matters in Plan 4 is that the tool call IS visible. An interview reviewer seeing `🔧 search_products({"query":"jacket","max_price":150})` followed by `{"kind":"product_list","products":[...]}` gets the point immediately.

### 2b. `AiAssistantDrawer.tsx`

```tsx
"use client";

import { useCallback, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { ScrollArea } from "@/components/ui/scroll-area";
import { getGoAccessToken } from "@/lib/go-auth";
import { sendChat, type AiEvent, type ChatMessage } from "@/lib/ai-service";

import { AiToolCallCard, type ToolCallView } from "./AiToolCallCard";

type DisplayItem =
  | { kind: "user"; text: string }
  | { kind: "assistant"; text: string }
  | { kind: "tool"; call: ToolCallView };

export function AiAssistantDrawer() {
  const [open, setOpen] = useState(false);
  const [input, setInput] = useState("");
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [items, setItems] = useState<DisplayItem[]>([]);
  const [busy, setBusy] = useState(false);
  const [errorText, setErrorText] = useState<string | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  const appendItem = useCallback((item: DisplayItem) => {
    setItems((prev) => [...prev, item]);
  }, []);

  const updateLastTool = useCallback(
    (fn: (call: ToolCallView) => ToolCallView) => {
      setItems((prev) => {
        for (let i = prev.length - 1; i >= 0; i--) {
          const it = prev[i];
          if (it.kind === "tool" && it.call.status === "running") {
            const next = [...prev];
            next[i] = { kind: "tool", call: fn(it.call) };
            return next;
          }
        }
        return prev;
      });
    },
    [],
  );

  const handleSend = useCallback(async () => {
    const trimmed = input.trim();
    if (!trimmed || busy) return;

    const userMsg: ChatMessage = { role: "user", content: trimmed };
    const nextMessages = [...messages, userMsg];
    setMessages(nextMessages);
    appendItem({ kind: "user", text: trimmed });
    setInput("");
    setBusy(true);
    setErrorText(null);

    const controller = new AbortController();
    abortRef.current = controller;

    try {
      const jwt = getGoAccessToken();
      let idCounter = 0;
      let assistantText = "";

      for await (const ev of sendChat({
        messages: nextMessages,
        jwt,
        signal: controller.signal,
      })) {
        handleEvent(
          ev,
          () => `tc-${idCounter++}`,
          appendItem,
          updateLastTool,
          (text: string) => {
            assistantText = text;
            setItems((prev) => {
              // Replace or append the trailing assistant bubble.
              if (prev.length > 0 && prev[prev.length - 1].kind === "assistant") {
                const next = [...prev];
                next[next.length - 1] = { kind: "assistant", text };
                return next;
              }
              return [...prev, { kind: "assistant", text }];
            });
          },
          (reason: string) => setErrorText(reason),
        );
      }

      if (assistantText) {
        setMessages((prev) => [
          ...prev,
          { role: "assistant", content: assistantText },
        ]);
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : "unknown error";
      setErrorText(msg);
    } finally {
      setBusy(false);
      abortRef.current = null;
    }
  }, [appendItem, busy, input, messages, updateLastTool]);

  const handleCancel = useCallback(() => {
    abortRef.current?.abort();
  }, []);

  return (
    <>
      {!open && (
        <button
          type="button"
          onClick={() => setOpen(true)}
          data-testid="ai-assistant-open"
          className="fixed bottom-6 right-6 z-40 rounded-full bg-primary px-5 py-3 text-primary-foreground shadow-lg hover:opacity-90"
        >
          Ask AI
        </button>
      )}
      {open && (
        <aside
          role="dialog"
          aria-label="AI assistant"
          data-testid="ai-assistant-drawer"
          className="fixed bottom-0 right-0 top-0 z-50 flex w-full max-w-md flex-col border-l bg-background shadow-xl"
        >
          <header className="flex items-center justify-between border-b px-4 py-3">
            <h2 className="text-base font-semibold">Shopping Assistant</h2>
            <button
              type="button"
              onClick={() => setOpen(false)}
              data-testid="ai-assistant-close"
              className="text-sm text-muted-foreground hover:underline"
            >
              Close
            </button>
          </header>
          <ScrollArea className="flex-1 px-4 py-3">
            {items.length === 0 && (
              <p className="text-sm text-muted-foreground">
                Try: "find me a waterproof jacket under $150" or "where's my last
                order?"
              </p>
            )}
            {items.map((it, i) => (
              <div key={i} className="mb-3">
                {it.kind === "user" && (
                  <div className="ml-auto w-fit max-w-[85%] rounded-lg bg-primary px-3 py-2 text-sm text-primary-foreground">
                    {it.text}
                  </div>
                )}
                {it.kind === "assistant" && (
                  <div
                    data-testid="ai-assistant-final"
                    className="mr-auto w-fit max-w-[90%] rounded-lg bg-muted px-3 py-2 text-sm"
                  >
                    {it.text}
                  </div>
                )}
                {it.kind === "tool" && <AiToolCallCard call={it.call} />}
              </div>
            ))}
            {errorText && (
              <div
                data-testid="ai-assistant-error"
                className="mt-2 rounded border border-red-500 p-2 text-sm text-red-600"
              >
                {errorText}
              </div>
            )}
          </ScrollArea>
          <div className="border-t p-3">
            <Textarea
              value={input}
              onChange={(e) => setInput(e.target.value)}
              placeholder="Ask the shopping assistant…"
              rows={2}
              data-testid="ai-assistant-input"
              disabled={busy}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !e.shiftKey) {
                  e.preventDefault();
                  void handleSend();
                }
              }}
            />
            <div className="mt-2 flex gap-2">
              <Button
                onClick={handleSend}
                disabled={busy || input.trim().length === 0}
                data-testid="ai-assistant-send"
              >
                {busy ? "Thinking…" : "Send"}
              </Button>
              {busy && (
                <Button variant="outline" onClick={handleCancel}>
                  Cancel
                </Button>
              )}
            </div>
          </div>
        </aside>
      )}
    </>
  );
}

function handleEvent(
  ev: AiEvent,
  nextId: () => string,
  appendItem: (item: DisplayItem) => void,
  updateLastTool: (fn: (call: ToolCallView) => ToolCallView) => void,
  setAssistant: (text: string) => void,
  setError: (reason: string) => void,
) {
  switch (ev.kind) {
    case "tool_call": {
      const id = nextId();
      appendItem({
        kind: "tool",
        call: { id, name: ev.name, args: ev.args, status: "running" },
      });
      break;
    }
    case "tool_result": {
      updateLastTool((call) =>
        call.name === ev.name
          ? { ...call, status: "success", display: ev.display }
          : call,
      );
      break;
    }
    case "tool_error": {
      updateLastTool((call) =>
        call.name === ev.name
          ? { ...call, status: "error", error: ev.error }
          : call,
      );
      break;
    }
    case "final": {
      setAssistant(ev.text);
      break;
    }
    case "error": {
      setError(ev.reason);
      break;
    }
  }
}
```

### 2c. Mount the drawer in the ecommerce layout

Modify `frontend/src/app/go/ecommerce/layout.tsx`:

```tsx
import { GoSubHeader } from "@/components/go/GoSubHeader";
import { AiAssistantDrawer } from "@/components/go/AiAssistantDrawer";

export default function GoEcommerceLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <>
      <GoSubHeader />
      {children}
      <AiAssistantDrawer />
    </>
  );
}
```

### Steps

- [ ] **Step 1: Read `frontend/AGENTS.md` and skim `node_modules/next/dist/docs/` for anything odd about client components in this Next version.** Don't change the approach, but confirm `"use client"` + `useState` + `fetch` + `ReadableStream` work as shown.

- [ ] **Step 2: Create `AiToolCallCard.tsx` and `AiAssistantDrawer.tsx` with the contents above.**

- [ ] **Step 3: Modify `app/go/ecommerce/layout.tsx` to mount the drawer.**

- [ ] **Step 4: Type-check + lint + build**

```bash
cd frontend && npx tsc --noEmit
cd frontend && npm run lint
cd frontend && npm run build
```

Expected: clean. If `npm run build` surfaces a Next version-specific complaint about the client component, STOP and read the relevant doc at `node_modules/next/dist/docs/` before editing — don't guess.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/go/AiAssistantDrawer.tsx \
        frontend/src/components/go/AiToolCallCard.tsx \
        frontend/src/app/go/ecommerce/layout.tsx
git commit -m "feat(frontend): mount AI assistant drawer on /go/ecommerce"
```

---

## Task 3: Playwright mocked E2E

**Files:**
- Create: `frontend/e2e/mocked/go-ai-assistant.spec.ts`

### Contents

```ts
import { test, expect } from "@playwright/test";

test.describe("Go ecommerce AI assistant drawer", () => {
  test("opens, streams a tool call and a final answer", async ({ page }) => {
    // Mock /chat SSE — a tool_call for search_products, then a tool_result, then final.
    await page.route("**/chat", (route) => {
      const sseBody = [
        'event: tool_call',
        'data: {"name":"search_products","args":{"query":"waterproof jacket","max_price":150}}',
        "",
        'event: tool_result',
        'data: {"name":"search_products","display":{"kind":"product_list","products":[{"id":"p1","name":"Waterproof Jacket","price":12999}]}}',
        "",
        'event: final',
        'data: {"text":"I found a waterproof jacket under $150."}',
        "",
      ].join("\n");

      return route.fulfill({
        status: 200,
        contentType: "text/event-stream",
        body: sseBody,
      });
    });

    // Some ecommerce routes may be hit by the page; stub them out loosely.
    await page.route("**/products**", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ products: [], total: 0, page: 1, limit: 20 }),
      }),
    );

    await page.goto("/go/ecommerce");

    // Open the drawer
    await page.getByTestId("ai-assistant-open").click();
    await expect(page.getByTestId("ai-assistant-drawer")).toBeVisible();

    // Send a message
    const input = page.getByTestId("ai-assistant-input");
    await input.fill("find me a waterproof jacket under $150");
    await page.getByTestId("ai-assistant-send").click();

    // User bubble appears
    await expect(
      page.getByText("find me a waterproof jacket under $150"),
    ).toBeVisible();

    // Tool call card appears with the tool name
    await expect(page.getByText("search_products")).toBeVisible();

    // Tool call args render as JSON
    await expect(page.getByText(/"max_price": 150/)).toBeVisible();

    // Final answer renders
    await expect(page.getByTestId("ai-assistant-final")).toHaveText(
      "I found a waterproof jacket under $150.",
    );
  });

  test("shows an error when ai-service returns 500", async ({ page }) => {
    await page.route("**/chat", (route) =>
      route.fulfill({ status: 500, body: "internal" }),
    );
    await page.route("**/products**", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ products: [], total: 0, page: 1, limit: 20 }),
      }),
    );

    await page.goto("/go/ecommerce");
    await page.getByTestId("ai-assistant-open").click();
    await page.getByTestId("ai-assistant-input").fill("hi");
    await page.getByTestId("ai-assistant-send").click();

    await expect(page.getByTestId("ai-assistant-error")).toBeVisible();
  });
});
```

### Steps

- [ ] **Step 1: Create the spec file with the content above.**

- [ ] **Step 2: Run Playwright mocked locally**

```bash
cd frontend && npm run e2e:mocked -- go-ai-assistant.spec.ts
```

(Or whatever the existing mocked test invocation is — check `frontend/package.json` scripts if this doesn't match.)

Expected: both tests PASS.

If `NEXT_PUBLIC_AI_SERVICE_URL` is unset during the test, the drawer will hit `http://localhost:8093/chat`, which the `page.route("**/chat")` mock intercepts — that's fine. If Playwright's route matcher doesn't catch the absolute URL because of origin rules, make the route matcher explicit: `"http://localhost:8093/chat"`.

- [ ] **Step 3: Commit**

```bash
git add frontend/e2e/mocked/go-ai-assistant.spec.ts
git commit -m "test(frontend): add mocked Playwright E2E for AI assistant drawer"
```

---

## Done criteria for Plan 4

- `npx tsc --noEmit`, `npm run lint`, and `npm run build` in `frontend/` all pass.
- `/go/ecommerce/*` routes show a floating "Ask AI" button.
- Clicking it opens a drawer; typing a message and pressing Send streams tool calls, results, and a final answer from the ai-service SSE.
- The mocked Playwright E2E for `go-ai-assistant.spec.ts` passes.
- `CLAUDE.md` lists `NEXT_PUBLIC_AI_SERVICE_URL` in the Vercel env var section.
- No changes to Apollo/GraphQL, no new shadcn components beyond what's already in `components/ui/`, no K8s/CI changes.
