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
