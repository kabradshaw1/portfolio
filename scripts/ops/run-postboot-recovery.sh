#!/usr/bin/env bash
# Run postboot recovery followed by a read-only health check.
# Intended to be copied to Debian and invoked by user crontab @reboot.

set -euo pipefail

INSTALL_DIR="${INSTALL_DIR:-/home/kyle/.local/lib/portfolio-postboot}"
LOG_DIR="${LOG_DIR:-/home/kyle/.local/state/portfolio-postboot}"
WAIT_SECONDS="${WAIT_SECONDS:-900}"

mkdir -p "$LOG_DIR"

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
log_file="${LOG_DIR}/postboot-${timestamp}.log"
latest_log="${LOG_DIR}/latest.log"

{
  echo "postboot recovery started at $(date -Is)"
  echo "install_dir=${INSTALL_DIR}"
  WAIT_SECONDS="$WAIT_SECONDS" "${INSTALL_DIR}/recover-after-host-boot.sh"
  "${INSTALL_DIR}/check-postboot-health.sh"
  echo "postboot recovery finished at $(date -Is)"
} >"$log_file" 2>&1

ln -sfn "$log_file" "$latest_log"
