# Fix: Kafka KRaft OOMKilled at 512Mi

- **Date:** 2026-04-22
- **Status:** Accepted

## Context

Kafka (Apache Kafka 3.7.0, KRaft mode) was in a CrashLoopBackOff in both QA (640 restarts) and prod (885 restarts). The container was being OOMKilled — `kubectl describe` showed `Reason: OOMKilled, Exit Code: 137`.

Kafka was configured with a 512Mi memory limit and no explicit JVM heap cap. In KRaft mode, Kafka runs both broker and controller in a single JVM process, which requires more memory than broker-only mode. The JVM's default ergonomic heap sizing (`MaxRAMPercentage=25%` of container limit = ~128Mi) left insufficient headroom for KRaft metadata, page cache, and direct memory buffers. Under load — particularly when the analytics consumer group rebalanced after each crash — memory spiked past the 512Mi limit.

The "Pod Restart Storm" alert was firing for Kafka but couldn't be delivered because the Telegram bot token was invalid (separate issue). This masked the problem — Kyle wasn't receiving any alerts despite active firing.

## Decision

### Memory limit: 512Mi → 1Gi

Doubled the container memory limit to 1Gi. The minikube node has 32Gi allocatable with only ~9.3Gi in use, so the additional 512Mi per Kafka pod (x2 environments) is well within headroom.

Considered 768Mi but chose 1Gi for margin. KRaft's metadata store grows with topic/partition count, and the analytics pipeline may add topics. 1Gi provides room for growth without revisiting this decision.

### Explicit heap cap: `KAFKA_HEAP_OPTS=-Xmx512m -Xms512m`

Added an explicit JVM heap cap rather than relying on container-aware ergonomics. With a 1Gi container limit and 512Mi heap, there's ~512Mi remaining for OS page cache (important for Kafka's zero-copy sendfile performance), JVM metaspace, direct buffers, and KRaft Raft log memory-mapped files.

The `-Xms512m` matches `-Xmx` to avoid heap resizing pauses during broker startup, which is when Kafka is most memory-hungry (loading topic metadata, replaying the Raft log).

### Applied immediately via `kubectl set resources` + `kubectl set env`

Rather than waiting for CI to deploy the updated manifest, applied the fix directly to both namespaces. The committed manifest ensures future deploys maintain the correct values. The imperative changes will be overwritten by the next Kustomize apply, which uses the same values.

## Consequences

- **Positive:** Kafka is stable in both environments with 0 restarts since the fix. QA Kafka at 308Mi, prod at 491Mi — both well within the 1Gi limit.
- **Positive:** The analytics consumer group (`analytics-group`) stops cycling through rebalances caused by Kafka crashes, which was wasting consumer offset generations (reached 604+).
- **Positive:** Saga event consumers (order-service) get a stable RabbitMQ-adjacent Kafka broker, reducing a source of transient failures in the checkout pipeline.
- **Trade-off:** 1Gi per Kafka pod (x2 namespaces) uses 2Gi of the node's 32Gi. Acceptable for a single-node portfolio cluster. Would need revisiting if minikube's 16Gi allocation becomes a constraint.
- **Lesson:** JVM-based stateful services in Kubernetes should always have explicit heap caps. Relying on container-aware ergonomics is fragile — the defaults don't account for the specific memory needs of the application (KRaft metadata, page cache, etc.).
