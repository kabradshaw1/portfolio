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
          grounded. Every node runs a right-sized Claude model, and every run is
          fully instrumented: per-role tokens, cost, and latency.
        </p>
        <p className="mt-4 leading-relaxed text-muted-foreground">
          This page is a progress narrative, not a pitch:{" "}
          <span className="text-foreground">build → instrument → measure →</span>{" "}
          learn what the data actually says. The headline finding is that the
          service&rsquo;s value isn&rsquo;t cost arbitrage — it&rsquo;s{" "}
          <span className="text-foreground">grounding rigor</span>: a validation
          agent that catches the subtle overclaiming which turns a plausible
          incident report into a wrong one.
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
            id="two-layer-validation"
            title="Two-layer validation — the real value"
            narrative={
              <>
                <p>
                  Hallucinated incident reports are worse than none. The agent
                  defends against that twice. First, every node emits a validated
                  Pydantic model rather than free text — the graph state is
                  type-checked end to end. Second, a dedicated validate agent
                  checks the findings against the <em>actual</em> evidence and
                  decides whether they&rsquo;re grounded, sending weak
                  investigations back for another bounded pass.
                </p>
                <p>
                  This isn&rsquo;t theater. In the captured runs the validator
                  caught specific, load-bearing overclaims — the kind that read
                  as confident and are quietly wrong:
                </p>
              </>
            }
            bullets={[
              "Inferred-not-stated bindings: “get_asset_context-20 contains NO field naming ‘jdoe’ … the findings label this ‘confirmed’, which overstates the evidence.”",
              "Duplicate evidence counted as independent: “get_logs-4/5/6 are byte-identical copies of the same two log lines … cited as if independent, artificially inflating the network evidence.”",
              "Unverifiable tool bindings: “lookup_ioc results don’t echo the queried indicator, so the reputation→IOC link is an inference, not grounded.”",
              "Bounded investigate↔validate loop over read-only evidence tools and deterministic fixtures — reproducible, and it always terminates.",
            ]}
            links={[{ label: "ADR: LangGraph multi-agent design", href: ADR_URL }]}
          />

          <PillarSection
            id="role-tiering"
            title="Role-based model tiering — and what measuring it revealed"
            narrative={
              <>
                <p>
                  Each node runs a right-sized model: <code>Haiku</code> triages
                  (~$0.002, ~2s), <code>Opus</code> does the heavy reasoning in
                  investigate and validate, and <code>Sonnet</code> drafts the
                  report. A callback handler taps the raw model calls to
                  attribute tokens per role — usage that{" "}
                  <code>with_structured_output</code> would otherwise discard.
                </p>
                <p>
                  I built the telemetry expecting a strong &ldquo;tiering saves
                  money&rdquo; story. The data said otherwise, and that&rsquo;s
                  the point of measuring: tiering saves only{" "}
                  <span className="text-foreground">~6% vs Opus-everywhere</span>,
                  because incident investigation is irreducibly reasoning-heavy —
                  ~80–85% of every run is the investigate step, which is{" "}
                  <em>already</em> on Opus. So I keep Opus where correctness
                  matters and spend the tiny savings budget on the validation
                  loop instead of cutting corners on reasoning.
                </p>
              </>
            }
            bullets={[
              <>
                <code>triage</code> → Haiku, <code>investigate</code> /{" "}
                <code>validate</code> → Opus, <code>report</code> → Sonnet
              </>,
              "Per-run summary: per-role tokens, cost, latency, and a measured tiered-vs-Opus factor (~1.06×)",
              "Prometheus metrics (LLM tokens, node duration, tool calls) exported for every run",
              "The honest finding drives the roadmap below — measurement pointing at the real bottleneck, not the assumed one",
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

        {/* Roadmap */}
        <section className="mt-12">
          <h2 className="text-2xl font-semibold">
            What the data revealed — and what&rsquo;s next
          </h2>
          <p className="mt-3 leading-relaxed text-muted-foreground">
            The measurements point at the real bottleneck, and — elegantly — the
            validator&rsquo;s own complaints double as the optimization backlog.
            The investigate step re-sends 27k–39k input tokens across 9–11 ReAct
            calls, and the validator keeps flagging tools that don&rsquo;t echo
            what they were queried for and logs it sees duplicated. Each is a
            concrete, measured next move:
          </p>
          <ul className="mt-4 space-y-2 list-disc pl-5 text-sm text-muted-foreground">
            <li>
              <span className="text-foreground">Prompt caching</span> on the
              investigate loop — cache the stable prefix re-sent every ReAct
              step. Cuts input cost sharply at zero quality cost. (Honest caveat:
              investigate&rsquo;s output tokens are the larger half, so this
              dents cost, it doesn&rsquo;t halve it.)
            </li>
            <li>
              <span className="text-foreground">Tools echo the queried
              indicator</span> — directly fixes the validator&rsquo;s most common
              complaint and improves grounding: a correctness <em>and</em> cost
              win.
            </li>
            <li>
              <span className="text-foreground">Deduplicate evidence</span> — the
              validator flagged byte-identical log copies; removing them means
              fewer tokens and cleaner evidence weighting.
            </li>
            <li>
              <span className="text-foreground">Re-measure</span> — publish a
              before/after so the progress arc is provable, not asserted.
            </li>
          </ul>
          <p className="mt-4 text-sm text-muted-foreground">
            That is the loop worth showing: build → measure → let the system tell
            you what to fix → measure again.
          </p>
        </section>
      </div>
    </div>
  );
}
