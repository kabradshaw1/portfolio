package metrics

import (
	"math"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
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

func TestPromRecorder_RecordToolSubLLM(t *testing.T) {
	ToolSubLLMDuration.Reset()
	r := PromRecorder{}
	r.RecordToolSubLLM("summarize_orders", 250*time.Millisecond)
	observer := ToolSubLLMDuration.WithLabelValues("summarize_orders")
	metric, ok := observer.(prometheus.Metric)
	if !ok {
		t.Fatalf("histogram observer does not implement prometheus.Metric")
	}
	dtoMetric := &dto.Metric{}
	if err := metric.Write(dtoMetric); err != nil {
		t.Fatalf("write metric: %v", err)
	}
	if got := dtoMetric.GetHistogram().GetSampleCount(); got != 1 {
		t.Errorf("sub-LLM histogram count = %d", got)
	}
	if got := dtoMetric.GetHistogram().GetSampleSum(); math.Abs(got-0.25) > 0.000001 {
		t.Errorf("sub-LLM histogram sum = %f", got)
	}
}
