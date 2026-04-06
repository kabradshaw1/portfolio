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

	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/model"
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

type ProductRepoForWorker interface {
	DecrementStock(ctx context.Context, productID uuid.UUID, qty int) error
}

type CacheInvalidator interface {
	InvalidateCache(ctx context.Context) error
}

type OrderProcessor struct {
	orderRepo   OrderRepoForWorker
	productRepo ProductRepoForWorker
	cache       CacheInvalidator
}

func NewOrderProcessor(orderRepo OrderRepoForWorker, productRepo ProductRepoForWorker, cache CacheInvalidator) *OrderProcessor {
	return &OrderProcessor{
		orderRepo:   orderRepo,
		productRepo: productRepo,
		cache:       cache,
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
		if err := p.productRepo.DecrementStock(ctx, item.ProductID, item.Quantity); err != nil {
			_ = p.orderRepo.UpdateStatus(ctx, orderID, model.OrderStatusFailed)
			ordersTotal.WithLabelValues("failed").Inc()
			return fmt.Errorf("decrement stock for product %s: %w", item.ProductID, err)
		}
	}

	if err := p.orderRepo.UpdateStatus(ctx, orderID, model.OrderStatusCompleted); err != nil {
		return fmt.Errorf("set completed: %w", err)
	}

	if err := p.cache.InvalidateCache(ctx); err != nil {
		log.Printf("WARN: failed to invalidate cache: %v", err)
	}

	ordersTotal.WithLabelValues("completed").Inc()
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
					var orderMsg model.OrderMessage
					if err := json.Unmarshal(msg.Body, &orderMsg); err != nil {
						log.Printf("ERROR: unmarshal message: %v", err)
						messagesProcessed.WithLabelValues("error").Inc()
						_ = msg.Nack(false, false)
						continue
					}

					if err := p.ProcessOrder(ctx, orderMsg.OrderID); err != nil {
						log.Printf("ERROR: process order %s: %v", orderMsg.OrderID, err)
						messagesProcessed.WithLabelValues("error").Inc()
						_ = msg.Nack(false, false)
						continue
					}

					messagesProcessed.WithLabelValues("success").Inc()
					_ = msg.Ack(false)
				}
			}
		}()
	}

	<-ctx.Done()
	return nil
}
