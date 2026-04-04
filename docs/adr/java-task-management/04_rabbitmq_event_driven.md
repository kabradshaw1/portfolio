# ADR 04: RabbitMQ and Event-Driven Architecture

## Overview

This document explains why the Java Task Management system uses RabbitMQ for inter-service communication, how AMQP concepts (exchanges, queues, bindings) map to our architecture, and how Spring AMQP turns what would be dozens of lines of channel management code into a handful of annotations and bean declarations.

The task-service publishes events when tasks are created, updated, assigned, or deleted. The activity-service and notification-service consume those events independently. No service calls another service's REST API -- they communicate entirely through messages. This decoupling is the central architectural decision of the system.

---

## Architecture Context

```
[task-service]
  |
  |  publishes to "task.events" topic exchange
  |  routing keys: task.created, task.assigned, task.status_changed, task.deleted
  |
  v
[RabbitMQ]
  |                                  |
  |  "task.*" binding                |  specific key bindings
  v                                  v
[activity.queue]              [notification.queue]
  |                                  |
  v                                  v
[activity-service]            [notification-service]
  saves all events              handles task.created,
  to MongoDB                    task.assigned,
                                task.status_changed
```

The activity-service uses a wildcard binding (`task.*`) because it logs *every* task event. The notification-service binds to specific routing keys because it only cares about events that should generate user notifications -- it intentionally ignores `task.deleted`.

---

## Why Event-Driven?

Consider the alternative: when a task is created, the task-service calls the activity-service's REST API to log the event, then calls the notification-service's REST API to send a notification. This synchronous approach has three problems:

1. **Coupling** -- the task-service must know the URLs, contracts, and health status of every downstream service. Adding a new consumer means modifying the task-service.
2. **Latency** -- the `createTask` response waits for both downstream calls to complete. If the notification-service is slow, task creation is slow.
3. **Failure cascading** -- if the activity-service is down, task creation fails even though the task itself was saved successfully.

Event-driven messaging solves all three. The task-service publishes a message and moves on. Consumers process it independently, at their own pace. Adding a new consumer is a matter of creating a new queue and binding -- the task-service is untouched.

**Go comparison:** In Go microservices, you might use NATS or gRPC streaming for the same pattern. The concept is identical -- the implementation details differ.

---

## Why RabbitMQ?

| Broker | Pros | Cons | Verdict |
|--------|------|------|---------|
| **RabbitMQ** | Mature, flexible routing (topic exchanges), great Spring AMQP support, message acknowledgment, simple ops for small teams | Not designed for massive log streaming | **Chosen** -- right-sized for task events |
| Apache Kafka | Excellent for high-throughput event streaming, replay capability, consumer groups | Overkill for our volume, heavier ops (ZooKeeper/KRaft), more complex consumer model | Too much infrastructure for task CRUD events |
| Redis Pub/Sub | Ultra-simple, already in many stacks | No persistence -- if consumer is down, messages are lost | Unacceptable for task events that must be processed |
| AWS SQS | Managed, no ops burden | Vendor lock-in, no topic exchange routing, would need SNS for fan-out | Adds cloud dependency we do not need |

RabbitMQ hits the sweet spot: it has rich routing (topic exchanges with wildcard bindings), message durability (persistent queues survive broker restarts), and Spring AMQP makes the integration almost declarative.

---

## AMQP Concepts for Go/TS Developers

If you have used NATS or Redis Pub/Sub, AMQP adds one important layer between publisher and consumer: the **exchange**.

### The Three Pieces

```
Producer  -->  Exchange  -->  Binding  -->  Queue  -->  Consumer
```

- **Exchange** -- receives messages from producers. Does NOT store them. Routes them to queues based on bindings.
- **Queue** -- stores messages until a consumer processes them. Durable queues survive broker restarts.
- **Binding** -- a rule that connects an exchange to a queue. Specifies which messages the queue should receive (via routing key patterns).

### Exchange Types

| Type | Routing Logic | Example |
|------|--------------|---------|
| **Direct** | Exact routing key match | `task.created` goes only to queues bound with `task.created` |
| **Fanout** | Broadcasts to all bound queues (ignores routing key) | All consumers get everything |
| **Topic** | Pattern matching with `*` (one word) and `#` (zero or more words) | `task.*` matches `task.created`, `task.deleted`, etc. |
| **Headers** | Match on message headers instead of routing key | Rarely used |

We use a **topic exchange** because it gives us the flexibility to:
- Bind a queue with `task.*` to receive ALL task events (activity-service).
- Bind a queue with specific keys like `task.created` and `task.assigned` to receive only those events (notification-service).
- Add new event types (e.g., `task.comment_added`) without changing existing bindings -- the activity-service picks them up automatically via the wildcard.

---

## Go/TS Comparison

| Concept | Go (streadway/amqp) | TypeScript (amqplib) | Java (Spring AMQP) |
|---------|---------------------|---------------------|---------------------|
| Connection | `amqp.Dial(url)` + manual error handling | `amqplib.connect(url)` + callback/promise | Auto-configured from `spring.rabbitmq.*` properties |
| Channel | `conn.Channel()` -- must manage lifecycle | `conn.createChannel()` -- must close manually | Hidden behind `RabbitTemplate` |
| Declare exchange | `ch.ExchangeDeclare("task.events", "topic", true, ...)` | `ch.assertExchange("task.events", "topic", {durable: true})` | `@Bean TopicExchange` -- Spring declares it on startup |
| Declare queue | `ch.QueueDeclare("activity.queue", true, ...)` | `ch.assertQueue("activity.queue", {durable: true})` | `@Bean Queue` -- Spring declares it on startup |
| Bind | `ch.QueueBind("activity.queue", "task.*", "task.events")` | `ch.bindQueue("activity.queue", "task.events", "task.*")` | `@Bean Binding` using `BindingBuilder` fluent API |
| Publish | `ch.Publish("task.events", "task.created", false, false, msg)` | `ch.publish("task.events", "task.created", Buffer.from(json))` | `rabbitTemplate.convertAndSend(exchange, routingKey, object)` |
| Consume | `ch.Consume(queue, "", ...)` returns `<-chan Delivery` | `ch.consume(queue, (msg) => { ... })` | `@RabbitListener(queues = "...")` on a method |
| Serialization | Manual `json.Marshal` / `json.Unmarshal` | Manual `JSON.stringify` / `JSON.parse` | `Jackson2JsonMessageConverter` bean -- automatic |
| Reconnection | Manual (or use a wrapper like `rabbitmq/amqp091-go`) | Manual (or use `amqp-connection-manager`) | Built into Spring AMQP with retry policies |

**The key takeaway:** In Go and Node.js, you manage connections, channels, declarations, serialization, and reconnection yourself. Spring AMQP handles all of that. You declare beans for the topology (exchange, queue, binding) and annotate a method with `@RabbitListener` -- Spring does the rest.

---

## Build It

### Step 1: The Message Contract -- TaskEventMessage

Every event flowing through RabbitMQ uses the same record type:

```java
// TaskEventMessage.java
public record TaskEventMessage(
        UUID eventId,
        String eventType,
        Instant timestamp,
        UUID actorId,
        UUID projectId,
        UUID taskId,
        Map<String, Object> data) {

    public static TaskEventMessage of(String eventType, UUID actorId,
                                       UUID projectId, UUID taskId,
                                       Map<String, Object> data) {
        return new TaskEventMessage(
                UUID.randomUUID(), eventType, Instant.now(),
                actorId, projectId, taskId, data);
    }
}
```

**Design decisions:**

- **Java `record`** -- immutable, auto-generates `equals()`, `hashCode()`, `toString()`. Perfect for DTOs. In Go, this would be a struct; in TypeScript, a `readonly` interface.
- **`eventId` as UUID** -- uniquely identifies each event for idempotency. If a consumer receives the same event twice (network retry), it can deduplicate by eventId.
- **`data` as `Map<String, Object>`** -- a flexible bag for event-specific details. `TASK_CREATED` includes `task_title`; `TASK_ASSIGNED` includes `assignee_id` and `task_title`. This avoids creating a separate message type for every event.
- **`of()` factory method** -- auto-generates `eventId` and `timestamp`, so callers only provide the business-relevant fields.

### Step 2: Producer-Side RabbitConfig (task-service)

```java
// RabbitConfig.java (task-service)
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

Two beans, and that is the entire producer-side configuration:

1. **`TopicExchange`** -- Spring AMQP will declare this exchange on RabbitMQ when the application starts. If it already exists, the declaration is a no-op. The name `task.events` is shared across all services via the constant.

2. **`Jackson2JsonMessageConverter`** -- tells Spring AMQP to serialize outgoing messages as JSON (using Jackson) instead of Java serialization. This is critical because consumers may be written in different languages. It also means `TaskEventMessage` is serialized using the same Jackson that handles your REST API DTOs.

**Go comparison:** In Go, you would call `ch.ExchangeDeclare("task.events", "topic", true, false, false, false, nil)` -- seven positional boolean parameters. The Spring bean approach is more readable.

### Step 3: TaskEventPublisher -- The Single Publish Point

```java
// TaskEventPublisher.java
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

`RabbitTemplate` is Spring AMQP's high-level abstraction over AMQP channels. It handles:
- Connection pooling (channels are expensive to create).
- Serialization via the configured `MessageConverter`.
- Retry on connection failure.

The `convertAndSend` call does three things in one line:
1. Serialize `message` to JSON using Jackson.
2. Create an AMQP message with the JSON body and content-type header.
3. Publish to the `task.events` exchange with the given routing key.

**Why wrap `RabbitTemplate` in `TaskEventPublisher` instead of injecting it directly into services?** Encapsulation. The exchange name is defined in one place. If we ever need to add common message headers, retry logic, or logging, there is a single place to do it.

### Step 4: Publishing Events from TaskService

Here is how the task-service publishes events during business operations:

```java
// TaskService.java (excerpts)
@Transactional
public Task createTask(CreateTaskRequest request, UUID actorId) {
    var project = projectRepo.findById(request.projectId())
            .orElseThrow(() -> new IllegalArgumentException("Project not found"));
    var task = new Task(project, request.title(), request.description(),
                        priority, request.dueDate());
    task = taskRepo.save(task);

    eventPublisher.publish("task.created", TaskEventMessage.of(
            "TASK_CREATED", actorId, project.getId(), task.getId(),
            Map.of("task_title", task.getTitle())));

    return task;
}
```

Notice the pattern: **save to database first, then publish**. This ordering matters because:
- If the database save fails, no event is published (correct -- nothing happened).
- If the publish fails after the save, the task exists but the event is lost. This is the classic "dual write" problem. For a task management app, this is acceptable -- a missed activity log entry is not catastrophic. For financial systems, you would use the transactional outbox pattern.

The routing keys follow a consistent convention:
| Operation | Routing Key | Event Type |
|-----------|-------------|------------|
| Create | `task.created` | `TASK_CREATED` |
| Status change | `task.status_changed` | `STATUS_CHANGED` |
| Assign | `task.assigned` | `TASK_ASSIGNED` |
| Delete | `task.deleted` | `TASK_DELETED` |

Each has a `task.` prefix, which is what makes the topic exchange wildcard binding `task.*` work.

### Step 5: Consumer-Side RabbitConfig (activity-service)

```java
// RabbitConfig.java (activity-service)
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
        return new Queue(QUEUE_NAME, true);  // true = durable
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

The consumer-side config declares more beans than the producer because it needs to set up the **receiving infrastructure**:

1. **`TopicExchange`** -- re-declared here. Both producer and consumer declare the exchange. RabbitMQ treats identical declarations as idempotent, so it does not matter which service starts first.

2. **`Queue("activity.queue", true)`** -- creates a durable queue. `true` means the queue survives RabbitMQ restarts. Messages in a durable queue are also persisted to disk if they are marked as persistent (Spring AMQP does this by default).

3. **`Binding`** -- the critical piece. `BindingBuilder.bind(activityQueue).to(taskExchange).with("task.*")` creates a rule: route any message published to `task.events` with a routing key matching `task.*` into `activity.queue`. The `*` matches exactly one dot-delimited word, so `task.created`, `task.deleted`, and `task.assigned` all match, but `task.comment.added` would NOT match (that would need `task.#`).

### Step 6: Consumer-Side RabbitConfig (notification-service)

```java
// RabbitConfig.java (notification-service)
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
    public Binding bindingTaskCreated(Queue notificationQueue, TopicExchange taskExchange) {
        return BindingBuilder.bind(notificationQueue).to(taskExchange).with("task.created");
    }

    @Bean
    public Binding bindingTaskAssigned(Queue notificationQueue, TopicExchange taskExchange) {
        return BindingBuilder.bind(notificationQueue).to(taskExchange).with("task.assigned");
    }

    @Bean
    public Binding bindingStatusChanged(Queue notificationQueue, TopicExchange taskExchange) {
        return BindingBuilder.bind(notificationQueue).to(taskExchange).with("task.status_changed");
    }

    @Bean
    public Binding bindingCommentAdded(Queue notificationQueue, TopicExchange taskExchange) {
        return BindingBuilder.bind(notificationQueue).to(taskExchange).with("task.comment_added");
    }

    @Bean
    public MessageConverter jsonMessageConverter() {
        return new Jackson2JsonMessageConverter();
    }
}
```

**Why specific bindings instead of `task.*`?** The notification-service intentionally does NOT handle `task.deleted` -- deleting a task should not generate a user notification. By listing specific routing keys, the queue only receives the events the service cares about. Messages with routing key `task.deleted` are routed to `activity.queue` (via `task.*`) but not to `notification.queue`.

This is the power of the topic exchange: **consumers declare their own interest level**, and the producer does not need to know or care which consumers exist.

### Step 7: The Activity-Service Listener

```java
// TaskEventListener.java (activity-service)
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
                message.data());
        activityRepo.save(event);
    }
}
```

The entire consumer is one method with `@RabbitListener`. Spring AMQP handles:
- Connecting to RabbitMQ.
- Subscribing to `activity.queue`.
- Deserializing the JSON message body into a `TaskEventMessage` (using Jackson).
- Calling `handleTaskEvent()` with the deserialized object.
- Acknowledging the message after the method returns successfully.
- Requeuing the message if the method throws an exception.

**Go comparison:** In Go with `streadway/amqp`, this would be roughly 30 lines: open channel, set QoS, consume, loop over deliveries, unmarshal JSON, process, ack. The `@RabbitListener` annotation replaces all of that.

The activity-service is a simple audit log -- it converts every task event into an `ActivityEvent` document and saves it to MongoDB. No filtering, no business logic. The wildcard binding ensures it captures everything.

### Step 8: The Notification-Service Listener

```java
// TaskEventListener.java (notification-service)
@Component
public class TaskEventListener {

    private final NotificationService notificationService;

    @RabbitListener(queues = RabbitConfig.QUEUE_NAME)
    public void handleTaskEvent(TaskEventMessage message) {
        String taskId = message.taskId() != null ? message.taskId().toString() : null;
        String actorId = message.actorId() != null ? message.actorId().toString() : null;

        switch (message.eventType()) {
            case "TASK_ASSIGNED" -> {
                String assigneeId = extractString(message, "assigneeId");
                if (assigneeId != null) {
                    String taskTitle = extractString(message, "taskTitle");
                    String msg = taskTitle != null
                            ? "You were assigned to task: " + taskTitle
                            : "You were assigned a task";
                    notificationService.addNotification(
                            assigneeId, Notification.create("TASK_ASSIGNED", msg, taskId));
                }
            }
            case "STATUS_CHANGED" -> { /* similar pattern */ }
            case "TASK_CREATED" -> { /* similar pattern */ }
            default -> { /* unhandled event types are intentionally ignored */ }
        }
    }
}
```

Unlike the activity-service, the notification-service has **business logic in the listener**. It switches on event type and constructs user-facing notification messages. Each event type maps to a different notification template.

The `default` case ignores unrecognized events. This is important for forward compatibility -- if the task-service starts publishing `task.comment_added` events and the notification-service has not been updated yet, the messages are silently consumed and discarded rather than causing errors or filling a dead letter queue.

**Design note:** The listener extracts data from the `Map<String, Object>` using a helper method. This is a trade-off of the flexible `data` map approach -- you lose compile-time type safety. In a more complex system, you might define specific event payload records for each event type.

---

## How Messages Flow End-to-End

Let's trace a `createTask` call through the entire system:

1. **User** calls `POST /api/tasks` with a JWT.
2. **Gateway** validates JWT, forwards request with `X-User-Id` to task-service.
3. **TaskService.createTask()** saves the task to PostgreSQL.
4. **TaskEventPublisher.publish()** sends a `TaskEventMessage` to the `task.events` exchange with routing key `task.created`.
5. **RabbitMQ** evaluates bindings:
   - `activity.queue` is bound with `task.*` -- matches `task.created` -- message is routed.
   - `notification.queue` is bound with `task.created` -- exact match -- message is routed.
6. **Activity-service** listener picks up the message, saves an ActivityEvent to MongoDB.
7. **Notification-service** listener picks up the message, creates a notification for the user.

Steps 6 and 7 happen concurrently and independently. If the notification-service is down, messages accumulate in `notification.queue` and are processed when it comes back up. The task-service and activity-service are unaffected.

---

## Experiment

Try these changes to deepen your understanding:

1. **Add a new event type.** Publish `task.comment_added` with routing key `task.comment_added` from the task-service. Verify that the activity-service receives it (via the `task.*` wildcard) but the notification-service also receives it (it already has a binding for `task.comment_added`). Check the notification-service listener's `default` case -- what happens?

2. **Change the activity-service binding from `task.*` to `task.created`.** Restart and create a task, update its status, and delete it. Verify the activity-service only logs the creation -- the other events are dropped at the exchange level, never reaching the queue.

3. **Explore the RabbitMQ Management UI** at `http://localhost:15672` (guest/guest). Navigate to Queues and observe:
   - Message rates (messages/sec published and consumed).
   - Queue depth (messages waiting to be consumed).
   - Consumer count per queue.

4. **Stop the notification-service** and create several tasks. Watch the `notification.queue` depth grow in the management UI. Restart the service and watch it drain the queue.

5. **Test message durability.** Stop RabbitMQ with `docker compose stop rabbitmq`, then restart it. Check if your queues and unprocessed messages survived the restart (they should, because both queues and messages are durable).

6. **Try a fanout exchange.** Change `TopicExchange` to `FanoutExchange` in all three services. Remove the routing key from bindings. Publish a message. Both consumers receive it regardless of routing key. Then switch back -- and appreciate why topic exchanges give better control.

---

## Check Your Understanding

1. If the task-service publishes a message with routing key `task.subtask.created`, would the activity-service receive it with its `task.*` binding? What about `task.#`? Explain the difference between `*` and `#` in AMQP topic routing.

2. Both the producer and consumer declare the same `TopicExchange`. What happens if they declare it with different configurations (e.g., one durable and one non-durable)?

3. The task-service saves to the database first, then publishes the event. What happens if the publish fails? How would you solve this with the transactional outbox pattern?

4. In Go, you manage AMQP channels explicitly and must handle reconnection yourself. What does Spring AMQP do when the RabbitMQ connection drops mid-operation? How does `@RabbitListener` handle reconnection?

5. The notification-service uses specific routing key bindings while the activity-service uses a wildcard. What would happen if both used wildcards? Would the system still be correct? What would change?

6. Why do we use `Jackson2JsonMessageConverter` instead of Java's default serialization? Think about what would happen if the activity-service (which might be rewritten in Go or Node.js) needed to consume these messages.
