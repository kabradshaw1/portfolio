package workflows

import (
	"time"

	"github.com/kabradshaw1/portfolio/go/observability-mcp-service/internal/observability"
)

type SourceStatus struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Endpoint string `json:"endpoint,omitempty"`
}

type Window struct {
	From     time.Time `json:"from"`
	To       time.Time `json:"to"`
	Duration string    `json:"duration"`
}

type Signal struct {
	Name        string                       `json:"name"`
	Query       string                       `json:"query,omitempty"`
	Samples     []observability.MetricSample `json:"samples,omitempty"`
	Value       *float64                     `json:"value,omitempty"`
	Unit        string                       `json:"unit,omitempty"`
	Description string                       `json:"description,omitempty"`
}

type Finding struct {
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Evidence string `json:"evidence"`
}

type SourceError struct {
	Source    string `json:"source"`
	Operation string `json:"operation"`
	Message   string `json:"message"`
}

type EvidenceBundle struct {
	Tool     string                       `json:"tool"`
	Status   string                       `json:"status"`
	Window   Window                       `json:"window"`
	Sources  []SourceStatus               `json:"sources"`
	Signals  []Signal                     `json:"signals"`
	Logs     []observability.LogLine      `json:"logs"`
	Traces   []observability.TraceSummary `json:"traces"`
	Alerts   observability.AlertSummary   `json:"alerts,omitempty"`
	Findings []Finding                    `json:"findings"`
	Partial  bool                         `json:"partial"`
	Errors   []SourceError                `json:"errors"`
}

func NewBundle(tool, duration string) EvidenceBundle {
	now := time.Now().UTC()
	return EvidenceBundle{
		Tool:   tool,
		Status: "unknown",
		Window: Window{From: now, To: now, Duration: duration},
	}
}

func (b *EvidenceBundle) AddError(source, operation, message string) {
	b.Partial = true
	b.Errors = append(b.Errors, SourceError{Source: source, Operation: operation, Message: message})
}
