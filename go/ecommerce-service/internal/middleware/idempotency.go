package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	"github.com/redis/go-redis/v9"
)

const (
	idempotencyProcessingTTL = 30 * time.Second  // marker while request is in-flight
	idempotencyDoneTTL       = 24 * time.Hour    // cache completed response for replay
	idempotencyKeyPrefix     = "idempotency"
)

// idempotencyEntry is the value stored in Redis for a completed request.
type idempotencyEntry struct {
	Status     string `json:"status"`
	StatusCode int    `json:"status_code"`
	Body       string `json:"body"`
}

// responseCapture wraps gin.ResponseWriter to buffer the response body
// so the middleware can cache it after the handler returns.
type responseCapture struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *responseCapture) Write(data []byte) (int, error) {
	w.body.Write(data)
	return w.ResponseWriter.Write(data)
}

// Idempotency returns a Gin middleware that deduplicates mutating requests via
// a Redis-backed idempotency key (Idempotency-Key header).
//
// If required is true, requests without the header are rejected with 400.
// If required is false, requests without the header pass through unchanged.
//
// When redisClient is nil the middleware fails open (passes every request through).
func Idempotency(redisClient *redis.Client, required bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader("Idempotency-Key")

		// Step 1-2: header presence check.
		if key == "" {
			if required {
				_ = c.Error(apperror.BadRequest("MISSING_IDEMPOTENCY_KEY", "Idempotency-Key header is required"))
				c.Abort()
				return
			}
			c.Next()
			return
		}

		// Step 4: validate the key is a valid UUID.
		if _, err := uuid.Parse(key); err != nil {
			_ = c.Error(apperror.BadRequest("INVALID_IDEMPOTENCY_KEY", "Idempotency-Key must be a valid UUID"))
			c.Abort()
			return
		}

		// Step 5: fail open when Redis is unavailable.
		if redisClient == nil {
			c.Next()
			return
		}

		userId := c.GetString("userId")
		redisKey := fmt.Sprintf("%s:%s:%s", idempotencyKeyPrefix, userId, key)
		ctx := c.Request.Context()

		// Step 6: check for an existing entry.
		raw, err := redisClient.Get(ctx, redisKey).Result()
		if err != nil && err != redis.Nil {
			slog.Warn("idempotency: redis get failed, failing open", "error", err, "key", redisKey)
			metrics.IdempotencyOps.WithLabelValues("error").Inc()
			c.Next()
			return
		}

		if err == nil {
			// Key exists — decode and inspect.
			var entry idempotencyEntry
			if jsonErr := json.Unmarshal([]byte(raw), &entry); jsonErr != nil {
				// Corrupt entry — treat as missing and proceed.
				slog.Warn("idempotency: corrupt entry, treating as miss", "error", jsonErr, "key", redisKey)
				metrics.IdempotencyOps.WithLabelValues("error").Inc()
			} else if entry.Status == "done" {
				// Replay cached response.
				metrics.IdempotencyOps.WithLabelValues("hit").Inc()
				c.Data(entry.StatusCode, "application/json; charset=utf-8", []byte(entry.Body))
				c.Abort()
				return
			} else {
				// status == "processing": duplicate in-flight request.
				metrics.IdempotencyOps.WithLabelValues("conflict").Inc()
				_ = c.Error(apperror.Conflict("DUPLICATE_REQUEST", "a request with this idempotency key is already being processed"))
				c.Abort()
				return
			}
		}

		// Step 7: set processing marker with NX mode to guard against races.
		processingEntry, _ := json.Marshal(idempotencyEntry{Status: "processing"})
		setResult, setErr := redisClient.SetArgs(ctx, redisKey, string(processingEntry), redis.SetArgs{
			Mode: "NX",
			TTL:  idempotencyProcessingTTL,
		}).Result()
		if setErr != nil && setErr != redis.Nil {
			slog.Warn("idempotency: redis set nx failed, failing open", "error", setErr, "key", redisKey)
			metrics.IdempotencyOps.WithLabelValues("error").Inc()
			c.Next()
			return
		}
		if setResult != "OK" {
			// Another goroutine won the race.
			metrics.IdempotencyOps.WithLabelValues("conflict").Inc()
			_ = c.Error(apperror.Conflict("DUPLICATE_REQUEST", "a request with this idempotency key is already being processed"))
			c.Abort()
			return
		}

		// Step 8: wrap writer to capture response body, then call handlers.
		capture := &responseCapture{
			ResponseWriter: c.Writer,
			body:           &bytes.Buffer{},
		}
		c.Writer = capture

		c.Next()

		// Step 9: cache completed response only for 2xx status codes.
		statusCode := c.Writer.Status()
		if statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices {
			done, _ := json.Marshal(idempotencyEntry{
				Status:     "done",
				StatusCode: statusCode,
				Body:       capture.body.String(),
			})
			if cacheErr := redisClient.Set(ctx, redisKey, string(done), idempotencyDoneTTL).Err(); cacheErr != nil {
				slog.Warn("idempotency: failed to cache response", "error", cacheErr, "key", redisKey)
			}
		} else {
			// Non-2xx: delete the processing marker so the client can retry.
			redisClient.Del(ctx, redisKey)
		}

		metrics.IdempotencyOps.WithLabelValues("miss").Inc()
	}
}
