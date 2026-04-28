// migration-lint statically analyzes golang-migrate .up.sql files for
// operationally unsafe DDL patterns. See docs/runbooks/postgres-migrations.md.
package main

import (
	"fmt"
	"io"
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

func run(args []string, stderr io.Writer) int {
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
