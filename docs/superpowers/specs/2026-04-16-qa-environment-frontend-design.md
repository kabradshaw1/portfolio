# QA Environment Frontend Visibility

**Date:** 2026-04-16
**Status:** Draft
**Audience:** Hiring managers reviewing the portfolio

## Problem

The QA environment (`qa.kylebradshaw.dev`) is live and functional, but invisible to portfolio visitors. Hiring managers have no way to know a pre-prod environment exists, no way to see what's staged for review, and no visual signal when they land on QA vs production. The README also has no mention of the QA branch model.

## Goals

1. Surface the QA environment on the frontend so hiring managers see it as a portfolio feature
2. Show what's currently staged on QA vs production (commit diff)
3. Give a clear visual signal when viewing the QA build
4. Document the QA branch model in the README

## Non-Goals

- Runtime API calls from the client (no GitHub API at runtime, no auth tokens)
- Environment switching UX (no toggle between prod/QA in the nav)
- Dismissable banners or persistent state (no cookies, no local storage)

## Data Source

### `src/lib/deployInfo.ts`

A build-time module that assembles deploy metadata from two sources:

**Vercel build environment variables** (available automatically during Vercel builds):
- `VERCEL_GIT_COMMIT_REF` — branch name (`main` or `qa`)
- `VERCEL_GIT_COMMIT_SHA` — full commit SHA
- `VERCEL_GIT_COMMIT_MESSAGE` — commit subject line

**GitHub public API** (called at build time, no auth needed for a public repo):
- **Commit detail:** `https://api.github.com/repos/kabradshaw1/gen_ai_engineer/commits/<sha>` — provides `commit.author.date` for the current deploy's commit timestamp (Vercel does not expose a timestamp env var).
- **Compare:** `https://api.github.com/repos/kabradshaw1/gen_ai_engineer/compare/main...qa` — returns commits on `qa` but not `main`, with SHA, subject, author, and date.
- Both called once during build and baked into static output — zero runtime cost.

**Returned type:**

```typescript
interface DeployInfo {
  branch: string;          // "main", "qa", or "local"
  commitSha: string;       // short SHA (7 chars)
  commitMessage: string;   // commit subject line
  commitDate: string;      // ISO timestamp
  isQA: boolean;           // branch === "qa"
  qaAheadOfMain: Commit[]; // commits on qa not yet on main
}

interface Commit {
  sha: string;             // short SHA
  message: string;         // subject line
  date: string;            // ISO timestamp
  url: string;             // GitHub commit URL
}
```

**Local dev fallback:** When Vercel env vars are absent, default to `branch: "local"`, `commitSha: "dev"`, `commitDate: ""` (EnvironmentChip shows `local · dev`), and no banner. The GitHub API calls still fire locally (public repo), so QADiffSection renders real commit diff data even in dev. The commit detail call is skipped when no SHA is available.

## Components

### 1. `<EnvironmentChip />`

**Location:** Rendered in `layout.tsx`, appears on every route.

**Position:** Top-right of the page body, small and unobtrusive.

**Content:** `main · abc1234 · 2h ago` (prod) or `qa · xyz7890 · 2h ago` (QA).

**Styling:** Monospace font for the SHA, muted foreground color, small text. The SHA links to the GitHub commit page (`https://github.com/kabradshaw1/gen_ai_engineer/commit/<full-sha>`).

**Behavior:** Server-rendered. No interactivity. Returns deploy metadata baked at build time.

### 2. `<QABanner />`

**Location:** Rendered in `layout.tsx`, top of every page.

**Visibility:** Only renders when `isQA === true` (i.e., `VERCEL_GIT_COMMIT_REF === "qa"`). On production builds, the component returns `null`.

**Content:** A thin colored bar (indigo or blue to distinguish from errors/warnings): *"You're viewing the QA environment — latest pre-prod build. Production is live at [kylebradshaw.dev](https://kylebradshaw.dev)."*

**Styling:** Full-width, thin (single line of text), subtle background color, contrasting text. Not dismissable — no state management needed.

### 3. `<QADiffSection />`

**Location:** Rendered on the `/cicd` page, inside the existing "QA Environment" section, below the namespace comparison table.

**Heading:** "What's currently staged on QA"

**Content when qa is ahead of main:** A list of commits, each showing:
- Short SHA (linked to GitHub commit page)
- Commit subject
- Relative date

Capped at 20 commits. If more, show a "View all on GitHub" link to the compare page.

**Content when qa == main (empty state):** *"QA is caught up with production — latest work is live."*

**Styling:** Matches the existing card/table style on the `/cicd` page.

## Page Changes

### Homepage (`src/app/page.tsx`)

Update the existing CI/CD card only — no new cards, no layout changes.

- **`CardDescription`:** Change to: *"Unified GitHub Actions workflow with a live QA environment at qa.kylebradshaw.dev for pre-prod review"*
- **`CardContent`:** Append a sentence: *"See what's currently staged for production review on the CI/CD page."*

### `/cicd` page (`src/app/cicd/page.tsx`)

Two additions within the existing "QA Environment" section:

1. **Above the architecture diagram:** A prominent call-to-action link styled as a button: *"Visit the QA environment →"* pointing at `https://qa.kylebradshaw.dev`. Subtitle: *"See the latest pre-prod build before it ships."*
2. **Below the namespace comparison table:** Render `<QADiffSection />`.

No other sections on the page change.

### `layout.tsx`

Add `<QABanner />` at the top of the body (before any page content) and `<EnvironmentChip />` positioned top-right.

## README Changes

Add a new section after "Frontend" (after line 53) and before "Infrastructure & DevOps":

```markdown
## QA Environment

Every change goes through a QA branch before reaching production. Feature branches merge into `qa`, which auto-deploys to a parallel set of Kubernetes namespaces (`ai-services-qa`, `java-tasks-qa`, `go-ecommerce-qa`) and a separate Vercel frontend build. Once visually inspected, `qa` merges into `main` for production deploy.

- **QA frontend:** [qa.kylebradshaw.dev](https://qa.kylebradshaw.dev)
- **QA API:** `qa-api.kylebradshaw.dev`
- **Production:** [kylebradshaw.dev](https://kylebradshaw.dev) / `api.kylebradshaw.dev`

The `/cicd` page on the live site shows what's currently staged on QA vs production.
```

## Files to Create or Modify

| File | Action |
|---|---|
| `frontend/src/lib/deployInfo.ts` | **Create** — build-time deploy metadata module |
| `frontend/src/components/EnvironmentChip.tsx` | **Create** — layout-wide deploy metadata chip |
| `frontend/src/components/QABanner.tsx` | **Create** — qa-only top banner |
| `frontend/src/components/QADiffSection.tsx` | **Create** — commit diff section for /cicd |
| `frontend/src/app/layout.tsx` | **Modify** — add EnvironmentChip and QABanner |
| `frontend/src/app/page.tsx` | **Modify** — update CI/CD card copy |
| `frontend/src/app/cicd/page.tsx` | **Modify** — add CTA and QADiffSection |
| `README.md` | **Modify** — add QA Environment section |

## Verification

1. **`npm run build`** — must succeed. `deployInfo.ts` handles missing Vercel env vars gracefully.
2. **`npm run dev`** — EnvironmentChip renders with fallback values. QABanner does NOT render. QADiffSection renders from live GitHub API data.
3. **`make preflight-frontend`** — lint + types + build passes.
4. **After deploy to `qa` on Vercel:**
   - `qa.kylebradshaw.dev` shows QABanner at top.
   - EnvironmentChip shows `qa · <sha> · <time>`.
   - `/cicd` page shows commits ahead of main (or "caught up" message).
5. **After deploy to `main` on Vercel:**
   - `kylebradshaw.dev` has NO banner.
   - EnvironmentChip shows `main · <sha>`.
6. **README** — visual check on GitHub that the new section renders with working links.
