# Database, Observability, And Security Rehearsal

Use this for SQL design, indexing, migrations, pool tuning, health checks,
tracing, logging, metrics, and database security questions.

## Repo Anchors

- `go/pkg/apperror`: structured error envelope, validation errors, safe 500
  responses, and request IDs.
- `go/pkg/db/*_integration_test.go`: PgBouncer, backup verification, Postgres
  extensions, and WAL archive integration checks.
- `go/pkg/tracing`: OpenTelemetry init plus AMQP, Kafka, Redis, and logging
  trace propagation helpers.
- `go/pkg/resilience/pgx.go`: PostgreSQL retry classification that avoids
  retrying domain, no-rows, duplicate-key, and constraint errors.
- `go/order-service/internal/db/pools.go`: tuned `pgxpool` config, primary and
  reporting pools, application names, read-replica fallback, and pool health.
- `go/*-service/migrations`: schema evolution, constraints, indexes,
  partitioning, materialized views, outbox, processed events, and search index
  migrations.
- `go/product-service/migrations/005_add_product_search_index.up.sql`: trigram
  GIN index for product-name search with `CREATE INDEX CONCURRENTLY`.
- `go/order-service/migrations/007_add_constraints_and_indexes.up.sql`: check
  constraints and access-pattern indexes.
- `go/*-service/internal/handler/health.go`: dependency health checks for
  Postgres, Redis, Kafka, and service readiness.

## High-Frequency Questions

### 1. How do you design a database schema for a backend service?

Fast answer:

> I start from access patterns and invariants. Tables should model owned data
> for the service, constraints should enforce business rules, and indexes should
> match common filters, joins, and ordering. I avoid using the database as a
> shared integration layer between services. In this repo, services own their
> migrations, and order/cart/payment schemas include constraints, saga state,
> processed events, outbox rows, and indexes for real query paths.

Follow-ups:

- How do you choose primary keys?
- What belongs in a unique constraint?
- When do you denormalize?
- How do you represent workflow state?

### 2. How do you optimize a slow SQL query?

Fast answer:

> I start with the actual query plan, not guesses. I check whether the query is
> scanning too much data, missing an index, sorting in memory, joining poorly, or
> waiting on locks. Then I match an index to the WHERE, JOIN, and ORDER BY
> pattern, reduce selected columns, or change pagination. The repo has explicit
> pagination indexes and a trigram GIN index for product-name search.

Follow-ups:

- What does `EXPLAIN ANALYZE` show?
- How do you know an index is unused?
- When can an index hurt writes?
- How do you optimize `ILIKE` search?

### 3. How do you handle migrations safely?

Fast answer:

> Migrations should be small, reversible, reviewed, and compatible with running
> app versions. For large tables, avoid long exclusive locks: add nullable
> columns first, backfill separately, add constraints as `NOT VALID`, validate
> later, and create indexes concurrently when needed. In this repo, Go services
> use numbered `up` and `down` migrations, and the product search migration uses
> `CREATE INDEX CONCURRENTLY` in a single-statement file.

Follow-ups:

- Why can `CREATE INDEX` be risky?
- How do you roll back a bad migration?
- What is an expand-contract migration?
- Why do migration jobs use a direct database URL?

### 4. How do pgx pools and PgBouncer fit together?

Fast answer:

> The app should use a bounded `pgxpool` so it does not open unlimited database
> connections. PgBouncer sits in front of Postgres to multiplex many app
> connections into fewer server connections, which matters with multiple
> replicas. Migrations should bypass PgBouncer when session behavior matters. In
> this repo, pools use `pgxpool.ParseConfig`, `NewWithConfig`, max/min conns,
> health checks, and Kubernetes migration jobs use `DATABASE_URL_DIRECT`.

Follow-ups:

- How do you size a pool?
- What happens when the pool is exhausted?
- Transaction pooling versus session pooling?
- Why set `application_name`?

### 5. What database errors are retryable?

Fast answer:

> Retry connection resets, temporary network failures, and transient database
> unavailability. Do not retry domain errors, validation errors, no rows,
> duplicate keys, or constraint violations because retries will not change the
> outcome and can amplify load. The shared `resilience.IsPgRetryable` helper
> explicitly excludes app errors, no-rows, duplicate-key, and constraint
> messages.

Follow-ups:

- Would you retry serialization failures?
- How do retries interact with transactions?
- How do you avoid retry storms?
- What metrics show retry pressure?

### 6. How do you secure database access?

Fast answer:

> I use least-privilege credentials, separate app and migration roles, secrets
> outside source control, TLS where appropriate, parameterized queries, input
> validation, and logs that never include credentials or sensitive row data.
> Service ownership also matters: one service should not reach into another
> service's tables. In this repo, each Go service has its own database and
> migration set, and shared error handling prevents leaking internal failures.

Follow-ups:

- How do you prevent SQL injection?
- What should never be logged?
- App role versus migration role?
- How do you rotate database credentials?

### 7. What belongs in a health check?

Fast answer:

> Liveness should answer whether the process is alive. Readiness should answer
> whether it can serve traffic, including critical dependencies. Dependency
> checks need tight timeouts so health endpoints do not hang. In this repo,
> service health handlers ping Postgres, Redis, Kafka, or downstream services,
> and the AI service separates `/health` from `/ready` style dependency probes.

Follow-ups:

- When should readiness fail?
- Should liveness check the database?
- How do probes affect rolling deploys?
- How do you avoid health-check thundering herds?

### 8. How do structured errors help security and operations?

Fast answer:

> Structured errors give clients stable codes and request IDs while keeping
> internal details out of responses. Operators still get logs with the real
> cause. In this repo, handlers attach `apperror` values to Gin context, and the
> middleware returns a consistent JSON envelope. Unknown errors become a safe
> `INTERNAL_ERROR` message while the real error is logged.

Follow-ups:

- What should a 422 response include?
- How do you preserve request IDs?
- What details are safe for clients?
- How do you test error middleware?

### 9. How do traces, logs, and metrics work together?

Fast answer:

> Metrics tell me something is wrong, traces show where time went, and logs
> explain representative events with business context. I want the same trace or
> request ID across all three. The repo initializes OpenTelemetry through OTLP,
> propagates trace context across AMQP, Kafka, and Redis, and has logging helpers
> so trace IDs appear in structured logs.

Follow-ups:

- What would you put in a span?
- What should be a metric versus a log?
- How do you trace async workflows?
- What if traces are sampled out?

### 10. How do you design backups and WAL recovery?

Fast answer:

> Backups are only real if restore is tested. I want base backups, WAL archiving
> for point-in-time recovery, retention policy, encryption, access control, and
> a regular restore verification job. The repo has shared DB integration tests
> around backup verification and WAL archive behavior, which is the right signal:
> recovery is an operational requirement, not just a setting.

Follow-ups:

- RPO versus RTO?
- What is point-in-time recovery?
- How often should restore be tested?
- Who can access backups?

## Scenario Drills

### Scenario: Product search gets slow under traffic.

Answer outline:

> I would inspect query plans and p95/p99 latency, then check whether the search
> pattern is using the trigram GIN index. If traffic is high, I would also check
> pool saturation, cache hit rate, and whether queries are selecting too much
> data. The repo anchor is the product search index migration using
> `gin_trgm_ops` for `ILIKE`-style name search.

### Scenario: Database CPU is high after a deploy.

Answer outline:

> First compare query volume, slow queries, pool usage, and retry counts before
> and after deploy. Then check for missing indexes, new N+1 patterns, bad query
> plans, or retries amplifying load. If needed, fail readiness for unhealthy
> pods, reduce traffic, or roll back the app change before changing the database.

### Scenario: A migration needs to add a constraint to a large table.

Answer outline:

> I would avoid a blocking one-shot migration. Add the constraint as `NOT VALID`,
> backfill or clean invalid rows, validate the constraint later, and make sure
> the app can run during every step. I would also include a rollback plan and run
> migration preflight before commit.

## Coding Exercises

### Exercise 1: Safe repository method

Prompt:

> Write a repository method that fetches a row by ID using context, parameterized
> SQL, maps no rows to a typed not-found error, and wraps unexpected database
> errors safely.

What to say while coding:

- Always pass `context.Context`.
- Use `$1` parameters, never string formatting.
- Treat `pgx.ErrNoRows` as not found.
- Return safe app errors at the boundary.

### Exercise 2: Health handler

Prompt:

> Implement an HTTP health handler that pings Postgres and Redis with a short
> timeout and returns a structured checks object.

What to say while coding:

- Use a child context with timeout.
- Report each dependency separately.
- Return 503 if a critical dependency fails.
- Do not leak credentials or DSNs.

### Exercise 3: Query optimization explanation

Prompt:

> Given a slow `WHERE user_id = $1 ORDER BY created_at DESC LIMIT 50` query,
> explain and write the index you would add.

Fast answer:

> I would use a composite index on `(user_id, created_at DESC)` because the
> filter narrows by user and the order can be satisfied from the same index.
> Then I would confirm with `EXPLAIN ANALYZE` and watch write overhead.
