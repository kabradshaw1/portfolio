package rules

import "testing"

func TestMIG005_FlagsDropColumn(t *testing.T) {
	violations := runRule(t, &MIG005{},
		`ALTER TABLE users DROP COLUMN deprecated_field;`)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %+v", violations)
	}
}

func TestMIG005_IgnoreDirectiveSuppresses(t *testing.T) {
	src := `-- migration-lint: ignore=MIG005 reason="confirmed unused via pg_stat_user_columns 2026-04-15"
ALTER TABLE users DROP COLUMN deprecated_field;`
	violations := runRule(t, &MIG005{}, src)
	if len(violations) != 0 {
		t.Fatalf("expected suppression, got %+v", violations)
	}
}
