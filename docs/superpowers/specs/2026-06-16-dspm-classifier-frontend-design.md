# DSPM Classifier — Frontend Learning Page Design

**Date:** 2026-06-16
**Status:** Approved (brainstorming phase)
**Author:** Kyle Bradshaw (with Claude)
**Service spec:** [`2026-06-16-dspm-classifier-design.md`](./2026-06-16-dspm-classifier-design.md)
**Plan 1 (engine):** [`../plans/2026-06-16-dspm-classifier-plan-1-engine.md`](../plans/2026-06-16-dspm-classifier-plan-1-engine.md)
**Purpose:** A static, observability-style learning page at `/dspm` that
documents the DSPM classifier service so Kyle can study the workflow during
interviews. Covers the full Plans 1–3 vision; sections gated with build-state
badges so the page is honest about what's shipped vs. designed.

---

## 1. Goals & non-goals

### Goals

- One scrollable page Kyle can read end-to-end to understand the service:
  what it does, how data flows, how it fails safely, how it deploys, and what
  it exposes for ops.
- Visual aids first. Architecture, classification pipeline, retry/DLQ, and
  K8s topology are diagrams, not paragraphs.
- Mirror the established teaching-page pattern (matches `/observability`'s
  rhythm of prose → diagram → small custom component → callout).
- Stay accurate as the service grows: per-section `BuildStateBadge` chips
  show whether the content reflects shipped code or designed-but-unbuilt work.

### Non-goals

- No live data. The page does not call any service API.
- No animated/interactive pipeline. Static diagrams + static CLI output block.
- No site-nav rework. One home-page tile + a new route is the only surface
  change outside `frontend/src/app/dspm/` and `frontend/src/components/dspm/`.

---

## 2. Route & file structure

```
frontend/src/
├── app/
│   ├── page.tsx                       # add tile linking to /dspm
│   └── dspm/
│       └── page.tsx                   # the learning page (server component)
├── components/dspm/
│   ├── PipelineStageCard.tsx          # per-tier card (regex / NER / LLM)
│   ├── FailurePolicyTable.tsx         # spec §5 matrix
│   ├── MetricsGrid.tsx                # metrics catalog cards
│   └── BuildStateBadge.tsx            # "Plan N — status" chip
└── lib/dspm/
    └── content.ts                     # diagram strings + structured arrays
```

Layout pattern matches `/observability`:
- Diagrams live as Mermaid strings inside `lib/dspm/content.ts`.
- `page.tsx` is layout + composition only. No inline content.
- Each custom component owns a single visual unit and renders pure props.

---

## 3. Page sections (in render order)

Each section's `BuildStateBadge` shows status: **Plan 1 — in progress**
flips to **shipped** when the engine merges; Plans 2 and 3 stay **designed**
until their work lands.

### 3.1 Hero / framing

- Eyebrow: `DSPM CLASSIFIER · SENIOR PYTHON / VARONIS DAC`.
- H1: "Sensitive-data classification at Kafka scale".
- Two short paragraphs: what DSPM is, what the service exercises (async
  Python, Kafka, K8s, AWS-compatible storage, Gen-AI). Mirrors §1 of the
  service spec.
- Row of three `BuildStateBadge`s (Plan 1 / 2 / 3) so the reader sees overall
  progress at a glance.

### 3.2 End-to-end architecture

`MermaidDiagram` with a `flowchart TD` adapted from the spec's §2 ASCII
diagram. Subgraphs:

- **Ingest** — S3 (MinIO local / AWS S3 deployed), `data-events` Kafka topic
  keyed by `tenant_id`.
- **Process** — worker pool: S3 fetch → idempotency check → pipeline.
- **Persist** — Postgres `findings` table, `findings` Kafka topic emit.
- **Read** — `api` FastAPI deployment over the findings index.
- **Failure paths** — `data-events.retry` and `data-events.dlq` branches.

Caption (≤80 words) below the diagram explains:
- Partition key = `tenant_id`: per-tenant ordering and isolation.
- At-least-once on the wire + idempotency table = effectively-once end-to-end.
- Two K8s deployments, one image, so reads and writes scale independently.

### 3.3 Classification pipeline deep-dive

- `MermaidDiagram` `flowchart LR`: regex pass → NER pass → LLM pass, with
  escalation arrows annotated by the confidence threshold and a final
  "merge + dedupe spans" step that feeds the `Finding` writer.
- Three `PipelineStageCard`s side-by-side on md+, stacked on mobile.
- Static **CLI output block** below the cards: a styled `<pre>` showing
  expected output of `python scripts/classify_one.py s3://acme-uploads/contracts/q2.pdf`
  against a sample text. Grounds the abstract pipeline in a runnable example
  using the Plan-1 CLI driver. Example shape (real text populated from spec
  examples):

  ```
  $ python scripts/classify_one.py s3://acme-uploads/contracts/q2.pdf
  [regex]   matched 1 (FINANCIAL.CREDIT_CARD)
  [ner]     matched 3 (PII.PERSON, PII.EMAIL, PII.PHONE)  conf=0.92
  [llm]     skipped (NER confidence above threshold)
  ─────────────────────────────────────────────
  finding   tenant=acme-corp  event=evt_01HZ...  sensitivity=HIGH
            categories=[PII, FINANCIAL]  match_count=4
            pipeline_version=1  llm_failed=false
  ```

### 3.4 Reliability story

- `FailurePolicyTable` rendering the spec's §5 matrix verbatim: each row is
  `(failure, classification, action)` with the classification cell rendered
  as a colored chip (expected / permanent / transient / unexpected).
- `MermaidDiagram` `sequenceDiagram` of the retry path:
  `worker → data-events.retry (attempt=N, sleep=backoff(N)) → worker → … → DLQ at MAX_ATTEMPTS`.
- Two callout cards (same border/typography as `LessonCallout`):
  - **Idempotency** — `processed_messages(event_id PK, …, pipeline_version)`
    inserted in the same transaction as the findings upsert. Duplicate event
    → PK violation → transaction rolls back → offset still commits. Bumping
    `pipeline_version` lets a re-classification pass intentionally reprocess.
  - **Backpressure** — `BoundedWorkPool` owns `asyncio.Semaphore(N)` plus the
    set of paused `TopicPartition`s. Pauses at high-water, resumes at
    low-water, so a slow LLM doesn't unbounded-queue messages. The consumer
    loop just asks "can I dispatch?" — it doesn't know how backpressure works.

### 3.5 K8s + AWS deployment topology

- `MermaidDiagram` `flowchart` showing: one Docker image → two Deployments
  (`worker` replicas ≤ partitions, `api` replicas=2) → Service (api only) →
  ConfigMap, sealed Secret, migrations Job, ServiceMonitor, HPA on consumer
  lag.
- Short paragraph (≤120 words) on AWS positioning: `aioboto3` with
  `S3_ENDPOINT_URL` swap targets MinIO locally and real AWS S3 in deploy,
  same SDK, same IAM patterns, zero code change.
- `BuildStateBadge` on this section: **Plan 3 — designed**.

### 3.6 Observability & metrics catalog

`MetricsGrid` rendering one card per Prometheus metric from spec §5:

| Metric (mono) | Labels | Meaning | Alert intent |
|---|---|---|---|
| `findings_total` | `tenant_id`, `sensitivity` | classifications produced | volume sanity |
| `pipeline_stage_latency_seconds` | `stage` | per-tier latency histogram | LLM-tier slow path |
| `kafka_consumer_lag_seconds` | `topic`, `partition` | end-to-end backlog | HPA + paging |
| `in_flight_work` | – | concurrent classifications | backpressure visibility |
| `partitions_paused` | – | paused partition count | sustained pause = page |
| `dlq_total` | `reason` | permanent failures | alertable |
| `duplicate_total` | – | idempotency-rejected events | replay health |

Each card: monospace metric name, label chips, one-sentence meaning,
one-sentence alert intent. Same border style as the `GapsGrid` pattern.

### 3.7 Interview talking points

Bulleted list mirroring spec §9. Each bullet prefixed with a topic tag:

- `[Async + perf]` …
- `[Kafka at scale]` …
- `[K8s/microservices]` …
- `[AWS]` …
- `[Gen-AI]` …
- `[DSPM domain]` …
- `[Production rigor]` …

Same visual treatment as the `reliabilityPractices` list on `/async`.

### 3.8 Related artifacts

Bottom button row (same pattern as `/async` and `/observability`):

- Spec doc (`docs/superpowers/specs/2026-06-16-dspm-classifier-design.md`).
- Plan 1 (engine) — current.
- Plan 2 (worker) / Plan 3 (api) — rendered as disabled buttons until
  written.
- Code: link to GitHub path `services/dspm-classifier/`.

---

## 4. Components — props & behavior

All components are server-renderable (no client state) and take plain data.
The only client component on the page is the existing `MermaidDiagram`.

### `BuildStateBadge`

```ts
type BuildState = "in-progress" | "designed" | "shipped";
interface BuildStateBadgeProps {
  plan: 1 | 2 | 3;
  status: BuildState;
  label?: string; // optional override, e.g. "Plan 1 — engine"
}
```

- Single chip: dot + text. Colors: `shipped` green, `in-progress` amber,
  `designed` muted.
- Renders to `<span>` so it can sit inline in section headers.

### `PipelineStageCard`

```ts
interface PipelineStageCardProps {
  name: "Regex" | "NER" | "LLM";
  tagline: string;            // 1 sentence
  detects: string[];          // bullet list, e.g. ["SSN", "Credit card (Luhn)", …]
  cost: "cheap" | "moderate" | "expensive";
  escalatesWhen: string;      // e.g. "Always passes through to NER (never escalates further)"
  example: { input: string; matches: string[] };
}
```

- Bordered card with the same rounded/border treatment as `/observability`'s
  cards.
- `cost` chip color: cheap=green, moderate=amber, expensive=red.
- 3-column CSS grid on md+, stacked on mobile.

### `FailurePolicyTable`

```ts
type FailureClass = "expected" | "permanent" | "transient" | "unexpected";
interface FailureRow {
  failure: string;
  classification: FailureClass;
  action: string;
}
interface FailurePolicyTableProps {
  rows: FailureRow[];
}
```

- Real semantic `<table>` for screen-reader readability.
- `classification` rendered as a colored chip per class.
- Rows sourced from `content.ts`, mirroring spec §5 verbatim.

### `MetricsGrid`

```ts
interface MetricCardData {
  name: string;
  labels: string[];
  meaning: string;
  alertIntent: string;
}
interface MetricsGridProps {
  metrics: MetricCardData[];
}
```

- Card grid (2 cols md, 3 cols lg).
- Metric `name` rendered in `<code>` with mono font.
- Label chips beneath the name. Two short paragraphs for meaning + alert
  intent.

---

## 5. Content source (`lib/dspm/content.ts`)

Exports:

- `architectureDiagram: string` — Mermaid `flowchart TD` source.
- `pipelineDiagram: string` — Mermaid `flowchart LR` source.
- `retryDlqDiagram: string` — Mermaid `sequenceDiagram` source.
- `k8sTopologyDiagram: string` — Mermaid `flowchart` source.
- `pipelineStages: PipelineStageCardProps[]` — three entries.
- `failurePolicy: FailureRow[]` — rows from spec §5.
- `metrics: MetricCardData[]` — entries from spec §5 observability table.
- `cliOutputExample: string` — the static `<pre>` payload (see §3.3).
- `interviewBullets: { tag: string; body: string }[]` — entries from §9.
- `planStatuses: { plan: 1|2|3; status: BuildState; label: string }[]`.

`planStatuses` is the single source of truth for badges across the page. To
"flip" a section to shipped, edit one entry.

---

## 6. Styling & accessibility

- Reuses existing Tailwind tokens (`border-foreground/10`,
  `text-muted-foreground`, `rounded-lg`, etc.) — no new design tokens.
- Section width: `mx-auto max-w-4xl` matching `/async` and `/observability`.
- Section rhythm: `mt-12` between top-level sections, `mt-5` between heading
  and content, mirroring `/observability`'s spacing.
- Tables and grids use semantic markup. Colored chips include the class name
  in text (not color-only).
- All Mermaid diagrams render via the existing `<MermaidDiagram>` client
  component, which already handles theme + a11y.

---

## 7. Testing

### Unit / component tests

Not strictly required — the page is presentational. If we add anything, it's:

- `BuildStateBadge` snapshot per state.
- `FailurePolicyTable` renders a row per input.
- `MetricsGrid` renders a card per metric.

These are nice-to-have, not gating. Skip if they slow the work down.

### Playwright smoke

Add one smoke test to the existing `playwright.smoke.config.ts` suite:

- Navigate to `/dspm`.
- Assert the H1 text is visible.
- Assert at least one `[data-testid="mermaid-svg"]` (or the equivalent the
  Mermaid component renders) is present after hydration.
- Assert the "Spec" link in the bottom nav points to the correct GitHub URL.

Targets the page's existence and that diagrams hydrate; doesn't fight Mermaid
internals.

### CI hooks

- `tsc` passes (component types + content.ts exports).
- `eslint` passes per repo config.
- No new env vars; no new server actions; no new test infra.

---

## 8. Out of scope (deferred)

- Animated/interactive pipeline traversal — could be added later as a
  client-only component without disturbing the page structure.
- Live metric previews from a real Prometheus — would require an API route
  and a client component, and risks tying the page to a running service.
- Cross-linking from the service code's `README.md` back to this page — fine
  to add when convenient but not part of this work.
- Plan 2 and Plan 3 deep-dive subpages. The current design intentionally
  keeps everything on one scroll.

---

## 9. Acceptance criteria

- `frontend/src/app/dspm/page.tsx` exists; visiting `/dspm` renders all eight
  sections.
- `BuildStateBadge` row in the hero reflects current plan statuses from
  `content.ts`.
- Architecture, pipeline, retry/DLQ, and K8s topology diagrams all render as
  SVG (verified in Playwright smoke).
- `FailurePolicyTable`, `MetricsGrid`, and three `PipelineStageCard`s render
  the data from `content.ts`.
- Static CLI output block is present below the pipeline cards.
- Home page (`app/page.tsx`) shows a new tile linking to `/dspm`.
- `npm run lint` and `tsc --noEmit` (via existing make targets) pass.
- Playwright smoke for `/dspm` passes locally.
