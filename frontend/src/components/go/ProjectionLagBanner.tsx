"use client";

import { useEffect, useState } from "react";
import { goProjectorFetch } from "@/lib/go-projector-api";

interface HealthResponse {
  status: string;
  replaying?: boolean;
  lagSeconds?: number;
}

export default function ProjectionLagBanner() {
  const [health, setHealth] = useState<HealthResponse | null>(null);

  useEffect(() => {
    goProjectorFetch("/health")
      .then((r) => {
        if (!r.ok) return null;
        return r.json();
      })
      .then((data) => {
        if (data) setHealth(data);
      })
      .catch(() => {});
  }, []);

  if (!health) return null;

  if (health.replaying) {
    return (
      <div className="rounded-lg border border-blue-500/20 bg-blue-500/10 px-4 py-2 text-sm text-blue-600 dark:text-blue-400">
        Read models rebuilding — data may be temporarily incomplete.
      </div>
    );
  }

  if (health.lagSeconds !== undefined && health.lagSeconds > 5) {
    return (
      <div className="rounded-lg border border-yellow-500/20 bg-yellow-500/10 px-4 py-2 text-sm text-yellow-600 dark:text-yellow-400">
        Projection lag: {health.lagSeconds.toFixed(1)}s behind latest events
      </div>
    );
  }

  return null;
}
