"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { ProductCard } from "@/components/go/ProductCard";
import { useGoAuth } from "@/components/go/GoAuthProvider";
import { GO_ECOMMERCE_URL } from "@/lib/go-auth";

interface Product {
  id: string;
  name: string;
  category: string;
  priceCents: number;
  imageUrl?: string;
}

interface Category {
  id: string;
  name: string;
}

export default function EcommercePage() {
  const { user, isLoggedIn, login, register, logout } = useGoAuth();
  const [products, setProducts] = useState<Product[]>([]);
  const [categories, setCategories] = useState<Category[]>([]);
  const [activeCategory, setActiveCategory] = useState<string | null>(null);
  const [authMode, setAuthMode] = useState<"login" | "register">("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [name, setName] = useState("");
  const [authError, setAuthError] = useState("");

  useEffect(() => {
    fetch(`${GO_ECOMMERCE_URL}/products`)
      .then((r) => r.json())
      .then((data) => setProducts(data ?? []))
      .catch(() => {});
    fetch(`${GO_ECOMMERCE_URL}/categories`)
      .then((r) => r.json())
      .then((data) => setCategories(data ?? []))
      .catch(() => {});
  }, []);

  const handleAuth = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      setAuthError("");
      try {
        if (authMode === "login") {
          await login(email, password);
        } else {
          await register(email, password, name);
        }
        setEmail("");
        setPassword("");
        setName("");
      } catch (err) {
        setAuthError(err instanceof Error ? err.message : "Auth failed");
      }
    },
    [authMode, email, password, name, login, register],
  );

  const filtered = activeCategory
    ? products.filter((p) => p.category === activeCategory)
    : products;

  return (
    <div className="mx-auto max-w-5xl px-6 py-8">
      {/* Header bar */}
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-foreground/10 pb-4">
        <h1 className="text-2xl font-bold">Store</h1>

        {isLoggedIn ? (
          <div className="flex items-center gap-4 text-sm">
            <span className="text-muted-foreground">{user?.name}</span>
            <Link
              href="/go/ecommerce/cart"
              className="text-muted-foreground hover:text-foreground transition-colors"
            >
              Cart
            </Link>
            <Link
              href="/go/ecommerce/orders"
              className="text-muted-foreground hover:text-foreground transition-colors"
            >
              Orders
            </Link>
            <button
              onClick={logout}
              className="text-muted-foreground hover:text-foreground transition-colors"
            >
              Sign out
            </button>
          </div>
        ) : (
          <form onSubmit={handleAuth} className="flex flex-wrap items-end gap-2 text-sm">
            {authMode === "register" && (
              <input
                type="text"
                placeholder="Name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
                className="rounded border border-foreground/20 bg-background px-2 py-1 text-sm"
              />
            )}
            <input
              type="email"
              placeholder="Email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
              className="rounded border border-foreground/20 bg-background px-2 py-1 text-sm"
            />
            <input
              type="password"
              placeholder="Password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              className="rounded border border-foreground/20 bg-background px-2 py-1 text-sm"
            />
            <button
              type="submit"
              className="rounded bg-primary px-3 py-1 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
            >
              {authMode === "login" ? "Sign in" : "Register"}
            </button>
            <button
              type="button"
              onClick={() => setAuthMode(authMode === "login" ? "register" : "login")}
              className="text-muted-foreground hover:text-foreground transition-colors"
            >
              {authMode === "login" ? "Register instead" : "Sign in instead"}
            </button>
            {authError && (
              <span className="text-sm text-red-500">{authError}</span>
            )}
          </form>
        )}
      </div>

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
            key={cat.id}
            onClick={() => setActiveCategory(cat.name)}
            className={`rounded-full px-3 py-1 text-sm transition-colors ${
              activeCategory === cat.name
                ? "bg-primary text-primary-foreground"
                : "bg-muted text-muted-foreground hover:text-foreground"
            }`}
          >
            {cat.name}
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
            priceCents={product.priceCents}
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
