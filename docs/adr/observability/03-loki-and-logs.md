# Log Aggregation with Loki

## Why Centralized Logging Matters

The motivation for Loki was concrete: when a service got OOM-killed on the Debian server, diagnosing it required SSH-ing in and running `kubectl logs`, hoping the pod had not been rescheduled (losing its logs). With 12+ services across multiple namespaces, checking logs one pod at a time is not sustainable. Loki centralizes all logs into a single queryable store accessible from Grafana alongside metrics and traces.

## Architecture

Loki's full architecture has three components: distributors accept log streams, ingesters batch and compress them, and a storage backend persists chunks. This project uses single-binary mode, collapsing all three into one process with `replication_factor: 1`, in-memory ring, and filesystem storage. A 7-day retention period (`retention_period: 168h`) keeps disk usage bounded.

## Label-Based Indexing

Loki's fundamental design tradeoff sets it apart from Elasticsearch/ELK. ELK indexes every word in every log line, building an inverted index that makes full-text search fast but storage and compute expensive. Loki indexes only the labels attached to each log stream -- `namespace`, `pod`, `app`, `level` -- and stores the log content as compressed chunks. Searching within chunks requires decompressing and scanning (essentially grep), which is slower for ad-hoc text searches but dramatically cheaper in storage and memory. For a single-node Minikube cluster with limited resources, Loki's approach is the pragmatic choice.

## LogQL

LogQL queries start with a stream selector and optionally add pipeline stages. The simplest query selects by labels:

```logql
{namespace="go-ecommerce"}
```

This returns all log lines from the go-ecommerce namespace. Adding a JSON parsing stage and a filter narrows the results:

```logql
{namespace="go-ecommerce"} | json | level="error"
```

The `| json` stage parses each line as JSON and extracts fields. The `| level="error"` stage filters to only error-level entries. LogQL also supports metric queries, turning logs into time series:

```logql
rate({namespace="go-ecommerce"} | json | level="error" [5m])
```

This calculates the per-second rate of error logs over a 5-minute window. The ability to derive metrics from logs means you do not need a Prometheus counter for every error path if the error is already logged.

## Promtail

Promtail is the agent that discovers, tails, and ships logs to Loki. It runs as a DaemonSet (one instance per node) and mounts `/var/log/pods` as a hostPath volume, giving it access to every container's log files. Kubernetes service discovery (`kubernetes_sd_configs` with `role: pod`) tells it which pods exist and provides metadata for labeling.

The pipeline stages in this project's Promtail configuration handle log parsing in sequence. First, `cri: {}` strips the CRI container runtime's timestamp and stream prefix from each line. Then `json` extracts the `level`, `msg`, and `traceID` fields from the JSON payload. The `labels` stage promotes `level` to a Loki label, making it indexable (so `| level="error"` is a fast label filter, not a slow content scan). Finally, `output` replaces the raw log line with just the `msg` field, reducing storage and improving readability.

## Structured Logging

The pipeline stages above only work if log output is consistently structured. All Go services use `slog` with a JSON handler and all Java services use Logback's `JsonLayout`. Consistent field names (`level`, `msg`, `traceID`) across services mean one Promtail configuration handles all services identically.
