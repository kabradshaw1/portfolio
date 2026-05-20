package history

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("history record not found")

const (
	StatusInvestigating = "investigating"
	StatusMitigated     = "mitigated"
	StatusResolved      = "resolved"

	EventEvidenceSnapshot = "evidence_snapshot"
	EventNoteAdded        = "note_added"
	EventStatusChanged    = "status_changed"

	EventManagementActionPreviewed = "management_action_previewed"
	EventManagementActionStarted   = "management_action_started"
	EventManagementActionCompleted = "management_action_completed"
	EventManagementActionFailed    = "management_action_failed"
	EventManagementActionBlocked   = "management_action_blocked"
)

type IncidentUpsert struct {
	Key      string
	Title    string
	Status   string
	Severity string
	Service  string
}

type Incident struct {
	ID        int64     `json:"id"`
	Key       string    `json:"incident_key"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	Severity  string    `json:"severity,omitempty"`
	Service   string    `json:"service,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SnapshotInput struct {
	IncidentKey    string
	IncidentTitle  string
	Severity       string
	Service        string
	Tool           string
	WindowFrom     time.Time
	WindowTo       time.Time
	WindowDuration string
	Status         string
	Partial        bool
	CriticalCount  int
	WarningCount   int
	SignalCount    int
	LogCount       int
	TraceCount     int
	SourceStatuses []SourceStatus
	BundleJSON     []byte
}

type SourceStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type Snapshot struct {
	ID              int64          `json:"id"`
	IncidentID      *int64         `json:"incident_id,omitempty"`
	TimelineEventID *int64         `json:"timeline_event_id,omitempty"`
	Tool            string         `json:"tool"`
	WindowFrom      time.Time      `json:"window_from"`
	WindowTo        time.Time      `json:"window_to"`
	WindowDuration  string         `json:"window_duration"`
	Status          string         `json:"status"`
	Partial         bool           `json:"partial"`
	CriticalCount   int            `json:"critical_findings"`
	WarningCount    int            `json:"warning_findings"`
	SignalCount     int            `json:"signal_count"`
	LogCount        int            `json:"log_count"`
	TraceCount      int            `json:"trace_count"`
	SourceStatuses  []SourceStatus `json:"source_statuses"`
	BundleJSON      []byte         `json:"-"`
	CreatedAt       time.Time      `json:"created_at"`
}

type Event struct {
	ID         int64     `json:"id"`
	IncidentID int64     `json:"incident_id"`
	Type       string    `json:"event_type"`
	Summary    string    `json:"summary"`
	Details    string    `json:"details,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	Snapshot   *Snapshot `json:"snapshot,omitempty"`
}

type IncidentSummary struct {
	Incident      Incident  `json:"incident"`
	SnapshotCount int       `json:"snapshot_count"`
	LastEventAt   time.Time `json:"last_event_at"`
}

type IncidentHistory struct {
	Incident Incident `json:"incident"`
	Events   []Event  `json:"events"`
}

type ListFilter struct {
	Status   string
	Service  string
	Severity string
	Limit    int
}

type AddNoteInput struct {
	IncidentKey string
	Note        string
	Status      string
}

type ManagementActionInput struct {
	IncidentKey   string
	IncidentTitle string
	Severity      string
	Service       string
	ActionID      string
	RiskTier      string
	Decision      string
	Status        string
	Summary       string
	DetailsJSON   []byte
}

type ManagementActionFilter struct {
	IncidentKey string
	ActionID    string
	Status      string
	Decision    string
	Limit       int
}

type Store interface {
	Close() error
	Migrate(context.Context) error
	RecordSnapshot(context.Context, SnapshotInput) (Snapshot, error)
	ListIncidents(context.Context, ListFilter) ([]IncidentSummary, error)
	GetIncidentHistory(context.Context, string) (IncidentHistory, error)
	AddIncidentNote(context.Context, AddNoteInput) (Event, error)
	GetSnapshot(context.Context, int64) (Snapshot, error)
	RecordManagementAction(context.Context, ManagementActionInput) (Event, error)
	ListManagementActions(context.Context, ManagementActionFilter) ([]Event, error)
}

func NormalizeStatus(status string) string {
	if status == "" {
		return StatusInvestigating
	}
	return status
}

func ValidStatus(status string) bool {
	switch status {
	case StatusInvestigating, StatusMitigated, StatusResolved:
		return true
	default:
		return false
	}
}
