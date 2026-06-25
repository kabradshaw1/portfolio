# Minikube Disk-Cleanup Automation + Disk Alert — Design

**Date:** 2026-06-25
**Status:** Approved (design)
**Author:** Kyle Bradshaw (with Claude)

## Background

On 2026-06-25 a Grafana alert storm fired: *Java Gateway Error Rate High* (35%),
*Pod Restart Storm* (kafka-0 in go-ecommerce + go-ecommerce-qa, mongodb in
java-tasks), and *Deployment Replicas Unavailable* (activity-service, mongodb).

Investigation traced every symptom to a single root cause: the Debian host's root
filesystem (`/dev/nvme1n1p2`, 444 GB) was **100% full (13 MB free)**. Two unrelated
services in two namespaces hit `No space left on device` simultaneously:

- **kafka-0** — JVM could not create `/opt/kafka/.../logs` → exit 1, crash loop.
- **mongodb** — WiredTiger `pwrite /data/db/_mdb_catalog.wt` → ENOSPC (errno 28) →
  fatal assertion, exit 14. This cascaded: activity-service / task-service /
  notification-service / gateway-service could not reach their DB, driving the 35%
  gateway error rate.

The disk was consumed by Docker image bloat inside the single-node minikube cluster:
`/var/lib/docker/overlay2` held **170 GB** across **792 images** accumulated from
repeated QA + prod deploys, never garbage-collected.

Manual remediation: `docker exec minikube docker container prune -f` +
`docker image prune -f` (dangling only) reclaimed **154 GB**, dropping the disk to
65%. All pods recovered after clearing their CrashLoopBackOff backoff timers.

### Why this was not caught automatically

- **No disk-usage alert existed.** Nothing paged until services were already
  crash-looping at 100% full.
- **Kubelet image GC did not trigger.** minikube-on-Docker reports a stale
  filesystem view; `DiskPressure` stayed `False` even at 100% full, so kubelet kept
  scheduling pods (which then crash-looped) instead of evicting/GCing images. The
  kubelet's own disk-based image GC therefore cannot be relied on in this
  environment.

## Goals

1. Prevent recurrence by automatically reclaiming dangling Docker images before the
   disk fills.
2. Detect disk pressure *before* 100% via a Grafana alert (the missing signal).
3. Make the automation observable so a silently-dead cron is itself alertable.

## Non-goals

- Eliminating the upstream cause of image churn (`:latest` tags, per-deploy builds
  into minikube's Docker daemon). Tracked as a separate follow-up.
- Fixing kubelet image GC in minikube-on-Docker (unreliable here by design).
- Pruning tagged-but-unreferenced images. Locally-built images may use
  `imagePullPolicy: Never/IfNotPresent` and cannot be re-pulled; removing them would
  brick those pods. **Only dangling (`<none>`) images are ever removed.**

## Design

### Component 1 — Cleanup script (`scripts/ops/prune-minikube-docker.sh`)

Runs **on the Debian host**, invoked locally by cron (no `ssh`). Behavior:

1. Read root filesystem usage percentage for `/`.
2. If usage `< THRESHOLD` (default **70%**): log "below threshold, skipping" and
   exit 0. Most runs are a near-noop.
3. If usage `>= THRESHOLD`:
   - `docker exec minikube docker container prune -f`
   - `docker exec minikube docker image prune -f`  *(dangling only — never `-a`)*
4. Log a timestamped line (before %, after %, bytes reclaimed) to
   `/home/kyle/.local/lib/portfolio-diskclean/prune.log`.
5. Push metrics to the existing **pushgateway** (`monitoring` namespace):
   - `minikube_docker_disk_used_ratio` (gauge, 0–1)
   - `minikube_docker_prune_reclaimed_bytes` (gauge, last run)
   - `minikube_docker_prune_last_success_timestamp_seconds` (gauge)

**Flags / config:**
- `--dry-run` — logs what would be pruned, omits `-f`, pushes no success timestamp.
- `THRESHOLD` env var (default `70`).

**Conventions:** `#!/usr/bin/env bash`, `set -euo pipefail`, header comment with
`# Idempotent: yes`, matching `scripts/ops/` style.

**Open implementation detail (verify on box):** whether cron-user `kyle` can run
`docker` without `sudo` (docker-group membership). The manual fix used `sudo docker`.
If sudo is required, the cron entry / script invokes `sudo docker ...` and relies on
passwordless sudo scoped to that command. Confirm and adapt during implementation.

### Component 2 — Cron installer (`scripts/ops/install-minikube-prune-cron.sh`)

Idempotent installer mirroring `install-postboot-recovery-autostart.sh`:

- `scp` the cleanup script to `/home/kyle/.local/lib/portfolio-diskclean/`.
- Install a **managed crontab entry** tagged with a marker comment
  (`# portfolio-diskclean`), schedule **hourly** (`0 * * * *`). Hourly is cheap
  because the script is threshold-guarded — it only prunes when disk crosses 70%.
- Re-running the installer must not duplicate the crontab entry.

### Component 3 — Grafana alerts (`k8s/monitoring/configmaps/grafana-alerting.yml`)

Two new rules in the existing **Infrastructure** group → existing Telegram contact
point:

1. **Host Disk Usage High** — node-exporter query for `/` usage:
   - `warning` at `>= 80%`, `for: 5m`
   - `critical` at `>= 90%`, `for: 5m`
   - The exact series/labels (e.g. `node_filesystem_avail_bytes` /
     `node_filesystem_size_bytes`, correct `mountpoint`/`fstype` selector) must be
     **validated against live Prometheus** before finalizing, since the daemonset
     node-exporter reports host rootfs under a specific mountpoint and `DiskPressure`
     is not trustworthy here.

2. **Minikube Prune Cron Stalled** (deadman) — fires when
   `time() - minikube_docker_prune_last_success_timestamp_seconds > ~3h`, i.e. the
   cleanup automation stopped running. `severity: warning`.

## Error handling

- `set -euo pipefail`; a docker failure exits non-zero but only *after* logging, so
  failures are visible and the deadman alert eventually fires (no success timestamp
  pushed).
- Robust `df` parsing (e.g. `df --output=pcent /` stripped to an integer).
- Threshold read from env with a safe default; reject non-numeric input.

## Testing & validation

- `shellcheck` both scripts (lint).
- Run `prune-minikube-docker.sh --dry-run` on debian; confirm log line written and
  metrics pushed to pushgateway (visible in Prometheus).
- Run the installer twice; confirm exactly one managed crontab entry exists.
- `kustomize build k8s/monitoring` renders without error.
- Query both alert expressions against live Prometheus; confirm sane current values
  (disk-usage rule reflects the real ~65% now; deadman reflects a recent timestamp
  after a manual run).

## Delivery

- Work in a worktree off `origin/qa`; branch `feat/minikube-disk-cleanup`.
- Open a **PR to qa** (not main). Do not push doc-only commits alone — push when the
  scripts/alerts land so CI has code to run against.

## Follow-up (separate work)

- Reduce upstream image churn: pin deploy tags instead of `:latest`, and/or prune at
  build/CI time so minikube's Docker daemon does not accumulate orphaned layers.
- The stuck `story-chat` rollout (replicaset `5c965cbf4f`, image
  `ghcr.io/kabradshaw1/story/chat:latest`, created 15:54 on 2026-06-25) is an
  unrelated bad deploy surfaced during this investigation — not addressed here.
