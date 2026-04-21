package saga

import (
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestExtractXDeath_ValidHeaders(t *testing.T) {
	headers := amqp.Table{
		"x-death": []any{
			amqp.Table{
				"exchange":     "ecommerce.saga",
				"routing-keys": []any{"saga.cart.commands"},
				"queue":        "saga.cart.commands",
				"reason":       "rejected",
			},
		},
	}

	rk, ex := extractXDeath(headers)
	if rk != "saga.cart.commands" {
		t.Errorf("expected routing key saga.cart.commands, got %s", rk)
	}
	if ex != "ecommerce.saga" {
		t.Errorf("expected exchange ecommerce.saga, got %s", ex)
	}
}

func TestExtractXDeath_MissingHeader(t *testing.T) {
	rk, ex := extractXDeath(amqp.Table{})
	if rk != "" || ex != "" {
		t.Errorf("expected empty strings, got rk=%q ex=%q", rk, ex)
	}
}

func TestExtractXDeath_EmptyDeathList(t *testing.T) {
	headers := amqp.Table{
		"x-death": []any{},
	}
	rk, ex := extractXDeath(headers)
	if rk != "" || ex != "" {
		t.Errorf("expected empty strings, got rk=%q ex=%q", rk, ex)
	}
}
