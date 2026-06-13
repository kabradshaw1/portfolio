#!/usr/bin/env bash
# Writes /etc/docker/daemon.json on the debian host so the docker daemon
# (and any container it spawns) has explicit DNS fallbacks beyond the
# Tailscale MagicDNS resolver. Prevents repeats of the 2026-06-13 incident
# where Tailscale @ 100.100.100.100 returned SERVFAIL after a docker
# +tailscale restart and pods couldn't be created with usable DNS.
#
# Resolver order:
#   100.100.100.100  Tailscale MagicDNS (preserves MagicDNS for tailnet hosts)
#   1.1.1.1          Cloudflare (public fallback)
#   8.8.8.8          Google (public fallback)
#
# IMPORTANT: The new DNS config does NOT take effect until docker is
# restarted (`sudo systemctl restart docker`). On this host minikube runs
# with the docker driver, so the docker restart will restart the minikube
# container and cause ~minutes of cluster downtime. Schedule this for a
# maintenance window.
#
# Idempotent: re-running is safe; it produces the same file.

set -euo pipefail

SSH_HOST="${SSH_HOST:-debian}"
DAEMON_JSON='{
  "dns": ["100.100.100.100", "1.1.1.1", "8.8.8.8"]
}'

echo "=== Pre-state ==="
ssh "$SSH_HOST" "sudo test -f /etc/docker/daemon.json && sudo cat /etc/docker/daemon.json || echo '(no daemon.json)'"

echo "=== Writing /etc/docker/daemon.json ==="
ssh "$SSH_HOST" "sudo install -d -m 0755 /etc/docker"
ssh "$SSH_HOST" "echo '${DAEMON_JSON}' | sudo tee /etc/docker/daemon.json >/dev/null"

echo "=== Validating JSON ==="
ssh "$SSH_HOST" "sudo cat /etc/docker/daemon.json | python3 -m json.tool >/dev/null && echo OK"

echo "=== Post-state ==="
ssh "$SSH_HOST" "sudo cat /etc/docker/daemon.json"

cat <<'EOF'

Next step (NOT done by this script — causes minikube downtime):
  ssh debian "sudo systemctl restart docker"

After restart:
  - the docker daemon and any newly-created containers will use the new DNS
  - minikube control plane comes back automatically; deployments roll
  - verify with: ssh debian "kubectl get pods -A | grep -vE 'Running|Completed'"
EOF
