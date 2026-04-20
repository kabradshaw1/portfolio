package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

func Logging() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := uuid.New().String()
		c.Set("requestId", requestID)
		c.Header("X-Request-ID", requestID)
		start := time.Now()
		c.Next()
		latency := time.Since(start)

		attrs := []any{
			"requestId", requestID,
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency", latency.String(),
			"ip", c.ClientIP(),
			"userId", c.GetString("userId"),
		}

		sc := trace.SpanContextFromContext(c.Request.Context())
		if sc.HasTraceID() {
			attrs = append(attrs, "traceID", sc.TraceID().String())
		}

		slog.InfoContext(c.Request.Context(), "request", attrs...)
	}
}
