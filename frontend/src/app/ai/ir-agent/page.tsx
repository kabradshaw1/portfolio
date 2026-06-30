import Link from "next/link";
import { MermaidDiagram } from "@/components/MermaidDiagram";
import { PillarSection } from "@/components/database/PillarSection";
import {
  CAPTURED_ON,
  IR_AGENT_METRICS,
  MEASURED,
} from "@/lib/ir-agent/metrics";

const ADR_URL =
  "https://github.com/kabradshaw1/portfolio/blob/main/docs/adr/ir-agent/01_langgraph_multi_agent_design.ipynb";

const pipelineDiagram = `flowchart LR
  I[Incident\nalert] --> T[Triage\nHaiku]
  T --> N[Investigate\nOpus + tools]
  N --> V[Validate\nOpus]
  V -->|grounded| R[Report\nSonnet]
  V -->|needs more evidence| N
  R --> S[Report +\ncost summary]
`;

export default function IRAgentPage() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <div className="mx-auto max-w-3xl px-6 py-12">
        <Link
          href="/ai"
          className="text-sm text-muted-foreground transition-colors hover:text-foreground"
        >
          &larr; Back to AI
        </Link>

        <h1 className="mt-8 text-3xl font-bold">Incident-Response Agent</h1>
        <p className="mt-4 leading-relaxed text-muted-foreground">
          A LangGraph multi-agent service that investigates security incidents
          end to end. Four specialized agents — triage, investigate, validate,
          and report — pass typed state through a graph, calling read-only
          evidence tools and looping until a validator confirms the findings are
          grounded. Every node is assigned the cheapest Claude model that can do
          its job, and every run is fully instrumented: per-role tokens, cost,
          latency, and the savings from tiering versus running Opus everywhere.
        </p>

        <div className="mt-6 flex flex-wrap gap-3">
          <Link
            href="/ai/ir-agent/demo"
            className="inline-flex items-center gap-2 rounded-lg bg-primary px-5 py-2.5 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
          >
            Try the live demo &rarr;
          </Link>
          <a
            href={ADR_URL}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-2 rounded-lg border px-5 py-2.5 text-sm font-medium transition-colors hover:bg-accent"
          >
            Read the design ADR &rarr;
          </a>
        </div>

        {/* Pipeline diagram */}
        <section className="mt-12">
          <h2 className="text-2xl font-semibold">The pipeline</h2>
          <p className="mt-3 leading-relaxed text-muted-foreground">
            State flows left to right, but the validator can send work back: if
            the findings aren&rsquo;t grounded in collected evidence, the graph
            loops to investigate again, bounded by a max-attempts guard so it
            always terminates.
          </p>
          <div className="mt-6 rounded-xl border border-foreground/10 bg-card p-6">
            <MermaidDiagram chart={pipelineDiagram} />
          </div>
        </section>

        {/* Pillars */}
        <div className="mt-12 space-y-12">
          <PillarSection
            id="role-tiering"
            title="Role-based model tiering"
            narrative={
              <>
                <p>
                  Not every step needs the same horsepower. Triage is a fast
                  classification, so it runs on <code>Haiku</code>. The
                  investigate and validate agents do the heavy reasoning over
                  evidence, so they run on <code>Opus</code>. Drafting the final
                  report is a structured write-up, well within{" "}
                  <code>Sonnet</code>&rsquo;s reach.
                </p>
                <p>
                  The service is Anthropic-locked by design — the per-role
                  tiering <em>is</em> the architecture. Each run reports its
                  actual cost against a hypothetical &ldquo;Opus everywhere&rdquo;
                  baseline, so the savings are measured, not asserted.
                </p>
              </>
            }
            bullets={[
              <>
                <code>triage</code> → Haiku, <code>investigate</code> /{" "}
                <code>validate</code> → Opus, <code>report</code> → Sonnet
              </>,
              "A callback handler taps raw model calls to attribute tokens per role — usage that with_structured_output would otherwise discard",
              "Per-run summary: per-role tokens, cost, latency, and a tiered-vs-Opus savings factor",
              "Prometheus metrics (LLM tokens, node duration, tool calls) exported for every run",
            ]}
            links={[{ label: "ADR: LangGraph multi-agent design", href: ADR_URL }]}
          />

          <PillarSection
            id="two-layer-validation"
            title="Two-layer validation"
            narrative={
              <>
                <p>
                  Hallucinated incident reports are worse than none. The agent
                  defends against that twice. First, every node emits a
                  validated Pydantic model rather than free text — the graph
                  state is type-checked end to end. Second, a dedicated validate
                  agent checks the findings against the actual evidence and
                  decides whether they&rsquo;re grounded.
                </p>
                <p>
                  If the validator finds unsupported claims or gaps, it sends
                  the investigation back for another pass. A bounded loop keeps
                  it from running forever — after the attempt cap, it proceeds to
                  the report with what it has.
                </p>
              </>
            }
            bullets={[
              "Structured outputs (Pydantic) at every node — no free-text state",
              "A validator agent scores grounding and flags unsupported claims and gaps",
              "Bounded investigate↔validate loop: re-investigates on weak evidence, always terminates",
              "Read-only evidence tools over deterministic fixtures — fully reproducible runs",
            ]}
            links={[{ label: "ADR: LangGraph multi-agent design", href: ADR_URL }]}
          />
        </div>

        {/* Measured outcomes */}
        <section className="mt-12">
          <h2 className="text-2xl font-semibold">Measured outcomes</h2>
          {MEASURED && IR_AGENT_METRICS.length > 0 ? (
            <>
              <p className="mt-3 leading-relaxed text-muted-foreground">
                Real numbers from a capture run across the three fixture
                incidents{CAPTURED_ON ? ` (${CAPTURED_ON})` : ""}. The same
                transcripts power the always-on replay demo.
              </p>
              <div className="mt-6 overflow-x-auto rounded-xl border border-foreground/10">
                <table className="w-full text-sm">
                  <thead className="bg-muted/50 text-left text-xs uppercase tracking-wide text-muted-foreground">
                    <tr>
                      <th className="px-4 py-2 font-medium">Incident</th>
                      <th className="px-4 py-2 text-right font-medium">Cost</th>
                      <th className="px-4 py-2 text-right font-medium">vs all-Opus</th>
                      <th className="px-4 py-2 text-right font-medium">Savings</th>
                      <th className="px-4 py-2 text-right font-medium">Tokens</th>
                      <th className="px-4 py-2 text-right font-medium">Latency</th>
                      <th className="px-4 py-2 text-right font-medium">Passes</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-foreground/10">
                    {IR_AGENT_METRICS.map((m) => (
                      <tr key={m.incidentId}>
                        <td className="px-4 py-2">
                          <div className="font-medium text-foreground">{m.title}</div>
                          <code className="font-mono text-xs text-muted-foreground">
                            {m.incidentId}
                          </code>
                        </td>
                        <td className="px-4 py-2 text-right tabular-nums text-foreground">
                          ${m.costUsd.toFixed(4)}
                        </td>
                        <td className="px-4 py-2 text-right tabular-nums text-muted-foreground">
                          ${m.opusCostUsd.toFixed(4)}
                        </td>
                        <td className="px-4 py-2 text-right tabular-nums text-foreground">
                          {m.savingsFactor.toFixed(2)}×
                        </td>
                        <td className="px-4 py-2 text-right tabular-nums text-muted-foreground">
                          {m.totalTokens.toLocaleString()}
                        </td>
                        <td className="px-4 py-2 text-right tabular-nums text-muted-foreground">
                          {m.seconds.toFixed(1)}s
                        </td>
                        <td className="px-4 py-2 text-right tabular-nums text-muted-foreground">
                          {m.investigateAttempts}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          ) : (
            <p className="mt-3 leading-relaxed text-muted-foreground">
              Measured cost, token, and latency figures are captured from real
              runs and published here alongside the replay demo. See the{" "}
              <Link
                href="/ai/ir-agent/demo"
                className="underline hover:text-foreground"
              >
                live demo
              </Link>{" "}
              for per-run numbers.
            </p>
          )}
        </section>
      </div>
    </div>
  );
}
