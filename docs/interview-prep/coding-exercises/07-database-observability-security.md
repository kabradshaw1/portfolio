# Database Observability Security Coding Exercises

### 1. Safe repository method

Prompt:

> Write a repository method that fetches a row by ID using context, parameterized
> SQL, maps no rows to a typed not-found error, and wraps unexpected database
> errors safely.

What to say while coding:

- Always pass `context.Context`.
- Use `$1` parameters, never string formatting.
- Treat `pgx.ErrNoRows` as not found.
- Return safe app errors at the boundary.

Fast design:

> Describe the expected design, edge cases, tests, and tradeoffs.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

### 2. Health handler

Prompt:

> Implement an HTTP health handler that pings Postgres and Redis with a short
> timeout and returns a structured checks object.

What to say while coding:

- Use a child context with timeout.
- Report each dependency separately.
- Return 503 if a critical dependency fails.
- Do not leak credentials or DSNs.

Fast design:

> Describe the expected design, edge cases, tests, and tradeoffs.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

### 3. Query optimization explanation

Prompt:

> Given a slow `WHERE user_id = $1 ORDER BY created_at DESC LIMIT 50` query,
> explain and write the index you would add.

Fast answer:

> I would use a composite index on `(user_id, created_at DESC)` because the
> filter narrows by user and the order can be satisfied from the same index.
> Then I would confirm with `EXPLAIN ANALYZE` and watch write overhead.

Fast design:

> Describe the expected design, edge cases, tests, and tradeoffs.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500
