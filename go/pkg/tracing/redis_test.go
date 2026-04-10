package tracing

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestRedisSpan_CreatesSpanWithAttributes(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())

	ctx, span := RedisSpan(context.Background(), "GET", "ai:cache:key1")
	_ = ctx
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	s := spans[0]
	if s.Name != "redis.GET" {
		t.Errorf("name = %q, want redis.GET", s.Name)
	}

	attrs := make(map[string]string)
	for _, a := range s.Attributes {
		attrs[string(a.Key)] = a.Value.AsString()
	}
	if attrs["db.system"] != "redis" {
		t.Errorf("db.system = %q", attrs["db.system"])
	}
	if attrs["db.operation"] != "GET" {
		t.Errorf("db.operation = %q", attrs["db.operation"])
	}
	if attrs["db.redis.key"] != "ai:cache:key1" {
		t.Errorf("db.redis.key = %q", attrs["db.redis.key"])
	}
}
