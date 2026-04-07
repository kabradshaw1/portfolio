"use client";

import { useEffect, useState } from "react";
import { ProductCard } from "@/components/go/ProductCard";
import { GO_ECOMMERCE_URL } from "@/lib/go-auth";

interface Product {
  id: string;
  name: string;
  category: string;
  price: number;
  imageUrl?: string;
}

type Category = string;

export default function EcommercePage() {
  const [products, setProducts] = useState<Product[]>([]);
  const [categories, setCategories] = useState<Category[]>([]);
  const [activeCategory, setActiveCategory] = useState<string | null>(null);

  useEffect(() => {
    fetch(`${GO_ECOMMERCE_URL}/products`)
      .then((r) => r.json())
      .then((data) => setProducts(data?.products ?? []))
      .catch(() => {});
    fetch(`${GO_ECOMMERCE_URL}/categories`)
      .then((r) => r.json())
      .then((data) => setCategories(data?.categories ?? []))
      .catch(() => {});
  }, []);

  const filtered = activeCategory
    ? products.filter((p) => p.category === activeCategory)
    : products;

  return (
    <div className="mx-auto max-w-5xl px-6 py-8">
      <h1 className="text-2xl font-bold">Store</h1>

      {/* Category filter */}
      <div className="mt-6 flex flex-wrap gap-2">
        <button
          onClick={() => setActiveCategory(null)}
          className={`rounded-full px-3 py-1 text-sm transition-colors ${
            activeCategory === null
              ? "bg-primary text-primary-foreground"
              : "bg-muted text-muted-foreground hover:text-foreground"
          }`}
        >
          All
        </button>
        {categories.map((cat) => (
          <button
            key={cat}
            onClick={() => setActiveCategory(cat)}
            className={`rounded-full px-3 py-1 text-sm transition-colors ${
              activeCategory === cat
                ? "bg-primary text-primary-foreground"
                : "bg-muted text-muted-foreground hover:text-foreground"
            }`}
          >
            {cat}
          </button>
        ))}
      </div>

      {/* Product grid */}
      <div className="mt-8 grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4">
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
