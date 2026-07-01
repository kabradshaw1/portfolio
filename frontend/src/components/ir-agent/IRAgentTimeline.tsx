import {
  AlertTriangle,
  CheckCircle2,
  FileText,
  Lightbulb,
  Search,
  ShieldAlert,
} from "lucide-react";
import type { ReactNode } from "react";
import type {
  EvidenceItem,
  Findings,
  IRAgentEvent,
  IRReport,
  Severity,
  TriageResult,
  ValidationVerdict,
} from "@/lib/ir-agent/types";

const SEVERITY_CLASS: Record<Severity, string> = {
  low: "bg-sky-500/15 text-sky-600 dark:text-sky-400 border-sky-500/30",
  medium: "bg-amber-500/15 text-amber-600 dark:text-amber-400 border-amber-500/30",
  high: "bg-orange-500/15 text-orange-600 dark:text-orange-400 border-orange-500/30",
  critical: "bg-red-500/15 text-red-600 dark:text-red-400 border-red-500/30",
};

function Pill({ children, className = "" }: { children: ReactNode; className?: string }) {
  return (
    <span
      className={`inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium ${className}`}
    >
      {children}
    </span>
  );
}

function SeverityPill({ severity }: { severity: Severity }) {
  return <Pill className={SEVERITY_CLASS[severity]}>{severity.toUpperCase()}</Pill>;
}

function NodeCard({
  icon,
  role,
  title,
  children,
}: {
  icon: ReactNode;
  role: string;
  title: string;
  children: ReactNode;
}) {
  return (
    <div className="rounded-xl border border-foreground/10 bg-card p-5">
      <div className="flex items-center gap-2">
        <span className="text-muted-foreground">{icon}</span>
        <span className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          {role}
        </span>
        <span className="text-sm font-semibold text-foreground">— {title}</span>
      </div>
      <div className="mt-3 text-sm text-muted-foreground">{children}</div>
    </div>
  );
}

function ChipList({ label, items }: { label: string; items: string[] }) {
  if (!items?.length) return null;
  return (
    <div className="mt-3">
      <div className="text-xs font-medium uppercase tracking-wide text-foreground/70">
        {label}
      </div>
      <div className="mt-1 flex flex-wrap gap-1.5">
        {items.map((item) => (
          <code
            key={item}
            className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs text-foreground"
          >
            {item}
          </code>
        ))}
      </div>
    </div>
  );
}

function TriageCard({ data }: { data: TriageResult }) {
  return (
    <NodeCard icon={<ShieldAlert size={16} />} role="Triage · Haiku" title="Initial assessment">
      <div className="flex flex-wrap items-center gap-2">
        <SeverityPill severity={data.severity} />
        <Pill className="border-foreground/15 bg-muted text-foreground/80">{data.category}</Pill>
        <Pill className="border-foreground/15 bg-muted text-foreground/80">
          confidence {(data.confidence * 100).toFixed(0)}%
        </Pill>
      </div>
      <p className="mt-3 leading-relaxed">{data.rationale}</p>
    </NodeCard>
  );
}

function EvidenceCard({ items }: { items: EvidenceItem[] }) {
  return (
    <NodeCard
      icon={<Search size={16} />}
      role="Investigate · Opus"
      title={`Gathered ${items.length} piece${items.length === 1 ? "" : "s"} of evidence`}
    >
      <ul className="space-y-2">
        {items.map((item) => (
          <li key={item.id} className="rounded-lg border border-foreground/10 bg-background/50 p-3">
            <div className="flex items-center gap-2">
              <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs text-foreground">
                {item.source_tool}
              </code>
              <span className="text-xs text-muted-foreground">“{item.query}”</span>
            </div>
            <pre className="mt-2 max-h-40 overflow-auto whitespace-pre-wrap break-words text-xs leading-relaxed text-foreground/80">
              {item.content}
            </pre>
          </li>
        ))}
      </ul>
    </NodeCard>
  );
}

function FindingsCard({ data }: { data: Findings }) {
  return (
    <NodeCard icon={<Lightbulb size={16} />} role="Investigate · Opus" title="Findings">
      <p className="leading-relaxed text-foreground/80">{data.hypothesis}</p>
      <p className="mt-2 leading-relaxed">{data.summary}</p>
      <ChipList label="IOCs" items={data.iocs} />
      <ChipList label="Affected assets" items={data.affected_assets} />
    </NodeCard>
  );
}

function VerdictCard({ data }: { data: ValidationVerdict }) {
  const ok = data.grounded && !data.needs_more_investigation;
  return (
    <NodeCard
      icon={ok ? <CheckCircle2 size={16} /> : <AlertTriangle size={16} />}
      role="Validate · Opus"
      title={ok ? "Findings grounded in evidence" : "Sent back for more investigation"}
    >
      <div className="flex flex-wrap gap-2">
        <Pill
          className={
            data.grounded
              ? "border-emerald-500/30 bg-emerald-500/15 text-emerald-600 dark:text-emerald-400"
              : "border-red-500/30 bg-red-500/15 text-red-600 dark:text-red-400"
          }
        >
          {data.grounded ? "grounded" : "ungrounded"}
        </Pill>
        {data.needs_more_investigation && (
          <Pill className="border-amber-500/30 bg-amber-500/15 text-amber-600 dark:text-amber-400">
            needs more investigation
          </Pill>
        )}
      </div>
      <ChipList label="Unsupported claims" items={data.unsupported_claims} />
      <ChipList label="Gaps" items={data.gaps} />
    </NodeCard>
  );
}

function ReportCard({ data }: { data: IRReport }) {
  return (
    <NodeCard icon={<FileText size={16} />} role="Report · Sonnet" title="Incident report">
      <div className="flex flex-wrap items-center gap-2">
        <SeverityPill severity={data.severity} />
        <Pill className="border-foreground/15 bg-muted text-foreground/80">
          confidence {(data.confidence * 100).toFixed(0)}%
        </Pill>
      </div>
      <p className="mt-3 leading-relaxed text-foreground">{data.executive_summary}</p>
      {data.timeline?.length > 0 && (
        <div className="mt-3">
          <div className="text-xs font-medium uppercase tracking-wide text-foreground/70">
            Timeline
          </div>
          <ol className="mt-1 list-decimal space-y-1 pl-5 leading-relaxed">
            {data.timeline.map((step, i) => (
              <li key={i}>{step}</li>
            ))}
          </ol>
        </div>
      )}
      <ChipList label="MITRE ATT&CK" items={data.mitre_attack} />
      <ChipList label="IOCs" items={data.iocs} />
      {data.recommended_containment?.length > 0 && (
        <div className="mt-3">
          <div className="text-xs font-medium uppercase tracking-wide text-foreground/70">
            Recommended containment
          </div>
          <ul className="mt-1 list-disc space-y-1 pl-5 leading-relaxed">
            {data.recommended_containment.map((rec, i) => (
              <li key={i}>{rec}</li>
            ))}
          </ul>
        </div>
      )}
    </NodeCard>
  );
}

/** Renders the ordered investigation events as a vertical pipeline timeline. */
export function IRAgentTimeline({ events }: { events: IRAgentEvent[] }) {
  if (events.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        Select an incident and run the investigation to watch the agents work.
      </p>
    );
  }

  return (
    <div className="space-y-4">
      {events.map((event, i) => {
        switch (event.event) {
          case "triage":
            return <TriageCard key={i} data={event.data} />;
          case "evidence":
            return <EvidenceCard key={i} items={event.data} />;
          case "findings":
            return <FindingsCard key={i} data={event.data} />;
          case "verdict":
            return <VerdictCard key={i} data={event.data} />;
          case "report":
            return <ReportCard key={i} data={event.data} />;
          case "error":
            return (
              <div
                key={i}
                className="rounded-lg border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive"
              >
                {event.data.detail}
              </div>
            );
          default:
            return null; // summary + done render in the outcomes panel
        }
      })}
    </div>
  );
}
