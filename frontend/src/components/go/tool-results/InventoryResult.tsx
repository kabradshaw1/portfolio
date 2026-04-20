export function InventoryResult({
  productId: _productId,
  stock,
  inStock,
}: {
  productId: string;
  stock: number;
  inStock: boolean;
}) {
  return (
    <div className="flex items-center gap-2">
      <span className="text-sm" aria-hidden>
        {inStock ? "✅" : "❌"}
      </span>
      <div className="text-xs">
        {inStock ? (
          <span>
            <span className="font-semibold text-green-600">{stock}</span> in
            stock
          </span>
        ) : (
          <span className="font-semibold text-red-600">Out of stock</span>
        )}
      </div>
    </div>
  );
}
