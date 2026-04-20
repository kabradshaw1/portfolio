# Alerting Philosophy and SLOs

## Symptom-Based vs Cause-Based Alerting

Cause-based alerting fires on underlying conditions: "disk is 80% full," "CPU is above 90%." These alerts are easy to write but generate noise -- high CPU during a batch job is fine, and each false positive erodes trust.

Symptom-based alerting fires on user-visible impact: "error rate exceeds 5%," "p95 latency is above 2 seconds." These correlate directly with degraded experience. The practical approach combines both: symptom-based for paging, cause-based for dashboard visibility.

## The RED Method

For request-driven services, the RED method provides a standard framework: Rate (requests per second), Errors (failed requests as a percentage), and Duration (latency distribution at p50, p95, p99). These map directly to user experience. The Go services expose all three through `http_requests_total` (counter, split by status code) and `http_request_duration_seconds_bucket` (histogram).

## SLIs, SLOs, and SLAs

An **SLI** (Service Level Indicator) is the measurement: "percentage of requests returning non-5xx." An **SLO** (Service Level Objective) is the target: "99% of requests should succeed." An **SLA** (Service Level Agreement) is a contractual commitment with consequences for breach.

This project's SLOs reflect each service's nature. The AI service targets 5% error rate because LLM calls are inherently unreliable (Ollama timeouts, model loading delays). The ecommerce and Java gateway services target 2% as transaction-oriented services where reliability matters more. Latency targets follow similar logic: 30 seconds at p95 for AI (inference is slow), 2 seconds for ecommerce (CRUD should be fast).

## Alert Fatigue

The `for` field specifies how long a condition must persist before firing. GPU temperature uses `for: 5m` (brief spikes during model loading are normal), OOM kill uses `for: 0s` (always significant), and pod restart storm uses `for: 5m` (distinguishes rolling updates from crash loops).

This project uses `severity: critical` for immediate-attention conditions (OOM kills, node pressure) and `severity: warning` for non-emergencies (elevated error rates, Kafka lag).

## Alert Hierarchy

The alert groups in this project are ordered by scope: Infrastructure alerts (node pressure, disk pressure) indicate environment-level problems affecting all services. Kubernetes Health alerts (OOM kills, restart storms, unavailable replicas) indicate platform-level issues. Application SLO alerts (error rates, latency) indicate service-level degradation. Streaming Analytics alerts (Kafka consumer lag) indicate data pipeline health. This ordering provides a natural triage path: if infrastructure alerts fire alongside application alerts, fix the infrastructure first.
