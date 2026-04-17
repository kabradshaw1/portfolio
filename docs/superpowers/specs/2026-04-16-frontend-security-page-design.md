# Frontend Security Section Design

**Date:** 2026-04-16
**Status:** Approved (via Claude Code plan-mode review on 2026-04-16)

## Context

The 2026-04-15 application-layer security audit and the 2026-04-16 Debian 13 host hardening shipped substantial security work, captured in two evidence-cited assessments at `docs/security/security-assessment.md` and `docs/security/linux-server-hardening.md`. Both went live on `main` in commits `98913ff` and `c6b4120`.

That work is currently invisible to anyone visiting the live portfolio at `kylebradshaw.dev` — discovery requires finding the GitHub repo and digging into `docs/security/`. Hiring managers and recruiters who land on the site won't see any of it.

This spec adds a visible "Security" entry on the live site that conveys the depth of the security work and links to the source-of-truth `.md` files for evidence-curious readers. It mirrors the existing `/aws` and `/cicd` pattern (info-dense single page, prose + tables + one diagram, no sub-routes).

## Decisions made

Confirmed via clarifying questions during brainstorming:

| Decision | Choice | Rejected alternatives |
|---|---|---|
| Page structure | Single `/security` page | Sub-pages (`/security/application` + `/security/linux-host`); tabs |
| Landing-page treatment | 6th card in the existing card grid | Hero callout above the grid; both card + status strip |
| Content density | Curated summary with deep-link to docs | Full verbatim port; tiered weight per topic |
| Visual treatment | One Mermaid defense-in-depth diagram + tables + prose | Tables-only; two diagrams (defense + attack-surface) |

## Files

| Path | Action |
|---|---|
| `frontend/src/app/page.tsx` | Modify — add a 6th card for "Security" in the existing card grid |
| `frontend/src/app/security/page.tsx` | Create — the new security landing page (~250 lines TSX, mirrors `/aws` and `/cicd` shape) |
| `frontend/src/components/SiteHeader.tsx` | Modify — add "Security" nav item between "CI/CD" and the Grafana link |
| `README.md` | Modify — add `/security` to the numbered "For hiring managers" read-this-first list |

## Reusable existing patterns (don't reinvent)

- **`Card` / `CardHeader` / `CardTitle` / `CardDescription` / `CardContent`** at `frontend/src/components/ui/card.tsx`
- **`Badge`** at `frontend/src/components/ui/badge.tsx` — for status pills in the summary table
- **`MermaidDiagram`** at `frontend/src/components/MermaidDiagram.tsx` — already used by `/aws`, `/cicd`, `/go`, `/ai`
- **Page layout pattern from `/aws/page.tsx` and `/cicd/page.tsx`** — `<main>` container, h2/h3 typography, callout box styling
- **`SiteHeader`** at `frontend/src/components/SiteHeader.tsx` — `usePathname()` for active-link highlighting

No new dependencies. No new design system primitives.

## Detailed changes

### 1. Landing page — `frontend/src/app/page.tsx`

Add a 6th `<Card>` to the existing grid, identical structure to the existing 5. Place it after the CI/CD card.

- **Title:** `Security`
- **Description:** `Defense-in-depth across the stack — application, CI/CD, Kubernetes, and the hardened Linux host that runs it all. Lynis baseline 77.`
- **Link target:** `/security`

Match the existing visual treatment exactly (same `<Card>` slot composition, same `hover:ring-foreground/20` effect, same `<Link>` wrapper).

### 2. New page — `frontend/src/app/security/page.tsx`

Single-file TSX, ~250 lines, mirroring the structure of `frontend/src/app/aws/page.tsx`. Sections in order:

1. **Hero / intro paragraph** (50-60 words) — one sentence framing the work as layered, one sentence stating evidence stance, two prominent inline links to the two `.md` files on GitHub (use the new repo URL `kabradshaw1/portfolio` per the GitHub redirect).

2. **Defense-in-depth diagram** (Mermaid, top-down flowchart) wrapped in the `<MermaidDiagram>` component. Layers: Internet → Cloudflare Tunnel → Debian host → Minikube cluster → Application → Data. Each layer annotated with the controls that defend it.

3. **Public attack surface callout** — one-line bordered box: "Public attack surface: zero. `nmap` against the public IP returns no open ports."

4. **Status summary table** — port the summary table from `security-assessment.md` plus the host-OS row, rendered as a real `<table>`. Eleven rows. `<Badge>` for the status column with variant by status: Strong → `default`, Adequate → `secondary`, Foundation only → `outline`. (No "Weak" rows currently.)

5. **Application & infrastructure highlights** (h2) — one subsection each for Authentication & Authorization, Shift-left CI, Kubernetes runtime, Supply chain. ~3-5 sentences each. Each subsection ends with a "Full breakdown: §N of [doc]" link to the section anchor on GitHub.

6. **Linux host hardening** (h2) — single subsection covering: SSH Tailscale-only, UFW default-deny + LAN-Ollama leak fix, narrow passwordless sudo, auditd immutable rules, sysctl drop-in, fail2ban, lynis 77. Highlights the `security.debian.org` repo gap as the most "interesting" finding. Ends with link to `linux-server-hardening.md`.

7. **Recommended next steps** — short list of 3-4 priorities from the assessment's recommended-next-steps section: PSS + NetworkPolicy, remote audit log forwarding, image digest pinning, Java OWASP gating.

8. **Footer "evidence" pointer** — one-paragraph closing.

**Metadata:** `export const metadata: Metadata = { title: "Security · Kyle Bradshaw", description: "Defense-in-depth security assessment of the portfolio: application, CI/CD, Kubernetes, and the Debian 13 host." }` — match how `/aws/page.tsx` exports metadata.

### 3. Site header — `frontend/src/components/SiteHeader.tsx`

Add `Security` nav item to the existing nav-link list, between `CI/CD` and the Grafana external link. Same shape as the other internal nav items. Active-state highlighting works automatically via existing `usePathname()` logic.

### 4. README — `README.md`

Add `docs/security/` to the "For hiring managers" numbered list (lines 114-125), suggested as a new item 5. Renumber subsequent items.

## Verification

Frontend-only change. Verification steps:

1. `cd frontend && npm run dev` — visual check at `localhost:3000`. Click new Security card, confirm page renders with diagram.
2. `cd frontend && npx tsc --noEmit` — must pass clean.
3. `cd frontend && npm run lint` — must pass clean.
4. `cd frontend && npm run build` — must succeed; new route appears in build output.
5. Mobile responsive check at ~375px width — card grid reflows, Mermaid diagram doesn't overflow.
6. Both GitHub deep links resolve.

## Branch & commit strategy

Per CLAUDE.md feature-branch flow:
- Worktree at `.claude/worktrees/agent-feat-security-page/` on branch `agent/feat-security-page` (created from `main`).
- Commit on the feature branch, push, watch CI, open PR to `qa`.
- Single commit if work is tight; split README/SiteHeader if they feel separable.

## Out of scope (deferred)

- Sub-pages under `/security` — single-page chosen for now; can split later if content density grows.
- Markdown rendering library — would let us render the `.md` files directly, but adds a dependency for a one-page use case. Hardcoded JSX matches the existing `/aws` and `/cicd` pattern.
- E2E test for the new page — content-only with no interactive behavior; visual verification + build success suffice.
