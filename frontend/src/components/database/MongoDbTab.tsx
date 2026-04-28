import Link from "next/link";

export function MongoDbTab() {
  return (
    <div data-testid="mongodb-tab" className="space-y-6">
      <p className="text-muted-foreground leading-relaxed">
        MongoDB powers the activity feed and analytics aggregations in the
        Java task-management portfolio — document-shaped activity events
        and time-bucketed aggregations served back through GraphQL. A
        dedicated MongoDB section is on the way; for now, the running code
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
