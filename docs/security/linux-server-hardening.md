# Linux Server Hardening — Debian 13 Home-Lab Host

A focused security assessment of the Debian 13 server (`100.82.52.82`, alias `debian`) that hosts Minikube, Ollama, the Cloudflare Tunnel, and all backend services for this project. Written as a sibling to [`security-assessment.md`](security-assessment.md), which covers the application, CI/CD, and Kubernetes layers.

**Scope:** the host operating system — install, network exposure, SSH, sudo, audit logging, kernel hardening, patch management, and operational resilience. Does **not** cover the Kubernetes workloads or the application-layer controls (those are in `security-assessment.md`).

**Methodology:** `lynis audit system` baseline plus manual inspection of `/etc/ssh/`, `/etc/sudoers.d/`, `/etc/audit/`, `/etc/sysctl.d/`, `/etc/apt/`, and `systemd` units. No dynamic testing.

**Date:** 2026-04-16

**Companion documents:**
- Engineering ADR with as-built decisions and surprises caught during the work — [`docs/adr/2026-04-16-debian-host-hardening.md`](../adr/2026-04-16-debian-host-hardening.md)
- Implementation spec — [`docs/superpowers/specs/2026-04-16-debian-host-hardening-design.md`](../superpowers/specs/2026-04-16-debian-host-hardening-design.md)
- Captured lynis report — [`lynis-baseline-2026-04-16.log`](lynis-baseline-2026-04-16.log)

## Summary of findings

| Area | Status | Notes |
|---|---|---|
| OS install & base setup | **Strong** | Hand-installed Debian 13, NVIDIA drivers, Tailscale, key-only SSH from initial setup |
| Network firewall (UFW) | **Strong** | Default-deny incoming, narrow allow rules, no public service ports — `nmap` from the public internet returns empty |
| SSH hardening | **Strong** | Tailscale-only listener, lynis-recommended additions, race-condition-resilient via systemd drop-in |
| Privilege management | **Strong** | Narrow passwordless sudo allowlist for routine ops; privilege-changing actions still gated |
| Audit logging | **Adequate** | `auditd` with immutable baseline rules; persistent `journald` with retention caps. Local-only (no remote forwarding) |
| Kernel hardening | **Strong** | `sysctl` drop-in covering `kptr_restrict`, `ptrace_scope`, `unprivileged_bpf_disabled`, network-stack hygiene |
| Patch management | **Strong** | Found and fixed missing `security.debian.org` repository; 15 stale patches applied; `unattended-upgrades` now actually pulling security updates |
| Service auto-start | **Strong** | Minikube + tunnel + Ollama + cloudflared + ethernet all auto-start; verified across two reboots |
| Intrusion prevention | **Adequate** | `fail2ban` `[sshd]` jail tuned to `bantime=1h`, `maxretry=3` |
| Lynis baseline | **Strong** | Hardening index 77 (target ≥75); deferred items documented in §11 |

The overall posture is strong for a single-tenant home-lab box. The remaining gaps (file-integrity monitoring, remote audit-log forwarding) are documented as accepted risks.

---

## 1. OS install & initial setup

**Status:** Strong.

The host is a fresh hand-installed Debian 13 (trixie) on the home-lab machine, kernel 6.12.74. The migration replaced a Windows PC running the same workloads — see [`docs/superpowers/specs/2026-04-15-debian-server-migration-design.md`](../superpowers/specs/2026-04-15-debian-server-migration-design.md) for the migration record.

Established from initial install:
- **Single privileged user** `kyle` (uid 1000) in the `sudo` and `docker` groups.
- **SSH key-based authentication only.** `PasswordAuthentication no` and `PermitRootLogin no` set in the base `/etc/ssh/sshd_config` from day one.
- **Tailscale** installed for out-of-band reachability — gives every machine in Kyle's tailnet a stable IP (`100.82.52.82` for this host).
- **NVIDIA drivers + CUDA toolkit** installed for Ollama GPU inference on the RTX 3090.
- **Docker engine** installed via the official `docker.com` apt repository.
- **Minikube** installed at `/usr/local/bin/minikube`, runs the Kubernetes control plane in a Docker container.

---

## 2. Network firewall (UFW)

**Status:** Strong.

Default policies: `deny (incoming)`, `allow (outgoing)`. Allow rules:

| Source | Port | Purpose |
|---|---|---|
| `100.64.0.0/10` (Tailscale CGNAT range) | `22/tcp` | SSH from any tailnet member |
| `127.0.0.1` | `80/tcp` | Cloudflare Tunnel inbound (loopback only — the tunnel daemon is the only listener) |
| `127.0.0.1` | `11434/tcp` | Ollama localhost |
| `172.16.0.0/12` | `11434/tcp` | Ollama from Docker bridge networks |
| `172.17.0.0/16` | `11434/tcp` | Ollama from default Docker bridge specifically |
| `192.168.49.0/24` | `11434/tcp` | Ollama from Minikube bridge |
| `100.64.0.0/10` | `11434/tcp` | Ollama from any tailnet member |
| Anywhere | `41641/udp` | Tailscale WireGuard control plane |

**Note on Ollama exposure:** Ollama itself binds `0.0.0.0:11434` (required because Minikube reaches it via `host.minikube.internal` which resolves to the Docker bridge IP, not loopback). The firewall is the fence — it constrains who can reach the open port.

**Operational discovery, 2026-04-16:** A previously-existing rule `192.168.0.0/16 → 11434/tcp` silently exposed Ollama to the home LAN (since `192.168.1.0/24` ⊂ `192.168.0.0/16`). The rule was removed during the hardening pass; the per-bridge rules above cover all legitimate paths.

**Verification:** `nmap` from outside the tailnet against the host's public IP returns no open ports — Cloudflare Tunnel is outbound-only and exposes nothing inbound.

---

## 3. SSH hardening

**Status:** Strong.

`/etc/ssh/sshd_config.d/10-hardening.conf`:

```
ListenAddress 100.82.52.82
ListenAddress 127.0.0.1
AllowUsers kyle
PermitRootLogin no
PasswordAuthentication no
KbdInteractiveAuthentication no
PermitEmptyPasswords no
MaxAuthTries 3
LoginGraceTime 20
ClientAliveInterval 300
ClientAliveCountMax 2

# Lynis-recommended additions (safe because SSH is Tailscale-only)
AllowTcpForwarding no
AllowAgentForwarding no
X11Forwarding no
LogLevel VERBOSE
MaxSessions 2
TCPKeepAlive no
```

`sshd` listens only on the Tailscale IP and loopback — never `0.0.0.0:22`. The base `/etc/ssh/sshd_config` is unchanged; the drop-in is a pure overlay so console rollback is "delete one file, reload one service."

**Race-condition fix:** `/etc/systemd/system/ssh.service.d/wait-for-tailscale.conf`

```ini
[Unit]
After=tailscaled.service network-online.target
Wants=tailscaled.service network-online.target

[Service]
Restart=on-failure
RestartSec=10
```

Without this, the first reboot after enabling the hardening drop-in caused `sshd` to race ahead of `tailscaled`, fail to bind `100.82.52.82:22`, and leave the host SSH-unreachable until manual `systemctl restart ssh` from console. The drop-in fixes the ordering and adds a 10-second auto-retry safety net for the brief window where `tailscaled` is "active" but the IP isn't yet on the interface.

---

## 4. Privilege management (sudo)

**Status:** Strong.

`/etc/sudoers.d/kyle-ops`:

```
kyle ALL=(root) NOPASSWD: /usr/bin/systemctl, \
                          /usr/bin/journalctl, \
                          /usr/bin/apt, \
                          /usr/bin/apt-get, \
                          /usr/bin/dpkg, \
                          /usr/local/bin/kubectl, \
                          /usr/local/bin/minikube, \
                          /usr/bin/docker, \
                          /usr/sbin/ufw status, \
                          /usr/sbin/ufw status verbose, \
                          /usr/bin/lynis audit system
```

Routine operations (service management, log inspection, package updates, container/cluster management, firewall status checks, security audits) are passwordless. Privilege-changing operations (`useradd`, `ufw enable/disable`, edits to `/etc/...`, `chmod` on system paths, `mount`) still require Kyle's password via the default `%sudo` group rule in `/etc/sudoers`.

**Lesson recorded:** an earlier draft included a trailing explicit `kyle ALL=(ALL) ALL` line in `kyle-ops` to make "everything else needs password" obvious. Sudo's last-match-wins evaluation meant this line *overrode* the NOPASSWD entries above it, making nothing passwordless. Removing the explicit catch-all and relying on the implicit `%sudo` group membership fixed it. Captured in the ADR as a reusable lesson.

---

## 5. Audit logging

**Status:** Adequate.

`/etc/audit/rules.d/10-baseline.rules`:

```
# Identity changes
-w /etc/passwd -p wa -k identity
-w /etc/shadow -p wa -k identity
-w /etc/group -p wa -k identity
-w /etc/gshadow -p wa -k identity

# Sudo + sshd config
-w /etc/sudoers -p wa -k sudoers
-w /etc/sudoers.d/ -p wa -k sudoers
-w /etc/ssh/sshd_config -p wa -k sshd
-w /etc/ssh/sshd_config.d/ -p wa -k sshd

# Privilege escalation invocations
-w /usr/bin/sudo -p x -k sudo_exec
-w /usr/bin/su -p x -k su_exec

# Cron + scheduled-task tampering
-w /etc/crontab -p wa -k cron
-w /etc/cron.d/ -p wa -k cron
-w /etc/cron.daily/ -p wa -k cron
-w /etc/cron.hourly/ -p wa -k cron

-e 2
```

The trailing `-e 2` makes the rule set immutable until the next reboot — an attacker who gains root cannot remove or alter the rules without a power cycle. Log rotation: `max_log_file = 50` MB, `num_logs = 4` — caps disk use at ~200 MB.

`journald` made persistent at `/var/log/journal/`, retention capped at 2 GB / 30 days via `/etc/systemd/journald.conf.d/retention.conf`. Logs survive reboots, which they didn't before.

**Accepted limitation:** logs are local-only. A remote forwarder (Loki, journald-remote, syslog → SIEM) would harden the chain against a host compromise that wipes audit logs. Listed in §11.

---

## 6. Kernel hardening

**Status:** Strong.

`/etc/sysctl.d/99-hardening.conf`:

```
# Kernel
kernel.kptr_restrict=2
kernel.dmesg_restrict=1
kernel.yama.ptrace_scope=1
kernel.unprivileged_bpf_disabled=1

# Network
net.ipv4.conf.all.rp_filter=1
net.ipv4.conf.default.rp_filter=1
net.ipv4.conf.all.accept_source_route=0
net.ipv4.conf.all.accept_redirects=0
net.ipv4.conf.all.send_redirects=0
net.ipv6.conf.all.accept_redirects=0
net.ipv4.tcp_syncookies=1
net.ipv4.icmp_echo_ignore_broadcasts=1

# Filesystem
fs.protected_hardlinks=1
fs.protected_symlinks=1
```

Standard server-hardening profile: hide kernel pointers from `/proc`, restrict `dmesg` and `ptrace` to root and parent processes, block unprivileged eBPF, tighten the network stack against spoofing/redirects, enforce SYN cookies, protect against hardlink/symlink TOCTOU attacks.

**Deliberate non-change:** `net.ipv4.ip_forward` stays at `1` — Docker and Minikube both require it.

---

## 7. Patch management

**Status:** Strong.

The most consequential operational finding of the 2026-04-16 hardening pass: `unattended-upgrades` was active and configured to allow security-origin packages, but `/etc/apt/sources.list` was missing the `security.debian.org` repository entry. The migration to Debian 13 had silently broken automatic security updates.

Fix: appended to `/etc/apt/sources.list`:

```
deb http://security.debian.org/debian-security trixie-security main contrib non-free non-free-firmware
```

The first `apt-get update` after this fetched 15 stale patches: `openssh-server`, `openssh-client`, `openssl`, `libssl3t64`, `libpng16-16t64`, `libtiff6`, `libfreetype6`, `libgdk-pixbuf-2.0-0` (and related), `linux-image-amd64` (kernel `6.12.73 → 6.12.74`), `linux-libc-dev`. All applied via `apt-get -o Dpkg::Options::="--force-confold" dist-upgrade -y`; the kernel update landed via reboot.

`unattended-upgrades` is now both enabled **and** has a security source to pull from. Verified by reading `/var/log/unattended-upgrades/unattended-upgrades.log` — the daemon now logs allowed origins including `Debian-Security`.

---

## 8. Service auto-start (operational resilience)

**Status:** Strong.

A 2026-04-15 power outage revealed that nothing on the box auto-restarted cleanly: Minikube did not come back, ethernet stayed down until manually `ifup`'d. Both fixed during the 2026-04-16 hardening:

- **`/etc/systemd/system/minikube.service`** (new) — `Type=oneshot`, `RemainAfterExit=yes`, runs `/usr/local/bin/minikube start` as `kyle` after `docker.service`. `Restart=on-failure`, `TimeoutStartSec=600`.
- **`/etc/systemd/system/minikube-tunnel.service`** (modified) — now `Requires=minikube.service`, so the LoadBalancer tunnel can't start before the cluster is up.
- **`/etc/network/interfaces`** — added `auto enp4s0 / iface enp4s0 inet dhcp`. The Debian 13 default install had `lo` only.

The stock `cloudflared.service` and `ollama.service` are enabled and confirmed to come back on boot. `fail2ban`, `auditd`, `ssh` (with the wait-for-tailscale drop-in from §3), and the new `minikube.service` round out the always-on set.

**Verified across two reboots** during the 2026-04-16 hardening: from cold boot, every service comes back without intervention, ethernet auto-attaches, the cluster reports `Ready` within ~60 seconds.

---

## 9. Intrusion prevention

**Status:** Adequate.

`fail2ban` `[sshd]` jail tuned in `/etc/fail2ban/jail.local`:

```
[DEFAULT]
bantime = 1h
maxretry = 3

[sshd]
enabled = true
bantime = 1h
maxretry = 3
```

Three failed SSH attempts within `findtime` (default 10m) → 1-hour ban. Effective practical limit, even though SSH is Tailscale-only and brute-force is essentially infeasible from outside the tailnet.

---

## 10. Lynis baseline

**Status:** Strong.

`lynis audit system` final hardening index: **77** (target was ≥75). Pre-remediation score: 70.

The 7-point gap was closed primarily by:
- Adding the `security.debian.org` repository (single largest contributor)
- The lynis-recommended SSH additions in §3 (`AllowTcpForwarding no`, `LogLevel VERBOSE`, etc.)
- `login.defs` tightening: `PASS_MAX_DAYS=90`, `PASS_MIN_DAYS=1`, `UMASK=027`, `SHA_CRYPT_MIN_ROUNDS=10000`, `SHA_CRYPT_MAX_ROUNDS=10000`

Full report captured at [`lynis-baseline-2026-04-16.log`](lynis-baseline-2026-04-16.log) (32 KB) for diffing against future runs.

---

## 11. Recovery posture

**Status:** Strong.

Every Phase-2 lockdown change (SSH binding, sudo narrowing, UFW rule changes) was implemented as a **new file**, never an edit of an existing config. This means console recovery is always "delete one file, reload one service" — not "remember what the original `/etc/ssh/sshd_config` said and undo your edit."

Snapshots taken before risky changes:
- `/root/etc-ssh-backup-2026-04-16/` — full `/etc/ssh` directory
- `/root/sudoers-backup-2026-04-16/` — full `/etc/sudoers.d` directory (including the bootstrap `kyle-temp` file)
- `/root/sysctl.conf.bak-2026-04-16` — `/etc/sysctl.conf` (turned out to be absent on Debian 13 default; snapshot is a placeholder)

Console fallback (keyboard + monitor at the box) was used once during the work — the wait-for-tailscale race took out SSH on the first reboot of the new config, and recovery via console (`systemctl restart ssh`) took under a minute.

---

## Recommended next steps

Ordered by impact-to-effort ratio:

1. **Remote audit log forwarding.** `auditd` and `journald` are local-only. A remote sink (Loki via `journald-remote`, or a SIEM) means logs survive a host compromise. Medium effort, high value for incident response.
2. **AIDE (Advanced Intrusion Detection Environment).** File-integrity monitoring with a daily scheduled scan against a baseline. Catches root-kit-style file tampering. Deferred during the 2026-04-16 work because tuning false positives is a real ongoing cost — worth doing if the box ever leaves single-tenant home use.
3. **GRUB password.** Prevents an attacker with physical access from boot-time recovery shell. Marginal value — physical access already implies game-over, but lynis flags it.
4. **SSH certificate-based authentication.** Replaces raw key trust with a CA-signed model. Worth it if more than one trusted client device exists.
5. **`/home` and `/var` on separate partitions.** Lynis suggestion. Requires a partition rebuild — defer to next OS reinstall, not worth a forced rebuild.
6. **`kernel` modules signing & `lockdown` mode.** Higher-effort hardening for a host that's not running custom kernel modules anyway. Defer.

---

## Evidence index

**Host paths:**
- `/etc/ssh/sshd_config.d/10-hardening.conf` — SSH hardening drop-in
- `/etc/systemd/system/ssh.service.d/wait-for-tailscale.conf` — sshd-vs-tailscaled race fix
- `/etc/sudoers.d/kyle-ops` — narrow sudo allowlist
- `/etc/audit/rules.d/10-baseline.rules` — auditd baseline
- `/etc/audit/auditd.conf` — log rotation (max_log_file=50, num_logs=4)
- `/etc/systemd/journald.conf.d/retention.conf` — journald persistence + retention
- `/etc/sysctl.d/99-hardening.conf` — kernel sysctl hardening
- `/etc/fail2ban/jail.local` — fail2ban tuning
- `/etc/apt/sources.list` — security repository addition
- `/etc/login.defs` — password aging + umask + SHA crypt rounds
- `/etc/systemd/system/minikube.service` — Minikube auto-start
- `/etc/systemd/system/minikube-tunnel.service` — tunnel auto-start (depends on minikube)
- `/etc/network/interfaces` — ethernet auto-connect
- `/var/log/lynis.log` — full lynis output

**Repo paths:**
- [`docs/superpowers/specs/2026-04-16-debian-host-hardening-design.md`](../superpowers/specs/2026-04-16-debian-host-hardening-design.md) — design spec
- [`docs/superpowers/plans/2026-04-16-debian-host-hardening.md`](../superpowers/plans/2026-04-16-debian-host-hardening.md) — implementation plan
- [`docs/adr/2026-04-16-debian-host-hardening.md`](../adr/2026-04-16-debian-host-hardening.md) — engineering ADR with as-built decisions and surprises
- [`docs/superpowers/specs/2026-04-15-debian-server-migration-design.md`](../superpowers/specs/2026-04-15-debian-server-migration-design.md) — predecessor migration spec
- [`docs/security/lynis-baseline-2026-04-16.log`](lynis-baseline-2026-04-16.log) — captured lynis baseline report
- [`docs/security/security-assessment.md`](security-assessment.md) — application/CI/K8s security assessment (sibling)
