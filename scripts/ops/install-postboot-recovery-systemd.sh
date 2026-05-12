#!/usr/bin/env bash
# Copy postboot scripts to Debian and install systemd units for them.
# Idempotent: yes.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REMOTE_INSTALL_DIR="${REMOTE_INSTALL_DIR:-/home/kyle/.local/lib/portfolio-postboot}"
RECOVERY_SOURCE="${SCRIPT_DIR}/recover-after-host-boot.sh"
HEALTH_SOURCE="${SCRIPT_DIR}/check-postboot-health.sh"

if [[ ! -x "$RECOVERY_SOURCE" ]]; then
  echo "ERROR: recovery script is missing or not executable: ${RECOVERY_SOURCE}" >&2
  exit 1
fi

if [[ ! -x "$HEALTH_SOURCE" ]]; then
  echo "ERROR: health script is missing or not executable: ${HEALTH_SOURCE}" >&2
  exit 1
fi

ssh debian install -d -m 0755 "$REMOTE_INSTALL_DIR"
scp "$RECOVERY_SOURCE" "$HEALTH_SOURCE" "debian:${REMOTE_INSTALL_DIR}/"

ssh debian bash -s -- "$REMOTE_INSTALL_DIR" <<'REMOTE'
set -euo pipefail

REMOTE_INSTALL_DIR="$1"
RECOVERY_SCRIPT="${REMOTE_INSTALL_DIR}/recover-after-host-boot.sh"
HEALTH_SCRIPT="${REMOTE_INSTALL_DIR}/check-postboot-health.sh"

chmod 0755 "$RECOVERY_SCRIPT" "$HEALTH_SCRIPT"

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
WorkingDirectory=/home/kyle
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
WorkingDirectory=/home/kyle
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
