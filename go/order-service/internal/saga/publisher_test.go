package saga

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
)

type fakeConfirmPublisher struct {
	err        error
	exchange   string
	routingKey string
	msg        amqp.Publishing
	calls      int
}

func (f *fakeConfirmPublisher) Publish(_ context.Context, exchange, routingKey string, msg amqp.Publishing) error {
	f.calls++
	f.exchange = exchange
	f.routingKey = routingKey
	f.msg = msg
	return f.err
}

func TestPublishCommandUsesPersistentConfirmedPublisher(t *testing.T) {
	pub := &fakeConfirmPublisher{}
	publisher := NewPublisherWithConfirmPublisher(pub)

	err := publisher.PublishCommand(context.Background(), Command{
		Command: CmdReserveItems,
		OrderID: "550e8400-e29b-41d4-a716-446655440000",
		UserID:  "650e8400-e29b-41d4-a716-446655440000",
	})

	if err != nil {
		t.Fatalf("PublishCommand returned error: %v", err)
	}
	if pub.calls != 1 {
		t.Fatalf("publish calls = %d, want 1", pub.calls)
	}
	if pub.exchange != SagaExchange || pub.routingKey != CartCommandsKey {
		t.Fatalf("published to %s/%s, want %s/%s", pub.exchange, pub.routingKey, SagaExchange, CartCommandsKey)
	}
	if pub.msg.DeliveryMode != amqp.Persistent {
		t.Fatalf("delivery mode = %d, want persistent", pub.msg.DeliveryMode)
	}
	if got := pub.msg.Headers["x-retry-count"]; got != int32(0) {
		t.Fatalf("retry header = %#v, want int32(0)", got)
	}

	var got Command
	if err := json.Unmarshal(pub.msg.Body, &got); err != nil {
		t.Fatalf("unmarshal command: %v", err)
	}
	if got.Command != CmdReserveItems || got.Timestamp.IsZero() {
		t.Fatalf("command = %+v, want reserve command with timestamp", got)
	}
}

func TestPublishCommandPropagatesConfirmPublisherError(t *testing.T) {
	want := errors.New("publish failed")
	publisher := NewPublisherWithConfirmPublisher(&fakeConfirmPublisher{err: want})

	err := publisher.PublishCommand(context.Background(), Command{Command: CmdClearCart})

	if !errors.Is(err, want) {
		t.Fatalf("PublishCommand error = %v, want %v", err, want)
	}
}
