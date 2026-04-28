# ADR: PostgreSQL Migration Linter

- **Date:** 2026-04-27
- **Status:** Accepted
- **Builds on:**
  - `docs/adr/ecommerce/go-database-optimization.md` (schema hardening pass)
  - `docs/adr/ecommerce/go-sql-optimization-reporting.md` (the partitioning migration that surfaced the gap)
- **Roadmap position:** item 2 of 10 in the `db-roadmap` GitHub label

## Context

Every Go service ships migrations as `golang-migrate` `NNN_name.up.sql` /
`.down.sql` pairs. `make preflight-go-migrations` already spins up Postgres
in Docker and runs every migration end-to-end, catching syntactic failures.

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
   (MIG001–MIG008) that target the most common unsafe patterns.
3. Supports per-statement opt-out via
   `-- migration-lint: ignore=MIGNNN reason="..."` comments. The
   `reason="..."` field is mandatory — every exception is a checked-in
   artifact of *why* it's safe.
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
is a developer-tool binary, not a runtime dependency. CI runners (Linux gcc)
and the Mac dev machine (Apple clang) both build it without configuration.

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
ships with the *why* so future maintainers (and reviewers) can audit whether
the exception still applies. This mirrors the senior-engineer convention of
recording rationale in code, not in slack.

### "No historical rewrites" decision

`golang-migrate` tracks migrations by version number, not by content. A
migration that has been applied to production *cannot* be edited without
creating a divergence between the dev and prod schema-history table. The
worked-example migration is therefore a *new* migration; existing migrations
get ignore directives where their patterns would trip the linter today.

### Statement attribution: `StmtStart` vs `StmtLocation`

`pg_query_go` reports `RawStmt.StmtLocation` as the byte offset where the
parser began consuming tokens for the statement, which can include leading
whitespace and `--` line comments that visually belong "above" the
statement. We compute a `StmtStart` helper that walks past that prefix to
the first SQL keyword. This matters because:

- Line numbers in violation messages should point at the SQL, not at a
  preceding comment.
- Ignore-directive attribution is range-based: a directive applies to the
  next statement only if no non-comment content sits between them.

If we used `StmtLocation` directly, a `-- migration-lint: ignore=...`
comment would appear *after* its target's reported start and never match.

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
- Java migrations are not covered. Acceptable — Spring/JPA-managed schemas
  don't have DDL files to lint.

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
    ├── parser.go                         # pg_query_go wrapper, StmtStart helper
    ├── ignore.go                         # `-- migration-lint: ignore=` parser
    ├── rule.go                           # Rule interface, types
    └── rules/                            # MIG001-MIG008

docs/runbooks/postgres-migrations.md      # 8-recipe playbook
docs/adr/database/migration-lint.md       # this ADR
go/product-service/migrations/004_enable_pg_trgm.up.sql           # supporting extension
go/product-service/migrations/005_add_product_search_index.up.sql # CONCURRENTLY in own file
```
