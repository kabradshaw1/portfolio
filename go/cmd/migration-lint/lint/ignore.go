package lint

import pg_query "github.com/pganalyze/pg_query_go/v6"

// parseIgnoreDirectives is implemented in Task 4. Returns nil for now.
func parseIgnoreDirectives(_ []byte, _ []*pg_query.RawStmt) []IgnoreDirective {
	return nil
}

// isIgnored reports whether the given stmt is suppressed for ruleID.
func isIgnored(ctx *FileContext, stmt *pg_query.RawStmt, ruleID string) bool {
	for _, dir := range ctx.Ignores {
		if dir.AppliesToByte != int(stmt.StmtLocation) {
			continue
		}
		for _, r := range dir.Rules {
			if r == ruleID {
				return true
			}
		}
	}
	return false
}
