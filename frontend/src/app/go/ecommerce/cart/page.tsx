"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { ArrowLeft } from "lucide-react";
import { useGoCart } from "@/components/go/GoCartProvider";
import { goOrderFetch } from "@/lib/go-order-api";
import { goCartFetch } from "@/lib/go-cart-api";

interface CartItem {
  id: string;
  productId: string;
  productName: string;
  quantity: number;
  productPrice: number;
}

function formatPrice(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

export default function CartPage() {
  const router = useRouter();
  const { refresh } = useGoCart();
  const [items, setItems] = useState<CartItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [checkingOut, setCheckingOut] = useState(false);
  const [message, setMessage] = useState("");

  const fetchCart = useCallback(async () => {
    try {
      const res = await goCartFetch("/cart");
      if (res.status === 401 || res.status === 403) {
        router.replace("/go/login?next=/go/ecommerce/cart");
        return;
      }
      if (res.ok) {
        const data = await res.json();
        setItems(data?.items ?? []);
      }
    } finally {
      setLoading(false);
    }
  }, [router]);

  useEffect(() => {
    fetchCart();
  }, [fetchCart]);

  async function removeItem(itemId: string) {
    const res = await goCartFetch(`/cart/${itemId}`, {
      method: "DELETE",
    });
    if (res.status === 401 || res.status === 403) {
      router.replace("/go/login?next=/go/ecommerce/cart");
      return;
    }
    if (res.ok) {
      setItems((prev) => prev.filter((i) => i.id !== itemId));
      await refresh();
    }
  }

  async function checkout() {
    setCheckingOut(true);
    setMessage("");
    try {
      const res = await goOrderFetch("/orders", {
        method: "POST",
        headers: { "Idempotency-Key": crypto.randomUUID() },
      });
      if (res.status === 401 || res.status === 403) {
        router.replace("/go/login?next=/go/ecommerce/cart");
        return;
      }
      if (!res.ok) {
        const text = await res.text();
        setMessage(text || "Checkout failed");
        return;
      }

      const order = await res.json();
      await refresh();

      // Poll for checkout URL (saga creates payment asynchronously)
      const pollInterval = 1500;
      const maxAttempts = 10;

      for (let attempt = 0; attempt < maxAttempts; attempt++) {
        await new Promise((r) => setTimeout(r, pollInterval));
        const pollRes = await goOrderFetch(`/orders/${order.id}`);
        if (!pollRes.ok) continue;
        const updated = await pollRes.json();

        if (updated.checkoutUrl) {
          window.location.href = updated.checkoutUrl;
          return;
        }
        if (updated.status === "completed") {
          setItems([]);
          router.push(`/go/ecommerce/checkout/success?order=${order.id}`);
          return;
        }
        if (updated.status === "failed") {
          setMessage("Order failed. Please try again.");
          return;
        }
      }

      setMessage("Payment is taking longer than expected. Check your orders page for status.");
    } finally {
      setCheckingOut(false);
    }
  }

  const total = items.reduce(
    (sum, item) => sum + item.productPrice * item.quantity,
    0,
  );

  if (loading) {
    return (
      <div className="mx-auto max-w-3xl px-6 py-12">
        <p className="text-muted-foreground">Loading cart...</p>
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
      <h1 className="mt-6 text-2xl font-bold">Shopping Cart</h1>

      {items.length === 0 ? (
        <p className="mt-8 text-muted-foreground">
          {message || "Your cart is empty."}
        </p>
      ) : (
        <>
          <div className="mt-8 space-y-4">
            {items.map((item) => (
              <div
                key={item.id}
                className="flex items-center justify-between rounded-lg border border-foreground/10 p-4"
              >
                <div>
                  <p className="font-medium">{item.productName}</p>
                  <p className="text-sm text-muted-foreground">
                    Qty: {item.quantity} &times;{" "}
                    {formatPrice(item.productPrice)} ={" "}
                    {formatPrice(item.productPrice * item.quantity)}
                  </p>
                </div>
                <button
                  onClick={() => removeItem(item.id)}
                  className="text-sm text-red-500 hover:text-red-400 transition-colors"
                >
                  Remove
                </button>
              </div>
            ))}
          </div>

          <div className="mt-8 flex items-center justify-between border-t border-foreground/10 pt-4">
            <p className="text-lg font-semibold">Total: {formatPrice(total)}</p>
            <button
              onClick={checkout}
              disabled={checkingOut}
              className="rounded-lg bg-primary px-6 py-3 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors disabled:opacity-50"
            >
              {checkingOut ? "Processing payment..." : "Checkout"}
            </button>
          </div>

          {message && (
            <p className="mt-4 text-sm text-muted-foreground">{message}</p>
          )}
        </>
      )}
    </div>
  );
}
