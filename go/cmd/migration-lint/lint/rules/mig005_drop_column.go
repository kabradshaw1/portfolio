package rules

import (
	"fmt"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/kabradshaw1/portfolio/go/cmd/migration-lint/lint"
)

// MIG005 flags every DROP COLUMN. The rule cannot determine whether the
// application has stopped writing this column, so dropping requires an
// explicit ignore directive that documents the verification.
type MIG005 struct{}

func (MIG005) ID() string              { return "MIG005" }
func (MIG005) Severity() lint.Severity { return lint.SeverityError }
func (MIG005) Description() string {
	return "DROP COLUMN must be confirmed with an ignore directive documenting that the app has stopped using it"
}

func (r MIG005) Check(stmt *pg_query.RawStmt, ctx *lint.FileContext) []lint.Violation {
	alter := stmt.Stmt.GetAlterTableStmt()
	if alter == nil {
		return nil
	}
	var out []lint.Violation
	for _, cmd := range alter.Cmds {
		c := cmd.GetAlterTableCmd()
		if c == nil {
			continue
		}
		if c.Subtype != pg_query.AlterTableType_AT_DropColumn {
			continue
		}
		out = append(out, lint.Violation{
			File:     ctx.Filename,
			Line:     lint.LineFromOffset(ctx.Source, lint.StmtStart(ctx.Source, stmt)),
			Rule:     r.ID(),
			Severity: r.Severity(),
			Message: fmt.Sprintf(
				"DROP COLUMN %q requires an ignore directive with a documented reason "+
					"(recipe 2 in the runbook)", c.Name),
		})
	}
	return out
}
