package rules

import "testing"

func TestMIG006_FlagsRenameColumn(t *testing.T) {
	violations := runRule(t, &MIG006{},
		`ALTER TABLE users RENAME COLUMN email TO email_address;`)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %+v", violations)
	}
}

func TestMIG006_IgnoresRenameTable(t *testing.T) {
	violations := runRule(t, &MIG006{},
		`ALTER TABLE users RENAME TO accounts;`)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations on RENAME TABLE, got %+v", violations)
	}
}

func TestMIG006_IgnoreDirectiveSuppresses(t *testing.T) {
	src := `-- migration-lint: ignore=MIG006 reason="cutover migration; old name no longer referenced"
ALTER TABLE users RENAME COLUMN email TO email_address;`
	violations := runRule(t, &MIG006{}, src)
	if len(violations) != 0 {
		t.Fatalf("expected suppression, got %+v", violations)
	}
}
