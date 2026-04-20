package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type HealthHandler struct {
	pool  *pgxpool.Pool
	redis *redis.Client
}

func NewHealthHandler(pool *pgxpool.Pool, redisClient *redis.Client) *HealthHandler {
	return &HealthHandler{pool: pool, redis: redisClient}
}

func (h *HealthHandler) Health(c *gin.Context) {
	ctx := c.Request.Context()
	checks := gin.H{}
	if err := h.pool.Ping(ctx); err != nil {
		checks["postgres"] = "unhealthy"
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy", "checks": checks})
		return
	}
	checks["postgres"] = "healthy"
	if h.redis != nil {
		if err := h.redis.Ping(ctx).Err(); err != nil {
			checks["redis"] = "unhealthy"
		} else {
			checks["redis"] = "healthy"
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "healthy", "checks": checks})
}
