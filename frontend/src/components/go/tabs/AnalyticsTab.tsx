import { MermaidDiagram } from "@/components/MermaidDiagram";

export function AnalyticsTab() {
  return (
    <div className="mt-8">
      {/* Why Streaming Analytics */}
      <section>
        <h3 className="text-lg font-medium">Why Streaming Analytics?</h3>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          Batch analytics introduces latency &mdash; you know what sold
          yesterday, not what&apos;s trending right now. Streaming analytics
          processes events as they occur: every order placed, every cart
          updated, every product viewed flows through Kafka and into
          in-memory sliding windows that update within seconds. The result
          is live revenue tracking, real-time trending products, and
          cart abandonment signals that reflect the current state of the
          platform.
        </p>
      </section>

      {/* Architecture */}
      <section className="mt-12">
        <h3 className="text-xl font-semibold">Architecture</h3>
        <div className="mt-6">
          <MermaidDiagram
            chart={`flowchart LR
  ORD[order-service]
  CART[cart-service]
  KF{{Apache Kafka<br/>KRaft mode}}
  ANA[analytics-service<br/>Go consumer]
  WIN[In-memory<br/>sliding windows]
  API[REST API<br/>:8094]
  FE[Frontend<br/>30s polling]
  ORD -->|order events| KF
  CART -->|cart events| KF
  KF -->|analytics-group| ANA
  ANA --> WIN
  WIN --> API
  API --> FE`}
          />
        </div>
      </section>

      {/* Tech Stack */}
      <section className="mt-8">
        <h3 className="text-lg font-medium">Tech Stack</h3>
        <div className="mt-3 flex flex-wrap gap-2">
          {[
            "Apache Kafka 3.7 (KRaft)",
            "segmentio/kafka-go",
            "Sliding window aggregation",
            "Prometheus metrics",
            "OTel trace propagation",
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

      {/* What It Surfaces */}
      <section className="mt-12">
        <h3 className="text-xl font-semibold">What It Surfaces</h3>
        <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-3">
          {[
            {
              title: "Revenue per Hour",
              desc: "Rolling 1-hour window of completed order totals. Updates within seconds of each order.completed Kafka event.",
            },
            {
              title: "Trending Products",
              desc: "Products ranked by view and add-to-cart event frequency in the last 30 minutes. No database query needed.",
            },
            {
              title: "Cart Abandonment",
              desc: "Carts with items added but no checkout event in the last hour, surfaced as an abandonment signal.",
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
