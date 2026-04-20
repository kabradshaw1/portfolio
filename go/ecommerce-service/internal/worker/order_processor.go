package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/kafka"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

var (
	ordersTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "orders_total",
		Help: "Total number of orders processed by status",
	}, []string{"status"})

	messagesProcessed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "rabbitmq_messages_processed_total",
		Help: "Total RabbitMQ messages processed by result",
	}, []string{"result"})
)

type OrderRepoForWorker interface {
	FindByID(ctx context.Context, id uuid.UUID) (*model.Order, error)
	UpdateStatus(ctx context.Context, orderID uuid.UUID, status model.OrderStatus) error
}

// ProductClient abstracts product-service calls (gRPC in production).
type ProductClient interface {
	DecrementStock(ctx context.Context, productID uuid.UUID, qty int) error
	InvalidateCache(ctx context.Context) error
}

type OrderProcessor struct {
	orderRepo      OrderRepoForWorker
	productClient  ProductClient
	kafkaPublisher kafka.Producer
}

func NewOrderProcessor(orderRepo OrderRepoForWorker, productClient ProductClient, kafkaPub kafka.Producer) *OrderProcessor {
	return &OrderProcessor{
		orderRepo:      orderRepo,
		productClient:  productClient,
		kafkaPublisher: kafkaPub,
	}
}

func (p *OrderProcessor) ProcessOrder(ctx context.Context, orderIDStr string) error {
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		return fmt.Errorf("parse order ID: %w", err)
	}

	order, err := p.orderRepo.FindByID(ctx, orderID)
	if err != nil {
		return fmt.Errorf("find order: %w", err)
	}

	if err := p.orderRepo.UpdateStatus(ctx, orderID, model.OrderStatusProcessing); err != nil {
		return fmt.Errorf("set processing: %w", err)
	}

	for _, item := range order.Items {
		if err := p.productClient.DecrementStock(ctx, item.ProductID, item.Quantity); err != nil {
			_ = p.orderRepo.UpdateStatus(ctx, orderID, model.OrderStatusFailed)
			ordersTotal.WithLabelValues("failed").Inc()
			kafka.SafePublish(ctx, p.kafkaPublisher, "ecommerce.orders", orderIDStr, kafka.Event{
				Type: "order.failed",
				Data: map[string]any{"orderID": orderIDStr, "userID": order.UserID.String()},
			})
			return fmt.Errorf("decrement stock for product %s: %w", item.ProductID, err)
		}
	}

	if err := p.orderRepo.UpdateStatus(ctx, orderID, model.OrderStatusCompleted); err != nil {
		return fmt.Errorf("set completed: %w", err)
	}

	if err := p.productClient.InvalidateCache(ctx); err != nil {
		log.Printf("WARN: failed to invalidate cache: %v", err)
	}

	ordersTotal.WithLabelValues("completed").Inc()
	kafka.SafePublish(ctx, p.kafkaPublisher, "ecommerce.orders", orderIDStr, kafka.Event{
		Type: "order.completed",
		Data: map[string]any{
			"orderID":    orderIDStr,
			"userID":     order.UserID.String(),
			"totalCents": order.Total,
		},
	})
	return nil
}

func (p *OrderProcessor) StartConsumer(ctx context.Context, ch *amqp.Channel, concurrency int) error {
	err := ch.ExchangeDeclare("ecommerce", "topic", true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("declare exchange: %w", err)
	}

	q, err := ch.QueueDeclare("ecommerce.orders", true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("declare queue: %w", err)
	}

	if err := ch.QueueBind(q.Name, "order.created", "ecommerce", false, nil); err != nil {
		return fmt.Errorf("bind queue: %w", err)
	}

	msgs, err := ch.Consume(q.Name, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}

	for i := 0; i < concurrency; i++ {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case msg, ok := <-msgs:
					if !ok {
						return
					}

					// Extract trace context from AMQP headers.
					headers := make(map[string]interface{})
					for k, v := range msg.Headers {
						headers[k] = v
					}
					msgCtx := tracing.ExtractAMQP(ctx, headers)
					msgCtx, span := otel.Tracer("rabbitmq").Start(msgCtx, "rabbitmq.consume",
						trace.WithAttributes(
							attribute.String("messaging.system", "rabbitmq"),
							attribute.String("messaging.destination", "ecommerce.orders"),
						),
					)

					var orderMsg model.OrderMessage
					if err := json.Unmarshal(msg.Body, &orderMsg); err != nil {
						log.Printf("ERROR: unmarshal message: %v", err)
						messagesProcessed.WithLabelValues("error").Inc()
						span.End()
						_ = msg.Nack(false, false)
						continue
					}

					if err := p.ProcessOrder(msgCtx, orderMsg.OrderID); err != nil {
						log.Printf("ERROR: process order %s: %v", orderMsg.OrderID, err)
						messagesProcessed.WithLabelValues("error").Inc()
						span.End()
						_ = msg.Nack(false, false)
						continue
					}

					messagesProcessed.WithLabelValues("success").Inc()
					span.End()
					_ = msg.Ack(false)
				}
			}
		}()
	}

	<-ctx.Done()
	return nil
}
