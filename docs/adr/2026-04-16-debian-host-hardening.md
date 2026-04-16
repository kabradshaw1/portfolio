# ADR: Debian 13 Host Hardening (2026-04-16)

## Status
Accepted

## Context
The production Debian 13 host (`debian` / `100.82.52.82`) was migrated from a Windows PC per `2026-04-15-debian-server-migration-design.md`. That migration's Phase 1 OS hardening section was partially executed (UFW was installed and enabled with rules, fail2ban + unattended-upgrades both running) but a 2026-04-15 power outage revealed gaps: Minikube did not auto-restart, ethernet did not auto-connect, and the Debian security repository was missing from `/etc/apt/sources.list` (so `unattended-upgrades` had been silently doing nothing for security patches since the migration).

This ADR records the as-built hardened state and the key decisions made during the 2026-04-16 hardening pass. Spec: `docs/superpowers/specs/2026-04-16-debian-host-hardening-design.md`. Implementation plan: `docs/superpowers/plans/2026-04-16-debian-host-hardening.md`.

## Decisions

### SSH binds only to Tailscale + loopback
`/etc/ssh/sshd_config.d/10-hardening.conf` sets `ListenAddress 100.82.52.82` and `ListenAddress 127.0.0.1`. Public SSH is gone. Includes `AllowUsers kyle`, `MaxSessions 2`, `MaxAuthTries 3`, `LoginGraceTime 20`, plus the lynis-recommended additions: `AllowTcpForwarding no`, `AllowAgentForwarding no`, `X11Forwarding no`, `LogLevel VERBOSE`, `TCPKeepAlive no`. Recovery if Tailscale is down: physical console.

**Race fix (`/etc/systemd/system/ssh.service.d/wait-for-tailscale.conf`):** Because `ListenAddress 100.82.52.82` requires the Tailscale IP to exist before sshd starts, the original install caused sshd to fail to bind on the first reboot — the box came up but SSH was unreachable until manual `systemctl restart ssh` from console. The drop-in adds `After=tailscaled.service` + `Wants=tailscaled.service` and `Restart=on-failure RestartSec=10` so sshd waits for tailscaled to start and self-recovers if it races within tailscaled's startup window. Future boots verified clean.

### Ollama is fenced by firewall, not by bind address
Ollama keeps `OLLAMA_HOST=0.0.0.0:11434` because Minikube reaches it via `host.minikube.internal` which resolves to the docker bridge IP (`172.17.0.1`), not loopback. UFW restricts reach to:
- `127.0.0.1` (loopback)
- `172.16.0.0/12` and `172.17.0.0/16` (docker bridges)
- `192.168.49.0/24` (minikube subnet specifically)
- `100.64.0.0/10` (Tailscale CGNAT range — Mac access)

A previously-existing rule allowing `192.168.0.0/16 → 11434` (which silently exposed Ollama to the home LAN, since `192.168.1.0/24` ⊂ `192.168.0.0/16`) was removed during this pass.

### UFW: default-deny, narrow allowlist
`Status: active`, `default deny (incoming) / allow (outgoing)`. Allow rules cover SSH from Tailscale CIDR, Ollama from the fenced ranges above, port 80 from loopback (cloudflared), and Tailscale UDP 41641. Two redundant per-host SSH allows for individual tailnet IPs were removed in favor of the CIDR rule.

### Sudo: narrow allowlist, not NOPASSWD ALL
`/etc/sudoers.d/kyle-ops` grants passwordless sudo for `systemctl`, `journalctl`, `apt`, `apt-get`, `dpkg`, `kubectl`, `minikube`, `docker`, `ufw status` (read-only), and `lynis audit system`. Privilege-changing actions (`useradd`, `ufw enable/disable`, sshd config edits, `chmod`/`chown` on system paths) still require Kyle's password (via the default `%sudo` group rule in `/etc/sudoers`).

**Sudoers gotcha (recorded for future):** The first draft included an explicit trailing `kyle ALL=(ALL) ALL` line in `kyle-ops` intending to make "everything else needs password" explicit. Sudo's last-match-wins rule meant that line *overrode* the NOPASSWD entries above it, making nothing passwordless. The fix: remove the catch-all line and rely on the implicit password-required access from `%sudo` group membership. Lesson: only put explicit `NOPASSWD:` overrides in drop-in files, never a catch-all that could re-shadow them.

### Sequencing: narrow sudo *after* Phase 3, not after Phase 2
The original spec had Task 10 (narrow sudo) as the last step of Phase 2, before Phase 3. But Phase 3 needs sudo for `tee`, `sed`, `mkdir`, `sysctl`, `augenrules` — none of which are in the narrow allowlist (and shouldn't be: `sudo tee` can write any file and effectively equals NOPASSWD ALL). Re-sequencing Task 10 to *after* Phase 3 avoided either bloating the allowlist or requiring 10+ password prompts during Phase 3.

### Defense-in-depth additions
- `auditd` with baseline rules covering `/etc/passwd`, `/etc/shadow`, `/etc/sudoers*`, `/etc/ssh/sshd_config*`, sudo/su execution, cron paths. Rules made immutable via `-e 2` (changes require reboot). Log rotation at 50MB × 4 files (~200MB cap).
- `journald` made persistent (`/var/log/journal`), retention capped at 2GB / 30 days.
- `sysctl` hardening drop-in (`/etc/sysctl.d/99-hardening.conf`): `kptr_restrict=2`, `dmesg_restrict=1`, `yama.ptrace_scope=1`, `unprivileged_bpf_disabled=1`, plus standard network-stack hygiene (`rp_filter`, `accept_redirects=0`, `tcp_syncookies=1`).
- `fail2ban`: existing `jail.local` updated to `bantime=1h`, `maxretry=3`. Global `[DEFAULT]` defaults updated too.
- AppArmor verified loaded with 4 profiles in enforce mode (docker-default, lsb_release, nvidia_modprobe). 23 profiles in complain mode are all desktop/GUI (Xorg, plasmashell, sbuild-*, transmission-*) and intentionally not flipped.
- `lynis` installed; baseline audit captured at `docs/security/lynis-baseline-2026-04-16.log`.

### Security repository (the silent gap)
`/etc/apt/sources.list` lacked the `security.debian.org` entry. `unattended-upgrades` was configured to allow security-origin packages but had no source to fetch them from — silently doing nothing since the migration. Added `deb http://security.debian.org/debian-security trixie-security main contrib non-free non-free-firmware`. The first `apt-get update` after this fetched 15 pending security patches (OpenSSL, OpenSSH, libpng, libtiff, libfreetype, gdk-pixbuf, kernel 6.12.73 → 6.12.74). All applied, and the box rebooted onto the new kernel as part of the Phase 3 verification.

### Minikube auto-start via systemd oneshot
`minikube.service` (`Type=oneshot RemainAfterExit=yes`) runs `minikube start` as `kyle` after `docker.service`. `minikube-tunnel.service` updated to `Requires=minikube.service` so the tunnel doesn't start before the cluster.

### Ethernet auto-connect via ifupdown
Debian 13 default install left `enp4s0` completely absent from `/etc/network/interfaces` (only `lo` was configured). Added `auto enp4s0 / iface enp4s0 inet dhcp`. Verified by reboot.

### Lynis hardening index: 77
Final score 77 (target was ≥75). Pre-remediation score: 70. The 7-point gap was closed primarily by adding the security repository (single biggest contributor), the SSH lynis additions, and `login.defs` tightening (`PASS_MAX_DAYS=90`, `PASS_MIN_DAYS=1`, `UMASK=027`, `SHA_CRYPT_*_ROUNDS=10000`).

## Out of scope (deferred, intentional)
- File-integrity monitoring (AIDE), `rkhunter`, `chkrootkit` — high maintenance cost, low marginal value for a single-tenant home box.
- Docker `userns-remap` — risk of breaking Minikube outweighs gain.
- SSH cert-based auth instead of raw keys — fine for a single user / single trusted client.
- Backups / DR — separate concern, separate spec.
- Separate `/home` and `/var` partitions, GRUB password, USB driver disable — flagged by lynis but require partition rebuild or are unnecessary for the threat model.

## References
- Spec: `docs/superpowers/specs/2026-04-16-debian-host-hardening-design.md`
- Plan: `docs/superpowers/plans/2026-04-16-debian-host-hardening.md`
- Migration spec (predecessor): `docs/superpowers/specs/2026-04-15-debian-server-migration-design.md`
- Baseline lynis report: `docs/security/lynis-baseline-2026-04-16.log`
