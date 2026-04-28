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
