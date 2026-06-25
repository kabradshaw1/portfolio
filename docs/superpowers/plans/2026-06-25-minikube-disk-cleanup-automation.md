# Minikube Disk-Cleanup Automation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Automatically reclaim dangling Docker images on the single-node minikube host before the disk fills, and alert on host disk usage before it reaches 100%.

**Architecture:** A threshold-guarded bash script runs hourly via a managed crontab entry on the Debian host. It prunes stopped containers and dangling (`<none>`) images inside the minikube node only when root-fs usage crosses a threshold, and writes observability metrics to a node-exporter textfile collector. Two Grafana alerts (host disk usage, and a deadman for the cron) are provisioned declaratively.

**Tech Stack:** Bash, cron, Docker CLI (inside minikube), Prometheus node-exporter textfile collector, Grafana provisioned alerting (ConfigMap), kustomize.

## Global Constraints

- **Never run `docker image prune -a`.** Only dangling (`<none>`) images may be removed — locally-built images may use `imagePullPolicy: Never/IfNotPresent` and cannot be re-pulled. (Verbatim from spec non-goals.)
- All `scripts/ops/*.sh` use `#!/usr/bin/env bash`, `set -euo pipefail`, and a header comment ending `# Idempotent: yes`.
- Default prune threshold: **70%** root-fs usage. Disk alert thresholds: **warning ≥ 80%**, **critical ≥ 90%**. Deadman staleness: **3h**.
- Delivery: worktree off `origin/qa`, branch `feat/minikube-disk-cleanup`, PR targets **qa** (not main). Do not push doc-only commits alone.
- Host paths: script install dir `/home/kyle/.local/lib/portfolio-diskclean/`; textfile dir `/var/lib/node_exporter/textfile_collector/`.

---

### Task 1: Cleanup script `scripts/ops/prune-minikube-docker.sh`

**Files:**
- Create: `scripts/ops/prune-minikube-docker.sh`

**Interfaces:**
- Consumes: nothing (entry point invoked by cron).
- Produces: a host executable accepting optional `--dry-run`; honors env `THRESHOLD` (int %, default 70), `DOCKER` (default `docker`), `TEXTFILE_DIR` (default `/var/lib/node_exporter/textfile_collector`), `LOG_DIR` (default `/home/kyle/.local/lib/portfolio-diskclean`). Writes textfile `${TEXTFILE_DIR}/minikube_prune.prom` exposing metrics `minikube_docker_disk_used_ratio`, `minikube_docker_prune_reclaimed_bytes`, `minikube_docker_prune_last_run_timestamp_seconds`, `minikube_docker_prune_last_success`.

- [ ] **Step 1: Write the script**

Create `scripts/ops/prune-minikube-docker.sh`:

```bash
#!/usr/bin/env bash
# Prune dangling Docker images/containers inside the minikube node when the host
# root filesystem crosses a usage threshold. Reclaims <none> layers only — never
# tagged images (locally-built images may use imagePullPolicy: Never). Writes a
# node_exporter textfile metric for observability.
# Idempotent: yes.

set -euo pipefail

THRESHOLD="${THRESHOLD:-70}"
DOCKER="${DOCKER:-docker}"
TEXTFILE_DIR="${TEXTFILE_DIR:-/var/lib/node_exporter/textfile_collector}"
TEXTFILE="${TEXTFILE_DIR}/minikube_prune.prom"
LOG_DIR="${LOG_DIR:-/home/kyle/.local/lib/portfolio-diskclean}"
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

mkdir -p "$LOG_DIR" "$TEXTFILE_DIR"

ts() { date -u +%Y-%m-%dT%H:%M:%SZ; }
log() { echo "$(ts) $*" | tee -a "$LOG"; }

disk_pct()        { df --output=pcent / | tail -1 | tr -dc '0-9'; }
disk_used_bytes() { df -B1 --output=used / | tail -1 | tr -dc '0-9'; }
disk_size_bytes() { df -B1 --output=size / | tail -1 | tr -dc '0-9'; }

write_metrics() {
  # $1 = reclaimed_bytes, $2 = success (1/0)
  local reclaimed="$1" success="$2" used size ratio tmp
  used="$(disk_used_bytes)"; size="$(disk_size_bytes)"
  ratio="$(awk -v u="$used" -v s="$size" 'BEGIN{ if (s>0) printf "%.4f", u/s; else print "0" }')"
  tmp="${TEXTFILE}.$$"
  {
    echo "# HELP minikube_docker_disk_used_ratio Root filesystem used fraction (0-1)."
    echo "# TYPE minikube_docker_disk_used_ratio gauge"
    echo "minikube_docker_disk_used_ratio ${ratio}"
    echo "# HELP minikube_docker_prune_reclaimed_bytes Bytes reclaimed on the last prune run."
    echo "# TYPE minikube_docker_prune_reclaimed_bytes gauge"
    echo "minikube_docker_prune_reclaimed_bytes ${reclaimed}"
    echo "# HELP minikube_docker_prune_last_run_timestamp_seconds Unix time of last prune run."
    echo "# TYPE minikube_docker_prune_last_run_timestamp_seconds gauge"
    echo "minikube_docker_prune_last_run_timestamp_seconds $(date -u +%s)"
    echo "# HELP minikube_docker_prune_last_success Whether the last prune run succeeded (1/0)."
    echo "# TYPE minikube_docker_prune_last_success gauge"
    echo "minikube_docker_prune_last_success ${success}"
  } > "$tmp"
  mv "$tmp" "$TEXTFILE"
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
```

- [ ] **Step 2: Make executable and lint**

```bash
chmod +x scripts/ops/prune-minikube-docker.sh
shellcheck scripts/ops/prune-minikube-docker.sh
```
Expected: exit 0, no warnings. (If `shellcheck` is unavailable locally, run `bash -n scripts/ops/prune-minikube-docker.sh` for a syntax check — expected: no output, exit 0.)

- [ ] **Step 3: Dry-run on the host below threshold**

Copy to the box and run with a high threshold so it takes the skip path (disk is ~65% now):

```bash
scp scripts/ops/prune-minikube-docker.sh debian:/tmp/prune-test.sh
ssh debian "THRESHOLD=99 LOG_DIR=/tmp TEXTFILE_DIR=/tmp bash /tmp/prune-test.sh; echo '--- metric ---'; cat /tmp/minikube_prune.prom"
```
Expected: log line `disk NN% < threshold 99% — skipping prune`; `.prom` file contains all four metric names with `minikube_docker_prune_last_success 1`.

- [ ] **Step 4: Dry-run on the host above threshold**

```bash
ssh debian "THRESHOLD=1 LOG_DIR=/tmp TEXTFILE_DIR=/tmp bash /tmp/prune-test.sh --dry-run"
```
Expected: log line `DRY-RUN: disk NN% >= 1% — would prune ...` followed by the dangling-image list (or an empty list). No actual prune performed. Clean up: `ssh debian "rm -f /tmp/prune-test.sh /tmp/minikube_prune.prom /tmp/prune.log"`.

- [ ] **Step 5: Commit**

```bash
git add scripts/ops/prune-minikube-docker.sh
git commit -m "feat(ops): threshold-guarded minikube docker dangling-image prune script"
```

---

### Task 2: Enable node-exporter textfile collector

**Files:**
- Modify: `k8s/monitoring/daemonsets/node-exporter.yml` (args list)

**Interfaces:**
- Consumes: textfile path written by Task 1's script (`/var/lib/node_exporter/textfile_collector` on the host, visible at `/host/root/...` inside the container via the existing read-only `root` mount).
- Produces: node-exporter scraping `*.prom` files from that directory, surfacing the Task 1 metrics in Prometheus.

- [ ] **Step 1: Add the textfile collector arg**

In `k8s/monitoring/daemonsets/node-exporter.yml`, add one line to the container `args` list (after the existing `--collector.filesystem.mount-points-exclude=...` line):

```yaml
            - "--collector.textfile.directory=/host/root/var/lib/node_exporter/textfile_collector"
```

(The existing `--path.rootfs=/host/root` mount makes the host dir `/var/lib/node_exporter/textfile_collector` readable at this path. No new volume is required.)

- [ ] **Step 2: Render check**

```bash
kustomize build k8s/monitoring > /dev/null && echo OK
```
Expected: `OK`, no error.

- [ ] **Step 3: Create the host dir and apply**

```bash
ssh debian "sudo install -d -m 0777 /var/lib/node_exporter/textfile_collector"
ssh debian "kubectl apply -k k8s/monitoring"
ssh debian "kubectl -n monitoring rollout status ds/node-exporter --timeout=120s"
```
Expected: daemonset rolls out; rollout status reports success. (Dir mode 0777 so the cron user can write without sudo; adjust to a dedicated owner if preferred.)

- [ ] **Step 4: Commit**

```bash
git add k8s/monitoring/daemonsets/node-exporter.yml
git commit -m "feat(monitoring): enable node-exporter textfile collector for ops metrics"
```

---

### Task 3: Cron installer `scripts/ops/install-minikube-prune-cron.sh`

**Files:**
- Create: `scripts/ops/install-minikube-prune-cron.sh`

**Interfaces:**
- Consumes: `scripts/ops/prune-minikube-docker.sh` (Task 1); the textfile dir from Task 2.
- Produces: the prune script installed at `/home/kyle/.local/lib/portfolio-diskclean/prune-minikube-docker.sh` on debian and a managed hourly crontab entry tagged `# portfolio-diskclean`.

- [ ] **Step 1: Write the installer**

Create `scripts/ops/install-minikube-prune-cron.sh`:

```bash
#!/usr/bin/env bash
# Install the minikube docker prune script on Debian plus a managed hourly crontab
# entry. Detects whether docker needs sudo. Idempotent: yes.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SOURCE="${SCRIPT_DIR}/prune-minikube-docker.sh"
REMOTE_DIR="${REMOTE_DIR:-/home/kyle/.local/lib/portfolio-diskclean}"
TEXTFILE_DIR="${TEXTFILE_DIR:-/var/lib/node_exporter/textfile_collector}"

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

mkdir -p "$TEXTFILE_DIR" 2>/dev/null || sudo install -d -m 0777 "$TEXTFILE_DIR"

CRON_LINE="0 * * * * DOCKER=\"${DOCKER}\" TEXTFILE_DIR=\"${TEXTFILE_DIR}\" ${SCRIPT} ${MARKER}"
existing="$(crontab -l 2>/dev/null | grep -v "$MARKER" || true)"
printf '%s\n%s\n' "$existing" "$CRON_LINE" | grep -v '^[[:space:]]*$' | crontab -
echo "installed crontab entries tagged ${MARKER}:"
crontab -l | grep "$MARKER"
REMOTE
echo "done"
```

- [ ] **Step 2: Lint**

```bash
chmod +x scripts/ops/install-minikube-prune-cron.sh
shellcheck scripts/ops/install-minikube-prune-cron.sh
```
Expected: exit 0. (Note: `shellcheck` does not parse the remote heredoc body as code — that's expected. Fall back to `bash -n` if shellcheck is unavailable.)

- [ ] **Step 3: Run the installer and verify idempotency**

```bash
bash scripts/ops/install-minikube-prune-cron.sh
echo "--- run again (must not duplicate) ---"
bash scripts/ops/install-minikube-prune-cron.sh
ssh debian "crontab -l | grep -c 'portfolio-diskclean'"
```
Expected: prints the docker invocation and the installed cron line both times; final count is **exactly `1`**.

- [ ] **Step 4: Exercise the real prune path once and confirm metric is scraped**

Force a real run (low threshold) using the installed copy, then confirm node-exporter surfaces the metric in Prometheus:

```bash
ssh debian "DOCKER=\"\$(docker ps >/dev/null 2>&1 && echo docker || echo 'sudo docker')\" THRESHOLD=1 /home/kyle/.local/lib/portfolio-diskclean/prune-minikube-docker.sh"
ssh debian "tail -3 /home/kyle/.local/lib/portfolio-diskclean/prune.log"
# wait one scrape interval, then query Prometheus via the grafana datasource proxy
sleep 20
curl -s 'https://grafana.kylebradshaw.dev/api/datasources/proxy/uid/PBFA97CFB590B2093/api/v1/query?query=minikube_docker_prune_last_success' | grep -o '"value":\[[^]]*\]'
```
Expected: log shows a completed prune (`prune complete: ... reclaimed N bytes`); the Prometheus query returns a result with value `1`.

- [ ] **Step 5: Commit**

```bash
git add scripts/ops/install-minikube-prune-cron.sh
git commit -m "feat(ops): idempotent installer for hourly minikube prune cron"
```

---

### Task 4: Grafana alerts (disk usage + cron deadman)

**Files:**
- Modify: `k8s/monitoring/configmaps/grafana-alerting.yml` (append rules to the `Infrastructure` group)

**Interfaces:**
- Consumes: `node_filesystem_avail_bytes` / `node_filesystem_size_bytes` (mountpoint `/`) from node-exporter; `minikube_docker_prune_last_run_timestamp_seconds` (Task 1 + Task 2).
- Produces: three provisioned alert rules routed to the existing `telegram` contact point.

- [ ] **Step 1: Confirm the disk metric and current value**

```bash
curl -s 'https://grafana.kylebradshaw.dev/api/datasources/proxy/uid/PBFA97CFB590B2093/api/v1/query?query=100%20*%20(1%20-%20node_filesystem_avail_bytes%7Bmountpoint%3D%22%2F%22%2Cfstype!~%22tmpfs%7Coverlay%22%7D%20%2F%20node_filesystem_size_bytes%7Bmountpoint%3D%22%2F%22%2Cfstype!~%22tmpfs%7Coverlay%22%7D)' | grep -o '"value":\[[^]]*\]'
```
Expected: a single result near the current real usage (~65 now). If the selector returns zero or multiple series, adjust the `mountpoint`/`fstype` matchers until exactly one series for the host root fs is returned, and use that matcher in Step 2.

- [ ] **Step 2: Append the three rules**

In `k8s/monitoring/configmaps/grafana-alerting.yml`, append to the `rules:` list under the `Infrastructure` group (match the existing A=query / B=reduce / C=threshold three-node shape; use `instant: true` and `relativeTimeRange` from 300 to 0 like the existing rules):

```yaml
          - uid: host-disk-usage-warning
            title: Host Disk Usage High
            condition: C
            data:
              - refId: A
                relativeTimeRange: { from: 300, to: 0 }
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: 100 * (1 - node_filesystem_avail_bytes{mountpoint="/",fstype!~"tmpfs|overlay"} / node_filesystem_size_bytes{mountpoint="/",fstype!~"tmpfs|overlay"})
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange: { from: 300, to: 0 }
                datasourceUid: __expr__
                model: { type: reduce, expression: A, reducer: last, refId: B }
              - refId: C
                relativeTimeRange: { from: 300, to: 0 }
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator: { type: gt, params: [80] }
                  refId: C
            for: 5m
            labels: { severity: warning }
            annotations:
              summary: "Host root filesystem usage above 80% — minikube node disk filling"

          - uid: host-disk-usage-critical
            title: Host Disk Usage Critical
            condition: C
            data:
              - refId: A
                relativeTimeRange: { from: 300, to: 0 }
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: 100 * (1 - node_filesystem_avail_bytes{mountpoint="/",fstype!~"tmpfs|overlay"} / node_filesystem_size_bytes{mountpoint="/",fstype!~"tmpfs|overlay"})
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange: { from: 300, to: 0 }
                datasourceUid: __expr__
                model: { type: reduce, expression: A, reducer: last, refId: B }
              - refId: C
                relativeTimeRange: { from: 300, to: 0 }
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator: { type: gt, params: [90] }
                  refId: C
            for: 5m
            labels: { severity: critical }
            annotations:
              summary: "Host root filesystem usage above 90% — ENOSPC imminent on minikube node"

          - uid: minikube-prune-cron-stalled
            title: Minikube Prune Cron Stalled
            condition: C
            data:
              - refId: A
                relativeTimeRange: { from: 300, to: 0 }
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: time() - max(minikube_docker_prune_last_run_timestamp_seconds)
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange: { from: 300, to: 0 }
                datasourceUid: __expr__
                model: { type: reduce, expression: A, reducer: last, refId: B }
              - refId: C
                relativeTimeRange: { from: 300, to: 0 }
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator: { type: gt, params: [10800] }
                  refId: C
            for: 10m
            labels: { severity: warning }
            annotations:
              summary: "minikube disk-prune cron has not run in over 3h — automation may be down"
```

- [ ] **Step 3: Render check**

```bash
kustomize build k8s/monitoring > /dev/null && echo OK
```
Expected: `OK`. (If Grafana rejects the compact `{ }` flow-map YAML on apply, expand those maps to block style; re-render and re-apply.)

- [ ] **Step 4: Apply and reload Grafana alerting**

```bash
ssh debian "kubectl apply -k k8s/monitoring"
bash scripts/ops/2026-05-09-reload-grafana-alerting.sh
```
Expected: ConfigMap updated; reload script completes and reports the live alert count increased by 3.

- [ ] **Step 5: Verify the rules evaluate**

```bash
curl -s 'https://grafana.kylebradshaw.dev/api/prometheus/grafana/api/v1/rules' | grep -o '"name":"\(Host Disk Usage High\|Host Disk Usage Critical\|Minikube Prune Cron Stalled\)"'
```
Expected: all three rule names present. With disk at ~65% and a fresh cron run, all three should be in `Normal`/`inactive` state (not firing).

- [ ] **Step 6: Commit**

```bash
git add k8s/monitoring/configmaps/grafana-alerting.yml
git commit -m "feat(monitoring): alert on host disk usage and stalled prune cron"
```

---

### Task 5: Open PR to qa

- [ ] **Step 1: Push the branch and open the PR**

```bash
git push -u origin feat/minikube-disk-cleanup
gh pr create --base qa --title "feat(ops): automate minikube disk cleanup + disk alerts" \
  --body "Prevents recurrence of the 2026-06-25 ENOSPC incident. Hourly threshold-guarded dangling-image prune (host cron), node-exporter textfile metrics, and Grafana alerts for host disk usage (80%/90%) plus a cron deadman. See docs/superpowers/specs/2026-06-25-minikube-disk-cleanup-automation-design.md.

🤖 Generated with [Claude Code](https://claude.com/claude-code)"
```
Expected: PR created against `qa`.

---

## Self-Review

**Spec coverage:**
- Cleanup script (threshold-guarded, dangling-only, dry-run, logging, metrics) → Task 1. ✅
- Host crontab installer (idempotent, marker, sudo detection) → Task 3. ✅
- Metrics observability mechanism → Task 1 (writes textfile) + Task 2 (node-exporter scrapes). ✅
- Host disk usage alert (80/90) → Task 4. ✅
- Deadman/cron-stalled alert → Task 4. ✅
- Validation (shellcheck, dry-run, idempotency, kustomize render, live Prometheus/alert checks) → embedded in each task. ✅
- Delivery off origin/qa, PR to qa → Task 5 + Global Constraints. ✅
- `never -a`, threshold default, paths → Global Constraints. ✅

**Placeholder scan:** No TBD/TODO; all script and YAML content is complete. The two genuinely environment-dependent items (sudo-for-docker, exact disk metric selector) are handled by explicit detection/validation steps (Task 3 Step 1 auto-detects docker; Task 4 Step 1 validates the selector), not left vague.

**Type/name consistency:** Metric names (`minikube_docker_prune_last_run_timestamp_seconds`, `minikube_docker_prune_last_success`, `minikube_docker_disk_used_ratio`, `minikube_docker_prune_reclaimed_bytes`) are identical across Task 1 (writer), Task 3 Step 4 (query), and Task 4 (deadman expr). Textfile dir path consistent across Tasks 1–3. Marker `# portfolio-diskclean` consistent in Task 3.
