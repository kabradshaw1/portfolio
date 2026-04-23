package handler

import (
	"context"
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

// WebhookService processes validated Stripe webhook events.
type WebhookService interface {
	HandlePaymentSucceeded(ctx context.Context, eventID, intentID string) error
	HandlePaymentFailed(ctx context.Context, eventID, intentID string) error
	HandleRefund(ctx context.Context, eventID, intentID string) error
}

// EventVerifier validates the Stripe-Signature header and extracts event fields.
type EventVerifier interface {
	VerifyAndParse(payload []byte, sigHeader string) (eventType, eventID, intentID string, err error)
}

// WebhookHandler handles incoming Stripe webhook events.
type WebhookHandler struct {
	svc      WebhookService
	verifier EventVerifier
}

// NewWebhookHandler creates a WebhookHandler with the given service and verifier.
func NewWebhookHandler(svc WebhookService, verifier EventVerifier) *WebhookHandler {
	return &WebhookHandler{svc: svc, verifier: verifier}
}

// HandleWebhook reads the raw body, verifies the Stripe signature, and routes
// the event to the appropriate service method.
func (h *WebhookHandler) HandleWebhook(c *gin.Context) {
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		_ = c.Error(apperror.BadRequest("WEBHOOK_READ_FAILED", "failed to read webhook body"))
		return
	}

	sigHeader := c.GetHeader("Stripe-Signature")
	eventType, eventID, intentID, err := h.verifier.VerifyAndParse(payload, sigHeader)
	if err != nil {
		_ = c.Error(apperror.BadRequest("WEBHOOK_INVALID_SIGNATURE", "webhook signature verification failed"))
		return
	}

	ctx := c.Request.Context()
	slog.InfoContext(ctx, "webhook event received", "eventType", eventType, "eventID", eventID, "intentID", intentID)

	switch eventType {
	case "payment_intent.succeeded":
		if err := h.svc.HandlePaymentSucceeded(ctx, eventID, intentID); err != nil {
			metrics.WebhookEvents.WithLabelValues(eventType, "error").Inc()
			_ = c.Error(err)
			return
		}
		metrics.WebhookEvents.WithLabelValues(eventType, "processed").Inc()
	case "payment_intent.payment_failed":
		if err := h.svc.HandlePaymentFailed(ctx, eventID, intentID); err != nil {
			metrics.WebhookEvents.WithLabelValues(eventType, "error").Inc()
			_ = c.Error(err)
			return
		}
		metrics.WebhookEvents.WithLabelValues(eventType, "processed").Inc()
	case "charge.refunded":
		if err := h.svc.HandleRefund(ctx, eventID, intentID); err != nil {
			metrics.WebhookEvents.WithLabelValues(eventType, "error").Inc()
			_ = c.Error(err)
			return
		}
		metrics.WebhookEvents.WithLabelValues(eventType, "processed").Inc()
	default:
		slog.InfoContext(ctx, "received unknown webhook event type, ignoring", "eventType", eventType, "eventID", eventID)
	}

	c.JSON(http.StatusOK, gin.H{"received": true})
}
