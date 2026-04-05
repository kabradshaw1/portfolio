# Java Analytics & Database Optimization Design

**Date:** 2026-04-05
**Status:** Approved
**Author:** Kyle Bradshaw + Claude

## Context

The Java task-management package has solid microservices architecture (4 services, polyglot persistence, event-driven messaging) but the database layer relies entirely on Spring Boot defaults. There are no explicit indexes, no database migrations, no caching strategy, and no queries more complex than simple CRUD with two JOIN FETCH optimizations.

This work adds an analytics query layer that *requires* database optimization to perform well, then layers in Flyway migrations, compound/partial indexes, Redis caching, HikariCP tuning, and MongoDB aggregation pipelines. The goal is to demonstrate three job description requirements:

1. "Ensure the application is optimized for speed and scalability"
2. "Design efficient database schemas and write optimized SQL queries"
3. "Perform database tuning and optimization for performance"

## Approach

Analytics-first: build the query layer, then optimize it. Each optimization has a clear reason rather than being retrofitted onto CRUD queries that don't need it.

---

## 1. Analytics Endpoints (task-service)

### 1.1 Project Dashboard Stats

**Endpoint:** `GET /api/analytics/projects/{id}/stats`

**Response:**
```json
{
  "taskCountByStatus": { "TODO": 5, "IN_PROGRESS": 3, "DONE": 12 },
  "taskCountByPriority": { "LOW": 4, "MEDIUM": 8, "HIGH": 8 },
  "overdueCount": 2,
  "avgCompletionTimeHours": 48.5,
  "memberWorkload": [
    { "userId": "...", "name": "...", "assignedCount": 4, "completedCount": 7 }
  ]
}
```

**SQL techniques:** GROUP BY with CASE expressions, COUNT/AVG aggregations, LEFT JOINs across tasks/users/project_members, date arithmetic (`completed_at - created_at`).

**New classes:**
- `AnalyticsController` — REST endpoint
- `AnalyticsService` — business logic, cache annotations
- `AnalyticsRepository` — simple aggregations via `@Query(nativeQuery = true)` on a Spring Data interface; complex queries (window functions, CTEs, percentiles) via `JdbcTemplate` for full SQL control

### 1.2 Velocity Metrics

**Endpoint:** `GET /api/analytics/projects/{id}/velocity?weeks=8`

**Response:**
```json
{
  "weeklyThroughput": [
    { "week": "2026-W14", "completed": 5, "created": 8 },
    { "week": "2026-W13", "completed": 3, "created": 4 }
  ],
  "avgLeadTimeHours": 36.2,
  "leadTimePercentiles": { "p50": 24.0, "p75": 48.0, "p95": 120.0 }
}
```

**SQL techniques:** Window functions (`DATE_TRUNC`, `PERCENTILE_CONT`), CTEs for weekly bucketing, time-series aggregation.

### 1.3 Schema Change

Add `completed_at` timestamp column to `tasks` table. Set automatically when status changes to `DONE` (in `TaskService.updateTask()` or `TaskService.assignTask()` where status transitions occur).

---

## 2. Flyway Migrations (task-service)

Replace `ddl-auto: update` with Flyway versioned migrations.

### Migration files

Located in `task-service/src/main/resources/db/migration/`:

| Migration | Purpose |
|---|---|
| `V1__baseline_schema.sql` | Create `users`, `projects`, `project_members`, `tasks` tables — existing schema, now explicitly defined |
| `V2__add_completed_at.sql` | Add `completed_at` timestamp to `tasks` |
| `V3__analytics_indexes.sql` | Compound and partial indexes for analytics queries |

### Index Strategy (V3)

| Index | Columns | Type | Purpose |
|---|---|---|---|
| `idx_tasks_project_status` | `(project_id, status)` | Compound | Dashboard GROUP BY status |
| `idx_tasks_project_priority` | `(project_id, priority)` | Compound | Dashboard GROUP BY priority |
| `idx_tasks_project_assignee` | `(project_id, assignee_id)` | Compound | Workload query |
| `idx_tasks_project_completed_at` | `(project_id, completed_at)` WHERE `completed_at IS NOT NULL` | Partial | Velocity queries — only indexes completed tasks |
| `idx_tasks_due_date` | `(project_id, due_date)` WHERE `status != 'DONE'` | Partial | Overdue count — only indexes open tasks |

**Why partial indexes:** Smaller, faster, and demonstrate understanding that not all rows need indexing.

### Config Changes

- `spring.jpa.hibernate.ddl-auto`: `update` → `validate`
- Add `spring.flyway.enabled: true`
- Add `org.flywaydb:flyway-core` and `org.flywaydb:flyway-database-postgresql` to `build.gradle`

---

## 3. Redis Caching (task-service + gateway)

### Dependencies

Add to task-service `build.gradle`:
- `spring-boot-starter-cache`
- `spring-boot-starter-data-redis`

### Configuration

New `CacheConfig` class with `@EnableCaching`:
- `RedisCacheManager` bean with per-cache TTL configuration
- Jackson JSON serialization for inspectable cached values

### Cache Strategy

| Endpoint | Cache name | TTL | Eviction trigger |
|---|---|---|---|
| Project dashboard stats | `project-stats` | 5 minutes | Task created/updated/deleted in that project |
| Velocity metrics | `project-velocity` | 15 minutes | Task status changed to DONE |
| Cross-service rollup | `project-health` | 10 minutes | Any event from RabbitMQ for that project |

### Annotations

- `@Cacheable("project-stats", key = "#projectId")` on `AnalyticsService.getProjectStats()`
- `@Cacheable("project-velocity", key = "#projectId + '-' + #weeks")` on `AnalyticsService.getVelocityMetrics()`
- `@CacheEvict` on task mutation methods in `TaskService` — invalidate relevant project's cache entries on write

### Gateway Caching

The `projectHealth` cross-service rollup is cached in the gateway using the same Redis instance. Gateway already has network access to Redis (same K8s namespace).

---

## 4. HikariCP Tuning (task-service)

Explicit configuration in `application.yml`:

```yaml
spring:
  datasource:
    hikari:
      maximum-pool-size: 5
      minimum-idle: 2
      connection-timeout: 10000    # 10s - fail fast
      idle-timeout: 300000         # 5min - release idle connections
      max-lifetime: 1200000        # 20min - rotate before DB-side timeout
      leak-detection-threshold: 30000  # 30s - log warning if connection held too long
```

**Pool size rationale:** Formula `connections = (core_count * 2) + effective_spindle_count`. Single-core K8s pod (200m-500m CPU) + SSD (spindle_count = 0) = ~2-4. Size 5 gives headroom. Smaller pools outperform larger ones under contention.

**Leak detection:** Analytics queries are most likely to hold connections longer than expected. 30s threshold logs warnings without killing connections.

**Test profile:** `application-test.yml` with `maximum-pool-size: 2`.

---

## 5. MongoDB Aggregation Pipeline (activity-service)

### New Endpoint

`GET /api/activities/project/{projectId}/stats`

**Response:**
```json
{
  "totalEvents": 142,
  "eventCountByType": { "task.created": 20, "task.status_changed": 85, "task.assigned": 37 },
  "commentCount": 24,
  "activeContributors": 5,
  "weeklyActivity": [
    { "week": "2026-W14", "events": 32, "comments": 6 },
    { "week": "2026-W13", "events": 28, "comments": 4 }
  ]
}
```

### Implementation

Custom repository using `MongoTemplate.aggregate()` with pipeline stages:
- `$match` — filter by projectId and date range
- `$group` — bucket by event type and ISO week
- `$sort` — order by week descending
- `$project` — shape the output

### MongoDB Indexes

- `{ projectId: 1, timestamp: -1 }` — covers match + sort for project activity queries
- `{ taskId: 1, timestamp: -1 }` — covers task-level activity lookups (already queried but unindexed)

Created via `@CompoundIndex` annotations on the `ActivityEvent` document class.

**Why aggregation pipeline over Java-side processing:** Pushes computation to the database, avoiding transfer of potentially thousands of raw events over the network.

---

## 6. GraphQL Gateway Analytics

### Schema Additions

```graphql
type ProjectStats {
  taskCountByStatus: TaskStatusCounts!
  taskCountByPriority: TaskPriorityCounts!
  overdueCount: Int!
  avgCompletionTimeHours: Float
  memberWorkload: [MemberWorkload!]!
}

type TaskStatusCounts {
  todo: Int!
  inProgress: Int!
  done: Int!
}

type TaskPriorityCounts {
  low: Int!
  medium: Int!
  high: Int!
}

type MemberWorkload {
  userId: ID!
  name: String!
  assignedCount: Int!
  completedCount: Int!
}

type WeeklyThroughput {
  week: String!
  completed: Int!
  created: Int!
}

type Percentiles {
  p50: Float!
  p75: Float!
  p95: Float!
}

type VelocityMetrics {
  weeklyThroughput: [WeeklyThroughput!]!
  avgLeadTimeHours: Float
  leadTimePercentiles: Percentiles!
}

type ActivityStats {
  totalEvents: Int!
  eventCountByType: [EventTypeCount!]!
  commentCount: Int!
  activeContributors: Int!
  weeklyActivity: [WeeklyActivity!]!
}

type EventTypeCount {
  eventType: String!
  count: Int!
}

type WeeklyActivity {
  week: String!
  events: Int!
  comments: Int!
}

type ProjectHealth {
  stats: ProjectStats!
  velocity: VelocityMetrics!
  activity: ActivityStats!
}

extend type Query {
  projectStats(projectId: ID!): ProjectStats!
  projectVelocity(projectId: ID!, weeks: Int = 8): VelocityMetrics!
  projectHealth(projectId: ID!): ProjectHealth!
}
```

### Gateway Implementation

- New `AnalyticsResolver` in gateway-service — resolves the three analytics queries
- `TaskServiceClient` gets new methods: `getProjectStats()`, `getProjectVelocity()`
- `ActivityServiceClient` gets new method: `getActivityStats()`
- `projectHealth` resolver calls both clients and composes the result
- Gateway-level `@Cacheable("project-health")` on the composite query (backed by Redis)

---

## 7. ADR Notebook

New file: `docs/adr/java-task-management/07_analytics_and_optimization.md`

**Sections:**
1. Overview — why analytics queries need different optimization strategies than CRUD
2. Architecture Context — where the analytics layer fits in the microservices topology
3. Flyway Migrations — why ddl-auto is a red flag, how versioned migrations work
4. Index Design — compound indexes, partial indexes, explain-plan output (before/after)
5. Caching Strategy — why @Cacheable with Redis, TTL reasoning, eviction patterns
6. HikariCP Tuning — pool sizing formula, why smaller pools outperform larger ones
7. MongoDB Aggregation — pipeline stages vs Java-side processing, index strategy
8. Experiment — modify a TTL, add an index, run an explain plan
9. Check Your Understanding — quiz questions

Format: Markdown (consistent with existing Java ADRs `01` through `06`).

---

## 8. Testing Strategy

### Unit Tests

- `AnalyticsServiceTest` — mock repository, verify aggregation logic and cache key generation
- `AnalyticsControllerTest` — mock service, verify endpoint responses and HTTP status codes
- `CacheConfig` test — verify TTLs and serialization configuration

### Integration Tests

- `AnalyticsIntegrationTest` — Testcontainers (PostgreSQL + Redis + RabbitMQ)
  - Seed data via Flyway migrations (validates migrations work)
  - Insert tasks across statuses/priorities, some with `completed_at`
  - Hit analytics endpoints, verify aggregated numbers
  - Verify caching: call twice, assert second call hits cache
  - Verify eviction: create a task, assert cache invalidated

- `ActivityStatsIntegrationTest` — Testcontainers (MongoDB)
  - Seed activity events, run aggregation pipeline endpoint
  - Verify weekly bucketing and counts

### Index Validation

- Integration test runs `EXPLAIN ANALYZE` on analytics queries against seeded database
- Assert index scans (not sequential scans)
- Output used in ADR notebook

### Flyway Migration Tests

Flyway runs automatically in Testcontainers — broken migrations fail test startup. No separate test needed.

---

## Out of Scope

- Frontend UI for analytics
- New Kubernetes manifests (existing ones work)
- Changes to notification-service beyond existing Redis setup
- Load testing / benchmarking infrastructure

---

## Files Summary

### Modified (existing)
- `task-service/build.gradle` — add Flyway, cache, Redis dependencies
- `task-service/src/main/resources/application.yml` — ddl-auto → validate, Flyway, HikariCP, Redis cache config
- `task-service/src/main/java/.../entity/Task.java` — add `completedAt` field
- `task-service/src/main/java/.../service/TaskService.java` — set `completedAt` on status change, add `@CacheEvict`
- `activity-service/src/main/java/.../document/ActivityEvent.java` — add `@CompoundIndex` annotations
- `gateway-service/build.gradle` — add cache, Redis dependencies
- `gateway-service/src/main/resources/graphql/schema.graphqls` — analytics types and queries
- `gateway-service/src/main/resources/application.yml` — Redis cache config

### Created (new)
- `task-service/src/main/resources/db/migration/V1__baseline_schema.sql`
- `task-service/src/main/resources/db/migration/V2__add_completed_at.sql`
- `task-service/src/main/resources/db/migration/V3__analytics_indexes.sql`
- `task-service/src/main/java/.../controller/AnalyticsController.java`
- `task-service/src/main/java/.../service/AnalyticsService.java`
- `task-service/src/main/java/.../repository/AnalyticsRepository.java`
- `task-service/src/main/java/.../config/CacheConfig.java`
- `task-service/src/main/java/.../dto/ProjectStatsDto.java` (and related DTOs)
- `task-service/src/main/java/.../dto/VelocityMetricsDto.java`
- `task-service/src/test/java/.../service/AnalyticsServiceTest.java`
- `task-service/src/test/java/.../controller/AnalyticsControllerTest.java`
- `task-service/src/test/java/.../integration/AnalyticsIntegrationTest.java`
- `activity-service/src/main/java/.../repository/ActivityStatsRepository.java`
- `activity-service/src/main/java/.../service/ActivityStatsService.java`
- `activity-service/src/main/java/.../controller/ActivityStatsController.java`
- `activity-service/src/test/java/.../integration/ActivityStatsIntegrationTest.java`
- `gateway-service/src/main/java/.../resolver/AnalyticsResolver.java`
- `gateway-service/src/main/java/.../config/CacheConfig.java`
- `docs/adr/java-task-management/07_analytics_and_optimization.md`
