package rules

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kabradshaw1/portfolio/go/cmd/migration-lint/lint"
)

// TestSnapshot_RepoMigrations runs every rule over every .up.sql file in the
// repository and asserts zero violations. It pins the current clean baseline
// so any new unsafe pattern (or any drift in an existing migration) breaks
// the test until either the migration is fixed or an ignore directive is
// added.
//
// The test resolves the repo root by walking up from this test file until
// it finds a directory whose name matches the worktree (it tolerates being
// run from any worktree). It skips when no migrations are found, so the
// rule package remains testable in isolation (e.g. when extracted).
func TestSnapshot_RepoMigrations(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Skipf("could not resolve repo root from %s: %v", mustGetwd(t), err)
	}
	pattern := filepath.Join(root, "go", "*", "migrations", "*.up.sql")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Skipf("no migration files found at %s", pattern)
	}
	allRules := []lint.Rule{
		&MIG001{},
		&MIG002{},
		&MIG003{},
		&MIG004{},
		&MIG005{},
		&MIG006{},
		&MIG007{},
		&MIG008{},
	}
	violations, err := lint.Lint(matches, allRules)
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if len(violations) != 0 {
		t.Errorf("expected zero violations across %d migrations; got %d:", len(matches), len(violations))
		for _, v := range violations {
			t.Errorf("  %s:%d [%s %s] %s", v.File, v.Line, v.Rule, v.Severity, v.Message)
		}
	}
}

// repoRoot walks up from the current working directory until it finds a
// directory containing both a `go/` subdirectory and a `Makefile`. That
// matches the project layout in any worktree.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		_, errGo := os.Stat(filepath.Join(dir, "go"))
		_, errMake := os.Stat(filepath.Join(dir, "Makefile"))
		if errGo == nil && errMake == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		return "<getwd failed>"
	}
	return wd
}
