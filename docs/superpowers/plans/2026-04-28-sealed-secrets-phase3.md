# Sealed Secrets — Phase 3: close template-vs-live drift

Spec: [`docs/superpowers/specs/2026-04-28-secrets-management-design.md`](../specs/2026-04-28-secrets-management-design.md), Phase 3.

## Context

After Phase 2 sealed the four live keys in `java-tasks/java-secrets`
(`google-client-id`, `google-client-secret`, `jwt-secret`,
`postgres-password`), three keys that manifests reference still don't
exist anywhere — neither in the live Secret nor in the sealed file:

| Key | Consumer | Status today |
|-----|----------|--------------|
| `pgbouncer-auth-password` | `pgbouncer-auth-bootstrap` Job, `pgbouncer` Deployment | Pod `Init:CreateContainerConfigError` |
| `replicator-password` | `postgres-replicator-bootstrap` Job, `postgres-replica` StatefulSet | Both stuck |
| `grafana-reader-password` | `postgres-grafana-reader` Job | Stuck 21h |

The qa→main merge in PR #188 made this visible because pgbouncer's
manifests only just reached main — `deploy.sh` does
`kubectl rollout restart deployment/pgbouncer` and waits, which times
out and red-lights deploys.

There's also a secondary bug: the sealed `java-secrets` reports
`failed update: Resource "java-secrets" already exists and is not
managed`. The controller refuses to take over an existing live Secret
without the `sealedsecrets.bitnami.com/managed: "true"` annotation.
Phase 3 fixes this too.

## Out of scope

- DSN component split (Phase 4).
- CI guardrail for password-bearing URLs in ConfigMaps (Phase 5).
- Java service Secrets and QA-namespace Secrets (Phase 6).
- The `postgres-replica` StatefulSet's `pg_hba.conf` gap — the primary
  rejects replication connections from the replica's pod IP because
  the running pg_hba doesn't have a `host replication replicator
  10.244.0.0/16` entry. This is a pre-existing bug from the
  read-replica feature (PR #181), surfaced after Phase 3 cleared the
  blocking `replicator-password` issue. Tracked separately.
- The `edoburu/pgbouncer:1.23.1` ImagePullBackOff observed during
  Phase 3 verification — looks like local-image-cache eviction on the
  Minikube node. Not caused by Phase 3; will resolve on the next
  successful pull.

## Implementation

### 1. Generate the three values (operator workstation)

```bash
PGB_AUTH=$(openssl rand -base64 32 | tr -d '\n')
REPL=$(openssl rand -base64 32 | tr -d '\n')
GRAF=$(openssl rand -base64 32 | tr -d '\n')
```

Plain alphanumeric/base64 — these go into Postgres role passwords and
PgBouncer's `auth_query` lookup; no shell-escaping concerns inside the
Job's parameterised psql call.

### 2. Patch the live `java-tasks/java-secrets` to add the three keys

```bash
ssh debian "kubectl patch secret java-secrets -n java-tasks \
  --type=merge -p '$(jq -n --arg p \"$PGB_AUTH\" --arg r \"$REPL\" --arg g \"$GRAF\" \
    '{stringData: {\"pgbouncer-auth-password\":$p, \"replicator-password\":$r, \"grafana-reader-password\":$g}}')'"
```

This is the source-of-truth update. Phase 2's sealed file will be
re-sealed from this state in step 4. The four existing keys are
unchanged.

### 3. Add the `managed: "true"` annotation to the materialized Secret

Two places:

1. The SealedSecret's `spec.template.metadata.annotations` — so the
   controller propagates the annotation onto the Secret it creates,
   surviving any future `kubectl delete secret` + reconcile cycle.
2. The existing live Secret directly, via
   `kubectl annotate secret java-secrets sealedsecrets.bitnami.com/managed=true`
   — the controller checks the annotation on the *live* Secret to
   decide whether it's allowed to take over an existing one. Without
   this, the controller errors with `already exists and is not
   managed` and never reconciles.

After both annotations are in place, restart the controller once
(`kubectl rollout restart deployment/sealed-secrets-controller -n
kube-system`) so it re-runs the adoption check.

### 4. Re-seal from cluster

```bash
scripts/seal-from-cluster.sh java-secrets
```

This rewrites `k8s/secrets/java-tasks/java-secrets.sealed.yml` with
all seven encrypted keys. The `managed` annotation from step 3 is
preserved because `seal-from-cluster.sh` strips only runtime metadata
(resourceVersion, uid, creationTimestamp, managedFields), not
intentional annotations on the SealedSecret resource.

Verify by `grep`:

```bash
grep -E 'pgbouncer-auth|replicator-password|grafana-reader' \
  k8s/secrets/java-tasks/java-secrets.sealed.yml
# should print three encrypted lines
```

### 5. Apply, watch the Jobs and pgbouncer come up

```bash
ssh debian "kubectl apply -f -" < k8s/secrets/java-tasks/java-secrets.sealed.yml

# Force the stuck Jobs to recreate. They're not CronJobs, so delete +
# reapply is the way.
ssh debian "kubectl delete job -n java-tasks pgbouncer-auth-bootstrap postgres-replicator-bootstrap postgres-grafana-reader --ignore-not-found"
ssh debian "kubectl apply -f -" < java/k8s/jobs/pgbouncer-auth-bootstrap.yml
ssh debian "kubectl apply -f -" < java/k8s/jobs/postgres-replicator-bootstrap.yml
ssh debian "kubectl apply -f -" < java/k8s/jobs/postgres-grafana-reader.yml

# Force the stuck pgbouncer + replica pods to recreate.
ssh debian "kubectl delete pod -n java-tasks -l app=pgbouncer"
ssh debian "kubectl delete pod -n java-tasks postgres-replica-0"
```

Verify:

```bash
ssh debian "kubectl get pods -n java-tasks -l app=pgbouncer"
# pgbouncer-xxx  2/2  Running

ssh debian "kubectl get jobs -n java-tasks"
# pgbouncer-auth-bootstrap        Complete  1/1
# postgres-replicator-bootstrap   Complete  1/1
# postgres-grafana-reader         Complete  1/1

ssh debian "kubectl get statefulset -n java-tasks postgres-replica"
# postgres-replica  1/1
```

### 6. Commit and PR

Single commit with the updated `*.sealed.yml`. Branch:
`agent/feat-secrets-phase3` → PR to `qa`.

## Risks and how they're handled

- **Generated passwords leak via shell history.** The
  `kubectl patch ... stringData` call is the only place they appear in
  cleartext outside the cluster. Run from a shell with `HISTCONTROL=ignorespace`
  prefixed (leading space), or `unset HISTFILE` for the session.
- **`managed: "true"` makes the SealedSecret authoritative.** Once
  applied, any drift between sealed file and live Secret is resolved
  in favour of the sealed file. That's the desired Phase 3 endpoint.
- **Bootstrap Jobs aren't idempotent across password changes.** If the
  `pgbouncer_auth` role already exists with a different password, the
  Job's `ALTER ROLE ... PASSWORD` branch handles it. Same for the
  `replicator` role (the Job creates or updates). Verified by reading
  the Job manifests.
- **The bootstrap Jobs were broken on first run.** They had three
  latent bugs that only surface when the Job actually executes (the
  Jobs had been failing on missing-secret since they were committed,
  so the SQL was never exercised):
    - The DO blocks used bare `$$` dollar-quoting. Some shells in the
      job containers (busybox sh / dash) expanded `$$` to the shell
      PID despite the heredoc being single-quoted (`<<'SQL'`).
      Switched to a shell-side `if [ EXISTS ]; then ALTER else CREATE
      fi` pattern with the SQL piped via stdin (where psql variable
      substitution `:'pw'` works) instead of `-c` (where it doesn't).
    - psql `:'name'` substitution doesn't expand inside dollar-quoted
      `DO $body$ ... $body$` blocks — psql treats those as opaque.
      Same fix as above (top-level CREATE/ALTER outside any DO block).
    - The pgbouncer-auth Job iterated a hardcoded list of databases
      that included `taskdb_qa` (doesn't exist) and missed
      `ecommercedb`/`ecommercedb_qa` (do exist). Switched to runtime
      discovery via `SELECT datname FROM pg_database` so the Job
      tracks reality as services are added/removed.
- **No QA equivalent action in this phase.** QA's `java-secrets` lives
  in the `java-tasks` namespace too (QA shares Postgres with prod per
  CLAUDE.md), so the same Secret update covers both. Phase 6 will
  introduce a separate QA-namespace Secret if needed.

## Verification

- `kubectl get pods -n java-tasks` shows pgbouncer + postgres-replica Running.
- `kubectl get jobs -n java-tasks` shows the three bootstrap Jobs Complete.
- `kubectl get sealedsecret -n java-tasks java-secrets -o jsonpath='{.status.conditions}'` no longer reports the "not managed" failure.
- Next deploy on `main` or `qa` doesn't time out on the pgbouncer rollout.
