package composite

import (
	"context"
	"sync"

	"golang.org/x/sync/errgroup"
)

// OrderRecord is the primary order row consulted by investigate_my_order.
type OrderRecord struct {
	ID            string
	Status        string
	TraceID       string
	CorrelationID string
	CreatedUnix   int64
	UpdatedUnix   int64
}

// SagaHistory is the saga state machine history for an order.
type SagaHistory struct {
	Step    string
	Retries int
	Events  []string
}

// PaymentRecord is the payment outbox row corresponding to an order.
type PaymentRecord struct {
	StripeChargeID  string
	WebhookReceived bool
}

// CartReservation captures whether the cart hold for an order has been released.
type CartReservation struct {
	Released   bool
	ReleasedAt int64
}

// RabbitEvent records a RabbitMQ message observed for an order's correlation id.
type RabbitEvent struct {
	Name      string
	Timestamp int64
}

// SpanSummary is a Jaeger span reduced to name and duration.
type SpanSummary struct {
	Name       string
	DurationMs int64
}

// TraceSummary is the stitched trace for an order.
type TraceSummary struct {
	ID    string
	Spans []SpanSummary
}

// Source interfaces — small, single-purpose, mockable.
type OrderSource interface {
	FetchOrder(ctx context.Context, id string) (OrderRecord, error)
}
type SagaSource interface {
	FetchSaga(ctx context.Context, id string) (SagaHistory, error)
}
type PaymentSource interface {
	FetchPayment(ctx context.Context, id string) (PaymentRecord, error)
}
type CartSource interface {
	FetchCartReservation(ctx context.Context, id string) (CartReservation, error)
}
type RabbitSource interface {
	FetchEvents(ctx context.Context, correlationID string) ([]RabbitEvent, error)
}
type TraceSource interface {
	FetchTrace(ctx context.Context, traceID string) (TraceSummary, error)
}
type LogSource interface {
	FetchLogs(ctx context.Context, services []string, from, to int64) ([]string, error)
}

// EvidenceBundle is the cross-source artifact a verdict is computed from.
type EvidenceBundle struct {
	Order         OrderRecord
	Saga          SagaHistory
	Payment       PaymentRecord
	Cart          CartReservation
	Rabbit        []RabbitEvent
	Trace         TraceSummary
	Logs          []string
	Partial       bool
	PartialReason []string
}

// EvidenceFetcher owns all sources and fans out in parallel.
type EvidenceFetcher struct {
	Order   OrderSource
	Saga    SagaSource
	Payment PaymentSource
	Cart    CartSource
	Rabbit  RabbitSource
	Trace   TraceSource
	Logs    LogSource
}

// Fetch runs every source in parallel. The order fetch is primary —
// failure there returns a hard error. All other sources are best-effort
// and contribute Partial=true on failure rather than aborting.
func (f EvidenceFetcher) Fetch(ctx context.Context, orderID string) (EvidenceBundle, error) {
	var bundle EvidenceBundle

	order, err := f.Order.FetchOrder(ctx, orderID)
	if err != nil {
		return bundle, err
	}
	bundle.Order = order

	g, gctx := errgroup.WithContext(ctx)

	var (
		mu             sync.Mutex
		partial        bool
		partialReasons []string
	)
	mark := func(reason string) {
		mu.Lock()
		defer mu.Unlock()
		partialReasons = append(partialReasons, reason)
		partial = true
	}

	var (
		saga    SagaHistory
		payment PaymentRecord
		cart    CartReservation
		rabbit  []RabbitEvent
		trace   TraceSummary
		logs    []string
	)

	g.Go(func() error {
		v, e := f.Saga.FetchSaga(gctx, orderID)
		if e != nil {
			mark("saga: " + e.Error())
			return nil
		}
		saga = v
		return nil
	})
	g.Go(func() error {
		v, e := f.Payment.FetchPayment(gctx, orderID)
		if e != nil {
			mark("payment: " + e.Error())
			return nil
		}
		payment = v
		return nil
	})
	g.Go(func() error {
		v, e := f.Cart.FetchCartReservation(gctx, orderID)
		if e != nil {
			mark("cart: " + e.Error())
			return nil
		}
		cart = v
		return nil
	})
	g.Go(func() error {
		v, e := f.Rabbit.FetchEvents(gctx, order.CorrelationID)
		if e != nil {
			mark("rabbit: " + e.Error())
			return nil
		}
		rabbit = v
		return nil
	})
	g.Go(func() error {
		if order.TraceID == "" {
			return nil
		}
		v, e := f.Trace.FetchTrace(gctx, order.TraceID)
		if e != nil {
			mark("trace: " + e.Error())
			return nil
		}
		trace = v
		return nil
	})
	g.Go(func() error {
		v, e := f.Logs.FetchLogs(gctx, []string{"order-service", "payment-service", "cart-service"}, order.CreatedUnix, order.UpdatedUnix)
		if e != nil {
			mark("logs: " + e.Error())
			return nil
		}
		logs = v
		return nil
	})

	_ = g.Wait() // all goroutines swallow errors; partial results carried via partialReasons

	bundle.Saga = saga
	bundle.Payment = payment
	bundle.Cart = cart
	bundle.Rabbit = rabbit
	bundle.Trace = trace
	bundle.Logs = logs
	bundle.Partial = partial
	bundle.PartialReason = partialReasons
	return bundle, nil
}
