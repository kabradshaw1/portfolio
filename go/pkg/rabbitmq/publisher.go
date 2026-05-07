package rabbitmq

import (
	"context"
	"fmt"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

const publisherSequenceHeader = "x-publisher-confirm-sequence"

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
	ch              ConfirmChannel
	confirms        <-chan amqp.Confirmation
	returns         <-chan amqp.Return
	publishMux      sync.Mutex
	nextDeliveryTag uint64
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

	p.drainReturns()
	expectedDeliveryTag := p.nextDeliveryTag + 1

	if msg.DeliveryMode == 0 {
		msg.DeliveryMode = amqp.Persistent
	}
	msg = publishingWithSequence(msg, expectedDeliveryTag)
	if err := p.ch.PublishWithContext(ctx, exchange, routingKey, true, false, msg); err != nil {
		return fmt.Errorf("publish rabbitmq message: %w", err)
	}
	p.nextDeliveryTag = expectedDeliveryTag

	returns := p.returns
	for {
		select {
		case ret, ok := <-returns:
			if ok && returnMatchesSequence(ret, expectedDeliveryTag) {
				return fmt.Errorf("rabbitmq message returned: %s", ret.ReplyText)
			}
			if !ok {
				returns = nil
			}
		case confirmation, ok := <-p.confirms:
			if !ok {
				return fmt.Errorf("rabbitmq confirm channel closed")
			}
			if confirmation.DeliveryTag < expectedDeliveryTag {
				continue
			}
			if confirmation.DeliveryTag > expectedDeliveryTag {
				return fmt.Errorf("rabbitmq unexpected confirm delivery tag %d, want %d", confirmation.DeliveryTag, expectedDeliveryTag)
			}
			if !confirmation.Ack {
				return fmt.Errorf("rabbitmq publish nack")
			}
			if ret, ok := currentReturn(returns, expectedDeliveryTag); ok {
				return fmt.Errorf("rabbitmq message returned: %s", ret.ReplyText)
			}
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func publishingWithSequence(msg amqp.Publishing, seq uint64) amqp.Publishing {
	headers := amqp.Table{}
	for key, value := range msg.Headers {
		headers[key] = value
	}
	headers[publisherSequenceHeader] = int64(seq)
	msg.Headers = headers
	return msg
}

func currentReturn(returns <-chan amqp.Return, expectedDeliveryTag uint64) (amqp.Return, bool) {
	for {
		select {
		case ret, ok := <-returns:
			if !ok {
				return amqp.Return{}, false
			}
			if returnMatchesSequence(ret, expectedDeliveryTag) {
				return ret, true
			}
		default:
			return amqp.Return{}, false
		}
	}
}

func returnMatchesSequence(ret amqp.Return, expectedDeliveryTag uint64) bool {
	seq, ok := ret.Headers[publisherSequenceHeader]
	if !ok {
		return false
	}
	switch v := seq.(type) {
	case int:
		return uint64(v) == expectedDeliveryTag
	case int32:
		return uint64(v) == expectedDeliveryTag
	case int64:
		return uint64(v) == expectedDeliveryTag
	case uint64:
		return v == expectedDeliveryTag
	default:
		return false
	}
}

func (p *Publisher) drainReturns() {
	for {
		select {
		case _, ok := <-p.returns:
			if !ok {
				return
			}
		default:
			return
		}
	}
}
