package rules

import "testing"

func TestMIG003_FlagsAlterColumnType(t *testing.T) {
	violations := runRule(t, &MIG003{},
		`ALTER TABLE orders ALTER COLUMN total TYPE BIGINT;`)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %+v", violations)
	}
}

func TestMIG003_IgnoresAddColumn(t *testing.T) {
	violations := runRule(t, &MIG003{},
		`ALTER TABLE orders ADD COLUMN total BIGINT;`)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %+v", violations)
	}
}
