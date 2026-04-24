package projection

import "testing"

func TestStatusFromEventType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		eventType string
		want      string
	}{
		{"order.created", "created"},
		{"order.reserved", "reserved"},
		{"order.payment_initiated", "payment_initiated"},
		{"order.payment_completed", "payment_completed"},
		{"order.completed", "completed"},
		{"order.failed", "failed"},
		{"order.cancelled", "cancelled"},
		{"order.unknown_type", "unknown"},
		{"", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			t.Parallel()
			got := statusFromEventType(tt.eventType)
			if got != tt.want {
				t.Errorf("statusFromEventType(%q) = %q, want %q", tt.eventType, got, tt.want)
			}
		})
	}
}
