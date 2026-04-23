"use client";

import { useEffect } from "react";

export default function AdminError({
  error,
  unstable_retry,
}: {
  error: Error & { digest?: string };
  unstable_retry: () => void;
}) {
  useEffect(() => {
    console.error(error);
  }, [error]);

  return (
    <div className="mx-auto max-w-3xl px-6 py-12 text-center">
      <h2 className="text-xl font-bold">Could not load admin panel</h2>
      <p className="mt-2 text-muted-foreground">
        {error.message || "The admin service may be temporarily unavailable."}
      </p>
      <button
        onClick={() => unstable_retry()}
        className="mt-4 rounded-lg bg-primary px-6 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
      >
        Try again
      </button>
    </div>
  );
}
