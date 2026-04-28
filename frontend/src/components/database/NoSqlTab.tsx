import Link from "next/link";

export function NoSqlTab() {
  return (
    <div data-testid="nosql-tab" className="space-y-6">
      <p className="text-muted-foreground leading-relaxed">
        MongoDB powers the activity feed and analytics aggregations in the
        Java task-management portfolio — document-shaped activity events,
        time-bucketed aggregations, and a Redis read-cache layered on top.
        A dedicated NoSQL section is on the way; for now, the running code
        and its supporting docs live on the Java page.
      </p>
      <div>
        <Link
          href="/java"
          className="inline-flex items-center gap-2 rounded-lg border px-6 py-3 text-sm font-medium hover:bg-accent transition-colors"
        >
          View MongoDB usage in /java &rarr;
        </Link>
      </div>
    </div>
  );
}
