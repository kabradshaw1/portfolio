# Java Task Management Backend — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build four Spring Boot microservices (task-service, activity-service, notification-service, gateway-service) that power a task/project management application, demonstrating Spring Boot, PostgreSQL, MongoDB, Redis, RabbitMQ, GraphQL, Google OAuth + JWT auth, and Testcontainers.

**Architecture:** A Gradle multi-module project under `java/` with four Spring Boot services. task-service owns core business logic and auth (PostgreSQL). activity-service stores audit logs and comments (MongoDB). notification-service manages user notifications (Redis). gateway-service exposes a unified GraphQL API, validates JWTs, and routes to downstream services via REST.

**Tech Stack:** Java 21, Spring Boot 3.4.4, Gradle 8.12 (Groovy DSL), PostgreSQL, MongoDB, Redis, RabbitMQ, Spring for GraphQL, Spring Security, Spring Data JPA/MongoDB/Redis, Spring AMQP, jjwt 0.12.6, JUnit 5, Mockito, Testcontainers

**Related plans:** Frontend plan (TBD), DevOps/CI/K8s plan (TBD)

---

## File Structure

```
java/
├── build.gradle                          # Root Gradle build — shared plugins/config
├── settings.gradle                       # Module declarations
├── gradle/
│   └── wrapper/
│       ├── gradle-wrapper.jar
│       └── gradle-wrapper.properties
├── gradlew                               # Unix wrapper script
├── gradlew.bat                           # Windows wrapper script
├── docker-compose.yml                    # Infrastructure: PostgreSQL, MongoDB, Redis, RabbitMQ
│
├── task-service/
│   ├── build.gradle
│   └── src/
│       ├── main/
│       │   ├── java/dev/kylebradshaw/task/
│       │   │   ├── TaskServiceApplication.java
│       │   │   ├── config/
│       │   │   │   ├── SecurityConfig.java
│       │   │   │   └── RabbitConfig.java
│       │   │   ├── controller/
│       │   │   │   ├── AuthController.java
│       │   │   │   ├── ProjectController.java
│       │   │   │   └── TaskController.java
│       │   │   ├── dto/
│       │   │   │   ├── AuthRequest.java
│       │   │   │   ├── AuthResponse.java
│       │   │   │   ├── CreateProjectRequest.java
│       │   │   │   ├── ProjectResponse.java
│       │   │   │   ├── CreateTaskRequest.java
│       │   │   │   ├── UpdateTaskRequest.java
│       │   │   │   ├── TaskResponse.java
│       │   │   │   └── TaskEventMessage.java
│       │   │   ├── entity/
│       │   │   │   ├── User.java
│       │   │   │   ├── RefreshToken.java
│       │   │   │   ├── Project.java
│       │   │   │   ├── ProjectMember.java
│       │   │   │   ├── ProjectRole.java
│       │   │   │   ├── Task.java
│       │   │   │   ├── TaskStatus.java
│       │   │   │   └── TaskPriority.java
│       │   │   ├── repository/
│       │   │   │   ├── UserRepository.java
│       │   │   │   ├── RefreshTokenRepository.java
│       │   │   │   ├── ProjectRepository.java
│       │   │   │   ├── ProjectMemberRepository.java
│       │   │   │   └── TaskRepository.java
│       │   │   ├── security/
│       │   │   │   ├── JwtService.java
│       │   │   │   └── JwtAuthenticationFilter.java
│       │   │   └── service/
│       │   │       ├── AuthService.java
│       │   │       ├── ProjectService.java
│       │   │       ├── TaskService.java
│       │   │       └── TaskEventPublisher.java
│       │   └── resources/
│       │       └── application.yml
│       └── test/
│           └── java/dev/kylebradshaw/task/
│               ├── service/
│               │   ├── ProjectServiceTest.java
│               │   ├── TaskServiceTest.java
│               │   └── AuthServiceTest.java
│               ├── controller/
│               │   ├── ProjectControllerTest.java
│               │   ├── TaskControllerTest.java
│               │   └── AuthControllerTest.java
│               └── integration/
│                   └── TaskServiceIntegrationTest.java
│
├── activity-service/
│   ├── build.gradle
│   └── src/
│       ├── main/
│       │   ├── java/dev/kylebradshaw/activity/
│       │   │   ├── ActivityServiceApplication.java
│       │   │   ├── config/
│       │   │   │   └── RabbitConfig.java
│       │   │   ├── controller/
│       │   │   │   ├── ActivityController.java
│       │   │   │   └── CommentController.java
│       │   │   ├── document/
│       │   │   │   ├── ActivityEvent.java
│       │   │   │   └── Comment.java
│       │   │   ├── dto/
│       │   │   │   ├── TaskEventMessage.java
│       │   │   │   ├── CreateCommentRequest.java
│       │   │   │   └── CommentResponse.java
│       │   │   ├── listener/
│       │   │   │   └── TaskEventListener.java
│       │   │   ├── repository/
│       │   │   │   ├── ActivityEventRepository.java
│       │   │   │   └── CommentRepository.java
│       │   │   └── service/
│       │   │       ├── ActivityService.java
│       │   │       └── CommentService.java
│       │   └── resources/
│       │       └── application.yml
│       └── test/
│           └── java/dev/kylebradshaw/activity/
│               ├── service/
│               │   ├── ActivityServiceTest.java
│               │   └── CommentServiceTest.java
│               ├── controller/
│               │   ├── ActivityControllerTest.java
│               │   └── CommentControllerTest.java
│               └── listener/
│                   └── TaskEventListenerTest.java
│
├── notification-service/
│   ├── build.gradle
│   └── src/
│       ├── main/
│       │   ├── java/dev/kylebradshaw/notification/
│       │   │   ├── NotificationServiceApplication.java
│       │   │   ├── config/
│       │   │   │   └── RabbitConfig.java
│       │   │   ├── controller/
│       │   │   │   └── NotificationController.java
│       │   │   ├── dto/
│       │   │   │   ├── TaskEventMessage.java
│       │   │   │   ├── NotificationResponse.java
│       │   │   │   └── Notification.java
│       │   │   ├── listener/
│       │   │   │   └── TaskEventListener.java
│       │   │   └── service/
│       │   │       └── NotificationService.java
│       │   └── resources/
│       │       └── application.yml
│       └── test/
│           └── java/dev/kylebradshaw/notification/
│               ├── service/
│               │   └── NotificationServiceTest.java
│               ├── controller/
│               │   └── NotificationControllerTest.java
│               └── listener/
│                   └── TaskEventListenerTest.java
│
└── gateway-service/
    ├── build.gradle
    └── src/
        ├── main/
        │   ├── java/dev/kylebradshaw/gateway/
        │   │   ├── GatewayServiceApplication.java
        │   │   ├── client/
        │   │   │   ├── TaskServiceClient.java
        │   │   │   ├── ActivityServiceClient.java
        │   │   │   └── NotificationServiceClient.java
        │   │   ├── config/
        │   │   │   ├── SecurityConfig.java
        │   │   │   └── RestClientConfig.java
        │   │   ├── dto/
        │   │   │   ├── ProjectDto.java
        │   │   │   ├── TaskDto.java
        │   │   │   ├── UserDto.java
        │   │   │   ├── ActivityEventDto.java
        │   │   │   ├── CommentDto.java
        │   │   │   └── NotificationDto.java
        │   │   ├── resolver/
        │   │   │   ├── QueryResolver.java
        │   │   │   └── MutationResolver.java
        │   │   └── security/
        │   │       ├── JwtService.java
        │   │       └── JwtAuthenticationFilter.java
        │   └── resources/
        │       ├── application.yml
        │       └── graphql/
        │           └── schema.graphqls
        └── test/
            └── java/dev/kylebradshaw/gateway/
                ├── resolver/
                │   ├── QueryResolverTest.java
                │   └── MutationResolverTest.java
                └── security/
                    └── JwtAuthenticationFilterTest.java
```

---

## Phase 1: Project Scaffolding

### Task 1: Gradle Multi-Module Project Setup

**Files:**
- Create: `java/build.gradle`
- Create: `java/settings.gradle`

- [ ] **Step 1: Generate Gradle wrapper**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer
mkdir -p java && cd java
gradle wrapper --gradle-version 8.12
```

Expected: `gradlew`, `gradlew.bat`, `gradle/wrapper/` created.

- [ ] **Step 2: Write root build.gradle**

Create `java/build.gradle`:

```groovy
plugins {
    id 'java'
    id 'org.springframework.boot' version '3.4.4' apply false
    id 'io.spring.dependency-management' version '1.1.7' apply false
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
        toolchain {
            languageVersion = JavaLanguageVersion.of(21)
        }
    }

    repositories {
        mavenCentral()
    }

    dependencies {
        implementation 'org.springframework.boot:spring-boot-starter-web'
        implementation 'org.springframework.boot:spring-boot-starter-actuator'
        testImplementation 'org.springframework.boot:spring-boot-starter-test'
    }

    tasks.named('test') {
        useJUnitPlatform()
    }

    checkstyle {
        toolVersion = '10.21.4'
        configFile = rootProject.file('config/checkstyle/checkstyle.xml')
    }
}
```

- [ ] **Step 3: Write settings.gradle**

Create `java/settings.gradle`:

```groovy
rootProject.name = 'java-task-management'

include 'task-service'
include 'activity-service'
include 'notification-service'
include 'gateway-service'
```

- [ ] **Step 4: Create Checkstyle config**

Create `java/config/checkstyle/checkstyle.xml`:

```xml
<?xml version="1.0"?>
<!DOCTYPE module PUBLIC
    "-//Checkstyle//DTD Checkstyle Configuration 1.3//EN"
    "https://checkstyle.org/dtds/configuration_1_3.dtd">
<module name="Checker">
    <module name="TreeWalker">
        <module name="AvoidStarImport"/>
        <module name="UnusedImports"/>
        <module name="NeedBraces"/>
        <module name="LeftCurly"/>
        <module name="RightCurly"/>
    </module>
    <module name="FileTabCharacter"/>
    <module name="NewlineAtEndOfFile">
        <property name="lineSeparator" value="lf"/>
    </module>
</module>
```

- [ ] **Step 5: Verify Gradle resolves**

```bash
cd java && ./gradlew tasks --no-daemon
```

Expected: Lists available tasks (build will fail since subprojects don't exist yet — that's fine).

- [ ] **Step 6: Commit**

```bash
git add java/build.gradle java/settings.gradle java/gradlew java/gradlew.bat java/gradle/ java/config/
git commit -m "feat(java): scaffold Gradle multi-module project"
```

---

### Task 2: Docker Compose for Infrastructure

**Files:**
- Create: `java/docker-compose.yml`
- Create: `java/.env.example`

- [ ] **Step 1: Write docker-compose.yml**

Create `java/docker-compose.yml`:

```yaml
services:
  postgres:
    image: postgres:17-alpine
    ports:
      - "5432:5432"
    environment:
      POSTGRES_DB: taskdb
      POSTGRES_USER: ${POSTGRES_USER:-taskuser}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-taskpass}
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U taskuser -d taskdb"]
      interval: 5s
      timeout: 3s
      retries: 5

  mongodb:
    image: mongo:7
    ports:
      - "27017:27017"
    volumes:
      - mongo_data:/data/db
    healthcheck:
      test: ["CMD", "mongosh", "--eval", "db.adminCommand('ping')"]
      interval: 5s
      timeout: 3s
      retries: 5

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 5

  rabbitmq:
    image: rabbitmq:3-management-alpine
    ports:
      - "5672:5672"
      - "15672:15672"
    environment:
      RABBITMQ_DEFAULT_USER: ${RABBITMQ_USER:-guest}
      RABBITMQ_DEFAULT_PASS: ${RABBITMQ_PASSWORD:-guest}
    volumes:
      - rabbitmq_data:/var/lib/rabbitmq
    healthcheck:
      test: ["CMD", "rabbitmq-diagnostics", "-q", "ping"]
      interval: 5s
      timeout: 3s
      retries: 5

volumes:
  postgres_data:
  mongo_data:
  redis_data:
  rabbitmq_data:
```

- [ ] **Step 2: Write .env.example**

Create `java/.env.example`:

```bash
# PostgreSQL
POSTGRES_USER=taskuser
POSTGRES_PASSWORD=taskpass

# RabbitMQ
RABBITMQ_USER=guest
RABBITMQ_PASSWORD=guest

# Google OAuth (task-service)
GOOGLE_CLIENT_ID=your-google-client-id
GOOGLE_CLIENT_SECRET=your-google-client-secret

# JWT
JWT_SECRET=your-jwt-secret-at-least-32-characters-long
```

- [ ] **Step 3: Commit**

```bash
git add java/docker-compose.yml java/.env.example
git commit -m "feat(java): add Docker Compose for PostgreSQL, MongoDB, Redis, RabbitMQ"
```

---

### Task 3: Spring Boot Application Shells

**Files:**
- Create: `java/task-service/build.gradle`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/TaskServiceApplication.java`
- Create: `java/task-service/src/main/resources/application.yml`
- Create: `java/activity-service/build.gradle`
- Create: `java/activity-service/src/main/java/dev/kylebradshaw/activity/ActivityServiceApplication.java`
- Create: `java/activity-service/src/main/resources/application.yml`
- Create: `java/notification-service/build.gradle`
- Create: `java/notification-service/src/main/java/dev/kylebradshaw/notification/NotificationServiceApplication.java`
- Create: `java/notification-service/src/main/resources/application.yml`
- Create: `java/gateway-service/build.gradle`
- Create: `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/GatewayServiceApplication.java`
- Create: `java/gateway-service/src/main/resources/application.yml`

- [ ] **Step 1: Write task-service/build.gradle**

```groovy
dependencies {
    implementation 'org.springframework.boot:spring-boot-starter-data-jpa'
    implementation 'org.springframework.boot:spring-boot-starter-security'
    implementation 'org.springframework.boot:spring-boot-starter-amqp'
    implementation 'org.springframework.boot:spring-boot-starter-validation'
    runtimeOnly 'org.postgresql:postgresql'

    implementation 'io.jsonwebtoken:jjwt-api:0.12.6'
    runtimeOnly 'io.jsonwebtoken:jjwt-impl:0.12.6'
    runtimeOnly 'io.jsonwebtoken:jjwt-jackson:0.12.6'

    testImplementation 'org.springframework.security:spring-security-test'
    testImplementation 'org.testcontainers:junit-jupiter'
    testImplementation 'org.testcontainers:postgresql'
    testImplementation 'org.testcontainers:rabbitmq'
}
```

- [ ] **Step 2: Write TaskServiceApplication.java**

Create `java/task-service/src/main/java/dev/kylebradshaw/task/TaskServiceApplication.java`:

```java
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

- [ ] **Step 3: Write task-service application.yml**

Create `java/task-service/src/main/resources/application.yml`:

```yaml
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
    access-token-ttl: 15m
    refresh-token-ttl: 7d
  google:
    client-id: ${GOOGLE_CLIENT_ID:}
    client-secret: ${GOOGLE_CLIENT_SECRET:}
    token-url: https://oauth2.googleapis.com/token
    userinfo-url: https://www.googleapis.com/oauth2/v3/userinfo
  allowed-origins: ${ALLOWED_ORIGINS:http://localhost:3000}
```

- [ ] **Step 4: Write activity-service/build.gradle**

```groovy
dependencies {
    implementation 'org.springframework.boot:spring-boot-starter-data-mongodb'
    implementation 'org.springframework.boot:spring-boot-starter-amqp'
    implementation 'org.springframework.boot:spring-boot-starter-validation'

    testImplementation 'org.testcontainers:junit-jupiter'
    testImplementation 'org.testcontainers:mongodb'
    testImplementation 'org.testcontainers:rabbitmq'
}
```

- [ ] **Step 5: Write ActivityServiceApplication.java**

Create `java/activity-service/src/main/java/dev/kylebradshaw/activity/ActivityServiceApplication.java`:

```java
package dev.kylebradshaw.activity;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

@SpringBootApplication
public class ActivityServiceApplication {

    public static void main(String[] args) {
        SpringApplication.run(ActivityServiceApplication.class, args);
    }
}
```

- [ ] **Step 6: Write activity-service application.yml**

Create `java/activity-service/src/main/resources/application.yml`:

```yaml
server:
  port: 8082

spring:
  data:
    mongodb:
      uri: mongodb://${MONGODB_HOST:localhost}:27017/activitydb
  rabbitmq:
    host: ${RABBITMQ_HOST:localhost}
    port: 5672
    username: ${RABBITMQ_USER:guest}
    password: ${RABBITMQ_PASSWORD:guest}
```

- [ ] **Step 7: Write notification-service/build.gradle**

```groovy
dependencies {
    implementation 'org.springframework.boot:spring-boot-starter-data-redis'
    implementation 'org.springframework.boot:spring-boot-starter-amqp'
    implementation 'org.springframework.boot:spring-boot-starter-validation'

    testImplementation 'org.testcontainers:junit-jupiter'
    testImplementation 'org.testcontainers:testcontainers'
}
```

- [ ] **Step 8: Write NotificationServiceApplication.java**

Create `java/notification-service/src/main/java/dev/kylebradshaw/notification/NotificationServiceApplication.java`:

```java
package dev.kylebradshaw.notification;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

@SpringBootApplication
public class NotificationServiceApplication {

    public static void main(String[] args) {
        SpringApplication.run(NotificationServiceApplication.class, args);
    }
}
```

- [ ] **Step 9: Write notification-service application.yml**

Create `java/notification-service/src/main/resources/application.yml`:

```yaml
server:
  port: 8083

spring:
  data:
    redis:
      host: ${REDIS_HOST:localhost}
      port: 6379
  rabbitmq:
    host: ${RABBITMQ_HOST:localhost}
    port: 5672
    username: ${RABBITMQ_USER:guest}
    password: ${RABBITMQ_PASSWORD:guest}
```

- [ ] **Step 10: Write gateway-service/build.gradle**

```groovy
dependencies {
    implementation 'org.springframework.boot:spring-boot-starter-graphql'
    implementation 'org.springframework.boot:spring-boot-starter-security'
    implementation 'org.springframework.boot:spring-boot-starter-validation'

    implementation 'io.jsonwebtoken:jjwt-api:0.12.6'
    runtimeOnly 'io.jsonwebtoken:jjwt-impl:0.12.6'
    runtimeOnly 'io.jsonwebtoken:jjwt-jackson:0.12.6'

    testImplementation 'org.springframework.security:spring-security-test'
    testImplementation 'org.springframework.graphql:spring-graphql-test'
}
```

- [ ] **Step 11: Write GatewayServiceApplication.java**

Create `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/GatewayServiceApplication.java`:

```java
package dev.kylebradshaw.gateway;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

@SpringBootApplication
public class GatewayServiceApplication {

    public static void main(String[] args) {
        SpringApplication.run(GatewayServiceApplication.class, args);
    }
}
```

- [ ] **Step 12: Write gateway-service application.yml**

Create `java/gateway-service/src/main/resources/application.yml`:

```yaml
server:
  port: 8080

spring:
  graphql:
    graphiql:
      enabled: true
    schema:
      locations: classpath:graphql/

app:
  jwt:
    secret: ${JWT_SECRET:dev-secret-key-at-least-32-characters-long}
  services:
    task-url: ${TASK_SERVICE_URL:http://localhost:8081}
    activity-url: ${ACTIVITY_SERVICE_URL:http://localhost:8082}
    notification-url: ${NOTIFICATION_SERVICE_URL:http://localhost:8083}
  allowed-origins: ${ALLOWED_ORIGINS:http://localhost:3000}
```

- [ ] **Step 13: Verify all modules compile**

```bash
cd java && ./gradlew compileJava --no-daemon
```

Expected: BUILD SUCCESSFUL — all 4 modules compile.

- [ ] **Step 14: Commit**

```bash
git add java/task-service/ java/activity-service/ java/notification-service/ java/gateway-service/
git commit -m "feat(java): add Spring Boot application shells for all 4 services"
```

---

## Phase 2: task-service Domain Layer

### Task 4: Enums and JPA Entities

**Files:**
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/entity/TaskStatus.java`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/entity/TaskPriority.java`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/entity/ProjectRole.java`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/entity/User.java`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/entity/Project.java`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/entity/ProjectMember.java`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/entity/Task.java`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/entity/RefreshToken.java`

- [ ] **Step 1: Write enums**

Create `java/task-service/src/main/java/dev/kylebradshaw/task/entity/TaskStatus.java`:

```java
package dev.kylebradshaw.task.entity;

public enum TaskStatus {
    TODO, IN_PROGRESS, DONE
}
```

Create `java/task-service/src/main/java/dev/kylebradshaw/task/entity/TaskPriority.java`:

```java
package dev.kylebradshaw.task.entity;

public enum TaskPriority {
    LOW, MEDIUM, HIGH
}
```

Create `java/task-service/src/main/java/dev/kylebradshaw/task/entity/ProjectRole.java`:

```java
package dev.kylebradshaw.task.entity;

public enum ProjectRole {
    OWNER, MEMBER
}
```

- [ ] **Step 2: Write User entity**

Create `java/task-service/src/main/java/dev/kylebradshaw/task/entity/User.java`:

```java
package dev.kylebradshaw.task.entity;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.GeneratedValue;
import jakarta.persistence.GenerationType;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import java.time.Instant;
import java.util.UUID;

@Entity
@Table(name = "users")
public class User {

    @Id
    @GeneratedValue(strategy = GenerationType.UUID)
    private UUID id;

    @Column(unique = true, nullable = false)
    private String email;

    @Column(nullable = false)
    private String name;

    @Column(name = "avatar_url")
    private String avatarUrl;

    @Column(name = "created_at", updatable = false)
    private Instant createdAt = Instant.now();

    protected User() {
    }

    public User(String email, String name, String avatarUrl) {
        this.email = email;
        this.name = name;
        this.avatarUrl = avatarUrl;
    }

    public UUID getId() {
        return id;
    }

    public String getEmail() {
        return email;
    }

    public String getName() {
        return name;
    }

    public void setName(String name) {
        this.name = name;
    }

    public String getAvatarUrl() {
        return avatarUrl;
    }

    public void setAvatarUrl(String avatarUrl) {
        this.avatarUrl = avatarUrl;
    }

    public Instant getCreatedAt() {
        return createdAt;
    }
}
```

- [ ] **Step 3: Write Project entity**

Create `java/task-service/src/main/java/dev/kylebradshaw/task/entity/Project.java`:

```java
package dev.kylebradshaw.task.entity;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.FetchType;
import jakarta.persistence.GeneratedValue;
import jakarta.persistence.GenerationType;
import jakarta.persistence.Id;
import jakarta.persistence.JoinColumn;
import jakarta.persistence.ManyToOne;
import jakarta.persistence.Table;
import java.time.Instant;
import java.util.UUID;

@Entity
@Table(name = "projects")
public class Project {

    @Id
    @GeneratedValue(strategy = GenerationType.UUID)
    private UUID id;

    @Column(nullable = false)
    private String name;

    private String description;

    @ManyToOne(fetch = FetchType.LAZY)
    @JoinColumn(name = "owner_id", nullable = false)
    private User owner;

    @Column(name = "created_at", updatable = false)
    private Instant createdAt = Instant.now();

    protected Project() {
    }

    public Project(String name, String description, User owner) {
        this.name = name;
        this.description = description;
        this.owner = owner;
    }

    public UUID getId() {
        return id;
    }

    public String getName() {
        return name;
    }

    public void setName(String name) {
        this.name = name;
    }

    public String getDescription() {
        return description;
    }

    public void setDescription(String description) {
        this.description = description;
    }

    public User getOwner() {
        return owner;
    }

    public Instant getCreatedAt() {
        return createdAt;
    }
}
```

- [ ] **Step 4: Write ProjectMember entity**

Create `java/task-service/src/main/java/dev/kylebradshaw/task/entity/ProjectMember.java`:

```java
package dev.kylebradshaw.task.entity;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.EnumType;
import jakarta.persistence.Enumerated;
import jakarta.persistence.FetchType;
import jakarta.persistence.Id;
import jakarta.persistence.IdClass;
import jakarta.persistence.JoinColumn;
import jakarta.persistence.ManyToOne;
import jakarta.persistence.Table;
import java.io.Serializable;
import java.util.Objects;
import java.util.UUID;

@Entity
@Table(name = "project_members")
@IdClass(ProjectMember.ProjectMemberId.class)
public class ProjectMember {

    @Id
    @Column(name = "project_id")
    private UUID projectId;

    @Id
    @Column(name = "user_id")
    private UUID userId;

    @ManyToOne(fetch = FetchType.LAZY)
    @JoinColumn(name = "project_id", insertable = false, updatable = false)
    private Project project;

    @ManyToOne(fetch = FetchType.LAZY)
    @JoinColumn(name = "user_id", insertable = false, updatable = false)
    private User user;

    @Enumerated(EnumType.STRING)
    @Column(nullable = false)
    private ProjectRole role;

    protected ProjectMember() {
    }

    public ProjectMember(UUID projectId, UUID userId, ProjectRole role) {
        this.projectId = projectId;
        this.userId = userId;
        this.role = role;
    }

    public UUID getProjectId() {
        return projectId;
    }

    public UUID getUserId() {
        return userId;
    }

    public ProjectRole getRole() {
        return role;
    }

    public void setRole(ProjectRole role) {
        this.role = role;
    }

    public static class ProjectMemberId implements Serializable {
        private UUID projectId;
        private UUID userId;

        public ProjectMemberId() {
        }

        public ProjectMemberId(UUID projectId, UUID userId) {
            this.projectId = projectId;
            this.userId = userId;
        }

        @Override
        public boolean equals(Object o) {
            if (this == o) return true;
            if (!(o instanceof ProjectMemberId that)) return false;
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

- [ ] **Step 5: Write Task entity**

Create `java/task-service/src/main/java/dev/kylebradshaw/task/entity/Task.java`:

```java
package dev.kylebradshaw.task.entity;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.EnumType;
import jakarta.persistence.Enumerated;
import jakarta.persistence.FetchType;
import jakarta.persistence.GeneratedValue;
import jakarta.persistence.GenerationType;
import jakarta.persistence.Id;
import jakarta.persistence.JoinColumn;
import jakarta.persistence.ManyToOne;
import jakarta.persistence.Table;
import java.time.Instant;
import java.time.LocalDate;
import java.util.UUID;

@Entity
@Table(name = "tasks")
public class Task {

    @Id
    @GeneratedValue(strategy = GenerationType.UUID)
    private UUID id;

    @ManyToOne(fetch = FetchType.LAZY)
    @JoinColumn(name = "project_id", nullable = false)
    private Project project;

    @Column(nullable = false)
    private String title;

    private String description;

    @Enumerated(EnumType.STRING)
    @Column(nullable = false)
    private TaskStatus status = TaskStatus.TODO;

    @Enumerated(EnumType.STRING)
    @Column(nullable = false)
    private TaskPriority priority = TaskPriority.MEDIUM;

    @ManyToOne(fetch = FetchType.LAZY)
    @JoinColumn(name = "assignee_id")
    private User assignee;

    @Column(name = "due_date")
    private LocalDate dueDate;

    @Column(name = "created_at", updatable = false)
    private Instant createdAt = Instant.now();

    @Column(name = "updated_at")
    private Instant updatedAt = Instant.now();

    protected Task() {
    }

    public Task(Project project, String title, String description,
                TaskPriority priority, LocalDate dueDate) {
        this.project = project;
        this.title = title;
        this.description = description;
        this.priority = priority;
        this.dueDate = dueDate;
    }

    public UUID getId() {
        return id;
    }

    public Project getProject() {
        return project;
    }

    public String getTitle() {
        return title;
    }

    public void setTitle(String title) {
        this.title = title;
        this.updatedAt = Instant.now();
    }

    public String getDescription() {
        return description;
    }

    public void setDescription(String description) {
        this.description = description;
        this.updatedAt = Instant.now();
    }

    public TaskStatus getStatus() {
        return status;
    }

    public void setStatus(TaskStatus status) {
        this.status = status;
        this.updatedAt = Instant.now();
    }

    public TaskPriority getPriority() {
        return priority;
    }

    public void setPriority(TaskPriority priority) {
        this.priority = priority;
        this.updatedAt = Instant.now();
    }

    public User getAssignee() {
        return assignee;
    }

    public void setAssignee(User assignee) {
        this.assignee = assignee;
        this.updatedAt = Instant.now();
    }

    public LocalDate getDueDate() {
        return dueDate;
    }

    public void setDueDate(LocalDate dueDate) {
        this.dueDate = dueDate;
        this.updatedAt = Instant.now();
    }

    public Instant getCreatedAt() {
        return createdAt;
    }

    public Instant getUpdatedAt() {
        return updatedAt;
    }
}
```

- [ ] **Step 6: Write RefreshToken entity**

Create `java/task-service/src/main/java/dev/kylebradshaw/task/entity/RefreshToken.java`:

```java
package dev.kylebradshaw.task.entity;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.FetchType;
import jakarta.persistence.GeneratedValue;
import jakarta.persistence.GenerationType;
import jakarta.persistence.Id;
import jakarta.persistence.JoinColumn;
import jakarta.persistence.ManyToOne;
import jakarta.persistence.Table;
import java.time.Instant;
import java.util.UUID;

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

    protected RefreshToken() {
    }

    public RefreshToken(User user, String token, Instant expiresAt) {
        this.user = user;
        this.token = token;
        this.expiresAt = expiresAt;
    }

    public UUID getId() {
        return id;
    }

    public User getUser() {
        return user;
    }

    public String getToken() {
        return token;
    }

    public Instant getExpiresAt() {
        return expiresAt;
    }

    public boolean isExpired() {
        return Instant.now().isAfter(expiresAt);
    }
}
```

- [ ] **Step 7: Verify compilation**

```bash
cd java && ./gradlew :task-service:compileJava --no-daemon
```

Expected: BUILD SUCCESSFUL.

- [ ] **Step 8: Commit**

```bash
git add java/task-service/src/main/java/dev/kylebradshaw/task/entity/
git commit -m "feat(task-service): add JPA entities — User, Project, Task, ProjectMember, RefreshToken"
```

---

### Task 5: Spring Data JPA Repositories

**Files:**
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/repository/UserRepository.java`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/repository/ProjectRepository.java`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/repository/ProjectMemberRepository.java`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/repository/TaskRepository.java`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/repository/RefreshTokenRepository.java`

- [ ] **Step 1: Write all repositories**

Create `java/task-service/src/main/java/dev/kylebradshaw/task/repository/UserRepository.java`:

```java
package dev.kylebradshaw.task.repository;

import dev.kylebradshaw.task.entity.User;
import java.util.Optional;
import java.util.UUID;
import org.springframework.data.jpa.repository.JpaRepository;

public interface UserRepository extends JpaRepository<User, UUID> {
    Optional<User> findByEmail(String email);
}
```

Create `java/task-service/src/main/java/dev/kylebradshaw/task/repository/ProjectRepository.java`:

```java
package dev.kylebradshaw.task.repository;

import dev.kylebradshaw.task.entity.Project;
import java.util.UUID;
import org.springframework.data.jpa.repository.JpaRepository;

public interface ProjectRepository extends JpaRepository<Project, UUID> {
}
```

Create `java/task-service/src/main/java/dev/kylebradshaw/task/repository/ProjectMemberRepository.java`:

```java
package dev.kylebradshaw.task.repository;

import dev.kylebradshaw.task.entity.ProjectMember;
import dev.kylebradshaw.task.entity.ProjectRole;
import java.util.List;
import java.util.Optional;
import java.util.UUID;
import org.springframework.data.jpa.repository.JpaRepository;

public interface ProjectMemberRepository
        extends JpaRepository<ProjectMember, ProjectMember.ProjectMemberId> {

    List<ProjectMember> findByUserId(UUID userId);

    Optional<ProjectMember> findByProjectIdAndUserId(UUID projectId, UUID userId);

    boolean existsByProjectIdAndUserIdAndRole(UUID projectId, UUID userId, ProjectRole role);
}
```

Create `java/task-service/src/main/java/dev/kylebradshaw/task/repository/TaskRepository.java`:

```java
package dev.kylebradshaw.task.repository;

import dev.kylebradshaw.task.entity.Task;
import java.util.List;
import java.util.UUID;
import org.springframework.data.jpa.repository.JpaRepository;

public interface TaskRepository extends JpaRepository<Task, UUID> {
    List<Task> findByProjectId(UUID projectId);
}
```

Create `java/task-service/src/main/java/dev/kylebradshaw/task/repository/RefreshTokenRepository.java`:

```java
package dev.kylebradshaw.task.repository;

import dev.kylebradshaw.task.entity.RefreshToken;
import java.util.Optional;
import java.util.UUID;
import org.springframework.data.jpa.repository.JpaRepository;

public interface RefreshTokenRepository extends JpaRepository<RefreshToken, UUID> {
    Optional<RefreshToken> findByToken(String token);
    void deleteByUserId(UUID userId);
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd java && ./gradlew :task-service:compileJava --no-daemon
```

Expected: BUILD SUCCESSFUL.

- [ ] **Step 3: Commit**

```bash
git add java/task-service/src/main/java/dev/kylebradshaw/task/repository/
git commit -m "feat(task-service): add Spring Data JPA repositories"
```

---

### Task 6: DTOs and TaskEventMessage

**Files:**
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/dto/CreateProjectRequest.java`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/dto/ProjectResponse.java`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/dto/CreateTaskRequest.java`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/dto/UpdateTaskRequest.java`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/dto/TaskResponse.java`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/dto/TaskEventMessage.java`

- [ ] **Step 1: Write all DTOs**

Create `java/task-service/src/main/java/dev/kylebradshaw/task/dto/CreateProjectRequest.java`:

```java
package dev.kylebradshaw.task.dto;

import jakarta.validation.constraints.NotBlank;

public record CreateProjectRequest(
        @NotBlank String name,
        String description
) {
}
```

Create `java/task-service/src/main/java/dev/kylebradshaw/task/dto/ProjectResponse.java`:

```java
package dev.kylebradshaw.task.dto;

import dev.kylebradshaw.task.entity.Project;
import java.time.Instant;
import java.util.UUID;

public record ProjectResponse(
        UUID id,
        String name,
        String description,
        UUID ownerId,
        String ownerName,
        Instant createdAt
) {
    public static ProjectResponse from(Project project) {
        return new ProjectResponse(
                project.getId(),
                project.getName(),
                project.getDescription(),
                project.getOwner().getId(),
                project.getOwner().getName(),
                project.getCreatedAt()
        );
    }
}
```

Create `java/task-service/src/main/java/dev/kylebradshaw/task/dto/CreateTaskRequest.java`:

```java
package dev.kylebradshaw.task.dto;

import dev.kylebradshaw.task.entity.TaskPriority;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotNull;
import java.time.LocalDate;
import java.util.UUID;

public record CreateTaskRequest(
        @NotNull UUID projectId,
        @NotBlank String title,
        String description,
        TaskPriority priority,
        LocalDate dueDate
) {
}
```

Create `java/task-service/src/main/java/dev/kylebradshaw/task/dto/UpdateTaskRequest.java`:

```java
package dev.kylebradshaw.task.dto;

import dev.kylebradshaw.task.entity.TaskPriority;
import dev.kylebradshaw.task.entity.TaskStatus;
import java.time.LocalDate;

public record UpdateTaskRequest(
        String title,
        String description,
        TaskStatus status,
        TaskPriority priority,
        LocalDate dueDate
) {
}
```

Create `java/task-service/src/main/java/dev/kylebradshaw/task/dto/TaskResponse.java`:

```java
package dev.kylebradshaw.task.dto;

import dev.kylebradshaw.task.entity.Task;
import dev.kylebradshaw.task.entity.TaskPriority;
import dev.kylebradshaw.task.entity.TaskStatus;
import java.time.Instant;
import java.time.LocalDate;
import java.util.UUID;

public record TaskResponse(
        UUID id,
        UUID projectId,
        String title,
        String description,
        TaskStatus status,
        TaskPriority priority,
        UUID assigneeId,
        String assigneeName,
        LocalDate dueDate,
        Instant createdAt,
        Instant updatedAt
) {
    public static TaskResponse from(Task task) {
        return new TaskResponse(
                task.getId(),
                task.getProject().getId(),
                task.getTitle(),
                task.getDescription(),
                task.getStatus(),
                task.getPriority(),
                task.getAssignee() != null ? task.getAssignee().getId() : null,
                task.getAssignee() != null ? task.getAssignee().getName() : null,
                task.getDueDate(),
                task.getCreatedAt(),
                task.getUpdatedAt()
        );
    }
}
```

Create `java/task-service/src/main/java/dev/kylebradshaw/task/dto/TaskEventMessage.java`:

```java
package dev.kylebradshaw.task.dto;

import java.time.Instant;
import java.util.Map;
import java.util.UUID;

public record TaskEventMessage(
        UUID eventId,
        String eventType,
        Instant timestamp,
        UUID actorId,
        UUID projectId,
        UUID taskId,
        Map<String, Object> data
) {
    public static TaskEventMessage of(String eventType, UUID actorId,
                                       UUID projectId, UUID taskId,
                                       Map<String, Object> data) {
        return new TaskEventMessage(
                UUID.randomUUID(), eventType, Instant.now(),
                actorId, projectId, taskId, data
        );
    }
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd java && ./gradlew :task-service:compileJava --no-daemon
```

- [ ] **Step 3: Commit**

```bash
git add java/task-service/src/main/java/dev/kylebradshaw/task/dto/
git commit -m "feat(task-service): add request/response DTOs and TaskEventMessage"
```

---

### Task 7: RabbitMQ Config and TaskEventPublisher

**Files:**
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/config/RabbitConfig.java`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/service/TaskEventPublisher.java`

- [ ] **Step 1: Write RabbitConfig**

Create `java/task-service/src/main/java/dev/kylebradshaw/task/config/RabbitConfig.java`:

```java
package dev.kylebradshaw.task.config;

import org.springframework.amqp.core.TopicExchange;
import org.springframework.amqp.support.converter.Jackson2JsonMessageConverter;
import org.springframework.amqp.support.converter.MessageConverter;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
public class RabbitConfig {

    public static final String EXCHANGE_NAME = "task.events";

    @Bean
    public TopicExchange taskExchange() {
        return new TopicExchange(EXCHANGE_NAME);
    }

    @Bean
    public MessageConverter jsonMessageConverter() {
        return new Jackson2JsonMessageConverter();
    }
}
```

- [ ] **Step 2: Write TaskEventPublisher**

Create `java/task-service/src/main/java/dev/kylebradshaw/task/service/TaskEventPublisher.java`:

```java
package dev.kylebradshaw.task.service;

import dev.kylebradshaw.task.config.RabbitConfig;
import dev.kylebradshaw.task.dto.TaskEventMessage;
import org.springframework.amqp.rabbit.core.RabbitTemplate;
import org.springframework.stereotype.Component;

@Component
public class TaskEventPublisher {

    private final RabbitTemplate rabbitTemplate;

    public TaskEventPublisher(RabbitTemplate rabbitTemplate) {
        this.rabbitTemplate = rabbitTemplate;
    }

    public void publish(String routingKey, TaskEventMessage message) {
        rabbitTemplate.convertAndSend(RabbitConfig.EXCHANGE_NAME, routingKey, message);
    }
}
```

- [ ] **Step 3: Commit**

```bash
git add java/task-service/src/main/java/dev/kylebradshaw/task/config/RabbitConfig.java \
        java/task-service/src/main/java/dev/kylebradshaw/task/service/TaskEventPublisher.java
git commit -m "feat(task-service): add RabbitMQ config and event publisher"
```

---

### Task 8: ProjectService with TDD

**Files:**
- Create: `java/task-service/src/test/java/dev/kylebradshaw/task/service/ProjectServiceTest.java`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/service/ProjectService.java`

- [ ] **Step 1: Write the failing test**

Create `java/task-service/src/test/java/dev/kylebradshaw/task/service/ProjectServiceTest.java`:

```java
package dev.kylebradshaw.task.service;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

import dev.kylebradshaw.task.dto.CreateProjectRequest;
import dev.kylebradshaw.task.entity.Project;
import dev.kylebradshaw.task.entity.ProjectMember;
import dev.kylebradshaw.task.entity.ProjectRole;
import dev.kylebradshaw.task.entity.User;
import dev.kylebradshaw.task.repository.ProjectMemberRepository;
import dev.kylebradshaw.task.repository.ProjectRepository;
import dev.kylebradshaw.task.repository.UserRepository;
import java.util.List;
import java.util.Optional;
import java.util.UUID;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.ArgumentCaptor;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

@ExtendWith(MockitoExtension.class)
class ProjectServiceTest {

    @Mock
    private ProjectRepository projectRepo;

    @Mock
    private ProjectMemberRepository memberRepo;

    @Mock
    private UserRepository userRepo;

    private ProjectService service;

    @BeforeEach
    void setUp() {
        service = new ProjectService(projectRepo, memberRepo, userRepo);
    }

    @Test
    void createProject_savesProjectAndAddsMemberAsOwner() {
        UUID userId = UUID.randomUUID();
        User user = new User("test@example.com", "Test User", null);
        when(userRepo.findById(userId)).thenReturn(Optional.of(user));
        when(projectRepo.save(any(Project.class))).thenAnswer(inv -> inv.getArgument(0));

        var request = new CreateProjectRequest("My Project", "A description");
        Project result = service.createProject(request, userId);

        assertThat(result.getName()).isEqualTo("My Project");
        assertThat(result.getDescription()).isEqualTo("A description");
        assertThat(result.getOwner()).isEqualTo(user);

        ArgumentCaptor<ProjectMember> memberCaptor = ArgumentCaptor.forClass(ProjectMember.class);
        verify(memberRepo).save(memberCaptor.capture());
        assertThat(memberCaptor.getValue().getRole()).isEqualTo(ProjectRole.OWNER);
    }

    @Test
    void getProjectsForUser_returnsProjectsWhereUserIsMember() {
        UUID userId = UUID.randomUUID();
        UUID projectId = UUID.randomUUID();
        var membership = new ProjectMember(projectId, userId, ProjectRole.OWNER);
        when(memberRepo.findByUserId(userId)).thenReturn(List.of(membership));

        User owner = new User("test@example.com", "Test User", null);
        Project project = new Project("Project", "Desc", owner);
        when(projectRepo.findAllById(List.of(projectId))).thenReturn(List.of(project));

        List<Project> result = service.getProjectsForUser(userId);
        assertThat(result).hasSize(1);
        assertThat(result.getFirst().getName()).isEqualTo("Project");
    }

    @Test
    void deleteProject_whenNotOwner_throws() {
        UUID projectId = UUID.randomUUID();
        UUID userId = UUID.randomUUID();
        when(memberRepo.existsByProjectIdAndUserIdAndRole(projectId, userId, ProjectRole.OWNER))
                .thenReturn(false);

        assertThatThrownBy(() -> service.deleteProject(projectId, userId))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("Only the owner");
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd java && ./gradlew :task-service:test --tests "dev.kylebradshaw.task.service.ProjectServiceTest" --no-daemon
```

Expected: FAIL — `ProjectService` does not exist yet.

- [ ] **Step 3: Write ProjectService**

Create `java/task-service/src/main/java/dev/kylebradshaw/task/service/ProjectService.java`:

```java
package dev.kylebradshaw.task.service;

import dev.kylebradshaw.task.dto.CreateProjectRequest;
import dev.kylebradshaw.task.entity.Project;
import dev.kylebradshaw.task.entity.ProjectMember;
import dev.kylebradshaw.task.entity.ProjectRole;
import dev.kylebradshaw.task.entity.User;
import dev.kylebradshaw.task.repository.ProjectMemberRepository;
import dev.kylebradshaw.task.repository.ProjectRepository;
import dev.kylebradshaw.task.repository.UserRepository;
import java.util.List;
import java.util.UUID;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

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

    @Transactional
    public Project createProject(CreateProjectRequest request, UUID userId) {
        User owner = userRepo.findById(userId)
                .orElseThrow(() -> new IllegalArgumentException("User not found"));
        Project project = new Project(request.name(), request.description(), owner);
        project = projectRepo.save(project);

        var member = new ProjectMember(project.getId(), userId, ProjectRole.OWNER);
        memberRepo.save(member);
        return project;
    }

    public List<Project> getProjectsForUser(UUID userId) {
        List<UUID> projectIds = memberRepo.findByUserId(userId)
                .stream()
                .map(ProjectMember::getProjectId)
                .toList();
        return projectRepo.findAllById(projectIds);
    }

    public Project getProject(UUID projectId) {
        return projectRepo.findById(projectId)
                .orElseThrow(() -> new IllegalArgumentException("Project not found"));
    }

    @Transactional
    public Project updateProject(UUID projectId, UUID userId, String name, String description) {
        if (!memberRepo.existsByProjectIdAndUserIdAndRole(projectId, userId, ProjectRole.OWNER)) {
            throw new IllegalStateException("Only the owner can update the project");
        }
        Project project = getProject(projectId);
        if (name != null) {
            project.setName(name);
        }
        if (description != null) {
            project.setDescription(description);
        }
        return projectRepo.save(project);
    }

    @Transactional
    public void deleteProject(UUID projectId, UUID userId) {
        if (!memberRepo.existsByProjectIdAndUserIdAndRole(projectId, userId, ProjectRole.OWNER)) {
            throw new IllegalStateException("Only the owner can delete the project");
        }
        projectRepo.deleteById(projectId);
    }
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd java && ./gradlew :task-service:test --tests "dev.kylebradshaw.task.service.ProjectServiceTest" --no-daemon
```

Expected: All 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add java/task-service/src/test/java/dev/kylebradshaw/task/service/ProjectServiceTest.java \
        java/task-service/src/main/java/dev/kylebradshaw/task/service/ProjectService.java
git commit -m "feat(task-service): add ProjectService with unit tests"
```

---

### Task 9: TaskService with TDD

**Files:**
- Create: `java/task-service/src/test/java/dev/kylebradshaw/task/service/TaskServiceTest.java`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/service/TaskService.java`

- [ ] **Step 1: Write the failing test**

Create `java/task-service/src/test/java/dev/kylebradshaw/task/service/TaskServiceTest.java`:

```java
package dev.kylebradshaw.task.service;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

import dev.kylebradshaw.task.dto.CreateTaskRequest;
import dev.kylebradshaw.task.dto.TaskEventMessage;
import dev.kylebradshaw.task.dto.UpdateTaskRequest;
import dev.kylebradshaw.task.entity.Project;
import dev.kylebradshaw.task.entity.Task;
import dev.kylebradshaw.task.entity.TaskPriority;
import dev.kylebradshaw.task.entity.TaskStatus;
import dev.kylebradshaw.task.entity.User;
import dev.kylebradshaw.task.repository.ProjectRepository;
import dev.kylebradshaw.task.repository.TaskRepository;
import dev.kylebradshaw.task.repository.UserRepository;
import java.util.List;
import java.util.Optional;
import java.util.UUID;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

@ExtendWith(MockitoExtension.class)
class TaskServiceTest {

    @Mock
    private TaskRepository taskRepo;

    @Mock
    private ProjectRepository projectRepo;

    @Mock
    private UserRepository userRepo;

    @Mock
    private TaskEventPublisher eventPublisher;

    private TaskService service;

    @BeforeEach
    void setUp() {
        service = new TaskService(taskRepo, projectRepo, userRepo, eventPublisher);
    }

    @Test
    void createTask_savesAndPublishesEvent() {
        UUID userId = UUID.randomUUID();
        User owner = new User("test@example.com", "Owner", null);
        Project project = new Project("Project", "Desc", owner);
        UUID projectId = UUID.randomUUID();

        when(projectRepo.findById(projectId)).thenReturn(Optional.of(project));
        when(taskRepo.save(any(Task.class))).thenAnswer(inv -> inv.getArgument(0));

        var request = new CreateTaskRequest(projectId, "Fix bug", "Fix the login bug",
                TaskPriority.HIGH, null);
        Task result = service.createTask(request, userId);

        assertThat(result.getTitle()).isEqualTo("Fix bug");
        assertThat(result.getPriority()).isEqualTo(TaskPriority.HIGH);
        verify(eventPublisher).publish(eq("task.created"), any(TaskEventMessage.class));
    }

    @Test
    void updateTask_changesFieldsAndPublishesStatusEvent() {
        UUID userId = UUID.randomUUID();
        User owner = new User("test@example.com", "Owner", null);
        Project project = new Project("Project", "Desc", owner);
        Task task = new Task(project, "Old title", "Old desc", TaskPriority.LOW, null);
        UUID taskId = UUID.randomUUID();

        when(taskRepo.findById(taskId)).thenReturn(Optional.of(task));
        when(taskRepo.save(any(Task.class))).thenAnswer(inv -> inv.getArgument(0));

        var request = new UpdateTaskRequest("New title", null, TaskStatus.IN_PROGRESS, null, null);
        Task result = service.updateTask(taskId, request, userId);

        assertThat(result.getTitle()).isEqualTo("New title");
        assertThat(result.getStatus()).isEqualTo(TaskStatus.IN_PROGRESS);
        verify(eventPublisher).publish(eq("task.status_changed"), any(TaskEventMessage.class));
    }

    @Test
    void assignTask_setsAssigneeAndPublishesEvent() {
        UUID userId = UUID.randomUUID();
        UUID assigneeId = UUID.randomUUID();
        User owner = new User("test@example.com", "Owner", null);
        User assignee = new User("dev@example.com", "Developer", null);
        Project project = new Project("Project", "Desc", owner);
        Task task = new Task(project, "Task", "Desc", TaskPriority.MEDIUM, null);
        UUID taskId = UUID.randomUUID();

        when(taskRepo.findById(taskId)).thenReturn(Optional.of(task));
        when(userRepo.findById(assigneeId)).thenReturn(Optional.of(assignee));
        when(taskRepo.save(any(Task.class))).thenAnswer(inv -> inv.getArgument(0));

        Task result = service.assignTask(taskId, assigneeId, userId);

        assertThat(result.getAssignee()).isEqualTo(assignee);
        verify(eventPublisher).publish(eq("task.assigned"), any(TaskEventMessage.class));
    }

    @Test
    void getTasksByProject_returnsAll() {
        UUID projectId = UUID.randomUUID();
        User owner = new User("test@example.com", "Owner", null);
        Project project = new Project("Project", "Desc", owner);
        Task task = new Task(project, "Task", null, TaskPriority.LOW, null);
        when(taskRepo.findByProjectId(projectId)).thenReturn(List.of(task));

        List<Task> result = service.getTasksByProject(projectId);
        assertThat(result).hasSize(1);
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd java && ./gradlew :task-service:test --tests "dev.kylebradshaw.task.service.TaskServiceTest" --no-daemon
```

Expected: FAIL — `TaskService` does not exist yet.

- [ ] **Step 3: Write TaskService**

Create `java/task-service/src/main/java/dev/kylebradshaw/task/service/TaskService.java`:

```java
package dev.kylebradshaw.task.service;

import dev.kylebradshaw.task.dto.CreateTaskRequest;
import dev.kylebradshaw.task.dto.TaskEventMessage;
import dev.kylebradshaw.task.dto.UpdateTaskRequest;
import dev.kylebradshaw.task.entity.Task;
import dev.kylebradshaw.task.entity.User;
import dev.kylebradshaw.task.repository.ProjectRepository;
import dev.kylebradshaw.task.repository.TaskRepository;
import dev.kylebradshaw.task.repository.UserRepository;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

@Service
public class TaskService {

    private final TaskRepository taskRepo;
    private final ProjectRepository projectRepo;
    private final UserRepository userRepo;
    private final TaskEventPublisher eventPublisher;

    public TaskService(TaskRepository taskRepo, ProjectRepository projectRepo,
                       UserRepository userRepo, TaskEventPublisher eventPublisher) {
        this.taskRepo = taskRepo;
        this.projectRepo = projectRepo;
        this.userRepo = userRepo;
        this.eventPublisher = eventPublisher;
    }

    @Transactional
    public Task createTask(CreateTaskRequest request, UUID actorId) {
        var project = projectRepo.findById(request.projectId())
                .orElseThrow(() -> new IllegalArgumentException("Project not found"));
        var priority = request.priority() != null ? request.priority()
                : dev.kylebradshaw.task.entity.TaskPriority.MEDIUM;
        var task = new Task(project, request.title(), request.description(),
                priority, request.dueDate());
        task = taskRepo.save(task);

        eventPublisher.publish("task.created", TaskEventMessage.of(
                "TASK_CREATED", actorId, project.getId(), task.getId(),
                Map.of("task_title", task.getTitle())
        ));
        return task;
    }

    @Transactional
    public Task updateTask(UUID taskId, UpdateTaskRequest request, UUID actorId) {
        Task task = taskRepo.findById(taskId)
                .orElseThrow(() -> new IllegalArgumentException("Task not found"));
        boolean statusChanged = false;

        if (request.title() != null) {
            task.setTitle(request.title());
        }
        if (request.description() != null) {
            task.setDescription(request.description());
        }
        if (request.status() != null && request.status() != task.getStatus()) {
            task.setStatus(request.status());
            statusChanged = true;
        }
        if (request.priority() != null) {
            task.setPriority(request.priority());
        }
        if (request.dueDate() != null) {
            task.setDueDate(request.dueDate());
        }
        task = taskRepo.save(task);

        if (statusChanged) {
            eventPublisher.publish("task.status_changed", TaskEventMessage.of(
                    "STATUS_CHANGED", actorId, task.getProject().getId(), task.getId(),
                    Map.of("task_title", task.getTitle(), "new_status", task.getStatus().name())
            ));
        }
        return task;
    }

    @Transactional
    public Task assignTask(UUID taskId, UUID assigneeId, UUID actorId) {
        Task task = taskRepo.findById(taskId)
                .orElseThrow(() -> new IllegalArgumentException("Task not found"));
        User assignee = userRepo.findById(assigneeId)
                .orElseThrow(() -> new IllegalArgumentException("Assignee not found"));
        task.setAssignee(assignee);
        task = taskRepo.save(task);

        eventPublisher.publish("task.assigned", TaskEventMessage.of(
                "TASK_ASSIGNED", actorId, task.getProject().getId(), task.getId(),
                Map.of("assignee_id", assigneeId.toString(), "task_title", task.getTitle())
        ));
        return task;
    }

    public Task getTask(UUID taskId) {
        return taskRepo.findById(taskId)
                .orElseThrow(() -> new IllegalArgumentException("Task not found"));
    }

    public List<Task> getTasksByProject(UUID projectId) {
        return taskRepo.findByProjectId(projectId);
    }

    @Transactional
    public void deleteTask(UUID taskId, UUID actorId) {
        Task task = taskRepo.findById(taskId)
                .orElseThrow(() -> new IllegalArgumentException("Task not found"));
        eventPublisher.publish("task.deleted", TaskEventMessage.of(
                "TASK_DELETED", actorId, task.getProject().getId(), task.getId(),
                Map.of("task_title", task.getTitle())
        ));
        taskRepo.delete(task);
    }
}
```

- [ ] **Step 4: Run tests**

```bash
cd java && ./gradlew :task-service:test --tests "dev.kylebradshaw.task.service.TaskServiceTest" --no-daemon
```

Expected: All 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add java/task-service/src/test/java/dev/kylebradshaw/task/service/TaskServiceTest.java \
        java/task-service/src/main/java/dev/kylebradshaw/task/service/TaskService.java
git commit -m "feat(task-service): add TaskService with unit tests"
```

---

### Task 10: ProjectController with TDD

**Files:**
- Create: `java/task-service/src/test/java/dev/kylebradshaw/task/controller/ProjectControllerTest.java`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/controller/ProjectController.java`

- [ ] **Step 1: Write the failing test**

Create `java/task-service/src/test/java/dev/kylebradshaw/task/controller/ProjectControllerTest.java`:

```java
package dev.kylebradshaw.task.controller;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

import com.fasterxml.jackson.databind.ObjectMapper;
import dev.kylebradshaw.task.dto.CreateProjectRequest;
import dev.kylebradshaw.task.entity.Project;
import dev.kylebradshaw.task.entity.User;
import dev.kylebradshaw.task.service.ProjectService;
import java.util.List;
import java.util.UUID;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.boot.test.context.TestConfiguration;
import org.springframework.context.annotation.Bean;
import org.springframework.http.MediaType;
import org.springframework.security.config.annotation.web.builders.HttpSecurity;
import org.springframework.security.web.SecurityFilterChain;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.test.web.servlet.MockMvc;

@WebMvcTest(ProjectController.class)
class ProjectControllerTest {

    @TestConfiguration
    static class TestSecurityConfig {
        @Bean
        public SecurityFilterChain testFilterChain(HttpSecurity http) throws Exception {
            return http.csrf(c -> c.disable())
                    .authorizeHttpRequests(a -> a.anyRequest().permitAll())
                    .build();
        }
    }

    @Autowired
    private MockMvc mockMvc;

    @Autowired
    private ObjectMapper objectMapper;

    @MockitoBean
    private ProjectService projectService;

    @Test
    void createProject_returns201() throws Exception {
        User owner = new User("test@example.com", "Test User", null);
        Project project = new Project("My Project", "Desc", owner);
        when(projectService.createProject(any(CreateProjectRequest.class), any(UUID.class)))
                .thenReturn(project);

        var request = new CreateProjectRequest("My Project", "Desc");

        mockMvc.perform(post("/api/projects")
                        .header("X-User-Id", UUID.randomUUID().toString())
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(objectMapper.writeValueAsString(request)))
                .andExpect(status().isCreated())
                .andExpect(jsonPath("$.name").value("My Project"));
    }

    @Test
    void getMyProjects_returns200() throws Exception {
        UUID userId = UUID.randomUUID();
        User owner = new User("test@example.com", "Test User", null);
        Project project = new Project("Project", "Desc", owner);
        when(projectService.getProjectsForUser(eq(userId))).thenReturn(List.of(project));

        mockMvc.perform(get("/api/projects")
                        .header("X-User-Id", userId.toString()))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$[0].name").value("Project"));
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd java && ./gradlew :task-service:test --tests "dev.kylebradshaw.task.controller.ProjectControllerTest" --no-daemon
```

Expected: FAIL — `ProjectController` does not exist.

- [ ] **Step 3: Write ProjectController**

Create `java/task-service/src/main/java/dev/kylebradshaw/task/controller/ProjectController.java`:

```java
package dev.kylebradshaw.task.controller;

import dev.kylebradshaw.task.dto.CreateProjectRequest;
import dev.kylebradshaw.task.dto.ProjectResponse;
import dev.kylebradshaw.task.service.ProjectService;
import jakarta.validation.Valid;
import java.util.List;
import java.util.UUID;
import org.springframework.http.HttpStatus;
import org.springframework.web.bind.annotation.DeleteMapping;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.PutMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestHeader;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.ResponseStatus;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/api/projects")
public class ProjectController {

    private final ProjectService projectService;

    public ProjectController(ProjectService projectService) {
        this.projectService = projectService;
    }

    @PostMapping
    @ResponseStatus(HttpStatus.CREATED)
    public ProjectResponse createProject(
            @Valid @RequestBody CreateProjectRequest request,
            @RequestHeader("X-User-Id") UUID userId) {
        return ProjectResponse.from(projectService.createProject(request, userId));
    }

    @GetMapping
    public List<ProjectResponse> getMyProjects(@RequestHeader("X-User-Id") UUID userId) {
        return projectService.getProjectsForUser(userId).stream()
                .map(ProjectResponse::from)
                .toList();
    }

    @GetMapping("/{id}")
    public ProjectResponse getProject(@PathVariable UUID id) {
        return ProjectResponse.from(projectService.getProject(id));
    }

    @PutMapping("/{id}")
    public ProjectResponse updateProject(
            @PathVariable UUID id,
            @Valid @RequestBody CreateProjectRequest request,
            @RequestHeader("X-User-Id") UUID userId) {
        return ProjectResponse.from(
                projectService.updateProject(id, userId, request.name(), request.description()));
    }

    @DeleteMapping("/{id}")
    @ResponseStatus(HttpStatus.NO_CONTENT)
    public void deleteProject(
            @PathVariable UUID id,
            @RequestHeader("X-User-Id") UUID userId) {
        projectService.deleteProject(id, userId);
    }
}
```

- [ ] **Step 4: Run tests**

```bash
cd java && ./gradlew :task-service:test --tests "dev.kylebradshaw.task.controller.ProjectControllerTest" --no-daemon
```

Expected: All 2 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add java/task-service/src/test/java/dev/kylebradshaw/task/controller/ProjectControllerTest.java \
        java/task-service/src/main/java/dev/kylebradshaw/task/controller/ProjectController.java
git commit -m "feat(task-service): add ProjectController with MockMvc tests"
```

---

### Task 11: TaskController with TDD

**Files:**
- Create: `java/task-service/src/test/java/dev/kylebradshaw/task/controller/TaskControllerTest.java`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/controller/TaskController.java`

- [ ] **Step 1: Write the failing test**

Create `java/task-service/src/test/java/dev/kylebradshaw/task/controller/TaskControllerTest.java`:

```java
package dev.kylebradshaw.task.controller;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.put;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

import com.fasterxml.jackson.databind.ObjectMapper;
import dev.kylebradshaw.task.dto.CreateTaskRequest;
import dev.kylebradshaw.task.dto.UpdateTaskRequest;
import dev.kylebradshaw.task.entity.Project;
import dev.kylebradshaw.task.entity.Task;
import dev.kylebradshaw.task.entity.TaskPriority;
import dev.kylebradshaw.task.entity.TaskStatus;
import dev.kylebradshaw.task.entity.User;
import dev.kylebradshaw.task.service.TaskService;
import java.util.List;
import java.util.UUID;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.boot.test.context.TestConfiguration;
import org.springframework.context.annotation.Bean;
import org.springframework.http.MediaType;
import org.springframework.security.config.annotation.web.builders.HttpSecurity;
import org.springframework.security.web.SecurityFilterChain;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.test.web.servlet.MockMvc;

@WebMvcTest(TaskController.class)
class TaskControllerTest {

    @TestConfiguration
    static class TestSecurityConfig {
        @Bean
        public SecurityFilterChain testFilterChain(HttpSecurity http) throws Exception {
            return http.csrf(c -> c.disable())
                    .authorizeHttpRequests(a -> a.anyRequest().permitAll())
                    .build();
        }
    }

    @Autowired
    private MockMvc mockMvc;

    @Autowired
    private ObjectMapper objectMapper;

    @MockitoBean
    private TaskService taskService;

    @Test
    void createTask_returns201() throws Exception {
        User owner = new User("test@example.com", "Owner", null);
        Project project = new Project("Project", "Desc", owner);
        Task task = new Task(project, "Fix bug", "Fix login", TaskPriority.HIGH, null);
        UUID projectId = UUID.randomUUID();

        when(taskService.createTask(any(CreateTaskRequest.class), any(UUID.class)))
                .thenReturn(task);

        var request = new CreateTaskRequest(projectId, "Fix bug", "Fix login",
                TaskPriority.HIGH, null);

        mockMvc.perform(post("/api/tasks")
                        .header("X-User-Id", UUID.randomUUID().toString())
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(objectMapper.writeValueAsString(request)))
                .andExpect(status().isCreated())
                .andExpect(jsonPath("$.title").value("Fix bug"))
                .andExpect(jsonPath("$.priority").value("HIGH"));
    }

    @Test
    void getTasksByProject_returns200() throws Exception {
        UUID projectId = UUID.randomUUID();
        User owner = new User("test@example.com", "Owner", null);
        Project project = new Project("Project", "Desc", owner);
        Task task = new Task(project, "Task 1", null, TaskPriority.LOW, null);
        when(taskService.getTasksByProject(eq(projectId))).thenReturn(List.of(task));

        mockMvc.perform(get("/api/tasks")
                        .param("projectId", projectId.toString()))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$[0].title").value("Task 1"));
    }

    @Test
    void updateTask_returns200() throws Exception {
        UUID taskId = UUID.randomUUID();
        User owner = new User("test@example.com", "Owner", null);
        Project project = new Project("Project", "Desc", owner);
        Task task = new Task(project, "Updated", null, TaskPriority.LOW, null);
        task.setStatus(TaskStatus.IN_PROGRESS);

        when(taskService.updateTask(eq(taskId), any(UpdateTaskRequest.class), any(UUID.class)))
                .thenReturn(task);

        var request = new UpdateTaskRequest("Updated", null, TaskStatus.IN_PROGRESS, null, null);

        mockMvc.perform(put("/api/tasks/" + taskId)
                        .header("X-User-Id", UUID.randomUUID().toString())
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(objectMapper.writeValueAsString(request)))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.title").value("Updated"))
                .andExpect(jsonPath("$.status").value("IN_PROGRESS"));
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd java && ./gradlew :task-service:test --tests "dev.kylebradshaw.task.controller.TaskControllerTest" --no-daemon
```

Expected: FAIL — `TaskController` does not exist.

- [ ] **Step 3: Write TaskController**

Create `java/task-service/src/main/java/dev/kylebradshaw/task/controller/TaskController.java`:

```java
package dev.kylebradshaw.task.controller;

import dev.kylebradshaw.task.dto.CreateTaskRequest;
import dev.kylebradshaw.task.dto.TaskResponse;
import dev.kylebradshaw.task.dto.UpdateTaskRequest;
import dev.kylebradshaw.task.service.TaskService;
import jakarta.validation.Valid;
import java.util.List;
import java.util.UUID;
import org.springframework.http.HttpStatus;
import org.springframework.web.bind.annotation.DeleteMapping;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.PutMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestHeader;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.ResponseStatus;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/api/tasks")
public class TaskController {

    private final TaskService taskService;

    public TaskController(TaskService taskService) {
        this.taskService = taskService;
    }

    @PostMapping
    @ResponseStatus(HttpStatus.CREATED)
    public TaskResponse createTask(
            @Valid @RequestBody CreateTaskRequest request,
            @RequestHeader("X-User-Id") UUID userId) {
        return TaskResponse.from(taskService.createTask(request, userId));
    }

    @GetMapping
    public List<TaskResponse> getTasksByProject(@RequestParam UUID projectId) {
        return taskService.getTasksByProject(projectId).stream()
                .map(TaskResponse::from)
                .toList();
    }

    @GetMapping("/{id}")
    public TaskResponse getTask(@PathVariable UUID id) {
        return TaskResponse.from(taskService.getTask(id));
    }

    @PutMapping("/{id}")
    public TaskResponse updateTask(
            @PathVariable UUID id,
            @Valid @RequestBody UpdateTaskRequest request,
            @RequestHeader("X-User-Id") UUID userId) {
        return TaskResponse.from(taskService.updateTask(id, request, userId));
    }

    @PutMapping("/{id}/assign/{assigneeId}")
    public TaskResponse assignTask(
            @PathVariable UUID id,
            @PathVariable UUID assigneeId,
            @RequestHeader("X-User-Id") UUID userId) {
        return TaskResponse.from(taskService.assignTask(id, assigneeId, userId));
    }

    @DeleteMapping("/{id}")
    @ResponseStatus(HttpStatus.NO_CONTENT)
    public void deleteTask(
            @PathVariable UUID id,
            @RequestHeader("X-User-Id") UUID userId) {
        taskService.deleteTask(id, userId);
    }
}
```

- [ ] **Step 4: Run tests**

```bash
cd java && ./gradlew :task-service:test --tests "dev.kylebradshaw.task.controller.TaskControllerTest" --no-daemon
```

Expected: All 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add java/task-service/src/test/java/dev/kylebradshaw/task/controller/TaskControllerTest.java \
        java/task-service/src/main/java/dev/kylebradshaw/task/controller/TaskController.java
git commit -m "feat(task-service): add TaskController with MockMvc tests"
```

---

## Phase 3: task-service Authentication

### Task 12: JwtService with TDD

**Files:**
- Create: `java/task-service/src/test/java/dev/kylebradshaw/task/service/AuthServiceTest.java` (JWT portion)
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/security/JwtService.java`

- [ ] **Step 1: Write the failing test**

Create `java/task-service/src/test/java/dev/kylebradshaw/task/security/JwtServiceTest.java`:

```java
package dev.kylebradshaw.task.security;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import java.util.UUID;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

class JwtServiceTest {

    private JwtService jwtService;

    @BeforeEach
    void setUp() {
        jwtService = new JwtService(
                "test-secret-key-that-is-at-least-32-characters-long-for-hs256",
                900000L,   // 15 minutes in ms
                604800000L // 7 days in ms
        );
    }

    @Test
    void generateAndValidateAccessToken() {
        UUID userId = UUID.randomUUID();
        String email = "test@example.com";

        String token = jwtService.generateAccessToken(userId, email);
        assertThat(token).isNotBlank();

        UUID extractedId = jwtService.extractUserId(token);
        assertThat(extractedId).isEqualTo(userId);

        String extractedEmail = jwtService.extractEmail(token);
        assertThat(extractedEmail).isEqualTo(email);
    }

    @Test
    void expiredToken_throwsException() {
        JwtService shortLived = new JwtService(
                "test-secret-key-that-is-at-least-32-characters-long-for-hs256",
                0L, 0L // instant expiry
        );
        UUID userId = UUID.randomUUID();
        String token = shortLived.generateAccessToken(userId, "test@example.com");

        assertThatThrownBy(() -> shortLived.extractUserId(token))
                .isInstanceOf(io.jsonwebtoken.ExpiredJwtException.class);
    }

    @Test
    void generateRefreshTokenString_isUUID() {
        String refreshToken = jwtService.generateRefreshTokenString();
        assertThat(UUID.fromString(refreshToken)).isNotNull();
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd java && ./gradlew :task-service:test --tests "dev.kylebradshaw.task.security.JwtServiceTest" --no-daemon
```

Expected: FAIL — `JwtService` does not exist.

- [ ] **Step 3: Write JwtService**

Create `java/task-service/src/main/java/dev/kylebradshaw/task/security/JwtService.java`:

```java
package dev.kylebradshaw.task.security;

import io.jsonwebtoken.Claims;
import io.jsonwebtoken.Jwts;
import io.jsonwebtoken.security.Keys;
import java.nio.charset.StandardCharsets;
import java.util.Date;
import java.util.UUID;
import javax.crypto.SecretKey;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Service;

@Service
public class JwtService {

    private final SecretKey signingKey;
    private final long accessTokenTtlMs;
    private final long refreshTokenTtlMs;

    public JwtService(
            @Value("${app.jwt.secret}") String secret,
            @Value("#{T(java.time.Duration).parse('PT' + '${app.jwt.access-token-ttl}'.toUpperCase()).toMillis()}") long accessTokenTtlMs,
            @Value("#{T(java.time.Duration).parse('P' + '${app.jwt.refresh-token-ttl}'.toUpperCase()).toMillis()}") long refreshTokenTtlMs) {
        this.signingKey = Keys.hmacShaKeyFor(secret.getBytes(StandardCharsets.UTF_8));
        this.accessTokenTtlMs = accessTokenTtlMs;
        this.refreshTokenTtlMs = refreshTokenTtlMs;
    }

    public String generateAccessToken(UUID userId, String email) {
        Date now = new Date();
        return Jwts.builder()
                .subject(userId.toString())
                .claim("email", email)
                .issuedAt(now)
                .expiration(new Date(now.getTime() + accessTokenTtlMs))
                .signWith(signingKey)
                .compact();
    }

    public String generateRefreshTokenString() {
        return UUID.randomUUID().toString();
    }

    public long getRefreshTokenTtlMs() {
        return refreshTokenTtlMs;
    }

    public UUID extractUserId(String token) {
        Claims claims = parseClaims(token);
        return UUID.fromString(claims.getSubject());
    }

    public String extractEmail(String token) {
        Claims claims = parseClaims(token);
        return claims.get("email", String.class);
    }

    public boolean isValid(String token) {
        try {
            parseClaims(token);
            return true;
        } catch (Exception e) {
            return false;
        }
    }

    private Claims parseClaims(String token) {
        return Jwts.parser()
                .verifyWith(signingKey)
                .build()
                .parseSignedClaims(token)
                .getPayload();
    }
}
```

**Note for unit test:** The constructor uses `@Value` annotations for Spring, but in the unit test we call the constructor directly with raw values. For the test to compile with the `@Value` annotations, the constructor just needs matching parameter types (String, long, long). The `@Value` SpEL expressions only resolve in a Spring context. To make the test work, use a simpler constructor:

Replace the `JwtService` constructor to accept plain values:

```java
    public JwtService(String secret, long accessTokenTtlMs, long refreshTokenTtlMs) {
        this.signingKey = Keys.hmacShaKeyFor(secret.getBytes(StandardCharsets.UTF_8));
        this.accessTokenTtlMs = accessTokenTtlMs;
        this.refreshTokenTtlMs = refreshTokenTtlMs;
    }
```

And configure it as a `@Bean` in a config class instead of relying on `@Value` in constructor. Create `java/task-service/src/main/java/dev/kylebradshaw/task/config/JwtConfig.java`:

```java
package dev.kylebradshaw.task.config;

import dev.kylebradshaw.task.security.JwtService;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
public class JwtConfig {

    @Bean
    public JwtService jwtService(
            @Value("${app.jwt.secret}") String secret,
            @Value("${app.jwt.access-token-ttl-ms:900000}") long accessTokenTtlMs,
            @Value("${app.jwt.refresh-token-ttl-ms:604800000}") long refreshTokenTtlMs) {
        return new JwtService(secret, accessTokenTtlMs, refreshTokenTtlMs);
    }
}
```

Update `application.yml` to use millisecond values:

```yaml
app:
  jwt:
    secret: ${JWT_SECRET:dev-secret-key-at-least-32-characters-long}
    access-token-ttl-ms: 900000      # 15 minutes
    refresh-token-ttl-ms: 604800000  # 7 days
```

Remove `@Service` from `JwtService` (it's now a `@Bean`).

- [ ] **Step 4: Run tests**

```bash
cd java && ./gradlew :task-service:test --tests "dev.kylebradshaw.task.security.JwtServiceTest" --no-daemon
```

Expected: All 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add java/task-service/src/main/java/dev/kylebradshaw/task/security/JwtService.java \
        java/task-service/src/main/java/dev/kylebradshaw/task/config/JwtConfig.java \
        java/task-service/src/test/java/dev/kylebradshaw/task/security/JwtServiceTest.java \
        java/task-service/src/main/resources/application.yml
git commit -m "feat(task-service): add JwtService with config and unit tests"
```

---

### Task 13: AuthService with TDD

**Files:**
- Create: `java/task-service/src/test/java/dev/kylebradshaw/task/service/AuthServiceTest.java`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/service/AuthService.java`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/dto/AuthRequest.java`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/dto/AuthResponse.java`

- [ ] **Step 1: Write DTOs**

Create `java/task-service/src/main/java/dev/kylebradshaw/task/dto/AuthRequest.java`:

```java
package dev.kylebradshaw.task.dto;

import jakarta.validation.constraints.NotBlank;

public record AuthRequest(@NotBlank String code, @NotBlank String redirectUri) {
}
```

Create `java/task-service/src/main/java/dev/kylebradshaw/task/dto/AuthResponse.java`:

```java
package dev.kylebradshaw.task.dto;

import java.util.UUID;

public record AuthResponse(
        String accessToken,
        String refreshToken,
        UUID userId,
        String email,
        String name,
        String avatarUrl
) {
}
```

- [ ] **Step 2: Write the failing test**

Create `java/task-service/src/test/java/dev/kylebradshaw/task/service/AuthServiceTest.java`:

```java
package dev.kylebradshaw.task.service;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.when;

import dev.kylebradshaw.task.dto.AuthResponse;
import dev.kylebradshaw.task.entity.RefreshToken;
import dev.kylebradshaw.task.entity.User;
import dev.kylebradshaw.task.repository.RefreshTokenRepository;
import dev.kylebradshaw.task.repository.UserRepository;
import dev.kylebradshaw.task.security.JwtService;
import java.util.Optional;
import java.util.UUID;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

@ExtendWith(MockitoExtension.class)
class AuthServiceTest {

    @Mock
    private UserRepository userRepo;

    @Mock
    private RefreshTokenRepository refreshTokenRepo;

    @Mock
    private JwtService jwtService;

    private AuthService authService;

    @BeforeEach
    void setUp() {
        authService = new AuthService(userRepo, refreshTokenRepo, jwtService);
    }

    @Test
    void authenticateGoogleUser_existingUser_returnsTokens() {
        String email = "user@gmail.com";
        String name = "Test User";
        String avatarUrl = "https://example.com/avatar.jpg";
        User existingUser = new User(email, name, avatarUrl);

        when(userRepo.findByEmail(email)).thenReturn(Optional.of(existingUser));
        when(jwtService.generateAccessToken(any(), any())).thenReturn("access-token");
        when(jwtService.generateRefreshTokenString()).thenReturn(UUID.randomUUID().toString());
        when(jwtService.getRefreshTokenTtlMs()).thenReturn(604800000L);
        when(refreshTokenRepo.save(any(RefreshToken.class))).thenAnswer(inv -> inv.getArgument(0));

        AuthResponse response = authService.authenticateGoogleUser(email, name, avatarUrl);

        assertThat(response.accessToken()).isEqualTo("access-token");
        assertThat(response.refreshToken()).isNotBlank();
        assertThat(response.email()).isEqualTo(email);
    }

    @Test
    void authenticateGoogleUser_newUser_createsAndReturnsTokens() {
        String email = "new@gmail.com";
        String name = "New User";

        when(userRepo.findByEmail(email)).thenReturn(Optional.empty());
        when(userRepo.save(any(User.class))).thenAnswer(inv -> inv.getArgument(0));
        when(jwtService.generateAccessToken(any(), any())).thenReturn("access-token");
        when(jwtService.generateRefreshTokenString()).thenReturn(UUID.randomUUID().toString());
        when(jwtService.getRefreshTokenTtlMs()).thenReturn(604800000L);
        when(refreshTokenRepo.save(any(RefreshToken.class))).thenAnswer(inv -> inv.getArgument(0));

        AuthResponse response = authService.authenticateGoogleUser(email, name, null);

        assertThat(response.accessToken()).isEqualTo("access-token");
        assertThat(response.name()).isEqualTo("New User");
    }
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
cd java && ./gradlew :task-service:test --tests "dev.kylebradshaw.task.service.AuthServiceTest" --no-daemon
```

Expected: FAIL — `AuthService` does not exist.

- [ ] **Step 4: Write AuthService**

Create `java/task-service/src/main/java/dev/kylebradshaw/task/service/AuthService.java`:

```java
package dev.kylebradshaw.task.service;

import dev.kylebradshaw.task.dto.AuthResponse;
import dev.kylebradshaw.task.entity.RefreshToken;
import dev.kylebradshaw.task.entity.User;
import dev.kylebradshaw.task.repository.RefreshTokenRepository;
import dev.kylebradshaw.task.repository.UserRepository;
import dev.kylebradshaw.task.security.JwtService;
import java.time.Instant;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

@Service
public class AuthService {

    private final UserRepository userRepo;
    private final RefreshTokenRepository refreshTokenRepo;
    private final JwtService jwtService;

    public AuthService(UserRepository userRepo, RefreshTokenRepository refreshTokenRepo,
                       JwtService jwtService) {
        this.userRepo = userRepo;
        this.refreshTokenRepo = refreshTokenRepo;
        this.jwtService = jwtService;
    }

    @Transactional
    public AuthResponse authenticateGoogleUser(String email, String name, String avatarUrl) {
        User user = userRepo.findByEmail(email)
                .map(existing -> {
                    existing.setName(name);
                    if (avatarUrl != null) {
                        existing.setAvatarUrl(avatarUrl);
                    }
                    return userRepo.save(existing);
                })
                .orElseGet(() -> userRepo.save(new User(email, name, avatarUrl)));

        String accessToken = jwtService.generateAccessToken(user.getId(), user.getEmail());
        String refreshTokenStr = jwtService.generateRefreshTokenString();

        Instant expiresAt = Instant.now().plusMillis(jwtService.getRefreshTokenTtlMs());
        refreshTokenRepo.save(new RefreshToken(user, refreshTokenStr, expiresAt));

        return new AuthResponse(
                accessToken, refreshTokenStr,
                user.getId(), user.getEmail(), user.getName(), user.getAvatarUrl()
        );
    }

    @Transactional
    public AuthResponse refreshAccessToken(String refreshTokenStr) {
        RefreshToken refreshToken = refreshTokenRepo.findByToken(refreshTokenStr)
                .orElseThrow(() -> new IllegalArgumentException("Invalid refresh token"));
        if (refreshToken.isExpired()) {
            refreshTokenRepo.delete(refreshToken);
            throw new IllegalArgumentException("Refresh token expired");
        }
        User user = refreshToken.getUser();
        String accessToken = jwtService.generateAccessToken(user.getId(), user.getEmail());

        return new AuthResponse(
                accessToken, refreshTokenStr,
                user.getId(), user.getEmail(), user.getName(), user.getAvatarUrl()
        );
    }
}
```

- [ ] **Step 5: Run tests**

```bash
cd java && ./gradlew :task-service:test --tests "dev.kylebradshaw.task.service.AuthServiceTest" --no-daemon
```

Expected: All 2 tests PASS.

- [ ] **Step 6: Commit**

```bash
git add java/task-service/src/main/java/dev/kylebradshaw/task/dto/Auth*.java \
        java/task-service/src/main/java/dev/kylebradshaw/task/service/AuthService.java \
        java/task-service/src/test/java/dev/kylebradshaw/task/service/AuthServiceTest.java
git commit -m "feat(task-service): add AuthService with Google OAuth user flow and unit tests"
```

---

### Task 14: SecurityConfig, JwtAuthenticationFilter, and AuthController

**Files:**
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/security/JwtAuthenticationFilter.java`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/config/SecurityConfig.java`
- Create: `java/task-service/src/main/java/dev/kylebradshaw/task/controller/AuthController.java`
- Create: `java/task-service/src/test/java/dev/kylebradshaw/task/controller/AuthControllerTest.java`

- [ ] **Step 1: Write JwtAuthenticationFilter**

Create `java/task-service/src/main/java/dev/kylebradshaw/task/security/JwtAuthenticationFilter.java`:

```java
package dev.kylebradshaw.task.security;

import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import java.io.IOException;
import java.util.List;
import java.util.UUID;
import org.springframework.security.authentication.UsernamePasswordAuthenticationToken;
import org.springframework.security.core.context.SecurityContextHolder;
import org.springframework.stereotype.Component;
import org.springframework.web.filter.OncePerRequestFilter;

@Component
public class JwtAuthenticationFilter extends OncePerRequestFilter {

    private final JwtService jwtService;

    public JwtAuthenticationFilter(JwtService jwtService) {
        this.jwtService = jwtService;
    }

    @Override
    protected void doFilterInternal(HttpServletRequest request, HttpServletResponse response,
                                    FilterChain filterChain) throws ServletException, IOException {
        String header = request.getHeader("Authorization");
        if (header != null && header.startsWith("Bearer ")) {
            String token = header.substring(7);
            if (jwtService.isValid(token)) {
                UUID userId = jwtService.extractUserId(token);
                var auth = new UsernamePasswordAuthenticationToken(
                        userId.toString(), null, List.of());
                SecurityContextHolder.getContext().setAuthentication(auth);
            }
        }

        // Also accept X-User-Id header (from gateway)
        String userIdHeader = request.getHeader("X-User-Id");
        if (userIdHeader != null && SecurityContextHolder.getContext().getAuthentication() == null) {
            var auth = new UsernamePasswordAuthenticationToken(
                    userIdHeader, null, List.of());
            SecurityContextHolder.getContext().setAuthentication(auth);
        }

        filterChain.doFilter(request, response);
    }
}
```

- [ ] **Step 2: Write SecurityConfig**

Create `java/task-service/src/main/java/dev/kylebradshaw/task/config/SecurityConfig.java`:

```java
package dev.kylebradshaw.task.config;

import dev.kylebradshaw.task.security.JwtAuthenticationFilter;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.security.config.annotation.web.builders.HttpSecurity;
import org.springframework.security.config.annotation.web.configuration.EnableWebSecurity;
import org.springframework.security.config.http.SessionCreationPolicy;
import org.springframework.security.web.SecurityFilterChain;
import org.springframework.security.web.authentication.UsernamePasswordAuthenticationFilter;
import org.springframework.web.cors.CorsConfiguration;
import org.springframework.web.cors.CorsConfigurationSource;
import org.springframework.web.cors.UrlBasedCorsConfigurationSource;
import java.util.List;

@Configuration
@EnableWebSecurity
public class SecurityConfig {

    private final JwtAuthenticationFilter jwtFilter;

    @Value("${app.allowed-origins}")
    private String allowedOrigins;

    public SecurityConfig(JwtAuthenticationFilter jwtFilter) {
        this.jwtFilter = jwtFilter;
    }

    @Bean
    public SecurityFilterChain filterChain(HttpSecurity http) throws Exception {
        return http
                .cors(cors -> cors.configurationSource(corsConfigurationSource()))
                .csrf(csrf -> csrf.disable())
                .sessionManagement(s -> s.sessionCreationPolicy(SessionCreationPolicy.STATELESS))
                .authorizeHttpRequests(auth -> auth
                        .requestMatchers("/api/auth/**", "/actuator/health").permitAll()
                        .anyRequest().authenticated()
                )
                .addFilterBefore(jwtFilter, UsernamePasswordAuthenticationFilter.class)
                .build();
    }

    @Bean
    public CorsConfigurationSource corsConfigurationSource() {
        CorsConfiguration config = new CorsConfiguration();
        config.setAllowedOrigins(List.of(allowedOrigins.split(",")));
        config.setAllowedMethods(List.of("GET", "POST", "PUT", "DELETE", "OPTIONS"));
        config.setAllowedHeaders(List.of("*"));
        config.setAllowCredentials(true);
        UrlBasedCorsConfigurationSource source = new UrlBasedCorsConfigurationSource();
        source.registerCorsConfiguration("/**", config);
        return source;
    }
}
```

- [ ] **Step 3: Write AuthController**

Create `java/task-service/src/main/java/dev/kylebradshaw/task/controller/AuthController.java`:

```java
package dev.kylebradshaw.task.controller;

import com.fasterxml.jackson.databind.JsonNode;
import dev.kylebradshaw.task.dto.AuthRequest;
import dev.kylebradshaw.task.dto.AuthResponse;
import dev.kylebradshaw.task.service.AuthService;
import jakarta.validation.Valid;
import java.util.Map;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.http.MediaType;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;
import org.springframework.web.client.RestClient;

@RestController
@RequestMapping("/api/auth")
public class AuthController {

    private final AuthService authService;
    private final RestClient restClient;

    @Value("${app.google.client-id}")
    private String googleClientId;

    @Value("${app.google.client-secret}")
    private String googleClientSecret;

    @Value("${app.google.token-url}")
    private String googleTokenUrl;

    @Value("${app.google.userinfo-url}")
    private String googleUserInfoUrl;

    public AuthController(AuthService authService) {
        this.authService = authService;
        this.restClient = RestClient.create();
    }

    @PostMapping("/google")
    public AuthResponse googleLogin(@Valid @RequestBody AuthRequest request) {
        // Exchange authorization code for access token
        JsonNode tokenResponse = restClient.post()
                .uri(googleTokenUrl)
                .contentType(MediaType.APPLICATION_JSON)
                .body(Map.of(
                        "code", request.code(),
                        "client_id", googleClientId,
                        "client_secret", googleClientSecret,
                        "redirect_uri", request.redirectUri(),
                        "grant_type", "authorization_code"
                ))
                .retrieve()
                .body(JsonNode.class);

        String googleAccessToken = tokenResponse.get("access_token").asText();

        // Get user info from Google
        JsonNode userInfo = restClient.get()
                .uri(googleUserInfoUrl)
                .header("Authorization", "Bearer " + googleAccessToken)
                .retrieve()
                .body(JsonNode.class);

        String email = userInfo.get("email").asText();
        String name = userInfo.get("name").asText();
        String picture = userInfo.has("picture") ? userInfo.get("picture").asText() : null;

        return authService.authenticateGoogleUser(email, name, picture);
    }

    @PostMapping("/refresh")
    public AuthResponse refresh(@RequestBody Map<String, String> body) {
        String refreshToken = body.get("refreshToken");
        if (refreshToken == null || refreshToken.isBlank()) {
            throw new IllegalArgumentException("refreshToken is required");
        }
        return authService.refreshAccessToken(refreshToken);
    }
}
```

- [ ] **Step 4: Write AuthController test**

Create `java/task-service/src/test/java/dev/kylebradshaw/task/controller/AuthControllerTest.java`:

```java
package dev.kylebradshaw.task.controller;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

import com.fasterxml.jackson.databind.ObjectMapper;
import dev.kylebradshaw.task.dto.AuthResponse;
import dev.kylebradshaw.task.service.AuthService;
import java.util.Map;
import java.util.UUID;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.boot.test.context.TestConfiguration;
import org.springframework.context.annotation.Bean;
import org.springframework.http.MediaType;
import org.springframework.security.config.annotation.web.builders.HttpSecurity;
import org.springframework.security.web.SecurityFilterChain;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.test.web.servlet.MockMvc;

@WebMvcTest(AuthController.class)
class AuthControllerTest {

    @TestConfiguration
    static class TestSecurityConfig {
        @Bean
        public SecurityFilterChain testFilterChain(HttpSecurity http) throws Exception {
            return http.csrf(c -> c.disable())
                    .authorizeHttpRequests(a -> a.anyRequest().permitAll())
                    .build();
        }
    }

    @Autowired
    private MockMvc mockMvc;

    @Autowired
    private ObjectMapper objectMapper;

    @MockitoBean
    private AuthService authService;

    @Test
    void refresh_returnsNewTokens() throws Exception {
        var response = new AuthResponse("new-access", "refresh-tok",
                UUID.randomUUID(), "test@example.com", "Test", null);
        when(authService.refreshAccessToken(any())).thenReturn(response);

        mockMvc.perform(post("/api/auth/refresh")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(objectMapper.writeValueAsString(
                                Map.of("refreshToken", "refresh-tok"))))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.accessToken").value("new-access"));
    }
}
```

- [ ] **Step 5: Run tests**

```bash
cd java && ./gradlew :task-service:test --no-daemon
```

Expected: All task-service tests PASS.

- [ ] **Step 6: Commit**

```bash
git add java/task-service/src/main/java/dev/kylebradshaw/task/security/ \
        java/task-service/src/main/java/dev/kylebradshaw/task/config/SecurityConfig.java \
        java/task-service/src/main/java/dev/kylebradshaw/task/controller/AuthController.java \
        java/task-service/src/test/java/dev/kylebradshaw/task/controller/AuthControllerTest.java
git commit -m "feat(task-service): add JWT auth filter, security config, and AuthController"
```

---

## Phase 4: activity-service

### Task 15: MongoDB Documents and Repositories

**Files:**
- Create: `java/activity-service/src/main/java/dev/kylebradshaw/activity/document/ActivityEvent.java`
- Create: `java/activity-service/src/main/java/dev/kylebradshaw/activity/document/Comment.java`
- Create: `java/activity-service/src/main/java/dev/kylebradshaw/activity/repository/ActivityEventRepository.java`
- Create: `java/activity-service/src/main/java/dev/kylebradshaw/activity/repository/CommentRepository.java`

- [ ] **Step 1: Write documents and repositories**

Create `java/activity-service/src/main/java/dev/kylebradshaw/activity/document/ActivityEvent.java`:

```java
package dev.kylebradshaw.activity.document;

import java.time.Instant;
import java.util.Map;
import org.springframework.data.annotation.Id;
import org.springframework.data.mongodb.core.mapping.Document;

@Document(collection = "activity_events")
public class ActivityEvent {

    @Id
    private String id;
    private String projectId;
    private String taskId;
    private String actorId;
    private String eventType;
    private Map<String, Object> metadata;
    private Instant timestamp;

    public ActivityEvent() {
    }

    public ActivityEvent(String projectId, String taskId, String actorId,
                         String eventType, Map<String, Object> metadata) {
        this.projectId = projectId;
        this.taskId = taskId;
        this.actorId = actorId;
        this.eventType = eventType;
        this.metadata = metadata;
        this.timestamp = Instant.now();
    }

    public String getId() { return id; }
    public String getProjectId() { return projectId; }
    public String getTaskId() { return taskId; }
    public String getActorId() { return actorId; }
    public String getEventType() { return eventType; }
    public Map<String, Object> getMetadata() { return metadata; }
    public Instant getTimestamp() { return timestamp; }
}
```

Create `java/activity-service/src/main/java/dev/kylebradshaw/activity/document/Comment.java`:

```java
package dev.kylebradshaw.activity.document;

import java.time.Instant;
import org.springframework.data.annotation.Id;
import org.springframework.data.mongodb.core.mapping.Document;

@Document(collection = "comments")
public class Comment {

    @Id
    private String id;
    private String taskId;
    private String authorId;
    private String body;
    private Instant createdAt;

    public Comment() {
    }

    public Comment(String taskId, String authorId, String body) {
        this.taskId = taskId;
        this.authorId = authorId;
        this.body = body;
        this.createdAt = Instant.now();
    }

    public String getId() { return id; }
    public String getTaskId() { return taskId; }
    public String getAuthorId() { return authorId; }
    public String getBody() { return body; }
    public Instant getCreatedAt() { return createdAt; }
}
```

Create `java/activity-service/src/main/java/dev/kylebradshaw/activity/repository/ActivityEventRepository.java`:

```java
package dev.kylebradshaw.activity.repository;

import dev.kylebradshaw.activity.document.ActivityEvent;
import java.util.List;
import org.springframework.data.mongodb.repository.MongoRepository;

public interface ActivityEventRepository extends MongoRepository<ActivityEvent, String> {
    List<ActivityEvent> findByTaskIdOrderByTimestampDesc(String taskId);
    List<ActivityEvent> findByProjectIdOrderByTimestampDesc(String projectId);
}
```

Create `java/activity-service/src/main/java/dev/kylebradshaw/activity/repository/CommentRepository.java`:

```java
package dev.kylebradshaw.activity.repository;

import dev.kylebradshaw.activity.document.Comment;
import java.util.List;
import org.springframework.data.mongodb.repository.MongoRepository;

public interface CommentRepository extends MongoRepository<Comment, String> {
    List<Comment> findByTaskIdOrderByCreatedAtAsc(String taskId);
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd java && ./gradlew :activity-service:compileJava --no-daemon
```

Expected: BUILD SUCCESSFUL.

- [ ] **Step 3: Commit**

```bash
git add java/activity-service/src/main/java/dev/kylebradshaw/activity/document/ \
        java/activity-service/src/main/java/dev/kylebradshaw/activity/repository/
git commit -m "feat(activity-service): add MongoDB documents and repositories"
```

---

### Task 16: Activity DTOs, RabbitMQ Config, and Event Listener

**Files:**
- Create: `java/activity-service/src/main/java/dev/kylebradshaw/activity/dto/TaskEventMessage.java`
- Create: `java/activity-service/src/main/java/dev/kylebradshaw/activity/config/RabbitConfig.java`
- Create: `java/activity-service/src/main/java/dev/kylebradshaw/activity/listener/TaskEventListener.java`
- Create: `java/activity-service/src/test/java/dev/kylebradshaw/activity/listener/TaskEventListenerTest.java`

- [ ] **Step 1: Write DTOs and config**

Create `java/activity-service/src/main/java/dev/kylebradshaw/activity/dto/TaskEventMessage.java`:

```java
package dev.kylebradshaw.activity.dto;

import java.time.Instant;
import java.util.Map;
import java.util.UUID;

public record TaskEventMessage(
        UUID eventId,
        String eventType,
        Instant timestamp,
        UUID actorId,
        UUID projectId,
        UUID taskId,
        Map<String, Object> data
) {
}
```

Create `java/activity-service/src/main/java/dev/kylebradshaw/activity/config/RabbitConfig.java`:

```java
package dev.kylebradshaw.activity.config;

import org.springframework.amqp.core.Binding;
import org.springframework.amqp.core.BindingBuilder;
import org.springframework.amqp.core.Queue;
import org.springframework.amqp.core.TopicExchange;
import org.springframework.amqp.support.converter.Jackson2JsonMessageConverter;
import org.springframework.amqp.support.converter.MessageConverter;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
public class RabbitConfig {

    public static final String QUEUE_NAME = "activity.queue";
    public static final String EXCHANGE_NAME = "task.events";

    @Bean
    public TopicExchange taskExchange() {
        return new TopicExchange(EXCHANGE_NAME);
    }

    @Bean
    public Queue activityQueue() {
        return new Queue(QUEUE_NAME, true);
    }

    @Bean
    public Binding binding(Queue activityQueue, TopicExchange taskExchange) {
        return BindingBuilder.bind(activityQueue).to(taskExchange).with("task.*");
    }

    @Bean
    public MessageConverter jsonMessageConverter() {
        return new Jackson2JsonMessageConverter();
    }
}
```

- [ ] **Step 2: Write the failing test**

Create `java/activity-service/src/test/java/dev/kylebradshaw/activity/listener/TaskEventListenerTest.java`:

```java
package dev.kylebradshaw.activity.listener;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.verify;

import dev.kylebradshaw.activity.dto.TaskEventMessage;
import dev.kylebradshaw.activity.repository.ActivityEventRepository;
import java.time.Instant;
import java.util.Map;
import java.util.UUID;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

@ExtendWith(MockitoExtension.class)
class TaskEventListenerTest {

    @Mock
    private ActivityEventRepository activityRepo;

    @InjectMocks
    private TaskEventListener listener;

    @Test
    void handleTaskEvent_savesActivityEvent() {
        var message = new TaskEventMessage(
                UUID.randomUUID(), "TASK_CREATED", Instant.now(),
                UUID.randomUUID(), UUID.randomUUID(), UUID.randomUUID(),
                Map.of("task_title", "Fix bug")
        );

        listener.handleTaskEvent(message);

        verify(activityRepo).save(any());
    }
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
cd java && ./gradlew :activity-service:test --tests "dev.kylebradshaw.activity.listener.TaskEventListenerTest" --no-daemon
```

Expected: FAIL — `TaskEventListener` does not exist.

- [ ] **Step 4: Write TaskEventListener**

Create `java/activity-service/src/main/java/dev/kylebradshaw/activity/listener/TaskEventListener.java`:

```java
package dev.kylebradshaw.activity.listener;

import dev.kylebradshaw.activity.config.RabbitConfig;
import dev.kylebradshaw.activity.document.ActivityEvent;
import dev.kylebradshaw.activity.dto.TaskEventMessage;
import dev.kylebradshaw.activity.repository.ActivityEventRepository;
import org.springframework.amqp.rabbit.annotation.RabbitListener;
import org.springframework.stereotype.Component;

@Component
public class TaskEventListener {

    private final ActivityEventRepository activityRepo;

    public TaskEventListener(ActivityEventRepository activityRepo) {
        this.activityRepo = activityRepo;
    }

    @RabbitListener(queues = RabbitConfig.QUEUE_NAME)
    public void handleTaskEvent(TaskEventMessage message) {
        var event = new ActivityEvent(
                message.projectId().toString(),
                message.taskId().toString(),
                message.actorId().toString(),
                message.eventType(),
                message.data()
        );
        activityRepo.save(event);
    }
}
```

- [ ] **Step 5: Run tests**

```bash
cd java && ./gradlew :activity-service:test --tests "dev.kylebradshaw.activity.listener.TaskEventListenerTest" --no-daemon
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add java/activity-service/src/main/java/dev/kylebradshaw/activity/dto/ \
        java/activity-service/src/main/java/dev/kylebradshaw/activity/config/ \
        java/activity-service/src/main/java/dev/kylebradshaw/activity/listener/ \
        java/activity-service/src/test/java/dev/kylebradshaw/activity/listener/
git commit -m "feat(activity-service): add RabbitMQ listener and activity event storage"
```

---

### Task 17: CommentService and Controllers with TDD

**Files:**
- Create: `java/activity-service/src/main/java/dev/kylebradshaw/activity/dto/CreateCommentRequest.java`
- Create: `java/activity-service/src/main/java/dev/kylebradshaw/activity/dto/CommentResponse.java`
- Create: `java/activity-service/src/main/java/dev/kylebradshaw/activity/service/CommentService.java`
- Create: `java/activity-service/src/main/java/dev/kylebradshaw/activity/service/ActivityService.java`
- Create: `java/activity-service/src/main/java/dev/kylebradshaw/activity/controller/CommentController.java`
- Create: `java/activity-service/src/main/java/dev/kylebradshaw/activity/controller/ActivityController.java`
- Create: `java/activity-service/src/test/java/dev/kylebradshaw/activity/service/CommentServiceTest.java`

- [ ] **Step 1: Write DTOs**

Create `java/activity-service/src/main/java/dev/kylebradshaw/activity/dto/CreateCommentRequest.java`:

```java
package dev.kylebradshaw.activity.dto;

import jakarta.validation.constraints.NotBlank;

public record CreateCommentRequest(@NotBlank String body) {
}
```

Create `java/activity-service/src/main/java/dev/kylebradshaw/activity/dto/CommentResponse.java`:

```java
package dev.kylebradshaw.activity.dto;

import dev.kylebradshaw.activity.document.Comment;
import java.time.Instant;

public record CommentResponse(String id, String taskId, String authorId,
                               String body, Instant createdAt) {
    public static CommentResponse from(Comment comment) {
        return new CommentResponse(comment.getId(), comment.getTaskId(),
                comment.getAuthorId(), comment.getBody(), comment.getCreatedAt());
    }
}
```

- [ ] **Step 2: Write the failing test**

Create `java/activity-service/src/test/java/dev/kylebradshaw/activity/service/CommentServiceTest.java`:

```java
package dev.kylebradshaw.activity.service;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.when;

import dev.kylebradshaw.activity.document.Comment;
import dev.kylebradshaw.activity.repository.CommentRepository;
import java.util.List;
import java.util.UUID;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

@ExtendWith(MockitoExtension.class)
class CommentServiceTest {

    @Mock
    private CommentRepository commentRepo;

    @InjectMocks
    private CommentService commentService;

    @Test
    void addComment_savesAndReturns() {
        String taskId = UUID.randomUUID().toString();
        String authorId = UUID.randomUUID().toString();
        Comment comment = new Comment(taskId, authorId, "Looks good!");
        when(commentRepo.save(any(Comment.class))).thenReturn(comment);

        Comment result = commentService.addComment(taskId, authorId, "Looks good!");
        assertThat(result.getBody()).isEqualTo("Looks good!");
    }

    @Test
    void getCommentsByTask_returnsSorted() {
        String taskId = UUID.randomUUID().toString();
        Comment c1 = new Comment(taskId, "user1", "First");
        Comment c2 = new Comment(taskId, "user2", "Second");
        when(commentRepo.findByTaskIdOrderByCreatedAtAsc(taskId)).thenReturn(List.of(c1, c2));

        List<Comment> result = commentService.getCommentsByTask(taskId);
        assertThat(result).hasSize(2);
    }
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
cd java && ./gradlew :activity-service:test --tests "dev.kylebradshaw.activity.service.CommentServiceTest" --no-daemon
```

Expected: FAIL — `CommentService` does not exist.

- [ ] **Step 4: Write services and controllers**

Create `java/activity-service/src/main/java/dev/kylebradshaw/activity/service/CommentService.java`:

```java
package dev.kylebradshaw.activity.service;

import dev.kylebradshaw.activity.document.Comment;
import dev.kylebradshaw.activity.repository.CommentRepository;
import java.util.List;
import org.springframework.stereotype.Service;

@Service
public class CommentService {

    private final CommentRepository commentRepo;

    public CommentService(CommentRepository commentRepo) {
        this.commentRepo = commentRepo;
    }

    public Comment addComment(String taskId, String authorId, String body) {
        return commentRepo.save(new Comment(taskId, authorId, body));
    }

    public List<Comment> getCommentsByTask(String taskId) {
        return commentRepo.findByTaskIdOrderByCreatedAtAsc(taskId);
    }
}
```

Create `java/activity-service/src/main/java/dev/kylebradshaw/activity/service/ActivityService.java`:

```java
package dev.kylebradshaw.activity.service;

import dev.kylebradshaw.activity.document.ActivityEvent;
import dev.kylebradshaw.activity.repository.ActivityEventRepository;
import java.util.List;
import org.springframework.stereotype.Service;

@Service
public class ActivityService {

    private final ActivityEventRepository activityRepo;

    public ActivityService(ActivityEventRepository activityRepo) {
        this.activityRepo = activityRepo;
    }

    public List<ActivityEvent> getActivityByTask(String taskId) {
        return activityRepo.findByTaskIdOrderByTimestampDesc(taskId);
    }

    public List<ActivityEvent> getActivityByProject(String projectId) {
        return activityRepo.findByProjectIdOrderByTimestampDesc(projectId);
    }
}
```

Create `java/activity-service/src/main/java/dev/kylebradshaw/activity/controller/CommentController.java`:

```java
package dev.kylebradshaw.activity.controller;

import dev.kylebradshaw.activity.dto.CommentResponse;
import dev.kylebradshaw.activity.dto.CreateCommentRequest;
import dev.kylebradshaw.activity.service.CommentService;
import jakarta.validation.Valid;
import java.util.List;
import org.springframework.http.HttpStatus;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestHeader;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.ResponseStatus;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/api/comments")
public class CommentController {

    private final CommentService commentService;

    public CommentController(CommentService commentService) {
        this.commentService = commentService;
    }

    @PostMapping("/{taskId}")
    @ResponseStatus(HttpStatus.CREATED)
    public CommentResponse addComment(
            @PathVariable String taskId,
            @RequestHeader("X-User-Id") String userId,
            @Valid @RequestBody CreateCommentRequest request) {
        return CommentResponse.from(commentService.addComment(taskId, userId, request.body()));
    }

    @GetMapping("/{taskId}")
    public List<CommentResponse> getComments(@PathVariable String taskId) {
        return commentService.getCommentsByTask(taskId).stream()
                .map(CommentResponse::from)
                .toList();
    }
}
```

Create `java/activity-service/src/main/java/dev/kylebradshaw/activity/controller/ActivityController.java`:

```java
package dev.kylebradshaw.activity.controller;

import dev.kylebradshaw.activity.document.ActivityEvent;
import dev.kylebradshaw.activity.service.ActivityService;
import java.util.List;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/api/activity")
public class ActivityController {

    private final ActivityService activityService;

    public ActivityController(ActivityService activityService) {
        this.activityService = activityService;
    }

    @GetMapping("/task/{taskId}")
    public List<ActivityEvent> getByTask(@PathVariable String taskId) {
        return activityService.getActivityByTask(taskId);
    }

    @GetMapping("/project/{projectId}")
    public List<ActivityEvent> getByProject(@PathVariable String projectId) {
        return activityService.getActivityByProject(projectId);
    }
}
```

- [ ] **Step 5: Run tests**

```bash
cd java && ./gradlew :activity-service:test --no-daemon
```

Expected: All activity-service tests PASS.

- [ ] **Step 6: Commit**

```bash
git add java/activity-service/src/
git commit -m "feat(activity-service): add CommentService, ActivityService, and REST controllers"
```

---

## Phase 5: notification-service

### Task 18: Notification DTO, Redis Service, and RabbitMQ Listener

**Files:**
- Create: `java/notification-service/src/main/java/dev/kylebradshaw/notification/dto/TaskEventMessage.java`
- Create: `java/notification-service/src/main/java/dev/kylebradshaw/notification/dto/Notification.java`
- Create: `java/notification-service/src/main/java/dev/kylebradshaw/notification/dto/NotificationResponse.java`
- Create: `java/notification-service/src/main/java/dev/kylebradshaw/notification/config/RabbitConfig.java`
- Create: `java/notification-service/src/main/java/dev/kylebradshaw/notification/service/NotificationService.java`
- Create: `java/notification-service/src/main/java/dev/kylebradshaw/notification/listener/TaskEventListener.java`
- Create: `java/notification-service/src/test/java/dev/kylebradshaw/notification/service/NotificationServiceTest.java`

- [ ] **Step 1: Write DTOs**

Create `java/notification-service/src/main/java/dev/kylebradshaw/notification/dto/TaskEventMessage.java`:

```java
package dev.kylebradshaw.notification.dto;

import java.time.Instant;
import java.util.Map;
import java.util.UUID;

public record TaskEventMessage(
        UUID eventId,
        String eventType,
        Instant timestamp,
        UUID actorId,
        UUID projectId,
        UUID taskId,
        Map<String, Object> data
) {
}
```

Create `java/notification-service/src/main/java/dev/kylebradshaw/notification/dto/Notification.java`:

```java
package dev.kylebradshaw.notification.dto;

import java.time.Instant;
import java.util.UUID;

public record Notification(
        String id,
        String type,
        String message,
        String taskId,
        boolean read,
        Instant createdAt
) {
    public static Notification create(String type, String message, String taskId) {
        return new Notification(
                UUID.randomUUID().toString(), type, message, taskId, false, Instant.now()
        );
    }
}
```

Create `java/notification-service/src/main/java/dev/kylebradshaw/notification/dto/NotificationResponse.java`:

```java
package dev.kylebradshaw.notification.dto;

import java.util.List;

public record NotificationResponse(List<Notification> notifications, long unreadCount) {
}
```

- [ ] **Step 2: Write RabbitConfig**

Create `java/notification-service/src/main/java/dev/kylebradshaw/notification/config/RabbitConfig.java`:

```java
package dev.kylebradshaw.notification.config;

import org.springframework.amqp.core.Binding;
import org.springframework.amqp.core.BindingBuilder;
import org.springframework.amqp.core.Queue;
import org.springframework.amqp.core.TopicExchange;
import org.springframework.amqp.support.converter.Jackson2JsonMessageConverter;
import org.springframework.amqp.support.converter.MessageConverter;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
public class RabbitConfig {

    public static final String QUEUE_NAME = "notification.queue";
    public static final String EXCHANGE_NAME = "task.events";

    @Bean
    public TopicExchange taskExchange() {
        return new TopicExchange(EXCHANGE_NAME);
    }

    @Bean
    public Queue notificationQueue() {
        return new Queue(QUEUE_NAME, true);
    }

    @Bean
    public Binding bindCreated(Queue notificationQueue, TopicExchange taskExchange) {
        return BindingBuilder.bind(notificationQueue).to(taskExchange).with("task.created");
    }

    @Bean
    public Binding bindAssigned(Queue notificationQueue, TopicExchange taskExchange) {
        return BindingBuilder.bind(notificationQueue).to(taskExchange).with("task.assigned");
    }

    @Bean
    public Binding bindStatus(Queue notificationQueue, TopicExchange taskExchange) {
        return BindingBuilder.bind(notificationQueue).to(taskExchange).with("task.status_changed");
    }

    @Bean
    public Binding bindComment(Queue notificationQueue, TopicExchange taskExchange) {
        return BindingBuilder.bind(notificationQueue).to(taskExchange).with("task.comment_added");
    }

    @Bean
    public MessageConverter jsonMessageConverter() {
        return new Jackson2JsonMessageConverter();
    }
}
```

- [ ] **Step 3: Write the failing test**

Create `java/notification-service/src/test/java/dev/kylebradshaw/notification/service/NotificationServiceTest.java`:

```java
package dev.kylebradshaw.notification.service;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyDouble;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

import dev.kylebradshaw.notification.dto.Notification;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Set;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.data.redis.core.ValueOperations;
import org.springframework.data.redis.core.ZSetOperations;

@ExtendWith(MockitoExtension.class)
class NotificationServiceTest {

    @Mock
    private StringRedisTemplate redisTemplate;

    @Mock
    private ZSetOperations<String, String> zSetOps;

    @Mock
    private ValueOperations<String, String> valueOps;

    @InjectMocks
    private NotificationService notificationService;

    @Test
    void addNotification_addsToSortedSetAndIncrementsCount() {
        String userId = "user-123";
        when(redisTemplate.opsForZSet()).thenReturn(zSetOps);
        when(redisTemplate.opsForValue()).thenReturn(valueOps);

        var notification = Notification.create("TASK_ASSIGNED", "You were assigned a task", "task-1");
        notificationService.addNotification(userId, notification);

        verify(zSetOps).add(eq("notifications:" + userId), anyString(), anyDouble());
        verify(valueOps).increment("notification_count:" + userId);
    }

    @Test
    void getNotifications_returnsFromRedis() {
        String userId = "user-123";
        when(redisTemplate.opsForZSet()).thenReturn(zSetOps);
        when(redisTemplate.opsForValue()).thenReturn(valueOps);
        when(valueOps.get("notification_count:" + userId)).thenReturn("2");

        String json = """
                {"id":"1","type":"TASK_ASSIGNED","message":"Assigned","taskId":"t1","read":false,"createdAt":"2026-04-03T00:00:00Z"}""";
        Set<String> set = new LinkedHashSet<>();
        set.add(json);
        when(zSetOps.reverseRange("notifications:" + userId, 0, -1)).thenReturn(set);

        var response = notificationService.getNotifications(userId, false);
        assertThat(response.unreadCount()).isEqualTo(2);
        assertThat(response.notifications()).hasSize(1);
    }
}
```

- [ ] **Step 4: Run test to verify it fails**

```bash
cd java && ./gradlew :notification-service:test --tests "dev.kylebradshaw.notification.service.NotificationServiceTest" --no-daemon
```

Expected: FAIL — `NotificationService` does not exist.

- [ ] **Step 5: Write NotificationService**

Create `java/notification-service/src/main/java/dev/kylebradshaw/notification/service/NotificationService.java`:

```java
package dev.kylebradshaw.notification.service;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;
import dev.kylebradshaw.notification.dto.Notification;
import dev.kylebradshaw.notification.dto.NotificationResponse;
import java.util.ArrayList;
import java.util.List;
import java.util.Set;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.stereotype.Service;

@Service
public class NotificationService {

    private static final String NOTIFICATIONS_KEY = "notifications:";
    private static final String COUNT_KEY = "notification_count:";

    private final StringRedisTemplate redisTemplate;
    private final ObjectMapper objectMapper;

    public NotificationService(StringRedisTemplate redisTemplate) {
        this.redisTemplate = redisTemplate;
        this.objectMapper = new ObjectMapper();
        this.objectMapper.registerModule(new JavaTimeModule());
    }

    public void addNotification(String userId, Notification notification) {
        try {
            String json = objectMapper.writeValueAsString(notification);
            redisTemplate.opsForZSet().add(
                    NOTIFICATIONS_KEY + userId, json,
                    notification.createdAt().toEpochMilli()
            );
            redisTemplate.opsForValue().increment(COUNT_KEY + userId);
        } catch (JsonProcessingException e) {
            throw new RuntimeException("Failed to serialize notification", e);
        }
    }

    public NotificationResponse getNotifications(String userId, boolean unreadOnly) {
        Set<String> entries = redisTemplate.opsForZSet()
                .reverseRange(NOTIFICATIONS_KEY + userId, 0, -1);

        List<Notification> notifications = new ArrayList<>();
        if (entries != null) {
            for (String json : entries) {
                try {
                    Notification n = objectMapper.readValue(json, Notification.class);
                    if (!unreadOnly || !n.read()) {
                        notifications.add(n);
                    }
                } catch (JsonProcessingException e) {
                    // skip malformed entries
                }
            }
        }

        String countStr = redisTemplate.opsForValue().get(COUNT_KEY + userId);
        long unreadCount = countStr != null ? Long.parseLong(countStr) : 0;

        return new NotificationResponse(notifications, unreadCount);
    }

    public void markRead(String userId, String notificationId) {
        // Remove the old entry, update read flag, re-add
        Set<String> entries = redisTemplate.opsForZSet()
                .reverseRange(NOTIFICATIONS_KEY + userId, 0, -1);
        if (entries == null) return;

        for (String json : entries) {
            try {
                Notification n = objectMapper.readValue(json, Notification.class);
                if (n.id().equals(notificationId) && !n.read()) {
                    redisTemplate.opsForZSet().remove(NOTIFICATIONS_KEY + userId, json);
                    Notification updated = new Notification(
                            n.id(), n.type(), n.message(), n.taskId(), true, n.createdAt());
                    redisTemplate.opsForZSet().add(
                            NOTIFICATIONS_KEY + userId,
                            objectMapper.writeValueAsString(updated),
                            n.createdAt().toEpochMilli()
                    );
                    redisTemplate.opsForValue().decrement(COUNT_KEY + userId);
                    return;
                }
            } catch (JsonProcessingException e) {
                // skip
            }
        }
    }

    public void markAllRead(String userId) {
        redisTemplate.opsForValue().set(COUNT_KEY + userId, "0");
    }
}
```

- [ ] **Step 6: Write TaskEventListener**

Create `java/notification-service/src/main/java/dev/kylebradshaw/notification/listener/TaskEventListener.java`:

```java
package dev.kylebradshaw.notification.listener;

import dev.kylebradshaw.notification.config.RabbitConfig;
import dev.kylebradshaw.notification.dto.Notification;
import dev.kylebradshaw.notification.dto.TaskEventMessage;
import dev.kylebradshaw.notification.service.NotificationService;
import java.util.Map;
import org.springframework.stereotype.Component;
import org.springframework.amqp.rabbit.annotation.RabbitListener;

@Component
public class TaskEventListener {

    private final NotificationService notificationService;

    public TaskEventListener(NotificationService notificationService) {
        this.notificationService = notificationService;
    }

    @RabbitListener(queues = RabbitConfig.QUEUE_NAME)
    public void handleTaskEvent(TaskEventMessage message) {
        Map<String, Object> data = message.data();
        String taskTitle = data.getOrDefault("task_title", "a task").toString();

        switch (message.eventType()) {
            case "TASK_ASSIGNED" -> {
                String assigneeId = data.getOrDefault("assignee_id", "").toString();
                if (!assigneeId.isBlank()) {
                    notificationService.addNotification(assigneeId,
                            Notification.create("TASK_ASSIGNED",
                                    "You were assigned to: " + taskTitle,
                                    message.taskId().toString()));
                }
            }
            case "STATUS_CHANGED" -> {
                String newStatus = data.getOrDefault("new_status", "").toString();
                notificationService.addNotification(message.actorId().toString(),
                        Notification.create("STATUS_CHANGED",
                                taskTitle + " moved to " + newStatus,
                                message.taskId().toString()));
            }
            case "TASK_CREATED" -> notificationService.addNotification(
                    message.actorId().toString(),
                    Notification.create("TASK_CREATED",
                            "New task: " + taskTitle,
                            message.taskId().toString()));
            default -> { }
        }
    }
}
```

- [ ] **Step 7: Write NotificationController**

Create `java/notification-service/src/main/java/dev/kylebradshaw/notification/controller/NotificationController.java`:

```java
package dev.kylebradshaw.notification.controller;

import dev.kylebradshaw.notification.dto.NotificationResponse;
import dev.kylebradshaw.notification.service.NotificationService;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestHeader;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/api/notifications")
public class NotificationController {

    private final NotificationService notificationService;

    public NotificationController(NotificationService notificationService) {
        this.notificationService = notificationService;
    }

    @GetMapping
    public NotificationResponse getNotifications(
            @RequestHeader("X-User-Id") String userId,
            @RequestParam(defaultValue = "false") boolean unreadOnly) {
        return notificationService.getNotifications(userId, unreadOnly);
    }

    @PostMapping("/{id}/read")
    public void markRead(
            @RequestHeader("X-User-Id") String userId,
            @PathVariable String id) {
        notificationService.markRead(userId, id);
    }

    @PostMapping("/read-all")
    public void markAllRead(@RequestHeader("X-User-Id") String userId) {
        notificationService.markAllRead(userId);
    }
}
```

- [ ] **Step 8: Run all notification-service tests**

```bash
cd java && ./gradlew :notification-service:test --no-daemon
```

Expected: All tests PASS.

- [ ] **Step 9: Commit**

```bash
git add java/notification-service/src/
git commit -m "feat(notification-service): add Redis notifications, RabbitMQ listener, and REST API"
```

---

## Phase 6: gateway-service

### Task 19: GraphQL Schema

**Files:**
- Create: `java/gateway-service/src/main/resources/graphql/schema.graphqls`

- [ ] **Step 1: Write GraphQL schema**

Create `java/gateway-service/src/main/resources/graphql/schema.graphqls`:

```graphql
type Query {
    me: User
    project(id: ID!): Project
    myProjects: [Project!]!
    task(id: ID!): Task
    taskActivity(taskId: ID!): [ActivityEvent!]!
    taskComments(taskId: ID!): [Comment!]!
    myNotifications(unreadOnly: Boolean): NotificationResponse!
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

type User {
    id: ID!
    email: String!
    name: String!
    avatarUrl: String
}

type Project {
    id: ID!
    name: String!
    description: String
    ownerId: ID!
    ownerName: String!
    createdAt: String!
}

type Task {
    id: ID!
    projectId: ID!
    title: String!
    description: String
    status: TaskStatus!
    priority: TaskPriority!
    assigneeId: ID
    assigneeName: String
    dueDate: String
    createdAt: String!
    updatedAt: String!
}

type ActivityEvent {
    id: ID!
    projectId: String!
    taskId: String!
    actorId: String!
    eventType: String!
    metadata: String
    timestamp: String!
}

type Comment {
    id: ID!
    taskId: String!
    authorId: String!
    body: String!
    createdAt: String!
}

type Notification {
    id: ID!
    type: String!
    message: String!
    taskId: String!
    read: Boolean!
    createdAt: String!
}

type NotificationResponse {
    notifications: [Notification!]!
    unreadCount: Int!
}

enum TaskStatus {
    TODO
    IN_PROGRESS
    DONE
}

enum TaskPriority {
    LOW
    MEDIUM
    HIGH
}

input CreateProjectInput {
    name: String!
    description: String
}

input UpdateProjectInput {
    name: String
    description: String
}

input CreateTaskInput {
    projectId: ID!
    title: String!
    description: String
    priority: TaskPriority
    dueDate: String
}

input UpdateTaskInput {
    title: String
    description: String
    status: TaskStatus
    priority: TaskPriority
    dueDate: String
}
```

- [ ] **Step 2: Commit**

```bash
git add java/gateway-service/src/main/resources/graphql/schema.graphqls
git commit -m "feat(gateway-service): add GraphQL schema"
```

---

### Task 20: Gateway DTO Records

**Files:**
- Create: `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/dto/ProjectDto.java`
- Create: `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/dto/TaskDto.java`
- Create: `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/dto/UserDto.java`
- Create: `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/dto/ActivityEventDto.java`
- Create: `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/dto/CommentDto.java`
- Create: `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/dto/NotificationDto.java`

- [ ] **Step 1: Write all DTOs**

Create `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/dto/UserDto.java`:

```java
package dev.kylebradshaw.gateway.dto;

public record UserDto(String id, String email, String name, String avatarUrl) {
}
```

Create `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/dto/ProjectDto.java`:

```java
package dev.kylebradshaw.gateway.dto;

public record ProjectDto(String id, String name, String description,
                          String ownerId, String ownerName, String createdAt) {
}
```

Create `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/dto/TaskDto.java`:

```java
package dev.kylebradshaw.gateway.dto;

public record TaskDto(String id, String projectId, String title, String description,
                      String status, String priority, String assigneeId, String assigneeName,
                      String dueDate, String createdAt, String updatedAt) {
}
```

Create `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/dto/ActivityEventDto.java`:

```java
package dev.kylebradshaw.gateway.dto;

public record ActivityEventDto(String id, String projectId, String taskId,
                                String actorId, String eventType, String metadata,
                                String timestamp) {
}
```

Create `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/dto/CommentDto.java`:

```java
package dev.kylebradshaw.gateway.dto;

public record CommentDto(String id, String taskId, String authorId,
                          String body, String createdAt) {
}
```

Create `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/dto/NotificationDto.java`:

```java
package dev.kylebradshaw.gateway.dto;

import java.util.List;

public record NotificationDto(List<NotificationItem> notifications, int unreadCount) {

    public record NotificationItem(String id, String type, String message,
                                    String taskId, boolean read, String createdAt) {
    }
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd java && ./gradlew :gateway-service:compileJava --no-daemon
```

- [ ] **Step 3: Commit**

```bash
git add java/gateway-service/src/main/java/dev/kylebradshaw/gateway/dto/
git commit -m "feat(gateway-service): add DTO records for downstream service responses"
```

---

### Task 21: RestClient Config and Service Clients

**Files:**
- Create: `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/config/RestClientConfig.java`
- Create: `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/client/TaskServiceClient.java`
- Create: `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/client/ActivityServiceClient.java`
- Create: `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/client/NotificationServiceClient.java`

- [ ] **Step 1: Write RestClientConfig**

Create `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/config/RestClientConfig.java`:

```java
package dev.kylebradshaw.gateway.config;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.web.client.RestClient;

@Configuration
public class RestClientConfig {

    @Bean
    public RestClient taskServiceClient(@Value("${app.services.task-url}") String baseUrl) {
        return RestClient.builder().baseUrl(baseUrl).build();
    }

    @Bean
    public RestClient activityServiceClient(@Value("${app.services.activity-url}") String baseUrl) {
        return RestClient.builder().baseUrl(baseUrl).build();
    }

    @Bean
    public RestClient notificationServiceClient(@Value("${app.services.notification-url}") String baseUrl) {
        return RestClient.builder().baseUrl(baseUrl).build();
    }
}
```

- [ ] **Step 2: Write TaskServiceClient**

Create `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/client/TaskServiceClient.java`:

```java
package dev.kylebradshaw.gateway.client;

import dev.kylebradshaw.gateway.dto.ProjectDto;
import dev.kylebradshaw.gateway.dto.TaskDto;
import java.util.List;
import java.util.Map;
import org.springframework.beans.factory.annotation.Qualifier;
import org.springframework.core.ParameterizedTypeReference;
import org.springframework.stereotype.Component;
import org.springframework.web.client.RestClient;

@Component
public class TaskServiceClient {

    private final RestClient restClient;

    public TaskServiceClient(@Qualifier("taskServiceClient") RestClient restClient) {
        this.restClient = restClient;
    }

    public List<ProjectDto> getMyProjects(String userId) {
        return restClient.get()
                .uri("/api/projects")
                .header("X-User-Id", userId)
                .retrieve()
                .body(new ParameterizedTypeReference<>() {});
    }

    public ProjectDto getProject(String id) {
        return restClient.get()
                .uri("/api/projects/{id}", id)
                .retrieve()
                .body(ProjectDto.class);
    }

    public ProjectDto createProject(String userId, Map<String, Object> input) {
        return restClient.post()
                .uri("/api/projects")
                .header("X-User-Id", userId)
                .body(input)
                .retrieve()
                .body(ProjectDto.class);
    }

    public ProjectDto updateProject(String id, String userId, Map<String, Object> input) {
        return restClient.put()
                .uri("/api/projects/{id}", id)
                .header("X-User-Id", userId)
                .body(input)
                .retrieve()
                .body(ProjectDto.class);
    }

    public void deleteProject(String id, String userId) {
        restClient.delete()
                .uri("/api/projects/{id}", id)
                .header("X-User-Id", userId)
                .retrieve()
                .toBodilessEntity();
    }

    public TaskDto getTask(String id) {
        return restClient.get()
                .uri("/api/tasks/{id}", id)
                .retrieve()
                .body(TaskDto.class);
    }

    public List<TaskDto> getTasksByProject(String projectId) {
        return restClient.get()
                .uri("/api/tasks?projectId={projectId}", projectId)
                .retrieve()
                .body(new ParameterizedTypeReference<>() {});
    }

    public TaskDto createTask(String userId, Map<String, Object> input) {
        return restClient.post()
                .uri("/api/tasks")
                .header("X-User-Id", userId)
                .body(input)
                .retrieve()
                .body(TaskDto.class);
    }

    public TaskDto updateTask(String id, String userId, Map<String, Object> input) {
        return restClient.put()
                .uri("/api/tasks/{id}", id)
                .header("X-User-Id", userId)
                .body(input)
                .retrieve()
                .body(TaskDto.class);
    }

    public TaskDto assignTask(String taskId, String assigneeId, String userId) {
        return restClient.put()
                .uri("/api/tasks/{taskId}/assign/{assigneeId}", taskId, assigneeId)
                .header("X-User-Id", userId)
                .retrieve()
                .body(TaskDto.class);
    }

    public void deleteTask(String id, String userId) {
        restClient.delete()
                .uri("/api/tasks/{id}", id)
                .header("X-User-Id", userId)
                .retrieve()
                .toBodilessEntity();
    }
}
```

- [ ] **Step 3: Write ActivityServiceClient**

Create `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/client/ActivityServiceClient.java`:

```java
package dev.kylebradshaw.gateway.client;

import dev.kylebradshaw.gateway.dto.ActivityEventDto;
import dev.kylebradshaw.gateway.dto.CommentDto;
import java.util.List;
import java.util.Map;
import org.springframework.beans.factory.annotation.Qualifier;
import org.springframework.core.ParameterizedTypeReference;
import org.springframework.stereotype.Component;
import org.springframework.web.client.RestClient;

@Component
public class ActivityServiceClient {

    private final RestClient restClient;

    public ActivityServiceClient(@Qualifier("activityServiceClient") RestClient restClient) {
        this.restClient = restClient;
    }

    public List<ActivityEventDto> getActivityByTask(String taskId) {
        return restClient.get()
                .uri("/api/activity/task/{taskId}", taskId)
                .retrieve()
                .body(new ParameterizedTypeReference<>() {});
    }

    public List<CommentDto> getCommentsByTask(String taskId) {
        return restClient.get()
                .uri("/api/comments/{taskId}", taskId)
                .retrieve()
                .body(new ParameterizedTypeReference<>() {});
    }

    public CommentDto addComment(String taskId, String userId, String body) {
        return restClient.post()
                .uri("/api/comments/{taskId}", taskId)
                .header("X-User-Id", userId)
                .body(Map.of("body", body))
                .retrieve()
                .body(CommentDto.class);
    }
}
```

- [ ] **Step 4: Write NotificationServiceClient**

Create `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/client/NotificationServiceClient.java`:

```java
package dev.kylebradshaw.gateway.client;

import dev.kylebradshaw.gateway.dto.NotificationDto;
import org.springframework.beans.factory.annotation.Qualifier;
import org.springframework.stereotype.Component;
import org.springframework.web.client.RestClient;

@Component
public class NotificationServiceClient {

    private final RestClient restClient;

    public NotificationServiceClient(@Qualifier("notificationServiceClient") RestClient restClient) {
        this.restClient = restClient;
    }

    public NotificationDto getNotifications(String userId, Boolean unreadOnly) {
        return restClient.get()
                .uri(uriBuilder -> uriBuilder
                        .path("/api/notifications")
                        .queryParam("unreadOnly", unreadOnly != null ? unreadOnly : false)
                        .build())
                .header("X-User-Id", userId)
                .retrieve()
                .body(NotificationDto.class);
    }

    public void markRead(String userId, String notificationId) {
        restClient.post()
                .uri("/api/notifications/{id}/read", notificationId)
                .header("X-User-Id", userId)
                .retrieve()
                .toBodilessEntity();
    }

    public void markAllRead(String userId) {
        restClient.post()
                .uri("/api/notifications/read-all")
                .header("X-User-Id", userId)
                .retrieve()
                .toBodilessEntity();
    }
}
```

- [ ] **Step 5: Verify compilation**

```bash
cd java && ./gradlew :gateway-service:compileJava --no-daemon
```

- [ ] **Step 6: Commit**

```bash
git add java/gateway-service/src/main/java/dev/kylebradshaw/gateway/config/RestClientConfig.java \
        java/gateway-service/src/main/java/dev/kylebradshaw/gateway/client/
git commit -m "feat(gateway-service): add RestClient config and downstream service clients"
```

---

### Task 22: GraphQL Resolvers with TDD

**Files:**
- Create: `java/gateway-service/src/test/java/dev/kylebradshaw/gateway/resolver/QueryResolverTest.java`
- Create: `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/resolver/QueryResolver.java`
- Create: `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/resolver/MutationResolver.java`
- Create: `java/gateway-service/src/test/java/dev/kylebradshaw/gateway/resolver/MutationResolverTest.java`

- [ ] **Step 1: Write the failing QueryResolver test**

Create `java/gateway-service/src/test/java/dev/kylebradshaw/gateway/resolver/QueryResolverTest.java`:

```java
package dev.kylebradshaw.gateway.resolver;

import static org.mockito.Mockito.when;

import dev.kylebradshaw.gateway.client.ActivityServiceClient;
import dev.kylebradshaw.gateway.client.NotificationServiceClient;
import dev.kylebradshaw.gateway.client.TaskServiceClient;
import dev.kylebradshaw.gateway.dto.ProjectDto;
import java.util.List;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.graphql.GraphQlTest;
import org.springframework.boot.test.context.TestConfiguration;
import org.springframework.context.annotation.Bean;
import org.springframework.graphql.test.tester.GraphQlTester;
import org.springframework.security.config.annotation.web.builders.HttpSecurity;
import org.springframework.security.web.SecurityFilterChain;
import org.springframework.test.context.bean.override.mockito.MockitoBean;

@GraphQlTest(QueryResolver.class)
class QueryResolverTest {

    @TestConfiguration
    static class TestSecurityConfig {
        @Bean
        public SecurityFilterChain testFilterChain(HttpSecurity http) throws Exception {
            return http.csrf(c -> c.disable())
                    .authorizeHttpRequests(a -> a.anyRequest().permitAll())
                    .build();
        }
    }

    @Autowired
    private GraphQlTester graphQlTester;

    @MockitoBean
    private TaskServiceClient taskClient;

    @MockitoBean
    private ActivityServiceClient activityClient;

    @MockitoBean
    private NotificationServiceClient notificationClient;

    @Test
    void myProjects_returnsProjects() {
        when(taskClient.getMyProjects("test-user"))
                .thenReturn(List.of(new ProjectDto("1", "Project 1", "Desc",
                        "owner-1", "Owner", "2026-04-03T00:00:00Z")));

        graphQlTester.document("""
                        query {
                            myProjects {
                                id
                                name
                            }
                        }
                        """)
                .httpHeaders(headers -> headers.set("X-User-Id", "test-user"))
                .execute()
                .path("myProjects[0].name").entity(String.class).isEqualTo("Project 1");
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd java && ./gradlew :gateway-service:test --tests "dev.kylebradshaw.gateway.resolver.QueryResolverTest" --no-daemon
```

Expected: FAIL — `QueryResolver` does not exist.

- [ ] **Step 3: Write QueryResolver**

Create `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/resolver/QueryResolver.java`:

```java
package dev.kylebradshaw.gateway.resolver;

import dev.kylebradshaw.gateway.client.ActivityServiceClient;
import dev.kylebradshaw.gateway.client.NotificationServiceClient;
import dev.kylebradshaw.gateway.client.TaskServiceClient;
import dev.kylebradshaw.gateway.dto.ActivityEventDto;
import dev.kylebradshaw.gateway.dto.CommentDto;
import dev.kylebradshaw.gateway.dto.NotificationDto;
import dev.kylebradshaw.gateway.dto.ProjectDto;
import dev.kylebradshaw.gateway.dto.TaskDto;
import graphql.schema.DataFetchingEnvironment;
import java.util.List;
import org.springframework.graphql.data.method.annotation.Argument;
import org.springframework.graphql.data.method.annotation.QueryMapping;
import org.springframework.stereotype.Controller;

@Controller
public class QueryResolver {

    private final TaskServiceClient taskClient;
    private final ActivityServiceClient activityClient;
    private final NotificationServiceClient notificationClient;

    public QueryResolver(TaskServiceClient taskClient,
                         ActivityServiceClient activityClient,
                         NotificationServiceClient notificationClient) {
        this.taskClient = taskClient;
        this.activityClient = activityClient;
        this.notificationClient = notificationClient;
    }

    @QueryMapping
    public List<ProjectDto> myProjects(DataFetchingEnvironment env) {
        String userId = env.getGraphQlContext().get("userId");
        return taskClient.getMyProjects(userId);
    }

    @QueryMapping
    public ProjectDto project(@Argument String id) {
        return taskClient.getProject(id);
    }

    @QueryMapping
    public TaskDto task(@Argument String id) {
        return taskClient.getTask(id);
    }

    @QueryMapping
    public List<ActivityEventDto> taskActivity(@Argument String taskId) {
        return activityClient.getActivityByTask(taskId);
    }

    @QueryMapping
    public List<CommentDto> taskComments(@Argument String taskId) {
        return activityClient.getCommentsByTask(taskId);
    }

    @QueryMapping
    public NotificationDto myNotifications(@Argument Boolean unreadOnly,
                                            DataFetchingEnvironment env) {
        String userId = env.getGraphQlContext().get("userId");
        return notificationClient.getNotifications(userId, unreadOnly);
    }
}
```

- [ ] **Step 4: Write MutationResolver**

Create `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/resolver/MutationResolver.java`:

```java
package dev.kylebradshaw.gateway.resolver;

import dev.kylebradshaw.gateway.client.ActivityServiceClient;
import dev.kylebradshaw.gateway.client.NotificationServiceClient;
import dev.kylebradshaw.gateway.client.TaskServiceClient;
import dev.kylebradshaw.gateway.dto.CommentDto;
import dev.kylebradshaw.gateway.dto.ProjectDto;
import dev.kylebradshaw.gateway.dto.TaskDto;
import graphql.schema.DataFetchingEnvironment;
import java.util.Map;
import org.springframework.graphql.data.method.annotation.Argument;
import org.springframework.graphql.data.method.annotation.MutationMapping;
import org.springframework.stereotype.Controller;

@Controller
public class MutationResolver {

    private final TaskServiceClient taskClient;
    private final ActivityServiceClient activityClient;
    private final NotificationServiceClient notificationClient;

    public MutationResolver(TaskServiceClient taskClient,
                            ActivityServiceClient activityClient,
                            NotificationServiceClient notificationClient) {
        this.taskClient = taskClient;
        this.activityClient = activityClient;
        this.notificationClient = notificationClient;
    }

    @MutationMapping
    public ProjectDto createProject(@Argument Map<String, Object> input,
                                     DataFetchingEnvironment env) {
        String userId = env.getGraphQlContext().get("userId");
        return taskClient.createProject(userId, input);
    }

    @MutationMapping
    public ProjectDto updateProject(@Argument String id, @Argument Map<String, Object> input,
                                     DataFetchingEnvironment env) {
        String userId = env.getGraphQlContext().get("userId");
        return taskClient.updateProject(id, userId, input);
    }

    @MutationMapping
    public boolean deleteProject(@Argument String id, DataFetchingEnvironment env) {
        String userId = env.getGraphQlContext().get("userId");
        taskClient.deleteProject(id, userId);
        return true;
    }

    @MutationMapping
    public TaskDto createTask(@Argument Map<String, Object> input,
                               DataFetchingEnvironment env) {
        String userId = env.getGraphQlContext().get("userId");
        return taskClient.createTask(userId, input);
    }

    @MutationMapping
    public TaskDto updateTask(@Argument String id, @Argument Map<String, Object> input,
                               DataFetchingEnvironment env) {
        String userId = env.getGraphQlContext().get("userId");
        return taskClient.updateTask(id, userId, input);
    }

    @MutationMapping
    public boolean deleteTask(@Argument String id, DataFetchingEnvironment env) {
        String userId = env.getGraphQlContext().get("userId");
        taskClient.deleteTask(id, userId);
        return true;
    }

    @MutationMapping
    public TaskDto assignTask(@Argument String taskId, @Argument String userId,
                               DataFetchingEnvironment env) {
        String actorId = env.getGraphQlContext().get("userId");
        return taskClient.assignTask(taskId, userId, actorId);
    }

    @MutationMapping
    public CommentDto addComment(@Argument String taskId, @Argument String body,
                                  DataFetchingEnvironment env) {
        String userId = env.getGraphQlContext().get("userId");
        return activityClient.addComment(taskId, userId, body);
    }

    @MutationMapping
    public boolean markNotificationRead(@Argument String id, DataFetchingEnvironment env) {
        String userId = env.getGraphQlContext().get("userId");
        notificationClient.markRead(userId, id);
        return true;
    }

    @MutationMapping
    public boolean markAllNotificationsRead(DataFetchingEnvironment env) {
        String userId = env.getGraphQlContext().get("userId");
        notificationClient.markAllRead(userId);
        return true;
    }
}
```

- [ ] **Step 5: Run tests**

```bash
cd java && ./gradlew :gateway-service:test --no-daemon
```

Expected: All tests PASS.

- [ ] **Step 6: Commit**

```bash
git add java/gateway-service/src/main/java/dev/kylebradshaw/gateway/resolver/ \
        java/gateway-service/src/test/java/dev/kylebradshaw/gateway/resolver/
git commit -m "feat(gateway-service): add GraphQL query and mutation resolvers with tests"
```

---

### Task 23: Gateway JWT Security Filter and Interceptor

**Files:**
- Create: `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/security/JwtService.java`
- Create: `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/security/JwtAuthenticationFilter.java`
- Create: `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/config/SecurityConfig.java`
- Create: `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/config/GraphQlInterceptor.java`

- [ ] **Step 1: Write JwtService (gateway copy — validates only, no token generation)**

Create `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/security/JwtService.java`:

```java
package dev.kylebradshaw.gateway.security;

import io.jsonwebtoken.Claims;
import io.jsonwebtoken.Jwts;
import io.jsonwebtoken.security.Keys;
import java.nio.charset.StandardCharsets;
import java.util.UUID;
import javax.crypto.SecretKey;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Service;

@Service
public class JwtService {

    private final SecretKey signingKey;

    public JwtService(@Value("${app.jwt.secret}") String secret) {
        this.signingKey = Keys.hmacShaKeyFor(secret.getBytes(StandardCharsets.UTF_8));
    }

    public UUID extractUserId(String token) {
        Claims claims = parseClaims(token);
        return UUID.fromString(claims.getSubject());
    }

    public boolean isValid(String token) {
        try {
            parseClaims(token);
            return true;
        } catch (Exception e) {
            return false;
        }
    }

    private Claims parseClaims(String token) {
        return Jwts.parser()
                .verifyWith(signingKey)
                .build()
                .parseSignedClaims(token)
                .getPayload();
    }
}
```

- [ ] **Step 2: Write JwtAuthenticationFilter**

Create `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/security/JwtAuthenticationFilter.java`:

```java
package dev.kylebradshaw.gateway.security;

import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import java.io.IOException;
import java.util.List;
import java.util.UUID;
import org.springframework.security.authentication.UsernamePasswordAuthenticationToken;
import org.springframework.security.core.context.SecurityContextHolder;
import org.springframework.stereotype.Component;
import org.springframework.web.filter.OncePerRequestFilter;

@Component
public class JwtAuthenticationFilter extends OncePerRequestFilter {

    private final JwtService jwtService;

    public JwtAuthenticationFilter(JwtService jwtService) {
        this.jwtService = jwtService;
    }

    @Override
    protected void doFilterInternal(HttpServletRequest request, HttpServletResponse response,
                                    FilterChain filterChain) throws ServletException, IOException {
        String header = request.getHeader("Authorization");
        if (header != null && header.startsWith("Bearer ")) {
            String token = header.substring(7);
            if (jwtService.isValid(token)) {
                UUID userId = jwtService.extractUserId(token);
                var auth = new UsernamePasswordAuthenticationToken(
                        userId.toString(), null, List.of());
                SecurityContextHolder.getContext().setAuthentication(auth);
            }
        }
        filterChain.doFilter(request, response);
    }
}
```

- [ ] **Step 3: Write SecurityConfig**

Create `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/config/SecurityConfig.java`:

```java
package dev.kylebradshaw.gateway.config;

import dev.kylebradshaw.gateway.security.JwtAuthenticationFilter;
import java.util.List;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.security.config.annotation.web.builders.HttpSecurity;
import org.springframework.security.config.annotation.web.configuration.EnableWebSecurity;
import org.springframework.security.config.http.SessionCreationPolicy;
import org.springframework.security.web.SecurityFilterChain;
import org.springframework.security.web.authentication.UsernamePasswordAuthenticationFilter;
import org.springframework.web.cors.CorsConfiguration;
import org.springframework.web.cors.CorsConfigurationSource;
import org.springframework.web.cors.UrlBasedCorsConfigurationSource;

@Configuration
@EnableWebSecurity
public class SecurityConfig {

    private final JwtAuthenticationFilter jwtFilter;

    @Value("${app.allowed-origins}")
    private String allowedOrigins;

    public SecurityConfig(JwtAuthenticationFilter jwtFilter) {
        this.jwtFilter = jwtFilter;
    }

    @Bean
    public SecurityFilterChain filterChain(HttpSecurity http) throws Exception {
        return http
                .cors(cors -> cors.configurationSource(corsConfigurationSource()))
                .csrf(csrf -> csrf.disable())
                .sessionManagement(s -> s.sessionCreationPolicy(SessionCreationPolicy.STATELESS))
                .authorizeHttpRequests(auth -> auth
                        .requestMatchers("/graphiql/**", "/actuator/health").permitAll()
                        .anyRequest().authenticated()
                )
                .addFilterBefore(jwtFilter, UsernamePasswordAuthenticationFilter.class)
                .build();
    }

    @Bean
    public CorsConfigurationSource corsConfigurationSource() {
        CorsConfiguration config = new CorsConfiguration();
        config.setAllowedOrigins(List.of(allowedOrigins.split(",")));
        config.setAllowedMethods(List.of("GET", "POST", "OPTIONS"));
        config.setAllowedHeaders(List.of("*"));
        config.setAllowCredentials(true);
        UrlBasedCorsConfigurationSource source = new UrlBasedCorsConfigurationSource();
        source.registerCorsConfiguration("/**", config);
        return source;
    }
}
```

- [ ] **Step 4: Write GraphQL interceptor to pass userId to resolvers**

Create `java/gateway-service/src/main/java/dev/kylebradshaw/gateway/config/GraphQlInterceptor.java`:

```java
package dev.kylebradshaw.gateway.config;

import org.springframework.graphql.server.WebGraphQlInterceptor;
import org.springframework.graphql.server.WebGraphQlRequest;
import org.springframework.graphql.server.WebGraphQlResponse;
import org.springframework.security.core.context.SecurityContextHolder;
import org.springframework.stereotype.Component;
import reactor.core.publisher.Mono;

@Component
public class GraphQlInterceptor implements WebGraphQlInterceptor {

    @Override
    public Mono<WebGraphQlResponse> intercept(WebGraphQlRequest request, Chain chain) {
        var auth = SecurityContextHolder.getContext().getAuthentication();
        if (auth != null && auth.getPrincipal() instanceof String userId) {
            request.configureExecutionInput((input, builder) ->
                    builder.graphQLContext(ctx -> ctx.put("userId", userId)).build()
            );
        }
        return chain.next(request);
    }
}
```

- [ ] **Step 5: Verify compilation**

```bash
cd java && ./gradlew :gateway-service:compileJava --no-daemon
```

Expected: BUILD SUCCESSFUL.

- [ ] **Step 6: Commit**

```bash
git add java/gateway-service/src/main/java/dev/kylebradshaw/gateway/security/ \
        java/gateway-service/src/main/java/dev/kylebradshaw/gateway/config/
git commit -m "feat(gateway-service): add JWT security filter and GraphQL context interceptor"
```

---

## Phase 7: Docker Compose Full Stack and Smoke Test

### Task 24: Docker Compose with All Services

**Files:**
- Create: `java/task-service/Dockerfile`
- Create: `java/activity-service/Dockerfile`
- Create: `java/notification-service/Dockerfile`
- Create: `java/gateway-service/Dockerfile`
- Modify: `java/docker-compose.yml`

- [ ] **Step 1: Write shared Dockerfile pattern — task-service**

Create `java/task-service/Dockerfile`:

```dockerfile
FROM eclipse-temurin:21-jre-alpine

WORKDIR /app

COPY build/libs/*.jar app.jar

EXPOSE 8081

ENTRYPOINT ["java", "-jar", "app.jar"]
```

- [ ] **Step 2: Write activity-service Dockerfile**

Create `java/activity-service/Dockerfile`:

```dockerfile
FROM eclipse-temurin:21-jre-alpine

WORKDIR /app

COPY build/libs/*.jar app.jar

EXPOSE 8082

ENTRYPOINT ["java", "-jar", "app.jar"]
```

- [ ] **Step 3: Write notification-service Dockerfile**

Create `java/notification-service/Dockerfile`:

```dockerfile
FROM eclipse-temurin:21-jre-alpine

WORKDIR /app

COPY build/libs/*.jar app.jar

EXPOSE 8083

ENTRYPOINT ["java", "-jar", "app.jar"]
```

- [ ] **Step 4: Write gateway-service Dockerfile**

Create `java/gateway-service/Dockerfile`:

```dockerfile
FROM eclipse-temurin:21-jre-alpine

WORKDIR /app

COPY build/libs/*.jar app.jar

EXPOSE 8080

ENTRYPOINT ["java", "-jar", "app.jar"]
```

- [ ] **Step 5: Update docker-compose.yml to include all services**

Replace `java/docker-compose.yml` with:

```yaml
services:
  postgres:
    image: postgres:17-alpine
    ports:
      - "5432:5432"
    environment:
      POSTGRES_DB: taskdb
      POSTGRES_USER: ${POSTGRES_USER:-taskuser}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-taskpass}
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U taskuser -d taskdb"]
      interval: 5s
      timeout: 3s
      retries: 5

  mongodb:
    image: mongo:7
    ports:
      - "27017:27017"
    volumes:
      - mongo_data:/data/db
    healthcheck:
      test: ["CMD", "mongosh", "--eval", "db.adminCommand('ping')"]
      interval: 5s
      timeout: 3s
      retries: 5

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 5

  rabbitmq:
    image: rabbitmq:3-management-alpine
    ports:
      - "5672:5672"
      - "15672:15672"
    environment:
      RABBITMQ_DEFAULT_USER: ${RABBITMQ_USER:-guest}
      RABBITMQ_DEFAULT_PASS: ${RABBITMQ_PASSWORD:-guest}
    volumes:
      - rabbitmq_data:/var/lib/rabbitmq
    healthcheck:
      test: ["CMD", "rabbitmq-diagnostics", "-q", "ping"]
      interval: 5s
      timeout: 3s
      retries: 5

  task-service:
    build: ./task-service
    ports:
      - "8081:8081"
    environment:
      POSTGRES_HOST: postgres
      RABBITMQ_HOST: rabbitmq
      JWT_SECRET: ${JWT_SECRET:-dev-secret-key-at-least-32-characters-long}
      GOOGLE_CLIENT_ID: ${GOOGLE_CLIENT_ID:-}
      GOOGLE_CLIENT_SECRET: ${GOOGLE_CLIENT_SECRET:-}
      ALLOWED_ORIGINS: ${ALLOWED_ORIGINS:-http://localhost:3000}
    depends_on:
      postgres:
        condition: service_healthy
      rabbitmq:
        condition: service_healthy

  activity-service:
    build: ./activity-service
    ports:
      - "8082:8082"
    environment:
      MONGODB_HOST: mongodb
      RABBITMQ_HOST: rabbitmq
    depends_on:
      mongodb:
        condition: service_healthy
      rabbitmq:
        condition: service_healthy

  notification-service:
    build: ./notification-service
    ports:
      - "8083:8083"
    environment:
      REDIS_HOST: redis
      RABBITMQ_HOST: rabbitmq
    depends_on:
      redis:
        condition: service_healthy
      rabbitmq:
        condition: service_healthy

  gateway-service:
    build: ./gateway-service
    ports:
      - "8080:8080"
    environment:
      JWT_SECRET: ${JWT_SECRET:-dev-secret-key-at-least-32-characters-long}
      TASK_SERVICE_URL: http://task-service:8081
      ACTIVITY_SERVICE_URL: http://activity-service:8082
      NOTIFICATION_SERVICE_URL: http://notification-service:8083
      ALLOWED_ORIGINS: ${ALLOWED_ORIGINS:-http://localhost:3000}
    depends_on:
      - task-service
      - activity-service
      - notification-service

volumes:
  postgres_data:
  mongo_data:
  redis_data:
  rabbitmq_data:
```

- [ ] **Step 6: Build all JARs**

```bash
cd java && ./gradlew build --no-daemon
```

Expected: BUILD SUCCESSFUL for all 4 services.

- [ ] **Step 7: Commit**

```bash
git add java/task-service/Dockerfile java/activity-service/Dockerfile \
        java/notification-service/Dockerfile java/gateway-service/Dockerfile \
        java/docker-compose.yml
git commit -m "feat(java): add Dockerfiles and full-stack Docker Compose"
```

---

### Task 25: Integration Test with Testcontainers (task-service)

**Files:**
- Create: `java/task-service/src/test/java/dev/kylebradshaw/task/integration/TaskServiceIntegrationTest.java`

- [ ] **Step 1: Write integration test**

Create `java/task-service/src/test/java/dev/kylebradshaw/task/integration/TaskServiceIntegrationTest.java`:

```java
package dev.kylebradshaw.task.integration;

import static org.assertj.core.api.Assertions.assertThat;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

import com.fasterxml.jackson.databind.ObjectMapper;
import dev.kylebradshaw.task.dto.CreateProjectRequest;
import dev.kylebradshaw.task.entity.User;
import dev.kylebradshaw.task.repository.UserRepository;
import java.util.Map;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.AutoConfigureMockMvc;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.http.MediaType;
import org.springframework.test.context.DynamicPropertyRegistry;
import org.springframework.test.context.DynamicPropertySource;
import org.springframework.test.web.servlet.MockMvc;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.containers.RabbitMQContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

@SpringBootTest
@AutoConfigureMockMvc
@Testcontainers
class TaskServiceIntegrationTest {

    @Container
    static PostgreSQLContainer<?> postgres = new PostgreSQLContainer<>("postgres:17-alpine")
            .withDatabaseName("taskdb")
            .withUsername("test")
            .withPassword("test");

    @Container
    static RabbitMQContainer rabbitmq = new RabbitMQContainer("rabbitmq:3-management-alpine");

    @DynamicPropertySource
    static void configureProperties(DynamicPropertyRegistry registry) {
        registry.add("spring.datasource.url", postgres::getJdbcUrl);
        registry.add("spring.datasource.username", postgres::getUsername);
        registry.add("spring.datasource.password", postgres::getPassword);
        registry.add("spring.rabbitmq.host", rabbitmq::getHost);
        registry.add("spring.rabbitmq.port", rabbitmq::getAmqpPort);
        registry.add("app.jwt.secret",
                () -> "integration-test-secret-key-at-least-32-characters");
        registry.add("app.jwt.access-token-ttl-ms", () -> "900000");
        registry.add("app.jwt.refresh-token-ttl-ms", () -> "604800000");
        registry.add("app.allowed-origins", () -> "http://localhost:3000");
        registry.add("app.google.client-id", () -> "test-client-id");
        registry.add("app.google.client-secret", () -> "test-client-secret");
    }

    @Autowired
    private MockMvc mockMvc;

    @Autowired
    private ObjectMapper objectMapper;

    @Autowired
    private UserRepository userRepo;

    private User testUser;

    @BeforeEach
    void setUp() {
        testUser = userRepo.findByEmail("integration@test.com")
                .orElseGet(() -> userRepo.save(
                        new User("integration@test.com", "Integration User", null)));
    }

    @Test
    void createAndGetProject() throws Exception {
        var request = new CreateProjectRequest("Integration Project", "Testing");

        String responseJson = mockMvc.perform(post("/api/projects")
                        .header("X-User-Id", testUser.getId().toString())
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(objectMapper.writeValueAsString(request)))
                .andExpect(status().isCreated())
                .andExpect(jsonPath("$.name").value("Integration Project"))
                .andReturn().getResponse().getContentAsString();

        mockMvc.perform(get("/api/projects")
                        .header("X-User-Id", testUser.getId().toString()))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$[?(@.name == 'Integration Project')]").exists());
    }

    @Test
    void createTask_viaProject() throws Exception {
        var projectReq = new CreateProjectRequest("Task Test Project", "For tasks");
        String projectJson = mockMvc.perform(post("/api/projects")
                        .header("X-User-Id", testUser.getId().toString())
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(objectMapper.writeValueAsString(projectReq)))
                .andExpect(status().isCreated())
                .andReturn().getResponse().getContentAsString();

        String projectId = objectMapper.readTree(projectJson).get("id").asText();

        mockMvc.perform(post("/api/tasks")
                        .header("X-User-Id", testUser.getId().toString())
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(objectMapper.writeValueAsString(Map.of(
                                "projectId", projectId,
                                "title", "Integration Task",
                                "priority", "HIGH"
                        ))))
                .andExpect(status().isCreated())
                .andExpect(jsonPath("$.title").value("Integration Task"))
                .andExpect(jsonPath("$.status").value("TODO"));
    }
}
```

- [ ] **Step 2: Run integration test**

```bash
cd java && ./gradlew :task-service:test --tests "dev.kylebradshaw.task.integration.TaskServiceIntegrationTest" --no-daemon
```

Expected: All 2 tests PASS (requires Docker running for Testcontainers).

- [ ] **Step 3: Commit**

```bash
git add java/task-service/src/test/java/dev/kylebradshaw/task/integration/
git commit -m "feat(task-service): add Testcontainers integration tests"
```

---

### Task 26: Run Full Test Suite

- [ ] **Step 1: Run all tests across all modules**

```bash
cd java && ./gradlew test --no-daemon
```

Expected: All tests PASS across all 4 modules.

- [ ] **Step 2: Run Checkstyle**

```bash
cd java && ./gradlew checkstyleMain checkstyleTest --no-daemon
```

Expected: BUILD SUCCESSFUL — no style violations.

- [ ] **Step 3: Fix any issues, then commit**

```bash
git add -A java/
git commit -m "chore(java): fix any checkstyle issues from full test run"
```

(Skip this commit if no changes needed.)

---

## Summary

**26 tasks** across 7 phases covering:
- Gradle multi-module scaffolding
- task-service: JPA entities, repositories, services, controllers, auth, JWT, security, RabbitMQ publishing
- activity-service: MongoDB documents, RabbitMQ consumer, comments, activity API
- notification-service: Redis data model, RabbitMQ consumer, notifications API
- gateway-service: GraphQL schema, resolvers, JWT filter, downstream REST clients
- Docker Compose for full stack
- Integration tests with Testcontainers

**Not covered in this plan (separate plans):**
- Frontend (Next.js pages at `/java/*`, Apollo Client, components)
- CI/CD (GitHub Actions `java-ci.yml`, Checkstyle, SpotBugs, Testcontainers in CI)
- Kubernetes (Minikube manifests in `java/k8s/`)
