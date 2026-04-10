package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// RedisSpan starts a span for a Redis operation. The caller must end the span.
func RedisSpan(ctx context.Context, operation, key string) (context.Context, trace.Span) {
	return otel.Tracer("redis").Start(ctx, "redis."+operation,
		trace.WithAttributes(
			attribute.String("db.system", "redis"),
			attribute.String("db.operation", operation),
			attribute.String("db.redis.key", key),
		),
	)
}
