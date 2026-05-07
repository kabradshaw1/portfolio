package kafkaconsumer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
)

type Writer interface {
	WriteMessages(ctx context.Context, msgs ...kafka.Message) error
}

type DLQSource struct {
	Topic     string         `json:"topic"`
	Partition int            `json:"partition"`
	Offset    int64          `json:"offset"`
	Key       []byte         `json:"key,omitempty"`
	Value     []byte         `json:"value"`
	Headers   []kafka.Header `json:"headers,omitempty"`
	Time      time.Time      `json:"time"`
}

type DLQEnvelope struct {
	Source        DLQSource `json:"source"`
	ConsumerGroup string    `json:"consumerGroup"`
	ErrorClass    string    `json:"errorClass"`
	ErrorMessage  string    `json:"errorMessage"`
	FailedAt      time.Time `json:"failedAt"`
}

type DLQPublisher struct {
	writer        Writer
	topic         string
	consumerGroup string
	now           func() time.Time
}

func NewDLQPublisher(writer Writer, topic string, consumerGroup string) *DLQPublisher {
	return &DLQPublisher{
		writer:        writer,
		topic:         topic,
		consumerGroup: consumerGroup,
		now:           time.Now,
	}
}

func (p *DLQPublisher) Publish(ctx context.Context, msg kafka.Message, errorClass string, cause error) error {
	env := DLQEnvelope{
		Source: DLQSource{
			Topic:     msg.Topic,
			Partition: msg.Partition,
			Offset:    msg.Offset,
			Key:       msg.Key,
			Value:     msg.Value,
			Headers:   msg.Headers,
			Time:      msg.Time,
		},
		ConsumerGroup: p.consumerGroup,
		ErrorClass:    errorClass,
		ErrorMessage:  cause.Error(),
		FailedAt:      p.now().UTC(),
	}
	value, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal dlq envelope: %w", err)
	}
	if err := p.writer.WriteMessages(ctx, kafka.Message{
		Topic: p.topic,
		Key:   msg.Key,
		Value: value,
	}); err != nil {
		return fmt.Errorf("publish dlq message: %w", err)
	}
	return nil
}
