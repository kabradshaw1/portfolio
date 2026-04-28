import { MermaidDiagram } from "@/components/MermaidDiagram";
import { GapsGrid, type Gap } from "@/components/observability/GapsGrid";
import { type Incident } from "@/components/observability/IncidentCard";
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
  {
    name: "PostgreSQL",
    color: "border-red-500/50",
    description:
      "Connection saturation, replication lag, deadlocks, backup freshness, and query-level latency, regression, slow-query rate, and auto_explain stalled signals",
    count: 8,
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
      'Middleware now logs every 5xx `AppError` via `slog.Error` with code, message, status, and request ID before responding — silent server errors are no longer possible. QA runs on a dedicated RabbitMQ `/qa` vhost, fully isolating saga flow. A `saga-order-stalled` Grafana alert fires when `saga_steps_total{step="PAYMENT_CREATED"}` increases but neither `COMPLETED` nor `COMPENSATION_COMPLETE` does within 30 minutes.',
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
      'Six-layer structured logging covers HTTP handler, agent loop, LLM clients, cache, guardrails, and tools. All agent-loop logs use `slog.InfoContext(ctx, ...)` so `tracing.NewLogHandler()` injects the OTel traceID into every record. OpenAI and Anthropic clients now emit `openai.chat` / `anthropic.chat` spans with token attributes — provider comparison is visible in Jaeger. A single Loki query (`{app="ai-service"} | json | traceID="..."`) shows the complete request lifecycle.',
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
      'Every CI rollout posts a Grafana annotation tagged with namespace + short SHA via `/api/annotations`, with anonymous-Viewer auth preserved for public dashboard viewing. `kubernetes-event-exporter` (resmoio fork) ships Warning-only events into Loki under `{job="kube-event-exporter"}` with namespace/reason/kind/name labels. Every Go service\'s `DATABASE_URL` includes `application_name=<service-name>`, so a "Connections by Service" dashboard panel attributes every Postgres connection in seconds.',
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
    title: "In-app PostgreSQL query tracing",
    description:
      "Manual OpenTelemetry spans around slow queries — partly displaced by the pg_stat_statements + auto_explain layer below. Still useful for tying a slow query span to the surrounding business operation in Jaeger; deferred until the database-side data exposes a query that warrants per-call attribution.",
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

      {/* Event Sourcing & CQRS */}
      <section className="mt-12">
        <div className="border-l-4 border-cyan-500/60 pl-4">
          <h2 className="text-2xl font-semibold">
            Event Sourcing &amp; CQRS &mdash; Order Projector
          </h2>
          <p className="mt-4 text-muted-foreground leading-relaxed">
            The same{" "}
            <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
              ecommerce.orders
            </code>{" "}
            Kafka topic that drives saga state also feeds an{" "}
            <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
              order-projector
            </code>{" "}
            consumer, which writes a denormalized read model into{" "}
            <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
              projectordb
            </code>
            . Reads against the projection are independent of the OLTP write
            path —{" "}
            <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
              order-service
            </code>{" "}
            owns the write schema, the projector owns the read schema, and the
            two evolve on different timelines.
          </p>
          <p className="mt-4 text-muted-foreground leading-relaxed">
            The pay-off is operational: dashboard and reporting reads on the
            projection don&apos;t compete with checkout writes for primary-pool
            connections, and the projection&apos;s shape can be tuned to query
            patterns instead of transactional invariants. Trace context arrives
            on Kafka headers, so a single order&apos;s lifecycle (HTTP →{" "}
            <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
              order-service
            </code>{" "}
            → Kafka → projector) renders as one trace in Jaeger. The projector
            reuses the same metric vocabulary as the analytics consumer (
            <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
              kafka_consumer_lag
            </code>
            , aggregation latency), so a single dashboard panel covers both
            consumers.
          </p>
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

      {/* Database Query Performance */}
      <section className="mt-12">
        <div className="border-l-4 border-blue-500/60 pl-4">
          <h2 className="text-2xl font-semibold">
            Database Query Performance &mdash; pg_stat_statements + auto_explain
          </h2>
          <p className="mt-4 text-muted-foreground leading-relaxed">
            System-level Postgres metrics (connections, cache hit, deadlocks,
            backup freshness) tell you the database is alive. They don&rsquo;t
            tell you which queries are slow, drifting, or eating CPU. The
            shared Postgres 17 instance now preloads{" "}
            <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
              pg_stat_statements
            </code>{" "}
            and{" "}
            <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
              auto_explain
            </code>
            , exposing per-query latency, call rate, IO behavior, and full
            execution plans for anything over 500&nbsp;ms.
          </p>
          <p className="mt-4 text-muted-foreground leading-relaxed">
            Three independent paths feed Grafana. The{" "}
            <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
              postgres_exporter
            </code>{" "}
            sidecar runs custom queries that export the top-50 statements as
            time-series metrics &mdash;{" "}
            <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
              pg_stat_statements_mean_exec_time
            </code>
            ,{" "}
            <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
              pg_stat_statements_calls_total
            </code>
            ,{" "}
            <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
              pg_stat_statements_shared_blks_read
            </code>
            . A read-only{" "}
            <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
              grafana_reader
            </code>{" "}
            role with the{" "}
            <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
              pg_monitor
            </code>{" "}
            predefined role powers per-database PostgreSQL data sources for
            live SQL inspection. And{" "}
            <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
              auto_explain.log_format = json
            </code>{" "}
            writes plans to Postgres logs &mdash; Promtail extracts them into
            Loki so plans render inline in a Grafana logs panel filtered by
            queryid.
          </p>
          <div className="mt-4 rounded-lg border border-foreground/10 bg-muted/50 p-3">
            <code className="text-xs text-blue-400">
              {'{namespace="java-tasks", app="postgres"} |= "auto_explain" | json | query_id=~"$queryid"'}
            </code>
          </div>
          <p className="mt-4 text-muted-foreground leading-relaxed">
            A new{" "}
            <strong className="text-foreground">
              PostgreSQL Query Performance
            </strong>{" "}
            dashboard ties it together: top-N tables by mean and total exec
            time, p95 latency per queryid, slow-query call rate, cache hit
            ratio, and a plan-viewer panel driven by a queryid template
            variable. Four new alerts cover the realistic failure modes &mdash;
            a hard <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">&gt;1&nbsp;s</code>{" "}
            ceiling, a regression rule that fires when current mean is
            <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">&gt;2&times;</code>{" "}
            its 7-day baseline, a slow-query rate spike rule, and an{" "}
            <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">
              auto_explain
            </code>{" "}
            stalled rule that catches misconfigurations before they hide a
            regression.
          </p>
          <LessonCallout adrIds={["pg-query"]}>
            Hard latency thresholds miss the realistic failure mode &mdash; a
            query that quietly drifts from 50&nbsp;ms to 200&nbsp;ms after a
            planner change. The 7-day baseline regression alert catches that;
            the hard ceiling catches genuinely terrible queries. Together they
            give the database a working measurement layer for the rest of the
            <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">
              db-roadmap
            </code>{" "}
            (replication, retention, vacuum tuning, partitioning) to build on.
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
