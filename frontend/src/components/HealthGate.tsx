"use client";

import { useEffect, useState } from "react";
import Link from "next/link";

interface HealthGateProps {
  endpoint: string;
  stack: string;
  docsHref: string;
  children: React.ReactNode;
  degraded?: boolean;
}

export function HealthGate({ endpoint, stack, docsHref, children, degraded }: HealthGateProps) {
  const [status, setStatus] = useState<"checking" | "healthy" | "unhealthy">("checking");

  useEffect(() => {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 3000);

    fetch(endpoint, { signal: controller.signal })
      .then((res) => {
        setStatus(res.ok ? "healthy" : "unhealthy");
      })
      .catch(() => {
        setStatus("unhealthy");
      })
      .finally(() => {
        clearTimeout(timeout);
      });

    return () => {
      controller.abort();
      clearTimeout(timeout);
    };
  }, [endpoint]);

  if (status === "checking") {
    return (
      <div className="flex min-h-[60vh] items-center justify-center bg-background">
        <div className="h-6 w-6 animate-spin rounded-full border-2 border-muted-foreground border-t-transparent" />
      </div>
    );
  }

  if (status === "unhealthy" && !degraded) {
    const message =
      process.env.NEXT_PUBLIC_MAINTENANCE_MESSAGE ||
      "The backend services are currently offline for maintenance. Please check back later.";

    return (
      <div className="flex min-h-[60vh] items-center justify-center bg-background px-6">
        <div className="max-w-md text-center">
          <div className="text-5xl">🔧</div>
          <h2 className="mt-4 text-2xl font-bold text-foreground">
            Server Maintenance
          </h2>
          <p className="mt-3 text-muted-foreground">{message}</p>
          <div className="mt-6 rounded-lg border border-border bg-muted/50 px-4 py-3 text-sm text-muted-foreground">
            <strong className="text-foreground">{stack}</strong> is currently
            unavailable.
          </div>
          <Link
            href={docsHref}
            className="mt-6 inline-block text-sm text-primary hover:underline"
          >
            &larr; View documentation instead
          </Link>
        </div>
      </div>
    );
  }

  return (
    <>
      {status === "unhealthy" && degraded && (
        <div className="border-b border-amber-500/30 bg-amber-500/10 px-4 py-2 text-center text-sm text-amber-700 dark:text-amber-400">
          <strong>{stack}</strong> is currently unavailable &mdash; some features may not work.
        </div>
      )}
      {children}
    </>
  );
}
