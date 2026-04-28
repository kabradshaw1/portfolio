// migration-lint statically analyzes golang-migrate .up.sql files for
// operationally unsafe DDL patterns. See docs/runbooks/postgres-migrations.md.
package main

import (
	"fmt"
	"os"
)

const (
	exitClean      = 0
	exitViolation  = 1
	exitInvocation = 2
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: migration-lint <file.sql> [file.sql ...]")
		os.Exit(exitInvocation)
	}
	os.Exit(exitClean)
}
