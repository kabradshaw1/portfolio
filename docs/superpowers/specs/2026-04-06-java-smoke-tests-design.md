# Java Task Management Smoke Tests

## Problem

The production smoke test suite only covers the AI/RAG features (Document Q&A). The Java task management system at `/java/tasks` has no smoke test coverage. After fixing 403/404 regressions, we need automated verification that registration, login, and task creation work after every deploy. A `DELETE /auth/user` endpoint is also needed so smoke tests can clean up test users.

## Design Decisions

- **Full browser journey**: Register, login, create project, create task — all through the UI. Tests the real user experience from first visit to task creation.
- **Fresh user per run**: Register `smoke-{timestamp}@test.com` to avoid collisions if cleanup fails.
- **Full cleanup via API**: Delete task, project, and user account after the browser test completes.
- **Single file**: Add to existing `smoke.spec.ts` in a new describe block.
- **New endpoint**: `DELETE /auth/user` — authenticated users delete themselves, cascading to all owned data.

## Part 1: Delete User Endpoint

### Task Service

**New repository methods:**

- `TaskRepository`: `deleteByProjectIdIn(List<UUID> projectIds)` and `clearAssigneeByUserId(UUID userId)` (JPQL UPDATE setting assignee to null)
- `ProjectMemberRepository`: `deleteByUserId(UUID userId)`
- `ProjectRepository`: `findByOwnerId(UUID ownerId)` and `deleteByOwnerId(UUID ownerId)`

**AuthService.deleteUser(UUID userId) — cascade order:**

1. Find all projects owned by the user (`projectRepository.findByOwnerId`)
2. Delete all tasks in those projects (`taskRepository.deleteByProjectIdIn`)
3. Unassign user from tasks in other projects (`taskRepository.clearAssigneeByUserId`)
4. Delete all project memberships (`projectMemberRepository.deleteByUserId`)
5. Delete owned projects (`projectRepository.deleteByOwnerId`)
6. Delete refresh tokens (`refreshTokenRepository.deleteByUserId` — exists)
7. Delete password reset tokens (`passwordResetTokenRepository.deleteByUserId` — exists)
8. Delete the user (`userRepository.deleteById`)

**AuthController endpoint:**

```
DELETE /auth/user
Header: X-User-Id (UUID)
Response: 204 No Content
```

### Gateway Service

**TaskServiceClient**: Add `deleteUser(String userId)` — `DELETE /auth/user` with `X-User-Id` header.

**MutationResolver**: Add `deleteAccount` mutation.

**GraphQL schema**: Add `deleteAccount: Boolean!` to the Mutation type.

### Unit Tests

- Add test to `AuthControllerTest` verifying `DELETE /auth/user` returns 204.

## Part 2: Playwright Smoke Tests

### Environment Variables (already in CI)

- `SMOKE_FRONTEND_URL` = `https://kylebradshaw.dev`
- `SMOKE_API_URL` = `https://api.kylebradshaw.dev`
- `SMOKE_GRAPHQL_URL` = `https://api.kylebradshaw.dev/graphql`

### Test 1: "java tasks page loads"

Navigate to `${FRONTEND_URL}/java/tasks`. Assert: "Task Manager" heading, email input, password input, and sign-in button are all visible.

### Test 2: "register, create project and task with cleanup"

**Browser journey (full UI — no API setup):**

1. Navigate to `/java/tasks` (sees login form)
2. Click "Create account" link
3. Fill registration form: name, email (`smoke-{timestamp}@test.com`), password (`SmokeTest123!`), confirm password
4. Submit registration
5. Assert: project list area appears (registration + auto-login succeeded)
6. Click new project button, fill name ("Smoke Test Project") and description, submit
7. Assert: project appears in list
8. Click into the project
9. Click new task button, fill title ("Smoke Test Task") and priority (HIGH), submit
10. Assert: task appears on the board

**Cleanup (API, in afterAll/finally):**

To get an access token for API cleanup, call `POST ${API_URL}/auth/login` with the test credentials, then:

- GraphQL `deleteTask(id)` with Bearer token
- GraphQL `deleteProject(id)` with Bearer token
- GraphQL `deleteAccount` with Bearer token

Cleanup runs even if assertions fail so test data doesn't accumulate.

### UI Selectors to Verify During Implementation

Exact selectors (button text, input placeholders, roles) need to be confirmed by reading:
- `TasksPageContent.tsx` — login form and "Create account" link
- `RegisterForm.tsx` — registration form fields
- `CreateProjectDialog.tsx` — project creation form
- `CreateTaskDialog.tsx` — task creation form
- `ProjectList.tsx` — project list rendering
- `KanbanBoard.tsx` — task board rendering

### Files to Modify

**Part 1 (delete endpoint):**
- `java/task-service/src/main/java/dev/kylebradshaw/task/repository/TaskRepository.java`
- `java/task-service/src/main/java/dev/kylebradshaw/task/repository/ProjectMemberRepository.java`
- `java/task-service/src/main/java/dev/kylebradshaw/task/repository/ProjectRepository.java`
- `java/task-service/src/main/java/dev/kylebradshaw/task/service/AuthService.java`
- `java/task-service/src/main/java/dev/kylebradshaw/task/controller/AuthController.java`
- `java/task-service/src/test/java/dev/kylebradshaw/task/controller/AuthControllerTest.java`
- `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/client/TaskServiceClient.java`
- `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/resolver/MutationResolver.java`
- `java/gateway-service/src/main/resources/graphql/schema.graphqls`

**Part 2 (smoke tests):**
- `frontend/e2e/smoke.spec.ts`
