package rules

import (
	"fmt"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/kabradshaw1/portfolio/go/cmd/migration-lint/lint"
)

// MIG003 flags ALTER COLUMN ... TYPE — generally rewrites every row unless the
// type change is binary-compatible. Use expand-and-contract instead.
type MIG003 struct{}

func (MIG003) ID() string              { return "MIG003" }
func (MIG003) Severity() lint.Severity { return lint.SeverityError }
func (MIG003) Description() string {
	return "ALTER COLUMN TYPE rewrites the table; expand-and-contract instead"
}

func (r MIG003) Check(stmt *pg_query.RawStmt, ctx *lint.FileContext) []lint.Violation {
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
		if c.Subtype != pg_query.AlterTableType_AT_AlterColumnType {
			continue
		}
		out = append(out, lint.Violation{
			File:     ctx.Filename,
			Line:     lint.LineFromOffset(ctx.Source, lint.StmtStart(ctx.Source, stmt)),
			Rule:     r.ID(),
			Severity: r.Severity(),
			Message: fmt.Sprintf(
				"ALTER COLUMN %q TYPE will rewrite the table; "+
					"add a new column, backfill, switch reads/writes, drop old (recipe 7)",
				c.Name),
		})
	}
	return out
}
