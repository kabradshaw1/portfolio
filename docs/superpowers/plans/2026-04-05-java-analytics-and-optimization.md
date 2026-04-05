# Java Analytics & Database Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an analytics query layer to the Java task-management services that demonstrates database optimization skills — Flyway migrations, compound/partial indexes, Redis caching, HikariCP tuning, and MongoDB aggregation pipelines.

**Architecture:** Analytics-first approach. Build new query endpoints in task-service and activity-service, expose them through the GraphQL gateway, then optimize with Flyway-managed indexes, @Cacheable Redis caching, and tuned connection pooling. Each optimization exists because a real query needs it.

**Tech Stack:** Spring Boot 3.4.4, PostgreSQL 17, Flyway, Spring Cache + Redis, HikariCP, MongoDB aggregation pipelines, Spring GraphQL

---

### Task 1: Add Flyway and Replace ddl-auto

**Files:**
- Modify: `java/task-service/build.gradle`
- Modify: `java/task-service/src/main/resources/application.yml`
- Create: `java/task-service/src/main/resources/db/migration/V1__baseline_schema.sql`

- [ ] **Step 1: Add Flyway dependencies to task-service build.gradle**

Add after the existing `runtimeOnly 'org.postgresql:postgresql'` line in `java/task-service/build.gradle`:

```gradle
    implementation 'org.flywaydb:flyway-core'
    runtimeOnly 'org.flywaydb:flyway-database-postgresql'
```

- [ ] **Step 2: Create baseline migration**

Create `java/task-service/src/main/resources/db/migration/V1__baseline_schema.sql`:

```sql
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    avatar_url VARCHAR(512),
    password_hash VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    owner_id UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS project_members (
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id),
    role VARCHAR(50) NOT NULL,
    PRIMARY KEY (project_id, user_id)
);

CREATE TABLE IF NOT EXISTS tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    status VARCHAR(50) NOT NULL DEFAULT 'TODO',
    priority VARCHAR(50) NOT NULL DEFAULT 'MEDIUM',
    assignee_id UUID REFERENCES users(id),
    due_date DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS password_reset_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    token VARCHAR(255) NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    token VARCHAR(255) NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

- [ ] **Step 3: Update application.yml to use Flyway**

In `java/task-service/src/main/resources/application.yml`, change `ddl-auto: update` to `ddl-auto: validate` and add Flyway config. The `spring:` section should become:

```yaml
spring:
  datasource:
    url: jdbc:postgresql://${POSTGRES_HOST:localhost}:5432/taskdb
    username: ${POSTGRES_USER:taskuser}
    password: ${POSTGRES_PASSWORD:taskpass}
  flyway:
    baseline-on-migrate: true
    baseline-version: 0
  jpa:
    hibernate:
      ddl-auto: validate
    open-in-view: false
    properties:
      hibernate:
        dialect: org.hibernate.dialect.PostgreSQLDialect
  rabbitmq:
    host: ${RABBITMQ_HOST:localhost}
    port: 5672
    username: ${RABBITMQ_USER:guest}
    password: ${RABBITMQ_PASSWORD:guest}
```

Note: `baseline-on-migrate: true` with `baseline-version: 0` handles existing databases that already have tables — Flyway will baseline them and only run new migrations.

- [ ] **Step 4: Verify the build compiles**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/java && ./gradlew task-service:compileJava`
Expected: BUILD SUCCESSFUL

- [ ] **Step 5: Commit**

```bash
git add java/task-service/build.gradle java/task-service/src/main/resources/application.yml java/task-service/src/main/resources/db/migration/V1__baseline_schema.sql
git commit -m "feat(java): add Flyway migrations, replace ddl-auto with validate"
```

---

### Task 2: Add completed_at Column and Set on Status Change

**Files:**
- Create: `java/task-service/src/main/resources/db/migration/V2__add_completed_at.sql`
- Modify: `java/task-service/src/main/java/dev/kylebradshaw/task/entity/Task.java`
- Modify: `java/task-service/src/main/java/dev/kylebradshaw/task/service/TaskService.java`
- Modify: `java/task-service/src/main/java/dev/kylebradshaw/task/dto/TaskResponse.java`
- Test: `java/task-service/src/test/java/dev/kylebradshaw/task/service/TaskServiceTest.java`

- [ ] **Step 1: Write the failing test**

Add to `java/task-service/src/test/java/dev/kylebradshaw/task/service/TaskServiceTest.java`:

```java
@Test
void updateTask_setsCompletedAtWhenDone() {
    UUID userId = UUID.randomUUID();
    User owner = new User("test@example.com", "Owner", null);
    Project project = new Project("Project", "Desc", owner);
    Task task = new Task(project, "Task", "Desc", TaskPriority.MEDIUM, null);
    UUID taskId = UUID.randomUUID();

    when(taskRepo.findById(taskId)).thenReturn(Optional.of(task));
    when(taskRepo.save(any(Task.class))).thenAnswer(inv -> inv.getArgument(0));

    var request = new UpdateTaskRequest(null, null, TaskStatus.DONE, null, null);
    Task result = service.updateTask(taskId, request, userId);

    assertThat(result.getCompletedAt()).isNotNull();
}

@Test
void updateTask_clearsCompletedAtWhenReopened() {
    UUID userId = UUID.randomUUID();
    User owner = new User("test@example.com", "Owner", null);
    Project project = new Project("Project", "Desc", owner);
    Task task = new Task(project, "Task", "Desc", TaskPriority.MEDIUM, null);
    task.setStatus(TaskStatus.DONE);
    task.setCompletedAt(Instant.now());
    UUID taskId = UUID.randomUUID();

    when(taskRepo.findById(taskId)).thenReturn(Optional.of(task));
    when(taskRepo.save(any(Task.class))).thenAnswer(inv -> inv.getArgument(0));

    var request = new UpdateTaskRequest(null, null, TaskStatus.TODO, null, null);
    Task result = service.updateTask(taskId, request, userId);

    assertThat(result.getCompletedAt()).isNull();
}
```

Add `import java.time.Instant;` to the imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/java && ./gradlew task-service:test --tests "dev.kylebradshaw.task.service.TaskServiceTest.updateTask_setsCompletedAtWhenDone"`
Expected: FAIL — `getCompletedAt()` and `setCompletedAt()` do not exist on Task

- [ ] **Step 3: Create the migration**

Create `java/task-service/src/main/resources/db/migration/V2__add_completed_at.sql`:

```sql
ALTER TABLE tasks ADD COLUMN completed_at TIMESTAMPTZ;
```

- [ ] **Step 4: Add completedAt to Task entity**

Add to `java/task-service/src/main/java/dev/kylebradshaw/task/entity/Task.java`, after the `updatedAt` field (line 53):

```java
    @Column(name = "completed_at")
    private Instant completedAt;
```

Add getter and setter after the existing `getUpdatedAt()` method:

```java
    public Instant getCompletedAt() {
        return completedAt;
    }

    public void setCompletedAt(Instant completedAt) {
        this.completedAt = completedAt;
    }
```

- [ ] **Step 5: Update TaskService.updateTask to set completedAt**

In `java/task-service/src/main/java/dev/kylebradshaw/task/service/TaskService.java`, inside `updateTask()`, after `task.setStatus(request.status());` (line 57) and before `statusChanged = true;`, add:

```java
            if (request.status() == TaskStatus.DONE) {
                task.setCompletedAt(Instant.now());
            } else {
                task.setCompletedAt(null);
            }
```

Add `import java.time.Instant;` to the imports of TaskService.java. Also add `import dev.kylebradshaw.task.entity.TaskStatus;` if not already present.

- [ ] **Step 6: Update TaskResponse to include completedAt**

Change the `TaskResponse` record in `java/task-service/src/main/java/dev/kylebradshaw/task/dto/TaskResponse.java` to:

```java
public record TaskResponse(UUID id, UUID projectId, String title, String description, TaskStatus status,
                           TaskPriority priority, UUID assigneeId, String assigneeName, LocalDate dueDate,
                           Instant createdAt, Instant updatedAt, Instant completedAt) {
    public static TaskResponse from(Task task) {
        return new TaskResponse(task.getId(), task.getProject().getId(), task.getTitle(), task.getDescription(),
                task.getStatus(), task.getPriority(),
                task.getAssignee() != null ? task.getAssignee().getId() : null,
                task.getAssignee() != null ? task.getAssignee().getName() : null,
                task.getDueDate(), task.getCreatedAt(), task.getUpdatedAt(), task.getCompletedAt());
    }
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/java && ./gradlew task-service:test`
Expected: All tests PASS

- [ ] **Step 8: Commit**

```bash
git add java/task-service/src/main/resources/db/migration/V2__add_completed_at.sql java/task-service/src/main/java/dev/kylebradshaw/task/entity/Task.java java/task-service/src/main/java/dev/kylebradshaw/task/service/TaskService.java java/task-service/src/main/java/dev/kylebradshaw/task/dto/TaskResponse.java java/task-service/src/test/java/dev/kylebradshaw/task/service/TaskServiceTest.java
git commit -m "feat(java): add completed_at column with Flyway migration, set on status change to DONE"
```

---

### Task 3: Analytics DTOs and Repository (Project Dashboard Stats)

**Files:**
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/dto/ProjectStatsResponse.java`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/dto/MemberWorkloadRow.java`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/repository/AnalyticsRepository.java`
- Test: `java/task-service/src/test/java/dev/kylebradshaw/task/service/AnalyticsServiceTest.java`

- [ ] **Step 1: Create ProjectStatsResponse DTO**

Create `java/task-service/src/main/java/dev/kylebradshaw/task/dto/ProjectStatsResponse.java`:

```java
package dev.kylebradshaw.task.dto;

import java.util.List;
import java.util.Map;

public record ProjectStatsResponse(
        Map<String, Integer> taskCountByStatus,
        Map<String, Integer> taskCountByPriority,
        int overdueCount,
        Double avgCompletionTimeHours,
        List<MemberWorkloadRow> memberWorkload) {
}
```

- [ ] **Step 2: Create MemberWorkloadRow DTO**

Create `java/task-service/src/main/java/dev/kylebradshaw/task/dto/MemberWorkloadRow.java`:

```java
package dev.kylebradshaw.task.dto;

import java.util.UUID;

public record MemberWorkloadRow(UUID userId, String name, int assignedCount, int completedCount) {
}
```

- [ ] **Step 3: Create AnalyticsRepository**

Create `java/task-service/src/main/java/dev/kylebradshaw/task/repository/AnalyticsRepository.java`:

```java
package dev.kylebradshaw.task.repository;

import dev.kylebradshaw.task.dto.MemberWorkloadRow;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import org.springframework.jdbc.core.simple.JdbcClient;
import org.springframework.stereotype.Repository;

@Repository
public class AnalyticsRepository {

    private final JdbcClient jdbc;

    public AnalyticsRepository(JdbcClient jdbc) {
        this.jdbc = jdbc;
    }

    public Map<String, Integer> countByStatus(UUID projectId) {
        var rows = jdbc.sql("""
                SELECT status, COUNT(*) AS cnt
                FROM tasks
                WHERE project_id = :projectId
                GROUP BY status
                """)
                .param("projectId", projectId)
                .query((rs, rowNum) -> Map.entry(rs.getString("status"), rs.getInt("cnt")))
                .list();
        return Map.ofEntries(rows.toArray(Map.Entry[]::new));
    }

    public Map<String, Integer> countByPriority(UUID projectId) {
        var rows = jdbc.sql("""
                SELECT priority, COUNT(*) AS cnt
                FROM tasks
                WHERE project_id = :projectId
                GROUP BY priority
                """)
                .param("projectId", projectId)
                .query((rs, rowNum) -> Map.entry(rs.getString("priority"), rs.getInt("cnt")))
                .list();
        return Map.ofEntries(rows.toArray(Map.Entry[]::new));
    }

    public int countOverdue(UUID projectId) {
        return jdbc.sql("""
                SELECT COUNT(*) AS cnt
                FROM tasks
                WHERE project_id = :projectId
                  AND status != 'DONE'
                  AND due_date < CURRENT_DATE
                """)
                .param("projectId", projectId)
                .query((rs, rowNum) -> rs.getInt("cnt"))
                .single();
    }

    public Double avgCompletionTimeHours(UUID projectId) {
        return jdbc.sql("""
                SELECT AVG(EXTRACT(EPOCH FROM (completed_at - created_at)) / 3600) AS avg_hours
                FROM tasks
                WHERE project_id = :projectId
                  AND completed_at IS NOT NULL
                """)
                .param("projectId", projectId)
                .query((rs, rowNum) -> {
                    double val = rs.getDouble("avg_hours");
                    return rs.wasNull() ? null : val;
                })
                .single();
    }

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
}
```

- [ ] **Step 4: Write the AnalyticsService test**

Create `java/task-service/src/test/java/dev/kylebradshaw/task/service/AnalyticsServiceTest.java`:

```java
package dev.kylebradshaw.task.service;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.when;

import dev.kylebradshaw.task.dto.MemberWorkloadRow;
import dev.kylebradshaw.task.dto.ProjectStatsResponse;
import dev.kylebradshaw.task.repository.AnalyticsRepository;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

@ExtendWith(MockitoExtension.class)
class AnalyticsServiceTest {

    @Mock private AnalyticsRepository analyticsRepo;

    private AnalyticsService service;

    @BeforeEach
    void setUp() {
        service = new AnalyticsService(analyticsRepo);
    }

    @Test
    void getProjectStats_assemblesAllMetrics() {
        UUID projectId = UUID.randomUUID();
        when(analyticsRepo.countByStatus(projectId)).thenReturn(Map.of("TODO", 3, "DONE", 5));
        when(analyticsRepo.countByPriority(projectId)).thenReturn(Map.of("HIGH", 2, "MEDIUM", 6));
        when(analyticsRepo.countOverdue(projectId)).thenReturn(1);
        when(analyticsRepo.avgCompletionTimeHours(projectId)).thenReturn(24.5);
        when(analyticsRepo.memberWorkload(projectId)).thenReturn(List.of(
                new MemberWorkloadRow(UUID.randomUUID(), "Alice", 3, 5)));

        ProjectStatsResponse result = service.getProjectStats(projectId);

        assertThat(result.taskCountByStatus()).containsEntry("TODO", 3);
        assertThat(result.taskCountByStatus()).containsEntry("DONE", 5);
        assertThat(result.overdueCount()).isEqualTo(1);
        assertThat(result.avgCompletionTimeHours()).isEqualTo(24.5);
        assertThat(result.memberWorkload()).hasSize(1);
    }
}
```

- [ ] **Step 5: Run test to verify it fails**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/java && ./gradlew task-service:test --tests "dev.kylebradshaw.task.service.AnalyticsServiceTest"`
Expected: FAIL — `AnalyticsService` does not exist

- [ ] **Step 6: Create AnalyticsService**

Create `java/task-service/src/main/java/dev/kylebradshaw/task/service/AnalyticsService.java`:

```java
package dev.kylebradshaw.task.service;

import dev.kylebradshaw.task.dto.ProjectStatsResponse;
import dev.kylebradshaw.task.repository.AnalyticsRepository;
import java.util.UUID;
import org.springframework.stereotype.Service;

@Service
public class AnalyticsService {

    private final AnalyticsRepository analyticsRepo;

    public AnalyticsService(AnalyticsRepository analyticsRepo) {
        this.analyticsRepo = analyticsRepo;
    }

    public ProjectStatsResponse getProjectStats(UUID projectId) {
        return new ProjectStatsResponse(
                analyticsRepo.countByStatus(projectId),
                analyticsRepo.countByPriority(projectId),
                analyticsRepo.countOverdue(projectId),
                analyticsRepo.avgCompletionTimeHours(projectId),
                analyticsRepo.memberWorkload(projectId));
    }
}
```

- [ ] **Step 7: Run test to verify it passes**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/java && ./gradlew task-service:test --tests "dev.kylebradshaw.task.service.AnalyticsServiceTest"`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add java/task-service/src/main/java/dev/kylebradshaw/task/dto/ProjectStatsResponse.java java/task-service/src/main/java/dev/kylebradshaw/task/dto/MemberWorkloadRow.java java/task-service/src/main/java/dev/kylebradshaw/task/repository/AnalyticsRepository.java java/task-service/src/main/java/dev/kylebradshaw/task/service/AnalyticsService.java java/task-service/src/test/java/dev/kylebradshaw/task/service/AnalyticsServiceTest.java
git commit -m "feat(java): add project dashboard stats analytics service with SQL aggregations"
```

---

### Task 4: Velocity Metrics (Window Functions, CTEs, Percentiles)

**Files:**
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/dto/VelocityResponse.java`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/dto/WeeklyThroughputRow.java`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/dto/PercentilesRow.java`
- Modify: `java/task-service/src/main/java/dev/kylebradshaw/task/repository/AnalyticsRepository.java`
- Modify: `java/task-service/src/main/java/dev/kylebradshaw/task/service/AnalyticsService.java`
- Test: `java/task-service/src/test/java/dev/kylebradshaw/task/service/AnalyticsServiceTest.java`

- [ ] **Step 1: Create DTOs**

Create `java/task-service/src/main/java/dev/kylebradshaw/task/dto/WeeklyThroughputRow.java`:

```java
package dev.kylebradshaw.task.dto;

public record WeeklyThroughputRow(String week, int completed, int created) {
}
```

Create `java/task-service/src/main/java/dev/kylebradshaw/task/dto/PercentilesRow.java`:

```java
package dev.kylebradshaw.task.dto;

public record PercentilesRow(double p50, double p75, double p95) {
}
```

Create `java/task-service/src/main/java/dev/kylebradshaw/task/dto/VelocityResponse.java`:

```java
package dev.kylebradshaw.task.dto;

import java.util.List;

public record VelocityResponse(
        List<WeeklyThroughputRow> weeklyThroughput,
        Double avgLeadTimeHours,
        PercentilesRow leadTimePercentiles) {
}
```

- [ ] **Step 2: Write the failing test**

Add to `java/task-service/src/test/java/dev/kylebradshaw/task/service/AnalyticsServiceTest.java`:

```java
@Test
void getVelocityMetrics_assemblesAllMetrics() {
    UUID projectId = UUID.randomUUID();
    when(analyticsRepo.weeklyThroughput(projectId, 4)).thenReturn(List.of(
            new WeeklyThroughputRow("2026-W14", 5, 8)));
    when(analyticsRepo.avgCompletionTimeHours(projectId)).thenReturn(36.2);
    when(analyticsRepo.leadTimePercentiles(projectId)).thenReturn(
            new PercentilesRow(24.0, 48.0, 120.0));

    VelocityResponse result = service.getVelocityMetrics(projectId, 4);

    assertThat(result.weeklyThroughput()).hasSize(1);
    assertThat(result.weeklyThroughput().getFirst().week()).isEqualTo("2026-W14");
    assertThat(result.avgLeadTimeHours()).isEqualTo(36.2);
    assertThat(result.leadTimePercentiles().p50()).isEqualTo(24.0);
}
```

Add these imports to the test file:

```java
import dev.kylebradshaw.task.dto.VelocityResponse;
import dev.kylebradshaw.task.dto.WeeklyThroughputRow;
import dev.kylebradshaw.task.dto.PercentilesRow;
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/java && ./gradlew task-service:test --tests "dev.kylebradshaw.task.service.AnalyticsServiceTest.getVelocityMetrics_assemblesAllMetrics"`
Expected: FAIL — methods don't exist

- [ ] **Step 4: Add repository methods**

Add to `java/task-service/src/main/java/dev/kylebradshaw/task/repository/AnalyticsRepository.java`:

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

    public PercentilesRow leadTimePercentiles(UUID projectId) {
        return jdbc.sql("""
                SELECT COALESCE(PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (completed_at - created_at)) / 3600), 0) AS p50,
                       COALESCE(PERCENTILE_CONT(0.75) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (completed_at - created_at)) / 3600), 0) AS p75,
                       COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (completed_at - created_at)) / 3600), 0) AS p95
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

Add these imports to AnalyticsRepository:

```java
import dev.kylebradshaw.task.dto.WeeklyThroughputRow;
import dev.kylebradshaw.task.dto.PercentilesRow;
```

- [ ] **Step 5: Add getVelocityMetrics to AnalyticsService**

Add to `java/task-service/src/main/java/dev/kylebradshaw/task/service/AnalyticsService.java`:

```java
    public VelocityResponse getVelocityMetrics(UUID projectId, int weeks) {
        return new VelocityResponse(
                analyticsRepo.weeklyThroughput(projectId, weeks),
                analyticsRepo.avgCompletionTimeHours(projectId),
                analyticsRepo.leadTimePercentiles(projectId));
    }
```

Add import: `import dev.kylebradshaw.task.dto.VelocityResponse;`

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/java && ./gradlew task-service:test --tests "dev.kylebradshaw.task.service.AnalyticsServiceTest"`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
git add java/task-service/src/main/java/dev/kylebradshaw/task/dto/VelocityResponse.java java/task-service/src/main/java/dev/kylebradshaw/task/dto/WeeklyThroughputRow.java java/task-service/src/main/java/dev/kylebradshaw/task/dto/PercentilesRow.java java/task-service/src/main/java/dev/kylebradshaw/task/repository/AnalyticsRepository.java java/task-service/src/main/java/dev/kylebradshaw/task/service/AnalyticsService.java java/task-service/src/test/java/dev/kylebradshaw/task/service/AnalyticsServiceTest.java
git commit -m "feat(java): add velocity metrics with CTEs, window functions, and percentiles"
```

---

### Task 5: Analytics Controller and Endpoint Tests

**Files:**
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/controller/AnalyticsController.java`
- Create: `java/task-service/src/test/java/dev/kylebradshaw/task/controller/AnalyticsControllerTest.java`

- [ ] **Step 1: Write the failing controller test**

Create `java/task-service/src/test/java/dev/kylebradshaw/task/controller/AnalyticsControllerTest.java`:

```java
package dev.kylebradshaw.task.controller;

import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

import dev.kylebradshaw.task.dto.MemberWorkloadRow;
import dev.kylebradshaw.task.dto.PercentilesRow;
import dev.kylebradshaw.task.dto.ProjectStatsResponse;
import dev.kylebradshaw.task.dto.VelocityResponse;
import dev.kylebradshaw.task.dto.WeeklyThroughputRow;
import dev.kylebradshaw.task.service.AnalyticsService;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.boot.test.context.TestConfiguration;
import org.springframework.context.annotation.Bean;
import org.springframework.security.config.annotation.web.builders.HttpSecurity;
import org.springframework.security.web.SecurityFilterChain;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.test.web.servlet.MockMvc;

@WebMvcTest(AnalyticsController.class)
class AnalyticsControllerTest {

    @TestConfiguration
    static class TestSecurityConfig {
        @Bean
        public SecurityFilterChain testFilterChain(HttpSecurity http) throws Exception {
            return http.csrf(c -> c.disable())
                    .authorizeHttpRequests(a -> a.anyRequest().permitAll())
                    .build();
        }
    }

    @Autowired private MockMvc mockMvc;
    @MockitoBean private AnalyticsService analyticsService;

    @Test
    void getProjectStats_returns200() throws Exception {
        UUID projectId = UUID.randomUUID();
        var stats = new ProjectStatsResponse(
                Map.of("TODO", 3, "DONE", 5),
                Map.of("HIGH", 2),
                1, 24.5,
                List.of(new MemberWorkloadRow(UUID.randomUUID(), "Alice", 3, 5)));
        when(analyticsService.getProjectStats(projectId)).thenReturn(stats);

        mockMvc.perform(get("/api/analytics/projects/{id}/stats", projectId))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.overdueCount").value(1))
                .andExpect(jsonPath("$.avgCompletionTimeHours").value(24.5))
                .andExpect(jsonPath("$.memberWorkload[0].name").value("Alice"));
    }

    @Test
    void getVelocityMetrics_returns200() throws Exception {
        UUID projectId = UUID.randomUUID();
        var velocity = new VelocityResponse(
                List.of(new WeeklyThroughputRow("2026-W14", 5, 8)),
                36.2,
                new PercentilesRow(24.0, 48.0, 120.0));
        when(analyticsService.getVelocityMetrics(projectId, 8)).thenReturn(velocity);

        mockMvc.perform(get("/api/analytics/projects/{id}/velocity", projectId))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.weeklyThroughput[0].week").value("2026-W14"))
                .andExpect(jsonPath("$.avgLeadTimeHours").value(36.2))
                .andExpect(jsonPath("$.leadTimePercentiles.p50").value(24.0));
    }

    @Test
    void getVelocityMetrics_customWeeks() throws Exception {
        UUID projectId = UUID.randomUUID();
        var velocity = new VelocityResponse(List.of(), null, new PercentilesRow(0, 0, 0));
        when(analyticsService.getVelocityMetrics(projectId, 4)).thenReturn(velocity);

        mockMvc.perform(get("/api/analytics/projects/{id}/velocity", projectId)
                        .param("weeks", "4"))
                .andExpect(status().isOk());
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/java && ./gradlew task-service:test --tests "dev.kylebradshaw.task.controller.AnalyticsControllerTest"`
Expected: FAIL — `AnalyticsController` does not exist

- [ ] **Step 3: Create AnalyticsController**

Create `java/task-service/src/main/java/dev/kylebradshaw/task/controller/AnalyticsController.java`:

```java
package dev.kylebradshaw.task.controller;

import dev.kylebradshaw.task.dto.ProjectStatsResponse;
import dev.kylebradshaw.task.dto.VelocityResponse;
import dev.kylebradshaw.task.service.AnalyticsService;
import java.util.UUID;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/api/analytics")
public class AnalyticsController {

    private final AnalyticsService analyticsService;

    public AnalyticsController(AnalyticsService analyticsService) {
        this.analyticsService = analyticsService;
    }

    @GetMapping("/projects/{id}/stats")
    public ProjectStatsResponse getProjectStats(@PathVariable UUID id) {
        return analyticsService.getProjectStats(id);
    }

    @GetMapping("/projects/{id}/velocity")
    public VelocityResponse getVelocityMetrics(
            @PathVariable UUID id,
            @RequestParam(defaultValue = "8") int weeks) {
        return analyticsService.getVelocityMetrics(id, weeks);
    }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/java && ./gradlew task-service:test --tests "dev.kylebradshaw.task.controller.AnalyticsControllerTest"`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add java/task-service/src/main/java/dev/kylebradshaw/task/controller/AnalyticsController.java java/task-service/src/test/java/dev/kylebradshaw/task/controller/AnalyticsControllerTest.java
git commit -m "feat(java): add analytics REST endpoints for project stats and velocity"
```

---

### Task 6: Analytics Indexes (Flyway Migration)

**Files:**
- Create: `java/task-service/src/main/resources/db/migration/V3__analytics_indexes.sql`

- [ ] **Step 1: Create the index migration**

Create `java/task-service/src/main/resources/db/migration/V3__analytics_indexes.sql`:

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

- [ ] **Step 2: Verify build compiles**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/java && ./gradlew task-service:compileJava`
Expected: BUILD SUCCESSFUL

- [ ] **Step 3: Commit**

```bash
git add java/task-service/src/main/resources/db/migration/V3__analytics_indexes.sql
git commit -m "feat(java): add compound and partial indexes for analytics queries"
```

---

### Task 7: Redis Caching on Analytics

**Files:**
- Modify: `java/task-service/build.gradle`
- Modify: `java/task-service/src/main/resources/application.yml`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/config/CacheConfig.java`
- Modify: `java/task-service/src/main/java/dev/kylebradshaw/task/service/AnalyticsService.java`
- Modify: `java/task-service/src/main/java/dev/kylebradshaw/task/service/TaskService.java`

- [ ] **Step 1: Add Redis and cache dependencies**

Add to `java/task-service/build.gradle` after the existing `runtimeOnly 'org.flywaydb:flyway-database-postgresql'` line:

```gradle
    implementation 'org.springframework.boot:spring-boot-starter-cache'
    implementation 'org.springframework.boot:spring-boot-starter-data-redis'
```

- [ ] **Step 2: Add Redis config to application.yml**

Add under the `spring:` key in `java/task-service/src/main/resources/application.yml`, after the `rabbitmq:` section:

```yaml
  data:
    redis:
      host: ${REDIS_HOST:localhost}
      port: 6379
  cache:
    type: redis
```

- [ ] **Step 3: Create CacheConfig**

Create `java/task-service/src/main/java/dev/kylebradshaw/task/config/CacheConfig.java`:

```java
package dev.kylebradshaw.task.config;

import com.fasterxml.jackson.annotation.JsonTypeInfo;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.jsontype.impl.LaissezFaireSubTypeValidator;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;
import java.time.Duration;
import org.springframework.cache.annotation.EnableCaching;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.data.redis.cache.RedisCacheConfiguration;
import org.springframework.data.redis.cache.RedisCacheManager;
import org.springframework.data.redis.connection.RedisConnectionFactory;
import org.springframework.data.redis.serializer.GenericJackson2JsonRedisSerializer;
import org.springframework.data.redis.serializer.RedisSerializationContext;

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
        var serializationPair = RedisSerializationContext.SerializationPair.fromSerializer(jsonSerializer);

        var defaultConfig = RedisCacheConfiguration.defaultCacheConfig()
                .serializeValuesWith(serializationPair)
                .entryTtl(Duration.ofMinutes(5));

        return RedisCacheManager.builder(connectionFactory)
                .cacheDefaults(defaultConfig)
                .withCacheConfiguration("project-stats",
                        defaultConfig.entryTtl(Duration.ofMinutes(5)))
                .withCacheConfiguration("project-velocity",
                        defaultConfig.entryTtl(Duration.ofMinutes(15)))
                .build();
    }
}
```

- [ ] **Step 4: Add @Cacheable to AnalyticsService**

Update `java/task-service/src/main/java/dev/kylebradshaw/task/service/AnalyticsService.java`:

```java
package dev.kylebradshaw.task.service;

import dev.kylebradshaw.task.dto.ProjectStatsResponse;
import dev.kylebradshaw.task.dto.VelocityResponse;
import dev.kylebradshaw.task.repository.AnalyticsRepository;
import java.util.UUID;
import org.springframework.cache.annotation.Cacheable;
import org.springframework.stereotype.Service;

@Service
public class AnalyticsService {

    private final AnalyticsRepository analyticsRepo;

    public AnalyticsService(AnalyticsRepository analyticsRepo) {
        this.analyticsRepo = analyticsRepo;
    }

    @Cacheable(value = "project-stats", key = "#projectId")
    public ProjectStatsResponse getProjectStats(UUID projectId) {
        return new ProjectStatsResponse(
                analyticsRepo.countByStatus(projectId),
                analyticsRepo.countByPriority(projectId),
                analyticsRepo.countOverdue(projectId),
                analyticsRepo.avgCompletionTimeHours(projectId),
                analyticsRepo.memberWorkload(projectId));
    }

    @Cacheable(value = "project-velocity", key = "#projectId + '-' + #weeks")
    public VelocityResponse getVelocityMetrics(UUID projectId, int weeks) {
        return new VelocityResponse(
                analyticsRepo.weeklyThroughput(projectId, weeks),
                analyticsRepo.avgCompletionTimeHours(projectId),
                analyticsRepo.leadTimePercentiles(projectId));
    }
}
```

- [ ] **Step 5: Add @CacheEvict to TaskService**

In `java/task-service/src/main/java/dev/kylebradshaw/task/service/TaskService.java`, add imports:

```java
import org.springframework.cache.annotation.CacheEvict;
import org.springframework.cache.annotation.Caching;
```

Add `@Caching` annotations to the mutation methods. Before `createTask()`:

```java
    @Caching(evict = {
            @CacheEvict(value = "project-stats", key = "#result.project.id"),
            @CacheEvict(value = "project-velocity", allEntries = true)
    })
```

Note: `@CacheEvict` on `createTask` uses `#result.project.id` — Spring evaluates this after the method returns. For `updateTask`, `assignTask`, and `deleteTask`, the eviction must use the task's project ID from the result or must evict all entries.

Update `updateTask()` to add before it:

```java
    @Caching(evict = {
            @CacheEvict(value = "project-stats", allEntries = true),
            @CacheEvict(value = "project-velocity", allEntries = true)
    })
```

Update `deleteTask()` to add before it:

```java
    @Caching(evict = {
            @CacheEvict(value = "project-stats", allEntries = true),
            @CacheEvict(value = "project-velocity", allEntries = true)
    })
```

- [ ] **Step 6: Verify compilation**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/java && ./gradlew task-service:compileJava`
Expected: BUILD SUCCESSFUL

- [ ] **Step 7: Run unit tests (caching is transparent to mocked tests)**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/java && ./gradlew task-service:test`
Expected: All PASS

- [ ] **Step 8: Commit**

```bash
git add java/task-service/build.gradle java/task-service/src/main/resources/application.yml java/task-service/src/main/java/dev/kylebradshaw/task/config/CacheConfig.java java/task-service/src/main/java/dev/kylebradshaw/task/service/AnalyticsService.java java/task-service/src/main/java/dev/kylebradshaw/task/service/TaskService.java
git commit -m "feat(java): add Redis caching with @Cacheable on analytics, @CacheEvict on mutations"
```

---

### Task 8: HikariCP Tuning

**Files:**
- Modify: `java/task-service/src/main/resources/application.yml`

- [ ] **Step 1: Add HikariCP configuration**

In `java/task-service/src/main/resources/application.yml`, under `spring.datasource`, add the `hikari` section:

```yaml
spring:
  datasource:
    url: jdbc:postgresql://${POSTGRES_HOST:localhost}:5432/taskdb
    username: ${POSTGRES_USER:taskuser}
    password: ${POSTGRES_PASSWORD:taskpass}
    hikari:
      maximum-pool-size: 5        # (core_count * 2) + spindle_count; single-core K8s pod + SSD
      minimum-idle: 2
      connection-timeout: 10000   # 10s — fail fast rather than queue indefinitely
      idle-timeout: 300000        # 5min — release idle connections to free DB resources
      max-lifetime: 1200000       # 20min — rotate before PostgreSQL's default timeout
      leak-detection-threshold: 30000  # 30s — warn if a connection is held too long
```

- [ ] **Step 2: Verify build compiles**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/java && ./gradlew task-service:compileJava`
Expected: BUILD SUCCESSFUL

- [ ] **Step 3: Commit**

```bash
git add java/task-service/src/main/resources/application.yml
git commit -m "feat(java): add explicit HikariCP connection pool tuning with documented rationale"
```

---

### Task 9: MongoDB Aggregation Pipeline (Activity Stats)

**Files:**
- Create: `java/activity-service/src/main/java/dev/kylebradshaw/activity/dto/ActivityStatsResponse.java`
- Create: `java/activity-service/src/main/java/dev/kylebradshaw/activity/dto/WeeklyActivityRow.java`
- Create: `java/activity-service/src/main/java/dev/kylebradshaw/activity/dto/EventTypeCountRow.java`
- Create: `java/activity-service/src/main/java/dev/kylebradshaw/activity/repository/ActivityStatsRepository.java`
- Create: `java/activity-service/src/main/java/dev/kylebradshaw/activity/service/ActivityStatsService.java`
- Create: `java/activity-service/src/main/java/dev/kylebradshaw/activity/controller/ActivityStatsController.java`
- Modify: `java/activity-service/src/main/java/dev/kylebradshaw/activity/document/ActivityEvent.java`
- Test: `java/activity-service/src/test/java/dev/kylebradshaw/activity/service/ActivityStatsServiceTest.java`

- [ ] **Step 1: Create DTOs**

Create `java/activity-service/src/main/java/dev/kylebradshaw/activity/dto/EventTypeCountRow.java`:

```java
package dev.kylebradshaw.activity.dto;

public record EventTypeCountRow(String eventType, int count) {
}
```

Create `java/activity-service/src/main/java/dev/kylebradshaw/activity/dto/WeeklyActivityRow.java`:

```java
package dev.kylebradshaw.activity.dto;

public record WeeklyActivityRow(String week, int events, int comments) {
}
```

Create `java/activity-service/src/main/java/dev/kylebradshaw/activity/dto/ActivityStatsResponse.java`:

```java
package dev.kylebradshaw.activity.dto;

import java.util.List;

public record ActivityStatsResponse(
        int totalEvents,
        List<EventTypeCountRow> eventCountByType,
        int commentCount,
        int activeContributors,
        List<WeeklyActivityRow> weeklyActivity) {
}
```

- [ ] **Step 2: Add MongoDB compound indexes to ActivityEvent**

Update `java/activity-service/src/main/java/dev/kylebradshaw/activity/document/ActivityEvent.java` to add `@CompoundIndex` annotations. Add before the class declaration:

```java
import org.springframework.data.mongodb.core.index.CompoundIndex;
import org.springframework.data.mongodb.core.index.CompoundIndexes;
```

Add annotation before the `@Document` annotation:

```java
@CompoundIndexes({
    @CompoundIndex(name = "idx_project_timestamp", def = "{'projectId': 1, 'timestamp': -1}"),
    @CompoundIndex(name = "idx_task_timestamp", def = "{'taskId': 1, 'timestamp': -1}")
})
```

- [ ] **Step 3: Write the failing test**

Create `java/activity-service/src/test/java/dev/kylebradshaw/activity/service/ActivityStatsServiceTest.java`:

```java
package dev.kylebradshaw.activity.service;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.when;

import dev.kylebradshaw.activity.dto.ActivityStatsResponse;
import dev.kylebradshaw.activity.dto.EventTypeCountRow;
import dev.kylebradshaw.activity.dto.WeeklyActivityRow;
import dev.kylebradshaw.activity.repository.ActivityStatsRepository;
import java.util.List;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

@ExtendWith(MockitoExtension.class)
class ActivityStatsServiceTest {

    @Mock private ActivityStatsRepository statsRepo;

    private ActivityStatsService service;

    @BeforeEach
    void setUp() {
        service = new ActivityStatsService(statsRepo);
    }

    @Test
    void getProjectStats_assemblesAllMetrics() {
        String projectId = "proj-123";
        when(statsRepo.countEvents(projectId)).thenReturn(142);
        when(statsRepo.countByEventType(projectId)).thenReturn(List.of(
                new EventTypeCountRow("task.created", 20),
                new EventTypeCountRow("task.status_changed", 85)));
        when(statsRepo.countComments(projectId)).thenReturn(24);
        when(statsRepo.countActiveContributors(projectId)).thenReturn(5);
        when(statsRepo.weeklyActivity(projectId, 8)).thenReturn(List.of(
                new WeeklyActivityRow("2026-W14", 32, 6)));

        ActivityStatsResponse result = service.getProjectStats(projectId, 8);

        assertThat(result.totalEvents()).isEqualTo(142);
        assertThat(result.eventCountByType()).hasSize(2);
        assertThat(result.commentCount()).isEqualTo(24);
        assertThat(result.activeContributors()).isEqualTo(5);
        assertThat(result.weeklyActivity()).hasSize(1);
    }
}
```

- [ ] **Step 4: Run test to verify it fails**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/java && ./gradlew activity-service:test --tests "dev.kylebradshaw.activity.service.ActivityStatsServiceTest"`
Expected: FAIL — classes don't exist

- [ ] **Step 5: Create ActivityStatsRepository**

Create `java/activity-service/src/main/java/dev/kylebradshaw/activity/repository/ActivityStatsRepository.java`:

```java
package dev.kylebradshaw.activity.repository;

import dev.kylebradshaw.activity.dto.EventTypeCountRow;
import dev.kylebradshaw.activity.dto.WeeklyActivityRow;
import java.time.Instant;
import java.time.temporal.ChronoUnit;
import java.util.List;
import org.bson.Document;
import org.springframework.data.mongodb.core.MongoTemplate;
import org.springframework.data.mongodb.core.aggregation.Aggregation;
import org.springframework.data.mongodb.core.aggregation.AggregationResults;
import org.springframework.data.mongodb.core.query.Criteria;
import org.springframework.data.mongodb.core.query.Query;
import org.springframework.stereotype.Repository;

@Repository
public class ActivityStatsRepository {

    private final MongoTemplate mongo;

    public ActivityStatsRepository(MongoTemplate mongo) {
        this.mongo = mongo;
    }

    public int countEvents(String projectId) {
        Query query = new Query(Criteria.where("projectId").is(projectId));
        return (int) mongo.count(query, "activity_events");
    }

    public List<EventTypeCountRow> countByEventType(String projectId) {
        Aggregation agg = Aggregation.newAggregation(
                Aggregation.match(Criteria.where("projectId").is(projectId)),
                Aggregation.group("eventType").count().as("count"),
                Aggregation.sort(org.springframework.data.domain.Sort.Direction.DESC, "count"));

        AggregationResults<Document> results = mongo.aggregate(agg, "activity_events", Document.class);
        return results.getMappedResults().stream()
                .map(doc -> new EventTypeCountRow(doc.getString("_id"), doc.getInteger("count")))
                .toList();
    }

    public int countComments(String projectId) {
        // Comments don't have projectId directly — they have taskId.
        // We count all comments whose taskId appears in activity events for this project.
        Aggregation agg = Aggregation.newAggregation(
                Aggregation.match(Criteria.where("projectId").is(projectId)),
                Aggregation.group("taskId"));
        AggregationResults<Document> taskIds = mongo.aggregate(agg, "activity_events", Document.class);
        List<String> ids = taskIds.getMappedResults().stream()
                .map(doc -> doc.getString("_id"))
                .toList();
        if (ids.isEmpty()) {
            return 0;
        }
        Query query = new Query(Criteria.where("taskId").in(ids));
        return (int) mongo.count(query, "comments");
    }

    public int countActiveContributors(String projectId) {
        Aggregation agg = Aggregation.newAggregation(
                Aggregation.match(Criteria.where("projectId").is(projectId)),
                Aggregation.group("actorId"));
        return mongo.aggregate(agg, "activity_events", Document.class)
                .getMappedResults().size();
    }

    public List<WeeklyActivityRow> weeklyActivity(String projectId, int weeks) {
        Instant cutoff = Instant.now().minus(weeks * 7L, ChronoUnit.DAYS);

        Aggregation agg = Aggregation.newAggregation(
                Aggregation.match(Criteria.where("projectId").is(projectId)
                        .and("timestamp").gte(cutoff)),
                Aggregation.project()
                        .and("timestamp").extractIsoWeek().as("isoWeek")
                        .and("timestamp").extractYear().as("isoYear"),
                Aggregation.group("isoYear", "isoWeek").count().as("events"),
                Aggregation.sort(org.springframework.data.domain.Sort.Direction.DESC, "_id.isoYear", "_id.isoWeek"));

        AggregationResults<Document> results = mongo.aggregate(agg, "activity_events", Document.class);
        return results.getMappedResults().stream()
                .map(doc -> {
                    Document id = doc.get("_id", Document.class);
                    String week = String.format("%d-W%02d", id.getInteger("isoYear"), id.getInteger("isoWeek"));
                    return new WeeklyActivityRow(week, doc.getInteger("events"), 0);
                })
                .toList();
    }
}
```

- [ ] **Step 6: Create ActivityStatsService**

Create `java/activity-service/src/main/java/dev/kylebradshaw/activity/service/ActivityStatsService.java`:

```java
package dev.kylebradshaw.activity.service;

import dev.kylebradshaw.activity.dto.ActivityStatsResponse;
import dev.kylebradshaw.activity.repository.ActivityStatsRepository;
import org.springframework.stereotype.Service;

@Service
public class ActivityStatsService {

    private final ActivityStatsRepository statsRepo;

    public ActivityStatsService(ActivityStatsRepository statsRepo) {
        this.statsRepo = statsRepo;
    }

    public ActivityStatsResponse getProjectStats(String projectId, int weeks) {
        return new ActivityStatsResponse(
                statsRepo.countEvents(projectId),
                statsRepo.countByEventType(projectId),
                statsRepo.countComments(projectId),
                statsRepo.countActiveContributors(projectId),
                statsRepo.weeklyActivity(projectId, weeks));
    }
}
```

- [ ] **Step 7: Create ActivityStatsController**

Create `java/activity-service/src/main/java/dev/kylebradshaw/activity/controller/ActivityStatsController.java`:

```java
package dev.kylebradshaw.activity.controller;

import dev.kylebradshaw.activity.dto.ActivityStatsResponse;
import dev.kylebradshaw.activity.service.ActivityStatsService;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/api/activity")
public class ActivityStatsController {

    private final ActivityStatsService statsService;

    public ActivityStatsController(ActivityStatsService statsService) {
        this.statsService = statsService;
    }

    @GetMapping("/project/{projectId}/stats")
    public ActivityStatsResponse getProjectStats(
            @PathVariable String projectId,
            @RequestParam(defaultValue = "8") int weeks) {
        return statsService.getProjectStats(projectId, weeks);
    }
}
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/java && ./gradlew activity-service:test`
Expected: All PASS

- [ ] **Step 9: Commit**

```bash
git add java/activity-service/src/main/java/dev/kylebradshaw/activity/dto/ java/activity-service/src/main/java/dev/kylebradshaw/activity/repository/ActivityStatsRepository.java java/activity-service/src/main/java/dev/kylebradshaw/activity/service/ActivityStatsService.java java/activity-service/src/main/java/dev/kylebradshaw/activity/controller/ActivityStatsController.java java/activity-service/src/main/java/dev/kylebradshaw/activity/document/ActivityEvent.java java/activity-service/src/test/java/dev/kylebradshaw/activity/service/ActivityStatsServiceTest.java
git commit -m "feat(java): add MongoDB aggregation pipeline for activity stats with compound indexes"
```

---

### Task 10: GraphQL Gateway Analytics

**Files:**
- Modify: `java/gateway-service/build.gradle`
- Modify: `java/gateway-service/src/main/resources/application.yml`
- Modify: `java/gateway-service/src/main/resources/graphql/schema.graphqls`
- Create: `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/dto/ProjectStatsDto.java`
- Create: `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/dto/VelocityDto.java`
- Create: `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/dto/ActivityStatsDto.java`
- Create: `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/dto/ProjectHealthDto.java`
- Modify: `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/client/TaskServiceClient.java`
- Modify: `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/client/ActivityServiceClient.java`
- Create: `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/resolver/AnalyticsResolver.java`
- Create: `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/config/CacheConfig.java`

- [ ] **Step 1: Add cache + Redis dependencies to gateway build.gradle**

Add to `java/gateway-service/build.gradle` after the existing `implementation 'org.springframework.boot:spring-boot-starter-validation'` line:

```gradle
    implementation 'org.springframework.boot:spring-boot-starter-cache'
    implementation 'org.springframework.boot:spring-boot-starter-data-redis'
```

- [ ] **Step 2: Add Redis config to gateway application.yml**

Add under the `spring:` key in `java/gateway-service/src/main/resources/application.yml`:

```yaml
  data:
    redis:
      host: ${REDIS_HOST:localhost}
      port: 6379
  cache:
    type: redis
```

- [ ] **Step 3: Extend GraphQL schema**

Append to `java/gateway-service/src/main/resources/graphql/schema.graphqls` after the existing `input UpdateTaskInput` line:

```graphql

type TaskStatusCounts { todo: Int!, inProgress: Int!, done: Int! }
type TaskPriorityCounts { low: Int!, medium: Int!, high: Int! }
type MemberWorkload { userId: ID!, name: String!, assignedCount: Int!, completedCount: Int! }
type ProjectStats {
    taskCountByStatus: TaskStatusCounts!
    taskCountByPriority: TaskPriorityCounts!
    overdueCount: Int!
    avgCompletionTimeHours: Float
    memberWorkload: [MemberWorkload!]!
}

type WeeklyThroughput { week: String!, completed: Int!, created: Int! }
type Percentiles { p50: Float!, p75: Float!, p95: Float! }
type VelocityMetrics {
    weeklyThroughput: [WeeklyThroughput!]!
    avgLeadTimeHours: Float
    leadTimePercentiles: Percentiles!
}

type EventTypeCount { eventType: String!, count: Int! }
type WeeklyActivity { week: String!, events: Int!, comments: Int! }
type ActivityStats {
    totalEvents: Int!
    eventCountByType: [EventTypeCount!]!
    commentCount: Int!
    activeContributors: Int!
    weeklyActivity: [WeeklyActivity!]!
}

type ProjectHealth {
    stats: ProjectStats!
    velocity: VelocityMetrics!
    activity: ActivityStats!
}
```

Also add to the `type Query` block (add before the closing `}`):

```graphql
    projectStats(projectId: ID!): ProjectStats!
    projectVelocity(projectId: ID!, weeks: Int = 8): VelocityMetrics!
    projectHealth(projectId: ID!): ProjectHealth!
```

- [ ] **Step 4: Create gateway DTOs**

Create `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/dto/ProjectStatsDto.java`:

```java
package dev.kylebradshaw.gateway.dto;

import java.util.List;
import java.util.Map;

public record ProjectStatsDto(
        Map<String, Integer> taskCountByStatus,
        Map<String, Integer> taskCountByPriority,
        int overdueCount,
        Double avgCompletionTimeHours,
        List<MemberWorkloadDto> memberWorkload) {

    public record MemberWorkloadDto(String userId, String name, int assignedCount, int completedCount) {}
}
```

Create `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/dto/VelocityDto.java`:

```java
package dev.kylebradshaw.gateway.dto;

import java.util.List;

public record VelocityDto(
        List<WeeklyThroughputDto> weeklyThroughput,
        Double avgLeadTimeHours,
        PercentilesDto leadTimePercentiles) {

    public record WeeklyThroughputDto(String week, int completed, int created) {}
    public record PercentilesDto(double p50, double p75, double p95) {}
}
```

Create `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/dto/ActivityStatsDto.java`:

```java
package dev.kylebradshaw.gateway.dto;

import java.util.List;

public record ActivityStatsDto(
        int totalEvents,
        List<EventTypeCountDto> eventCountByType,
        int commentCount,
        int activeContributors,
        List<WeeklyActivityDto> weeklyActivity) {

    public record EventTypeCountDto(String eventType, int count) {}
    public record WeeklyActivityDto(String week, int events, int comments) {}
}
```

Create `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/dto/ProjectHealthDto.java`:

```java
package dev.kylebradshaw.gateway.dto;

public record ProjectHealthDto(
        ProjectStatsDto stats,
        VelocityDto velocity,
        ActivityStatsDto activity) {
}
```

- [ ] **Step 5: Add client methods**

Add to `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/client/TaskServiceClient.java`:

```java
    public ProjectStatsDto getProjectStats(String projectId) {
        return client.get()
                .uri("/analytics/projects/{id}/stats", projectId)
                .retrieve()
                .body(ProjectStatsDto.class);
    }

    public VelocityDto getProjectVelocity(String projectId, int weeks) {
        return client.get()
                .uri("/analytics/projects/{id}/velocity?weeks={weeks}", projectId, weeks)
                .retrieve()
                .body(VelocityDto.class);
    }
```

Add imports:

```java
import dev.kylebradshaw.gateway.dto.ProjectStatsDto;
import dev.kylebradshaw.gateway.dto.VelocityDto;
```

Add to `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/client/ActivityServiceClient.java`:

```java
    public ActivityStatsDto getActivityStats(String projectId, int weeks) {
        return client.get()
                .uri("/activity/project/{projectId}/stats?weeks={weeks}", projectId, weeks)
                .retrieve()
                .body(ActivityStatsDto.class);
    }
```

Add import: `import dev.kylebradshaw.gateway.dto.ActivityStatsDto;`

- [ ] **Step 6: Create gateway CacheConfig**

Create `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/config/CacheConfig.java`:

```java
package dev.kylebradshaw.gateway.config;

import com.fasterxml.jackson.annotation.JsonTypeInfo;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.jsontype.impl.LaissezFaireSubTypeValidator;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;
import java.time.Duration;
import org.springframework.cache.annotation.EnableCaching;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.data.redis.cache.RedisCacheConfiguration;
import org.springframework.data.redis.cache.RedisCacheManager;
import org.springframework.data.redis.connection.RedisConnectionFactory;
import org.springframework.data.redis.serializer.GenericJackson2JsonRedisSerializer;
import org.springframework.data.redis.serializer.RedisSerializationContext;

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
        var serializationPair = RedisSerializationContext.SerializationPair.fromSerializer(jsonSerializer);

        var defaultConfig = RedisCacheConfiguration.defaultCacheConfig()
                .serializeValuesWith(serializationPair)
                .entryTtl(Duration.ofMinutes(10));

        return RedisCacheManager.builder(connectionFactory)
                .cacheDefaults(defaultConfig)
                .withCacheConfiguration("project-health",
                        defaultConfig.entryTtl(Duration.ofMinutes(10)))
                .build();
    }
}
```

- [ ] **Step 7: Create AnalyticsResolver**

Create `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/resolver/AnalyticsResolver.java`:

```java
package dev.kylebradshaw.gateway.resolver;

import dev.kylebradshaw.gateway.client.ActivityServiceClient;
import dev.kylebradshaw.gateway.client.TaskServiceClient;
import dev.kylebradshaw.gateway.dto.ActivityStatsDto;
import dev.kylebradshaw.gateway.dto.ProjectHealthDto;
import dev.kylebradshaw.gateway.dto.ProjectStatsDto;
import dev.kylebradshaw.gateway.dto.VelocityDto;
import org.springframework.cache.annotation.Cacheable;
import org.springframework.graphql.data.method.annotation.Argument;
import org.springframework.graphql.data.method.annotation.QueryMapping;
import org.springframework.stereotype.Controller;

@Controller
public class AnalyticsResolver {

    private final TaskServiceClient taskClient;
    private final ActivityServiceClient activityClient;

    public AnalyticsResolver(TaskServiceClient taskClient, ActivityServiceClient activityClient) {
        this.taskClient = taskClient;
        this.activityClient = activityClient;
    }

    @QueryMapping
    public ProjectStatsDto projectStats(@Argument String projectId) {
        return taskClient.getProjectStats(projectId);
    }

    @QueryMapping
    public VelocityDto projectVelocity(@Argument String projectId, @Argument Integer weeks) {
        int w = weeks != null ? weeks : 8;
        return taskClient.getProjectVelocity(projectId, w);
    }

    @QueryMapping
    @Cacheable(value = "project-health", key = "#projectId")
    public ProjectHealthDto projectHealth(@Argument String projectId) {
        ProjectStatsDto stats = taskClient.getProjectStats(projectId);
        VelocityDto velocity = taskClient.getProjectVelocity(projectId, 8);
        ActivityStatsDto activity = activityClient.getActivityStats(projectId, 8);
        return new ProjectHealthDto(stats, velocity, activity);
    }
}
```

- [ ] **Step 8: Verify compilation**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/java && ./gradlew gateway-service:compileJava`
Expected: BUILD SUCCESSFUL

- [ ] **Step 9: Run all tests**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/java && ./gradlew test`
Expected: All PASS

- [ ] **Step 10: Commit**

```bash
git add java/gateway-service/
git commit -m "feat(java): add GraphQL analytics queries with cross-service health rollup and Redis caching"
```

---

### Task 11: Analytics Integration Test

**Files:**
- Modify: `java/task-service/src/test/java/dev/kylebradshaw/task/integration/TaskServiceIntegrationTest.java`

- [ ] **Step 1: Add Redis testcontainer and analytics test**

Update `java/task-service/src/test/java/dev/kylebradshaw/task/integration/TaskServiceIntegrationTest.java`. Add the Redis container and analytics test.

Add import:

```java
import org.testcontainers.containers.GenericContainer;
```

Add after the RabbitMQ container declaration:

```java
    @Container
    static GenericContainer<?> redis = new GenericContainer<>("redis:7-alpine")
            .withExposedPorts(6379);
```

Add to `configureProperties`:

```java
        registry.add("spring.data.redis.host", redis::getHost);
        registry.add("spring.data.redis.port", () -> redis.getMappedPort(6379));
```

Add test method:

```java
    @Test
    void analyticsEndpoints_returnStats() throws Exception {
        // Create a project
        var projectReq = new CreateProjectRequest("Analytics Project", "For analytics");
        String projectJson = mockMvc.perform(post("/api/projects")
                        .header("Authorization", "Bearer " + accessToken)
                        .header("X-User-Id", testUser.getId().toString())
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(objectMapper.writeValueAsString(projectReq)))
                .andExpect(status().isCreated())
                .andReturn().getResponse().getContentAsString();

        String projectId = objectMapper.readTree(projectJson).get("id").asText();

        // Create tasks with different priorities
        for (String title : List.of("Task 1", "Task 2", "Task 3")) {
            mockMvc.perform(post("/api/tasks")
                            .header("Authorization", "Bearer " + accessToken)
                            .header("X-User-Id", testUser.getId().toString())
                            .contentType(MediaType.APPLICATION_JSON)
                            .content(objectMapper.writeValueAsString(Map.of(
                                    "projectId", projectId, "title", title, "priority", "HIGH"))))
                    .andExpect(status().isCreated());
        }

        // Get project stats
        mockMvc.perform(get("/api/analytics/projects/{id}/stats", projectId)
                        .header("Authorization", "Bearer " + accessToken))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.taskCountByStatus.TODO").value(3))
                .andExpect(jsonPath("$.taskCountByPriority.HIGH").value(3))
                .andExpect(jsonPath("$.overdueCount").value(0));

        // Get velocity (may be empty but should return valid structure)
        mockMvc.perform(get("/api/analytics/projects/{id}/velocity", projectId)
                        .header("Authorization", "Bearer " + accessToken))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.weeklyThroughput").isArray());
    }
```

Add `import java.util.List;` if not already present.

- [ ] **Step 2: Add Testcontainers Redis dependency**

Add to `java/task-service/build.gradle` in the test dependencies section:

```gradle
    testRuntimeOnly 'org.testcontainers:testcontainers'
```

(Note: `GenericContainer` comes from `org.testcontainers:testcontainers` which is a transitive dependency of `org.testcontainers:postgresql`, but explicit is better for clarity.)

- [ ] **Step 3: Run integration tests**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/java && ./gradlew task-service:integrationTest`
Expected: All PASS (requires Docker)

- [ ] **Step 4: Commit**

```bash
git add java/task-service/src/test/java/dev/kylebradshaw/task/integration/TaskServiceIntegrationTest.java java/task-service/build.gradle
git commit -m "test(java): add analytics integration test with Redis + PostgreSQL testcontainers"
```

---

### Task 12: Run Full Preflight and Fix Issues

**Files:**
- Any files that need fixes from preflight

- [ ] **Step 1: Run Java preflight**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer && make preflight-java`
Expected: checkstyle + unit tests pass

- [ ] **Step 2: Fix any checkstyle or test failures**

Address any issues found in step 1.

- [ ] **Step 3: Run security preflight**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer && make preflight-security`
Expected: PASS

- [ ] **Step 4: Commit any fixes**

```bash
git add -A
git commit -m "fix(java): address preflight checkstyle and test issues"
```

---

### Task 13: ADR Notebook

**Files:**
- Create: `docs/adr/java-task-management/07_analytics_and_optimization.md`

- [ ] **Step 1: Write the ADR**

Create `docs/adr/java-task-management/07_analytics_and_optimization.md` with the following content covering all optimization decisions. The ADR should follow the existing pattern in the series (check `docs/adr/java-task-management/01_spring_boot_and_gradle.md` for format) and include these sections:

1. **Overview** — Why analytics queries need different optimization than CRUD
2. **Architecture Context** — Where analytics fits in the microservices topology (diagram: gateway → task-service analytics + activity-service aggregation)
3. **Flyway Migrations** — Why ddl-auto is dangerous in production, baseline-on-migrate strategy, migration versioning. Go/TS comparison: "In Go you'd use golang-migrate or goose; Spring's Flyway integration auto-discovers migrations on classpath"
4. **Index Design** — Compound indexes for GROUP BY, partial indexes for WHERE clauses. Show the actual index definitions. Include EXPLAIN ANALYZE output format showing Index Scan vs Seq Scan
5. **SQL Techniques** — CTEs for weekly bucketing, PERCENTILE_CONT for lead time, FILTER clause for conditional counting. Show actual queries from AnalyticsRepository
6. **Redis Caching** — @Cacheable/@CacheEvict pattern, TTL strategy (5min stats, 15min velocity), JSON serialization config. Go/TS comparison: "In Go you'd use go-redis directly; Spring abstracts cache providers behind annotations"
7. **HikariCP Tuning** — Pool sizing formula, why 5 not 10, leak detection. Include the actual YAML config
8. **MongoDB Aggregation** — Pipeline stages ($match, $group, $sort), compound indexes, why push computation to DB
9. **Experiment** — Change the `project-stats` TTL to 1 minute and observe cache behavior. Add a new compound index and run EXPLAIN ANALYZE to see the difference
10. **Check Your Understanding** — 5 quiz questions covering index selection, cache invalidation timing, pool sizing rationale, when to use partial indexes, CTE vs subquery tradeoffs

- [ ] **Step 2: Commit**

```bash
git add docs/adr/java-task-management/07_analytics_and_optimization.md
git commit -m "docs(java): add ADR notebook for analytics and database optimization"
```
