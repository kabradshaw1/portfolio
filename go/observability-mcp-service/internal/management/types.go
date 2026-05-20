package management

import "time"

type RiskTier string
type Decision string
type Status string

const (
	RiskDiagnostic       RiskTier = "diagnostic"
	RiskLowRiskMutation RiskTier = "low_risk_mutation"
	RiskHighRiskMutation RiskTier = "high_risk_mutation"

	DecisionAllow       Decision = "allow"
	DecisionBlock       Decision = "block"
	DecisionPreviewOnly Decision = "preview_only"

	StatusPreviewed Status = "previewed"
	StatusBlocked   Status = "blocked"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusTimedOut  Status = "timed_out"
)

type Action struct {
	ID               string        `json:"id"`
	Title            string        `json:"title"`
	Description      string        `json:"description"`
	RiskTier         RiskTier      `json:"risk_tier"`
	ScriptPath       string        `json:"script_path"`
	AllowedArgs      []Argument    `json:"allowed_args,omitempty"`
	Timeout          time.Duration `json:"-"`
	TimeoutText      string        `json:"timeout"`
	RequiresIncident bool          `json:"requires_incident"`
	Idempotent        bool          `json:"idempotent"`
	Preflight        string        `json:"preflight"`
	Postflight       string        `json:"postflight"`
	RedactPatterns   []string      `json:"-"`
	NextSteps        string        `json:"next_steps"`
}

type Argument struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

type PolicyDecision struct {
	Decision Decision `json:"decision"`
	Reason   string   `json:"reason"`
}

type ActionResult struct {
	ActionID          string    `json:"action_id"`
	RiskTier          RiskTier  `json:"risk_tier"`
	Decision          Decision  `json:"decision"`
	Status            Status    `json:"status"`
	ScriptPath        string    `json:"script_path"`
	IncidentKey       string    `json:"incident_key,omitempty"`
	StartedAt         time.Time `json:"started_at,omitempty"`
	CompletedAt       time.Time `json:"completed_at,omitempty"`
	DurationMillis    int64     `json:"duration_ms,omitempty"`
	ExitCode          int       `json:"exit_code,omitempty"`
	Stdout            string    `json:"stdout,omitempty"`
	Stderr            string    `json:"stderr,omitempty"`
	OutputTruncated   bool      `json:"output_truncated,omitempty"`
	RedactionsApplied int       `json:"redactions_applied,omitempty"`
	PolicyReason      string    `json:"policy_reason,omitempty"`
	HistoryEventIDs   []int64   `json:"history_event_ids,omitempty"`
	Warning           string    `json:"warning,omitempty"`
}

type ActionRequest struct {
	ActionID      string         `json:"action_id"`
	Args          map[string]any `json:"args,omitempty"`
	IncidentKey   string         `json:"incident_key,omitempty"`
	IncidentTitle string         `json:"incident_title,omitempty"`
	Severity      string         `json:"severity,omitempty"`
	Service       string         `json:"service,omitempty"`
}
