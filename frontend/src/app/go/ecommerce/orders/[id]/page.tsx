"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { ArrowLeft } from "lucide-react";
import {
  goProjectorFetch,
  type OrderSummary,
  type TimelineEvent,
} from "@/lib/go-projector-api";
import OrderTimeline from "@/components/go/OrderTimeline";

function formatPrice(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

function statusBadge(status: string): string {
  switch (status.toLowerCase()) {
    case "completed":
      return "bg-green-500/10 text-green-500";
    case "processing":
      return "bg-yellow-500/10 text-yellow-500";
    case "failed":
      return "bg-red-500/10 text-red-500";
    case "pending":
    case "created":
      return "bg-blue-500/10 text-blue-500";
    case "cancelled":
      return "bg-red-500/10 text-red-500";
    default:
      return "bg-muted text-muted-foreground";
  }
}

export default function OrderDetailPage() {
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const [summary, setSummary] = useState<OrderSummary | null>(null);
  const [events, setEvents] = useState<TimelineEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [lagMs, setLagMs] = useState<number | null>(null);

  useEffect(() => {
    if (!params.id) return;

    const orderId = params.id;

    Promise.all([
      goProjectorFetch(`/orders/${orderId}`),
      goProjectorFetch(`/orders/${orderId}/timeline`),
    ])
      .then(async ([summaryRes, timelineRes]) => {
        if (
          summaryRes.status === 401 ||
          summaryRes.status === 403 ||
          timelineRes.status === 401 ||
          timelineRes.status === 403
        ) {
          router.replace("/go/login?next=/go/ecommerce/orders");
          return;
        }

        // Check for projection lag header
        const lag = summaryRes.headers.get("X-Projection-Lag");
        if (lag) {
          const parsed = parseFloat(lag);
          if (!isNaN(parsed)) setLagMs(parsed);
        }

        if (summaryRes.ok) {
          const data = await summaryRes.json();
          setSummary(data);
        }
        if (timelineRes.ok) {
          const data = await timelineRes.json();
          setEvents(data.events ?? []);
        }
      })
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [params.id, router]);

  if (loading) {
    return (
      <div className="mx-auto max-w-3xl px-6 py-12">
        <p className="text-muted-foreground">Loading order...</p>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <Link
        href="/go/ecommerce/orders"
        className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground transition-colors"
      >
        <ArrowLeft className="size-4" />
        Back to orders
      </Link>

      {lagMs !== null && lagMs > 0 && (
        <div className="mt-4 rounded-lg border border-yellow-500/20 bg-yellow-500/10 px-4 py-2 text-sm text-yellow-600 dark:text-yellow-400">
          Projection lag: {(lagMs / 1000).toFixed(1)}s behind latest events
        </div>
      )}

      {summary ? (
        <div className="mt-6">
          <div className="flex items-start justify-between">
            <div>
              <h1 className="text-2xl font-bold">Order Details</h1>
              <p className="mt-1 font-mono text-sm text-muted-foreground">
                {summary.orderId}
              </p>
            </div>
            <span
              className={`inline-block rounded-full px-3 py-1 text-sm font-medium ${statusBadge(summary.status)}`}
            >
              {summary.status}
            </span>
          </div>

          <div className="mt-6 grid grid-cols-2 gap-4 rounded-lg border border-foreground/10 p-4">
            <div>
              <p className="text-xs text-muted-foreground">Total</p>
              <p className="text-lg font-semibold">
                {formatPrice(summary.totalCents)}
              </p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">Currency</p>
              <p className="text-lg font-semibold">
                {summary.currency || "USD"}
              </p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">Created</p>
              <p className="text-sm">
                {new Date(summary.createdAt).toLocaleString()}
              </p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">Updated</p>
              <p className="text-sm">
                {new Date(summary.updatedAt).toLocaleString()}
              </p>
            </div>
            {summary.completedAt && (
              <div>
                <p className="text-xs text-muted-foreground">Completed</p>
                <p className="text-sm">
                  {new Date(summary.completedAt).toLocaleString()}
                </p>
              </div>
            )}
            {summary.failureReason && (
              <div className="col-span-2">
                <p className="text-xs text-muted-foreground">Failure Reason</p>
                <p className="text-sm text-red-500">{summary.failureReason}</p>
              </div>
            )}
          </div>

          <h2 className="mt-8 text-lg font-semibold">Event Timeline</h2>
          <div className="mt-4">
            <OrderTimeline events={events} />
          </div>
        </div>
      ) : (
        <div className="mt-8">
          <p className="text-muted-foreground">Order not found.</p>
        </div>
      )}
    </div>
  );
}
