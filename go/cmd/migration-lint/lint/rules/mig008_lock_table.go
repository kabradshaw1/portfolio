package rules

import (
	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/kabradshaw1/portfolio/go/cmd/migration-lint/lint"
)

// MIG008 flags LOCK TABLE. Severity Warning — there are legitimate uses, but
// each one should ship with a documented rationale (via ignore directive).
type MIG008 struct{}

func (MIG008) ID() string              { return "MIG008" }
func (MIG008) Severity() lint.Severity { return lint.SeverityWarning }
func (MIG008) Description() string {
	return "LOCK TABLE must document its rationale via an ignore directive"
}

func (r MIG008) Check(stmt *pg_query.RawStmt, ctx *lint.FileContext) []lint.Violation {
	if stmt.Stmt.GetLockStmt() == nil {
		return nil
	}
	return []lint.Violation{{
		File:     ctx.Filename,
		Line:     lint.LineFromOffset(ctx.Source, lint.StmtStart(ctx.Source, stmt)),
		Rule:     r.ID(),
		Severity: r.Severity(),
		Message:  "LOCK TABLE without a documented purpose; add an ignore directive with reason= explaining why",
	}}
}
