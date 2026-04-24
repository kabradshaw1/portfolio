"use client";

import { useEffect } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { CheckCircle } from "lucide-react";
import { useGoCart } from "@/components/go/GoCartProvider";

export default function CheckoutSuccessPage() {
  const searchParams = useSearchParams();
  const orderId = searchParams.get("order");
  const { refresh } = useGoCart();

  useEffect(() => {
    refresh();
  }, [refresh]);

  return (
    <div className="mx-auto max-w-lg px-6 py-20 text-center">
      <CheckCircle className="mx-auto size-16 text-green-500" />
      <h1 className="mt-6 text-2xl font-bold">Payment Successful!</h1>
      <p className="mt-2 text-muted-foreground">
        Your order is being processed. You&apos;ll see it update shortly.
      </p>
      {orderId && (
        <p className="mt-4 text-sm text-muted-foreground">
          Order: <span className="font-mono">{orderId.slice(0, 8)}...</span>
        </p>
      )}
      <div className="mt-8 flex justify-center gap-4">
        <Link
          href="/go/ecommerce/orders"
          className="rounded-lg bg-primary px-6 py-3 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
        >
          View Orders
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
