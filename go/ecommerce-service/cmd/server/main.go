package main

import (
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"

	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/handler"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/middleware"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/repository"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/service"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/worker"
)

type rabbitPublisher struct {
	ch *amqp.Channel
}

func (p *rabbitPublisher) PublishOrderCreated(orderID string) error {
	body, _ := json.Marshal(model.OrderMessage{OrderID: orderID})
	return p.ch.Publish("ecommerce", "order.created", false, false, amqp.Publishing{
		ContentType: "application/json",
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

	// Connect to Postgres
	pool, err := pgxpool.New(ctx, databaseURL)
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
	productRepo := repository.NewProductRepository(pool)
	cartRepo := repository.NewCartRepository(pool)
	orderRepo := repository.NewOrderRepository(pool)

	publisher := &rabbitPublisher{ch: ch}

	productSvc := service.NewProductService(productRepo, redisClient)
	cartSvc := service.NewCartService(cartRepo)
	orderSvc := service.NewOrderService(orderRepo, cartRepo, publisher)

	productHandler := handler.NewProductHandler(productSvc)
	cartHandler := handler.NewCartHandler(cartSvc)
	orderHandler := handler.NewOrderHandler(orderSvc)
	healthHandler := handler.NewHealthHandler(pool, redisClient)

	// Start order processor worker
	processor := worker.NewOrderProcessor(orderRepo, productRepo, productSvc)
	go func() {
		if err := processor.StartConsumer(ctx, ch, 3); err != nil {
			slog.Error("order processor failed", "error", err)
		}
	}()

	// Set up Gin
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.Logging())
	router.Use(middleware.Metrics())
	router.Use(middleware.CORS(allowedOrigins))

	// Public routes
	router.GET("/products", productHandler.List)
	router.GET("/products/:id", productHandler.GetByID)
	router.GET("/categories", productHandler.Categories)
	router.GET("/health", healthHandler.Health)
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Authenticated routes
	auth := router.Group("/")
	auth.Use(middleware.Auth(jwtSecret))
	{
		auth.GET("/cart", cartHandler.GetCart)
		auth.POST("/cart", cartHandler.AddItem)
		auth.PUT("/cart/:itemId", cartHandler.UpdateQuantity)
		auth.DELETE("/cart/:itemId", cartHandler.RemoveItem)

		auth.POST("/orders", orderHandler.Checkout)
		auth.GET("/orders", orderHandler.List)
		auth.GET("/orders/:id", orderHandler.GetByID)
	}

	// Start server
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: router,
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
