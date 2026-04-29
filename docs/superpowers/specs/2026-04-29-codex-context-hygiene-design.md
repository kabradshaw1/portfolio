# Codex Context Hygiene Design

## Problem

Codex sessions are consuming too much context early in long-running tasks. The
root `AGENTS.md` currently acts as both an operating manual and an architecture
reference. That makes every session carry infrastructure, service internals,
observability notes, CI/CD details, and historical documentation rules even when
the task only touches one small area.

The repo also contains a large `docs/` tree. Most of it is ADRs, specs,
runbooks, generated Superpowers artifacts, product-catalog RAG fixtures, and
security notes. These files are useful reference material, but they should not
be loaded during default project discovery.

## Goals

- Keep root `AGENTS.md` around 80-120 lines.
- Preserve the project quality bar and safety-critical workflow rules.
- Make context loading pull-based: agents load only the directory instructions,
  skill, or reference doc relevant to the current task.
- Prevent root instructions from growing back into a 300-400 line architecture
  encyclopedia.
- Keep future context additions easy: the agent should know where to put new
  rules without asking.

## Non-Goals

- Do not delete existing project knowledge.
- Do not rewrite ADRs, runbooks, or specs as part of this refactor.
- Do not rely on broad `docs/` scanning for routine development context.
- Do not create many new skills before there is evidence that the workflow is
  repeated and procedural.

## Design

Root `AGENTS.md` becomes a router plus invariant rule set. It should contain:

- project intent and quality bar
- search scope and documentation loading rules
- context hygiene rules
- a compact project map
- branch and autonomy workflow
- verification expectations
- high-risk safety gates
- a routing index for directory `AGENTS.md` files and skills

Detailed, situational content moves into the narrowest relevant location:

- `go/AGENTS.md`: Go services, proto/gRPC, migrations, ecommerce, Kafka,
  service testing, and new-service guidance.
- `frontend/AGENTS.md`: Next.js, Vercel, browser configuration, frontend env
  variable rules, and frontend verification.
- `services/AGENTS.md`: Python AI services, package selection, deprecations,
  service testing, and local AI service conventions.
- `java/AGENTS.md`: Spring services, JDK 21 limitation, heap sizing, Java tests,
  and Java deployment caveats.
- `k8s/AGENTS.md`: Minikube, namespaces, ingress, sealed secrets,
  cert-manager, deployment locality, shared environment mutation rules, and
  ops-as-code triggers.
- `.github/AGENTS.md`: CI/CD workflow behavior, quality gates, deploy trigger
  matrix, Tailscale auth key maintenance, and compose-smoke realism.
- `docs/AGENTS.md`: documentation search policy, ADR/runbook loading policy,
  generated Superpowers artifact rules, and product-catalog fixture warnings.

Skills remain workflow-oriented:

- `debug-observability`: runtime debugging, alerts, logs, traces, and
  post-incident verification.
- `ops-as-code`: any mutating action against shared environments.
- `scaffold-go-service`: new or extracted Go service creation.
- A new context hygiene skill is optional later, but not required for this first
  refactor. Root and `docs/AGENTS.md` should be enough to prevent most bloat.

## Documentation Loading Policy

The root file should make this rule explicit:

```md
Do not read or summarize `docs/` broadly. Docs are pull-based reference
material. Load only the specific doc needed for the current task. Never scan
`docs/adr/`, `docs/superpowers/`, product PDFs, notebooks, or runbooks unless
the task explicitly requires them.
```

`docs/AGENTS.md` should add the details:

- ADRs are loaded only for architecture decisions or when explicitly referenced.
- Runbooks are loaded only for matching operational tasks.
- `docs/product-catalog/` contains RAG/test fixture content, not engineering
  onboarding context.
- `docs/superpowers/` contains generated specs/plans and should stay ignored
  except when the user or a skill explicitly asks for a spec or plan.
- Prefer `rg` with `.ignore` respected over `find` for repository searches.

## Root Growth Guardrail

Root `AGENTS.md` should include a maintenance rule:

```md
Do not add detailed architecture, service internals, environment inventories,
or troubleshooting runbooks to this root file. Add them to the narrowest
directory `AGENTS.md` or a Codex skill. Root may only contain rules that apply
to nearly every task.
```

When new context is discovered during work, place it by scope:

- applies to every task: root `AGENTS.md`
- applies to a directory or stack: that directory's `AGENTS.md`
- describes a repeated procedure: a skill
- explains a decision or deep reference: `docs/`, loaded only by trigger

## Autonomy Rules To Preserve

The refactor should preserve and tighten the non-destructive workflow rules:

- Do not ask before running repo-local tests, preflights, linters, type checks,
  formatters, Playwright, Gradle, pytest, or Go tests.
- Do not ask before inspecting files, creating commits, pushing branches where
  branch rules allow it, or creating/updating pull requests where branch rules
  say to create a PR.
- Do not ask before deleting temporary untracked files Codex created during the
  current task under the repo, `/tmp`, or `~/.codex/tmp`, as long as they are
  not tracked by git and are not user-authored source/config/docs.
- Still ask before destructive git operations, deleting tracked files, mutating
  shared environments, changing secrets, or touching `main`/production outside
  the branch rules.

## Verification

After refactoring:

- `wc -l AGENTS.md` should report roughly 80-120 lines.
- `rg --files docs` should continue respecting `.ignore`.
- Root should mention each child `AGENTS.md` location in the routing index.
- No architecture details should be lost; moved content should remain in a
  directory `AGENTS.md`, existing project skill, or explicitly referenced doc.
- Git diff should show mostly moves/reorganization, not behavior changes.

## Rollout Plan

1. Create `docs/AGENTS.md`, `k8s/AGENTS.md`, `.github/AGENTS.md`, and
   `java/AGENTS.md`.
2. Move situational root sections into those files or existing child
   `AGENTS.md` files.
3. Rewrite root `AGENTS.md` as the compact router and invariant rule set.
4. Run line-count and targeted grep checks.
5. Commit the docs-only refactor locally. Do not push until a later code change
   or an explicit request.

