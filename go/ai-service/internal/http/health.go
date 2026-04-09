package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ReadyCheck is a single dependency probe. Returns nil when healthy.
type ReadyCheck func() error

// RegisterHealthRoutes adds GET /health (liveness) and GET /ready (dependency probes).
func RegisterHealthRoutes(r *gin.Engine, checks map[string]ReadyCheck) {
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/ready", func(c *gin.Context) {
		results := map[string]string{}
		allOK := true
		for name, fn := range checks {
			if err := fn(); err != nil {
				results[name] = err.Error()
				allOK = false
			} else {
				results[name] = "ok"
			}
		}
		status := http.StatusOK
		if !allOK {
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, gin.H{"checks": results})
	})
}

// RegisterMetricsRoute wires GET /metrics using the Prometheus default handler.
func RegisterMetricsRoute(r *gin.Engine) {
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
}
