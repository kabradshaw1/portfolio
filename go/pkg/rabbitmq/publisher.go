package rabbitmq

import (
	"context"
	"fmt"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

// PublisherCorrelationHeader is reserved for Publisher return correlation.
// Publish overwrites this header on every outbound message.
const PublisherCorrelationHeader = "x-publisher-correlation-id"

type ConfirmChannel interface {
	Confirm(noWait bool) error
	NotifyPublish(confirm chan amqp.Confirmation) chan amqp.Confirmation
	NotifyReturn(ret chan amqp.Return) chan amqp.Return
	GetNextPublishSeqNo() uint64
	PublishWithContext(
		ctx context.Context,
		exchange, key string,
		mandatory, immediate bool,
		msg amqp.Publishing,
	) error
}

type Publisher struct {
	ch             ConfirmChannel
	confirms       <-chan amqp.Confirmation
	returns        <-chan amqp.Return
	publishMux     sync.Mutex
	stateMux       sync.Mutex
	nextReturnID   uint64
	confirmsClosed bool
	currentWaiter  *publishWaiter
	waiters        map[uint64]*publishWaiter
}

type publishWaiter struct {
	confirmCh chan amqp.Confirmation
	returnCh  chan amqp.Return
	returnID  string
}

func NewPublisher(ch ConfirmChannel) (*Publisher, error) {
	if err := ch.Confirm(false); err != nil {
		return nil, err
	}
	p := &Publisher{
		ch:       ch,
		confirms: ch.NotifyPublish(make(chan amqp.Confirmation, 1)),
		returns:  ch.NotifyReturn(make(chan amqp.Return, 1)),
		waiters:  make(map[uint64]*publishWaiter),
	}
	go p.drainEvents()
	return p, nil
}

func (p *Publisher) Publish(ctx context.Context, exchange, routingKey string, msg amqp.Publishing) error {
	p.publishMux.Lock()
	defer p.publishMux.Unlock()

	if msg.DeliveryMode == 0 {
		msg.DeliveryMode = amqp.Persistent
	}

	expectedDeliveryTag := p.ch.GetNextPublishSeqNo()
	returnID := p.reserveReturnID()
	waiter, ok := p.registerWaiter(expectedDeliveryTag, returnID)
	if !ok {
		return fmt.Errorf("rabbitmq confirm channel closed")
	}
	defer p.unregisterWaiter(expectedDeliveryTag)

	msg = publishingWithCorrelation(msg, returnID)
	if err := p.ch.PublishWithContext(ctx, exchange, routingKey, true, false, msg); err != nil {
		return fmt.Errorf("publish rabbitmq message: %w", err)
	}

	for {
		select {
		case ret := <-waiter.returnCh:
			return fmt.Errorf("rabbitmq message returned: %s", ret.ReplyText)
		case confirmation, ok := <-waiter.confirmCh:
			if !ok {
				return fmt.Errorf("rabbitmq confirm channel closed")
			}
			if !confirmation.Ack {
				return fmt.Errorf("rabbitmq publish nack")
			}
			if ret, ok := currentReturn(waiter); ok {
				return fmt.Errorf("rabbitmq message returned: %s", ret.ReplyText)
			}
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (p *Publisher) reserveReturnID() string {
	p.stateMux.Lock()
	defer p.stateMux.Unlock()

	p.nextReturnID++
	return fmt.Sprintf("publisher-%d", p.nextReturnID)
}

func (p *Publisher) registerWaiter(deliveryTag uint64, returnID string) (*publishWaiter, bool) {
	waiter := &publishWaiter{
		confirmCh: make(chan amqp.Confirmation, 1),
		returnCh:  make(chan amqp.Return, 1),
		returnID:  returnID,
	}
	p.stateMux.Lock()
	if p.confirmsClosed {
		p.stateMux.Unlock()
		return nil, false
	}
	p.waiters[deliveryTag] = waiter
	p.currentWaiter = waiter
	p.stateMux.Unlock()
	return waiter, true
}

func (p *Publisher) unregisterWaiter(deliveryTag uint64) {
	p.stateMux.Lock()
	if p.currentWaiter == p.waiters[deliveryTag] {
		p.currentWaiter = nil
	}
	delete(p.waiters, deliveryTag)
	p.stateMux.Unlock()
}

func (p *Publisher) drainEvents() {
	confirms := p.confirms
	returns := p.returns
	for confirms != nil || returns != nil {
		select {
		case ret, ok := <-returns:
			if !ok {
				returns = nil
				continue
			}
			p.dispatchReturn(ret)
		case confirmation, ok := <-confirms:
			if !ok {
				confirms = nil
				p.closeConfirmWaiters()
				continue
			}
			p.drainReadyReturns()
			p.dispatchConfirm(confirmation)
		}
	}
}

func (p *Publisher) closeConfirmWaiters() {
	p.stateMux.Lock()
	defer p.stateMux.Unlock()

	if p.confirmsClosed {
		return
	}
	p.confirmsClosed = true
	for _, waiter := range p.waiters {
		close(waiter.confirmCh)
	}
}

func (p *Publisher) drainReadyReturns() {
	for {
		select {
		case ret, ok := <-p.returns:
			if !ok {
				return
			}
			p.dispatchReturn(ret)
		default:
			return
		}
	}
}

func (p *Publisher) dispatchConfirm(confirmation amqp.Confirmation) {
	waiter := p.waiter(confirmation.DeliveryTag)
	if waiter == nil {
		return
	}
	select {
	case waiter.confirmCh <- confirmation:
	default:
	}
}

func (p *Publisher) dispatchReturn(ret amqp.Return) {
	waiter := p.currentReturnWaiter()
	if waiter == nil {
		return
	}
	if ret.Headers[PublisherCorrelationHeader] != waiter.returnID {
		return
	}
	select {
	case waiter.returnCh <- ret:
	default:
	}
}

func (p *Publisher) currentReturnWaiter() *publishWaiter {
	p.stateMux.Lock()
	defer p.stateMux.Unlock()
	return p.currentWaiter
}

func (p *Publisher) waiter(deliveryTag uint64) *publishWaiter {
	p.stateMux.Lock()
	defer p.stateMux.Unlock()
	return p.waiters[deliveryTag]
}

func publishingWithCorrelation(msg amqp.Publishing, returnID string) amqp.Publishing {
	headers := amqp.Table{}
	for key, value := range msg.Headers {
		headers[key] = value
	}
	headers[PublisherCorrelationHeader] = returnID
	msg.Headers = headers
	return msg
}

func currentReturn(waiter *publishWaiter) (amqp.Return, bool) {
	select {
	case ret := <-waiter.returnCh:
		return ret, true
	default:
		return amqp.Return{}, false
	}
}
