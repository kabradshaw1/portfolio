package main

import (
	"context"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/kabradshaw1/portfolio/go/auth-service/authmiddleware"
	authpb "github.com/kabradshaw1/portfolio/go/auth-service/pb/auth/v1"
	grpcsrv "github.com/kabradshaw1/portfolio/go/cart-service/internal/grpc"
	"github.com/kabradshaw1/portfolio/go/cart-service/internal/handler"
	"github.com/kabradshaw1/portfolio/go/cart-service/internal/productclient"
	"github.com/kabradshaw1/portfolio/go/cart-service/internal/repository"
	"github.com/kabradshaw1/portfolio/go/cart-service/internal/service"
	"github.com/kabradshaw1/portfolio/go/cart-service/internal/worker"
	pb "github.com/kabradshaw1/portfolio/go/cart-service/pb/cart/v1"
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

	shutdownTracer, err := tracing.Init(ctx, "cart-service", cfg.OTELEndpoint)
	if err != nil {
		log.Fatalf("tracing init: %v", err)
	}
	slog.SetDefault(slog.New(
		tracing.NewLogHandler(slog.NewJSONHandler(os.Stdout, nil)),
	))
	buildinfo.Log()

	pool := connectPostgres(ctx, cfg.DatabaseURL)

	redisClient := connectRedis(ctx, cfg.RedisURL)
	kafkaPub := connectKafka(cfg.KafkaBrokers)
	defer kafkaPub.Close()

	pgBreaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name:          "cart-postgres",
		OnStateChange: resilience.ObserveStateChange,
	})

	// Resolve gRPC client credentials — mTLS if TLS_CERT_DIR is set
	var grpcClientCreds credentials.TransportCredentials
	if certDir := os.Getenv("TLS_CERT_DIR"); certDir != "" {
		var clientTLSErr error
		grpcClientCreds, clientTLSErr = tlsconfig.ClientTLS(certDir)
		if clientTLSErr != nil {
			log.Fatalf("client tls config: %v", clientTLSErr)
		}
	} else {
		grpcClientCreds = insecure.NewCredentials()
	}

	var prodClient *productclient.GRPCClient
	if cfg.ProductGRPCAddr != "" {
		prodClient, err = productclient.New(cfg.ProductGRPCAddr, grpcClientCreds)
		if err != nil {
			log.Fatalf("product gRPC client: %v", err)
		}
		defer prodClient.Close()
		slog.Info("connected to product-service gRPC", "addr", cfg.ProductGRPCAddr)
	}

	cartRepo := repository.NewCartRepository(pool, pgBreaker)
	cartSvc := service.NewCartService(cartRepo, kafkaPub, prodClient)

	// RabbitMQ saga handler (optional)
	var sagaSupervisor *cartSagaSupervisor
	if cfg.RabbitmqURL != "" {
		sagaSupervisor = newCartSagaSupervisor(cfg.RabbitmqURL, cartSvc)
		go sagaSupervisor.Run(ctx)
		slog.Info("saga command handler enabled", "url", cfg.RabbitmqURL)
	}

	// Auth-service gRPC connection for denylist checks.
	authConn, err := grpc.NewClient(cfg.AuthGRPCURL,
		grpc.WithTransportCredentials(grpcClientCreds),
		grpc.WithUnaryInterceptor(grpcmetrics.UnaryClientInterceptor("auth-service")),
	)
	if err != nil {
		log.Fatalf("auth gRPC dial: %v", err)
	}
	defer authConn.Close()
	authClient := authpb.NewAuthServiceClient(authConn)
	authMw := authmiddleware.New(cfg.JWTSecret, authClient)

	// REST server
	router := setupRouter(cfg,
		handler.NewCartHandler(cartSvc),
		handler.NewHealthHandler(pool, redisClient),
		redisClient,
		authMw,
	)

	httpSrv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("REST server starting", "port", cfg.Port)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("REST server failed: %v", err)
		}
	}()

	// gRPC server — mTLS if TLS_CERT_DIR is set, plaintext otherwise
	var grpcServer *grpc.Server
	var tlsWatchStop func()
	if certDir := os.Getenv("TLS_CERT_DIR"); certDir != "" {
		serverTLS, tlsErr := tlsconfig.ServerTLS(certDir)
		if tlsErr != nil {
			log.Fatalf("tls config: %v", tlsErr)
		}
		grpcServer = grpc.NewServer(
			grpc.Creds(credentials.NewTLS(serverTLS)),
			grpc.StatsHandler(otelgrpc.NewServerHandler()),
		)
		certPtr, _, tlsErr := tlsconfig.Load(certDir)
		if tlsErr != nil {
			log.Fatalf("tls cert pointer: %v", tlsErr)
		}
		tlsWatchStop, tlsErr = tlsconfig.Watch(certDir, certPtr)
		if tlsErr != nil {
			log.Fatalf("tls watcher: %v", tlsErr)
		}
		slog.Info("mTLS enabled for gRPC server", "certDir", certDir)
	} else {
		grpcServer = grpc.NewServer(
			grpc.StatsHandler(otelgrpc.NewServerHandler()),
		)
	}
	pb.RegisterCartServiceServer(grpcServer, grpcsrv.NewCartGRPCServer(cartSvc))

	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthSrv)
	healthSrv.SetServingStatus("cart.v1.CartService", healthpb.HealthCheckResponse_SERVING)

	reflection.Register(grpcServer)

	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		log.Fatalf("gRPC listen: %v", err)
	}

	go func() {
		slog.Info("gRPC server starting", "port", cfg.GRPCPort)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("gRPC server failed: %v", err)
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
	sm.Register("drain-http", 0, shutdown.DrainHTTP("cart-http", httpSrv))
	sm.Register("drain-grpc", 0, shutdown.DrainGRPC("cart-grpc", grpcServer))
	if sagaSupervisor != nil {
		sm.Register("wait-saga", 10, shutdown.WaitForInflight("cart-saga", sagaSupervisor.IsIdle, 100*time.Millisecond))
	}
	sm.Register("postgres", 20, func(_ context.Context) error {
		pool.Close()
		return nil
	})
	sm.Register("otel", 30, func(ctx context.Context) error {
		return shutdownTracer(ctx)
	})
	sm.Wait()
}

type cartSagaSupervisor struct {
	url string
	svc worker.CartServiceForSaga
	mu  sync.RWMutex
	cur *worker.SagaHandler
}

func newCartSagaSupervisor(url string, svc worker.CartServiceForSaga) *cartSagaSupervisor {
	return &cartSagaSupervisor{url: url, svc: svc}
}

func (s *cartSagaSupervisor) Run(ctx context.Context) {
	attempt := 0
	for ctx.Err() == nil {
		err := s.runOnce(ctx)
		s.setHandler(nil)
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

func (s *cartSagaSupervisor) runOnce(ctx context.Context) error {
	conn, ch, err := connectRabbitMQ(s.url)
	if err != nil {
		return err
	}
	defer func() {
		_ = ch.Close()
		_ = conn.Close()
	}()

	if err := worker.DeclareSagaTopology(ch); err != nil {
		return err
	}
	publisher, err := rabbitmq.NewPublisher(ch)
	if err != nil {
		return err
	}
	handler := worker.NewSagaHandlerWithPublisher(s.svc, ch, publisher)
	s.setHandler(handler)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	handlerDone := make(chan error, 1)
	go func() {
		handlerDone <- handler.Start(runCtx)
	}()

	connClosed := conn.NotifyClose(make(chan *amqp.Error, 1))
	chClosed := ch.NotifyClose(make(chan *amqp.Error, 1))

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-handlerDone:
		return err
	case err := <-connClosed:
		cancel()
		return rabbitCloseError(err)
	case err := <-chClosed:
		cancel()
		return rabbitCloseError(err)
	}
}

func (s *cartSagaSupervisor) IsIdle() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur == nil || s.cur.IsIdle()
}

func (s *cartSagaSupervisor) setHandler(handler *worker.SagaHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cur = handler
}

func rabbitCloseError(err *amqp.Error) error {
	if err == nil {
		return context.Canceled
	}
	return err
}
