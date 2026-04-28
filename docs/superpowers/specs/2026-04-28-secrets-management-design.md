# Design: Secrets Management — Sealed Secrets + DSN Component Split

- **Date:** 2026-04-28
- **Status:** Draft — pending approval
- **Roadmap:** Standalone infra/security item (not part of `db-roadmap`)
- **GitHub issue:** TBD

## Context

The repo's current credential story is a mix of patterns that drifted as
features landed. Auditing the live Minikube cluster (kyle@debian, 2026-04-28)
against the manifests in `main` shows:

### What's in the live cluster

| Namespace | Secret | Keys |
|---|---|---|
| `java-tasks` | `java-secrets` | `google-client-id`, `google-client-secret`, `jwt-secret`, `postgres-password` |
| `go-ecommerce` | `go-secrets` | `google-client-id`, `google-client-secret`, `jwt-secret` |
| `go-ecommerce` | `stripe-secrets` | `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET` |
| `monitoring` | `telegram-bot` | `bot-token` |
| (per service) | `*-grpc-tls` | mTLS cert/key (managed by cert-manager — already proper) |

### What the manifests claim

- `java/k8s/secrets/java-secrets.yml.template` — 7 keys, including
  `replicator-password`, `grafana-reader-password`,
  `pgbouncer-auth-password`. None of those three are in the live Secret.
- `go/k8s/secrets/go-secrets.yml.template` — structural reference for
  `go-secrets`.
- Neither template is applied by CI; both are pure documentation
  artifacts shipped with placeholder base64 values that look like
  credentials.

### Concrete gaps this audit surfaces

1. **Drift between template and live.** Three secret keys are in the
   committed template but missing from the live cluster. Jobs that
   reference them (`postgres-grafana-reader.yml`,
   `postgres-replicator-bootstrap.yml`,
   `pgbouncer-auth-bootstrap.yml`) declare `secretKeyRef` lookups that
   would fail at admission time. Either the Jobs failed silently (and
   nothing notices because no downstream depends on those roles yet) or
   they were never applied to that namespace. We do not know which —
   that uncertainty is itself a bug.

2. **Credentials inside ConfigMaps, not Secrets.** Twelve occurrences of
   `taskuser:taskpass` baked into `DATABASE_URL` strings across every
   Go service ConfigMap and the QA overlay; six occurrences of
   `guest:guest` in `RABBITMQ_URL` strings. ConfigMaps are not encrypted
   at rest in etcd and are not access-controlled the way Secrets are.

3. **Templates that look like real Secrets.** The `*.template.yml`
   files have `kind: Secret` and base64-encoded "placeholder" values.
   At a glance — and on a `kubectl apply -f` mis-targeting — they are
   indistinguishable from a real Secret. A reviewer would reasonably
   ask whether the placeholder is the prod value (it isn't, but
   nothing says so loudly).

4. **No bootstrap runbook.** "What does a fresh cluster need to come
   online?" is implicit. Today it lives in the operator's head.

5. **No CI guardrail for credentials in ConfigMaps.** `gitleaks` runs,
   but `taskuser:taskpass` doesn't match any of its patterns — it
   doesn't look like an API key. Future copy-paste would slip past.

## Goals

- Move all live Secret data under a tool that lets us commit the
  encrypted form to git, so the cluster's secret state is reproducible
  from `main`.
- Split DSN strings so credentials live in Secrets and the rest
  (host/port/db/options) lives in ConfigMaps.
- Document the bootstrap path: a runbook that takes a fresh Minikube
  cluster and produces a working set of Secrets without manual
  copy-paste of base64 strings.
- Add a CI guardrail that catches plaintext credentials inside
  ConfigMap data fields.
- Eliminate the drift between `*.template.yml` and the live cluster by
  removing the templates as a credential-source artifact.

## Non-goals

- A cloud KMS-backed solution (External Secrets Operator with AWS/GCP
  Secrets Manager). The portfolio runs on Minikube; introducing a
  cloud dependency just to manage local development credentials is
  the wrong trade-off.
- Rotating the actual credential values. This work is about *how*
  secrets are stored, not *which* secrets are valid. Rotation is its
  own follow-up.
- Re-architecting cert-manager. Per-service mTLS certs are already
  managed properly via `Certificate` resources; out of scope.
- Migrating GitHub Actions secrets (Tailscale authkey, GHCR creds,
  etc.). Those live in GitHub's encrypted store, which is a different
  problem domain.

## Decision

**Adopt [Sealed Secrets](https://github.com/bitnami-labs/sealed-secrets)**
as the in-cluster secrets management tool, and **split DSN strings**
into ConfigMap (non-credential parts) + Secret (credentials), with the
application building the connection string at startup.

### Why Sealed Secrets

| Option | Fit | Verdict |
|---|---|---|
| **Sealed Secrets** | Bitnami controller in cluster; `kubeseal` CLI encrypts a `Secret` against the controller's public key; the encrypted `SealedSecret` is safe to commit to git. Controller decrypts back into a real Secret on apply. Single-cluster simple. | **Choose.** Matches the Minikube + GitOps-ish pattern already in use. Zero cloud dependency. The encrypted form is committable, which closes the "what's in prod?" gap. |
| External Secrets Operator | Pulls from cloud KMS / Vault / Secrets Manager. Best when secrets already live in a cloud provider. | Overkill — we don't have a cloud provider in scope. |
| SOPS | Encrypts files; works with kustomize's `sops` plugin. No controller. | Decent, but the kustomize integration is fiddly and there's no central rotation story. Sealed Secrets' controller model is cleaner. |
| HashiCorp Vault | Heavy. Operator + storage + auth. | Overkill for portfolio scale. |
| Stay with manual `kubectl create secret` | Status quo. | Doesn't solve drift, doesn't solve bootstrap docs. Reject. |

### Why split DSNs

Today every Go service's ConfigMap looks like:

```yaml
DATABASE_URL: postgres://taskuser:taskpass@postgres.java-tasks.svc.cluster.local:5432/orderdb?sslmode=disable
```

A ConfigMap is the wrong place for a password. The right shape:

```yaml
# ConfigMap (non-secret, freely committable)
DB_HOST: postgres.java-tasks.svc.cluster.local
DB_PORT: "5432"
DB_NAME: orderdb
DB_OPTIONS: sslmode=disable

# Secret (sealed)
DB_USER: taskuser
DB_PASSWORD: <strong>
```

The application builds the DSN at startup. This is how 12-factor /
production teams structure connection config — credentials never enter
the ConfigMap surface.

## Architecture

```
sealed-secrets-system namespace
├── sealed-secrets-controller Deployment (bitnami/sealed-secrets-controller)
│   └── Generates a cluster-wide public/private keypair on first start;
│       private key never leaves the cluster.
└── sealing-key Secret (managed by controller; backed up out-of-band)

Repo
├── secrets/
│   ├── java-tasks/
│   │   └── java-secrets.sealed.yml          (committable; encrypted)
│   ├── go-ecommerce/
│   │   ├── go-secrets.sealed.yml
│   │   └── stripe-secrets.sealed.yml
│   ├── monitoring/
│   │   └── telegram-bot.sealed.yml
│   └── README.md                            (bootstrap runbook)
└── (delete) java/k8s/secrets/, go/k8s/secrets/

Application code
├── go/<service>/cmd/server/config.go
│   └── Build DATABASE_URL from DB_HOST/DB_PORT/DB_NAME/DB_OPTIONS (ConfigMap)
│       + DB_USER/DB_PASSWORD (Secret).
│       Same pattern for RABBITMQ_URL, REDIS_URL.
└── (no change to imports / framework code)

Kubernetes manifests
├── go/k8s/configmaps/<service>-config.yml
│   └── DATABASE_URL removed; DB_HOST/DB_PORT/DB_NAME/DB_OPTIONS added
├── go/k8s/deployments/<service>.yml
│   └── envFrom: { configMapRef: <service>-config, secretRef: <service>-db }
└── k8s/overlays/qa-go/kustomization.yaml
    └── DB_NAME patched to *_qa; everything else inherits
```

## Implementation phases

### Phase 1 — Land Sealed Secrets controller + bootstrap runbook

Goal: the tool is installed, encrypted secrets are committable, but no
existing manifests change yet.

- Add `k8s/sealed-secrets/` with the controller install manifest
  (single YAML pinned to a release tag).
- Update `k8s/deploy.sh` and CI to apply it.
- Document `kubeseal` install + sealing-key backup in
  `secrets/README.md`. The sealing key backup is the load-bearing
  operational artifact: lose it and *all* sealed secrets in the repo
  become un-decryptable.
- Verify by sealing one trivial secret end-to-end and confirming the
  controller decrypts it.

### Phase 2 — Migrate the four live Secrets into the repo as `SealedSecret` resources

For each of `java-secrets`, `go-secrets`, `stripe-secrets`,
`telegram-bot`:

1. Read the live values from the cluster (out-of-band — operator's
   responsibility, not the agent's).
2. Run `kubeseal` to produce the encrypted YAML.
3. Commit it under `secrets/<namespace>/<name>.sealed.yml`.
4. Reference from the appropriate kustomization.
5. Delete the corresponding `*.template.yml` from the repo.

The ordering matters: we apply the new `SealedSecret`, the controller
materializes a `Secret` with the same name, the app sees no change.
Then we delete the template.

### Phase 3 — Add the missing keys

For each key that the manifests reference but the live cluster doesn't
have (`replicator-password`, `grafana-reader-password`,
`pgbouncer-auth-password`):

1. Generate a strong random value (operator runs `openssl rand`).
2. Seal it; commit.
3. Re-run the corresponding bootstrap Job
   (`postgres-replicator-bootstrap`, etc.); confirm it succeeds.

This phase closes the live-vs-manifest drift.

### Phase 4 — Split DSNs (Go services first)

Per Go service:

1. Add `DB_HOST`, `DB_PORT`, `DB_NAME`, `DB_OPTIONS` to the ConfigMap;
   remove `DATABASE_URL`. Keep `DATABASE_URL_DIRECT` for migration
   Jobs (or split it the same way — same fields, different `DB_NAME`
   modes). Decision in the implementation plan.
2. Add `DB_USER`, `DB_PASSWORD` to a per-service `<service>-db` Secret
   (sealed).
3. In `cmd/server/config.go`, build the DSN at startup. Existing
   `pgxpool` consumers don't change.
4. Repeat for `RABBITMQ_URL` (`MQ_HOST`, `MQ_PORT`, `MQ_VHOST` in
   ConfigMap; `MQ_USER`, `MQ_PASSWORD` in Secret).
5. `REDIS_URL` follows the same pattern when it grows a password (it
   doesn't today; defer).

Order services from least-to-most central — auth-service is most
central, do it last. Roll out one service per merge so a rollback is
narrow.

### Phase 5 — CI guardrail

Add a CI check that fails when a ConfigMap `data` field contains
`://[^@]*@` (a password-bearing URL) or matches a small set of
known-bad patterns (`password=`, `:taskpass`, `guest:guest`).

Implementation: a small Go or shell script in `.github/scripts/` plus
a workflow step. Tests against fixtures that exercise both the
positive and negative cases.

### Phase 6 — Java services + monitoring

The Java services use Spring's environment-variable substitution; the
DSN-split pattern is the same shape but with different config wiring.
Defer to its own phase to keep individual PRs reviewable.

## Bootstrap runbook (deliverable)

`secrets/README.md` will document the fresh-cluster bootstrap:

1. Install Sealed Secrets controller: `kubectl apply -f k8s/sealed-secrets/`.
2. Wait for the controller; export the public cert (`kubeseal --fetch-cert`).
3. Apply all `secrets/**/*.sealed.yml`.
4. Confirm Secrets materialize: `kubectl get secrets -A`.
5. Sealing-key backup procedure (out-of-band; operator's encrypted
   personal vault). Without this, a `minikube delete` is unrecoverable.

## Trade-offs

**Positive:**
- The cluster's secret state becomes reproducible from `main` (modulo
  the sealing key, which is the one out-of-band artifact).
- Drift between manifests and live cluster is closed and stays closed.
- ConfigMaps stop carrying credentials, which is the structurally
  correct shape and a recognizable pattern at code review time.
- A real bootstrap runbook exists.
- A CI guardrail catches the regression class going forward.

**Trade-offs / costs:**
- One more controller in the cluster (modest — Sealed Secrets is small).
- The sealing key is a load-bearing artifact: lose it and committed
  sealed secrets become useless. The mitigation is documenting and
  honoring the backup procedure.
- Existing app code grows a small DSN-builder helper. Net effect on
  readability is positive (config struct fields read like a 12-factor
  app), but it is a code change.
- Migration is touchy: each service migration is a coordinated
  ConfigMap + Secret + Deployment change. Ordering matters; do one
  service per PR.

**Phase-2-and-beyond ideas (not now):**
- Cloud KMS-backed secrets via External Secrets Operator if the
  portfolio grows a cloud target.
- Automated rotation (separate concern, separate spec).
- A `make seal` developer command that wraps `kubeseal` for the
  common case.

## Companion ADR

`docs/adr/security/secrets-management.md` covering:

- Why Sealed Secrets over External Secrets / SOPS / Vault for this
  context.
- Why DSN split rather than "store the whole connection string in a
  Secret."
- The sealing-key custody decision and its operational implications.
- Why the migration is multi-phase (per-service) rather than a single
  big-bang PR.

## Open questions

- **Stripe & Google OAuth keys** — these are real (non-portfolio)
  credentials. The migration must not log or echo them. Operator-only
  step; agent should not handle their values.
- **Sealing key backup destination** — operator's call. The runbook
  will state requirements, not prescribe a destination.
- **Where to put `secrets/` in the repo** — top-level or under
  `k8s/secrets/`? Slight preference for top-level because it's a
  cross-cutting concern (touches every namespace), but happy to defer
  to convention.
