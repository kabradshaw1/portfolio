package observability

import "time"

type MetricSample struct {
	Metric map[string]string `json:"metric"`
	Value  float64           `json:"value"`
	Time   time.Time         `json:"time"`
}

type LogLine struct {
	Time   time.Time         `json:"time"`
	Labels map[string]string `json:"labels"`
	Line   string            `json:"line"`
}

type TraceSpan struct {
	Service    string `json:"service,omitempty"`
	Operation  string `json:"operation"`
	DurationMS int64  `json:"duration_ms"`
	Error      bool   `json:"error,omitempty"`
}

type TraceSummary struct {
	TraceID   string      `json:"trace_id"`
	SpanCount int         `json:"span_count"`
	Spans     []TraceSpan `json:"spans"`
	Truncated bool        `json:"truncated"`
}

type LogQuery struct {
	Service string
	Pattern string
	Start   time.Time
	End     time.Time
	Limit   int
}
