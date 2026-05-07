package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

var ErrPublisherUnavailable = errors.New("rabbitmq publisher unavailable")

type PublishingClient interface {
	Publish(ctx context.Context, exchange, routingKey string, msg amqp.Publishing) error
}

type ReconnectingPublisher struct {
	mu        sync.RWMutex
	publisher PublishingClient
	err       error
}

func NewReconnectingPublisher() *ReconnectingPublisher {
	return &ReconnectingPublisher{err: ErrPublisherUnavailable}
}

func (p *ReconnectingPublisher) SetPublisher(publisher PublishingClient) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.publisher = publisher
	p.err = nil
}

func (p *ReconnectingPublisher) SetUnavailable(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.publisher = nil
	if err == nil {
		err = ErrPublisherUnavailable
	}
	p.err = err
}

func (p *ReconnectingPublisher) Publish(ctx context.Context, exchange, routingKey string, msg amqp.Publishing) error {
	p.mu.RLock()
	publisher := p.publisher
	err := p.err
	p.mu.RUnlock()

	if publisher == nil {
		if err == nil {
			err = ErrPublisherUnavailable
		}
		if errors.Is(err, ErrPublisherUnavailable) {
			return err
		}
		return fmt.Errorf("%w: %v", ErrPublisherUnavailable, err)
	}
	return publisher.Publish(ctx, exchange, routingKey, msg)
}
