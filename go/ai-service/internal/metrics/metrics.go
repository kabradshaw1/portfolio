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

	OllamaRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "ollama_request_duration_seconds",
		Help:    "Wall-clock time for Ollama API calls.",
		Buckets: prometheus.ExponentialBuckets(0.1, 2, 12),
	}, []string{"service", "model", "operation"})

	OllamaTokens = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ollama_tokens_total",
		Help: "Total tokens processed by Ollama.",
	}, []string{"service", "model", "kind"})

	OllamaEvalDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "ollama_eval_duration_seconds",
		Help:    "Ollama model evaluation duration (from response metadata).",
		Buckets: prometheus.ExponentialBuckets(0.1, 2, 10),
	}, []string{"service", "model"})
)

// Recorder is the interface the agent loop uses to emit metrics. It keeps the
// agent package from importing Prometheus directly and makes tests trivial.
type Recorder interface {
	RecordTurn(outcome string, steps int, dur time.Duration)
	RecordTool(name, outcome string, dur time.Duration)
	RecordOllamaCall(model, operation string, dur time.Duration, promptTokens, completionTokens int, evalDurNs int)
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

func (PromRecorder) RecordOllamaCall(model, operation string, dur time.Duration, promptTokens, completionTokens int, evalDurNs int) {
	svc := "ai-service"
	OllamaRequestDuration.WithLabelValues(svc, model, operation).Observe(dur.Seconds())
	if promptTokens > 0 {
		OllamaTokens.WithLabelValues(svc, model, "prompt").Add(float64(promptTokens))
	}
	if completionTokens > 0 {
		OllamaTokens.WithLabelValues(svc, model, "completion").Add(float64(completionTokens))
	}
	if evalDurNs > 0 {
		OllamaEvalDuration.WithLabelValues(svc, model).Observe(float64(evalDurNs) / 1e9)
	}
}

// NopRecorder is the zero-value substitute for tests that don't care about metrics.
type NopRecorder struct{}

func (NopRecorder) RecordTurn(string, int, time.Duration)    {}
func (NopRecorder) RecordTool(string, string, time.Duration) {}
func (NopRecorder) RecordOllamaCall(string, string, time.Duration, int, int, int) {}
