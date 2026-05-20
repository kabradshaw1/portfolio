package management

type Policy struct {
	ActionsEnabled bool
	AllowHighRisk  bool
}

func (p Policy) Evaluate(action Action) PolicyDecision {
	if !p.ActionsEnabled {
		return PolicyDecision{Decision: DecisionBlock, Reason: "management actions are disabled; set OBS_MANAGEMENT_ACTIONS_ENABLED=true"}
	}
	if action.RiskTier == RiskHighRiskMutation && !p.AllowHighRisk {
		return PolicyDecision{Decision: DecisionPreviewOnly, Reason: "high-risk management actions require OBS_MANAGEMENT_ALLOW_HIGH_RISK=true"}
	}
	return PolicyDecision{Decision: DecisionAllow, Reason: "action is allowed by management policy"}
}
