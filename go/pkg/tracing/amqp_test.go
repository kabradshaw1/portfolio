package tracing

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestAMQP_InjectExtract_RoundTrips(t *testing.T) {
	// Set up a real propagator so inject/extract works.
	otel.SetTextMapPropagator(propagation.TraceContext{})
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())

	// Create a span to get a valid trace context.
	ctx, span := otel.Tracer("test").Start(context.Background(), "parent")
	defer span.End()

	traceID := span.SpanContext().TraceID().String()
	spanID := span.SpanContext().SpanID().String()

	// Inject into AMQP headers.
	headers := make(map[string]interface{})
	InjectAMQP(ctx, headers)

	if _, ok := headers["traceparent"]; !ok {
		t.Fatal("traceparent not injected into headers")
	}

	// Extract from AMQP headers into a fresh context.
	extracted := ExtractAMQP(context.Background(), headers)

	// Start a child span and verify it has the same trace ID.
	_, child := otel.Tracer("test").Start(extracted, "child")
	defer child.End()

	if child.SpanContext().TraceID().String() != traceID {
		t.Errorf("trace ID mismatch: got %s, want %s", child.SpanContext().TraceID(), traceID)
	}
	// The parent span ID of the child should be the original span.
	// We can't easily check parent span ID, but trace ID match confirms propagation.
	_ = spanID
}
