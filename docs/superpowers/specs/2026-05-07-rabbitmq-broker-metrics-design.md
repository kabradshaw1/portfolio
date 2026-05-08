# RabbitMQ Broker Metrics Design

Date: 2026-05-07

## Goal

Expose RabbitMQ broker-level queue depth metrics to Prometheus so production and
QA queue backlogs are visible by virtual host and queue name. This is Phase 1 of
the remaining observability gaps roadmap and should provide the metric
foundation for a later saga DLQ depth alert.

## Problem

Spring AMQP and application metrics show client-side publisher/listener
behavior, but they do not expose current broker queue depth. The observability
stack can currently detect new saga DLQ events through
`increase(saga_dlq_messages_total[10m])`, but it cannot answer the operationally
different question: "Are messages sitting in the DLQ right now?"

Without broker queue metrics, a DLQ or hot queue can accumulate messages after a
short burst and remain invisible once the event-rate alert resolves.

## Current State

RabbitMQ runs as a single Kubernetes deployment in `java-tasks` using
`rabbitmq:3-management-alpine`. The broker already defines both production and
QA virtual hosts in `rabbitmq-definitions`:

- `/` for production Java task traffic.
- `qa` for QA Java task traffic.

The QA overlay deletes the RabbitMQ deployment and points its RabbitMQ service
back to `rabbitmq.java-tasks.svc.cluster.local` with an `ExternalName`, so
Prometheus only needs to scrape the real broker once. Production vs QA
separation should come from RabbitMQ's `vhost` metric label, not Kubernetes
namespace.

Prometheus already has Kubernetes pod discovery for annotated pods, but RabbitMQ
queue depth requires per-queue metrics. RabbitMQ's built-in Prometheus plugin
exposes a dedicated metrics port on `15692` and supports a detailed endpoint
that can return only selected metric families.

Reference: https://www.rabbitmq.com/docs/4.2/prometheus

## Decision

Use RabbitMQ's built-in `rabbitmq_prometheus` plugin instead of adding a
separate exporter sidecar.

The built-in plugin is the conservative choice here because:

- The current image already includes RabbitMQ management support.
- No new image, credentials, sidecar lifecycle, or management API polling path
  is needed.
- The `/metrics/detailed` endpoint can expose just queue depth and consumer
  count metrics without enabling broad per-object metrics on `/metrics`.
- The metrics naturally include RabbitMQ labels such as virtual host and queue.

## Design

### RabbitMQ plugin configuration

Add an `enabled_plugins` entry to the repo-managed RabbitMQ ConfigMap and mount
it into the broker container:

```erlang
[rabbitmq_management,rabbitmq_prometheus].
```

Keep `management.load_definitions = /etc/rabbitmq/definitions.json` in
`rabbitmq.conf` so existing user, permission, and vhost bootstrapping remains
unchanged.

### Kubernetes exposure

Expose the Prometheus plugin port from the existing broker pod and service:

- Container port: `15692`
- Service port: `15692`
- Suggested port name: `prometheus`

This keeps the endpoint reachable as:

```text
rabbitmq.java-tasks.svc.cluster.local:15692
```

### Prometheus scraping

Add a dedicated RabbitMQ scrape job instead of relying on generic pod
annotations. The job should scrape the detailed endpoint with only the metric
families needed for queue depth and consumer visibility:

```yaml
- job_name: "rabbitmq"
  metrics_path: /metrics/detailed
  params:
    family:
      - queue_coarse_metrics
      - queue_consumer_count
  static_configs:
    - targets:
        - rabbitmq.java-tasks.svc.cluster.local:15692
```

The expected metrics are:

- `rabbitmq_detailed_queue_messages`
- `rabbitmq_detailed_queue_messages_ready`
- `rabbitmq_detailed_queue_messages_unacked`
- `rabbitmq_detailed_queue_consumers`

The implementation should not set `prometheus.return_per_object_metrics = true`
globally unless the detailed endpoint proves unavailable in the deployed
RabbitMQ version.

## Files

| File | Change |
| --- | --- |
| `java/k8s/configmaps/rabbitmq-definitions.yml` | Add `enabled_plugins` data for management and Prometheus plugins |
| `java/k8s/deployments/rabbitmq.yml` | Mount `enabled_plugins` and expose container port `15692` |
| `java/k8s/services/rabbitmq.yml` | Add service port `15692` |
| `k8s/monitoring/configmaps/prometheus-config.yml` | Add dedicated RabbitMQ scrape job |

## Acceptance Criteria

- Prometheus target for RabbitMQ is up after deployment.
- Prometheus returns `rabbitmq_detailed_queue_messages` series.
- Queue metrics include a queue label that can identify saga queues and DLQs.
- Queue metrics include a vhost label that distinguishes `/` and `qa`.
- The saga DLQ queue is visible through a query filtered by queue name.
- No live RabbitMQ mutation is required outside repo-managed manifests.
- Existing RabbitMQ management UI and application connectivity continue to work.

## Verification

Local verification:

```bash
make preflight
```

If the full sweep is too broad for this manifest-only change, run the repo's
Kubernetes YAML validation path and leave live metric confirmation to the deploy
environment.

Post-deploy verification:

```promql
up{job="rabbitmq"}
rabbitmq_detailed_queue_messages
rabbitmq_detailed_queue_messages_ready
rabbitmq_detailed_queue_messages_unacked
rabbitmq_detailed_queue_consumers
```

Then record the observed `vhost` label value for the default vhost. RabbitMQ may
render `/` directly or in an escaped form depending on version and endpoint.
That observed value should be used by Phase 2 dashboards and alerts.

## Rollout

Deploy the manifest changes through the normal repo-managed Kubernetes path.
RabbitMQ will restart to pick up the plugin mount and port exposure. Since this
is a single in-cluster broker with no persistent volume in the current manifest,
rollout should be scheduled with awareness that transient Java RabbitMQ traffic
may reconnect during the pod restart.

After Prometheus starts scraping the new endpoint, proceed to Phase 2: add the
accumulated saga DLQ depth alert using the observed metric and label names.

## Out Of Scope

This phase does not add the saga DLQ depth alert, change RabbitMQ topology,
purge queues, replay DLQ messages, migrate RabbitMQ to a managed service, or
change application publisher/consumer reliability behavior.
