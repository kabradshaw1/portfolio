# migration-lint

Static analyzer for `golang-migrate` `.up.sql` files. Flags operationally
unsafe DDL patterns (table-rewrite ALTERs, blocking CREATE INDEX, missing
NOT VALID, etc.) before the migration ever reaches Postgres.

See `docs/runbooks/postgres-migrations.md` for the safe-pattern playbook
and `docs/adr/database/migration-lint.md` for design rationale.

## Usage

```
migration-lint go/*/migrations/*.up.sql
```

Exit codes:

- `0` — no violations
- `1` — at least one error-severity violation
- `2` — invocation error or parse failure

## Ignore directives

A single-line comment immediately above a statement opts that statement out
of one or more rules. The `reason="..."` field is required.

```sql
-- migration-lint: ignore=MIG001 reason="initial table creation, table is empty"
CREATE INDEX idx_users_email ON users (email);
```

Multiple rules: `ignore=MIG001,MIG004`.

## Build

```
cd go/cmd/migration-lint
go build -o /tmp/migration-lint .
```

CGO is required (the parser links against `libpg_query`). Ubuntu CI runners
and the Mac dev machine have suitable toolchains by default.
