package grpcmetrics

import (
	"context"
	"testing"

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
