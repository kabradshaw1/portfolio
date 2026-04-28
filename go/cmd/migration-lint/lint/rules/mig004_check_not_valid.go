package rules

import (
	"fmt"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/kabradshaw1/portfolio/go/cmd/migration-lint/lint"
)

// MIG004 flags `ADD CONSTRAINT ... CHECK (...)` without `NOT VALID`. The
// implicit validation locks the table for a full scan.
type MIG004 struct{}

func (MIG004) ID() string              { return "MIG004" }
func (MIG004) Severity() lint.Severity { return lint.SeverityError }
func (MIG004) Description() string {
	return "ADD CONSTRAINT ... CHECK without NOT VALID scans the entire table; use NOT VALID + VALIDATE CONSTRAINT"
}

func (r MIG004) Check(stmt *pg_query.RawStmt, ctx *lint.FileContext) []lint.Violation {
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
		if c.Subtype != pg_query.AlterTableType_AT_AddConstraint {
			continue
		}
		con := c.Def.GetConstraint()
		if con == nil {
			continue
		}
		if con.Contype != pg_query.ConstrType_CONSTR_CHECK {
			continue
		}
		if con.SkipValidation {
			continue
		}
		out = append(out, lint.Violation{
			File:     ctx.Filename,
			Line:     lint.LineFromOffset(ctx.Source, lint.StmtStart(ctx.Source, stmt)),
			Rule:     r.ID(),
			Severity: r.Severity(),
			Message: fmt.Sprintf(
				"CHECK constraint %q without NOT VALID; "+
					"add NOT VALID first then VALIDATE CONSTRAINT in a separate migration (recipe 5)",
				con.Conname),
		})
	}
	return out
}
