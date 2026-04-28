package rules

import (
	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/kabradshaw1/portfolio/go/cmd/migration-lint/lint"
)

// MIG007 fires when a `CREATE INDEX CONCURRENTLY` shares a file with any other
// statement. golang-migrate wraps each migration in a transaction, and
// CONCURRENTLY cannot run inside one.
type MIG007 struct{}

func (MIG007) ID() string              { return "MIG007" }
func (MIG007) Severity() lint.Severity { return lint.SeverityError }
func (MIG007) Description() string {
	return "CREATE INDEX CONCURRENTLY cannot run inside a transaction; isolate it in its own migration file"
}

func (r MIG007) Check(stmt *pg_query.RawStmt, ctx *lint.FileContext) []lint.Violation {
	idx := stmt.Stmt.GetIndexStmt()
	if idx == nil || !idx.Concurrent {
		return nil
	}
	if len(ctx.Statements) == 1 {
		return nil
	}
	return []lint.Violation{{
		File:     ctx.Filename,
		Line:     lint.LineFromOffset(ctx.Source, lint.StmtStart(ctx.Source, stmt)),
		Rule:     r.ID(),
		Severity: r.Severity(),
		Message: "CREATE INDEX CONCURRENTLY shares this file with " +
			"other statements; golang-migrate runs each file in a transaction. " +
			"Move CONCURRENTLY into its own migration (recipe 4).",
	}}
}
