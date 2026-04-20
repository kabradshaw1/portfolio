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

export function OrderCardResult({ order }: { order: OrderSummary }) {
  return (
    <div>
      <div className="text-xs font-semibold">Order #{order.id.slice(0, 8)}</div>
      <div className="mt-1 space-y-1 text-[10px] text-muted-foreground">
        <div>
          Status: <span className="font-semibold text-foreground">{order.status}</span>
        </div>
        <div>
          Total: <span className="font-semibold text-green-600">{formatPrice(order.total)}</span>
        </div>
        <div>Placed: {formatDate(order.created_at)}</div>
      </div>
    </div>
  );
}
