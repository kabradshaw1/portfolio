# RabbitMQ Broker Metrics and Saga DLQ Depth Alerting

- **Date:** 2026-05-07
- **Status:** Accepted

## Context

The ecommerce checkout saga uses RabbitMQ for asynchronous coordination, and
failed saga messages are routed to `ecommerce.saga.dlq`. The application already
emits `saga_dlq_messages_total`, and Grafana already has a
`saga-dlq-accumulating` alert based on
`increase(saga_dlq_messages_total[10m])`.

That event-rate alert is useful for detecting newly dead-lettered messages, but
it does not answer the operational question: "Are messages sitting in the DLQ
right now?" A short burst can move messages into the DLQ, the rate alert can
resolve, and the accumulated backlog can remain invisible.

Production and QA Java traffic share one RabbitMQ broker in the `java-tasks`
namespace. QA is isolated by RabbitMQ virtual host (`qa`), not by a separate
broker. Any broker-level metric strategy therefore has to preserve environment
distinction through RabbitMQ labels such as `vhost`.

## Decision

### 1. Use RabbitMQ's built-in Prometheus plugin

Enable RabbitMQ's built-in `rabbitmq_prometheus` plugin through the
repo-managed RabbitMQ ConfigMap instead of deploying a separate exporter
sidecar.

The broker pod and Service expose port `15692`, and Prometheus scrapes the
broker through a dedicated static job:

```yaml
- job_name: "rabbitmq"
  metrics_path: /metrics/detailed
  params:
    family:
      - queue_coarse_metrics
      - queue_consumer_count
  static_configs:
    - targets: ["rabbitmq.java-tasks.svc.cluster.local:15692"]
```

The detailed endpoint keeps the scrape focused on queue depth and consumer
visibility. The expected queue depth series is
`rabbitmq_detailed_queue_messages`, with RabbitMQ labels including `vhost` and
`queue`.

### 2. Keep event-rate and depth alerts separate

Keep the existing `saga-dlq-accumulating` alert unchanged:

```promql
increase(saga_dlq_messages_total[10m])
```

Add a second Grafana managed alert for accumulated queue depth:

```promql
max by (vhost, queue) (
  rabbitmq_detailed_queue_messages{queue="ecommerce.saga.dlq"}
)
```

The alert fires when the current depth is greater than zero for 5 minutes. It
groups by `vhost` and `queue`, so production and QA DLQ backlog alerts remain
distinguishable even though they come from the same broker.

### 3. Add focused dashboard visibility

Add a `Saga DLQ Depth` panel to the existing Go Services dashboard's
`Cache & RabbitMQ` row. The panel uses the same
`rabbitmq_detailed_queue_messages{queue="ecommerce.saga.dlq"}` metric and
legends series by `vhost` and `queue`.

The dashboard source of truth remains
`monitoring/grafana/dashboards/go-services.json`; the Kubernetes ConfigMap is
regenerated with `make grafana-sync`.

## Consequences

**Positive:**

- Prometheus can now see RabbitMQ queue depth without a new exporter image,
  sidecar lifecycle, or management API credentials.
- Current saga DLQ backlog is detectable even after the application event-rate
  alert resolves.
- Existing DLQ event-rate alert still detects fresh dead-letter activity.
- QA and production backlog are separated by RabbitMQ `vhost` label while
  continuing to share the same broker.
- The dashboard now shows the current saga DLQ depth in the same section as
  existing RabbitMQ publish metrics.

**Trade-offs:**

- RabbitMQ must restart during deployment to pick up the enabled plugin file and
  exposed port.
- Queue depth alerting depends on the RabbitMQ detailed metrics endpoint and
  the `rabbitmq_detailed_queue_messages` metric name. If a future RabbitMQ
  version changes the detailed endpoint shape, the alert and dashboard queries
  need to be updated together.
- A nonempty DLQ during intentional replay or manual testing can trigger the
  depth alert after 5 minutes. That is acceptable because a DLQ with messages
  waiting is still an operational state that should be visible.

**Deferred:**

- No live RabbitMQ, Kubernetes, database, secret, or queue mutation is part of
  this decision. Rollout happens through the repo-managed manifests.
- This does not add automated DLQ remediation. Inspection and replay remain
  separate operational workflows.
