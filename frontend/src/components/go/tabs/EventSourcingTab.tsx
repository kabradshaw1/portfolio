import { MermaidDiagram } from "@/components/MermaidDiagram";

export function EventSourcingTab() {
  return (
    <div className="mt-8">
      {/* Why Event Sourcing */}
      <section>
        <h3 className="text-lg font-medium">Why Event Sourcing?</h3>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          Traditional CRUD systems overwrite state on every update &mdash;
          once an order moves from &quot;pending&quot; to &quot;completed&quot;,
          the intermediate steps are lost. Event sourcing records every state
          transition as an immutable event in a Kafka topic, creating a full
          audit trail. A separate{" "}
          <span className="text-foreground font-medium">
            CQRS projection service
          </span>{" "}
          consumes this stream and builds read-optimized views, enabling
          independent scaling, schema evolution, and the ability to rebuild
          all read models from scratch via event replay.
        </p>
      </section>

      {/* Architecture */}
      <section className="mt-12">
        <h3 className="text-xl font-semibold">Architecture</h3>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          The order-service publishes 7 domain events at each saga step to a
          compacted Kafka topic. The order-projector consumes these events
          via its own consumer group and builds three PostgreSQL read models:
          a full timeline, a denormalized summary, and hourly stats.
        </p>
        <div className="mt-6">
          <MermaidDiagram
            chart={`flowchart LR
  ORD[order-service<br/>saga orchestrator]
  KF{{Kafka<br/>ecommerce.order-events<br/>compacted}}
  PROJ[order-projector<br/>CQRS consumer]
  TL[(order_timeline<br/>audit trail)]
  SUM[(order_summary<br/>latest state)]
  STATS[(order_stats<br/>hourly agg)]
  FE[Frontend<br/>timeline UI]
  ORD -->|7 event types| KF
  KF -->|projector-group| PROJ
  PROJ --> TL
  PROJ --> SUM
  PROJ --> STATS
  SUM --> FE
  TL --> FE`}
          />
        </div>
      </section>

      {/* Tech Stack */}
      <section className="mt-8">
        <h3 className="text-lg font-medium">Tech Stack</h3>
        <div className="mt-3 flex flex-wrap gap-2">
          {[
            "Event Sourcing",
            "CQRS",
            "Kafka Log Compaction",
            "Schema Evolution",
            "Event Replay",
            "Versioned JSON",
          ].map((tech) => (
            <span
              key={tech}
              className="rounded-full bg-primary/10 px-3 py-1 text-xs font-medium text-primary"
            >
              {tech}
            </span>
          ))}
        </div>
      </section>

      {/* Event Flow */}
      <section className="mt-12">
        <h3 className="text-xl font-semibold">
          Request flow: Order event lifecycle
        </h3>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          Each saga step publishes a domain event to Kafka. The projector
          deserializes the event (applying version upgrades if needed),
          then writes to all three read models in a single pass.
        </p>
        <div className="mt-6">
          <MermaidDiagram
            chart={`sequenceDiagram
  participant S as Saga Orchestrator
  participant K as Kafka
  participant P as order-projector
  participant DB as projectordb
  participant FE as Frontend
  S->>K: order.created (v1)
  S->>K: order.reserved
  S->>K: order.payment_initiated
  S->>K: order.payment_completed
  S->>K: order.completed
  Note over K,P: Consumer group: order-projector-group
  K->>P: FetchMessage
  P->>P: Deserialize + version upgrade (v1→v2)
  P->>DB: INSERT order_timeline (idempotent)
  P->>DB: UPSERT order_summary
  P->>DB: UPSERT order_stats (hourly bucket)
  P->>K: CommitOffset
  FE->>P: GET /orders/:id/timeline
  P->>FE: Full event history + X-Projection-Lag header`}
          />
        </div>
      </section>

      {/* What It Demonstrates */}
      <section className="mt-12">
        <h3 className="text-xl font-semibold">What It Demonstrates</h3>
        <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2">
          {[
            {
              title: "Immutable Event Log",
              desc: "Every saga step recorded as an append-only event. Full audit trail queryable via timeline endpoint.",
            },
            {
              title: "CQRS Read/Write Split",
              desc: "Separate projector service with its own database. Denormalized schemas optimized for reads, independent from the write path.",
            },
            {
              title: "Schema Evolution",
              desc: "Versioned JSON events with chainable upgrade functions. V1\u2192V2 demonstrated with currency field backfill during replay.",
            },
            {
              title: "Event Replay",
              desc: "Admin endpoint truncates read models and rebuilds from the full event stream. Disaster recovery in one API call.",
            },
          ].map((card) => (
            <div
              key={card.title}
              className="rounded-lg border border-foreground/10 p-4"
            >
              <h4 className="text-sm font-semibold">{card.title}</h4>
              <p className="mt-1 text-xs text-muted-foreground leading-relaxed">
                {card.desc}
              </p>
            </div>
          ))}
        </div>
      </section>

      {/* ADR Link */}
      <section className="mt-8">
        <p className="text-sm text-muted-foreground leading-relaxed">
          Full design rationale documented in the{" "}
          <a
            href="https://github.com/kabradshaw1/portfolio/blob/main/docs/adr/ecommerce/kafka-event-sourcing-cqrs.md"
            target="_blank"
            rel="noopener noreferrer"
            className="underline hover:text-foreground transition-colors"
          >
            Event Sourcing &amp; CQRS ADR
          </a>
          .
        </p>
      </section>
    </div>
  );
}
