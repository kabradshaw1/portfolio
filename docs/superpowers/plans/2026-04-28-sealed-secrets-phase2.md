# Plan: Sealed Secrets — Phase 2 (migrate the four live Secrets)

Spec: `docs/superpowers/specs/2026-04-28-secrets-management-design.md`
Branch: `agent/feat-sealed-secrets-phase2`

Phase 2 migrates the four live Secrets — `java-tasks/java-secrets`, `go-ecommerce/go-secrets`, `go-ecommerce/stripe-secrets`, `monitoring/telegram-bot` — into committed `*.sealed.yml` resources, then deletes the misleading `*.template.yml` files.

After this PR:
- `k8s/secrets/<namespace>/<name>.sealed.yml` files are committed and decrypt cleanly against the cluster's sealing key.
- Deploy applies them; the controller materializes the same Secrets that were already running. Apps see no change.
- The `*.template.yml` files are gone — no more "is this the prod value or a placeholder?" ambiguity.
- The README documents the deliberate tradeoff that the sealing key is **not** backed up out-of-band (portfolio scope; recovery = re-seal). That's a more honest signal of engineering judgment than fake-backing-up to a tool the operator doesn't otherwise use.

## Step 1 — README fixes
Update `k8s/sealed-secrets/README.md`:
- **Bug fix:** the v0.36.6 label is `sealedsecrets.bitnami.com/sealed-secrets-key=active` (not `…/sealing-key=active` which is what the Phase 1 README shipped with).
- **Backup posture rewrite:** replace the "store in 1Password / encrypted vault" runbook with the documented tradeoff:
  > Sealing key not backed up out-of-band. Recovery from key loss is "regenerate the keypair and re-seal from source-of-truth values," accepted because committed `*.sealed.yml` only encrypts portfolio-scope dev credentials. A real production deployment would back this up; the decision and reasoning live in `docs/adr/security/secrets-management.md`.

## Step 2 — Sealing script
Add `scripts/seal-from-cluster.sh`:
- For each `(namespace, secret-name)` tuple, runs `ssh debian "kubectl get secret -o yaml" | kubeseal --controller-namespace=kube-system --format=yaml > k8s/secrets/<ns>/<name>.sealed.yml`.
- Strips runtime metadata (`resourceVersion`, `uid`, `creationTimestamp`, managed fields) before sealing so the output is deterministic and reviewable.
- Cleartext never lands on disk; it flows through a pipe into kubeseal.

Operator runs this once per migration. Output files commit cleanly.

## Step 3 — Directory scaffolding
- `k8s/secrets/README.md` — what lives here and the kustomization wiring.
- `k8s/secrets/<namespace>/` — one directory per namespace (`java-tasks`, `go-ecommerce`, `monitoring`).
- `k8s/secrets/kustomization.yaml` — top-level kustomization referencing each namespace's resources.

## Step 4 — Sealed files (operator step)
Operator runs `scripts/seal-from-cluster.sh`. Output:
- `k8s/secrets/java-tasks/java-secrets.sealed.yml`
- `k8s/secrets/go-ecommerce/go-secrets.sealed.yml`
- `k8s/secrets/go-ecommerce/stripe-secrets.sealed.yml`
- `k8s/secrets/monitoring/telegram-bot.sealed.yml`

Operator commits the result onto this branch.

## Step 5 — Deploy wiring
- `k8s/deploy.sh` (minikube/aws/qa paths): `kubectl apply -k k8s/secrets/` after the controller is Ready.
- `.github/workflows/ci.yml` (Deploy QA + Deploy Production): same.

The wiring is order-sensitive — sealed secrets need the controller alive to decrypt — so it goes after the controller-install steps.

## Step 6 — Delete templates
- `rm java/k8s/secrets/java-secrets.yml.template`
- `rm go/k8s/secrets/go-secrets.yml.template`
- Update the existing manual-secret-application section in `k8s/deploy.sh` (it currently warns "java-secrets.yml.template not found — copy and fill in"). That whole stanza becomes unreachable once `k8s/secrets/` is the source of truth.

## Step 7 — ADR
Add `docs/adr/security/secrets-management.md`. Captures:
- Why Sealed Secrets over alternatives (links back to spec).
- The DSN component split (forward reference; arrives in Phase 4).
- The sealing-key-backup tradeoff as a deliberate scope decision.

## Out of scope (later phases)
- Phase 3 — close the template-vs-live drift (`replicator-password`, `grafana-reader-password`, `pgbouncer-auth-password` keys missing from live `java-secrets`).
- Phase 4 — DSN component split (ConfigMap host/port/db, Secret user/password).
- Phase 5 — CI guardrail against credentials in ConfigMaps.
- Phase 6 — Java services follow-up.

## Test plan
- `K8s Manifest Validation` CI job continues to pass; `k8s/secrets/*.sealed.yml` resources validate as `SealedSecret` CRD instances (CRD is installed cluster-wide by the Phase 1 controller).
- After merge to qa: `Deploy QA` succeeds; `kubectl get secrets -n java-tasks java-secrets -o jsonpath='{.metadata.ownerReferences}'` shows ownership by the corresponding `SealedSecret` (controller signature on the materialized Secret).
- App pods continue running with the same credential values (since Phase 2 keeps values identical).
- After merge to main: same in prod.
