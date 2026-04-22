package grpcserver_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/grpcserver"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/service"
	pb "github.com/kabradshaw1/portfolio/go/payment-service/pb/payment/v1"
)

// mockPaymentService is a test double for PaymentServiceAPI.
type mockPaymentService struct {
	createResult *service.CreatePaymentResult
	createErr    error

	statusResult *model.Payment
	statusErr    error

	refundPayment  *model.Payment
	refundID       string
	refundErr      error
}

func (m *mockPaymentService) CreatePayment(
	_ context.Context,
	_ uuid.UUID,
	_ int,
	_, _, _ string,
) (*service.CreatePaymentResult, error) {
	return m.createResult, m.createErr
}

func (m *mockPaymentService) GetPaymentStatus(_ context.Context, _ uuid.UUID) (*model.Payment, error) {
	return m.statusResult, m.statusErr
}

func (m *mockPaymentService) RefundPayment(_ context.Context, _ uuid.UUID, _ string) (*model.Payment, string, error) {
	return m.refundPayment, m.refundID, m.refundErr
}

func TestCreatePayment_HappyPath(t *testing.T) {
	orderID := uuid.New()
	paymentID := uuid.New()

	mock := &mockPaymentService{
		createResult: &service.CreatePaymentResult{
			Payment: &model.Payment{
				ID:      paymentID,
				OrderID: orderID,
				Status:  model.PaymentStatusPending,
				CreatedAt: time.Now(),
			},
			CheckoutURL: "https://checkout.stripe.com/pay/cs_test_abc123",
		},
	}

	srv := grpcserver.NewServer(mock)

	req := &pb.CreatePaymentRequest{
		OrderId:    orderID.String(),
		AmountCents: 4999,
		Currency:   "usd",
		SuccessUrl: "https://example.com/success",
		CancelUrl:  "https://example.com/cancel",
	}

	resp, err := srv.CreatePayment(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.PaymentId != paymentID.String() {
		t.Errorf("payment_id: got %q, want %q", resp.PaymentId, paymentID.String())
	}
	if resp.CheckoutSessionUrl != mock.createResult.CheckoutURL {
		t.Errorf("checkout_session_url: got %q, want %q", resp.CheckoutSessionUrl, mock.createResult.CheckoutURL)
	}
	if resp.Status != string(model.PaymentStatusPending) {
		t.Errorf("status: got %q, want %q", resp.Status, model.PaymentStatusPending)
	}
}

func TestCreatePayment_InvalidOrderID(t *testing.T) {
	srv := grpcserver.NewServer(&mockPaymentService{})

	_, err := srv.CreatePayment(context.Background(), &pb.CreatePaymentRequest{
		OrderId: "not-a-uuid",
	})
	if err == nil {
		t.Fatal("expected error for invalid order_id, got nil")
	}
}
