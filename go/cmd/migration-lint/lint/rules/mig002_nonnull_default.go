package rules

import (
	"fmt"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/kabradshaw1/portfolio/go/cmd/migration-lint/lint"
)

// MIG002 flags `ALTER TABLE ... ADD COLUMN ... NOT NULL DEFAULT <expr>` where
// <expr> is non-constant. PG 11+ fast-paths constant defaults; volatile
// expressions still rewrite every row.
type MIG002 struct{}

func (MIG002) ID() string              { return "MIG002" }
func (MIG002) Severity() lint.Severity { return lint.SeverityError }
func (MIG002) Description() string {
	return "ADD COLUMN NOT NULL with a non-constant default rewrites the entire table; backfill in a separate migration"
}

func (r MIG002) Check(stmt *pg_query.RawStmt, ctx *lint.FileContext) []lint.Violation {
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
		if c.Subtype != pg_query.AlterTableType_AT_AddColumn {
			continue
		}
		colDef := c.Def.GetColumnDef()
		if colDef == nil {
			continue
		}
		if !columnIsNotNull(colDef) {
			continue
		}
		def := defaultExpr(colDef)
		if def == nil {
			continue
		}
		if isConstant(def) {
			continue
		}
		out = append(out, lint.Violation{
			File:     ctx.Filename,
			Line:     lint.LineFromOffset(ctx.Source, lint.StmtStart(ctx.Source, stmt)),
			Rule:     r.ID(),
			Severity: r.Severity(),
			Message: fmt.Sprintf(
				"ADD COLUMN %q NOT NULL with non-constant default rewrites the table; "+
					"add nullable, backfill, then SET NOT NULL (see runbook recipe 1)",
				colDef.Colname),
		})
	}
	return out
}

func columnIsNotNull(c *pg_query.ColumnDef) bool {
	if c.IsNotNull {
		return true
	}
	for _, con := range c.Constraints {
		cc := con.GetConstraint()
		if cc == nil {
			continue
		}
		if cc.Contype == pg_query.ConstrType_CONSTR_NOTNULL {
			return true
		}
	}
	return false
}

func defaultExpr(c *pg_query.ColumnDef) *pg_query.Node {
	for _, con := range c.Constraints {
		cc := con.GetConstraint()
		if cc == nil {
			continue
		}
		if cc.Contype == pg_query.ConstrType_CONSTR_DEFAULT {
			return cc.RawExpr
		}
	}
	return nil
}

// isConstant reports whether expr is a literal (A_Const) or a TypeCast wrapping
// a literal. Anything else (FuncCall, expressions) is considered volatile.
func isConstant(expr *pg_query.Node) bool {
	if expr == nil {
		return false
	}
	if expr.GetAConst() != nil {
		return true
	}
	if tc := expr.GetTypeCast(); tc != nil {
		return isConstant(tc.Arg)
	}
	return false
}
