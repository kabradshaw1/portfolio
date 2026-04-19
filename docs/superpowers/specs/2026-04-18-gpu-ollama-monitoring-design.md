# GPU & Ollama Monitoring with Telegram Alerts

**Date:** 2026-04-18
**Status:** Draft
**Trigger:** Production incident — NVIDIA kernel module not rebuilt after kernel update, Ollama fell back to CPU silently, shopping assistant became unresponsive (~45s per request instead of ~2s).

## Problem

There is no alerting when the GPU becomes unavailable or when Ollama degrades. Today's incident went unnoticed until a manual test revealed the shopping assistant was non-functional. The root cause chain was:

1. Debian kernel auto-updated from 6.12.73 → 6.12.74
2. Kernel headers weren't installed, so DKMS couldn't rebuild the NVIDIA module
3. Ollama silently fell back to 100% CPU
4. The 14B model became too slow for the agent loop (~45s per turn, no tool calls)

## Scope

Six components, all additive — no changes to existing application code or dashboards.

## Component 1: Ollama Keep-Alive Override

**What:** Systemd override file at `/etc/systemd/system/ollama.service.d/override.conf` that sets `OLLAMA_KEEP_ALIVE=-1`.

**Why:** Models must stay loaded in GPU memory permanently. Without this, models unload after 5 minutes of inactivity (Ollama default), causing cold-start latency on the next request. The override survives both Ollama restarts and package updates — the main service file may be replaced by upgrades, but the override directory is never touched.

**Config:**
```ini
[Service]
Environment=OLLAMA_KEEP_ALIVE=-1
```

**Commands:**
```bash
sudo systemctl edit ollama   # creates override.conf
# paste the [Service] block above
sudo systemctl daemon-reload
sudo systemctl restart ollama
```

## Component 2: nvidia_gpu_exporter on Debian

**What:** Install the `nvidia_gpu_exporter` binary on the Debian host, run as a systemd service on port 9835.

**Why:** Prometheus already has an `nvidia-gpu` scrape job targeting `host.minikube.internal:9835`, but nothing is listening there. The exporter was previously set up on the Windows host — now it needs to run on Debian where the GPU lives.

**Details:**
- Download the linux-amd64 binary from the [nvidia_gpu_exporter GitHub releases](https://github.com/utkuozdemir/nvidia_gpu_exporter/releases)
- Install to `/usr/local/bin/nvidia_gpu_exporter`
- Systemd unit: `/etc/systemd/system/nvidia_gpu_exporter.service`
- Runs as a dedicated `nvidia-exporter` user (no root needed, just access to `nvidia-smi`)
- Exports: GPU utilization %, VRAM used/total, temperature, power draw, fan speed
- Port 9835 matches existing Prometheus scrape config — no Prometheus changes needed

**Systemd unit:**
```ini
[Unit]
Description=NVIDIA GPU Exporter
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/nvidia_gpu_exporter
User=nvidia-exporter
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

## Component 3: Kernel Header Auto-Install

**What:** Install the `linux-headers-amd64` meta-package on the Debian host.

**Why:** This meta-package automatically pulls in headers matching the current kernel. When `unattended-upgrades` or `apt upgrade` installs a new kernel, the matching headers come with it, and DKMS rebuilds the NVIDIA module automatically. This is the direct fix for today's root cause.

**Command:**
```bash
sudo apt install linux-headers-amd64
```

**Verification:** After next kernel update, check `ls /lib/modules/$(uname -r)/updates/dkms/` for `nvidia-current.ko*`.

## Component 4: Telegram Bot + Grafana Contact Point

**What:** Create a Telegram bot and configure it as a Grafana alert notification channel.

**Why:** Push notifications to Kyle's phone when critical infrastructure fails. Telegram is free, Grafana has native Telegram integration (built-in contact point type), and the notification experience is equivalent to SMS/WhatsApp.

**Setup steps:**
1. Message @BotFather on Telegram → `/newbot` → name it (e.g., "Portfolio Alerts")
2. Save the bot token
3. Create a Telegram group or use direct messages — get the chat ID via `https://api.telegram.org/bot<token>/getUpdates` after sending the bot a message
4. In Grafana → Alerting → Contact Points → New → Telegram:
   - Bot token: `<from BotFather>`
   - Chat ID: `<from step 3>`
5. Set as default contact point for all alert rules

**Note:** Grafana is configured with anonymous access (viewer role) and login disabled. To configure alerting, we'll need to either:
- Temporarily enable admin login, or
- Configure the contact point via Grafana provisioning (ConfigMap) — this is the preferred approach since it's declarative and survives pod restarts

We'll use provisioning: add a `grafana-alerting.yml` ConfigMap with the contact point and notification policy, mounted into the Grafana pod.

## Component 5: Prometheus Alert Rules

**What:** Add alerting rules to the Prometheus ConfigMap and configure Prometheus to evaluate them. Alerts fire to Grafana via its Prometheus datasource (Grafana evaluates alert rules against Prometheus queries).

**Why:** Detect GPU, Ollama, and ai-service failures automatically.

**Approach:** Use Grafana-managed alerts (not Prometheus Alertmanager). Grafana queries Prometheus and evaluates alert conditions on its own schedule. This avoids deploying Alertmanager — simpler for a single notification channel.

**Alert rules (configured in Grafana provisioning):**

| Alert | PromQL | Severity | For | Description |
|-------|--------|----------|-----|-------------|
| GPUExporterDown | `up{job="nvidia-gpu"} == 0` | critical | 2m | GPU exporter unreachable — GPU may be down |
| OllamaDown | `up{job="ollama"} == 0` | critical | 2m | Ollama not responding to health checks |
| AIServiceNotReady | `kube_pod_status_ready{pod=~"go-ai-service.*", condition="true"} == 0` | critical | 3m | AI service pod not ready |
| GPUTemperatureHigh | `nvidia_gpu_temperature_celsius > 85` | warning | 5m | GPU running hot |
| GPUVRAMHigh | `(nvidia_gpu_vram_used_bytes / nvidia_gpu_vram_total_bytes) > 0.9` | warning | 10m | GPU memory nearly full |

**OllamaCPUFallback detection:** The GPU exporter going down while Ollama stays up is the signal for CPU fallback (exactly today's scenario). The GPUExporterDown alert covers this case. If we want an explicit "Ollama on CPU" alert, we could add a simple script that checks `ollama ps` output — but the GPU exporter down alert is sufficient for now.

## Component 6: Ollama Prometheus Scrape Target

**What:** Add an `ollama` job to the Prometheus ConfigMap scraping `host.minikube.internal:11434`.

**Why:** Gives us the `up{job="ollama"}` metric for the OllamaDown alert. Ollama doesn't expose a `/metrics` endpoint, but Prometheus still records target health (`up` metric) from any HTTP endpoint. We'll use `/api/tags` as the health check path (returns 200 with the model list).

**Addition to prometheus-config ConfigMap:**
```yaml
- job_name: "ollama"
  metrics_path: /api/tags
  static_configs:
    - targets: ["host.minikube.internal:11434"]
```

## Out of Scope

- **Grafana dashboard changes** — existing panels already cover GPU and Ollama metrics
- **Alertmanager deployment** — Grafana's built-in alerting handles this directly
- **Jaeger fix** — ImagePullBackOff is a separate issue (explains OTel trace export errors in ai-service logs)
- **Kafka CrashLoopBackOff** — observed during investigation, separate issue

## Files Changed

| File | Change |
|------|--------|
| `k8s/monitoring/configmaps/prometheus-config.yml` | Add `ollama` scrape job |
| `k8s/monitoring/configmaps/grafana-alerting.yml` | New — alert rules, contact point, notification policy provisioning |
| `k8s/monitoring/deployments/grafana.yml` | Mount alerting provisioning ConfigMap |

## Manual Steps (Debian server, requires sudo)

1. Install `linux-headers-amd64`
2. Create Ollama systemd override with `OLLAMA_KEEP_ALIVE=-1`
3. Download and install `nvidia_gpu_exporter` binary + systemd service
4. Create Telegram bot and record token + chat ID
5. Add bot token and chat ID to a Kubernetes secret for Grafana provisioning
