---
name: merge-conflict-resolution
description: Use when a PR, branch, merge, rebase, cherry-pick, or GitHub conflict-resolution flow has merge conflicts, failed mergeability, or a suspected bad conflict resolution. Requires conflict mapping before edits, semantic escalation, final diff review, and targeted verification.
---

# Merge Conflict Resolution

Resolve conflicts by preserving intent, not by choosing a side wholesale.

## Required First Pass

Before editing conflicted files:

- [ ] Confirm repository, branch, base branch, and working tree state.
- [ ] Fetch the base branch and PR/head branch.
- [ ] List conflicted files with `git diff --name-only --diff-filter=U` or the PR mergeability data.
- [ ] For each conflicted file, summarize:
  - what current/head changed;
  - what incoming/base changed;
  - whether the conflict is mechanical, integration, or decision-bearing;
  - the proposed resolution.
- [ ] Do not resolve decision-bearing conflicts until Kyle has the conflict map.

## Conflict Classes

Mechanical conflicts can usually be resolved autonomously:

- imports;
- formatting;
- nearby test updates;
- lockfile or generated-file reconciliation;
- renamed symbols where intent is obvious.

Integration conflicts can often be resolved autonomously after stating the
strategy:

- both branches added compatible behavior;
- one branch moved code and another edited it;
- one branch added auth/config/retry wiring while another changed business
logic;
- tests need to combine assertions from both sides.

Decision-bearing conflicts must be escalated before editing:

- architecture or source-of-truth changes;
- product behavior changes;
- data ownership or persistence changes;
- public API contracts;
- database migrations or rollback behavior;
- authentication, authorization, secrets, or security posture;
- deployment manifests, CI/CD behavior, image builds, or runtime wiring;
- conflicts where accepting one side silently discards user-approved spec or
  plan intent.

## Resolution Rules

- Never use "accept all incoming" or "accept all current" as the full strategy
  for a conflicted PR unless the conflict map proves every conflicted file is
  mechanical.
- Prefer combining both sides when both contain valid, compatible changes.
- Preserve the repo's approved specs, plans, and source-of-truth decisions.
- Preserve newer base-branch fixes unless they conflict with an explicit
  approved requirement.
- When deleting a concept, search for remaining references after resolution.
- Do not hide uncertainty. If the correct behavior depends on product or
  architecture intent, stop and ask.

## GitHub Conflict Editor Guidance

GitHub's editor is acceptable only for small mechanical conflicts. For anything
semantic:

- fetch the branch locally;
- reproduce the merge or inspect the merge commit;
- resolve in a local worktree;
- run targeted checks;
- push a follow-up commit.

After a GitHub-side conflict resolution, inspect the result locally before
assuming it is correct.

## Useful Commands

```bash
git status --short
git branch --show-current
git fetch origin <base> <head>
git diff --name-only --diff-filter=U
git diff --stat origin/<base>...HEAD
git diff origin/<base>...HEAD -- <path>
```

For suspected stale concepts, use targeted searches. Examples:

```bash
rg "OLD_ENV_VAR|oldPackage|removedConcept" <path>
rg "<<<<<<<|=======|>>>>>>>" <path>
```

## Final Review Before Commit Or Push

- [ ] No conflict markers remain.
- [ ] Final diff against the base branch matches the intended PR behavior.
- [ ] Removed concepts have no live references.
- [ ] Tests cover the combined behavior where practical.
- [ ] Relevant local preflight or focused checks pass.
- [ ] The final response includes:
  - files or areas resolved;
  - what was kept from each side;
  - what was intentionally discarded;
  - verification run;
  - any remaining risk or decision that still needs Kyle.

