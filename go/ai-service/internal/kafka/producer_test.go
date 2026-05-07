package kafka

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeProducer struct {
	err error
}

func (f fakeProducer) Publish(context.Context, string, string, Event) error { return f.err }
func (f fakeProducer) Close() error                                         { return nil }

type fakeRecorder struct {
	messages  map[string]int
	durations int
	asyncMode bool
}

func (f *fakeRecorder) ObserveMessage(_, _, outcome string) {
	if f.messages == nil {
		f.messages = make(map[string]int)
	}
	f.messages[outcome]++
}

func (f *fakeRecorder) ObserveWriteDuration(_, _ string, _ time.Duration) {
	f.durations++
}

func (f *fakeRecorder) SetAsyncMode(_ string, enabled bool) {
	f.asyncMode = enabled
}

func TestSafePublishRecordsSuccessAndSwallowsErrors(t *testing.T) {
	rec := &fakeRecorder{}
	previous := metricsRecorder
	metricsRecorder = rec
	t.Cleanup(func() { metricsRecorder = previous })

	SafePublish(context.Background(), fakeProducer{}, "ecommerce.views", "key", Event{Type: "view"})
	SafePublish(context.Background(), fakeProducer{err: errors.New("write failed")}, "ecommerce.views", "key", Event{Type: "view"})

	if rec.messages["success"] != 1 {
		t.Fatalf("success count = %d, want 1", rec.messages["success"])
	}
	if rec.messages["error"] != 1 {
		t.Fatalf("error count = %d, want 1", rec.messages["error"])
	}
	if rec.durations != 2 {
		t.Fatalf("durations = %d, want 2", rec.durations)
	}
}
