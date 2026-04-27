export type Gap = {
  title: string;
  description: string;
  source: string;
};

export function GapsGrid({ gaps }: { gaps: Gap[] }) {
  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
      {gaps.map((gap) => (
        <div
          key={gap.title}
          className="rounded-lg border border-foreground/10 bg-card p-4"
        >
          <div className="flex items-baseline justify-between gap-2">
            <h3 className="text-sm font-semibold">{gap.title}</h3>
            <span className="font-mono text-xs text-muted-foreground">
              {gap.source}
            </span>
          </div>
          <p className="mt-2 text-xs text-muted-foreground leading-relaxed">
            {gap.description}
          </p>
        </div>
      ))}
    </div>
  );
}
