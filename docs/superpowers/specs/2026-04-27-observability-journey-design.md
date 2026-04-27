# Observability Journey Page Restructure

- **Date:** 2026-04-27
- **Status:** Approved
- **Spec marker:** `observability-journey`

## Context

The `/observability` page is a polished *system snapshot* — architecture diagram, the three pillars (metrics, logs, traces), 16 alert rules grouped by domain, and a correlation flow diagram. It tells a hiring manager "here's what's instrumented."

What it doesn't tell them is the journey.

Over the past week, four production incidents in QA and prod exposed real instrumentation gaps and drove substantive changes to the stack. Those incidents are documented as ADRs 07-10 under `docs/adr/observability/` but are completely invisible to a frontend reader. The arc — *deployed monitoring stack → tested under fire → found gaps → closed them* — is the most differentiating signal on the page, and it's currently missing.

This spec restructures `/observability` so the system snapshot stays intact (the architecture diagram, pillar copy, alert grid, and correlation flow are all good content) but is now framed by the journey that produced it. Every capability gets a reason it exists. Hiring managers see operational maturity, not just tool selection.

## Goal

Restructure the `/observability` page to lead with the production-incident journey, then present the existing pillar/alerting/correlation content with inline "Lessons from production" callouts that tie specific instrumentation to specific incidents. End with an honest "what's next" section pulled from the ADRs' "Remaining gaps" sections.

## Non-goals

- No backend changes. This is purely a frontend restructure.
- No new ADRs or doc rewrites. The four incident ADRs (07-10) are the source of truth.
- No new dynamic routing. ADR detail lives in the GitHub repo; the page links out.
- No animations, scroll triggers, or interactive timeline reveals. State-free, mobile-friendly, scannable.
- No replacement of the architecture or correlation Mermaid diagrams. They remain as-is.

## Page Structure (final)

```
1. Hero (refreshed framing)
2. Journey timeline                                NEW
3. Architecture diagram                            (kept, unchanged)
4. Metrics pillar     + "Lessons from production"  (inline callout NEW)
5. Logs pillar        + "Lessons from production"  (inline callout NEW)
6. Traces pillar      + "Lessons from production"  (inline callout NEW)
7. Alerting           + "Lessons from production"  (inline callout NEW)
8. Correlation diagram                             (kept, repositioned as "the payoff")
9. What's next / Remaining gaps                    NEW
10. CTAs: Grafana + ADRs on GitHub
```

## 1. Hero refresh

Replace the existing intro paragraph. New copy:

> **Three pillars. Sixteen alert rules. And four production incidents that taught us what was missing.**
>
> Every service in this portfolio is instrumented for metrics, logs, and traces. The journey from "monitoring deployed" to "production-debuggable" took real incidents — gRPC handshakes that hung silently, webhook 500s lost in middleware, agent loops that were black boxes, and Postgres connections we couldn't attribute. Here's what broke, what we shipped, and what we still want to close.

The headline becomes a strong, journey-oriented sentence; the second paragraph names the four incidents inline so the reader knows what's coming.

## 2. Journey timeline (NEW)

A vertical timeline component. Date column on the left, incident card on the right, connecting rail down the side. No JS state. Mobile: rail collapses, dates stack above each card.

### Component: `JourneyTimeline`

Container for the four incident cards plus the connecting rail.

- **File:** `frontend/src/components/observability/JourneyTimeline.tsx`
- **Props:** `incidents: Incident[]`
- **Layout:** Two-column grid on desktop (`grid-cols-[120px_1fr]`), single-column on mobile. Vertical rail rendered with a `border-l` on the cards' wrapper, with circular dot markers via `::before` pseudo-elements positioned on the rail.

### Component: `IncidentCard`

Renders one incident.

- **File:** `frontend/src/components/observability/IncidentCard.tsx`
- **Props:**
  ```ts
  type Incident = {
    date: string;          // "Apr 22, 2026"
    title: string;         // "The mTLS Handshake"
    namespace: string;     // "go-ecommerce" | "ai-services" | "all"
    accentColor: "orange" | "green" | "purple" | "red";
    symptom: string;       // 1 line
    before: string;        // 1-2 sentences
    after: string;         // 1-2 sentences, may include monospace identifiers
    fixes: string[];       // 3-4 short chips
    adrPath: string;       // "docs/adr/observability/07-..."
    adrLabel: string;      // "ADR 07"
  };
  ```
- **Layout:**
  - Left-border accent in the colour matching `accentColor` (matches existing pillar colours: orange = metrics, green = logs, purple = traces, red = cross-cutting/alerting).
  - **Header:** title + namespace badge (`bg-muted text-xs rounded`).
  - **Symptom:** italicised quote-style line, muted foreground.
  - **Before / After:** two-column grid on `sm:` and up, stacked on mobile. Each block has a small label ("Before" / "After") in red-400 / green-400 respectively, then the body. Monospace identifiers (`grpc_client_request_duration_seconds`, `application_name`) wrapped in `<code>` with `bg-muted` styling.
  - **Fix chips:** flex-wrap row of monospace-styled chips.
  - **Footer link:** "Read ADR 07 →" linking to the GitHub blob URL.

### Incident data

Hard-coded as a constant in `frontend/src/app/observability/page.tsx` (or co-located file). Source values:

#### 1. Apr 22, 2026 — The mTLS Handshake (ADR 07)

- **Namespace:** `go-ecommerce`
- **Accent:** red (cross-cutting — gRPC + CI + saga + alerts all touched)
- **Symptom:** *"Order failed. Please try again." — and Loki had nothing useful for 45 minutes.*
- **Before:** "A gRPC mTLS handshake to payment-service hung silently for 30 seconds. The saga blocked, no metrics fired, and the only signal was a stuck order status. Discovering it required `kubectl exec`, `openssl`, and reading `git diff HEAD~1` to realise CI hadn't rebuilt the fix."
- **After:** "A shared `grpcmetrics` client interceptor records `grpc_client_requests_total` and `grpc_client_request_duration_seconds` per target, and emits `slog.ErrorContext` on every non-OK result. All gRPC calls have 30s context deadlines. Saga steps are timed via `saga_step_duration_seconds`. Build SHA is logged at startup so `{app=...} | json | gitSHA=...` answers 'is my fix deployed?' from Loki. CI image-change detection moved from `HEAD~1` to `HEAD~5`."
- **Fixes:** `gRPC interceptor`, `saga step duration`, `build SHA in logs`, `CI HEAD~5`
- **ADR link:** `docs/adr/observability/07-debuggability-and-instrumentation-gaps.md`

#### 2. Apr 23, 2026 — The Silent Webhook (ADR 08)

- **Namespace:** `go-ecommerce`
- **Accent:** green (logs — the dominant gap)
- **Symptom:** *Customer completes Stripe checkout. Cart still full. No order confirmation. Loki shows zero ERROR logs for 24 hours.*
- **Before:** "The `apperror.ErrorHandler()` middleware silently converted `AppError` instances to JSON responses without logging — a webhook 500 vanished. QA and production also shared a RabbitMQ instance with identical queue names, so a QA `clear.cart` command was being consumed by the production cart-service."
- **After:** "Middleware now logs every 5xx `AppError` via `slog.Error` with code, message, status, and request ID before responding — silent server errors are no longer possible. QA runs on a dedicated RabbitMQ `/qa` vhost, fully isolating saga flow. A `saga-order-stalled` Grafana alert fires when `saga_steps_total{step=\"PAYMENT_CREATED\"}` increases but neither `COMPLETED` nor `COMPENSATION_COMPLETE` does within 30 minutes."
- **Fixes:** `5xx middleware logging`, `RabbitMQ /qa vhost`, `saga-order-stalled alert`, `webhook event-type panel`
- **ADR link:** `docs/adr/observability/08-webhook-incident-and-environment-isolation.md`

#### 3. Apr 23, 2026 — The Black-Box Agent Loop (ADR 09)

- **Namespace:** `ai-services`
- **Accent:** purple (traces — six-layer instrumentation centred on traceID propagation)
- **Symptom:** *Loki was deployed and ai-service was emitting JSON logs — but only 4 `slog` calls existed in the entire codebase. A failed agent request gave you "turn started" and "turn ended" with nothing in between.*
- **Before:** "The agent loop made 3-8 LLM roundtrips per request, each potentially triggering 1-N tool calls. When something went wrong the question was always 'which step failed, and why?' — and the only answer was 'add print statements and redeploy.' The OpenAI and Anthropic clients didn't emit OTel spans, so provider comparison wasn't possible in Jaeger."
- **After:** "Six-layer structured logging covers HTTP handler, agent loop, LLM clients, cache, guardrails, and tools. All agent-loop logs use `slog.InfoContext(ctx, ...)` so `tracing.NewLogHandler()` injects the OTel traceID into every record. OpenAI and Anthropic clients now emit `openai.chat` / `anthropic.chat` spans with token attributes — provider comparison is visible in Jaeger. A single Loki query (`{app=\"ai-service\"} | json | traceID=\"...\"`) shows the complete request lifecycle."
- **Fixes:** `6-layer slog`, `OTel span parity`, `traceID-in-logs`, `truncation discipline`
- **ADR link:** `docs/adr/observability/09-ai-service-observability.md`

#### 4. Apr 24, 2026 — The Postgres WAL Incident (ADR 10)

- **Namespace:** `all`
- **Accent:** red (cross-cutting — CI, K8s, DB connection layer)
- **Symptom:** *During a Postgres WAL corruption incident, three observability gaps made diagnosis harder than necessary. We couldn't tell which deploy preceded the metric change. K8s Warning events expired before we looked. And `pg_stat_activity` showed every connection as the same `taskuser` — no way to tell which service owned them.*
- **Before:** "Deploy timestamps had to be reconstructed from `kubectl get events`. K8s Warning events (OOM kills, probe failures, evictions) lived 1 hour and weren't queryable from Grafana. All six Go services shared the same Postgres credentials — a connection leak in one was indistinguishable from normal load across all."
- **After:** "Every CI rollout posts a Grafana annotation tagged with namespace + short SHA via `/api/annotations`, with anonymous-Viewer auth preserved for public dashboard viewing. `kubernetes-event-exporter` (resmoio fork) ships Warning-only events into Loki under `{job=\"kube-event-exporter\"}` with namespace/reason/kind/name labels. Every Go service's `DATABASE_URL` includes `application_name=<service-name>`, so a 'Connections by Service' dashboard panel attributes every Postgres connection in seconds."
- **Fixes:** `CI deploy annotations`, `K8s events → Loki`, `application_name in DSN`, `connection attribution panel`
- **ADR link:** `docs/adr/observability/10-observability-gaps.md`

## 3. Architecture diagram (kept)

The existing Mermaid `flowchart TD` block stays unchanged — including the Apps / Collection / Storage / Output subgraphs and the Grafana → Telegram alerting edge.

## 4-7. Pillars + inline lesson callouts

Each existing pillar section (Metrics, Logs, Traces, Alerting) keeps its body copy, code chips, and visual treatment exactly as-is. Below the body of each pillar, render a small inset "Lessons from production" callout.

### Component: `LessonCallout`

- **File:** `frontend/src/components/observability/LessonCallout.tsx`
- **Props:**
  ```ts
  type LessonCallout = {
    badges: string[];   // e.g., ["ADR 07", "ADR 10"]
    children: React.ReactNode;
  };
  ```
- **Layout:** Inset card, `bg-muted/30 border border-foreground/10 rounded-lg p-4 mt-4`. Top row: badge pills (`bg-foreground/10 text-xs rounded px-2 py-0.5`) labelled "ADR 07" etc. Body: prose with `<code>` highlights for metric and identifier names.

### Callout content

- **Metrics — ADRs 07, 10**
  > After **ADR 07**, every outbound gRPC call is metered: `grpc_client_request_duration_seconds{target=...}` and `grpc_client_requests_total` with target/method/code labels. After **ADR 10**, every PostgreSQL connection is attributed via `application_name=` in the `DATABASE_URL`, surfaced in a Grafana "Connections by Service" panel.

- **Logs — ADRs 08, 09**
  > After **ADR 08**, the `apperror` middleware logs every 5xx `AppError` — silent server errors are no longer possible. After **ADR 09**, the AI agent loop emits structured logs at six layers (HTTP, agent, LLM client, cache, guardrails, tools), all with the OTel traceID injected, so `{app="ai-service"} | json | traceID="..."` returns the full request lifecycle.

- **Traces — ADR 09**
  > After **ADR 09**, OpenAI and Anthropic clients emit `openai.chat` / `anthropic.chat` spans with `otelhttp.NewTransport`-based propagation, matching the existing Ollama instrumentation. Provider comparison is now possible in Jaeger as child spans of the agent turn.

- **Alerting — ADRs 08, 10**
  > After **ADR 08**, a `saga-order-stalled` rule fires when `saga_steps_total{step="PAYMENT_CREATED"}` increases without a matching `COMPLETED` within 30 minutes. After **ADR 10**, every CI rollout posts a Grafana annotation tagged with namespace and short SHA, so dashboards mark the exact deploy preceding any metric change.

## 8. Correlation diagram (repositioned)

Move the existing correlation section so it sits *after* the alerting pillar — it functions as the payoff: once the journey + pillars + alerts are established, this is the alert→logs→trace flow that ties them together. The Mermaid sequence diagram and surrounding copy remain unchanged.

## 9. What's next (NEW)

A small grid of "remaining gaps" cards, pulled from the explicit "Remaining gaps" sections in ADRs 07, 09, and 10. Frames the page honestly: *"production maturity is continuous — here's what's on the roadmap."* This is a strong "I know what I don't have" signal.

### Component: `GapsGrid`

- **File:** `frontend/src/components/observability/GapsGrid.tsx`
- **Props:** `gaps: Gap[]`
- **Layout:** 2-column grid on `sm:`, single-column on mobile. Each card: `bg-card border border-foreground/10 rounded-lg p-4`. Title in `text-sm font-semibold`, description in `text-xs text-muted-foreground`.

### Gap content

```ts
const GAPS = [
  {
    title: "PostgreSQL query tracing",
    description: "Manual OpenTelemetry spans around slow queries. Useful but a significant lift for incremental value over pgxpool's existing connection metrics.",
    source: "ADR 07",
  },
  {
    title: "RabbitMQ queue depth metrics",
    description: "Requires scraping the RabbitMQ management API. Saga DLQ depth alerts depend on it being shipped first.",
    source: "ADR 07",
  },
  {
    title: "Grafana dashboard for AI agent",
    description: "A dedicated panel set for agent-loop debugging. Deferred until QA logs accumulate enough volume to know which queries are most useful.",
    source: "ADR 09",
  },
  {
    title: "Promtail trace_id field for Python",
    description: "Pipeline change deployed but not yet verified end-to-end. Once verified, Python ingestion / chat / debug services join the unified traceID-in-logs experience.",
    source: "ADR 07",
  },
];
```

Sub-headline above the grid: *"Production maturity is a continuous process. Here's what's on the roadmap."*

## 10. CTAs

The existing "View Live Grafana Dashboard" button is preserved. Add a sibling "View ADRs on GitHub" button.

- **Layout:** Side-by-side on desktop (`flex flex-col sm:flex-row gap-3 justify-center`), stacked on mobile. Both use the existing primary button style; the second uses an outline / secondary variant to differentiate.
- **Grafana button:** unchanged. Links to existing system-overview dashboard.
- **GitHub button:** links to `https://github.com/kabradshaw1/gen_ai_engineer/tree/main/docs/adr/observability`.

## Component inventory

New components, all under `frontend/src/components/observability/`:

| File | Purpose |
| --- | --- |
| `JourneyTimeline.tsx` | Vertical rail + date column wrapper |
| `IncidentCard.tsx` | Single incident card (header / symptom / before-after / chips / link) |
| `LessonCallout.tsx` | Inset card for inline pillar callouts |
| `GapsGrid.tsx` | "What's next" grid |

Reused, unchanged: `MermaidDiagram`, `next/link`.

Data lives in `frontend/src/app/observability/page.tsx` as page-local constants (mirrors the existing `ALERT_GROUPS` constant pattern). No external data fetching, no API calls.

## GitHub URL handling

The four ADR links point at GitHub blob URLs on `main`:

```
https://github.com/kabradshaw1/gen_ai_engineer/blob/main/docs/adr/observability/07-debuggability-and-instrumentation-gaps.md
https://github.com/kabradshaw1/gen_ai_engineer/blob/main/docs/adr/observability/08-webhook-incident-and-environment-isolation.md
https://github.com/kabradshaw1/gen_ai_engineer/blob/main/docs/adr/observability/09-ai-service-observability.md
https://github.com/kabradshaw1/gen_ai_engineer/blob/main/docs/adr/observability/10-observability-gaps.md
```

Centralised in a `frontend/src/lib/observability/adrs.ts` constant so the URL prefix isn't repeated four times. All links open in a new tab (`target="_blank"` + `rel="noopener noreferrer"`).

## Visual / styling rules

- **Accent colours** match the existing pillar palette so the page feels coherent: `orange-500/60` for metrics-driven incidents, `green-500/60` for logs, `purple-500/60` for traces, `red-500/60` for cross-cutting / alerting.
- **Date typography** in the timeline: `text-xs text-muted-foreground font-mono uppercase tracking-wide`.
- **Before / After labels:** `text-xs font-semibold` with `text-red-400` / `text-green-400` colour cues. Body copy is normal `text-sm text-muted-foreground leading-relaxed`.
- **Fix chips:** `rounded bg-muted px-1.5 py-0.5 text-xs font-mono text-muted-foreground` — same treatment as the existing metric-name chips on the page.
- **Mobile breakpoint:** vertical rail collapses below `sm:`. Date moves above the card. Before/After blocks stack.

## Things explicitly NOT changing

- Architecture and correlation Mermaid diagrams (content + framing).
- Pillar body copy (orange/green/purple/red sections; only the callouts are added beneath each).
- Existing CTA button styling (we add a sibling, don't restyle the primary button).
- Page max-width and outer padding (`max-w-3xl px-6 py-12`).

## Acceptance criteria

- [ ] `frontend/src/app/observability/page.tsx` renders in the order: hero → journey timeline → architecture diagram → metrics + callout → logs + callout → traces + callout → alerting + callout → correlation diagram → what's next → CTAs.
- [ ] Four incident cards render with the data above. Each has a working "Read ADR XX →" link to GitHub on main.
- [ ] Each pillar shows a "Lessons from production" callout beneath its body. Callouts include the named ADR badges (07/08/09/10).
- [ ] "What's next" section lists the four gap items.
- [ ] "View ADRs on GitHub" button renders alongside the existing Grafana button.
- [ ] Page degrades gracefully on mobile: timeline collapses, before/after blocks stack, gap grid becomes single-column.
- [ ] No TypeScript errors. `make preflight-frontend` passes (tsc + Next.js build).
- [ ] No regression in existing diagrams — both Mermaid blocks render unchanged.

## Out of scope

- Telemetry on the page itself (no analytics changes).
- Any backend, alert, or instrumentation changes — those are already shipped via ADRs 07-10.
- Rendering ADRs in-app via MDX (option C from brainstorming).
- E2E test coverage for the page (Playwright suite in `frontend/e2e/` does not currently cover `/observability`; adding it is a separate spec).
- Updates to the homepage card linking into `/observability`.
