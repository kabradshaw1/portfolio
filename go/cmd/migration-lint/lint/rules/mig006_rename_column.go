package rules

import (
	"fmt"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/kabradshaw1/portfolio/go/cmd/migration-lint/lint"
)

// MIG006 flags column renames. Like MIG005, this requires an explicit ignore
// directive — old code paths usually still reference the old name.
type MIG006 struct{}

func (MIG006) ID() string              { return "MIG006" }
func (MIG006) Severity() lint.Severity { return lint.SeverityError }
func (MIG006) Description() string {
	return "RENAME COLUMN must be confirmed with an ignore directive (callers usually still reference old name)"
}

func (r MIG006) Check(stmt *pg_query.RawStmt, ctx *lint.FileContext) []lint.Violation {
	rn := stmt.Stmt.GetRenameStmt()
	if rn == nil {
		return nil
	}
	if rn.RenameType != pg_query.ObjectType_OBJECT_COLUMN {
		return nil
	}
	return []lint.Violation{{
		File:     ctx.Filename,
		Line:     lint.LineFromOffset(ctx.Source, lint.StmtStart(ctx.Source, stmt)),
		Rule:     r.ID(),
		Severity: r.Severity(),
		Message: fmt.Sprintf(
			"RENAME COLUMN %q -> %q requires an ignore directive (recipe 3 — expand-and-contract)",
			rn.Subname, rn.Newname),
	}}
}
