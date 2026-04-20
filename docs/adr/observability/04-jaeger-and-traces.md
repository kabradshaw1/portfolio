# Distributed Tracing with Jaeger and OpenTelemetry

## Spans and Traces

A span represents a single unit of work: an HTTP handler processing a request, a Redis GET, a Kafka message publish. Each span records a start time, duration, status, key-value tags, and a reference to its parent span. A trace is a tree of spans sharing a common traceID. The root span is typically the initial HTTP request; child spans represent downstream operations triggered by that request. Together, the spans in a trace reconstruct the full path of an operation through the system.

Consider a user placing an order. The root span covers the HTTP handler. Child spans cover the PostgreSQL query, Redis cache invalidation, and Kafka publish. The analytics consumer creates its own span linked to the same trace. Jaeger displays this as a timeline showing which operation took the most time.

## Context Propagation

For traces to span services, the traceID must travel with the request. The W3C `traceparent` header format (`00-<traceID>-<spanID>-<flags>`) is the standard. The `otelhttp` transport injects this header into outbound HTTP requests; the receiving service's `otelgin` middleware extracts it and creates a child span. No application code is required for HTTP propagation.

## This Project's OTel Setup

Tracing initialization lives in `go/pkg/tracing/tracing.go`. The `Init` function creates an OTLP gRPC exporter pointing at Jaeger's collector on port 4317, sets up a `TracerProvider` with the service name, and registers the W3C TraceContext propagator. If `OTEL_EXPORTER_OTLP_ENDPOINT` is empty, tracing is disabled (no-op).

Auto-instrumentation covers HTTP in both directions: `otelgin` creates spans for incoming requests, `otelhttp.NewTransport` propagates context on outgoing calls. For non-HTTP operations, services create manual spans -- Redis uses a `RedisSpan` helper, and the ai-service's agent loop creates parent-child spans for each tool call.

## Kafka Trace Propagation

HTTP propagation relies on request-response pairs where headers flow naturally. Kafka is asynchronous: the producer writes a message and moves on, the consumer reads it later. There are no HTTP headers. The solution in `go/pkg/tracing/kafka.go` is a `kafkaHeaderCarrier` adapter that implements OpenTelemetry's `TextMapCarrier` interface over Kafka message headers.

When the ecommerce service publishes an order event:

```go
tracing.InjectKafka(ctx, &msg.Headers)
```

This writes the `traceparent` and `tracestate` headers into the Kafka message. When the analytics consumer reads the message:

```go
ctx = tracing.ExtractKafka(ctx, msg.Headers)
```

This reconstructs the span context, and any spans created with this context become children of the original trace. The result is that a single user request can be traced from the initial HTTP call through the ecommerce handler, into Kafka, and through the analytics consumer's processing -- all visible as one connected trace in Jaeger.

## When Tracing Helps vs When It Is Noise

Tracing excels at latency debugging: if p95 spikes, a trace shows whether the bottleneck is the database, a downstream call, or the service itself. It also reveals dependency topology and per-hop latency.

Tracing becomes noise in high-volume batch processing. Creating a span per record in a 10,000-item job produces overwhelming data. Sampling helps, but tracing is fundamentally designed for request-response workflows with clear start and end points.
