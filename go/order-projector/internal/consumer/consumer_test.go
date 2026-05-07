package consumer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
)

type fakeReader struct {
	msg        kafka.Message
	fetchErr   error
	committed  bool
	commitErr  error
	fetchCalled bool
}

func (r *fakeReader) FetchMessage(context.Context) (kafka.Message, error) {
	r.fetchCalled = true
	return r.msg, r.fetchErr
}

func (r *fakeReader) CommitMessages(context.Context, ...kafka.Message) error {
	if r.commitErr != nil {
		return r.commitErr
	}
	r.committed = true
	return nil
}

func (r *fakeReader) Close() error { return nil }

type fakeProcessor struct {
	err      error
	received bool
}

func (p *fakeProcessor) ProcessOrderEvent(context.Context, *OrderEventMessage) error {
	p.received = true
	return p.err
}

type fakeDLQ struct {
	err       error
	published bool
}

func (d *fakeDLQ) Publish(context.Context, kafka.Message, string, error) error {
	if d.err != nil {
		return d.err
	}
	d.published = true
	return nil
}

func TestProcessOneDoesNotCommitOnProcessingError(t *testing.T) {
	reader := &fakeReader{msg: kafka.Message{Topic: "ecommerce.order-events", Value: validOrderEventJSON(t)}}
	processor := &fakeProcessor{err: errors.New("postgres unavailable")}
	cons := NewWithDependencies(reader, processor, nil, Config{
		GroupID:     "order-projector-group",
		RetryConfig: RetryConfig{MaxAttempts: 1, BaseDelay: time.Nanosecond, MaxDelay: time.Nanosecond},
	})

	err := cons.processOne(context.Background())
	if err == nil {
		t.Fatal("expected processing error")
	}
	if reader.committed {
		t.Fatal("source offset was committed after processing failure")
	}
}

func TestProcessOneCommitsAfterPoisonRecordDLQ(t *testing.T) {
	reader := &fakeReader{msg: kafka.Message{Topic: "ecommerce.order-events", Value: []byte(`{bad json`)}}
	dlq := &fakeDLQ{}
	cons := NewWithDependencies(reader, &fakeProcessor{}, dlq, Config{
		GroupID:     "order-projector-group",
		RetryConfig: RetryConfig{MaxAttempts: 1, BaseDelay: time.Nanosecond, MaxDelay: time.Nanosecond},
	})

	err := cons.processOne(context.Background())
	if err != nil {
		t.Fatalf("processOne returned error: %v", err)
	}
	if !dlq.published {
		t.Fatal("poison record was not published to DLQ")
	}
	if !reader.committed {
		t.Fatal("source offset was not committed after DLQ publish")
	}
}

func validOrderEventJSON(t *testing.T) []byte {
	t.Helper()
	return []byte(`{
		"id":"00000000-0000-0000-0000-000000000001",
		"type":"order.created",
		"version":2,
		"source":"order-service",
		"order_id":"00000000-0000-0000-0000-000000000002",
		"timestamp":"2026-05-07T12:00:00Z",
		"traceID":"trace-1",
		"data":{"userID":"00000000-0000-0000-0000-000000000003","totalCents":1200,"currency":"USD","items":[]}
	}`)
}
