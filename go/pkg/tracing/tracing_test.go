package tracing

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
)

func TestInit_EmptyEndpoint_DisablesTracing(t *testing.T) {
	shutdown, err := Init(context.Background(), "test-service", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}
}

func TestInit_SetsGlobalProvider(t *testing.T) {
	// Use a non-existent endpoint — the SDK retries in background, won't block.
	shutdown, err := Init(context.Background(), "test-service", "localhost:4317")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer shutdown(context.Background())

	// Verify a tracer can be obtained without panicking.
	tracer := otel.Tracer("test")
	_, span := tracer.Start(context.Background(), "test-span")
	span.End()
}
