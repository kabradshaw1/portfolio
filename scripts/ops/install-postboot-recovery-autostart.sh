#!/usr/bin/env bash
# Copy postboot scripts to Debian and install a no-sudo boot autostart entry.
# Idempotent: yes.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REMOTE_INSTALL_DIR="${REMOTE_INSTALL_DIR:-/home/kyle/.local/lib/portfolio-postboot}"
RECOVERY_SOURCE="${SCRIPT_DIR}/recover-after-host-boot.sh"
HEALTH_SOURCE="${SCRIPT_DIR}/check-postboot-health.sh"
RUNNER_SOURCE="${SCRIPT_DIR}/run-postboot-recovery.sh"

if [[ ! -x "$RECOVERY_SOURCE" ]]; then
  echo "ERROR: recovery script is missing or not executable: ${RECOVERY_SOURCE}" >&2
  exit 1
fi

if [[ ! -x "$HEALTH_SOURCE" ]]; then
  echo "ERROR: health script is missing or not executable: ${HEALTH_SOURCE}" >&2
  exit 1
fi

if [[ ! -x "$RUNNER_SOURCE" ]]; then
  echo "ERROR: runner script is missing or not executable: ${RUNNER_SOURCE}" >&2
  exit 1
fi

ssh debian install -d -m 0755 "$REMOTE_INSTALL_DIR"
scp "$RECOVERY_SOURCE" "$HEALTH_SOURCE" "$RUNNER_SOURCE" "debian:${REMOTE_INSTALL_DIR}/"

ssh debian bash -s -- "$REMOTE_INSTALL_DIR" <<'REMOTE'
set -euo pipefail

REMOTE_INSTALL_DIR="$1"
RECOVERY_SCRIPT="${REMOTE_INSTALL_DIR}/recover-after-host-boot.sh"
HEALTH_SCRIPT="${REMOTE_INSTALL_DIR}/check-postboot-health.sh"
RUNNER_SCRIPT="${REMOTE_INSTALL_DIR}/run-postboot-recovery.sh"
CRON_MARKER="# portfolio-postboot-recovery"
CRON_LINE="@reboot sleep 90; ${RUNNER_SCRIPT} ${CRON_MARKER}"

chmod 0755 "$RECOVERY_SCRIPT" "$HEALTH_SCRIPT" "$RUNNER_SCRIPT"

if [[ ! -x "$RECOVERY_SCRIPT" ]]; then
  echo "ERROR: recovery script is missing or not executable: ${RECOVERY_SCRIPT}" >&2
  exit 1
fi

if [[ ! -x "$HEALTH_SCRIPT" ]]; then
  echo "ERROR: health script is missing or not executable: ${HEALTH_SCRIPT}" >&2
  exit 1
fi

if ! command -v crontab >/dev/null 2>&1; then
  echo "ERROR: crontab is not installed on Debian." >&2
  exit 1
fi

echo "Installing managed @reboot crontab entry..."
existing_cron="$(crontab -l 2>/dev/null || true)"
{
  printf '%s\n' "$existing_cron" | grep -vF "$CRON_MARKER" || true
  printf '%s\n' "$CRON_LINE"
} | sed '/^$/d' | crontab -

echo "Installed postboot recovery autostart."
echo "Scripts copied to: ${REMOTE_INSTALL_DIR}"
echo "Boot logs: /home/kyle/.local/state/portfolio-postboot/latest.log"
echo "Run recovery manually with: ${RUNNER_SCRIPT}"
REMOTE
