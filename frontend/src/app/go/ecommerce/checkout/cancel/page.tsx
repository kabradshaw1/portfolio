"use client";

import Link from "next/link";
import { XCircle } from "lucide-react";

export default function CheckoutCancelPage() {
  return (
    <div className="mx-auto max-w-lg px-6 py-20 text-center">
      <XCircle className="mx-auto size-16 text-red-500" />
      <h1 className="mt-6 text-2xl font-bold">Payment Cancelled</h1>
      <p className="mt-2 text-muted-foreground">
        Your cart is still saved. You can try again whenever you&apos;re ready.
      </p>
      <div className="mt-8 flex justify-center gap-4">
        <Link
          href="/go/ecommerce/cart"
          className="rounded-lg bg-primary px-6 py-3 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
        >
          Back to Cart
        </Link>
        <Link
          href="/go/ecommerce"
          className="rounded-lg border border-foreground/10 px-6 py-3 text-sm font-medium hover:bg-muted transition-colors"
        >
          Continue Shopping
        </Link>
      </div>
    </div>
  );
}
