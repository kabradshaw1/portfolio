# 01 — Spring Boot and Gradle Multi-Module Setup

## Overview

This document covers the foundational layer of the `java-task-management` project: the Spring Boot
framework and the Gradle multi-module build system. If you are coming from Go or TypeScript, the
Java ecosystem will feel verbose at first. The payoff is a rich, opinionated ecosystem where most
infrastructure concerns — dependency injection, configuration binding, embedded servers, health
checks — are solved before you write a single line of business logic.

By the end of this document you will be able to:

- Explain what Spring Boot actually does vs. what plain Spring does
- Read and modify the root `build.gradle` and per-module `build.gradle` files
- Understand why constructor injection is preferred over field injection
- Map `application.yml` config patterns to environment variable overrides
- Recognise the role each Spring Boot starter plays
- Compare all of these patterns to their Go and TypeScript equivalents

---

## Architecture Context

The project is a multi-service system with four services:

```
java-task-management/          (root Gradle project)
├── task-service/              (core CRUD: projects, tasks, users)
├── activity-service/          (audit log stream consumer)
├── notification-service/      (email/push fan-out)
└── gateway-service/           (routing, auth token validation)
```

All four services are Java 21, Spring Boot 3.4.x applications. They share a single Gradle root
project so that common dependencies, compiler settings, and code-quality tooling (Checkstyle) are
defined once and inherited by every subproject. Each service is also a fully independent deployable
JAR — they share build config but not runtime classes.

Spring Boot 3.4.x requires Java 17 as a minimum but this project targets Java 21 to take advantage
of virtual threads, records, and pattern matching. The combination of Spring Boot 3 + Java 21 is the
current production baseline in the Java ecosystem as of early 2026.

---

## Package Introductions

### Spring Boot

Spring Boot is an opinionated wrapper around the Spring Framework that removes the XML configuration
era entirely. It works through three mechanisms:

1. **Auto-configuration** — hundreds of `@Configuration` classes in `spring-boot-autoconfigure.jar`
   activate conditionally based on what is on the classpath and what properties are set. Put the
   PostgreSQL driver on the classpath and set `spring.datasource.*` properties → a fully configured
   `DataSource` bean appears automatically.

2. **Starters** — curated dependency bundles. `spring-boot-starter-web` pulls in Spring MVC,
   Jackson, and an embedded Tomcat. You do not manage individual library versions; Spring Boot's
   BOM (bill of materials) manages them all together.

3. **Embedded server** — the application packages as a fat JAR. You run `java -jar app.jar` and the
   web server starts inside the process. No WAR files, no external Tomcat.

**Alternatives considered:**

| Framework | Why not chosen |
|-----------|---------------|
| **Micronaut** | Compile-time DI (no reflection), faster startup, smaller memory. Strong choice for serverless/GraalVM native. Less mature ecosystem, fewer Spring Boot integrations in the wild. |
| **Quarkus** | Similar to Micronaut — excellent for Kubernetes/native images. The JVM startup time of Spring Boot is acceptable for long-lived microservices on a Windows PC backend. |
| **Plain Spring MVC** | Spring Boot 3 _is_ Spring MVC plus auto-config. There is no practical reason to use Spring MVC without Boot for new projects. |
| **Helidon / Vert.x** | Reactive/non-blocking. Correct when you need extreme throughput with minimal threads. Adds significant conceptual overhead for a portfolio project. |

Spring Boot was chosen because it is the dominant Java framework in 2026 industry hiring, has the
richest ecosystem, and its convention-over-configuration approach maps well to how you already think
about Express or Gin.

### Gradle

Gradle is the build tool (equivalent to `npm`/`go build` combined). This project uses Gradle's
multi-project build feature, which lets one root `build.gradle` act like a shared `package.json`
for all subprojects.

**Alternatives considered:**

| Tool | Why not chosen |
|------|---------------|
| **Maven** | XML-based, still extremely common in enterprise Java. Verbose. Gradle is now the default for Spring Initializr and Android. |
| **Bazel** | Monorepo at scale. Complete overkill for four services. |

### Checkstyle

Checkstyle enforces code style rules (indentation, import ordering, line length). Equivalent to
`eslint` + `prettier` in the TypeScript world or `gofmt` enforcement in Go CI. Configured via
`config/checkstyle/checkstyle.xml` and wired into every subproject's build.

### Spring Boot Actuator

`spring-boot-starter-actuator` adds production-ready endpoints: `/actuator/health`,
`/actuator/metrics`, `/actuator/info`, etc. The `application.yml` exposes only `health` and hides
its details — a sensible default for a public-facing service.

---

## Go / TypeScript Comparison

| Concern | Go | TypeScript / Node | Java / Spring Boot |
|---------|----|--------------------|-------------------|
| **Entry point** | `func main()` in `main.go` | `app.listen(3000)` in `index.ts` | `SpringApplication.run(...)` in `main()` |
| **Dependency injection** | Manual struct init / wire | NestJS decorators, InversifyJS | `@Autowired` / constructor injection (Spring IoC container) |
| **Config from env** | `os.Getenv`, `envconfig` structs | `process.env`, `dotenv` | `${ENV_VAR:default}` in `application.yml` → `@Value` or `@ConfigurationProperties` |
| **HTTP server** | `net/http`, Gin, Echo | Express, Fastify | Embedded Tomcat via `spring-boot-starter-web` |
| **Build tool** | `go build`, `Makefile` | `npm`, `turbo` | Gradle (multi-module) or Maven |
| **Dependency management** | `go.mod` / `go.sum` | `package.json` / `package-lock.json` | `build.gradle` + Spring Boot BOM for versions |
| **Fat binary** | Single static binary | `node_modules` bundle | Fat JAR (`java -jar app.jar`) |
| **Code style enforcement** | `gofmt`, `golangci-lint` | ESLint, Prettier | Checkstyle, SpotBugs |
| **Health checks** | Custom `/health` handler | Custom route or libraries | `spring-boot-starter-actuator` → `/actuator/health` |
| **Module system** | One `go.mod` per repo (or workspace) | `package.json` workspaces | Gradle multi-project `settings.gradle` |

---

## Build It

### Step 1 — The Root `settings.gradle`

```groovy
// java/settings.gradle
rootProject.name = 'java-task-management'

include 'task-service'
include 'activity-service'
include 'notification-service'
include 'gateway-service'
```

`settings.gradle` is the first file Gradle reads. It declares the root project name and registers
every subproject. This is equivalent to listing workspaces in a root `package.json`, or listing
modules in a Go workspace `go.work` file. After this file, Gradle knows to look for
`task-service/build.gradle`, `activity-service/build.gradle`, and so on.

### Step 2 — The Root `build.gradle`

```groovy
// java/build.gradle
plugins {
    id 'java'
    id 'org.springframework.boot' version '3.4.4' apply false      // (A)
    id 'io.spring.dependency-management' version '1.1.7' apply false // (B)
    id 'checkstyle'
}

allprojects {
    group = 'dev.kylebradshaw'
    version = '0.1.0'
}

subprojects {
    apply plugin: 'java'
    apply plugin: 'org.springframework.boot'
    apply plugin: 'io.spring.dependency-management'
    apply plugin: 'checkstyle'

    java {
        sourceCompatibility = JavaVersion.VERSION_21
        targetCompatibility = JavaVersion.VERSION_21
    }

    repositories {
        mavenCentral()
    }

    dependencies {
        implementation 'org.springframework.boot:spring-boot-starter-web'       // (C)
        implementation 'org.springframework.boot:spring-boot-starter-actuator'  // (D)
        testImplementation 'org.springframework.boot:spring-boot-starter-test'
    }

    tasks.named('test') {
        useJUnitPlatform {
            excludeTags 'integration'                                             // (E)
        }
        jvmArgs '-Dnet.bytebuddy.experimental=true'
    }

    tasks.register('integrationTest', Test) {
        description = 'Runs Testcontainers integration tests (requires Docker).'
        group = 'verification'
        useJUnitPlatform {
            includeTags 'integration'
        }
        jvmArgs '-Dnet.bytebuddy.experimental=true'
    }

    checkstyle {
        toolVersion = '10.21.4'
        configFile = rootProject.file('config/checkstyle/checkstyle.xml')
    }
}
```

**Annotation (A) — `apply false`**
Declaring a plugin at the root with `apply false` registers it in Gradle's plugin resolution cache
without activating it on the root project itself. The root project is not a Spring Boot app; only
the subprojects are. The `subprojects { apply plugin: ... }` block then activates it on each
subproject. This is a standard Gradle pattern for multi-module Spring Boot projects.

**Annotation (B) — `io.spring.dependency-management`**
This plugin imports Spring Boot's BOM (Bill of Materials). A BOM is a curated list of dependency
versions that are known to work together. When you write
`implementation 'org.springframework.boot:spring-boot-starter-web'` with no version number, the
BOM fills in the correct version automatically. This is equivalent to using a pinned `package-lock`
entry — you get a tested, compatible set of library versions without version-wrangling.

**Annotation (C) — `spring-boot-starter-web`**
Adds: Spring MVC (HTTP routing/controllers), embedded Tomcat, Jackson (JSON serialization),
`@RestController`, `@RequestMapping`, etc. Every service in this project serves HTTP, so it belongs
in the shared subprojects block.

**Annotation (D) — `spring-boot-starter-actuator`**
Adds production health/metrics endpoints. Shared across all services so every one of them is
observable the same way.

**Annotation (E) — Splitting unit vs. integration tests**
The `test` task excludes JUnit 5 tags named `integration`. The `integrationTest` task includes only
those tags. Integration tests use Testcontainers (real Docker containers for Postgres, RabbitMQ) and
are too slow for the fast feedback loop of `./gradlew test`. Running `./gradlew integrationTest`
spins up Docker — you do this less frequently or only in CI.

### Step 3 — The Service `build.gradle`

```groovy
// java/task-service/build.gradle
dependencies {
    implementation 'org.springframework.boot:spring-boot-starter-data-jpa'   // (F)
    implementation 'org.springframework.boot:spring-boot-starter-security'   // (G)
    implementation 'org.springframework.boot:spring-boot-starter-amqp'       // (H)
    implementation 'org.springframework.boot:spring-boot-starter-validation' // (I)
    runtimeOnly 'org.postgresql:postgresql'                                   // (J)

    implementation 'io.jsonwebtoken:jjwt-api:0.12.6'
    runtimeOnly 'io.jsonwebtoken:jjwt-impl:0.12.6'
    runtimeOnly 'io.jsonwebtoken:jjwt-jackson:0.12.6'

    testImplementation 'org.springframework.security:spring-security-test'
    testImplementation 'org.testcontainers:junit-jupiter'
    testImplementation 'org.testcontainers:postgresql'
    testImplementation 'org.testcontainers:rabbitmq'
}
```

This file only declares _additional_ dependencies beyond what the root `subprojects` block provides.
It does not repeat `spring-boot-starter-web` because that is inherited.

**Annotation (F) — `spring-boot-starter-data-jpa`**
Pulls in Hibernate ORM, Spring Data JPA (the repository abstraction), and wires them to a
`DataSource`. Without this starter you would need to configure each piece manually. See ADR 02 for
the full JPA story.

**Annotation (G) — `spring-boot-starter-security`**
Activates Spring Security. By default, Spring Security locks down _all_ endpoints with HTTP Basic
auth. The security config in `task-service` replaces this default with JWT-based authentication.
See ADR 03 for the security story.

**Annotation (H) — `spring-boot-starter-amqp`**
Adds Spring AMQP + RabbitMQ client. Enables `@RabbitListener`, `RabbitTemplate`, and auto-
configuration of the connection factory from `spring.rabbitmq.*` properties.

**Annotation (I) — `spring-boot-starter-validation`**
Adds Bean Validation (Jakarta Validation API + Hibernate Validator). Enables `@NotBlank`, `@Email`,
`@Size` annotations on DTOs. Triggered via `@Valid` on controller method parameters.

**Annotation (J) — `runtimeOnly`**
The PostgreSQL JDBC driver is only needed at runtime — your code never imports a class from it
directly. Gradle's `runtimeOnly` scope keeps it off the compile classpath. Equivalent to listing a
package as a `dependency` vs. `devDependency` in npm, though not a perfect analogy.

### Step 4 — The Application Entry Point

```java
// task-service/src/main/java/dev/kylebradshaw/task/TaskServiceApplication.java
package dev.kylebradshaw.task;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

@SpringBootApplication
public class TaskServiceApplication {
    public static void main(String[] args) {
        SpringApplication.run(TaskServiceApplication.class, args);
    }
}
```

`@SpringBootApplication` is a shorthand for three annotations:

- `@SpringBootConfiguration` — marks this class as a configuration source (like `@Configuration`)
- `@EnableAutoConfiguration` — tells Spring Boot to activate auto-configuration based on the
  classpath
- `@ComponentScan` — tells Spring to scan the current package and all sub-packages for annotated
  classes (`@Service`, `@Repository`, `@Controller`, `@Component`, etc.)

The package `dev.kylebradshaw.task` is the scan root. Any class in
`dev.kylebradshaw.task.service`, `dev.kylebradshaw.task.repository`, etc. will be discovered
automatically. This is why package structure matters in Spring Boot — if you put a class outside
the scan root it will not be picked up.

`SpringApplication.run(...)` bootstraps the entire application context, runs auto-configuration,
starts the embedded Tomcat on the configured port, and blocks until the process is terminated.

In Go you would write something like:

```go
func main() {
    cfg := config.Load()
    db := postgres.Connect(cfg.DatabaseURL)
    repo := repository.New(db)
    svc := service.New(repo)
    router := api.NewRouter(svc)
    http.ListenAndServe(":8081", router)
}
```

The Spring Boot entry point is that entire chain collapsed into one line. The framework wires
everything together for you based on annotations and configuration.

### Step 5 — `application.yml` Configuration

```yaml
# task-service/src/main/resources/application.yml
server:
  port: 8081

spring:
  datasource:
    url: jdbc:postgresql://${POSTGRES_HOST:localhost}:5432/taskdb
    username: ${POSTGRES_USER:taskuser}
    password: ${POSTGRES_PASSWORD:taskpass}
  jpa:
    hibernate:
      ddl-auto: update
    open-in-view: false
    properties:
      hibernate:
        dialect: org.hibernate.dialect.PostgreSQLDialect
  rabbitmq:
    host: ${RABBITMQ_HOST:localhost}
    port: 5672
    username: ${RABBITMQ_USER:guest}
    password: ${RABBITMQ_PASSWORD:guest}

app:
  jwt:
    secret: ${JWT_SECRET:dev-secret-key-at-least-32-characters-long}
    access-token-ttl-ms: 900000
    refresh-token-ttl-ms: 604800000
  google:
    client-id: ${GOOGLE_CLIENT_ID:}
    client-secret: ${GOOGLE_CLIENT_SECRET:}
    token-url: https://oauth2.googleapis.com/token
    userinfo-url: https://www.googleapis.com/oauth2/v3/userinfo
  allowed-origins: ${ALLOWED_ORIGINS:http://localhost:3000}

management:
  endpoints:
    web:
      exposure:
        include: health
  endpoint:
    health:
      show-details: never
```

**The `${ENV_VAR:default}` syntax** is Spring's property placeholder with a fallback value. This is
identical in purpose to Go's:

```go
host := os.Getenv("POSTGRES_HOST")
if host == "" {
    host = "localhost"
}
```

Or TypeScript's:

```typescript
const host = process.env.POSTGRES_HOST ?? 'localhost';
```

The YAML structure maps directly to Spring Boot's auto-configuration property namespaces:
- `spring.datasource.*` → configures the `DataSource` bean (JDBC connection pool)
- `spring.jpa.*` → configures Hibernate / JPA
- `spring.rabbitmq.*` → configures the AMQP connection factory
- `management.*` → configures Spring Boot Actuator

The `app.*` namespace is custom — anything under `app` is your own configuration. Spring Boot does
not interpret it automatically; you read it via `@ConfigurationProperties` beans or `@Value`
annotations.

**`open-in-view: false`** deserves attention. The default value is `true`, which keeps the
Hibernate `EntityManager` (the database session) open for the entire duration of an HTTP request,
including during response serialization. This is the "Open Session in View" anti-pattern — it
causes lazy-loaded associations to trigger extra SQL queries during JSON serialization, making it
hard to reason about what database work is happening. Setting it to `false` forces all database
work to complete inside `@Transactional` service methods, which is the correct architecture.

### Step 6 — Constructor Injection

```java
// task-service/src/main/java/dev/kylebradshaw/task/service/ProjectService.java
@Service
public class ProjectService {
    private final ProjectRepository projectRepo;
    private final ProjectMemberRepository memberRepo;
    private final UserRepository userRepo;

    public ProjectService(
        ProjectRepository projectRepo,
        ProjectMemberRepository memberRepo,
        UserRepository userRepo
    ) {
        this.projectRepo = projectRepo;
        this.memberRepo = memberRepo;
        this.userRepo = userRepo;
    }
    // ...
}
```

`@Service` registers this class with Spring's IoC (Inversion of Control) container. When the
application context starts, Spring sees that `ProjectService` needs three constructor arguments —
`ProjectRepository`, `ProjectMemberRepository`, and `UserRepository`. It looks them up in the
container (they are also registered as beans via their own annotations) and injects them.

**Why constructor injection over field injection?**

Field injection looks simpler:

```java
@Service
public class ProjectService {
    @Autowired private ProjectRepository projectRepo;  // do NOT do this
    @Autowired private ProjectMemberRepository memberRepo;
    @Autowired private UserRepository userRepo;
}
```

But constructor injection is universally preferred because:

1. **Testability** — you can instantiate `ProjectService` in a unit test by passing mock objects to
   the constructor. With field injection, you need Spring's reflection-based `@MockBean` machinery
   to inject mocks.
2. **Immutability** — dependencies are `final`, preventing accidental reassignment after
   construction.
3. **Explicitness** — all dependencies are visible at the construction site. Field injection hides
   them inside the class body.
4. **Compiler safety** — if a required dependency is missing, the application fails to start with a
   clear error rather than throwing a `NullPointerException` at runtime.

When a class has exactly one constructor, Spring 4.3+ injects it automatically without requiring
`@Autowired` on the constructor — which is why you do not see that annotation in `ProjectService`.

### Step 7 — Java 21 Records as DTOs

```java
// task-service/src/main/java/dev/kylebradshaw/task/dto/CreateProjectRequest.java
package dev.kylebradshaw.task.dto;

import jakarta.validation.constraints.NotBlank;

public record CreateProjectRequest(@NotBlank String name, String description) {}
```

A Java `record` is an immutable data carrier. The compiler generates:
- A canonical constructor
- `private final` fields
- `name()` and `description()` accessor methods (not `getName()` — records use bare method names)
- `equals()`, `hashCode()`, and `toString()` implementations

This is equivalent to Go's:

```go
type CreateProjectRequest struct {
    Name        string `json:"name" validate:"required"`
    Description string `json:"description"`
}
```

Or TypeScript's:

```typescript
interface CreateProjectRequest {
  name: string;
  description?: string;
}
```

Records are ideal for DTOs (Data Transfer Objects) — request/response shapes that should not be
mutated. The `@NotBlank` annotation from Jakarta Validation is processed when the controller method
is annotated with `@Valid`. If `name` is blank, Spring returns a 400 Bad Request automatically.

Note that records **cannot be JPA entities** (they have no no-arg constructor and are immutable —
both disqualify them for Hibernate). Records are for DTOs; plain classes are for entities.

---

## Experiment

These are parameter tweaks worth trying to understand trade-offs.

### 1 — Change the server port

In `application.yml`, change `server.port` from `8081` to `0`:

```yaml
server:
  port: 0
```

Port `0` tells the OS to assign a random available port. Spring Boot logs the actual port at
startup: `Tomcat started on port(s): 52341 (http)`. This is commonly used in integration tests to
avoid port conflicts when running multiple services on the same machine.

### 2 — Enable all Actuator endpoints

```yaml
management:
  endpoints:
    web:
      exposure:
        include: "*"
```

Restart and visit `http://localhost:8081/actuator`. You will see a full list of available endpoints.
Try `/actuator/env` to see all resolved configuration values (including which came from environment
variables vs. defaults). Try `/actuator/beans` to see every bean in the application context. This
is enormously useful for debugging — the equivalent of `go tool nm` but for runtime Spring beans.
**Important:** never expose `*` in production. It leaks secrets via `/actuator/env`.

### 3 — Switch `open-in-view` to `true` and observe the warning

```yaml
spring:
  jpa:
    open-in-view: true
```

Spring Boot will log a `WARN` at startup:

```
spring.jpa.open-in-view is enabled by default. Therefore, database queries may be
performed during view rendering. Explicitly configure spring.jpa.open-in-view to disable
this warning.
```

This is Spring's own team warning you against their own default. Leave it `false`.

### 4 — Add a second Spring profile

Create `src/main/resources/application-prod.yml`:

```yaml
spring:
  jpa:
    hibernate:
      ddl-auto: validate    # never mutate schema in production
management:
  endpoint:
    health:
      show-details: always  # show to internal monitoring only
```

Activate it with `SPRING_PROFILES_ACTIVE=prod`. Spring merges `application.yml` and
`application-prod.yml`, with the profile-specific file taking precedence. This is the Spring
equivalent of loading a `.env.production` override file.

### 5 — Explore the multi-module build commands

```bash
# Build and test only task-service
./gradlew :task-service:test

# Build the fat JAR for task-service
./gradlew :task-service:bootJar

# Run all unit tests across all modules
./gradlew test

# Run integration tests (requires Docker)
./gradlew integrationTest

# Check code style
./gradlew checkstyleMain
```

The `:task-service:` prefix scopes a task to one subproject. Without the prefix, Gradle runs the
task across all subprojects that define it.

---

## Check Your Understanding

1. **The root `build.gradle` declares the Spring Boot plugin with `apply false`. What would happen
   if you removed `apply false`? What would break and why?**

   The root project would try to become a Spring Boot application. It has no `main()` method, no
   `src/main/java` source set with a `@SpringBootApplication` class, and no application-specific
   dependencies. The `bootJar` task would fail because there is no main class to package. The
   intent of the root project is to be a build coordination layer only, not a runnable application.

2. **`ProjectService` uses `private final` fields. Why would making them non-final be a code smell,
   even though Spring would still inject the dependencies?**

   Non-final fields can be reassigned after construction. Another developer (or a test helper) could
   accidentally replace a repository reference. `final` makes the contract explicit: once
   constructed, dependencies do not change. The Spring team's own guidance and checkstyle rules
   enforce immutable injection fields.

3. **The `application.yml` has `${JWT_SECRET:dev-secret-key-at-least-32-characters-long}` with a
   hardcoded fallback. What is the risk of deploying this to production without setting the
   `JWT_SECRET` environment variable?**

   The fallback value is publicly visible in the source repository. Any attacker who reads the code
   can forge JWT tokens signed with `dev-secret-key-at-least-32-characters-long` and authenticate
   as any user. This is a critical secret that must be set via an environment variable or secrets
   manager in every non-development environment.

4. **`CreateProjectRequest` is a Java record, but `Project` is a plain class. What specifically
   prevents a record from being a JPA entity?**

   JPA/Hibernate requires a no-arg constructor (to instantiate objects from `ResultSet` rows via
   reflection) and mutable setters (to populate fields after construction). Records have neither —
   they are immutable and have only a canonical constructor. Hibernate also relies on subclassing
   entities to create proxy objects for lazy loading, and records are `final` and cannot be
   subclassed.

5. **What does the `@ComponentScan` in `@SpringBootApplication` actually scan? If you created a
   class `com.example.helper.HelperService` annotated with `@Service`, would Spring find it?**

   `@ComponentScan` scans the package of the annotated class and all sub-packages recursively. The
   annotated class is in `dev.kylebradshaw.task`. A class in `com.example.helper` is completely
   outside that tree — Spring would not find it. To include it you would need to add an explicit
   `@ComponentScan(basePackages = {"dev.kylebradshaw.task", "com.example.helper"})` or move the
   class into the correct package tree.

6. **The `test` Gradle task excludes `integration` tags, and `integrationTest` includes them.
   Could you run both in a single `./gradlew check` invocation? Would that be a good idea?**

   You could make `check` depend on `integrationTest`, but it would be a bad default. Integration
   tests require Docker, take minutes, and should not block a fast local unit-test feedback loop.
   The current separation lets developers run `./gradlew test` in seconds during development and
   reserve `./gradlew integrationTest` for pre-merge CI or explicit local verification.
