package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"

	rabbitmq "github.com/kabradshaw1/portfolio/go/pkg/rabbitmq"
)

type fakeRabbitConnection struct {
	channel    *fakeConfirmChannel
	channelErr error
	closeCh    chan *amqp.Error
	closeCalls int
}

func newFakeRabbitConnection(channel *fakeConfirmChannel) *fakeRabbitConnection {
	return &fakeRabbitConnection{
		channel: channel,
		closeCh: make(chan *amqp.Error, 1),
	}
}

func (f *fakeRabbitConnection) Channel() (rabbitmq.ConfirmChannel, error) {
	if f.channelErr != nil {
		return nil, f.channelErr
	}
	return f.channel, nil
}

func (f *fakeRabbitConnection) NotifyClose(_ chan *amqp.Error) chan *amqp.Error {
	return f.closeCh
}

func (f *fakeRabbitConnection) Close() error {
	f.closeCalls++
	return nil
}

type fakeConfirmChannel struct {
	confirmCh chan amqp.Confirmation
	returnCh  chan amqp.Return
	err       error

	publishCalls int
	mandatory    bool
	immediate    bool
	deliveryMode uint8
}

func (f *fakeConfirmChannel) Confirm(bool) error {
	return nil
}

func (f *fakeConfirmChannel) NotifyPublish(confirm chan amqp.Confirmation) chan amqp.Confirmation {
	f.confirmCh = confirm
	return confirm
}

func (f *fakeConfirmChannel) NotifyReturn(ret chan amqp.Return) chan amqp.Return {
	f.returnCh = ret
	return ret
}

func (f *fakeConfirmChannel) GetNextPublishSeqNo() uint64 {
	return uint64(f.publishCalls + 1)
}

func (f *fakeConfirmChannel) PublishWithContext(
	_ context.Context,
	_, _ string,
	mandatory, immediate bool,
	msg amqp.Publishing,
) error {
	f.publishCalls++
	f.mandatory = mandatory
	f.immediate = immediate
	f.deliveryMode = msg.DeliveryMode
	if f.err != nil {
		return f.err
	}
	f.confirmCh <- amqp.Confirmation{DeliveryTag: uint64(f.publishCalls), Ack: true}
	return nil
}

func TestReconnectingRabbitMQPublisherReconnectsAfterPublishFailure(t *testing.T) {
	firstPublishErr := errors.New("channel closed")
	firstConn := newFakeRabbitConnection(&fakeConfirmChannel{err: firstPublishErr})
	secondChannel := &fakeConfirmChannel{}
	secondConn := newFakeRabbitConnection(secondChannel)

	connections := []rabbitMQConnection{firstConn, secondConn}
	dials := 0
	publisher := newReconnectingRabbitMQPublisher("amqp://test", func(string) (rabbitMQConnection, error) {
		conn := connections[dials]
		dials++
		return conn, nil
	})

	err := publisher.Publish(context.Background(), "exchange", "routing.key", amqp.Publishing{})
	if !errors.Is(err, firstPublishErr) {
		t.Fatalf("first Publish error = %v, want %v", err, firstPublishErr)
	}
	if dials != 1 {
		t.Fatalf("dials after first Publish = %d, want 1", dials)
	}
	if firstConn.closeCalls != 1 {
		t.Fatalf("first connection close calls = %d, want 1", firstConn.closeCalls)
	}

	if err := publisher.Publish(context.Background(), "exchange", "routing.key", amqp.Publishing{}); err != nil {
		t.Fatalf("second Publish returned error: %v", err)
	}
	if dials != 2 {
		t.Fatalf("dials after second Publish = %d, want 2", dials)
	}
	if !secondChannel.mandatory {
		t.Fatal("second Publish mandatory = false, want true")
	}
	if secondChannel.immediate {
		t.Fatal("second Publish immediate = true, want false")
	}
	if secondChannel.deliveryMode != amqp.Persistent {
		t.Fatalf("second Publish delivery mode = %d, want %d", secondChannel.deliveryMode, amqp.Persistent)
	}
}

func TestReconnectingRabbitMQPublisherSurfacesDialFailure(t *testing.T) {
	want := errors.New("broker unavailable")
	publisher := newReconnectingRabbitMQPublisher("amqp://test", func(string) (rabbitMQConnection, error) {
		return nil, want
	})

	err := publisher.Publish(context.Background(), "exchange", "routing.key", amqp.Publishing{})
	if !errors.Is(err, want) {
		t.Fatalf("Publish error = %v, want %v", err, want)
	}
	if !strings.Contains(err.Error(), "connect RabbitMQ") {
		t.Fatalf("Publish error = %q, want connect context", err)
	}
}

func TestReconnectingRabbitMQPublisherReconnectsAfterConnectionClose(t *testing.T) {
	firstConn := newFakeRabbitConnection(&fakeConfirmChannel{})
	secondConn := newFakeRabbitConnection(&fakeConfirmChannel{})

	connections := []rabbitMQConnection{firstConn, secondConn}
	dials := 0
	publisher := newReconnectingRabbitMQPublisher("amqp://test", func(string) (rabbitMQConnection, error) {
		conn := connections[dials]
		dials++
		return conn, nil
	})

	if err := publisher.Publish(context.Background(), "exchange", "routing.key", amqp.Publishing{}); err != nil {
		t.Fatalf("first Publish returned error: %v", err)
	}
	firstConn.closeCh <- &amqp.Error{Reason: "connection forced"}

	if err := publisher.Publish(context.Background(), "exchange", "routing.key", amqp.Publishing{}); err != nil {
		t.Fatalf("second Publish returned error: %v", err)
	}
	if dials != 2 {
		t.Fatalf("dials = %d, want 2", dials)
	}
	if firstConn.closeCalls != 1 {
		t.Fatalf("first connection close calls = %d, want 1", firstConn.closeCalls)
	}
}
