# ADR: Secrets and Configuration — Practices and Guardrails

- **Date:** 2026-04-28
- **Status:** Accepted
- **Companion to:** [`secrets-management.md`](./secrets-management.md) (the migration decision; this document is the steady-state ruleset that follows from it)

## Context

While migrating to Sealed Secrets in Phases 1–2, the project surfaced a cluster of concerns that go beyond "where do Secret values live." This ADR codifies the practices and guardrails that should keep the same regression class from coming back. It is the document a reviewer reaches for when looking at a PR that touches credentials, DSNs, or anything that ends up in a ConfigMap or Secret.

Concrete things we uncovered in this work:

1. **Credentials inside ConfigMaps.** 12 ConfigMap occurrences of `taskuser:taskpass` baked into `DATABASE_URL` strings; 6 occurrences of `guest:guest` in `RABBITMQ_URL`. ConfigMaps are not encrypted at rest in etcd and not access-controlled the same way Secrets are.
2. **Drift between manifests and live cluster.** Templates committed at `*/k8s/secrets/*.yml.template` claimed 7 keys; the live `java-secrets` had 4. Bootstrap Jobs referenced missing keys (`replicator-password`, `grafana-reader-password`, `pgbouncer-auth-password`); they would have failed at admission time, but no downstream consumer noticed.
3. **Deploy ordering bugs from shared-infra ExternalName routing.** PR #182 cut auth-service over to `pgbouncer.java-tasks-qa.svc.cluster.local` — a Service that ExternalName-resolved to a `pgbouncer.java-tasks` that didn't yet exist in prod, because the QA deploy job doesn't apply prod-namespace manifests. Result: auth-service crashlooped on every QA deploy until the cutover was reverted (PR #183).
4. **Multiple sources of truth for the same Secret.** `stripe-secrets` was simultaneously created by CI's `kubectl create secret generic --from-literal=$STRIPE_SECRET_KEY` (sourced from a GitHub Actions secret), described by a misleading template file, and never traced through code. "Where does this value come from?" had no single answer.
5. **`*.template.yml` files indistinguishable from real Secrets.** Same `kind: Secret`, same shape, base64 placeholder values. A reviewer or a misfired `kubectl apply -f` couldn't tell from the file whether the placeholder was the prod value.
6. **Existing CI guardrails miss this regression class.** `gitleaks` scans for known patterns (AWS keys, GitHub tokens, etc.). Hand-rolled `user:pass` strings inside DSNs don't match. The class of "credential ends up in a ConfigMap" is invisible to the security scanners we already run.
7. **No bootstrap runbook.** "What does a fresh cluster need to come online?" lived in the operator's head. New keys (`replicator-password`, etc.) would need to be created manually, with no documented procedure to point at.

These concerns are addressed in different layers — some by Sealed Secrets (Phases 1–2), some by the DSN component split (Phase 4), some by a CI guardrail (Phase 5), some by this ADR. The map below makes the assignments explicit.

## Decision

The following rules apply to all manifests, application code, and CI/CD configuration in this repo. They are appropriate for the project's portfolio scope and would be appropriate at modest production scale; they are *not* sufficient for a team running real customer credentials at scale (different scope, different ADR).

### 1. Credentials never live in ConfigMap data

Connection strings that contain a username and password are split into two pieces:

- **ConfigMap:** the non-secret parts of the connection — host, port, database name, options, application_name, pool sizing constants.
- **Secret:** the credential parts — username, password, API key, token.

The application assembles the final connection string at startup. This pattern is canonical for 12-factor / production services and means a leaked ConfigMap dump is harmless. The shape will land service-by-service in Phase 4 of the migration (one PR per service to keep blast radius narrow).

Rationale: ConfigMaps are not encrypted at rest in etcd, are routinely included in support bundles and `kubectl describe` output, and may be readable by service accounts that should not be reading credentials. Putting `taskuser:taskpass` into a ConfigMap was a category-level error, not just a scope decision.

### 2. Application Secrets live as committed `SealedSecret` resources

The single source of truth for any Secret consumed by an application Deployment is a file at `k8s/secrets/<namespace>/<name>.sealed.yml`. Updating a Secret means: change the file, commit, deploy. Use `scripts/seal-from-cluster.sh` to re-seal from cluster state, or the recipe in [`k8s/sealed-secrets/README.md`](../../../k8s/sealed-secrets/README.md) to seal a fresh value.

Three corollaries:

- **No `kubectl edit` on a live application Secret.** It will be overwritten on next deploy and the change won't survive a `minikube delete`.
- **No `kubectl create secret generic --from-literal=…` in CI** for application Secrets. The committed sealed file is the source of truth. (Cluster-scoped infra secrets like cert-manager-issued mTLS certs are out of scope for this rule — those are managed by their respective controllers.)
- **No `*.template.yml` files** that look like real Secrets. The pattern is misleading and is the proximate cause of the live-vs-manifest drift we just spent a phase closing.

### 3. Shared infrastructure deploys to prod before QA can route to it

QA's overlay pattern is `ExternalName` Services pointing at the prod namespace's actual Services (postgres, redis, rabbitmq, mongodb, postgres-replica, pgbouncer). This works *only* when the prod-namespace target exists.

Rule: **a new shared-infra resource must be deployed to its prod namespace in the same PR (or an earlier PR) that adds the QA overlay's ExternalName for it.** The CI workflow currently deploys QA-namespace manifests on push-to-qa and prod-namespace manifests on push-to-main; that asymmetry is what surfaces the ordering bug. Until the workflow is restructured (own ADR, deferred), the rule above is the discipline that prevents it.

When introducing a new shared service, also add it to the bootstrap runbook in [`k8s/sealed-secrets/README.md`](../../../k8s/sealed-secrets/README.md) (or wherever fresh-cluster setup ends up consolidated) so a `minikube delete` recovery doesn't surface the same chicken-and-egg.

### 4. Each Secret has exactly one provisioning path

For any Secret in any namespace, there is one answer to "where does this value come from?" The acceptable answers are:

- A committed `SealedSecret` at `k8s/secrets/<ns>/<name>.sealed.yml` (preferred for application Secrets).
- A controller-managed resource (cert-manager `Certificate`, Sealed Secrets' own sealing key, K8s service-account tokens). These are owned by their controllers; the repo doesn't manage them.
- An out-of-band operator-applied value, *only* for clusterwide bootstrap concerns documented in a runbook.

Mixed provisioning — "CI creates this from a GH Actions secret AND there's a SealedSecret too" — is forbidden because the two sources will drift and the failure mode is silent.

### 5. CI catches plaintext credentials in ConfigMaps

The next phase of the migration adds a CI script that fails when a ConfigMap `data` field contains a password-bearing URL pattern (`://[^@]*@`) or matches a small allowlist of known-bad fragments (`taskpass`, `guest:guest`, `password=`). This is Phase 5 of the migration plan and exists because `gitleaks` does not catch this class.

Until Phase 5 ships, the rule applies in code review.

### 6. QA Secrets are intentionally not prod Secrets

QA-namespace Secrets (`go-secrets` in `go-ecommerce-qa`, etc.) are placeholder values for credentials that QA shouldn't share with prod (Google OAuth, Stripe). This is correct behavior, not drift. When migrating QA Secrets onto Sealed Secrets in a future phase, the sealed file should encode the placeholder explicitly with a comment, and the QA overlay's kustomization should reference it.

### 7. The bootstrap runbook is the operator's contract

Anything a fresh cluster needs that isn't in `kubectl apply -k`-able manifests goes into the bootstrap runbook in [`k8s/sealed-secrets/README.md`](../../../k8s/sealed-secrets/README.md) (today: kubeseal install, sealing-key custody decision, recovery procedure). When new operator-only steps appear, they extend the runbook. "It's in the operator's head" is not an acceptable end state.

## Enforcement

| Rule | Enforced by | Today | Future |
|---|---|---|---|
| 1. No credentials in ConfigMap data | CI guardrail (Phase 5) | Code review | Automated check |
| 2. Sealed file is source of truth for app Secrets | Code review + deploy structure | Both | Both |
| 3. Shared infra in prod before QA routes to it | Code review | Review | Workflow restructure (separate ADR) |
| 4. One provisioning path per Secret | Code review | Review | Future tooling could enumerate |
| 5. Plaintext credentials in CM data fail CI | CI guardrail | Pending Phase 5 | Phase 5 |
| 6. QA placeholders are intentional | Code review + comments in sealed files | Both | Both |
| 7. Operator-only steps live in the runbook | Code review | Review | Review |

## Consequences

**Positive:**

- The "where do credentials live" question now has one answer per Secret, traceable from `git log`.
- The regression class that produced the pgbouncer DNS-resolution incident has a written rule and a future CI guardrail behind it.
- New contributors and future-me have a single document explaining how the codebase expects to be edited.
- The rules are appropriate for the project's scope without overreaching into custody/rotation/KMS territory that's out of band.

**Trade-offs:**

- Splitting DSN strings adds a small startup-time concatenation per service. Negligible, but real.
- Sealing every Secret change adds friction (commit + PR vs. `kubectl edit`). The added friction is the point — values should flow through review.
- The "shared infra in prod before QA routes to it" rule is enforced by review, not CI. Until the deploy workflow is restructured to apply prod-namespace manifests on qa-branch deploys (separate ADR), discipline is the guardrail.

## Out of scope (explicitly)

- Credential rotation cadence, automated rotation, dynamic credentials. These are real production concerns; we accept that they are not addressed here and would warrant their own ADR + tooling.
- Cloud-KMS-backed sealing-key custody. The portfolio-scope tradeoff (no out-of-band backup; recovery = re-seal) is documented in [`secrets-management.md`](./secrets-management.md) and stands.
- GitHub Actions secrets management. Tailscale authkey, GHCR tokens, etc. are managed in GitHub's encrypted store; they're a different problem domain.
- Audit logging of who-touched-what-Secret-when. Possible via Kubernetes audit policy + log retention, but out of scope here.

## Open follow-ups

- **Phase 3** of the migration plan closes the live-vs-manifest drift for the missing `replicator-password`, `grafana-reader-password`, `pgbouncer-auth-password` keys.
- **Phase 4** lands the DSN component split per Go service, one PR per service, auth-service last.
- **Phase 5** lands the CI guardrail referenced in rule 5.
- **Workflow ordering ADR** (separate) addresses rule 3's "discipline today, automation tomorrow" gap by reshaping the QA deploy job to apply prod-namespace base manifests as part of the qa-branch flow when they change.
- **Java services** (Phase 6) follow the same DSN-split pattern with Spring's environment-variable substitution.
