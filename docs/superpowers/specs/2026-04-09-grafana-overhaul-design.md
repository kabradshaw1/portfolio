# Grafana Observability Overhaul — Design Spec

**Date:** 2026-04-09
**Status:** Approved, pending implementation plan
**Scope:** Fix broken GPU + services panels, instrument all application services with real Prometheus metrics, rebuild dashboards around an AI-engineering narrative.

## Problem

The current Grafana stack has degraded into a mostly-dead dashboard:

1. **GPU metrics are missing.** `nvidia_gpu_exporter` is no longer running on the Windows host (`PC@100.79.113.84`). Nothing is listening on `:9835`. `nvidia-smi` itself works (RTX 3090, Ollama using ~17 GiB VRAM), so the issue is purely that the exporter process/service is gone — unrelated to GeForce Experience being disabled.
2. **The "Services" panel is dead.** It was built against docker cAdvisor from the old Compose stack; the current k8s Prometheus has no cAdvisor target and no kube-state-metrics, so the panel shows nothing.
3. **No real service metrics are being collected.** The k8s Prometheus scrape config points at `/health` and `/actuator/health` endpoints that return JSON health payloads, not Prometheus exposition format. Every application "scrape" is silently useless.
4. **No storage retention.** Prometheus uses `emptyDir`; history is lost on every pod restart.

## Goals

- Restore GPU visibility (VRAM, utilization, temp, power).
- Replace the broken Services panel with accurate Kubernetes pod/container views.
- Instrument every application service with real Prometheus metrics, with special focus on the AI pipeline and the Go ai-service + ecommerce-service.
- Rebuild dashboards to tell a Gen-AI-engineering story: GPU load correlated with Ollama request rate, RAG latency broken down by stage, agent loop metrics, business metrics for the ecommerce service.
- Persist metric history across pod restarts (15-day retention).

## Non-goals

- Grafana Cloud / any external remote-write.
- Custom business metrics on the Java stack (RED + JVM defaults only).
- Custom metrics on the Go auth-service beyond RED.
- Alerting rules / Alertmanager. (Future work.)
- Log aggregation (Loki, etc.). (Future work.)

## Architecture

### New and modified components

**Windows host** (`PC@100.79.113.84`):
- `nvidia_gpu_exporter` (utkuozdemir) — installed fresh, run as a Windows service via NSSM, auto-start on boot, listening on `:9835`.
- `windows_exporter` — unchanged, still on `:9182`.

**`monitoring` namespace (Minikube):**
- **Prometheus** — upgraded: 10 GiB PVC, 15-day retention, 8 GiB size cap, Kubernetes pod service-discovery, RBAC ClusterRole/ServiceAccount.
- **kube-state-metrics** — new deployment with its own ServiceAccount + ClusterRole (standard upstream manifest).
- **node-exporter** — new DaemonSet.
- **Grafana** — infra unchanged; new dashboards provisioned from ConfigMaps in git.

**`java-tasks` namespace:**
- **rabbitmq_exporter** — new deployment, scraped by Prometheus, exposes `rabbitmq_queue_messages{queue}` etc.

**Application services:** every service gains a `/metrics` endpoint and pod annotations `prometheus.io/scrape: "true"`, `prometheus.io/port`, `prometheus.io/path`.

### Data flow

1. Prometheus (in `monitoring` ns) discovers pods via Kubernetes SD and keeps only those annotated `prometheus.io/scrape: "true"`.
2. Windows exporters remain static scrape targets via `host.minikube.internal:9182` and `host.minikube.internal:9835`.
3. Prometheus writes to its PVC at `/prometheus`, 15-day retention.
4. Grafana reads from Prometheus as its single datasource. Dashboards provisioned from `monitoring/grafana/dashboards/*.json` via ConfigMap.

## Scrape configuration

Replace all hand-written in-cluster `static_configs` with a single pod-SD job:

```yaml
- job_name: 'k8s-pods'
  kubernetes_sd_configs: [{ role: pod }]
  relabel_configs:
    - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
      action: keep
      regex: "true"
    - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_path]
      action: replace
      target_label: __metrics_path__
      regex: (.+)
    - source_labels: [__address__, __meta_kubernetes_pod_annotation_prometheus_io_port]
      action: replace
      regex: ([^:]+)(?::\d+)?;(\d+)
      replacement: $1:$2
      target_label: __address__
    - source_labels: [__meta_kubernetes_namespace]
      target_label: namespace
    - source_labels: [__meta_kubernetes_pod_name]
      target_label: pod
    - source_labels: [__meta_kubernetes_pod_label_app]
      target_label: service
```

**Static jobs retained:** `prometheus` (self), `windows` (`host.minikube.internal:9182`), `nvidia-gpu` (`host.minikube.internal:9835`).

**Additional k8s SD jobs:** `kube-state-metrics`, `node-exporter` — each as a dedicated job for clearer labeling.

## Metric catalog

### Python services (chat, ingestion, debug) — level B

**Free via `prometheus-fastapi-instrumentator`:** `http_requests_total`, `http_request_duration_seconds`, `http_requests_in_progress`, `process_*`, `python_gc_*`.

**Custom (shared `metrics.py` helper):**
- `ollama_request_duration_seconds{service, model, operation}` — histogram wrapping every Ollama call.
- `ollama_tokens_total{service, model, kind=prompt|completion}` — counter fed from the Ollama response `prompt_eval_count` / `eval_count`.
- `ollama_eval_duration_seconds{service, model}` — histogram fed from `eval_duration` (nanoseconds → seconds).
- `embedding_duration_seconds{service, model}` — histogram (ingestion + chat).
- `qdrant_search_duration_seconds{collection}` — histogram.
- `qdrant_search_results{collection}` — histogram of hit counts.
- `rag_pipeline_duration_seconds{stage=retrieve|build_prompt|generate}` — histogram (chat service).
- `rag_pipeline_errors_total{stage}` — counter.

### Java services (gateway, task, activity, notification) — level B

**Free via `micrometer-registry-prometheus` + actuator at `/actuator/prometheus`:** `http_server_requests_seconds_*` (RED), `jvm_memory_*`, `jvm_gc_*`, `jvm_threads_*`, `hikaricp_connections_*`, `process_cpu_usage`.

No custom metrics.

### Go services (auth, ecommerce, ai-service) — level C

**Shared `internal/metrics` package provides:**
- Runtime via `collectors.NewGoCollector()` + `NewProcessCollector()`.
- RED middleware: `http_requests_total{service, method, path, status}` and `http_request_duration_seconds{service, method, path}`.

**ecommerce-service business metrics:**
- `ecommerce_cart_items_added_total{product_id}`
- `ecommerce_orders_placed_total{status}`
- `ecommerce_order_value_dollars` (histogram)
- `ecommerce_product_views_total{product_id}`
- `ecommerce_cache_operations_total{operation, result=hit|miss}`
- `ecommerce_rabbitmq_publish_total{queue, result}`

**ai-service agent metrics:**
- `agent_loop_iterations{service}` (histogram)
- `agent_tool_calls_total{tool, result=success|error}`
- `agent_tool_duration_seconds{tool}` (histogram)
- `ollama_request_duration_seconds{service, model, operation}` — same metric name as Python (shared convention for cross-stack panels).
- `ollama_tokens_total{service, model, kind}` — same.
- `ollama_eval_duration_seconds{service, model}` — same.

**auth-service:** RED + runtime only.

### Infrastructure
- **nvidia_gpu_exporter:** `nvidia_smi_memory_used_bytes`, `nvidia_smi_memory_total_bytes`, `nvidia_smi_utilization_gpu_ratio`, `nvidia_smi_temperature_gpu`, `nvidia_smi_power_draw_watts`.
- **windows_exporter:** CPU, memory, disk, network.
- **kube-state-metrics:** `kube_pod_status_phase`, `kube_pod_container_status_restarts_total`, `kube_deployment_status_replicas_available`, etc.
- **node-exporter:** node-level CPU/memory/disk/network.
- **rabbitmq_exporter:** `rabbitmq_queue_messages`, `rabbitmq_queue_messages_ready`, `rabbitmq_queue_consumers`.

## Dashboards

All provisioned from `monitoring/grafana/dashboards/*.json` via ConfigMap. Four dashboards:

### 1. `system-overview.json` (landing page)

Rewritten from scratch. Rows:
- **KPI strip:** GPU VRAM used/total, GPU util %, running pods, RAG p95 latency, total req/s across all services, global error rate %.
- **GPU row:** VRAM timeseries, GPU util %, GPU temp + power.
- **Kubernetes row:** pod count by namespace (stacked), pod restarts (1h), CPU/mem by namespace.
- **AI pipeline row:** Ollama tokens/sec by model, RAG E2E p50/p95, Qdrant search p95.
- **Per-stack RED row:** three side-by-side panels (Python / Java / Go) showing req/s, error %, p95 latency.
- **Dashboard links** to AI Pipeline, Go Services, Kubernetes.

The old broken "Services" panel is deleted.

### 2. `ai-pipeline.json`

Variables: `$service` (chat/debug/ai-service), `$model`.

- Ollama request rate, latency histogram (p50/p95/p99), tokens/sec, prompt vs completion tokens stacked, eval_duration histogram — all filterable by model.
- Embedding latency (nomic-embed-text).
- Qdrant search latency + hit count distribution.
- RAG end-to-end latency broken down by stage (retrieve → prompt-build → generate).
- Agent loop metrics: tool calls by tool name, tool latency, avg tools per request, agent loop iterations histogram.
- GPU VRAM overlay on the same time axis as Ollama request rate.

### 3. `go-services.json`

Variable: `$service`.

- RED triplet per service.
- Go runtime: goroutines, heap, GC pause, mem stats.
- ecommerce business metrics: cart adds/min, orders/min, cart→order conversion %, top-10 products viewed, RabbitMQ queue depth (order-worker).
- ai-service agent metrics: drill-downs of the ai-pipeline tool-call panels.

### 4. `kubernetes.json`

Community dashboard imported as-is (grafana.com `kube-state-metrics-v2`, pinned version in the ConfigMap). No hand-editing.

**Java:** no dedicated dashboard; represented only by the per-stack row on Overview.

## Storage & RBAC

- New `ClusterRole` granting Prometheus `get/list/watch` on `pods`, `services`, `endpoints`, `nodes`, `nodes/metrics`.
- New `ServiceAccount prometheus` in `monitoring` ns, bound to the ClusterRole.
- New `PersistentVolumeClaim prometheus-data` — 10 GiB, default storage class.
- Prometheus args: `--storage.tsdb.retention.time=15d`, `--storage.tsdb.retention.size=8GB`. Mount PVC at `/prometheus`.
- `kube-state-metrics`: its own ServiceAccount + ClusterRole (standard upstream manifest).

## Docker Compose parity

Per CLAUDE.md's compose-smoke realism rule, every `/metrics` endpoint and annotation change lands in **both** `docker-compose.yml` and `k8s/ai-services/`. Python services expose `/metrics` in both stacks. Compose-smoke asserts `GET /metrics` → 200 with `http_requests_total` present so drift fails CI.

## Testing

- **Unit:** shared metrics helpers (Python `metrics.py`, Go `internal/metrics`) get unit tests asserting counter increments and histogram observations.
- **Integration — Python:** pytest assertions that `/metrics` returns 200 with expected metric names.
- **Integration — Go:** `metrics_test.go` per service hitting the handler and inspecting exposition output.
- **Integration — Java:** assert `/actuator/prometheus` returns 200 in existing Spring tests.
- **Smoke (post-deploy):** smoke script queries `prometheus:9090/api/v1/query?query=up` and asserts every expected job is `up==1`.
- **GPU exporter verification:** SSH step running `curl localhost:9835/metrics | grep nvidia_smi_memory_used_bytes`.

## Rollout

Each stage is an independently mergeable feature branch off `main`.

1. **Stage A — GPU:** Install `nvidia_gpu_exporter` on Windows via NSSM. Verify existing Prometheus starts scraping VRAM. No code change to repo beyond documenting the install in `monitoring/README.md`.
2. **Stage B — Kubernetes visibility:** Add Prometheus PVC, RBAC, ServiceAccount, k8s pod-SD scrape config. Deploy kube-state-metrics + node-exporter. Import community Kubernetes dashboard. Replace the broken Services panel (interim fix on the old overview dashboard until Stage D).
3. **Stage C1 — Python instrumentation:** Add `prometheus-fastapi-instrumentator`, shared `metrics.py`, custom Ollama/embedding/Qdrant/RAG metrics. Update compose + k8s annotations. Ship.
4. **Stage C2 — Java instrumentation:** Add `micrometer-registry-prometheus`, expose `/actuator/prometheus`, update k8s annotations. Ship.
5. **Stage C3 — Go instrumentation:** Add shared `internal/metrics`, RED middleware, runtime collectors, ai-service agent metrics, ecommerce business metrics, `rabbitmq_exporter` deployment. Ship.
6. **Stage D — Dashboards:** Build `system-overview.json` v2, `ai-pipeline.json`, `go-services.json`. Provision via ConfigMap. Delete old dashboard JSON. Ship.

## Open questions

None. All six clarifying questions resolved during brainstorming.
