# QA Environment Frontend Visibility — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface the QA environment on the portfolio frontend so hiring managers can see pre-prod builds, view what's staged vs production, and clearly tell which environment they're on.

**Architecture:** A build-time data module (`deployInfo.ts`) fetches deploy metadata from Vercel env vars and the GitHub public API during `next build`, baking the result into static pages. Three server components consume this data: a layout-wide deploy chip, a QA-only banner, and a commit diff section on `/cicd`. No runtime API calls.

**Tech Stack:** Next.js 16 (server components), TypeScript, Tailwind CSS, GitHub REST API (public, unauthenticated)

**Spec:** `docs/superpowers/specs/2026-04-16-qa-environment-frontend-design.md`

**Worktree:** `.claude/worktrees/agent/feat-qa-frontend-visibility/`

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `frontend/src/lib/deployInfo.ts` | Create | Build-time data module: reads Vercel env vars, calls GitHub API, returns typed `DeployInfo` |
| `frontend/src/components/EnvironmentChip.tsx` | Create | Layout-wide chip: branch + SHA + relative time |
| `frontend/src/components/QABanner.tsx` | Create | Thin banner at top of page, QA builds only |
| `frontend/src/components/QADiffSection.tsx` | Create | Commit diff section for `/cicd` page |
| `frontend/src/app/layout.tsx` | Modify | Add QABanner + EnvironmentChip to layout |
| `frontend/src/app/page.tsx` | Modify | Update CI/CD card copy |
| `frontend/src/app/cicd/page.tsx` | Modify | Add CTA link + QADiffSection to QA Environment section |
| `README.md` | Modify | Add QA Environment section |

---

### Task 1: Build-time deploy info module

**Files:**
- Create: `frontend/src/lib/deployInfo.ts`

- [ ] **Step 1: Create `deployInfo.ts`**

```typescript
const REPO = "kabradshaw1/gen_ai_engineer";

export interface Commit {
  sha: string;
  message: string;
  date: string;
  url: string;
}

export interface DeployInfo {
  branch: string;
  commitSha: string;
  fullSha: string;
  commitMessage: string;
  commitDate: string;
  isQA: boolean;
  qaAheadOfMain: Commit[];
}

function timeAgo(dateStr: string): string {
  if (!dateStr) return "";
  const seconds = Math.floor(
    (Date.now() - new Date(dateStr).getTime()) / 1000
  );
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

async function fetchCommitDate(sha: string): Promise<string> {
  try {
    const res = await fetch(
      `https://api.github.com/repos/${REPO}/commits/${sha}`,
      { next: { revalidate: false } }
    );
    if (!res.ok) return "";
    const data = await res.json();
    return data.commit?.author?.date ?? "";
  } catch {
    return "";
  }
}

async function fetchQADiff(): Promise<Commit[]> {
  try {
    const res = await fetch(
      `https://api.github.com/repos/${REPO}/compare/main...qa`,
      { next: { revalidate: false } }
    );
    if (!res.ok) return [];
    const data = await res.json();
    const commits: Commit[] = (data.commits ?? [])
      .slice(0, 20)
      .map(
        (c: {
          sha: string;
          commit: { message: string; author: { date: string } };
          html_url: string;
        }) => ({
          sha: c.sha.slice(0, 7),
          message: c.commit.message.split("\n")[0],
          date: c.commit.author.date,
          url: c.html_url,
        })
      );
    return commits;
  } catch {
    return [];
  }
}

export async function getDeployInfo(): Promise<DeployInfo> {
  const branch = process.env.VERCEL_GIT_COMMIT_REF ?? "local";
  const fullSha = process.env.VERCEL_GIT_COMMIT_SHA ?? "";
  const commitSha = fullSha ? fullSha.slice(0, 7) : "dev";
  const commitMessage = process.env.VERCEL_GIT_COMMIT_MESSAGE ?? "";

  const commitDate = fullSha ? await fetchCommitDate(fullSha) : "";
  const qaAheadOfMain = await fetchQADiff();

  return {
    branch,
    commitSha,
    fullSha,
    commitMessage,
    commitDate,
    isQA: branch === "qa",
    qaAheadOfMain,
  };
}

export { timeAgo };
```

- [ ] **Step 2: Verify types compile**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add frontend/src/lib/deployInfo.ts
git commit -m "feat: add build-time deploy info module"
```

---

### Task 2: QABanner component

**Files:**
- Create: `frontend/src/components/QABanner.tsx`

- [ ] **Step 1: Create `QABanner.tsx`**

```tsx
import { getDeployInfo } from "@/lib/deployInfo";

export async function QABanner() {
  const info = await getDeployInfo();
  if (!info.isQA) return null;

  return (
    <div className="bg-indigo-600 text-white text-center text-sm py-1.5 px-4">
      You&apos;re viewing the QA environment — latest pre-prod build.
      Production is live at{" "}
      <a
        href="https://kylebradshaw.dev"
        className="underline underline-offset-2 hover:text-indigo-100"
      >
        kylebradshaw.dev
      </a>
      .
    </div>
  );
}
```

- [ ] **Step 2: Verify types compile**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/QABanner.tsx
git commit -m "feat: add QA-only banner component"
```

---

### Task 3: EnvironmentChip component

**Files:**
- Create: `frontend/src/components/EnvironmentChip.tsx`

- [ ] **Step 1: Create `EnvironmentChip.tsx`**

```tsx
import { getDeployInfo, timeAgo } from "@/lib/deployInfo";

export async function EnvironmentChip() {
  const info = await getDeployInfo();

  const commitUrl = info.fullSha
    ? `https://github.com/kabradshaw1/gen_ai_engineer/commit/${info.fullSha}`
    : undefined;

  const age = timeAgo(info.commitDate);

  return (
    <div className="fixed top-16 right-4 z-40 text-xs text-muted-foreground opacity-60 hover:opacity-100 transition-opacity">
      <span>{info.branch}</span>
      <span className="mx-1">·</span>
      {commitUrl ? (
        <a
          href={commitUrl}
          target="_blank"
          rel="noopener noreferrer"
          className="font-mono hover:text-foreground transition-colors"
        >
          {info.commitSha}
        </a>
      ) : (
        <span className="font-mono">{info.commitSha}</span>
      )}
      {age && (
        <>
          <span className="mx-1">·</span>
          <span>{age}</span>
        </>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Verify types compile**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/EnvironmentChip.tsx
git commit -m "feat: add layout-wide environment chip"
```

---

### Task 4: Wire QABanner and EnvironmentChip into layout

**Files:**
- Modify: `frontend/src/app/layout.tsx`

- [ ] **Step 1: Update `layout.tsx`**

Add imports at the top:

```typescript
import { QABanner } from "@/components/QABanner";
import { EnvironmentChip } from "@/components/EnvironmentChip";
```

Update the `<body>` contents to add both components. QABanner goes before SiteHeader, EnvironmentChip goes after SiteHeader:

Replace:

```tsx
        <AuthProvider>
          <SiteHeader />
          {children}
          <SiteFooter />
        </AuthProvider>
```

With:

```tsx
        <AuthProvider>
          <QABanner />
          <SiteHeader />
          <EnvironmentChip />
          {children}
          <SiteFooter />
        </AuthProvider>
```

- [ ] **Step 2: Verify build succeeds**

Run: `cd frontend && npx next build`
Expected: build completes without errors

- [ ] **Step 3: Commit**

```bash
git add frontend/src/app/layout.tsx
git commit -m "feat: add QABanner and EnvironmentChip to layout"
```

---

### Task 5: QADiffSection component

**Files:**
- Create: `frontend/src/components/QADiffSection.tsx`

- [ ] **Step 1: Create `QADiffSection.tsx`**

```tsx
import { getDeployInfo, timeAgo } from "@/lib/deployInfo";

export async function QADiffSection() {
  const info = await getDeployInfo();
  const commits = info.qaAheadOfMain;

  return (
    <div className="mt-8">
      <h3 className="text-lg font-semibold">
        What&apos;s currently staged on QA
      </h3>
      {commits.length === 0 ? (
        <p className="mt-2 text-sm text-muted-foreground">
          QA is caught up with production — latest work is live.
        </p>
      ) : (
        <>
          <p className="mt-2 text-sm text-muted-foreground">
            {commits.length} commit{commits.length !== 1 ? "s" : ""} on{" "}
            <code>qa</code> not yet on <code>main</code>:
          </p>
          <div className="mt-3 space-y-2">
            {commits.map((c) => (
              <div
                key={c.sha}
                className="flex items-baseline gap-3 text-sm"
              >
                <a
                  href={c.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="font-mono text-xs text-muted-foreground hover:text-foreground transition-colors shrink-0"
                >
                  {c.sha}
                </a>
                <span className="text-foreground truncate">{c.message}</span>
                <span className="text-xs text-muted-foreground shrink-0">
                  {timeAgo(c.date)}
                </span>
              </div>
            ))}
          </div>
          {commits.length >= 20 && (
            <a
              href={`https://github.com/kabradshaw1/gen_ai_engineer/compare/main...qa`}
              target="_blank"
              rel="noopener noreferrer"
              className="mt-3 inline-block text-sm text-muted-foreground underline underline-offset-2 hover:text-foreground"
            >
              View all on GitHub →
            </a>
          )}
        </>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Verify types compile**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/QADiffSection.tsx
git commit -m "feat: add QA diff section component for /cicd page"
```

---

### Task 6: Update `/cicd` page with CTA and QADiffSection

**Files:**
- Modify: `frontend/src/app/cicd/page.tsx`

- [ ] **Step 1: Add import at top of file**

```typescript
import { QADiffSection } from "@/components/QADiffSection";
```

- [ ] **Step 2: Add CTA above the architecture diagram**

Inside the "QA Environment" section (`<section className="mt-12">` containing `<h2>QA Environment</h2>`), insert a CTA block between the section's description paragraph and the `<MermaidDiagram>`. Add after the existing `<p>` tag (line 227) and before `<div className="mt-4">` (line 228):

```tsx
          <div className="mt-4 flex items-center gap-4">
            <a
              href="https://qa.kylebradshaw.dev"
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center rounded-md bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-500 transition-colors"
            >
              Visit the QA environment →
            </a>
            <span className="text-sm text-muted-foreground">
              See the latest pre-prod build before it ships.
            </span>
          </div>
```

- [ ] **Step 3: Add QADiffSection below the namespace comparison table**

After the closing `</table>` and its wrapper `</div>` of the namespace comparison table (the table with "Production" / "QA" headers, ending around line 257), add:

```tsx
          <QADiffSection />
```

- [ ] **Step 4: Verify build succeeds**

Run: `cd frontend && npx next build`
Expected: build completes without errors

- [ ] **Step 5: Commit**

```bash
git add frontend/src/app/cicd/page.tsx
git commit -m "feat: add QA CTA and diff section to /cicd page"
```

---

### Task 7: Update homepage CI/CD card copy

**Files:**
- Modify: `frontend/src/app/page.tsx`

- [ ] **Step 1: Update CI/CD card description and content**

Find the CI/CD `<Link>` card (around lines 112-129). Replace the `CardDescription` and `CardContent` text:

Replace:

```tsx
                <CardDescription>
                  Unified GitHub Actions workflow with QA environment and agent
                  automation
                </CardDescription>
```

With:

```tsx
                <CardDescription>
                  Unified GitHub Actions workflow with a live QA environment at
                  qa.kylebradshaw.dev for pre-prod review
                </CardDescription>
```

Replace:

```tsx
                <p className="text-muted-foreground text-sm">
                  A single workflow handles quality checks, image builds, and
                  deployments for three service stacks — designed for a solo
                  developer with automated spec-to-production delivery.
                </p>
```

With:

```tsx
                <p className="text-muted-foreground text-sm">
                  A single workflow handles quality checks, image builds, and
                  deployments for three service stacks — designed for a solo
                  developer with automated spec-to-production delivery. See
                  what&apos;s currently staged for production review on the
                  CI/CD page.
                </p>
```

- [ ] **Step 2: Verify build succeeds**

Run: `cd frontend && npx next build`
Expected: build completes without errors

- [ ] **Step 3: Commit**

```bash
git add frontend/src/app/page.tsx
git commit -m "feat: update homepage CI/CD card to reference QA environment"
```

---

### Task 8: Update README with QA Environment section

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add QA Environment section**

After the "Frontend" section (after line 51, the `---` separator) and before "Infrastructure & DevOps" (line 55), insert:

```markdown

## QA Environment

Every change goes through a QA branch before reaching production. Feature branches merge into `qa`, which auto-deploys to a parallel set of Kubernetes namespaces (`ai-services-qa`, `java-tasks-qa`, `go-ecommerce-qa`) and a separate Vercel frontend build. Once visually inspected, `qa` merges into `main` for production deploy.

- **QA frontend:** [qa.kylebradshaw.dev](https://qa.kylebradshaw.dev)
- **QA API:** `qa-api.kylebradshaw.dev`
- **Production:** [kylebradshaw.dev](https://kylebradshaw.dev) / `api.kylebradshaw.dev`

The `/cicd` page on the live site shows what's currently staged on QA vs production.

---
```

- [ ] **Step 2: Verify README renders correctly**

Run: `head -70 README.md` and confirm the new section sits between "Frontend" and "Infrastructure & DevOps".

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: add QA Environment section to README"
```

---

### Task 9: Final verification

- [ ] **Step 1: Run full frontend preflight**

Run: `make preflight-frontend`
Expected: lint, types, and build all pass

- [ ] **Step 2: Run dev server and visually verify**

Run: `cd frontend && npm run dev`

Check in browser at `http://localhost:3000`:
- EnvironmentChip appears top-right showing `local · dev`
- No QA banner appears (correct — not a QA build)
- `/cicd` page has the "Visit the QA environment →" button in the QA Environment section
- `/cicd` page shows QADiffSection below the namespace table (live GitHub API data — either shows commits or "caught up" message)
- Homepage CI/CD card mentions `qa.kylebradshaw.dev`

- [ ] **Step 3: Stop dev server, final commit if needed**

If any visual adjustments were made, commit them.

- [ ] **Step 4: Push and watch CI**

```bash
git push -u origin agent/feat-qa-frontend-visibility
```

Watch CI. If it passes, create PR to `qa`.
