export function CollectionsResult({
  collections,
}: {
  collections: { name: string; point_count: number }[];
}) {
  if (collections.length === 0) {
    return (
      <p className="text-xs text-muted-foreground">No collections found.</p>
    );
  }

  return (
    <div className="divide-y divide-border">
      {collections.map((c) => (
        <div key={c.name} className="flex items-center justify-between py-1.5">
          <span className="text-xs font-semibold">{c.name}</span>
          <span className="text-[10px] text-muted-foreground">
            {c.point_count} chunks
          </span>
        </div>
      ))}
    </div>
  );
}
