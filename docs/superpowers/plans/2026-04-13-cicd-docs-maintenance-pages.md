# CI/CD Documentation Page & Maintenance Pages Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add health-check maintenance screens to all interactive demo pages and a CI/CD pipeline documentation page to the portfolio site.

**Architecture:** A shared `<HealthGate>` client component checks a health endpoint on mount and renders either the demo content or a maintenance screen. The CI/CD page is a static documentation page using the same layout pattern as the existing `/ai`, `/java`, `/go` overview pages. The site header navigation is reordered to match the homepage card order.

**Tech Stack:** Next.js, TypeScript, React, shadcn/ui, Mermaid (diagrams)

**Spec:** `docs/superpowers/specs/2026-04-13-cicd-docs-maintenance-pages-design.md`

---

## File Structure

### New Files

| File | Responsibility |
|------|---------------|
| `frontend/src/components/HealthGate.tsx` | Health-check wrapper: checks endpoint on mount, renders children or maintenance screen |
| `frontend/src/app/cicd/page.tsx` | CI/CD pipeline documentation page with diagrams and code snippets |

### Modified Files

| File | Change |
|------|--------|
| `frontend/src/components/SiteHeader.tsx` | Reorder nav links (Go, AWS, Java, AI, CI/CD), add CI/CD link |
| `frontend/src/app/ai/rag/page.tsx` | Wrap content with HealthGate |
| `frontend/src/app/ai/debug/page.tsx` | Wrap content with HealthGate |
| `frontend/src/app/java/tasks/layout.tsx` | Wrap content with HealthGate |
| `frontend/src/app/java/dashboard/page.tsx` | Wrap content with HealthGate |
| `frontend/src/app/go/ecommerce/layout.tsx` | Wrap content with HealthGate |
| `frontend/src/app/go/login/page.tsx` | Wrap content with HealthGate |
| `frontend/src/app/go/register/page.tsx` | Wrap content with HealthGate |

---

## Task 1: Create HealthGate Component

**Files:**
- Create: `frontend/src/components/HealthGate.tsx`

- [ ] **Step 1: Create the HealthGate component**

```tsx
// frontend/src/components/HealthGate.tsx
"use client";

import { useEffect, useState } from "react";
import Link from "next/link";

interface HealthGateProps {
  endpoint: string;
  stack: string;
  docsHref: string;
  children: React.ReactNode;
}

export function HealthGate({ endpoint, stack, docsHref, children }: HealthGateProps) {
  const [status, setStatus] = useState<"checking" | "healthy" | "unhealthy">("checking");

  useEffect(() => {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 3000);

    fetch(endpoint, { signal: controller.signal })
      .then((res) => {
        setStatus(res.ok ? "healthy" : "unhealthy");
      })
      .catch(() => {
        setStatus("unhealthy");
      })
      .finally(() => {
        clearTimeout(timeout);
      });

    return () => {
      controller.abort();
      clearTimeout(timeout);
    };
  }, [endpoint]);

  if (status === "checking") {
    return (
      <div className="flex min-h-[60vh] items-center justify-center bg-background">
        <div className="h-6 w-6 animate-spin rounded-full border-2 border-muted-foreground border-t-transparent" />
      </div>
    );
  }

  if (status === "unhealthy") {
    const message =
      process.env.NEXT_PUBLIC_MAINTENANCE_MESSAGE ||
      "The backend services are currently offline for maintenance. Please check back later.";

    return (
      <div className="flex min-h-[60vh] items-center justify-center bg-background px-6">
        <div className="max-w-md text-center">
          <div className="text-5xl">🔧</div>
          <h2 className="mt-4 text-2xl font-bold text-foreground">
            Server Maintenance
          </h2>
          <p className="mt-3 text-muted-foreground">{message}</p>
          <div className="mt-6 rounded-lg border border-border bg-muted/50 px-4 py-3 text-sm text-muted-foreground">
            <strong className="text-foreground">{stack}</strong> is currently
            unavailable.
          </div>
          <Link
            href={docsHref}
            className="mt-6 inline-block text-sm text-primary hover:underline"
          >
            &larr; View documentation instead
          </Link>
        </div>
      </div>
    );
  }

  return <>{children}</>;
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit 2>&1 | head -20`

Expected: No errors related to HealthGate.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/HealthGate.tsx
git commit -m "feat: add HealthGate component for backend health checks"
```

---

## Task 2: Wrap AI Demo Pages with HealthGate

**Files:**
- Modify: `frontend/src/app/ai/rag/page.tsx`
- Modify: `frontend/src/app/ai/debug/page.tsx`

- [ ] **Step 1: Wrap the RAG demo page**

In `frontend/src/app/ai/rag/page.tsx`, add the import at the top (after the existing imports):

```tsx
import { HealthGate } from "@/components/HealthGate";
```

Then wrap the return JSX. The current return starts with:
```tsx
return (
    <div className="flex h-screen flex-col bg-background text-foreground">
```

Change the return to:
```tsx
  const chatHealthUrl = `${apiUrl}/chat/health`;

  return (
    <HealthGate endpoint={chatHealthUrl} stack="Python AI Services" docsHref="/ai">
      <div className="flex h-screen flex-col bg-background text-foreground">
```

And add the closing `</HealthGate>` before the final `);`:
```tsx
      </div>
    </HealthGate>
  );
```

- [ ] **Step 2: Wrap the Debug demo page**

In `frontend/src/app/ai/debug/page.tsx`, add the import:

```tsx
import { HealthGate } from "@/components/HealthGate";
```

The debug page's return starts with a containing `<div>`. Wrap similarly:

```tsx
  const debugHealthUrl = `${apiUrl}/debug/health`;

  return (
    <HealthGate endpoint={debugHealthUrl} stack="Python AI Services" docsHref="/ai">
```

And close `</HealthGate>` before the final `);`.

- [ ] **Step 3: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit 2>&1 | head -20`

Expected: No errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/app/ai/rag/page.tsx frontend/src/app/ai/debug/page.tsx
git commit -m "feat: add health gate to AI demo pages"
```

---

## Task 3: Wrap Java Demo Pages with HealthGate

**Files:**
- Modify: `frontend/src/app/java/tasks/layout.tsx`
- Modify: `frontend/src/app/java/dashboard/page.tsx`

- [ ] **Step 1: Wrap the Java tasks layout**

The Java tasks layout at `frontend/src/app/java/tasks/layout.tsx` currently wraps children with a `JavaSubHeader`. Adding HealthGate here covers all task sub-pages (task list, project detail, task detail, reset-password).

This layout is currently a server component (no "use client" directive). HealthGate is a client component, so the layout needs to become a client component OR we wrap children in a client component. The simplest approach: add "use client" and import HealthGate.

Replace the entire file content:

```tsx
"use client";

import { JavaSubHeader } from "@/components/java/JavaSubHeader";
import { HealthGate } from "@/components/HealthGate";

const gatewayUrl =
  process.env.NEXT_PUBLIC_GATEWAY_URL || "http://localhost:8080";

export default function JavaTasksLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <HealthGate
      endpoint={`${gatewayUrl}/actuator/health`}
      stack="Java Task Management"
      docsHref="/java"
    >
      <JavaSubHeader />
      {children}
    </HealthGate>
  );
}
```

- [ ] **Step 2: Wrap the Java dashboard page**

The dashboard page at `frontend/src/app/java/dashboard/page.tsx` is a server component that renders a Suspense boundary around `DashboardClient`. Replace the entire file:

```tsx
"use client";

import { Suspense } from "react";
import { HealthGate } from "@/components/HealthGate";
import { DashboardClient } from "./dashboard-client";

const gatewayUrl =
  process.env.NEXT_PUBLIC_GATEWAY_URL || "http://localhost:8080";

export default function DashboardPage() {
  return (
    <HealthGate
      endpoint={`${gatewayUrl}/actuator/health`}
      stack="Java Task Management"
      docsHref="/java"
    >
      <Suspense
        fallback={
          <div className="p-6 text-sm text-muted-foreground">Loading…</div>
        }
      >
        <DashboardClient />
      </Suspense>
    </HealthGate>
  );
}
```

- [ ] **Step 3: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit 2>&1 | head -20`

Expected: No errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/app/java/tasks/layout.tsx frontend/src/app/java/dashboard/page.tsx
git commit -m "feat: add health gate to Java demo pages"
```

---

## Task 4: Wrap Go Demo Pages with HealthGate

**Files:**
- Modify: `frontend/src/app/go/ecommerce/layout.tsx`
- Modify: `frontend/src/app/go/login/page.tsx`
- Modify: `frontend/src/app/go/register/page.tsx`

- [ ] **Step 1: Wrap the Go ecommerce layout**

The Go ecommerce layout at `frontend/src/app/go/ecommerce/layout.tsx` currently renders GoSubHeader, children, and AiAssistantDrawer. It's a server component. Replace the entire file:

```tsx
"use client";

import { GoSubHeader } from "@/components/go/GoSubHeader";
import { AiAssistantDrawer } from "@/components/go/AiAssistantDrawer";
import { HealthGate } from "@/components/HealthGate";

const goEcommerceUrl =
  process.env.NEXT_PUBLIC_GO_ECOMMERCE_URL || "http://localhost:8092";

export default function GoEcommerceLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <HealthGate
      endpoint={`${goEcommerceUrl}/health`}
      stack="Go Ecommerce"
      docsHref="/go"
    >
      <GoSubHeader />
      {children}
      <AiAssistantDrawer />
    </HealthGate>
  );
}
```

- [ ] **Step 2: Wrap the Go login page**

In `frontend/src/app/go/login/page.tsx`, add the import after the existing imports:

```tsx
import { HealthGate } from "@/components/HealthGate";
```

The page renders a `<Suspense>` wrapper around `<GoLoginPageInner />`. Wrap the Suspense in HealthGate. Find the return in the `GoLoginPage` function:

```tsx
export default function GoLoginPage() {
  return (
    <Suspense fallback={<div className="mx-auto max-w-sm px-6 py-12">Loading…</div>}>
      <GoLoginPageInner />
    </Suspense>
  );
}
```

Replace with:

```tsx
const goAuthUrl =
  process.env.NEXT_PUBLIC_GO_AUTH_URL || "http://localhost:8091";

export default function GoLoginPage() {
  return (
    <HealthGate
      endpoint={`${goAuthUrl}/health`}
      stack="Go Ecommerce"
      docsHref="/go"
    >
      <Suspense fallback={<div className="mx-auto max-w-sm px-6 py-12">Loading…</div>}>
        <GoLoginPageInner />
      </Suspense>
    </HealthGate>
  );
}
```

Note: move the `const goAuthUrl` declaration OUTSIDE the component function, at module level (before the function definition).

- [ ] **Step 3: Wrap the Go register page**

In `frontend/src/app/go/register/page.tsx`, add the import:

```tsx
import { HealthGate } from "@/components/HealthGate";
```

The page directly renders the form. Wrap the return. Find the return in `GoRegisterPage`:

Add a module-level constant before the component:

```tsx
const goAuthUrl =
  process.env.NEXT_PUBLIC_GO_AUTH_URL || "http://localhost:8091";
```

Then wrap the component's return with HealthGate. The current return starts with a `<div>`. Change to:

```tsx
  return (
    <HealthGate
      endpoint={`${goAuthUrl}/health`}
      stack="Go Ecommerce"
      docsHref="/go"
    >
      <div className="...">
        {/* existing content */}
      </div>
    </HealthGate>
  );
```

- [ ] **Step 4: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit 2>&1 | head -20`

Expected: No errors.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/app/go/ecommerce/layout.tsx frontend/src/app/go/login/page.tsx frontend/src/app/go/register/page.tsx
git commit -m "feat: add health gate to Go demo pages"
```

---

## Task 5: Reorder SiteHeader Navigation and Add CI/CD Link

**Files:**
- Modify: `frontend/src/components/SiteHeader.tsx`

- [ ] **Step 1: Reorder nav links and add CI/CD**

Replace the `<nav>` block in `frontend/src/components/SiteHeader.tsx` (lines 25-37):

```tsx
          <nav className="flex items-center gap-4">
            <Link href="/ai" className={navLinkClass("/ai")}>
              AI
            </Link>
            <Link href="/java" className={navLinkClass("/java")}>
              Java
            </Link>
            <Link href="/go" className={navLinkClass("/go")}>
              Go
            </Link>
            <Link href="/aws" className={navLinkClass("/aws")}>
              AWS
            </Link>
          </nav>
```

With:

```tsx
          <nav className="flex items-center gap-4">
            <Link href="/go" className={navLinkClass("/go")}>
              Go
            </Link>
            <Link href="/aws" className={navLinkClass("/aws")}>
              AWS
            </Link>
            <Link href="/java" className={navLinkClass("/java")}>
              Java
            </Link>
            <Link href="/ai" className={navLinkClass("/ai")}>
              AI
            </Link>
            <Link href="/cicd" className={navLinkClass("/cicd")}>
              CI/CD
            </Link>
          </nav>
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit 2>&1 | head -20`

Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/SiteHeader.tsx
git commit -m "feat: reorder nav to match homepage, add CI/CD link"
```

---

## Task 6: Create CI/CD Pipeline Documentation Page

**Files:**
- Create: `frontend/src/app/cicd/page.tsx`

- [ ] **Step 1: Create the CI/CD documentation page**

This is a large static page following the same pattern as `/ai/page.tsx` — Mermaid diagrams, prose sections, and code snippets. Create `frontend/src/app/cicd/page.tsx`:

```tsx
import { MermaidDiagram } from "@/components/MermaidDiagram";

const pipelineFlowDiagram = `flowchart LR
  subgraph PR["Pull Request to qa"]
    direction LR
    A[PR Created] --> B[Quality Checks]
    B --> C{All Pass?}
    C -->|Yes| D[Ready for Review]
    C -->|No| E[Fix & Push]
    E --> B
  end

  subgraph QA["Push to qa"]
    direction LR
    F[PR Merged] --> G[Quality Checks]
    G --> H[Build Images]
    H --> I[Deploy to QA]
    I --> J[Smoke Tests]
  end

  subgraph Prod["Push to main"]
    direction LR
    K[Ship It] --> L[Quality Checks]
    L --> M[Build Images]
    M --> N[Deploy to Prod]
    N --> O[Smoke Tests]
  end
`;

const qaArchitectureDiagram = `flowchart TB
  subgraph Minikube["Minikube Cluster"]
    subgraph ProdNS["Production Namespaces"]
      direction LR
      P1[ai-services]
      P2[java-tasks]
      P3[go-ecommerce]
      P4[monitoring]
    end
    subgraph QANS["QA Namespaces"]
      direction LR
      Q1[ai-services-qa]
      Q2[java-tasks-qa]
      Q3[go-ecommerce-qa]
    end
  end

  CF1[api.kylebradshaw.dev] --> P1
  CF2[qa-api.kylebradshaw.dev] --> Q1

  QANS -.->|shared infra| ProdNS
`;

export default function CICDPage() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <div className="mx-auto max-w-3xl px-6 py-12">
        <h1 className="mt-8 text-3xl font-bold">CI/CD Pipeline</h1>

        {/* Overview */}
        <section className="mt-8">
          <p className="text-muted-foreground leading-relaxed">
            A unified GitHub Actions pipeline built for a solo developer. One
            workflow file handles all quality checks, image builds, and
            deployments for three service stacks (Python, Java, Go) and a Next.js
            frontend. Designed to automate everything from code push to production
            deploy, with a QA environment for visual inspection before shipping.
          </p>
        </section>

        {/* Pipeline Flow */}
        <section className="mt-12">
          <h2 className="text-xl font-semibold">Pipeline Flow</h2>
          <p className="mt-2 text-sm text-muted-foreground">
            Three triggers, one workflow. Every code change follows the same path
            through quality gates before reaching production.
          </p>
          <div className="mt-4">
            <MermaidDiagram chart={pipelineFlowDiagram} />
          </div>
        </section>

        {/* Why Unified */}
        <section className="mt-12">
          <h2 className="text-xl font-semibold">Why a Unified Workflow</h2>
          <p className="mt-2 text-muted-foreground leading-relaxed">
            The project originally had three separate workflow files — one per
            language stack. Each ran its own lint, test, and build jobs. For a
            solo developer, this created unnecessary complexity: three files to
            maintain, three sets of CI status checks to monitor, and path-based
            filtering that occasionally skipped checks when cross-stack changes
            were made.
          </p>
          <p className="mt-4 text-muted-foreground leading-relaxed">
            Consolidating into a single workflow simplified everything. All
            quality gates run unconditionally on every trigger — no path
            filtering, no change detection for quality checks. This is slower
            (every push runs all checks regardless of what changed) but simpler
            and catches cross-stack issues that path filtering would miss.
          </p>
        </section>

        {/* Trigger Matrix */}
        <section className="mt-12">
          <h2 className="text-xl font-semibold">Trigger Matrix</h2>
          <div className="mt-4 overflow-x-auto">
            <table className="w-full text-sm border-collapse">
              <thead>
                <tr className="border-b border-border">
                  <th className="py-2 pr-4 text-left font-medium">Job</th>
                  <th className="py-2 px-4 text-center font-medium">
                    PR to qa
                  </th>
                  <th className="py-2 px-4 text-center font-medium">
                    Push to qa
                  </th>
                  <th className="py-2 px-4 text-center font-medium">
                    Push to main
                  </th>
                </tr>
              </thead>
              <tbody className="text-muted-foreground">
                <tr className="border-b border-border/50">
                  <td className="py-2 pr-4">Quality checks</td>
                  <td className="py-2 px-4 text-center">✓</td>
                  <td className="py-2 px-4 text-center">✓</td>
                  <td className="py-2 px-4 text-center">✓</td>
                </tr>
                <tr className="border-b border-border/50">
                  <td className="py-2 pr-4">Build images</td>
                  <td className="py-2 px-4 text-center">—</td>
                  <td className="py-2 px-4 text-center">✓</td>
                  <td className="py-2 px-4 text-center">✓</td>
                </tr>
                <tr className="border-b border-border/50">
                  <td className="py-2 pr-4">Deploy</td>
                  <td className="py-2 px-4 text-center">—</td>
                  <td className="py-2 px-4 text-center">QA</td>
                  <td className="py-2 px-4 text-center">Prod</td>
                </tr>
                <tr>
                  <td className="py-2 pr-4">Smoke tests</td>
                  <td className="py-2 px-4 text-center">—</td>
                  <td className="py-2 px-4 text-center">✓</td>
                  <td className="py-2 px-4 text-center">✓</td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>

        {/* Quality Gates */}
        <section className="mt-12">
          <h2 className="text-xl font-semibold">Quality Gates</h2>
          <p className="mt-2 text-sm text-muted-foreground">
            22 parallel jobs run on every trigger. All must pass before images are
            built.
          </p>
          <div className="mt-4 grid gap-3">
            <div className="rounded-lg border border-border p-4">
              <h3 className="text-sm font-medium">Python</h3>
              <p className="mt-1 text-sm text-muted-foreground">
                Ruff lint + format, pytest with coverage (ingestion, chat, debug),
                Bandit SAST, pip-audit
              </p>
            </div>
            <div className="rounded-lg border border-border p-4">
              <h3 className="text-sm font-medium">Java</h3>
              <p className="mt-1 text-sm text-muted-foreground">
                Checkstyle, unit tests (4 services), integration tests with
                Testcontainers
              </p>
            </div>
            <div className="rounded-lg border border-border p-4">
              <h3 className="text-sm font-medium">Go</h3>
              <p className="mt-1 text-sm text-muted-foreground">
                golangci-lint, go test -race (3 services), migration pipeline test
              </p>
            </div>
            <div className="rounded-lg border border-border p-4">
              <h3 className="text-sm font-medium">Frontend</h3>
              <p className="mt-1 text-sm text-muted-foreground">
                ESLint, TypeScript type check, Next.js build, npm audit
              </p>
            </div>
            <div className="rounded-lg border border-border p-4">
              <h3 className="text-sm font-medium">Security</h3>
              <p className="mt-1 text-sm text-muted-foreground">
                Gitleaks (secrets), Hadolint (Dockerfiles), CORS guardrail (no
                wildcard origins)
              </p>
            </div>
            <div className="rounded-lg border border-border p-4">
              <h3 className="text-sm font-medium">Infrastructure</h3>
              <p className="mt-1 text-sm text-muted-foreground">
                K8s manifest validation (kubeconform + kind dry-run), Grafana
                dashboard sync, Compose smoke test
              </p>
            </div>
          </div>
        </section>

        {/* QA Environment */}
        <section className="mt-12">
          <h2 className="text-xl font-semibold">QA Environment</h2>
          <p className="mt-2 text-muted-foreground leading-relaxed">
            QA runs in the same Minikube cluster as production using separate
            Kubernetes namespaces. Kustomize overlays patch the base manifests to
            set QA-specific CORS origins, database names, and ingress hosts —
            without duplicating the manifests themselves.
          </p>
          <div className="mt-4">
            <MermaidDiagram chart={qaArchitectureDiagram} />
          </div>
          <div className="mt-4 overflow-x-auto">
            <table className="w-full text-sm border-collapse">
              <thead>
                <tr className="border-b border-border">
                  <th className="py-2 pr-4 text-left font-medium">
                    Production
                  </th>
                  <th className="py-2 px-4 text-left font-medium">QA</th>
                </tr>
              </thead>
              <tbody className="text-muted-foreground">
                <tr className="border-b border-border/50">
                  <td className="py-2 pr-4">ai-services</td>
                  <td className="py-2 px-4">ai-services-qa</td>
                </tr>
                <tr className="border-b border-border/50">
                  <td className="py-2 pr-4">java-tasks</td>
                  <td className="py-2 px-4">java-tasks-qa</td>
                </tr>
                <tr>
                  <td className="py-2 pr-4">go-ecommerce</td>
                  <td className="py-2 px-4">go-ecommerce-qa</td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>

        {/* Image Tagging */}
        <section className="mt-12">
          <h2 className="text-xl font-semibold">Image Tagging</h2>
          <p className="mt-2 text-muted-foreground leading-relaxed">
            All 10 service images are built in a single matrix job and pushed to
            GitHub Container Registry. QA images use a commit-pinned tag for
            traceability; production uses <code>:latest</code>.
          </p>
          <pre className="mt-4 overflow-x-auto rounded-lg border border-border bg-muted/50 p-4 text-sm">
{`# QA (push to qa branch)
ghcr.io/kabradshaw1/portfolio/ingestion:qa-abc1234

# Production (push to main branch)
ghcr.io/kabradshaw1/portfolio/ingestion:latest`}
          </pre>
        </section>

        {/* Deploy Mechanism */}
        <section className="mt-12">
          <h2 className="text-xl font-semibold">Deploy Mechanism</h2>
          <p className="mt-2 text-muted-foreground leading-relaxed">
            GitHub Actions joins a Tailscale VPN to reach the home server, then
            deploys via SSH. Kustomize overlays are built on the runner and piped
            to the server via <code>kubectl apply</code>.
          </p>
          <pre className="mt-4 overflow-x-auto rounded-lg border border-border bg-muted/50 p-4 text-sm">
{`# CI runner joins Tailscale VPN
- uses: tailscale/github-action@v3

# Build overlay locally, apply remotely
kubectl kustomize k8s/overlays/qa/ | \\
  ssh PC@100.79.113.84 "kubectl apply -f -"

# Restart deployments to pull new images
ssh PC@100.79.113.84 \\
  "kubectl rollout restart deployment -n ai-services-qa"`}
          </pre>
        </section>

        {/* No Branch Protection */}
        <section className="mt-12">
          <h2 className="text-xl font-semibold">Why No Branch Protection</h2>
          <p className="mt-2 text-muted-foreground leading-relaxed">
            This is a solo developer project. By the time code reaches{" "}
            <code>main</code>, it has passed all quality checks on the PR, been
            deployed to QA, and been visually inspected. Branch protection
            requiring PR approval would mean one person approving their own PR —
            ceremony with no value.
          </p>
          <p className="mt-4 text-muted-foreground leading-relaxed">
            The real protection is the CI pipeline itself: 22 quality gates that
            run on every push. If any fail, the deploy doesn&apos;t happen.
          </p>
        </section>

        {/* Agent Automation */}
        <section className="mt-12">
          <h2 className="text-xl font-semibold">Agent Automation</h2>
          <p className="mt-2 text-muted-foreground leading-relaxed">
            Claude Code agents drive the development workflow from spec to
            production. The pipeline is designed so agents can operate
            autonomously through the QA stage, with human review at two points:
            PR approval and the &ldquo;ship it&rdquo; command.
          </p>
          <div className="mt-4 rounded-lg border border-border p-4">
            <ol className="space-y-2 text-sm text-muted-foreground">
              <li>
                <strong className="text-foreground">1. Spec &rarr; Plan:</strong>{" "}
                Agent brainstorms design, writes implementation plan
              </li>
              <li>
                <strong className="text-foreground">
                  2. Implement &rarr; PR:
                </strong>{" "}
                Agent creates feature branch, implements, pushes, creates PR to{" "}
                <code>qa</code>
              </li>
              <li>
                <strong className="text-foreground">3. CI Watch:</strong> Agent
                monitors CI, fixes lint/format/config failures autonomously
              </li>
              <li>
                <strong className="text-foreground">4. QA Deploy:</strong> Kyle
                reviews PR, merges. QA deploys automatically.
              </li>
              <li>
                <strong className="text-foreground">5. Ship It:</strong> Kyle
                inspects QA, tells agent to ship. Agent merges to main, watches
                prod deploy, cleans up.
              </li>
            </ol>
          </div>
        </section>

        {/* Smoke Tests */}
        <section className="mt-12 mb-16">
          <h2 className="text-xl font-semibold">Smoke Tests</h2>
          <p className="mt-2 text-muted-foreground leading-relaxed">
            After every deployment, automated smoke tests verify the services are
            healthy. QA runs health endpoint checks against{" "}
            <code>qa-api.kylebradshaw.dev</code>. Production runs Playwright
            tests against the live site — including an end-to-end RAG flow that
            uploads a PDF, asks a question, and verifies a streamed response.
          </p>
        </section>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Verify TypeScript compiles and the page builds**

Run: `cd frontend && npx tsc --noEmit 2>&1 | head -20`

Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/app/cicd/page.tsx
git commit -m "feat: add CI/CD pipeline documentation page"
```

---

## Task 7: Add CI/CD Card to Homepage

**Files:**
- Modify: `frontend/src/app/page.tsx`

- [ ] **Step 1: Add CI/CD card after the AI card**

In `frontend/src/app/page.tsx`, find the closing `</Link>` of the AI Engineer card (around line 111). After it, before the closing `</div>` of the grid, add:

```tsx
          <Link href="/cicd" className="block">
            <Card className="hover:ring-foreground/20 transition-all">
              <CardHeader>
                <CardTitle>CI/CD Pipeline</CardTitle>
                <CardDescription>
                  Unified GitHub Actions workflow with QA environment and agent
                  automation
                </CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-muted-foreground text-sm">
                  A single workflow handles quality checks, image builds, and
                  deployments for three service stacks — designed for a solo
                  developer with automated spec-to-production delivery.
                </p>
              </CardContent>
            </Card>
          </Link>
```

- [ ] **Step 2: Verify build**

Run: `cd frontend && npx tsc --noEmit 2>&1 | head -20`

Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/app/page.tsx
git commit -m "feat: add CI/CD card to homepage"
```

---

## Task 8: Preflight Validation

- [ ] **Step 1: Run full frontend preflight**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer && make preflight-frontend`

Expected: lint, tsc, and build all pass.

- [ ] **Step 2: Fix any issues found**

If lint or type errors appear, fix them and commit.

- [ ] **Step 3: Add Vercel env var**

Run:
```bash
printf 'Migrating to Linux for improved performance and reliability.' | vercel env add NEXT_PUBLIC_MAINTENANCE_MESSAGE production
```

Expected: Environment variable added.

- [ ] **Step 4: Verify git log**

Run: `git log --oneline main..HEAD`

Expected: 7 commits covering HealthGate component, AI/Java/Go page wrapping, SiteHeader reorder, CI/CD page, homepage card.
