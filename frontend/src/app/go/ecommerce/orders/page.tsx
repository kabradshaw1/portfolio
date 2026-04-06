"use client";

import { useEffect, useState } from "react";
import { goApiFetch } from "@/lib/go-api";

interface Order {
  id: string;
  createdAt: string;
  totalCents: number;
  status: string;
}

function formatPrice(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

function statusColor(status: string): string {
  switch (status.toLowerCase()) {
    case "completed":
      return "text-green-500";
    case "processing":
      return "text-yellow-500";
    case "cancelled":
      return "text-red-500";
    case "pending":
      return "text-blue-500";
    default:
      return "text-muted-foreground";
  }
}

export default function OrdersPage() {
  const [orders, setOrders] = useState<Order[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    goApiFetch("/orders")
      .then((r) => {
        if (!r.ok) throw new Error("Failed to load orders");
        return r.json();
      })
      .then((data) => setOrders(data ?? []))
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  if (loading) {
    return (
      <div className="mx-auto max-w-3xl px-6 py-12">
        <p className="text-muted-foreground">Loading orders...</p>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <h1 className="text-2xl font-bold">Orders</h1>

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
                <p className="font-semibold">{formatPrice(order.totalCents)}</p>
                <p className={`text-sm font-medium ${statusColor(order.status)}`}>
                  {order.status}
                </p>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
