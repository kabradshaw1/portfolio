package grpcmetrics

import (
	"context"
	"log/slog"
	"time"

	"github.com/kabradshaw1/portfolio/go/pkg/admission"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	clientRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "grpc_client_requests_total",
		Help: "Total outbound gRPC requests.",
	}, []string{"target", "method", "status"})

	clientRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "grpc_client_request_duration_seconds",
		Help:    "Outbound gRPC request duration.",
		Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5, 10, 30},
	}, []string{"target", "method"})

	serverInFlight = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "grpc_server_in_flight",
		Help: "Current admitted in-flight gRPC unary requests.",
	}, []string{"service", "method"})

	serverAdmissionRejections = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "grpc_server_admission_rejections_total",
		Help: "gRPC unary requests rejected by server admission control.",
	}, []string{"service", "method"})
)

// UnaryClientInterceptor returns a gRPC unary client interceptor that
// records Prometheus metrics and logs errors for every outbound call.
func UnaryClientInterceptor(target string) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		elapsed := time.Since(start)

		st, _ := status.FromError(err)
		code := st.Code().String()

		clientRequestsTotal.WithLabelValues(target, method, code).Inc()
		clientRequestDuration.WithLabelValues(target, method).Observe(elapsed.Seconds())

		if err != nil {
			slog.ErrorContext(ctx, "gRPC client call failed",
				"target", target,
				"method", method,
				"status", code,
				"duration", elapsed,
				"error", err,
			)
		}

		return err
	}
}

// UnaryServerAdmissionInterceptor returns a gRPC unary server interceptor that
// rejects saturated calls with ResourceExhausted and records admission metrics.
func UnaryServerAdmissionInterceptor(service string, limiter *admission.Limiter) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if limiter == nil {
			return handler(ctx, req)
		}
		method := "unknown"
		if info != nil && info.FullMethod != "" {
			method = info.FullMethod
		}
		permit, ok := limiter.TryAcquire(ctx)
		if !ok {
			serverAdmissionRejections.WithLabelValues(service, method).Inc()
			return nil, status.Error(codes.ResourceExhausted, "server admission limit reached")
		}
		serverInFlight.WithLabelValues(service, method).Set(float64(limiter.InFlight()))
		defer func() {
			permit.Release()
			serverInFlight.WithLabelValues(service, method).Set(float64(limiter.InFlight()))
		}()

		return handler(ctx, req)
	}
}
