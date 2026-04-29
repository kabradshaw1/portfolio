# Handoff — pgbouncer recovery + Phase 4 continuation

Written 2026-04-28 evening. The previous agent's context filled up mid-debugging a live QA-deploy failure. This doc lets a fresh agent pick up cold.

## TL;DR — the immediate live problem

PR #196 (Phase 4 / order-projector DSN split) merged to `qa`. The `qa` deploy fails because the new order-projector pod can't reach pgbouncer, and pgbouncer itself has been quietly broken since 2026-04-28 morning. We've fixed almost everything; **one credential mismatch remains** that needs explicit Kyle authorization to rotate.

Read `docs/superpowers/specs/2026-04-28-secrets-management-design.md` for the multi-phase plan this is part of, and `docs/adr/security/secrets-management.md` + `docs/adr/security/secrets-and-config-practices.md` for the why/how. The user's memory at `~/.claude/projects/-Users-kylebradshaw-repos-gen-ai-engineer/memory/MEMORY.md` captures personal preferences and process rules that override defaults.

## What "live cluster" means here

- All backend runs on Minikube on a Debian server reached via `ssh debian` (Tailscale).
- `kubectl` only works **on debian**, not locally. So everything is `ssh debian "kubectl ..."`.
- Two namespaces relevant tonight: `java-tasks` (prod), `java-tasks-qa` (QA, mostly ExternalName-points at prod's shared infra).
- Pgbouncer Deployment lives **only in `java-tasks`** — QA's overlay deletes the QA copy and rewires the QA Service to ExternalName at prod's pgbouncer. So fixing pgbouncer in prod is what unblocks QA's deploy.

## What's in the live cluster right now

Pgbouncer pod (`java-tasks/pgbouncer-*`):
- Image `edoburu/pgbouncer:v1.23.1-p3` (was `1.23.1`, Docker Hub renamed it).
- Container command override `/usr/bin/pgbouncer` (the new image's `/entrypoint.sh` wants `DB_HOST` env var; we bypass it because we ship a pre-rendered config file).
- Live ConfigMap `pgbouncer-config` has been patched with `unix_socket_dir =` (empty — read-only root fs blocks the default `/tmp` socket) and `auth_dbname = postgres` (newer pgbouncer refuses to run `auth_query` against the reserved `pgbouncer` admin db).
- Init container chowns `/rendered/userlist.txt` to `70:70` so the pgbouncer process (uid 70) can read the file the busybox init container wrote as root.
- Pod state is currently **1/2 Ready**: the `pgbouncer` container is healthy on TCP 6432; the `exporter` sidecar is in CrashLoopBackOff because pgbouncer's own server-login to postgres fails:

  ```
  WARNING server login failed: FATAL password authentication failed for user "pgbouncer_auth"
  ```

That last error is the only remaining problem.

## The remaining problem (and the fix)

The plaintext password in pgbouncer's `/rendered/userlist.txt` doesn't match the `pgbouncer_auth` role's password in postgres. Confirmed by hashing both:

```
userlist password (sha256): 2330592b65a6edd513ac35dafb66d7909dfe80de22a4565b4828a5e516484f82
secret-as-source-of-truth (sha256): a98e746ed7f92910ceafb4a89bac22a792a41d31c149b8549ecde6651a03dc1b
userlist length 44, secret length 43
```

A manual `psql` test as `pgbouncer_auth` using the secret's value succeeds against postgres. So **the postgres role has the secret value; the userlist has something else** (a stale snapshot from earlier in the same debugging session — the bootstrap Job and the pgbouncer init container were created at different points and saw different values).

### Fix (needs Kyle's explicit OK before running — credential rotation)

```bash
# 1. Generate a fresh password and patch the live secret atomically.
{
  PGB=$(openssl rand -base64 32 | tr -d '\n=')
  cat <<EOF | ssh debian "cat > /tmp/p.yaml && kubectl patch secret java-secrets -n java-tasks --type=merge --patch-file=/tmp/p.yaml && rm -f /tmp/p.yaml"
stringData:
  pgbouncer-auth-password: "$PGB"
EOF
}

# 2. Re-run the bootstrap Job so postgres's pgbouncer_auth role gets the new password.
ssh debian "kubectl delete job pgbouncer-auth-bootstrap -n java-tasks --ignore-not-found"
cat java/k8s/jobs/pgbouncer-auth-bootstrap.yml | ssh debian "kubectl apply -f -"
# wait until Complete
until ssh debian "kubectl get job pgbouncer-auth-bootstrap -n java-tasks -o jsonpath='{.status.conditions[0].type}'" | grep -qE 'Complete|Failed'; do sleep 5; done

# 3. Restart pgbouncer so its init container renders a userlist with the same new value.
ssh debian "kubectl rollout restart deployment/pgbouncer -n java-tasks"
ssh debian "kubectl rollout status deployment/pgbouncer -n java-tasks --timeout=120s"
```

**Why this is risky enough to need approval:** rotates a credential in shared production state. Mitigated by everything that uses `pgbouncer_auth` (only pgbouncer itself + the exporter sidecar) being restarted as part of step 3.

### After the rotation works

Verify both halves of pgbouncer's pod come up:

```bash
ssh debian "kubectl get pods -n java-tasks -l app=pgbouncer"
# expect: 2/2 Running
ssh debian "kubectl logs -n java-tasks -l app=pgbouncer -c pgbouncer --tail=10"
# no more "password authentication failed"
```

Then check that order-projector follows:

```bash
ssh debian "kubectl get pods -n go-ecommerce-qa -l app=go-order-projector"
ssh debian "kubectl get pods -n go-ecommerce -l app=go-order-projector"
# expect both new pods 1/1 Running, old pods Terminated
```

If those look good, the qa deploy that was red can be re-run from the GitHub Actions UI and should pass.

## Uncommitted local edits that need to land

The previous agent applied several fixes directly to live cluster resources. Two of them are reflected as uncommitted edits in the local checkout but not yet pushed:

- `java/k8s/configmaps/pgbouncer-config.yml` — added `unix_socket_dir =` and `auth_dbname = postgres`.
- `java/k8s/deployments/pgbouncer.yml` — added `chown 70:70 /rendered/userlist.txt` in the `render-userlist` initContainer.

**Not** in the local manifest yet: the `command: ["/usr/bin/pgbouncer"]` override on the `pgbouncer` container. It's currently only on the live cluster via `kubectl patch`. Add it to the manifest before committing so the manifest matches reality.

Once the live fix is verified, commit those three manifest changes onto `qa` so the source of truth doesn't drift:

```bash
git add java/k8s/configmaps/pgbouncer-config.yml java/k8s/deployments/pgbouncer.yml
git commit -m "fix(pgbouncer): unix_socket_dir, auth_dbname, userlist chown, command override

Catch up the manifest with the live-cluster patches needed to bring pgbouncer
up after the v1.23.1-p3 image bump. The new image runs as uid 70 (so the
init container has to chown the userlist) and ships a different default
entrypoint (so we override command to /usr/bin/pgbouncer instead of letting
the entrypoint script run). The newer pgbouncer also rejects auth_query
against the reserved admin db (auth_dbname = postgres) and won't bind a
unix socket on a read-only root (unix_socket_dir =)."
git push origin qa
```

CLAUDE.md allows direct push to `qa`. The next qa deploy will be a no-op on the cluster (the fix is already there) but keeps git as source of truth.

## Phase 4 process state — pick up later, not tonight

This work is part of the [secrets-management migration](docs/superpowers/specs/2026-04-28-secrets-management-design.md). Status by phase:

- **Phase 1** (Sealed Secrets controller install) — shipped to main (PR #184/#185).
- **Phase 2** (seal four live Secrets, remove templates) — shipped to main (PR #186/qa→main #188).
- **Phase 3** (close template-vs-live drift, add three missing keys) — shipped to qa (PR #191). The follow-on QA-overlay fix shipped via PR #192 and the `java-tasks-qa` Sealed Secret via PR #193. **Not yet shipped to main.**
- **Phase 5** (CI guardrail R3 — fail on credential URLs in ConfigMap data) — shipped to qa (PR #195). **Not yet shipped to main.**
- **Phase 4** (DSN component split, one Go service per PR) — first service `order-projector` shipped to qa via PR #196. **This is the in-flight work.** Five services left in least-to-most-central order: `cart-service`, `order-service`, `payment-service`, `product-service`, then `auth-service` last. The pattern is documented at [`docs/superpowers/plans/2026-04-28-dsn-split-order-projector.md`](../plans/2026-04-28-dsn-split-order-projector.md) — re-run the same recipe per service.
- **Phase 6** (Java services + monitoring DSN-split) — not started.

The R3 allowlist (`scripts/k8s-policy-check.sh`) is the running list of ConfigMaps still carrying credential URLs; each Phase 4 PR removes one entry. When the allowlist is empty, Phase 4 is done.

## Conversation context the next agent should know

- Auto mode is active (per the user's session prompt). Default is to execute autonomously, but **destructive actions on shared/prod state still need explicit Kyle authorization** — that's what got us here at the credential-rotation step.
- The user's `~/.claude/projects/-Users-kylebradshaw-repos-gen-ai-engineer/memory/MEMORY.md` is loaded into every session; it includes the agent push rules (no main pushes without "ship it"), worktree conventions, quality bar, and other guidance.
- This is a portfolio project. The quality bar is "would pass code review at a serious company" — see `feedback_production_quality.md` in user memory.
- The user is refreshing Python skills for a Gen AI Engineer role and applying to Go+Postgres optimization roles. Code is written by Claude; learning happens via lesson notebooks (memory: `feedback_write_own_code.md`).

## What to do first

1. Read this whole file.
2. Read `git log -5` and `git status` to see the current state of `qa`.
3. Confirm with Kyle that you want to run the credential rotation in the "Fix" section above.
4. After it works, commit the uncommitted manifest edits per the "Uncommitted local edits" section.
5. Stop there. Phase 4 cart-service is the next-after-that task — do not start it tonight.
