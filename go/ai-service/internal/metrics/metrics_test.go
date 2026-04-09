package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestPromRecorder_RecordTurn(t *testing.T) {
	TurnsTotal.Reset()
	r := PromRecorder{}
	r.RecordTurn("final", 3, 500*time.Millisecond)
	if got := testutil.ToFloat64(TurnsTotal.WithLabelValues("final")); got != 1 {
		t.Errorf("turns counter = %v", got)
	}
}

func TestPromRecorder_RecordTool(t *testing.T) {
	ToolCallsTotal.Reset()
	r := PromRecorder{}
	r.RecordTool("search_products", "success", 10*time.Millisecond)
	if got := testutil.ToFloat64(ToolCallsTotal.WithLabelValues("search_products", "success")); got != 1 {
		t.Errorf("tool counter = %v", got)
	}
}
