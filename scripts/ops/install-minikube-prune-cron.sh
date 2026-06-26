#!/usr/bin/env bash
# Install the minikube docker prune script on Debian plus a managed hourly crontab
# entry. Detects whether docker needs sudo. Idempotent: yes.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SOURCE="${SCRIPT_DIR}/prune-minikube-docker.sh"
REMOTE_DIR="${REMOTE_DIR:-/home/kyle/.local/lib/portfolio-diskclean}"
TEXTFILE_DIR="${TEXTFILE_DIR:-/var/tmp/portfolio-diskclean}"

[[ -f "$SOURCE" ]] || { echo "ERROR: missing $SOURCE" >&2; exit 1; }

ssh debian install -d -m 0755 "$REMOTE_DIR"
scp "$SOURCE" "debian:${REMOTE_DIR}/prune-minikube-docker.sh"

ssh debian bash -s -- "$REMOTE_DIR" "$TEXTFILE_DIR" <<'REMOTE'
set -euo pipefail
REMOTE_DIR="$1"; TEXTFILE_DIR="$2"
SCRIPT="${REMOTE_DIR}/prune-minikube-docker.sh"
MARKER="# portfolio-diskclean"
chmod 0755 "$SCRIPT"

if docker ps >/dev/null 2>&1; then DOCKER="docker"; else DOCKER="sudo docker"; fi
echo "docker invocation: ${DOCKER}"

# The metrics textfile dir lives inside the minikube node and is created by the
# prune script via docker exec — nothing to create on the host here.

CRON_LINE="0 * * * * DOCKER=\"${DOCKER}\" TEXTFILE_DIR=\"${TEXTFILE_DIR}\" ${SCRIPT} ${MARKER}"
existing="$(crontab -l 2>/dev/null | grep -v "$MARKER" || true)"
printf '%s\n%s\n' "$existing" "$CRON_LINE" | grep -v '^[[:space:]]*$' | crontab -
echo "installed crontab entries tagged ${MARKER}:"
crontab -l | grep "$MARKER"
REMOTE
echo "done"
