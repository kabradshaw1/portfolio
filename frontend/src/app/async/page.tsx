import Link from "next/link";

const grafanaUrl =
  "https://grafana.kylebradshaw.dev/d/system-overview/system-overview?orgId=1&from=now-1h&to=now&timezone=browser";

const rabbitMqWork = [
  "Checkout saga orchestration with command/reply queues across order, payment, inventory, and shipping boundaries.",
  "Bounded retries with explicit DLQ routing so transient broker or service failures do not become infinite poison-message loops.",
  "Replay/admin panel support for inspected DLQ messages, including operator-controlled recovery paths.",
  "Publisher confirms, reconnect-aware publishing, graceful shutdown, and crash recovery around the RabbitMQ client lifecycle.",
];

const kafkaWork = [
  "Order domain events published as the durable event stream behind event sourcing and downstream projections.",
  "CQRS projector paths that rebuild read models from replayable events instead of coupling UI reads to transactional writes.",
  "Schema evolution, replay discipline, Kafka DLQ envelopes, consumer lag metrics, and streaming analytics over order activity.",
];

const reliabilityPractices = [
  "Idempotency at async boundaries so duplicate deliveries and retries remain safe.",
  "Retry classification that separates transient failures from validation and poison-message failures.",
  "DLQs with enough envelope context to debug, alert, and replay without guessing.",
  "Prometheus metrics for consumer lag, DLQ depth, saga progress, publish outcomes, and worker health.",
  "Trace propagation through RabbitMQ headers and Kafka message headers for cross-service debugging.",
  "Focused tests around retry behavior, replay paths, graceful shutdown, and crash recovery.",
];

const demoLinks = [
  { href: "/go/ecommerce", label: "Go ecommerce" },
  { href: "/go/analytics", label: "Streaming Analytics" },
  { href: "/go/ecommerce/orders", label: "Order Timeline" },
  { href: "/go/admin", label: "Admin Panel" },
  { href: grafanaUrl, label: "Grafana", external: true },
];

export default function AsyncPage() {
  return (
    <div className="mx-auto max-w-4xl px-6 py-12">
      <section className="mt-8">
        <p className="text-sm font-medium uppercase text-muted-foreground">
          Go ecommerce platform
        </p>
        <h1 className="mt-3 text-4xl font-bold">
          Asynchronous Systems Engineering
        </h1>
        <p className="mt-6 max-w-3xl text-lg leading-relaxed text-muted-foreground">
          This section highlights the production-grade async work behind the Go
          ecommerce services: RabbitMQ for checkout saga coordination, Kafka for
          order domain events and replayable projections, and the operational
          guardrails that make message-driven systems debuggable after they
          leave the happy path.
        </p>
        <p className="mt-4 max-w-3xl leading-relaxed text-muted-foreground">
          The implementation is intentionally practical: Go, Kafka, and
          RabbitMQ are used where they solve concrete reliability problems in
          checkout, analytics, event sourcing, CQRS read models, and incident
          response.
        </p>
      </section>

      <section className="mt-12 grid gap-8 md:grid-cols-2">
        <div>
          <h2 className="text-2xl font-semibold">RabbitMQ Checkout Saga</h2>
          <p className="mt-3 leading-relaxed text-muted-foreground">
            RabbitMQ coordinates the checkout workflow where services need
            explicit commands, replies, compensation, and operator recovery.
          </p>
          <ul className="mt-5 space-y-3 text-sm leading-relaxed text-muted-foreground">
            {rabbitMqWork.map((item) => (
              <li key={item} className="border-l border-foreground/15 pl-4">
                {item}
              </li>
            ))}
          </ul>
        </div>

        <div>
          <h2 className="text-2xl font-semibold">Kafka Event Streams</h2>
          <p className="mt-3 leading-relaxed text-muted-foreground">
            Kafka carries order facts as an append-only stream for event
            sourcing, CQRS projection, replay, and analytics.
          </p>
          <ul className="mt-5 space-y-3 text-sm leading-relaxed text-muted-foreground">
            {kafkaWork.map((item) => (
              <li key={item} className="border-l border-foreground/15 pl-4">
                {item}
              </li>
            ))}
          </ul>
        </div>
      </section>

      <section className="mt-14">
        <h2 className="text-2xl font-semibold">Reliability Practices</h2>
        <div className="mt-5 grid gap-3 md:grid-cols-2">
          {reliabilityPractices.map((practice) => (
            <div
              key={practice}
              className="rounded-lg border border-foreground/10 p-4"
            >
              <p className="text-sm leading-relaxed text-muted-foreground">
                {practice}
              </p>
            </div>
          ))}
        </div>
      </section>

      <section className="mt-14">
        <h2 className="text-2xl font-semibold">Related Demos</h2>
        <div className="mt-5 flex flex-wrap gap-3">
          {demoLinks.map((link) =>
            link.external ? (
              <a
                key={link.href}
                href={link.href}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center rounded-lg border px-4 py-2 text-sm font-medium transition-colors hover:bg-accent"
              >
                {link.label}
              </a>
            ) : (
              <Link
                key={link.href}
                href={link.href}
                className="inline-flex items-center rounded-lg border px-4 py-2 text-sm font-medium transition-colors hover:bg-accent"
              >
                {link.label}
              </Link>
            ),
          )}
        </div>
      </section>
    </div>
  );
}
