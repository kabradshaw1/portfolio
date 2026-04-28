package lint

import pg_query "github.com/pganalyze/pg_query_go/v6"

// Severity classifies a violation.
type Severity int

const (
	SeverityWarning Severity = iota
	SeverityError
)

func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	default:
		return "unknown"
	}
}

// Violation is a single rule finding against one statement.
type Violation struct {
	File     string
	Line     int // 1-based
	Rule     string
	Severity Severity
	Message  string
}

// FileContext carries everything a rule needs to evaluate one file. The runner
// sets Index before invoking each rule so rules can inspect earlier statements
// in the same file (e.g. to allow CREATE INDEX on a CREATE TABLE that appears
// just above).
type FileContext struct {
	Filename   string
	Source     []byte
	Statements []*pg_query.RawStmt
	Index      int
	Ignores    []IgnoreDirective
}

// IgnoreDirective is a parsed `-- migration-lint: ignore=` comment. The
// AppliesToByte field is the byte offset of the statement this directive
// suppresses (set during attribution by parseIgnoreDirectives).
type IgnoreDirective struct {
	AppliesToByte int
	Rules         []string
	Reason        string
}

// Rule is the contract every linter rule implements.
type Rule interface {
	ID() string
	Description() string
	Severity() Severity
	Check(stmt *pg_query.RawStmt, ctx *FileContext) []Violation
}
