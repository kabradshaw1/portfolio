# PostgreSQL Migration Linter — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go CLI (`migration-lint`) that statically analyzes `golang-migrate` `.up.sql` files for operationally unsafe DDL patterns, integrate it into `make preflight-go-migrations`, ship a worked-example migration that demonstrates the safe pattern, a runbook of safe migration recipes, and a companion ADR.

**Architecture:** Standalone Go module at `go/cmd/migration-lint/` (own `go.mod`) with a CLI entrypoint (`main.go`) and a `lint` package containing the rule infrastructure and eight rules. SQL is parsed via `github.com/pganalyze/pg_query_go/v6` (CGO wrapper around `libpg_query`, the upstream PostgreSQL parser). Ignore directives (`-- migration-lint: ignore=MIG001 reason="..."`) are scraped from comments immediately preceding each statement and matched by source byte range.

**Tech Stack:**
- Go 1.26 (CGO enabled — repo already requires it)
- `github.com/pganalyze/pg_query_go/v6` for SQL parsing
- Standard `testing` package — no external test deps
- `golangci-lint` (already configured in repo root) for lint
- Wired into existing `make preflight-go-migrations` target and CI matrix

---

## File Structure

**New files (to create):**

```
go/cmd/migration-lint/
├── go.mod                            # module github.com/kabradshaw1/portfolio/go/cmd/migration-lint
├── go.sum
├── main.go                           # CLI entry: argv → Lint() → format → exit code
├── main_test.go                      # smoke-test the CLI plumbing
├── README.md                         # short pointer to runbook + flag reference
└── lint/
    ├── lint.go                       # Lint(files []string) ([]Violation, error) + LineFromOffset
    ├── lint_test.go                  # multi-file, ignore-directive, snapshot tests
    ├── rule.go                       # Rule interface, Violation, Severity, FileContext
    ├── parser.go                     # parseFile() — wraps pg_query_go, returns FileContext
    ├── parser_test.go                # parser unit tests (parse error path)
    ├── ignore.go                     # parseIgnoreDirectives(src) → []Directive
    ├── ignore_test.go                # directive parser unit tests
    ├── runner.go                     # Run(files, rules) — composes parser, ignores, rules
    └── rules/
        ├── mig001_concurrent_index.go
        ├── mig001_concurrent_index_test.go
        ├── mig002_nonnull_default.go
        ├── mig002_nonnull_default_test.go
        ├── mig003_alter_column_type.go
        ├── mig003_alter_column_type_test.go
        ├── mig004_check_not_valid.go
        ├── mig004_check_not_valid_test.go
        ├── mig005_drop_column.go
        ├── mig005_drop_column_test.go
        ├── mig006_rename_column.go
        ├── mig006_rename_column_test.go
        ├── mig007_concurrent_in_tx.go
        ├── mig007_concurrent_in_tx_test.go
        ├── mig008_lock_table.go
        └── mig008_lock_table_test.go

go/product-service/migrations/
├── 004_add_product_search_index.up.sql       # NEW worked-example migration
└── 004_add_product_search_index.down.sql

docs/runbooks/
└── postgres-migrations.md                     # NEW playbook (8 recipes)

docs/adr/database/
└── migration-lint.md                          # NEW companion ADR (new dir)
```

**Existing files (to modify):**

- `Makefile` — extend `preflight-go-migrations` to build and run the linter before spinning up Postgres
- `.github/workflows/ci.yml` — add `migration-lint` matrix entries to `go-lint` and `go-tests`
- `go/<service>/migrations/*.up.sql` — add `-- migration-lint: ignore=...` directives to existing migrations whose patterns are safe-in-context but trigger rules (early-stage migrations that ran on empty tables)

**Decomposition rationale:** Each rule lives in its own pair of files (`mig0XX*.go` + `_test.go`) so that adding a new rule is mechanical and rules can be reasoned about independently. The `lint/` package is its own subpackage rather than `internal/lint/` so the `rules` subpackage can import from it without internal-visibility friction. The CLI binary is at the module root (`main.go`) for the standard Go layout.

---

## Pre-flight notes for the executing engineer

- **CGO is required.** `pg_query_go` wraps `libpg_query` — Go must build with `CGO_ENABLED=1`. macOS ships clang; Ubuntu CI runners ship gcc; both are fine. If you see `cgo: C compiler "cc" not found`, install build-essential (Linux) or accept Xcode CLT (macOS).
- **The shared `go/pkg/` module is NOT a dependency of this binary.** This module is intentionally standalone — it has no runtime overlap with services and no need for `apperror`/`tracing`/`resilience`.
- **Linter binary is throw-away.** Always built fresh into `/tmp/migration-lint` from the worktree by the Makefile. Do not commit the binary.
- **Do not modify historical migrations to silence rules.** `golang-migrate` tracks migrations by version number, so editing a migration that has already been applied creates dev/prod divergence. Use ignore directives instead. The worked-example migration is the only place we add new SQL.
- **Existing test pattern.** Project Go services use `go test ./... -v -race`. Match that.

---

## Task 1: Bootstrap the module and CLI entrypoint

**Files:**
- Create: `go/cmd/migration-lint/go.mod`
- Create: `go/cmd/migration-lint/main.go`
- Create: `go/cmd/migration-lint/README.md`
- Create: `go/cmd/migration-lint/main_test.go`

- [ ] **Step 1: Create `go/cmd/migration-lint/go.mod`**

```
module github.com/kabradshaw1/portfolio/go/cmd/migration-lint

go 1.26
```

- [ ] **Step 2: Write the failing CLI smoke test (`main_test.go`)**

```go
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestCLI_NoArgs ensures the binary exits with code 2 (invocation error) when no files are supplied.
func TestCLI_NoArgs(t *testing.T) {
	bin := buildBinary(t)
	cmd := exec.Command(bin)
	err := cmd.Run()
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if exitErr.ExitCode() != 2 {
		t.Fatalf("expected exit code 2, got %d", exitErr.ExitCode())
	}
}

func buildBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "migration-lint")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build failed: %v", err)
	}
	return bin
}
```

- [ ] **Step 3: Write minimal `main.go` to make the test pass**

```go
// migration-lint statically analyzes golang-migrate .up.sql files for
// operationally unsafe DDL patterns. See docs/runbooks/postgres-migrations.md.
package main

import (
	"fmt"
	"os"
)

const (
	exitClean        = 0
	exitViolation    = 1
	exitInvocation   = 2
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: migration-lint <file.sql> [file.sql ...]")
		os.Exit(exitInvocation)
	}
	// Wired up in later tasks.
	os.Exit(exitClean)
}
```

- [ ] **Step 4: Run the test**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent/feat-migration-lint/go/cmd/migration-lint
go test ./... -v
```

Expected: `PASS — TestCLI_NoArgs`.

- [ ] **Step 5: Write a short README at `go/cmd/migration-lint/README.md`**

```markdown
# migration-lint

Static analyzer for `golang-migrate` `.up.sql` files. Flags operationally unsafe
DDL patterns (table-rewrite ALTERs, blocking CREATE INDEX, missing NOT VALID, etc.)
before the migration ever reaches Postgres.

See `docs/runbooks/postgres-migrations.md` for the safe-pattern playbook and
`docs/adr/database/migration-lint.md` for design rationale.

## Usage

    migration-lint go/*/migrations/*.up.sql

Exit codes:
- `0` — no violations
- `1` — at least one error-severity violation
- `2` — invocation error or parse failure

## Ignore directives

A single-line comment immediately above a statement opts that statement out
of one or more rules. The `reason="..."` field is required.

    -- migration-lint: ignore=MIG001 reason="initial table creation, table is empty"
    CREATE INDEX idx_users_email ON users (email);

Multiple rules: `ignore=MIG001,MIG004`.
```

- [ ] **Step 6: Commit**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent/feat-migration-lint
git add go/cmd/migration-lint/
git commit -m "feat(migration-lint): bootstrap CLI module skeleton"
```

---

## Task 2: Add pg_query_go dependency and parse a trivial statement

**Files:**
- Modify: `go/cmd/migration-lint/go.mod`
- Create: `go/cmd/migration-lint/lint/parser.go`
- Create: `go/cmd/migration-lint/lint/parser_test.go`

- [ ] **Step 1: Add the parser dep**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent/feat-migration-lint/go/cmd/migration-lint
go get github.com/pganalyze/pg_query_go/v6
go mod tidy
```

- [ ] **Step 2: Write the failing parser test (`lint/parser_test.go`)**

```go
package lint

import "testing"

func TestParseFile_SingleCreateTable(t *testing.T) {
	src := []byte("CREATE TABLE users (id UUID PRIMARY KEY);")
	ctx, err := parseSource("inline.sql", src)
	if err != nil {
		t.Fatalf("parseSource: %v", err)
	}
	if got := len(ctx.Statements); got != 1 {
		t.Fatalf("expected 1 statement, got %d", got)
	}
	if ctx.Filename != "inline.sql" {
		t.Errorf("Filename: got %q want inline.sql", ctx.Filename)
	}
}

func TestParseFile_SyntaxError(t *testing.T) {
	src := []byte("CREATE TABBLE users (id UUID);")
	if _, err := parseSource("bad.sql", src); err == nil {
		t.Fatal("expected parse error, got nil")
	}
}
```

- [ ] **Step 3: Implement `lint/parser.go`**

```go
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
```

- [ ] **Step 4: Add minimal `lint/rule.go` so `FileContext` exists**

```go
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

// Violation is a single rule finding.
type Violation struct {
	File     string
	Line     int // 1-based
	Rule     string
	Severity Severity
	Message  string
}

// FileContext is everything a rule needs to evaluate one file.
type FileContext struct {
	Filename   string
	Source     []byte
	Statements []*pg_query.RawStmt
	// Index of the statement currently being checked. Set by the runner.
	Index int
	// Ignore directives parsed from the source, populated in Task 6.
	Ignores []IgnoreDirective
}

// IgnoreDirective is a parsed `-- migration-lint: ignore=` comment. Defined
// here so FileContext can reference it; the parser is in ignore.go.
type IgnoreDirective struct {
	// AppliesToByte is the byte offset of the statement this directive
	// suppresses (set during attribution).
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
```

- [ ] **Step 5: Run the parser tests**

```bash
go test ./lint -run TestParseFile -v
```

Expected: `PASS — TestParseFile_SingleCreateTable`, `PASS — TestParseFile_SyntaxError`.

- [ ] **Step 6: Commit**

```bash
git add go/cmd/migration-lint/
git commit -m "feat(migration-lint): add pg_query_go parser wrapper and core types"
```

---

## Task 3: Implement byte-offset-to-line conversion and a minimal Lint() entrypoint

**Files:**
- Modify: `go/cmd/migration-lint/lint/lint.go` (create)
- Modify: `go/cmd/migration-lint/lint/lint_test.go` (create)

- [ ] **Step 1: Write failing tests for `LineFromOffset` and a no-rule `Lint()` (`lint/lint_test.go`)**

```go
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
		{0, 1}, // 'a'
		{2, 2}, // 'b'
		{5, 3}, // empty line
		{6, 4}, // 'd'
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
```

- [ ] **Step 2: Implement `lint/lint.go`**

```go
package lint

// LineFromOffset returns the 1-based line number containing the byte at offset
// in src. Out-of-range offsets clamp to the last line.
func LineFromOffset(src []byte, offset int) int {
	if offset < 0 {
		offset = 0
	}
	if offset > len(src) {
		offset = len(src)
	}
	line := 1
	for i := 0; i < offset; i++ {
		if src[i] == '\n' {
			line++
		}
	}
	return line
}

// Lint parses each file in paths and runs the supplied rules over each
// statement. Returns the aggregated violations. A parse error stops processing
// of that file (and is returned as the error) but other files continue.
//
// Caller responsibility: dedupe paths, expand globs.
func Lint(paths []string, rules []Rule) ([]Violation, error) {
	var all []Violation
	for _, path := range paths {
		ctx, err := parseFile(path)
		if err != nil {
			return all, err
		}
		ctx.Ignores = parseIgnoreDirectives(ctx.Source, ctx.Statements)
		for i, stmt := range ctx.Statements {
			ctx.Index = i
			for _, rule := range rules {
				if isIgnored(ctx, stmt, rule.ID()) {
					continue
				}
				all = append(all, rule.Check(stmt, ctx)...)
			}
		}
	}
	return all, nil
}
```

- [ ] **Step 3: Add a stub `parseIgnoreDirectives` and `isIgnored` so the file compiles**

In a new file `lint/ignore.go`:

```go
package lint

import pg_query "github.com/pganalyze/pg_query_go/v6"

// parseIgnoreDirectives is implemented in Task 6. Returns nil for now.
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
```

- [ ] **Step 4: Run the tests**

```bash
go test ./lint -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add go/cmd/migration-lint/
git commit -m "feat(migration-lint): add Lint() entry, line conversion, ignore stubs"
```

---

## Task 4: Implement the ignore-directive parser

**Files:**
- Modify: `go/cmd/migration-lint/lint/ignore.go`
- Create: `go/cmd/migration-lint/lint/ignore_test.go`

- [ ] **Step 1: Write failing tests for the directive parser (`lint/ignore_test.go`)**

```go
package lint

import (
	"reflect"
	"testing"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

func TestParseIgnoreDirectives_SingleRule(t *testing.T) {
	src := []byte(`-- migration-lint: ignore=MIG001 reason="empty table"
CREATE INDEX idx_x ON t (a);
`)
	stmts := mustParse(t, src)
	got := parseIgnoreDirectives(src, stmts)
	want := []IgnoreDirective{{
		AppliesToByte: int(stmts[0].StmtLocation),
		Rules:         []string{"MIG001"},
		Reason:        "empty table",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestParseIgnoreDirectives_MultipleRules(t *testing.T) {
	src := []byte(`-- migration-lint: ignore=MIG001,MIG004 reason="seed migration"
CREATE INDEX idx_x ON t (a);
`)
	stmts := mustParse(t, src)
	got := parseIgnoreDirectives(src, stmts)
	if len(got) != 1 {
		t.Fatalf("expected 1 directive, got %d", len(got))
	}
	if !reflect.DeepEqual(got[0].Rules, []string{"MIG001", "MIG004"}) {
		t.Errorf("rules: %v", got[0].Rules)
	}
}

func TestParseIgnoreDirectives_RequiresReason(t *testing.T) {
	src := []byte(`-- migration-lint: ignore=MIG001
CREATE INDEX idx_x ON t (a);
`)
	stmts := mustParse(t, src)
	if got := parseIgnoreDirectives(src, stmts); len(got) != 0 {
		t.Errorf("expected directive without reason to be dropped, got %+v", got)
	}
}

func TestParseIgnoreDirectives_AttachesToNextStatement(t *testing.T) {
	src := []byte(`CREATE TABLE t (id int);
-- migration-lint: ignore=MIG001 reason="seed"
CREATE INDEX idx_x ON t (a);
`)
	stmts := mustParse(t, src)
	got := parseIgnoreDirectives(src, stmts)
	if len(got) != 1 {
		t.Fatalf("expected 1 directive, got %d", len(got))
	}
	// CREATE INDEX is the second statement.
	if got[0].AppliesToByte != int(stmts[1].StmtLocation) {
		t.Errorf("attached byte=%d want=%d", got[0].AppliesToByte, stmts[1].StmtLocation)
	}
}

func TestParseIgnoreDirectives_NotApplied_IfStatementBetween(t *testing.T) {
	src := []byte(`-- migration-lint: ignore=MIG001 reason="seed"
SELECT 1;
CREATE INDEX idx_x ON t (a);
`)
	stmts := mustParse(t, src)
	got := parseIgnoreDirectives(src, stmts)
	// The directive applies to the SELECT, not the CREATE INDEX.
	if len(got) != 1 {
		t.Fatalf("expected 1 directive, got %d", len(got))
	}
	if got[0].AppliesToByte != int(stmts[0].StmtLocation) {
		t.Errorf("attached to wrong stmt byte=%d", got[0].AppliesToByte)
	}
}

func mustParse(t *testing.T, src []byte) []*pg_query.RawStmt {
	t.Helper()
	r, err := pg_query.Parse(string(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return r.Stmts
}
```

- [ ] **Step 2: Replace the stub `parseIgnoreDirectives` with the real implementation**

Edit `lint/ignore.go`:

```go
package lint

import (
	"regexp"
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

// directiveRe matches `-- migration-lint: ignore=R1[,R2] reason="..."`
// anchored to the start of a comment line (allowing leading whitespace).
var directiveRe = regexp.MustCompile(
	`(?m)^[\t ]*--[\t ]*migration-lint:[\t ]*ignore=([A-Za-z0-9_,]+)[\t ]+reason="([^"]+)"[\t ]*$`,
)

// parseIgnoreDirectives scans src for `migration-lint: ignore=` comments and
// attaches each one to the *next* statement that begins after the comment line,
// with no intervening non-whitespace, non-comment content. Directives without
// a `reason="..."` are silently dropped (the regex requires it).
func parseIgnoreDirectives(src []byte, stmts []*pg_query.RawStmt) []IgnoreDirective {
	matches := directiveRe.FindAllSubmatchIndex(src, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]IgnoreDirective, 0, len(matches))
	for _, m := range matches {
		// m[0]/m[1] = full match, m[2]/m[3] = rules group, m[4]/m[5] = reason.
		commentEnd := m[1]
		rules := strings.Split(string(src[m[2]:m[3]]), ",")
		for i := range rules {
			rules[i] = strings.TrimSpace(rules[i])
		}
		reason := string(src[m[4]:m[5]])

		stmt := nextStatementAfter(stmts, commentEnd)
		if stmt == nil {
			continue
		}
		// Reject if any non-whitespace, non-comment content appears between
		// commentEnd and stmt.StmtLocation. The directive only applies if it's
		// "immediately above" — see spec.
		between := src[commentEnd:stmt.StmtLocation]
		if hasNonCommentContent(between) {
			// Treat as not-applicable; do not attach.
			continue
		}
		out = append(out, IgnoreDirective{
			AppliesToByte: int(stmt.StmtLocation),
			Rules:         rules,
			Reason:        reason,
		})
	}
	return out
}

func nextStatementAfter(stmts []*pg_query.RawStmt, byteOffset int) *pg_query.RawStmt {
	for _, s := range stmts {
		if int(s.StmtLocation) >= byteOffset {
			return s
		}
	}
	return nil
}

// hasNonCommentContent reports whether the given byte slice contains any token
// other than whitespace or `--` line comments.
func hasNonCommentContent(b []byte) bool {
	i := 0
	for i < len(b) {
		c := b[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '-' && i+1 < len(b) && b[i+1] == '-':
			// Skip the rest of the line.
			for i < len(b) && b[i] != '\n' {
				i++
			}
		default:
			return true
		}
	}
	return false
}
```

- [ ] **Step 3: Run the directive tests**

```bash
go test ./lint -run TestParseIgnoreDirectives -v
```

Expected: all five tests PASS.

- [ ] **Step 4: Commit**

```bash
git add go/cmd/migration-lint/
git commit -m "feat(migration-lint): parse ignore directives with reason= required"
```

---

## Task 5: MIG001 — `CREATE INDEX` without `CONCURRENTLY` on existing table

**Files:**
- Create: `go/cmd/migration-lint/lint/rules/mig001_concurrent_index.go`
- Create: `go/cmd/migration-lint/lint/rules/mig001_concurrent_index_test.go`

- [ ] **Step 1: Write failing tests (`lint/rules/mig001_concurrent_index_test.go`)**

```go
package rules

import (
	"testing"

	"github.com/kabradshaw1/portfolio/go/cmd/migration-lint/lint"
)

func TestMIG001_FlagsBareCreateIndex(t *testing.T) {
	violations := runRule(t, &MIG001{},
		`CREATE INDEX idx_users_email ON users (email);`)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(violations), violations)
	}
	if violations[0].Rule != "MIG001" {
		t.Errorf("rule: %s", violations[0].Rule)
	}
	if violations[0].Severity != lint.SeverityError {
		t.Errorf("severity: %v", violations[0].Severity)
	}
}

func TestMIG001_AcceptsConcurrently(t *testing.T) {
	violations := runRule(t, &MIG001{},
		`CREATE INDEX CONCURRENTLY idx_users_email ON users (email);`)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %+v", violations)
	}
}

func TestMIG001_AllowsIndexOnSameFileTable(t *testing.T) {
	src := `CREATE TABLE users (id UUID PRIMARY KEY, email TEXT);
CREATE INDEX idx_users_email ON users (email);`
	violations := runRule(t, &MIG001{}, src)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations on same-file table, got %+v", violations)
	}
}

func TestMIG001_IgnoreDirectiveSuppresses(t *testing.T) {
	src := `-- migration-lint: ignore=MIG001 reason="early-stage seed migration"
CREATE INDEX idx_users_email ON users (email);`
	violations := runRule(t, &MIG001{}, src)
	if len(violations) != 0 {
		t.Fatalf("expected directive to suppress, got %+v", violations)
	}
}
```

- [ ] **Step 2: Add the test helper at `lint/rules/helper_test.go`**

```go
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
```

- [ ] **Step 3: Implement MIG001 (`lint/rules/mig001_concurrent_index.go`)**

```go
package rules

import (
	"fmt"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/kabradshaw1/portfolio/go/cmd/migration-lint/lint"
)

// MIG001 flags `CREATE INDEX ...` on a table that wasn't created earlier in
// the same file. Such an index acquires ACCESS EXCLUSIVE on the target for
// the duration of the build. Use `CREATE INDEX CONCURRENTLY` instead.
type MIG001 struct{}

func (MIG001) ID() string             { return "MIG001" }
func (MIG001) Severity() lint.Severity { return lint.SeverityError }
func (MIG001) Description() string {
	return "CREATE INDEX without CONCURRENTLY on an existing table; locks writers for the duration of the build"
}

func (r MIG001) Check(stmt *pg_query.RawStmt, ctx *lint.FileContext) []lint.Violation {
	idx := stmt.Stmt.GetIndexStmt()
	if idx == nil {
		return nil
	}
	if idx.Concurrent {
		return nil
	}
	target := idx.GetRelation().GetRelname()
	if target == "" {
		return nil
	}
	if tableCreatedEarlierInFile(ctx, target) {
		return nil
	}
	return []lint.Violation{{
		File:     ctx.Filename,
		Line:     lint.LineFromOffset(ctx.Source, int(stmt.StmtLocation)),
		Rule:     r.ID(),
		Severity: r.Severity(),
		Message: fmt.Sprintf(
			"CREATE INDEX on %q without CONCURRENTLY locks the table; "+
				"use CREATE INDEX CONCURRENTLY in its own migration (see runbook recipe 4)", target),
	}}
}

// tableCreatedEarlierInFile reports whether name was the target of a
// CREATE TABLE earlier in this file.
func tableCreatedEarlierInFile(ctx *lint.FileContext, name string) bool {
	for i := 0; i < ctx.Index; i++ {
		c := ctx.Statements[i].Stmt.GetCreateStmt()
		if c == nil {
			continue
		}
		if c.GetRelation().GetRelname() == name {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run the tests**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent/feat-migration-lint/go/cmd/migration-lint
go test ./lint/rules -run TestMIG001 -v
```

Expected: all four tests PASS.

- [ ] **Step 5: Commit**

```bash
git add go/cmd/migration-lint/
git commit -m "feat(migration-lint): add MIG001 (CREATE INDEX without CONCURRENTLY)"
```

---

## Task 6: MIG002 — `ADD COLUMN ... NOT NULL` with non-constant default

**Files:**
- Create: `go/cmd/migration-lint/lint/rules/mig002_nonnull_default.go`
- Create: `go/cmd/migration-lint/lint/rules/mig002_nonnull_default_test.go`

- [ ] **Step 1: Write failing tests**

```go
package rules

import "testing"

func TestMIG002_FlagsNotNullWithVolatileDefault(t *testing.T) {
	violations := runRule(t, &MIG002{},
		`ALTER TABLE users ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT now();`)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %+v", violations)
	}
	if violations[0].Rule != "MIG002" {
		t.Errorf("rule: %s", violations[0].Rule)
	}
}

func TestMIG002_AcceptsConstantDefault(t *testing.T) {
	violations := runRule(t, &MIG002{},
		`ALTER TABLE users ADD COLUMN active BOOLEAN NOT NULL DEFAULT true;`)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %+v", violations)
	}
}

func TestMIG002_AcceptsNullableColumn(t *testing.T) {
	violations := runRule(t, &MIG002{},
		`ALTER TABLE users ADD COLUMN updated_at TIMESTAMPTZ DEFAULT now();`)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations on nullable col, got %+v", violations)
	}
}

func TestMIG002_AcceptsNotNullNoDefault(t *testing.T) {
	violations := runRule(t, &MIG002{},
		`ALTER TABLE users ADD COLUMN tier TEXT NOT NULL;`)
	// PG will reject this at runtime if the table has rows, but the rule
	// is specifically scoped to the rewrite-on-volatile-default pattern.
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %+v", violations)
	}
}
```

- [ ] **Step 2: Implement MIG002**

```go
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
			Line:     lint.LineFromOffset(ctx.Source, int(stmt.StmtLocation)),
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
```

- [ ] **Step 3: Run tests**

```bash
go test ./lint/rules -run TestMIG002 -v
```

Expected: all four tests PASS.

- [ ] **Step 4: Commit**

```bash
git add go/cmd/migration-lint/
git commit -m "feat(migration-lint): add MIG002 (NOT NULL ADD COLUMN with volatile default)"
```

---

## Task 7: MIG003 — `ALTER COLUMN ... TYPE`

**Files:**
- Create: `go/cmd/migration-lint/lint/rules/mig003_alter_column_type.go`
- Create: `go/cmd/migration-lint/lint/rules/mig003_alter_column_type_test.go`

- [ ] **Step 1: Write failing tests**

```go
package rules

import "testing"

func TestMIG003_FlagsAlterColumnType(t *testing.T) {
	violations := runRule(t, &MIG003{},
		`ALTER TABLE orders ALTER COLUMN total TYPE BIGINT;`)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %+v", violations)
	}
}

func TestMIG003_IgnoresAddColumn(t *testing.T) {
	violations := runRule(t, &MIG003{},
		`ALTER TABLE orders ADD COLUMN total BIGINT;`)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %+v", violations)
	}
}
```

- [ ] **Step 2: Implement MIG003**

```go
package rules

import (
	"fmt"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/kabradshaw1/portfolio/go/cmd/migration-lint/lint"
)

// MIG003 flags ALTER COLUMN ... TYPE — generally rewrites every row unless the
// type change is binary-compatible. Use expand-and-contract instead.
type MIG003 struct{}

func (MIG003) ID() string              { return "MIG003" }
func (MIG003) Severity() lint.Severity { return lint.SeverityError }
func (MIG003) Description() string {
	return "ALTER COLUMN TYPE rewrites the table; expand-and-contract instead"
}

func (r MIG003) Check(stmt *pg_query.RawStmt, ctx *lint.FileContext) []lint.Violation {
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
		if c.Subtype != pg_query.AlterTableType_AT_AlterColumnType {
			continue
		}
		out = append(out, lint.Violation{
			File:     ctx.Filename,
			Line:     lint.LineFromOffset(ctx.Source, int(stmt.StmtLocation)),
			Rule:     r.ID(),
			Severity: r.Severity(),
			Message: fmt.Sprintf(
				"ALTER COLUMN %q TYPE will rewrite the table; "+
					"add a new column, backfill, switch reads/writes, drop old (recipe 7)",
				c.Name),
		})
	}
	return out
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./lint/rules -run TestMIG003 -v
```

- [ ] **Step 4: Commit**

```bash
git add go/cmd/migration-lint/
git commit -m "feat(migration-lint): add MIG003 (ALTER COLUMN TYPE)"
```

---

## Task 8: MIG004 — `ADD CONSTRAINT ... CHECK` without `NOT VALID`

**Files:**
- Create: `go/cmd/migration-lint/lint/rules/mig004_check_not_valid.go`
- Create: `go/cmd/migration-lint/lint/rules/mig004_check_not_valid_test.go`

- [ ] **Step 1: Write failing tests**

```go
package rules

import "testing"

func TestMIG004_FlagsCheckWithoutNotValid(t *testing.T) {
	violations := runRule(t, &MIG004{},
		`ALTER TABLE orders ADD CONSTRAINT total_positive CHECK (total > 0);`)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %+v", violations)
	}
}

func TestMIG004_AcceptsNotValid(t *testing.T) {
	violations := runRule(t, &MIG004{},
		`ALTER TABLE orders ADD CONSTRAINT total_positive CHECK (total > 0) NOT VALID;`)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %+v", violations)
	}
}

func TestMIG004_IgnoresNonCheckConstraints(t *testing.T) {
	violations := runRule(t, &MIG004{},
		`ALTER TABLE orders ADD CONSTRAINT orders_pkey PRIMARY KEY (id);`)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %+v", violations)
	}
}
```

- [ ] **Step 2: Implement MIG004**

```go
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
			// NOT VALID set.
			continue
		}
		out = append(out, lint.Violation{
			File:     ctx.Filename,
			Line:     lint.LineFromOffset(ctx.Source, int(stmt.StmtLocation)),
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
```

- [ ] **Step 3: Run tests**

```bash
go test ./lint/rules -run TestMIG004 -v
```

- [ ] **Step 4: Commit**

```bash
git add go/cmd/migration-lint/
git commit -m "feat(migration-lint): add MIG004 (CHECK constraint without NOT VALID)"
```

---

## Task 9: MIG005 — `DROP COLUMN`

**Files:**
- Create: `go/cmd/migration-lint/lint/rules/mig005_drop_column.go`
- Create: `go/cmd/migration-lint/lint/rules/mig005_drop_column_test.go`

- [ ] **Step 1: Write failing tests**

```go
package rules

import "testing"

func TestMIG005_FlagsDropColumn(t *testing.T) {
	violations := runRule(t, &MIG005{},
		`ALTER TABLE users DROP COLUMN deprecated_field;`)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %+v", violations)
	}
}

func TestMIG005_IgnoreDirectiveSuppresses(t *testing.T) {
	src := `-- migration-lint: ignore=MIG005 reason="confirmed unused via pg_stat_user_columns 2026-04-15"
ALTER TABLE users DROP COLUMN deprecated_field;`
	violations := runRule(t, &MIG005{}, src)
	if len(violations) != 0 {
		t.Fatalf("expected suppression, got %+v", violations)
	}
}
```

- [ ] **Step 2: Implement MIG005**

```go
package rules

import (
	"fmt"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/kabradshaw1/portfolio/go/cmd/migration-lint/lint"
)

// MIG005 flags every DROP COLUMN. The rule cannot determine whether the
// application has stopped writing this column, so dropping requires an
// explicit ignore directive that documents the verification.
type MIG005 struct{}

func (MIG005) ID() string              { return "MIG005" }
func (MIG005) Severity() lint.Severity { return lint.SeverityError }
func (MIG005) Description() string {
	return "DROP COLUMN must be confirmed with an ignore directive documenting that the app has stopped using it"
}

func (r MIG005) Check(stmt *pg_query.RawStmt, ctx *lint.FileContext) []lint.Violation {
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
		if c.Subtype != pg_query.AlterTableType_AT_DropColumn {
			continue
		}
		out = append(out, lint.Violation{
			File:     ctx.Filename,
			Line:     lint.LineFromOffset(ctx.Source, int(stmt.StmtLocation)),
			Rule:     r.ID(),
			Severity: r.Severity(),
			Message: fmt.Sprintf(
				"DROP COLUMN %q requires an ignore directive with a documented reason "+
					"(recipe 2 in the runbook)", c.Name),
		})
	}
	return out
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./lint/rules -run TestMIG005 -v
```

- [ ] **Step 4: Commit**

```bash
git add go/cmd/migration-lint/
git commit -m "feat(migration-lint): add MIG005 (DROP COLUMN requires confirmation)"
```

---

## Task 10: MIG006 — `RENAME COLUMN`

**Files:**
- Create: `go/cmd/migration-lint/lint/rules/mig006_rename_column.go`
- Create: `go/cmd/migration-lint/lint/rules/mig006_rename_column_test.go`

- [ ] **Step 1: Write failing tests**

```go
package rules

import "testing"

func TestMIG006_FlagsRenameColumn(t *testing.T) {
	violations := runRule(t, &MIG006{},
		`ALTER TABLE users RENAME COLUMN email TO email_address;`)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %+v", violations)
	}
}

func TestMIG006_IgnoresRenameTable(t *testing.T) {
	violations := runRule(t, &MIG006{},
		`ALTER TABLE users RENAME TO accounts;`)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations on RENAME TABLE, got %+v", violations)
	}
}

func TestMIG006_IgnoreDirectiveSuppresses(t *testing.T) {
	src := `-- migration-lint: ignore=MIG006 reason="cutover migration; old name no longer referenced"
ALTER TABLE users RENAME COLUMN email TO email_address;`
	violations := runRule(t, &MIG006{}, src)
	if len(violations) != 0 {
		t.Fatalf("expected suppression, got %+v", violations)
	}
}
```

- [ ] **Step 2: Implement MIG006**

```go
package rules

import (
	"fmt"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/kabradshaw1/portfolio/go/cmd/migration-lint/lint"
)

// MIG006 flags column renames. Like MIG005, this requires an explicit ignore
// directive — old code paths usually still reference the old name.
type MIG006 struct{}

func (MIG006) ID() string              { return "MIG006" }
func (MIG006) Severity() lint.Severity { return lint.SeverityError }
func (MIG006) Description() string {
	return "RENAME COLUMN must be confirmed with an ignore directive (callers usually still reference old name)"
}

func (r MIG006) Check(stmt *pg_query.RawStmt, ctx *lint.FileContext) []lint.Violation {
	rn := stmt.Stmt.GetRenameStmt()
	if rn == nil {
		return nil
	}
	if rn.RenameType != pg_query.ObjectType_OBJECT_COLUMN {
		return nil
	}
	return []lint.Violation{{
		File:     ctx.Filename,
		Line:     lint.LineFromOffset(ctx.Source, int(stmt.StmtLocation)),
		Rule:     r.ID(),
		Severity: r.Severity(),
		Message: fmt.Sprintf(
			"RENAME COLUMN %q -> %q requires an ignore directive (recipe 3 — expand-and-contract)",
			rn.Subname, rn.Newname),
	}}
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./lint/rules -run TestMIG006 -v
```

- [ ] **Step 4: Commit**

```bash
git add go/cmd/migration-lint/
git commit -m "feat(migration-lint): add MIG006 (RENAME COLUMN requires confirmation)"
```

---

## Task 11: MIG007 — `CREATE INDEX CONCURRENTLY` mixed with other statements

**Files:**
- Create: `go/cmd/migration-lint/lint/rules/mig007_concurrent_in_tx.go`
- Create: `go/cmd/migration-lint/lint/rules/mig007_concurrent_in_tx_test.go`

- [ ] **Step 1: Write failing tests**

```go
package rules

import "testing"

func TestMIG007_FlagsConcurrentlyMixedWithOtherStatement(t *testing.T) {
	src := `CREATE INDEX CONCURRENTLY idx_users_email ON users (email);
ALTER TABLE users ADD COLUMN tier TEXT;`
	violations := runRule(t, &MIG007{}, src)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %+v", violations)
	}
	if violations[0].Rule != "MIG007" {
		t.Errorf("rule: %s", violations[0].Rule)
	}
}

func TestMIG007_AllowsLoneConcurrently(t *testing.T) {
	src := `CREATE INDEX CONCURRENTLY idx_users_email ON users (email);`
	violations := runRule(t, &MIG007{}, src)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %+v", violations)
	}
}

func TestMIG007_IgnoresFileWithoutConcurrently(t *testing.T) {
	src := `ALTER TABLE users ADD COLUMN tier TEXT;
ALTER TABLE users ADD COLUMN active BOOLEAN DEFAULT true;`
	violations := runRule(t, &MIG007{}, src)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %+v", violations)
	}
}
```

- [ ] **Step 2: Implement MIG007 (file-level rule, fires once on the offending CONCURRENTLY stmt)**

```go
package rules

import (
	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/kabradshaw1/portfolio/go/cmd/migration-lint/lint"
)

// MIG007 fires when a `CREATE INDEX CONCURRENTLY` shares a file with any other
// statement. golang-migrate wraps each migration in a transaction, and
// CONCURRENTLY cannot run inside one.
type MIG007 struct{}

func (MIG007) ID() string              { return "MIG007" }
func (MIG007) Severity() lint.Severity { return lint.SeverityError }
func (MIG007) Description() string {
	return "CREATE INDEX CONCURRENTLY cannot run inside a transaction; isolate it in its own migration file"
}

func (r MIG007) Check(stmt *pg_query.RawStmt, ctx *lint.FileContext) []lint.Violation {
	idx := stmt.Stmt.GetIndexStmt()
	if idx == nil || !idx.Concurrent {
		return nil
	}
	if len(ctx.Statements) == 1 {
		return nil
	}
	return []lint.Violation{{
		File:     ctx.Filename,
		Line:     lint.LineFromOffset(ctx.Source, int(stmt.StmtLocation)),
		Rule:     r.ID(),
		Severity: r.Severity(),
		Message: "CREATE INDEX CONCURRENTLY shares this file with " +
			"other statements; golang-migrate runs each file in a transaction. " +
			"Move CONCURRENTLY into its own migration (recipe 4).",
	}}
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./lint/rules -run TestMIG007 -v
```

- [ ] **Step 4: Commit**

```bash
git add go/cmd/migration-lint/
git commit -m "feat(migration-lint): add MIG007 (CONCURRENTLY mixed with other statements)"
```

---

## Task 12: MIG008 — `LOCK TABLE`

**Files:**
- Create: `go/cmd/migration-lint/lint/rules/mig008_lock_table.go`
- Create: `go/cmd/migration-lint/lint/rules/mig008_lock_table_test.go`

- [ ] **Step 1: Write failing tests**

```go
package rules

import (
	"testing"

	"github.com/kabradshaw1/portfolio/go/cmd/migration-lint/lint"
)

func TestMIG008_FlagsLockTable(t *testing.T) {
	violations := runRule(t, &MIG008{},
		`LOCK TABLE orders IN ACCESS EXCLUSIVE MODE;`)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %+v", violations)
	}
	if violations[0].Severity != lint.SeverityWarning {
		t.Errorf("severity: %v", violations[0].Severity)
	}
}

func TestMIG008_IgnoreDirectiveSuppresses(t *testing.T) {
	src := `-- migration-lint: ignore=MIG008 reason="serializing checkout-saga compaction"
LOCK TABLE orders IN ACCESS EXCLUSIVE MODE;`
	violations := runRule(t, &MIG008{}, src)
	if len(violations) != 0 {
		t.Fatalf("expected suppression, got %+v", violations)
	}
}
```

- [ ] **Step 2: Implement MIG008**

```go
package rules

import (
	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/kabradshaw1/portfolio/go/cmd/migration-lint/lint"
)

// MIG008 flags LOCK TABLE. Severity Warning — there are legitimate uses, but
// each one should ship with a documented rationale (via ignore directive).
type MIG008 struct{}

func (MIG008) ID() string              { return "MIG008" }
func (MIG008) Severity() lint.Severity { return lint.SeverityWarning }
func (MIG008) Description() string {
	return "LOCK TABLE must document its rationale via an ignore directive"
}

func (r MIG008) Check(stmt *pg_query.RawStmt, ctx *lint.FileContext) []lint.Violation {
	if stmt.Stmt.GetLockStmt() == nil {
		return nil
	}
	return []lint.Violation{{
		File:     ctx.Filename,
		Line:     lint.LineFromOffset(ctx.Source, int(stmt.StmtLocation)),
		Rule:     r.ID(),
		Severity: r.Severity(),
		Message:  "LOCK TABLE without a documented purpose; add an ignore directive with reason= explaining why",
	}}
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./lint/rules -run TestMIG008 -v
```

- [ ] **Step 4: Commit**

```bash
git add go/cmd/migration-lint/
git commit -m "feat(migration-lint): add MIG008 (LOCK TABLE warning)"
```

---

## Task 13: Wire CLI to rules and produce human-readable output

**Files:**
- Modify: `go/cmd/migration-lint/main.go`
- Modify: `go/cmd/migration-lint/main_test.go`

- [ ] **Step 1: Replace the CLI smoke test with end-to-end tests**

Edit `go/cmd/migration-lint/main_test.go`:

```go
package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI_NoArgs(t *testing.T) {
	cmd := exec.Command(buildBinary(t))
	if err := cmd.Run(); err == nil {
		t.Fatal("expected non-zero exit")
	} else if exitCode(err) != 2 {
		t.Fatalf("exit code: got %d want 2", exitCode(err))
	}
}

func TestCLI_CleanFile_ExitsZero(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	clean := filepath.Join(dir, "001_clean.up.sql")
	if err := os.WriteFile(clean, []byte("CREATE TABLE x (id int);\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, clean)
	if err := cmd.Run(); err != nil {
		t.Fatalf("expected clean run, got %v", err)
	}
}

func TestCLI_ReportsViolations_ExitOne(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	bad := filepath.Join(dir, "002_bad.up.sql")
	src := []byte("CREATE INDEX idx_x ON existing_table (a);\n")
	if err := os.WriteFile(bad, src, 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd := exec.Command(bin, bad)
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit")
	}
	if exitCode(err) != 1 {
		t.Fatalf("exit code: got %d want 1; stderr=%s", exitCode(err), stderr.String())
	}
	if !strings.Contains(stderr.String(), "MIG001") {
		t.Errorf("expected MIG001 in stderr, got %q", stderr.String())
	}
}

func TestCLI_ParseError_ExitTwo(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	broken := filepath.Join(dir, "003_broken.up.sql")
	if err := os.WriteFile(broken, []byte("CRATE TABBLE bad;"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, broken)
	err := cmd.Run()
	if exitCode(err) != 2 {
		t.Fatalf("exit code: got %d want 2", exitCode(err))
	}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if e, ok := err.(*exec.ExitError); ok {
		return e.ExitCode()
	}
	return -1
}

func buildBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "migration-lint")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build failed: %v", err)
	}
	return bin
}
```

- [ ] **Step 2: Implement the real `main.go`**

```go
// migration-lint statically analyzes golang-migrate .up.sql files for
// operationally unsafe DDL patterns. See docs/runbooks/postgres-migrations.md.
package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/kabradshaw1/portfolio/go/cmd/migration-lint/lint"
	"github.com/kabradshaw1/portfolio/go/cmd/migration-lint/lint/rules"
)

const (
	exitClean      = 0
	exitViolation  = 1
	exitInvocation = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

func run(args []string, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: migration-lint <file.sql> [file.sql ...]")
		return exitInvocation
	}

	violations, err := lint.Lint(args, allRules())
	if err != nil {
		fmt.Fprintf(stderr, "migration-lint: %v\n", err)
		return exitInvocation
	}

	sort.SliceStable(violations, func(i, j int) bool {
		if violations[i].File != violations[j].File {
			return violations[i].File < violations[j].File
		}
		return violations[i].Line < violations[j].Line
	})

	hasError := false
	for _, v := range violations {
		fmt.Fprintf(stderr, "%s:%d: [%s %s] %s\n",
			v.File, v.Line, v.Rule, v.Severity, v.Message)
		if v.Severity == lint.SeverityError {
			hasError = true
		}
	}
	if hasError {
		return exitViolation
	}
	return exitClean
}

// allRules returns every rule registered with the linter.
func allRules() []lint.Rule {
	return []lint.Rule{
		&rules.MIG001{},
		&rules.MIG002{},
		&rules.MIG003{},
		&rules.MIG004{},
		&rules.MIG005{},
		&rules.MIG006{},
		&rules.MIG007{},
		&rules.MIG008{},
	}
}
```

- [ ] **Step 3: Run all tests at the module root**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent/feat-migration-lint/go/cmd/migration-lint
go test ./... -v -race
```

Expected: every test PASS.

- [ ] **Step 4: Commit**

```bash
git add go/cmd/migration-lint/
git commit -m "feat(migration-lint): wire CLI to rule registry with sorted output and exit codes"
```

---

## Task 14: Add a snapshot test that runs the linter over the existing migration tree

**Files:**
- Modify: `go/cmd/migration-lint/lint/lint_test.go`

The snapshot test pins the *current* clean state of the repo's migrations. After Task 15 adds the necessary ignore directives, running this test re-asserts a zero-violation baseline going forward.

- [ ] **Step 1: Add the snapshot test**

Append to `go/cmd/migration-lint/lint/lint_test.go`:

```go
import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kabradshaw1/portfolio/go/cmd/migration-lint/lint/rules"
)

func TestSnapshot_RepoMigrations(t *testing.T) {
	// Resolve the worktree root: go/cmd/migration-lint/lint -> ../../../..
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := filepath.Clean(filepath.Join(wd, "..", "..", "..", ".."))
	pattern := filepath.Join(repoRoot, "go", "*", "migrations", "*.up.sql")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Skipf("no migration files found at %s; running outside repo?", pattern)
	}
	allRules := []Rule{
		&rules.MIG001{},
		&rules.MIG002{},
		&rules.MIG003{},
		&rules.MIG004{},
		&rules.MIG005{},
		&rules.MIG006{},
		&rules.MIG007{},
		&rules.MIG008{},
	}
	violations, err := Lint(matches, allRules)
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
```

- [ ] **Step 2: Run the snapshot test — expect failures**

```bash
go test ./lint -run TestSnapshot_RepoMigrations -v 2>&1 | head -80
```

Capture the output. Each reported violation needs to be triaged in Task 15.

- [ ] **Step 3: Do NOT commit yet — Task 15 fixes the failures.**

---

## Task 15: Triage existing-migration violations and add ignore directives

**Files:**
- Modify: existing `go/<service>/migrations/*.up.sql` files reported by Task 14's snapshot test

The portfolio's early migrations were authored against empty databases and ran in seconds; they're "unsafe" by the linter's letter but harmless in context. The fix is per-statement ignore directives that document *why*.

**Decision matrix for each reported violation:**

| Pattern | Likely fix |
|---|---|
| Bare `CREATE INDEX` in an early-stage migration on what is now a populated table | `-- migration-lint: ignore=MIG001 reason="early-stage migration; ran against empty dev DBs prior to GA"` |
| `ADD COLUMN ... NOT NULL DEFAULT 'CREATED'` (constant string) | Should not fire — if it does, examine the rule. |
| `ALTER COLUMN ... TYPE TEXT` on early dev migration | `-- migration-lint: ignore=MIG003 reason="dev-only schema fix before any prod data existed"` |
| `LOCK TABLE` in any existing migration | `-- migration-lint: ignore=MIG008 reason="<actual reason>"` |
| `DROP COLUMN`/`RENAME COLUMN` in any existing migration | `-- migration-lint: ignore=MIG005 reason="<verification>"` or `MIG006` accordingly. If unclear, ASK Kyle. |

**Rule of thumb:** if the migration ran successfully and is in production today, an ignore directive with the early-stage reason is acceptable. If the *next* migration of that shape would be unsafe, the rule will fire on it correctly.

- [ ] **Step 1: For each violation reported by Task 14, edit the offending migration to add an ignore directive immediately above the flagged statement**

Concrete edit pattern:

Before:
```sql
CREATE INDEX idx_cart_items_user_reserved ON cart_items (user_id, reserved);
```

After:
```sql
-- migration-lint: ignore=MIG001 reason="early-stage migration; ran on empty cart_items table before launch"
CREATE INDEX idx_cart_items_user_reserved ON cart_items (user_id, reserved);
```

- [ ] **Step 2: Re-run the snapshot test**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent/feat-migration-lint/go/cmd/migration-lint
go test ./lint -run TestSnapshot_RepoMigrations -v
```

If new violations remain, repeat Step 1.

- [ ] **Step 3: If a violation is in `008_partition_orders.up.sql` (the partitioning migration), use a more substantive reason that references the prior ADR**

Example:
```sql
-- migration-lint: ignore=MIG001 reason="partitioning migration — see docs/adr/ecommerce/go-sql-optimization-reporting.md; tables empty in this rebuild step"
```

- [ ] **Step 4: Commit the snapshot pass and the ignore directives together**

```bash
git add go/cmd/migration-lint/lint/lint_test.go go/*/migrations/*.up.sql
git commit -m "test(migration-lint): snapshot existing migrations green via ignore directives"
```

---

## Task 16: Add the worked-example migration in `product-service`

**Files:**
- Create: `go/product-service/migrations/004_add_product_search_index.up.sql`
- Create: `go/product-service/migrations/004_add_product_search_index.down.sql`

The example demonstrates the safe pattern: a `CREATE INDEX CONCURRENTLY` in its own migration file, with a comment that points readers at the runbook recipe.

- [ ] **Step 1: Create the up migration**

```sql
-- 004_add_product_search_index.up.sql
--
-- Adds a trigram GIN index on products.name to accelerate the LIKE/ILIKE
-- search path used by GET /products?search=...  See repository.go SearchByName.
--
-- Pattern: CREATE INDEX CONCURRENTLY in its own migration file.
-- See docs/runbooks/postgres-migrations.md (recipe 4) for why this pattern
-- is required.

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_products_name_trgm
  ON products USING gin (name gin_trgm_ops);
```

- [ ] **Step 2: Create the down migration**

```sql
-- 004_add_product_search_index.down.sql
DROP INDEX IF EXISTS idx_products_name_trgm;
-- pg_trgm extension is left in place; other indexes may use it.
```

- [ ] **Step 3: Verify the linter accepts the new migration**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent/feat-migration-lint
cd go/cmd/migration-lint && go build -o /tmp/migration-lint . && cd ../../..
/tmp/migration-lint go/product-service/migrations/004_add_product_search_index.up.sql
echo "exit=$?"
```

Expected: no output, `exit=0`.

> **Pitfall — `CREATE EXTENSION` in the same file as `CREATE INDEX CONCURRENTLY`.** MIG007 forbids this combination because golang-migrate wraps each migration in a transaction. The linter will trip if this stays. **Fix:** split into two files (`004_enable_pg_trgm.up.sql` to `CREATE EXTENSION pg_trgm`, `005_add_product_search_index.up.sql` for the index). Apply that split before continuing — re-number the rest of this task and Task 19's down migration accordingly.

- [ ] **Step 4: Verify the migration actually runs against Postgres**

```bash
make preflight-go-migrations
```

Expected: completes successfully, the `idx_products_name_trgm` index appears in `productdb`.

> If `make preflight-go-migrations` skips due to no Docker, run `colima start` first.

- [ ] **Step 5: Commit**

```bash
git add go/product-service/migrations/004_*.sql
# (or 004 + 005 if the EXTENSION had to be split out — also git add the renamed files)
git commit -m "feat(product-service): add trigram search index via safe CONCURRENTLY pattern"
```

---

## Task 17: Wire the linter into `make preflight-go-migrations`

**Files:**
- Modify: `Makefile` (lines around the `preflight-go-migrations` target, around L73)

- [ ] **Step 1: Add a new prerequisite target `preflight-go-migration-lint` and have `preflight-go-migrations` depend on it**

Edit `Makefile`. Locate the `.PHONY` line (L1) and the `preflight-go-migrations:` target (L73).

In `.PHONY`, add `preflight-go-migration-lint`:

```makefile
.PHONY: preflight preflight-python preflight-frontend preflight-e2e preflight-java preflight-java-integration preflight-go preflight-go-integration preflight-go-migrations preflight-go-migration-lint preflight-security preflight-compose-config preflight-ai-service preflight-ai-service-evals grafana-sync grafana-sync-check worktree-cleanup install-pre-commit
```

Add the new target before `preflight-go-migrations`:

```makefile
# --- Go migration static lint (no Docker required) ---
# Builds the migration-lint binary fresh and runs it over every service's
# .up.sql files. Catches operationally unsafe DDL patterns before the runtime
# migration test even starts.
preflight-go-migration-lint:
	@echo "\n=== Go: migration static lint ==="
	@cd go/cmd/migration-lint && go build -o /tmp/migration-lint .
	@/tmp/migration-lint go/*/migrations/*.up.sql
	@echo "  ✅ migration-lint clean"
```

Then update `preflight-go-migrations` to depend on it:

```makefile
preflight-go-migrations: preflight-go-migration-lint
	@echo "\n=== Go: migration pipeline test ==="
	@if ! docker info >/dev/null 2>&1; then \
	... (existing body unchanged)
```

- [ ] **Step 2: Run the new target standalone**

```bash
make preflight-go-migration-lint
```

Expected: `migration-lint clean` printed; exit 0.

- [ ] **Step 3: Run the full migration pipeline (requires Docker)**

```bash
make preflight-go-migrations
```

Expected: lint runs, then Postgres pipeline runs, all green.

> If Docker isn't available, the lint step still runs (it's a hard prereq) and the pipeline portion skips with a warning — that's the existing behavior.

- [ ] **Step 4: Commit**

```bash
git add Makefile
git commit -m "build(make): wire migration-lint into preflight-go-migrations"
```

---

## Task 18: Add the new module to CI matrices

**Files:**
- Modify: `.github/workflows/ci.yml` (around L213 and L255)

The `go-lint` and `go-tests` jobs use a per-service matrix. The migration-lint module joins as another matrix entry.

- [ ] **Step 1: Add a matrix entry to `go-lint`**

Edit the `include:` list under `go-lint:` (around L218). Append:

```yaml
          - service: cmd/migration-lint
            paths: go/cmd/migration-lint
```

Note: the `working-directory` template uses `go/${{ matrix.service }}` — that resolves to `go/cmd/migration-lint`, which is correct.

- [ ] **Step 2: Add the same entry to `go-tests` (around L260)**

```yaml
          - service: cmd/migration-lint
            paths: go/cmd/migration-lint
```

- [ ] **Step 3: Validate the YAML locally**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"
```

Expected: no output (parses cleanly).

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci(go): lint and test migration-lint module"
```

---

## Task 19: Write the safe-migration playbook

**Files:**
- Create: `docs/runbooks/postgres-migrations.md`

Each recipe follows the same six-section structure: what, unsafe version, why, safe version, linter rule, cross-references.

- [ ] **Step 1: Create the runbook**

```markdown
# Safe PostgreSQL Migrations — Recipe Book

A working catalog of safe patterns for `golang-migrate` migrations against
production Postgres. Each recipe is paired with a rule in `migration-lint`
(see `go/cmd/migration-lint/`). The linter is the gate; this document is the
fix.

> **Convention:** All recipes assume `golang-migrate` numbering. Each step in
> a multi-step recipe ships as its *own* `NNN_*.up.sql` file deployed in a
> separate release.

---

## Recipe 1 — Add a NOT NULL column to a busy table

**Goal:** Add `tier TEXT NOT NULL DEFAULT 'standard'` to `users`.

### Unsafe one-shot

```sql
ALTER TABLE users ADD COLUMN tier TEXT NOT NULL DEFAULT now()::text;
```

**Why unsafe:** A volatile default forces PG to rewrite every row. On a 100M-row table that means an `ACCESS EXCLUSIVE` lock for the duration of the rewrite — minutes, sometimes longer. (PG 11+ fast-paths *constant* defaults; volatile expressions still rewrite.)

### Safe multi-step

```sql
-- 042_add_users_tier_column.up.sql
ALTER TABLE users ADD COLUMN tier TEXT;
```

```sql
-- 043_backfill_users_tier.up.sql
-- Run as a chunked job from the application instead of inline if the table is large.
UPDATE users SET tier = 'standard' WHERE tier IS NULL;
```

```sql
-- 044_users_tier_set_default.up.sql
ALTER TABLE users ALTER COLUMN tier SET DEFAULT 'standard';
```

```sql
-- 045_users_tier_set_not_null.up.sql
ALTER TABLE users ALTER COLUMN tier SET NOT NULL;
```

### Linter rule

`MIG002` — `ADD COLUMN ... NOT NULL` with a non-constant default.

### Cross-references

- `docs/adr/database/migration-lint.md`
- Recipe 5 (CHECK constraint) uses a similar two-phase shape

---

## Recipe 2 — Drop a column

**Goal:** Remove `users.deprecated_field`.

### Unsafe one-shot

```sql
ALTER TABLE users DROP COLUMN deprecated_field;
```

**Why unsafe:** The DDL itself is fast — PG just marks the column attisdropped. The danger is the *application*: any running code that still references the column will start erroring. Linters can't see your application graph; only you can.

### Safe procedure

1. **Stop writing the column** (deploy code that no longer SETs it).
2. **Stop reading the column** (deploy code that no longer SELECTs it).
3. **Verify in production** — `SELECT pg_stat_all_tables.last_seq_scan, n_live_tup FROM pg_stat_all_tables WHERE relname = 'users';` plus app-side metrics that confirm callers stopped touching the column.
4. **Drop** with an ignore directive that records the verification.

```sql
-- migration-lint: ignore=MIG005 reason="last write 2026-04-01; pg_stat confirms zero reads since 2026-04-15; deploy 2026-04-22 removed final reference"
ALTER TABLE users DROP COLUMN deprecated_field;
```

### Linter rule

`MIG005` — DROP COLUMN must include a documented ignore directive.

---

## Recipe 3 — Rename a column (expand-and-contract)

**Goal:** Rename `users.email` to `users.email_address`.

### Unsafe one-shot

```sql
ALTER TABLE users RENAME COLUMN email TO email_address;
```

**Why unsafe:** Rolling deploys mean old replicas of your application are still issuing queries against `users.email` while new replicas are queryng `users.email_address`. Either side errors.

### Safe multi-step

1. **Add new column** (nullable):

```sql
-- 060_add_email_address_column.up.sql
ALTER TABLE users ADD COLUMN email_address TEXT;
```

2. **Backfill + dual-write trigger** during the transition window:

```sql
-- 061_dual_write_email.up.sql
CREATE OR REPLACE FUNCTION users_email_dual_write() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.email_address IS NULL THEN NEW.email_address := NEW.email; END IF;
  IF NEW.email IS NULL THEN NEW.email := NEW.email_address; END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER users_email_dual_write_trg BEFORE INSERT OR UPDATE ON users
  FOR EACH ROW EXECUTE FUNCTION users_email_dual_write();

UPDATE users SET email_address = email WHERE email_address IS NULL;
```

3. **Switch reads** in the application (deploy).
4. **Switch writes** in the application (deploy). Drop the trigger.

```sql
-- 062_drop_dual_write.up.sql
DROP TRIGGER users_email_dual_write_trg ON users;
DROP FUNCTION users_email_dual_write();
```

5. **Drop the old column** with an ignore directive (see recipe 2).

### Linter rule

`MIG006` — RENAME COLUMN requires an ignore directive (don't actually rename — use expand-and-contract).

---

## Recipe 4 — Add an index without locking writers

**Goal:** Index `orders.user_id` on a 50M-row table.

### Unsafe one-shot

```sql
CREATE INDEX idx_orders_user_id ON orders (user_id);
```

**Why unsafe:** Plain `CREATE INDEX` takes `ACCESS EXCLUSIVE` for the whole build. Writers are blocked.

### Safe pattern (one statement per migration file)

```sql
-- 070_orders_user_id_idx.up.sql
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_orders_user_id ON orders (user_id);
```

**Two non-negotiables:**

1. The migration file contains exactly **one** statement. `CREATE INDEX CONCURRENTLY` cannot run inside a transaction, and `golang-migrate` wraps each migration file in one.
2. Use `IF NOT EXISTS` so a half-built index from a prior failed run can be retried. (Drop and rebuild it manually if it's invalid — `\d+` shows `INVALID` next to the index name.)

### Linter rules

- `MIG001` — bare `CREATE INDEX` against a table not created in the same file.
- `MIG007` — `CONCURRENTLY` mixed with any other statement.

### Cross-references

- The worked example: `go/product-service/migrations/004_add_product_search_index.up.sql`

---

## Recipe 5 — Add a CHECK constraint

**Goal:** Enforce `orders.total > 0`.

### Unsafe one-shot

```sql
ALTER TABLE orders ADD CONSTRAINT total_positive CHECK (total > 0);
```

**Why unsafe:** PG validates the constraint by full-table scan under `ACCESS EXCLUSIVE`.

### Safe two-phase

```sql
-- 080_check_total_positive_not_valid.up.sql
ALTER TABLE orders ADD CONSTRAINT total_positive CHECK (total > 0) NOT VALID;
```

```sql
-- 081_validate_total_positive.up.sql
ALTER TABLE orders VALIDATE CONSTRAINT total_positive;
```

`NOT VALID` skips the scan; `VALIDATE` does it later under a `SHARE UPDATE EXCLUSIVE` lock that doesn't block writes.

### Linter rule

`MIG004` — CHECK constraint without `NOT VALID`.

---

## Recipe 6 — Rename a table

**Goal:** Rename `users` to `accounts`.

### Unsafe one-shot

```sql
ALTER TABLE users RENAME TO accounts;
```

**Why unsafe:** Same rolling-deploy problem as column rename.

### Safe procedure (compatibility view)

```sql
-- 090_rename_users_to_accounts.up.sql
ALTER TABLE users RENAME TO accounts;
CREATE VIEW users AS SELECT * FROM accounts;
```

The view bridges the deploy window. After the application has been fully cut over to `accounts`, drop the view in a follow-up migration.

### Linter rule

None today — table renames don't have a dedicated rule. Add one if it becomes a recurring pattern.

---

## Recipe 7 — Change a column type

**Goal:** Change `orders.total` from `INTEGER` to `BIGINT`.

### Unsafe one-shot

```sql
ALTER TABLE orders ALTER COLUMN total TYPE BIGINT;
```

**Why unsafe:** Type changes generally rewrite every row. (`varchar(N)` → `varchar(M)` where M > N is the famous binary-compatible exception, but plan for the rewrite by default.)

### Safe expand-and-contract

1. Add `total_v2 BIGINT` (nullable).
2. Dual-write the new column from the application.
3. Backfill in chunks.
4. Switch reads to `total_v2`.
5. Drop `total` (recipe 2).
6. Optionally rename `total_v2` → `total` (recipe 3).

### Linter rule

`MIG003` — ALTER COLUMN TYPE.

---

## Recipe 8 — Partition an existing table

**Goal:** Partition `orders` by month on `created_at`.

This is genuinely complex enough that it gets its own ADR rather than a fits-in-a-section recipe. See `docs/adr/ecommerce/go-sql-optimization-reporting.md` for the full migration sequence and the FK trade-offs that come with composite primary keys.

### Linter rule

The migration is in production and has its violations annotated with ignore directives. The runbook entry is here so future readers know where to look.
```

- [ ] **Step 2: Commit**

```bash
git add docs/runbooks/postgres-migrations.md
git commit -m "docs: add safe Postgres migration playbook"
```

---

## Task 20: Write the companion ADR

**Files:**
- Create: `docs/adr/database/migration-lint.md` (creates the new `database/` subdir)

- [ ] **Step 1: Create the ADR**

```markdown
# ADR: PostgreSQL Migration Linter

- **Date:** 2026-04-27
- **Status:** Accepted
- **Builds on:**
  - `docs/adr/ecommerce/go-database-optimization.md` (schema hardening pass)
  - `docs/adr/ecommerce/go-sql-optimization-reporting.md` (the partitioning migration that surfaced the gap)
- **Roadmap position:** Item 2 of 10 in the `db-roadmap` GitHub label

## Context

Every Go service ships migrations as `golang-migrate` `NNN_name.up.sql` /
`.down.sql` pairs. `make preflight-go-migrations` already spins up Postgres in
Docker and runs every migration end-to-end, catching syntactic failures.

What that test misses is *operationally unsafe but syntactically valid*
migrations: ones that compile and run on an empty database but would lock a
busy production table for minutes, rewrite millions of rows during DDL, or
leave indexes in `INVALID` state. These are also the most common backend
interview topic and the most common source of preventable production
incidents. We need to push that check earlier in the pipeline.

The trigger to act was the partitioning migration in 2026-04-22 — three CI
failures from index-name collisions and FK constraints, all caught only
because the migration ran against real Postgres. The right next step is a
static linter that catches the unsafe pattern at lint time, before any
container starts.

## Decision

Build a custom Go linter (`go/cmd/migration-lint/`) that:

1. Parses every `.up.sql` via `pganalyze/pg_query_go` (CGO wrapper around
   `libpg_query`, the upstream PG parser).
2. Walks the parsed statement tree with a curated set of eight rules
   (MIG001-MIG008) that target the most common unsafe patterns.
3. Supports per-statement opt-out via `-- migration-lint: ignore=MIGNNN reason="..."`
   comments. The `reason="..."` field is mandatory — every exception is a
   checked-in artifact of *why* it's safe.
4. Wires into `make preflight-go-migrations` as a hard prerequisite, so an
   unsafe migration commit fails preflight before Docker even starts.

The companion runbook (`docs/runbooks/postgres-migrations.md`) catalogs eight
recipes pairing each rule with the safe pattern that resolves it.

### Why custom Go over `squawk` / `atlas` / regex

- `squawk` is a Rust project with a much broader ruleset (~30 rules) than
  this portfolio needs and an extra runtime dep at preflight time. The
  curated, in-repo Go ruleset doubles as a portfolio artifact: hiring
  managers can read the AST walks and see how the engineer reasons about
  unsafe DDL.
- `atlas` is full-fledged schema-management tooling — overkill for a
  single-developer portfolio with one schema per service.
- Regex linters fall over on real-world migrations: dollar-quoted strings,
  multi-line statements, comment placement, and complex expressions all
  break naive matchers. `libpg_query` is *the* Postgres parser extracted
  from the server; using it sidesteps an entire class of false
  positives/negatives.

The CGO build dep that comes with `pg_query_go` is acceptable — the linter
is a developer-tool binary, not a runtime dependency. CI runners (Linux
gcc) and the Mac dev machine (Apple clang) both build it without
configuration.

### Why these eight rules

The selection criteria were:

1. The unsafe form locks or rewrites in a way that scales with table size.
2. The safe form is well-known to PG community: it's in any "online schema
   change" talk.
3. There's an unambiguous AST signature — no false-positive sprawl.

The eight rules cover the patterns that come up in interviews and incident
post-mortems. Phase 2 candidates (backfill detection, transaction-mixing for
non-CONCURRENTLY rules, large-INSERT detection) are easy to add — the
infrastructure is rule-pluggable.

### Why mandatory `reason="..."`

Linters that allow nameless ignores rot. Every exception in this codebase
ships with the *why* so future maintainers (and reviewers) can audit
whether the exception still applies. This mirrors the senior-engineer
convention of recording rationale in code, not in slack.

### "No historical rewrites" decision

`golang-migrate` tracks migrations by version number, not by content. A
migration that has been applied to production *cannot* be edited without
creating a divergence between the dev and prod schema-history table. The
worked-example migration is therefore a *new* migration; existing
migrations get ignore directives where their patterns would trip the
linter today.

## Consequences

**Positive:**

- Unsafe migrations fail at lint time, before Docker spin-up or CI runtime.
- The rule set is a checked-in answer to "which patterns are unsafe and
  why" — strong interview talking point.
- The runbook covers eight scenarios any backend engineer will hit.
- The worked-example migration leaves a real-world reference in the
  codebase.

**Trade-offs:**

- CGO build dep via `pg_query_go`. Acceptable — developer tool only.
- Curated rule set is narrower than `squawk`'s ~30 rules. Acceptable —
  depth over breadth, and additional rules can be added incrementally.
- Heuristics (especially MIG001's "existing table" detection) will produce
  occasional false positives. The ignore-directive pattern handles these
  without weakening the rule.
- Java migrations are not covered. Acceptable — Spring/JPA-managed
  schemas don't have DDL files to lint.

**Phase 2 candidates:**

- `MIG009` — backfill detection (UPDATEs that touch unbounded row counts).
- Pre-commit hook that runs the linter on staged `.sql` files for faster
  feedback than `make preflight-go-migrations`.
- IDE integration via a custom diagnostic LSP, if the linter sees enough
  use to justify it.

## File map

```
go/cmd/migration-lint/                    # the linter binary
├── main.go                               # CLI: argv → Lint() → format → exit
├── README.md
└── lint/
    ├── lint.go                           # Lint() entry, line-from-offset
    ├── parser.go                         # pg_query_go wrapper
    ├── ignore.go                         # `-- migration-lint: ignore=` parser
    ├── rule.go                           # Rule interface, types
    └── rules/                            # MIG001-MIG008

docs/runbooks/postgres-migrations.md      # 8-recipe playbook
docs/adr/database/migration-lint.md       # this ADR
go/product-service/migrations/004_*.sql   # worked-example: CONCURRENTLY in own file
```
```

- [ ] **Step 2: Commit**

```bash
git add docs/adr/database/migration-lint.md
git commit -m "docs(adr): document migration-lint design and rule selection"
```

---

## Task 21: Final preflight, push, and PR

**Files:** none (verification + push)

- [ ] **Step 1: Run the full preflight gates that touch Go**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent/feat-migration-lint
make preflight-go
make preflight-go-migration-lint
```

If `make preflight-go` complains that `cmd/migration-lint` isn't in its hard-coded loop, that's expected — the existing target only iterates the deployable services. Skip the lint/test loop concern; CI's matrix entry from Task 18 covers it.

If you have Docker:

```bash
make preflight-go-migrations
```

- [ ] **Step 2: Verify the linter end-to-end against every migration**

```bash
cd go/cmd/migration-lint && go build -o /tmp/migration-lint . && cd ../../..
/tmp/migration-lint go/*/migrations/*.up.sql
echo "exit=$?"
```

Expected: no output, `exit=0`.

- [ ] **Step 3: Confirm the branch state**

```bash
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent/feat-migration-lint status
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent/feat-migration-lint log --oneline main..HEAD
```

You should see ~10 focused commits and a clean working tree.

- [ ] **Step 4: Push the branch**

```bash
git -C /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent/feat-migration-lint push -u origin agent/feat-migration-lint
```

- [ ] **Step 5: Open the PR to `qa`**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent/feat-migration-lint
gh pr create --base qa --title "feat: PostgreSQL migration linter (db-roadmap #156)" --body "$(cat <<'EOF'
## Summary

- New Go CLI at `go/cmd/migration-lint/` that statically analyzes every `golang-migrate` `.up.sql` for operationally unsafe DDL patterns
- Eight curated rules (MIG001–MIG008) covering CONCURRENTLY-less indexes, NOT-NULL-with-volatile-default, table-rewrite ALTERs, NOT-VALID-less CHECKs, DROP/RENAME COLUMN, CONCURRENTLY-mixed-with-other-DDL, and LOCK TABLE
- Per-statement ignore directive: `-- migration-lint: ignore=MIGNNN reason="..."` (reason mandatory)
- Wired into `make preflight-go-migrations` so unsafe patterns fail before Postgres even starts
- Worked-example migration `004_add_product_search_index` shows the safe `CONCURRENTLY in its own file` pattern
- New runbook (`docs/runbooks/postgres-migrations.md`) — eight recipes pairing each rule with the safe alternative
- New ADR (`docs/adr/database/migration-lint.md`) covers rule selection and the custom-Go-vs-squawk decision

## Closes / refs

- Resolves item 2 of `db-roadmap` (#156)
- Builds on `docs/adr/ecommerce/go-database-optimization.md` and `go-sql-optimization-reporting.md`

## Test plan

- [ ] CI: `go-lint` and `go-tests` jobs run for the new `cmd/migration-lint` matrix entry
- [ ] CI: `Go Migration Pipeline Test` runs the linter (via Makefile prereq) before the Postgres pipeline
- [ ] Local: `make preflight-go-migration-lint` exits clean
- [ ] Local: `make preflight-go-migrations` (with Docker) builds, lints, runs migrations end-to-end
- [ ] Spot-check the new `004_add_product_search_index` migration creates `idx_products_name_trgm` in `productdb`

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 6: Notify Kyle and stop**

Per CLAUDE.md feature-branch flow: do NOT watch CI. Kyle will check results.

---

## Self-review checklist (executed by the plan author)

**Spec coverage map:**
- ✅ Eight rules MIG001–MIG008 → Tasks 5–12
- ✅ pg_query_go (CGO) parser → Task 2
- ✅ Exit codes 0/1/2 → Task 13 + Task 1 stub
- ✅ Single-line ignore directives with `reason="..."` → Task 4
- ✅ Snapshot test + adding directives to existing migrations → Tasks 14–15
- ✅ Worked-example new migration in product-service → Task 16
- ✅ Wire into `make preflight-go-migrations` → Task 17
- ✅ Companion ADR at `docs/adr/database/migration-lint.md` (new dir) → Task 20
- ✅ Runbook with 8 recipes → Task 19
- ✅ CI matrix wiring → Task 18

**Placeholder scan:** none found — every step has the actual SQL/Go code or the actual command. Task 15 documents per-violation triage but cannot enumerate exact files until the snapshot runs (the spec acknowledges this in step 2 of "Rollout").

**Type/name consistency check:** `Rule` interface, `Violation`/`Severity`/`FileContext`/`IgnoreDirective` types are defined in Task 2 (`rule.go`) and used unchanged through Tasks 3–13. The `parseIgnoreDirectives` stub in Task 3 is replaced (signature unchanged) in Task 4. `MIG00X` types are exported structs with consistent `ID()`/`Severity()`/`Description()`/`Check()` method signatures across Tasks 5–12.
