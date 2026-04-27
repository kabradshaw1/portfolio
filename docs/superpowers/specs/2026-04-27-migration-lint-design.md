# Design: PostgreSQL Migration Linter & Safe-Migration Playbook

- **Date:** 2026-04-27
- **Status:** Draft — pending implementation
- **Roadmap position:** Item 2 of 10 in the `db-roadmap` GitHub label
- **GitHub issue:** [#156 — Online migration patterns + safe-migration lint](https://github.com/kabradshaw1/portfolio/issues/156)
- **Builds on:**
  - `go/Makefile` `preflight-go-migrations` target (existing)
  - `golang-migrate` setup across all Go services
  - `docs/adr/ecommerce/go-sql-optimization-reporting.md` (the partitioning migration that taught the project why migration safety matters)

## Context

Every Go service uses `golang-migrate` with `NNN_name.up.sql` / `NNN_name.down.sql` pairs. Migrations run as K8s Jobs on every deploy. The project already has a Makefile target (`make preflight-go-migrations`) that spins up Postgres in Docker and runs the migrations end-to-end — that catches *syntactic* failures.

What it doesn't catch is *operationally unsafe* migrations: ones that compile and run fine on an empty database but would lock a busy production table for minutes, rewrite millions of rows during DDL, or cause cascading replica lag. These patterns are the single most common backend-interview topic ("how do you add a NOT NULL column to a 100M-row table without downtime?") and the single most common cause of preventable production incidents.

The partitioning migration in 2026-04-22 surfaced this directly — three CI failures from index name collisions and FK constraints on a partitioned table, only caught because the migration ran end-to-end against real Postgres. The right next step is to push the safety check earlier in the pipeline: detect unsafe patterns at lint time, before they ever reach Postgres.

## Goals

- Detect unsafe migration patterns at lint time, not at Postgres runtime.
- Encode *which* patterns are unsafe (and why) in checked-in Go code that anyone can read and reason about.
- Provide a written playbook of the safe alternatives — not just "the linter said no" but "here's the multi-step recipe."
- Integrate with `make preflight-go-migrations` so unsafe patterns fail CI.
- Demonstrate the safe pattern with a real migration in the codebase.

## Non-goals

- Linting Java migrations (Spring/JPA owns the schema; no DDL files to lint).
- Reaching `squawk` parity on rule count — we want a curated, well-documented set, not exhaustive coverage.
- Replacing or duplicating the existing `make preflight-go-migrations` runtime checks. Both layers stay.
- Atlas / pg-osc-style online migration tooling — overkill at portfolio scale.
- Linting CREATE TABLE on first-migration-of-a-service files — those run against an empty schema and the patterns that are unsafe on existing tables are fine here.

## Architecture

```
go/cmd/migration-lint/
├── main.go                  CLI entry — globs migrations, applies rules, exits non-zero on errors
└── lint/
    ├── lint.go              Lint(files []string) ([]Violation, error)
    ├── rule.go              Rule interface, Violation struct, Severity enum
    ├── parser.go            thin wrapper around pganalyze/pg_query_go for parsing + statement walking
    ├── ignore.go            parses `-- migration-lint: ignore=MIG00X reason="…"` directives from comments
    └── rules/
        ├── mig001_concurrent_index.go     CREATE INDEX without CONCURRENTLY on existing table
        ├── mig002_nonnull_default.go      ALTER TABLE … ADD COLUMN … NOT NULL DEFAULT (non-static)
        ├── mig003_alter_column_type.go    ALTER COLUMN TYPE causing table rewrite
        ├── mig004_check_not_valid.go      ADD CONSTRAINT … CHECK without NOT VALID
        ├── mig005_drop_column.go          DROP COLUMN — require explicit confirm directive
        ├── mig006_rename_column.go        RENAME COLUMN — require explicit confirm directive
        ├── mig007_concurrent_in_tx.go     CONCURRENTLY mixed with other DDL in the same file
        └── mig008_lock_table.go           LOCK TABLE — require documented purpose
```

The linter is a single Go binary that takes a list of `.sql` files (or a glob) and prints findings. Exit codes: `0` clean, `1` error-severity violation, `2` parse failure or invocation error.

Why CGO via `pg_query_go`? Postgres SQL is genuinely hard to parse — multi-line statements, dollar-quoted strings, comment placement, complex expressions. Regex linters fall over on real-world migrations. `pganalyze/pg_query_go` wraps `libpg_query`, which is *the* Postgres parser extracted from the server source. It's the only correct way to walk the statement tree. The CGO dep is acceptable; the linter is a developer-tool binary, not a runtime dependency.

## Rule design

Every rule implements:

```go
type Rule interface {
    ID() string                                              // e.g., "MIG001"
    Description() string                                      // human-readable purpose
    Severity() Severity                                       // Error | Warning
    Check(stmt *pg_query.RawStmt, ctx *FileContext) []Violation
}
```

`FileContext` carries: the filename, line offsets for accurate error reporting, statements seen earlier in the same file (so MIG001 can know whether `CREATE INDEX` targets a table created by an earlier `CREATE TABLE` in the same migration), and the file's parsed ignore directives.

### MIG001 — `CREATE INDEX` without `CONCURRENTLY` on an existing table

`CREATE INDEX` takes an `ACCESS EXCLUSIVE` lock on the table for the duration of the build. On a busy production table this can lock writers for minutes. `CREATE INDEX CONCURRENTLY` builds the index without blocking writes (slower but online).

Heuristic for "existing table": if the file contains a `CREATE TABLE foo` earlier than the `CREATE INDEX … ON foo`, it's a same-migration index — allow non-CONCURRENT. Otherwise the table is presumed to exist in a prior migration → require `CONCURRENTLY`. (Heuristic is conservative — false positives are easy to silence with the ignore directive.)

### MIG002 — `ADD COLUMN ... NOT NULL` with a non-constant default

PG 11+ supports fast-path `ADD COLUMN ... NOT NULL DEFAULT <constant>` without a table rewrite. But a *volatile* default (`now()`, `uuid_generate_v4()`, a function call) still rewrites every row. Detect any non-constant default expression on a NOT NULL `ADD COLUMN`.

### MIG003 — `ALTER COLUMN ... TYPE`

Type changes generally rewrite every row unless the conversion is binary-compatible. The safe pattern is expand-and-contract (add new column → backfill → switch reads/writes → drop old). Always flag.

### MIG004 — `ADD CONSTRAINT ... CHECK` without `NOT VALID`

A plain `ADD CONSTRAINT … CHECK` scans the entire table to validate. The two-step pattern (`ADD … NOT VALID` → `VALIDATE CONSTRAINT`) avoids the long lock during the initial DDL.

### MIG005 — `DROP COLUMN`

Linters can't know whether the application has stopped using the column. Require an explicit per-statement directive: `-- migration-lint: ignore=MIG005 reason="app deploy 2026-05-01 stopped writing this column; verified empty in pg_stat_user_tables"`. Without that comment, the rule fails.

### MIG006 — `RENAME COLUMN`

In code-driven systems, a column rename is rarely safe in one shot — old code paths still reference the old name. Same opt-in directive pattern as MIG005.

### MIG007 — `CREATE INDEX CONCURRENTLY` mixed with other statements

`golang-migrate` wraps each migration in a transaction by default. `CREATE INDEX CONCURRENTLY` cannot run inside a transaction and will fail. The standard convention is one `CREATE INDEX CONCURRENTLY` statement per migration file, with no other statements. Detect: any file that contains both `CONCURRENTLY` and any other statement (CREATE TABLE, ALTER, INSERT, etc.).

### MIG008 — `LOCK TABLE` without a documented purpose

Explicit `LOCK TABLE` calls are rarely correct in migrations. Require an inline comment on the statement explaining why the lock is needed. Default severity: Warning (not Error) — there are legitimate uses, but each one should have its rationale checked in.

## Ignore directives

Single-line comments in migration files can opt out of specific rules:

```sql
-- migration-lint: ignore=MIG001 reason="initial table creation, table is empty"
CREATE INDEX idx_users_email ON users (email);
```

The directive applies to the *next* SQL statement only. Multiple rules can be listed: `ignore=MIG001,MIG004`. The `reason="…"` is required — this is the senior-engineer convention: every exception checks in *why* it's safe.

`pg_query_go` reports each statement's byte range in the source (`RawStmt.StmtLocation` / `StmtLen`); the ignore-directive parser scans the original SQL text for comment lines that fall immediately above each statement's range, so attribution is exact even when statements are interleaved.

## Worked example: a new migration demonstrating the safe pattern

Rather than rewriting historical migrations (`golang-migrate` tracks by number, not content — modifying an applied migration is unsafe), the spec ships a *new* migration in `go/product-service/migrations/` that adds a useful index using the safe pattern:

```sql
-- 004_add_product_search_index.up.sql
-- See docs/runbooks/postgres-migrations.md (recipe 4) for why this pattern is required.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_products_name_trgm
  ON products USING gin (name gin_trgm_ops);
```

The exact target table and index will be picked during implementation, but the requirements are: (a) addresses a real query path that benefits from the index, (b) the file contains exactly one `CREATE INDEX CONCURRENTLY` statement and nothing else, (c) the down migration drops the index with `IF EXISTS`.

This proves the linter doesn't trip on the *correct* pattern, and the migration file itself becomes the canonical reference cited from the playbook.

## Playbook content

`docs/runbooks/postgres-migrations.md` — eight recipes, each section structured the same way:

1. **What you want to do** (e.g., "Add a non-null column to a busy table")
2. **The unsafe one-shot version** (the SQL that would lock prod)
3. **Why it's unsafe** (one or two sentences on the failure mode)
4. **The safe multi-step version** (one SQL block per migration file in the sequence, with `golang-migrate` numbering example)
5. **Linter rule that catches the unsafe form** (e.g., MIG002)
6. **Cross-references** (related ADRs, related rules)

The eight recipes:

| # | Title | Linter rule |
|---|---|---|
| 1 | Add a NOT NULL column | MIG002 |
| 2 | Drop a column | MIG005 |
| 3 | Rename a column (expand-and-contract) | MIG006 |
| 4 | Add an index (`CONCURRENTLY` in its own file) | MIG001, MIG007 |
| 5 | Add a CHECK constraint (`NOT VALID` + `VALIDATE`) | MIG004 |
| 6 | Rename a table (`CREATE VIEW` compat shim) | — |
| 7 | Change a column type (expand-and-contract) | MIG003 |
| 8 | Partition an existing table | cross-references the partitioning ADR |

## Integration with preflight

`make preflight-go-migrations` currently:
1. Spins up Postgres via Docker
2. Runs `migrate up` for each Go service
3. Verifies tables exist

The new step runs *before* (1) — fast-fail before any container starts:

```makefile
preflight-go-migrations:
	@cd go && go build -o /tmp/migration-lint ./cmd/migration-lint
	@/tmp/migration-lint go/*/migrations/*.sql
	@# (existing Postgres up + migrate up steps continue here)
```

The linter binary is built fresh each run from the worktree to avoid stale binaries.

## Testing

**Per-rule tests** in `go/cmd/migration-lint/lint/lint_test.go` (or one file per rule, sibling to the rule source). Each rule has at least three SQL fixtures:

- A positive case (migration that should trigger the rule) → asserts violation reported with correct line/severity
- A negative case (migration that should NOT trigger the rule) → asserts no violation
- An ignore case (positive case with a `-- migration-lint: ignore=` directive) → asserts violation suppressed

**Snapshot test** — run the linter against the entire `go/*/migrations/*.sql` tree as it stands after the worked-example refactor and assert zero violations. This becomes the regression net for future migrations.

**Preflight integration test** — `make preflight-go-migrations` is the integration check; once the linter is wired in, any unsafe migration commit fails preflight.

## Rollout

Each step is an independently mergeable PR.

1. Land the linter binary + rules + tests with no preflight wiring (zero blast radius).
2. Run it locally over existing migrations, file an inventory of *current* violations and add ignore directives where safe (initial-empty-table cases).
3. Wire into `make preflight-go-migrations` (this is the gate that enforces it going forward).
4. Add the worked-example migration.
5. Land the playbook.
6. Land the companion ADR.

## ADR

A companion ADR will be written at `docs/adr/database/migration-lint.md` (new directory — no DB-specific ADR home currently exists; previous DB ADRs lived under `ecommerce/` and `infrastructure/` for historical reasons). The ADR documents:

- Why custom Go over `squawk`/`atlas`/regex (the AST + portfolio narrative argument)
- Rule selection criteria — why these eight, why not more
- Ignore-directive design — why `reason="…"` is mandatory
- The "no historical rewrites" decision and its implications for the worked example

## Consequences

**Positive:**
- Unsafe migrations fail at lint time, before Docker spin-up or CI runtime.
- The rule set becomes a checked-in answer to "which patterns are unsafe and why" — strong interview talking point.
- The playbook covers eight scenarios any backend engineer will hit.
- The worked-example migration leaves a real-world reference in the codebase.

**Trade-offs:**
- CGO build dep via `pg_query_go`. Acceptable — the linter is a developer tool, not runtime.
- Curated rule set is narrower than `squawk`'s ~30 rules. Acceptable — depth over breadth, and additional rules can be added incrementally.
- Heuristics (especially MIG001's "existing table" detection) will produce occasional false positives. The ignore-directive pattern handles these without weakening the rule.
- Java migrations are not covered. Acceptable — Spring/JPA-managed schemas don't have DDL files to lint.

**Phase 2 (future):**
- Add `MIG009` — backfill detection (update statements that touch unbounded row counts).
- Pre-commit hook that runs the linter on staged `.sql` files for faster feedback than `make preflight-go-migrations`.
- IDE integration via a custom diagnostic LSP, if the linter sees enough use to justify it.
