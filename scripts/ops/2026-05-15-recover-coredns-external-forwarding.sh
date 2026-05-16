#!/usr/bin/env bash
# Recover CoreDNS external forwarding when in-cluster service DNS works but
# public names return SERVFAIL from the Debian-hosted Minikube cluster.
# Idempotent: yes.

set -euo pipefail

ssh debian bash <<'REMOTE'
set -euo pipefail

verify_external_dns() {
  kubectl exec -n go-ecommerce deploy/go-auth-service -- \
    sh -c 'getent hosts oauth2.googleapis.com >/dev/null && getent hosts www.googleapis.com >/dev/null'
}

echo "Checking external DNS from the auth service pod..."
if verify_external_dns; then
  echo "External DNS is already healthy."
  exit 0
fi

echo "Repairing Minikube node resolver configuration..."
minikube ssh -- \
  "printf '%s\n' 'nameserver 100.100.100.100' 'nameserver 1.1.1.1' 'nameserver 8.8.8.8' 'options ndots:0' | sudo tee /etc/resolv.conf >/dev/null" \
  </dev/null

echo "Verifying DNS from inside the Minikube node..."
minikube ssh -- 'nslookup oauth2.googleapis.com >/dev/null && nslookup www.googleapis.com >/dev/null' </dev/null

echo "Restarting CoreDNS so it picks up the repaired node resolver..."
kubectl rollout restart deployment/coredns -n kube-system
kubectl rollout status deployment/coredns -n kube-system --timeout=2m

echo "Waiting for auth service external DNS to recover..."
for _ in $(seq 1 30); do
  if verify_external_dns; then
    echo "External DNS recovered."
    exit 0
  fi
  sleep 2
done

echo "ERROR: external DNS still fails from the auth service pod." >&2
kubectl logs -n kube-system deploy/coredns --tail=80 >&2
exit 1
REMOTE
