package grpcmetrics

import (
	"context"
	"testing"

	"github.com/kabradshaw1/portfolio/go/pkg/admission"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestUnaryClientInterceptor_RecordsMetrics(t *testing.T) {
	interceptor := UnaryClientInterceptor("test-service")

	err := interceptor(
		context.Background(),
		"/test.Service/Method",
		nil, nil, nil,
		func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			return nil
		},
	)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestUnaryClientInterceptor_LogsErrors(t *testing.T) {
	interceptor := UnaryClientInterceptor("test-service")

	err := interceptor(
		context.Background(),
		"/test.Service/Method",
		nil, nil, nil,
		func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			return status.Error(codes.Unavailable, "connection refused")
		},
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Fatalf("expected Unavailable, got %v", st.Code())
	}
}

func TestUnaryServerAdmissionInterceptorRejectsWhenSaturated(t *testing.T) {
	limiter, err := admission.NewLimiter(1)
	if err != nil {
		t.Fatalf("NewLimiter: %v", err)
	}
	permit, ok := limiter.TryAcquire(context.Background())
	if !ok {
		t.Fatal("failed to saturate limiter")
	}
	defer permit.Release()

	interceptor := UnaryServerAdmissionInterceptor("test-service", limiter)
	_, err = interceptor(
		context.Background(),
		nil,
		&grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"},
		func(context.Context, any) (any, error) {
			t.Fatal("handler should not run when saturated")
			return nil, nil
		},
	)

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.ResourceExhausted {
		t.Fatalf("code = %v, want ResourceExhausted", st.Code())
	}
}

func TestUnaryServerAdmissionInterceptorReleasesAfterHandler(t *testing.T) {
	limiter, err := admission.NewLimiter(1)
	if err != nil {
		t.Fatalf("NewLimiter: %v", err)
	}
	interceptor := UnaryServerAdmissionInterceptor("test-service", limiter)

	_, err = interceptor(
		context.Background(),
		nil,
		&grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"},
		func(context.Context, any) (any, error) {
			if got := limiter.InFlight(); got != 1 {
				t.Fatalf("in-flight in handler = %d, want 1", got)
			}
			return "ok", nil
		},
	)
	if err != nil {
		t.Fatalf("interceptor error: %v", err)
	}
	if got := limiter.InFlight(); got != 0 {
		t.Fatalf("in-flight after handler = %d, want 0", got)
	}
}
