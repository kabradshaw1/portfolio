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
