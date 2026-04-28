package lint

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLineFromOffset(t *testing.T) {
	src := []byte("a\nbc\n\nd\n")
	cases := []struct {
		offset int
		want   int
	}{
		{0, 1},
		{2, 2},
		{5, 3},
		{6, 4},
	}
	for _, c := range cases {
		if got := LineFromOffset(src, c.offset); got != c.want {
			t.Errorf("offset %d: got line %d, want %d", c.offset, got, c.want)
		}
	}
}

func TestLint_NoRules_NoViolations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "001_smoke.up.sql")
	if err := os.WriteFile(path, []byte("CREATE TABLE x (id int);"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Lint([]string{path}, nil)
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected zero violations, got %d", len(got))
	}
}
