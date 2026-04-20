import type { ProductItem } from "./types";

function formatPrice(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

export function ProductCardResult({ product }: { product: ProductItem }) {
  return (
    <div>
      <div className="text-sm font-semibold">{product.name}</div>
      {product.category && (
        <div className="text-[10px] uppercase text-muted-foreground">
          {product.category}
        </div>
      )}
      <div className="mt-1 text-lg font-bold text-green-600">
        {formatPrice(product.price)}
      </div>
      {product.stock !== undefined && (
        <div className="mt-1 text-[10px] text-muted-foreground">
          {product.stock > 0 ? `${product.stock} in stock` : "Out of stock"}
        </div>
      )}
    </div>
  );
}
