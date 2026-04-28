package lint

import (
	"fmt"
	"os"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

// parseFile reads path from disk and parses it.
func parseFile(path string) (*FileContext, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return parseSource(path, src)
}

// parseSource parses src into a FileContext. The filename is used only for
// error reporting; src is the source of truth.
func parseSource(filename string, src []byte) (*FileContext, error) {
	result, err := pg_query.Parse(string(src))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filename, err)
	}
	return &FileContext{
		Filename:   filename,
		Source:     src,
		Statements: result.Stmts,
	}, nil
}

// StmtStart returns the byte offset of the first SQL keyword in stmt, skipping
// any leading whitespace and `--` line comments that pg_query attributed to
// the statement. Use this (not stmt.StmtLocation) when reporting line numbers
// or matching ignore directives, because pg_query treats preceding comments
// as part of the statement's source range.
func StmtStart(src []byte, stmt *pg_query.RawStmt) int {
	i := int(stmt.StmtLocation)
	end := i + int(stmt.StmtLen)
	if end > len(src) {
		end = len(src)
	}
	for i < end {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '-' && i+1 < end && src[i+1] == '-':
			for i < end && src[i] != '\n' {
				i++
			}
		default:
			return i
		}
	}
	return i
}
