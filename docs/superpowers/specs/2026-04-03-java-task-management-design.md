# Java Task Management Portfolio Project — Design Spec

## Purpose

A task/project management application built as a portfolio project for a Full Stack Java Developer role. Demonstrates Spring Boot microservices, PostgreSQL, MongoDB, Redis, RabbitMQ, GraphQL, Google OAuth + JWT auth, Kubernetes (Minikube), GitHub Actions CI/CD, and Testcontainers — all technologies listed in the target job description.

The application lives within the existing Next.js portfolio site at `/java`, with the task management app at `/java/tasks`.

## Architecture Overview

Four Spring Boot microservices behind a GraphQL gateway, each owning its data store:

```
Frontend (Next.js)
    │
    ▼
gateway-service (GraphQL + JWT validation)
    │ REST
    ├── task-service → PostgreSQL
    ├── activity-service → MongoDB
    └── notification-service → Redis
         ▲
         │ RabbitMQ
    task-service ──publishes──► RabbitMQ ──► activity-service
                                         ──► notification-service
```

## Project Structure

```
java/
├── build.gradle                # Root Gradle multi-module build
├── settings.gradle             # Module declarations
├── docker-compose.yml          # PostgreSQL, MongoDB, Redis, RabbitMQ, all services
├── k8s/                        # Minikube manifests
│   ├── deployments/
│   ├── services/
│   ├── configmaps/
│   └── secrets/
├── gateway-service/
│   ├── build.gradle
│   └── src/main/java/dev/kylebradshaw/gateway/
├── task-service/
│   ├── build.gradle
│   └── src/main/java/dev/kylebradshaw/task/
├── activity-service/
│   ├── build.gradle
│   └── src/main/java/dev/kylebradshaw/activity/
└── notification-service/
    ├── build.gradle
    └── src/main/java/dev/kylebradshaw/notification/
```

Frontend additions live in the existing `frontend/` directory.

## Services

### gateway-service

**Purpose:** Single entry point for the frontend. Exposes a GraphQL API, validates JWTs, and routes requests to downstream services via REST.

**Tech:** Spring Cloud Gateway, Spring GraphQL, Spring Security (JWT validation only)

**No database.** Stateless routing and aggregation.

**GraphQL schema:**

```graphql
type Query {
  me: User
  project(id: ID!): Project
  myProjects: [Project!]!
  task(id: ID!): Task
  taskActivity(taskId: ID!): [ActivityEvent!]!
  taskComments(taskId: ID!): [Comment!]!
  myNotifications(unreadOnly: Boolean): [Notification!]!
}

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
}
```

The gateway resolves queries by making REST calls to the appropriate service and composing responses.

### task-service (PostgreSQL)

**Purpose:** Core business logic — users, projects, tasks, authentication.

**Tech:** Spring Boot, Spring Data JPA, Spring Security, PostgreSQL

**Responsibilities:**
- Google OAuth2 login: receives authorization code, exchanges with Google for user info, creates/finds user in PostgreSQL, issues JWT (access + refresh token)
- CRUD for projects and tasks
- Role-based access control (OWNER / MEMBER per project)
- Publishes events to RabbitMQ on task changes

**Database schema:**

```sql
users (
  id UUID PRIMARY KEY,
  email VARCHAR UNIQUE NOT NULL,
  name VARCHAR NOT NULL,
  avatar_url VARCHAR,
  created_at TIMESTAMP DEFAULT NOW()
)

refresh_tokens (
  id UUID PRIMARY KEY,
  user_id UUID REFERENCES users(id),
  token VARCHAR UNIQUE NOT NULL,
  expires_at TIMESTAMP NOT NULL
)

projects (
  id UUID PRIMARY KEY,
  name VARCHAR NOT NULL,
  description TEXT,
  owner_id UUID REFERENCES users(id),
  created_at TIMESTAMP DEFAULT NOW()
)

project_members (
  project_id UUID REFERENCES projects(id),
  user_id UUID REFERENCES users(id),
  role VARCHAR NOT NULL CHECK (role IN ('OWNER', 'MEMBER')),
  PRIMARY KEY (project_id, user_id)
)

tasks (
  id UUID PRIMARY KEY,
  project_id UUID REFERENCES projects(id),
  title VARCHAR NOT NULL,
  description TEXT,
  status VARCHAR NOT NULL CHECK (status IN ('TODO', 'IN_PROGRESS', 'DONE')),
  priority VARCHAR NOT NULL CHECK (priority IN ('LOW', 'MEDIUM', 'HIGH')),
  assignee_id UUID REFERENCES users(id),
  due_date DATE,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW()
)
```

### activity-service (MongoDB)

**Purpose:** Activity logs, audit trail, and comments on tasks.

**Tech:** Spring Boot, Spring Data MongoDB, RabbitMQ consumer

**Responsibilities:**
- Consumes RabbitMQ events from task-service
- Stores activity log entries as flexible documents
- REST API for querying activity history by project/task/user
- CRUD for comments on tasks

**Document schemas:**

```json
// activity_events collection
{
  "_id": "ObjectId",
  "project_id": "uuid",
  "task_id": "uuid",
  "actor_id": "uuid",
  "event_type": "TASK_CREATED | TASK_ASSIGNED | STATUS_CHANGED | COMMENT_ADDED",
  "metadata": {},
  "timestamp": "ISODate"
}

// comments collection
{
  "_id": "ObjectId",
  "task_id": "uuid",
  "author_id": "uuid",
  "body": "string",
  "created_at": "ISODate"
}
```

### notification-service (Redis)

**Purpose:** User notifications for task events.

**Tech:** Spring Boot, Spring Data Redis, RabbitMQ consumer

**Responsibilities:**
- Consumes RabbitMQ events (task assigned, status changed, etc.)
- Stores notifications per user in Redis sorted sets (scored by timestamp)
- Maintains unread count per user
- REST API for fetching and dismissing notifications

**Redis data model:**

```
notifications:{user_id}  →  sorted set (score = timestamp)
  member: JSON { id, type, message, task_id, read: false, created_at }

notification_count:{user_id}  →  integer (unread count for badge display)
```

## RabbitMQ Event Design

**Exchange:** `task.events` (topic exchange)

| Event | Routing Key | Consumed By |
|-------|-------------|-------------|
| Task created | `task.created` | activity-service, notification-service |
| Task assigned | `task.assigned` | activity-service, notification-service |
| Status changed | `task.status_changed` | activity-service, notification-service |
| Comment added | `task.comment_added` | activity-service, notification-service |
| Task deleted | `task.deleted` | activity-service |

**Queues:**
- `activity.queue` — bound to `task.*`
- `notification.queue` — bound to `task.created`, `task.assigned`, `task.status_changed`, `task.comment_added`

**Message payload:**

```json
{
  "event_id": "uuid",
  "event_type": "TASK_ASSIGNED",
  "timestamp": "ISO-8601",
  "actor_id": "uuid",
  "project_id": "uuid",
  "task_id": "uuid",
  "data": {
    "assignee_id": "uuid",
    "task_title": "Fix login bug"
  }
}
```

## Authentication Flow

### Google OAuth Login

1. User clicks "Sign in with Google" in the frontend
2. Frontend redirects to Google OAuth2 consent screen
3. Google redirects back with an authorization code
4. Frontend sends the code to gateway-service → task-service
5. task-service exchanges the code with Google for user info (email, name, avatar)
6. task-service creates or finds the user in PostgreSQL
7. task-service issues a JWT access token (15 min TTL) and refresh token (7 day TTL)
8. Frontend stores the JWT and sends it as `Authorization: Bearer <token>` on subsequent requests

### Inter-Service Auth

1. gateway-service validates the JWT on every incoming GraphQL request
2. Gateway forwards validated user context (user ID, roles) as headers to downstream services
3. Individual services trust the gateway — no re-validation needed

## Frontend

### Site-Wide Header

A persistent header across all pages containing:
- **GitHub** link → https://github.com/kabradshaw1
- **LinkedIn** link → https://www.linkedin.com/in/kyle-bradshaw-15950988/
- **Resume** link → placeholder PDF (Java-developer-specific resume to be added later)

### Navigation

- **`/` (root)** — existing landing page with a button/card linking to `/java`
- **`/java`** — portfolio landing page:
  - Bio section about Kyle
  - Project description (what the task management app is, tech stack overview)
  - Link/button to `/java/tasks`
- **`/java/tasks`** — project dashboard (user's projects)
- **`/java/tasks/[projectId]`** — Kanban board (TODO / IN_PROGRESS / DONE columns)
- **`/java/tasks/[projectId]/[taskId]`** — task detail with comments and activity timeline

### Key Components

- `SiteHeader` — persistent header with GitHub, LinkedIn, resume links
- `GoogleLoginButton` — triggers OAuth flow
- `ProjectList` — user's projects with create/delete
- `KanbanBoard` — drag-and-drop task columns
- `TaskCard` — task summary (title, assignee avatar, priority badge)
- `TaskDetail` — full task view with description, comments, activity timeline
- `NotificationBell` — icon with unread count badge, dropdown of recent notifications

### API Communication

- Apollo Client for GraphQL queries/mutations to gateway-service
- Auth token sent as Bearer header on all requests

## CI/CD Pipeline

### GitHub Actions (`java-ci.yml`)

| Stage | What Runs |
|-------|-----------|
| Lint | Checkstyle (style) + SpotBugs (bug detection) on all 4 services |
| Unit tests | JUnit 5 + Mockito per service (`./gradlew test`) |
| Integration tests | Testcontainers — real PostgreSQL, MongoDB, Redis, RabbitMQ |
| Build | `./gradlew build` — produces JARs |
| Docker | Build images for all 4 services, push to GHCR |
| Security | OWASP dependency-check |

### Testing Strategy

**Unit tests (JUnit 5 + Mockito):**
- Service layer logic with mocked repositories
- Controller layer with MockMvc
- Per-service test suites

**Integration tests (Testcontainers):**
- Repository layer against real databases
- RabbitMQ message publishing and consuming
- End-to-end service flows with real dependencies in Docker

**E2E tests (Playwright):**
- Task management UI flows in existing `frontend/e2e/` setup
- Create project, create task, assign task, add comment, check notifications

## Infrastructure

### Docker Compose (local dev)

Services:
- PostgreSQL (port 5432)
- MongoDB (port 27017)
- Redis (port 6379)
- RabbitMQ (port 5672, management UI on 15672)
- gateway-service (port 8080)
- task-service (port 8081)
- activity-service (port 8082)
- notification-service (port 8083)

### Minikube (local K8s)

Manifests in `java/k8s/` for all services and databases:
- Deployments with resource limits
- Services (ClusterIP for internal, NodePort/Ingress for gateway)
- ConfigMaps for environment configuration
- Secrets for credentials

Not part of CI — manual `kubectl apply` for local demonstration.

## Build Tool

Gradle multi-module project. Root `build.gradle` defines shared dependencies and plugins. Each service has its own `build.gradle` with service-specific dependencies.
