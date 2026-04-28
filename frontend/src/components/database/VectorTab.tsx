import Link from "next/link";

export function VectorTab() {
  return (
    <div data-testid="vector-tab" className="space-y-6">
      <p className="text-muted-foreground leading-relaxed">
        Qdrant backs the retrieval layer behind the Document Q&amp;A
        assistant and the code-aware Debug Assistant — chunked PDF
        embeddings, code-aware splitting, and approximate-nearest-neighbor
        search feeding RAG prompts. A dedicated vector-database section is
        on the way; for now, the running code and its supporting docs live
        on the AI page.
      </p>
      <div>
        <Link
          href="/ai"
          className="inline-flex items-center gap-2 rounded-lg border px-6 py-3 text-sm font-medium hover:bg-accent transition-colors"
        >
          View vector DB usage in /ai &rarr;
        </Link>
      </div>
    </div>
  );
}
