# Java Task Management Smoke Tests Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add production smoke tests for the Java task management system and a delete-user endpoint for test cleanup.

**Architecture:** Two parts — (1) a cascade-delete endpoint in the task service exposed through the gateway's GraphQL API, and (2) Playwright smoke tests that register a user through the browser, create a project and task, then clean up via API calls.

**Tech Stack:** Spring Boot (Java 21), Spring Data JPA, Spring GraphQL, Playwright (TypeScript)

**Spec:** `docs/superpowers/specs/2026-04-06-java-smoke-tests-design.md`

---

### Task 1: Add repository methods for cascade delete

**Files:**
- Modify: `java/task-service/src/main/java/dev/kylebradshaw/task/repository/TaskRepository.java`
- Modify: `java/task-service/src/main/java/dev/kylebradshaw/task/repository/ProjectMemberRepository.java`
- Modify: `java/task-service/src/main/java/dev/kylebradshaw/task/repository/ProjectRepository.java`

- [ ] **Step 1: Add methods to TaskRepository**

```java
// Add these imports at the top:
import org.springframework.data.jpa.repository.Modifying;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

// Add these methods to the interface:
void deleteByProjectIdIn(List<UUID> projectIds);

@Modifying
@Query("UPDATE Task t SET t.assignee = null WHERE t.assignee.id = :userId")
void clearAssigneeByUserId(@Param("userId") UUID userId);
```

- [ ] **Step 2: Add deleteByUserId to ProjectMemberRepository**

```java
// Add this method to the interface:
void deleteByUserId(UUID userId);
```

- [ ] **Step 3: Add methods to ProjectRepository**

```java
// Add this method to the interface:
List<Project> findByOwnerId(UUID ownerId);
```

- [ ] **Step 4: Verify compilation**

Run: `cd java && ./gradlew :task-service:compileJava --no-daemon`
Expected: BUILD SUCCESSFUL

- [ ] **Step 5: Commit**

```bash
git add java/task-service/src/main/java/dev/kylebradshaw/task/repository/TaskRepository.java \
        java/task-service/src/main/java/dev/kylebradshaw/task/repository/ProjectMemberRepository.java \
        java/task-service/src/main/java/dev/kylebradshaw/task/repository/ProjectRepository.java
git commit -m "feat: add repository methods for cascade user deletion"
```

---

### Task 2: Add deleteUser to AuthService

**Files:**
- Modify: `java/task-service/src/main/java/dev/kylebradshaw/task/service/AuthService.java`

- [ ] **Step 1: Add new repository dependencies to constructor**

Add `ProjectRepository`, `ProjectMemberRepository`, and `TaskRepository` as fields and constructor parameters:

```java
// New fields (add alongside existing ones):
private final ProjectRepository projectRepository;
private final ProjectMemberRepository projectMemberRepository;
private final TaskRepository taskRepository;

// Updated constructor (add 3 new params at the end):
public AuthService(UserRepository userRepository,
                   RefreshTokenRepository refreshTokenRepository,
                   JwtService jwtService,
                   PasswordEncoder passwordEncoder,
                   PasswordResetTokenRepository passwordResetTokenRepository,
                   EmailService emailService,
                   ProjectRepository projectRepository,
                   ProjectMemberRepository projectMemberRepository,
                   TaskRepository taskRepository) {
    this.userRepository = userRepository;
    this.refreshTokenRepository = refreshTokenRepository;
    this.jwtService = jwtService;
    this.passwordEncoder = passwordEncoder;
    this.passwordResetTokenRepository = passwordResetTokenRepository;
    this.emailService = emailService;
    this.projectRepository = projectRepository;
    this.projectMemberRepository = projectMemberRepository;
    this.taskRepository = taskRepository;
}
```

Add these imports:

```java
import dev.kylebradshaw.task.entity.Project;
import dev.kylebradshaw.task.repository.ProjectMemberRepository;
import dev.kylebradshaw.task.repository.ProjectRepository;
import dev.kylebradshaw.task.repository.TaskRepository;
import java.util.List;
```

- [ ] **Step 2: Add deleteUser method**

Add this method to AuthService (after the `resetPassword` method):

```java
@Transactional
public void deleteUser(UUID userId) {
    List<Project> ownedProjects = projectRepository.findByOwnerId(userId);
    List<UUID> ownedProjectIds = ownedProjects.stream().map(Project::getId).toList();

    if (!ownedProjectIds.isEmpty()) {
        taskRepository.deleteByProjectIdIn(ownedProjectIds);
    }
    taskRepository.clearAssigneeByUserId(userId);
    projectMemberRepository.deleteByUserId(userId);
    projectRepository.deleteAll(ownedProjects);
    refreshTokenRepository.deleteByUserId(userId);
    passwordResetTokenRepository.deleteByUserId(userId);
    userRepository.deleteById(userId);
}
```

- [ ] **Step 3: Verify compilation**

Run: `cd java && ./gradlew :task-service:compileJava --no-daemon`
Expected: BUILD SUCCESSFUL

- [ ] **Step 4: Commit**

```bash
git add java/task-service/src/main/java/dev/kylebradshaw/task/service/AuthService.java
git commit -m "feat: add cascade deleteUser to AuthService"
```

---

### Task 3: Add DELETE /auth/user endpoint and unit test

**Files:**
- Modify: `java/task-service/src/main/java/dev/kylebradshaw/task/controller/AuthController.java`
- Modify: `java/task-service/src/test/java/dev/kylebradshaw/task/controller/AuthControllerTest.java`

- [ ] **Step 1: Write the failing test**

Add to `AuthControllerTest.java`. First add the import for `delete`:

```java
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.delete;
```

Then add this test method:

```java
@Test
void deleteUser_returnsNoContent() throws Exception {
    UUID userId = UUID.randomUUID();
    doNothing().when(authService).deleteUser(userId);

    mockMvc.perform(delete("/auth/user")
                    .header("X-User-Id", userId.toString()))
            .andExpect(status().isNoContent());
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd java && ./gradlew :task-service:test --tests "dev.kylebradshaw.task.controller.AuthControllerTest.deleteUser_returnsNoContent" --no-daemon`
Expected: FAIL (no mapping for DELETE /auth/user)

- [ ] **Step 3: Add the endpoint to AuthController**

Add this import:

```java
import org.springframework.web.bind.annotation.DeleteMapping;
import org.springframework.web.bind.annotation.RequestHeader;
import org.springframework.http.HttpStatus;
import org.springframework.web.bind.annotation.ResponseStatus;
```

Add this method to AuthController (after the `resetPassword` method):

```java
@DeleteMapping("/user")
@ResponseStatus(HttpStatus.NO_CONTENT)
public void deleteUser(@RequestHeader("X-User-Id") UUID userId) {
    authService.deleteUser(userId);
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd java && ./gradlew :task-service:test --tests "dev.kylebradshaw.task.controller.AuthControllerTest.deleteUser_returnsNoContent" --no-daemon`
Expected: PASS

- [ ] **Step 5: Run all task-service tests**

Run: `cd java && ./gradlew :task-service:test --no-daemon`
Expected: BUILD SUCCESSFUL (all tests pass)

- [ ] **Step 6: Commit**

```bash
git add java/task-service/src/main/java/dev/kylebradshaw/task/controller/AuthController.java \
        java/task-service/src/test/java/dev/kylebradshaw/task/controller/AuthControllerTest.java
git commit -m "feat: add DELETE /auth/user endpoint with cascade delete"
```

---

### Task 4: Add deleteAccount to gateway GraphQL API

**Files:**
- Modify: `java/gateway-service/src/main/resources/graphql/schema.graphqls`
- Modify: `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/client/TaskServiceClient.java`
- Modify: `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/resolver/MutationResolver.java`

- [ ] **Step 1: Add deleteAccount to GraphQL schema**

In `schema.graphqls`, add `deleteAccount: Boolean!` to the end of the Mutation type:

```graphql
type Mutation {
    createProject(input: CreateProjectInput!): Project!
    updateProject(id: ID!, input: UpdateProjectInput!): Project!
    deleteProject(id: ID!): Boolean!
    createTask(input: CreateTaskInput!): Task!
    updateTask(id: ID!, input: UpdateTaskInput!): Task!
    deleteTask(id: ID!): Boolean!
    assignTask(taskId: ID!, userId: ID!): Task!
    addComment(taskId: ID!, body: String!): Comment!
    markNotificationRead(id: ID!): Boolean!
    markAllNotificationsRead: Boolean!
    deleteAccount: Boolean!
}
```

- [ ] **Step 2: Add deleteUser to TaskServiceClient**

Add this method to `TaskServiceClient` (after the `getProjectVelocity` method):

```java
public void deleteUser(String userId) {
    client.delete()
            .uri("/auth/user")
            .header("X-User-Id", userId)
            .retrieve()
            .toBodilessEntity();
}
```

- [ ] **Step 3: Add deleteAccount resolver to MutationResolver**

Add this method to `MutationResolver` (after the `markAllNotificationsRead` method):

```java
@MutationMapping
public Boolean deleteAccount(DataFetchingEnvironment env) {
    String userId = env.getGraphQlContext().get("userId");
    taskClient.deleteUser(userId);
    return true;
}
```

- [ ] **Step 4: Verify compilation**

Run: `cd java && ./gradlew :gateway-service:compileJava --no-daemon`
Expected: BUILD SUCCESSFUL

- [ ] **Step 5: Run all Java tests**

Run: `make preflight-java`
Expected: BUILD SUCCESSFUL (checkstyle + all unit tests pass)

- [ ] **Step 6: Commit**

```bash
git add java/gateway-service/src/main/resources/graphql/schema.graphqls \
        java/gateway-service/src/main/java/dev/kylebradshaw/gateway/client/TaskServiceClient.java \
        java/gateway-service/src/main/java/dev/kylebradshaw/gateway/resolver/MutationResolver.java
git commit -m "feat: add deleteAccount GraphQL mutation to gateway"
```

---

### Task 5: Add SecurityConfig permitAll for DELETE /auth/user

**Files:**
- Modify: `java/task-service/src/main/java/dev/kylebradshaw/task/config/SecurityConfig.java`

The `/auth/**` path is already permitted in SecurityConfig, so `DELETE /auth/user` should work without changes. However, verify this is the case.

- [ ] **Step 1: Verify /auth/** is already permitted**

Read `SecurityConfig.java` and confirm `.requestMatchers("/auth/**").permitAll()` exists. Since `DELETE /auth/user` falls under `/auth/**`, no change is needed.

- [ ] **Step 2: Skip commit (no changes needed)**

---

### Task 6: Add Playwright smoke tests for Java task management

**Files:**
- Modify: `frontend/e2e/smoke.spec.ts`

- [ ] **Step 1: Add the Java smoke test describe block**

Add this new describe block at the end of `smoke.spec.ts`, after the closing `});` of the existing "Production smoke tests" describe block:

```typescript
const GRAPHQL_URL =
  process.env.SMOKE_GRAPHQL_URL || "https://api.kylebradshaw.dev/graphql";

test.describe("Java task management smoke tests", () => {
  // Shared state for cleanup
  let accessToken: string;
  let projectId: string;
  let taskId: string;
  const testEmail = `smoke-${Date.now()}@test.com`;
  const testPassword = "SmokeTest123!";

  test("java tasks page loads", async ({ page }) => {
    await page.goto(`${FRONTEND_URL}/java/tasks`);
    await expect(
      page.locator("h1", { hasText: "Task Manager" })
    ).toBeVisible();
    await expect(page.getByPlaceholder("Email")).toBeVisible();
    await expect(page.getByPlaceholder("Password")).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Sign in" })
    ).toBeVisible();
  });

  test("register, create project and task", async ({ page, request }) => {
    // Step 1: Navigate to login page and click "Create account"
    await page.goto(`${FRONTEND_URL}/java/tasks`);
    await page.getByRole("button", { name: "Create account" }).click();

    // Step 2: Fill registration form
    await expect(
      page.locator("h1", { hasText: "Create Account" })
    ).toBeVisible();
    await page.getByPlaceholder("Name").fill("Smoke Test");
    await page.getByPlaceholder("Email").fill(testEmail);
    await page.getByPlaceholder("Password (min 8 characters)").fill(testPassword);
    await page.getByPlaceholder("Confirm password").fill(testPassword);
    await page.getByRole("button", { name: "Create account" }).click();

    // Step 3: Verify login succeeded — project list appears
    await expect(page.locator("h2", { hasText: "My Projects" })).toBeVisible({
      timeout: 10000,
    });

    // Step 4: Create a project
    await page.getByRole("button", { name: "New Project" }).click();
    await expect(
      page.locator("h3", { hasText: "New Project" })
    ).toBeVisible();
    await page.getByPlaceholder("My Project").fill("Smoke Test Project");
    await page.getByPlaceholder("Optional description").first().fill("Automated smoke test");
    await page.getByRole("button", { name: "Create" }).click();

    // Step 5: Verify project appears and click into it
    await expect(page.getByText("Smoke Test Project")).toBeVisible({
      timeout: 5000,
    });
    await page.getByText("Smoke Test Project").click();

    // Step 6: Verify project page loaded
    await expect(
      page.locator("h1", { hasText: "Smoke Test Project" })
    ).toBeVisible({ timeout: 5000 });

    // Step 7: Create a task
    await page.getByRole("button", { name: "New Task" }).click();
    await expect(
      page.locator("h3", { hasText: "New Task" })
    ).toBeVisible();
    await page.getByPlaceholder("Task title").fill("Smoke Test Task");
    await page.locator("select").selectOption("HIGH");
    await page.getByRole("button", { name: "Create" }).click();

    // Step 8: Verify task appears on the board
    await expect(page.getByText("Smoke Test Task")).toBeVisible({
      timeout: 5000,
    });

    // Step 9: Get access token and IDs for cleanup
    const loginResponse = await request.post(`${API_URL}/auth/login`, {
      data: { email: testEmail, password: testPassword },
    });
    expect(loginResponse.ok()).toBeTruthy();
    const loginData = await loginResponse.json();
    accessToken = loginData.accessToken;

    // Get project ID from the URL
    const url = page.url();
    const urlParts = url.split("/");
    projectId = urlParts[urlParts.indexOf("tasks") + 1];

    // Get task ID by querying the page
    const taskLink = page.getByText("Smoke Test Task");
    const taskHref = await taskLink.evaluate((el) => {
      const link = el.closest("a");
      return link ? link.getAttribute("href") : null;
    });
    if (taskHref) {
      const taskParts = taskHref.split("/");
      taskId = taskParts[taskParts.length - 1];
    }
  });

  test.afterAll(async ({ request }) => {
    if (!accessToken) return;

    const headers = {
      Authorization: `Bearer ${accessToken}`,
      "Content-Type": "application/json",
    };

    // Delete task
    if (taskId) {
      await request.post(GRAPHQL_URL, {
        headers,
        data: {
          query: `mutation { deleteTask(id: "${taskId}") }`,
        },
      });
    }

    // Delete project
    if (projectId) {
      await request.post(GRAPHQL_URL, {
        headers,
        data: {
          query: `mutation { deleteProject(id: "${projectId}") }`,
        },
      });
    }

    // Delete user account
    await request.post(GRAPHQL_URL, {
      headers,
      data: {
        query: `mutation { deleteAccount }`,
      },
    });
  });
});
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add frontend/e2e/smoke.spec.ts
git commit -m "feat: add Playwright smoke tests for Java task management"
```

---

### Task 7: Run preflight checks

- [ ] **Step 1: Run Java preflight**

Run: `make preflight-java`
Expected: Checkstyle + all unit tests pass

- [ ] **Step 2: Run frontend preflight**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 3: Run security preflight**

Run: `make preflight-security`
Expected: No new issues
