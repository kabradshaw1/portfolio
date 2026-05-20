package management

import "testing"

func TestPolicyDecisions(t *testing.T) {
	tests := []struct {
		name   string
		policy Policy
		risk   RiskTier
		want   Decision
	}{
		{name: "disabled diagnostic blocked", policy: Policy{}, risk: RiskDiagnostic, want: DecisionBlock},
		{name: "disabled low risk blocked", policy: Policy{}, risk: RiskLowRiskMutation, want: DecisionBlock},
		{name: "enabled diagnostic allowed", policy: Policy{ActionsEnabled: true}, risk: RiskDiagnostic, want: DecisionAllow},
		{name: "enabled low risk allowed", policy: Policy{ActionsEnabled: true}, risk: RiskLowRiskMutation, want: DecisionAllow},
		{name: "high risk preview", policy: Policy{ActionsEnabled: true}, risk: RiskHighRiskMutation, want: DecisionPreviewOnly},
		{name: "high risk enabled", policy: Policy{ActionsEnabled: true, AllowHighRisk: true}, risk: RiskHighRiskMutation, want: DecisionAllow},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.policy.Evaluate(Action{ID: "a", RiskTier: tc.risk})
			if got.Decision != tc.want {
				t.Fatalf("Decision = %q, want %q; reason=%s", got.Decision, tc.want, got.Reason)
			}
		})
	}
}
