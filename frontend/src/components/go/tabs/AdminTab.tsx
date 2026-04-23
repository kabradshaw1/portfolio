import { MermaidDiagram } from "@/components/MermaidDiagram";

export function AdminTab() {
  return (
    <div className="mt-8">
      {/* Why a DLQ Admin Panel */}
      <section>
        <h3 className="text-lg font-medium">Why a DLQ Admin Panel?</h3>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          Sagas fail. Messages get nacked to the Dead Letter Queue when a
          saga step errors repeatedly &mdash; stock unavailable, payment
          declined, database constraint violated. Without tooling, those
          messages sit invisible in the DLQ while orders are stuck in
          FAILED or COMPENSATING state, and the only recovery path is
          manual SQL updates and queue inspection via the RabbitMQ
          management console.
        </p>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          The admin panel exposes DLQ messages through the order-service
          REST API, giving operators a web UI to inspect failed saga
          messages, understand why they failed, and replay them with a
          single click after the root cause is resolved.
        </p>
      </section>

      {/* How Replay Works */}
      <section className="mt-12">
        <h3 className="text-xl font-semibold">How Replay Works</h3>
        <div className="mt-6">
          <MermaidDiagram
            chart={`flowchart LR
  DLQ{{Dead Letter Queue<br/>RabbitMQ}}
  AP[Admin Panel<br/>Frontend]
  API[order-service<br/>Admin REST API]
  MQ{{Saga Exchange<br/>RabbitMQ}}
  SAGA[Saga Orchestrator<br/>order-service consumer]
  AP -->|GET /admin/dlq/messages| API
  API -->|peek messages| DLQ
  AP -->|POST /admin/dlq/replay| API
  API -->|re-publish with original routing key| MQ
  MQ -->|saga event| SAGA
  SAGA -->|resumes from last step| SAGA`}
          />
        </div>
      </section>

      {/* What It Shows */}
      <section className="mt-12">
        <h3 className="text-xl font-semibold">What It Shows</h3>
        <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2">
          {[
            {
              title: "Message Inspection",
              desc: "View DLQ message payloads including orderID, saga step, routing key, and the error that caused the nack.",
            },
            {
              title: "One-Click Replay",
              desc: "Re-publish a DLQ message to the saga exchange with its original routing key. The orchestrator resumes from the failed step.",
            },
            {
              title: "DLQ Count",
              desc: "Live count of messages in the DLQ. Grafana alert fires when this exceeds threshold, prompting operator review.",
            },
            {
              title: "Operational Awareness",
              desc: "Pairs with Loki logs and Jaeger traces so operators can drill from a failed message to the full saga trace in one workflow.",
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
    </div>
  );
}
