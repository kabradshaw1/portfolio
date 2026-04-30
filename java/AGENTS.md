# Java Services

Spring Boot microservices live under `java/`:

- `task-service` - task/project CRUD, PostgreSQL, JPA
- `activity-service` - activity feed, MongoDB, Redis caching, analytics aggregation
- `notification-service` - event-driven notifications, RabbitMQ consumer
- `gateway-service` - GraphQL gateway
- `k8s/` - Java-specific Kubernetes manifests

## Schema Ownership

Java services own schema through Spring/JPA at startup. Do not add a separate
migration framework unless the service design changes explicitly.

## Resource Limits

Java services use `-Xmx512m` heap cap with 768Mi container memory limits. New
Java service Dockerfiles must include the heap cap in `ENTRYPOINT`; otherwise
JVM auto-sizing can cause OOM kills.

## Deployment Notes

Shared-infra services must exist in their production namespace before QA can
ExternalName-route to them. The QA deploy job does not apply prod-namespace
manifests; pointing QA at a missing prod Service causes DNS-resolution failures.

## Verification

`make preflight-java` is the expected Java check. If local execution is blocked
by missing JDK 21, report the blocker and leave Java verification to CI. Do not
run Java tests on Debian as a workaround unless Kyle explicitly authorizes that
specific exception.
