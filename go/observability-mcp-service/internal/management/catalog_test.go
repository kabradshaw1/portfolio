package management

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultCatalogContainsInitialActions(t *testing.T) {
	catalog := DefaultCatalog()
	for _, id := range []string{"reload_grafana_alerting", "run_postgres_backup_verify"} {
		action, ok := catalog.Get(id)
		if !ok {
			t.Fatalf("missing action %q", id)
		}
		if action.RiskTier != RiskLowRiskMutation {
			t.Fatalf("%s risk = %q", id, action.RiskTier)
		}
		if action.ScriptPath == "" || filepath.IsAbs(action.ScriptPath) {
			t.Fatalf("%s script path = %q", id, action.ScriptPath)
		}
		if action.Timeout <= 0 || action.Timeout > 45*time.Minute {
			t.Fatalf("%s timeout = %s", id, action.Timeout)
		}
	}
}

func TestDefaultCatalogScriptsExist(t *testing.T) {
	catalog := DefaultCatalog()
	repoRoot := filepath.Clean(filepath.Join("..", "..", "..", ".."))
	if err := catalog.ValidateScripts(repoRoot); err != nil {
		t.Fatalf("ValidateScripts() error = %v", err)
	}
}

func TestCatalogValidationRejectsBadEntries(t *testing.T) {
	tests := []struct {
		name    string
		actions []Action
	}{
		{name: "duplicate id", actions: []Action{{ID: "a", Title: "A", Description: "desc", RiskTier: RiskDiagnostic, ScriptPath: "scripts/ops/a.sh", Timeout: time.Second}, {ID: "a", Title: "B", Description: "desc", RiskTier: RiskDiagnostic, ScriptPath: "scripts/ops/b.sh", Timeout: time.Second}}},
		{name: "absolute path", actions: []Action{{ID: "a", Title: "A", Description: "desc", RiskTier: RiskDiagnostic, ScriptPath: "/tmp/a.sh", Timeout: time.Second}}},
		{name: "path traversal", actions: []Action{{ID: "a", Title: "A", Description: "desc", RiskTier: RiskDiagnostic, ScriptPath: "../a.sh", Timeout: time.Second}}},
		{name: "invalid risk", actions: []Action{{ID: "a", Title: "A", Description: "desc", RiskTier: RiskTier("surprise"), ScriptPath: "scripts/ops/a.sh", Timeout: time.Second}}},
		{name: "no timeout", actions: []Action{{ID: "a", Title: "A", Description: "desc", RiskTier: RiskDiagnostic, ScriptPath: "scripts/ops/a.sh"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewCatalog(tc.actions); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateScriptsRejectsMissingScript(t *testing.T) {
	catalog, err := NewCatalog([]Action{{ID: "a", Title: "A", Description: "desc", RiskTier: RiskDiagnostic, ScriptPath: "scripts/ops/missing.sh", Timeout: time.Second}})
	if err != nil {
		t.Fatal(err)
	}
	if err := catalog.ValidateScripts(t.TempDir()); err == nil {
		t.Fatal("expected missing script error")
	}
}
