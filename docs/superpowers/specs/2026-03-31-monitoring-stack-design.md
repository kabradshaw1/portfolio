# Monitoring Stack Design

## Overview

Add Prometheus + Grafana monitoring to the Windows machine (PC@100.79.113.84) to track system resources (CPU, RAM), GPU usage (RTX 3090), and Docker container health. Grafana is exposed publicly via Cloudflare Tunnel for portfolio use.

## Goals

- Monitor host CPU and RAM usage
- Monitor NVIDIA GPU utilization, VRAM, and temperature
- Monitor all Docker container health (CPU, memory, network, uptime)
- Provide a polished Grafana dashboard suitable for embedding in a portfolio
- Public read-only access via `grafana.kylebradshaw.dev`

## Architecture

### New Services

| Service | Image | Port | Runs In |
|---------|-------|------|---------|
| Prometheus | `prom/prometheus` | 9090 | Docker |
| Grafana | `grafana/grafana` | 3000 | Docker |
| cAdvisor | `gcr.io/cadvisor/cadvisor` | 8080 | Docker |
| nvidia_gpu_exporter | `utkuozdemir/nvidia_gpu_exporter` | 9835 | Docker |
| windows_exporter | Native binary | 9182 | Windows service |

### Data Flow

```
windows_exporter (host:9182) ──┐
nvidia_gpu_exporter (:9835) ───┤
cAdvisor (:8080) ──────────────┼──→ Prometheus (:9090) ──→ Grafana (:3000) ──→ Cloudflare Tunnel
ingestion /health (:8001) ─────┤
chat /health (:8002) ──────────┘
```

### Why These Tools

- **windows_exporter**: The standard `node_exporter` is Linux-only. `windows_exporter` is the official Prometheus exporter for Windows host metrics. Must run as a native Windows service (not Docker) because it needs direct WMI/PDH access.
- **nvidia_gpu_exporter** (`utkuozdemir/nvidia_gpu_exporter`): Lightweight container that shells out to `nvidia-smi` on the host. Requires `--runtime=nvidia` or device mapping to access the GPU.
- **cAdvisor**: Google's container advisor. Provides per-container CPU, memory, network I/O, and disk metrics via Prometheus-compatible `/metrics` endpoint.
- **Prometheus**: Industry-standard metrics collection and storage. Pull-based scraping model fits our exporter setup perfectly.
- **Grafana**: Industry-standard dashboards. Supports anonymous read-only access for public viewing.

## Configuration

### Prometheus (`monitoring/prometheus.yml`)

Scrape targets:
- `windows_exporter` at `host.docker.internal:9182` — host CPU, RAM, disk
- `nvidia_gpu_exporter` at `nvidia-gpu-exporter:9835` — GPU metrics
- `cadvisor` at `cadvisor:8080` — container metrics
- `ingestion` at `ingestion:8000` — app health (if `/metrics` endpoint exists, otherwise skip)
- `chat` at `chat:8000` — app health (if `/metrics` endpoint exists, otherwise skip)

Scrape interval: 15s (default, reasonable for this workload).

### Grafana

**Authentication:**
- Anonymous access enabled, org role = Viewer (read-only)
- Admin user for dashboard editing, password set via `GF_SECURITY_ADMIN_PASSWORD` env var

**Provisioning (`monitoring/grafana/provisioning/`):**
- `datasources/prometheus.yml` — auto-configures Prometheus as default datasource
- `dashboards/dashboard.yml` — points to provisioned dashboard JSON
- `dashboards/system-overview.json` — the main dashboard

### Cloudflare Tunnel

Add route to existing cloudflared config on the Windows machine:
- `grafana.kylebradshaw.dev` → `http://localhost:3000`

This follows the same pattern as the existing `api-chat` and `api-ingestion` tunnel routes.

## Grafana Dashboard: System Overview

### System Row
| Panel | Metric Source | Query Pattern |
|-------|--------------|---------------|
| CPU Usage % | windows_exporter | `windows_cpu_time_total` (calculate idle inverse) |
| RAM Usage | windows_exporter | `windows_os_physical_memory_free_bytes` vs `windows_cs_physical_memory_bytes` |
| Disk Usage | windows_exporter | `windows_logical_disk_free_bytes` |

### GPU Row
| Panel | Metric Source | Query Pattern |
|-------|--------------|---------------|
| GPU Utilization % | nvidia_gpu_exporter | `nvidia_gpu_duty_cycle` |
| VRAM Usage | nvidia_gpu_exporter | `nvidia_gpu_memory_used_bytes` / `nvidia_gpu_memory_total_bytes` |
| GPU Temperature | nvidia_gpu_exporter | `nvidia_gpu_temperature_celsius` |

### Containers Row
| Panel | Metric Source | Query Pattern |
|-------|--------------|---------------|
| Per-container CPU % | cAdvisor | `container_cpu_usage_seconds_total` (rate) |
| Per-container Memory | cAdvisor | `container_memory_usage_bytes` |
| Container Uptime | cAdvisor | `container_start_time_seconds` |
| Network I/O | cAdvisor | `container_network_receive_bytes_total` / `transmit` |

## Docker Compose Changes

Add to `docker-compose.yml`:

```yaml
prometheus:
  image: prom/prometheus:latest
  ports:
    - "9090:9090"
  volumes:
    - prometheus_data:/prometheus
    - ./monitoring/prometheus.yml:/etc/prometheus/prometheus.yml:ro
  extra_hosts:
    - "host.docker.internal:host-gateway"

grafana:
  image: grafana/grafana:latest
  ports:
    - "3000:3000"
  volumes:
    - grafana_data:/var/lib/grafana
    - ./monitoring/grafana/provisioning:/etc/grafana/provisioning:ro
    - ./monitoring/grafana/dashboards:/var/lib/grafana/dashboards:ro
  environment:
    - GF_SECURITY_ADMIN_PASSWORD=${GRAFANA_ADMIN_PASSWORD:-admin}
    - GF_AUTH_ANONYMOUS_ENABLED=true
    - GF_AUTH_ANONYMOUS_ORG_ROLE=Viewer
    - GF_SERVER_ROOT_URL=https://grafana.kylebradshaw.dev
  depends_on:
    - prometheus

cadvisor:
  image: gcr.io/cadvisor/cadvisor:latest
  ports:
    - "8080:8080"
  volumes:
    - //var/run/docker.sock:/var/run/docker.sock:ro
  privileged: true

nvidia-gpu-exporter:
  image: utkuozdemir/nvidia_gpu_exporter:1.2.1
  ports:
    - "9835:9835"
  deploy:
    resources:
      reservations:
        devices:
          - driver: nvidia
            count: 1
            capabilities: [gpu]
```

Named volumes to add:
```yaml
volumes:
  qdrant_data:
  prometheus_data:
  grafana_data:
```

## File Structure

```
monitoring/
├── prometheus.yml
└── grafana/
    ├── provisioning/
    │   ├── datasources/
    │   │   └── prometheus.yml
    │   └── dashboards/
    │       └── dashboard.yml
    └── dashboards/
        └── system-overview.json
```

## windows_exporter Setup

Installed separately on the Windows host (not in Docker):

1. Download `windows_exporter` MSI from GitHub releases
2. Install as Windows service with default collectors (cpu, memory, logical_disk, net, os, cs)
3. Verify at `http://localhost:9182/metrics`

This is the one component that can't run in Docker because it needs direct access to Windows performance counters (WMI/PDH).

## Environment Variables

Add to `.env`:
```
GRAFANA_ADMIN_PASSWORD=<secure-password>
```

Add to `.env.example`:
```
# Grafana
GRAFANA_ADMIN_PASSWORD=changeme
```

## Security

- Grafana anonymous access is read-only (Viewer role) — cannot edit dashboards or admin settings
- Prometheus is not exposed via Cloudflare Tunnel — only accessible from Docker network and localhost
- cAdvisor, nvidia_gpu_exporter, and windows_exporter are only accessible on localhost/Docker network
- Grafana admin password is stored in `.env` (already gitignored)

## Testing

1. `docker compose up` — all new services start healthy
2. `http://localhost:9090/targets` — Prometheus shows all targets as UP
3. `http://localhost:3000` — Grafana loads, dashboard visible without login
4. `https://grafana.kylebradshaw.dev` — public access works via Cloudflare Tunnel
5. Dashboard panels show live data for CPU, RAM, GPU, and containers
