export interface MetricCardData {
  name: string;
  labels: readonly string[];
  meaning: string;
  alertIntent: string;
}

export interface MetricsGridProps {
  metrics: readonly MetricCardData[];
}

export function MetricsGrid({ metrics }: MetricsGridProps) {
  return (
    <div
      className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3"
      data-testid="metrics-grid"
    >
      {metrics.map((m) => (
        <article
          key={m.name}
          className="flex flex-col rounded-lg border border-foreground/10 bg-card p-4"
        >
          <header>
            <code className="block font-mono text-sm text-foreground">
              {m.name}
            </code>
            {m.labels.length > 0 && (
              <ul className="mt-2 flex flex-wrap gap-1.5">
                {m.labels.map((label) => (
                  <li
                    key={label}
                    className="rounded bg-foreground/10 px-2 py-0.5 font-mono text-[11px] text-muted-foreground"
                  >
                    {label}
                  </li>
                ))}
              </ul>
            )}
          </header>
          <p className="mt-3 text-xs leading-relaxed text-muted-foreground">
            <span className="font-semibold uppercase tracking-wide">
              Meaning ·{" "}
            </span>
            {m.meaning}
          </p>
          <p className="mt-2 text-xs leading-relaxed text-muted-foreground">
            <span className="font-semibold uppercase tracking-wide">
              Alert ·{" "}
            </span>
            {m.alertIntent}
          </p>
        </article>
      ))}
    </div>
  );
}
