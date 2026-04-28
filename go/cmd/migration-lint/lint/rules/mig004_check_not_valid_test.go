package rules

import "testing"

func TestMIG004_FlagsCheckWithoutNotValid(t *testing.T) {
	violations := runRule(t, &MIG004{},
		`ALTER TABLE orders ADD CONSTRAINT total_positive CHECK (total > 0);`)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %+v", violations)
	}
}

func TestMIG004_AcceptsNotValid(t *testing.T) {
	violations := runRule(t, &MIG004{},
		`ALTER TABLE orders ADD CONSTRAINT total_positive CHECK (total > 0) NOT VALID;`)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %+v", violations)
	}
}

func TestMIG004_IgnoresNonCheckConstraints(t *testing.T) {
	violations := runRule(t, &MIG004{},
		`ALTER TABLE orders ADD CONSTRAINT orders_pkey PRIMARY KEY (id);`)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %+v", violations)
	}
}
