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

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/handler"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/repository"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/service"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/worker"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

type rabbitPublisher struct {
	ch *amqp.Channel
}

func (p *rabbitPublisher) PublishOrderCreated(orderID string) error {
	body, _ := json.Marshal(model.OrderMessage{OrderID: orderID})

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
	cfg := loadConfig()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shutdownTracer, err := tracing.Init(ctx, "ecommerce-service", cfg.OTELEndpoint)
	if err != nil {
		log.Fatalf("tracing init: %v", err)
	}
	defer func() { _ = shutdownTracer(ctx) }()

	pool := connectPostgres(ctx, cfg.DatabaseURL)
	defer pool.Close()

	redisClient := connectRedis(ctx, cfg.RedisURL)

	conn, ch := connectRabbitMQ(cfg.RabbitmqURL)
	defer conn.Close()
	defer ch.Close()

	kafkaPub := connectKafka(cfg.KafkaBrokers)
	defer kafkaPub.Close()

	pgBreaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name:          "ecommerce-postgres",
		OnStateChange: resilience.ObserveStateChange,
	})
	productRepo := repository.NewProductRepository(pool, pgBreaker)
	cartRepo := repository.NewCartRepository(pool, pgBreaker)
	orderRepo := repository.NewOrderRepository(pool, pgBreaker)
	returnRepo := repository.NewReturnRepository(pool, pgBreaker)

	publisher := &rabbitPublisher{ch: ch}
	productSvc := service.NewProductService(productRepo, redisClient)
	cartSvc := service.NewCartService(cartRepo, kafkaPub)
	orderSvc := service.NewOrderService(orderRepo, cartRepo, publisher, kafkaPub)
	returnSvc := service.NewReturnService(returnRepo, orderSvc)

	processor := worker.NewOrderProcessor(orderRepo, productRepo, productSvc, kafkaPub)
	go func() {
		if err := processor.StartConsumer(ctx, ch, cfg.WorkerConcurrency); err != nil {
			slog.Error("order processor failed", "error", err)
		}
	}()

	router := setupRouter(cfg,
		handler.NewProductHandler(productSvc),
		handler.NewCartHandler(cartSvc),
		handler.NewOrderHandler(orderSvc),
		handler.NewReturnHandler(returnSvc),
		handler.NewHealthHandler(pool, redisClient),
		redisClient,
	)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("server starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down server")

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server forced to shutdown: %v", err)
	}
	slog.Info("server stopped")
}
