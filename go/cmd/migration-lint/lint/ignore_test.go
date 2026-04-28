package lint

import (
	"reflect"
	"testing"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

func TestParseIgnoreDirectives_SingleRule(t *testing.T) {
	src := []byte(`-- migration-lint: ignore=MIG001 reason="empty table"
CREATE INDEX idx_x ON t (a);
`)
	stmts := mustParse(t, src)
	got := parseIgnoreDirectives(src, stmts)
	want := []IgnoreDirective{{
		AppliesToByte: StmtStart(src, stmts[0]),
		Rules:         []string{"MIG001"},
		Reason:        "empty table",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestParseIgnoreDirectives_MultipleRules(t *testing.T) {
	src := []byte(`-- migration-lint: ignore=MIG001,MIG004 reason="seed migration"
CREATE INDEX idx_x ON t (a);
`)
	stmts := mustParse(t, src)
	got := parseIgnoreDirectives(src, stmts)
	if len(got) != 1 {
		t.Fatalf("expected 1 directive, got %d", len(got))
	}
	if !reflect.DeepEqual(got[0].Rules, []string{"MIG001", "MIG004"}) {
		t.Errorf("rules: %v", got[0].Rules)
	}
}

func TestParseIgnoreDirectives_RequiresReason(t *testing.T) {
	src := []byte(`-- migration-lint: ignore=MIG001
CREATE INDEX idx_x ON t (a);
`)
	stmts := mustParse(t, src)
	if got := parseIgnoreDirectives(src, stmts); len(got) != 0 {
		t.Errorf("expected directive without reason to be dropped, got %+v", got)
	}
}

func TestParseIgnoreDirectives_AttachesToNextStatement(t *testing.T) {
	src := []byte(`CREATE TABLE t (id int);
-- migration-lint: ignore=MIG001 reason="seed"
CREATE INDEX idx_x ON t (a);
`)
	stmts := mustParse(t, src)
	got := parseIgnoreDirectives(src, stmts)
	if len(got) != 1 {
		t.Fatalf("expected 1 directive, got %d", len(got))
	}
	wantStart := StmtStart(src, stmts[1])
	if got[0].AppliesToByte != wantStart {
		t.Errorf("attached byte=%d want=%d", got[0].AppliesToByte, wantStart)
	}
}

func TestParseIgnoreDirectives_NotApplied_IfStatementBetween(t *testing.T) {
	src := []byte(`-- migration-lint: ignore=MIG001 reason="seed"
SELECT 1;
CREATE INDEX idx_x ON t (a);
`)
	stmts := mustParse(t, src)
	got := parseIgnoreDirectives(src, stmts)
	if len(got) != 1 {
		t.Fatalf("expected 1 directive, got %d", len(got))
	}
	wantStart := StmtStart(src, stmts[0])
	if got[0].AppliesToByte != wantStart {
		t.Errorf("attached to wrong stmt byte=%d", got[0].AppliesToByte)
	}
}

func mustParse(t *testing.T, src []byte) []*pg_query.RawStmt {
	t.Helper()
	r, err := pg_query.Parse(string(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return r.Stmts
}
