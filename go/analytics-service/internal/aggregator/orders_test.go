package aggregator

import "testing"

func TestOrderAggregator_Stats(t *testing.T) {
	a := NewOrderAggregator()

	a.RecordCreated(5000)
	a.RecordCreated(3000)
	a.RecordCompleted(5000)
	a.RecordFailed()

	stats := a.Stats()

	if stats.StatusBreakdown.Created != 2 {
		t.Errorf("expected 2 created, got %d", stats.StatusBreakdown.Created)
	}
	if stats.StatusBreakdown.Completed != 1 {
		t.Errorf("expected 1 completed, got %d", stats.StatusBreakdown.Completed)
	}
	if stats.StatusBreakdown.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", stats.StatusBreakdown.Failed)
	}
	if stats.OrdersPerHour <= 0 {
		t.Error("expected positive ordersPerHour")
	}
	if stats.CompletionRate <= 0 {
		t.Error("expected positive completionRate")
	}
}

func TestOrderAggregator_Empty(t *testing.T) {
	a := NewOrderAggregator()
	stats := a.Stats()

	if stats.OrdersPerHour != 0 {
		t.Errorf("expected 0, got %f", stats.OrdersPerHour)
	}
	if stats.CompletionRate != 0 {
		t.Errorf("expected 0, got %f", stats.CompletionRate)
	}
}
