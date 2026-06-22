# Home Page (`/`) Refresh Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Update the portfolio home page bio and project cards to reflect the current portfolio and position Kyle as a full-stack / Go-microservices / AI-integration engineer.

**Architecture:** Single-page content edit. `frontend/src/app/page.tsx` is rewritten to a new bio (skills-first, React-led) and four themed `<section>` groups of cards. The two AI cards merge into one linking to `/ai`; `/ai/page.tsx` gains a link to `/dspm` so that page is not orphaned. One Playwright assertion that checks a renamed card title is updated.

**Tech Stack:** Next.js (App Router), React, TypeScript, Tailwind, shadcn/ui `Card`, Playwright (mocked e2e).

## Global Constraints

- Reuse existing `Card` / `CardHeader` / `CardTitle` / `CardDescription` / `CardContent` components — no new component types.
- Every card is a fully-clickable `<Link href=...>` wrapping a `Card`, matching the current pattern. No nested anchors.
- All existing `href` route targets are unchanged: `/galaxy`, `/go`, `/java`, `/database`, `/async`, `/ai`, `/observability`, `/cicd`, `/aws`, `/security`.
- Project-page `<h1>`s are NOT changed; only home-page card titles change.
- Card count on the home page after this change: **10** (DSPM card merged into the AI card).
- Bio copy is Option B, verbatim (see Task 2). The existing Grafana paragraph is preserved unchanged.
- Frontend gate before any commit: `npx tsc --noEmit` passes and `npm run lint` passes (per repo convention).

---

### Task 1: Add `/dspm` reachability link on the `/ai` page

Ship this first so `/dspm` is reachable before its home-page card is removed in Task 2.

**Files:**
- Modify: `frontend/src/app/ai/page.tsx` (insert a "Related Work" section after the Debug Demo Link section, before the closing `</div>`s — currently around line 194)

**Interfaces:**
- Consumes: nothing. `Link` is already imported at `frontend/src/app/ai/page.tsx:1`.
- Produces: a visible link with `href="/dspm"` on the `/ai` page.

- [ ] **Step 1: Add the Related Work section**

In `frontend/src/app/ai/page.tsx`, immediately after the closing `</section>` of the "Debug Demo Link" block (the `<Link href="/ai/debug">…</Link>` section) and before the two closing `</div>` tags, insert:

```tsx
        {/* Related Work */}
        <section className="mt-16">
          <h2 className="text-2xl font-semibold">Related Work</h2>
          <p className="mt-4 text-muted-foreground leading-relaxed">
            For AI applied to data security, see the DSPM Classifier — a
            Kafka-scale service that detects sensitive data with a tiered
            regex &rarr; NER &rarr; LLM classification pipeline.
          </p>
          <Link
            href="/dspm"
            className="mt-6 inline-flex items-center gap-2 rounded-lg bg-primary px-6 py-3 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
          >
            View the DSPM Classifier &rarr;
          </Link>
        </section>
```

- [ ] **Step 2: Type-check**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 3: Lint**

Run: `cd frontend && npm run lint`
Expected: no errors for `src/app/ai/page.tsx`.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/app/ai/page.tsx
git commit -m "feat(ai): link to DSPM classifier as related work"
```

---

### Task 2: Rewrite the home page (bio + four themed sections + renamed/merged cards)

**Files:**
- Modify (full rewrite): `frontend/src/app/page.tsx`
- Modify: `frontend/e2e/mocked/async-page.spec.ts:56-58` (update assertion for the renamed Async card)

**Interfaces:**
- Consumes: the `/dspm` link added in Task 1 (so the removed DSPM home card stays reachable).
- Produces: a home page with `<h2>` section headings "Featured Project", "Backend & Data Engineering", "AI Systems", "Platform & Operations", and 10 cards total.

- [ ] **Step 1: Update the failing e2e assertion (red)**

The Async card title changes from "Asynchronous Systems Engineering" to "Asynchronous Systems". Update the homepage assertion in `frontend/e2e/mocked/async-page.spec.ts` (lines 56-58):

Replace:

```ts
    await expect(
      page.getByRole("link", { name: /Asynchronous Systems Engineering/ }),
    ).toHaveAttribute("href", "/async");
```

with:

```ts
    await expect(
      page.getByRole("link", { name: /Asynchronous Systems/ }),
    ).toHaveAttribute("href", "/async");
```

- [ ] **Step 2: Run the async spec to confirm it now fails (red)**

Run: `cd frontend && npx playwright test e2e/mocked/async-page.spec.ts -g "homepage and primary navigation"`
Expected: FAIL — the home page still renders the old title "Asynchronous Systems Engineering", but the test now also matches; the assertion that will fail in the next state is the title rename. (If it passes here because the regex still matches the old longer title, that is acceptable — the rewrite in Step 3 is what makes the new title authoritative. The key red→green driver is `tsc` + the full async spec passing after Step 3.)

- [ ] **Step 3: Rewrite `frontend/src/app/page.tsx`**

Replace the entire file with:

```tsx
import Link from "next/link";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@/components/ui/card";

export default function Home() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <div className="mx-auto max-w-3xl px-6 py-16">
        {/* Name & Bio */}
        <h1 className="text-4xl font-bold">Kyle Bradshaw</h1>
        <p className="mt-6 text-lg text-muted-foreground leading-relaxed">
          Full-stack engineer — React, Go and Python microservices, and LLM/RAG
          integration. Four years of experience, the last stretch spent
          consulting and building production systems independently: designing
          the APIs, shipping the frontends, and running the whole stack on
          Kubernetes. Everything below is deployed and instrumented, not a demo.
        </p>
        <p className="mt-4 text-sm text-muted-foreground leading-relaxed">
          Every service in this portfolio ships Prometheus metrics to a live{" "}
          <a
            href="https://grafana.kylebradshaw.dev/d/system-overview/system-overview?orgId=1&from=now-1h&to=now&timezone=browser"
            target="_blank"
            rel="noopener noreferrer"
            className="underline hover:text-foreground transition-colors"
          >
            Grafana dashboard
          </a>
          .
        </p>

        {/* Featured Project */}
        <h2 className="mt-16 text-2xl font-semibold">Featured Project</h2>
        <div className="mt-6 grid gap-4">
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
        </div>

        {/* Backend & Data Engineering */}
        <h2 className="mt-16 text-2xl font-semibold">
          Backend &amp; Data Engineering
        </h2>
        <div className="mt-6 grid gap-4">
          <Link href="/go" className="block">
            <Card className="hover:ring-foreground/20 transition-all">
              <CardHeader>
                <CardTitle>Go Ecommerce Platform</CardTitle>
                <CardDescription>
                  Microservices ecommerce platform built with Go, PostgreSQL,
                  Redis, and RabbitMQ
                </CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-muted-foreground text-sm">
                  Microservices architecture with JWT authentication, product
                  catalog, cart, orders, and asynchronous worker pools —
                  deployed on Kubernetes.
                </p>
              </CardContent>
            </Card>
          </Link>
          <Link href="/java" className="block">
            <Card className="hover:ring-foreground/20 transition-all">
              <CardHeader>
                <CardTitle>Full-Stack Java</CardTitle>
                <CardDescription>
                  Task Management System built with Spring Boot, GraphQL, and
                  Kubernetes
                </CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-muted-foreground text-sm">
                  Microservices architecture with PostgreSQL, MongoDB, Redis,
                  RabbitMQ, Google OAuth, and CI/CD automation.
                </p>
              </CardContent>
            </Card>
          </Link>
          <Link href="/database" className="block">
            <Card className="hover:ring-foreground/20 transition-all">
              <CardHeader>
                <CardTitle>Database Engineering</CardTitle>
                <CardDescription>
                  Production PostgreSQL — pooling, replication, optimization,
                  partitioning, migration safety, and reliability
                </CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-muted-foreground text-sm">
                  Real benchmarks against PostgreSQL 16, transaction-mode
                  PgBouncer pooling, an async streaming read replica with a
                  separate reporting pool, range partitioning with materialized
                  views, a custom AST-based migration linter, and verified
                  point-in-time recovery.
                </p>
              </CardContent>
            </Card>
          </Link>
          <Link href="/async" className="block">
            <Card className="hover:ring-foreground/20 transition-all">
              <CardHeader>
                <CardTitle>Asynchronous Systems</CardTitle>
                <CardDescription>
                  Go ecommerce messaging with Kafka event streams, RabbitMQ
                  sagas, DLQs, replay, and production observability
                </CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-muted-foreground text-sm">
                  Checkout saga command/reply queues, bounded retries, publisher
                  confirms, reconnect-aware RabbitMQ publishing, Kafka-backed
                  order events, CQRS projection, streaming analytics, DLQ
                  envelopes, and traceable recovery paths.
                </p>
              </CardContent>
            </Card>
          </Link>
        </div>

        {/* AI Systems */}
        <h2 className="mt-16 text-2xl font-semibold">AI Systems</h2>
        <div className="mt-6 grid gap-4">
          <Link href="/ai" className="block">
            <Card className="hover:ring-foreground/20 transition-all">
              <CardHeader>
                <CardTitle>Document Q&amp;A Assistant</CardTitle>
                <CardDescription>
                  A RAG document assistant plus a Kafka-scale sensitive-data
                  classifier — retrieval-augmented generation and applied LLM
                  classification
                </CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-muted-foreground text-sm">
                  A full-stack retrieval-augmented generation system (FastAPI,
                  Qdrant, Ollama) for PDF Q&amp;A, plus a DSPM classifier that
                  detects sensitive data at Kafka scale with a tiered regex
                  &rarr; NER &rarr; LLM pipeline. Explore both from the AI page.
                </p>
              </CardContent>
            </Card>
          </Link>
        </div>

        {/* Platform & Operations */}
        <h2 className="mt-16 text-2xl font-semibold">
          Platform &amp; Operations
        </h2>
        <div className="mt-6 grid gap-4">
          <Link href="/observability" className="block">
            <Card className="hover:ring-foreground/20 transition-all">
              <CardHeader>
                <CardTitle>Observability</CardTitle>
                <CardDescription>
                  Production-journey instrumentation — Prometheus metrics, Loki
                  logs, Jaeger traces, and live alerting
                </CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-muted-foreground text-sm">
                  Three-pillar stack with deploy annotations, Kubernetes event
                  exporter, gRPC client interceptors, saga-stalled alerts, and
                  Kafka-header trace propagation across the async boundary.
                </p>
              </CardContent>
            </Card>
          </Link>
          <Link href="/cicd" className="block">
            <Card className="hover:ring-foreground/20 transition-all">
              <CardHeader>
                <CardTitle>CI/CD Pipeline</CardTitle>
                <CardDescription>
                  Unified GitHub Actions workflow with a live QA environment at
                  qa.kylebradshaw.dev for pre-prod review
                </CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-muted-foreground text-sm">
                  A single workflow handles quality checks, image builds, and
                  deployments for three service stacks — designed for a solo
                  developer with automated spec-to-production delivery. See
                  what&apos;s currently staged for production review on the
                  CI/CD page.
                </p>
              </CardContent>
            </Card>
          </Link>
          <Link href="/aws" className="block">
            <Card className="hover:ring-foreground/20 transition-all">
              <CardHeader>
                <CardTitle>Infrastructure &amp; Deployment</CardTitle>
                <CardDescription>
                  Production Kubernetes on a home server, AWS-ready with
                  Terraform and EKS
                </CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-muted-foreground text-sm">
                  Two deployment architectures for the same services — a
                  cost-effective Minikube cluster with Cloudflare Tunnel serving
                  production today, and a one-command AWS deployment with EKS,
                  RDS, ElastiCache, and Amazon MQ.
                </p>
              </CardContent>
            </Card>
          </Link>
          <Link href="/security" className="block">
            <Card className="hover:ring-foreground/20 transition-all">
              <CardHeader>
                <CardTitle>Security</CardTitle>
                <CardDescription>
                  Defense-in-depth across the stack — application, CI/CD,
                  Kubernetes, and the hardened Linux host that runs it all
                </CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-muted-foreground text-sm">
                  Six CI security gates, JWT + httpOnly cookies, pod security
                  contexts, Sealed Secrets for GitOps-friendly secret
                  management, UFW default-deny firewall, Tailscale-only SSH,
                  auditd, sysctl hardening, and a lynis baseline score of 77.
                </p>
              </CardContent>
            </Card>
          </Link>
        </div>
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Type-check (green gate)**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 5: Lint**

Run: `cd frontend && npm run lint`
Expected: no errors for `src/app/page.tsx`.

- [ ] **Step 6: Run the async e2e spec (green)**

Run: `cd frontend && npx playwright test e2e/mocked/async-page.spec.ts`
Expected: PASS — all three tests, including "homepage and primary navigation link to /async", which now matches the renamed "Asynchronous Systems" card.

- [ ] **Step 7: Visual confirmation**

Run: `cd frontend && npm run dev`, open `http://localhost:3000/`, and confirm:
- Bio reads the new Option B copy; Grafana link still present.
- Four section headings appear in order: Featured Project, Backend & Data Engineering, AI Systems, Platform & Operations.
- 10 cards total, each present once; every card navigates to its route.
- The AI Systems card navigates to `/ai`; `/ai` shows a "Related Work" link to `/dspm`.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/app/page.tsx frontend/e2e/mocked/async-page.spec.ts
git commit -m "feat(home): refresh bio and group project cards into themed sections"
```

---

## Self-Review

**Spec coverage:**
- Bio Option B → Task 2 Step 3 (verbatim). ✓
- Four themed sections in order → Task 2 Step 3. ✓
- Card titles de-job-titled (Go, Java, Async, AI) → Task 2 Step 3. ✓
- AI cards merged to one linking to `/ai` → Task 2 Step 3. ✓
- `/dspm` reachability via `/ai` → Task 1. ✓
- async e2e assertion updated → Task 2 Steps 1-2, 6. ✓
- Project-page `<h1>`s untouched (non-goal) → respected; only `/ai` gains a link, no `<h1>` change. ✓

**Placeholder scan:** No TBD/TODO; all code shown in full. ✓

**Type/name consistency:** All cards use the same `Card`/`CardHeader`/`CardTitle`/`CardDescription`/`CardContent` imports as the current file; `Link` import unchanged. Route hrefs all match existing routes. ✓
