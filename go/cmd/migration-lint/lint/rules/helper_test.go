package rules

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kabradshaw1/portfolio/go/cmd/migration-lint/lint"
)

// runRule writes src to a temp file and runs the rule via the public Lint()
// entry, so the runner's ignore-directive logic and per-statement walking
// behave exactly as in production.
func runRule(t *testing.T, r lint.Rule, src string) []lint.Violation {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "002_test.up.sql")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	v, err := lint.Lint([]string{path}, []lint.Rule{r})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	return v
}
