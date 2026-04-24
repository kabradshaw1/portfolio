# Debian 13 Server Migration

- **Date:** 2026-04-15
- **Status:** Accepted
- **Supersedes:** [Windows SSH Setup](windows-ssh-setup.md) (partially), updates [Deployment Architecture](deployment-architecture.md)

## Context

The portfolio's backend infrastructure — Minikube cluster, Ollama with RTX 3090, Cloudflare tunnel, and CI/CD deploy target — ran on a Windows 11 PC accessed via Tailscale. This worked but had friction points:

- **Windows scheduled tasks** for Ollama and `minikube tunnel` were brittle and hard to debug remotely
- **Secure Boot** blocked unsigned kernel modules (required manual BIOS intervention for NVIDIA driver on fresh installs)
- **Docker Desktop** on Windows added licensing complexity and memory overhead compared to Docker CE on Linux
- **No unattended security updates** — Windows Update required interactive approval
- The Windows installation was lost (not backed up), requiring a full rebuild regardless

The same physical machine (i7-11700K, 32GB RAM, RTX 3090, 2x NVMe) was available for a fresh OS install. The question was whether to reinstall Windows or switch to Linux.

## Decision

Replace Windows 11 with **Debian 13 (Trixie)** as the deployment host. Debian was chosen over other Linux distributions because:

- **Stability over bleeding-edge** — Debian stable releases are conservative, which suits a server that should run unattended. Ubuntu Server was the main alternative but adds Snap and Canonical-specific tooling that isn't needed.
- **systemd-native services** — Ollama, `minikube tunnel`, `cloudflared`, and `tailscaled` all run as systemd units with automatic restart, replacing the fragile Windows Scheduled Tasks.
- **Minimal install** — Debian's netinst produces a small base system with no desktop environment, reducing attack surface and resource usage.
- **Native Docker CE** — No Docker Desktop license required. Docker runs as a regular systemd service.

### Infrastructure Mapping

| Component | Windows (before) | Debian (after) |
|-----------|-----------------|----------------|
| OS | Windows 11 Pro | Debian 13 (Trixie) |
| Init system | Scheduled Tasks (PowerShell) | systemd units |
| Docker | Docker Desktop | Docker CE 29.4 |
| NVIDIA driver | Windows driver (auto-install) | `nvidia-driver` 550.163 via apt (requires Secure Boot disabled) |
| Ollama | Scheduled task, user `PC` | systemd service, user `ollama`, `OLLAMA_HOST=0.0.0.0` |
| Minikube tunnel | Scheduled task, SYSTEM user | systemd service, root (needs `ip route` privileges) |
| Cloudflare tunnel | `cloudflared` Windows service | `cloudflared` systemd service |
| Tailscale | Tailscale Windows service | `tailscaled` systemd service |
| SSH | OpenSSH Server (Windows feature) | `openssh-server` (Debian package) |
| Firewall | Windows Firewall (basic) | UFW with restrictive rules |
| Tailscale IP | 100.79.113.84 | 100.82.52.82 |
| SSH user | `PC` | `kyle` |

### Security Hardening

The Debian server includes production-grade hardening that the Windows setup lacked:

- **SSH:** Key-only authentication, root login disabled, password auth disabled
- **UFW firewall:** Default deny incoming, SSH allowed only from Tailscale subnet (100.64.0.0/10), Ollama port restricted to localhost and Docker/Minikube networks
- **fail2ban:** SSH brute-force protection (5 attempts, 10-minute ban)
- **Unattended upgrades:** Automatic security patches from Debian stable
- **Routed traffic:** UFW allows routed traffic (`allow routed`) because Docker containers use kernel-level forwarding to reach host services — standard UFW INPUT rules don't apply to this path

### Configuration Fixes Discovered During Migration

**LLM_BASE_URL in ConfigMaps:** The Python AI services' `Settings` class has both `llm_base_url` and `ollama_base_url` fields. The K8s ConfigMaps only set `OLLAMA_BASE_URL`, which maps to `ollama_base_url` (the legacy fallback). But `get_llm_base_url()` returns `llm_base_url` first, which defaults to `http://host.docker.internal:11434` — a Docker Compose address that doesn't exist in Kubernetes. The fix was adding `LLM_BASE_URL` and `EMBEDDING_BASE_URL` to all three AI service ConfigMaps. This was a latent bug masked on Windows because the services happened to start after Ollama was ready and the health check behavior differed.

**Cloudflare tunnel target:** On Windows, `minikube tunnel` created a LoadBalancer that bound to `localhost:80`. On Debian with Docker driver, the ingress controller uses NodePort instead. The Cloudflare tunnel config was changed from `http://localhost:80` to `http://192.168.49.2:80` (the Minikube node IP) to reach the ingress directly.

**Kustomize cycle detection:** kubectl 1.35 (bundled Kustomize v5.7.1) detects false cycles when overlay directories exist inside the base directory. The `java/k8s/overlays/minikube/kustomization.yaml` references `../../` as its base, and the new Kustomize considers this a cycle. Workaround: apply the base directly with `kubectl kustomize java/k8s/ | kubectl apply -f -` instead of `kubectl apply -k java/k8s/overlays/minikube`. This needs a permanent fix in CI if the deploy script uses overlay paths.

### CI/CD Changes

- SSH target in `.github/workflows/ci.yml` changed from `PC@100.79.113.84` to `kyle@100.82.52.82`
- New ED25519 deploy key generated on Debian, public key in `~/.ssh/authorized_keys`, private key in GitHub secret `SSH_PRIVATE_KEY`
- GHCR pull secret (`ghcr-secret`) created in all 7 namespaces using existing GitHub PAT

## Consequences

### Positive

- **Reliability:** systemd services with `Restart=on-failure` are more robust than Windows Scheduled Tasks. Services that crash restart automatically without the 3-retry limit.
- **Security:** UFW + fail2ban + key-only SSH + unattended upgrades provide defense-in-depth that the Windows setup didn't have.
- **Simpler remote management:** Everything is configurable over SSH. No RDP or interactive Windows sessions needed.
- **Lower resource overhead:** No Windows desktop, no Docker Desktop. More memory available for Minikube and Ollama.
- **Portfolio value:** Demonstrates Linux server administration, systemd service management, and security hardening — relevant skills for a Gen AI Engineer role.

### Trade-offs

- **Secure Boot disabled:** The NVIDIA driver kernel module isn't signed for Debian. Disabling Secure Boot was the pragmatic choice over MOK key enrollment. Acceptable for a server behind Tailscale with no physical access concerns.
- **No automatic network on boot:** Required manually configuring the network interface to auto-connect. This caused confusion during the initial NVIDIA driver reboot.
- **Tailscale static binary:** Initially installed from a tarball (not the apt package) because the machine had no network access. The systemd service was created manually. Should be replaced with the proper apt package when convenient.
- **Minikube node IP hardcoded:** The Cloudflare tunnel points to `192.168.49.2:80` which is stable for Minikube's Docker driver but would break if the cluster is recreated with a different subnet.
- **Kustomize compatibility:** The overlay cycle issue needs a permanent fix — either pin Kustomize version or restructure the overlay directories.

### Remaining Work

- Set root password (skipped during install)
- Google OAuth credentials for Java/Go services
- Fix Jaeger image version in monitoring namespace
- Deploy QA namespace workloads
- Add Kubernetes NetworkPolicies (production-grade hardening goal)
- Replace Tailscale static binary with apt package
- Fix Kustomize overlay cycle for Java manifests

## See Also

- [Deployment Architecture](deployment-architecture.md) — high-level architecture (needs updating for Debian references)
- [CI/CD Pipeline](cicd-pipeline.md) — pipeline details
- [Migration Design Spec](../superpowers/specs/2026-04-15-debian-server-migration-design.md) — original spec for this work
