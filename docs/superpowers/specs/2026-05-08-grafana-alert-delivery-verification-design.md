# Grafana Alert Delivery Verification Design

Date: 2026-05-08
Status: Approved for implementation planning

## TL;DR

Add production-grade proof that Grafana alerts reach Telegram. Provision a
low-noise alert delivery canary, add dashboard visibility for notification
failures, create a narrow read-only verification command, and fix the known
`pg-auto-explain-stalled` Loki range problem so alerting noise does not hide
real delivery failures.

## Background

During the Go analytics and order-projector outage investigation on May 7,
2026, Grafana logs showed local notification routing for
`circuit-breaker-open`, but the sampled evidence did not prove the Telegram
message was delivered. The alerting path was therefore only partially verified:
Grafana evaluated a rule and attempted notification routing, but the final
Grafana-to-Telegram delivery step was not confirmed.

The same investigation also found repeated evaluation failures for
`pg-auto-explain-stalled`. Its Loki query uses a 24-hour range, and Grafana logs
reported failures because the query range exceeded Loki's 1-day limit. That
alert is unrelated to the projector outage, but noisy evaluation failures make
real alerting problems harder to notice.

## Goals

- Prove the real Grafana evaluator -> notification policy -> Telegram contact
  point path on a predictable cadence.
- Make Grafana notification failures and alert-rule evaluation failures visible
  from dashboards and a narrow command.
- Document a repeatable read-only command for confirming recent alert delivery
  behavior without broad Loki scans.
- Fix `pg-auto-explain-stalled` so it no longer creates recurring Loki range
  errors.
- Keep all changes repo-managed through provisioned Grafana/Kubernetes config.

## Non-Goals

- Do not add a second notification provider.
- Do not build a custom alerting or Telegram sender service.
- Do not imperatively edit live Grafana alert rules, contact points, Kubernetes
  Secrets, or ConfigMaps.
- Do not solve every Go analytics/projector prevention item from the handoff.
  Contract tests, stricter read-model smokes, derived-data readiness, tighter
  Kafka lag alerts, and DLQ handling remain separate phases.

## Current State

Grafana alerting is provisioned from
`k8s/monitoring/configmaps/grafana-alerting.yml`. The existing contact point is
named `telegram` and uses `TELEGRAM_BOT_TOKEN` from the `telegram-bot` Secret
plus chat ID `8710936679`. Notification policies route all alert folders to
that receiver.

Dashboards are source-controlled under `monitoring/grafana/dashboards/*.json`
and synced into `k8s/monitoring/configmaps/grafana-dashboards.yml` with
`make grafana-sync`. CI/preflight checks use `make grafana-sync-check` to catch
dashboard drift.

`scripts/loki-query` already protects ad hoc Loki queries from windows above
24 hours and trims the exact 24-hour path slightly below the limit. The
provisioned `pg-auto-explain-stalled` alert does not yet have that protection.

## Design

### 1. Alert Delivery Canary

Add a Grafana-managed rule named `alert-delivery-canary` to
`k8s/monitoring/configmaps/grafana-alerting.yml`.

The rule should:

- Use the Prometheus datasource UID `PBFA97CFB590B2093`.
- Fire once daily from 15:00-15:05 UTC, which is 09:00-09:05
  America/Guatemala.
- Route through the existing `telegram` contact point by using the existing
  notification policy.
- Resolve automatically after the canary window closes.
- Carry bounded labels such as:
  - `severity: info`
  - `purpose: alert-delivery-canary`
- Include annotations that make the alert obviously intentional, for example:
  "Daily alert delivery canary. No operator action required."

The canary should exercise Grafana alert evaluation and notification routing,
not only the Telegram bot API. A direct Telegram-send script would prove the bot
token works, but it would bypass the part of the system that failed to provide
conclusive evidence during the outage investigation.

The rule should use a time-based PromQL expression with Prometheus functions
such as `hour()` and `minute()` so it fires only during that five-minute window.

### 2. Notification Health Visibility

Add an "Alert Delivery" row to
`monitoring/grafana/dashboards/system-overview.json`.

The row should include panels for:

- Grafana alert notification failures over a recent window.
- Grafana alert notification latency, if the metric is available.
- Grafana alert-rule evaluation failures or datasource errors.
- Last alert-delivery canary firing/evaluation signal, if exposed through
  Prometheus metrics.

If Prometheus is not scraping Grafana `/metrics`, add or adjust the Grafana
scrape target in `k8s/monitoring/configmaps/prometheus-config.yml` before
adding panels that depend on Grafana meta-monitoring metrics. Do not fake this
with log-only panels if first-class metrics are available.

Dashboard edits should be made in `monitoring/grafana/dashboards/*.json`, then
synced with `make grafana-sync` so
`k8s/monitoring/configmaps/grafana-dashboards.yml` stays generated.

### 3. Read-Only Verification Command

Add a narrow script under `scripts/ops/`, for example
`scripts/ops/check-grafana-alert-delivery.sh`.

The command should be read-only. It may use SSH and `kubectl` for diagnostics,
but it must not apply manifests, edit resources, restart pods, mutate secrets,
or send ad hoc Telegram messages.

The output should answer:

- Did the alert-delivery canary fire or evaluate recently?
- Were there Grafana notification failures in the lookback window?
- Were there Grafana alert-rule evaluation or datasource errors in the lookback
  window?
- If logs are consulted, what precise filtered Grafana log query supports the
  result?

Prefer Prometheus and Grafana APIs/metrics as the primary evidence. Filtered
Grafana logs are acceptable as supporting evidence, but the script should avoid
broad Loki scans because that was one of the investigation pain points.

The command should default to a practical lookback window, such as 24 hours,
and allow an override. It should exit non-zero when the canary has not fired in
the expected window or when notification failures are present.

### 4. `pg-auto-explain-stalled` Fix

Fix `pg-auto-explain-stalled` in
`k8s/monitoring/configmaps/grafana-alerting.yml` so its Loki query does not
exceed Loki's 1-day query limit.

Keep the alert's intent: warn when auto_explain plans stop flowing. The
implementation can either:

- reduce the query range to a safe value under 24 hours, or
- keep a near-24-hour intent while trimming the actual query window below
  Loki's hard limit.

The alert summary should remain precise about the effective window. For
example, if the query uses 23 hours, the summary should say "No auto_explain log
lines in 23h" rather than "24h".

This fix belongs in Phase 5 because repeated alert evaluation errors are
alerting-system noise. They can mask real notification or datasource failures.

## Operational Model

All configuration changes remain declarative:

- Grafana alert rules and contact routing are provisioned through ConfigMaps.
- Dashboard JSON is checked in and synced through `make grafana-sync`.
- Runtime verification is read-only.
- Any future Kubernetes mutation still follows the repo's ops-as-code rules.

The expected deployment path is the normal branch workflow. After deployment,
run the verification command and confirm that the scheduled canary arrives in
Telegram.

## Testing And Validation

Implementation should include:

- YAML validation for modified Kubernetes manifests where practical.
- `make grafana-sync-check` after dashboard edits.
- Targeted inspection that `alert-delivery-canary` is present in the
  provisioned alerting ConfigMap.
- Targeted inspection that `pg-auto-explain-stalled` no longer uses a Loki
  query window that can exceed the 1-day limit.
- Local shell lint or a direct dry run path for the new verification script
  where practical.

Post-deploy validation should include:

- Run the new read-only verification command.
- Confirm the Telegram canary message arrives on schedule.
- Confirm the dashboard's Alert Delivery row renders with real data.
- Confirm Grafana no longer logs repeated `pg-auto-explain-stalled` Loki range
  evaluation failures.

## Risks And Tradeoffs

- A scheduled canary creates intentional alert noise. Keeping it once daily,
  clearly labelled as informational, and automatically resolving keeps that
  noise bounded.
- Grafana metric names can vary by version. Implementation should inspect the
  live `/metrics` output or Prometheus series before finalizing dashboard
  queries.
- A direct Telegram API test would be easier to reason about but would not
  exercise Grafana notification routing. This design deliberately tests the
  production alerting path instead.
- The verification command may need cluster access because Grafana and
  Prometheus live inside Minikube on Debian. That is acceptable for read-only
  runtime diagnostics under the existing project rules.

## Acceptance Criteria

- `alert-delivery-canary` is provisioned as code and routes through the existing
  Telegram contact point.
- A read-only script can report recent canary evidence and notification
  failures without broad Loki scans.
- A Grafana dashboard row exposes alert delivery health and notification
  failure signals.
- `pg-auto-explain-stalled` no longer fails evaluation because of Loki's 1-day
  range cap.
- `make grafana-sync-check` passes after dashboard changes.
- Post-deploy verification proves the canary reaches Telegram.
