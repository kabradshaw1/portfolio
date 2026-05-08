# Async Portfolio Section Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a recruiter-facing `/async` portfolio page that explains the Go ecommerce Kafka and RabbitMQ reliability work.

**Architecture:** Keep this as a static Next.js App Router narrative page with local arrays for repeated page content. Wire it into the existing first-class portfolio navigation and homepage card grid without adding runtime dependencies or backend calls.

**Tech Stack:** Next.js App Router, TypeScript, Tailwind CSS, shadcn/ui card components, Playwright mocked route tests.

---

### Task 1: Add Mocked Route Coverage

**Files:**
- Create: `frontend/e2e/mocked/async-page.spec.ts`

- [ ] **Step 1: Create the failing Playwright tests**

```ts
import { test, expect } from "./fixtures";

test.describe("/async page", () => {
  test("renders the recruiter-facing async systems narrative", async ({ page }) => {
    await page.goto("/async");

    await expect(
      page.getByRole("heading", {
        name: "Asynchronous Systems Engineering",
        level: 1,
      }),
    ).toBeVisible();
    await expect(page.getByText("Go, Kafka, and RabbitMQ", { exact: false })).toBeVisible();
    await expect(page.getByText("command/reply queues", { exact: false })).toBeVisible();
    await expect(page.getByText("Kafka DLQ envelopes", { exact: false })).toBeVisible();
    await expect(page.getByText("Prometheus metrics", { exact: false })).toBeVisible();
    await expect(page.getByText("trace propagation", { exact: false })).toBeVisible();
  });

  test("links to the related demos", async ({ page }) => {
    await page.goto("/async");

    await expect(page.getByRole("link", { name: "Go ecommerce" })).toHaveAttribute(
      "href",
      "/go/ecommerce",
    );
    await expect(page.getByRole("link", { name: "Streaming Analytics" })).toHaveAttribute(
      "href",
      "/go/analytics",
    );
    await expect(page.getByRole("link", { name: "Order Timeline" })).toHaveAttribute(
      "href",
      "/go/ecommerce/orders",
    );
    await expect(page.getByRole("link", { name: "Admin Panel" })).toHaveAttribute(
      "href",
      "/go/admin",
    );
    await expect(page.getByRole("link", { name: "Grafana" })).toHaveAttribute(
      "href",
      /grafana\.kylebradshaw\.dev/,
    );
  });

  test("homepage and primary navigation link to /async", async ({ page }) => {
    await page.goto("/");

    await expect(page.getByRole("link", { name: "Async", exact: true })).toHaveAttribute(
      "href",
      "/async",
    );
    await expect(
      page.getByRole("link", { name: /Asynchronous Systems Engineering/ }),
    ).toHaveAttribute("href", "/async");
  });
});
```

- [ ] **Step 2: Run the test and verify it fails**

Run: `cd frontend && npx playwright test e2e/mocked/async-page.spec.ts --config=playwright.config.ts`

Expected: FAIL because `/async` does not exist yet and homepage/header do not link to it.

### Task 2: Build `/async`

**Files:**
- Create: `frontend/src/app/async/page.tsx`

- [ ] **Step 1: Add the static narrative page**

```tsx
import Link from "next/link";

const grafanaUrl =
  "https://grafana.kylebradshaw.dev/d/system-overview/system-overview?orgId=1&from=now-1h&to=now&timezone=browser";

const rabbitMqWork = [
  "Checkout saga orchestration with command/reply queues across order, payment, inventory, and shipping boundaries.",
  "Bounded retries with explicit DLQ routing so transient broker/service failures do not become infinite poison-message loops.",
  "Replay/admin panel support for inspected DLQ messages, including operator-controlled recovery paths.",
  "Publisher confirms, reconnect-aware publishing, graceful shutdown, and crash recovery around the RabbitMQ client lifecycle.",
];

const kafkaWork = [
  "Order domain events published as the durable event stream behind event sourcing and downstream projections.",
  "CQRS projector paths that rebuild read models from replayable events instead of coupling UI reads to transactional writes.",
  "Schema evolution, replay discipline, Kafka DLQ envelopes, consumer lag metrics, and streaming analytics over order activity.",
];

const reliabilityPractices = [
  "Idempotency at async boundaries so duplicate deliveries and retries remain safe.",
  "Retry classification that separates transient failures from validation and poison-message failures.",
  "DLQs with enough envelope context to debug, alert, and replay without guessing.",
  "Prometheus metrics for consumer lag, DLQ depth, saga progress, publish outcomes, and worker health.",
  "Trace propagation through RabbitMQ headers and Kafka message headers for cross-service debugging.",
  "Focused tests around retry behavior, replay paths, graceful shutdown, and crash recovery.",
];

const demoLinks = [
  { href: "/go/ecommerce", label: "Go ecommerce" },
  { href: "/go/analytics", label: "Streaming Analytics" },
  { href: "/go/ecommerce/orders", label: "Order Timeline" },
  { href: "/go/admin", label: "Admin Panel" },
  { href: grafanaUrl, label: "Grafana", external: true },
];

export default function AsyncPage() {
  return (
    <div className="mx-auto max-w-4xl px-6 py-12">
      <section className="mt-8">
        <p className="text-sm font-medium uppercase tracking-wide text-muted-foreground">
          Go ecommerce platform
        </p>
        <h1 className="mt-3 text-4xl font-bold tracking-normal">
          Asynchronous Systems Engineering
        </h1>
        <p className="mt-6 max-w-3xl text-lg leading-relaxed text-muted-foreground">
          This section highlights the production-grade async work behind the Go
          ecommerce services: RabbitMQ for checkout saga coordination, Kafka for
          order domain events and replayable projections, and the operational
          guardrails that make message-driven systems debuggable after they
          leave the happy path.
        </p>
        <p className="mt-4 max-w-3xl leading-relaxed text-muted-foreground">
          The implementation is intentionally practical: Go, Kafka, and
          RabbitMQ are used where they solve concrete reliability problems in
          checkout, analytics, event sourcing, CQRS read models, and incident
          response.
        </p>
      </section>

      <section className="mt-12 grid gap-8 md:grid-cols-2">
        <div>
          <h2 className="text-2xl font-semibold">RabbitMQ Checkout Saga</h2>
          <p className="mt-3 leading-relaxed text-muted-foreground">
            RabbitMQ coordinates the checkout workflow where services need
            explicit commands, replies, compensation, and operator recovery.
          </p>
          <ul className="mt-5 space-y-3 text-sm leading-relaxed text-muted-foreground">
            {rabbitMqWork.map((item) => (
              <li key={item} className="border-l border-foreground/15 pl-4">
                {item}
              </li>
            ))}
          </ul>
        </div>

        <div>
          <h2 className="text-2xl font-semibold">Kafka Event Streams</h2>
          <p className="mt-3 leading-relaxed text-muted-foreground">
            Kafka carries order facts as an append-only stream for event
            sourcing, CQRS projection, replay, and analytics.
          </p>
          <ul className="mt-5 space-y-3 text-sm leading-relaxed text-muted-foreground">
            {kafkaWork.map((item) => (
              <li key={item} className="border-l border-foreground/15 pl-4">
                {item}
              </li>
            ))}
          </ul>
        </div>
      </section>

      <section className="mt-14">
        <h2 className="text-2xl font-semibold">Reliability Practices</h2>
        <div className="mt-5 grid gap-3 md:grid-cols-2">
          {reliabilityPractices.map((practice) => (
            <div key={practice} className="rounded-lg border border-foreground/10 p-4">
              <p className="text-sm leading-relaxed text-muted-foreground">{practice}</p>
            </div>
          ))}
        </div>
      </section>

      <section className="mt-14">
        <h2 className="text-2xl font-semibold">Related Demos</h2>
        <div className="mt-5 flex flex-wrap gap-3">
          {demoLinks.map((link) =>
            link.external ? (
              <a
                key={link.href}
                href={link.href}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center rounded-lg border px-4 py-2 text-sm font-medium transition-colors hover:bg-accent"
              >
                {link.label}
              </a>
            ) : (
              <Link
                key={link.href}
                href={link.href}
                className="inline-flex items-center rounded-lg border px-4 py-2 text-sm font-medium transition-colors hover:bg-accent"
              >
                {link.label}
              </Link>
            ),
          )}
        </div>
      </section>
    </div>
  );
}
```

- [ ] **Step 2: Run the async page test**

Run: `cd frontend && npx playwright test e2e/mocked/async-page.spec.ts --config=playwright.config.ts`

Expected: still FAIL because the header and homepage links are not wired yet.

### Task 3: Wire Navigation and Homepage Portfolio Entry

**Files:**
- Modify: `frontend/src/components/SiteHeader.tsx`
- Modify: `frontend/src/app/page.tsx`

- [ ] **Step 1: Add Async to the primary nav after Database**

In `frontend/src/components/SiteHeader.tsx`, add:

```tsx
<Link href="/async" className={navLinkClass("/async")}>
  Async
</Link>
```

Place it immediately after the `/database` link and before `/ai`.

- [ ] **Step 2: Add the homepage card after Database Engineering**

In `frontend/src/app/page.tsx`, add:

```tsx
<Link href="/async" className="block">
  <Card className="hover:ring-foreground/20 transition-all">
    <CardHeader>
      <CardTitle>Asynchronous Systems Engineering</CardTitle>
      <CardDescription>
        Go ecommerce messaging with Kafka event streams, RabbitMQ sagas, DLQs,
        replay, and production observability
      </CardDescription>
    </CardHeader>
    <CardContent>
      <p className="text-muted-foreground text-sm">
        Checkout saga command/reply queues, bounded retries, publisher
        confirms, reconnect-aware RabbitMQ publishing, Kafka-backed order
        events, CQRS projection, streaming analytics, DLQ envelopes, and
        traceable recovery paths.
      </p>
    </CardContent>
  </Card>
</Link>
```

Place it immediately after the Database Engineering card and before the AI Engineer card.

- [ ] **Step 3: Run the focused mocked test**

Run: `cd frontend && npx playwright test e2e/mocked/async-page.spec.ts --config=playwright.config.ts`

Expected: PASS.

### Task 4: Verify and Ship

**Files:**
- No source files changed in this task.

- [ ] **Step 1: Run frontend preflight**

Run: `make preflight-frontend`

Expected: PASS.

- [ ] **Step 2: Run e2e preflight when local dependencies are available**

Run: `make preflight-e2e`

Expected: PASS, or report the exact local dependency/tooling blocker and leave remaining verification to CI.

- [ ] **Step 3: Inspect git diff**

Run: `git status --short && git diff -- frontend/src/app/async/page.tsx frontend/src/components/SiteHeader.tsx frontend/src/app/page.tsx frontend/e2e/mocked/async-page.spec.ts docs/superpowers/plans/2026-05-08-async-portfolio-section.md`

Expected: only the planned files changed.

- [ ] **Step 4: Commit**

Run:

```bash
git add docs/superpowers/plans/2026-05-08-async-portfolio-section.md frontend/src/app/async/page.tsx frontend/src/components/SiteHeader.tsx frontend/src/app/page.tsx frontend/e2e/mocked/async-page.spec.ts
git commit -m "feat: add async systems portfolio page"
```

Expected: commit succeeds on `feat/async-portfolio-section`.

- [ ] **Step 5: Push and create PR to qa**

Run:

```bash
git push -u origin feat/async-portfolio-section
gh pr create --base qa --head feat/async-portfolio-section --title "Add async systems portfolio page" --body "## Summary
- add recruiter-facing /async portfolio page for Kafka and RabbitMQ engineering
- link the new section from the primary nav and homepage portfolio grid
- cover the page and links with mocked Playwright tests

## Verification
- make preflight-frontend
- make preflight-e2e"
```

Expected: branch is pushed and a PR targeting `qa` is created. Do not watch CI.

### Self-Review

- Spec coverage: the page title, Go/Kafka/RabbitMQ hero language, RabbitMQ saga content, Kafka event-stream content, reliability practices, demo links, header nav, homepage card, and mocked Playwright coverage are all mapped to tasks.
- Placeholder scan: no TBD/TODO/fill-in-later steps are present.
- Type consistency: the route is static and does not introduce shared TypeScript types; local arrays use inferred literal object shapes.
