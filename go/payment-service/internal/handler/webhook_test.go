package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

// --- mocks ---

type mockWebhookService struct {
	succeededErr error
	failedErr    error
	refundErr    error
	calls        []string
}

func (m *mockWebhookService) HandlePaymentSucceeded(_ context.Context, _, _ string) error {
	m.calls = append(m.calls, "succeeded")
	return m.succeededErr
}

func (m *mockWebhookService) HandlePaymentFailed(_ context.Context, _, _ string) error {
	m.calls = append(m.calls, "failed")
	return m.failedErr
}

func (m *mockWebhookService) HandleRefund(_ context.Context, _, _ string) error {
	m.calls = append(m.calls, "refund")
	return m.refundErr
}

type mockEventVerifier struct {
	eventType string
	eventID   string
	intentID  string
	err       error
}

func (m *mockEventVerifier) VerifyAndParse(_ []byte, _ string) (string, string, string, error) {
	return m.eventType, m.eventID, m.intentID, m.err
}

// --- helpers ---

func setupWebhookRouter(svc WebhookService, verifier EventVerifier) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(apperror.ErrorHandler())
	h := NewWebhookHandler(svc, verifier)
	r.POST("/webhook", h.HandleWebhook)
	return r
}

// --- tests ---

func TestWebhookHandler_InvalidSignature(t *testing.T) {
	svc := &mockWebhookService{}
	verifier := &mockEventVerifier{err: errors.New("invalid signature")}
	router := setupWebhookRouter(svc, verifier)

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`{}`))
	req.Header.Set("Stripe-Signature", "bad-sig")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestWebhookHandler_PaymentSucceeded(t *testing.T) {
	svc := &mockWebhookService{}
	verifier := &mockEventVerifier{
		eventType: "payment_intent.succeeded",
		eventID:   "evt_123",
		intentID:  "pi_123",
	}
	router := setupWebhookRouter(svc, verifier)

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`{}`))
	req.Header.Set("Stripe-Signature", "v1=abc")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if len(svc.calls) != 1 || svc.calls[0] != "succeeded" {
		t.Errorf("expected succeeded call, got %v", svc.calls)
	}
}
