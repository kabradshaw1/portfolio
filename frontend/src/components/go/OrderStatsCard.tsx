"use client";

import { useEffect, useState } from "react";
import { goProjectorFetch, type OrderStats } from "@/lib/go-projector-api";

function formatPrice(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

export default function OrderStatsCard() {
  const [stats, setStats] = useState<OrderStats | null>(null);

  useEffect(() => {
    goProjectorFetch("/stats/orders?hours=24")
      .then((r) => {
        if (!r.ok) return null;
        return r.json();
      })
      .then((data) => {
        if (data) setStats(data);
      })
      .catch(() => {});
  }, []);

  if (!stats) return null;

  const completionRate =
    stats.ordersCreated > 0
      ? ((stats.ordersCompleted / stats.ordersCreated) * 100).toFixed(0)
      : "0";

  return (
    <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
      <div className="rounded-lg border border-foreground/10 p-4">
        <p className="text-xs text-muted-foreground">Orders (24h)</p>
        <p className="mt-1 text-2xl font-bold">{stats.ordersCreated}</p>
      </div>
      <div className="rounded-lg border border-foreground/10 p-4">
        <p className="text-xs text-muted-foreground">Completed</p>
        <p className="mt-1 text-2xl font-bold">{stats.ordersCompleted}</p>
      </div>
      <div className="rounded-lg border border-foreground/10 p-4">
        <p className="text-xs text-muted-foreground">Completion Rate</p>
        <p className="mt-1 text-2xl font-bold">{completionRate}%</p>
      </div>
      <div className="rounded-lg border border-foreground/10 p-4">
        <p className="text-xs text-muted-foreground">Revenue</p>
        <p className="mt-1 text-2xl font-bold">
          {formatPrice(stats.totalRevenueCents)}
        </p>
      </div>
    </div>
  );
}
