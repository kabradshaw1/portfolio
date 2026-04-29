---
name: ops-as-code
description: Use before any mutating action against a shared environment — kubectl apply/exec/rollout/scale/delete, ALTER ROLE, secret edits, queue purges, /tmp shell scripts, or any "let me run this in prod" instinct. Establishes that mutating actions must exist as committed code in this repo before they run. Use proactively when planning infra/database/ops work, when scaffolding services that need bootstrap state, when triaging prod incidents that lead to mutations, and any time auto-mode blocks a kubectl write.
---

# Ops-as-Code

**The rule.** No mutating action runs against a shared environment unless its exact behavior is committed code in this repo.

Read-only operations are unrestricted: `kubectl get/describe/logs`, `psql -c "SELECT ..."`, `loki-query`, Prometheus queries, port-forwards for inspection. Diagnostics never need to be committed.

**Writes** must take one of these three shapes. Pick by procedure cadence:

| Procedure shape | Tier | Location |
|---|---|---|
| Tracks declarative state (secret, manifest, schema) every deploy | 1 | `<area>/k8s/jobs/ops/<name>.yml` |
| One-shot fix tied to a specific incident | 2 | `scripts/ops/YYYY-MM-DD-<topic>.sh` |
| Procedure that might recur but isn't deploy-cadence | 2 | `scripts/ops/<name>.sh` |
| Pre-staged break-glass action | 2 | `scripts/ops/break-glass/<name>.sh` |
| Any of the above + want a click-to-run button | + 3 | `.github/workflows/ops-<name>.yml` |

Tier 3 wraps an *already-committed* Tier 2 script with no free-form inputs. It's optional.

## What this means in practice

- **Never write executable scripts to `/tmp/`.** If you find yourself writing a `bash <<EOF` heredoc for the user to run, stop. Commit the script to `scripts/ops/` instead.
- **Harness denials are signals, not obstacles.** When auto mode blocks `kubectl exec ... <mutation>`, the response is to write the procedure as code and run it through the proper channel — not to find another command form.
- **Date-prefix one-shot incident scripts** so a future agent reading `git log scripts/ops/` can reconstruct what changed and why. Procedure-style scripts (re-runnable) get a topic-only name.
- **Telling the user to run a kubectl mutation in chat is the same anti-pattern as running it yourself.** Both produce zero artifacts.

## Tier 1 Job template

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: <service>-<verb>
  namespace: <ns>
  annotations:
    ops.kylebradshaw.dev/runs-on: every-deploy
    ops.kylebradshaw.dev/idempotent: "true"
spec:
  ttlSecondsAfterFinished: 600
  backoffLimit: 2
  template:
    spec:
      restartPolicy: OnFailure
      serviceAccountName: ops-runner
      containers:
        - name: sync
          image: postgres:17-alpine   # or whatever the action needs
          env:
            - name: SOME_SECRET
              valueFrom:
                secretKeyRef: { name: ..., key: ... }
          command: ["/bin/sh","-c"]
          args: ["..."]
```

CI delete-then-applies the Job (mirrors existing migrate Jobs at `.github/workflows/ci.yml:1405,1418`). Idempotent contract: re-running on a healthy system is a no-op.

## Tier 2 Script template

```bash
#!/usr/bin/env bash
# What this does, in one sentence.
# Why it exists / which incident or procedure it addresses.
# Idempotent: yes/no.

set -euo pipefail

ssh debian bash <<'REMOTE'
set -euo pipefail
# … the actual operation …
REMOTE
```

Conventions:
- One SSH heredoc per script — no chained `ssh "..."` calls.
- Read secrets via `kubectl get secret ... -o jsonpath` *inside* the heredoc; never echo secret material.
- For `psql`, feed SQL via stdin with `:'var'` substitution rather than `psql -c "ALTER ... :'var'"` — `psql -c` does not interpolate variables.

## Common scenarios

**"Auto mode blocked my `kubectl exec ... ALTER ROLE`."**
Write the action as a Tier 1 Job (if it tracks declarative state) or a Tier 2 script (if it's one-shot). Commit it. Then either CI applies it on next deploy (Tier 1) or you `bash scripts/ops/<name>.sh` from the committed file (Tier 2).

**"I need to bootstrap a new database / role / queue / topic for a new service."**
That's a Tier 1 Job in the new service's manifest set, applied as part of its deploy. Not a `kubectl exec ... CREATE DATABASE ...` step in a runbook.

**"Something is on fire in prod."**
Diagnose with read-only commands — those are free. Once you know the action you need, write it to `scripts/ops/YYYY-MM-DD-<incident>.sh`, commit (acceptable to commit-and-immediately-apply during a fire), then run.

**"I want to give the user a script to paste into their terminal."**
Don't paste; commit. Write the script to `scripts/ops/<name>.sh`, commit to the working branch, then tell the user `bash scripts/ops/<name>.sh`. They can read the file in the repo, you have a record, and the action survives the conversation.

## Reference

Full design and rationale: `docs/superpowers/specs/2026-04-28-ops-as-code-design.md`.

The `debug-observability` skill carries a short version of this rule for incident contexts.
