package tracing

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Init sets up the OpenTelemetry tracer provider with an OTLP gRPC exporter.
// If endpoint is empty, tracing is disabled (no-op). The returned shutdown
// function flushes any pending spans and should be deferred.
func Init(ctx context.Context, serviceName, endpoint string) (func(context.Context) error, error) {
	noop := func(context.Context) error { return nil }

	if endpoint == "" {
		slog.Info("tracing disabled (no OTEL_EXPORTER_OTLP_ENDPOINT)")
		return noop, nil
	}

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return noop, err
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceNameKey.String(serviceName)),
	)
	if err != nil {
		return noop, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	slog.Info("tracing enabled", "endpoint", endpoint, "service", serviceName)
	return tp.Shutdown, nil
}
