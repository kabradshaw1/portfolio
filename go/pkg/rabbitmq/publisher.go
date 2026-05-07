package rabbitmq

import (
	"context"
	"fmt"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

type ConfirmChannel interface {
	Confirm(noWait bool) error
	NotifyPublish(confirm chan amqp.Confirmation) chan amqp.Confirmation
	NotifyReturn(ret chan amqp.Return) chan amqp.Return
	PublishWithContext(
		ctx context.Context,
		exchange, key string,
		mandatory, immediate bool,
		msg amqp.Publishing,
	) error
}

type Publisher struct {
	ch         ConfirmChannel
	confirms   <-chan amqp.Confirmation
	returns    <-chan amqp.Return
	publishMux sync.Mutex
}

func NewPublisher(ch ConfirmChannel) (*Publisher, error) {
	if err := ch.Confirm(false); err != nil {
		return nil, err
	}
	return &Publisher{
		ch:       ch,
		confirms: ch.NotifyPublish(make(chan amqp.Confirmation, 1)),
		returns:  ch.NotifyReturn(make(chan amqp.Return, 1)),
	}, nil
}

func (p *Publisher) Publish(ctx context.Context, exchange, routingKey string, msg amqp.Publishing) error {
	p.publishMux.Lock()
	defer p.publishMux.Unlock()

	if msg.DeliveryMode == 0 {
		msg.DeliveryMode = amqp.Persistent
	}
	if err := p.ch.PublishWithContext(ctx, exchange, routingKey, true, false, msg); err != nil {
		return fmt.Errorf("publish rabbitmq message: %w", err)
	}

	for {
		select {
		case ret, ok := <-p.returns:
			if ok {
				return fmt.Errorf("rabbitmq message returned: %s", ret.ReplyText)
			}
		case confirmation, ok := <-p.confirms:
			if !ok {
				return fmt.Errorf("rabbitmq confirm channel closed")
			}
			if !confirmation.Ack {
				return fmt.Errorf("rabbitmq publish nack")
			}
			select {
			case ret, ok := <-p.returns:
				if ok {
					return fmt.Errorf("rabbitmq message returned: %s", ret.ReplyText)
				}
			default:
			}
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
