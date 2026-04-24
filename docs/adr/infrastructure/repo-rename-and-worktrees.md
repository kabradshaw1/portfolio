# Repository Rename and Git Worktree Workflow

- **Date:** 2026-04-05
- **Status:** Accepted

## Context

### Repo Rename

The repository was originally named `gen_ai_engineer` to match the initial project scope — a portfolio focused on generative AI engineering. As the project expanded to include Java microservices, Kubernetes deployment, and full-stack frontend work, the name no longer reflected the broader scope. The GitHub repository was renamed to `portfolio`.

### Git Workflow

The project uses Claude Code agents for implementation. Originally, agents committed directly to `main` or worked on feature branches. This caused problems:

- **Branch switching disrupts the working tree.** If an agent switches to a feature branch, any uncommitted work in the main working tree is at risk.
- **Parallel agents conflict.** Two agents can't work on different feature branches simultaneously without stepping on each other.
- **Main gets polluted.** Agents committing directly to `main` bypass the staging → CI → production flow.

## Decision

### Repo Rename: `gen_ai_engineer` → `portfolio`

GitHub was renamed first (GitHub automatically redirects the old URL). Then all active configuration files were updated:

**Files that broke without updating:**
- `docker-compose.yml` — 3 GHCR image references (`ghcr.io/kabradshaw1/gen_ai_engineer/...`)
- `k8s/ai-services/deployments/*.yml` — 3 GHCR image references
- `java/k8s/deployments/*.yml` — 4 GHCR image references
- `Makefile` — SSH path to Windows PC clone
- `k8s/setup-windows.ps1` — default repo path suggestion

**Files that didn't break but were updated:**
- `CLAUDE.md` — path references throughout

**Files intentionally left unchanged:**
- `docs/superpowers/specs/` and `docs/superpowers/plans/` — historical records that reference the old name. Updating them would rewrite history for no functional benefit.

**GitHub Actions CI** required no changes — workflows use `${{ github.repository }}` for GHCR paths, which automatically resolved to the new name.

**Vercel** needed manual reconnection — the Git repository link in the Vercel dashboard had to be updated to point to `kabradshaw1/portfolio`.

### Git Worktrees for Agent Isolation

All code changes now use `git worktree` instead of feature branches:

```
main (untouched)
├── .claude/worktrees/agent-a1d8a312/  (branch: worktree-agent-a1d8a312)
├── .claude/worktrees/agent-aeec164f/  (branch: worktree-agent-aeec164f)
└── .claude/worktrees/adr-docs/        (branch: worktree-adr-docs)
```

Each worktree is a full checkout of the repo with its own branch, but they share the `.git` directory (so they're lighter than full clones). Agents commit to their worktree branch without touching `main`.

**Workflow:**
1. Agent spawns with `isolation: "worktree"` — gets a worktree in `.claude/worktrees/`
2. Agent implements, tests, commits on the worktree branch
3. Kyle reviews: `git diff main..<worktree-branch>`
4. Kyle pushes the branch to GitHub for CI
5. Kyle merges into `staging`, pushes — CI runs E2E tests
6. Kyle merges `staging` into `main` — CI deploys

**Why worktrees over feature branches?**
- `main` stays clean — no direct commits
- Multiple agents can work in parallel (each in its own worktree)
- No branch switching — the main working tree is never disturbed
- Cleanup is explicit: `git worktree remove` + `git branch -d`

**Trade-off:** Worktrees consume disk space (one full checkout per worktree). Cleanup after merging is required. The `git worktree list` and `git worktree prune` commands help manage this.

## Consequences

**Positive:**
- Repo name reflects actual project scope
- GitHub redirect handles old URLs gracefully
- CI workflows required zero changes for the rename
- Worktrees enable safe parallel agent work
- `main` branch is protected from accidental direct commits

**Trade-offs:**
- 10+ files needed manual GHCR path updates for the rename
- Historical docs reference the old name (intentional — not worth rewriting)
- Vercel required manual dashboard intervention (CLI couldn't fix the Git link)
- Worktrees add a cleanup step that's easy to forget (disk space accumulates)
- The worktree workflow is unfamiliar — has a learning curve compared to simple branching
