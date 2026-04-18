package aggregator

import "time"

// OrderSlot holds per-minute order metrics.
type OrderSlot struct {
	Created   int
	Completed int
	Failed    int
	Revenue   int // cents
}

func newOrderSlot() OrderSlot { return OrderSlot{} }

// OrderAggregator tracks order events in a 24-hour sliding window.
type OrderAggregator struct {
	window *Window[OrderSlot]
}

const orderWindowDuration = 24 * time.Hour

// NewOrderAggregator creates an aggregator with a 24-hour window.
func NewOrderAggregator() *OrderAggregator {
	return &OrderAggregator{
		window: NewWindow(orderWindowDuration, newOrderSlot),
	}
}

// RecordCreated records an order.created event.
func (a *OrderAggregator) RecordCreated(totalCents int) {
	a.window.Update(func(s *OrderSlot) {
		s.Created++
		s.Revenue += totalCents
	})
}

// RecordCompleted records an order.completed event.
func (a *OrderAggregator) RecordCompleted(totalCents int) {
	a.window.Update(func(s *OrderSlot) {
		s.Completed++
		s.Revenue += totalCents
	})
}

// RecordFailed records an order.failed event.
func (a *OrderAggregator) RecordFailed() {
	a.window.Update(func(s *OrderSlot) {
		s.Failed++
	})
}

// OrderStats returns aggregate order statistics.
type OrderStats struct {
	OrdersPerHour  float64        `json:"ordersPerHour"`
	RevenuePerHour float64        `json:"revenuePerHour"`
	CompletionRate float64        `json:"completionRate"`
	Hourly         []HourlyBucket `json:"hourly"`
	StatusBreakdown StatusBreakdown `json:"statusBreakdown"`
}

// HourlyBucket is a per-hour aggregation for charting.
type HourlyBucket struct {
	Hour    string  `json:"hour"`
	Count   int     `json:"count"`
	Revenue float64 `json:"revenue"`
}

// StatusBreakdown shows counts by status.
type StatusBreakdown struct {
	Created   int `json:"created"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
}

// Stats computes aggregate statistics from the window.
func (a *OrderAggregator) Stats() OrderStats {
	entries := a.window.Get()

	var totalCreated, totalCompleted, totalFailed, totalRevenue int
	hourBuckets := make(map[string]*HourlyBucket)

	for _, e := range entries {
		totalCreated += e.Value.Created
		totalCompleted += e.Value.Completed
		totalFailed += e.Value.Failed
		totalRevenue += e.Value.Revenue

		hourKey := e.Start.Truncate(time.Hour).Format(time.RFC3339)
		bucket, ok := hourBuckets[hourKey]
		if !ok {
			bucket = &HourlyBucket{Hour: hourKey}
			hourBuckets[hourKey] = bucket
		}
		bucket.Count += e.Value.Created + e.Value.Completed + e.Value.Failed
		bucket.Revenue += float64(e.Value.Revenue) / 100
	}

	hours := float64(a.window.duration) / float64(time.Hour)
	total := totalCreated + totalCompleted + totalFailed

	var completionRate float64
	if total > 0 {
		completionRate = float64(totalCompleted) / float64(total)
	}

	hourly := make([]HourlyBucket, 0, len(hourBuckets))
	for _, b := range hourBuckets {
		hourly = append(hourly, *b)
	}

	return OrderStats{
		OrdersPerHour:  float64(total) / hours,
		RevenuePerHour: float64(totalRevenue) / 100 / hours,
		CompletionRate: completionRate,
		Hourly:         hourly,
		StatusBreakdown: StatusBreakdown{
			Created:   totalCreated,
			Completed: totalCompleted,
			Failed:    totalFailed,
		},
	}
}
