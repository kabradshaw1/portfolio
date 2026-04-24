package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ConsumerStatus reports the health of the Kafka consumer.
// Implemented by the consumer package (created in a later task).
type ConsumerStatus interface {
	Connected() bool
	LatestEventTime() time.Time
}

// HealthHandler serves liveness and readiness probes.
type HealthHandler struct {
	pool     *pgxpool.Pool
	consumer ConsumerStatus
}

// NewHealthHandler creates a HealthHandler. consumer may be nil during startup.
func NewHealthHandler(pool *pgxpool.Pool, consumer ConsumerStatus) *HealthHandler {
	return &HealthHandler{pool: pool, consumer: consumer}
}

// Health checks database connectivity and consumer status, returning an
// aggregate health response with projection lag metadata.
func (h *HealthHandler) Health(c *gin.Context) {
	ctx := c.Request.Context()

	dbOK := true
	if err := h.pool.Ping(ctx); err != nil {
		dbOK = false
	}

	kafkaOK := false
	var lagSeconds float64

	if h.consumer != nil {
		kafkaOK = h.consumer.Connected()
		if kafkaOK {
			latest := h.consumer.LatestEventTime()
			if !latest.IsZero() {
				lagSeconds = time.Since(latest).Seconds()
				c.Header("X-Projection-Lag", fmt.Sprintf("%.1f", lagSeconds))
			}
		}
	}

	status := "ok"
	httpStatus := http.StatusOK
	if !dbOK {
		// Database is critical — service cannot serve reads without it.
		status = "degraded"
		httpStatus = http.StatusServiceUnavailable
	} else if !kafkaOK {
		// Kafka disconnected is informational — the service can still serve
		// existing read models. An empty topic is a valid steady state.
		status = "degraded"
	}

	c.JSON(httpStatus, gin.H{
		"status":     status,
		"database":   dbOK,
		"kafka":      kafkaOK,
		"lagSeconds": lagSeconds,
	})
}
