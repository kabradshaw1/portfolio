# Debian 13 Host Hardening & Production-Readiness

**Date:** 2026-04-16
**Status:** Draft
**Approach:** Three-phase implementation with verification gates between phases.

## Context

The production Debian 13 host (`100.82.52.82`, alias `debian`) was migrated from a Windows PC per `2026-04-15-debian-server-migration-design.md`. That migration's "Phase 1: OS Hardening" section was aspirational — UFW, sudo policy, and SSH lockdown were never applied. A power outage on 2026-04-15 also revealed that Minikube does not auto-start on boot (only `minikube-tunnel.service` was installed, with no `minikube.service` underneath it), and the ethernet interface requires a manual command to come up.

This spec finishes the Phase 1 hardening from the migration plan, fixes the auto-start gaps surfaced by the outage, and establishes a sudo policy that lets the agent perform routine ops without password prompts while keeping privilege-changing actions gated.

## Goals

1. Public attack surface = zero (Cloudflare Tunnel is outbound-only; no listening ports reachable from the internet).
2. SSH is reachable only over Tailscale (key-based, `kyle` only).
3. Power-cycle the box → Minikube, the tunnel, Ollama, cloudflared, and ethernet all come back unaided.
4. Ollama (port 11434) is reachable from the Minikube cluster and from Kyle's tailnet, but not from the public internet or the LAN.
5. The agent can run routine ops (`systemctl`, `journalctl`, `apt`, `kubectl`, `docker`, `minikube`) without prompting for a password. Anything else still requires Kyle's password.
6. `lynis audit system` baseline score ≥ 75.

## Non-Goals

- Backups / disaster recovery (separate spec).
- Kubernetes / application-layer hardening (covered by `2026-04-15-security-audit-hardening-design.md`).
- Docker `userns-remap` and other deep container hardening (deferred — risk of breaking Minikube outweighs marginal gain on a single-tenant box).
- File-integrity monitoring (AIDE), `rkhunter`, SSH cert-based auth (deferred — high maintenance cost relative to threat model).
- Targeted-attacker / supply-chain threat models.

## Threat Model

**In scope:**
- Opportunistic internet scanners hitting public IPs (SSH probes, exposed-service probes).
- Compromise of one layer (e.g., Cloudflare misroute, app-level RCE) attempting to escalate to the host.
- Host coming back from power loss without manual intervention.

**Out of scope:**
- Targeted attacker with physical house access.
- Attacker who already has Kyle's SSH key *and* tailnet access (no realistic mitigation).

## Current State (2026-04-16)

- Debian 13.4, kernel 6.12.73, single user `kyle` (uid 1000, in `sudo` and `docker` groups).
- SSH: key-only, root login disabled, password auth disabled. Listens on `0.0.0.0:22`.
- `fail2ban` and `unattended-upgrades` active.
- No UFW / nftables rules in place; firewall effectively absent.
- Ollama listens on `*:11434` — exposed on every interface.
- No `NetworkManager` and no `systemd-networkd` — networking is via `ifupdown` (`/etc/network/interfaces`).
- `/etc/sudoers.d/` contains only the default `README` — no `kyle` drop-in.
- `minikube.service` does not exist; `minikube-tunnel.service` is installed but waits on a cluster nothing starts.
- Tailnet contains stale `pc-master-race` node (offline 2 days, the old Windows PC).

## Final State

- UFW enabled, default deny incoming, allow outgoing. Specific allow rules listed in Phase 2.
- SSH binds to `100.82.52.82` (Tailscale) and `127.0.0.1` only. `AllowUsers kyle`. `0.0.0.0:22` is gone.
- Ollama still binds `0.0.0.0:11434` but UFW restricts reach to loopback, docker bridge (`172.17.0.0/16`), and Tailscale (`100.64.0.0/10`). Direct binding to multiple interfaces is not Ollama-supported; the firewall is the fence.
- `minikube.service` (Type=oneshot, `ExecStart=minikube start`, runs as `kyle`) installed and enabled. `minikube-tunnel.service` updated with `Requires=minikube.service`.
- Ethernet auto-connects on boot via `ifupdown` `auto` directive (or systemd-networkd if `ifupdown` config is absent / wrong).
- `/etc/sudoers.d/kyle-ops`: passwordless for the routine-ops binary list (Section "Phase 2 → C.3"); typed-password for everything else.
- `auditd` running with baseline rule set; `journald` persistent with retention caps; sysctl hardening drop-in applied; AppArmor confirmed enforcing; `lynis` installed and baseline report saved.
- Stale `pc-master-race` node removed from Tailscale admin (Kyle handles via web console).

## Phase 1: Bootstrap & Quick Wins

### Bootstrap (one Kyle-typed sudo command)

Kyle runs (with `! ssh -t debian sudo ...`) a single command that:
- Writes `/etc/sudoers.d/kyle-temp` containing `kyle ALL=(ALL) NOPASSWD: ALL`, mode 0440.
- Validates with `visudo -c`.

This grants the agent unrestricted sudo for the duration of Phase 1 and Phase 2. Phase 2's last step (C.3) replaces it with the narrow allowlist.

### B.1 — Minikube auto-start

- Install `/etc/systemd/system/minikube.service`:
  ```ini
  [Unit]
  Description=Minikube Cluster
  After=docker.service network-online.target
  Requires=docker.service
  Wants=network-online.target

  [Service]
  Type=oneshot
  RemainAfterExit=yes
  User=kyle
  Group=kyle
  Environment=HOME=/home/kyle
  Environment=MINIKUBE_HOME=/home/kyle/.minikube
  ExecStart=/usr/local/bin/minikube start
  ExecStop=/usr/local/bin/minikube stop
  TimeoutStartSec=600
  Restart=on-failure
  RestartSec=30

  [Install]
  WantedBy=multi-user.target
  ```
- Replace `/etc/systemd/system/minikube-tunnel.service` so its `[Unit]` block has `After=minikube.service` and `Requires=minikube.service`.
- `systemctl daemon-reload && systemctl enable minikube.service minikube-tunnel.service`.

### B.2 — Bind Ollama safely (defer the actual binding to the firewall)

Ollama keeps `OLLAMA_HOST=0.0.0.0:11434` because Minikube reaches it via `host.minikube.internal` → docker bridge IP, not loopback. The firewall (B.3 + Phase 2) constrains reachability.

No Ollama config change in Phase 1; the work is the firewall draft in B.3.

### B.3 — Install UFW with rules drafted but disabled

- `apt install ufw`
- Default policies: `deny incoming`, `allow outgoing`.
- Allow rules drafted (not yet enabled):
  - `22/tcp` from `100.64.0.0/10` (Tailscale)
  - `22/tcp` from `192.168.1.0/24` (LAN — kept until Phase 2 confirms tailnet works, then removed)
  - `11434/tcp` from `172.17.0.0/16` (docker bridge → Ollama for K8s)
  - `11434/tcp` from `100.64.0.0/10` (Tailscale → Ollama for Mac access)
- `ufw` left disabled at end of Phase 1. Phase 2 enables it.

### B.4 — Ethernet auto-connect

- Inspect `/etc/network/interfaces`. If the ethernet interface lacks an `auto <iface>` line, add it.
- If the file is missing or unusable, install a minimal `systemd-networkd` `.network` file:
  ```ini
  [Match]
  Name=en*

  [Network]
  DHCP=yes
  ```
  Enable `systemd-networkd` and `systemd-resolved`.
- Verify by toggling the link down/up; confirm DHCP lease.

### B.5 — Tailnet hygiene (Kyle action)

Agent flags the offline `pc-master-race` node. Kyle removes it from the Tailscale admin console (control-plane action; not doable from the host).

### Phase 1 Verification Gate

- Reboot the box.
- Confirm SSH, ethernet, minikube, ollama, cloudflared all come up unaided (`systemctl status` for each).
- `kubectl get pods -A` — no Error states beyond the pre-existing `jaeger` ImagePull issue.
- From a Minikube pod: `curl host.minikube.internal:11434/api/tags` succeeds.
- From Kyle's Mac (over LAN): `curl <debian-lan-ip>:11434/api/tags` succeeds (UFW not on yet).

Kyle approves before Phase 2.

## Phase 2: Lockdown

This is the only phase with lockout risk. Each step is followed by a verification before the next.

### C.1 — Enable UFW

- `ufw enable` (allow rules from B.3 take effect).
- Verify:
  - From inside cluster: K8s pods still reach Ollama.
  - From Mac over Tailscale: `ssh debian` works.
  - From Mac over LAN: `ssh kyle@192.168.1.x` works (LAN rule still present).

Kyle confirms before C.2.

### C.2 — Lock SSH to Tailscale-only

Install `/etc/ssh/sshd_config.d/10-hardening.conf`:
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
```

- `systemctl reload ssh` (not `restart` — reload preserves existing sessions if config is broken, leaving Kyle a live shell to recover).
- Update UFW: delete the `192.168.1.0/24 → 22/tcp` rule (LAN SSH no longer needed since SSH no longer binds the LAN interface).

Verify:
- `ss -tlnp | grep :22` shows listener on `100.82.52.82:22` and `127.0.0.1:22` only — **not** `0.0.0.0:22`.
- Open a *new* terminal on Mac, `ssh debian` succeeds.
- From LAN: `ssh kyle@192.168.1.x` returns "connection refused" (success signal).

Kyle confirms before C.3.

### C.3 — Narrow sudo

Install `/etc/sudoers.d/kyle-ops`:
```
# Routine ops — passwordless
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

# Everything else still requires password
kyle ALL=(ALL) ALL
```

- `visudo -c` validates before installing.
- Delete `/etc/sudoers.d/kyle-temp`.

**Excluded from passwordless** (intentional — these change privilege/security state and should be Kyle-confirmed):
- `ufw enable/disable/allow/delete`
- `useradd`, `usermod`, `passwd`
- `chmod`, `chown` (outside `/home/kyle`)
- `mount`, `cryptsetup`
- Any direct edit of `/etc/...` config

### Phase 2 Verification Gate

- Agent runs `sudo systemctl status sshd` (succeeds without password).
- Agent runs `sudo ufw disable` (prompts for password — denied successfully).
- Kyle confirms ssh, kubectl, and Ollama-from-pod all still work.

## Phase 3: Auditing & Kernel Hardening

Low-risk; mostly observability and defense-in-depth. No verification gates within the phase — bundled together.

### D.1 — auditd

- `apt install auditd`
- Install `/etc/audit/rules.d/10-baseline.rules`:
  - `-w /etc/passwd -p wa -k identity`
  - `-w /etc/shadow -p wa -k identity`
  - `-w /etc/group -p wa -k identity`
  - `-w /etc/sudoers -p wa -k sudoers`
  - `-w /etc/sudoers.d/ -p wa -k sudoers`
  - `-w /etc/ssh/sshd_config -p wa -k sshd`
  - `-w /etc/ssh/sshd_config.d/ -p wa -k sshd`
  - `-w /usr/bin/sudo -p x -k sudo_exec`
  - `-w /usr/bin/su -p x -k su_exec`
  - `-w /etc/crontab -p wa -k cron`
  - `-w /etc/cron.d/ -p wa -k cron`
- `augenrules --load`.
- Configure log rotation: `max_log_file = 50`, `num_logs = 4` in `/etc/audit/auditd.conf` (caps at ~200MB).

### D.2 — journald persistence

- `mkdir -p /var/log/journal` (triggers persistence).
- `/etc/systemd/journald.conf.d/retention.conf`:
  ```ini
  [Journal]
  Storage=persistent
  SystemMaxUse=2G
  SystemKeepFree=1G
  MaxRetentionSec=30day
  ```
- `systemctl restart systemd-journald`.

### D.3 — sysctl kernel hardening

Install `/etc/sysctl.d/99-hardening.conf`:
```
kernel.kptr_restrict=2
kernel.dmesg_restrict=1
kernel.yama.ptrace_scope=1
kernel.unprivileged_bpf_disabled=1
net.ipv4.conf.all.rp_filter=1
net.ipv4.conf.default.rp_filter=1
net.ipv4.conf.all.accept_source_route=0
net.ipv4.conf.all.accept_redirects=0
net.ipv4.conf.all.send_redirects=0
net.ipv6.conf.all.accept_redirects=0
net.ipv4.tcp_syncookies=1
net.ipv4.icmp_echo_ignore_broadcasts=1
fs.protected_hardlinks=1
fs.protected_symlinks=1
```

- `sysctl --system` to apply.
- **Not set:** `net.ipv4.ip_forward` — Docker/Minikube need forwarding=1; leave as-is.

### D.4 — AppArmor verification

- Run `aa-status`.
- Confirm AppArmor is enforcing the Debian default profile set.
- If any default profiles are in `complain` mode, flip them to `enforce` with `aa-enforce`.
- No new custom profiles authored.

### D.5 — fail2ban tuning

- Confirm `sshd` jail is enabled (`fail2ban-client status sshd`).
- Verify the log path matches actual SSH logging (journald vs `/var/log/auth.log`); adjust if needed.
- Bump `bantime = 1h`, `maxretry = 3` in `/etc/fail2ban/jail.local`.
- `systemctl reload fail2ban`.

### D.6 — Lynis baseline

- `apt install lynis`.
- `lynis audit system | tee /var/log/lynis-baseline-2026-04-16.log`.
- Copy the report to `docs/security/lynis-baseline-2026-04-16.log` in the repo (commit it; future runs compare against this baseline).

### Phase 3 Verification Gate

- `lynis audit system` score ≥ 75.
- `auditctl -l` lists baseline rules.
- `journalctl --disk-usage` shows persistent storage.
- `sysctl kernel.kptr_restrict` returns `2`.
- Reboot once. All services come back unaided.

## Rollback

Each change is a discrete file `install`. Rollback per phase:

- **Phase 1:** `rm` the unit file or config drop-in, `systemctl daemon-reload`. UFW not yet enabled, no network risk.
- **Phase 2 — UFW:** `ufw disable`. Drops all rules immediately.
- **Phase 2 — SSH:** `rm /etc/ssh/sshd_config.d/10-hardening.conf && systemctl reload ssh`. Reverts to prior config. Existing sessions survive `reload`, so a broken config doesn't immediately disconnect.
- **Phase 2 — sudo:** Restore `kyle-temp` from `/root/sudoers-backup-2026-04-16/`.
- **Phase 3:** Each piece independent. `apt purge auditd lynis`, `rm /etc/sysctl.d/99-hardening.conf`, `sysctl -p /etc/sysctl.conf`.

## Lockout Recovery

If after C.2 SSH fails over both Tailscale and LAN:
1. Plug keyboard + monitor into the box.
2. Log in as `kyle` at console.
3. `sudo systemctl status sshd` — usually shows the config error.
4. `sudo rm /etc/ssh/sshd_config.d/10-hardening.conf && sudo systemctl reload ssh` — SSH back to pre-Phase-2 state.
5. `sudo ufw disable` if firewall is the cause.

Every Phase 2 change is a *new* file (never an edit of an existing config). Console recovery is always "delete one file, reload one service."

## Snapshots Taken

Before Phase 2:
```
cp -a /etc/ssh /root/etc-ssh-backup-2026-04-16/
cp -a /etc/sudoers.d /root/sudoers-backup-2026-04-16/
```

Before Phase 3:
```
cp /etc/sysctl.conf /root/sysctl.conf.bak-2026-04-16
```

## Done Criteria

- All Goals (Section "Goals") pass verification.
- `docs/security/lynis-baseline-2026-04-16.log` committed.
- A short ADR added under `docs/adr/` capturing the final state and decisions (Tailscale-only SSH, narrow sudo allowlist, Ollama-by-firewall pattern) so the rationale is recoverable later.

## Open Questions

None at spec-write time — all clarifying questions answered during brainstorming.
