package rules

import (
	"fmt"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/kabradshaw1/portfolio/go/cmd/migration-lint/lint"
)

// MIG001 flags `CREATE INDEX ...` on a table that wasn't created earlier in
// the same file. Such an index acquires ACCESS EXCLUSIVE on the target for
// the duration of the build. Use `CREATE INDEX CONCURRENTLY` instead.
type MIG001 struct{}

func (MIG001) ID() string              { return "MIG001" }
func (MIG001) Severity() lint.Severity { return lint.SeverityError }
func (MIG001) Description() string {
	return "CREATE INDEX without CONCURRENTLY on an existing table; locks writers for the duration of the build"
}

func (r MIG001) Check(stmt *pg_query.RawStmt, ctx *lint.FileContext) []lint.Violation {
	idx := stmt.Stmt.GetIndexStmt()
	if idx == nil {
		return nil
	}
	if idx.Concurrent {
		return nil
	}
	target := idx.GetRelation().GetRelname()
	if target == "" {
		return nil
	}
	if tableCreatedEarlierInFile(ctx, target) {
		return nil
	}
	return []lint.Violation{{
		File:     ctx.Filename,
		Line:     lint.LineFromOffset(ctx.Source, lint.StmtStart(ctx.Source, stmt)),
		Rule:     r.ID(),
		Severity: r.Severity(),
		Message: fmt.Sprintf(
			"CREATE INDEX on %q without CONCURRENTLY locks the table; "+
				"use CREATE INDEX CONCURRENTLY in its own migration (see runbook recipe 4)", target),
	}}
}

// tableCreatedEarlierInFile reports whether name was the target of a CREATE
// TABLE earlier in this file.
func tableCreatedEarlierInFile(ctx *lint.FileContext, name string) bool {
	for i := 0; i < ctx.Index; i++ {
		c := ctx.Statements[i].Stmt.GetCreateStmt()
		if c == nil {
			continue
		}
		if c.GetRelation().GetRelname() == name {
			return true
		}
	}
	return false
}
