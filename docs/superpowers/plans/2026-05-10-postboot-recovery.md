# Postboot Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add repo-owned Debian postboot recovery and health-check scripts so the portfolio stack self-recovers after host outages.

**Architecture:** Implement focused ops scripts for mutating recovery, read-only health verification, boot runner orchestration, and autostart installation. The installer copies the runtime scripts to Debian over SSH and installs a managed user crontab `@reboot` entry, so the Linux host does not need a repo checkout or host-level sudo. Keep recovery idempotent and limited to Minikube DNS repair plus deletion of pods already blocked on image pulls.

**Tech Stack:** Bash, SSH, systemd, kubectl, minikube, curl.

---

### Task 1: Add Shared Postboot Recovery Script

**Files:**
- Create: `scripts/ops/recover-after-host-boot.sh`

- [ ] **Step 1: Create the script**

Implement helpers for waiting on systemd services, waiting on the Kubernetes node, repairing Minikube DNS, recycling image-pull-blocked pods, and waiting for Deployments.

- [ ] **Step 2: Syntax check**

Run: `bash -n scripts/ops/recover-after-host-boot.sh`
Expected: exit 0.

### Task 2: Add Read-Only Health Check Script

**Files:**
- Create: `scripts/ops/check-postboot-health.sh`

- [ ] **Step 1: Create the script**

Implement read-only checks for node readiness, deployment availability,
image-pull failures, representative endpoints, and representative external
URLs.

- [ ] **Step 2: Syntax check**

Run: `bash -n scripts/ops/check-postboot-health.sh`
Expected: exit 0.

### Task 3: Add Autostart Installer

**Files:**
- Create: `scripts/ops/run-postboot-recovery.sh`
- Create: `scripts/ops/install-postboot-recovery-autostart.sh`

- [ ] **Step 1: Create the installer**

Copy the recovery and health scripts to
`/home/kyle/.local/lib/portfolio-postboot`, then install a managed user crontab
`@reboot` entry that runs `run-postboot-recovery.sh`.

- [ ] **Step 2: Syntax check**

Run: `bash -n scripts/ops/run-postboot-recovery.sh && bash -n scripts/ops/install-postboot-recovery-autostart.sh`
Expected: exit 0.

### Task 4: Verify and Commit

**Files:**
- Modify: `docs/superpowers/specs/2026-05-10-postboot-recovery-design.md`
- Modify: `docs/superpowers/plans/2026-05-10-postboot-recovery.md`
- Create: `scripts/ops/recover-after-host-boot.sh`
- Create: `scripts/ops/check-postboot-health.sh`
- Create: `scripts/ops/run-postboot-recovery.sh`
- Create: `scripts/ops/install-postboot-recovery-autostart.sh`

- [ ] **Step 1: Run script syntax checks**

Run:

```bash
bash -n scripts/ops/recover-after-host-boot.sh
bash -n scripts/ops/check-postboot-health.sh
bash -n scripts/ops/run-postboot-recovery.sh
bash -n scripts/ops/install-postboot-recovery-autostart.sh
```

Expected: all exit 0.

- [ ] **Step 2: Run read-only runtime health check**

Run: `bash scripts/ops/check-postboot-health.sh`
Expected: all checks pass against Debian.

- [ ] **Step 3: Commit**

Run:

```bash
git add docs/superpowers/specs/2026-05-10-postboot-recovery-design.md docs/superpowers/plans/2026-05-10-postboot-recovery.md scripts/ops/recover-after-host-boot.sh scripts/ops/check-postboot-health.sh scripts/ops/run-postboot-recovery.sh scripts/ops/install-postboot-recovery-autostart.sh
git commit -m "Add postboot recovery automation"
```
