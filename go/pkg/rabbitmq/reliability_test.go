package rabbitmq_test

import (
	"testing"

	rabbitmq "github.com/kabradshaw1/portfolio/go/pkg/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
)

func TestRetryCountHandlesMissingAndAMQPIntegerTypes(t *testing.T) {
	headers := amqp.Table{}
	if got := rabbitmq.RetryCount(headers); got != 0 {
		t.Fatalf("missing retry count = %d, want 0", got)
	}
	headers[rabbitmq.RetryCountHeader] = int32(2)
	if got := rabbitmq.RetryCount(headers); got != 2 {
		t.Fatalf("int32 retry count = %d, want 2", got)
	}
	headers[rabbitmq.RetryCountHeader] = int64(3)
	if got := rabbitmq.RetryCount(headers); got != 3 {
		t.Fatalf("int64 retry count = %d, want 3", got)
	}
}

func TestFailureClassification(t *testing.T) {
	if !rabbitmq.IsPermanent(rabbitmq.PermanentErrorf("bad payload")) {
		t.Fatal("PermanentErrorf should classify as permanent")
	}
	if rabbitmq.IsPermanent(rabbitmq.RetryableErrorf("db unavailable")) {
		t.Fatal("RetryableErrorf should not classify as permanent")
	}
}
