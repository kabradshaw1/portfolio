# Sealed Secrets — committed encrypted resources

This directory holds `*.sealed.yml` files: encrypted `SealedSecret` resources that the in-cluster Sealed Secrets controller decrypts into ordinary Kubernetes Secrets at apply time. **These files are safe to commit to git.** The cluster's controller is the only thing that can decrypt them.

```
k8s/secrets/
├── README.md                                   # this file
├── kustomization.yaml                          # references every namespace below
├── java-tasks/
│   └── java-secrets.sealed.yml                 # google-client-id/secret, jwt-secret, postgres-password
├── go-ecommerce/
│   ├── go-secrets.sealed.yml                   # google-client-id/secret, jwt-secret
│   └── stripe-secrets.sealed.yml               # STRIPE_SECRET_KEY, STRIPE_WEBHOOK_SECRET
└── monitoring/
    └── telegram-bot.sealed.yml                 # bot-token
```

## How this is wired

- The Sealed Secrets controller lives in `kube-system` (installed by `k8s/deploy.sh` and `.github/workflows/ci.yml`; see [`k8s/sealed-secrets/README.md`](../sealed-secrets/README.md) for the install + version pin).
- Every deploy applies this directory after the controller is Ready: `kubectl apply -k k8s/secrets/`.
- The controller watches `SealedSecret` resources cluster-wide. For each one, it decrypts the payload using the cluster's private sealing key and creates (or updates) a regular `Secret` in the same namespace with the same name.
- App workloads (Deployments, StatefulSets, Jobs) reference these Secrets via `secretKeyRef` exactly as they would any other Secret. They don't know or care that the source-of-truth lives in git.

## Adding a new sealed Secret

1. Add the `(namespace, name)` pair to the `SECRETS` list in `scripts/seal-from-cluster.sh`.
2. Add the live Secret to the cluster out-of-band (operator step — values come from your records, an external service console, etc.).
3. Run `scripts/seal-from-cluster.sh <name>` to read the live Secret, strip runtime metadata, encrypt against the controller's public key, and write the result to `k8s/secrets/<namespace>/<name>.sealed.yml`.
4. Add the new path to `k8s/secrets/<namespace>/kustomization.yaml`.
5. Commit, push, deploy.

For a Secret created from scratch (no live counterpart yet), see the "Create a new Secret from scratch" section of [`k8s/sealed-secrets/README.md`](../sealed-secrets/README.md).

## Re-sealing after key loss

If the cluster's sealing key is regenerated (because it was lost, intentionally rotated, or the cluster was rebuilt), every committed `*.sealed.yml` here becomes un-decryptable against the new key. Recovery is to re-run `scripts/seal-from-cluster.sh` against the live cluster (which now has the fresh public key) — provided the live Secrets still exist or have been re-created from your records. See [`k8s/sealed-secrets/README.md`](../sealed-secrets/README.md) for the full path.

## What's NOT here

- Secrets that the cluster manages itself (cert-manager-issued mTLS certs, service-account tokens, Sealed Secrets' own sealing key). Those are cluster-internal and live where they live.
- Secrets in CI / GitHub Actions (Tailscale authkey, GHCR credentials). Those live in GitHub's encrypted secrets store — different problem domain.
