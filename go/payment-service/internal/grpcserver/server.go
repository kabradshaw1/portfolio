package grpcserver

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/kabradshaw1/portfolio/go/payment-service/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/service"
	pb "github.com/kabradshaw1/portfolio/go/payment-service/pb/payment/v1"
)

// PaymentServiceAPI is the interface the gRPC server uses from the service layer.
type PaymentServiceAPI interface {
	CreatePayment(ctx context.Context, orderID uuid.UUID, amountCents int, currency, successURL, cancelURL string) (*service.CreatePaymentResult, error)
	GetPaymentStatus(ctx context.Context, orderID uuid.UUID) (*model.Payment, error)
	RefundPayment(ctx context.Context, orderID uuid.UUID, reason string) (*model.Payment, string, error)
}

// Server implements pb.PaymentServiceServer.
type Server struct {
	pb.UnimplementedPaymentServiceServer
	svc PaymentServiceAPI
}

// NewServer creates a Server wrapping the given PaymentServiceAPI.
func NewServer(svc PaymentServiceAPI) *Server {
	return &Server{svc: svc}
}

// CreatePayment opens a Stripe Checkout Session and returns the redirect URL.
func (s *Server) CreatePayment(ctx context.Context, req *pb.CreatePaymentRequest) (*pb.CreatePaymentResponse, error) {
	orderID, err := uuid.Parse(req.GetOrderId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid order_id: %v", err)
	}

	result, err := s.svc.CreatePayment(
		ctx,
		orderID,
		int(req.GetAmountCents()),
		req.GetCurrency(),
		req.GetSuccessUrl(),
		req.GetCancelUrl(),
	)
	if err != nil {
		metrics.PaymentsCreated.WithLabelValues("failed").Inc()
		return nil, status.Errorf(codes.Internal, "create payment: %v", err)
	}

	metrics.PaymentsCreated.WithLabelValues("succeeded").Inc()

	return &pb.CreatePaymentResponse{
		PaymentId:          result.Payment.ID.String(),
		CheckoutSessionUrl: result.CheckoutURL,
		Status:             string(result.Payment.Status),
	}, nil
}

// GetPaymentStatus returns the current payment record for an order.
func (s *Server) GetPaymentStatus(ctx context.Context, req *pb.GetPaymentStatusRequest) (*pb.GetPaymentStatusResponse, error) {
	orderID, err := uuid.Parse(req.GetOrderId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid order_id: %v", err)
	}

	payment, err := s.svc.GetPaymentStatus(ctx, orderID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get payment status: %v", err)
	}

	return &pb.GetPaymentStatusResponse{
		PaymentId:   payment.ID.String(),
		OrderId:     payment.OrderID.String(),
		Status:      string(payment.Status),
		AmountCents: int32(payment.AmountCents),
		Currency:    payment.Currency,
		CreatedAt:   timestamppb.New(payment.CreatedAt),
	}, nil
}

// RefundPayment issues a Stripe refund and transitions the payment to refunded.
func (s *Server) RefundPayment(ctx context.Context, req *pb.RefundPaymentRequest) (*pb.RefundPaymentResponse, error) {
	orderID, err := uuid.Parse(req.GetOrderId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid order_id: %v", err)
	}

	payment, refundID, err := s.svc.RefundPayment(ctx, orderID, req.GetReason())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "refund payment: %v", err)
	}

	return &pb.RefundPaymentResponse{
		PaymentId:      payment.ID.String(),
		Status:         string(payment.Status),
		StripeRefundId: refundID,
	}, nil
}
