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

> I usually choose opaque surrogate keys for internal identity and keep business
> identifiers as separate unique columns. That avoids changing foreign keys when
> a customer-facing ID format changes, but it means uniqueness still has to be
> enforced explicitly. In this repo, migration files model both: primary keys
> identify rows, while constraints and processed-event tables protect business
> invariants and idempotency.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: What belongs in a unique constraint?

Fast answer:

> A unique constraint belongs on data where duplicates would violate the domain,
> not just where the UI happens to check first. The production failure mode is a
> race: two requests pass application validation and insert the same logical
> record. I would let Postgres enforce the invariant and map duplicate-key
> failures into structured `apperror` responses instead of leaking SQL details.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: When do you denormalize?

Fast answer:

> I denormalize when the read path is hot enough that repeated joins or
> aggregation become the bottleneck, and I can define how the copy stays fresh.
> The tradeoff is write complexity and stale data. In this repo, outbox rows,
> processed-event tables, reporting pools, and materialized/search-oriented
> migrations are better examples than letting services read each other's tables.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: How do you represent workflow state?

Fast answer:

> Workflow state should be explicit, constrained, and monotonic where possible.
> I prefer status columns, timestamps, and transition checks over inferring state
> from scattered side effects. The failure mode is a saga or payment getting
> stuck in an impossible state; repo migrations for orders, outbox rows, and
> processed events show that state and idempotency are part of the schema.

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

> `EXPLAIN ANALYZE` shows the planner's chosen path and the actual runtime:
> scans, joins, row estimates versus real rows, sort cost, timing, and buffers if
> enabled. The key is comparing estimated and actual cardinality, because bad
> estimates often explain bad plans. For repo search issues, I would use it to
> confirm the product-name query is using the trigram GIN index.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: How do you know an index is unused?

Fast answer:

> I look at database stats such as scans, tuples read, and index size, then
> compare that with query plans for real traffic. An index with no reads still
> costs storage, vacuum work, and write amplification, so I would not keep it
> just because it looks plausible. Before dropping one, I would check deploy
> history and slower periodic jobs, not only the last few minutes of traffic.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: When can an index hurt writes?

Fast answer:

> Every index has to be maintained on insert, update, and delete, and it can
> increase lock time, WAL volume, cache pressure, and vacuum work. That is worth
> it for hot read paths, but bad for low-selectivity or unused indexes. In this
> repo, the order indexes should map to real access patterns, while the product
> trigram index is justified by search behavior.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: How do you optimize `ILIKE` search?

Fast answer:

> Plain `ILIKE '%term%'` usually cannot use a normal btree index, so I would use
> `pg_trgm` with a GIN index or move to a search engine if ranking and language
> features matter. The repo has `004_enable_pg_trgm` and
> `005_add_product_search_index` using `gin_trgm_ops`, which is the right
> Postgres-native fix for product-name search.

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

> A normal `CREATE INDEX` can hold locks that block writes on a busy table, and
> the longer the table, the bigger the incident window. `CREATE INDEX
> CONCURRENTLY` reduces that blocking but has restrictions, like running outside
> a transaction. That is why the repo's product search index is isolated in a
> single-statement migration file.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: How do you roll back a bad migration?

Fast answer:

> First decide whether the bad migration changed schema, data, or both. A clean
> down migration may be enough for a harmless index, but destructive data
> changes often need a forward fix, restore, or targeted repair. In this repo I
> would use the paired `down.sql` files where safe, but I would not blindly roll
> back a migration that dropped or transformed production data.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: What is an expand-contract migration?

Fast answer:

> Expand-contract means first adding the new shape in a backward-compatible way,
> then moving the application to use it, and only later removing the old shape.
> It avoids forcing every pod and migration to switch atomically. For this repo,
> that means nullable columns, dual reads or writes when needed, backfills, and
> later constraint validation in the service migration directories.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: Why do migration jobs use a direct database URL?

Fast answer:

> Migration jobs often need session-level behavior and commands that do not fit
> PgBouncer transaction pooling, especially DDL and `CREATE INDEX CONCURRENTLY`.
> A direct URL also makes it clear that migrations are privileged operational
> work, not normal app traffic. The repo calls this out with
> `DATABASE_URL_DIRECT` for Kubernetes migration jobs.

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

> I size pools from database capacity, replica count, p95 query time, and
> expected concurrency, then leave headroom for migrations and operators. The
> failure mode is every pod opening a reasonable-looking pool that multiplies
> into too many Postgres connections. The order service anchors this with a
> bounded `pgxpool` config, health checks, and separate primary/reporting pools.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: What happens when the pool is exhausted?

Fast answer:

> Requests wait for a connection until their context deadline, then fail or
> cascade into higher latency. If handlers do not set timeouts, pool exhaustion
> can turn into stuck goroutines and client timeouts. I would watch pgx acquire
> latency, in-use connections, request latency, and readiness behavior before
> simply raising `MaxConns`.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: Transaction pooling versus session pooling?

Fast answer:

> Transaction pooling returns the server connection to PgBouncer after each
> transaction, which scales better but breaks assumptions about session state,
> temp tables, prepared statements, and some DDL workflows. Session pooling keeps
> state but uses more server connections. In this repo, app traffic can benefit
> from pooling, while migrations use a direct database URL.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: Why set `application_name`?

Fast answer:

> `application_name` makes database activity attributable: slow queries, locks,
> and connection spikes can be tied back to a service and pool. Without it, all
> clients look the same during an incident. The order service sets distinct
> runtime params for primary and reporting pools so database diagnostics have
> useful labels.

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

> Yes, but only by retrying the entire transaction from the beginning. A
> serialization failure means the database rejected the interleaving, so retrying
> one statement inside the old transaction is wrong. The retry has to be bounded,
> idempotent, and observable, matching the repo's pattern of classifying
> transient Postgres failures separately from domain errors.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: How do retries interact with transactions?

Fast answer:

> A retryable transaction must wrap all reads, writes, and commit in one retry
> function, because the snapshot and side effects belong together. External
> effects should be outside the transaction or represented through an outbox so a
> retry does not double-send messages. The payment-service outbox and
> processed-events migrations are the repo anchor for that pattern.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: How do you avoid retry storms?

Fast answer:

> Use small retry budgets, exponential backoff with jitter, context deadlines,
> and circuit breaking when the database is unhealthy. The failure mode is
> synchronized retries multiplying load exactly when Postgres is already
> saturated. I would expose retry counters and pool metrics, and let readiness
> fail when the service cannot make progress.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: What metrics show retry pressure?

Fast answer:

> I would track retry attempts by error class, final retry failures, transaction
> duration, pool acquire latency, pool saturation, and database connection or
> lock waits. A rising retry rate with rising latency means retries are adding
> pressure, not masking it. Logs should include request or trace IDs for
> examples, but metrics should carry the aggregate signal.

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

> Use parameterized queries, typed query arguments, and allowlisted dynamic
> identifiers when a column or sort direction must be dynamic. Escaping strings
> manually is not a security boundary. In the Go services, pgx placeholders and
> repository methods should carry user values as parameters, with `apperror`
> returning safe validation errors instead of SQL text.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: What should never be logged?

Fast answer:

> Never log database passwords, connection URLs, tokens, raw payment data,
> session secrets, or full rows containing PII. The tradeoff is still preserving
> enough context to debug, so log stable IDs, error codes, request IDs, and trace
> IDs. The repo's `apperror` middleware logs server errors with request IDs while
> returning safe client responses.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: App role versus migration role?

Fast answer:

> The app role should have only the privileges needed at runtime, usually DML on
> its service schema. The migration role can create tables, alter schema, add
> indexes, and manage extensions, so it should not be used by the web process.
> This split limits blast radius if an app credential leaks and matches the
> repo's direct migration URL pattern.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: How do you rotate database credentials?

Fast answer:

> I rotate by adding the new credential, rolling app pods to use it, confirming
> both old and new are accepted during the transition, then revoking the old one.
> The failure mode is cutting over in one step and taking every pod down with
> authentication failures. In Kubernetes I would update the secret through the
> repo's ops-as-code path, roll the deployment, and verify health checks.

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

> Readiness should fail when the pod cannot serve real traffic, such as losing a
> required database, Redis, Kafka, or downstream dependency. It should not fail
> for optional degradation unless the service contract requires that dependency.
> The repo's Go health handlers check dependencies so Kubernetes can remove an
> unhealthy pod from service endpoints.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: Should liveness check the database?

Fast answer:

> Usually no. Liveness should prove the process can make progress, not restart a
> healthy process because Postgres is temporarily unavailable. Database checks
> belong in readiness; otherwise a database incident can cause mass pod restarts
> and make recovery noisier.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: How do probes affect rolling deploys?

Fast answer:

> During a rolling deploy, readiness controls when a new pod receives traffic
> and when an old pod can be removed. If readiness turns green before migrations,
> caches, or database pools are ready, users get errors from a pod Kubernetes
> thinks is healthy. Tight dependency checks and sensible timeouts protect the
> Go services during rollout.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: How do you avoid health-check thundering herds?

Fast answer:

> Keep probes cheap, bounded, and cached where appropriate, and avoid expensive
> dependency queries on every probe from every pod. The failure mode is health
> checks becoming their own production load spike. In this repo, a simple ping
> with a short context timeout is better than running real business queries from
> health handlers.

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

> A 422 should include a stable error code, a human-readable message, request ID
> when present, and field-level validation details when the client can fix the
> request. It should not include stack traces or database errors. That lines up
> with `go/pkg/apperror`, which has validation detail fields and a consistent
> JSON envelope.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: How do you preserve request IDs?

Fast answer:

> Accept or generate a request ID at the edge, store it in request context, and
> copy it into logs and error responses. The important part is consistency: the
> client, API logs, and operators should all have the same lookup key. The
> `apperror` middleware already includes `request_id` in safe responses and
> server-side logs.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: What details are safe for clients?

Fast answer:

> Safe client details are stable codes, user-actionable messages, validation
> fields, and request IDs. Unsafe details include SQL text, stack traces,
> connection strings, internal hostnames, secret names, and raw dependency
> errors. In this repo, unknown errors become a generic `INTERNAL_ERROR` while
> the logs retain operational context.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: How do you test error middleware?

Fast answer:

> I test known app errors, validation errors, unknown panics or returned errors,
> request-ID propagation, response status codes, and log behavior. The important
> failure mode is accidentally leaking an internal error while trying to be
> helpful. `go/pkg/apperror/middleware_test.go` is the repo anchor for these
> cases.

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

> I include the operation name, service, dependency, safe identifiers like order
> ID, result status, and important timing or retry attributes. I avoid high
> cardinality or sensitive values such as raw SQL with parameters or tokens. The
> repo's tracing package already wraps Redis, AMQP, Kafka, and logging so spans
> can connect service work to async dependencies.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: What should be a metric versus a log?

Fast answer:

> Metrics are for aggregate numeric signals you alert or graph: latency,
> throughput, error rate, retry count, and pool saturation. Logs are for
> representative events and debugging context tied to a request or trace ID. I
> would not put user IDs or raw search strings into metric labels because that
> creates cardinality and privacy problems.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: How do you trace async workflows?

Fast answer:

> Propagate W3C trace context through message headers, then extract it in the
> consumer before starting the next span. That keeps the order, payment, or
> event-processing work in one trace even when it crosses Kafka or AMQP. The
> repo has `InjectAMQP`, `ExtractAMQP`, `InjectKafka`, and `ExtractKafka` helpers
> for exactly that.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: What if traces are sampled out?

Fast answer:

> Sampling means traces are not the only source of truth, so metrics and
> structured logs still need enough signal to diagnose production issues. I
> would keep error counts, latency histograms, retry metrics, and request IDs in
> logs even when a trace is absent. The repo's logging helper adds trace IDs when
> present, but the log event should still be useful without one.

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

> RPO is how much data loss the business can tolerate, and RTO is how long
> recovery may take. WAL archiving mainly improves RPO by letting you recover
> close to the failure time; restore automation and rehearsals improve RTO. The
> repo's backup and WAL integration checks are the right kind of evidence that
> those targets are testable.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: What is point-in-time recovery?

Fast answer:

> Point-in-time recovery restores a base backup and then replays WAL records up
> to a chosen timestamp or transaction boundary. It is what you use for
> accidental deletes or bad writes, not only disk failure. The operational risk
> is discovering during an incident that WAL archives are missing, corrupt, or
> not restorable.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: How often should restore be tested?

Fast answer:

> Restore should be tested on a schedule and after meaningful backup, Postgres,
> storage, or schema changes. Frequency depends on the service's RPO/RTO, but an
> untested backup should be treated as no backup. I like automated restore
> verification plus periodic manual drills, which matches the repo's DB
> integration tests around backup verification.

Repo anchors:
- `go/pkg/apperror` - structured error envelope, validation errors, safe 500

#### Follow-up: Who can access backups?

Fast answer:

> Backup access should be tightly limited to restore automation and a small
> operator group, with encryption, audit logs, and separate credentials from the
> app. Backups often contain the whole database, so they can be more sensitive
> than individual service credentials. I would verify both restore permissions
> and denial of normal app access.

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
