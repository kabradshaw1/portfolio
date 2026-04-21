import type { CartItem } from "./types";

function formatPrice(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

export function CartResult({
  cart,
}: {
  cart: { items: CartItem[]; total: number };
}) {
  if (cart.items.length === 0) {
    return <p className="text-xs text-muted-foreground">Your cart is empty.</p>;
  }

  return (
    <div>
      <div className="divide-y divide-border">
        {cart.items.map((item) => (
          <div key={item.id} className="flex items-center justify-between py-2">
            <div className="min-w-0 flex-1">
              <div className="truncate text-xs font-semibold">
                {item.product_name}
              </div>
              <div className="text-[10px] text-muted-foreground">
                Qty: {item.quantity} × {formatPrice(item.product_price)}
              </div>
            </div>
            <div className="text-xs font-semibold">
              {formatPrice(item.product_price * item.quantity)}
            </div>
          </div>
        ))}
      </div>
      <div className="mt-2 flex justify-between border-t pt-2">
        <span className="text-xs font-semibold">Total</span>
        <span className="text-sm font-bold text-green-600">
          {formatPrice(cart.total)}
        </span>
      </div>
    </div>
  );
}
