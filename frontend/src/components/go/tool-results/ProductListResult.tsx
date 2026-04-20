import type { ProductItem } from "./types";

function formatPrice(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

export function ProductListResult({ products }: { products: ProductItem[] }) {
  if (products.length === 0) {
    return <p className="text-xs text-muted-foreground">No products found.</p>;
  }

  return (
    <div>
      <p className="mb-2 text-[10px] text-muted-foreground">
        {products.length} product{products.length !== 1 ? "s" : ""} found
      </p>
      <div className="divide-y divide-border">
        {products.map((p) => (
          <div key={p.id} className="flex items-center gap-3 py-2">
            <div className="flex h-8 w-8 items-center justify-center rounded bg-muted text-sm">
              🛒
            </div>
            <div className="flex-1 min-w-0">
              <div className="truncate text-xs font-semibold">{p.name}</div>
              {p.category && (
                <div className="text-[10px] text-muted-foreground">
                  {p.category}
                </div>
              )}
            </div>
            <div className="text-sm font-semibold text-green-600">
              {formatPrice(p.price)}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
