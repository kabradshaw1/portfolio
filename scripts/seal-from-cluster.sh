#!/usr/bin/env bash
# seal-from-cluster.sh — read each tracked live Secret from the cluster,
# strip runtime metadata, and emit a SealedSecret to the right location
# under k8s/secrets/<namespace>/.
#
# Cleartext Secret YAML flows through a pipe into kubeseal and never
# lands on disk. The committed *.sealed.yml is encrypted against the
# cluster's controller public key (see k8s/sealed-secrets/README.md).
#
# Usage:
#   scripts/seal-from-cluster.sh           # seal everything
#   scripts/seal-from-cluster.sh <name>    # seal one (e.g., telegram-bot)
#
# Prereqs:
#   - kubeseal installed locally (`brew install kubeseal`).
#   - SSH access to the cluster (`ssh debian` configured).
#   - Sealed Secrets controller live in kube-system (Phase 1).

set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SECRETS_DIR="$REPO_DIR/k8s/secrets"

# (namespace, secret-name) pairs to migrate. Add a row when introducing
# a new live Secret that should land in the repo.
SECRETS=(
  "java-tasks/java-secrets"
  "java-tasks-qa/java-secrets"
  "go-ecommerce/go-secrets"
  "go-ecommerce/stripe-secrets"
  "go-ecommerce/order-projector-db"
  "go-ecommerce-qa/order-projector-db"
  "monitoring/telegram-bot"
)

# Optional filter: if the caller passed a single secret name, only
# process matching rows.
FILTER="${1:-}"

# kubeseal needs the controller's public cert. Pull the cert (only) out
# of the active sealing-key Secret in kube-system. The cert is public —
# only the private half (which never leaves the cluster) can decrypt.
# We fetch via SSH because the operator's local workstation typically
# doesn't have a kubeconfig pointed at the Minikube cluster on debian.
CERT_FILE="$(mktemp)"
trap 'rm -f "$CERT_FILE"' EXIT
ssh debian "kubectl get secret -n kube-system \
    -l sealedsecrets.bitnami.com/sealed-secrets-key=active \
    -o jsonpath='{.items[0].data.tls\.crt}'" \
  | base64 -d > "$CERT_FILE"

if [ ! -s "$CERT_FILE" ]; then
  echo "ERROR: failed to fetch the Sealed Secrets controller certificate." >&2
  echo "       Is the controller installed in kube-system? See k8s/sealed-secrets/README.md." >&2
  exit 1
fi

for entry in "${SECRETS[@]}"; do
  ns="${entry%/*}"
  name="${entry#*/}"

  if [ -n "$FILTER" ] && [ "$FILTER" != "$name" ]; then
    continue
  fi

  out_dir="$SECRETS_DIR/$ns"
  out_file="$out_dir/$name.sealed.yml"
  mkdir -p "$out_dir"

  echo "==> Sealing $ns/$name -> ${out_file#$REPO_DIR/}"

  # Pipe cleartext Secret YAML through:
  #   1. kubectl neat / yq cleanup to strip resourceVersion, uid,
  #      creationTimestamp, managedFields (so the sealed output is
  #      reviewable and deterministic).
  #   2. kubeseal to encrypt against the cluster's public key.
  #
  # Cleartext is never written to disk in this pipeline.
  ssh debian "kubectl get secret -n '$ns' '$name' -o yaml" \
    | yq eval '
        del(.metadata.resourceVersion) |
        del(.metadata.uid) |
        del(.metadata.creationTimestamp) |
        del(.metadata.managedFields) |
        del(.metadata.annotations."kubectl.kubernetes.io/last-applied-configuration")
      ' - \
    | kubeseal --cert "$CERT_FILE" --format=yaml \
    > "$out_file"

  echo "    wrote $(wc -l < "$out_file") lines"
done

echo
echo "Done. Review the sealed files, commit them onto this branch, and let the deploy"
echo "pick them up. The materialized Secret values are unchanged — only the storage"
echo "mechanism (in-cluster live Secret -> committed SealedSecret -> materialized"
echo "Secret) has changed."
