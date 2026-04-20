"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { useParams, useRouter } from "next/navigation";
import { useGoAuth } from "@/components/go/GoAuthProvider";
import { useGoCart } from "@/components/go/GoCartProvider";
import { goApiFetch } from "@/lib/go-api";
import { GO_PRODUCT_URL } from "@/lib/go-auth";

interface Product {
  id: string;
  name: string;
  description: string;
  category: string;
  price: number;
  stock: number;
  imageUrl?: string;
}

function formatPrice(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

export default function ProductDetailPage() {
  const params = useParams<{ productId: string }>();
  const router = useRouter();
  const { isLoggedIn } = useGoAuth();
  const { items: cartItems, refresh } = useGoCart();
  const [product, setProduct] = useState<Product | null>(null);
  const [adding, setAdding] = useState(false);
  const [added, setAdded] = useState(false);
  const [error, setError] = useState("");
  const [quantity, setQuantity] = useState(1);
  const [removing, setRemoving] = useState(false);

  const cartEntry = product
    ? cartItems.find((i) => i.productId === product.id)
    : undefined;

  async function removeFromCart() {
    if (!cartEntry) return;
    setError("");
    setRemoving(true);
    try {
      const res = await goApiFetch(`/cart/${cartEntry.id}`, {
        method: "DELETE",
      });
      if (res.ok) {
        await refresh();
      } else if (res.status === 401 || res.status === 403) {
        router.push(`/go/login?next=/go/ecommerce/${cartEntry.productId}`);
      } else {
        setError("Failed to remove from cart. Please try again.");
      }
    } finally {
      setRemoving(false);
    }
  }

  useEffect(() => {
    fetch(`${GO_PRODUCT_URL}/products/${params.productId}`)
      .then((r) => {
        if (!r.ok) throw new Error("Not found");
        return r.json();
      })
      .then(setProduct)
      .catch(() => {});
  }, [params.productId]);

  async function addToCart() {
    if (!product) return;
    if (!isLoggedIn) {
      router.push(`/go/login?next=/go/ecommerce/${product.id}`);
      return;
    }
    setError("");
    setAdding(true);
    try {
      const res = await goApiFetch("/cart", {
        method: "POST",
        body: JSON.stringify({ productId: product.id, quantity }),
      });
      if (res.ok) {
        setAdded(true);
        await refresh();
        setTimeout(() => setAdded(false), 2000);
      } else if (res.status === 401 || res.status === 403) {
        router.push(`/go/login?next=/go/ecommerce/${product.id}`);
      } else {
        setError("Failed to add to cart. Please try again.");
      }
    } finally {
      setAdding(false);
    }
  }

  if (!product) {
    return (
      <div className="mx-auto max-w-3xl px-6 py-12">
        <p className="text-muted-foreground">Loading...</p>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <Link
        href="/go/ecommerce"
        className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground transition-colors"
      >
        <ArrowLeft className="size-4" />
        Back to store
      </Link>
      <div className="mt-6 grid gap-8 md:grid-cols-2">
        <div className="aspect-square rounded-lg bg-muted flex items-center justify-center overflow-hidden">
          {product.imageUrl ? (
            <img
              src={product.imageUrl}
              alt={product.name}
              className="size-full object-cover"
            />
          ) : (
            <span className="text-5xl text-muted-foreground/40">&#128722;</span>
          )}
        </div>

        <div>
          <p className="text-xs uppercase tracking-wider text-muted-foreground">
            {product.category}
          </p>
          <h1 className="mt-2 text-2xl font-bold">{product.name}</h1>
          <p className="mt-4 text-xl font-semibold">
            {formatPrice(product.price)}
          </p>
          <p className="mt-4 text-muted-foreground leading-relaxed">
            {product.description}
          </p>
          <p className="mt-4 text-sm text-muted-foreground">
            {product.stock > 0
              ? `${product.stock} in stock`
              : "Out of stock"}
          </p>

          {product.stock > 0 && (
            <div className="mt-6 flex items-center gap-3">
              <div className="flex items-center rounded-lg border border-foreground/20">
                <button
                  type="button"
                  onClick={() => setQuantity((q) => Math.max(1, q - 1))}
                  disabled={quantity <= 1 || adding}
                  className="px-3 py-2 text-sm text-muted-foreground hover:text-foreground transition-colors disabled:opacity-30"
                  aria-label="Decrease quantity"
                >
                  −
                </button>
                <span className="min-w-8 text-center text-sm font-medium">
                  {quantity}
                </span>
                <button
                  type="button"
                  onClick={() =>
                    setQuantity((q) => Math.min(product.stock, q + 1))
                  }
                  disabled={quantity >= product.stock || adding}
                  className="px-3 py-2 text-sm text-muted-foreground hover:text-foreground transition-colors disabled:opacity-30"
                  aria-label="Increase quantity"
                >
                  +
                </button>
              </div>
              <button
                onClick={addToCart}
                disabled={adding}
                className="rounded-lg bg-primary px-6 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors disabled:opacity-50"
              >
                {added ? "Added!" : adding ? "Adding..." : "Add to Cart"}
              </button>
            </div>
          )}
          {cartEntry && (
            <div className="mt-4 flex items-center gap-3 text-sm">
              <span className="text-muted-foreground">
                {cartEntry.quantity} in cart
              </span>
              <button
                type="button"
                onClick={removeFromCart}
                disabled={removing}
                className="text-red-500 hover:text-red-400 transition-colors disabled:opacity-50"
              >
                {removing ? "Removing…" : "Remove from cart"}
              </button>
            </div>
          )}
          {error && <p className="mt-3 text-sm text-red-500">{error}</p>}
        </div>
      </div>
    </div>
  );
}
