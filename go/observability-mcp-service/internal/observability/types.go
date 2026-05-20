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

type AlertInstance struct {
	Name         string            `json:"name,omitempty"`
	State        string            `json:"state,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
	Annotations  map[string]string `json:"annotations,omitempty"`
	StartsAt     time.Time         `json:"starts_at,omitempty"`
	EndsAt       time.Time         `json:"ends_at,omitempty"`
	GeneratorURL string            `json:"generator_url,omitempty"`
	DashboardURL string            `json:"dashboard_url,omitempty"`
	RuleUID      string            `json:"rule_uid,omitempty"`
}

type AlertRule struct {
	UID        string            `json:"uid,omitempty"`
	Title      string            `json:"title,omitempty"`
	FolderUID  string            `json:"folder_uid,omitempty"`
	Namespace  string            `json:"namespace,omitempty"`
	RuleGroup  string            `json:"rule_group,omitempty"`
	Condition  string            `json:"condition,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	Provenance string            `json:"provenance,omitempty"`
}

type AlertSummary struct {
	ActiveAlerts []AlertInstance `json:"active_alerts,omitempty"`
	Rules        []AlertRule     `json:"rules,omitempty"`
	Truncated    bool            `json:"truncated,omitempty"`
}

type LogQuery struct {
	Service string
	Pattern string
	Start   time.Time
	End     time.Time
	Limit   int
}
