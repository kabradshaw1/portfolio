import type { OrderSummary } from "./types";

function formatPrice(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

const STATUS_COLORS: Record<string, string> = {
  pending: "bg-yellow-900 text-yellow-300",
  confirmed: "bg-blue-900 text-blue-300",
  shipped: "bg-purple-900 text-purple-300",
  delivered: "bg-green-900 text-green-300",
  cancelled: "bg-red-900 text-red-300",
  returned: "bg-gray-800 text-gray-300",
};

export function OrderListResult({ orders }: { orders: OrderSummary[] }) {
  if (orders.length === 0) {
    return <p className="text-xs text-muted-foreground">No orders found.</p>;
  }

  return (
    <div className="divide-y divide-border">
      {orders.map((o) => (
        <div key={o.id} className="flex items-center justify-between py-2">
          <div>
            <div className="text-xs font-semibold">
              Order #{o.id.slice(0, 8)}
            </div>
            <div className="text-[10px] text-muted-foreground">
              {formatDate(o.created_at)}
            </div>
          </div>
          <div className="flex items-center gap-2">
            <span
              className={`rounded px-2 py-0.5 text-[10px] ${STATUS_COLORS[o.status] ?? "bg-muted text-muted-foreground"}`}
            >
              {o.status}
            </span>
            <span className="text-xs font-semibold">
              {formatPrice(o.total)}
            </span>
          </div>
        </div>
      ))}
    </div>
  );
}
