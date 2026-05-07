# RabbitMQ Java Reliability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Harden Java task-event RabbitMQ publishing and consuming with explicit DLQs, bounded retries, idempotency, and observable failure behavior.

**Architecture:** Add Spring AMQP reliability configuration to each Java service that participates in task events. `task-service` publishes persistent mandatory events with confirms/returns. `activity-service` and `notification-service` consume with bounded retry, DLQ routing, listener metrics, and event-level deduplication.

**Tech Stack:** Java 21, Spring Boot, Spring AMQP, Micrometer, MongoDB for activity events, Redis for notifications, Testcontainers RabbitMQ where broker behavior matters.

---

## File Structure

- Modify `java/task-service/src/main/java/dev/kylebradshaw/task/config/RabbitConfig.java`: publisher confirms/returns, persistent converter/template configuration, and exchange declaration.
- Modify `java/task-service/src/main/java/dev/kylebradshaw/task/service/TaskEventPublisher.java`: surface returned/failed publish behavior.
- Modify `java/task-service/src/main/resources/application.yml`: enable correlated publisher confirms, returns, and mandatory publishing.
- Modify `java/activity-service/src/main/java/dev/kylebradshaw/activity/config/RabbitConfig.java`: queue DLX/DLQ declarations, retry interceptor, listener container settings.
- Modify `java/activity-service/src/main/java/dev/kylebradshaw/activity/listener/TaskEventListener.java`: validation, permanent failure classification, metrics, dedupe.
- Modify `java/activity-service/src/main/java/dev/kylebradshaw/activity/document/ActivityEvent.java`: store event id if missing.
- Modify `java/activity-service/src/main/java/dev/kylebradshaw/activity/repository/ActivityEventRepository.java`: add event-id lookup or unique lookup.
- Modify `java/notification-service/src/main/java/dev/kylebradshaw/notification/config/RabbitConfig.java`: queue DLX/DLQ declarations, retry interceptor, listener container settings.
- Modify `java/notification-service/src/main/java/dev/kylebradshaw/notification/listener/TaskEventListener.java`: validation, permanent failure classification, metrics, dedupe.
- Modify `java/notification-service/src/main/java/dev/kylebradshaw/notification/dto/Notification.java`: include source event id if needed.
- Modify `java/notification-service/src/main/java/dev/kylebradshaw/notification/service/NotificationService.java`: deduplicate notification writes by recipient and event id.
- Add or modify unit and integration tests in the three Java services.

## Task 1: Add Event Identifier To Task Events

**Files:**
- Modify: `java/task-service/src/main/java/dev/kylebradshaw/task/dto/TaskEventMessage.java`
- Modify call sites that instantiate `TaskEventMessage`
- Test: existing task service tests that publish events

- [ ] **Step 1: Inspect current record signature**

Run: `sed -n '1,120p' java/task-service/src/main/java/dev/kylebradshaw/task/dto/TaskEventMessage.java`

Expected: confirm existing fields before editing.

- [ ] **Step 2: Add failing test that published events have stable IDs**

Add a unit test around the service method that emits a task event. Assert the captured `TaskEventMessage.eventId()` is non-null.

- [ ] **Step 3: Run test and verify it fails**

Run: `cd java && ./gradlew :task-service:test --tests '*TaskServiceTest*'`

Expected: fail because `eventId()` is missing.

- [ ] **Step 4: Add `UUID eventId` to `TaskEventMessage`**

Update the record with `UUID eventId` as the first field, and update every constructor call to pass `UUID.randomUUID()` for new domain events.

- [ ] **Step 5: Run task-service tests**

Run: `cd java && ./gradlew :task-service:test`

Expected: pass.

- [ ] **Step 6: Commit**

```bash
git add java/task-service
git commit -m "feat: add task event identifiers"
```

## Task 2: Task-Service Publisher Confirms And Returns

**Files:**
- Modify: `java/task-service/src/main/java/dev/kylebradshaw/task/config/RabbitConfig.java`
- Modify: `java/task-service/src/main/java/dev/kylebradshaw/task/service/TaskEventPublisher.java`
- Modify: `java/task-service/src/main/resources/application.yml`
- Test: `java/task-service/src/test/java/dev/kylebradshaw/task/service/TaskEventPublisherTest.java`

- [ ] **Step 1: Write failing publisher configuration test**

Create a test that loads `RabbitTemplate` and asserts mandatory publishing is enabled and messages are persistent.

- [ ] **Step 2: Run test and verify it fails**

Run: `cd java && ./gradlew :task-service:test --tests '*TaskEventPublisherTest*'`

Expected: fail before template callbacks/configuration exist.

- [ ] **Step 3: Configure publisher confirms and returns**

Set in `application.yml`:

```yaml
spring:
  rabbitmq:
    publisher-confirm-type: correlated
    publisher-returns: true
    template:
      mandatory: true
```

Configure `RabbitTemplate` with confirm and returns callbacks that log and increment Micrometer counters for failed confirms and returned messages.

- [ ] **Step 4: Make task events persistent**

Use a message post-processor in `TaskEventPublisher.publish` to set `MessageDeliveryMode.PERSISTENT`.

- [ ] **Step 5: Run task-service tests**

Run: `cd java && ./gradlew :task-service:test`

Expected: pass.

- [ ] **Step 6: Commit**

```bash
git add java/task-service
git commit -m "feat: harden java task event publisher"
```

## Task 3: Activity Consumer DLQ, Retry, And Dedupe

**Files:**
- Modify: `java/activity-service/src/main/java/dev/kylebradshaw/activity/config/RabbitConfig.java`
- Modify: `java/activity-service/src/main/java/dev/kylebradshaw/activity/listener/TaskEventListener.java`
- Modify: `java/activity-service/src/main/java/dev/kylebradshaw/activity/document/ActivityEvent.java`
- Modify: `java/activity-service/src/main/java/dev/kylebradshaw/activity/repository/ActivityEventRepository.java`
- Test: `java/activity-service/src/test/java/dev/kylebradshaw/activity/listener/TaskEventListenerTest.java`

- [ ] **Step 1: Write failing listener tests**

Add tests for missing required identifiers throwing a permanent exception and duplicate event IDs saving only once.

- [ ] **Step 2: Run tests and verify they fail**

Run: `cd java && ./gradlew :activity-service:test --tests '*TaskEventListenerTest*'`

Expected: fail because validation and dedupe do not exist.

- [ ] **Step 3: Declare DLX/DLQ and listener container settings**

Declare:

- `activity.queue`
- `activity.queue.dlq`
- `activity.dlx`

Set queue arguments `x-dead-letter-exchange=activity.dlx`. Configure prefetch `10`, default concurrency `1`, max concurrency `4`, and retry interceptor with 3 attempts and exponential backoff.

- [ ] **Step 4: Add validation and dedupe**

Reject messages with missing `eventId`, `projectId`, `taskId`, `actorId`, or `eventType` using a permanent listener exception. Before saving, check repository by `eventId`; return without saving when already seen.

- [ ] **Step 5: Add metrics**

Inject `MeterRegistry` and increment counters:

- `task_events_consumed_total{service="activity",outcome="success"}`
- `task_events_consumed_total{service="activity",outcome="duplicate"}`
- `task_events_consumed_total{service="activity",outcome="failure"}`

- [ ] **Step 6: Run activity tests**

Run: `cd java && ./gradlew :activity-service:test`

Expected: pass.

- [ ] **Step 7: Commit**

```bash
git add java/activity-service
git commit -m "feat: harden activity rabbitmq consumer"
```

## Task 4: Notification Consumer DLQ, Retry, And Dedupe

**Files:**
- Modify: `java/notification-service/src/main/java/dev/kylebradshaw/notification/config/RabbitConfig.java`
- Modify: `java/notification-service/src/main/java/dev/kylebradshaw/notification/listener/TaskEventListener.java`
- Modify: `java/notification-service/src/main/java/dev/kylebradshaw/notification/dto/Notification.java`
- Modify: `java/notification-service/src/main/java/dev/kylebradshaw/notification/service/NotificationService.java`
- Test: `java/notification-service/src/test/java/dev/kylebradshaw/notification/service/NotificationServiceTest.java`
- Test: create `java/notification-service/src/test/java/dev/kylebradshaw/notification/listener/TaskEventListenerTest.java` if missing

- [ ] **Step 1: Write failing duplicate notification test**

Call the listener twice with the same event id and recipient. Assert only one notification is stored and unread count increments once.

- [ ] **Step 2: Run tests and verify they fail**

Run: `cd java && ./gradlew :notification-service:test`

Expected: fail because dedupe does not exist.

- [ ] **Step 3: Declare DLX/DLQ and listener container settings**

Declare:

- `notification.queue`
- `notification.queue.dlq`
- `notification.dlx`

Set queue arguments `x-dead-letter-exchange=notification.dlx`. Configure prefetch `10`, default concurrency `1`, max concurrency `4`, and retry interceptor with 3 attempts and exponential backoff.

- [ ] **Step 4: Add notification dedupe**

Store a Redis key such as `notification_event:{recipientId}:{eventId}` with `SETNX`. Only create the sorted-set notification and increment unread count when the dedupe key is newly created.

- [ ] **Step 5: Add validation and metrics**

Reject missing `eventId` or `eventType` as permanent failures. Count success, duplicate, ignored, and failure outcomes with Micrometer.

- [ ] **Step 6: Run notification tests**

Run: `cd java && ./gradlew :notification-service:test`

Expected: pass.

- [ ] **Step 7: Commit**

```bash
git add java/notification-service
git commit -m "feat: harden notification rabbitmq consumer"
```

## Task 5: Broker-Backed Java RabbitMQ Integration Tests

**Files:**
- Modify or create integration tests under:
  - `java/activity-service/src/test/java/dev/kylebradshaw/activity/integration/`
  - `java/notification-service/src/test/java/dev/kylebradshaw/notification/integration/`

- [ ] **Step 1: Add activity DLQ integration test**

Use Testcontainers RabbitMQ. Publish a malformed or invalid message to `task.events` with routing key `task.created`. Assert it lands in `activity.queue.dlq` after retry exhaustion.

- [ ] **Step 2: Add notification DLQ integration test**

Use Testcontainers RabbitMQ. Publish an invalid notification event and assert it lands in `notification.queue.dlq` after retry exhaustion.

- [ ] **Step 3: Run integration-focused tests**

Run: `cd java && ./gradlew :activity-service:test :notification-service:test --tests '*Rabbit*IntegrationTest'`

Expected: pass when Docker/Colima is available.

- [ ] **Step 4: Commit**

```bash
git add java/activity-service/src/test java/notification-service/src/test
git commit -m "test: cover java rabbitmq dlq behavior"
```

## Task 6: Java Preflight

**Files:**
- Verify all Java changes.

- [ ] **Step 1: Run service tests**

Run:

```bash
cd java
./gradlew :task-service:test :activity-service:test :notification-service:test
```

Expected: pass.

- [ ] **Step 2: Run Java preflight**

Run: `make preflight-java`

Expected: pass. If blocked by missing JDK 21, report that blocker and leave Java verification to CI.

- [ ] **Step 3: Commit any remaining fixes**

```bash
git status --short
git add <changed java files>
git commit -m "fix: complete java rabbitmq reliability preflight"
```
