package tracing

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// LogHandler wraps an slog.Handler to inject traceID from OpenTelemetry span context.
type LogHandler struct {
	slog.Handler
}

// NewLogHandler creates a handler that adds traceID to log records when a span is active.
func NewLogHandler(h slog.Handler) *LogHandler {
	return &LogHandler{Handler: h}
}

// Handle adds the traceID attribute if a valid span context exists.
func (h *LogHandler) Handle(ctx context.Context, r slog.Record) error {
	sc := trace.SpanContextFromContext(ctx)
	if sc.HasTraceID() {
		r.AddAttrs(slog.String("traceID", sc.TraceID().String()))
	}
	return h.Handler.Handle(ctx, r)
}

// WithAttrs returns a new LogHandler wrapping the inner handler's WithAttrs.
func (h *LogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &LogHandler{Handler: h.Handler.WithAttrs(attrs)}
}

// WithGroup returns a new LogHandler wrapping the inner handler's WithGroup.
func (h *LogHandler) WithGroup(name string) slog.Handler {
	return &LogHandler{Handler: h.Handler.WithGroup(name)}
}
