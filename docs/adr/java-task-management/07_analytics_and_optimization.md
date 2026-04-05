# ADR 07: Analytics Queries and Database Optimization

## Overview

CRUD endpoints (create a task, update a status, fetch by ID) hit a single row or a small set of rows. The database barely breaks a sweat. Analytics is different: it scans thousands of rows, groups them, computes aggregates, and returns a summary. Without deliberate optimization -- indexes, query design, connection pooling, caching -- these queries become the slowest endpoints in the system and put disproportionate load on the database.

This document walks through the optimization stack for the Java Task Management analytics layer: Flyway migrations for schema management, compound and partial indexes for query performance, advanced SQL techniques for time-series and percentile calculations, Redis caching to avoid re-running expensive queries, HikariCP tuning for connection efficiency, and MongoDB aggregation pipelines for activity analytics.

The job description calls for: "Ensure the application is optimized for speed and scalability", "Design efficient database schemas and write optimized SQL queries", and "Perform database tuning and optimization for performance." This ADR addresses all three.

---

## Architecture Context

```
[Frontend Dashboard]
  |
  | GET /graphql { projectStats, projectVelocity, projectActivity }
  v
[gateway-service]  (GraphQL aggregation layer)
  |                              |
  | REST: /api/analytics/...     | REST: /api/activity/stats/...
  v                              v
[task-service]              [activity-service]
  |                              |
  | PostgreSQL                   | MongoDB
  | - Compound indexes           | - Aggregation pipelines
  | - Partial indexes            | - Compound indexes
  | - PERCENTILE_CONT            | - $match, $group, $sort
  | - generate_series CTE        |
  v                              v
[Redis Cache]
  - project-stats: 5min TTL
  - project-velocity: 15min TTL
```

The gateway-service issues parallel REST calls to task-service and activity-service, then merges the results into a single GraphQL response. The task-service handles all PostgreSQL-backed analytics (status counts, priority distribution, overdue tasks, completion time, weekly throughput, lead time percentiles). The activity-service handles MongoDB-backed analytics (event counts, event type breakdowns, active contributors, weekly activity). Redis sits in front of the task-service analytics to prevent repeated expensive queries from hitting PostgreSQL.

---

## Flyway Migrations

### Why `ddl-auto: update` Is Dangerous

Hibernate's `ddl-auto: update` seems convenient -- it compares your JPA entities to the database schema and applies ALTER statements automatically. In development, it works fine. In production, it creates three serious problems:

1. **Schema drift** -- there is no record of what changed or when. If two developers make conflicting entity changes, the schema ends up in an undefined state.
2. **No rollback** -- if an ALTER fails halfway, you are left with a partially migrated schema and no way to revert.
3. **No audit trail** -- compliance and debugging both require knowing exactly what SQL ran against the database and when.

Our `application.yml` sets `ddl-auto: validate` instead:

```yaml
spring:
  jpa:
    hibernate:
      ddl-auto: validate
```

This means Hibernate checks that entities match the schema at startup but never modifies it. Schema changes are Flyway's job.

### How Flyway Works

Flyway discovers versioned SQL files in `src/main/resources/db/migration/`:

```
db/migration/
  V1__initial_schema.sql
  V2__add_completed_at.sql
  V3__analytics_indexes.sql
```

On application startup, Flyway:
1. Checks the `flyway_schema_history` table for which versions have already been applied.
2. Runs any new migrations in version order.
3. Records each migration's checksum so it can detect tampering.

Our configuration:

```yaml
spring:
  flyway:
    baseline-on-migrate: true
    baseline-version: 0
```

`baseline-on-migrate: true` tells Flyway to establish a baseline for existing databases that were not originally managed by Flyway. This avoids the "found non-empty schema without metadata table" error when adopting Flyway on a database that already has tables.

**Go/TS comparison:** In Go you'd use golang-migrate or goose with raw SQL files; Spring's Flyway integration auto-discovers migrations and validates entity/schema alignment. The migration files themselves are identical -- plain SQL -- but Go requires you to run the migration tool manually or wire it into your startup code, whereas Spring Boot auto-runs Flyway before the application context initializes.

---

## Index Design

### Compound Indexes and the Leftmost Prefix Rule

A compound index on `(project_id, status)` can satisfy queries that filter on `project_id` alone OR `project_id` + `status`, but NOT `status` alone. This is the **leftmost prefix rule**: the database can only use a compound index if the query filters on a leading prefix of the index columns.

Every analytics query in this system filters by `project_id` first, then groups by a second column. This makes compound indexes with `project_id` as the leading column ideal.

### Partial Indexes

A partial index includes only rows matching a WHERE clause. It is smaller than a full index (fewer rows to store and scan) and faster for queries that match the same condition.

### The V3 Migration

Here is `V3__analytics_indexes.sql`:

```sql
-- Compound indexes for analytics GROUP BY queries
CREATE INDEX idx_tasks_project_status ON tasks (project_id, status);
CREATE INDEX idx_tasks_project_priority ON tasks (project_id, priority);
CREATE INDEX idx_tasks_project_assignee ON tasks (project_id, assignee_id);

-- Partial index: only completed tasks (smaller, faster for velocity queries)
CREATE INDEX idx_tasks_project_completed_at ON tasks (project_id, completed_at)
    WHERE completed_at IS NOT NULL;

-- Partial index: only open tasks with due dates (for overdue count)
CREATE INDEX idx_tasks_overdue ON tasks (project_id, due_date)
    WHERE status != 'DONE' AND due_date IS NOT NULL;
```

**Why these specific indexes:**

| Index | Query It Serves | Why Not a Full Index? |
|-------|----------------|----------------------|
| `idx_tasks_project_status` | `countByStatus` -- groups tasks by status within a project | Full compound index is appropriate here -- all rows need status |
| `idx_tasks_project_priority` | `countByPriority` -- groups by priority within a project | Same reasoning |
| `idx_tasks_project_assignee` | `memberWorkload` -- groups by assignee within a project | Same reasoning |
| `idx_tasks_project_completed_at` | `weeklyThroughput`, `avgCompletionTimeHours`, `leadTimePercentiles` | Partial: only completed tasks have `completed_at`. Excluding NULL rows makes the index smaller and faster. |
| `idx_tasks_overdue` | `countOverdue` -- counts open tasks past their due date | Partial: excludes DONE tasks and tasks without due dates. On a mature project, most tasks are DONE, so this index can be 80-90% smaller than a full index. |

### EXPLAIN ANALYZE

To verify that PostgreSQL is using an index rather than scanning the entire table, run:

```sql
EXPLAIN ANALYZE
SELECT status, COUNT(*) AS cnt
FROM tasks
WHERE project_id = 'some-uuid'
GROUP BY status;
```

What you want to see:

```
HashAggregate  (cost=... rows=4)
  Group Key: status
  ->  Index Scan using idx_tasks_project_status on tasks  (cost=... rows=...)
        Index Cond: (project_id = 'some-uuid'::uuid)
Planning Time: 0.2ms
Execution Time: 0.5ms
```

What you do NOT want to see:

```
  ->  Seq Scan on tasks  (cost=... rows=...)
        Filter: (project_id = 'some-uuid'::uuid)
```

A `Seq Scan` means PostgreSQL is reading every row in the table. For small tables this might be fine (the planner may choose a sequential scan if the table fits in a few pages), but for tables with thousands of rows it indicates a missing or unused index.

**Go/TS comparison:** In Go you'd define indexes in migration files directly; JPA validates entities match the schema but doesn't own index creation -- that's Flyway's job. The index definitions are pure SQL in both ecosystems.

---

## SQL Techniques

### JdbcClient Over JPA for Analytics

The analytics queries use `JdbcClient` (Spring's modern JDBC wrapper) rather than JPA's `@Query` annotation. JPA is great for CRUD -- it maps rows to entities automatically. But analytics queries return aggregated results that do not map to any entity. `JdbcClient` gives full control over SQL without fighting the ORM.

### Conditional Aggregation with COUNT(*) FILTER

The `memberWorkload` query computes two different counts in a single pass:

```java
public List<MemberWorkloadRow> memberWorkload(UUID projectId) {
    return jdbc.sql("""
            SELECT u.id AS user_id,
                   u.name,
                   COUNT(*) FILTER (WHERE t.status != 'DONE') AS assigned_count,
                   COUNT(*) FILTER (WHERE t.status = 'DONE') AS completed_count
            FROM tasks t
            JOIN users u ON u.id = t.assignee_id
            WHERE t.project_id = :projectId
              AND t.assignee_id IS NOT NULL
            GROUP BY u.id, u.name
            ORDER BY assigned_count DESC
            """)
            .param("projectId", projectId)
            .query((rs, rowNum) -> new MemberWorkloadRow(
                    rs.getObject("user_id", UUID.class),
                    rs.getString("name"),
                    rs.getInt("assigned_count"),
                    rs.getInt("completed_count")))
            .list();
}
```

`COUNT(*) FILTER (WHERE ...)` is a PostgreSQL extension that applies a condition to each aggregate independently. Without it, you would need two separate queries or a CASE expression inside COUNT. The FILTER syntax is cleaner and the query planner can optimize it into a single table scan.

### CTE with generate_series for Time-Series Bucketing

The `weeklyThroughput` query needs to return a row for every week, even weeks with zero activity. Without `generate_series`, weeks with no completed or created tasks would simply be missing from the results -- and the frontend chart would have gaps.

```java
public List<WeeklyThroughputRow> weeklyThroughput(UUID projectId, int weeks) {
    return jdbc.sql("""
            WITH week_series AS (
                SELECT generate_series(
                    date_trunc('week', now()) - ((:weeks - 1) * INTERVAL '1 week'),
                    date_trunc('week', now()),
                    INTERVAL '1 week'
                ) AS week_start
            )
            SELECT to_char(ws.week_start, 'IYYY-"W"IW') AS week,
                   COALESCE(SUM(CASE WHEN t.completed_at >= ws.week_start
                                      AND t.completed_at < ws.week_start + INTERVAL '1 week'
                                     THEN 1 ELSE 0 END), 0) AS completed,
                   COALESCE(SUM(CASE WHEN t.created_at >= ws.week_start
                                      AND t.created_at < ws.week_start + INTERVAL '1 week'
                                     THEN 1 ELSE 0 END), 0) AS created
            FROM week_series ws
            LEFT JOIN tasks t ON t.project_id = :projectId
            GROUP BY ws.week_start
            ORDER BY ws.week_start DESC
            """)
            .param("projectId", projectId)
            .param("weeks", weeks)
            .query((rs, rowNum) -> new WeeklyThroughputRow(
                    rs.getString("week"),
                    rs.getInt("completed"),
                    rs.getInt("created")))
            .list();
}
```

How it works:
1. The **CTE** (`WITH week_series AS ...`) generates a row for each week in the range using `generate_series`. This is a virtual table of week boundaries.
2. The **LEFT JOIN** to `tasks` ensures weeks with no matching tasks still appear (with zero counts via COALESCE).
3. **`to_char(ws.week_start, 'IYYY-"W"IW')`** formats the week as ISO week notation (e.g., `2026-W14`), which the frontend uses directly as chart labels.

### PERCENTILE_CONT for Lead Time Distribution

```java
public PercentilesRow leadTimePercentiles(UUID projectId) {
    return jdbc.sql("""
            SELECT COALESCE(PERCENTILE_CONT(0.50) WITHIN GROUP
                       (ORDER BY EXTRACT(EPOCH FROM (completed_at - created_at)) / 3600), 0) AS p50,
                   COALESCE(PERCENTILE_CONT(0.75) WITHIN GROUP
                       (ORDER BY EXTRACT(EPOCH FROM (completed_at - created_at)) / 3600), 0) AS p75,
                   COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP
                       (ORDER BY EXTRACT(EPOCH FROM (completed_at - created_at)) / 3600), 0) AS p95
            FROM tasks
            WHERE project_id = :projectId
              AND completed_at IS NOT NULL
            """)
            .param("projectId", projectId)
            .query((rs, rowNum) -> new PercentilesRow(
                    rs.getDouble("p50"),
                    rs.getDouble("p75"),
                    rs.getDouble("p95")))
            .single();
}
```

`PERCENTILE_CONT(0.50)` computes the 50th percentile (median) using continuous interpolation. The `WITHIN GROUP (ORDER BY ...)` clause tells PostgreSQL which values to compute the percentile over. Here, it calculates lead time in hours (`EXTRACT(EPOCH FROM ...) / 3600`) for all completed tasks.

Why three percentiles? P50 (median) shows typical lead time. P75 shows the upper quartile -- tasks that took longer than most. P95 reveals outliers -- tasks that got stuck. Together, they tell a more useful story than a simple average, which can be skewed by a single task that sat open for months.

---

## Redis Caching

### The @Cacheable / @CacheEvict Pattern

Spring's cache abstraction intercepts method calls. When a method annotated with `@Cacheable` is called, Spring checks Redis first. If the key exists, the cached value is returned without executing the method. If not, the method runs and its return value is stored in Redis.

When a task is created, updated, or deleted, `@CacheEvict` annotations on the mutation methods clear the relevant cache entries so stale data is never served.

### TTL Strategy

```java
@Configuration
@EnableCaching
public class CacheConfig {

    @Bean
    public RedisCacheManager cacheManager(RedisConnectionFactory connectionFactory) {
        ObjectMapper mapper = new ObjectMapper();
        mapper.registerModule(new JavaTimeModule());
        mapper.activateDefaultTyping(
                LaissezFaireSubTypeValidator.instance,
                ObjectMapper.DefaultTyping.NON_FINAL,
                JsonTypeInfo.As.PROPERTY);

        var jsonSerializer = new GenericJackson2JsonRedisSerializer(mapper);
        var serializationPair =
                RedisSerializationContext.SerializationPair.fromSerializer(jsonSerializer);

        var defaultConfig = RedisCacheConfiguration.defaultCacheConfig()
                .serializeValuesWith(serializationPair)
                .entryTtl(Duration.ofMinutes(5));

        return RedisCacheManager.builder(connectionFactory)
                .cacheDefaults(defaultConfig)
                .withCacheConfiguration(
                        "project-stats", defaultConfig.entryTtl(Duration.ofMinutes(5)))
                .withCacheConfiguration(
                        "project-velocity", defaultConfig.entryTtl(Duration.ofMinutes(15)))
                .build();
    }
}
```

**Why different TTLs?**

- **`project-stats` (5 minutes):** Dashboard stats (status counts, overdue count, priority distribution) change every time a task is created, moved, or completed. A 5-minute TTL keeps the dashboard reasonably fresh. Even with cache eviction on mutations, the short TTL acts as a safety net -- if an eviction is missed (e.g., a direct database update), stale data expires quickly.

- **`project-velocity` (15 minutes):** Velocity data (weekly throughput, lead time percentiles) aggregates over weeks. A single task change barely moves the needle. The longer TTL reduces PostgreSQL load for queries that scan large time ranges.

### JSON Serialization

The `ObjectMapper` is configured with `JavaTimeModule` to handle `Instant`, `LocalDate`, and other `java.time` types that appear in analytics DTOs. Without it, Jackson would fail to serialize temporal fields. `activateDefaultTyping` embeds type information in the JSON so that `GenericJackson2JsonRedisSerializer` can deserialize values back to the correct Java types, not just `LinkedHashMap`.

**Go/TS comparison:** In Go you'd use go-redis directly with manual cache keys; Spring abstracts cache providers behind annotations so you can swap Redis for Caffeine without code changes. The abstraction means switching from Redis to an in-memory cache for tests is a one-line config change.

---

## HikariCP Tuning

### Pool Sizing

```yaml
spring:
  datasource:
    hikari:
      maximum-pool-size: 5        # (core_count * 2) + spindle_count; single-core K8s pod + SSD
      minimum-idle: 2
      connection-timeout: 10000   # 10s — fail fast rather than queue indefinitely
      idle-timeout: 300000        # 5min — release idle connections to free DB resources
      max-lifetime: 1200000       # 20min — rotate before PostgreSQL's default timeout
      leak-detection-threshold: 30000  # 30s — warn if a connection is held too long
```

The pool sizing formula comes from the HikariCP wiki:

```
connections = (core_count * 2) + effective_spindle_count
```

For a single-core Kubernetes pod with an SSD (spindle count = 0), this gives `(1 * 2) + 0 = 2`. We use 5 to provide headroom for burst traffic, but the key insight is: **the optimal pool size is far smaller than most developers expect**.

### Why Smaller Pools Outperform Larger Ones

A database connection is a thread on the PostgreSQL side. When you have 50 connections, PostgreSQL schedules 50 threads that compete for CPU, memory, and disk I/O. This creates:

1. **Context switching overhead** -- the OS spends more time switching between threads than doing actual work.
2. **Lock contention** -- more concurrent transactions means more locks held simultaneously, more waiting, more deadlock potential.
3. **Cache thrashing** -- each connection's working set competes for PostgreSQL's shared buffer cache.

With 5 connections, requests queue in HikariCP (which is fast, in-memory queuing) instead of queueing inside PostgreSQL (which involves thread scheduling and lock management). The result is higher throughput and lower latency under load.

### Leak Detection

`leak-detection-threshold: 30000` (30 seconds) logs a warning with a stack trace if a connection is checked out and not returned within 30 seconds. This catches the classic bug where a connection is acquired but never closed (e.g., a missing `try-with-resources` or a `@Transactional` method that blocks on an external call). Without this setting, leaked connections silently exhaust the pool and the application eventually hangs on connection checkout.

---

## MongoDB Aggregation

### Why Push Computation to the Database

The activity-service could fetch all activity events for a project and count them in Java. For 100 events, this works fine. For 10,000 events, you are transferring megabytes of documents over the network, deserializing them into Java objects, and looping over them in memory. MongoDB's aggregation pipeline does the same work server-side and returns only the summary.

### Pipeline Design

Here is the `countByEventType` aggregation:

```java
public List<EventTypeCountRow> countByEventType(String projectId) {
    Aggregation agg =
            Aggregation.newAggregation(
                    Aggregation.match(Criteria.where("projectId").is(projectId)),
                    Aggregation.group("eventType").count().as("count"),
                    Aggregation.sort(Sort.Direction.DESC, "count"));

    AggregationResults<Document> results =
            mongo.aggregate(agg, "activity_events", Document.class);
    return results.getMappedResults().stream()
            .map(doc -> new EventTypeCountRow(doc.getString("_id"), doc.getInteger("count")))
            .toList();
}
```

The pipeline has three stages:
1. **`$match`** -- filters to a single project. This is always first because it reduces the number of documents flowing through subsequent stages. Without it, `$group` would process every document in the collection.
2. **`$group`** -- groups by `eventType` and counts occurrences. The `_id` field of each output document is the event type.
3. **`$sort`** -- orders by count descending so the most common event types appear first.

### Weekly Activity with ISO Week Extraction

```java
public List<WeeklyActivityRow> weeklyActivity(String projectId, int weeks) {
    Instant cutoff = Instant.now().minus(weeks * 7L, ChronoUnit.DAYS);

    Aggregation agg =
            Aggregation.newAggregation(
                    Aggregation.match(
                            Criteria.where("projectId")
                                    .is(projectId)
                                    .and("timestamp")
                                    .gte(cutoff)),
                    Aggregation.project()
                            .and(isoDatePart("$isoWeek"))
                            .as("isoWeek")
                            .and(isoDatePart("$isoWeekYear"))
                            .as("isoYear"),
                    Aggregation.group("isoYear", "isoWeek").count().as("events"),
                    Aggregation.sort(Sort.Direction.DESC, "_id.isoYear", "_id.isoWeek"));

    AggregationResults<Document> results =
            mongo.aggregate(agg, "activity_events", Document.class);
    return results.getMappedResults().stream()
            .map(doc -> {
                Document id = doc.get("_id", Document.class);
                String week = String.format(
                        "%d-W%02d",
                        id.getInteger("isoYear"),
                        id.getInteger("isoWeek"));
                return new WeeklyActivityRow(week, doc.getInteger("events"), 0);
            })
            .toList();
}
```

This pipeline:
1. **`$match`** -- filters by project and timestamp (only events within the requested week range).
2. **`$project`** -- extracts ISO week number and year from the timestamp using MongoDB's date operators.
3. **`$group`** -- groups by year + week and counts events per bucket.
4. **`$sort`** -- orders chronologically (descending).

### Compound Indexes

For the MongoDB aggregation queries to be efficient, `activity_events` should have a compound index on `{projectId: 1, timestamp: -1}`. The `projectId` field narrows to a single project, and the `timestamp` descending order supports both the time-range filter and chronological sorting without a separate sort step.

**Go/TS comparison:** In Go you'd use the mongo-driver's `Aggregate()` with bson pipeline definitions; Spring Data MongoDB's `Aggregation` API provides type-safe pipeline construction. The Go approach uses raw BSON documents (`bson.D{{"$match", bson.D{{"projectId", id}}}}`) which is flexible but error-prone -- typos in field names are not caught at compile time. Spring's fluent API catches structural errors at compile time, though field name strings are still unchecked.

---

## Experiment

Try these exercises to deepen your understanding:

1. **Change the cache TTL.** In `CacheConfig.java`, change the `project-stats` TTL from 5 minutes to 1 minute:

   ```java
   .withCacheConfiguration(
           "project-stats", defaultConfig.entryTtl(Duration.ofMinutes(1)))
   ```

   Restart the task-service, then open a terminal and run:

   ```bash
   redis-cli MONITOR
   ```

   Hit the dashboard endpoint twice. The first call should show a `SET` command in Redis (cache miss). The second call (within 1 minute) should show no Redis `SET` -- only a `GET` (cache hit). Wait 1 minute, call again, and observe the `SET` indicating the cache expired and was repopulated.

2. **Run EXPLAIN ANALYZE.** Connect to PostgreSQL and run an analytics query with EXPLAIN ANALYZE:

   ```bash
   docker exec -it postgres psql -U taskuser -d taskdb
   ```

   ```sql
   EXPLAIN ANALYZE
   SELECT status, COUNT(*) AS cnt
   FROM tasks
   WHERE project_id = '<a-real-project-uuid>'
   GROUP BY status;
   ```

   Verify the output shows `Index Scan using idx_tasks_project_status` rather than `Seq Scan`. If you see a sequential scan on a table with many rows, the index may not have been created -- check that the V3 migration ran by querying `SELECT * FROM flyway_schema_history;`.

---

## Check Your Understanding

1. When should you use a partial index vs a full compound index?

2. Why does the velocity cache have a longer TTL than the dashboard stats cache?

3. What is the formula for HikariCP pool sizing, and why does a smaller pool often outperform a larger one?

4. In the MongoDB aggregation pipeline, why is `$match` placed before `$group`?

5. If you add a new task mutation endpoint (e.g., `reassignTask`), what cache annotation would you add and why?
