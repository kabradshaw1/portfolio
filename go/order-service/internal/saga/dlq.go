package saga

import (
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// DLQMessage represents a message sitting in the dead-letter queue.
type DLQMessage struct {
	Index      int                    `json:"index"`
	RoutingKey string                 `json:"routing_key"`
	Exchange   string                 `json:"exchange"`
	Timestamp  time.Time              `json:"timestamp"`
	RetryCount int32                  `json:"retry_count"`
	Headers    map[string]interface{} `json:"headers"`
	Body       json.RawMessage        `json:"body"`
}

// maxDLQList is the upper bound on messages returned by List.
const maxDLQList = 200

// DLQClient provides operations on the saga dead-letter queue.
type DLQClient struct {
	ch *amqp.Channel
}

// NewDLQClient creates a DLQ client wrapping the given channel.
func NewDLQClient(ch *amqp.Channel) *DLQClient {
	return &DLQClient{ch: ch}
}

// List peeks at up to limit messages from the DLQ without removing them.
// Messages are fetched via basic.get and immediately nacked with requeue.
func (d *DLQClient) List(limit int) ([]DLQMessage, error) {
	if limit <= 0 || limit > maxDLQList {
		limit = 50
	}

	var messages []DLQMessage

	for i := 0; i < limit; i++ {
		msg, ok, err := d.ch.Get(SagaDLQ, false) // autoAck=false
		if err != nil {
			return nil, fmt.Errorf("get from DLQ: %w", err)
		}
		if !ok {
			break // queue is empty
		}

		var retryCount int32
		if rc, exists := msg.Headers["x-retry-count"]; exists {
			if v, ok := rc.(int32); ok {
				retryCount = v
			}
		}

		// Extract original routing key and exchange from x-death headers.
		routingKey, exchange := extractXDeath(msg.Headers)
		if routingKey == "" {
			routingKey = msg.RoutingKey
		}
		if exchange == "" {
			exchange = msg.Exchange
		}

		messages = append(messages, DLQMessage{
			Index:      i,
			RoutingKey: routingKey,
			Exchange:   exchange,
			Timestamp:  msg.Timestamp,
			RetryCount: retryCount,
			Headers:    msg.Headers,
			Body:       json.RawMessage(msg.Body),
		})

		// Requeue the message so it stays in DLQ.
		if err := msg.Nack(false, true); err != nil {
			return nil, fmt.Errorf("nack DLQ message: %w", err)
		}
	}

	return messages, nil
}

// extractXDeath reads the original routing key and exchange from RabbitMQ's
// x-death header, which is automatically added when a message is dead-lettered.
func extractXDeath(headers amqp.Table) (routingKey, exchange string) {
	xdeath, ok := headers["x-death"]
	if !ok {
		return "", ""
	}

	deaths, ok := xdeath.([]interface{})
	if !ok || len(deaths) == 0 {
		return "", ""
	}

	first, ok := deaths[0].(amqp.Table)
	if !ok {
		return "", ""
	}

	if rks, ok := first["routing-keys"].([]interface{}); ok && len(rks) > 0 {
		if rk, ok := rks[0].(string); ok {
			routingKey = rk
		}
	}
	if ex, ok := first["exchange"].(string); ok {
		exchange = ex
	}

	return routingKey, exchange
}

// Replay removes the message at the given index from the DLQ and republishes
// it to its original exchange with the original routing key.
func (d *DLQClient) Replay(index int) (*DLQMessage, error) {
	if index < 0 {
		return nil, fmt.Errorf("invalid index: %d", index)
	}

	// Consume messages up to the target index. Non-target messages are
	// nacked with requeue so they remain in the DLQ.
	for i := 0; i <= index; i++ {
		msg, ok, err := d.ch.Get(SagaDLQ, false)
		if err != nil {
			return nil, fmt.Errorf("get from DLQ at position %d: %w", i, err)
		}
		if !ok {
			return nil, fmt.Errorf("DLQ exhausted at position %d, target index %d not found", i, index)
		}

		if i < index {
			// Not the target — put it back.
			if err := msg.Nack(false, true); err != nil {
				return nil, fmt.Errorf("nack non-target message at %d: %w", i, err)
			}
			continue
		}

		// This is the target message. Ack to remove from DLQ.
		if err := msg.Ack(false); err != nil {
			return nil, fmt.Errorf("ack target message: %w", err)
		}

		// Extract original destination.
		routingKey, exchange := extractXDeath(msg.Headers)
		if routingKey == "" {
			routingKey = msg.RoutingKey
		}
		if exchange == "" {
			exchange = SagaExchange
		}

		// Increment retry count.
		if msg.Headers == nil {
			msg.Headers = make(amqp.Table)
		}
		var retryCount int32
		if rc, ok := msg.Headers["x-retry-count"].(int32); ok {
			retryCount = rc
		}
		retryCount++
		msg.Headers["x-retry-count"] = retryCount

		// Republish to original destination.
		err = d.ch.Publish(exchange, routingKey, false, false, amqp.Publishing{
			ContentType: msg.ContentType,
			Headers:     msg.Headers,
			Body:        msg.Body,
			Timestamp:   msg.Timestamp,
		})
		if err != nil {
			return nil, fmt.Errorf("republish to %s/%s: %w", exchange, routingKey, err)
		}

		SagaDLQReplayed.WithLabelValues(routingKey, "success").Inc()

		return &DLQMessage{
			Index:      index,
			RoutingKey: routingKey,
			Exchange:   exchange,
			Timestamp:  msg.Timestamp,
			RetryCount: retryCount,
			Headers:    msg.Headers,
			Body:       json.RawMessage(msg.Body),
		}, nil
	}

	return nil, fmt.Errorf("index %d not reached", index)
}
