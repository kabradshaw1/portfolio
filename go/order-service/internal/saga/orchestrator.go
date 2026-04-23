package saga

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/kafka"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/model"
)

// OrderRepository abstracts order persistence for the saga.
type OrderRepository interface {
	FindByID(ctx context.Context, id uuid.UUID) (*model.Order, error)
	UpdateSagaStep(ctx context.Context, orderID uuid.UUID, step string) error
	UpdateStatus(ctx context.Context, orderID uuid.UUID, status model.OrderStatus) error
	UpdateCheckoutURL(ctx context.Context, orderID uuid.UUID, url string) error
}

// SagaPublisher abstracts RabbitMQ command publishing.
type SagaPublisher interface {
	PublishCommand(ctx context.Context, cmd Command) error
}

// StockChecker abstracts product-service stock validation.
type StockChecker interface {
	CheckAvailability(ctx context.Context, productID uuid.UUID, quantity int) (bool, error)
}

// PaymentCreator abstracts payment-service interactions for the saga.
type PaymentCreator interface {
	CreatePayment(ctx context.Context, orderID uuid.UUID, amountCents int, currency, successURL, cancelURL string) (string, error)
	RefundPayment(ctx context.Context, orderID uuid.UUID, reason string) error
}

// Orchestrator drives the checkout saga state machine.
type Orchestrator struct {
	repo        OrderRepository
	pub         SagaPublisher
	stock       StockChecker
	payment     PaymentCreator
	kafkaPub    kafka.Producer
	frontendURL string // used to build Stripe success/cancel redirect URLs
}

// NewOrchestrator creates a saga orchestrator.
func NewOrchestrator(repo OrderRepository, pub SagaPublisher, stock StockChecker, payment PaymentCreator, kafkaPub kafka.Producer, frontendURL string) *Orchestrator {
	return &Orchestrator{repo: repo, pub: pub, stock: stock, payment: payment, kafkaPub: kafkaPub, frontendURL: frontendURL}
}

// Advance moves the saga forward from its current step.
func (o *Orchestrator) Advance(ctx context.Context, orderID uuid.UUID) error {
	order, err := o.repo.FindByID(ctx, orderID)
	if err != nil {
		return fmt.Errorf("find order: %w", err)
	}

	slog.InfoContext(ctx, "advancing saga", "orderID", orderID, "currentStep", order.SagaStep)

	start := time.Now()
	var stepErr error

	switch order.SagaStep {
	case StepCreated:
		stepErr = o.handleCreated(ctx, order)
	case StepItemsReserved:
		stepErr = o.handleItemsReserved(ctx, order)
	case StepStockValidated:
		stepErr = o.handleStockValidated(ctx, order)
	case StepPaymentCreated:
		return nil // Waiting for webhook confirmation via outbox poller
	case StepPaymentConfirmed:
		stepErr = o.handlePaymentConfirmed(ctx, order)
	case StepCompensating:
		return nil // Compensation command already sent, waiting for reply
	case StepCompleted, StepCompensationComplete, StepFailed:
		return nil // Terminal states
	default:
		SagaStepsTotal.WithLabelValues(order.SagaStep, "error").Inc()
		return fmt.Errorf("unknown saga step: %s", order.SagaStep)
	}

	elapsed := time.Since(start)
	outcome := "success"
	if stepErr != nil {
		outcome = "error"
	}
	SagaStepDuration.WithLabelValues(order.SagaStep, outcome).Observe(elapsed.Seconds())

	return stepErr
}

func (o *Orchestrator) handleCreated(ctx context.Context, order *model.Order) error {
	items := make([]CommandItem, len(order.Items))
	for i, item := range order.Items {
		items[i] = CommandItem{
			ProductID: item.ProductID.String(),
			Quantity:  item.Quantity,
		}
	}

	SagaStepsTotal.WithLabelValues(StepCreated, "success").Inc()

	return o.pub.PublishCommand(ctx, Command{
		Command: CmdReserveItems,
		OrderID: order.ID.String(),
		UserID:  order.UserID.String(),
		Items:   items,
	})
}

func (o *Orchestrator) handleItemsReserved(ctx context.Context, order *model.Order) error {
	for _, item := range order.Items {
		available, err := o.stock.CheckAvailability(ctx, item.ProductID, item.Quantity)
		if err != nil {
			return fmt.Errorf("check stock for %s: %w", item.ProductID, err)
		}
		if !available {
			slog.WarnContext(ctx, "stock insufficient, compensating",
				"orderID", order.ID, "productID", item.ProductID)
			SagaStepsTotal.WithLabelValues(StepItemsReserved, "error").Inc()
			return o.compensate(ctx, order)
		}
	}

	if err := o.repo.UpdateSagaStep(ctx, order.ID, StepStockValidated); err != nil {
		return err
	}
	order.SagaStep = StepStockValidated
	SagaStepsTotal.WithLabelValues(StepStockValidated, "success").Inc()

	return o.Advance(ctx, order.ID)
}

func (o *Orchestrator) handleStockValidated(ctx context.Context, order *model.Order) error {
	if o.payment != nil {
		payCtx, payCancel := context.WithTimeout(ctx, 30*time.Second)
		defer payCancel()
		_, err := o.payment.CreatePayment(payCtx, order.ID, order.Total, "usd",
			"https://kylebradshaw.dev/go/ecommerce/checkout/success?order="+order.ID.String(),
			"https://kylebradshaw.dev/go/ecommerce/checkout/cancel?order="+order.ID.String(),
		)
		if err != nil {
			slog.ErrorContext(ctx, "create payment failed, compensating",
				"orderID", order.ID, "error", err)
			SagaStepsTotal.WithLabelValues(StepStockValidated, "error").Inc()
			return o.compensate(ctx, order)
		}
		if err := o.repo.UpdateSagaStep(ctx, order.ID, StepPaymentCreated); err != nil {
			return err
		}
		SagaStepsTotal.WithLabelValues(StepPaymentCreated, "success").Inc()
		return nil // Wait for webhook confirmation
	}
	// No payment service configured — skip payment (dev mode).
	return o.handlePaymentConfirmed(ctx, order)
}

func (o *Orchestrator) handlePaymentConfirmed(ctx context.Context, order *model.Order) error {
	if err := o.repo.UpdateSagaStep(ctx, order.ID, StepPaymentConfirmed); err != nil {
		return err
	}
	SagaStepsTotal.WithLabelValues(StepPaymentConfirmed, "success").Inc()

	return o.pub.PublishCommand(ctx, Command{
		Command: CmdClearCart,
		OrderID: order.ID.String(),
		UserID:  order.UserID.String(),
	})
}

func (o *Orchestrator) completeOrder(ctx context.Context, orderID uuid.UUID) error {
	order, err := o.repo.FindByID(ctx, orderID)
	if err != nil {
		return fmt.Errorf("find order for completion: %w", err)
	}

	if err := o.repo.UpdateStatus(ctx, order.ID, model.OrderStatusCompleted); err != nil {
		return err
	}
	if err := o.repo.UpdateSagaStep(ctx, order.ID, StepCompleted); err != nil {
		return err
	}

	SagaStepsTotal.WithLabelValues(StepCompleted, "success").Inc()
	SagaDuration.Observe(time.Since(order.CreatedAt).Seconds())

	slog.InfoContext(ctx, "saga completed", "orderID", order.ID)

	// Publish Kafka analytics event (fire-and-forget).
	if o.kafkaPub != nil {
		type itemData struct {
			ProductID  string `json:"productID"`
			Quantity   int    `json:"quantity"`
			PriceCents int    `json:"priceCents"`
		}
		items := make([]itemData, len(order.Items))
		for i, oi := range order.Items {
			items[i] = itemData{
				ProductID:  oi.ProductID.String(),
				Quantity:   oi.Quantity,
				PriceCents: oi.PriceAtPurchase,
			}
		}
		kafka.SafePublish(ctx, o.kafkaPub, "ecommerce.orders", order.UserID.String(), kafka.Event{
			Type: "order.completed",
			Data: map[string]any{
				"orderID":    order.ID.String(),
				"userID":     order.UserID.String(),
				"totalCents": order.Total,
				"items":      items,
			},
		})
	}

	return nil
}

func (o *Orchestrator) compensate(ctx context.Context, order *model.Order) error {
	// Refund if payment was already created or confirmed.
	if o.payment != nil && (order.SagaStep == StepPaymentConfirmed || order.SagaStep == StepPaymentCreated) {
		if err := o.payment.RefundPayment(ctx, order.ID, "saga compensation"); err != nil {
			slog.ErrorContext(ctx, "refund failed during compensation",
				"orderID", order.ID, "error", err)
			SagaStepsTotal.WithLabelValues("refund", "error").Inc()
		}
	}

	if err := o.repo.UpdateStatus(ctx, order.ID, model.OrderStatusFailed); err != nil {
		return err
	}
	if err := o.repo.UpdateSagaStep(ctx, order.ID, StepCompensating); err != nil {
		return err
	}
	order.Status = model.OrderStatusFailed
	order.SagaStep = StepCompensating
	SagaStepsTotal.WithLabelValues(StepCompensating, "success").Inc()

	return o.pub.PublishCommand(ctx, Command{
		Command: CmdReleaseItems,
		OrderID: order.ID.String(),
		UserID:  order.UserID.String(),
	})
}

// HandleEvent processes a saga reply event and advances the saga.
func (o *Orchestrator) HandleEvent(ctx context.Context, evt Event) error {
	orderID, err := uuid.Parse(evt.OrderID)
	if err != nil {
		return fmt.Errorf("parse order ID: %w", err)
	}

	slog.InfoContext(ctx, "handling saga event", "event", evt.Event, "orderID", evt.OrderID)

	switch evt.Event {
	case EvtItemsReserved:
		if err := o.repo.UpdateSagaStep(ctx, orderID, StepItemsReserved); err != nil {
			return err
		}
		SagaStepsTotal.WithLabelValues(StepItemsReserved, "success").Inc()
		return o.Advance(ctx, orderID)

	case EvtPaymentConfirmed:
		if err := o.repo.UpdateSagaStep(ctx, orderID, StepPaymentConfirmed); err != nil {
			return err
		}
		SagaStepsTotal.WithLabelValues(StepPaymentConfirmed, "success").Inc()
		return o.Advance(ctx, orderID)

	case EvtPaymentFailed:
		SagaStepsTotal.WithLabelValues(StepPaymentCreated, "error").Inc()
		order, err := o.repo.FindByID(ctx, orderID)
		if err != nil {
			return fmt.Errorf("find order for payment failure: %w", err)
		}
		return o.compensate(ctx, order)

	case EvtCartCleared:
		return o.completeOrder(ctx, orderID)

	case EvtItemsReleased:
		SagaStepsTotal.WithLabelValues(StepCompensationComplete, "success").Inc()
		return o.repo.UpdateSagaStep(ctx, orderID, StepCompensationComplete)

	default:
		SagaStepsTotal.WithLabelValues("unknown_event", "error").Inc()
		return fmt.Errorf("unknown saga event: %s", evt.Event)
	}
}
