# Plan: Sealed Secrets — Phase 1 (controller install + bootstrap runbook)

Spec: `docs/superpowers/specs/2026-04-28-secrets-management-design.md`
Branch: `agent/feat-sealed-secrets-phase1`

Phase 1 lands the tool only. No existing manifests change; no live secrets are migrated yet (that's Phase 2). After this PR:

- The Sealed Secrets controller runs in the cluster.
- `kubeseal` install + sealing-key backup procedure is documented.
- The deploy pipeline knows how to keep the controller up to date.

## Step 1 — Repo home for Sealed Secrets artifacts

- Create `k8s/sealed-secrets/` directory.
- `k8s/sealed-secrets/README.md` — describes the controller version pin, install procedure, sealing-key backup procedure, kubeseal CLI install, and forward-references where Phase 2 will land committed `*.sealed.yml` resources.

The directory mirrors `k8s/cert-manager/`: a small home for the cluster-level concern, with the actual heavy install applied from a pinned remote URL (so we don't vendor a 2000-line third-party manifest into the repo).

## Step 2 — Pipeline integration (`.github/workflows/ci.yml`)

Mirror the existing cert-manager pattern:

- **Deploy QA** job: after Tailscale, before applying the QA overlays, apply Sealed Secrets controller and wait for it to be Ready.
- **Deploy Production** job: same.

Pinned version: `v0.36.6` (latest stable as of 2026-04-28). Refresh procedure documented in the README.

K8s Manifest Validation: add `-not -path '*/sealed-secrets/*'` to the `find` skip list (the directory only contains a README today, but the exclusion is consistent with cert-manager and future-proofs against committing the controller YAML).

## Step 3 — Local deploy script (`k8s/deploy.sh`)

Same pattern: install Sealed Secrets controller in the `minikube`, `qa`, and `aws` paths before applying overlays. Best-effort (`|| true`) so a re-run doesn't fail when the controller is already installed.

## Step 4 — Verification (post-deploy, not in CI)

The PR description's test plan asks the operator to confirm end-to-end:

1. Controller pod is Running in `kube-system` (the default namespace for the bitnami install).
2. `kubeseal --fetch-cert` returns a public certificate.
3. Seal a trivial test secret, apply it, confirm the controller materializes a real Secret.

This isn't part of the PR's CI gates because it requires the live cluster; it's the operator handoff at merge time.

## Out of scope for Phase 1

- Migrating any existing live Secret. Phase 2.
- Closing the template-vs-live drift (replicator-password, etc.). Phase 3.
- DSN component split. Phase 4.
- CI guardrail. Phase 5.
- Java services. Phase 6.

## Test plan

- `K8s Manifest Validation` CI job passes (the new directory is excluded but the find still terminates cleanly).
- After merge to qa: `Deploy QA` succeeds; `kubectl get deploy -n kube-system | grep sealed-secrets-controller` shows the controller available.
- After merge to main: same in prod.
- Operator confirms `kubeseal --fetch-cert` returns a key against both QA and prod.
