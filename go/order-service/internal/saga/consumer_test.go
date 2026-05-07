package saga

import (
	"context"
	"testing"

	rabbitmq "github.com/kabradshaw1/portfolio/go/pkg/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
)

func TestAckDecisionPermanentErrorDeadLetters(t *testing.T) {
	headers := amqp.Table{}

	decision := DecideFailure(headers, rabbitmq.PermanentErrorf("bad payload"), 3)

	if decision.Action != FailureDeadLetter {
		t.Fatalf("action = %v, want dead-letter", decision.Action)
	}
	if decision.RetryCount != 0 {
		t.Fatalf("retry count = %d, want 0", decision.RetryCount)
	}
}

func TestAckDecisionRetryableRepublishesWithIncrementedHeader(t *testing.T) {
	headers := amqp.Table{rabbitmq.RetryCountHeader: int32(1)}

	decision := DecideFailure(headers, rabbitmq.RetryableErrorf("db down"), 3)

	if decision.Action != FailureRetryRepublish {
		t.Fatalf("action = %v, want retry republish", decision.Action)
	}
	if got := rabbitmq.RetryCount(decision.Headers); got != 2 {
		t.Fatalf("retry count = %d, want 2", got)
	}
}

func TestAckDecisionRetryableExhaustedDeadLetters(t *testing.T) {
	headers := amqp.Table{rabbitmq.RetryCountHeader: int32(3)}

	decision := DecideFailure(headers, rabbitmq.RetryableErrorf("db down"), 3)

	if decision.Action != FailureDeadLetter {
		t.Fatalf("action = %v, want dead-letter", decision.Action)
	}
}

func TestHandleMessageInvalidJSONIsPermanent(t *testing.T) {
	consumer := NewConsumer(nil)

	_, err := consumer.handleMessage(context.Background(), amqp.Delivery{Body: []byte("{")})

	if !rabbitmq.IsPermanent(err) {
		t.Fatalf("invalid json should be permanent, got %v", err)
	}
}

func TestHandleMessageInvalidOrderIDIsPermanent(t *testing.T) {
	consumer := NewConsumer(nil)
	body := []byte(`{"event":"items.reserved","order_id":"not-a-uuid"}`)

	_, err := consumer.handleMessage(context.Background(), amqp.Delivery{Body: body})

	if !rabbitmq.IsPermanent(err) {
		t.Fatalf("invalid order ID should be permanent, got %v", err)
	}
}

func TestHandleMessageUnknownEventIsPermanent(t *testing.T) {
	consumer := NewConsumer(nil)
	body := []byte(`{"event":"unknown.event","order_id":"550e8400-e29b-41d4-a716-446655440000"}`)

	_, err := consumer.handleMessage(context.Background(), amqp.Delivery{Body: body})

	if !rabbitmq.IsPermanent(err) {
		t.Fatalf("unknown event should be permanent, got %v", err)
	}
}
