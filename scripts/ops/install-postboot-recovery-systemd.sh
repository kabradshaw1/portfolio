#!/usr/bin/env bash
# Install Debian systemd units for portfolio postboot recovery and health checks.
# Idempotent: yes.

set -euo pipefail

REMOTE_REPO="${REMOTE_REPO:-/home/kyle/repos/gen_ai_engineer}"

ssh debian bash -s -- "$REMOTE_REPO" <<'REMOTE'
set -euo pipefail

REMOTE_REPO="$1"
RECOVERY_SCRIPT="${REMOTE_REPO}/scripts/ops/recover-after-host-boot.sh"
HEALTH_SCRIPT="${REMOTE_REPO}/scripts/ops/check-postboot-health.sh"

if [[ ! -x "$RECOVERY_SCRIPT" ]]; then
  echo "ERROR: recovery script is missing or not executable: ${RECOVERY_SCRIPT}" >&2
  exit 1
fi

if [[ ! -x "$HEALTH_SCRIPT" ]]; then
  echo "ERROR: health script is missing or not executable: ${HEALTH_SCRIPT}" >&2
  exit 1
fi

echo "Installing portfolio-postboot-recovery.service..."
sudo tee /etc/systemd/system/portfolio-postboot-recovery.service >/dev/null <<UNIT
[Unit]
Description=Portfolio postboot recovery
After=network-online.target docker.service tailscaled.service minikube.service minikube-tunnel.service cloudflared.service
Wants=network-online.target docker.service tailscaled.service minikube.service minikube-tunnel.service cloudflared.service

[Service]
Type=oneshot
User=kyle
WorkingDirectory=${REMOTE_REPO}
Environment=WAIT_SECONDS=900
ExecStart=${RECOVERY_SCRIPT}
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
UNIT

echo "Installing portfolio-postboot-health.service..."
sudo tee /etc/systemd/system/portfolio-postboot-health.service >/dev/null <<UNIT
[Unit]
Description=Portfolio postboot health check
After=network-online.target portfolio-postboot-recovery.service
Wants=network-online.target

[Service]
Type=oneshot
User=kyle
WorkingDirectory=${REMOTE_REPO}
ExecStart=${HEALTH_SCRIPT}
StandardOutput=journal
StandardError=journal
UNIT

echo "Installing portfolio-postboot-health.timer..."
sudo tee /etc/systemd/system/portfolio-postboot-health.timer >/dev/null <<'UNIT'
[Unit]
Description=Run portfolio postboot health checks periodically

[Timer]
OnBootSec=5min
OnUnitActiveSec=15min
Persistent=true
Unit=portfolio-postboot-health.service

[Install]
WantedBy=timers.target
UNIT

sudo systemctl daemon-reload
sudo systemctl enable portfolio-postboot-recovery.service
sudo systemctl enable --now portfolio-postboot-health.timer

echo "Installed postboot recovery units."
echo "Run recovery manually with: sudo systemctl start portfolio-postboot-recovery.service"
echo "Check health with: sudo systemctl start portfolio-postboot-health.service && systemctl status portfolio-postboot-health.service"
REMOTE
