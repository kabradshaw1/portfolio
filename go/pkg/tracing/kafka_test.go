package tracing

import (
	"context"
	"testing"

	"github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestKafka_InjectExtract_RoundTrips(t *testing.T) {
	// Set up a real propagator so inject/extract works.
	otel.SetTextMapPropagator(propagation.TraceContext{})
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())

	// Create a span to get a valid trace context.
	ctx, span := otel.Tracer("test").Start(context.Background(), "parent")
	defer span.End()

	traceID := span.SpanContext().TraceID().String()

	// Inject into Kafka headers.
	var headers []kafka.Header
	InjectKafka(ctx, &headers)

	found := false
	for _, h := range headers {
		if h.Key == "traceparent" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("traceparent not injected into Kafka headers")
	}

	// Extract from Kafka headers into a fresh context.
	extracted := ExtractKafka(context.Background(), headers)

	// Start a child span and verify it has the same trace ID.
	_, child := otel.Tracer("test").Start(extracted, "child")
	defer child.End()

	if child.SpanContext().TraceID().String() != traceID {
		t.Errorf("trace ID mismatch: got %s, want %s", child.SpanContext().TraceID(), traceID)
	}
}

func TestKafkaHeaderCarrier_SetOverwrites(t *testing.T) {
	headers := []kafka.Header{
		{Key: "foo", Value: []byte("bar")},
	}
	carrier := &kafkaHeaderCarrier{headers: &headers}

	carrier.Set("foo", "baz")

	if got := carrier.Get("foo"); got != "baz" {
		t.Errorf("expected baz, got %s", got)
	}
	if len(*carrier.headers) != 1 {
		t.Errorf("expected 1 header, got %d", len(*carrier.headers))
	}
}

func TestKafkaHeaderCarrier_Keys(t *testing.T) {
	headers := []kafka.Header{
		{Key: "a", Value: []byte("1")},
		{Key: "b", Value: []byte("2")},
	}
	carrier := &kafkaHeaderCarrier{headers: &headers}

	keys := carrier.Keys()
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
	if keys[0] != "a" || keys[1] != "b" {
		t.Errorf("unexpected keys: %v", keys)
	}
}
