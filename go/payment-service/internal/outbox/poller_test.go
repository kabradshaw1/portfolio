package outbox

import (
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestNewPoller(t *testing.T) {
	p := NewPoller(nil, (*amqp.Channel)(nil), time.Second, 10)
	if p == nil {
		t.Fatal("expected non-nil Poller")
	}
}
