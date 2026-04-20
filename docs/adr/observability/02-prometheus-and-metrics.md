# Metrics with Prometheus

## Pull vs Push

Prometheus uses a pull model: it scrapes HTTP endpoints on a schedule rather than waiting for services to push data. This design simplifies the service side (just expose `/metrics`, no client library configuration for destinations), eliminates the need for push infrastructure (message queues, aggregation servers), and makes service discovery the central concern rather than routing. If a target disappears, Prometheus notices immediately because the scrape fails -- you get a free "is it up?" signal from the `up` metric.

This project's Prometheus scrapes every 15 seconds. Some targets use static configuration -- the NVIDIA GPU exporter at `host.minikube.internal:9835` and the node-exporter at port 9100 are fixed addresses. Application services use Kubernetes service discovery: the `k8s-pods` job watches pod annotations, and any pod with `prometheus.io/scrape: "true"` gets scraped automatically. The port and path come from `prometheus.io/port` and `prometheus.io/path` annotations on the pod spec. When a new Go service deploys, it starts getting scraped with zero Prometheus configuration changes.

## The Four Metric Types

**Counters** only go up. `http_requests_total` is the canonical example -- it counts every HTTP request since the process started. The raw value is rarely useful; you apply `rate()` to get requests per second. The alert rule `sum(rate(http_requests_total{service="go-ecommerce-service",status=~"5.."}[5m])) / sum(rate(http_requests_total{service="go-ecommerce-service"}[5m])) * 100` calculates the 5xx error percentage over a 5-minute window. Counters reset to zero when the process restarts, and `rate()` handles this automatically.

**Gauges** can go up or down. `kafka_consumer_lag` is a gauge: the number of unconsumed messages fluctuates as producers write and consumers read. `go_goroutines` is another -- it reflects the current goroutine count, which changes constantly. Unlike counters, gauges are meaningful as raw values. The alert for Kafka consumer lag simply checks `kafka_consumer_lag > 1000`.

**Histograms** pre-bucket observations into configurable ranges. `http_request_duration_seconds_bucket` records how many requests fell into each latency bucket (e.g., under 100ms, under 250ms, under 500ms). The key function is `histogram_quantile()`, which estimates percentiles from bucket data. This project's AI service latency alert uses:

```promql
histogram_quantile(0.95,
  sum(rate(http_request_duration_seconds_bucket{service="go-ai-service"}[5m])) by (le)
)
```

This calculates the p95 latency -- the threshold below which 95% of requests complete. The `le` label (less-than-or-equal) identifies the bucket boundaries.

**Summaries** calculate quantiles on the client side. They are almost never the right choice because they cannot be aggregated across instances. If you have three replicas each reporting their own p99, you cannot combine them into a meaningful cluster-wide p99. Histograms can, because the raw bucket counts are additive. Use histograms.

## Infrastructure Metrics

Beyond application metrics, two exporters provide infrastructure visibility. **kube-state-metrics** queries the Kubernetes API to expose object-level metrics: `kube_pod_status_ready` (is the pod serving traffic?), `kube_deployment_spec_replicas` minus `kube_deployment_status_replicas_available` (are any replicas missing?), and `kube_pod_container_status_terminated_reason{reason="OOMKilled"}` (did a container run out of memory?). These power the Kubernetes Health alert group. **node-exporter** exposes machine-level metrics from the Debian server: CPU utilization, memory usage, disk I/O, and network throughput. Together, they answer infrastructure questions that application metrics cannot -- the application does not know it was OOM-killed, but kube-state-metrics does.

## PromQL in Practice

The queries in this project's alert rules demonstrate the core PromQL patterns. `rate()` over a counter gives per-second throughput. `increase()` gives the total change in a window -- `increase(kube_pod_container_status_restarts_total[30m]) > 3` detects restart storms. Division of two `rate()` calls gives a ratio (error rate). `histogram_quantile()` with `sum by (le)` gives latency percentiles. These four patterns cover the vast majority of real-world Prometheus usage.
