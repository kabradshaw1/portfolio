import type { ReactNode } from "react";
import { adrLabel, type AdrId } from "@/lib/observability/adrs";

export function LessonCallout({
  adrIds,
  children,
}: {
  adrIds: AdrId[];
  children: ReactNode;
}) {
  return (
    <div className="mt-4 rounded-lg border border-foreground/10 bg-muted/30 p-4">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          Lessons from production
        </span>
        {adrIds.map((id) => (
          <span
            key={id}
            className="rounded bg-foreground/10 px-2 py-0.5 font-mono text-xs text-muted-foreground"
          >
            {adrLabel(id)}
          </span>
        ))}
      </div>
      <div className="mt-3 text-sm text-muted-foreground leading-relaxed">
        {children}
      </div>
    </div>
  );
}
