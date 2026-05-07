package kafkaconsumer

import (
	"context"
	"time"

	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

type RetryConfig = resilience.RetryConfig

func DefaultRetryConfig() RetryConfig {
	return resilience.DefaultRetryConfig()
}

func Retry(ctx context.Context, cfg RetryConfig, fn func(context.Context) error) error {
	_, err := resilience.Retry(ctx, cfg, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, fn(ctx)
	})
	return err
}

func SleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
