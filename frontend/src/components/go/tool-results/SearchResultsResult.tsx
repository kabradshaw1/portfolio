import type { SearchChunk } from "./types";

export function SearchResultsResult({
  results,
}: {
  results: SearchChunk[];
}) {
  if (results.length === 0) {
    return (
      <p className="text-xs text-muted-foreground">
        No matching documents found.
      </p>
    );
  }

  return (
    <div className="space-y-2">
      {results.map((r, i) => (
        <div key={i} className="rounded border bg-background p-2">
          <p className="text-xs leading-relaxed line-clamp-3">{r.text}</p>
          <div className="mt-1 flex flex-wrap gap-1">
            <span className="rounded bg-blue-950 px-2 py-0.5 text-[10px] text-blue-300">
              📄 {r.filename}, p.{r.page_number}
            </span>
            <span className="rounded bg-muted px-2 py-0.5 text-[10px] text-muted-foreground">
              {(r.score * 100).toFixed(0)}% match
            </span>
          </div>
        </div>
      ))}
    </div>
  );
}
