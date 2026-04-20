import type { RagSource } from "./types";

export function RagAnswerResult({
  answer,
  sources,
}: {
  answer: string;
  sources: RagSource[];
}) {
  return (
    <div>
      <p className="text-xs leading-relaxed">{answer}</p>
      {sources.length > 0 && (
        <div className="mt-2 flex flex-wrap gap-1">
          {sources.map((s, i) => (
            <span
              key={i}
              className="rounded bg-blue-950 px-2 py-0.5 text-[10px] text-blue-300"
            >
              📄 {s.file}, p.{s.page}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}
