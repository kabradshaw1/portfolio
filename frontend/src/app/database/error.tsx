"use client";

export default function DatabaseError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <h1 className="text-2xl font-bold">Something went wrong on /database</h1>
      <p className="mt-4 text-muted-foreground">{error.message}</p>
      <button
        onClick={reset}
        className="mt-6 rounded-lg border px-4 py-2 text-sm hover:bg-accent transition-colors"
      >
        Try again
      </button>
    </div>
  );
}
