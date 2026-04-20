# Observability Frontend Page Design

## Context

The portfolio just shipped a full observability stack: Loki + Promtail for log aggregation, 16 Grafana alert rules across 4 groups, application SLOs, Kafka consumer lag monitoring, traceID log injection, and a correlation dashboard. None of this is visible in the frontend. The portfolio already has dedicated pages for AWS, CI/CD, and Security — observability deserves the same treatment.

**Goal:** Add a new top-level `/observability` page that showcases the three-pillar observability architecture for hiring managers. Narrative + diagrams, no data fetching.

---

## Page Structure

### 1. Hero
- Title: "Observability"
- One paragraph: production monitoring across three pillars (metrics, logs, traces) with automated alerting and cross-pillar correlation. Every service in this portfolio is instrumented.

### 2. Architecture Diagram (Mermaid)
Flowchart showing the full data flow:
- **Top:** Application services (Go, Java, Python) emit metrics, logs, and traces
- **Middle:** Collection layer — Prometheus scrapes metrics, Promtail ships logs, OTel SDK exports traces
- **Bottom:** Backend storage — Prometheus TSDB, Loki, Jaeger
- **Output:** Grafana (dashboards + alerting) → Telegram notifications

### 3. Metrics Section (Prometheus)
- Color accent: orange (border-left)
- Pull-based model, 15s scrape interval
- Infrastructure: kube-state-metrics, node-exporter, GPU exporter
- Application: pod annotations for auto-discovery
- Example metrics shown as inline code badges: `http_requests_total`, `kafka_consumer_lag`, `container_memory_working_set_bytes`

### 4. Logs Section (Loki + Promtail)
- Color accent: green
- Promtail DaemonSet tails all container logs
- Structured JSON logging (Go `slog`, Java `logstash-logback-encoder`)
- traceID extraction for log-to-trace correlation
- Example LogQL in a code block: `{namespace="go-ecommerce"} | json | level="error"`

### 5. Traces Section (Jaeger + OpenTelemetry)
- Color accent: purple
- OTel SDK with `otelgin` middleware for HTTP
- W3C traceparent propagation across HTTP boundaries
- Kafka message header trace propagation (`InjectKafka`/`ExtractKafka`)
- A request traces from HTTP → ecommerce → Kafka → analytics-service

### 6. Alerting Section
- Color accent: red
- 16 rules across 4 groups, displayed as a 2x2 card grid:
  - **Infrastructure:** GPU exporter, AI service health, GPU temp/VRAM
  - **Kubernetes Health:** OOM kills, restart storms, memory/disk pressure, stuck deployments
  - **Application SLOs:** Error rate + p95 latency (Go AI 5%/30s, Go ecommerce 2%/2s, Java gateway 2%/3s)
  - **Streaming Analytics:** Kafka consumer lag > 1000
- All route to Telegram via Grafana unified alerting

### 7. Correlation Section
- Color accent: yellow
- Mermaid sequence diagram showing the investigation workflow:
  1. Telegram alert fires
  2. Open Grafana metrics dashboard, see the spike
  3. Scroll to logs, filter by error level
  4. Find traceID in log line, click it
  5. Jaeger trace shows the full request path → root cause
- Explain: structured logging injects traceID → Promtail extracts it → Grafana derived fields create clickable Jaeger links

### 8. Footer CTA
- "View Live Grafana Dashboard →" button linking to `https://grafana.kylebradshaw.dev`

---

## Navigation

Add "Observability" to `SiteHeader.tsx` nav links, positioned between "CI/CD" and "Security".

---

## Files

- **Create:** `frontend/src/app/observability/page.tsx` — the full page
- **Modify:** `frontend/src/components/SiteHeader.tsx` — add nav link

---

## Patterns

- Layout: `<div className="mx-auto max-w-3xl px-6 py-12">` (matches all other pages)
- Diagrams: `<MermaidDiagram chart={...} />` component
- Cards: shadcn/ui `Card` components for the alerting grid
- Static content only — no API calls, no client components needed
- Section borders color-coded: orange (metrics), green (logs), purple (traces), red (alerting), yellow (correlation)

---

## Verification

- `npm run build` passes
- Page renders at `localhost:3000/observability`
- Nav link appears between CI/CD and Security, highlights correctly on active
- Both Mermaid diagrams render (architecture + correlation sequence)
- Grafana link opens correctly
- Page follows same visual style as `/cicd` and `/security`
