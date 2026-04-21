package saga

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	SagaExchange    = "ecommerce.saga"
	SagaDLX         = "ecommerce.saga.dlx"
	SagaDLQ         = "ecommerce.saga.dlq"
	CartCommandsKey = "saga.cart.commands"
	CartCommands    = "saga.cart.commands"
	OrderEvents     = "saga.order.events"
)

// DeclareTopology sets up the saga exchanges and queues.
func DeclareTopology(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(SagaExchange, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare saga exchange: %w", err)
	}

	if err := ch.ExchangeDeclare(SagaDLX, "fanout", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare DLX: %w", err)
	}
	if _, err := ch.QueueDeclare(SagaDLQ, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare DLQ: %w", err)
	}
	if err := ch.QueueBind(SagaDLQ, "", SagaDLX, false, nil); err != nil {
		return fmt.Errorf("bind DLQ: %w", err)
	}

	if _, err := ch.QueueDeclare(CartCommands, true, false, false, false, amqp.Table{
		"x-dead-letter-exchange": SagaDLX,
	}); err != nil {
		return fmt.Errorf("declare cart commands queue: %w", err)
	}
	if err := ch.QueueBind(CartCommands, CartCommandsKey, SagaExchange, false, nil); err != nil {
		return fmt.Errorf("bind cart commands: %w", err)
	}

	if _, err := ch.QueueDeclare(OrderEvents, true, false, false, false, amqp.Table{
		"x-dead-letter-exchange": SagaDLX,
	}); err != nil {
		return fmt.Errorf("declare order events queue: %w", err)
	}
	if err := ch.QueueBind(OrderEvents, "saga.order.events", SagaExchange, false, nil); err != nil {
		return fmt.Errorf("bind order events: %w", err)
	}

	return nil
}
