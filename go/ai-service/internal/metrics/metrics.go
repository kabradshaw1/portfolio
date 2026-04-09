package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	TurnsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ai_agent_turns_total",
		Help: "Agent turns by outcome.",
	}, []string{"outcome"})

	StepsPerTurn = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "ai_agent_steps_per_turn",
		Help:    "Number of LLM calls per turn.",
		Buckets: []float64{1, 2, 3, 4, 5, 6, 8, 10},
	})

	TurnDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "ai_agent_turn_duration_seconds",
		Help:    "End-to-end agent turn duration.",
		Buckets: prometheus.ExponentialBuckets(0.1, 2, 10),
	})

	ToolCallsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ai_tool_calls_total",
		Help: "Tool invocations by name and outcome.",
	}, []string{"name", "outcome"})

	ToolDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "ai_tool_duration_seconds",
		Help:    "Per-tool latency.",
		Buckets: prometheus.DefBuckets,
	}, []string{"name"})

	CacheEvents = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ai_cache_events_total",
		Help: "Cache events by cache and event type.",
	}, []string{"cache", "event"})
)

// Recorder is the interface the agent loop uses to emit metrics. It keeps the
// agent package from importing Prometheus directly and makes tests trivial.
type Recorder interface {
	RecordTurn(outcome string, steps int, dur time.Duration)
	RecordTool(name, outcome string, dur time.Duration)
}

// PromRecorder writes to the package-level Prometheus collectors.
type PromRecorder struct{}

func (PromRecorder) RecordTurn(outcome string, steps int, dur time.Duration) {
	TurnsTotal.WithLabelValues(outcome).Inc()
	StepsPerTurn.Observe(float64(steps))
	TurnDuration.Observe(dur.Seconds())
}

func (PromRecorder) RecordTool(name, outcome string, dur time.Duration) {
	ToolCallsTotal.WithLabelValues(name, outcome).Inc()
	ToolDuration.WithLabelValues(name).Observe(dur.Seconds())
}

// NopRecorder is the zero-value substitute for tests that don't care about metrics.
type NopRecorder struct{}

func (NopRecorder) RecordTurn(string, int, time.Duration)    {}
func (NopRecorder) RecordTool(string, string, time.Duration) {}
