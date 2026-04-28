# Sealed Secrets

This directory is the cluster-level home for [Sealed Secrets](https://github.com/bitnami-labs/sealed-secrets) — the secrets-management tool the project uses to commit encrypted Secret values to git safely.

The actual controller install is fetched from a pinned upstream release (mirrors how `k8s/cert-manager/` works). Committed `*.sealed.yml` resources for individual Secrets live in [`k8s/secrets/`](../secrets/); this directory holds the controller-level concerns only.

## Version pin

| Component | Version | Released | Source |
|---|---|---|---|
| `bitnami-labs/sealed-secrets` controller | `v0.36.6` | 2026-02 | <https://github.com/bitnami-labs/sealed-secrets/releases> |

Refresh procedure when bumping:

1. Check the [release notes](https://github.com/bitnami-labs/sealed-secrets/releases) for breaking changes.
2. Update the version pin in three places: this README, `k8s/deploy.sh`, and `.github/workflows/ci.yml` (Deploy QA + Deploy Production jobs).
3. Open a small PR. The controller is hot-reloadable; no application restart is required.

## Install (operator runbook)

The CI deploy job and `k8s/deploy.sh` apply this automatically. The manual steps below are for fresh-cluster bootstrap.

```bash
# 1. Apply the controller (creates kube-system/sealed-secrets-controller).
kubectl apply -f https://github.com/bitnami-labs/sealed-secrets/releases/download/v0.36.6/controller.yaml

# 2. Wait for the controller to be Ready.
kubectl wait --for=condition=available --timeout=120s deployment/sealed-secrets-controller -n kube-system

# 3. Verify the controller has minted a sealing keypair.
kubectl get secret -n kube-system -l sealedsecrets.bitnami.com/sealed-secrets-key=active
```

## kubeseal CLI install

`kubeseal` is the client-side tool that encrypts a normal `Secret` into a `SealedSecret` against the cluster's public key. Install once on the operator's workstation:

```bash
# macOS
brew install kubeseal

# Verify against the live cluster
kubeseal --fetch-cert --controller-namespace=kube-system
```

If `kubeseal --fetch-cert` returns a PEM block, the controller is reachable and the local CLI is wired up.

## Sealing-key custody — portfolio scope tradeoff

The single load-bearing operational artifact in this whole system is the cluster's sealing keypair. It lives as a Secret labeled `sealedsecrets.bitnami.com/sealed-secrets-key=active` in `kube-system`. If the cluster is destroyed and the sealing key is not restored, every committed `*.sealed.yml` in this repo becomes un-decryptable.

**Stance for this project: the key is *not* backed up out-of-band.**

This is a deliberate scope decision, not an oversight. The reasoning:

- This is a portfolio cluster running on a single Minikube instance. The committed `*.sealed.yml` files encrypt portfolio-scope dev credentials — placeholder Postgres passwords, dev JWT secrets, OAuth client IDs for a personal Google project. Nothing here protects revenue or PII.
- The recovery path on key loss is "regenerate the keypair, re-seal from source-of-truth values, push a PR." That's ~30 minutes of work, exercised exactly when needed. No standing operational tax.
- A real production deployment would back the key up to a managed KMS (AWS KMS, GCP KMS, Vault Transit) or an operator-controlled encrypted vault (1Password, age + hardware token). The cost of *that* setup — and the maintenance discipline it requires — is unjustified at portfolio scope.

The full reasoning lives in [`docs/adr/security/secrets-management.md`](../../docs/adr/security/secrets-management.md).

### If the key is ever lost

```bash
# 1. Regenerate (the controller mints a fresh keypair on next start).
kubectl delete secret -n kube-system -l sealedsecrets.bitnami.com/sealed-secrets-key=active
kubectl rollout restart deployment/sealed-secrets-controller -n kube-system
kubectl wait --for=condition=available --timeout=120s deployment/sealed-secrets-controller -n kube-system

# 2. Re-seal each committed *.sealed.yml from source-of-truth values.
#    See scripts/seal-from-cluster.sh for the cluster-current values, or
#    reconstruct from your local notes / external service consoles
#    (Stripe dashboard, Google Cloud Console, Telegram BotFather).

# 3. Open a PR with the re-sealed files. Apps continue running because
#    the materialized Secret values haven't changed — only the
#    SealedSecret encrypted form has.
```

## Sealing a Secret

Two paths depending on whether you're migrating an existing live Secret or creating a new one.

### Migrate an existing live Secret

Use `scripts/seal-from-cluster.sh`. It reads each tracked Secret from the cluster, strips runtime metadata, and writes the encrypted `SealedSecret` to the right location under `k8s/secrets/`. The cleartext flows through a pipe to `kubeseal` and never lands on disk.

### Create a new Secret from scratch

```bash
# 1. Build a regular Secret manifest (do not commit this file).
cat <<EOF > /tmp/example-secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: example-secret
  namespace: default
type: Opaque
stringData:
  api-key: super-secret-value
EOF

# 2. Encrypt it against the cluster's public key.
kubeseal --controller-namespace=kube-system --format=yaml \
  < /tmp/example-secret.yaml \
  > k8s/secrets/default/example-secret.sealed.yml

# 3. Commit the .sealed.yml. The controller materializes a real
#    Secret with the same name when the file is applied.
rm /tmp/example-secret.yaml
```

## What's NOT here

- The controller manifest YAML itself (~2000 lines). Fetched from the upstream release URL at deploy time, like cert-manager.
- Committed `*.sealed.yml` resources. Those live in [`k8s/secrets/<namespace>/`](../secrets/).
