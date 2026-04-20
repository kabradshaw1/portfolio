# Full-Stack Observability Design

## Context

Java services on the Debian server were getting OOM-killed with no alerting in place to catch it. The immediate fix (JVM heap caps + increased memory limits) is deployed, but it exposed significant observability gaps: no log aggregation, no K8s health alerts beyond GPU/AI-service, no application SLOs, no Kafka pipeline monitoring, and no way to correlate between metrics, logs, and traces.

This spec designs a comprehensive observability stack that:
1. Fills the operational gaps so issues like OOM kills are caught immediately
2. Adds the missing pillar (logs via Loki) to complete the metrics/logs/traces trifecta
3. Introduces application-level SLOs using the RED method
4. Monitors the Kafka streaming analytics pipeline
5. Connects all three pillars through a correlation dashboard
6. Includes learning documentation covering observability fundamentals

**Primary goal:** Portfolio showcase demonstrating distributed systems observability, with learning docs to build deep understanding for interviews.

---

## Section 1: Loki Log Aggregation

### Problem
Container logs are only accessible via `kubectl logs` over SSH. No centralized search, no persistence after pod restarts, no correlation with metrics or traces.

### Components

**Loki** (single-binary mode)
- Deployed as a StatefulSet in `monitoring` namespace
- PVC for log persistence (matching Prometheus's retention model)
- Single-binary mode is appropriate for a single-node Minikube cluster

**Promtail** (DaemonSet)
- Runs on every node, tails container logs from `/var/log/pods/`
- Adds Kubernetes labels: namespace, pod, container
- Parses structured JSON logs from Go services to extract `level`, `msg`, `traceID`
- Ships logs to Loki

**Grafana datasource**
- Add Loki as a datasource alongside Prometheus
- Configure derived fields to turn `traceID` values into clickable Jaeger links

### Why Loki
Loki indexes only labels (namespace, pod, level), not full text. This makes it lightweight for a single-node cluster. Built by the Grafana team, so correlation with Prometheus metrics is native. ELK would be overkill for this scale.

### Files
- `k8s/monitoring/statefulsets/loki.yml` — Loki StatefulSet
- `k8s/monitoring/daemonsets/promtail.yml` — Promtail DaemonSet
- `k8s/monitoring/configmaps/promtail-config.yml` — Promtail pipeline config
- `k8s/monitoring/configmaps/loki-config.yml` — Loki storage/retention config
- `k8s/monitoring/services/loki.yml` — Loki ClusterIP service
- `k8s/monitoring/configmaps/grafana-datasource.yml` — Add Loki datasource
- `k8s/monitoring/pvc/loki-data.yml` — Loki PVC
- `k8s/monitoring/kustomization.yaml` — Add new resources

---

## Section 2: Kubernetes Health Alerts

### Problem
Only 4 alert rules exist (GPU exporter down, AI service not ready, GPU temp, GPU VRAM). No alerts for OOM kills, pod restart storms, node pressure, or stuck deployments.

### New Alert Rules

Added to `k8s/monitoring/configmaps/grafana-alerting.yml` as a new "Kubernetes Health" alert group:

| Alert | PromQL | Severity | Pending |
|-------|--------|----------|---------|
| Container OOM Killed | `kube_pod_container_status_terminated_reason{reason="OOMKilled"} > 0` | critical | 0s |
| Pod Restart Storm | `increase(kube_pod_container_status_restarts_total[30m]) > 3` | warning | 5m |
| Container Memory High | `container_memory_working_set_bytes{container!=""} / on(pod,container,namespace) kube_pod_container_resource_limits{resource="memory"} > 0.85` | warning | 10m |
| Node Memory Pressure | `kube_node_status_condition{condition="MemoryPressure",status="true"} == 1` | critical | 2m |
| Node Disk Pressure | `kube_node_status_condition{condition="DiskPressure",status="true"} == 1` | critical | 2m |
| Deployment Replicas Unavailable | `kube_deployment_status_replicas_available < kube_deployment_spec_replicas` | warning | 5m |

All route to the existing Telegram contact point.

### Files
- `k8s/monitoring/configmaps/grafana-alerting.yml` — Add "Kubernetes Health" alert group

---

## Section 3: Application SLOs

### Problem
Infrastructure alerts tell you something broke. SLOs tell you the user experience is degrading before anything crashes. No application-level alerting exists today.

### Approach
Use the RED method (Rate, Errors, Duration) — the standard for request-driven services.

### SLO Definitions

| Service | SLI | SLO Target | Alert Threshold |
|---------|-----|------------|-----------------|
| Go AI service | HTTP 5xx rate | < 5% over 5m | > 5% for 5m |
| Go ecommerce | HTTP 5xx rate | < 2% over 5m | > 2% for 5m |
| Java gateway | HTTP 5xx rate | < 2% over 5m | > 2% for 5m |
| Go AI service | p95 latency | < 30s | > 30s for 5m |
| Go ecommerce | p95 latency | < 2s | > 2s for 5m |
| Java gateway | p95 latency | < 3s | > 3s for 5m |

These are percentage-over-window SLOs (not burn-rate). Simple and explainable — burn-rate can be layered on later.

### Metric Names
- **Go services:** `http_requests_total` (counter with `status` label), `http_request_duration_seconds_bucket` (histogram)
- **Java services:** `http_server_requests_seconds_count` (Actuator counter with `status` tag), `http_server_requests_seconds_bucket` (histogram)

SLO alert PromQL must use the correct metric name per service stack.

### Files
- `k8s/monitoring/configmaps/grafana-alerting.yml` — Add "Application SLOs" alert group

---

## Section 4: Kafka Consumer Lag Monitoring

### Problem
The ecommerce → Kafka → analytics-service pipeline is invisible. If the consumer falls behind or stops, events pile up silently.

### Approach
The `segmentio/kafka-go` reader exposes stats programmatically. Add Prometheus metrics to the analytics-service:

- `kafka_consumer_lag` — messages behind per topic/partition
- `kafka_consumer_messages_total` — total consumed per topic
- `kafka_consumer_errors_total` — read errors

### Alert

| Alert | Condition | Severity | Pending |
|-------|-----------|----------|---------|
| Kafka Consumer Lag High | `kafka_consumer_lag > 1000` | warning | 5m |

### Dashboard
Add a "Streaming Analytics" row to the Go services Grafana dashboard: consumer lag over time per topic, consumption rate, error rate.

### Files
- `go/analytics-service/internal/consumer/metrics.go` — Prometheus metrics registration
- `go/analytics-service/internal/consumer/consumer.go` — Export reader stats as metrics
- `k8s/monitoring/configmaps/grafana-alerting.yml` — Add Kafka lag alert
- `k8s/monitoring/configmaps/grafana-dashboards.yml` — Add streaming analytics panels to Go services dashboard

---

## Section 5: Correlation Dashboard

### Problem
Metrics, logs, and traces exist in separate views. The real value of observability is connecting them — seeing an error rate spike, clicking through to the logs, and following a traceID into the full request trace.

### Dashboard: "Observability Overview"

A single Grafana dashboard with three linked rows:

1. **Service Health (Prometheus)** — Error rate and p95 latency per service using SLO metrics. Clicking a time range filters the rows below.
2. **Logs (Loki)** — Filtered by namespace, showing error/warn logs for the selected window. Log lines with traceID are clickable links.
3. **Trace Links (Jaeger)** — Clicking a traceID from logs opens the Jaeger trace view showing the full request path, including Kafka message hops.

### How Correlation Works
- Promtail extracts `traceID` from structured JSON logs as a label
- Grafana derived fields on the Loki datasource turn traceID values into Jaeger datasource links
- This creates the metrics -> logs -> traces drill-down flow

### Files
- `k8s/monitoring/configmaps/grafana-dashboards.yml` — Add "Observability Overview" dashboard JSON
- `k8s/monitoring/configmaps/grafana-datasource.yml` — Add derived fields to Loki datasource config

---

## Section 6: Learning Documentation

### Location
`docs/adr/observability/` — markdown guides building from fundamentals to this implementation.

### Documents

1. **`01-three-pillars.md`** — The Three Pillars of Observability
   - Metrics, logs, traces: what each answers
   - When to reach for each one
   - Monitoring vs observability

2. **`02-prometheus-and-metrics.md`** — Metrics with Prometheus
   - Pull model, metric types (counter/gauge/histogram/summary)
   - PromQL with examples from this project's actual queries
   - How kube-state-metrics and node-exporter work

3. **`03-loki-and-logs.md`** — Log Aggregation with Loki
   - Why centralized logging matters in distributed systems
   - Loki's label indexing vs ELK's full-text indexing
   - LogQL basics with project examples
   - Structured logging and JSON log parsing

4. **`04-jaeger-and-traces.md`** — Distributed Tracing with Jaeger and OpenTelemetry
   - Spans, traces, context propagation
   - This project's OTel setup and Kafka header propagation
   - Request flow: gateway -> ecommerce -> Kafka -> analytics with trace context

5. **`05-alerting-and-slos.md`** — Alerting Philosophy and SLOs
   - Symptom-based vs cause-based alerting
   - RED method for request-driven services
   - SLO concepts: targets, SLIs, what breach means
   - This project's specific SLOs and rationale

6. **`06-correlation.md`** — Connecting the Pillars
   - The observability workflow: alert -> metrics -> logs -> traces
   - How traceID ties everything together
   - Correlation dashboard walkthrough
   - Kafka async observability challenges

---

## Verification

### Loki
- `kubectl get pods -n monitoring` — Loki and Promtail pods running
- Grafana Explore: query `{namespace="java-tasks"}` returns recent logs
- Grafana Explore: query `{namespace="go-ecommerce"} | json | level="error"` parses structured logs

### K8s Health Alerts
- Verify alerts appear in Grafana Alerting UI under "Kubernetes Health" group
- Test OOM alert: deploy a pod with artificially low memory limit, confirm Telegram notification

### Application SLOs
- Verify alerts appear under "Application SLOs" group
- Send requests to services, confirm metrics feed the alert rules

### Kafka Monitoring
- `kubectl exec -n go-ecommerce deploy/analytics-service -- wget -qO- localhost:8084/metrics | grep kafka_consumer_lag` — metric is exposed
- Check Go services dashboard has the new streaming analytics panels

### Correlation Dashboard
- Open "Observability Overview" dashboard
- Click a time range on the metrics panel, confirm logs panel filters
- Find a log line with traceID, click it, confirm Jaeger trace opens

### Learning Docs
- All 6 documents exist in `docs/adr/observability/`
- Each references actual project code, queries, and configurations
