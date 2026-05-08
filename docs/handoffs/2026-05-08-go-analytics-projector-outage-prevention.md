# Go Analytics / Projector Outage Prevention Handoff

## TL;DR

On May 7, 2026, creating orders in `/go/ecommerce` did not produce visible data
in `/go/analytics`, and the observability dashboard showed projector failures.
Two independent regressions were found and fixed on `qa` in commit
`e7a66b7 Fix Go analytics projection regressions`.

Root causes found so far:

- `analytics-service` consumed Kafka events but queried Redis with the wrong
  window-key format. Redis contained keys like `2026-05-08T01:22:00Z`; reads
  looked for keys like `2026-05-08T01`.
- `order-projector` expected `order_id` in the Kafka event body, while
  `order-service` publishes the order ID as the Kafka message key. The
  projector repeatedly retried those events, tripped the `projector-postgres`
  circuit breaker, and accumulated lag.

This handoff is for the next investigation: determine the exact blast radius,
confirm alert delivery behavior, and add prevention coverage so this class of
bug cannot silently pass again.

## Current Status

- Fix pushed to remote `qa`: `e7a66b7`.
- `make preflight-go` passed from a clean worktree based on `origin/qa`.
- Grafana/Prometheus currently showed no active Prometheus alerts at the time of
  investigation after the fix path was started.
- Existing untracked docs were left untouched:
  `docs/handoffs/` and
  `docs/superpowers/specs/2026-05-07-rabbitmq-broker-metrics-design.md`.

## Timeline Evidence

Git introduction dates:

- `analytics-service` Redis store regression likely dates to
  `9ac46a3 feat(analytics): add Redis store layer for windowed aggregation results`
  on April 22, 2026. That code used `windowKeyLayout = "2006-01-02T15"` while
  the window package flushed RFC3339 keys.
- `order-projector` event contract mismatch likely dates to
  `e284b55 feat(order-projector): add Kafka consumer with projection pipeline`
  on April 23, 2026. It introduced `OrderID string json:"order_id"`.
- `order-service` Kafka producer has published the Kafka message key separately
  since the original producer code on April 17, 2026, and did not put the order
  ID into the JSON envelope.

Runtime evidence observed on May 7, 2026:

- `order-service` completed at least one order end-to-end:
  `26672c8d-c32e-4f1c-afca-c59abe2d414c`.
- `analytics-group` had consumed through the end of `ecommerce.orders` and
  `ecommerce.cart` with lag `0`.
- `go-analytics-service /health` returned `{"kafka":"connected","status":"ok"}`,
  but `/analytics/revenue?hours=24` returned `{"stale":false,"windows":[]}`.
- Redis contained analytics keys such as:
  `analytics:trending:2026-05-08T01:22:00Z`.
- `order-projector-group` was observed with lag `13` on
  `ecommerce.order-events`.
- Kafka messages around the stuck offset had no top-level `order_id`; the order
  ID was in the Kafka message key.

Alert evidence:

- Grafana alerting config routes alerts to Telegram chat `8710936679`.
- Relevant existing rules:
  `circuit-breaker-open`, `circuit-breaker-flapping`, and
  `kafka-consumer-lag-high`.
- Grafana logs showed repeated local notifier sends for
  `rule_uid=circuit-breaker-open` starting around `2026-05-08T01:16:35Z`
  (May 7, 2026 19:16:35 America/Guatemala).
- Grafana logs did not prove Telegram delivery success in the sampled output;
  they proved Grafana attempted local notification routing.
- Prometheus `/api/v1/alerts` returned no active alerts during the later check.

Important uncertainty:

- The exact first user-visible failure is not proven from logs yet. Based on
  git history, analytics may have been broken since April 22, 2026 when Redis
  backed reads were introduced, but visible impact only occurs after orders are
  created and the analytics page is checked. Projector breakage may have existed
  since April 23, 2026 or since the first order event processed by the deployed
  projector.

## Root Cause Analysis

### Analytics Empty Data

The pipeline was misleadingly healthy:

- Kafka consumer connected.
- Kafka lag was zero.
- Flush metrics existed.
- Health endpoint was OK.

But the read model was empty because the store wrote and read incompatible
keys. This is a data-contract bug between `window` and `store`, not a Kafka
transport bug.

Why tests missed it:

- Store tests used `time.Now().UTC().Format(windowKeyLayout)` instead of the
  actual key emitted by the window package.
- Handler tests used mock stores and did not cover a real event -> flush -> read
  round trip.
- Smoke tests only checked service health or made analytics verification
  non-blocking.

### Projector / Observability Failure

The projector treated missing `order_id` as a valid empty string until it hit
Postgres UUID insertion. It retried processing instead of classifying the event
as a schema/contract problem. Repeated projection failures opened/flapped the
Postgres circuit breaker, so the observability dashboard correctly showed
dependency/projector failures even though Kubernetes pods were healthy.

Why tests missed it:

- Projector tests used JSON fixtures with `order_id` present.
- There was no cross-service contract test using an actual
  `order-service/internal/kafka.Event` plus Kafka key.
- There was no smoke test that creates an order and asserts the projector
  timeline/stats update.

## Prevention Work To Investigate

### 1. Add Contract Tests

Add a Go-level contract test that serializes an order-service event the same way
the producer does, including the Kafka message key, and feeds it into
order-projector deserialization/processing.

Acceptance criteria:

- `order-projector` can process current `order-service` events without
  requiring top-level `order_id`.
- Missing order identity with no Kafka key is treated as a terminal schema
  error and sent to DLQ, not retried indefinitely.
- The contract test fails if either service changes event shape incompatibly.

### 2. Add Analytics End-To-End Integration Coverage

Add a focused integration test for:

Kafka order/cart events -> analytics consumer -> flush -> Redis/store read APIs.

Acceptance criteria:

- Use the same window key emitted by the window package.
- Verify `/analytics/revenue`, `/analytics/trending`, and
  `/analytics/cart-abandonment` return data after events are consumed.
- Include one test where the current window is still open, so the dashboard does
  not wait an hour to show revenue.

### 3. Make Smoke Tests Blocking For Read Models

The existing prod smoke has analytics verification as non-blocking. That allowed
the portfolio page to stay green while the user-facing analytics page was empty.

Recommended smoke checks:

- Create/add cart item.
- Create and complete an order or publish deterministic test events to Kafka in
  an isolated QA topic/namespace.
- Assert analytics revenue has at least one window within a bounded timeout.
- Assert order-projector `/stats/orders?hours=24` increments.
- Assert order-projector `/orders/:id/timeline` has timeline events for the new
  order.

Keep prod smoke non-destructive unless there is already a sanctioned test user
and product fixture. QA smoke should be stricter and blocking.

### 4. Add Synthetic Readiness For Derived Data

Service health currently reports dependency connectivity, not useful read-model
freshness. Add a synthetic or admin endpoint/metric that reports:

- last consumed Kafka event timestamp,
- last successful persisted analytics window timestamp,
- last non-empty analytics read timestamp,
- projector source lag,
- projector last successful projection timestamp,
- DLQ count for analytics/projector topics.

Alert on stale derived data, not only process health.

### 5. Improve Alert Delivery Verification

Grafana logs showed notification routing for `circuit-breaker-open`, but the
investigation did not confirm Telegram delivery.

Add or document:

- a periodic alert delivery canary,
- a runbook command to confirm last Telegram notification delivery,
- Grafana alert history query that works without relying on broad Loki scans,
- a dashboard panel for current notification failures.

Also fix or review `pg-auto-explain-stalled`: Grafana logs show it repeatedly
failed evaluation because the query range exceeded Loki's 1-day limit. This is
unrelated noise that can hide real alerts.

### 6. Tighten Alert Rules

Current `kafka-consumer-lag-high` fires above `1000`, which did not catch the
projector outage at lag `13`. For low-volume portfolio services, absolute lag
thresholds should be much lower or paired with age-based lag.

Consider:

- projector lag > 0 for 10 minutes,
- projector latest event age > 5 minutes while topic log end advances,
- projection error rate > 0 for 5 minutes,
- circuit breaker changes > 3 in 5 minutes,
- analytics consumed events increasing while read windows stay empty.

### 7. DLQ Poison Handling

Projector processing errors currently retry and block the partition. For
contract/schema errors after successful deserialization but failed validation,
classify as terminal and DLQ. Do not let a poison event hold all later events
hostage.

Follow the `go-kafka-consumer-reliability` skill:

- bounded retries,
- DLQ quarantine for terminal failures,
- commit only after durable success or DLQ publish,
- metrics for retry exhaustion and DLQ path.

## Questions For Next Agent

- Was the analytics page ever showing real Redis-backed data after April 22,
  2026, or only mock/in-memory/local data?
- Did Telegram actually receive `circuit-breaker-open` notifications around
  `2026-05-08T01:16:35Z`, and if not, was it token/chat/config/routing?
- Should `order-service` start embedding `order_id` in the event envelope for
  future events, or is the Kafka key the canonical contract?
- Should projector projection errors be treated as DLQ-able poison messages
  after retry exhaustion?
- Which smoke suite should own blocking read-model assertions: CI compose, QA
  deploy smoke, prod smoke, or all three with different strictness?

## Useful Commands

Read-only observability checks used during investigation:

```bash
scripts/loki-query --ns go-ecommerce --app go-order-projector --hours 4 --limit 80
scripts/loki-query --ns go-ecommerce --app go-analytics-service --hours 4 --limit 50

ssh debian 'kubectl exec -n go-ecommerce kafka-0 -- /opt/kafka/bin/kafka-consumer-groups.sh --bootstrap-server localhost:9092 --describe --group analytics-group'
ssh debian 'kubectl exec -n go-ecommerce kafka-0 -- /opt/kafka/bin/kafka-consumer-groups.sh --bootstrap-server localhost:9092 --describe --group order-projector-group'

ssh debian 'kubectl exec -n go-ecommerce deploy/go-analytics-service -- wget -qO- "http://localhost:8094/analytics/revenue?hours=24"'
ssh debian 'kubectl exec -n go-ecommerce deploy/go-order-projector -- wget -qO- http://localhost:8097/health'

ssh debian "kubectl logs -n monitoring deploy/grafana --since=24h | grep -Ei 'telegram|notifier|alert|circuit|notification'"
```

Code references:

- `go/analytics-service/internal/store/redis.go`
- `go/analytics-service/internal/window/tumbling.go`
- `go/order-projector/internal/consumer/consumer.go`
- `go/order-service/internal/kafka/producer.go`
- `k8s/monitoring/configmaps/grafana-alerting.yml`

