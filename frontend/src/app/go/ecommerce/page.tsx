"use client";

import { useEffect, useState } from "react";
import { ProductCard } from "@/components/go/ProductCard";
import { useGoStore } from "@/components/go/GoStoreProvider";
import { GO_PRODUCT_URL } from "@/lib/go-auth";

interface Product {
  id: string;
  name: string;
  category: string;
  price: number;
  imageUrl?: string;
}

export default function EcommercePage() {
  const { activeCategory } = useGoStore();
  const [products, setProducts] = useState<Product[]>([]);

  useEffect(() => {
    fetch(`${GO_PRODUCT_URL}/products`)
      .then((r) => r.json())
      .then((data) => setProducts(data?.products ?? []))
      .catch(() => {});
  }, []);

  const filtered = activeCategory
    ? products.filter((p) => p.category === activeCategory)
    : products;

  return (
    <div className="mx-auto max-w-5xl px-6 py-8">
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4">
        {filtered.map((product) => (
          <ProductCard
            key={product.id}
            id={product.id}
            name={product.name}
            category={product.category}
            priceCents={product.price}
            imageUrl={product.imageUrl}
          />
        ))}
      </div>

      {filtered.length === 0 && (
        <p className="mt-12 text-center text-muted-foreground">
          No products found.
        </p>
      )}
    </div>
  );
}
