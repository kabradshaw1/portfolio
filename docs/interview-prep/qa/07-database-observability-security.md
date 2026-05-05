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

#### Follow-up: How do you choose primary keys?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: What belongs in a unique constraint?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: When do you denormalize?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: How do you represent workflow state?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

### 2. How do you optimize a slow SQL query?

Fast answer:

> I start with the actual query plan, not guesses. I check whether the query is
> scanning too much data, missing an index, sorting in memory, joining poorly, or
> waiting on locks. Then I match an index to the WHERE, JOIN, and ORDER BY
> pattern, reduce selected columns, or change pagination. The repo has explicit
> pagination indexes and a trigram GIN index for product-name search.

Follow-ups:

#### Follow-up: What does `EXPLAIN ANALYZE` show?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: How do you know an index is unused?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: When can an index hurt writes?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: How do you optimize `ILIKE` search?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

### 3. How do you handle migrations safely?

Fast answer:

> Migrations should be small, reversible, reviewed, and compatible with running
> app versions. For large tables, avoid long exclusive locks: add nullable
> columns first, backfill separately, add constraints as `NOT VALID`, validate
> later, and create indexes concurrently when needed. In this repo, Go services
> use numbered `up` and `down` migrations, and the product search migration uses
> `CREATE INDEX CONCURRENTLY` in a single-statement file.

Follow-ups:

#### Follow-up: Why can `CREATE INDEX` be risky?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: How do you roll back a bad migration?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: What is an expand-contract migration?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: Why do migration jobs use a direct database URL?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

### 4. How do pgx pools and PgBouncer fit together?

Fast answer:

> The app should use a bounded `pgxpool` so it does not open unlimited database
> connections. PgBouncer sits in front of Postgres to multiplex many app
> connections into fewer server connections, which matters with multiple
> replicas. Migrations should bypass PgBouncer when session behavior matters. In
> this repo, pools use `pgxpool.ParseConfig`, `NewWithConfig`, max/min conns,
> health checks, and Kubernetes migration jobs use `DATABASE_URL_DIRECT`.

Follow-ups:

#### Follow-up: How do you size a pool?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: What happens when the pool is exhausted?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: Transaction pooling versus session pooling?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: Why set `application_name`?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

### 5. What database errors are retryable?

Fast answer:

> Retry connection resets, temporary network failures, and transient database
> unavailability. Do not retry domain errors, validation errors, no rows,
> duplicate keys, or constraint violations because retries will not change the
> outcome and can amplify load. The shared `resilience.IsPgRetryable` helper
> explicitly excludes app errors, no-rows, duplicate-key, and constraint
> messages.

Follow-ups:

#### Follow-up: Would you retry serialization failures?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: How do retries interact with transactions?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: How do you avoid retry storms?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: What metrics show retry pressure?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

### 6. How do you secure database access?

Fast answer:

> I use least-privilege credentials, separate app and migration roles, secrets
> outside source control, TLS where appropriate, parameterized queries, input
> validation, and logs that never include credentials or sensitive row data.
> Service ownership also matters: one service should not reach into another
> service's tables. In this repo, each Go service has its own database and
> migration set, and shared error handling prevents leaking internal failures.

Follow-ups:

#### Follow-up: How do you prevent SQL injection?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: What should never be logged?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: App role versus migration role?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: How do you rotate database credentials?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

### 7. What belongs in a health check?

Fast answer:

> Liveness should answer whether the process is alive. Readiness should answer
> whether it can serve traffic, including critical dependencies. Dependency
> checks need tight timeouts so health endpoints do not hang. In this repo,
> service health handlers ping Postgres, Redis, Kafka, or downstream services,
> and the AI service separates `/health` from `/ready` style dependency probes.

Follow-ups:

#### Follow-up: When should readiness fail?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: Should liveness check the database?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: How do probes affect rolling deploys?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: How do you avoid health-check thundering herds?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

### 8. How do structured errors help security and operations?

Fast answer:

> Structured errors give clients stable codes and request IDs while keeping
> internal details out of responses. Operators still get logs with the real
> cause. In this repo, handlers attach `apperror` values to Gin context, and the
> middleware returns a consistent JSON envelope. Unknown errors become a safe
> `INTERNAL_ERROR` message while the real error is logged.

Follow-ups:

#### Follow-up: What should a 422 response include?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: How do you preserve request IDs?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: What details are safe for clients?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: How do you test error middleware?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

### 9. How do traces, logs, and metrics work together?

Fast answer:

> Metrics tell me something is wrong, traces show where time went, and logs
> explain representative events with business context. I want the same trace or
> request ID across all three. The repo initializes OpenTelemetry through OTLP,
> propagates trace context across AMQP, Kafka, and Redis, and has logging helpers
> so trace IDs appear in structured logs.

Follow-ups:

#### Follow-up: What would you put in a span?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: What should be a metric versus a log?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: How do you trace async workflows?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: What if traces are sampled out?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

### 10. How do you design backups and WAL recovery?

Fast answer:

> Backups are only real if restore is tested. I want base backups, WAL archiving
> for point-in-time recovery, retention policy, encryption, access control, and
> a regular restore verification job. The repo has shared DB integration tests
> around backup verification and WAL archive behavior, which is the right signal:
> recovery is an operational requirement, not just a setting.

Follow-ups:

#### Follow-up: RPO versus RTO?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: What is point-in-time recovery?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: How often should restore be tested?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: Who can access backups?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

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
