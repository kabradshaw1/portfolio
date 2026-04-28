package lint

import (
	"regexp"
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

// directiveRe matches `-- migration-lint: ignore=R1[,R2] reason="..."` anchored
// to the start of a comment line (allowing leading whitespace).
var directiveRe = regexp.MustCompile(
	`(?m)^[\t ]*--[\t ]*migration-lint:[\t ]*ignore=([A-Za-z0-9_,]+)[\t ]+reason="([^"]+)"[\t ]*$`,
)

// parseIgnoreDirectives scans src for `migration-lint: ignore=` comments and
// attaches each one to the next statement that begins after the comment line,
// provided no non-whitespace, non-comment content sits between the directive
// and that statement. Directives without a reason="..." are silently dropped
// (the regex requires it).
//
// Statements are keyed by their SQL keyword start (StmtStart), not pg_query's
// StmtLocation, because pg_query attributes leading comments to the
// statement's source range.
func parseIgnoreDirectives(src []byte, stmts []*pg_query.RawStmt) []IgnoreDirective {
	matches := directiveRe.FindAllSubmatchIndex(src, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]IgnoreDirective, 0, len(matches))
	for _, m := range matches {
		commentEnd := m[1]
		rules := strings.Split(string(src[m[2]:m[3]]), ",")
		for i := range rules {
			rules[i] = strings.TrimSpace(rules[i])
		}
		reason := string(src[m[4]:m[5]])

		stmt := nextStatementAfter(src, stmts, commentEnd)
		if stmt == nil {
			continue
		}
		keywordStart := StmtStart(src, stmt)
		between := src[commentEnd:keywordStart]
		if hasNonCommentContent(between) {
			continue
		}
		out = append(out, IgnoreDirective{
			AppliesToByte: keywordStart,
			Rules:         rules,
			Reason:        reason,
		})
	}
	return out
}

// isIgnored reports whether the given stmt is suppressed for ruleID.
func isIgnored(ctx *FileContext, stmt *pg_query.RawStmt, ruleID string) bool {
	stmtStart := StmtStart(ctx.Source, stmt)
	for _, dir := range ctx.Ignores {
		if dir.AppliesToByte != stmtStart {
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

// nextStatementAfter returns the first statement whose SQL keyword starts at or
// after byteOffset. It uses StmtStart (not StmtLocation) so leading comments
// attributed to the statement do not pull the match in front of the directive.
func nextStatementAfter(src []byte, stmts []*pg_query.RawStmt, byteOffset int) *pg_query.RawStmt {
	for _, s := range stmts {
		if StmtStart(src, s) >= byteOffset {
			return s
		}
	}
	return nil
}

// hasNonCommentContent reports whether b contains any token other than
// whitespace or `--` line comments.
func hasNonCommentContent(b []byte) bool {
	i := 0
	for i < len(b) {
		c := b[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '-' && i+1 < len(b) && b[i+1] == '-':
			for i < len(b) && b[i] != '\n' {
				i++
			}
		default:
			return true
		}
	}
	return false
}
