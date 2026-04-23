package paymentclient

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/kabradshaw1/portfolio/go/pkg/grpcmetrics"
	pb "github.com/kabradshaw1/portfolio/go/payment-service/pb/payment/v1"
)

// GRPCClient wraps the payment-service gRPC client for creating and
// refunding payments during the checkout saga.
type GRPCClient struct {
	conn   *grpc.ClientConn
	client pb.PaymentServiceClient
}

// New dials the payment-service gRPC endpoint and returns a ready client.
func New(addr string, creds credentials.TransportCredentials) (*GRPCClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(creds),
		grpc.WithUnaryInterceptor(grpcmetrics.UnaryClientInterceptor("payment-service")),
	)
	if err != nil {
		return nil, fmt.Errorf("payment grpc dial: %w", err)
	}
	return &GRPCClient{conn: conn, client: pb.NewPaymentServiceClient(conn)}, nil
}

// Close releases the underlying gRPC connection.
func (c *GRPCClient) Close() error { return c.conn.Close() }

// CreatePayment calls the payment-service to create a Stripe checkout session.
// Returns the checkout session URL for redirect.
func (c *GRPCClient) CreatePayment(ctx context.Context, orderID uuid.UUID, amountCents int, currency, successURL, cancelURL string) (string, error) {
	resp, err := c.client.CreatePayment(ctx, &pb.CreatePaymentRequest{
		OrderId:    orderID.String(),
		AmountCents: int32(amountCents),
		Currency:   currency,
		SuccessUrl: successURL,
		CancelUrl:  cancelURL,
	})
	if err != nil {
		return "", fmt.Errorf("create payment: %w", err)
	}
	return resp.CheckoutSessionUrl, nil
}

// RefundPayment calls the payment-service to refund a payment for the given order.
func (c *GRPCClient) RefundPayment(ctx context.Context, orderID uuid.UUID, reason string) error {
	_, err := c.client.RefundPayment(ctx, &pb.RefundPaymentRequest{
		OrderId: orderID.String(),
		Reason:  reason,
	})
	return err
}
