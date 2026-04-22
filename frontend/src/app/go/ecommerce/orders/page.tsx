"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { ArrowLeft } from "lucide-react";
import { goOrderFetch } from "@/lib/go-order-api";

interface Order {
  id: string;
  createdAt: string;
  total: number;
  status: string;
}

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
      return "bg-blue-500/10 text-blue-500";
    default:
      return "bg-muted text-muted-foreground";
  }
}

export default function OrdersPage() {
  const router = useRouter();
  const [orders, setOrders] = useState<Order[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    goOrderFetch("/orders")
      .then((r) => {
        if (r.status === 401 || r.status === 403) {
          router.replace("/go/login?next=/go/ecommerce/orders");
          return null;
        }
        if (!r.ok) throw new Error("Failed to load orders");
        return r.json();
      })
      .then((data) => {
        if (data) setOrders(data ?? []);
      })
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [router]);

  if (loading) {
    return (
      <div className="mx-auto max-w-3xl px-6 py-12">
        <p className="text-muted-foreground">Loading orders...</p>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <Link
        href="/go/ecommerce"
        className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground transition-colors"
      >
        <ArrowLeft className="size-4" />
        Back to store
      </Link>
      <h1 className="mt-6 text-2xl font-bold">Orders</h1>

      {orders.length === 0 ? (
        <p className="mt-8 text-muted-foreground">No orders yet.</p>
      ) : (
        <div className="mt-8 space-y-4">
          {orders.map((order) => (
            <div
              key={order.id}
              className="flex items-center justify-between rounded-lg border border-foreground/10 p-4"
            >
              <div>
                <p className="font-mono text-sm">
                  {order.id.slice(0, 8)}...
                </p>
                <p className="text-sm text-muted-foreground">
                  {new Date(order.createdAt).toLocaleDateString()}
                </p>
              </div>
              <div className="text-right">
                <p className="font-semibold">{formatPrice(order.total)}</p>
                <span className={`inline-block rounded-full px-2 py-0.5 text-xs font-medium ${statusBadge(order.status)}`}>
                  {order.status}
                </span>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
