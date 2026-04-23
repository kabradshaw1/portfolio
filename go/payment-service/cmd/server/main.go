package main

import (
	"context"
	"log" //nolint:depguard // stdlib log for fatal-before-slog-init
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"

	"github.com/kabradshaw1/portfolio/go/payment-service/internal/grpcserver"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/handler"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/outbox"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/repository"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/service"
	stripeClient "github.com/kabradshaw1/portfolio/go/payment-service/internal/stripe"
	pb "github.com/kabradshaw1/portfolio/go/payment-service/pb/payment/v1"
	"github.com/kabradshaw1/portfolio/go/pkg/buildinfo"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	"github.com/kabradshaw1/portfolio/go/pkg/shutdown"
	"github.com/kabradshaw1/portfolio/go/pkg/tlsconfig"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

const (
	outboxPollInterval = 5 * time.Second
	outboxBatchSize    = 100
)

func main() {
	cfg := loadConfig()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize tracing.
	shutdownTracer, err := tracing.Init(ctx, "payment-service", cfg.OTELEndpoint)
	if err != nil {
		log.Fatalf("tracing init: %v", err)
	}

	slog.SetDefault(slog.New(
		tracing.NewLogHandler(slog.NewJSONHandler(os.Stdout, nil)),
	))
	buildinfo.Log()

	// Infrastructure connections.
	pool := connectPostgres(ctx, cfg.DatabaseURL)

	conn, ch := connectRabbitMQ(cfg.RabbitmqURL)
	defer conn.Close()
	defer ch.Close()

	kafkaWriter := connectKafka(cfg.KafkaBrokers)

	// Circuit breaker for PostgreSQL.
	pgBreaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name:          "payment-postgres",
		OnStateChange: resilience.ObserveStateChange,
	})

	// Repositories.
	paymentRepo := repository.NewPaymentRepository(pool, pgBreaker)
	outboxRepo := repository.NewOutboxRepository(pool, pgBreaker)
	processedEventRepo := repository.NewProcessedEventRepository(pool, pgBreaker)

	// Stripe client and verifier.
	stripeCli := stripeClient.NewClient(cfg.StripeSecretKey)
	verifier := stripeClient.NewVerifier(cfg.StripeWebhookSecret)

	// Services.
	paymentSvc := service.NewPaymentService(paymentRepo, stripeCli)
	webhookSvc := service.NewWebhookService(paymentRepo, processedEventRepo, outboxRepo, pool)

	// Start outbox poller goroutine.
	poller := outbox.NewPoller(outboxRepo, ch, outboxPollInterval, outboxBatchSize)
	go poller.Run(ctx)

	// Handlers.
	webhookHandler := handler.NewWebhookHandler(webhookSvc, verifier)
	healthHandler := handler.NewHealthHandler(pool)

	// REST server.
	router := setupRouter(cfg, webhookHandler, healthHandler)

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

	// gRPC server — mTLS if TLS_CERT_DIR is set, insecure otherwise.
	grpcOpts := []grpc.ServerOption{grpc.StatsHandler(otelgrpc.NewServerHandler())}
	if certDir := os.Getenv("TLS_CERT_DIR"); certDir != "" {
		tlsCfg, tlsErr := tlsconfig.ServerTLS(certDir)
		if tlsErr != nil {
			log.Fatalf("server tls: %v", tlsErr)
		}
		grpcOpts = append(grpcOpts, grpc.Creds(credentials.NewTLS(tlsCfg)))
		slog.Info("mTLS enabled for gRPC server", "certDir", certDir)
	}
	grpcSrv := grpc.NewServer(grpcOpts...)
	pb.RegisterPaymentServiceServer(grpcSrv, grpcserver.NewServer(paymentSvc))
	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(grpcSrv, healthSrv)
	healthSrv.SetServingStatus("payment.v1.PaymentService", healthpb.HealthCheckResponse_SERVING)
	reflection.Register(grpcSrv)

	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		log.Fatalf("gRPC listen: %v", err)
	}

	go func() {
		slog.Info("gRPC server starting", "port", cfg.GRPCPort)
		if err := grpcSrv.Serve(lis); err != nil {
			log.Fatalf("gRPC server failed: %v", err)
		}
	}()

	// Graceful shutdown — priority order: cancel context and drain servers first,
	// then wait for background work, then close connections, then flush telemetry.
	sm := shutdown.New(15 * time.Second)

	sm.Register("cancel-ctx", 0, func(_ context.Context) error {
		cancel()
		return nil
	})
	sm.Register("drain-http", 0, shutdown.DrainHTTP("payment-http", httpSrv))
	sm.Register("drain-grpc", 0, shutdown.DrainGRPC("payment-grpc", grpcSrv))

	sm.Register("wait-outbox", 10, shutdown.WaitForInflight("payment-outbox", poller.IsIdle, 100*time.Millisecond))

	sm.Register("postgres", 20, func(_ context.Context) error {
		pool.Close()
		return nil
	})
	sm.Register("rabbitmq", 20, func(_ context.Context) error {
		_ = ch.Close()
		return conn.Close()
	})

	if kafkaWriter != nil {
		sm.Register("kafka", 20, func(_ context.Context) error {
			return kafkaWriter.Close()
		})
	}

	sm.Register("otel", 30, func(ctx context.Context) error {
		return shutdownTracer(ctx)
	})

	sm.Wait()
}
