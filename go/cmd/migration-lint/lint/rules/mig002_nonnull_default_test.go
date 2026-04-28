package rules

import "testing"

func TestMIG002_FlagsNotNullWithVolatileDefault(t *testing.T) {
	violations := runRule(t, &MIG002{},
		`ALTER TABLE users ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT now();`)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %+v", violations)
	}
	if violations[0].Rule != "MIG002" {
		t.Errorf("rule: %s", violations[0].Rule)
	}
}

func TestMIG002_AcceptsConstantDefault(t *testing.T) {
	violations := runRule(t, &MIG002{},
		`ALTER TABLE users ADD COLUMN active BOOLEAN NOT NULL DEFAULT true;`)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %+v", violations)
	}
}

func TestMIG002_AcceptsNullableColumn(t *testing.T) {
	violations := runRule(t, &MIG002{},
		`ALTER TABLE users ADD COLUMN updated_at TIMESTAMPTZ DEFAULT now();`)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations on nullable col, got %+v", violations)
	}
}

func TestMIG002_AcceptsNotNullNoDefault(t *testing.T) {
	violations := runRule(t, &MIG002{},
		`ALTER TABLE users ADD COLUMN tier TEXT NOT NULL;`)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %+v", violations)
	}
}
