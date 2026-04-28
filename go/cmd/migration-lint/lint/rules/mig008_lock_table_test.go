package rules

import (
	"testing"

	"github.com/kabradshaw1/portfolio/go/cmd/migration-lint/lint"
)

func TestMIG008_FlagsLockTable(t *testing.T) {
	violations := runRule(t, &MIG008{},
		`LOCK TABLE orders IN ACCESS EXCLUSIVE MODE;`)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %+v", violations)
	}
	if violations[0].Severity != lint.SeverityWarning {
		t.Errorf("severity: %v", violations[0].Severity)
	}
}

func TestMIG008_IgnoreDirectiveSuppresses(t *testing.T) {
	src := `-- migration-lint: ignore=MIG008 reason="serializing checkout-saga compaction"
LOCK TABLE orders IN ACCESS EXCLUSIVE MODE;`
	violations := runRule(t, &MIG008{}, src)
	if len(violations) != 0 {
		t.Fatalf("expected suppression, got %+v", violations)
	}
}
