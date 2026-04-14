# CI/CD Documentation Page & Maintenance Pages

## Context

The portfolio project is about to go through a server migration from Windows to Debian 12. During this transition (and any future downtime), interactive demo pages that depend on backend APIs will break silently — users see errors or blank states instead of a professional maintenance message.

Additionally, the CI/CD pipeline was recently rebuilt (unified workflow, QA environment, agent automation) and this work should be showcased on the portfolio site as a documentation page demonstrating DevOps skills.

## Goals

- Show a professional maintenance screen on interactive demo pages when the backend is down
- Add a CI/CD pipeline documentation page that explains the pipeline and why it's designed for a solo developer
- Reorder the site header navigation to match the homepage card order

## Health-Check Maintenance Screen

### HealthGate Component

A shared client component at `frontend/src/components/health-gate.tsx` that wraps each interactive demo page.

**Behavior:**
1. On mount, fetches the given health endpoint with a 3-second timeout
2. While checking: renders a minimal loading skeleton (matching page background) to avoid layout flash
3. If healthy (200 response): renders children (the demo page)
4. If unhealthy (fetch fails, timeout, non-200): renders the maintenance screen

**Props:**
- `endpoint` — health check URL (e.g., `https://api.kylebradshaw.dev/chat/health`)
- `stack` — display name for the service (e.g., "Python AI Services")
- `docsHref` — link back to the documentation page (e.g., `/ai`)
- `children` — the demo content to render when healthy

### Maintenance Screen Design

Style A from brainstorming: informative with context.

- Wrench icon (🔧)
- "Server Maintenance" heading
- Maintenance message from `NEXT_PUBLIC_MAINTENANCE_MESSAGE` env var. Fallback: "The backend services are currently offline for maintenance. Please check back later."
- Context box explaining what's happening
- Back link to the documentation page for that stack

### Health Endpoints Per Page

| Page | Health Endpoint | Stack Label | Docs Link |
|------|----------------|-------------|-----------|
| `/ai/rag` | `{API_URL}/chat/health` | Python AI Services | `/ai` |
| `/ai/debug` | `{API_URL}/debug/health` | Python AI Services | `/ai` |
| `/java/tasks` (+ sub-pages) | `{GATEWAY_URL}/actuator/health` | Java Task Management | `/java` |
| `/java/dashboard` | `{GATEWAY_URL}/actuator/health` | Java Task Management | `/java` |
| `/go/ecommerce` (+ sub-pages) | `{GO_ECOMMERCE_URL}/health` | Go Ecommerce | `/go` |
| `/go/login` | `{GO_AUTH_URL}/health` | Go Ecommerce | `/go` |
| `/go/register` | `{GO_AUTH_URL}/health` | Go Ecommerce | `/go` |

The `API_URL`, `GATEWAY_URL`, `GO_ECOMMERCE_URL`, and `GO_AUTH_URL` values come from the existing `NEXT_PUBLIC_*` env vars already used by each page.

For pages with layouts that wrap multiple sub-pages (`/java/tasks/layout.tsx`, `/go/ecommerce/layout.tsx`), the HealthGate wraps at the layout level so all child pages inherit the health check.

### Maintenance Message Env Var

`NEXT_PUBLIC_MAINTENANCE_MESSAGE` — set in Vercel. Initial value: "Migrating to Linux for improved performance and reliability."

Updated via `vercel env` when the reason for downtime changes. Falls back to a generic message if not set.

## CI/CD Pipeline Documentation Page

### Route

`/cicd` — new top-level page. Static content, no backend dependency, no HealthGate needed.

### Navigation

Added to `SiteHeader` after Infrastructure & Deployment. Full header reordered to match homepage card order:

**New nav order:** Go, Infrastructure & Deployment, Java, AI, CI/CD

### Content Sections

1. **Overview** — what the pipeline does, diagram of the full flow (PR → quality checks → QA deploy → prod deploy)
2. **Why a unified workflow** — rationale for consolidating 3 workflows into 1, why path filtering was removed for simplicity
3. **Trigger matrix** — table showing what runs on PR-to-qa, push-to-qa, push-to-main
4. **Quality gates** — list of all checks (lint, test, security, k8s validation) with brief descriptions
5. **QA environment** — separate namespaces, Kustomize overlays, CORS scoping, how QA mirrors production
6. **Image tagging strategy** — `:qa-<sha>` vs `:latest`, GHCR registry
7. **Deploy mechanism** — SSH via Tailscale to Windows/Linux PC, kubectl apply, selective restarts
8. **Why no branch protection** — solo developer, code already reviewed in QA, branch protection would be one person approving their own PR
9. **Agent automation** — how Claude Code agents drive the spec-to-QA pipeline, worktree lifecycle, the "ship it" flow
10. **Smoke tests** — health endpoint checks for QA, Playwright for production

### Implementation

Standard Next.js page component with shadcn/ui typography. Diagrams rendered as styled HTML/CSS (same approach as existing architecture diagrams on `/ai`, `/java`, `/go` overview pages). Code snippets in `<pre>` blocks showing YAML fragments and workflow configuration.

## Files Changed

| Action | File | Purpose |
|---|---|---|
| Create | `frontend/src/components/health-gate.tsx` | Shared health-check wrapper component + maintenance screen |
| Create | `frontend/src/app/cicd/page.tsx` | CI/CD pipeline documentation page |
| Modify | `frontend/src/components/site-header.tsx` | Add CI/CD link, reorder nav: Go, AWS, Java, AI, CI/CD |
| Modify | `frontend/src/app/ai/rag/page.tsx` | Wrap content with HealthGate |
| Modify | `frontend/src/app/ai/debug/page.tsx` | Wrap content with HealthGate |
| Modify | `frontend/src/app/java/tasks/layout.tsx` | Wrap with HealthGate (covers tasks + sub-pages) |
| Modify | `frontend/src/app/java/dashboard/page.tsx` | Wrap with HealthGate |
| Modify | `frontend/src/app/go/ecommerce/layout.tsx` | Wrap with HealthGate (covers ecommerce + sub-pages) |
| Modify | `frontend/src/app/go/login/page.tsx` | Wrap with HealthGate |
| Modify | `frontend/src/app/go/register/page.tsx` | Wrap with HealthGate |

**Vercel env var:** `NEXT_PUBLIC_MAINTENANCE_MESSAGE` — added to production environment.

## Verification

1. **HealthGate works when backend is up:** demo pages render normally
2. **HealthGate works when backend is down:** maintenance screen appears with correct stack name, message, and back link
3. **Maintenance message env var:** setting/unsetting `NEXT_PUBLIC_MAINTENANCE_MESSAGE` changes the displayed text
4. **CI/CD page:** renders with all sections, diagrams display correctly, code snippets are readable
5. **Navigation:** header order matches homepage card order, CI/CD link works
6. **Preflight:** `make preflight-frontend` passes (lint, tsc, build)

## Follow-up (Out of Scope)

- Separate QA database instances (address during Debian 12 migration)
- Live health status indicators on the CI/CD page (could show real-time stack health)
