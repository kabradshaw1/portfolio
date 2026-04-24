# Java Analytics and Database Optimization

- **Date:** 2026-04-05
- **Status:** Accepted

## Context

The Java task-management services (task-service, activity-service, notification-service, gateway-service) had solid microservices architecture — polyglot persistence (PostgreSQL, MongoDB, Redis), event-driven messaging via RabbitMQ, a GraphQL gateway — but the database layer relied entirely on Spring Boot defaults. Specifically:

- **No database migrations.** Hibernate's `ddl-auto: update` generated the schema on startup. This is a red flag in production: no audit trail, no rollback path, risk of schema drift between environments.
- **No explicit indexes.** The only optimization was `unique=true` on the email field and two `JOIN FETCH` queries to avoid N+1 on project-owner lookups.
- **No caching.** Every request hit the database, even for data that changes infrequently.
- **No connection pool tuning.** HikariCP ran with Spring Boot defaults (pool size 10, no leak detection).
- **No complex queries.** The entire SQL surface was CRUD — no aggregations, no window functions, no CTEs.

The target job description requires demonstrating three specific competencies:

1. "Ensure the application is optimized for speed and scalability"
2. "Design efficient database schemas and write optimized SQL queries"
3. "Perform database tuning and optimization for performance"

The existing codebase showed competent structure but no evidence of *deliberate* performance work. An interviewer reading the code would see Spring Boot defaults, not optimization.

## Decision

Add an analytics query layer that *requires* database optimization to perform well, then layer in each optimization as a natural consequence. This analytics-first approach means every optimization has a clear reason — we're not adding indexes to CRUD queries that don't need them.

### 1. Analytics Endpoints (task-service)

Two new REST endpoints behind `AnalyticsController`:

**Project Dashboard Stats** (`GET /api/analytics/projects/{id}/stats`): Task counts by status/priority, overdue count, average completion time, member workload distribution. SQL techniques: `GROUP BY`, `COUNT(*) FILTER (WHERE ...)`, `AVG()` with date arithmetic, `JOIN` across tasks/users.

**Velocity Metrics** (`GET /api/analytics/projects/{id}/velocity?weeks=8`): Weekly throughput (created vs completed), average lead time, lead time percentiles (p50/p75/p95). SQL techniques: CTEs with `generate_series` for time-series bucketing, `PERCENTILE_CONT` window function, `COALESCE` for null handling.

Both endpoints use `NamedParameterJdbcTemplate` (not JPA) for full SQL control over complex queries that don't map naturally to entity-based repositories.

### 2. Flyway Migrations

Replaced `ddl-auto: update` with Flyway versioned migrations:

- `V1__baseline_schema.sql` — Explicit CREATE TABLE for all 6 tables (users, projects, project_members, tasks, password_reset_tokens, refresh_tokens). Uses `baseline-on-migrate: true` for existing databases.
- `V2__add_completed_at.sql` — Added `completed_at` timestamp to tasks, set automatically when status changes to `DONE`, cleared when reopened.
- `V3__analytics_indexes.sql` — Compound and partial indexes (see below).

Hibernate switched to `ddl-auto: validate` — it validates entities match the schema but never modifies it. Flyway owns schema changes.

### 3. Index Strategy

Five indexes targeting the analytics queries, defined in a Flyway migration:

| Index | Columns | Type | Purpose |
|---|---|---|---|
| `idx_tasks_project_status` | `(project_id, status)` | Compound | Dashboard GROUP BY status |
| `idx_tasks_project_priority` | `(project_id, priority)` | Compound | Dashboard GROUP BY priority |
| `idx_tasks_project_assignee` | `(project_id, assignee_id)` | Compound | Member workload query |
| `idx_tasks_project_completed_at` | `(project_id, completed_at)` WHERE `completed_at IS NOT NULL` | Partial | Velocity — only indexes completed tasks |
| `idx_tasks_overdue` | `(project_id, due_date)` WHERE `status != 'DONE' AND due_date IS NOT NULL` | Partial | Overdue count — only indexes open tasks |

Partial indexes are smaller and faster because they exclude irrelevant rows. The overdue index only covers open tasks; the velocity index only covers completed ones.

### 4. Redis Caching

Added `spring-boot-starter-cache` and `spring-boot-starter-data-redis` to task-service. Cache configuration:

- `@Cacheable("project-stats", key = "#projectId")` on dashboard stats — 5 minute TTL
- `@Cacheable("project-velocity", key = "#projectId + '-' + #weeks")` on velocity — 15 minute TTL (changes less frequently)
- `@CacheEvict` on all task mutation methods (`createTask`, `updateTask`, `deleteTask`) — invalidates both caches when data changes

The gateway service also caches the cross-service `projectHealth` rollup with a 10 minute TTL.

All analytics DTOs implement `Serializable` for JDK serialization. We initially tried `GenericJackson2JsonRedisSerializer` with `DefaultTyping.NON_FINAL`, but this fails on Java records (final classes) and immutable collections — simplified to default JDK serialization which handles any `Serializable` object.

### 5. HikariCP Connection Pool Tuning

Explicit configuration replacing Spring Boot defaults:

```yaml
hikari:
  maximum-pool-size: 5        # (core_count * 2) + spindle_count; single-core K8s pod + SSD
  minimum-idle: 2
  connection-timeout: 10000   # 10s — fail fast
  idle-timeout: 300000        # 5min — release idle connections
  max-lifetime: 1200000       # 20min — rotate before PostgreSQL's default timeout
  leak-detection-threshold: 30000  # 30s — warn if connection held too long
```

Pool size 5 (not default 10) because the K8s pod has 200m-500m CPU. Smaller pools actually outperform larger ones under contention — fewer context switches, less lock contention.

### 6. MongoDB Aggregation Pipeline (activity-service)

New `GET /api/activity/project/{projectId}/stats` endpoint using `MongoTemplate.aggregate()` with pipeline stages (`$match`, `$group`, `$sort`, `$project`). Pushes computation to the database instead of pulling raw documents and aggregating in Java.

Added compound indexes on `{projectId: 1, timestamp: -1}` and `{taskId: 1, timestamp: -1}` via `@CompoundIndex` annotations.

### 7. GraphQL Gateway Cross-Service Rollup

Extended the GraphQL schema with `projectStats`, `projectVelocity`, and `projectHealth` queries. The `projectHealth` resolver aggregates data from both task-service (PostgreSQL analytics) and activity-service (MongoDB aggregation) into a single response, cached at the gateway level.

### 8. Testing

- Unit tests: `AnalyticsServiceTest` (Mockito), `AnalyticsControllerTest` (WebMvcTest)
- Integration test: `analyticsEndpoints_returnStats()` using Testcontainers (PostgreSQL + Redis + RabbitMQ) — creates a project with tasks, hits analytics endpoints, verifies aggregated results

## CI/CD Issues Encountered and Resolved

Several issues surfaced during CI that couldn't be caught locally (no JDK on Mac):

1. **`TypeMismatchDataAccessException`** — `JdbcClient.single()` doesn't handle nullable `RowMapper` returns correctly. Replaced with `NamedParameterJdbcTemplate` which has battle-tested null handling via `queryForObject`.

2. **`SerializationException`** — `GenericJackson2JsonRedisSerializer` with `DefaultTyping.NON_FINAL` can't serialize Java records (which are final classes) or `Map.ofEntries()` results (immutable internal types). Simplified to default JDK serialization and made all DTOs implement `Serializable`.

3. **E2E mock pattern collision** — The Playwright mock `**/ingest**` was matching `/ingestion/documents` because "ingestion" starts with "ingest". This silently intercepted the document list GET with the upload mock response, making `documents` undefined. Fixed by using exact paths: `**/ingestion/ingest` and `**/ingestion/documents`.

4. **Gradle CI debugging** — Added `--stacktrace` to all `./gradlew test` and `./gradlew integrationTest` commands in `java-ci.yml` for better failure diagnostics.

5. **Local E2E preflight** — Added `make preflight-e2e` target that runs mocked Playwright tests locally. This catches runtime rendering errors that `tsc` can't detect (because `fetch().json()` returns `any`, bypassing the type system). Added to CLAUDE.md as a required preflight for frontend changes.

## Consequences

**Positive:**
- The Java codebase now demonstrates deliberate database optimization work — not just Spring Boot defaults
- Every optimization exists because a real query needs it, making it easy to explain in interviews
- Flyway migrations signal production maturity (versioned schema changes with audit trail)
- The ADR notebook (`docs/adr/java-task-management/07_analytics_and_optimization.md`) walks through each decision with code examples and Go/TypeScript comparisons
- CI debugging is faster with `--stacktrace` and test report uploads
- `make preflight-e2e` catches a class of bugs that static analysis misses

**Trade-offs:**
- Analytics DTOs must implement `Serializable` — minor boilerplate
- `NamedParameterJdbcTemplate` is more verbose than `JdbcClient` — but more reliable for complex queries with nullable results
- Redis is now a dependency for task-service (was only used by notification-service) — acceptable since Redis was already in the stack
- `make preflight-e2e` adds ~3-5 seconds to local preflight — worth it for the bugs it catches
