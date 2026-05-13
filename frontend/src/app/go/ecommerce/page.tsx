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
  const [status, setStatus] = useState<"loading" | "error" | "success">(
    "loading",
  );

  useEffect(() => {
    const controller = new AbortController();
    const params = new URLSearchParams({ limit: "100" });

    async function loadProducts() {
      try {
        const res = await fetch(`${GO_PRODUCT_URL}/products?${params.toString()}`, {
          signal: controller.signal,
        });

        if (!res.ok) {
          throw new Error(`Failed to load products: ${res.status}`);
        }

        const data = await res.json();

        if (!Array.isArray(data?.products)) {
          throw new Error("Product response did not include a products array");
        }

        setProducts(data.products);
        setStatus("success");
      } catch (error) {
        if (error instanceof DOMException && error.name === "AbortError") {
          return;
        }

        setProducts([]);
        setStatus("error");
      }
    }

    loadProducts();

    return () => {
      controller.abort();
    };
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

      {status === "loading" && (
        <p className="mt-12 text-center text-muted-foreground">
          Loading products...
        </p>
      )}

      {status === "error" && (
        <p className="mt-12 text-center text-muted-foreground">
          Products are temporarily unavailable.
        </p>
      )}

      {status === "success" && filtered.length === 0 && (
        <p className="mt-12 text-center text-muted-foreground">
          No products found.
        </p>
      )}
    </div>
  );
}
