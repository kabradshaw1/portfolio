# Connecting the Pillars

## The Observability Workflow

The real value of an observability stack is moving between metrics, logs, and traces in a single investigation. Detection (metrics trigger an alert), investigation (logs reveal error details), and root cause analysis (traces show the distributed path) form the canonical workflow.

## A Concrete Scenario

A Telegram notification arrives: "Go ecommerce service error rate is above 2%." You open the Observability Overview dashboard and see the spike in the metrics row. Scrolling to the logs row, you filter:

```logql
{namespace="go-ecommerce"} | json | level="error"
```

The logs show repeated `"msg":"redis timeout","traceID":"a1b2c3d4e5f6..."`. Clicking the traceID opens the trace in Jaeger: root span is `POST /orders` (5.2s total), PostgreSQL query succeeds (12ms), Redis cache write times out (5.1s), and the Kafka publish never executes because the handler returned early.

Checking the Kubernetes Health section reveals the Redis pod crossed 85% memory and was OOM-killed 15 minutes ago. The restart caused connection timeouts until the new pod was ready. Root cause chain: Redis memory pressure caused OOM kill, causing timeouts, causing the error rate spike.

## How Correlation Works Technically

The link from logs to traces requires deliberate plumbing. The `LogHandler` in `go/pkg/tracing/logging.go` wraps `slog.Handler` and adds a `traceID` attribute when a span is active:

```go
sc := trace.SpanContextFromContext(ctx)
if sc.HasTraceID() {
    r.AddAttrs(slog.String("traceID", sc.TraceID().String()))
}
```

Promtail extracts the traceID from JSON and ships it to Loki. Grafana's Loki datasource defines a derived field: a regex matches `"traceID":"([a-f0-9]+)"` and turns each match into a clickable Jaeger link. The full chain: Go injects traceID into slog, Promtail ships it to Loki, Grafana renders it as a trace link.

## Kafka's Observability Challenge

HTTP calls have natural trace correlation, but Kafka decouples producers and consumers in time. Without explicit propagation, the consumer's work appears as an isolated trace. The solution is `InjectKafka` and `ExtractKafka` in `go/pkg/tracing/kafka.go`: the producer injects W3C trace context into Kafka message headers, and the consumer extracts it, creating linked spans. An order placed via HTTP can be traced through the ecommerce service, into Kafka, and through the analytics consumer -- even though those services never communicate directly.

## The Unknown Unknowns Argument

The interview answer to "why all three?" is straightforward. Metrics tell you something is wrong. Logs tell you what happened. Traces tell you where. Metrics without logs leave you guessing about the error message. Logs without traces leave you guessing which service in a chain is responsible. The platform's job is to make all three correlated, so the path from "something is wrong" to "here is the root cause" is as short as possible.
