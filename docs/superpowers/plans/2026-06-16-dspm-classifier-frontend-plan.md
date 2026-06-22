# DSPM Classifier Frontend Learning Page — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Spec:** [`../specs/2026-06-16-dspm-classifier-frontend-design.md`](../specs/2026-06-16-dspm-classifier-frontend-design.md)

**Goal:** Build a static `/dspm` learning page in the Next.js frontend that documents the DSPM classifier service end-to-end with diagrams, per-tier pipeline cards, a failure-policy table, a metrics catalog, a CLI-output preview, and per-section build-state badges.

**Architecture:** A new App Router server component at `frontend/src/app/dspm/page.tsx` composes four new server components from `frontend/src/components/dspm/` (`BuildStateBadge`, `PipelineStageCard`, `FailurePolicyTable`, `MetricsGrid`) plus the existing `MermaidDiagram` client component. All page text, Mermaid diagram source strings, and structured prop data live in a single `frontend/src/lib/dspm/content.ts` module to keep `page.tsx` layout-only. The home page (`app/page.tsx`) gains one tile that links to `/dspm`. A single Playwright smoke test verifies the page renders.

**Tech Stack:** Next.js 16 (App Router, React 19, server components by default), TypeScript 5 strict, Tailwind v4, existing `MermaidDiagram` (uses `mermaid` 10 + `dompurify`), shadcn `Card` primitives where applicable, Playwright for smoke testing.

## Global Constraints

- All new components are **server components by default**. Only re-use existing client components (`MermaidDiagram`) for diagrams.
- All page text, diagram strings, and structured arrays go in `frontend/src/lib/dspm/content.ts`. `page.tsx` contains layout and composition only.
- Reuse existing Tailwind tokens: `border-foreground/10`, `text-muted-foreground`, `bg-card`, `bg-muted/30`, `rounded-lg`. No new design tokens.
- Section width matches `/observability`: `mx-auto max-w-4xl px-6 py-12`.
- Section rhythm matches `/observability`: `mt-12` between sections.
- Colored chips must include the class name in text, not color-only (a11y).
- TypeScript: strict. No `any`. Props interfaces explicit at each component file.
- All commits use the conventional-commit prefix `feat(frontend):` or `test(frontend):` and end with the standard Claude co-author trailer.
- No unit-test framework exists in `frontend/` (only Playwright). Component correctness is verified by `tsc --noEmit`, `eslint`, and the integration smoke test at the end. Don't introduce Vitest/Jest — out of scope.
- Per `CLAUDE.md` memory: don't push doc-only commits. This plan ships code, so push at the end.

---

## File Structure

**Created:**
- `frontend/src/app/dspm/page.tsx` — the learning page (server component)
- `frontend/src/components/dspm/BuildStateBadge.tsx`
- `frontend/src/components/dspm/PipelineStageCard.tsx`
- `frontend/src/components/dspm/FailurePolicyTable.tsx`
- `frontend/src/components/dspm/MetricsGrid.tsx`
- `frontend/src/lib/dspm/content.ts` — single source of all page data

**Modified:**
- `frontend/src/app/page.tsx` — add a tile linking to `/dspm`
- `frontend/e2e/smoke-prod/smoke.spec.ts` — add one navigation test for `/dspm` (matches existing pattern)

---

## Task 1: `BuildStateBadge` component

**Files:**
- Create: `frontend/src/components/dspm/BuildStateBadge.tsx`

**Interfaces:**
- Consumes: nothing
- Produces:
  ```ts
  export type BuildState = "in-progress" | "designed" | "shipped";
  export interface BuildStateBadgeProps {
    plan: 1 | 2 | 3;
    status: BuildState;
    label?: string;
  }
  export function BuildStateBadge(props: BuildStateBadgeProps): JSX.Element;
  ```

- [ ] **Step 1: Create the file**

Create `frontend/src/components/dspm/BuildStateBadge.tsx`:

```tsx
export type BuildState = "in-progress" | "designed" | "shipped";

export interface BuildStateBadgeProps {
  plan: 1 | 2 | 3;
  status: BuildState;
  label?: string;
}

const STATUS_LABEL: Record<BuildState, string> = {
  "in-progress": "in progress",
  designed: "designed",
  shipped: "shipped",
};

const STATUS_DOT: Record<BuildState, string> = {
  "in-progress": "bg-amber-500",
  designed: "bg-foreground/30",
  shipped: "bg-green-500",
};

export function BuildStateBadge({
  plan,
  status,
  label,
}: BuildStateBadgeProps) {
  const display = label ?? `Plan ${plan} — ${STATUS_LABEL[status]}`;
  return (
    <span
      className="inline-flex items-center gap-2 rounded-full border border-foreground/10 bg-muted/30 px-3 py-1 text-xs font-medium text-muted-foreground"
      data-testid="build-state-badge"
      data-status={status}
    >
      <span
        aria-hidden="true"
        className={`h-2 w-2 rounded-full ${STATUS_DOT[status]}`}
      />
      <span>{display}</span>
      <span className="sr-only"> ({STATUS_LABEL[status]})</span>
    </span>
  );
}
```

- [ ] **Step 2: Verify types compile**

Run from `frontend/`:

```bash
npx tsc --noEmit
```

Expected: no errors related to `BuildStateBadge`.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/dspm/BuildStateBadge.tsx
git commit -m "$(cat <<'EOF'
feat(frontend): add BuildStateBadge for DSPM page section status

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `PipelineStageCard` component

**Files:**
- Create: `frontend/src/components/dspm/PipelineStageCard.tsx`

**Interfaces:**
- Consumes: nothing
- Produces:
  ```ts
  export type PipelineCost = "cheap" | "moderate" | "expensive";
  export interface PipelineStageCardProps {
    name: "Regex" | "NER" | "LLM";
    tagline: string;
    detects: readonly string[];
    cost: PipelineCost;
    escalatesWhen: string;
    example: { input: string; matches: readonly string[] };
  }
  export function PipelineStageCard(props: PipelineStageCardProps): JSX.Element;
  ```

- [ ] **Step 1: Create the file**

Create `frontend/src/components/dspm/PipelineStageCard.tsx`:

```tsx
export type PipelineCost = "cheap" | "moderate" | "expensive";

export interface PipelineStageCardProps {
  name: "Regex" | "NER" | "LLM";
  tagline: string;
  detects: readonly string[];
  cost: PipelineCost;
  escalatesWhen: string;
  example: { input: string; matches: readonly string[] };
}

const COST_CLASS: Record<PipelineCost, string> = {
  cheap: "border-green-500/40 bg-green-500/10 text-green-300",
  moderate: "border-amber-500/40 bg-amber-500/10 text-amber-300",
  expensive: "border-red-500/40 bg-red-500/10 text-red-300",
};

const COST_LABEL: Record<PipelineCost, string> = {
  cheap: "cheap",
  moderate: "moderate",
  expensive: "expensive",
};

export function PipelineStageCard({
  name,
  tagline,
  detects,
  cost,
  escalatesWhen,
  example,
}: PipelineStageCardProps) {
  return (
    <article
      className="flex h-full flex-col rounded-lg border border-foreground/10 bg-card p-5"
      data-testid={`pipeline-stage-${name.toLowerCase()}`}
    >
      <header className="flex items-baseline justify-between gap-2">
        <h3 className="text-lg font-semibold">{name}</h3>
        <span
          className={`rounded-full border px-2 py-0.5 text-xs font-medium ${COST_CLASS[cost]}`}
        >
          {COST_LABEL[cost]}
        </span>
      </header>
      <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
        {tagline}
      </p>

      <div className="mt-4">
        <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          Detects
        </p>
        <ul className="mt-2 space-y-1 text-sm text-muted-foreground">
          {detects.map((d) => (
            <li key={d} className="border-l border-foreground/15 pl-3">
              {d}
            </li>
          ))}
        </ul>
      </div>

      <div className="mt-4">
        <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          Escalates when
        </p>
        <p className="mt-1 text-sm leading-relaxed text-muted-foreground">
          {escalatesWhen}
        </p>
      </div>

      <div className="mt-4 rounded-md border border-foreground/10 bg-muted/30 p-3">
        <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          Example
        </p>
        <p className="mt-1 font-mono text-xs leading-relaxed text-muted-foreground">
          {example.input}
        </p>
        <ul className="mt-2 flex flex-wrap gap-1.5">
          {example.matches.map((m) => (
            <li
              key={m}
              className="rounded bg-foreground/10 px-2 py-0.5 font-mono text-[11px] text-muted-foreground"
            >
              {m}
            </li>
          ))}
        </ul>
      </div>
    </article>
  );
}
```

- [ ] **Step 2: Verify types compile**

```bash
npx tsc --noEmit
```

Expected: no errors related to `PipelineStageCard`.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/dspm/PipelineStageCard.tsx
git commit -m "$(cat <<'EOF'
feat(frontend): add PipelineStageCard for DSPM classification tiers

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `FailurePolicyTable` component

**Files:**
- Create: `frontend/src/components/dspm/FailurePolicyTable.tsx`

**Interfaces:**
- Consumes: nothing
- Produces:
  ```ts
  export type FailureClass = "expected" | "permanent" | "transient" | "unexpected";
  export interface FailureRow {
    failure: string;
    classification: FailureClass;
    action: string;
  }
  export interface FailurePolicyTableProps { rows: readonly FailureRow[]; }
  export function FailurePolicyTable(props: FailurePolicyTableProps): JSX.Element;
  ```

- [ ] **Step 1: Create the file**

Create `frontend/src/components/dspm/FailurePolicyTable.tsx`:

```tsx
export type FailureClass =
  | "expected"
  | "permanent"
  | "transient"
  | "unexpected";

export interface FailureRow {
  failure: string;
  classification: FailureClass;
  action: string;
}

export interface FailurePolicyTableProps {
  rows: readonly FailureRow[];
}

const CLASS_STYLE: Record<FailureClass, string> = {
  expected: "border-foreground/20 bg-foreground/5 text-muted-foreground",
  permanent: "border-red-500/40 bg-red-500/10 text-red-300",
  transient: "border-amber-500/40 bg-amber-500/10 text-amber-300",
  unexpected: "border-purple-500/40 bg-purple-500/10 text-purple-300",
};

export function FailurePolicyTable({ rows }: FailurePolicyTableProps) {
  return (
    <div className="overflow-x-auto rounded-lg border border-foreground/10">
      <table className="w-full text-left text-sm" data-testid="failure-policy-table">
        <thead className="bg-muted/30">
          <tr className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            <th scope="col" className="px-4 py-3">
              Failure
            </th>
            <th scope="col" className="px-4 py-3">
              Class
            </th>
            <th scope="col" className="px-4 py-3">
              Action
            </th>
          </tr>
        </thead>
        <tbody className="divide-y divide-foreground/10">
          {rows.map((row) => (
            <tr key={row.failure} className="align-top">
              <td className="px-4 py-3 text-muted-foreground">{row.failure}</td>
              <td className="px-4 py-3">
                <span
                  className={`inline-block rounded-full border px-2 py-0.5 text-xs font-medium ${CLASS_STYLE[row.classification]}`}
                >
                  {row.classification}
                </span>
              </td>
              <td className="px-4 py-3 text-muted-foreground">{row.action}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
```

- [ ] **Step 2: Verify types compile**

```bash
npx tsc --noEmit
```

Expected: no errors related to `FailurePolicyTable`.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/dspm/FailurePolicyTable.tsx
git commit -m "$(cat <<'EOF'
feat(frontend): add FailurePolicyTable for DSPM reliability matrix

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: `MetricsGrid` component

**Files:**
- Create: `frontend/src/components/dspm/MetricsGrid.tsx`

**Interfaces:**
- Consumes: nothing
- Produces:
  ```ts
  export interface MetricCardData {
    name: string;
    labels: readonly string[];
    meaning: string;
    alertIntent: string;
  }
  export interface MetricsGridProps { metrics: readonly MetricCardData[]; }
  export function MetricsGrid(props: MetricsGridProps): JSX.Element;
  ```

- [ ] **Step 1: Create the file**

Create `frontend/src/components/dspm/MetricsGrid.tsx`:

```tsx
export interface MetricCardData {
  name: string;
  labels: readonly string[];
  meaning: string;
  alertIntent: string;
}

export interface MetricsGridProps {
  metrics: readonly MetricCardData[];
}

export function MetricsGrid({ metrics }: MetricsGridProps) {
  return (
    <div
      className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3"
      data-testid="metrics-grid"
    >
      {metrics.map((m) => (
        <article
          key={m.name}
          className="flex flex-col rounded-lg border border-foreground/10 bg-card p-4"
        >
          <header>
            <code className="block font-mono text-sm text-foreground">
              {m.name}
            </code>
            {m.labels.length > 0 && (
              <ul className="mt-2 flex flex-wrap gap-1.5">
                {m.labels.map((label) => (
                  <li
                    key={label}
                    className="rounded bg-foreground/10 px-2 py-0.5 font-mono text-[11px] text-muted-foreground"
                  >
                    {label}
                  </li>
                ))}
              </ul>
            )}
          </header>
          <p className="mt-3 text-xs leading-relaxed text-muted-foreground">
            <span className="font-semibold uppercase tracking-wide">
              Meaning ·{" "}
            </span>
            {m.meaning}
          </p>
          <p className="mt-2 text-xs leading-relaxed text-muted-foreground">
            <span className="font-semibold uppercase tracking-wide">
              Alert ·{" "}
            </span>
            {m.alertIntent}
          </p>
        </article>
      ))}
    </div>
  );
}
```

- [ ] **Step 2: Verify types compile**

```bash
npx tsc --noEmit
```

Expected: no errors related to `MetricsGrid`.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/dspm/MetricsGrid.tsx
git commit -m "$(cat <<'EOF'
feat(frontend): add MetricsGrid for DSPM observability catalog

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Content module (`lib/dspm/content.ts`)

**Files:**
- Create: `frontend/src/lib/dspm/content.ts`

**Interfaces:**
- Consumes: types from Tasks 1–4.
- Produces:
  ```ts
  export const planStatuses: readonly { plan: 1 | 2 | 3; status: BuildState; label: string }[];
  export const architectureDiagram: string;
  export const pipelineDiagram: string;
  export const retryDlqDiagram: string;
  export const k8sTopologyDiagram: string;
  export const pipelineStages: readonly PipelineStageCardProps[];
  export const failurePolicy: readonly FailureRow[];
  export const metrics: readonly MetricCardData[];
  export const cliOutputExample: string;
  export const interviewBullets: readonly { tag: string; body: string }[];
  export const sectionPlanStatus: Record<
    "architecture" | "pipeline" | "reliability" | "deployment" | "observability",
    { plan: 1 | 2 | 3; status: BuildState }
  >;
  ```

- [ ] **Step 1: Create the file**

Create `frontend/src/lib/dspm/content.ts`:

```ts
import type {
  BuildState,
} from "@/components/dspm/BuildStateBadge";
import type { FailureRow } from "@/components/dspm/FailurePolicyTable";
import type { MetricCardData } from "@/components/dspm/MetricsGrid";
import type { PipelineStageCardProps } from "@/components/dspm/PipelineStageCard";

export const planStatuses: readonly {
  plan: 1 | 2 | 3;
  status: BuildState;
  label: string;
}[] = [
  { plan: 1, status: "in-progress", label: "Plan 1 — engine (in progress)" },
  { plan: 2, status: "designed", label: "Plan 2 — Kafka worker (designed)" },
  { plan: 3, status: "designed", label: "Plan 3 — FastAPI read API (designed)" },
];

export const sectionPlanStatus: Record<
  "architecture" | "pipeline" | "reliability" | "deployment" | "observability",
  { plan: 1 | 2 | 3; status: BuildState }
> = {
  architecture: { plan: 2, status: "designed" },
  pipeline: { plan: 1, status: "in-progress" },
  reliability: { plan: 2, status: "designed" },
  deployment: { plan: 3, status: "designed" },
  observability: { plan: 2, status: "designed" },
};

export const architectureDiagram = `flowchart TD
  subgraph Ingest["Ingest"]
    direction LR
    S3["S3 / MinIO<br/><i>aioboto3 · S3_ENDPOINT_URL swap</i>"]
    EVENTS["Kafka topic: data-events<br/><i>partition key = tenant_id</i>"]
  end

  subgraph Process["Process (worker)"]
    direction TB
    CONS["Consumer pool<br/><i>aiokafka · manual commit</i>"]
    FETCH["S3 fetch<br/><i>cap = MAX_OBJECT_BYTES</i>"]
    IDEM["Idempotency check<br/><i>processed_messages PK</i>"]
    PIPE["Pipeline.run<br/><i>regex → NER → LLM</i>"]
  end

  subgraph Persist["Persist"]
    direction LR
    PG[("Postgres<br/><i>findings index</i>")]
    FOUT["Kafka topic: findings"]
  end

  subgraph Read["Read (api)"]
    direction LR
    API["FastAPI<br/><i>/findings · /tenants/:id · /healthz</i>"]
  end

  subgraph Fail["Failure paths"]
    direction LR
    RETRY["data-events.retry<br/><i>attempt + backoff</i>"]
    DLQ["data-events.dlq<br/><i>final · alertable</i>"]
  end

  S3 -->|ObjectCreated| EVENTS
  EVENTS --> CONS
  CONS --> FETCH --> IDEM --> PIPE
  PIPE -->|upsert| PG
  PIPE -->|emit| FOUT
  PG --> API
  PIPE -.transient.-> RETRY
  RETRY -.exhausted.-> DLQ
  PIPE -.permanent.-> DLQ`;

export const pipelineDiagram = `flowchart LR
  IN["Object content"] --> REGEX["Regex pass<br/><i>SSN, CC+Luhn, JWT, AWS</i>"]
  REGEX --> NER["NER pass<br/><i>Presidio · spaCy</i>"]
  NER -- "conf ≥ threshold" --> MERGE["Merge + dedupe spans"]
  NER -- "conf < threshold or silent" --> LLM["LLM pass<br/><i>structured JSON</i>"]
  LLM --> MERGE
  REGEX -.always passes through.-> NER
  MERGE --> OUT["Finding<br/><i>sensitivity · categories</i>"]`;

export const retryDlqDiagram = `sequenceDiagram
  participant W as Worker
  participant E as data-events
  participant R as data-events.retry
  participant D as data-events.dlq
  participant LLM as LLM tier

  E->>W: event_id=evt_X
  W->>LLM: classify
  LLM-->>W: 503 / timeout
  W->>R: envelope(attempt=1, last_error=timeout)
  Note over R: sleep ∝ attempt (1s · 5s · 30s · 2m · 10m)
  R->>W: redeliver (attempt=2)
  W->>LLM: classify
  LLM-->>W: 503
  W->>R: envelope(attempt=2)
  Note over W: ... up to MAX_ATTEMPTS=5
  W->>D: envelope(failure_reason, traceback)`;

export const k8sTopologyDiagram = `flowchart TD
  IMG["Docker image<br/><i>multi-stage · python:3.12-slim</i>"]
  subgraph Cluster["Kubernetes cluster"]
    direction TB
    WORKER["Deployment: worker<br/><i>replicas ≤ partitions</i>"]
    API["Deployment: api<br/><i>replicas = 2</i>"]
    SVC["Service (ClusterIP)<br/><i>api only</i>"]
    CM["ConfigMap<br/><i>brokers · topics · thresholds</i>"]
    SEC["Sealed Secret<br/><i>SASL · AWS · Postgres · LLM key</i>"]
    JOB["Job: alembic upgrade head"]
    SM["ServiceMonitor<br/><i>/metrics scrape</i>"]
    HPA["HPA<br/><i>consumer-lag via prometheus-adapter</i>"]
  end

  IMG --> WORKER
  IMG --> API
  API --> SVC
  CM --> WORKER
  CM --> API
  SEC --> WORKER
  SEC --> API
  JOB -->|pre-deploy| WORKER
  SM --> WORKER
  SM --> API
  HPA --> WORKER`;

export const pipelineStages: readonly PipelineStageCardProps[] = [
  {
    name: "Regex",
    tagline:
      "Deterministic, high-precision pass for known-shape secrets and identifiers.",
    detects: [
      "SSN (rejects 000/666/9xx and 00 group)",
      "Credit card (Luhn-validated)",
      "JWT (3-segment base64url)",
      "AWS access key (AKIA/ASIA prefix)",
      "Generic API key (assignment pattern)",
    ],
    cost: "cheap",
    escalatesWhen:
      "Never escalates further on its own — always passes through to NER so it can contribute entities to the merged span set.",
    example: {
      input: "Card 4111 1111 1111 1111, key AKIAIOSFODNN7EXAMPLE",
      matches: ["FINANCIAL.CREDIT_CARD", "SECRETS.AWS_KEY"],
    },
  },
  {
    name: "NER",
    tagline:
      "Presidio + spaCy named-entity recognition for PII the regex tier cannot model.",
    detects: [
      "PERSON, EMAIL, PHONE, LOCATION",
      "IP_ADDRESS, IBAN, US_SSN (cross-check)",
      "MEDICAL_LICENSE",
    ],
    cost: "moderate",
    escalatesWhen:
      "Any returned entity score falls below NER_CONFIDENCE_THRESHOLD (default 0.6), or the sensitive-document heuristic fires while regex/NER were silent.",
    example: {
      input: "Contact Alice at alice@example.com or 415-555-1212.",
      matches: ["PII.PERSON", "PII.EMAIL", "PII.PHONE"],
    },
  },
  {
    name: "LLM",
    tagline:
      "Tiered LLM pass invoked only on ambiguous content, with structured JSON output.",
    detects: [
      "Category (PII / FINANCIAL / HEALTH / SECRETS / NONE)",
      "Sensitivity (none / low / medium / high)",
      "Free-form entities + reasoning",
    ],
    cost: "expensive",
    escalatesWhen:
      "Final tier — never escalates further. On parse failure or 5xx after retries, pipeline persists the finding with llm_failed=true using regex + NER results only.",
    example: {
      input:
        '"Re: signed NDA — please share the new contract on the shared drive."',
      matches: ["PII (medium)", "reasoning: contract context + names"],
    },
  },
];

export const failurePolicy: readonly FailureRow[] = [
  {
    failure: "Idempotency check shows already-processed",
    classification: "expected",
    action: "Skip, commit offset, increment duplicate_total.",
  },
  {
    failure: "S3 fetch: 404 / NoSuchKey",
    classification: "permanent",
    action: "Emit object_missing finding, persist, commit.",
  },
  {
    failure: "S3 fetch: 5xx / network",
    classification: "transient",
    action: "Send to retry topic with attempt counter.",
  },
  {
    failure: "Object > MAX_OBJECT_BYTES",
    classification: "permanent",
    action: "Persist too_large finding, no classification, commit.",
  },
  {
    failure: "Regex / NER pass raises",
    classification: "unexpected",
    action: "DLQ with traceback; do not retry (treat as a code bug).",
  },
  {
    failure: "LLM: timeout / 5xx / 429",
    classification: "transient",
    action: "Send to retry topic.",
  },
  {
    failure: "LLM: malformed JSON after N parse retries",
    classification: "permanent",
    action:
      "Persist finding with llm_failed=true using regex + NER results; commit.",
  },
  {
    failure: "Postgres: connection error",
    classification: "transient",
    action:
      "Back off; do not commit offset; partition pauses naturally via backpressure.",
  },
  {
    failure: "Kafka produce to findings fails",
    classification: "transient",
    action: "Retry the produce; do not commit consume offset.",
  },
];

export const metrics: readonly MetricCardData[] = [
  {
    name: "findings_total",
    labels: ["tenant_id", "sensitivity"],
    meaning: "Counter of classifications produced, partitioned per tenant and sensitivity.",
    alertIntent: "Volume sanity — sudden drops mean the pipeline is starved or stuck.",
  },
  {
    name: "pipeline_stage_latency_seconds",
    labels: ["stage"],
    meaning: "Per-tier latency histogram (regex / ner / llm).",
    alertIntent: "LLM-tier slow path: alert on p95 sustained above SLO.",
  },
  {
    name: "kafka_consumer_lag_seconds",
    labels: ["topic", "partition"],
    meaning: "End-to-end backlog measured from producer timestamp to consumer commit.",
    alertIntent: "Feeds the HPA; pages oncall above a hard ceiling.",
  },
  {
    name: "in_flight_work",
    labels: [],
    meaning: "Gauge of concurrent classifications currently held by the work pool.",
    alertIntent: "Backpressure visibility — sustained pinning means the LLM tier is the bottleneck.",
  },
  {
    name: "partitions_paused",
    labels: [],
    meaning: "Gauge of TopicPartitions currently paused by the BoundedWorkPool.",
    alertIntent: "Sustained pause across partitions = consumer is overwhelmed, scale or shed load.",
  },
  {
    name: "dlq_total",
    labels: ["reason"],
    meaning: "Counter of permanent failures sent to the dead-letter topic.",
    alertIntent: "Alertable on any non-zero rate; reason label tells you which path failed.",
  },
  {
    name: "duplicate_total",
    labels: [],
    meaning: "Counter of events rejected by the idempotency table.",
    alertIntent: "Replay health — sustained increase signals upstream is replaying without intent.",
  },
];

export const cliOutputExample = `$ python scripts/classify_one.py s3://acme-uploads/contracts/q2.pdf
[regex]   matched 1 (FINANCIAL.CREDIT_CARD)            t=2.1ms
[ner]     matched 3 (PII.PERSON, PII.EMAIL, PII.PHONE) conf=0.92  t=87ms
[llm]     skipped (NER confidence above threshold)
─────────────────────────────────────────────
finding   tenant=acme-corp  event=evt_01HZABCXYZ  sensitivity=HIGH
          categories=[PII, FINANCIAL]  match_count=4
          pipeline_version=1  llm_failed=false
persisted to findings(tenant_id=acme-corp, event_id=evt_01HZABCXYZ)`;

export const interviewBullets: readonly { tag: string; body: string }[] = [
  {
    tag: "Async + perf",
    body: "Backpressure via BoundedWorkPool (semaphore + partition pause/resume), tiered classifier so the cheap path stays cheap, asyncio throughout.",
  },
  {
    tag: "Kafka at scale",
    body: "Tenant-keyed partitioning, consumer group, manual commits + idempotency table for effectively-once, retry topic with exponential backoff, DLQ, graceful shutdown without redelivery storms.",
  },
  {
    tag: "K8s / microservices",
    body: "Shared-image two-deployment topology, HPA on consumer lag via prometheus-adapter, sealed-secret pattern, Alembic migration Job, ServiceMonitor for scrape.",
  },
  {
    tag: "AWS",
    body: "aioboto3 against real S3 in deploy and MinIO locally; same SDK, standard credential chain, IAM patterns unchanged.",
  },
  {
    tag: "Gen-AI",
    body: "Structured-output LLM pass with Pydantic schema, parse/retry loop, cost-aware tiering, model swap by env var.",
  },
  {
    tag: "DSPM domain",
    body: "PII / SECRETS / FINANCIAL / HEALTH categories, sensitivity levels, tenant-scoped findings, pipeline_version for re-classification passes.",
  },
  {
    tag: "Production rigor",
    body: "Idempotency, DLQ, structured logs with bound contextvars, Prometheus metrics mapped to alerts, graceful shutdown respecting the K8s lifecycle.",
  },
];
```

- [ ] **Step 2: Verify types compile**

```bash
npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/lib/dspm/content.ts
git commit -m "$(cat <<'EOF'
feat(frontend): add DSPM page content module (diagrams + structured data)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Assemble `/dspm` page

**Files:**
- Create: `frontend/src/app/dspm/page.tsx`

**Interfaces:**
- Consumes: all of Tasks 1–5.
- Produces: a Next.js App Router route at `/dspm`.

- [ ] **Step 1: Create the page**

Create `frontend/src/app/dspm/page.tsx`:

```tsx
import Link from "next/link";

import { MermaidDiagram } from "@/components/MermaidDiagram";
import { BuildStateBadge } from "@/components/dspm/BuildStateBadge";
import { FailurePolicyTable } from "@/components/dspm/FailurePolicyTable";
import { MetricsGrid } from "@/components/dspm/MetricsGrid";
import { PipelineStageCard } from "@/components/dspm/PipelineStageCard";
import {
  architectureDiagram,
  cliOutputExample,
  failurePolicy,
  interviewBullets,
  k8sTopologyDiagram,
  metrics,
  pipelineDiagram,
  pipelineStages,
  planStatuses,
  retryDlqDiagram,
  sectionPlanStatus,
} from "@/lib/dspm/content";

const SPEC_HREF =
  "https://github.com/kabradshaw1/gen_ai_engineer/blob/main/docs/superpowers/specs/2026-06-16-dspm-classifier-design.md";
const PLAN_HREF =
  "https://github.com/kabradshaw1/gen_ai_engineer/blob/main/docs/superpowers/plans/2026-06-16-dspm-classifier-plan-1-engine.md";
const CODE_HREF =
  "https://github.com/kabradshaw1/gen_ai_engineer/tree/main/services/dspm-classifier";

export default function DspmPage() {
  return (
    <div className="mx-auto max-w-4xl px-6 py-12">
      {/* 3.1 Hero */}
      <section className="mt-8">
        <p className="text-sm font-medium uppercase text-muted-foreground">
          DSPM classifier · senior python / varonis dac
        </p>
        <h1 className="mt-3 text-4xl font-bold">
          Sensitive-data classification at Kafka scale
        </h1>
        <p className="mt-6 max-w-3xl text-lg leading-relaxed text-muted-foreground">
          A Python microservice that consumes object-storage events from Kafka,
          classifies content for sensitive data (PII, secrets, financial,
          health) through a tiered regex → NER → LLM pipeline, persists
          findings to Postgres, and emits to a downstream Kafka topic. It ships
          as two Kubernetes deployments built from one image so reads and
          writes scale independently.
        </p>
        <p className="mt-4 max-w-3xl leading-relaxed text-muted-foreground">
          The service is designed to exercise the Varonis DSPM role&apos;s
          mandatory skills: async Python, Kafka at scale, Kubernetes
          microservices, AWS-compatible storage, and Gen-AI integration. The
          domain mirrors Varonis&apos;s DSPM product surface.
        </p>
        <div className="mt-6 flex flex-wrap gap-2">
          {planStatuses.map((p) => (
            <BuildStateBadge
              key={p.plan}
              plan={p.plan}
              status={p.status}
              label={p.label}
            />
          ))}
        </div>
      </section>

      {/* 3.2 Architecture */}
      <section className="mt-12">
        <div className="flex flex-wrap items-baseline gap-3">
          <h2 className="text-2xl font-semibold">End-to-end architecture</h2>
          <BuildStateBadge
            plan={sectionPlanStatus.architecture.plan}
            status={sectionPlanStatus.architecture.status}
          />
        </div>
        <p className="mt-3 max-w-3xl leading-relaxed text-muted-foreground">
          S3 events flow through Kafka into the worker pool, which fetches
          objects, runs the tiered pipeline, persists findings to Postgres, and
          emits to the findings topic. The api reads from Postgres only. Retry
          and DLQ topics handle transient and permanent failures so the live
          path stays clean.
        </p>
        <div className="mt-6">
          <MermaidDiagram chart={architectureDiagram} />
        </div>
        <p className="mt-4 max-w-3xl text-sm leading-relaxed text-muted-foreground">
          Partition key is <code>tenant_id</code>, so all of one
          tenant&apos;s events stay ordered and isolated to a single partition.
          The wire guarantee is at-least-once; the idempotency table makes it
          effectively-once end-to-end.
        </p>
      </section>

      {/* 3.3 Pipeline deep-dive */}
      <section className="mt-12">
        <div className="flex flex-wrap items-baseline gap-3">
          <h2 className="text-2xl font-semibold">Classification pipeline</h2>
          <BuildStateBadge
            plan={sectionPlanStatus.pipeline.plan}
            status={sectionPlanStatus.pipeline.status}
          />
        </div>
        <p className="mt-3 max-w-3xl leading-relaxed text-muted-foreground">
          Three tiers, ordered cheapest first. Each stage emits a confidence
          score; the pipeline decides whether to escalate. Merge and dedupe
          happen after the final tier so overlapping spans collapse into one
          finding.
        </p>
        <div className="mt-6">
          <MermaidDiagram chart={pipelineDiagram} />
        </div>
        <div className="mt-8 grid gap-4 md:grid-cols-3">
          {pipelineStages.map((stage) => (
            <PipelineStageCard key={stage.name} {...stage} />
          ))}
        </div>
        <div className="mt-8">
          <h3 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
            CLI smoke test (Plan 1)
          </h3>
          <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
            <code>scripts/classify_one.py</code> wires the engine for a manual
            end-to-end test against a single S3 object.
          </p>
          <pre
            className="mt-3 overflow-x-auto rounded-lg border border-foreground/10 bg-muted/30 p-4 font-mono text-xs leading-relaxed text-muted-foreground"
            data-testid="cli-output-example"
          >
            {cliOutputExample}
          </pre>
        </div>
      </section>

      {/* 3.4 Reliability */}
      <section className="mt-12">
        <div className="flex flex-wrap items-baseline gap-3">
          <h2 className="text-2xl font-semibold">
            Reliability: retries, DLQ, idempotency, backpressure
          </h2>
          <BuildStateBadge
            plan={sectionPlanStatus.reliability.plan}
            status={sectionPlanStatus.reliability.status}
          />
        </div>
        <p className="mt-3 max-w-3xl leading-relaxed text-muted-foreground">
          The failure matrix classifies every per-stage error so the worker
          knows what to do without inventing policy at runtime.
        </p>
        <div className="mt-6">
          <FailurePolicyTable rows={failurePolicy} />
        </div>
        <div className="mt-8">
          <MermaidDiagram chart={retryDlqDiagram} />
        </div>
        <div className="mt-6 grid gap-4 md:grid-cols-2">
          <div className="rounded-lg border border-foreground/10 bg-muted/30 p-4">
            <h3 className="text-sm font-semibold">Idempotency</h3>
            <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
              <code>processed_messages(event_id PK, …, pipeline_version)</code>{" "}
              is inserted in the same transaction as the findings upsert. A
              duplicate event is a PK violation that rolls the transaction back
              cleanly — the offset still commits. Bumping{" "}
              <code>pipeline_version</code> is how an intentional
              re-classification pass reprocesses everything.
            </p>
          </div>
          <div className="rounded-lg border border-foreground/10 bg-muted/30 p-4">
            <h3 className="text-sm font-semibold">Backpressure</h3>
            <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
              <code>BoundedWorkPool</code> owns an{" "}
              <code>asyncio.Semaphore(N)</code> plus the set of paused{" "}
              <code>TopicPartition</code>s. It pauses at the high-water mark
              and resumes at the low-water mark, so a slow LLM tier never
              unbounded-queues messages. The consumer loop only asks{" "}
              &ldquo;can I dispatch?&rdquo; — it doesn&apos;t know how
              backpressure works.
            </p>
          </div>
        </div>
      </section>

      {/* 3.5 Deployment */}
      <section className="mt-12">
        <div className="flex flex-wrap items-baseline gap-3">
          <h2 className="text-2xl font-semibold">
            Deployment: Kubernetes + AWS
          </h2>
          <BuildStateBadge
            plan={sectionPlanStatus.deployment.plan}
            status={sectionPlanStatus.deployment.status}
          />
        </div>
        <p className="mt-3 max-w-3xl leading-relaxed text-muted-foreground">
          One Docker image, two Deployments. The worker stays at or below the
          partition count; the api scales on HTTP load. Both pods share
          ConfigMap and sealed Secret; migrations run as a pre-deploy Job.
        </p>
        <div className="mt-6">
          <MermaidDiagram chart={k8sTopologyDiagram} />
        </div>
        <p className="mt-4 max-w-3xl text-sm leading-relaxed text-muted-foreground">
          The AWS positioning is intentionally cheap: <code>aioboto3</code>{" "}
          targets MinIO locally and real AWS S3 in deploy via the{" "}
          <code>S3_ENDPOINT_URL</code> env var. Same SDK, same IAM patterns,
          zero code change. Real cloud bills only show up if and when this
          actually deploys.
        </p>
      </section>

      {/* 3.6 Observability */}
      <section className="mt-12">
        <div className="flex flex-wrap items-baseline gap-3">
          <h2 className="text-2xl font-semibold">
            Observability &amp; metrics catalog
          </h2>
          <BuildStateBadge
            plan={sectionPlanStatus.observability.plan}
            status={sectionPlanStatus.observability.status}
          />
        </div>
        <p className="mt-3 max-w-3xl leading-relaxed text-muted-foreground">
          Each metric maps to an alert intent. Cardinality is bounded — labels
          are tenant_id, sensitivity, stage, and partition, not anything
          user-supplied.
        </p>
        <div className="mt-6">
          <MetricsGrid metrics={metrics} />
        </div>
      </section>

      {/* 3.7 Interview talking points */}
      <section className="mt-12">
        <h2 className="text-2xl font-semibold">Interview talking points</h2>
        <div className="mt-5 grid gap-3 md:grid-cols-2">
          {interviewBullets.map((bullet) => (
            <div
              key={bullet.tag}
              className="rounded-lg border border-foreground/10 p-4"
            >
              <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                {bullet.tag}
              </p>
              <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
                {bullet.body}
              </p>
            </div>
          ))}
        </div>
      </section>

      {/* 3.8 Related artifacts */}
      <section className="mt-12">
        <h2 className="text-2xl font-semibold">Related artifacts</h2>
        <div className="mt-5 flex flex-wrap gap-3">
          <a
            href={SPEC_HREF}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center rounded-lg border px-4 py-2 text-sm font-medium transition-colors hover:bg-accent"
          >
            Service spec
          </a>
          <a
            href={PLAN_HREF}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center rounded-lg border px-4 py-2 text-sm font-medium transition-colors hover:bg-accent"
          >
            Plan 1 — engine
          </a>
          <a
            href={CODE_HREF}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center rounded-lg border px-4 py-2 text-sm font-medium transition-colors hover:bg-accent"
          >
            Code: services/dspm-classifier
          </a>
          <Link
            href="/observability"
            className="inline-flex items-center rounded-lg border px-4 py-2 text-sm font-medium transition-colors hover:bg-accent"
          >
            Observability deep-dive
          </Link>
        </div>
      </section>
    </div>
  );
}
```

- [ ] **Step 2: Verify types compile**

```bash
npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Lint**

```bash
npm run lint
```

Expected: no errors. Warnings about `<code>` are fine if any appear; treat ESLint errors as blocking.

- [ ] **Step 4: Visually verify in the dev server**

```bash
npm run dev
```

Open `http://localhost:3000/dspm`. Confirm:
- H1 "Sensitive-data classification at Kafka scale" renders.
- Three `BuildStateBadge`s in the hero (Plan 1 in-progress amber, Plans 2 and 3 designed muted).
- Four Mermaid diagrams render as SVG (architecture, pipeline, retry/DLQ sequence, K8s topology).
- Three `PipelineStageCard`s render with cost chips colored green/amber/red.
- CLI output `<pre>` block renders below the cards.
- Failure-policy table renders with colored class chips.
- Metrics grid renders 7 cards.
- Interview-talking-points grid renders 7 cards.
- Related-artifacts buttons all resolve to real URLs.

Stop the dev server.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/app/dspm/page.tsx
git commit -m "$(cat <<'EOF'
feat(frontend): add /dspm learning page for DSPM classifier service

Composes BuildStateBadge, PipelineStageCard, FailurePolicyTable, and
MetricsGrid alongside four Mermaid diagrams (architecture, pipeline,
retry/DLQ, K8s topology), a CLI smoke-output preview, interview
talking points, and bottom-nav links to the spec, plan, and code.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Home tile + Playwright smoke test

**Files:**
- Modify: `frontend/src/app/page.tsx`
- Modify: `frontend/e2e/smoke-prod/smoke.spec.ts`

**Interfaces:**
- Consumes: the `/dspm` route from Task 6.
- Produces: a discoverable entry point from the home page, plus an end-to-end check.

- [ ] **Step 1: Add the home page tile**

Open `frontend/src/app/page.tsx`. Find the tile for `/observability` (currently around line 133). Insert a new `Link` block directly after the closing `</Link>` of that tile, before the `/cicd` tile.

The new block:

```tsx
<Link href="/dspm" className="block">
  <Card className="hover:ring-foreground/20 transition-all">
    <CardHeader>
      <CardTitle>DSPM Classifier</CardTitle>
      <CardDescription>
        Sensitive-data classification at Kafka scale — Python microservice
        designed for the Varonis DSPM senior engineering role
      </CardDescription>
    </CardHeader>
    <CardContent>
      <p className="text-muted-foreground text-sm">
        Tiered regex → NER → LLM pipeline, tenant-keyed Kafka
        partitioning, idempotency via processed_messages, backpressure
        via BoundedWorkPool, and a two-deployment K8s topology from one
        image. See the architecture walkthrough for the full design.
      </p>
    </CardContent>
  </Card>
</Link>
```

Save the file.

- [ ] **Step 2: Verify the home page still compiles and lints**

```bash
npx tsc --noEmit
npm run lint
```

Expected: no errors.

- [ ] **Step 3: Add the Playwright smoke test**

Open `frontend/e2e/smoke-prod/smoke.spec.ts`. Locate the existing `test("frontend loads", …)` block inside the `test.describe("Production smoke tests", …)` describe. Add a new test inside the same describe, immediately after `frontend loads`:

```ts
test("dspm learning page renders", async ({ page }) => {
  await page.goto(`${FRONTEND_URL}/dspm`);
  await expect(
    page.locator("h1", {
      hasText: "Sensitive-data classification at Kafka scale",
    })
  ).toBeVisible();
  await expect(page.getByTestId("cli-output-example")).toBeVisible();
  await expect(page.getByTestId("failure-policy-table")).toBeVisible();
  await expect(page.getByTestId("metrics-grid")).toBeVisible();
  await expect(page.getByTestId("pipeline-stage-regex")).toBeVisible();
});
```

Save the file.

- [ ] **Step 4: Run the smoke test locally**

In one terminal, start the dev server from `frontend/`:

```bash
npm run dev
```

In another terminal, run the smoke test against localhost:

```bash
cd frontend
SMOKE_FRONTEND_URL=http://localhost:3000 npx playwright test e2e/smoke-prod/smoke.spec.ts -g "dspm learning page renders"
```

Expected: `1 passed`. If it fails, fix and re-run.

Stop the dev server.

- [ ] **Step 5: Final CI checks (full type + lint pass)**

```bash
cd frontend
npx tsc --noEmit
npm run lint
```

Expected: both pass with no errors.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/app/page.tsx frontend/e2e/smoke-prod/smoke.spec.ts
git commit -m "$(cat <<'EOF'
feat(frontend): link /dspm from home page and smoke-test the route

Adds a portfolio tile linking to the new DSPM classifier learning page
and a Playwright smoke test that verifies the page hydrates with its
key landmarks (H1, CLI output block, failure policy table, metrics
grid, and the regex pipeline stage card).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 7: Push the branch (Kyle's rule: doc-only commits stay local, code commits ship)**

```bash
git push -u origin "$(git rev-parse --abbrev-ref HEAD)"
```

If on `main`, instead push only to a feature branch per `feedback_no_push`:

```bash
git switch -c feat/frontend-dspm-page
git push -u origin feat/frontend-dspm-page
```

Then open a PR to `qa` (per Kyle's agent-push rules).

---

## Self-Review

**1. Spec coverage:**
- Spec §2 (route + file structure) → Tasks 1–6 create exactly the files listed.
- Spec §3 (sections 3.1–3.8) → Task 6 assembles all eight sections in order.
- Spec §3.3 CLI output block → Task 5 defines `cliOutputExample`; Task 6 renders it in a styled `<pre>` with `data-testid="cli-output-example"`.
- Spec §4 component props → Tasks 1–4 expose the exact interfaces from §4.
- Spec §5 content source → Task 5 exports every named symbol from §5.
- Spec §6 styling/a11y → Global Constraints carry forward; Tasks 1–4 use the listed tokens; chips include text labels.
- Spec §7 testing → Task 7 adds the Playwright smoke test; unit tests are explicitly out of scope (no framework installed; documented in Global Constraints).
- Spec §9 acceptance criteria → covered by Task 6 step 4 (visual verify) and Task 7 step 4 (smoke).

**2. Placeholder scan:** No "TBD"/"TODO"/"similar to" — every code block is full. Verified by re-reading each task.

**3. Type consistency:**
- `BuildState` type defined in Task 1 is imported by Task 5 via `@/components/dspm/BuildStateBadge`.
- `FailureRow`, `MetricCardData`, `PipelineStageCardProps` all defined in Tasks 2–4 and imported by Task 5 from the correct module paths.
- `sectionPlanStatus` keys (`architecture`, `pipeline`, `reliability`, `deployment`, `observability`) match the section component usage in Task 6.
- `planStatuses` plan literal types are `1 | 2 | 3`, matching `BuildStateBadgeProps.plan`.
- Component prop fields used in Task 6's JSX (e.g. `stage.name`, `cliOutputExample` as string, `failurePolicy` as array) all match the exported shapes.

No issues found.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-06-16-dspm-classifier-frontend-plan.md`. Two execution options:

1. **Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration.
2. **Inline Execution** — Execute tasks in this session using `executing-plans`, batch execution with checkpoints.

Which approach?
