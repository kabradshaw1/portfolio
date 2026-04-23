"use client";

import { useEffect } from "react";
import Link from "next/link";

export default function EcommerceError({
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
      <h2 className="text-xl font-bold">Could not load this page</h2>
      <p className="mt-2 text-muted-foreground">
        {error.message || "The ecommerce service may be temporarily unavailable."}
      </p>
      <div className="mt-4 flex justify-center gap-3">
        <button
          onClick={() => unstable_retry()}
          className="rounded-lg bg-primary px-6 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
        >
          Try again
        </button>
        <Link
          href="/go/ecommerce"
          className="rounded-lg border px-6 py-2 text-sm font-medium hover:bg-accent transition-colors"
        >
          Back to store
        </Link>
      </div>
    </div>
  );
}
