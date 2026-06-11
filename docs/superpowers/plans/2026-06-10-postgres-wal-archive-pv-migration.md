# Postgres `wal-archive` PV Migration Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move `java-tasks/postgres-wal-archive` off the minikube-container overlay storage and onto host-backed storage so archived WAL segments survive minikube node recreation and are independently durable from `postgres-data`.

**Context — what we are fixing:**
- Incident on 2026-06-08: post-power-outage triage surfaced that `archive_command` had been silently failing since the volume was first created on 2026-04-28 (PV directory was `root:root 0755`; postgres runs as uid 70 and can't write).
- Immediate fix shipped in `fix/postgres-wal-archive-fsgroup`: added `securityContext.fsGroup: 70` to the postgres pod so kubelet chowns the volume on mount. WAL archival is currently catching up live.
- A deeper finding emerged from `/proc/mounts` inside the pod: the PVC bound to `wal-archive` is a minikube-hostpath PV provisioned at a path that lives inside the minikube container's overlayfs layer, not on the host's `/dev/nvme1n1p2`. Compare:
  - `postgres-data` → `/dev/nvme1n1p2 on /var/lib/postgresql/data type ext4` ✅
  - `wal-archive` → `overlay on /var/lib/postgresql/wal-archive type overlay (lowerdir=/var/lib/containerd/...)` ❌
- Practical risk: archives survive `kubectl rollout restart` and `docker restart minikube`, but are lost on `minikube delete`. PITR posture is weaker than `postgres-data` provides on its own.

**Out of scope for this plan:**
- Off-node WAL shipping (S3 / pgbackrest / restic). Worth a separate ADR.
- Replacing the minikube hostpath provisioner with a different StorageClass. We will keep the same provisioner and just point it at a host-backed path.
- Changing the postgres-replica WAL handling.

---

## Pre-flight verification

- [ ] **Confirm archive is currently flowing.** SSH `debian`, then:
      `kubectl -n java-tasks exec deploy/postgres -c postgres -- ls /var/lib/postgresql/wal-archive | wc -l` — expect a growing count.
      `kubectl -n java-tasks logs deploy/postgres -c postgres --tail=50 | grep -i archive` — expect no `Permission denied` lines newer than 2026-06-08 chown time.
- [ ] **Snapshot current archive count and `pg_wal` size** so we can compare after the migration.
- [ ] **Check disk headroom on `/dev/nvme1n1p2`.** Archive backlog catch-up doubles WAL usage briefly; the migration copy doubles archive storage briefly. Need ≥ (current archive size) free after the catch-up has settled.

## Implementation

### Step 1 — Decide the host-backed path

- [ ] Pick a path on the host's real filesystem for archived WAL — recommend `/var/lib/minikube-pv/postgres-wal-archive/` on the debian host (sibling to wherever `postgres-data` PV files actually live).
- [ ] Verify the chosen path is on `/dev/nvme1n1p2` (`df -h <path>`) and is owned by the kubelet/minikube container's mount-namespace UID (typically `docker:docker` on the host).

### Step 2 — Pre-create the destination directory and seed perms

- [ ] On the host: `sudo mkdir -p /var/lib/minikube-pv/postgres-wal-archive && sudo chown 70:70 /var/lib/minikube-pv/postgres-wal-archive && sudo chmod 750 /var/lib/minikube-pv/postgres-wal-archive`.
- [ ] Mount-check from inside the minikube container: `docker exec minikube ls -ld /var/lib/minikube-pv/postgres-wal-archive` — minikube mounts the host's `/var` paths, so this should appear with the same uid/gid.

### Step 3 — Author a static PV pointing at the host path

- [ ] Create `java/k8s/persistentvolumes/postgres-wal-archive-pv.yml`:
      `apiVersion: v1` `kind: PersistentVolume`
      capacity matching current PVC size (read from `kubectl get pvc -n java-tasks postgres-wal-archive -o jsonpath='{.spec.resources.requests.storage}'`)
      `accessModes: [ReadWriteOnce]`
      `persistentVolumeReclaimPolicy: Retain`
      `storageClassName: ""` (so the dynamic provisioner skips it)
      `hostPath: { path: /var/lib/minikube-pv/postgres-wal-archive, type: DirectoryOrCreate }`
      `claimRef:` pre-bound to `java-tasks/postgres-wal-archive` to claim it on creation.
- [ ] Update the existing PVC manifest (whichever file declares `postgres-wal-archive`) to set `storageClassName: ""` so it binds to the static PV, not a fresh dynamic one.

### Step 4 — Cut over (offline window required)

- [ ] Scale postgres down: `kubectl -n java-tasks scale deploy/postgres --replicas=0` and wait for pod termination (preStop runs `pg_ctl stop -m fast`, ~10–60s).
- [ ] On the host, `rsync -av` the contents of the current overlay-backed PV directory into `/var/lib/minikube-pv/postgres-wal-archive/`. Locate the current path via `kubectl get pv $(kubectl -n java-tasks get pvc postgres-wal-archive -o jsonpath='{.spec.volumeName}') -o jsonpath='{.spec.hostPath.path}'` — note this path is inside the minikube container, copy needs to be `docker exec minikube tar -C <old> -cf - . | tar -C /var/lib/minikube-pv/postgres-wal-archive -xf -` (or equivalent rsync through `docker exec`).
- [ ] Delete the old PVC and PV (`kubectl delete pvc postgres-wal-archive -n java-tasks` then `kubectl delete pv <old-pv>`). The Retain reclaim policy on the *new* PV protects the data; the *old* PV is what we're discarding.
- [ ] Apply the new PV + updated PVC manifests.
- [ ] Confirm bind: `kubectl get pvc -n java-tasks postgres-wal-archive` shows `Bound` to the new PV.
- [ ] Scale postgres back up: `kubectl -n java-tasks scale deploy/postgres --replicas=1`.

### Step 5 — Verify

- [ ] Pod becomes Ready; readiness probe (`pg_isready`) passes.
- [ ] Inside the pod, `mount | grep wal-archive` now shows a `type ext4` (or whatever the host fs is) entry, not `type overlay`.
- [ ] Inside the pod, the archive directory contains the migrated segments.
- [ ] Archive flow continues: `kubectl logs ... | grep -i archive | tail` shows no failures; new segment names appear in the directory within `archive_timeout` (300s).
- [ ] `pg_wal` size on the postgres-data PV continues to drop as the checkpointer recycles archived segments.
- [ ] Grafana alerts `Postgres Archive Command Failing` and `Postgres Backup Verification Stale` stay cleared.

### Step 6 — Documentation

- [ ] Add an ADR under `docs/adr/` recording the decision: "Use a static host-backed PV for postgres-wal-archive instead of the minikube dynamic hostpath provisioner."
- [ ] Update `docs/runbooks/postgres-recovery.md` with the new path and the cutover procedure for future cluster rebuilds.

---

## Rollback

If the cutover fails (e.g. corrupted copy, PV bind problems):

- [ ] Re-apply the old PVC manifest (no `storageClassName: ""`) so the dynamic provisioner creates a fresh PV.
- [ ] Scale postgres back to 1. Archive_command will start failing again until perms are right, but the cluster will be functionally back.
- [ ] The historical archive set will be lost, but PITR was already weak before this work, so net posture is unchanged.

## Open questions

- [ ] Does the team accept ~30–90s of postgres downtime for the cutover? If not, an alternative is to add a *second* PV at a different mountPath, switch `archive_command` to write there, and let new segments flow to the new location while the old PV stays around for replay. More moving parts, no downtime.
- [ ] Should the new PV's reclaim policy be `Retain` (safe but requires manual cleanup) or `Delete` (matches the rest of the cluster)? Recommend `Retain` — WAL archive is too easy to accidentally erase.
- [ ] Off-node shipping (S3 / pgbackrest) — should this plan defer that to a separate ADR, or include a stub `archive_command` change here?
