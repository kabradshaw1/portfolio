# Sealed Secrets

This directory is the cluster-level home for [Sealed Secrets](https://github.com/bitnami-labs/sealed-secrets) — the secrets-management tool the project uses to commit encrypted Secret values to git safely.

The actual controller install is fetched from a pinned upstream release (mirrors how `k8s/cert-manager/` works). Committed `*.sealed.yml` resources for individual Secrets land in `k8s/secrets/` in Phase 2 of the secrets-management migration; this directory holds the controller-level concerns only.

> **Status:** Phase 1 of the migration plan in
> `docs/superpowers/specs/2026-04-28-secrets-management-design.md`. The
> controller is installed but no Secrets are sealed yet. Existing
> `*.template.yml` files remain in place until Phase 2.

## Version pin

| Component | Version | Released | Source |
|---|---|---|---|
| `bitnami-labs/sealed-secrets` controller | `v0.36.6` | 2026-02 | <https://github.com/bitnami-labs/sealed-secrets/releases> |

Refresh procedure when bumping:

1. Check the [release notes](https://github.com/bitnami-labs/sealed-secrets/releases) for breaking changes — controller upgrades have historically been backwards-compatible, but read before assuming.
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
kubectl get secret -n kube-system -l sealedsecrets.bitnami.com/sealing-key=active
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

## Sealing-key custody

**The single load-bearing operational artifact in this whole system is the cluster's sealing keypair.** It lives as a Secret labeled `sealedsecrets.bitnami.com/sealing-key=active` in `kube-system`. If the cluster is destroyed and the sealing key is not restored, every committed `*.sealed.yml` in this repo becomes un-decryptable.

### Backup procedure

Run after the controller is first deployed, and again any time the controller rotates the key (it doesn't rotate by default — rotation is opt-in):

```bash
# Export the active sealing key(s) to a local file.
kubectl get secret -n kube-system \
  -l sealedsecrets.bitnami.com/sealing-key=active \
  -o yaml > sealing-key-backup-$(date +%Y%m%d).yaml
```

Store the exported file in the operator's encrypted personal vault (1Password, age-encrypted file in a separate repo, etc.). **Do not commit it.**

### Restore procedure (after `minikube delete` or cluster rebuild)

```bash
# Apply the backed-up key BEFORE the controller starts for the first time
# (or restart the controller after applying so it adopts the existing key).
kubectl apply -f sealing-key-backup-YYYYMMDD.yaml
kubectl rollout restart deployment/sealed-secrets-controller -n kube-system
```

The controller will adopt the restored key, and all committed `*.sealed.yml` in the repo will decrypt against it.

## Sealing a Secret (forward reference, Phase 2)

The full migration uses this pattern. Documented here for completeness; not yet exercised in this phase.

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
kubeseal \
  --controller-namespace=kube-system \
  --format=yaml \
  < /tmp/example-secret.yaml \
  > k8s/secrets/default/example-secret.sealed.yml

# 3. Commit the .sealed.yml. The controller materializes a real
#    Secret with the same name when applied.
rm /tmp/example-secret.yaml
```

## What's NOT here

- The controller manifest YAML itself (~2000 lines). Fetched from the upstream release URL at deploy time, like cert-manager.
- Committed `*.sealed.yml` resources. Those land in `k8s/secrets/<namespace>/` in Phase 2.
- The sealing-key backup file. Out-of-band, by design.
