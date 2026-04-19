# GPU & Ollama Monitoring with Telegram Alerts

- **Date:** 2026-04-18
- **Status:** Accepted

## Context

On 2026-04-18, the shopping assistant (Go ai-service) became unresponsive in production. Investigation revealed a silent failure chain:

1. Debian auto-updated the kernel from 6.12.73 to 6.12.74
2. Kernel headers weren't installed, so DKMS couldn't rebuild the NVIDIA module for the new kernel
3. On next module load, the GPU driver failed — `/dev/nvidia*` devices disappeared
4. Ollama silently fell back to 100% CPU inference
5. The 14B model went from ~2s per response to ~45s, causing the agent loop to effectively hang (no tool calls, timeouts)

There was no monitoring or alerting for any of these failure modes. The issue was only discovered through manual testing.

The infrastructure runs on a single Debian server (RTX 3090) with Minikube (Docker driver), Prometheus, and Grafana already deployed. Ollama runs as a host systemd service, not in Kubernetes.

## Decision

### Prevention: Kernel headers meta-package

Install `linux-headers-amd64` on the Debian host. This meta-package ensures that when `unattended-upgrades` installs a new kernel, the matching headers are pulled automatically, allowing DKMS to rebuild the NVIDIA module without manual intervention.

### Prevention: Ollama keep-alive override

Create a systemd override at `/etc/systemd/system/ollama.service.d/override.conf` setting `OLLAMA_KEEP_ALIVE=-1`. This keeps models loaded in GPU memory permanently, avoiding cold-start latency. The override file survives Ollama package updates (which may replace the main service file).

### Detection: nvidia_gpu_exporter

Install `nvidia_gpu_exporter` (v1.4.1, `.deb` package) as a host systemd service on port 9835. Prometheus already had a scrape target configured for this port from a previous Windows-based setup — no Prometheus config changes were needed for the exporter itself.

An iptables rule was required to allow Minikube containers to reach the host on port 9835 (persisted via `iptables-persistent`). Ollama on port 11434 worked without this rule for reasons that remain unclear — both services bind to `0.0.0.0` identically.

### Detection: Grafana alert rules with Telegram notifications

Four Grafana-managed alert rules, provisioned declaratively via a ConfigMap mounted into `/etc/grafana/provisioning/alerting/`:

| Alert | Condition | Severity | For |
|-------|-----------|----------|-----|
| GPU Exporter Down | `up{job="nvidia-gpu"} == 0` | critical | 2m |
| AI Service Not Ready | `kube_pod_status_ready{pod=~"go-ai-service.*"} == 0` | critical | 3m |
| GPU Temperature High | `nvidia_smi_temperature_gpu > 85` | warning | 5m |
| GPU VRAM High | `nvidia_smi_memory_used_bytes / nvidia_smi_memory_total_bytes > 0.9` | warning | 10m |

Notifications go to a Telegram bot via Grafana's native Telegram contact point.

### Alternatives considered and rejected

**AlertManager deployment.** Grafana's built-in alerting handles our single notification channel without needing a separate AlertManager pod. If we add more notification channels or complex routing, AlertManager would become worthwhile.

**DCGM Exporter (NVIDIA's official tool).** Designed for data center GPU fleets with rich metrics (ECC errors, PCIe throughput, per-process usage). Overkill for a single-GPU dev server. Requires NVIDIA Container Toolkit and runs as a K8s DaemonSet with host device access. `nvidia_gpu_exporter` is a single binary wrapping `nvidia-smi` — simpler and sufficient.

**Direct Ollama Prometheus scrape target.** We initially added an Ollama scrape job targeting `/api/tags`. This failed because Ollama 0.20.7 doesn't expose a `/metrics` endpoint — `/api/tags` returns JSON that Prometheus can't parse as metrics, causing the target to report `down` even when Ollama is healthy. We also tried `OLLAMA_METRICS=true` as an environment variable but the feature doesn't exist in this version.

The Ollama health gap is covered indirectly: the ai-service readiness probe (`/ready`) checks Ollama connectivity by hitting `/api/tags` with a 2-second timeout. If Ollama is down, the probe fails, Kubernetes marks the pod not-ready, and the AI Service Not Ready alert fires.

**Pushover / ntfy.sh for notifications.** Both are viable alternatives to Telegram. Pushover costs $5 one-time. ntfy.sh is free and self-hostable. Telegram was chosen because Grafana has native support (built-in contact point type, no webhook plumbing) and Kyle already uses messaging apps.

**Environment variable / secret-based Telegram credentials.** We tried both `$__file{}` (file-based secret mount) and `$__env{}` (environment variable) for the Telegram chat ID. Both failed because Grafana's provisioning YAML parser unmarshals the all-numeric chat ID as a JSON number, but the Telegram integration expects a string. The credentials are inlined in the ConfigMap — the bot token can only send messages to Kyle's chat, so the security impact is minimal.

## Consequences

**Positive:**
- Today's exact failure scenario (kernel update → GPU driver loss → CPU fallback) would now trigger the GPU Exporter Down alert within 2 minutes
- Ollama going down triggers AI Service Not Ready within 3 minutes via the readiness probe chain
- Kernel header auto-install prevents the root cause from recurring
- Models stay permanently loaded in GPU memory, eliminating cold-start latency
- All alerting config is declarative (ConfigMaps), versioned in git, and survives pod restarts

**Trade-offs:**
- Telegram bot token is stored in plaintext in a ConfigMap (git-tracked). Acceptable risk given the token's limited capability, but should be revisited if the repo goes public.
- No direct Ollama health metric — detection is indirect through the ai-service readiness probe, adding ~1 minute of latency to Ollama-down detection.
- The iptables rule for port 9835 is a manual host-level dependency outside of version control. Documented in the plan but could drift.
- `OLLAMA_KEEP_ALIVE=-1` means models never unload, consuming ~17GB VRAM permanently. Fine for a dedicated inference server, but would need adjustment if other GPU workloads are added.
