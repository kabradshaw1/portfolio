package outbox

import (
	"context"
	"log/slog"
	"math"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/kabradshaw1/portfolio/go/payment-service/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

// OutboxFetcher reads unpublished outbox messages and marks them published.
type OutboxFetcher interface {
	FetchUnpublished(ctx context.Context, limit int) ([]model.OutboxMessage, error)
	MarkPublished(ctx context.Context, id uuid.UUID) error
	Ping(ctx context.Context) error
}

// Poller periodically fetches unpublished outbox messages and publishes them to RabbitMQ.
type Poller struct {
	fetcher  OutboxFetcher
	ch       *amqp.Channel
	interval time.Duration
	batch    int
	idle     atomic.Bool
}

// NewPoller creates a Poller with the given fetcher, AMQP channel, polling interval, and batch size.
func NewPoller(fetcher OutboxFetcher, ch *amqp.Channel, interval time.Duration, batch int) *Poller {
	p := &Poller{
		fetcher:  fetcher,
		ch:       ch,
		interval: interval,
		batch:    batch,
	}
	p.idle.Store(true)
	return p
}

// IsIdle returns true when the poller is not currently processing messages.
func (p *Poller) IsIdle() bool {
	return p.idle.Load()
}

const (
	waitForDBInitialBackoff = 1 * time.Second
	waitForDBMaxBackoff     = 30 * time.Second
)

// waitForDB blocks until the database is reachable or the context is cancelled.
// Uses exponential backoff starting at 1s, capped at 30s.
func (p *Poller) waitForDB(ctx context.Context) {
	backoff := waitForDBInitialBackoff
	for {
		if err := p.fetcher.Ping(ctx); err == nil {
			slog.InfoContext(ctx, "outbox poller: database ready")
			return
		}

		slog.WarnContext(ctx, "outbox poller: database not ready, retrying", "backoff", backoff)

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		backoff = time.Duration(math.Min(float64(backoff*2), float64(waitForDBMaxBackoff)))
	}
}

// Run starts the polling loop. It ticks at the configured interval and calls poll on each tick.
// It stops when the context is cancelled.
func (p *Poller) Run(ctx context.Context) {
	p.waitForDB(ctx)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

// poll fetches unpublished messages and publishes each one to RabbitMQ.
// On publish error, it logs and continues to the next message.
func (p *Poller) poll(ctx context.Context) {
	p.idle.Store(false)
	defer p.idle.Store(true)

	messages, err := p.fetcher.FetchUnpublished(ctx, p.batch)
	if err != nil {
		slog.ErrorContext(ctx, "outbox poller: failed to fetch unpublished messages", "error", err)
		return
	}

	for _, msg := range messages {
		headers := make(amqp.Table)
		tracing.InjectAMQP(ctx, headers)

		err := p.ch.PublishWithContext(
			ctx,
			msg.Exchange,
			msg.RoutingKey,
			false, // mandatory
			false, // immediate
			amqp.Publishing{
				ContentType:  "application/json",
				DeliveryMode: amqp.Persistent,
				MessageId:    msg.ID.String(),
				Headers:      headers,
				Body:         msg.Payload,
			},
		)
		if err != nil {
			slog.ErrorContext(ctx, "outbox poller: failed to publish message",
				"messageID", msg.ID,
				"exchange", msg.Exchange,
				"routingKey", msg.RoutingKey,
				"error", err,
			)
			metrics.OutboxPublish.WithLabelValues("error").Inc()
			continue
		}

		metrics.OutboxPublish.WithLabelValues("success").Inc()

		if markErr := p.fetcher.MarkPublished(ctx, msg.ID); markErr != nil {
			slog.ErrorContext(ctx, "outbox poller: failed to mark message published",
				"messageID", msg.ID,
				"error", markErr,
			)
		}
	}
}
