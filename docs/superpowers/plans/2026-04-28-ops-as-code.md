# Ops-as-Code Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the first concrete artifacts of the ops-as-code rule from `docs/superpowers/specs/2026-04-28-ops-as-code-design.md` — commit the recovery work that's still in the working tree, make existing Tier 1 bootstrap Jobs visible to CI, capture today's recovery as a Tier 2 script, and add a short runbook.

**Architecture:** All work is doc-and-config — no application code. Three buckets:
1. Commit pgbouncer ConfigMap + Deployment fixes already applied to live prod (working-tree drift cleanup) and the spec amendments.
2. Add `kubectl wait` against the four existing `java-tasks` bootstrap Jobs in `.github/workflows/ci.yml` so a silent failure surfaces as a CI failure.
3. Codify today's incident response (`pgbouncer-config-fix-rollout`) as a date-prefixed Tier 2 script and add a short runbook page.

**Tech Stack:** Kubernetes manifests (Job, ConfigMap, Deployment), GitHub Actions YAML, bash, markdown.

---

## File Structure

| Path | Action | Responsibility |
|---|---|---|
| `java/k8s/configmaps/pgbouncer-config.yml` | Modify (already changed in working tree) | `auth_file` path fix + `unix_socket_dir =` + `auth_dbname = postgres` |
| `java/k8s/deployments/pgbouncer.yml` | Modify (already changed in working tree) | init container `chown 70:70` so pgbouncer (uid 70) can read userlist.txt |
| `docs/superpowers/specs/2026-04-28-ops-as-code-design.md` | Modify (already changed in working tree) | Context-section corrections after we discovered `pgbouncer-auth-bootstrap` already exists |
| `.github/workflows/ci.yml` | Modify | Add `kubectl wait --for=condition=complete` for each of the four `java-tasks` bootstrap Jobs |
| `scripts/ops/2026-04-28-pgbouncer-config-fix-rollout.sh` | Create | Tier 2 record of today's recovery: apply corrected ConfigMap, roll pgbouncer Deployment, verify |
| `docs/runbooks/ops-as-code.md` | Create | Short reference page indexing the spec's conventions; cites `pgbouncer-auth-bootstrap` as canonical Tier 1 example |

---

## Task 1: Commit the pgbouncer recovery + spec amendments

**Why first:** the working tree currently has unstaged changes that match what's live in prod. Leaving them uncommitted creates drift between the cluster and the manifests. This task gets the repo and cluster into the same state before any new work.

**Files:**
- Modify (already changed): `java/k8s/configmaps/pgbouncer-config.yml`
- Modify (already changed): `java/k8s/deployments/pgbouncer.yml`
- Modify (already changed): `docs/superpowers/specs/2026-04-28-ops-as-code-design.md`

- [ ] **Step 1: Confirm the working-tree diff matches what's live in prod**

Run:
```bash
git diff java/k8s/configmaps/pgbouncer-config.yml java/k8s/deployments/pgbouncer.yml
```

Expected: a diff that shows three changes in the ConfigMap (`unix_socket_dir =`, `auth_dbname = postgres`, `auth_file = /rendered/userlist.txt`) plus a comment block, and one change in the Deployment (`chown 70:70 /rendered/userlist.txt` line in the init container).

Cross-check against the live cluster:
```bash
ssh debian "kubectl get cm pgbouncer-config -n java-tasks -o jsonpath='{.data.pgbouncer\.ini}' | grep -E 'auth_file|auth_dbname|unix_socket_dir'"
```

Expected: live ConfigMap shows `auth_file = /rendered/userlist.txt`, `auth_dbname = postgres`, `unix_socket_dir =`. If any of these don't match the diff, stop and reconcile before committing.

- [ ] **Step 2: Stage and commit the changes**

```bash
git add java/k8s/configmaps/pgbouncer-config.yml \
        java/k8s/deployments/pgbouncer.yml \
        docs/superpowers/specs/2026-04-28-ops-as-code-design.md
git -c commit.gpgsign=false commit -m "$(cat <<'EOF'
fix(pgbouncer): correct auth_file path + supporting init/config tweaks

The ConfigMap pointed `auth_file` at the image's empty stock stub
(/etc/pgbouncer/userlist.txt) instead of the rendered emptyDir
(/rendered/userlist.txt) the init container actually writes to.
That made pgbouncer start with zero credentials for pgbouncer_auth
and reject every auth_query connection. Also: empty unix_socket_dir
for read-only-root compatibility, auth_dbname = postgres for v1.23+
admin-db restriction, and chown 70:70 on the rendered userlist so
the runtime user (uid 70) can read the file the root init wrote.

Spec amendment reflects the corrected understanding of the incident
root cause now that `pgbouncer-auth-bootstrap` is known to be doing
its job already.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Expected: commit succeeds; pre-commit hooks pass.

- [ ] **Step 3: Push to qa**

Per `CLAUDE.md` branch rules, qa pushes are autonomous for code changes. This commit changes deploy behavior, so it pushes (unlike doc-only commits earlier in the session).

```bash
git push origin qa
```

Expected: push succeeds. Do NOT watch CI — Kyle reports failures.

---

## Task 2: Add `kubectl wait` for the four `java-tasks` bootstrap Jobs in CI

**Why:** today's recovery exposed that the existing bootstrap Jobs (`pgbouncer-auth-bootstrap`, `postgres-replicator-bootstrap`, `postgres-grafana-reader`, `postgres-extensions-bootstrap`) are fire-and-forget in CI: they're applied via the bulk `find java/k8s | kubectl apply -f -` (ci.yml:1574) but nothing waits for them to complete or asserts success. A silent failure today would have looked exactly like a healthy deploy. Adding `kubectl wait` mirrors the pattern Go migrate Jobs already use.

**Files:**
- Modify: `.github/workflows/ci.yml` around line 1574

- [ ] **Step 1: Read the current bulk-apply step**

```bash
sed -n '1565,1580p' .github/workflows/ci.yml
```

Expected: lines 1573 (delete the four bootstrap Jobs) and 1574 (bulk apply java/k8s). Note the exact indentation (10 spaces for the body of the run step).

- [ ] **Step 2: Add wait commands after the bulk apply**

Edit `.github/workflows/ci.yml`. Find this block (around line 1573-1574):

```yaml
          $SSH "kubectl delete job -n java-tasks pgbouncer-auth-bootstrap postgres-replicator-bootstrap postgres-grafana-reader postgres-extensions-bootstrap --ignore-not-found --wait=true"
          for f in $(find java/k8s -name '*.yml' -not -name 'namespace.yml' -not -path '*/secrets/*'); do echo '---'; cat "$f"; done | $SSH "kubectl apply -f -"
```

Insert four `kubectl wait` lines immediately after the `find ... | kubectl apply -f -` line:

```yaml
          $SSH "kubectl delete job -n java-tasks pgbouncer-auth-bootstrap postgres-replicator-bootstrap postgres-grafana-reader postgres-extensions-bootstrap --ignore-not-found --wait=true"
          for f in $(find java/k8s -name '*.yml' -not -name 'namespace.yml' -not -path '*/secrets/*'); do echo '---'; cat "$f"; done | $SSH "kubectl apply -f -"
          # Tier 1 ops-as-code Jobs are fire-and-forget without an explicit wait.
          # Mirror the Go migrate-Job pattern so a silent failure surfaces in CI.
          # See docs/superpowers/specs/2026-04-28-ops-as-code-design.md.
          $SSH "kubectl wait --for=condition=complete --timeout=180s job/postgres-extensions-bootstrap -n java-tasks"
          $SSH "kubectl wait --for=condition=complete --timeout=180s job/postgres-replicator-bootstrap -n java-tasks"
          $SSH "kubectl wait --for=condition=complete --timeout=180s job/pgbouncer-auth-bootstrap -n java-tasks"
          $SSH "kubectl wait --for=condition=complete --timeout=180s job/postgres-grafana-reader -n java-tasks"
```

The order matters: `postgres-extensions-bootstrap` must complete before the others (it installs `pg_stat_statements` etc. that other Jobs may rely on); `pgbouncer-auth-bootstrap` and `postgres-grafana-reader` are independent of each other.

- [ ] **Step 3: Lint the workflow YAML**

```bash
make preflight-security 2>&1 | head -30
```

Expected: workflow YAML stays valid. (`make preflight-security` runs the workflow lint chain that gitleaks/hadolint use; if your local doesn't have it, fall back to `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))" && echo OK`.)

- [ ] **Step 4: Verify each Job name actually exists in `java/k8s/jobs/`**

```bash
for j in postgres-extensions-bootstrap postgres-replicator-bootstrap pgbouncer-auth-bootstrap postgres-grafana-reader; do
  test -f "java/k8s/jobs/${j}.yml" && echo "OK: $j" || echo "MISSING: $j"
done
```

Expected: four `OK:` lines. If any are missing, the wait would block forever — fix before committing.

- [ ] **Step 5: Stage and commit**

```bash
git add .github/workflows/ci.yml
git -c commit.gpgsign=false commit -m "$(cat <<'EOF'
ci(java-tasks): wait on bootstrap Jobs after bulk apply

The four java-tasks bootstrap Jobs (postgres-extensions,
postgres-replicator, pgbouncer-auth, postgres-grafana-reader) are
applied as part of the bulk find-pipe-apply at ci.yml:1574. Without
a kubectl wait, a silent Job failure looks identical to a healthy
deploy — exactly the gap that obscured today's pgbouncer auth
incident root-cause analysis.

Mirrors the Go migrate-Job pattern (ci.yml:1419-1420). Per the
ops-as-code spec, this is the minimum viable Tier 1 ops-Job
hardening: verify they ran, not just that they were applied.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Expected: commit succeeds. Don't push yet — bundle with later commits.

---

## Task 3: Capture today's recovery as a Tier 2 script

**Why:** the actual sequence we ran today (apply corrected ConfigMap, rollout pgbouncer Deployment, verify) lives only in chat scrollback. Per the spec rule, mutating actions exist as committed code. Backfilling the script makes the artifact a permanent record any future agent can read via `git log scripts/ops/`.

**Files:**
- Create: `scripts/ops/2026-04-28-pgbouncer-config-fix-rollout.sh`

- [ ] **Step 1: Verify the directory exists**

```bash
test -d scripts/ops || mkdir -p scripts/ops
ls -la scripts/ops 2>/dev/null
```

Expected: directory exists (will be empty if newly created).

- [ ] **Step 2: Write the script**

Create `scripts/ops/2026-04-28-pgbouncer-config-fix-rollout.sh` with:

```bash
#!/usr/bin/env bash
#
# Incident: pgbouncer auth_file pointed at the image's empty stock stub
# instead of the rendered emptyDir, so pgbouncer started with zero
# credentials for pgbouncer_auth. Recovery: apply corrected ConfigMap,
# roll pgbouncer, verify auth_query works.
#
# Idempotent: re-running on a healthy cluster is a no-op (the ConfigMap
# apply is content-addressed, the rollout-restart is harmless if the
# pod is already on the latest config, the verify step is read-only).
#
# Backfilled record of the actions taken on 2026-04-28 during recovery.
# See docs/superpowers/specs/2026-04-28-ops-as-code-design.md for the
# rule under which this script exists.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CONFIGMAP_FILE="${REPO_ROOT}/java/k8s/configmaps/pgbouncer-config.yml"

if [ ! -f "$CONFIGMAP_FILE" ]; then
  echo "ERROR: ConfigMap manifest not found at $CONFIGMAP_FILE" >&2
  exit 1
fi

scp "$CONFIGMAP_FILE" debian:/tmp/pgbouncer-config.yml

ssh debian bash <<'REMOTE'
set -euo pipefail

echo "--- apply pgbouncer-config ConfigMap ---"
kubectl apply -f /tmp/pgbouncer-config.yml

echo
echo "--- rollout restart pgbouncer Deployment ---"
kubectl rollout restart deployment/pgbouncer -n java-tasks

echo
echo "--- wait for rollout (timeout 120s) ---"
kubectl rollout status deployment/pgbouncer -n java-tasks --timeout=120s

echo
echo "--- pod state ---"
kubectl get pods -n java-tasks -l app=pgbouncer

echo
echo "--- verify pgbouncer can authenticate to postgres as pgbouncer_auth ---"
PW=$(kubectl get secret -n java-tasks java-secrets \
       -o jsonpath='{.data.pgbouncer-auth-password}' | base64 -d)
kubectl exec -i -n java-tasks deploy/postgres -- \
  env PGPASSWORD="$PW" psql -h postgres.java-tasks.svc.cluster.local \
       -U pgbouncer_auth -d postgres -tAc "SELECT 'login ok as ' || current_user;"

rm -f /tmp/pgbouncer-config.yml
REMOTE

echo
echo "Recovery complete."
```

- [ ] **Step 3: Make it executable and shellcheck it**

```bash
chmod +x scripts/ops/2026-04-28-pgbouncer-config-fix-rollout.sh
shellcheck scripts/ops/2026-04-28-pgbouncer-config-fix-rollout.sh 2>&1 | head -20 || echo "(shellcheck not installed locally — skip)"
```

Expected: no SC warnings other than maybe SC2317 (unreachable code in heredoc — false positive). If shellcheck isn't installed, skip.

- [ ] **Step 4: Stage and commit**

```bash
git add scripts/ops/2026-04-28-pgbouncer-config-fix-rollout.sh
git -c commit.gpgsign=false commit -m "$(cat <<'EOF'
docs(ops): backfill 2026-04-28 pgbouncer recovery as Tier 2 script

Per the ops-as-code rule, mutating actions against shared envs exist
as committed code. Today's incident response was run from /tmp
heredocs that left no record. This script captures the actual
sequence we executed (apply corrected ConfigMap, roll pgbouncer,
verify auth_query) so a future agent reading `git log scripts/ops/`
can reconstruct what changed and why.

Idempotent: re-runnable on a healthy cluster as a no-op.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Add the ops-as-code runbook reference page

**Why:** the spec at `docs/superpowers/specs/2026-04-28-ops-as-code-design.md` is the authoritative design but it's 300+ lines. A short reference under `docs/runbooks/` gives a quick lookup for the conventions, with a worked example pointing at the existing `pgbouncer-auth-bootstrap` Job.

**Files:**
- Create: `docs/runbooks/ops-as-code.md`

- [ ] **Step 1: Check whether `docs/runbooks/` exists**

```bash
test -d docs/runbooks || mkdir -p docs/runbooks
ls -la docs/runbooks 2>/dev/null
```

Expected: directory exists or is created.

- [ ] **Step 2: Write the runbook**

Create `docs/runbooks/ops-as-code.md` with:

```markdown
# Ops-as-Code Runbook

Quick reference for the rule defined in
[`docs/superpowers/specs/2026-04-28-ops-as-code-design.md`](../superpowers/specs/2026-04-28-ops-as-code-design.md).

## The rule

No mutating action runs against a shared environment unless its
exact behavior is committed code in this repo.

Read-only operations (`kubectl get/describe/logs`, `psql -c
"SELECT ..."`, `loki-query`, Prometheus queries) are unrestricted.

## Tier picker

| Procedure shape | Tier | Location |
|---|---|---|
| Tracks declarative state every deploy | 1 | `<area>/k8s/jobs/[ops/]<name>.yml` |
| One-shot fix tied to an incident | 2 | `scripts/ops/YYYY-MM-DD-<topic>.sh` |
| Recurring procedure that isn't deploy-cadence | 2 | `scripts/ops/<name>.sh` |
| Pre-staged break-glass action | 2 | `scripts/ops/break-glass/<name>.sh` |
| Above + click-to-run button | + 3 | `.github/workflows/ops-<name>.yml` |

## Canonical Tier 1 example

[`java/k8s/jobs/pgbouncer-auth-bootstrap.yml`](../../java/k8s/jobs/pgbouncer-auth-bootstrap.yml)
reconciles `pgbouncer_auth`'s Postgres password to the value stored
in the `java-secrets/pgbouncer-auth-password` Secret on every prod
deploy. Idempotent — `ALTER ROLE … PASSWORD :'pw'` re-stores the
same hash on a second run. CI delete-then-applies it
(`.github/workflows/ci.yml:1573-1574`) and waits for completion
(`ci.yml` after the bulk apply). If a future ops Job tracks
declarative state on every deploy, model it on this Job.

## Canonical Tier 2 example

[`scripts/ops/2026-04-28-pgbouncer-config-fix-rollout.sh`](../../scripts/ops/2026-04-28-pgbouncer-config-fix-rollout.sh)
captures the one-shot sequence run during the 2026-04-28 pgbouncer
incident: apply corrected ConfigMap, roll the Deployment, verify
auth_query works. Date-prefixed so `git log scripts/ops/` reads as
an incident timeline.

## What never lives in chat

- Telling the user to paste a `kubectl exec ... ALTER ROLE …` into
  a terminal.
- Writing a bash heredoc to `/tmp/<name>.sh` and asking the user to
  run it.
- A `workflow_dispatch` workflow that takes a free-form SQL string
  or shell command as input.

If you find yourself reaching for any of these, stop and write the
action as committed code instead. The ops-as-code skill
(`.claude/skills/ops-as-code/SKILL.md`) carries the full agent
contract.
```

- [ ] **Step 3: Stage and commit**

```bash
git add docs/runbooks/ops-as-code.md
git -c commit.gpgsign=false commit -m "$(cat <<'EOF'
docs(runbook): ops-as-code quick reference

Short lookup page under docs/runbooks/ for the ops-as-code rule.
Points at the canonical Tier 1 (pgbouncer-auth-bootstrap) and Tier 2
(2026-04-28 pgbouncer recovery) examples. The full design lives in
docs/superpowers/specs/2026-04-28-ops-as-code-design.md; this is
just a quick lookup for someone trying to remember "where do I put
this Job manifest."

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Push the bundled commits and verify CI picks up cleanly

**Why:** Tasks 1-4 produced multiple commits. Per CLAUDE.md branch rules, qa pushes are autonomous, but doc-only commits should ride along with code commits to avoid empty CI runs. Tasks 2 and 3 contain real CI/script changes, so the bundle is no longer doc-only — it should push.

- [ ] **Step 1: Confirm the commit chain on local qa**

```bash
git log --oneline origin/qa..HEAD
```

Expected: 4 commits on top of `origin/qa`:
1. `fix(pgbouncer): correct auth_file path + supporting init/config tweaks`
2. `ci(java-tasks): wait on bootstrap Jobs after bulk apply`
3. `docs(ops): backfill 2026-04-28 pgbouncer recovery as Tier 2 script`
4. `docs(runbook): ops-as-code quick reference`

Plus the three earlier doc-only commits from this session if they weren't already pushed:
- `bf7fa77 docs(spec): ops-as-code design …`
- `2e027d2 docs(skill): inline ops-as-code rule into debug-observability`
- `1e9ad3a docs(skills): add ops-as-code skill, update scaffold-go-service`

- [ ] **Step 2: Push**

```bash
git push origin qa
```

Expected: push succeeds. Do NOT watch CI — Kyle reports failures.

- [ ] **Step 3: Notify Kyle**

In chat: "Plan executed. Pushed 4 commits to qa (pgbouncer manifest fixes + CI wait additions + Tier 2 backfill + runbook), plus the 3 earlier doc-only commits that were sitting locally. CI will run on push; surface anything that breaks."

---

## Out of scope (deferred per spec)

- `ops-runner` per-namespace RBAC — defer until a second new Tier 1 Job has different permission needs.
- Reorganizing existing bootstrap Jobs into `<area>/k8s/jobs/ops/` — pure refactor with rollout risk; new Jobs go to `jobs/ops/`, existing stay put.
- Tier 3 `workflow_dispatch` wrappers — additive; build only when a specific procedure wants a button.
- A manifest-level validator that would have caught the `auth_file` typo — worth doing as a separate effort, not part of this plan.

## Self-review notes

Spec coverage check (each spec section → task that implements it):

| Spec section | Implementing task |
|---|---|
| The Rule | Documented in runbook (Task 4) |
| Tier 1 — Deploy-pipeline Job | Existing pgbouncer-auth-bootstrap; visibility hardened in Task 2 |
| Tier 2 — Committed script | Task 3 (today's recovery); Task 4 documents the convention |
| Tier 3 — workflow_dispatch wrapper | Out of scope (deferred) |
| Choosing a tier | Documented in runbook (Task 4) |
| Job shape template | Existing pgbouncer-auth-bootstrap matches the pattern |
| Re-apply mechanic | Already in CI; Task 2 hardens with `kubectl wait` |
| RBAC | Out of scope (deferred) |
| Script shape template | Task 3 follows it |
| Audit & observability | Existing infrastructure (Loki, Grafana annotations, GH Actions logs) |
| Agent contract | Lives in `.claude/skills/ops-as-code/SKILL.md` (already committed) |
| First concrete instances #1 (CI wait) | Task 2 |
| First concrete instances #2 (Tier 2 script) | Task 3 |
| First concrete instances #3 (runbook) | Task 4 |

Placeholder scan: no TBDs, every code/CI block contains the actual content. Type/name consistency: Job names and file paths verified against repo at plan-write time (`postgres-extensions-bootstrap`, `postgres-replicator-bootstrap`, `pgbouncer-auth-bootstrap`, `postgres-grafana-reader` all match `java/k8s/jobs/`).
