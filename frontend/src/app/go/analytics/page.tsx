"use client";

import { useEffect, useState, useCallback } from "react";
import Link from "next/link";
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from "recharts";

const ANALYTICS_URL =
  process.env.NEXT_PUBLIC_GO_ANALYTICS_URL || "http://localhost:8094";

const POLL_INTERVAL = 30_000; // 30 seconds

interface RevenueWindow {
  window_start: string;
  window_end: string;
  total_cents: number;
  order_count: number;
  avg_order_value_cents: number;
}

interface TrendingProduct {
  product_id: string;
  product_name: string;
  score: number;
  views: number;
  cart_adds: number;
}

interface TrendingData {
  window_end: string;
  products: TrendingProduct[];
  stale: boolean;
}

interface AbandonmentWindow {
  window_start: string;
  window_end: string;
  carts_started: number;
  carts_converted: number;
  carts_abandoned: number;
  abandonment_rate: number;
}

export default function AnalyticsPage() {
  const [revenue, setRevenue] = useState<RevenueWindow[]>([]);
  const [trending, setTrending] = useState<TrendingData | null>(null);
  const [abandonment, setAbandonment] = useState<AbandonmentWindow[]>([]);
  const [stale, setStale] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchAll = useCallback(async () => {
    try {
      const [revRes, trendRes, abandRes] = await Promise.all([
        fetch(`${ANALYTICS_URL}/analytics/revenue?hours=24`),
        fetch(`${ANALYTICS_URL}/analytics/trending?limit=10`),
        fetch(`${ANALYTICS_URL}/analytics/cart-abandonment?hours=12`),
      ]);

      if (revRes.ok) {
        const data = await revRes.json();
        setRevenue(data.windows ?? []);
        if (data.stale) setStale(true);
      }
      if (trendRes.ok) {
        const data = await trendRes.json();
        setTrending(data);
        if (data.stale) setStale(true);
      }
      if (abandRes.ok) {
        const data = await abandRes.json();
        setAbandonment(data.windows ?? []);
        if (data.stale) setStale(true);
      }
      setError(null);
    } catch {
      setError("Unable to reach analytics service");
    }
  }, []);

  useEffect(() => {
    let cancelled = false;

    async function poll() {
      if (!cancelled) await fetchAll();
    }

    poll();
    const interval = setInterval(poll, POLL_INTERVAL);
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [fetchAll]);

  const totalRevenue = revenue.reduce((sum, w) => sum + w.total_cents, 0);
  const totalOrders = revenue.reduce((sum, w) => sum + w.order_count, 0);
  const avgOrderValue =
    totalOrders > 0 ? totalRevenue / totalOrders : 0;

  const latestAbandonment =
    abandonment.length > 0 ? abandonment[abandonment.length - 1] : null;

  const revenueChartData = revenue.map((w) => ({
    hour: w.window_start,
    revenue: w.total_cents / 100,
  }));

  const abandonmentChartData = abandonment.map((w) => ({
    slot: w.window_start,
    rate: w.abandonment_rate * 100,
  }));

  return (
    <div className="mx-auto max-w-5xl px-6 py-8">
      <h1 className="mb-2 text-2xl font-bold">Kafka Streaming Analytics</h1>
      <p className="mb-6 text-sm text-muted-foreground">
        Real-time ecommerce metrics powered by Apache Kafka consumer groups and
        in-memory sliding window aggregation. Events are published by the
        order-service and cart-service as part of normal operations, consumed by
        the analytics-service, and aggregated into the dashboards below. Data
        refreshes every 30 seconds.
      </p>

      {stale && (
        <div className="mb-4 rounded border border-muted-foreground/20 bg-muted px-4 py-3 text-sm text-muted-foreground">
          No recent activity. Place orders in the{" "}
          <Link href="/go/ecommerce" className="underline hover:text-foreground">Store</Link>{" "}
          to see live metrics appear here.
        </div>
      )}

      {error && (
        <div className="mb-4 rounded border border-red-500/30 bg-red-500/10 px-4 py-2 text-sm text-red-600 dark:text-red-400">
          {error}
        </div>
      )}

      {/* Revenue per Hour */}
      <div className="mb-8">
        <h2 className="mb-3 text-lg font-semibold">Revenue per Hour</h2>
        <div className="mb-4 grid grid-cols-3 gap-4">
          <StatCard
            label="Total Revenue (24h)"
            value={`$${(totalRevenue / 100).toFixed(2)}`}
          />
          <StatCard
            label="Total Orders (24h)"
            value={totalOrders.toString()}
          />
          <StatCard
            label="Avg Order Value"
            value={`$${(avgOrderValue / 100).toFixed(2)}`}
          />
        </div>
        <div className="rounded border bg-card p-4">
          {revenueChartData.length > 0 ? (
            <ResponsiveContainer width="100%" height={250}>
              <BarChart data={revenueChartData}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis
                  dataKey="hour"
                  tickFormatter={(v: string) =>
                    new Date(v).toLocaleTimeString([], {
                      hour: "2-digit",
                      minute: "2-digit",
                    })
                  }
                  fontSize={12}
                />
                <YAxis
                  tickFormatter={(v: number) => `$${v.toFixed(0)}`}
                  fontSize={12}
                />
                <Tooltip
                  labelFormatter={(v) => new Date(String(v)).toLocaleString()}
                  formatter={(value) => [`$${Number(value).toFixed(2)}`, "Revenue"]}
                />
                <Bar
                  dataKey="revenue"
                  fill="hsl(var(--primary))"
                />
              </BarChart>
            </ResponsiveContainer>
          ) : (
            <p className="py-8 text-center text-muted-foreground">
              No revenue data yet
            </p>
          )}
        </div>
      </div>

      {/* Trending Products */}
      <div className="mb-8">
        <h2 className="mb-3 text-lg font-semibold">Trending Products</h2>
        <div className="rounded border bg-card">
          {trending?.products && trending.products.length > 0 ? (
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b text-left text-muted-foreground">
                  <th className="px-4 py-2">#</th>
                  <th className="px-4 py-2">Product</th>
                  <th className="px-4 py-2 text-right">Score</th>
                  <th className="px-4 py-2 text-right">Views</th>
                  <th className="px-4 py-2 text-right">Cart Adds</th>
                </tr>
              </thead>
              <tbody>
                {trending.products.map((p, i) => (
                  <tr key={p.product_id} className="border-b last:border-0">
                    <td className="px-4 py-2 text-muted-foreground">
                      {i + 1}
                    </td>
                    <td className="px-4 py-2 font-medium">
                      {p.product_name || p.product_id}
                    </td>
                    <td className="px-4 py-2 text-right font-semibold">
                      {p.score}
                    </td>
                    <td className="px-4 py-2 text-right">{p.views}</td>
                    <td className="px-4 py-2 text-right">{p.cart_adds}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          ) : (
            <p className="px-4 py-8 text-center text-muted-foreground">
              No trending products yet
            </p>
          )}
        </div>
      </div>

      {/* Cart Abandonment */}
      <div className="mb-8">
        <h2 className="mb-3 text-lg font-semibold">Cart Abandonment</h2>
        <div className="mb-4 grid grid-cols-3 gap-4">
          <StatCard
            label="Abandonment Rate"
            value={
              latestAbandonment
                ? `${(latestAbandonment.abandonment_rate * 100).toFixed(1)}%`
                : "---"
            }
            className="text-amber-500"
          />
          <StatCard
            label="Carts Started"
            value={latestAbandonment?.carts_started.toString() ?? "---"}
          />
          <StatCard
            label="Carts Converted"
            value={latestAbandonment?.carts_converted.toString() ?? "---"}
          />
        </div>
        <div className="rounded border bg-card p-4">
          {abandonmentChartData.length > 0 ? (
            <ResponsiveContainer width="100%" height={250}>
              <BarChart data={abandonmentChartData}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis
                  dataKey="slot"
                  tickFormatter={(v: string) =>
                    new Date(v).toLocaleTimeString([], {
                      hour: "2-digit",
                      minute: "2-digit",
                    })
                  }
                  fontSize={12}
                />
                <YAxis
                  tickFormatter={(v: number) => `${v.toFixed(0)}%`}
                  fontSize={12}
                />
                <Tooltip
                  labelFormatter={(v) => new Date(String(v)).toLocaleString()}
                  formatter={(value) => [`${Number(value).toFixed(1)}%`, "Abandonment Rate"]}
                />
                <Bar
                  dataKey="rate"
                  fill="#f59e0b"
                />
              </BarChart>
            </ResponsiveContainer>
          ) : (
            <p className="py-8 text-center text-muted-foreground">
              No cart abandonment data yet
            </p>
          )}
        </div>
      </div>
    </div>
  );
}

function StatCard({
  label,
  value,
  className,
}: {
  label: string;
  value: string;
  className?: string;
}) {
  return (
    <div className="rounded border bg-card px-4 py-3">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className={`mt-1 text-2xl font-bold ${className ?? ""}`}>{value}</p>
    </div>
  );
}
