package main

import (
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/handler"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/middleware"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/repository"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/service"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/worker"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	pkgmw "github.com/kabradshaw1/portfolio/go/pkg/middleware"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

type rabbitPublisher struct {
	ch *amqp.Channel
}

func (p *rabbitPublisher) PublishOrderCreated(orderID string) error {
	body, _ := json.Marshal(model.OrderMessage{OrderID: orderID})

	// Start a span and inject trace context into AMQP headers.
	ctx, span := otel.Tracer("rabbitmq").Start(context.Background(), "rabbitmq.publish",
		trace.WithAttributes(
			attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.destination", "ecommerce"),
			attribute.String("messaging.routing_key", "order.created"),
		),
	)
	defer span.End()

	headers := make(amqp.Table)
	tracing.InjectAMQP(ctx, headers)

	return p.ch.Publish("ecommerce", "order.created", false, false, amqp.Publishing{
		ContentType: "application/json",
		Headers:     headers,
		Body:        body,
	})
}

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET is required")
	}
	rabbitmqURL := os.Getenv("RABBITMQ_URL")
	if rabbitmqURL == "" {
		log.Fatal("RABBITMQ_URL is required")
	}
	allowedOrigins := os.Getenv("ALLOWED_ORIGINS")
	if allowedOrigins == "" {
		allowedOrigins = "http://localhost:3000"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8092"
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Tracing
	shutdownTracer, err := tracing.Init(ctx, "ecommerce-service", os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if err != nil {
		log.Fatalf("tracing init: %v", err)
	}
	defer func() { _ = shutdownTracer(ctx) }()

	// Connect to Postgres
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		log.Fatalf("failed to parse database URL: %v", err)
	}
	poolConfig.MaxConns = 25
	poolConfig.MinConns = 5
	poolConfig.MaxConnIdleTime = 5 * time.Minute
	poolConfig.MaxConnLifetime = 30 * time.Minute
	poolConfig.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("failed to ping database: %v", err)
	}
	slog.Info("connected to database")

	// Connect to Redis (optional)
	var redisClient *redis.Client
	redisURL := os.Getenv("REDIS_URL")
	if redisURL != "" {
		opts, err := redis.ParseURL(redisURL)
		if err != nil {
			log.Fatalf("failed to parse REDIS_URL: %v", err)
		}
		redisClient = redis.NewClient(opts)
		if err := redisClient.Ping(ctx).Err(); err != nil {
			slog.Warn("redis not available, continuing without cache", "error", err)
			redisClient = nil
		} else {
			slog.Info("connected to redis")
		}
	}

	// Connect to RabbitMQ
	conn, err := amqp.Dial(rabbitmqURL)
	if err != nil {
		log.Fatalf("failed to connect to RabbitMQ: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("failed to open RabbitMQ channel: %v", err)
	}
	defer ch.Close()

	if err := ch.ExchangeDeclare("ecommerce", "topic", true, false, false, false, nil); err != nil {
		log.Fatalf("failed to declare exchange: %v", err)
	}
	slog.Info("connected to RabbitMQ")

	// Wire dependencies
	pgBreaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name:          "ecommerce-postgres",
		OnStateChange: resilience.ObserveStateChange,
	})
	productRepo := repository.NewProductRepository(pool, pgBreaker)
	cartRepo := repository.NewCartRepository(pool, pgBreaker)
	orderRepo := repository.NewOrderRepository(pool, pgBreaker)

	publisher := &rabbitPublisher{ch: ch}

	productSvc := service.NewProductService(productRepo, redisClient)
	cartSvc := service.NewCartService(cartRepo)
	orderSvc := service.NewOrderService(orderRepo, cartRepo, publisher)

	returnRepo := repository.NewReturnRepository(pool, pgBreaker)
	returnSvc := service.NewReturnService(returnRepo, orderSvc)
	returnHandler := handler.NewReturnHandler(returnSvc)

	productHandler := handler.NewProductHandler(productSvc)
	cartHandler := handler.NewCartHandler(cartSvc)
	orderHandler := handler.NewOrderHandler(orderSvc)
	healthHandler := handler.NewHealthHandler(pool, redisClient)

	// Start order processor worker
	workerConcurrency := 3
	if v := os.Getenv("WORKER_CONCURRENCY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			workerConcurrency = n
		}
	}
	processor := worker.NewOrderProcessor(orderRepo, productRepo, productSvc)
	go func() {
		if err := processor.StartConsumer(ctx, ch, workerConcurrency); err != nil {
			slog.Error("order processor failed", "error", err)
		}
	}()

	// Set up Gin
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(pkgmw.SecurityHeaders())
	router.Use(otelgin.Middleware("ecommerce-service"))
	router.Use(middleware.Logging())
	router.Use(apperror.ErrorHandler())
	router.Use(middleware.Metrics())
	router.Use(middleware.CORS(allowedOrigins))

	// Public routes
	router.GET("/products", productHandler.List)
	router.GET("/products/:id", productHandler.GetByID)
	router.GET("/categories", productHandler.Categories)
	router.GET("/health", healthHandler.Health)
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	ecomLimiter := middleware.NewRateLimiter(redisClient, "ecom:ratelimit", 60, time.Minute)

	// Authenticated routes
	auth := router.Group("/")
	auth.Use(middleware.Auth(jwtSecret))
	auth.Use(ecomLimiter.Middleware())
	{
		auth.GET("/cart", cartHandler.GetCart)
		auth.POST("/cart", middleware.Idempotency(redisClient, false), cartHandler.AddItem)
		auth.PUT("/cart/:itemId", cartHandler.UpdateQuantity)
		auth.DELETE("/cart/:itemId", cartHandler.RemoveItem)

		auth.POST("/orders", middleware.Idempotency(redisClient, true), orderHandler.Checkout)
		auth.GET("/orders", orderHandler.List)
		auth.GET("/orders/:id", orderHandler.GetByID)
		auth.POST("/orders/:id/returns", returnHandler.Initiate)
	}

	// Start server
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("server starting", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server failed: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down server")

	cancel() // Stop workers

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server forced to shutdown: %v", err)
	}
	slog.Info("server stopped")
}
