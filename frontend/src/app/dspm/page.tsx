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
  engineeringHighlights,
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
          DSPM classifier · async python · kafka · kubernetes
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
          The service exercises a demanding production skill set: async Python,
          Kafka at scale, Kubernetes microservices, AWS-compatible storage, and
          Gen-AI integration. The domain mirrors a real DSPM (data security
          posture management) product surface.
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

      {/* 3.7 Engineering highlights */}
      <section className="mt-12">
        <h2 className="text-2xl font-semibold">Engineering highlights</h2>
        <div className="mt-5 grid gap-3 md:grid-cols-2">
          {engineeringHighlights.map((bullet) => (
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
