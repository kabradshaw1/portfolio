package rabbitmq_test

import (
	"context"
	"errors"
	"testing"

	rabbitmq "github.com/kabradshaw1/portfolio/go/pkg/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
)

type reconnectingFakePublisher struct {
	err        error
	exchange   string
	routingKey string
	msg        amqp.Publishing
}

func (f *reconnectingFakePublisher) Publish(_ context.Context, exchange, routingKey string, msg amqp.Publishing) error {
	f.exchange = exchange
	f.routingKey = routingKey
	f.msg = msg
	return f.err
}

func TestReconnectingPublisherReturnsUnavailableUntilPublisherIsSet(t *testing.T) {
	publisher := rabbitmq.NewReconnectingPublisher()

	err := publisher.Publish(context.Background(), "exchange", "key", amqp.Publishing{})
	if !errors.Is(err, rabbitmq.ErrPublisherUnavailable) {
		t.Fatalf("Publish error = %v, want ErrPublisherUnavailable", err)
	}
}

func TestReconnectingPublisherUsesCurrentPublisher(t *testing.T) {
	publisher := rabbitmq.NewReconnectingPublisher()
	current := &reconnectingFakePublisher{}
	publisher.SetPublisher(current)

	err := publisher.Publish(context.Background(), "exchange", "key", amqp.Publishing{Body: []byte("body")})
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	if current.exchange != "exchange" || current.routingKey != "key" || string(current.msg.Body) != "body" {
		t.Fatalf("Publish forwarded %q/%q/%q", current.exchange, current.routingKey, current.msg.Body)
	}
}

func TestReconnectingPublisherReturnsUnavailableAfterClear(t *testing.T) {
	publisher := rabbitmq.NewReconnectingPublisher()
	publisher.SetPublisher(&reconnectingFakePublisher{})
	publisher.SetUnavailable(errors.New("channel closed"))

	err := publisher.Publish(context.Background(), "exchange", "key", amqp.Publishing{})
	if !errors.Is(err, rabbitmq.ErrPublisherUnavailable) {
		t.Fatalf("Publish error = %v, want ErrPublisherUnavailable", err)
	}
}
