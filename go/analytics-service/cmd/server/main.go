package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/aggregator"
	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/consumer"
	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/handler"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	pkgmw "github.com/kabradshaw1/portfolio/go/pkg/middleware"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

func main() {
	port := getenv("PORT", "8094")
	kafkaBrokers := os.Getenv("KAFKA_BROKERS")
	if kafkaBrokers == "" {
		log.Fatal("KAFKA_BROKERS is required")
	}
	allowedOrigins := getenv("ALLOWED_ORIGINS", "http://localhost:3000")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Tracing
	shutdownTracer, err := tracing.Init(ctx, "analytics-service", os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if err != nil {
		log.Fatalf("tracing init: %v", err)
	}
	defer func() { _ = shutdownTracer(ctx) }()

	// Aggregators
	orders := aggregator.NewOrderAggregator()
	trending := aggregator.NewTrendingAggregator()
	carts := aggregator.NewCartAggregator()

	// Kafka consumer
	brokers := strings.Split(kafkaBrokers, ",")
	cons := consumer.New(brokers, orders, trending, carts)
	go func() {
		if err := cons.Run(ctx); err != nil {
			slog.Error("kafka consumer failed", "error", err)
		}
	}()

	// HTTP handlers
	analyticsHandler := handler.NewAnalyticsHandler(orders, trending, carts, cons)

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(pkgmw.SecurityHeaders())
	router.Use(otelgin.Middleware("analytics-service"))
	router.Use(corsMiddleware(allowedOrigins))
	router.Use(apperror.ErrorHandler())

	router.GET("/analytics/dashboard", analyticsHandler.Dashboard)
	router.GET("/analytics/trending", analyticsHandler.Trending)
	router.GET("/analytics/orders", analyticsHandler.Orders)
	router.GET("/health", analyticsHandler.Health)
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("analytics-service starting", "port", port, "brokers", kafkaBrokers)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server failed: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down analytics-service")

	cancel() // Stop consumer

	if err := cons.Close(); err != nil {
		slog.Error("kafka consumer close error", "error", err)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server forced to shutdown: %v", err)
	}
	slog.Info("analytics-service stopped")
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func corsMiddleware(allowedOrigins string) gin.HandlerFunc {
	originSet := make(map[string]bool)
	for _, o := range strings.Split(allowedOrigins, ",") {
		originSet[strings.TrimSpace(o)] = true
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if originSet[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, X-Requested-With")
			c.Header("Access-Control-Allow-Credentials", "true")
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
