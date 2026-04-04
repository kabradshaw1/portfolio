# ADR 05 — Polyglot Persistence: MongoDB for Activity Logs, Redis for Notifications

## Overview

Not every piece of data in a system belongs in the same database. The Java Task Management project deliberately uses three different data stores — PostgreSQL for tasks and projects, MongoDB for activity events and comments, and Redis for notifications — because each store is optimized for a different access pattern and data shape. This is called **polyglot persistence**: the idea that you should pick the right tool for the job rather than forcing every domain into one database technology.

This document walks through how the `activity-service` uses MongoDB to store audit logs with flexible, event-specific metadata, and how the `notification-service` uses Redis sorted sets to maintain per-user notification feeds with O(log N) insertion and instant delivery. By the end, you will understand why these choices were made, how Spring's data abstractions make them feel consistent despite the underlying differences, and where the sharp edges are.

---

## Architecture Context

```
gateway-service (GraphQL)
       |
       +---> task-service       (PostgreSQL — structured relational data)
       |
       +---> activity-service   (MongoDB — flexible document store)
       |          |
       |     activity_events collection  (audit log: what happened, by whom, with what metadata)
       |     comments collection         (threaded comments per task)
       |
       +---> notification-service (Redis — in-memory sorted sets)
                   |
             notifications:{userId}  (sorted set, score = epoch ms)
             notification_count:{userId}  (string counter)
```

The task-service owns the source of truth for tasks and projects. When a task is created, updated, or assigned, the gateway calls the task-service directly and then fires events downstream. The activity-service records what happened (the audit trail). The notification-service delivers real-time alerts to the affected user.

**Why these two services don't use PostgreSQL:**

- Activity events have a variable `metadata` field — a task creation event carries different fields than an assignment event. Forcing this into SQL requires either a generic key-value table (ugly, hard to query) or separate tables per event type (rigid, migration-heavy). MongoDB's schemaless documents are a natural fit.
- Notifications are ephemeral, user-scoped, and need sub-millisecond writes. Redis sorted sets give you ordered-by-timestamp storage with O(log N) insert, O(1) count, and instant expiry. PostgreSQL would require a polling query or a notification channel — much more infrastructure for a feature that doesn't need durability guarantees.

---

## Package Introductions

### Spring Data MongoDB

**What it is:** A Spring Data module that maps Java objects to MongoDB documents and provides repository interfaces that generate queries from method names — the same pattern you saw with Spring Data JPA in ADR 02.

**What you get:**
- `@Document` — annotates a class as a MongoDB collection mapping
- `@Id` — marks the field that maps to MongoDB's `_id` field
- `MongoRepository<T, ID>` — like `JpaRepository`, gives you `save`, `findById`, `findAll`, `delete`, etc. for free
- Derived query methods — `findByTaskIdOrderByTimestampDesc(String taskId)` is fully implemented by Spring based on the method name alone, no query written

**Maven dependency:**
```xml
<dependency>
    <groupId>org.springframework.boot</groupId>
    <artifactId>spring-boot-starter-data-mongodb</artifactId>
</dependency>
```

**Configuration (application.yml):**
```yaml
spring:
  data:
    mongodb:
      uri: ${MONGODB_URI:mongodb://localhost:27017/taskmanagement}
```

**Alternatives considered:**

| Option | Why not chosen |
|---|---|
| Raw MongoDB Java Driver (`MongoClient`) | Verbose. You write BSON manually. Spring Data wraps this for you. |
| Morphia (MongoDB ODM) | Third-party, less maintained, no Spring integration. |
| Spring Data JPA with PostgreSQL JSONB | Could store flexible data in a JSONB column, but loses MongoDB's native document querying and requires PostgreSQL which is already handling relational data. |
| Elasticsearch | Better for full-text search than audit logging. Heavier operational overhead. |

---

### Spring Data Redis — StringRedisTemplate

**What it is:** A Spring template class for interacting with Redis. The `String` variant serializes both keys and values as plain strings, which gives you full control over serialization — you serialize to JSON yourself before storing, and deserialize after reading.

**What you get:**
- `opsForZSet()` — operations on Redis sorted sets (add, remove, range queries by rank or score)
- `opsForValue()` — operations on Redis string values (get, set, increment, decrement)
- `opsForHash()`, `opsForList()`, `opsForSet()` — other Redis data structure operations (not used here)

**Maven dependency:**
```xml
<dependency>
    <groupId>org.springframework.boot</groupId>
    <artifactId>spring-boot-starter-data-redis</artifactId>
</dependency>
```

**Configuration:**
```yaml
spring:
  data:
    redis:
      host: ${REDIS_HOST:localhost}
      port: ${REDIS_PORT:6379}
```

**Alternatives considered:**

| Option | Why not chosen |
|---|---|
| `RedisTemplate<String, Object>` | Uses Java serialization by default — binary blobs, unreadable in redis-cli, fragile across deploys. `StringRedisTemplate` with manual JSON is more portable. |
| `ReactiveRedisTemplate` | Requires WebFlux/reactive stack. This project uses Spring MVC (servlet stack). Mixing reactive and servlet in one service is painful. |
| Redisson | Higher-level distributed primitives (locks, queues). Heavier dependency. Overkill for a sorted-set notification feed. |
| Spring Cache + Redis | Cache abstraction is great for caching method results, but too high-level for the sorted-set-specific operations needed here. |

---

### Jackson + JavaTimeModule

**What it is:** Jackson is the de-facto Java JSON library. `JavaTimeModule` is an extension that teaches Jackson to serialize and deserialize `java.time` classes — `Instant`, `LocalDate`, `ZonedDateTime`, etc.

By default, without `JavaTimeModule`, Jackson serializes an `Instant` as an array `[seconds, nanos]`. With the module registered and `WRITE_DATES_AS_TIMESTAMPS` disabled, it serializes as an ISO-8601 string: `"2024-01-15T10:30:00Z"`. This matters for Redis because the stored JSON must be human-readable and round-trippable.

```java
this.objectMapper = new ObjectMapper();
this.objectMapper.registerModule(new JavaTimeModule());
this.objectMapper.disable(SerializationFeature.WRITE_DATES_AS_TIMESTAMPS);
```

---

## Go / TypeScript Comparison

| Concept | Go | TypeScript | Java (Spring) |
|---|---|---|---|
| MongoDB client | `go.mongodb.org/mongo-driver` — explicit `context`, BSON tags | `mongoose` (ODM) — schema definitions with `Schema` class | `spring-boot-starter-data-mongodb` — `@Document`, `MongoRepository` |
| MongoDB document type | `struct` with `bson:"field"` tags | Mongoose `Document` extending a schema | Plain Java class with `@Document` and `@Id` |
| Query generation | Manual `bson.D{{"taskId", taskId}}` filter | Mongoose `Model.find({ taskId })` | Method name: `findByTaskIdOrderByTimestampDesc` |
| Flexible metadata field | `map[string]interface{}` | `Record<string, unknown>` or `mongoose.Schema.Types.Mixed` | `Map<String, Object>` |
| Redis client | `go-redis/redis` — `client.ZAdd`, `client.ZRange` | `ioredis` — `zadd`, `zrange`, `zrevrange` | `StringRedisTemplate` — `opsForZSet().add()`, `opsForZSet().reverseRange()` |
| Redis serialization | `json.Marshal` / `json.Unmarshal` | `JSON.stringify` / `JSON.parse` | `objectMapper.writeValueAsString` / `objectMapper.readValue` |
| Redis sorted set score | `float64` Unix milliseconds | `number` Unix milliseconds | `double` epoch millis from `Instant.toEpochMilli()` |
| Increment counter | `client.Incr(ctx, key)` | `redis.incr(key)` | `redisTemplate.opsForValue().increment(key)` |
| JSON time serialization | `time.Time` marshals to RFC3339 by default | `Date.toISOString()` | Requires `JavaTimeModule` — won't work without it |
| Repository pattern | Manual interface + implementation | Mongoose model is the repository | `MongoRepository` generates all CRUD — no implementation needed |

**The biggest conceptual shift from Go:** In Go, you write the MongoDB query yourself using BSON filters. In Spring, you name the method correctly and the framework generates the query at startup. It feels like magic until you understand the naming conventions.

**The biggest conceptual shift from TypeScript/Mongoose:** Mongoose defines the schema in code (model-first), and that schema enforces structure at the application level. Spring Data MongoDB has no schema — you define the Java class and MongoDB stores whatever documents you send. The `@Document` annotation is purely for mapping, not enforcement.

---

## Build It

### Step 1 — The ActivityEvent Document

```java
// activity-service/src/main/java/dev/kylebradshaw/activity/document/ActivityEvent.java

@Document(collection = "activity_events")  // (1)
public class ActivityEvent {
    @Id private String id;                  // (2)
    private String projectId;
    private String taskId;
    private String actorId;
    private String eventType;
    private Map<String, Object> metadata;   // (3)
    private Instant timestamp;

    public ActivityEvent() {}              // (4)

    public ActivityEvent(String projectId, String taskId, String actorId,
                         String eventType, Map<String, Object> metadata) {
        this.projectId = projectId;
        this.taskId = taskId;
        this.actorId = actorId;
        this.eventType = eventType;
        this.metadata = metadata;
        this.timestamp = Instant.now();    // (5)
    }
    // ... getters
}
```

**(1) `@Document(collection = "activity_events")`**
Tells Spring Data MongoDB that instances of this class map to documents in the `activity_events` collection. If `collection` is omitted, Spring uses the lowercased class name. This is analogous to Mongoose's `mongoose.model('ActivityEvent', schema)` — it binds the class to a collection name.

**(2) `@Id private String id`**
MongoDB's primary key is always `_id`. Spring maps the `@Id`-annotated field to `_id` automatically. Using `String` means MongoDB will store the auto-generated `ObjectId` as its hex string representation (e.g., `"507f1f77bcf86cd799439011"`). You could also use `ObjectId` directly, but `String` is simpler to pass through REST APIs and GraphQL.

**(3) `Map<String, Object> metadata`**
This is the whole reason MongoDB is the right choice for this domain. A task-created event might carry:
```json
{ "title": "Fix login bug", "priority": "HIGH" }
```
While a task-assigned event carries:
```json
{ "assigneeId": "user-123", "assigneeName": "Alice" }
```
If this were PostgreSQL, you'd need a JSONB column or a separate `event_metadata` table with a polymorphic design. In MongoDB, each document just has different fields in the embedded `metadata` object. In Go: `map[string]interface{}`. In TypeScript: `Record<string, unknown>`. In Java: `Map<String, Object>`. Same concept, different syntax.

**(4) `public ActivityEvent() {}`**
The no-args constructor is **required** for Spring Data MongoDB (and JPA). The framework uses reflection to deserialize documents from MongoDB into Java objects. Without this constructor, deserialization fails at runtime with a `MappingInstantiationException`. This is a Java-specific constraint — Go structs and TypeScript classes don't have this requirement.

**(5) `this.timestamp = Instant.now()`**
`Instant` is the right type for timestamps in modern Java — it's always UTC, has nanosecond precision, and serializes cleanly. MongoDB stores it as a `Date` type natively. Spring Data handles the conversion automatically.

---

### Step 2 — The Comment Document

```java
// activity-service/src/main/java/dev/kylebradshaw/activity/document/Comment.java

@Document(collection = "comments")
public class Comment {
    @Id private String id;
    private String taskId;
    private String authorId;
    private String body;
    private Instant createdAt;

    public Comment() {}

    public Comment(String taskId, String authorId, String body) {
        this.taskId = taskId;
        this.authorId = authorId;
        this.body = body;
        this.createdAt = Instant.now();
    }
    // ... getters
}
```

Comments are a simpler document — no flexible metadata, just a fixed schema. They live in a separate collection from activity events rather than being embedded inside them. This is a deliberate denormalization decision:

- **Separate collections:** Comments can be queried independently by taskId without loading all activity events. The gateway fetches comments and activity independently via different endpoints.
- **Why not embed comments inside the task document in task-service?** Comments grow unboundedly. Embedding them inside a task document in PostgreSQL or even MongoDB would mean fetching the entire comment history every time you load a task. Separate documents let you paginate.

---

### Step 3 — The Repositories

```java
// ActivityEventRepository.java
public interface ActivityEventRepository extends MongoRepository<ActivityEvent, String> {
    List<ActivityEvent> findByTaskIdOrderByTimestampDesc(String taskId);
    List<ActivityEvent> findByProjectIdOrderByTimestampDesc(String projectId);
}

// CommentRepository.java
public interface CommentRepository extends MongoRepository<Comment, String> {
    List<Comment> findByTaskIdOrderByCreatedAtAsc(String taskId);
}
```

These interfaces have **no implementation**. Spring Data generates the implementation at startup by parsing the method names.

**How the naming convention works:**

`findByTaskIdOrderByTimestampDesc`

| Segment | Meaning |
|---|---|
| `findBy` | SELECT / find |
| `TaskId` | WHERE taskId = ? (matches the `taskId` field) |
| `OrderBy` | ORDER BY |
| `Timestamp` | the `timestamp` field |
| `Desc` | descending |

This generates the MongoDB equivalent of:
```javascript
db.activity_events.find({ taskId: taskId }).sort({ timestamp: -1 })
```

For comments: `findByTaskIdOrderByCreatedAtAsc` generates:
```javascript
db.comments.find({ taskId: taskId }).sort({ createdAt: 1 })
```

**In Go**, this would require:
```go
cursor, err := collection.Find(ctx,
    bson.D{{"taskId", taskId}},
    options.Find().SetSort(bson.D{{"timestamp", -1}}),
)
```
Three lines of manual BSON construction versus one method signature in Java.

**In TypeScript/Mongoose:**
```typescript
ActivityEvent.find({ taskId }).sort({ timestamp: -1 })
```
Closer to Spring's ergonomics, but you still write the query explicitly. Spring generates it from the name.

The `MongoRepository<ActivityEvent, String>` generic parameters mean: "this repository manages `ActivityEvent` documents whose `@Id` field is of type `String`."

---

### Step 4 — The CommentService

```java
// CommentService.java
@Service
public class CommentService {
    private final CommentRepository commentRepo;

    public CommentService(CommentRepository commentRepo) {  // (1)
        this.commentRepo = commentRepo;
    }

    public Comment addComment(String taskId, String authorId, String body) {
        return commentRepo.save(new Comment(taskId, authorId, body));  // (2)
    }

    public List<Comment> getCommentsByTask(String taskId) {
        return commentRepo.findByTaskIdOrderByCreatedAtAsc(taskId);    // (3)
    }
}
```

**(1) Constructor injection**
Spring injects the `CommentRepository` implementation (generated at startup) into the service. This is the standard Spring DI pattern — no `@Autowired` annotation needed when there is a single constructor.

**(2) `commentRepo.save(new Comment(...))`**
`save()` is provided by `MongoRepository`. If the entity has no `@Id` value, MongoDB generates one and populates the `id` field on the returned object. The returned `Comment` from `save()` is the saved entity with the generated id — use the return value, not the input.

**(3) The derived query**
Spring generates the MongoDB query from the method name at application startup. At runtime, it is just a collection find with sort — no reflection overhead in the hot path.

---

### Step 5 — The Notification DTO (Java Record)

```java
// Notification.java
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
            UUID.randomUUID().toString(),
            type,
            message,
            taskId,
            false,
            Instant.now()
        );
    }
}
```

Java records (introduced in Java 16, stable in Java 17+) are immutable data carriers. The compiler generates: the constructor, all getters (named `id()`, `type()`, etc. — no `get` prefix), `equals()`, `hashCode()`, and `toString()`. This is Java's equivalent of TypeScript interfaces with readonly fields, or Go structs without methods.

Notice: `boolean read` (primitive) not `Boolean read` (boxed). Primitive boolean cannot be null. This is intentional — a notification is either read or not. If it could be null, you'd use `Boolean`.

The `create` factory method is a convenience that generates a UUID and stamps the current time. Factory methods on records replace constructor overloading.

---

### Step 6 — Writing to Redis (addNotification)

```java
// NotificationService.java
@Service
public class NotificationService {

    private final StringRedisTemplate redisTemplate;
    private final ObjectMapper objectMapper;

    public NotificationService(StringRedisTemplate redisTemplate) {
        this.redisTemplate = redisTemplate;
        this.objectMapper = new ObjectMapper();
        this.objectMapper.registerModule(new JavaTimeModule());          // (1)
        this.objectMapper.disable(SerializationFeature.WRITE_DATES_AS_TIMESTAMPS); // (2)
    }

    public void addNotification(String userId, Notification notification) {
        try {
            String json = objectMapper.writeValueAsString(notification); // (3)
            double score = notification.createdAt().toEpochMilli();      // (4)
            redisTemplate.opsForZSet()
                .add("notifications:" + userId, json, score);            // (5)
            redisTemplate.opsForValue()
                .increment("notification_count:" + userId);              // (6)
        } catch (JsonProcessingException e) {
            throw new RuntimeException("Failed to serialize notification", e);
        }
    }
```

**(1) `registerModule(new JavaTimeModule())`**
Without this, Jackson throws `InvalidDefinitionException: Java 8 date/time type not supported` when it encounters `Instant`. The `JavaTimeModule` is in the `jackson-datatype-jsr310` artifact, automatically included transitively by Spring Boot.

**(2) `disable(SerializationFeature.WRITE_DATES_AS_TIMESTAMPS)`**
By default, even with `JavaTimeModule`, `Instant` serializes as `[1705314600, 123456789]` (seconds + nanos array). Disabling timestamps produces `"2024-01-15T10:30:00Z"` — ISO-8601 string, much more readable and portable.

**(3) `objectMapper.writeValueAsString(notification)`**
Serializes the record to a JSON string. Jackson uses the record's component accessor methods (`id()`, `type()`, etc.) to read the fields. The result looks like:
```json
{"id":"abc-123","type":"TASK_ASSIGNED","message":"You were assigned Fix login bug","taskId":"task-456","read":false,"createdAt":"2024-01-15T10:30:00Z"}
```

**(4) `notification.createdAt().toEpochMilli()`**
The Redis sorted set score is a `double`. We use the creation timestamp in milliseconds as the score. This means the sorted set is automatically ordered chronologically — lower score = older, higher score = newer.

**(5) `opsForZSet().add(key, value, score)`**
`ZADD notifications:user-123 1705314600000 "{...json...}"`

The key pattern `notifications:{userId}` gives each user their own sorted set. In Go:
```go
client.ZAdd(ctx, "notifications:"+userId, &redis.Z{Score: score, Member: json})
```
In TypeScript/ioredis:
```typescript
await redis.zadd(`notifications:${userId}`, score, json)
```

**(6) `opsForValue().increment(key)`**
`INCR notification_count:user-123`

A separate string key holds the unread count. Incrementing it is O(1) and atomic in Redis. Reading the count for the UI badge doesn't require scanning the sorted set.

---

### Step 7 — Reading from Redis (getNotifications)

```java
public NotificationResponse getNotifications(String userId, boolean unreadOnly) {
    Set<String> entries = redisTemplate.opsForZSet()
            .reverseRange("notifications:" + userId, 0, -1);   // (1)

    List<Notification> notifications = new ArrayList<>();
    if (entries != null) {
        for (String json : entries) {
            try {
                Notification n = objectMapper.readValue(json, Notification.class); // (2)
                if (!unreadOnly || !n.read()) {                                    // (3)
                    notifications.add(n);
                }
            } catch (JsonProcessingException e) {
                // skip malformed entries                                           // (4)
            }
        }
    }

    String countStr = redisTemplate.opsForValue()
            .get("notification_count:" + userId);                               // (5)
    long unreadCount = countStr != null ? Long.parseLong(countStr) : 0L;

    return new NotificationResponse(notifications, unreadCount);
}
```

**(1) `reverseRange(key, 0, -1)`**
`ZREVRANGE notifications:user-123 0 -1`

Returns all members, highest score first (newest first). Range `0, -1` means "from rank 0 to rank -1" — in Redis, -1 means "the last element," so this is "all elements." For pagination, you would use `reverseRange(key, 0, 49)` for the first 50.

**(2) `objectMapper.readValue(json, Notification.class)`**
Deserializes the JSON string back into a `Notification` record. Jackson maps JSON fields to the record components by name. The `Instant` field is deserialized back from the ISO-8601 string because `JavaTimeModule` is registered.

**(3) `if (!unreadOnly || !n.read())`**
If `unreadOnly` is false, include everything. If `unreadOnly` is true, only include notifications where `read` is false. Short-circuit logic: if the first condition is true (not unread-only filter), skip the second check.

**(4) Swallowing `JsonProcessingException`**
If a Redis entry is somehow corrupt or written by a different schema version, this prevents one bad entry from crashing the entire notifications read. In production, you would log a warning or push to a dead-letter structure rather than silently dropping.

**(5) `opsForValue().get(key)`**
`GET notification_count:user-123`

Returns a `String` (may be null if the key doesn't exist). Parsed to `long` with a null-safe default of 0.

---

### Step 8 — The markRead Pattern (Remove + Re-Add)

```java
public void markRead(String userId, String notificationId) {
    Set<String> entries = redisTemplate.opsForZSet()
            .reverseRange("notifications:" + userId, 0, -1);   // (1)

    if (entries == null) return;

    for (String json : entries) {
        try {
            Notification n = objectMapper.readValue(json, Notification.class);
            if (n.id().equals(notificationId) && !n.read()) {             // (2)
                redisTemplate.opsForZSet()
                        .remove("notifications:" + userId, json);          // (3)
                Notification updated = new Notification(
                        n.id(), n.type(), n.message(), n.taskId(),
                        true,                                              // (4)
                        n.createdAt());
                String updatedJson = objectMapper.writeValueAsString(updated);
                redisTemplate.opsForZSet()
                        .add("notifications:" + userId, updatedJson,
                             n.createdAt().toEpochMilli());               // (5)
                redisTemplate.opsForValue()
                        .decrement("notification_count:" + userId);       // (6)
                return;
            }
        } catch (JsonProcessingException e) {
            // skip
        }
    }
}
```

This is the most algorithmically interesting part of the notification service. Redis sorted sets are indexed by score and by member value — they don't have a concept of "update the fields of element X." The member IS the stored string. To update the `read` flag:

**(1) Scan the sorted set**
Load all entries. This is O(N) in the number of notifications. For a typical user with tens or low hundreds of notifications, this is fine. At thousands of notifications, you'd want a secondary index (e.g., a Hash keyed by notification ID).

**(2) Find by ID**
Parse each JSON entry and check if it's the notification we're looking for, and that it's currently unread. If it's already marked read, skip it — no work to do.

**(3) `opsForZSet().remove(key, member)`**
`ZREM notifications:user-123 "{...old json...}"`

Removes the exact string from the sorted set. The string must match exactly — this is why it's critical that the same `ObjectMapper` configuration (and same field ordering) is used for serialization and deserialization.

**(4) Create updated record**
Records are immutable — you cannot set `read = true` on an existing instance. Instead, create a new `Notification` with all the same fields except `read` set to `true`. This is the functional programming style Java records encourage.

**(5) Re-add with same score**
`ZADD notifications:user-123 1705314600000 "{...updated json with read:true...}"`

The same score preserves the original position in the chronological order. A new score would change the notification's position in the feed.

**(6) Decrement the count**
`DECR notification_count:user-123`

Keep the unread badge counter consistent. Note: this is not atomic with the remove/add above — there's a small race condition. For a production system, you'd wrap these in a Redis transaction (`MULTI/EXEC`) or a Lua script.

---

## Experiment

### 1. Add a time-range query to ActivityEventRepository

Spring Data supports date range queries in method names:

```java
// Find events for a task within a time window
List<ActivityEvent> findByTaskIdAndTimestampBetweenOrderByTimestampDesc(
    String taskId, Instant from, Instant to);
```

Try calling this and observe the MongoDB query in logs by enabling:
```yaml
logging:
  level:
    org.springframework.data.mongodb.core.MongoTemplate: DEBUG
```

**Trade-off to observe:** `Between` is inclusive on both ends. If you need exclusive ranges, you must use `@Query` annotation with a MongoDB query expression.

### 2. Switch from reverseRange to reverseRangeByScore

The current implementation fetches all notifications and filters in Java. Redis can do this server-side:

```java
// Fetch only notifications in the last 24 hours
Instant cutoff = Instant.now().minus(24, ChronoUnit.HOURS);
double minScore = cutoff.toEpochMilli();
double maxScore = Double.MAX_VALUE;

Set<String> recent = redisTemplate.opsForZSet()
    .reverseRangeByScore("notifications:" + userId, minScore, maxScore);
```

**Trade-off:** `reverseRangeByScore` lets Redis do the filter, reducing network transfer. Use this pattern when you need time-bounded queries at scale.

### 3. Add TTL to user notification sets

Currently, notification sets grow forever. Add expiry:

```java
public void addNotification(String userId, Notification notification) {
    String key = "notifications:" + userId;
    String json = objectMapper.writeValueAsString(notification);
    double score = notification.createdAt().toEpochMilli();
    redisTemplate.opsForZSet().add(key, json, score);
    redisTemplate.expire(key, Duration.ofDays(30));  // reset TTL on each write
    redisTemplate.opsForValue().increment("notification_count:" + userId);
}
```

**Trade-off:** TTL resets on every write (last-write-wins semantics for expiry). A user who receives a notification every day will never have their set expire. A better approach is `ZREMRANGEBYSCORE` to trim old entries instead.

### 4. Trim old notifications instead of TTL

```java
// Keep only the last 100 notifications per user
public void addNotification(String userId, Notification notification) {
    String key = "notifications:" + userId;
    String json = objectMapper.writeValueAsString(notification);
    double score = notification.createdAt().toEpochMilli();
    redisTemplate.opsForZSet().add(key, json, score);
    // Keep only top 100 by score (newest 100)
    redisTemplate.opsForZSet().removeRange(key, 0, -101);
    redisTemplate.opsForValue().increment("notification_count:" + userId);
}
```

`removeRange(key, 0, -101)` removes elements from rank 0 to rank (size - 101), preserving the 100 highest-scored (newest) members.

### 5. Try storing Notification as a Java class instead of a record

Change `Notification` from a `record` to a regular class and remove the no-args constructor. Watch Jackson fail to deserialize from Redis with `InvalidDefinitionException`. Then add the no-args constructor. This reveals the difference between records (which Jackson can handle via constructor components) and regular classes (which need a no-args constructor for Jackson's default deserialization strategy).

---

## Check Your Understanding

1. **Why does `ActivityEvent` use `Map<String, Object>` for `metadata` instead of a strongly typed class?** What would you lose by replacing it with a concrete `TaskCreatedMetadata` class? What would you gain?

2. **Look at the method name `findByTaskIdOrderByTimestampDesc`.** Write out what the equivalent MongoDB shell query would be. Now write the equivalent Go mongo-driver code. Which is more readable? Which gives you more control?

3. **The `markRead` operation scans all of a user's notifications to find one by ID.** What is the time complexity of this operation? At what scale (number of notifications per user) does this become a problem? What data structure change in Redis would fix it?

4. **Why is `StringRedisTemplate` used instead of `RedisTemplate<String, Notification>`?** What would happen if you used `RedisTemplate<String, Object>` with default serialization and then tried to read the value with a different JVM process?

5. **The `ObjectMapper` is constructed directly in `NotificationService`'s constructor instead of being injected as a Spring bean.** What is the downside of this approach? How would you refactor it, and why would that be better in a team setting?

6. **There is a race condition in `markRead`.** Between the `remove` and the `add`, another thread could insert a new notification or read the set. In a single-instance deployment, is this a real problem? What changes if you run multiple instances of the notification-service?

7. **Comments and activity events are stored in separate MongoDB collections rather than nesting comments inside activity events.** Draw out what the document structure would look like if you embedded them. What queries become harder? What queries become easier?

8. **The `JavaTimeModule` is registered with `disable(SerializationFeature.WRITE_DATES_AS_TIMESTAMPS)`.** What does the stored JSON look like with and without this flag? Open redis-cli and run `ZRANGE notifications:some-user 0 -1 WITHSCORES` — does the JSON match what you expected?
