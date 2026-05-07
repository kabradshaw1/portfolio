package rabbitmq_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	rabbitmq "github.com/kabradshaw1/portfolio/go/pkg/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
)

type fakeChannel struct {
	confirmErr error
	publishErr error
	confirmCh  chan amqp.Confirmation
	returnCh   chan amqp.Return
	onPublish  func(*fakeChannel)

	publishCount int
	publishing   amqp.Publishing
	mandatory    bool
	immediate    bool
	deliveryMode uint8
}

func newFakeChannel() *fakeChannel {
	return &fakeChannel{
		confirmCh: make(chan amqp.Confirmation, 4),
		returnCh:  make(chan amqp.Return, 4),
	}
}

func (f *fakeChannel) Confirm(bool) error {
	return f.confirmErr
}

func (f *fakeChannel) NotifyPublish(confirm chan amqp.Confirmation) chan amqp.Confirmation {
	if f.confirmCh == nil {
		f.confirmCh = confirm
	}
	return f.confirmCh
}

func (f *fakeChannel) NotifyReturn(ret chan amqp.Return) chan amqp.Return {
	if f.returnCh == nil {
		f.returnCh = ret
	}
	return f.returnCh
}

func (f *fakeChannel) PublishWithContext(
	_ context.Context,
	_, _ string,
	mandatory, immediate bool,
	msg amqp.Publishing,
) error {
	f.publishCount++
	f.publishing = msg
	f.mandatory = mandatory
	f.immediate = immediate
	f.deliveryMode = msg.DeliveryMode
	if f.onPublish != nil {
		f.onPublish(f)
	}
	return f.publishErr
}

func TestNewPublisherReturnsConfirmError(t *testing.T) {
	want := errors.New("confirm unavailable")
	fake := newFakeChannel()
	fake.confirmErr = want

	_, err := rabbitmq.NewPublisher(fake)

	if !errors.Is(err, want) {
		t.Fatalf("NewPublisher error = %v, want %v", err, want)
	}
}

func TestPublisherUsesMandatoryPersistentPublish(t *testing.T) {
	fake := newFakeChannel()
	fake.onPublish = func(f *fakeChannel) {
		f.confirmCh <- amqp.Confirmation{DeliveryTag: 1, Ack: true}
	}
	publisher, err := rabbitmq.NewPublisher(fake)
	if err != nil {
		t.Fatalf("NewPublisher returned error: %v", err)
	}

	err = publisher.Publish(context.Background(), "exchange", "routing.key", amqp.Publishing{})

	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	if !fake.mandatory {
		t.Fatal("PublishWithContext mandatory = false, want true")
	}
	if fake.immediate {
		t.Fatal("PublishWithContext immediate = true, want false")
	}
	if fake.deliveryMode != amqp.Persistent {
		t.Fatalf("delivery mode = %d, want %d", fake.deliveryMode, amqp.Persistent)
	}
}

func TestPublisherReturnsErrorOnReturnedMessage(t *testing.T) {
	fake := newFakeChannel()
	fake.onPublish = func(f *fakeChannel) {
		f.returnCh <- amqp.Return{ReplyText: "NO_ROUTE", Headers: f.publishing.Headers}
		f.confirmCh <- amqp.Confirmation{DeliveryTag: 1, Ack: true}
	}
	publisher, err := rabbitmq.NewPublisher(fake)
	if err != nil {
		t.Fatalf("NewPublisher returned error: %v", err)
	}

	err = publisher.Publish(context.Background(), "exchange", "missing.key", amqp.Publishing{})

	if err == nil || !strings.Contains(err.Error(), "returned") {
		t.Fatalf("Publish error = %v, want returned message error", err)
	}
}

func TestPublisherReturnsErrorOnNack(t *testing.T) {
	fake := newFakeChannel()
	fake.onPublish = func(f *fakeChannel) {
		f.confirmCh <- amqp.Confirmation{DeliveryTag: 1, Ack: false}
	}
	publisher, err := rabbitmq.NewPublisher(fake)
	if err != nil {
		t.Fatalf("NewPublisher returned error: %v", err)
	}

	err = publisher.Publish(context.Background(), "exchange", "routing.key", amqp.Publishing{})

	if err == nil || !strings.Contains(err.Error(), "nack") {
		t.Fatalf("Publish error = %v, want nack error", err)
	}
}

func TestPublisherReturnsContextErrorWhenConfirmDoesNotArrive(t *testing.T) {
	fake := newFakeChannel()
	publisher, err := rabbitmq.NewPublisher(fake)
	if err != nil {
		t.Fatalf("NewPublisher returned error: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err = publisher.Publish(ctx, "exchange", "routing.key", amqp.Publishing{})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Publish error = %v, want context deadline exceeded", err)
	}
}

func TestPublisherReturnsErrorWhenReservedHeaderProvided(t *testing.T) {
	fake := newFakeChannel()
	publisher, err := rabbitmq.NewPublisher(fake)
	if err != nil {
		t.Fatalf("NewPublisher returned error: %v", err)
	}
	msg := amqp.Publishing{
		Headers: amqp.Table{
			rabbitmq.PublisherSequenceHeader: int64(99),
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err = publisher.Publish(ctx, "exchange", "routing.key", msg)

	if err == nil || !strings.Contains(err.Error(), rabbitmq.PublisherSequenceHeader) {
		t.Fatalf("Publish error = %v, want reserved header error", err)
	}
	if fake.publishCount != 0 {
		t.Fatalf("PublishWithContext called %d times, want 0", fake.publishCount)
	}
}

func TestPublisherIgnoresStaleConfirmAfterTimedOutPublish(t *testing.T) {
	fake := newFakeChannel()
	publisher, err := rabbitmq.NewPublisher(fake)
	if err != nil {
		t.Fatalf("NewPublisher returned error: %v", err)
	}
	firstCtx, firstCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer firstCancel()

	err = publisher.Publish(firstCtx, "exchange", "routing.key", amqp.Publishing{})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("first Publish error = %v, want context deadline exceeded", err)
	}

	secondPublished := make(chan struct{})
	fake.onPublish = func(f *fakeChannel) {
		if f.publishCount == 2 {
			close(secondPublished)
		}
	}
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- publisher.Publish(context.Background(), "exchange", "routing.key", amqp.Publishing{})
	}()

	select {
	case <-secondPublished:
	case <-time.After(time.Second):
		t.Fatal("second publish did not call PublishWithContext")
	}

	fake.confirmCh <- amqp.Confirmation{DeliveryTag: 1, Ack: true}
	select {
	case err := <-secondDone:
		t.Fatalf("second Publish returned from stale confirm with error %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	fake.confirmCh <- amqp.Confirmation{DeliveryTag: 2, Ack: true}
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("second Publish returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("second Publish did not return after current confirm")
	}
}

func TestPublisherContinuouslyDrainsStaleConfirmsAfterTimeouts(t *testing.T) {
	fake := newFakeChannel()
	fake.confirmCh = make(chan amqp.Confirmation, 1)
	publisher, err := rabbitmq.NewPublisher(fake)
	if err != nil {
		t.Fatalf("NewPublisher returned error: %v", err)
	}

	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		err = publisher.Publish(ctx, "exchange", "routing.key", amqp.Publishing{})
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Publish %d error = %v, want context deadline exceeded", i+1, err)
		}
	}

	for tag := uint64(1); tag <= 3; tag++ {
		select {
		case fake.confirmCh <- amqp.Confirmation{DeliveryTag: tag, Ack: true}:
		case <-time.After(time.Second):
			t.Fatalf("stale confirm for tag %d was not drained", tag)
		}
	}

	fourthPublished := make(chan struct{})
	fake.onPublish = func(f *fakeChannel) {
		if f.publishCount == 4 {
			close(fourthPublished)
		}
	}
	fourthDone := make(chan error, 1)
	go func() {
		fourthDone <- publisher.Publish(context.Background(), "exchange", "routing.key", amqp.Publishing{})
	}()

	select {
	case <-fourthPublished:
	case <-time.After(time.Second):
		t.Fatal("fourth publish did not call PublishWithContext")
	}

	select {
	case err := <-fourthDone:
		t.Fatalf("fourth Publish returned before current confirm with error %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	fake.confirmCh <- amqp.Confirmation{DeliveryTag: 4, Ack: true}
	select {
	case err := <-fourthDone:
		if err != nil {
			t.Fatalf("fourth Publish returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("fourth Publish did not return after current confirm")
	}
}
