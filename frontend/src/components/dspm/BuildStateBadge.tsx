export type BuildState = "in-progress" | "designed" | "shipped";

export interface BuildStateBadgeProps {
  plan: 1 | 2 | 3;
  status: BuildState;
  label?: string;
}

const STATUS_LABEL: Record<BuildState, string> = {
  "in-progress": "in progress",
  designed: "designed",
  shipped: "shipped",
};

const STATUS_DOT: Record<BuildState, string> = {
  "in-progress": "bg-amber-500",
  designed: "bg-foreground/30",
  shipped: "bg-green-500",
};

export function BuildStateBadge({
  plan,
  status,
  label,
}: BuildStateBadgeProps) {
  const display = label ?? `Plan ${plan} — ${STATUS_LABEL[status]}`;
  return (
    <span
      className="inline-flex items-center gap-2 rounded-full border border-foreground/10 bg-muted/30 px-3 py-1 text-xs font-medium text-muted-foreground"
      data-testid="build-state-badge"
      data-status={status}
    >
      <span
        aria-hidden="true"
        className={`h-2 w-2 rounded-full ${STATUS_DOT[status]}`}
      />
      <span>{display}</span>
      <span className="sr-only"> ({STATUS_LABEL[status]})</span>
    </span>
  );
}
