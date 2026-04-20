import type { CartItem } from "./types";

function formatPrice(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

export function CartItemResult({ item }: { item: CartItem }) {
  return (
    <div className="flex items-center gap-2">
      <span className="text-sm" aria-hidden>
        ✅
      </span>
      <div>
        <div className="text-xs font-semibold">
          Added {item.product_name} to cart
        </div>
        <div className="text-[10px] text-muted-foreground">
          Qty: {item.quantity} — {formatPrice(item.product_price)}
        </div>
      </div>
    </div>
  );
}
