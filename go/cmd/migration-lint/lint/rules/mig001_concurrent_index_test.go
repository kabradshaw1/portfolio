package rules

import (
	"testing"

	"github.com/kabradshaw1/portfolio/go/cmd/migration-lint/lint"
)

func TestMIG001_FlagsBareCreateIndex(t *testing.T) {
	violations := runRule(t, &MIG001{},
		`CREATE INDEX idx_users_email ON users (email);`)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(violations), violations)
	}
	if violations[0].Rule != "MIG001" {
		t.Errorf("rule: %s", violations[0].Rule)
	}
	if violations[0].Severity != lint.SeverityError {
		t.Errorf("severity: %v", violations[0].Severity)
	}
}

func TestMIG001_AcceptsConcurrently(t *testing.T) {
	violations := runRule(t, &MIG001{},
		`CREATE INDEX CONCURRENTLY idx_users_email ON users (email);`)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %+v", violations)
	}
}

func TestMIG001_AllowsIndexOnSameFileTable(t *testing.T) {
	src := `CREATE TABLE users (id UUID PRIMARY KEY, email TEXT);
CREATE INDEX idx_users_email ON users (email);`
	violations := runRule(t, &MIG001{}, src)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations on same-file table, got %+v", violations)
	}
}

func TestMIG001_IgnoreDirectiveSuppresses(t *testing.T) {
	src := `-- migration-lint: ignore=MIG001 reason="early-stage seed migration"
CREATE INDEX idx_users_email ON users (email);`
	violations := runRule(t, &MIG001{}, src)
	if len(violations) != 0 {
		t.Fatalf("expected directive to suppress, got %+v", violations)
	}
}
