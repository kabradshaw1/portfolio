# Design: Ops-as-Code — Committed Procedures for Mutating Operations

- **Date:** 2026-04-28
- **Status:** Draft — pending approval
- **Roadmap:** Standalone infra/process item
- **GitHub issue:** TBD

## Context

A pgbouncer-auth incident on 2026-04-28 surfaced a recurring pattern in
how this project handles operational changes:

1. The pgbouncer-auth secret was rotated as part of the Phase 4 DSN
   component split (`ae6ebd8`), but the corresponding
   `ALTER ROLE pgbouncer_auth PASSWORD …` against Postgres never ran. Sync
   between the secret and the database role drifted.
2. A latent ConfigMap bug (`auth_file = /etc/pgbouncer/userlist.txt`,
   pointing at the image's empty stock stub instead of the rendered
   emptyDir at `/rendered/userlist.txt`) was masked by the fact that
   nobody had previously had to re-authenticate against pgbouncer.
3. Recovery required ~half a dozen ad-hoc bash heredocs written into
   `/tmp/`, copy-pasted into the user's terminal over SSH, with the only
   record of what ran living in chat scrollback.

Item (1) is the kind of operational reconciliation that *should have run
as part of the deploy pipeline*. Item (2) is a manifest bug — orthogonal.
Item (3) is the specific failure this spec addresses: **mutating actions
against shared environments routinely existed only in transient
terminals, with no record, no review, and no way for a future agent or
human to reconstruct what changed when.**

The incident also exposed a friction with auto mode: the harness
correctly refuses agent-driven `kubectl exec … ALTER ROLE` calls against
prod, and the workaround we fell into was "agent writes a `/tmp` script,
user runs it." That workaround has all the same problems as running the
command directly, plus a thin veneer of process.

## Goal

Establish a single hard rule and a small set of supporting conventions
so that **every mutating operation against a shared environment exists
as committed code in this repo before it runs.**

This is the same shape as the existing migration pattern (per
`CLAUDE.md`: "K8s Jobs run `migrate up` on every deploy") but extended
beyond schema changes to all operational reconciliation and
break-glass response.

## Non-goals

- Replacing the existing CI/CD pipeline.
- Introducing Ansible, a custom ops CLI, or any non-K8s-native tooling.
- Auto-applying break-glass procedures. Break-glass artifacts are
  reviewed in calm time and applied explicitly during incidents.
- Auditing read-only operations (`kubectl get/logs/describe`, `psql -c
  "SELECT …"`, Loki/Prometheus queries). These remain free-hand.

## The Rule

**No mutating action runs against a shared environment unless its
exact behavior is committed code in this repo.**

The corollary: harness denials of mutating actions are signals to
*write the procedure as code*, not obstacles to route around.

This rule applies symmetrically to humans and agents.

## Triggering tiers

A mutating action exists as code in exactly one of three forms.
Choice is per-procedure based on cadence and urgency.

### Tier 1 — Deploy-pipeline Job

Routine reconciliation that should follow every deploy.

- **Location:** ops Jobs sit alongside the area's existing manifests,
  in a new `jobs/ops/` subdirectory:
  - `java/k8s/jobs/ops/<name>.yml` for `java-tasks`
  - `go/k8s/jobs/ops/<name>.yml` for `go-ecommerce`
  - `k8s/<area>/jobs/ops/<name>.yml` for top-level areas
    (`ai-services`, `monitoring`)
  This parallels the existing migrate-Job layout
  (`go/k8s/jobs/<service>-migrate.yml`); the `ops/` subdirectory
  separates operational reconciliation from schema migrations.
- **Lifecycle:** applied as part of the same `kubectl apply -k` that
  deploys the rest of the area's manifests.
- **Idempotent:** must be safe to re-run on a healthy system.
  Re-applying produces a no-op.
- **Audit:** k8s Job events, container logs in Loki, Grafana deploy
  annotations.

### Tier 2 — Committed script run directly

One-shot procedures and incident response.

- **Location:** `scripts/ops/<name>.sh` or, for procedures retained
  as fire extinguishers, `scripts/ops/break-glass/<name>.sh`.
- **Lifecycle:** committed first, run with `bash scripts/ops/<name>.sh`.
  Acceptable to commit-then-immediately-apply during a fire (the
  artifact is what matters; PR review can be retroactive).
- **Idempotent or single-shot:** scripts may be one-shots.
  Single-shots get a date prefix (`2026-04-28-pgbouncer-config-rollout.sh`)
  to mark them as point-in-time records.
- **Audit:** git history of `scripts/ops/`. Each commit message names
  the action and reason; future agents reconstruct system state by
  reading the directory.

### Tier 3 — `workflow_dispatch` wrapper (optional)

A GitHub Actions workflow that triggers an *already-committed* Tier 2
script. Used when a button + GH Actions audit log is wanted.

- **Location:** `.github/workflows/ops-<name>.yml`
- **Inputs:** none, or strictly typed enums.
  No free-form strings that would let the trigger redefine the action.
- **Body:** invokes the committed script verbatim.
  Example: `bash scripts/ops/<name>.sh`.
- **Audit:** GH Actions run history (who, when, inputs) layered on
  top of the script's own git history.

Tier 3 is additive. Procedures don't need a Tier 3 wrapper unless
there's a specific reason to want one.

## Choosing a tier

| Procedure shape | Tier |
|---|---|
| Tracks a declarative source-of-truth (secret, manifest, schema) | 1 |
| One-shot fix tied to a specific incident | 2 (date-prefixed) |
| Procedure that might recur but isn't deploy-cadence | 2 |
| Pre-staged break-glass action | 2 (`break-glass/`) |
| Any of the above + want a "click-to-run" button | + 3 |

Today's pgbouncer incident maps to:
- `java/k8s/jobs/ops/pgbouncer-auth-sync.yml` (Tier 1) — keeps
  `pgbouncer_auth`'s Postgres password in lockstep with the secret on
  every deploy. Would have prevented today's incident.
- `scripts/ops/2026-04-28-pgbouncer-config-fix-rollout.sh` (Tier 2,
  date-prefixed) — the actual one-shot we ran, captured as a record.

## Job shape (Tier 1 template)

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: pgbouncer-auth-sync
  namespace: java-tasks
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
          image: postgres:17-alpine
          env:
            - name: PGPASSWORD
              valueFrom:
                secretKeyRef:
                  name: java-secrets
                  key: pgbouncer-auth-password
          command: ["/bin/sh", "-c"]
          args:
            - |
              psql -h postgres.java-tasks.svc.cluster.local \
                   -U taskuser -d taskdb \
                   -v ON_ERROR_STOP=1 -v "newpw=$PGPASSWORD" <<'SQL'
              ALTER ROLE pgbouncer_auth PASSWORD :'newpw';
              SQL
```

### Re-apply mechanic

K8s Job objects are immutable, so re-applying the same name fails. The
deploy step does:

```sh
kubectl delete job <name> -n <ns> --ignore-not-found
kubectl apply -f <file>
```

This matches the pattern the existing Go-service migrate Jobs use
(`.github/workflows/ci.yml:1405,1418-1420`): delete with
`--ignore-not-found --wait=true`, apply, then `kubectl wait
--for=condition=complete --timeout=180s`.
Kustomize hash-suffix generators are an alternative but add a layer
that doesn't pull its weight at this scale.

### RBAC

One `ops-runner` ServiceAccount per namespace, with a Role that grants
only the specific Secret reads each Job needs. No cluster-wide perms,
no exec, no destroy. New Jobs that need additional Secret access amend
the Role explicitly.

## Script shape (Tier 2 template)

```bash
#!/usr/bin/env bash
# What this script does, in one sentence.
# Why it exists / which incident or procedure it addresses.
# Idempotent: yes/no. If no, what state it expects.

set -euo pipefail

# All work happens via a single SSH to debian to keep the script
# auditable as one shell session.
ssh debian bash <<'REMOTE'
set -euo pipefail

# … the actual operation …

REMOTE
```

Conventions:
- Always `set -euo pipefail`.
- Single SSH heredoc per script — no chained `ssh debian "…"` calls
  scattered through the file.
- Read secrets via `kubectl get secret … -o jsonpath` inside the
  heredoc; never echo secret material.
- For psql operations, prefer feeding SQL via stdin with `:'var'`
  substitution rather than `-c "ALTER … :'var'"` — `psql -c` does not
  perform variable interpolation.

## Audit & observability

The audit story already exists; ops-as-code attaches to it without
new infrastructure:

- **Tier 1 Jobs:** k8s events, container logs in Loki (filter by
  `job_name`), Grafana deploy annotations CI already posts.
- **Tier 2 scripts:** git log of `scripts/ops/`. Commit messages
  serve as the per-procedure journal entry.
- **Tier 3 workflows:** GH Actions run history; clickable from the
  PR or repo Actions tab.

A Tier 2 script run without a corresponding commit is a violation
of the rule. The harness already enforces this implicitly by denying
`kubectl exec … <mutation>` from arbitrary terminals; codifying the
rule means we stop trying to route around the denial.

## Agent contract

When acting under auto mode or otherwise:

- **Read-only kubectl is unrestricted.** `get`, `describe`, `logs`,
  `psql -c "SELECT …"`, `loki-query`, Prometheus queries — all
  free-hand. Diagnostics never need to be committed.
- **No mutating action exists only in chat.** If the proposed action
  is a write to a shared environment, the action's exact text must be
  in a committed file before it runs. If the file is uncommitted, the
  action has not been authorized.
- **No `/tmp/` scripts.** The agent does not write executable scripts
  outside the repo. If a script is needed, it lives in `scripts/ops/`
  and is committed.
- **Harness denials are signals.** When the harness blocks
  `kubectl exec … ALTER ROLE`, the correct response is to write the
  Job manifest or script, commit it, and re-trigger via the proper
  channel. It is not to find a different terminal command.
- **Pre-emptive break-glass is encouraged.** During calm time, the
  agent may propose `scripts/ops/break-glass/<name>.sh` artifacts so
  that during a fire we can `bash` a reviewed file rather than
  freelance.

## First concrete instances (in scope)

1. **`java/k8s/jobs/ops/pgbouncer-auth-sync.yml`** — Tier 1 Job that
   reconciles `pgbouncer_auth`'s Postgres password to the secret on
   every deploy. Closes the gap today's incident exposed.
2. **`scripts/ops/2026-04-28-pgbouncer-config-fix-rollout.sh`** —
   Tier 2 capture of the one-shot ConfigMap apply + rollout we ran
   during recovery. Already executed; committing it backfills the
   record.
3. **Per-namespace `ops-runner` RBAC** —
   `java/k8s/rbac/ops-runner.yml` and `go/k8s/rbac/ops-runner.yml`
   defining the ServiceAccount + Role + RoleBinding that Tier 1 Jobs
   bind to. Permissions limited to the specific Secret reads each
   namespace's Jobs need.
4. **`docs/runbooks/ops-as-code.md`** — short reference page indexing
   the conventions of this spec for quick lookup.

## Out of scope (deferred)

- Migrating existing migrate Jobs into the new `jobs/ops/` taxonomy.
  They keep working as-is; the new structure applies to new work.
- Building Tier 3 wrappers for any of the first concrete instances.
  Add them if there's a specific reason to want a button.
- A manifest-level validator that would have caught the
  `auth_file = /etc/pgbouncer/userlist.txt` typo. Worth doing
  separately but unrelated to the ops-as-code framing.
- Automated cleanup of historical ReplicaSets accumulating past
  `revisionHistoryLimit`. Janitorial, separate work.

## Open questions

1. Do Tier 1 Jobs run *before* or *after* the Deployments they
   reconcile? For pgbouncer-auth-sync the order is: ConfigMap →
   Job (alters role) → Deployment (rolls pgbouncer). Kustomize
   ordering may need an explicit hint or a separate apply phase
   in the deploy script.
2. Should `scripts/ops/` enforce a naming lint
   (`<date>-<topic>` for one-shots, `<topic>` for re-runnable)
   in CI? Lightweight pre-commit hook, or skip until we have
   enough scripts to make the convention worth enforcing?
3. Where does the `ops-runner` Role's permission set live —
   per-namespace alongside the Jobs that need it, or
   centralized? Per-namespace fits the current "manifests live
   next to the resource they touch" pattern; that's the default
   unless a reason emerges to centralize.
