package rules

import "testing"

func TestMIG007_FlagsConcurrentlyMixedWithOtherStatement(t *testing.T) {
	src := `CREATE INDEX CONCURRENTLY idx_users_email ON users (email);
ALTER TABLE users ADD COLUMN tier TEXT;`
	violations := runRule(t, &MIG007{}, src)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %+v", violations)
	}
	if violations[0].Rule != "MIG007" {
		t.Errorf("rule: %s", violations[0].Rule)
	}
}

func TestMIG007_AllowsLoneConcurrently(t *testing.T) {
	src := `CREATE INDEX CONCURRENTLY idx_users_email ON users (email);`
	violations := runRule(t, &MIG007{}, src)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %+v", violations)
	}
}

func TestMIG007_IgnoresFileWithoutConcurrently(t *testing.T) {
	src := `ALTER TABLE users ADD COLUMN tier TEXT;
ALTER TABLE users ADD COLUMN active BOOLEAN DEFAULT true;`
	violations := runRule(t, &MIG007{}, src)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %+v", violations)
	}
}
