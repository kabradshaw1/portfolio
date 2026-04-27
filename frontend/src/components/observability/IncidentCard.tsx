import Link from "next/link";
import { adrLabel, adrUrl, type AdrId } from "@/lib/observability/adrs";

export type IncidentAccent = "orange" | "green" | "purple" | "red";

export type Incident = {
  date: string;
  title: string;
  namespace: string;
  accent: IncidentAccent;
  symptom: string;
  before: string;
  after: string;
  fixes: string[];
  adrId: AdrId;
};

const ACCENT_BORDER: Record<IncidentAccent, string> = {
  orange: "border-orange-500/60",
  green: "border-green-500/60",
  purple: "border-purple-500/60",
  red: "border-red-500/60",
};

function renderWithCode(text: string): React.ReactNode {
  const parts = text.split(/(`[^`]+`)/g);
  return parts.map((part, i) => {
    if (part.startsWith("`") && part.endsWith("`")) {
      return (
        <code
          key={i}
          className="rounded bg-muted px-1.5 py-0.5 text-xs font-mono"
        >
          {part.slice(1, -1)}
        </code>
      );
    }
    return <span key={i}>{part}</span>;
  });
}

export function IncidentCard({ incident }: { incident: Incident }) {
  const borderClass = ACCENT_BORDER[incident.accent];

  return (
    <div
      className={`rounded-xl border border-foreground/10 border-l-4 ${borderClass} bg-card p-5`}
    >
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <h3 className="text-lg font-semibold">{incident.title}</h3>
        <span className="rounded bg-muted px-2 py-0.5 font-mono text-xs text-muted-foreground">
          {incident.namespace}
        </span>
      </div>
      <p className="mt-3 border-l-2 border-foreground/20 pl-3 text-sm italic text-muted-foreground leading-relaxed">
        {incident.symptom}
      </p>

      <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2">
        <div className="rounded-lg border border-red-500/20 bg-red-500/5 p-3">
          <div className="text-xs font-semibold uppercase tracking-wide text-red-400">
            Before
          </div>
          <p className="mt-2 text-sm text-muted-foreground leading-relaxed">
            {renderWithCode(incident.before)}
          </p>
        </div>
        <div className="rounded-lg border border-green-500/20 bg-green-500/5 p-3">
          <div className="text-xs font-semibold uppercase tracking-wide text-green-400">
            After
          </div>
          <p className="mt-2 text-sm text-muted-foreground leading-relaxed">
            {renderWithCode(incident.after)}
          </p>
        </div>
      </div>

      <div className="mt-4 flex flex-wrap gap-2">
        {incident.fixes.map((fix) => (
          <code
            key={fix}
            className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs text-muted-foreground"
          >
            {fix}
          </code>
        ))}
      </div>

      <div className="mt-4 flex justify-end">
        <Link
          href={adrUrl(incident.adrId)}
          target="_blank"
          rel="noopener noreferrer"
          className="text-xs font-medium text-primary hover:underline"
        >
          Read {adrLabel(incident.adrId)} &rarr;
        </Link>
      </div>
    </div>
  );
}
