# Galaxy AWS Deployment + /aws Refresh Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rewrite the `/galaxy` route to document the GalaxyVoyagers production AWS migration (homelab k3s → EKS), and lightly refresh `/aws` so the two pages read as a deliberate, cross-linked "ephemeral demo vs. production migration" pair.

**Architecture:** Two Next.js App Router page components (`frontend/src/app/galaxy/page.tsx`, `frontend/src/app/aws/page.tsx`), plus the galaxy Playwright e2e spec. No new components — reuse `MermaidDiagram`, `next/link` `Link`, and existing Tailwind table/card/pill patterns. Docs/frontend only; no backend, k8s, or Terraform changes.

**Tech Stack:** Next.js 16 (App Router), React 19, TypeScript, Tailwind, `mermaid` via `MermaidDiagram`, Playwright (mocked e2e), `lucide-react` icons.

## Global Constraints

- **Scope:** frontend/docs only in the `gen_ai_engineer` repo. Do NOT modify `~/repos/story`, this repo's `terraform/`, or any `k8s/` manifests.
- **Honesty:** the GalaxyVoyagers AWS migration is **in progress** (apps still serve from the homelab). Frame AWS as the **target**, never as live. No present-tense claim that application workloads run on AWS.
- **Reuse:** no new components. Use `MermaidDiagram`, `next/link` `Link`, and the page-local data-array + Tailwind table/card patterns already present in these files.
- **CI gates before every commit:** run `npx tsc --noEmit` and `npm run lint` from `frontend/` (per the "run CI checks before committing" rule). The pre-commit hook also runs Frontend Type Check + Frontend Lint.
- **Verified facts (use verbatim in diagrams/copy):**
  - gRPC services & ports: `story` :50051, `chat` :50052, `stripe` :50053, `storygen` :50054, `image` :50055, `authv2` :50056; GraphQL `gateway` :4000; Stripe webhook HTTP :4003.
  - Ingress path routing: `/graphql` → gateway:4000, `/webhook` → stripe:4003.
  - GalaxyVoyagers observability deployed: Prometheus scrape annotations, Loki log queries (`docs/observability/loki-queries.md`), Grafana dashboards (`k8s/story/observability/dashboards/`). Phrase the callout as the **Prometheus / Loki / Grafana** stack — true today.
  - AWS managed targets: Aurora PostgreSQL Serverless v2 (story + auth DBs), Amazon DocumentDB (chat), ElastiCache Serverless (Valkey) for Redis, Amazon MQ for RabbitMQ, IRSA (replaces static AWS keys), AWS Secrets Manager + External Secrets Operator (replaces sealed-secrets), AWS Load Balancer Controller (ALB) + ACM + external-dns (Route 53), ghcr.io retained, Next.js frontend stays on Vercel.

---

## File Structure

- `frontend/src/app/galaxy/page.tsx` — **full rewrite** (Task 1). Owns the GalaxyVoyagers case study + production AWS migration narrative.
- `frontend/e2e/mocked/galaxy-portfolio.spec.ts` — **modify** (Task 1). Add assertions for the new AWS content + cross-links; keep existing assertions.
- `frontend/src/app/aws/page.tsx` — **light edit** (Task 2). Add scope-clarifying intro sentence + contrast cross-link to `/galaxy`.

Two independent deliverables → two tasks. Both `/galaxy` and `/aws` routes already exist, so the bidirectional cross-links have no ordering dependency.

---

### Task 1: Rewrite the `/galaxy` page

**Files:**
- Modify (full rewrite): `frontend/src/app/galaxy/page.tsx`
- Test: `frontend/e2e/mocked/galaxy-portfolio.spec.ts`

**Interfaces:**
- Consumes: `MermaidDiagram` from `@/components/MermaidDiagram` (existing); `Link` from `next/link`; `ExternalLink` from `lucide-react`.
- Produces: a `/galaxy` route with headings `"GalaxyVoyagers.com"`, `"Technology Stack"`, `"Architecture"`, `"Production AWS Migration"`, `"Why GraphQL Was The Right Boundary"`, `"Engineering Focus"`; a status pill `"Migrating to AWS · Phase 1 of 4"`; a link to `/observability`; a link to `/aws`; the existing external link to `https://galaxyvoyagers.com`.

- [ ] **Step 1: Replace the entire contents of `frontend/src/app/galaxy/page.tsx`**

```tsx
import { ExternalLink } from "lucide-react";
import Link from "next/link";

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
  "AWS",
  "EKS",
  "Karpenter",
  "Graviton (ARM64)",
  "Terraform",
  "Aurora Serverless v2",
  "DocumentDB",
  "ElastiCache (Valkey)",
  "Amazon MQ",
  "ALB",
  "Route 53",
  "ACM",
  "IRSA",
  "External Secrets",
];

const datastoreMigration = [
  {
    from: "PostgreSQL (story, auth)",
    to: "Aurora PostgreSQL Serverless v2",
    note: "One cluster, two databases; autoscales from a low ACU floor.",
  },
  {
    from: "MongoDB (chat)",
    to: "Amazon DocumentDB",
    note: "CRUD-only chat usage; TLS required on the connection.",
  },
  {
    from: "Redis",
    to: "ElastiCache Serverless (Valkey)",
    note: "Pay-per-use, scales to a low idle floor.",
  },
  {
    from: "RabbitMQ",
    to: "Amazon MQ for RabbitMQ",
    note: "Single-instance broker for cost; cluster deployment for HA later.",
  },
  {
    from: "Static AWS keys in pods",
    to: "IRSA (scoped IAM roles)",
    note: "S3 image access without long-lived credentials.",
  },
  {
    from: "sealed-secrets",
    to: "Secrets Manager + External Secrets Operator",
    note: "Synced into native Kubernetes Secrets via IRSA.",
  },
  {
    from: "nginx ingress",
    to: "ALB + ACM + external-dns",
    note: "Route 53 record for api.galaxyvoyagers.com.",
  },
];

const phases = [
  {
    name: "Phase 1 — Foundation",
    status: "In progress",
    desc: "Remote state, VPC, EKS + Graviton baseline node group, Karpenter, and core cluster add-ons (LB Controller, external-dns, External Secrets Operator, metrics-server).",
  },
  {
    name: "Phase 2 — Data layer",
    status: "Upcoming",
    desc: "Aurora, DocumentDB, ElastiCache, and Amazon MQ; Secrets Manager entries synced into the cluster by External Secrets.",
  },
  {
    name: "Phase 3 — App layer",
    status: "Upcoming",
    desc: "IRSA roles, an EKS kustomize overlay pointing services at the managed datastores, and the ALB Ingress with ACM + external-dns.",
  },
  {
    name: "Phase 4 — Cutover",
    status: "Upcoming",
    desc: "Data migration, flip the api.galaxyvoyagers.com Route 53 record to the ALB, verify, then decommission the homelab stack.",
  },
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

const awsTargetDiagram = `flowchart TD
  VERCEL[Next.js on Vercel<br/>galaxyvoyagers.com]
  R53[Route 53 + ACM<br/>api.galaxyvoyagers.com]
  ALB[AWS ALB<br/>Load Balancer Controller]
  VERCEL -->|GraphQL API| R53
  R53 --> ALB

  subgraph EKS["EKS — Graviton baseline + Karpenter spot"]
    GW[gateway<br/>gqlgen :4000]
    STORY[story<br/>gRPC :50051]
    CHAT[chat<br/>gRPC :50052]
    STRIPE[stripe<br/>gRPC :50053 · webhook :4003]
    STORYGEN[storygen<br/>gRPC :50054]
    IMAGE[image<br/>gRPC :50055]
    AUTH[authv2<br/>gRPC :50056]
    ESO[External Secrets Operator]
  end

  ALB -->|/graphql| GW
  ALB -->|/webhook| STRIPE
  GW -->|gRPC| STORY
  GW -->|gRPC| CHAT
  GW -->|gRPC| STRIPE
  GW -->|gRPC| STORYGEN
  GW -->|gRPC| IMAGE
  GW -->|gRPC| AUTH

  subgraph Managed["AWS Managed Data"]
    AURORA[(Aurora PostgreSQL<br/>Serverless v2)]
    DOCDB[(Amazon DocumentDB)]
    EC[(ElastiCache<br/>Valkey)]
    MQ{{Amazon MQ<br/>RabbitMQ}}
  end

  STORY --> AURORA
  AUTH --> AURORA
  CHAT --> DOCDB
  STORY --> EC
  AUTH --> EC
  IMAGE --> EC
  GW --> MQ
  IMAGE --> MQ

  SM[Secrets Manager] --> ESO
  ESO -. syncs secrets .-> GW
  IMAGE -->|IRSA| S3[(S3 images bucket)]
  GHCR[ghcr.io] -. pulls images .-> EKS
  STORYGEN --> OPENAI[OpenAI]
  IMAGE --> OPENAI`;

export default function GalaxyPage() {
  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <section>
        <p className="text-sm font-medium text-primary">Deployed project</p>
        <h1 className="mt-3 text-3xl font-bold">GalaxyVoyagers.com</h1>
        <div className="mt-4 inline-flex items-center gap-2 rounded-full border border-foreground/15 bg-primary/5 px-3 py-1 text-xs font-medium text-primary">
          Migrating to AWS · Phase 1 of 4
        </div>
        <p className="mt-6 text-muted-foreground leading-relaxed">
          GalaxyVoyagers is a collaborative sci-fi worldbuilding platform for
          building stories, scenes, characters, organizations, locations, ships,
          conflicts, and supporting media. It is a separate production
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
        <p className="mt-4 text-sm text-muted-foreground leading-relaxed">
          The site is served today from a self-hosted k3s homelab and is
          actively migrating to a production AWS deployment — the target
          architecture is documented below.
        </p>
        <a
          href="https://galaxyvoyagers.com"
          target="_blank"
          rel="noopener noreferrer"
          className="mt-6 inline-flex items-center gap-2 rounded-lg bg-primary px-5 py-3 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
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
          contracts between services, proven datastores selected for the access
          pattern they serve, and a Terraform-defined AWS target for production
          hosting.
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
          The target AWS architecture preserves the existing kustomize layout
          and gRPC service topology while replacing self-hosted infrastructure
          with managed services. The Next.js frontend stays on Vercel and calls
          a single GraphQL endpoint; an ALB provisioned by the AWS Load Balancer
          Controller routes <code>/graphql</code> to the gateway and{" "}
          <code>/webhook</code> to the Stripe service. EKS runs the services on
          a Graviton baseline node group, with Karpenter adding spot capacity on
          demand.
        </p>
        <div className="mt-6">
          <MermaidDiagram chart={awsTargetDiagram} />
        </div>
      </section>

      <section className="mt-12">
        <h2 className="text-2xl font-semibold">Production AWS Migration</h2>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          The migration moves GalaxyVoyagers from a self-hosted homelab onto a
          managed, autoscaling AWS deployment without rewriting application
          services. Managed EKS runs the existing manifests; Karpenter
          provisions spot capacity over a small Graviton (ARM64) on-demand
          baseline, consolidating and scaling to zero extra nodes when idle.
          ARM instances run the Go services at roughly 20% lower cost than x86.
        </p>

        <h3 className="mt-8 text-lg font-medium">Self-hosted → AWS managed</h3>
        <div className="mt-4 overflow-x-auto">
          <table className="w-full text-sm text-muted-foreground">
            <thead>
              <tr className="border-b text-left">
                <th className="pb-2 pr-4 font-medium text-foreground">
                  Today (homelab)
                </th>
                <th className="pb-2 pr-4 font-medium text-foreground">
                  AWS managed target
                </th>
                <th className="pb-2 font-medium text-foreground">Notes</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {datastoreMigration.map((row) => (
                <tr key={row.from}>
                  <td className="py-2 pr-4">{row.from}</td>
                  <td className="py-2 pr-4 text-foreground">{row.to}</td>
                  <td className="py-2">{row.note}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <h3 className="mt-8 text-lg font-medium">Migration phases</h3>
        <div className="mt-4 space-y-3">
          {phases.map((phase) => (
            <div
              key={phase.name}
              className="rounded-lg border border-foreground/10 p-4"
            >
              <div className="flex items-center justify-between gap-3">
                <h4 className="text-sm font-semibold text-foreground">
                  {phase.name}
                </h4>
                <span
                  className={
                    phase.status === "In progress"
                      ? "rounded-full bg-primary/10 px-2.5 py-0.5 text-xs font-medium text-primary"
                      : "rounded-full bg-foreground/5 px-2.5 py-0.5 text-xs font-medium text-muted-foreground"
                  }
                >
                  {phase.status}
                </span>
              </div>
              <p className="mt-2 text-xs text-muted-foreground leading-relaxed">
                {phase.desc}
              </p>
            </div>
          ))}
        </div>

        <p className="mt-6 text-sm text-muted-foreground leading-relaxed">
          For a leaner, ephemeral take on the same AWS tools — spin up the
          portfolio&apos;s own services for a demo and tear them down after —
          see the{" "}
          <Link href="/aws" className="text-primary hover:underline">
            portfolio AWS deployment
          </Link>
          .
        </p>
      </section>

      <section className="mt-12">
        <div className="rounded-xl border border-foreground/10 bg-card p-6">
          <h2 className="text-lg font-semibold">Observability</h2>
          <p className="mt-3 text-sm text-muted-foreground leading-relaxed">
            GalaxyVoyagers ships the same Prometheus / Loki / Grafana
            observability stack used across this portfolio — Prometheus scrape
            annotations on pods, Loki log queries, and Grafana dashboards for the
            story services. The approach is documented in detail in the{" "}
            <Link
              href="/observability"
              className="text-primary hover:underline"
            >
              Observability section
            </Link>
            .
          </p>
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
          entities attached to each scene, fetch media, then fetch comments. The
          GraphQL gateway moves that composition into the backend, where it can
          resolve nested fields through service calls and datastore access
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
          The project highlights production-oriented backend design and
          infrastructure: a typed GraphQL boundary, protobuf service contracts,
          separate persistence models for relational worldbuilding data and
          document-style discussion data, async job handling for expensive
          generation work, and a Terraform-defined migration onto autoscaling
          AWS managed services (EKS with Karpenter and Graviton, Aurora
          Serverless v2, DocumentDB, IRSA, and External Secrets).
        </p>
      </section>
    </div>
  );
}
```

- [ ] **Step 2: Type-check and lint**

Run: `cd frontend && npx tsc --noEmit && npm run lint`
Expected: no type errors; eslint passes with no errors.

- [ ] **Step 3: Add new assertions to the galaxy e2e spec**

In `frontend/e2e/mocked/galaxy-portfolio.spec.ts`, inside the existing test (after the `"Why GraphQL Was The Right Boundary"` heading assertion and before the closing `});` of the test), add:

```ts
    await expect(page.getByText(/Migrating to AWS · Phase 1 of 4/i)).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "Production AWS Migration" })
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: /Observability section/i })
    ).toHaveAttribute("href", "/observability");
    await expect(
      page.getByRole("link", { name: /portfolio AWS deployment/i })
    ).toHaveAttribute("href", "/aws");
```

- [ ] **Step 4: Run the galaxy e2e spec**

Run: `cd frontend && npx playwright test e2e/mocked/galaxy-portfolio.spec.ts`
Expected: 1 passed. (If Playwright browsers are not installed, run `npx playwright install` first; if the mocked suite cannot start the dev server in this environment, record that and fall back to verifying via `npm run build`.)

- [ ] **Step 5: Commit**

```bash
git add frontend/src/app/galaxy/page.tsx frontend/e2e/mocked/galaxy-portfolio.spec.ts
git commit -m "feat(galaxy): document GalaxyVoyagers production AWS migration"
```

---

### Task 2: Refresh the `/aws` page (positioning + cross-link)

**Files:**
- Modify: `frontend/src/app/aws/page.tsx`

**Interfaces:**
- Consumes: nothing new. Adds an import of `Link` from `next/link` (the file currently imports only `MermaidDiagram`).
- Produces: a scope-clarifying intro sentence and a bordered contrast callout linking to `/galaxy`.

- [ ] **Step 1: Add the `Link` import**

At the top of `frontend/src/app/aws/page.tsx`, change:

```tsx
import { MermaidDiagram } from "@/components/MermaidDiagram";
```

to:

```tsx
import Link from "next/link";

import { MermaidDiagram } from "@/components/MermaidDiagram";
```

- [ ] **Step 2: Clarify the intro paragraph scope**

In the `{/* Intro */}` section, replace this paragraph:

```tsx
        <p className="text-muted-foreground leading-relaxed">
          Every service in this portfolio runs in Kubernetes — today on a home
          server behind Cloudflare Tunnel, and optionally on AWS with EKS and
          managed services. The home server costs nothing to run. The AWS
          deployment spins up in 15 minutes with a single script and tears down
          after to keep costs near zero. Same application code, different
          infrastructure — swapped via Kustomize overlays.
        </p>
```

with:

```tsx
        <p className="text-muted-foreground leading-relaxed">
          This page covers this portfolio&apos;s own services
          (<code>ai-services</code>, <code>java-tasks</code>,{" "}
          <code>go-ecommerce</code>) and an intentionally ephemeral,
          cost-first approach to AWS. Every service runs in Kubernetes — today
          on a home server behind Cloudflare Tunnel, and optionally on AWS with
          EKS and managed services. The home server costs nothing to run. The
          AWS deployment spins up in 15 minutes with a single script and tears
          down after to keep costs near zero. Same application code, different
          infrastructure — swapped via Kustomize overlays.
        </p>
```

- [ ] **Step 3: Add the contrast cross-link callout after the intro section**

Immediately after the closing `</section>` of the `{/* Intro */}` section (and before the `{/* Current Production */}` section), insert:

```tsx
      {/* Contrast cross-link */}
      <section className="mt-6">
        <div className="rounded-xl border border-foreground/10 bg-card p-5 text-sm text-muted-foreground leading-relaxed">
          This is the lean, ephemeral approach. For a production, always-on AWS
          migration — Karpenter spot autoscaling, Graviton, Aurora Serverless
          v2, IRSA, and External Secrets — see the{" "}
          <Link href="/galaxy" className="text-primary hover:underline">
            GalaxyVoyagers project
          </Link>
          .
        </div>
      </section>
```

- [ ] **Step 4: Type-check and lint**

Run: `cd frontend && npx tsc --noEmit && npm run lint`
Expected: no type errors; eslint passes with no errors.

- [ ] **Step 5: Verify the page builds**

Run: `cd frontend && npm run build`
Expected: build succeeds; `/aws` and `/galaxy` appear in the route list with no errors.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/app/aws/page.tsx
git commit -m "docs(aws): position as ephemeral counterpart to the GalaxyVoyagers migration"
```

---

## Self-Review

**1. Spec coverage:**
- `/galaxy` rewrite (hero + status pill + honesty sentence, AWS-target tech chips, AWS-target diagram, Production AWS Migration centerpiece with datastore table + phase strip, observability callout, GraphQL section kept, engineering focus updated) → Task 1. ✓
- `/aws` light refresh (scope intro sentence + contrast cross-link, content otherwise unchanged) → Task 2. ✓
- Cross-links both directions (`/galaxy`→`/aws`, `/galaxy`→`/observability`, `/aws`→`/galaxy`) → Tasks 1 & 2. ✓
- Honesty framing (status pill, phase strip marks Phase 1 in progress, "served today from k3s homelab" sentence, no present-tense AWS claim, external button still → live site) → Task 1. ✓
- Observability claim limited to what is deployed (Prometheus/Loki/Grafana — verified) → Task 1. ✓
- Verify-items (service set/ports, observability stack) resolved in Global Constraints. ✓
- Nav unchanged; no `SiteHeader` edit → respected (no task touches it). ✓
- No backend/k8s/terraform changes → respected. ✓

**2. Placeholder scan:** No TBD/TODO/"handle edge cases". Full file content given for `/galaxy`; exact old→new strings for `/aws`; concrete test assertions and commands. ✓

**3. Type consistency:** `stack`, `datastoreMigration` (`from`/`to`/`note`), `phases` (`name`/`status`/`desc`), `graphDomains` (`title`/`desc`), `awsTargetDiagram` are all defined and consumed within Task 1's single file. `Link`/`MermaidDiagram`/`ExternalLink` imports match usage. `/aws` adds `Link` import before use. ✓

---

## Execution Handoff

Both tasks are frontend/docs edits with their own type-check + lint (+ e2e/build) gate and independent commits.
