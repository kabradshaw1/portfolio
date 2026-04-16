# Debian 13 Host Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Take the Debian 13 production host (`debian` / `100.82.52.82`) from "works but partially exposed" to "internet-exposed-but-locked-down": Tailscale-only SSH, UFW with default-deny, narrow passwordless sudo for routine ops, auto-start for Minikube + ethernet, and baseline auditing (auditd, journald persistence, sysctl hardening, lynis report).

**Architecture:** Three phases with explicit Kyle-approval gates between them. Phase 1 stages everything risky (UFW rules, snapshots) without enabling. Phase 2 enables in order: UFW → SSH lockdown → sudo narrowing, with verification after each. Phase 3 adds auditing/kernel hardening (low risk, no per-step gates). Every Phase 2 change is a *new* file (not an edit), so console rollback is always "delete one file, reload one service." Snapshots taken before Phase 2 and Phase 3.

**Tech Stack:**
- Debian 13.4, kernel 6.12.73, single user `kyle`
- `systemd` (units, drop-ins, journald), `ufw`, `openssh-server`, `sudo`/`visudo`
- `auditd`, `lynis`, `fail2ban`, AppArmor (Debian default)
- Networking: `ifupdown` (`/etc/network/interfaces`) currently — fall back to `systemd-networkd` if needed
- Remote ops via `ssh debian` from Mac (key-based, alias in `~/.ssh/config`)

**Source spec:** `docs/superpowers/specs/2026-04-16-debian-host-hardening-design.md`

**Repo deliverables (committed on `main`, no push without Kyle approval):**
- `docs/security/lynis-baseline-2026-04-16.log` — baseline audit report
- `docs/adr/2026-04-16-debian-host-hardening.md` — short ADR capturing final state and key decisions

---

## Phase 1: Bootstrap & Quick Wins

### Task 1: Bootstrap — grant agent temporary NOPASSWD ALL

**Files:**
- Create on `debian`: `/etc/sudoers.d/kyle-temp`

This task requires Kyle to type the command (sudo password). Everything after this is unattended.

- [ ] **Step 1: Show Kyle the bootstrap command**

Output to chat:
```
! ssh -t debian 'sudo bash -c "
  cat > /tmp/kyle-temp.sudoers <<EOF
# Temporary unrestricted sudo for agent during host-hardening work.
# This file is removed automatically in Phase 2 (replaced by kyle-ops).
kyle ALL=(ALL) NOPASSWD: ALL
EOF
  visudo -c -f /tmp/kyle-temp.sudoers && \
    install -m 0440 -o root -g root /tmp/kyle-temp.sudoers /etc/sudoers.d/kyle-temp && \
    rm /tmp/kyle-temp.sudoers && \
    echo BOOTSTRAP_OK
"'
```

- [ ] **Step 2: Wait for Kyle to confirm `BOOTSTRAP_OK`**

If Kyle reports anything other than `BOOTSTRAP_OK` (e.g., `visudo` parse error), STOP and debug before continuing.

- [ ] **Step 3: Verify passwordless sudo works for the agent**

```bash
ssh debian 'sudo -n true && echo SUDO_OK'
```
Expected: `SUDO_OK`

- [ ] **Step 4: No commit (remote-only change)**

---

### Task 2: Install minikube auto-start systemd units

**Files:**
- Create on `debian`: `/etc/systemd/system/minikube.service`
- Replace on `debian`: `/etc/systemd/system/minikube-tunnel.service`

- [ ] **Step 1: Stage `minikube.service` to `/tmp` on debian**

```bash
ssh debian 'cat > /tmp/minikube.service <<EOF
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
EOF'
```

- [ ] **Step 2: Stage updated `minikube-tunnel.service` to `/tmp` on debian**

```bash
ssh debian 'cat > /tmp/minikube-tunnel.service <<EOF
[Unit]
Description=Minikube Tunnel
After=minikube.service
Requires=minikube.service

[Service]
Type=simple
ExecStart=/usr/local/bin/minikube tunnel --cleanup=true
Environment=HOME=/home/kyle
Environment=MINIKUBE_HOME=/home/kyle/.minikube
User=kyle
Group=kyle
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF'
```

- [ ] **Step 3: Verify `which minikube` is `/usr/local/bin/minikube` (the path baked into the units)**

```bash
ssh debian 'which minikube'
```
Expected: `/usr/local/bin/minikube`. If different, edit the staged unit files in `/tmp` to match before installing.

- [ ] **Step 4: Install both units, reload systemd, enable**

```bash
ssh debian 'sudo install -m 644 /tmp/minikube.service /etc/systemd/system/minikube.service && \
            sudo install -m 644 /tmp/minikube-tunnel.service /etc/systemd/system/minikube-tunnel.service && \
            sudo systemctl daemon-reload && \
            sudo systemctl enable minikube.service minikube-tunnel.service && \
            echo INSTALL_OK'
```
Expected: `INSTALL_OK` (plus `Created symlink` lines from `enable`).

- [ ] **Step 5: Verify both units are enabled and active**

```bash
ssh debian 'systemctl is-enabled minikube.service minikube-tunnel.service && \
            systemctl is-active minikube.service minikube-tunnel.service'
```
Expected: 4 lines, `enabled` `enabled` `active` `active`.

- [ ] **Step 6: No commit (remote-only change)**

---

### Task 3: Install UFW with default-deny and allow rules (rules drafted, firewall NOT enabled)

**Files:**
- Install on `debian`: `ufw` package
- Configure on `debian`: UFW rules (in-memory; written by `ufw allow` commands)

- [ ] **Step 1: Install ufw**

```bash
ssh debian 'sudo DEBIAN_FRONTEND=noninteractive apt-get install -y ufw && echo UFW_INSTALLED'
```
Expected: `UFW_INSTALLED`.

- [ ] **Step 2: Set default policies**

```bash
ssh debian 'sudo ufw default deny incoming && sudo ufw default allow outgoing'
```
Expected: two `Default ... policy changed to ...` confirmations.

- [ ] **Step 3: Add allow rules**

```bash
ssh debian '
  sudo ufw allow proto tcp from 100.64.0.0/10 to any port 22 comment "ssh from tailnet" && \
  sudo ufw allow proto tcp from 192.168.1.0/24 to any port 22 comment "ssh from LAN (removed in Phase 2)" && \
  sudo ufw allow proto tcp from 172.17.0.0/16 to any port 11434 comment "ollama from docker bridge" && \
  sudo ufw allow proto tcp from 100.64.0.0/10 to any port 11434 comment "ollama from tailnet"
'
```

- [ ] **Step 4: Verify rules listed but UFW still inactive**

```bash
ssh debian 'sudo ufw status verbose'
```
Expected: `Status: inactive` followed by the four allow rules in the "Anywhere" section. **Do not enable here** — that's Phase 2.

- [ ] **Step 5: No commit (remote-only change)**

---

### Task 4: Fix ethernet auto-connect

**Files:**
- Inspect on `debian`: `/etc/network/interfaces`
- Possibly modify or replace on `debian`: `/etc/network/interfaces` OR `/etc/systemd/network/10-ethernet.network`

The current config uses `ifupdown` (neither NetworkManager nor systemd-networkd is active). Two paths depending on what's already there: prefer fixing `ifupdown`, fall back to `systemd-networkd`.

- [ ] **Step 1: Identify the ethernet interface name**

```bash
ssh debian 'ls /sys/class/net/ | grep -E "^en|^eth"'
```
Expected: typically one interface like `enp4s0` or `eno1`. Note the name (used below as `<IFACE>`).

- [ ] **Step 2: Inspect `/etc/network/interfaces`**

```bash
ssh debian 'cat /etc/network/interfaces; echo ---; ls /etc/network/interfaces.d/ 2>/dev/null'
```

- [ ] **Step 3: Decision branch — `ifupdown` path**

If the interface (`<IFACE>`) is mentioned in `/etc/network/interfaces` or `/etc/network/interfaces.d/` but lacks an `auto <IFACE>` line, append it (using `sudo tee -a` so the redirection runs as root):

```bash
ssh debian 'sudo tee -a /etc/network/interfaces > /dev/null <<EOF

auto <IFACE>
iface <IFACE> inet dhcp
EOF'
```

(Adjust if it already has an `iface` block — append only the missing `auto <IFACE>` line.)

- [ ] **Step 3 (alternative): Decision branch — `systemd-networkd` path**

If `/etc/network/interfaces` is empty / does not mention the interface, install a minimal systemd-networkd config:

```bash
ssh debian 'sudo bash -c "cat > /etc/systemd/network/10-ethernet.network <<EOF
[Match]
Name=<IFACE>

[Network]
DHCP=yes
EOF
systemctl enable --now systemd-networkd systemd-resolved"'
```

- [ ] **Step 4: Test by toggling the link**

```bash
ssh debian 'sudo ip link set <IFACE> down && sleep 2 && sudo ip link set <IFACE> up && sleep 5 && ip -4 addr show <IFACE>'
```
Expected: an `inet 192.168.x.x/24` line within ~5s of bringing the interface back up.

- [ ] **Step 5: No commit (remote-only change)**

---

### Task 5: Flag stale Tailscale node for Kyle

**Files:** None — this is a Tailscale admin-console action.

- [ ] **Step 1: Confirm `pc-master-race` is still in the tailnet**

```bash
ssh debian 'tailscale status | grep pc-master-race || echo NODE_GONE'
```

If `NODE_GONE`, skip the rest of this task.

- [ ] **Step 2: Tell Kyle**

> "Heads up — `pc-master-race` (the old Windows PC) is still in your tailnet (offline 2 days). It's safe to delete from the Tailscale admin console at https://login.tailscale.com/admin/machines. I can't do this from the host. Confirm when removed (or skip)."

- [ ] **Step 3: Wait for Kyle's confirmation, then continue**

---

### Task 6: Phase 1 verification gate (reboot test)

**Files:** None — verification only.

- [ ] **Step 1: Tell Kyle Phase 1 is done and request a reboot**

> "Phase 1 staged. Want to reboot now to confirm everything auto-starts cleanly? Run: `! ssh debian sudo reboot`. SSH will drop; reconnect in ~60s."

- [ ] **Step 2: After Kyle confirms reboot, wait for SSH to recover**

```bash
ssh debian 'echo BACK; uptime'
```

- [ ] **Step 3: Verify all services are up**

```bash
ssh debian '
  echo "=== systemd units ==="
  systemctl is-active docker minikube minikube-tunnel ollama cloudflared fail2ban
  echo "=== ethernet ==="
  ip -4 addr show | grep inet | grep -v 127.0.0.1
  echo "=== ufw (should be inactive) ==="
  sudo ufw status | head -1
  echo "=== minikube cluster ==="
  kubectl get nodes
'
```
Expected: 6 `active` lines, an `inet 192.168.x.x` line, `Status: inactive`, one `Ready` node.

- [ ] **Step 4: Verify K8s pods are recovering**

```bash
ssh debian 'kubectl get pods -A | grep -vE "Running|Completed" | head -20'
```
Expected: empty (or only the pre-existing `jaeger` ImagePull issue).

If any new pods are stuck, debug before proceeding.

- [ ] **Step 5: Verify Ollama still reachable from a pod**

```bash
ssh debian 'kubectl -n ai-services exec deploy/chat -- curl -s --max-time 5 http://host.minikube.internal:11434/api/tags | head -c 80'
```
Expected: a JSON snippet starting `{"models":[`.

- [ ] **Step 6: Verify Ollama still reachable from Mac over LAN (UFW still off)**

```bash
curl -s --max-time 5 http://<debian-lan-ip>:11434/api/tags | head -c 80
```
Expected: same JSON snippet.

- [ ] **Step 7: Hand off to Kyle for Phase 2 approval**

> "Phase 1 verified clean: minikube, tunnel, ollama, ethernet all auto-started, kubectl works, pods reach Ollama. Ready for Phase 2 (lockdown — UFW enable, SSH Tailscale-only, sudo narrow). Approve to proceed?"

---

## Phase 2: Lockdown

### Task 7: Take pre-Phase-2 snapshots

**Files:**
- Create on `debian`: `/root/etc-ssh-backup-2026-04-16/`, `/root/sudoers-backup-2026-04-16/`

- [ ] **Step 1: Snapshot `/etc/ssh` and `/etc/sudoers.d`**

```bash
ssh debian 'sudo cp -a /etc/ssh /root/etc-ssh-backup-2026-04-16/ && \
            sudo cp -a /etc/sudoers.d /root/sudoers-backup-2026-04-16/ && \
            ls -la /root/ | grep 2026-04-16'
```
Expected: two new directories listed.

- [ ] **Step 2: No commit (remote-only)**

---

### Task 8: Enable UFW + verify

**Files:** None — applies the rules drafted in Task 3.

- [ ] **Step 1: Enable UFW**

```bash
ssh debian 'sudo ufw --force enable && sudo ufw status verbose'
```
Expected: `Status: active` plus the four allow rules. (`--force` skips the interactive "this may disconnect SSH" prompt — safe because Tailscale SSH is allowed.)

- [ ] **Step 2: Verify SSH (current session) still alive**

The current SSH session must not have dropped. If it did, that's a Tailscale routing issue — investigate immediately.

- [ ] **Step 3: Verify K8s pods can still reach Ollama**

```bash
ssh debian 'kubectl -n ai-services exec deploy/chat -- curl -s --max-time 5 http://host.minikube.internal:11434/api/tags | head -c 80'
```
Expected: `{"models":[...` JSON snippet.

- [ ] **Step 4: Verify LAN SSH still works (we kept the rule)**

From Mac: `ssh kyle@<debian-lan-ip> 'echo LAN_SSH_OK'`
Expected: `LAN_SSH_OK`. (LAN rule will be removed in Task 10.)

- [ ] **Step 5: Confirm with Kyle**

> "UFW enabled. SSH (Tailscale + LAN) still works, pods still reach Ollama. Proceed to SSH lockdown?"

---

### Task 9: Lock SSH to Tailscale-only

**Files:**
- Create on `debian`: `/etc/ssh/sshd_config.d/10-hardening.conf`

- [ ] **Step 1: Stage the SSH hardening drop-in**

```bash
ssh debian 'cat > /tmp/10-hardening.conf <<EOF
# SSH hardening — Tailscale-only listener, key-only auth.
# Remove this file and reload sshd to revert.
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
EOF
sudo sshd -t -f /etc/ssh/sshd_config && echo BASE_CONFIG_VALID'
```
Expected: `BASE_CONFIG_VALID` (validates the existing config still parses; the drop-in will be checked on install).

- [ ] **Step 2: Install the drop-in and validate full config**

```bash
ssh debian 'sudo install -m 644 /tmp/10-hardening.conf /etc/ssh/sshd_config.d/10-hardening.conf && \
            sudo sshd -t && echo SSHD_CONFIG_VALID'
```
Expected: `SSHD_CONFIG_VALID`. If this fails, `sudo rm /etc/ssh/sshd_config.d/10-hardening.conf` and debug.

- [ ] **Step 3: Reload sshd (does not drop existing sessions)**

```bash
ssh debian 'sudo systemctl reload ssh && echo RELOADED'
```
Expected: `RELOADED`. The current SSH session must remain alive.

- [ ] **Step 4: Verify SSH binds only on Tailscale IP + loopback**

```bash
ssh debian 'sudo ss -tlnp | grep :22'
```
Expected: lines containing `100.82.52.82:22` and `127.0.0.1:22` — and **no** `0.0.0.0:22` line.

- [ ] **Step 5: Open a fresh SSH session to confirm Tailscale SSH works**

From Mac (NEW terminal): `ssh debian 'echo TAILSCALE_SSH_OK'`
Expected: `TAILSCALE_SSH_OK`.

- [ ] **Step 6: Verify LAN SSH is now refused**

From Mac: `ssh -o ConnectTimeout=5 kyle@<debian-lan-ip> 'echo LAN_REACHED' 2>&1 | tail -3`
Expected: `Connection refused` or `Connection timed out`. (Either means success.)

If LAN SSH still works, the `ListenAddress` lines didn't take effect — debug `sshd -T | grep -i listen`.

- [ ] **Step 7: Remove now-unneeded LAN-SSH UFW rule**

```bash
ssh debian 'sudo ufw delete allow proto tcp from 192.168.1.0/24 to any port 22 && \
            sudo ufw status verbose | grep -A1 "Anywhere"'
```
Expected: rule deletion confirmation; LAN-SSH rule no longer listed.

- [ ] **Step 8: Confirm with Kyle**

> "SSH locked to Tailscale-only. Listener is `100.82.52.82:22` + loopback only. LAN SSH refused. Fresh tailnet SSH works. Proceed to sudo narrowing?"

---

### Task 10: Narrow sudo to routine-ops allowlist

**Files:**
- Create on `debian`: `/etc/sudoers.d/kyle-ops`
- Delete on `debian`: `/etc/sudoers.d/kyle-temp`

- [ ] **Step 1: Identify exact paths for binaries in the allowlist**

```bash
ssh debian 'for b in systemctl journalctl apt apt-get dpkg kubectl minikube docker ufw lynis; do printf "%-12s -> %s\n" "$b" "$(which $b 2>/dev/null || echo MISSING)"; done'
```
Expected: every binary except `lynis` resolves (lynis is installed in Phase 3 — that's fine, the sudoers entry is path-based and works once installed). If anything else is `MISSING`, adjust the staged sudoers in Step 2 to use the actual path.

- [ ] **Step 2: Stage `/etc/sudoers.d/kyle-ops`**

```bash
ssh debian 'cat > /tmp/kyle-ops <<EOF
# Routine ops for the agent — passwordless.
# Anything not listed here still requires kyles password.

kyle ALL=(root) NOPASSWD: /usr/bin/systemctl, \\
                          /usr/bin/journalctl, \\
                          /usr/bin/apt, \\
                          /usr/bin/apt-get, \\
                          /usr/bin/dpkg, \\
                          /usr/local/bin/kubectl, \\
                          /usr/local/bin/minikube, \\
                          /usr/bin/docker, \\
                          /usr/sbin/ufw status, \\
                          /usr/sbin/ufw status verbose, \\
                          /usr/bin/lynis audit system

# Privilege-changing actions still require password
kyle ALL=(ALL) ALL
EOF
sudo visudo -c -f /tmp/kyle-ops && echo SUDOERS_VALID'
```
Expected: `/tmp/kyle-ops: parsed OK` plus `SUDOERS_VALID`.

- [ ] **Step 3: Install kyle-ops, then delete kyle-temp**

```bash
ssh debian 'sudo install -m 0440 -o root -g root /tmp/kyle-ops /etc/sudoers.d/kyle-ops && \
            sudo rm /etc/sudoers.d/kyle-temp && \
            sudo visudo -c && \
            ls /etc/sudoers.d/'
```
Expected: `/etc/sudoers.d/kyle-ops: parsed OK`, `/etc/sudoers.d/README: parsed OK`, listing shows `README kyle-ops`.

- [ ] **Step 4: Verify routine ops still work without password**

```bash
ssh debian 'sudo -n systemctl is-active sshd && \
            sudo -n journalctl --since "5 min ago" -u ssh | tail -3 > /dev/null && \
            sudo -n ufw status | head -1 && \
            echo PASSWORDLESS_OPS_OK'
```
Expected: `active`, then ufw status line, then `PASSWORDLESS_OPS_OK`.

- [ ] **Step 5: Verify privilege-changing ops are gated**

```bash
ssh debian 'sudo -n useradd testuser 2>&1 | tail -1; sudo -n ufw disable 2>&1 | tail -1'
```
Expected: both lines say `sudo: a password is required`. (No actual user added; no firewall change.)

- [ ] **Step 6: Confirm with Kyle**

> "Sudo narrowed. Passwordless for systemctl/journalctl/apt/kubectl/minikube/docker/ufw-status/lynis. Anything privilege-changing (useradd, ufw disable, etc.) still needs your password. Phase 2 complete. Proceed to Phase 3 (auditing & kernel hardening)?"

---

## Phase 3: Auditing & Kernel Hardening

### Task 11: Pre-Phase-3 snapshot

**Files:**
- Create on `debian`: `/root/sysctl.conf.bak-2026-04-16`

- [ ] **Step 1: Snapshot `/etc/sysctl.conf`**

```bash
ssh debian 'sudo cp /etc/sysctl.conf /root/sysctl.conf.bak-2026-04-16 && ls -la /root/sysctl.conf.bak-2026-04-16'
```
Expected: file listed.

- [ ] **Step 2: No commit**

---

### Task 12: Install auditd with baseline rules and log rotation

**Files:**
- Install on `debian`: `auditd` package
- Create on `debian`: `/etc/audit/rules.d/10-baseline.rules`
- Modify on `debian`: `/etc/audit/auditd.conf`

- [ ] **Step 1: Install auditd**

```bash
ssh debian 'sudo DEBIAN_FRONTEND=noninteractive apt-get install -y auditd && echo AUDITD_INSTALLED'
```
Expected: `AUDITD_INSTALLED`.

- [ ] **Step 2: Stage baseline rules**

```bash
ssh debian 'sudo tee /etc/audit/rules.d/10-baseline.rules > /dev/null <<EOF
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

# Make rules immutable until reboot (last line)
-e 2
EOF
echo RULES_STAGED'
```
Expected: `RULES_STAGED`.

- [ ] **Step 3: Set log rotation in `auditd.conf`**

```bash
ssh debian 'sudo sed -i \
  -e "s/^max_log_file *=.*/max_log_file = 50/" \
  -e "s/^num_logs *=.*/num_logs = 4/" \
  /etc/audit/auditd.conf && \
  grep -E "^max_log_file|^num_logs" /etc/audit/auditd.conf'
```
Expected: `max_log_file = 50` and `num_logs = 4` printed.

- [ ] **Step 4: Reload rules + restart auditd**

```bash
ssh debian 'sudo augenrules --load && sudo systemctl restart auditd && sudo auditctl -l | head -20'
```
Expected: rule list (matches what we wrote) printed.

- [ ] **Step 5: No commit**

---

### Task 13: Enable journald persistence + retention caps

**Files:**
- Create on `debian`: `/var/log/journal/` directory
- Create on `debian`: `/etc/systemd/journald.conf.d/retention.conf`

- [ ] **Step 1: Create journal dir + drop-in config**

```bash
ssh debian 'sudo mkdir -p /var/log/journal /etc/systemd/journald.conf.d && \
            sudo tee /etc/systemd/journald.conf.d/retention.conf > /dev/null <<EOF
[Journal]
Storage=persistent
SystemMaxUse=2G
SystemKeepFree=1G
MaxRetentionSec=30day
EOF
echo JOURNALD_CONFIG_OK'
```
Expected: `JOURNALD_CONFIG_OK`.

- [ ] **Step 2: Restart journald**

```bash
ssh debian 'sudo systemctl restart systemd-journald && journalctl --disk-usage'
```
Expected: a line like `Archived and active journals take up 0B in the file system.` initially, growing over time.

- [ ] **Step 3: No commit**

---

### Task 14: Apply sysctl kernel hardening

**Files:**
- Create on `debian`: `/etc/sysctl.d/99-hardening.conf`

- [ ] **Step 1: Stage and install hardening sysctls**

```bash
ssh debian 'sudo tee /etc/sysctl.d/99-hardening.conf > /dev/null <<EOF
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
EOF
sudo sysctl --system 2>&1 | grep -E "99-hardening|error|invalid"'
```
Expected: lines like `* Applying /etc/sysctl.d/99-hardening.conf ...` followed by the assignments echoed back. No `error` / `invalid` lines.

- [ ] **Step 2: Verify a couple of values took effect**

```bash
ssh debian 'sysctl kernel.kptr_restrict kernel.yama.ptrace_scope net.ipv4.tcp_syncookies'
```
Expected: `kernel.kptr_restrict = 2`, `kernel.yama.ptrace_scope = 1`, `net.ipv4.tcp_syncookies = 1`.

- [ ] **Step 3: No commit**

---

### Task 15: Verify AppArmor is enforcing + tune fail2ban

**Files:**
- Possibly modify on `debian`: `/etc/fail2ban/jail.local`

- [ ] **Step 1: Check AppArmor status**

```bash
ssh debian 'sudo aa-status | head -20'
```
Expected: `apparmor module is loaded.` and `N profiles are in enforce mode.` (N ≥ 0). If any profiles are in `complain` mode that should be enforcing, escalate to Kyle before flipping.

- [ ] **Step 2: Inspect existing fail2ban config**

```bash
ssh debian 'sudo fail2ban-client status sshd; ls /etc/fail2ban/jail.local 2>&1'
```

- [ ] **Step 3: Tune sshd jail (create or update `jail.local`)**

If `/etc/fail2ban/jail.local` doesn't exist, create it:
```bash
ssh debian 'sudo tee /etc/fail2ban/jail.local > /dev/null <<EOF
[sshd]
enabled = true
bantime = 1h
maxretry = 3
EOF
sudo systemctl reload fail2ban && sudo fail2ban-client status sshd'
```
Expected: status with the new bantime/maxretry visible.

If it already exists, update only the `bantime` and `maxretry` keys in the `[sshd]` section using `sudo sed -i` (verify the section exists first; if not, append the block above).

- [ ] **Step 4: No commit**

---

### Task 16: Install lynis and run baseline audit

**Files:**
- Install on `debian`: `lynis` package
- Create on `debian`: `/var/log/lynis-baseline-2026-04-16.log`

- [ ] **Step 1: Install lynis**

```bash
ssh debian 'sudo DEBIAN_FRONTEND=noninteractive apt-get install -y lynis && lynis --version'
```
Expected: a version string (Lynis 3.x).

- [ ] **Step 2: Run the audit and save the report**

```bash
ssh debian 'sudo lynis audit system --quiet --no-colors 2>&1 | sudo tee /var/log/lynis-baseline-2026-04-16.log > /dev/null && \
            sudo grep -E "Hardening index|Tests performed" /var/log/lynis-baseline-2026-04-16.log'
```
Expected: a line like `Hardening index : 75` (or higher).

If score < 75: print the warnings/suggestions section (`sudo grep -A2 "Warning\|Suggestion" /var/log/lynis-baseline-2026-04-16.log | head -60`) and discuss with Kyle which to address now vs defer.

- [ ] **Step 3: Pull the report locally for the repo commit (Task 18)**

```bash
mkdir -p docs/security
scp debian:/var/log/lynis-baseline-2026-04-16.log docs/security/lynis-baseline-2026-04-16.log
```
Expected: file copied locally.

- [ ] **Step 4: No commit yet (commit happens in Task 18 with the ADR)**

---

### Task 17: Final reboot test

**Files:** None — verification only.

- [ ] **Step 1: Tell Kyle**

> "Phase 3 changes applied. Final reboot test? Run: `! ssh debian sudo reboot`."

- [ ] **Step 2: After reconnect, full health check**

```bash
ssh debian '
  echo "=== units ===";        systemctl is-active docker minikube minikube-tunnel ollama cloudflared fail2ban auditd
  echo "=== ufw ===";          sudo ufw status | head -1
  echo "=== sshd binding ==="; sudo ss -tlnp | grep :22
  echo "=== sysctl key ===";   sysctl kernel.kptr_restrict kernel.yama.ptrace_scope
  echo "=== auditd rules ==="; sudo auditctl -l | wc -l
  echo "=== journald ===";     journalctl --disk-usage
  echo "=== pods ===";         kubectl get pods -A | grep -vE "Running|Completed" | grep -v NAMESPACE | wc -l
'
```
Expected:
- 7 `active` lines
- `Status: active`
- only Tailscale + loopback :22 lines
- `kernel.kptr_restrict = 2`, `kernel.yama.ptrace_scope = 1`
- non-zero rule count
- a disk-usage line
- pod-not-ready count = 0 or 1 (jaeger only)

- [ ] **Step 3: Re-run lynis to confirm score holds across reboot**

```bash
ssh debian 'sudo lynis audit system --quiet --no-colors 2>&1 | grep "Hardening index"'
```
Expected: ≥ 75, ideally same as Task 16.

---

### Task 18: Commit lynis baseline + write ADR + open PR (or push)

**Files:**
- Already created: `docs/security/lynis-baseline-2026-04-16.log` (from Task 16, Step 3)
- Create: `docs/adr/2026-04-16-debian-host-hardening.md`

- [ ] **Step 1: Write the ADR**

Create `docs/adr/2026-04-16-debian-host-hardening.md` with this content:

```markdown
# ADR: Debian 13 Host Hardening (2026-04-16)

## Status
Accepted

## Context
The production Debian 13 host (`100.82.52.82`) was migrated from a Windows PC. The migration spec deferred most OS hardening, and a 2026-04-15 power outage revealed that Minikube and ethernet didn't auto-restart. This ADR records the final hardened state and the key decisions.

## Decisions

### SSH binds only to Tailscale + loopback
`/etc/ssh/sshd_config.d/10-hardening.conf` sets `ListenAddress 100.82.52.82` and `ListenAddress 127.0.0.1`. Public SSH is gone. Recovery if Tailscale is down: physical console.

### Ollama is fenced by firewall, not by bind address
Ollama keeps `OLLAMA_HOST=0.0.0.0:11434` because Minikube reaches it via `host.minikube.internal` (docker bridge IP), not loopback. UFW restricts reach to `172.17.0.0/16` (docker bridge) and `100.64.0.0/10` (Tailscale). Single-bind alternatives don't cleanly support both consumers.

### Sudo: narrow allowlist, not NOPASSWD ALL
`/etc/sudoers.d/kyle-ops` grants passwordless sudo for `systemctl`, `journalctl`, `apt`, `kubectl`, `minikube`, `docker`, `ufw status`, and `lynis audit system`. Privilege-changing actions (useradd, ufw enable/disable, sshd config edits) still require Kyle's password. The defense-in-depth gain is small once SSH is Tailscale-only, but the friction cost is also small.

### Auditd, journald-persistent, sysctl drop-in, lynis
Standard defense-in-depth posture. Baseline lynis report saved at `docs/security/lynis-baseline-2026-04-16.log` for future comparison.

### Minikube auto-start via systemd oneshot
`minikube.service` (`Type=oneshot RemainAfterExit=yes`) runs `minikube start` as `kyle` after `docker.service`. `minikube-tunnel.service` now `Requires=minikube.service` so the tunnel doesn't start before the cluster.

### Out of scope (not done, intentional)
- File-integrity monitoring (AIDE), rkhunter — high maintenance cost, low marginal value for single-tenant home box.
- Docker `userns-remap` — risk of breaking Minikube outweighs gain.
- SSH cert-based auth — raw keys are fine for a single user / single trusted client.
- Backups / DR — separate concern, separate spec.

## References
- Spec: `docs/superpowers/specs/2026-04-16-debian-host-hardening-design.md`
- Plan: `docs/superpowers/plans/2026-04-16-debian-host-hardening.md`
- Baseline lynis report: `docs/security/lynis-baseline-2026-04-16.log`
```

- [ ] **Step 2: Stage both files and run preflight (no per-language checks for docs)**

```bash
git add docs/security/lynis-baseline-2026-04-16.log docs/adr/2026-04-16-debian-host-hardening.md
git status
```
Expected: both files in "Changes to be committed".

- [ ] **Step 3: Ask Kyle whether to commit on `main` or branch + PR**

> "Phase 3 done. Two files to commit: the lynis baseline and the ADR. Both are docs only. Want me to:
> (a) commit straight to `main` (don't push — you push when ready), or
> (b) create a feature branch + worktree and open a PR to `qa` per the standard flow?"

Default to (a) — these are doc-only and the spec was committed on `main`.

- [ ] **Step 4: Commit per Kyle's choice**

If (a):
```bash
git commit -m "$(cat <<'EOF'
docs: add Debian 13 host hardening ADR + lynis baseline

Captures the final hardened state of the production host after the
2026-04-16 hardening spec: Tailscale-only SSH, narrow passwordless
sudo, UFW default-deny, auditd + journald + sysctl + lynis baseline.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>
EOF
)"
```

If (b): follow the feature-branch/worktree flow from CLAUDE.md (create `agent/feat-debian-host-hardening-docs` worktree, commit there, push, open PR to `qa`).

- [ ] **Step 5: Mark plan complete and report final state to Kyle**

> "Hardening complete. End state:
> - Public attack surface: zero (SSH Tailscale-only, Ollama firewall-fenced, no other listeners)
> - Auto-start: minikube + tunnel + ollama + cloudflared + ethernet + auditd all start on boot
> - Sudo: passwordless for routine ops, password for privilege-changing
> - Lynis baseline: <score>
> - Spec, plan, ADR, baseline report all in repo"

---

## Rollback Quick Reference

If anything breaks during execution:

| Phase | Symptom | Recovery |
|---|---|---|
| Phase 1 | Service won't start | `sudo rm /etc/systemd/system/minikube.service && sudo systemctl daemon-reload` |
| Phase 2 - UFW | Lost connectivity from Mac | (from console) `sudo ufw disable` |
| Phase 2 - SSH | Can't open new SSH session | (from console) `sudo rm /etc/ssh/sshd_config.d/10-hardening.conf && sudo systemctl reload ssh` |
| Phase 2 - sudo | Can't run any sudo | (from console) `sudo cp /root/sudoers-backup-2026-04-16/kyle-temp /etc/sudoers.d/` (kyle-temp survives in the snapshot) |
| Phase 3 - sysctl | Network weirdness | `sudo rm /etc/sysctl.d/99-hardening.conf && sudo sysctl -p /etc/sysctl.conf` |
| Phase 3 - auditd | Disk filling | `sudo systemctl stop auditd && sudo apt purge auditd` |

Snapshots are at `/root/etc-ssh-backup-2026-04-16/`, `/root/sudoers-backup-2026-04-16/`, `/root/sysctl.conf.bak-2026-04-16`.
