# Observability Journey Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restructure `/observability` to lead with a four-incident journey timeline (ADRs 07-10), add inline "Lessons from production" callouts beneath each pillar, and surface a "What's next" gaps section — without changing existing pillar copy or diagrams.

**Architecture:** Pure frontend, presentational components only. New components co-located under `frontend/src/components/observability/`. Page-local data plus a single shared constants module for ADR GitHub URLs. No backend, no data fetching, no animation, no JS state.

**Tech Stack:** Next.js 16 App Router, React 19, TypeScript, Tailwind CSS 4, existing `MermaidDiagram` component. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-04-27-observability-journey-design.md`

**Note on TDD:** The frontend has no unit-test framework — only Playwright E2E (out of scope per the spec). These are presentational components with no business logic, so verification per task is `npx tsc --noEmit` (type check) + visual inspection via `npm run dev`. Final verification is `make preflight-frontend` (lint + tsc + Next.js build).

**File structure:**

| File | Status | Responsibility |
| --- | --- | --- |
| `frontend/src/lib/observability/adrs.ts` | Create | GitHub URL builder + ADR slug → label map |
| `frontend/src/components/observability/IncidentCard.tsx` | Create | One incident card (title / symptom / before-after / chips / link) |
| `frontend/src/components/observability/JourneyTimeline.tsx` | Create | Vertical rail + date column wrapper around `IncidentCard[]` |
| `frontend/src/components/observability/LessonCallout.tsx` | Create | Inset card for inline pillar callouts |
| `frontend/src/components/observability/GapsGrid.tsx` | Create | "What's next" gap card grid |
| `frontend/src/app/observability/page.tsx` | Modify | Restructure: hero refresh, mount new components, add GitHub CTA |

---

### Task 1: ADR URL constants module

**Files:**
- Create: `frontend/src/lib/observability/adrs.ts`

- [ ] **Step 1: Create the directory**

```bash
mkdir -p frontend/src/lib/observability
```

- [ ] **Step 2: Create `adrs.ts` with the GitHub URL helper and ADR map**

Write the file with this exact content:

```ts
const REPO_BASE_URL =
  "https://github.com/kabradshaw1/gen_ai_engineer/blob/main";

export const ADR_DIRECTORY_URL =
  "https://github.com/kabradshaw1/gen_ai_engineer/tree/main/docs/adr/observability";

export type AdrId = "07" | "08" | "09" | "10";

const ADR_PATHS: Record<AdrId, string> = {
  "07": "docs/adr/observability/07-debuggability-and-instrumentation-gaps.md",
  "08": "docs/adr/observability/08-webhook-incident-and-environment-isolation.md",
  "09": "docs/adr/observability/09-ai-service-observability.md",
  "10": "docs/adr/observability/10-observability-gaps.md",
};

export function adrUrl(id: AdrId): string {
  return `${REPO_BASE_URL}/${ADR_PATHS[id]}`;
}

export function adrLabel(id: AdrId): string {
  return `ADR ${id}`;
}
```

- [ ] **Step 3: Verify the module type-checks**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors related to `lib/observability/adrs.ts`.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/lib/observability/adrs.ts
git -c commit.gpgsign=false commit -m "feat(observability): add ADR URL constants module"
```

---

### Task 2: `IncidentCard` component

**Files:**
- Create: `frontend/src/components/observability/IncidentCard.tsx`

- [ ] **Step 1: Create the directory**

```bash
mkdir -p frontend/src/components/observability
```

- [ ] **Step 2: Create `IncidentCard.tsx` with the full component**

Write the file with this exact content:

```tsx
import Link from "next/link";
import { adrLabel, adrUrl, type AdrId } from "@/lib/observability/adrs";

export type IncidentAccent = "orange" | "green" | "purple" | "red";

export type Incident = {
  date: string;
  title: string;
  namespace: string;
  accent: IncidentAccent;
  symptom: string;
  before: string;
  after: string;
  fixes: string[];
  adrId: AdrId;
};

const ACCENT_BORDER: Record<IncidentAccent, string> = {
  orange: "border-orange-500/60",
  green: "border-green-500/60",
  purple: "border-purple-500/60",
  red: "border-red-500/60",
};

function renderWithCode(text: string): React.ReactNode {
  const parts = text.split(/(`[^`]+`)/g);
  return parts.map((part, i) => {
    if (part.startsWith("`") && part.endsWith("`")) {
      return (
        <code
          key={i}
          className="rounded bg-muted px-1.5 py-0.5 text-xs font-mono"
        >
          {part.slice(1, -1)}
        </code>
      );
    }
    return <span key={i}>{part}</span>;
  });
}

export function IncidentCard({ incident }: { incident: Incident }) {
  const borderClass = ACCENT_BORDER[incident.accent];

  return (
    <div
      className={`rounded-xl border border-foreground/10 border-l-4 ${borderClass} bg-card p-5`}
    >
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <h3 className="text-lg font-semibold">{incident.title}</h3>
        <span className="rounded bg-muted px-2 py-0.5 font-mono text-xs text-muted-foreground">
          {incident.namespace}
        </span>
      </div>
      <p className="mt-3 border-l-2 border-foreground/20 pl-3 text-sm italic text-muted-foreground leading-relaxed">
        {incident.symptom}
      </p>

      <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2">
        <div className="rounded-lg border border-red-500/20 bg-red-500/5 p-3">
          <div className="text-xs font-semibold uppercase tracking-wide text-red-400">
            Before
          </div>
          <p className="mt-2 text-sm text-muted-foreground leading-relaxed">
            {renderWithCode(incident.before)}
          </p>
        </div>
        <div className="rounded-lg border border-green-500/20 bg-green-500/5 p-3">
          <div className="text-xs font-semibold uppercase tracking-wide text-green-400">
            After
          </div>
          <p className="mt-2 text-sm text-muted-foreground leading-relaxed">
            {renderWithCode(incident.after)}
          </p>
        </div>
      </div>

      <div className="mt-4 flex flex-wrap gap-2">
        {incident.fixes.map((fix) => (
          <code
            key={fix}
            className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs text-muted-foreground"
          >
            {fix}
          </code>
        ))}
      </div>

      <div className="mt-4 flex justify-end">
        <Link
          href={adrUrl(incident.adrId)}
          target="_blank"
          rel="noopener noreferrer"
          className="text-xs font-medium text-primary hover:underline"
        >
          Read {adrLabel(incident.adrId)} &rarr;
        </Link>
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Type-check**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/observability/IncidentCard.tsx
git -c commit.gpgsign=false commit -m "feat(observability): add IncidentCard component"
```

---

### Task 3: `JourneyTimeline` component

**Files:**
- Create: `frontend/src/components/observability/JourneyTimeline.tsx`

- [ ] **Step 1: Create `JourneyTimeline.tsx`**

Write the file with this exact content:

```tsx
import { IncidentCard, type Incident } from "./IncidentCard";

export function JourneyTimeline({ incidents }: { incidents: Incident[] }) {
  return (
    <ol className="relative space-y-6 sm:space-y-8">
      {incidents.map((incident, idx) => (
        <li
          key={incident.adrId}
          className="grid grid-cols-1 gap-3 sm:grid-cols-[120px_1fr] sm:gap-6"
        >
          <div className="flex sm:flex-col sm:items-end sm:pt-5">
            <span className="font-mono text-xs uppercase tracking-wide text-muted-foreground">
              {incident.date}
            </span>
            <span className="ml-2 sm:ml-0 sm:mt-1 font-mono text-xs text-muted-foreground/70">
              #{idx + 1}
            </span>
          </div>
          <IncidentCard incident={incident} />
        </li>
      ))}
    </ol>
  );
}
```

- [ ] **Step 2: Type-check**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/observability/JourneyTimeline.tsx
git -c commit.gpgsign=false commit -m "feat(observability): add JourneyTimeline component"
```

---

### Task 4: `LessonCallout` component

**Files:**
- Create: `frontend/src/components/observability/LessonCallout.tsx`

- [ ] **Step 1: Create `LessonCallout.tsx`**

Write the file with this exact content:

```tsx
import type { ReactNode } from "react";
import { adrLabel, type AdrId } from "@/lib/observability/adrs";

export function LessonCallout({
  adrIds,
  children,
}: {
  adrIds: AdrId[];
  children: ReactNode;
}) {
  return (
    <div className="mt-4 rounded-lg border border-foreground/10 bg-muted/30 p-4">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          Lessons from production
        </span>
        {adrIds.map((id) => (
          <span
            key={id}
            className="rounded bg-foreground/10 px-2 py-0.5 font-mono text-xs text-muted-foreground"
          >
            {adrLabel(id)}
          </span>
        ))}
      </div>
      <div className="mt-3 text-sm text-muted-foreground leading-relaxed">
        {children}
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Type-check**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/observability/LessonCallout.tsx
git -c commit.gpgsign=false commit -m "feat(observability): add LessonCallout component"
```

---

### Task 5: `GapsGrid` component

**Files:**
- Create: `frontend/src/components/observability/GapsGrid.tsx`

- [ ] **Step 1: Create `GapsGrid.tsx`**

Write the file with this exact content:

```tsx
export type Gap = {
  title: string;
  description: string;
  source: string;
};

export function GapsGrid({ gaps }: { gaps: Gap[] }) {
  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
      {gaps.map((gap) => (
        <div
          key={gap.title}
          className="rounded-lg border border-foreground/10 bg-card p-4"
        >
          <div className="flex items-baseline justify-between gap-2">
            <h3 className="text-sm font-semibold">{gap.title}</h3>
            <span className="font-mono text-xs text-muted-foreground">
              {gap.source}
            </span>
          </div>
          <p className="mt-2 text-xs text-muted-foreground leading-relaxed">
            {gap.description}
          </p>
        </div>
      ))}
    </div>
  );
}
```

- [ ] **Step 2: Type-check**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/observability/GapsGrid.tsx
git -c commit.gpgsign=false commit -m "feat(observability): add GapsGrid component"
```

---

### Task 6: Restructure `observability/page.tsx`

**Files:**
- Modify: `frontend/src/app/observability/page.tsx` (full rewrite — preserves the two Mermaid diagram strings, the `ALERT_GROUPS` constant, the architecture/correlation/metrics/logs/traces/alerting copy and code chips; adds the new sections and components around them).

- [ ] **Step 1: Replace the entire file with the restructured version**

Open `frontend/src/app/observability/page.tsx` and replace its full contents with:

```tsx
import { MermaidDiagram } from "@/components/MermaidDiagram";
import { GapsGrid, type Gap } from "@/components/observability/GapsGrid";
import {
  IncidentCard,
  type Incident,
} from "@/components/observability/IncidentCard";
import { JourneyTimeline } from "@/components/observability/JourneyTimeline";
import { LessonCallout } from "@/components/observability/LessonCallout";
import { ADR_DIRECTORY_URL } from "@/lib/observability/adrs";
import Link from "next/link";

const architectureDiagram = `flowchart TD
  subgraph Apps["Application Services"]
    direction LR
    GO["Go Services<br/><i>auth · ecommerce · ai · analytics</i>"]
    JAVA["Java Services<br/><i>task · activity · notification · gateway</i>"]
    PY["Python Services<br/><i>ingestion · chat · debug · eval</i>"]
  end

  subgraph Collect["Collection Layer"]
    direction LR
    PROM_SC["Prometheus Scrape<br/><i>15s interval · pod annotations</i>"]
    PROMTAIL["Promtail DaemonSet<br/><i>JSON parsing · traceID extraction</i>"]
    OTEL["OTel SDK<br/><i>W3C traceparent · Kafka headers</i>"]
  end

  subgraph Store["Storage"]
    direction LR
    PROM["Prometheus<br/><i>TSDB · 15d retention</i>"]
    LOKI["Loki<br/><i>label-indexed · 7d retention</i>"]
    JAEGER["Jaeger<br/><i>OTLP gRPC collector</i>"]
  end

  subgraph Out["Output"]
    direction LR
    GRAFANA["Grafana<br/><i>5 dashboards · 16 alert rules</i>"]
    TELEGRAM["Telegram<br/><i>instant notifications</i>"]
  end

  Apps -->|metrics| PROM_SC
  Apps -->|logs| PROMTAIL
  Apps -->|traces| OTEL

  PROM_SC --> PROM
  PROMTAIL --> LOKI
  OTEL --> JAEGER

  PROM --> GRAFANA
  LOKI --> GRAFANA
  JAEGER --> GRAFANA
  GRAFANA -->|alerts| TELEGRAM`;

const correlationDiagram = `sequenceDiagram
  participant T as Telegram
  participant G as Grafana
  participant P as Prometheus
  participant L as Loki
  participant J as Jaeger

  T->>G: Alert: error rate > 2%
  G->>P: Query error rate metrics
  P-->>G: Spike at 14:32
  G->>L: Filter logs by namespace + time
  L-->>G: Error logs with traceID
  G->>J: Open trace by traceID
  J-->>G: Full request path
  Note over G: Root cause identified`;

const ALERT_GROUPS = [
  {
    name: "Infrastructure",
    color: "border-red-500/50",
    description:
      "GPU exporter health, AI service readiness, GPU temperature and VRAM usage",
    count: 4,
  },
  {
    name: "Kubernetes Health",
    color: "border-red-500/50",
    description:
      "OOM kills, pod restart storms, container memory pressure, node disk pressure, stuck deployments",
    count: 6,
  },
  {
    name: "Application SLOs",
    color: "border-red-500/50",
    description:
      "HTTP error rate and p95 latency targets for Go AI, Go ecommerce, and Java gateway services",
    count: 6,
  },
  {
    name: "Streaming Analytics",
    color: "border-red-500/50",
    description:
      "Kafka consumer lag monitoring across order, cart, and product view event topics",
    count: 1,
  },
];

const INCIDENTS: Incident[] = [
  {
    date: "Apr 22, 2026",
    title: "The mTLS Handshake",
    namespace: "go-ecommerce",
    accent: "red",
    symptom:
      '"Order failed. Please try again." — and Loki had nothing useful for 45 minutes.',
    before:
      "A gRPC mTLS handshake to payment-service hung silently for 30 seconds. The saga blocked, no metrics fired, and the only signal was a stuck order status. Discovering it required `kubectl exec`, `openssl`, and reading `git diff HEAD~1` to realise CI had never rebuilt the fix.",
    after:
      "A shared `grpcmetrics` client interceptor records `grpc_client_requests_total` and `grpc_client_request_duration_seconds` per target, and emits `slog.ErrorContext` on every non-OK result. All gRPC calls now have 30s context deadlines. Saga steps are timed via `saga_step_duration_seconds`. Build SHA is logged at startup so `{app=...} | json | gitSHA=...` answers 'is my fix deployed?' from Loki. CI image-change detection moved from `HEAD~1` to `HEAD~5`.",
    fixes: [
      "gRPC interceptor",
      "saga step duration",
      "build SHA in logs",
      "CI HEAD~5",
    ],
    adrId: "07",
  },
  {
    date: "Apr 23, 2026",
    title: "The Silent Webhook",
    namespace: "go-ecommerce",
    accent: "green",
    symptom:
      "Customer completes Stripe checkout. Cart still full. No order confirmation. Loki shows zero ERROR logs for 24 hours.",
    before:
      "The `apperror.ErrorHandler()` middleware silently converted `AppError` instances to JSON responses without logging — a webhook 500 vanished. QA and production also shared a RabbitMQ instance with identical queue names, so a QA `clear.cart` command was being consumed by the production cart-service.",
    after:
      "Middleware now logs every 5xx `AppError` via `slog.Error` with code, message, status, and request ID before responding — silent server errors are no longer possible. QA runs on a dedicated RabbitMQ `/qa` vhost, fully isolating saga flow. A `saga-order-stalled` Grafana alert fires when `saga_steps_total{step=\"PAYMENT_CREATED\"}` increases but neither `COMPLETED` nor `COMPENSATION_COMPLETE` does within 30 minutes.",
    fixes: [
      "5xx middleware logging",
      "RabbitMQ /qa vhost",
      "saga-order-stalled alert",
      "webhook event-type panel",
    ],
    adrId: "08",
  },
  {
    date: "Apr 23, 2026",
    title: "The Black-Box Agent Loop",
    namespace: "ai-services",
    accent: "purple",
    symptom:
      "Loki was deployed and ai-service was emitting JSON logs — but only 4 `slog` calls existed in the entire codebase. A failed agent request gave you 'turn started' and 'turn ended' with nothing in between.",
    before:
      "The agent loop made 3-8 LLM roundtrips per request, each potentially triggering 1-N tool calls. When something went wrong the question was always 'which step failed, and why?' — and the only answer was 'add print statements and redeploy.' The OpenAI and Anthropic clients didn't emit OTel spans, so provider comparison wasn't possible in Jaeger.",
    after:
      "Six-layer structured logging covers HTTP handler, agent loop, LLM clients, cache, guardrails, and tools. All agent-loop logs use `slog.InfoContext(ctx, ...)` so `tracing.NewLogHandler()` injects the OTel traceID into every record. OpenAI and Anthropic clients now emit `openai.chat` / `anthropic.chat` spans with token attributes — provider comparison is visible in Jaeger. A single Loki query (`{app=\"ai-service\"} | json | traceID=\"...\"`) shows the complete request lifecycle.",
    fixes: [
      "6-layer slog",
      "OTel span parity",
      "traceID-in-logs",
      "truncation discipline",
    ],
    adrId: "09",
  },
  {
    date: "Apr 24, 2026",
    title: "The Postgres WAL Incident",
    namespace: "all",
    accent: "red",
    symptom:
      "During a Postgres WAL corruption incident, three observability gaps made diagnosis harder than necessary. We couldn't tell which deploy preceded the metric change. K8s Warning events expired before we looked. And `pg_stat_activity` showed every connection as the same `taskuser` — no way to tell which service owned them.",
    before:
      "Deploy timestamps had to be reconstructed from `kubectl get events`. K8s Warning events (OOM kills, probe failures, evictions) lived 1 hour and weren't queryable from Grafana. All six Go services shared the same Postgres credentials — a connection leak in one was indistinguishable from normal load across all.",
    after:
      "Every CI rollout posts a Grafana annotation tagged with namespace + short SHA via `/api/annotations`, with anonymous-Viewer auth preserved for public dashboard viewing. `kubernetes-event-exporter` (resmoio fork) ships Warning-only events into Loki under `{job=\"kube-event-exporter\"}` with namespace/reason/kind/name labels. Every Go service's `DATABASE_URL` includes `application_name=<service-name>`, so a 'Connections by Service' dashboard panel attributes every Postgres connection in seconds.",
    fixes: [
      "CI deploy annotations",
      "K8s events → Loki",
      "application_name in DSN",
      "connection attribution panel",
    ],
    adrId: "10",
  },
];

const GAPS: Gap[] = [
  {
    title: "PostgreSQL query tracing",
    description:
      "Manual OpenTelemetry spans around slow queries. Useful but a significant lift for incremental value over pgxpool's existing connection metrics.",
    source: "ADR 07",
  },
  {
    title: "RabbitMQ queue depth metrics",
    description:
      "Requires scraping the RabbitMQ management API. Saga DLQ depth alerts depend on it being shipped first.",
    source: "ADR 07",
  },
  {
    title: "Grafana dashboard for AI agent",
    description:
      "A dedicated panel set for agent-loop debugging. Deferred until QA logs accumulate enough volume to know which queries are most useful.",
    source: "ADR 09",
  },
  {
    title: "Promtail trace_id field for Python",
    description:
      "Pipeline change deployed but not yet verified end-to-end. Once verified, Python ingestion / chat / debug services join the unified traceID-in-logs experience.",
    source: "ADR 07",
  },
];

export default function ObservabilityPage() {
  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <h1 className="mt-8 text-3xl font-bold">Observability</h1>

      {/* Hero */}
      <section className="mt-8">
        <p className="text-lg font-medium leading-relaxed">
          Three pillars. Sixteen alert rules. And four production incidents that
          taught us what was missing.
        </p>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          Every service in this portfolio is instrumented for metrics, logs, and
          traces. The journey from &ldquo;monitoring deployed&rdquo; to
          &ldquo;production-debuggable&rdquo; took real incidents &mdash; gRPC
          handshakes that hung silently, webhook 500s lost in middleware, agent
          loops that were black boxes, and Postgres connections we
          couldn&rsquo;t attribute. Here&rsquo;s what broke, what we shipped,
          and what we still want to close.
        </p>
      </section>

      {/* Journey timeline */}
      <section className="mt-12">
        <h2 className="text-2xl font-semibold">The Journey</h2>
        <p className="mt-3 text-muted-foreground leading-relaxed">
          Four incidents in three days. Each exposed a gap, drove a concrete
          change, and shaped the system you see below.
        </p>
        <div className="mt-6">
          <JourneyTimeline incidents={INCIDENTS} />
        </div>
      </section>

      {/* Architecture diagram */}
      <section className="mt-12">
        <h2 className="text-2xl font-semibold">Architecture</h2>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          Three language stacks feed three collection pipelines. Everything
          converges in Grafana for unified dashboards and alerting.
        </p>
        <div className="mt-6 rounded-xl border border-foreground/10 bg-card p-6">
          <MermaidDiagram chart={architectureDiagram} />
        </div>
      </section>

      {/* Metrics */}
      <section className="mt-12">
        <div className="border-l-4 border-orange-500/60 pl-4">
          <h2 className="text-2xl font-semibold">Metrics &mdash; Prometheus</h2>
          <p className="mt-4 text-muted-foreground leading-relaxed">
            Prometheus scrapes every pod annotated with{" "}
            <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
              prometheus.io/scrape: &quot;true&quot;
            </code>{" "}
            on a 15-second interval. Infrastructure exporters provide cluster
            and hardware visibility: kube-state-metrics for pod status and
            deployment health, node-exporter for CPU, memory, and disk, and a
            GPU exporter for NVIDIA utilization and temperature.
          </p>
          <div className="mt-4 flex flex-wrap gap-2">
            {[
              "http_requests_total",
              "http_request_duration_seconds",
              "kafka_consumer_lag",
              "container_memory_working_set_bytes",
              "go_goroutines",
              "nvidia_smi_temperature_gpu",
            ].map((m) => (
              <code
                key={m}
                className="rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground"
              >
                {m}
              </code>
            ))}
          </div>
          <LessonCallout adrIds={["07", "10"]}>
            After ADR 07, every outbound gRPC call is metered:{" "}
            <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">
              grpc_client_request_duration_seconds&#123;target=...&#125;
            </code>{" "}
            and{" "}
            <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">
              grpc_client_requests_total
            </code>{" "}
            with target/method/code labels. After ADR 10, every PostgreSQL
            connection is attributed via{" "}
            <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">
              application_name=
            </code>{" "}
            in the{" "}
            <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">
              DATABASE_URL
            </code>
            , surfaced in a Grafana &ldquo;Connections by Service&rdquo; panel.
          </LessonCallout>
        </div>
      </section>

      {/* Logs */}
      <section className="mt-12">
        <div className="border-l-4 border-green-500/60 pl-4">
          <h2 className="text-2xl font-semibold">
            Logs &mdash; Loki + Promtail
          </h2>
          <p className="mt-4 text-muted-foreground leading-relaxed">
            Promtail runs as a DaemonSet on every node, tailing container logs
            from{" "}
            <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
              /var/log/pods/
            </code>
            . Go services emit structured JSON via{" "}
            <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
              slog
            </code>{" "}
            with a custom handler that injects the OpenTelemetry traceID into
            every log line. Java services use logstash-logback-encoder for the
            same JSON output. Loki indexes only labels &mdash; namespace, pod,
            level &mdash; keeping storage efficient on a single-node cluster.
          </p>
          <div className="mt-4 rounded-lg border border-foreground/10 bg-muted/50 p-3">
            <code className="text-xs text-green-400">
              {'{namespace="go-ecommerce"} | json | level="error"'}
            </code>
          </div>
          <LessonCallout adrIds={["08", "09"]}>
            After ADR 08, the{" "}
            <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">
              apperror
            </code>{" "}
            middleware logs every 5xx{" "}
            <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">
              AppError
            </code>{" "}
            &mdash; silent server errors are no longer possible. After ADR 09,
            the AI agent loop emits structured logs at six layers (HTTP, agent,
            LLM client, cache, guardrails, tools), all with the OTel traceID
            injected, so{" "}
            <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">
              {'{app="ai-service"} | json | traceID="..."'}
            </code>{" "}
            returns the full request lifecycle.
          </LessonCallout>
        </div>
      </section>

      {/* Traces */}
      <section className="mt-12">
        <div className="border-l-4 border-purple-500/60 pl-4">
          <h2 className="text-2xl font-semibold">
            Traces &mdash; Jaeger + OpenTelemetry
          </h2>
          <p className="mt-4 text-muted-foreground leading-relaxed">
            The Go services are instrumented with the OpenTelemetry SDK.{" "}
            <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
              otelgin
            </code>{" "}
            middleware auto-instruments HTTP handlers, and{" "}
            <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
              otelhttp
            </code>{" "}
            propagates W3C traceparent headers on outbound calls. Trace context
            also flows through Kafka message headers, so a single request can be
            traced from the HTTP gateway through ecommerce processing, across an
            async Kafka boundary, to the analytics consumer.
          </p>
          <LessonCallout adrIds={["09"]}>
            After ADR 09, OpenAI and Anthropic clients emit{" "}
            <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">
              openai.chat
            </code>{" "}
            /{" "}
            <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">
              anthropic.chat
            </code>{" "}
            spans with{" "}
            <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">
              otelhttp.NewTransport
            </code>
            -based propagation, matching the existing Ollama instrumentation.
            Provider comparison is now possible in Jaeger as child spans of the
            agent turn.
          </LessonCallout>
        </div>
      </section>

      {/* Alerting */}
      <section className="mt-12">
        <div className="border-l-4 border-red-500/60 pl-4">
          <h2 className="text-2xl font-semibold">
            Alerting &mdash; 16 Rules &rarr; Telegram
          </h2>
          <p className="mt-4 text-muted-foreground leading-relaxed">
            Four alert groups cover infrastructure through application layers.
            Symptom-based SLO alerts catch user-facing degradation before
            anything crashes. All alerts route to Telegram via Grafana unified
            alerting.
          </p>
          <div className="mt-6 grid grid-cols-1 gap-3 sm:grid-cols-2">
            {ALERT_GROUPS.map((group) => (
              <div
                key={group.name}
                className="rounded-lg border border-foreground/10 bg-card p-4"
              >
                <div className="flex items-center justify-between">
                  <h3 className="text-sm font-semibold">{group.name}</h3>
                  <span className="text-xs text-muted-foreground">
                    {group.count} {group.count === 1 ? "rule" : "rules"}
                  </span>
                </div>
                <p className="mt-1 text-xs text-muted-foreground leading-relaxed">
                  {group.description}
                </p>
              </div>
            ))}
          </div>
          <LessonCallout adrIds={["08", "10"]}>
            After ADR 08, a{" "}
            <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">
              saga-order-stalled
            </code>{" "}
            rule fires when{" "}
            <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">
              {'saga_steps_total{step="PAYMENT_CREATED"}'}
            </code>{" "}
            increases without a matching{" "}
            <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">
              COMPLETED
            </code>{" "}
            within 30 minutes. After ADR 10, every CI rollout posts a Grafana
            annotation tagged with namespace and short SHA, so dashboards mark
            the exact deploy preceding any metric change.
          </LessonCallout>
        </div>
      </section>

      {/* Correlation */}
      <section className="mt-12">
        <div className="border-l-4 border-yellow-500/60 pl-4">
          <h2 className="text-2xl font-semibold">
            Correlation &mdash; Connecting the Pillars
          </h2>
          <p className="mt-4 text-muted-foreground leading-relaxed">
            The real value of observability is connecting the pillars. Structured
            logging injects the OpenTelemetry traceID into every log line.
            Grafana&rsquo;s derived fields on the Loki datasource turn those
            traceIDs into clickable Jaeger links. When an alert fires, the
            investigation path is: metric spike &rarr; filtered logs &rarr;
            distributed trace &rarr; root cause.
          </p>
          <div className="mt-6 rounded-xl border border-foreground/10 bg-card p-6">
            <MermaidDiagram chart={correlationDiagram} />
          </div>
        </div>
      </section>

      {/* What's next */}
      <section className="mt-12">
        <h2 className="text-2xl font-semibold">What&rsquo;s Next</h2>
        <p className="mt-3 text-muted-foreground leading-relaxed">
          Production maturity is a continuous process. Here&rsquo;s what&rsquo;s
          on the roadmap, pulled from the &ldquo;Remaining gaps&rdquo; sections
          of the recent ADRs.
        </p>
        <div className="mt-6">
          <GapsGrid gaps={GAPS} />
        </div>
      </section>

      {/* Footer CTAs */}
      <section className="mt-16 flex flex-col items-center gap-3 sm:flex-row sm:justify-center">
        <Link
          href="https://grafana.kylebradshaw.dev/d/system-overview/system-overview?orgId=1&from=now-1h&to=now&timezone=browser"
          target="_blank"
          rel="noopener noreferrer"
          className="inline-block rounded-lg bg-primary px-6 py-3 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
        >
          View Live Grafana Dashboard &rarr;
        </Link>
        <Link
          href={ADR_DIRECTORY_URL}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-block rounded-lg border border-foreground/20 px-6 py-3 text-sm font-medium hover:bg-muted transition-colors"
        >
          View ADRs on GitHub &rarr;
        </Link>
      </section>
    </div>
  );
}
```

Note: the `IncidentCard` import is intentional — it's used transitively through `JourneyTimeline`, but importing it directly here keeps the type re-exports stable for any future per-card usage on this page. If `eslint-config-next` flags the unused import, remove the `IncidentCard` line and keep the type-only import: `import type { Incident } from "@/components/observability/IncidentCard";`.

- [ ] **Step 2: Run ESLint to detect any unused-import warnings**

Run: `cd frontend && npm run lint -- --max-warnings 0`
Expected: passes. If `IncidentCard` is reported as unused, change the import line to:

```tsx
import type { Incident } from "@/components/observability/IncidentCard";
```

(and keep `JourneyTimeline` / `LessonCallout` / `GapsGrid` imports as-is). Re-run lint.

- [ ] **Step 3: Type-check**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 4: Visually confirm the page renders**

Start the dev server (in a second terminal if needed):

```bash
cd frontend && npm run dev
```

Open `http://localhost:3000/observability`. Confirm:

- Hero shows the new lead sentence ("Three pillars. Sixteen alert rules...").
- "The Journey" section shows four cards with vertical date column on desktop, stacked on mobile (use browser devtools to test).
- Each pillar (Metrics / Logs / Traces / Alerting) has a "Lessons from production" callout below its body.
- "What's Next" section shows four gap cards.
- Footer shows two side-by-side CTAs.
- Both Mermaid diagrams (architecture + correlation) render with no regression.
- Each `Read ADR XX →` link opens the correct GitHub markdown file in a new tab.

Stop the dev server with Ctrl-C.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/app/observability/page.tsx
git -c commit.gpgsign=false commit -m "feat(observability): restructure page around production journey"
```

---

### Task 7: Final verification

**Files:** No file changes — verification only.

- [ ] **Step 1: Run the full frontend preflight**

```bash
make preflight-frontend
```

Expected output ends with all three sub-steps passing:
- `=== Frontend: lint ===` (eslint exits 0)
- `=== Frontend: type check ===` (tsc --noEmit exits 0)
- `=== Frontend: build ===` (Next.js build succeeds)

If any sub-step fails, fix it and re-run before proceeding. Lint/format errors are autonomous fixes; logic regressions stop and report to Kyle per the branch rules in `CLAUDE.md`.

- [ ] **Step 2: Verify all six commits on the branch**

```bash
git log --oneline main..HEAD
```

Expected (in order, oldest first):
```
docs(spec): observability journey page restructure
feat(observability): add ADR URL constants module
feat(observability): add IncidentCard component
feat(observability): add JourneyTimeline component
feat(observability): add LessonCallout component
feat(observability): add GapsGrid component
feat(observability): restructure page around production journey
```

(Reverse-order from `git log` — confirm all seven exist.)

- [ ] **Step 3: Push the branch and open the PR**

```bash
git push -u origin agent/feat-observability-journey-page
gh pr create --base qa --title "Restructure /observability around production journey" --body "$(cat <<'EOF'
## Summary
- Adds a four-incident journey timeline (ADRs 07-10) to `/observability` with before/after framing per card
- Adds inline "Lessons from production" callouts beneath each pillar (Metrics / Logs / Traces / Alerting), tying specific instrumentation to the incidents that drove it
- Adds a "What's Next" section pulled from the Remaining gaps in the source ADRs
- Adds a "View ADRs on GitHub" CTA alongside the existing Grafana button
- Refreshes the hero copy to lead with the journey arc

Spec: `docs/superpowers/specs/2026-04-27-observability-journey-design.md`
Plan: `docs/superpowers/plans/2026-04-27-observability-journey.md`

No backend changes. Architecture and correlation Mermaid diagrams unchanged. Existing pillar copy and alert grid preserved.

## Test plan
- [ ] `/observability` renders without console errors
- [ ] Journey timeline shows four cards in chronological order with correct dates
- [ ] Each "Read ADR XX →" link opens the correct GitHub markdown file
- [ ] All four pillars show a "Lessons from production" callout
- [ ] "What's Next" grid shows four gap cards
- [ ] CI passes (lint, tsc, Next.js build)
EOF
)"
```

Notify Kyle the PR is open. Do not watch CI.

---

## Self-Review

### Spec coverage check

| Spec section | Plan task |
| --- | --- |
| Hero refresh | Task 6 |
| Journey timeline component (`JourneyTimeline`) | Task 3 |
| `IncidentCard` component | Task 2 |
| `LessonCallout` component | Task 4 |
| `GapsGrid` component | Task 5 |
| ADR URL centralisation (`adrs.ts`) | Task 1 |
| Page restructure with new ordering | Task 6 |
| All four lesson callouts beneath pillars | Task 6 |
| "What's Next" section | Task 6 |
| "View ADRs on GitHub" CTA | Task 6 |
| Architecture / correlation diagrams unchanged | Task 6 (preserved verbatim in code block) |
| `make preflight-frontend` passes | Task 7 |
| Mobile-friendly stacking | Task 2/3/5 (Tailwind `sm:` breakpoint usage) |

No gaps detected.

### Placeholder scan

No "TBD", "TODO", "implement later", "fill in details", or vague "add appropriate error handling" language found. Each step contains the exact code, exact command, or exact expected output an engineer needs.

### Type consistency check

- `Incident` type defined in Task 2, imported and reused in Task 3 and Task 6.
- `Gap` type defined in Task 5, imported and reused in Task 6.
- `AdrId` type defined in Task 1, imported in Task 2 (`IncidentCard`) and Task 4 (`LessonCallout`).
- `IncidentAccent` defined in Task 2, used only within Task 2.
- `ADR_DIRECTORY_URL` exported in Task 1, imported in Task 6.
- `adrUrl` / `adrLabel` exported in Task 1, used in Tasks 2 and 4.

All type and identifier names consistent across tasks. No drift.
