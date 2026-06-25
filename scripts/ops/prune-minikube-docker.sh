#!/usr/bin/env bash
# Prune dangling Docker images/containers inside the minikube node when the host
# root filesystem crosses a usage threshold. Reclaims <none> layers only — never
# tagged images (locally-built images may use imagePullPolicy: Never). Writes a
# node_exporter textfile metric for observability.
# Idempotent: yes.

set -euo pipefail

THRESHOLD="${THRESHOLD:-70}"
DOCKER="${DOCKER:-docker}"
LOG_DIR="${LOG_DIR:-/home/kyle/.local/lib/portfolio-diskclean}"
# The textfile metrics are read by node-exporter, which runs INSIDE the minikube
# node — a different filesystem from this host. So the .prom file is written into
# the minikube container (via docker exec), not onto the host. TEXTFILE_DIR is a
# path inside the minikube node; node-exporter reads it via /host/root/<path>.
TEXTFILE_DIR="${TEXTFILE_DIR:-/var/tmp/portfolio-diskclean}"
TEXTFILE="${TEXTFILE_DIR}/minikube_prune.prom"
LOG="${LOG_DIR}/prune.log"
DRY_RUN=0

for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=1 ;;
    *) echo "unknown arg: $arg" >&2; exit 2 ;;
  esac
done

if ! [[ "$THRESHOLD" =~ ^[0-9]+$ ]]; then
  echo "THRESHOLD must be an integer percentage, got: $THRESHOLD" >&2
  exit 2
fi

mkdir -p "$LOG_DIR"

ts() { date -u +%Y-%m-%dT%H:%M:%SZ; }
log() { echo "$(ts) $*" | tee -a "$LOG"; }

disk_pct()        { df --output=pcent / | tail -1 | tr -dc '0-9'; }
disk_used_bytes() { df -B1 --output=used / | tail -1 | tr -dc '0-9'; }
disk_size_bytes() { df -B1 --output=size / | tail -1 | tr -dc '0-9'; }

write_metrics() {
  # $1 = reclaimed_bytes, $2 = success (1/0)
  local reclaimed="$1" success="$2" used size ratio now
  used="$(disk_used_bytes)"; size="$(disk_size_bytes)"
  ratio="$(awk -v u="$used" -v s="$size" 'BEGIN{ if (s>0) printf "%.4f", u/s; else print "0" }')"
  now="$(date -u +%s)"
  # Write into the minikube node (where node-exporter reads textfiles), atomically,
  # world-readable so node-exporter (uid nobody) can read it.
  $DOCKER exec minikube mkdir -p "$TEXTFILE_DIR" >/dev/null 2>&1 || true
  $DOCKER exec -i minikube sh -c "cat > '${TEXTFILE}.tmp' && chmod 0644 '${TEXTFILE}.tmp' && mv '${TEXTFILE}.tmp' '${TEXTFILE}'" <<EOF || log "WARN: failed to write metrics into minikube node"
# HELP minikube_docker_disk_used_ratio Root filesystem used fraction (0-1).
# TYPE minikube_docker_disk_used_ratio gauge
minikube_docker_disk_used_ratio ${ratio}
# HELP minikube_docker_prune_reclaimed_bytes Bytes reclaimed on the last prune run.
# TYPE minikube_docker_prune_reclaimed_bytes gauge
minikube_docker_prune_reclaimed_bytes ${reclaimed}
# HELP minikube_docker_prune_last_run_timestamp_seconds Unix time of last prune run.
# TYPE minikube_docker_prune_last_run_timestamp_seconds gauge
minikube_docker_prune_last_run_timestamp_seconds ${now}
# HELP minikube_docker_prune_last_success Whether the last prune run succeeded (1/0).
# TYPE minikube_docker_prune_last_success gauge
minikube_docker_prune_last_success ${success}
EOF
}

before="$(disk_pct)"
before_bytes="$(disk_used_bytes)"

if (( before < THRESHOLD )); then
  log "disk ${before}% < threshold ${THRESHOLD}% — skipping prune"
  write_metrics 0 1
  exit 0
fi

if (( DRY_RUN )); then
  log "DRY-RUN: disk ${before}% >= ${THRESHOLD}% — would prune stopped containers + dangling images:"
  $DOCKER exec minikube docker image ls -f dangling=true 2>&1 | tee -a "$LOG" || true
  exit 0
fi

log "disk ${before}% >= ${THRESHOLD}% — pruning"
if ! $DOCKER exec minikube docker container prune -f >>"$LOG" 2>&1; then
  log "ERROR: container prune failed"; write_metrics 0 0; exit 1
fi
if ! $DOCKER exec minikube docker image prune -f >>"$LOG" 2>&1; then
  log "ERROR: image prune failed"; write_metrics 0 0; exit 1
fi

after="$(disk_pct)"
reclaimed=$(( before_bytes - $(disk_used_bytes) ))
(( reclaimed < 0 )) && reclaimed=0
log "prune complete: ${before}% -> ${after}%, reclaimed ${reclaimed} bytes"
write_metrics "$reclaimed" 1
