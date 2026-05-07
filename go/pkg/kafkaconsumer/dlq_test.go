package kafkaconsumer

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
)

type recordingWriter struct {
	msgs []kafka.Message
	err  error
}

func (w *recordingWriter) WriteMessages(_ context.Context, msgs ...kafka.Message) error {
	if w.err != nil {
		return w.err
	}
	w.msgs = append(w.msgs, msgs...)
	return nil
}

func TestDLQPublisherPublishesOriginalRecordAndError(t *testing.T) {
	t.Parallel()

	writer := &recordingWriter{}
	pub := NewDLQPublisher(writer, "orders.dlq", "order-projector-group")
	msg := kafka.Message{
		Topic:     "ecommerce.order-events",
		Partition: 2,
		Offset:    42,
		Key:       []byte("order-1"),
		Value:     []byte(`{"bad":true}`),
		Headers:   []kafka.Header{{Key: "traceparent", Value: []byte("trace")}},
		Time:      time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC),
	}

	err := pub.Publish(context.Background(), msg, "decode", errors.New("invalid json"))
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	if len(writer.msgs) != 1 {
		t.Fatalf("messages written = %d, want 1", len(writer.msgs))
	}
	if writer.msgs[0].Topic != "orders.dlq" {
		t.Fatalf("topic = %q, want orders.dlq", writer.msgs[0].Topic)
	}

	var env DLQEnvelope
	if err := json.Unmarshal(writer.msgs[0].Value, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env.ConsumerGroup != "order-projector-group" {
		t.Errorf("consumer group = %q", env.ConsumerGroup)
	}
	if env.Source.Topic != "ecommerce.order-events" || env.Source.Partition != 2 || env.Source.Offset != 42 {
		t.Errorf("source = %+v", env.Source)
	}
	if env.ErrorClass != "decode" || env.ErrorMessage != "invalid json" {
		t.Errorf("error fields = %q %q", env.ErrorClass, env.ErrorMessage)
	}
	if string(env.Source.Value) != `{"bad":true}` {
		t.Errorf("source value = %s", env.Source.Value)
	}
}

func TestDLQPublisherReturnsWriterError(t *testing.T) {
	t.Parallel()

	writer := &recordingWriter{err: errors.New("broker unavailable")}
	pub := NewDLQPublisher(writer, "orders.dlq", "order-projector-group")

	err := pub.Publish(context.Background(), kafka.Message{Topic: "source"}, "decode", errors.New("bad"))
	if err == nil {
		t.Fatal("expected writer error")
	}
}
