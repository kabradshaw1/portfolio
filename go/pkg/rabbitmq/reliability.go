package rabbitmq

import (
	"errors"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	RetryCountHeader  = "x-retry-count"
	DefaultMaxRetries = 3
)

type PermanentError struct{ err error }

func PermanentErrorf(format string, args ...any) error {
	return PermanentError{err: fmt.Errorf(format, args...)}
}

func (e PermanentError) Error() string { return e.err.Error() }
func (e PermanentError) Unwrap() error { return e.err }

type RetryableError struct{ err error }

func RetryableErrorf(format string, args ...any) error {
	return RetryableError{err: fmt.Errorf(format, args...)}
}

func (e RetryableError) Error() string { return e.err.Error() }
func (e RetryableError) Unwrap() error { return e.err }

func IsPermanent(err error) bool {
	var permanent PermanentError
	return errors.As(err, &permanent)
}

func RetryCount(headers amqp.Table) int {
	if headers == nil {
		return 0
	}
	switch v := headers[RetryCountHeader].(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	default:
		return 0
	}
}

func IncrementRetry(headers amqp.Table) amqp.Table {
	if headers == nil {
		headers = amqp.Table{}
	}
	headers[RetryCountHeader] = int32(RetryCount(headers) + 1)
	return headers
}
