# 02 — JPA, Hibernate, and PostgreSQL

## Overview

This document covers the persistence layer of the `task-service`: how Java objects map to
PostgreSQL tables, how Spring Data JPA generates queries from method names, and why this
object-relational approach was chosen over raw SQL alternatives.

If you are coming from Go (where `sqlx` or `pgx` are common) or TypeScript (where Prisma owns the
schema), the JPA approach will feel very different. The core idea is that you annotate plain Java
classes with metadata about their database representation, and Hibernate — a JPA implementation —
handles the SQL automatically.

By the end of this document you will be able to:

- Annotate a Java class to map it to a PostgreSQL table
- Understand why JPA entities require a no-arg constructor
- Explain `FetchType.LAZY` and when it matters
- Model a composite primary key using `@IdClass`
- Read a Spring Data JPA repository interface and predict the SQL it generates
- Use `@Transactional` correctly and understand what it wraps
- Map these patterns to equivalent Go and TypeScript approaches

---

## Architecture Context

The `task-service` manages the core domain: users, projects, tasks, project memberships, and
refresh tokens. Each of these is a JPA entity backed by a PostgreSQL table. The service communicates
with the database through repository interfaces that Spring Data JPA implements at runtime.

The entity relationship diagram:

```
users
  │
  ├─< projects (owner_id → users.id)
  │     │
  │     └─< tasks (project_id → projects.id, assignee_id → users.id)
  │
  ├─< project_members (project_id + user_id = composite PK)
  │
  └─< refresh_tokens (user_id → users.id)
```

JPA manages this entire graph. When you call `projectRepo.save(project)`, Hibernate generates the
`INSERT INTO projects ...` SQL. When you call `project.getOwner()` (a lazy association), Hibernate
generates a `SELECT FROM users ...` only if and when that method is called inside an active
transaction.

---

## Package Introductions

### JPA (Jakarta Persistence API)

JPA is a specification — a standard API for object-relational mapping in Java. The specification
defines the annotations (`@Entity`, `@Table`, `@Id`, `@ManyToOne`, etc.) and the core interfaces
(`EntityManager`, `EntityManagerFactory`). JPA does not include any implementation code; it is
purely contracts.

### Hibernate

Hibernate is the JPA implementation used by Spring Boot by default. It is the most mature ORM in
the Java ecosystem (pre-dating JPA itself). When you add `spring-boot-starter-data-jpa`, you get
both the JPA API and Hibernate as the runtime. Hibernate translates your annotated entities into
SQL, manages transactions, handles connection pooling (via HikariCP, also bundled automatically),
and provides a first-level cache (within one transaction, the same entity is not fetched twice).

### Spring Data JPA

Spring Data JPA sits on top of Hibernate and JPA. Its primary contribution is the repository
abstraction: you define an interface that extends `JpaRepository`, declare method signatures
following a naming convention, and Spring Data generates the implementation at startup. You write
zero SQL for standard CRUD and most query patterns.

**Alternatives considered:**

| Library | Approach | Why not chosen |
|---------|----------|---------------|
| **JDBC Template** | Raw SQL with Spring helper | Full SQL control, no ORM magic. Verbose for CRUD. Good for complex reporting queries; awkward for entity lifecycle management. |
| **jOOQ** | Type-safe SQL DSL | Generates Java classes from schema; queries look like SQL. Excellent for complex queries with compile-time safety. Higher setup cost. Free tier requires open-source license. |
| **MyBatis** | XML/annotation-mapped SQL | Explicit SQL in XML or annotations. Popular in enterprise Java. More control than JPA, less magic. No query generation from method names. |
| **R2DBC** | Reactive/non-blocking JDBC | Required for reactive stacks (WebFlux). Not compatible with standard JPA. This service uses Spring MVC (blocking), so R2DBC offers no benefit. |
| **Hibernate without Spring Data** | JPA only, manual `EntityManager` | More control over session lifecycle. Adds significant boilerplate for basic CRUD. Spring Data JPA's repository abstraction is worth the dependency. |

JPA/Hibernate + Spring Data was chosen because it is the standard Java persistence stack, handles
entity lifecycle management transparently, and reduces CRUD boilerplate to near zero. The
cost — "magic" SQL generation — is acceptable given Hibernate's maturity and Spring Boot's tooling
for logging generated SQL.

---

## Go / TypeScript Comparison

| Concern | Go (`sqlx`/`pgx`) | TypeScript (Prisma) | Java (JPA/Spring Data) |
|---------|--------------------|----------------------|------------------------|
| **Schema definition** | SQL migrations (Goose, Flyway) | `schema.prisma` file | `@Entity` annotated classes; `ddl-auto` generates DDL |
| **Query writing** | Raw SQL strings in `db.Query(...)` | Prisma Client generated methods | Spring Data method names → generated SQL |
| **Object mapping** | Manual `rows.Scan(...)` | Fully generated, type-safe | Hibernate maps `ResultSet` → entity via reflection |
| **Relationships** | Manual JOIN queries | `include: { ... }` in query | `@ManyToOne`, `@OneToMany` with lazy/eager loading |
| **Transactions** | `db.BeginTx(ctx, nil)` → `tx.Commit()` | Prisma `$transaction([...])` | `@Transactional` annotation on service methods |
| **Type safety** | `struct` fields match SQL columns | Fully generated from schema | Annotation-based; mismatches caught at startup (with `ddl-auto: validate`) |
| **Migrations** | Explicit SQL files (Goose) | `prisma migrate dev` | `ddl-auto: update` (dev) / `validate` (prod) |
| **Primary key** | `int64` or `uuid.UUID` | Prisma `@id @default(uuid())` | `@Id @GeneratedValue(strategy = UUID)` |
| **Composite key** | Composite struct as map key | `@@id([field1, field2])` | `@IdClass` with a nested `Serializable` class |
| **N+1 detection** | Manual / query logging | Prisma logging | `spring.jpa.show-sql: true` + slow query logs |

---

## Build It

### Step 1 — The `User` Entity

```java
// task-service/src/main/java/dev/kylebradshaw/task/entity/User.java
package dev.kylebradshaw.task.entity;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.GeneratedValue;
import jakarta.persistence.GenerationType;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import java.time.Instant;
import java.util.UUID;

@Entity                          // (A)
@Table(name = "users")           // (B)
public class User {

    @Id                          // (C)
    @GeneratedValue(strategy = GenerationType.UUID)  // (D)
    private UUID id;

    @Column(unique = true, nullable = false)  // (E)
    private String email;

    @Column(nullable = false)
    private String name;

    @Column(name = "avatar_url") // (F)
    private String avatarUrl;

    @Column(name = "created_at", updatable = false) // (G)
    private Instant createdAt = Instant.now();

    protected User() {}          // (H)

    public User(String email, String name, String avatarUrl) {
        this.email = email;
        this.name = name;
        this.avatarUrl = avatarUrl;
    }

    // getters and selective setters...
}
```

**Annotation (A) — `@Entity`**
Tells Hibernate that this class is a persistent entity. Hibernate will look for a corresponding
database table and manage instances of this class through the JPA `EntityManager`. Without this
annotation the class is invisible to JPA.

**Annotation (B) — `@Table(name = "users")`**
By default, JPA maps the entity class name to a table name. `User` would map to a table named
`user` (or `User` depending on naming strategy). Specifying `name = "users"` is explicit and
avoids surprises — `user` is also a reserved word in PostgreSQL, which would cause subtle errors.
Always be explicit with table names.

**Annotation (C) — `@Id`**
Marks the primary key field. Every JPA entity must have exactly one `@Id` field (or use a composite
key — covered in Step 4 with `ProjectMember`). The `@Id` field uniquely identifies an entity in
the first-level cache and in the database.

**Annotation (D) — `@GeneratedValue(strategy = GenerationType.UUID)`**
Tells Hibernate to generate a UUID value for this field when a new entity is persisted.
`GenerationType.UUID` was added in Hibernate 6 (Spring Boot 3.x). For older versions you would use
`@GeneratedValue(strategy = GenerationType.AUTO)` with a `@GenericGenerator`. The UUID is generated
by Hibernate in Java (not the database), which means it is available on the entity object
immediately after construction — before the `INSERT` hits the database.

In Go you would use `github.com/google/uuid`:

```go
type User struct {
    ID        uuid.UUID `db:"id"`
    Email     string    `db:"email"`
    CreatedAt time.Time `db:"created_at"`
}

// Manually assigned before insert:
user.ID = uuid.New()
```

**Annotation (E) — `@Column(unique = true, nullable = false)`**
These constraints are applied both at the Hibernate schema generation level (`ddl-auto: update`
creates a `UNIQUE NOT NULL` column) and validated at the ORM level. If you try to `save()` a
`User` with a duplicate email, Hibernate will throw a `DataIntegrityViolationException` wrapping
the PostgreSQL unique constraint violation.

**Annotation (F) — `@Column(name = "avatar_url")`**
By default, Hibernate maps `avatarUrl` to a column named `avatar_url` via its default
`CamelCaseToUnderscoresNamingStrategy`. The `@Column(name = ...)` here is explicit but equivalent
to the default — it is good practice to be explicit when the database column name matters.

**Annotation (G) — `@Column(name = "created_at", updatable = false)`**
`updatable = false` tells Hibernate never to include this column in `UPDATE` statements. The field
is set once at construction time (`Instant.now()`) and never changes. This is JPA's version of a
`DEFAULT now()` + trigger in SQL — though here the value is set in Java, not the database.

**Annotation (H) — The protected no-arg constructor**
This is the most confusing part of JPA for developers coming from Go or TypeScript. JPA
**requires** a no-arg constructor so Hibernate can instantiate entity objects via reflection when
reading rows from the database. Hibernate calls `User()`, then populates each field individually
using reflection (bypassing normal Java access rules).

The constructor is `protected` rather than `public` or `private`:
- `public` would allow callers to create a `User` with no data, which is a bug-prone pattern.
- `private` would prevent Hibernate from using it (reflection can sometimes bypass this, but it is
  not guaranteed and triggers warnings).
- `protected` is the convention: invisible to external callers but accessible to Hibernate's
  reflection-based instantiation.

This is why records cannot be JPA entities — they have no no-arg constructor.

### Step 2 — The `Project` Entity with a Lazy Association

```java
// task-service/src/main/java/dev/kylebradshaw/task/entity/Project.java
@Entity
@Table(name = "projects")
public class Project {

    @Id
    @GeneratedValue(strategy = GenerationType.UUID)
    private UUID id;

    @Column(nullable = false)
    private String name;

    private String description;

    @ManyToOne(fetch = FetchType.LAZY)       // (A)
    @JoinColumn(name = "owner_id", nullable = false)  // (B)
    private User owner;

    @Column(name = "created_at", updatable = false)
    private Instant createdAt = Instant.now();

    protected Project() {}

    public Project(String name, String description, User owner) {
        this.name = name;
        this.description = description;
        this.owner = owner;
    }

    // getters and setters...
}
```

**Annotation (A) — `@ManyToOne(fetch = FetchType.LAZY)`**
This declares a many-to-one relationship: many projects can have the same owner (one user). The
`fetch = FetchType.LAZY` part is critical for performance.

There are two fetch strategies:
- `FetchType.EAGER` — when you load a `Project`, Hibernate immediately executes a JOIN (or a
  second `SELECT`) to also load the associated `User`. You always pay the join cost, whether or not
  you need the owner's data.
- `FetchType.LAZY` — Hibernate loads only the `projects` row. The `owner` field is a proxy object.
  The actual `SELECT FROM users ...` only executes when you call `project.getOwner()` — and only if
  there is an active Hibernate session (i.e., inside a `@Transactional` method).

`FetchType.LAZY` is almost always the correct choice for `@ManyToOne`. It avoids hidden join costs
and lets service methods control exactly what data they load.

**The N+1 problem with lazy loading:** if you load a list of 50 projects and then call
`getOwner()` on each one outside a transaction (or without a JOIN fetch query), Hibernate executes
1 query for the projects + 50 queries for the owners = 51 queries. The solution is a JPQL `JOIN
FETCH` query, or marking the association eager for specific queries. This is an important trade-off
to be aware of.

**Annotation (B) — `@JoinColumn(name = "owner_id")`**
Specifies the foreign key column name in the `projects` table. Without this annotation, Hibernate
derives the column name from the field name and the target entity's ID column: `owner_id` by
convention. Being explicit is better.

In Go with `sqlx` you would write:

```go
type Project struct {
    ID          uuid.UUID `db:"id"`
    Name        string    `db:"name"`
    OwnerID     uuid.UUID `db:"owner_id"`  // store the FK, load User separately
    CreatedAt   time.Time `db:"created_at"`
}

// Load separately:
var owner User
db.Get(&owner, "SELECT * FROM users WHERE id = $1", project.OwnerID)
```

JPA's approach embeds the `User` object directly — Hibernate manages the FK translation.

### Step 3 — The `Task` Entity with Enums

```java
// task-service/src/main/java/dev/kylebradshaw/task/entity/Task.java
@Entity
@Table(name = "tasks")
public class Task {

    @Enumerated(EnumType.STRING)        // (A)
    @Column(nullable = false)
    private TaskStatus status = TaskStatus.TODO;

    @Enumerated(EnumType.STRING)
    @Column(nullable = false)
    private TaskPriority priority = TaskPriority.MEDIUM;

    @ManyToOne(fetch = FetchType.LAZY)
    @JoinColumn(name = "assignee_id")  // nullable — tasks may be unassigned
    private User assignee;

    @Column(name = "due_date")
    private LocalDate dueDate;

    @Column(name = "updated_at")
    private Instant updatedAt = Instant.now();

    public void setStatus(TaskStatus status) {
        this.status = status;
        this.updatedAt = Instant.now();  // (B)
    }
    // ...
}
```

**Annotation (A) — `@Enumerated(EnumType.STRING)`**
Without this annotation, Hibernate stores enums by their ordinal position (0, 1, 2...). If you
later add a value in the middle of the enum, all existing rows have the wrong meaning. `EnumType.STRING`
stores the enum constant name as a `VARCHAR`. This is always the correct choice — ordinal-based
enum storage is a footgun.

In TypeScript/Prisma you would define:

```prisma
enum TaskStatus {
  TODO
  IN_PROGRESS
  DONE
}
```

Prisma also stores enum values as strings by default in PostgreSQL (via a native `CREATE TYPE`
enum), which is consistent with `EnumType.STRING`.

**Annotation (B) — Manual `updatedAt` in setters**
Every mutating setter updates `updatedAt = Instant.now()`. This ensures the `updated_at` column
reflects the last modification time without needing a database trigger. A more sophisticated
approach would use JPA's `@PreUpdate` lifecycle callback, but the inline approach is simpler and
works correctly as long as all mutations go through the entity's setter methods.

### Step 4 — The `ProjectMember` Entity with a Composite Key

```java
// task-service/src/main/java/dev/kylebradshaw/task/entity/ProjectMember.java
@Entity
@Table(name = "project_members")
@IdClass(ProjectMember.ProjectMemberId.class)    // (A)
public class ProjectMember {

    @Id
    @Column(name = "project_id")
    private UUID projectId;

    @Id
    @Column(name = "user_id")
    private UUID userId;

    @ManyToOne(fetch = FetchType.LAZY)
    @JoinColumn(name = "project_id", insertable = false, updatable = false)  // (B)
    private Project project;

    @ManyToOne(fetch = FetchType.LAZY)
    @JoinColumn(name = "user_id", insertable = false, updatable = false)
    private User user;

    @Enumerated(EnumType.STRING)
    @Column(nullable = false)
    private ProjectRole role;

    protected ProjectMember() {}

    public ProjectMember(UUID projectId, UUID userId, ProjectRole role) {
        this.projectId = projectId;
        this.userId = userId;
        this.role = role;
    }

    // (C) The composite key class:
    public static class ProjectMemberId implements Serializable {
        private UUID projectId;
        private UUID userId;

        public ProjectMemberId() {}  // required by JPA

        public ProjectMemberId(UUID projectId, UUID userId) {
            this.projectId = projectId;
            this.userId = userId;
        }

        @Override
        public boolean equals(Object o) {
            if (this == o) return true;
            if (!(o instanceof ProjectMemberId that)) return false;  // (D)
            return Objects.equals(projectId, that.projectId)
                && Objects.equals(userId, that.userId);
        }

        @Override
        public int hashCode() {
            return Objects.hash(projectId, userId);
        }
    }
}
```

**Annotation (A) — `@IdClass(ProjectMember.ProjectMemberId.class)`**
JPA has two ways to model composite keys: `@IdClass` and `@EmbeddedId`. `@IdClass` is used here.
It requires a separate class that mirrors the `@Id` fields of the entity. The entity declares
multiple `@Id` annotations on the raw UUID fields; the `@IdClass` tells JPA which class to use for
the combined key object.

The alternative `@EmbeddedId` approach uses a single `@EmbeddedId` field in the entity containing
an `@Embeddable` key class — slightly different syntax, same result. `@IdClass` is often preferred
when you also need to query by individual key columns directly (e.g., `findByUserId`) because the
ID fields remain as plain fields on the entity.

In TypeScript/Prisma, a composite key is declared as:

```prisma
model ProjectMember {
  projectId String
  userId    String
  role      ProjectRole

  @@id([projectId, userId])
}
```

In Go with `sqlx`, composite keys require no special treatment — you just use both columns in your
`WHERE` clause:

```go
const q = "SELECT * FROM project_members WHERE project_id = $1 AND user_id = $2"
db.Get(&member, q, projectID, userID)
```

**Annotation (B) — `insertable = false, updatable = false`**
`projectId` is declared twice: once as a bare `@Id UUID projectId` field and once as the `project`
`@ManyToOne` association with `@JoinColumn(name = "project_id")`. If JPA tried to manage both
columns simultaneously it would produce duplicate column mappings and errors. The `insertable =
false, updatable = false` on the association's `@JoinColumn` tells Hibernate "this association
reads the column value from the `@Id` field — do not manage it through the association".

This pattern is the standard way to have both the raw FK value and the lazy-loaded object available
on the same entity.

**Annotation (C) — The `ProjectMemberId` inner class**
The key class must:
1. Be `Serializable` — Hibernate serializes keys for first-level cache storage
2. Have a public no-arg constructor — Hibernate instantiates it reflectively
3. Override `equals()` and `hashCode()` — required for correct identity comparison in the cache

**Annotation (D) — Java 16 pattern matching for `instanceof`**
```java
if (!(o instanceof ProjectMemberId that)) return false;
```
This is Java 16's `instanceof` pattern matching. In older Java you would write:
```java
if (!(o instanceof ProjectMemberId)) return false;
ProjectMemberId that = (ProjectMemberId) o;
```
Pattern matching combines the type check and the cast into one expression, equivalent to TypeScript's
`if (!(o instanceof ProjectMemberId)) return false;` — which TypeScript has always had.

### Step 5 — The `RefreshToken` Entity

```java
// task-service/src/main/java/dev/kylebradshaw/task/entity/RefreshToken.java
@Entity
@Table(name = "refresh_tokens")
public class RefreshToken {

    @Id
    @GeneratedValue(strategy = GenerationType.UUID)
    private UUID id;

    @ManyToOne(fetch = FetchType.LAZY)
    @JoinColumn(name = "user_id", nullable = false)
    private User user;

    @Column(unique = true, nullable = false)
    private String token;

    @Column(name = "expires_at", nullable = false)
    private Instant expiresAt;

    protected RefreshToken() {}

    public RefreshToken(User user, String token, Instant expiresAt) {
        this.user = user;
        this.token = token;
        this.expiresAt = expiresAt;
    }

    public boolean isExpired() {              // (A)
        return Instant.now().isAfter(expiresAt);
    }
}
```

**Annotation (A) — Domain logic on the entity**
`isExpired()` is business logic living on the entity. The entity knows how to determine its own
expiration state rather than delegating that check to a service or utility class. This is
appropriate — the entity has the data it needs (`expiresAt`), and the logic is tightly coupled to
that data. This is a basic application of the "rich domain model" approach.

### Step 6 — Repository Interfaces

```java
// task-service/src/main/java/dev/kylebradshaw/task/repository/UserRepository.java
public interface UserRepository extends JpaRepository<User, UUID> {
    Optional<User> findByEmail(String email);
}
```

`JpaRepository<User, UUID>` declares that this repository manages `User` entities with a `UUID`
primary key. Spring Data JPA generates the implementation at startup. The generated class implements:
- `findById(UUID id)` → `SELECT * FROM users WHERE id = ?`
- `findAll()` → `SELECT * FROM users`
- `save(User user)` → `INSERT` or `UPDATE` depending on whether the entity is new
- `deleteById(UUID id)` → `DELETE FROM users WHERE id = ?`
- And about 15 more standard CRUD methods

`findByEmail(String email)` is a custom derived query. Spring Data parses the method name:
- `findBy` → `SELECT`
- `Email` → `WHERE email = ?`

Generated SQL: `SELECT * FROM users WHERE email = ?`

The return type `Optional<User>` signals that the result may be absent, and Spring Data correctly
returns `Optional.empty()` when no row matches.

```java
// task-service/src/main/java/dev/kylebradshaw/task/repository/ProjectMemberRepository.java
public interface ProjectMemberRepository
    extends JpaRepository<ProjectMember, ProjectMember.ProjectMemberId> {

    List<ProjectMember> findByUserId(UUID userId);
    Optional<ProjectMember> findByProjectIdAndUserId(UUID projectId, UUID userId);
    boolean existsByProjectIdAndUserIdAndRole(UUID projectId, UUID userId, ProjectRole role);
}
```

The composite key type `ProjectMember.ProjectMemberId` is passed as the second generic parameter.

`findByUserId` → `SELECT * FROM project_members WHERE user_id = ?`

`findByProjectIdAndUserId` → `SELECT * FROM project_members WHERE project_id = ? AND user_id = ?`
The `And` keyword combines conditions. Spring Data supports `And`, `Or`, `Not`, `Between`,
`LessThan`, `GreaterThan`, `Like`, `In`, `IsNull`, `OrderBy`, and more.

`existsByProjectIdAndUserIdAndRole` → `SELECT COUNT(*) > 0 FROM project_members WHERE project_id = ?
AND user_id = ? AND role = ?`. The `exists` prefix generates an existence check, which is more
efficient than fetching the entire entity when you only care about presence.

```java
// task-service/src/main/java/dev/kylebradshaw/task/repository/TaskRepository.java
public interface TaskRepository extends JpaRepository<Task, UUID> {
    List<Task> findByProjectId(UUID projectId);
}
```

`findByProjectId` queries by the `project_id` FK column, not by the `Project` object. Spring Data
JPA understands that `projectId` refers to the `@JoinColumn(name = "project_id")` on the `project`
field even though the entity field is named `project`, not `projectId`. Spring Data navigates the
association metadata automatically.

In Go you would write:
```go
const q = "SELECT * FROM tasks WHERE project_id = $1 ORDER BY created_at DESC"
rows, err := db.QueryContext(ctx, q, projectID)
```

### Step 7 — `@Transactional` in the Service Layer

```java
// task-service/src/main/java/dev/kylebradshaw/task/service/ProjectService.java
@Service
public class ProjectService {
    private final ProjectRepository projectRepo;
    private final ProjectMemberRepository memberRepo;
    private final UserRepository userRepo;

    public ProjectService(ProjectRepository projectRepo,
                          ProjectMemberRepository memberRepo,
                          UserRepository userRepo) {
        this.projectRepo = projectRepo;
        this.memberRepo = memberRepo;
        this.userRepo = userRepo;
    }

    @Transactional                                           // (A)
    public Project createProject(CreateProjectRequest request, UUID userId) {
        User owner = userRepo.findById(userId)
            .orElseThrow(() -> new IllegalArgumentException("User not found"));  // (B)
        Project project = new Project(request.name(), request.description(), owner);
        project = projectRepo.save(project);
        var member = new ProjectMember(project.getId(), userId, ProjectRole.OWNER);  // (C)
        memberRepo.save(member);
        return project;
    }

    public List<Project> getProjectsForUser(UUID userId) {
        List<UUID> projectIds = memberRepo.findByUserId(userId)
            .stream()
            .map(ProjectMember::getProjectId)
            .toList();                                       // (D)
        return projectRepo.findAllById(projectIds);
    }

    @Transactional
    public Project updateProject(UUID projectId, UUID userId, String name, String description) {
        if (!memberRepo.existsByProjectIdAndUserIdAndRole(projectId, userId, ProjectRole.OWNER)) {
            throw new IllegalStateException("Only the owner can update the project");
        }
        Project project = getProject(projectId);
        if (name != null) { project.setName(name); }
        if (description != null) { project.setDescription(description); }
        return projectRepo.save(project);                    // (E)
    }
}
```

**Annotation (A) — `@Transactional`**
This annotation wraps the method in a database transaction. Spring creates a proxy around
`ProjectService`. When `createProject` is called:
1. Spring intercepts the call, opens a Hibernate session, and begins a transaction.
2. Your method body executes — `findById`, `save(project)`, `save(member)`.
3. If the method returns normally, Spring commits the transaction.
4. If an unchecked exception is thrown, Spring rolls back.

This means both `projectRepo.save(project)` and `memberRepo.save(member)` are in the same
transaction. If the second save fails, the first is rolled back — the two operations are atomic.

In Go you would write this explicitly:
```go
tx, err := db.BeginTx(ctx, nil)
if err != nil { return err }
defer tx.Rollback()

project, err := insertProject(tx, name, description, ownerID)
if err != nil { return err }

if err := insertProjectMember(tx, project.ID, ownerID, "OWNER"); err != nil {
    return err
}

return tx.Commit()
```

Spring's `@Transactional` eliminates this boilerplate while giving the same atomicity guarantees.

**Annotation (B) — `orElseThrow`**
`findById` returns `Optional<User>`. `.orElseThrow(supplier)` unwraps the value or throws the
supplied exception if absent. This is Java's standard way to convert "not found" from an `Optional`
into an exception. Equivalent to Go's `if user == nil { return fmt.Errorf("user not found") }` or
TypeScript's `if (!user) throw new Error("User not found")`.

**Annotation (C) — Accessing `project.getId()` after `save()`**
After `projectRepo.save(project)`, Hibernate has executed the `INSERT` and the `id` field on the
returned `Project` object is populated with the generated UUID. This is why the next line can use
`project.getId()` as the `projectId` for the new `ProjectMember`. If you used the original `project`
variable before `save()`, `getId()` would return `null`.

**Annotation (D) — `.toList()` (Java 16)**
`stream().map(...).toList()` is Java 16's addition. It returns an unmodifiable list. The older
equivalent is `.collect(Collectors.toList())`. The Java 16 form is cleaner and signals that the
result should not be mutated.

**Annotation (E) — Explicit `save()` after mutation**
In `updateProject`, after calling `project.setName(name)`, the project entity is mutated in memory.
Because the method is `@Transactional`, Hibernate's "dirty checking" would normally detect the
change and automatically flush an `UPDATE` on transaction commit — without needing an explicit
`save()`. The explicit `projectRepo.save(project)` call here is defensive and also returns the
entity for the caller. Both approaches are correct; explicit `save()` is clearer about intent.

### Step 8 — The `application.yml` JPA Configuration

```yaml
spring:
  datasource:
    url: jdbc:postgresql://${POSTGRES_HOST:localhost}:5432/taskdb
    username: ${POSTGRES_USER:taskuser}
    password: ${POSTGRES_PASSWORD:taskpass}
  jpa:
    hibernate:
      ddl-auto: update     # (A)
    open-in-view: false    # (B)
    properties:
      hibernate:
        dialect: org.hibernate.dialect.PostgreSQLDialect  # (C)
```

**Annotation (A) — `ddl-auto: update`**
This controls whether and how Hibernate manages the database schema:

| Value | Behavior | Use when |
|-------|----------|----------|
| `none` | Hibernate does nothing to the schema | External migration tool manages schema |
| `validate` | Verifies schema matches entities; fails on mismatch | Production |
| `update` | Adds missing tables/columns; does not drop anything | Development |
| `create` | Drops and recreates all tables on startup | Testing (clean state) |
| `create-drop` | Creates on start, drops on shutdown | Integration tests |

`update` is acceptable during development — it adds `tasks.due_date` if you add the field to the
entity, without destroying existing data. In production, `validate` is the safe choice: it ensures
the live schema matches the entity model without making any changes.

**Annotation (B) — `open-in-view: false`**
Covered in ADR 01, but worth repeating in the JPA context: with `open-in-view: true`, the
Hibernate session stays open through the HTTP response rendering phase. This means:
- A controller serializes a `Project` to JSON
- Jackson encounters `project.getOwner()` (a lazy proxy)
- Hibernate fires an additional `SELECT FROM users` query
- This query runs outside your `@Transactional` service method

This is the Open Session in View anti-pattern. It makes it impossible to reason about database
query counts from service-layer code alone. Setting `open-in-view: false` forces all lazy
associations to be initialized before the `@Transactional` boundary closes — or to be explicitly
fetched if needed.

**Annotation (C) — PostgreSQL dialect**
Hibernate generates slightly different SQL for different databases (date functions, JSON operators,
`RETURNING` clauses, etc.). Specifying `PostgreSQLDialect` explicitly tells Hibernate to use
PostgreSQL-specific SQL syntax and type mappings. Spring Boot 3.x can auto-detect this from the
JDBC URL, but being explicit prevents surprises if the URL format changes.

---

## Experiment

### 1 — Enable SQL logging to see generated queries

Add to `application.yml`:

```yaml
spring:
  jpa:
    show-sql: true
    properties:
      hibernate:
        format_sql: true
logging:
  level:
    org.hibernate.SQL: DEBUG
    org.hibernate.orm.jdbc.bind: TRACE
```

Restart and create a project via the API. You will see the exact SQL Hibernate generates, including
bind parameter values. This is the most important debugging tool when learning JPA — always know
what SQL is actually running.

### 2 — Change `FetchType.LAZY` to `FetchType.EAGER` on `Project.owner`

In `Project.java`, change:

```java
@ManyToOne(fetch = FetchType.LAZY)
```
to:
```java
@ManyToOne(fetch = FetchType.EAGER)
```

With SQL logging enabled, call `getProjectsForUser`. Notice that Hibernate now generates a JOIN
to load the owner with every project query. For a list of 10 projects with different owners, compare
the SQL output — EAGER generates one query with JOINs; LAZY generates N additional queries. Now
revert to LAZY.

### 3 — Change `ddl-auto` to `create` and observe the DROP

Change `ddl-auto: create` in `application.yml` and restart the service. Watch the logs — Hibernate
drops all managed tables and recreates them. Any existing data is gone. Change it back to `update`
immediately. This illustrates why `create` is only appropriate for test fixtures.

### 4 — Remove the `protected User() {}` no-arg constructor and observe the error

In `User.java`, delete the `protected User() {}` line. Restart the service. Hibernate will fail to
start with an error similar to:

```
org.hibernate.InstantiationException: No default constructor for entity: dev.kylebradshaw.task.entity.User
```

This makes the requirement concrete and memorable. Re-add the constructor.

### 5 — Add a JPQL query to `ProjectMemberRepository`

Add a custom JPQL query for a more complex use case that method naming cannot express:

```java
import org.springframework.data.jpa.repository.Query;

@Query("SELECT pm FROM ProjectMember pm WHERE pm.projectId = :projectId ORDER BY pm.role")
List<ProjectMember> findMembersByProjectOrdered(@Param("projectId") UUID projectId);
```

JPQL queries reference entity class names and field names (not table/column names). This is how
you escape the method-naming convention for complex queries while staying with JPA rather than
dropping to native SQL.

### 6 — Test `@Transactional` rollback behavior

Temporarily add a line that throws a `RuntimeException` after `projectRepo.save(project)` but
before `memberRepo.save(member)` in `createProject`. Attempt the operation. Verify that no row
exists in either `projects` or `project_members` — the transaction rolled back both saves. This
demonstrates atomicity.

---

## Check Your Understanding

1. **`@GeneratedValue(strategy = GenerationType.UUID)` generates the UUID in Java, not in the
   database. What are the implications for `project.getId()` before vs. after `save()`? How does
   this differ from a `SERIAL`/`BIGSERIAL` auto-increment strategy?**

   With UUID generation: `project.getId()` returns `null` before construction finishes... actually,
   `@GeneratedValue` with UUID strategy means Hibernate generates the UUID _when the entity is
   first persisted_ (on `save()`). The field is `null` until `projectRepo.save(project)` returns.
   The returned entity from `save()` has the populated `id`. With `IDENTITY` (auto-increment),
   the ID is generated by the database and returned via `INSERT ... RETURNING id`, also only
   available after `save()`. The key difference: UUID can be generated client-side before the
   INSERT (useful for batching or referencing the ID before the record exists in the database);
   IDENTITY requires a round-trip to the database.

2. **`getProjectsForUser` is not annotated with `@Transactional`. Could calling `getOwner()` on
   the returned `Project` objects cause a `LazyInitializationException`? When and why?**

   Yes. When `getProjectsForUser` returns, the Hibernate session closes (because there is no
   `@Transactional` wrapping the method). Any call to `project.getOwner()` on the returned objects
   will try to trigger a lazy load of the `User` association — but there is no active session to
   execute the query. This throws `LazyInitializationException`. To fix this, either add
   `@Transactional(readOnly = true)` to the method (keeping the session open through the return),
   or add a `JOIN FETCH` in a custom JPQL query to load the owner eagerly for that specific call.

3. **Why does `ProjectMemberId` implement `Serializable`? What would happen if you removed it?**

   Hibernate serializes entity identifiers for first-level (session) and second-level (optional
   cache) storage. The JPA specification requires that `@IdClass` and `@EmbeddedId` key classes be
   `Serializable`. Removing it causes Hibernate to throw a `MappingException` at startup:
   `Composite id class must implement Serializable`.

4. **`@Column(updatable = false)` on `created_at` prevents Hibernate from including the column in
   UPDATE statements. But what if you call `setCreatedAt(someOtherInstant)` on the entity and then
   `save()` it? Does the database row change?**

   No. `updatable = false` instructs Hibernate's SQL generator to omit the column from all
   `UPDATE` SQL regardless of whether the Java field value changed. Even if you mutate
   `createdAt` in memory, the `UPDATE` statement Hibernate generates will not include it. The
   database value remains unchanged. This is a compile-time-invisible contract — there is no
   compile error if you call a non-existent setter, but the constraint is enforced at the Hibernate
   SQL generation level.

5. **`existsByProjectIdAndUserIdAndRole` in `ProjectMemberRepository` takes a `ProjectRole` enum
   parameter. Spring Data JPA has to match this against a `VARCHAR` column storing the string
   `"OWNER"`. How does Spring Data JPA know to convert the enum to its string value?**

   Spring Data JPA uses the JPA attribute converter associated with the field's `@Enumerated`
   annotation. Because `ProjectMember.role` is annotated with `@Enumerated(EnumType.STRING)`,
   Hibernate knows to convert `ProjectRole.OWNER` to the string `"OWNER"` for all SQL operations
   on that column — including the query generated from `existsByProjectIdAndUserIdAndRole`. The
   derived query methods inherit the same type mappings as the entity itself.

6. **The `updateProject` method calls `projectRepo.save(project)` explicitly after mutation. If
   you removed the `save()` call but kept `@Transactional`, would the database still be updated?
   Why or why not?**

   Yes, in most cases. Hibernate's "dirty checking" mechanism compares the entity's current state
   to a snapshot taken when the entity was loaded within the transaction. At transaction commit,
   Hibernate automatically flushes any changes to dirty (modified) entities via `UPDATE` statements.
   Since `updateProject` is `@Transactional`, the session persists through the method and dirty
   checking fires at commit. The explicit `save()` is redundant but makes the intent clear and
   returns the entity. The notable edge case: if the entity was loaded in a _different_ transaction
   and passed into this method as a detached entity, dirty checking would not apply — `save()` would
   be required to reattach and persist the changes.
