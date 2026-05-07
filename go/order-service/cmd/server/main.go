package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/kabradshaw1/portfolio/go/auth-service/authmiddleware"
	authpb "github.com/kabradshaw1/portfolio/go/auth-service/pb/auth/v1"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/cartclient"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/db"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/handler"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/partition"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/paymentclient"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/productclient"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/reporting"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/repository"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/saga"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/service"
	"github.com/kabradshaw1/portfolio/go/pkg/buildinfo"
	"github.com/kabradshaw1/portfolio/go/pkg/grpcmetrics"
	rabbitmq "github.com/kabradshaw1/portfolio/go/pkg/rabbitmq"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	"github.com/kabradshaw1/portfolio/go/pkg/shutdown"
	"github.com/kabradshaw1/portfolio/go/pkg/tlsconfig"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

func main() {
	cfg := loadConfig()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shutdownTracer, err := tracing.Init(ctx, "order-service", cfg.OTELEndpoint)
	if err != nil {
		log.Fatalf("tracing init: %v", err)
	}
	slog.SetDefault(slog.New(
		tracing.NewLogHandler(slog.NewJSONHandler(os.Stdout, nil)),
	))
	buildinfo.Log()

	pools, err := db.New(ctx, cfg.DatabaseURL, cfg.DatabaseURLReplica)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	pool := pools.Primary
	slog.Info("connected to database",
		"replica_configured", cfg.DatabaseURLReplica != "",
	)

	redisClient := connectRedis(ctx, cfg.RedisURL)

	kafkaPub := connectKafka(cfg.KafkaBrokers)
	defer kafkaPub.Close()

	pgBreaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name:          "order-postgres",
		OnStateChange: resilience.ObserveStateChange,
	})
	orderRepo := repository.NewOrderRepository(pool, pgBreaker)
	returnRepo := repository.NewReturnRepository(pool, pgBreaker)

	// Resolve gRPC transport credentials — mTLS if TLS_CERT_DIR is set
	var grpcCreds credentials.TransportCredentials
	var tlsWatchStop func()
	if certDir := os.Getenv("TLS_CERT_DIR"); certDir != "" {
		var tlsErr error
		grpcCreds, tlsErr = tlsconfig.ClientTLS(certDir)
		if tlsErr != nil {
			log.Fatalf("tls config: %v", tlsErr)
		}
		certPtr, _, tlsErr := tlsconfig.Load(certDir)
		if tlsErr != nil {
			log.Fatalf("tls cert pointer: %v", tlsErr)
		}
		tlsWatchStop, tlsErr = tlsconfig.Watch(certDir, certPtr)
		if tlsErr != nil {
			log.Fatalf("tls watcher: %v", tlsErr)
		}
		slog.Info("mTLS enabled for gRPC clients", "certDir", certDir)
	} else {
		grpcCreds = insecure.NewCredentials()
	}

	var prodClient *productclient.GRPCClient
	if cfg.ProductGRPCAddr != "" {
		var err error
		prodClient, err = productclient.New(cfg.ProductGRPCAddr, grpcCreds)
		if err != nil {
			log.Fatalf("product gRPC client: %v", err)
		}
		defer prodClient.Close()
		slog.Info("connected to product-service gRPC", "addr", cfg.ProductGRPCAddr)
	}

	var cartClient *cartclient.GRPCClient
	if cfg.CartGRPCAddr != "" {
		var err error
		cartClient, err = cartclient.New(cfg.CartGRPCAddr, cfg.ProductGRPCAddr, grpcCreds)
		if err != nil {
			log.Fatalf("cart gRPC client: %v", err)
		}
		defer cartClient.Close()
		slog.Info("connected to cart-service gRPC", "addr", cfg.CartGRPCAddr)
	}

	var payClient *paymentclient.GRPCClient
	if cfg.PaymentGRPCAddr != "" {
		var err error
		payClient, err = paymentclient.New(cfg.PaymentGRPCAddr, grpcCreds)
		if err != nil {
			log.Fatalf("payment gRPC client: %v", err)
		}
		defer payClient.Close()
		slog.Info("connected to payment-service gRPC", "addr", cfg.PaymentGRPCAddr)
	}

	// Create DLQ client for admin endpoints.
	dlqClient := &reconnectingDLQClient{url: cfg.RabbitmqURL}
	rabbitPublisher := rabbitmq.NewReconnectingPublisher()

	// Create saga orchestrator with stock checker adapter.
	sagaPub := saga.NewPublisherWithConfirmPublisher(rabbitPublisher)
	orch := saga.NewOrchestrator(orderRepo, sagaPub, prodClient, payClient, kafkaPub, cfg.FrontendURL)

	// Start saga event consumer.
	sagaSupervisor := newOrderSagaSupervisor(cfg.RabbitmqURL, orch, rabbitPublisher)
	go sagaSupervisor.Run(ctx)

	// Recover incomplete sagas from previous crashes.
	saga.RecoverIncomplete(ctx, orderRepo, orch)

	// Start partition maintenance
	partition.RunMaintenance(ctx, pool)

	// Start materialized view refresher
	refresher := reporting.NewRefresher(pool, 15*time.Minute)
	go refresher.Run(ctx)

	// Create reporting repository and handler. Reporting reads target the
	// streaming read replica when DATABASE_URL_REPLICA is set; otherwise
	// pools.Reporting falls back to the primary (see internal/db.New).
	reportingRepo := reporting.NewRepository(pools.Reporting, pgBreaker)
	reportingHandler := handler.NewReportingHandler(reportingRepo)

	orderSvc := service.NewOrderService(orderRepo, cartClient, orch)
	returnSvc := service.NewReturnService(returnRepo, orderSvc)

	// Auth-service gRPC connection for denylist checks.
	authConn, err := grpc.NewClient(cfg.AuthGRPCURL,
		grpc.WithTransportCredentials(grpcCreds),
		grpc.WithUnaryInterceptor(grpcmetrics.UnaryClientInterceptor("auth-service")),
	)
	if err != nil {
		log.Fatalf("auth gRPC dial: %v", err)
	}
	defer authConn.Close()
	authClient := authpb.NewAuthServiceClient(authConn)
	authMw := authmiddleware.New(cfg.JWTSecret, authClient)

	router := setupRouter(cfg,
		handler.NewOrderHandler(orderSvc),
		handler.NewReturnHandler(returnSvc),
		handler.NewHealthHandler(pool, redisClient),
		handler.NewAdminHandler(dlqClient),
		reportingHandler,
		redisClient,
		authMw,
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

	// Graceful shutdown
	sm := shutdown.New(15 * time.Second)
	if tlsWatchStop != nil {
		sm.Register("tls-watcher", 0, func(_ context.Context) error {
			tlsWatchStop()
			return nil
		})
	}
	sm.Register("cancel-ctx", 0, func(_ context.Context) error {
		cancel()
		return nil
	})
	sm.Register("drain-http", 0, shutdown.DrainHTTP("order-http", srv))
	sm.Register("wait-saga", 10, shutdown.WaitForInflight("order-saga", sagaSupervisor.IsIdle, 100*time.Millisecond))
	sm.Register("postgres", 20, func(_ context.Context) error {
		pools.Close()
		return nil
	})
	sm.Register("otel", 30, func(ctx context.Context) error {
		return shutdownTracer(ctx)
	})
	sm.Wait()
}

type orderSagaSupervisor struct {
	url       string
	orch      *saga.Orchestrator
	publisher *rabbitmq.ReconnectingPublisher
	mu        sync.RWMutex
	consumer  *saga.Consumer
}

func newOrderSagaSupervisor(url string, orch *saga.Orchestrator, publisher *rabbitmq.ReconnectingPublisher) *orderSagaSupervisor {
	return &orderSagaSupervisor{url: url, orch: orch, publisher: publisher}
}

func (s *orderSagaSupervisor) Run(ctx context.Context) {
	attempt := 0
	for ctx.Err() == nil {
		err := s.runOnce(ctx)
		s.setConsumer(nil)
		s.publisher.SetUnavailable(err)
		if ctx.Err() != nil {
			return
		}
		slog.WarnContext(ctx, "saga rabbitmq connection lost, reconnecting", "error", err)
		if !waitForRabbitMQReconnect(ctx, attempt) {
			return
		}
		attempt++
	}
}

func (s *orderSagaSupervisor) runOnce(ctx context.Context) error {
	conn, ch, err := connectRabbitMQ(s.url)
	if err != nil {
		return err
	}
	defer func() {
		_ = ch.Close()
		_ = conn.Close()
	}()

	if err := saga.DeclareTopology(ch); err != nil {
		return err
	}
	confirmedPublisher, err := rabbitmq.NewPublisher(ch)
	if err != nil {
		return err
	}
	s.publisher.SetPublisher(confirmedPublisher)

	consumer := saga.NewConsumerWithPublisher(s.orch, s.publisher)
	s.setConsumer(consumer)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	consumerDone := make(chan error, 1)
	go func() {
		consumerDone <- consumer.Start(runCtx, ch)
	}()

	connClosed := conn.NotifyClose(make(chan *amqp.Error, 1))
	chClosed := ch.NotifyClose(make(chan *amqp.Error, 1))

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-consumerDone:
		return err
	case err := <-connClosed:
		cancel()
		return rabbitCloseError(err)
	case err := <-chClosed:
		cancel()
		return rabbitCloseError(err)
	}
}

func (s *orderSagaSupervisor) IsIdle() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.consumer == nil || s.consumer.IsIdle()
}

func (s *orderSagaSupervisor) setConsumer(consumer *saga.Consumer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.consumer = consumer
}

type reconnectingDLQClient struct {
	url string
}

func (c *reconnectingDLQClient) List(limit int) ([]saga.DLQMessage, error) {
	conn, ch, err := connectRabbitMQ(c.url)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = ch.Close()
		_ = conn.Close()
	}()
	if err := saga.DeclareTopology(ch); err != nil {
		return nil, err
	}
	return saga.NewDLQClient(ch).List(limit)
}

func (c *reconnectingDLQClient) Replay(index int) (*saga.DLQMessage, error) {
	conn, ch, err := connectRabbitMQ(c.url)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = ch.Close()
		_ = conn.Close()
	}()
	if err := saga.DeclareTopology(ch); err != nil {
		return nil, err
	}
	return saga.NewDLQClient(ch).Replay(index)
}

func rabbitCloseError(err *amqp.Error) error {
	if err == nil {
		return context.Canceled
	}
	return err
}
