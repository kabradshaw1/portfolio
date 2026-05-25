# GalaxyVoyagers Portfolio Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add GalaxyVoyagers.com as the first portfolio landing-page card and build a `/galaxy` architecture case-study route.

**Architecture:** The root page remains a concise portfolio index and links visitors to `/galaxy` for the full explanation. The new route is a static Next.js App Router page that reuses the existing `MermaidDiagram` client component for architecture visualization and follows the existing portfolio typography/card patterns.

**Tech Stack:** Next.js App Router, TypeScript, React, shadcn/ui Card, Mermaid, Playwright, Story repo architecture facts.

---

## File Structure

- Modify `frontend/src/app/page.tsx`: insert the GalaxyVoyagers teaser card as the first portfolio card.
- Create `frontend/src/app/galaxy/page.tsx`: full static case-study page with external live-site link, stack narrative, architecture diagram, and GraphQL rationale.
- Create `frontend/e2e/mocked/galaxy-portfolio.spec.ts`: focused Playwright coverage for root-card navigation and `/galaxy` content.
- Read `frontend/node_modules/next/dist/docs/01-app/01-getting-started/03-layouts-and-pages.md` before implementation to satisfy the repo's Next.js version guidance.

## Execution Setup

### Task 0: Create And Enter The Feature Worktree

**Files:**
- No code files changed.

- [ ] **Step 1: Use the required worktree skill**

Use `superpowers:using-git-worktrees` before creating or selecting the worktree.

- [ ] **Step 2: Create the feature worktree from the repo root**

Run:

```bash
git branch --show-current
git worktree add .codex/worktrees/galaxyvoyagers-portfolio -b feature/galaxyvoyagers-portfolio
```

Expected: current branch is `main`, and Git creates `.codex/worktrees/galaxyvoyagers-portfolio`.

- [ ] **Step 3: Confirm all future work is inside the worktree**

Run:

```bash
pwd
git branch --show-current
git rev-parse --show-toplevel
```

Expected:

```text
/Users/kylebradshaw/repos/gen_ai_engineer/.codex/worktrees/galaxyvoyagers-portfolio
feature/galaxyvoyagers-portfolio
/Users/kylebradshaw/repos/gen_ai_engineer/.codex/worktrees/galaxyvoyagers-portfolio
```

- [ ] **Step 4: Read the relevant local Next.js docs**

Run:

```bash
sed -n '1,180p' frontend/node_modules/next/dist/docs/01-app/01-getting-started/03-layouts-and-pages.md
```

Expected: confirms the App Router route convention where `app/galaxy/page.tsx` creates `/galaxy`.

## Implementation Tasks

### Task 1: Add Failing Playwright Coverage

**Files:**
- Create: `frontend/e2e/mocked/galaxy-portfolio.spec.ts`

- [ ] **Step 1: Create the focused Playwright test**

Create `frontend/e2e/mocked/galaxy-portfolio.spec.ts` with:

```ts
import { expect, test } from "./fixtures";

test.describe("GalaxyVoyagers portfolio case study", () => {
  test("links the landing card to the Galaxy architecture page", async ({
    page,
  }) => {
    await page.goto("/");

    const cardLink = page.getByRole("link", {
      name: /GalaxyVoyagers\.com/i,
    });

    await expect(cardLink).toBeVisible();
    await expect(
      page.getByText(/View the architecture walkthrough/i)
    ).toBeVisible();

    await cardLink.click();

    await expect(page).toHaveURL(/\/galaxy$/);
    await expect(
      page.getByRole("heading", { name: "GalaxyVoyagers.com" })
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: /Open GalaxyVoyagers\.com/i })
    ).toHaveAttribute("href", "https://galaxyvoyagers.com");
    await expect(
      page.getByRole("heading", {
        name: "Why GraphQL Was The Right Boundary",
      })
    ).toBeVisible();
    await expect(
      page.getByText(/one operation/i)
    ).toBeVisible();
  });
});
```

- [ ] **Step 2: Run the new test and verify it fails**

Run:

```bash
cd frontend
npx playwright test e2e/mocked/galaxy-portfolio.spec.ts --reporter=list
```

Expected: FAIL because the `GalaxyVoyagers.com` landing card and `/galaxy` route do not exist yet.

- [ ] **Step 3: Commit the failing test**

Run:

```bash
git add frontend/e2e/mocked/galaxy-portfolio.spec.ts
git commit -m "test: cover GalaxyVoyagers portfolio route"
```

Expected: one commit containing only the new Playwright test.

### Task 2: Add The Landing Page Teaser Card

**Files:**
- Modify: `frontend/src/app/page.tsx`

- [ ] **Step 1: Insert the GalaxyVoyagers card as the first portfolio card**

In `frontend/src/app/page.tsx`, inside the `<div className="mt-6 grid gap-4">` portfolio list and before the existing `/go` card, add:

```tsx
          <Link href="/galaxy" className="block">
            <Card className="hover:ring-foreground/20 transition-all">
              <CardHeader>
                <CardTitle>GalaxyVoyagers.com</CardTitle>
                <CardDescription>
                  Deployed collaborative sci-fi worldbuilding platform with a
                  Go GraphQL gateway and AI-assisted creation tools
                </CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-muted-foreground text-sm">
                  Built with Next.js, Apollo Client, Go, gqlgen, gRPC,
                  PostgreSQL, MongoDB, Redis, RabbitMQ, and AI generation.
                  View the architecture walkthrough for the full system design.
                </p>
              </CardContent>
            </Card>
          </Link>
```

- [ ] **Step 2: Run the targeted Playwright test and verify the route is still missing**

Run:

```bash
cd frontend
npx playwright test e2e/mocked/galaxy-portfolio.spec.ts --reporter=list
```

Expected: FAIL after clicking the card because `/galaxy` has not been created yet.

### Task 3: Add The Galaxy Case-Study Route

**Files:**
- Create: `frontend/src/app/galaxy/page.tsx`

- [ ] **Step 1: Create the page**

Create `frontend/src/app/galaxy/page.tsx` with:

```tsx
import { ExternalLink } from "lucide-react";
import { MermaidDiagram } from "@/components/MermaidDiagram";

const stack = [
  "Next.js App Router",
  "React",
  "TypeScript",
  "Apollo Client",
  "Go",
  "gqlgen",
  "GraphQL subscriptions",
  "gRPC + Protobuf",
  "PostgreSQL",
  "MongoDB",
  "Redis",
  "RabbitMQ",
  "OpenAI",
  "Image generation",
  "Docker",
  "GitHub Actions",
  "Kubernetes",
  "Cloudflare Tunnel",
];

const graphDomains = [
  {
    title: "Connected worldbuilding data",
    desc: "Stories connect to scenes, characters, locations, organizations, conflicts, roles, ships, generated images, and discussion content.",
  },
  {
    title: "One declarative UI query",
    desc: "The frontend can ask for the nested shape a screen needs instead of fetching a story, then scenes, then related entities, then media through chained browser calls.",
  },
  {
    title: "Gateway-owned composition",
    desc: "The Go GraphQL gateway resolves fields across backend services and datastores while keeping the browser API explicit and stable.",
  },
  {
    title: "Subscriptions fit live generation",
    desc: "GraphQL subscriptions support streaming story suggestions and async creation flows without adding a second frontend API model.",
  },
];

export default function GalaxyPage() {
  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <section>
        <p className="text-sm font-medium text-primary">Deployed project</p>
        <h1 className="mt-3 text-3xl font-bold">GalaxyVoyagers.com</h1>
        <p className="mt-6 text-muted-foreground leading-relaxed">
          GalaxyVoyagers is a collaborative sci-fi worldbuilding platform for
          building stories, scenes, characters, organizations, locations,
          ships, conflicts, and supporting media. It is a separate production
          deployment that demonstrates how I design a full-stack application
          around a connected domain instead of treating each screen as an
          isolated CRUD form.
        </p>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          The live site uses a Next.js frontend backed by a Go GraphQL gateway.
          Behind that gateway, Go services communicate over gRPC, store domain
          data in PostgreSQL and MongoDB, use Redis for shared runtime state,
          and send async work through RabbitMQ for AI-assisted story and image
          generation flows.
        </p>
        <a
          href="https://galaxyvoyagers.com"
          target="_blank"
          rel="noopener noreferrer"
          className="mt-6 inline-flex items-center gap-2 rounded-lg bg-primary px-5 py-3 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
        >
          Open GalaxyVoyagers.com
          <ExternalLink className="size-4" aria-hidden="true" />
        </a>
      </section>

      <section className="mt-12">
        <h2 className="text-2xl font-semibold">Technology Stack</h2>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          The project is intentionally polyglot at the system boundary but
          conservative inside each service: TypeScript and Apollo Client in the
          browser, Go and gqlgen at the API gateway, protobuf-defined gRPC
          contracts between services, and proven datastores selected for the
          access pattern they serve.
        </p>
        <div className="mt-4 flex flex-wrap gap-2">
          {stack.map((tech) => (
            <span
              key={tech}
              className="rounded-full bg-primary/10 px-3 py-1 text-xs font-medium text-primary"
            >
              {tech}
            </span>
          ))}
        </div>
      </section>

      <section className="mt-12">
        <h2 className="text-2xl font-semibold">Architecture</h2>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          The browser talks to one GraphQL entry point for queries, mutations,
          and subscriptions. The gateway owns backend composition: it calls the
          story, chat, auth, image, story-generation, and Stripe services over
          gRPC, while async generation work moves through RabbitMQ and streams
          results back to the UI.
        </p>
        <div className="mt-6">
          <MermaidDiagram
            chart={`flowchart LR
  U[Browser]
  NEXT[Next.js App Router<br/>Apollo Client]
  GW[Go GraphQL Gateway<br/>gqlgen :4000]
  STORY[story-service<br/>gRPC :50051]
  CHAT[chat-service<br/>gRPC :50052]
  STRIPE[stripe-service<br/>gRPC :50053]
  STORYGEN[storygen-service<br/>gRPC :50054]
  IMAGE[image-service<br/>gRPC :50055]
  AUTH[authv2-service<br/>gRPC :50056]
  PG[(PostgreSQL<br/>world + auth data)]
  MONGO[(MongoDB<br/>chat/discussion data)]
  REDIS[(Redis<br/>shared runtime state)]
  MQ{{RabbitMQ<br/>async jobs}}
  AI[OpenAI<br/>story + image models]
  K8S[Kubernetes homelab<br/>Cloudflare Tunnel]
  U --> NEXT
  NEXT -->|GraphQL queries + mutations| GW
  NEXT -->|GraphQL subscriptions| GW
  K8S -. serves .-> NEXT
  K8S -. routes API .-> GW
  GW -->|gRPC| STORY
  GW -->|gRPC| CHAT
  GW -->|gRPC| AUTH
  GW -->|gRPC| IMAGE
  GW -->|gRPC| STORYGEN
  GW -->|gRPC| STRIPE
  STORY --> PG
  AUTH --> PG
  STRIPE --> PG
  CHAT --> MONGO
  STORY --> REDIS
  AUTH --> REDIS
  IMAGE --> REDIS
  GW --> MQ
  IMAGE --> MQ
  STORYGEN --> AI
  IMAGE --> AI`}
          />
        </div>
      </section>

      <section className="mt-12">
        <h2 className="text-2xl font-semibold">
          Why GraphQL Was The Right Boundary
        </h2>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          GalaxyVoyagers is not a flat resource catalog. A useful screen often
          needs a nested view: a story, its ordered scenes, the characters and
          locations in each scene, related organizations and conflicts,
          generated images, and discussion context. GraphQL fits that shape
          because the UI can request the exact graph it needs in one operation.
        </p>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          With a REST-only browser API, that same screen would tend to become a
          chain of dependent requests: fetch the story, fetch scenes, fetch the
          entities attached to each scene, fetch media, then fetch comments.
          The GraphQL gateway moves that composition into the backend, where it
          can resolve nested fields through service calls and datastore access
          without forcing the browser to coordinate every step.
        </p>
        <div className="mt-5 grid grid-cols-1 gap-3 sm:grid-cols-2">
          {graphDomains.map((item) => (
            <div
              key={item.title}
              className="rounded-lg border border-foreground/10 p-4"
            >
              <h3 className="text-sm font-semibold">{item.title}</h3>
              <p className="mt-2 text-xs text-muted-foreground leading-relaxed">
                {item.desc}
              </p>
            </div>
          ))}
        </div>
      </section>

      <section className="mt-12">
        <h2 className="text-2xl font-semibold">Engineering Focus</h2>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          The project highlights production-oriented backend design: a typed
          GraphQL boundary, protobuf service contracts, separate persistence
          models for relational worldbuilding data and document-style
          discussion data, async job handling for expensive generation work,
          and deployment through containerized services on Kubernetes.
        </p>
      </section>
    </div>
  );
}
```

- [ ] **Step 2: Run the targeted test and verify it passes**

Run:

```bash
cd frontend
npx playwright test e2e/mocked/galaxy-portfolio.spec.ts --reporter=list
```

Expected: PASS.

- [ ] **Step 3: Commit the frontend implementation**

Run:

```bash
git add frontend/src/app/page.tsx frontend/src/app/galaxy/page.tsx
git commit -m "feat: add GalaxyVoyagers portfolio case study"
```

Expected: one implementation commit after the test commit.

### Task 4: Run Required Verification

**Files:**
- No additional source changes expected unless verification fails.

- [ ] **Step 1: Run frontend preflight**

Run from the worktree root:

```bash
make preflight-frontend
```

Expected: PASS.

- [ ] **Step 2: Run frontend e2e preflight**

Run from the worktree root:

```bash
make preflight-e2e
```

Expected: PASS.

- [ ] **Step 3: Fix any failures with the smallest scoped patch**

If a check fails because of formatting, lint, route text, or locator ambiguity, apply the minimal file edit and rerun the failed command. Do not change unrelated portfolio pages.

- [ ] **Step 4: Commit verification fixes if needed**

If Step 3 changed files, run:

```bash
git add frontend/src/app/page.tsx frontend/src/app/galaxy/page.tsx frontend/e2e/mocked/galaxy-portfolio.spec.ts
git commit -m "fix: stabilize GalaxyVoyagers portfolio checks"
```

Expected: no commit if verification passed without edits; otherwise one focused fix commit.

### Task 5: Push And Open PR To QA

**Files:**
- No source changes.

- [ ] **Step 1: Check branch and final status**

Run:

```bash
git branch --show-current
git status --short
```

Expected: branch is `feature/galaxyvoyagers-portfolio` and no uncommitted changes remain, except ignored `.superpowers/` artifacts outside the worktree if present.

- [ ] **Step 2: Push the feature branch**

Run:

```bash
git push -u origin feature/galaxyvoyagers-portfolio
```

Expected: branch pushes successfully.

- [ ] **Step 3: Create the PR to `qa`**

Run:

```bash
gh pr create --base qa --head feature/galaxyvoyagers-portfolio --title "Add GalaxyVoyagers portfolio case study" --body "## Summary
- Add GalaxyVoyagers.com as the first portfolio card
- Add /galaxy architecture case-study route with live-site link
- Explain why GraphQL fits the connected worldbuilding domain
- Add Playwright coverage for the new landing-card flow

## Verification
- make preflight-frontend
- make preflight-e2e"
```

Expected: GitHub CLI returns the PR URL. Do not watch CI unless Kyle asks.

## Self-Review

- Spec coverage: root teaser card, `/galaxy` route, live-site link, full technology explanation, Mermaid diagram, GraphQL rationale, existing styling reuse, and required verification are all covered.
- Placeholder scan: no placeholder tokens or vague test instructions remain.
- Type consistency: the test expects headings and link text that are present in the planned page code.
