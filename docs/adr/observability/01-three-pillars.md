# The Three Pillars of Observability

## Metrics, Logs, and Traces

Observability in distributed systems rests on three complementary signal types: metrics, logs, and traces. Each answers a different question, and understanding when to reach for which one is fundamental to operating production systems.

**Metrics** are numeric measurements aggregated over time. In this project, Prometheus scrapes metrics from every service: `http_requests_total` counts every HTTP request (a counter that only goes up), `kafka_consumer_lag` tracks how far behind the analytics consumer is (a gauge that fluctuates), and `go_goroutines` shows concurrency pressure in Go services. Metrics are cheap to store, fast to query, and the foundation of alerting.

**Logs** are discrete events with context -- an error message, a stack trace, the input that caused a failure. This project's Go services emit structured JSON logs with consistent fields (`level`, `msg`, `traceID`) which Promtail ships to Loki. A well-formed JSON log line can be filtered with `{namespace="go-ecommerce"} | json | level="error"` in LogQL, while unstructured logs require fragile regex matching.

**Traces** are the distributed equivalent of a stack trace. A single trace represents one end-to-end operation decomposed into spans -- individual units of work with start times, durations, and parent-child relationships. When a request enters the ecommerce service, passes through Redis, publishes to Kafka, and gets consumed by analytics, OpenTelemetry connects all operations into a single trace visible in Jaeger.

## Monitoring vs Observability

Monitoring asks predefined questions: "Is the service up? Is error rate acceptable?" You set thresholds, and dashboards go green or red. Observability is the ability to ask new questions you did not anticipate. When a user reports that checkout is slow but only on Tuesdays, you need to slice metrics by time, grep logs for the specific flow, and trace requests through the service chain. You could not have written an alert for that scenario in advance.

## When to Reach for Each Pillar

The practical workflow follows a consistent pattern. Metrics are for detection: a Grafana alert fires because `rate(http_requests_total{status=~"5.."}[5m])` spiked. You know something is wrong, but not what. Logs are for investigation: filter Loki by namespace and error level to find the actual error messages. Traces are for distributed path analysis: when the error message says "upstream timeout," the trace shows which upstream, how long it waited, and what that upstream was doing at the time.

## Limitations of the Model

The "three pillars" framing is useful but imperfect. The pillars overlap: a metric can encode the same information as a count of log lines, and a trace is essentially a structured log with parent references. The real power is correlation -- connecting a metric spike to the log lines that caused it to the traces that show where it happened. Grafana serves as the unified UI because it displays Prometheus metrics, Loki logs, and Jaeger traces in a single dashboard, with derived fields that link traceIDs in log lines directly to trace views.
