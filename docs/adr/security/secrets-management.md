# ADR: Secrets Management — Sealed Secrets

- **Date:** 2026-04-28
- **Status:** Accepted (Phases 1–2 shipped; Phases 3–6 remain)
- **Spec:** [`docs/superpowers/specs/2026-04-28-secrets-management-design.md`](../../superpowers/specs/2026-04-28-secrets-management-design.md)
- **Related runbook:** [`k8s/sealed-secrets/README.md`](../../../k8s/sealed-secrets/README.md)

## Context

Before this work, the repo's credential story was a mix of patterns that drifted as features landed:

- Live cluster Secrets existed but couldn't be reproduced from `main` — committed `*.template.yml` files were structural references (placeholders), not source-of-truth, and had visibly drifted from the live cluster (3 of 7 keys claimed in the template were missing live; bootstrap Jobs that referenced them would fail silently).
- `kubectl create secret` stanzas in CI re-created some Secrets from GitHub Actions secrets on every deploy. That's a working pattern at small scale, but it spreads "where does this credential come from?" across multiple files and depends on GH Actions secrets being correctly populated by an out-of-band human process.
- DSN strings in ConfigMaps had passwords baked in (`taskuser:taskpass`, `guest:guest`) — ConfigMaps are not encrypted at rest in etcd and not access-controlled like Secrets.

The full audit is in the spec. This ADR captures the load-bearing decisions.

## Decision

**Adopt [Sealed Secrets](https://github.com/bitnami-labs/sealed-secrets) as the in-cluster secrets management tool.** Commit encrypted `*.sealed.yml` files to git; let the in-cluster controller decrypt them on apply. Make the committed sealed file the single source of truth for application Secrets.

**Defer the DSN component split** (separating credential parts into Secrets and config parts into ConfigMaps) to Phase 4 of the migration plan. Phase 2 keeps existing values byte-identical so the migration is reversible and doesn't entangle "change storage mechanism" with "change connection-string shape."

## Considered alternatives

### External Secrets Operator

Pulls from cloud KMS / Vault / Secrets Manager. Best when secrets already live in a cloud provider. Rejected because we don't have a cloud provider in scope — pulling in External Secrets just to manage local development credentials would be a layer of indirection without the upstream that justifies it.

### SOPS

Encrypts files; integrates with kustomize via the `sops` plugin. Decent option, but the kustomize integration is fiddly (KSOPS plugin variants, executable-path concerns) and there's no central rotation story or in-cluster controller. Sealed Secrets' controller model is operationally cleaner.

### HashiCorp Vault

Heavy: operator + storage + auth. Overkill for portfolio scale. Worth reconsidering if this project ever needed dynamic credentials, lease-based rotation, or fine-grained ACLs.

### Status quo (`kubectl create secret` from GH Actions secrets)

Keeps working but doesn't address the gaps that motivated the migration: drift between manifests and live cluster, no bootstrap runbook, credentials spread across multiple sources. Rejected.

## Sealing-key custody — a deliberate scope decision

A Sealed Secrets cluster has one load-bearing artifact: the **sealing keypair** the controller mints on first start. The private half lives only in the cluster; the public half is what `kubeseal` uses to encrypt. If the cluster is destroyed and the private key isn't restored, every committed `*.sealed.yml` becomes un-decryptable.

**Decision: do not back the sealing key up out-of-band.**

The reasoning:

- **Scope.** This is a portfolio cluster running on a single Minikube instance. The committed `*.sealed.yml` files encrypt portfolio-scope dev credentials — placeholder Postgres passwords, dev JWT secrets, OAuth client IDs for a personal Google project, Stripe *test*-mode keys. Nothing here protects revenue or PII.
- **Recovery cost.** "Regenerate the keypair, re-seal each `*.sealed.yml` from the live cluster (or external service consoles), open a small PR" is ~30 minutes of work, exercised exactly when needed. No standing operational tax. The runbook for this is in [`k8s/sealed-secrets/README.md`](../../../k8s/sealed-secrets/README.md).
- **Honest signaling.** Pretending to operate at a scale we don't (e.g., "store the key in 1Password" when the operator doesn't otherwise use 1Password) is a fake gesture. Documenting the deliberate scope decision is a more honest and more professional signal of engineering judgment.

**A real production deployment would back this up** — managed KMS (AWS KMS, GCP KMS, Vault Transit), or operator-controlled encrypted vault (1Password with hardware token, age + YubiKey, etc.) — and the procedure would itself be runbooked, tested, and exercised on cadence. That's outside this project's scope.

If this project ever grows real users or revenue, swapping the custody model is a one-PR change: produce the backup, document the destination and restore procedure, replace the "no backup" stance in the runbook.

## Why two pools / DSN split is deferred to Phase 4 (not Phase 2)

The migration plan separates "change *how* secrets are stored" (Phases 1–2) from "change *what* the application reads to assemble a connection string" (Phase 4). The reason:

- **Phases 1–2 are reversible by `git revert`.** No application-layer code changes; no ConfigMap structure changes; values are byte-identical to what was running. If something goes wrong, rolling back is mechanical.
- **Phase 4 is not byte-identical.** It changes ConfigMap shape (removes `DATABASE_URL`, adds `DB_HOST`/`DB_PORT`/`DB_NAME`/`DB_OPTIONS`) and Secret shape (adds `DB_USER`/`DB_PASSWORD`), and requires application code to assemble the DSN at startup. That's higher-risk and warrants per-service rollouts (auth-service last, since it gates everything).

Bundling them in one PR would couple a low-risk storage migration to a higher-risk shape change. Splitting them keeps each PR narrow and reversible.

## Consequences

**Positive:**

- The cluster's secret state is now reproducible from `main`, modulo the sealing key (which is the documented out-of-band artifact, intentionally not backed up).
- Drift between manifests and live cluster is closed: there's nothing in the repo claiming a Secret key exists that isn't actually applied.
- The misleading `*.template.yml` files are gone — no more "is this a placeholder or the prod value?" ambiguity.
- A real bootstrap runbook exists in `k8s/sealed-secrets/README.md` covering install, the sealing-key custody decision, and the recovery path.
- The pattern is recognizable to anyone familiar with GitOps secrets management.

**Trade-offs:**

- One more controller in the cluster (modest — Sealed Secrets is small).
- The sealing key is a load-bearing artifact whose loss costs ~30 minutes of re-sealing work. We accept this rather than build an out-of-band backup mechanism that would be exercised never and rot quietly.
- Changing a Secret value now requires the operator to seal it locally and commit, rather than `kubectl edit`. The added friction is the right friction — values should flow through code review.
- Existing `kubectl create secret` stanzas in the prod CI deploy were removed in this phase; the QA equivalents remain (out of scope) until a future phase moves QA-namespace secrets onto the same pattern.

**Phase-3-and-beyond ideas (not now):**

- Phase 3 — close the live-vs-manifest drift for the missing `replicator-password`, `grafana-reader-password`, `pgbouncer-auth-password` keys.
- Phase 4 — DSN component split (ConfigMap host/port/db, Secret user/pass, app assembles).
- Phase 5 — CI guardrail catching plaintext credentials inside ConfigMaps before they merge.
- Phase 6 — Java services and QA-namespace Secrets follow-up.
