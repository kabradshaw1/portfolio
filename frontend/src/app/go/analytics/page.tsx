"use client";

import { useEffect, useState, useCallback } from "react";
import Link from "next/link";
import {
  LineChart,
  Line,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from "recharts";
import { goOrderFetch } from "@/lib/go-order-api";

const ANALYTICS_URL =
  process.env.NEXT_PUBLIC_GO_ANALYTICS_URL || "http://localhost:8094";

const POLL_INTERVAL = 5000; // 5 seconds

interface DashboardData {
  ordersPerHour: number;
  revenuePerHour: number;
  completionRate: number;
  activeCarts: number;
  stale: boolean;
}

interface TrendingProduct {
  id: string;
  name: string;
  score: number;
  views: number;
  purchases: number;
}

interface TrendingData {
  products: TrendingProduct[];
  stale: boolean;
}

interface HourlyBucket {
  hour: string;
  count: number;
  revenue: number;
}

interface OrdersData {
  hourly: HourlyBucket[];
  statusBreakdown: {
    created: number;
    completed: number;
    failed: number;
  };
  stale: boolean;
}

interface SalesTrend {
  day: string;
  dailyRevenue: number;
  rolling7Day: number;
  rolling30Day: number;
}

interface ProductPerf {
  productId: string;
  productName: string;
  category: string;
  currentStock: number;
  totalUnitsSold: number;
  totalRevenueCents: number;
  totalOrders: number;
  avgOrderValueCents: number;
  returnCount: number;
  returnRatePct: number;
}

export default function AnalyticsPage() {
  const [dashboard, setDashboard] = useState<DashboardData | null>(null);
  const [trending, setTrending] = useState<TrendingData | null>(null);
  const [orders, setOrders] = useState<OrdersData | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [salesTrends, setSalesTrends] = useState<SalesTrend[]>([]);
  const [productPerf, setProductPerf] = useState<ProductPerf[]>([]);
  const [reportingError, setReportingError] = useState<string | null>(null);

  const fetchAll = useCallback(async () => {
    try {
      const [dashRes, trendRes, ordersRes] = await Promise.all([
        fetch(`${ANALYTICS_URL}/analytics/dashboard`),
        fetch(`${ANALYTICS_URL}/analytics/trending`),
        fetch(`${ANALYTICS_URL}/analytics/orders`),
      ]);

      if (dashRes.ok) setDashboard(await dashRes.json());
      if (trendRes.ok) setTrending(await trendRes.json());
      if (ordersRes.ok) setOrders(await ordersRes.json());
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

  useEffect(() => {
    async function fetchReporting() {
      try {
        const [trendsRes, perfRes] = await Promise.all([
          goOrderFetch("/reporting/sales-trends?days=30"),
          goOrderFetch("/reporting/product-performance"),
        ]);
        if (trendsRes.ok) {
          const data = await trendsRes.json();
          setSalesTrends(data.trends ?? []);
        }
        if (perfRes.ok) {
          const data = await perfRes.json();
          setProductPerf(data.products ?? []);
        }
      } catch {
        setReportingError("Unable to load reporting data");
      }
    }
    fetchReporting();
  }, []);

  const isStale = dashboard?.stale || trending?.stale || orders?.stale;

  return (
    <div className="mx-auto max-w-5xl px-6 py-8">
      <h1 className="mb-2 text-2xl font-bold">Kafka Streaming Analytics</h1>
      <p className="mb-6 text-sm text-muted-foreground">
        Real-time ecommerce metrics powered by Apache Kafka consumer groups and
        in-memory sliding window aggregation.
      </p>

      {isStale && (
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

      {/* Dashboard Cards */}
      <div className="mb-8 grid grid-cols-2 gap-4 sm:grid-cols-4">
        <StatCard
          label="Orders / Hour"
          value={dashboard?.ordersPerHour?.toFixed(1) ?? "—"}
        />
        <StatCard
          label="Revenue / Hour"
          value={
            dashboard?.revenuePerHour != null
              ? `$${dashboard.revenuePerHour.toFixed(2)}`
              : "—"
          }
        />
        <StatCard
          label="Completion Rate"
          value={
            dashboard?.completionRate != null
              ? `${(dashboard.completionRate * 100).toFixed(0)}%`
              : "—"
          }
        />
        <StatCard
          label="Active Carts"
          value={dashboard?.activeCarts?.toString() ?? "—"}
        />
      </div>

      {/* Order Volume Chart */}
      <div className="mb-8">
        <h2 className="mb-3 text-lg font-semibold">Order Volume (Hourly)</h2>
        <div className="rounded border bg-card p-4">
          {orders?.hourly && orders.hourly.length > 0 ? (
            <ResponsiveContainer width="100%" height={250}>
              <LineChart data={orders.hourly}>
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
                <YAxis fontSize={12} />
                <Tooltip
                  labelFormatter={(v) => new Date(String(v)).toLocaleString()}
                />
                <Line
                  type="monotone"
                  dataKey="count"
                  stroke="hsl(var(--primary))"
                  strokeWidth={2}
                  dot={false}
                />
              </LineChart>
            </ResponsiveContainer>
          ) : (
            <p className="py-8 text-center text-muted-foreground">
              No order data yet
            </p>
          )}
        </div>
      </div>

      {/* Status Breakdown */}
      {orders?.statusBreakdown && (
        <div className="mb-8">
          <h2 className="mb-3 text-lg font-semibold">Order Status Breakdown</h2>
          <div className="grid grid-cols-3 gap-4">
            <StatCard
              label="Created"
              value={orders.statusBreakdown.created.toString()}
            />
            <StatCard
              label="Completed"
              value={orders.statusBreakdown.completed.toString()}
            />
            <StatCard
              label="Failed"
              value={orders.statusBreakdown.failed.toString()}
            />
          </div>
        </div>
      )}

      {/* Trending Products */}
      <div>
        <h2 className="mb-3 text-lg font-semibold">Trending Products</h2>
        <div className="rounded border bg-card">
          {trending?.products && trending.products.length > 0 ? (
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b text-left text-muted-foreground">
                  <th className="px-4 py-2">#</th>
                  <th className="px-4 py-2">Product</th>
                  <th className="px-4 py-2 text-right">Views</th>
                  <th className="px-4 py-2 text-right">Purchases</th>
                  <th className="px-4 py-2 text-right">Score</th>
                </tr>
              </thead>
              <tbody>
                {trending.products.map((p, i) => (
                  <tr key={p.id} className="border-b last:border-0">
                    <td className="px-4 py-2 text-muted-foreground">
                      {i + 1}
                    </td>
                    <td className="px-4 py-2 font-medium">
                      {p.name || p.id}
                    </td>
                    <td className="px-4 py-2 text-right">{p.views}</td>
                    <td className="px-4 py-2 text-right">{p.purchases}</td>
                    <td className="px-4 py-2 text-right font-semibold">
                      {p.score}
                    </td>
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

      {/* Historical Reporting */}
      <div className="mt-12 border-t border-foreground/10 pt-8">
        <h2 className="mb-2 text-xl font-bold">Historical Reporting</h2>
        <p className="mb-6 text-sm text-muted-foreground">
          Pre-computed analytics from PostgreSQL materialized views with concurrent refresh.
          Revenue trends use rolling window functions over range-partitioned order data.
        </p>

        {reportingError && (
          <div className="mb-4 rounded border border-red-500/30 bg-red-500/10 px-4 py-2 text-sm text-red-600 dark:text-red-400">
            {reportingError}
          </div>
        )}

        {/* Sales Trends Chart */}
        <div className="mb-8">
          <h3 className="mb-3 text-lg font-semibold">Revenue Trends (30 Days)</h3>
          <div className="rounded border bg-card p-4">
            {salesTrends.length > 0 ? (
              <ResponsiveContainer width="100%" height={280}>
                <AreaChart data={salesTrends}>
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis
                    dataKey="day"
                    tickFormatter={(v: string) =>
                      new Date(v).toLocaleDateString([], { month: "short", day: "numeric" })
                    }
                    fontSize={12}
                  />
                  <YAxis
                    tickFormatter={(v: number) => `$${(v / 100).toFixed(0)}`}
                    fontSize={12}
                  />
                  <Tooltip
                    labelFormatter={(v) => new Date(String(v)).toLocaleDateString()}
                    formatter={(value, name) => [
                      typeof value === "number" ? `$${(value / 100).toFixed(2)}` : String(value ?? ""),
                      name === "rolling7Day" ? "7-Day Rolling" : "30-Day Rolling",
                    ]}
                  />
                  <Legend
                    formatter={(value: string) =>
                      value === "rolling7Day" ? "7-Day Rolling" : "30-Day Rolling"
                    }
                  />
                  <Area
                    type="monotone"
                    dataKey="rolling30Day"
                    stroke="hsl(var(--muted-foreground))"
                    fill="hsl(var(--muted-foreground) / 0.1)"
                    strokeWidth={1.5}
                    dot={false}
                  />
                  <Area
                    type="monotone"
                    dataKey="rolling7Day"
                    stroke="hsl(var(--primary))"
                    fill="hsl(var(--primary) / 0.15)"
                    strokeWidth={2}
                    dot={false}
                  />
                </AreaChart>
              </ResponsiveContainer>
            ) : (
              <p className="py-8 text-center text-muted-foreground">
                No revenue data yet
              </p>
            )}
          </div>
        </div>

        {/* Product Performance Table */}
        <div>
          <h3 className="mb-3 text-lg font-semibold">Product Performance</h3>
          <div className="rounded border bg-card">
            {productPerf.length > 0 ? (
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b text-left text-muted-foreground">
                    <th className="px-4 py-2">Product</th>
                    <th className="px-4 py-2">Category</th>
                    <th className="px-4 py-2 text-right">Units Sold</th>
                    <th className="px-4 py-2 text-right">Revenue</th>
                    <th className="px-4 py-2 text-right">Avg Order</th>
                    <th className="px-4 py-2 text-right">Return Rate</th>
                  </tr>
                </thead>
                <tbody>
                  {productPerf.map((p) => (
                    <tr key={p.productId} className="border-b last:border-0">
                      <td className="px-4 py-2 font-medium">{p.productName}</td>
                      <td className="px-4 py-2 text-muted-foreground">{p.category}</td>
                      <td className="px-4 py-2 text-right">{p.totalUnitsSold}</td>
                      <td className="px-4 py-2 text-right">
                        ${(p.totalRevenueCents / 100).toFixed(2)}
                      </td>
                      <td className="px-4 py-2 text-right">
                        ${(p.avgOrderValueCents / 100).toFixed(2)}
                      </td>
                      <td className="px-4 py-2 text-right">
                        {p.returnRatePct.toFixed(1)}%
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : (
              <p className="px-4 py-8 text-center text-muted-foreground">
                No product data yet
              </p>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

function StatCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded border bg-card px-4 py-3">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="mt-1 text-2xl font-bold">{value}</p>
    </div>
  );
}
