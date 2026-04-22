# Payment Frontend + Reporting UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add frontend visibility for the Stripe payment flow and SQL optimization reporting — checkout redirect, success/cancel pages, order status badges, sales trends chart, and product performance table.

**Architecture:** Modify the existing cart checkout to poll for a Stripe Checkout URL, add two new static pages for Stripe redirects, extend the analytics page with a reporting section, and add a `checkoutUrl` + `FRONTEND_URL` to the order-service backend.

**Tech Stack:** Next.js (App Router, client components), Recharts, Tailwind CSS, Go (order-service config + model changes)

**Spec:** `docs/superpowers/specs/2026-04-22-payment-frontend-reporting-design.md`

---

### Task 1: Backend — Add `checkoutUrl` to Order Model + Repository

**Files:**
- Modify: `go/order-service/internal/model/order.go`
- Modify: `go/order-service/migrations/` (new migration)
- Modify: `go/order-service/internal/repository/order.go`

- [ ] **Step 1: Create migration to add checkout_url column**

Create `go/order-service/migrations/010_add_checkout_url.up.sql`:

```sql
ALTER TABLE orders ADD COLUMN checkout_url TEXT;
```

Create `go/order-service/migrations/010_add_checkout_url.down.sql`:

```sql
ALTER TABLE orders DROP COLUMN IF EXISTS checkout_url;
```

- [ ] **Step 2: Add CheckoutURL field to Order model**

In `go/order-service/internal/model/order.go`, add to the `Order` struct after `SagaStep`:

```go
CheckoutURL string      `json:"checkoutUrl,omitempty"`
```

- [ ] **Step 3: Add UpdateCheckoutURL repository method**

In `go/order-service/internal/repository/order.go`, add:

```go
func (r *OrderRepository) UpdateCheckoutURL(ctx context.Context, orderID uuid.UUID, url string) error {
	_, err := resilience.Call(ctx, r.breaker, resilience.DefaultRetryConfig(), func() (any, error) {
		_, execErr := r.pool.Exec(ctx,
			`UPDATE orders SET checkout_url = $1, updated_at = now() WHERE id = $2`,
			url, orderID,
		)
		return nil, execErr
	})
	return err
}
```

Update the `FindByID` query to include `checkout_url` in the SELECT and scan it into the Order struct. The column is nullable, so scan into a `*string` and assign if non-nil (same pattern as the payment repository uses for nullable Stripe fields).

- [ ] **Step 4: Run tests**

```bash
cd go/order-service && go mod tidy && go test ./... -v -race
```

Expected: PASS (existing tests don't depend on checkout_url column)

- [ ] **Step 5: Commit**

```bash
git add go/order-service/migrations/010_add_checkout_url.up.sql go/order-service/migrations/010_add_checkout_url.down.sql go/order-service/internal/model/order.go go/order-service/internal/repository/order.go
git commit -m "feat(order): add checkout_url column to orders for Stripe redirect"
```

---

### Task 2: Backend — Store Checkout URL in Orchestrator + Add FRONTEND_URL Config

**Files:**
- Modify: `go/order-service/internal/saga/orchestrator.go`
- Modify: `go/order-service/cmd/server/config.go`
- Modify: `go/order-service/cmd/server/main.go`
- Modify: `go/k8s/configmaps/order-service-config.yml`
- Modify: `k8s/overlays/qa-go/kustomization.yaml`

- [ ] **Step 1: Add FRONTEND_URL to config**

In `go/order-service/cmd/server/config.go`, add to Config struct:

```go
FrontendURL string // default "http://localhost:3000"
```

In `loadConfig()`:

```go
FrontendURL: os.Getenv("FRONTEND_URL"),
```

And after other defaults:

```go
if cfg.FrontendURL == "" {
    cfg.FrontendURL = "http://localhost:3000"
}
```

- [ ] **Step 2: Add FrontendURL to Orchestrator and update handleStockValidated**

The `Orchestrator` needs the frontend URL and a way to save the checkout URL. Add to the `OrderRepository` interface in orchestrator.go:

```go
UpdateCheckoutURL(ctx context.Context, orderID uuid.UUID, url string) error
```

Add `frontendURL string` field to the `Orchestrator` struct. Update `NewOrchestrator` to accept it:

```go
func NewOrchestrator(repo OrderRepository, pub SagaPublisher, stock StockChecker, payment PaymentCreator, kafkaPub kafka.Producer, frontendURL string) *Orchestrator {
    return &Orchestrator{repo: repo, pub: pub, stock: stock, payment: payment, kafkaPub: kafkaPub, frontendURL: frontendURL}
}
```

Update `handleStockValidated` to use `o.frontendURL` and store the checkout URL:

```go
func (o *Orchestrator) handleStockValidated(ctx context.Context, order *model.Order) error {
    if o.payment != nil {
        checkoutURL, err := o.payment.CreatePayment(ctx, order.ID, order.Total, "usd",
            o.frontendURL+"/go/ecommerce/checkout/success?order="+order.ID.String(),
            o.frontendURL+"/go/ecommerce/checkout/cancel?order="+order.ID.String(),
        )
        if err != nil {
            slog.ErrorContext(ctx, "create payment failed, compensating",
                "orderID", order.ID, "error", err)
            return o.compensate(ctx, order)
        }

        if err := o.repo.UpdateCheckoutURL(ctx, order.ID, checkoutURL); err != nil {
            slog.ErrorContext(ctx, "store checkout URL failed", "orderID", order.ID, "error", err)
        }

        if err := o.repo.UpdateSagaStep(ctx, order.ID, StepPaymentCreated); err != nil {
            return err
        }
        SagaStepsTotal.WithLabelValues(StepPaymentCreated, "success").Inc()
        return nil
    }
    return o.handlePaymentConfirmed(ctx, order)
}
```

- [ ] **Step 3: Update main.go to pass FrontendURL**

Update the `saga.NewOrchestrator` call in `main.go`:

```go
orch := saga.NewOrchestrator(orderRepo, sagaPub, prodClient, payClient, kafkaPub, cfg.FrontendURL)
```

- [ ] **Step 4: Update K8s ConfigMaps**

Add to `go/k8s/configmaps/order-service-config.yml`:

```yaml
FRONTEND_URL: "https://kylebradshaw.dev"
```

Add to the order-service patch in `k8s/overlays/qa-go/kustomization.yaml`:

```yaml
FRONTEND_URL: "https://qa.kylebradshaw.dev"
```

- [ ] **Step 5: Fix all NewOrchestrator callers**

Update all test files that call `saga.NewOrchestrator` to include the new `frontendURL` parameter (pass `"http://localhost:3000"` in tests).

- [ ] **Step 6: Run tests**

```bash
cd go/order-service && go test ./... -v -race
```

Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add go/order-service/ go/k8s/configmaps/order-service-config.yml k8s/overlays/qa-go/kustomization.yaml
git commit -m "feat(order): store checkout URL from Stripe and add configurable FRONTEND_URL"
```

---

### Task 3: Frontend — Checkout Success Page

**Files:**
- Create: `frontend/src/app/go/ecommerce/checkout/success/page.tsx`

- [ ] **Step 1: Create the success page**

Create `frontend/src/app/go/ecommerce/checkout/success/page.tsx`:

```tsx
"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { CheckCircle } from "lucide-react";

export default function CheckoutSuccessPage() {
  const searchParams = useSearchParams();
  const orderId = searchParams.get("order");

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
```

- [ ] **Step 2: Verify it renders**

```bash
cd frontend && npm run dev
```

Navigate to `http://localhost:3000/go/ecommerce/checkout/success?order=test-123`. Should show the success page with truncated order ID.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/app/go/ecommerce/checkout/success/
git commit -m "feat(frontend): add checkout success page for Stripe redirect"
```

---

### Task 4: Frontend — Checkout Cancel Page

**Files:**
- Create: `frontend/src/app/go/ecommerce/checkout/cancel/page.tsx`

- [ ] **Step 1: Create the cancel page**

Create `frontend/src/app/go/ecommerce/checkout/cancel/page.tsx`:

```tsx
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
```

- [ ] **Step 2: Verify it renders**

Navigate to `http://localhost:3000/go/ecommerce/checkout/cancel`. Should show the cancel page.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/app/go/ecommerce/checkout/cancel/
git commit -m "feat(frontend): add checkout cancel page for Stripe redirect"
```

---

### Task 5: Frontend — Modify Cart Checkout to Poll for Stripe URL

**Files:**
- Modify: `frontend/src/app/go/ecommerce/cart/page.tsx`

- [ ] **Step 1: Replace the checkout function**

Replace the existing `checkout` function (lines 65-88) in `cart/page.tsx` with:

```tsx
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
    const pollInterval = 1500; // 1.5 seconds
    const maxAttempts = 10; // 15 seconds total

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
        // Payment skipped (dev mode) — order went straight to completed
        setItems([]);
        router.push(`/go/ecommerce/checkout/success?order=${order.id}`);
        return;
      }
      if (updated.status === "failed") {
        setMessage("Order failed. Please try again.");
        return;
      }
    }

    // Timeout — checkout URL never appeared
    setMessage("Payment is taking longer than expected. Check your orders page for status.");
  } finally {
    setCheckingOut(false);
  }
}
```

- [ ] **Step 2: Update the checkout button text**

Replace the button text on the checkout button (around line 149):

```tsx
{checkingOut ? "Processing payment..." : "Checkout"}
```

- [ ] **Step 3: Run type check**

```bash
cd frontend && npx tsc --noEmit
```

Expected: No type errors.

- [ ] **Step 4: Test manually**

Start dev server, add an item to cart, click checkout. Without payment-service running, it should time out with the "taking longer than expected" message (since no checkoutUrl gets set). With payment-service configured in QA, it should redirect to Stripe.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/app/go/ecommerce/cart/page.tsx
git commit -m "feat(frontend): modify cart checkout to poll for Stripe checkout URL"
```

---

### Task 6: Frontend — Enhance Order Status Display

**Files:**
- Modify: `frontend/src/app/go/ecommerce/orders/page.tsx`

- [ ] **Step 1: Add "failed" color and capitalize status display**

The `statusColor` function already handles the cases. Add a `statusLabel` function for display-friendly text and update the status rendering to use a badge-style pill:

Replace the status `<p>` tag (line 95-97) with:

```tsx
<span className={`inline-block rounded-full px-2 py-0.5 text-xs font-medium ${statusBadge(order.status)}`}>
  {order.status}
</span>
```

Add the `statusBadge` function (replaces `statusColor`):

```tsx
function statusBadge(status: string): string {
  switch (status.toLowerCase()) {
    case "completed":
      return "bg-green-500/10 text-green-500";
    case "processing":
      return "bg-yellow-500/10 text-yellow-500";
    case "failed":
      return "bg-red-500/10 text-red-500";
    case "pending":
      return "bg-blue-500/10 text-blue-500";
    default:
      return "bg-muted text-muted-foreground";
  }
}
```

Remove the old `statusColor` function.

- [ ] **Step 2: Run type check**

```bash
cd frontend && npx tsc --noEmit
```

Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/app/go/ecommerce/orders/page.tsx
git commit -m "feat(frontend): enhance order status with colored badge pills"
```

---

### Task 7: Frontend — Reporting Section on Analytics Page

**Files:**
- Modify: `frontend/src/app/go/analytics/page.tsx`

- [ ] **Step 1: Add reporting interfaces and state**

Add these interfaces after the existing interfaces (around line 55):

```tsx
interface SalesTrend {
  day: string;
  dailyRevenue: number;
  rolling7Day: number;
  rolling30Day: number;
}

interface ProductPerf {
  productId: string;
  productName: string;
  category: string;
  currentStock: number;
  totalUnitsSold: number;
  totalRevenueCents: number;
  totalOrders: number;
  avgOrderValueCents: number;
  returnCount: number;
  returnRatePct: number;
}
```

Add state variables inside the component (after existing state declarations around line 61):

```tsx
const [salesTrends, setSalesTrends] = useState<SalesTrend[]>([]);
const [productPerf, setProductPerf] = useState<ProductPerf[]>([]);
const [reportingError, setReportingError] = useState<string | null>(null);
```

- [ ] **Step 2: Add reporting data fetch**

Add an import at the top:

```tsx
import { goOrderFetch } from "@/lib/go-order-api";
```

Add a `useEffect` for one-time reporting fetch (after the existing polling `useEffect`):

```tsx
useEffect(() => {
  async function fetchReporting() {
    try {
      const [trendsRes, perfRes] = await Promise.all([
        goOrderFetch("/reporting/sales-trends?days=30"),
        goOrderFetch("/reporting/product-performance"),
      ]);
      if (trendsRes.ok) {
        const data = await trendsRes.json();
        setSalesTrends(data.trends ?? []);
      }
      if (perfRes.ok) {
        const data = await perfRes.json();
        setProductPerf(data.products ?? []);
      }
    } catch {
      setReportingError("Unable to load reporting data");
    }
  }
  fetchReporting();
}, []);
```

- [ ] **Step 3: Add the reporting section JSX**

Add after the closing `</div>` of the Trending Products section (before the final closing `</div>` of the page), add imports for `AreaChart`, `Area`, and `Legend` from recharts (add to existing recharts import), then:

```tsx
{/* Historical Reporting */}
<div className="mt-12 border-t border-foreground/10 pt-8">
  <h2 className="mb-2 text-xl font-bold">Historical Reporting</h2>
  <p className="mb-6 text-sm text-muted-foreground">
    Pre-computed analytics from PostgreSQL materialized views with concurrent refresh.
    Revenue trends use rolling window functions over range-partitioned order data.
  </p>

  {reportingError && (
    <div className="mb-4 rounded border border-red-500/30 bg-red-500/10 px-4 py-2 text-sm text-red-600 dark:text-red-400">
      {reportingError}
    </div>
  )}

  {/* Sales Trends Chart */}
  <div className="mb-8">
    <h3 className="mb-3 text-lg font-semibold">Revenue Trends (30 Days)</h3>
    <div className="rounded border bg-card p-4">
      {salesTrends.length > 0 ? (
        <ResponsiveContainer width="100%" height={280}>
          <AreaChart data={salesTrends}>
            <CartesianGrid strokeDasharray="3 3" />
            <XAxis
              dataKey="day"
              tickFormatter={(v: string) =>
                new Date(v).toLocaleDateString([], { month: "short", day: "numeric" })
              }
              fontSize={12}
            />
            <YAxis
              tickFormatter={(v: number) => `$${(v / 100).toFixed(0)}`}
              fontSize={12}
            />
            <Tooltip
              labelFormatter={(v) => new Date(String(v)).toLocaleDateString()}
              formatter={(value: number, name: string) => [
                `$${(value / 100).toFixed(2)}`,
                name === "rolling7Day" ? "7-Day Rolling" : "30-Day Rolling",
              ]}
            />
            <Legend
              formatter={(value: string) =>
                value === "rolling7Day" ? "7-Day Rolling" : "30-Day Rolling"
              }
            />
            <Area
              type="monotone"
              dataKey="rolling30Day"
              stroke="hsl(var(--muted-foreground))"
              fill="hsl(var(--muted-foreground) / 0.1)"
              strokeWidth={1.5}
              dot={false}
            />
            <Area
              type="monotone"
              dataKey="rolling7Day"
              stroke="hsl(var(--primary))"
              fill="hsl(var(--primary) / 0.15)"
              strokeWidth={2}
              dot={false}
            />
          </AreaChart>
        </ResponsiveContainer>
      ) : (
        <p className="py-8 text-center text-muted-foreground">
          No revenue data yet
        </p>
      )}
    </div>
  </div>

  {/* Product Performance Table */}
  <div>
    <h3 className="mb-3 text-lg font-semibold">Product Performance</h3>
    <div className="rounded border bg-card">
      {productPerf.length > 0 ? (
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b text-left text-muted-foreground">
              <th className="px-4 py-2">Product</th>
              <th className="px-4 py-2">Category</th>
              <th className="px-4 py-2 text-right">Units Sold</th>
              <th className="px-4 py-2 text-right">Revenue</th>
              <th className="px-4 py-2 text-right">Avg Order</th>
              <th className="px-4 py-2 text-right">Return Rate</th>
            </tr>
          </thead>
          <tbody>
            {productPerf.map((p) => (
              <tr key={p.productId} className="border-b last:border-0">
                <td className="px-4 py-2 font-medium">{p.productName}</td>
                <td className="px-4 py-2 text-muted-foreground">{p.category}</td>
                <td className="px-4 py-2 text-right">{p.totalUnitsSold}</td>
                <td className="px-4 py-2 text-right">
                  ${(p.totalRevenueCents / 100).toFixed(2)}
                </td>
                <td className="px-4 py-2 text-right">
                  ${(p.avgOrderValueCents / 100).toFixed(2)}
                </td>
                <td className="px-4 py-2 text-right">
                  {p.returnRatePct.toFixed(1)}%
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : (
        <p className="px-4 py-8 text-center text-muted-foreground">
          No product data yet
        </p>
      )}
    </div>
  </div>
</div>
```

- [ ] **Step 4: Update recharts import**

Add `AreaChart`, `Area`, and `Legend` to the existing recharts import at the top of the file:

```tsx
import {
  LineChart,
  Line,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from "recharts";
```

- [ ] **Step 5: Run type check**

```bash
cd frontend && npx tsc --noEmit
```

Expected: No errors.

- [ ] **Step 6: Verify visually**

Start dev server, navigate to `/go/analytics`. The page should show the existing real-time section at the top and a "Historical Reporting" section below with the sales trends chart and product performance table (data may be empty without orders in the system).

- [ ] **Step 7: Commit**

```bash
git add frontend/src/app/go/analytics/page.tsx
git commit -m "feat(frontend): add historical reporting section with sales trends and product performance"
```

---

### Task 8: Preflight + Final Verification

**Files:** None (verification only)

- [ ] **Step 1: Run Go preflight for order-service**

```bash
cd go/order-service && golangci-lint run ./... && go test ./... -v -race
```

Expected: PASS

- [ ] **Step 2: Run frontend preflight**

```bash
cd frontend && npx tsc --noEmit && npx next lint
```

Expected: PASS

- [ ] **Step 3: Commit any fixes**

If lint or type check found issues, fix and commit:

```bash
git add -A && git commit -m "fix: address lint and type issues"
```
