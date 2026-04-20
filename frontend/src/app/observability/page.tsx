import { MermaidDiagram } from "@/components/MermaidDiagram";
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
    description: "GPU exporter health, AI service readiness, GPU temperature and VRAM usage",
    count: 4,
  },
  {
    name: "Kubernetes Health",
    color: "border-red-500/50",
    description: "OOM kills, pod restart storms, container memory pressure, node disk pressure, stuck deployments",
    count: 6,
  },
  {
    name: "Application SLOs",
    color: "border-red-500/50",
    description: "HTTP error rate and p95 latency targets for Go AI, Go ecommerce, and Java gateway services",
    count: 6,
  },
  {
    name: "Streaming Analytics",
    color: "border-red-500/50",
    description: "Kafka consumer lag monitoring across order, cart, and product view event topics",
    count: 1,
  },
];

export default function ObservabilityPage() {
  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <h1 className="mt-8 text-3xl font-bold">Observability</h1>

      {/* Intro */}
      <section className="mt-8">
        <p className="text-muted-foreground leading-relaxed">
          Production monitoring across three pillars &mdash; metrics, logs, and
          traces &mdash; with automated alerting and cross-pillar correlation.
          Every service in this portfolio is instrumented, and a single traceID
          connects a Grafana metric spike to the relevant log lines to the full
          distributed trace in Jaeger.
        </p>
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

      {/* Footer CTA */}
      <section className="mt-16 text-center">
        <Link
          href="https://grafana.kylebradshaw.dev/d/system-overview/system-overview?orgId=1&from=now-1h&to=now&timezone=browser"
          target="_blank"
          rel="noopener noreferrer"
          className="inline-block rounded-lg bg-primary px-6 py-3 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
        >
          View Live Grafana Dashboard &rarr;
        </Link>
      </section>
    </div>
  );
}
