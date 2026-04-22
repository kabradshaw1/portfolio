# Payment Frontend + Reporting UI Design

**Date:** 2026-04-22
**Status:** Proposed
**Goal:** Add frontend visibility for the payment service and SQL optimization work — enough for recruiters to see the backend functionality without needing to read code.

## Overview

Three changes to the existing Next.js frontend:

1. **Payment checkout flow** — redirect through Stripe Checkout Sessions instead of direct order creation
2. **Reporting section** — sales trends chart and product performance table added below the existing real-time analytics dashboard
3. **Configurable frontend URL** — order-service passes dynamic success/cancel URLs to payment-service

The frontend is not the portfolio piece — it's a window into the backend. Minimal UI investment, maximum signal.

---

## 1. Payment Checkout Flow

### Modified Cart Page (`/go/ecommerce/cart/page.tsx`)

The existing checkout handler calls `POST /orders` and shows a success message inline. The new behavior:

1. User clicks "Checkout" on the cart page
2. Frontend calls `POST /orders` (same endpoint, same idempotency-key pattern)
3. Order is created in PENDING state. The saga runs asynchronously — the checkout response returns immediately with the order data but no payment URL yet.
4. Frontend polls `GET /orders/{id}` briefly (1-2s interval, up to 15s timeout) waiting for the order's `checkoutUrl` field to be populated. The order-service adds this field once the saga reaches PAYMENT_CREATED step.
5. Once `checkoutUrl` is available, frontend redirects with `window.location.href = checkoutUrl`
6. User completes payment on Stripe's hosted page
7. Stripe redirects back to success or cancel URL

**Backend change required:** The order-service `GET /orders/{id}` response needs a `checkoutUrl` field. When the saga creates the Stripe Checkout Session, it stores the session URL on the order (new column or fetched from payment-service via gRPC). The frontend reads it on poll.

**Fallback:** If the checkout URL doesn't appear within 15 seconds, show an error message: "Payment is taking longer than expected. Check your orders page for status."

No Stripe JS SDK needed. No `@stripe/react-stripe-js`. No embedded payment form. Stripe Checkout Sessions are entirely server-side redirects.

### New Success Page (`/go/ecommerce/checkout/success/page.tsx`)

Stripe redirects here after successful payment. The page:

- Reads the `order` query parameter from the URL
- Shows "Payment successful! Your order is being processed."
- Links to the order detail or order history page
- Simple client component, no data fetching beyond the query param

### New Cancel Page (`/go/ecommerce/checkout/cancel/page.tsx`)

Stripe redirects here if the user cancels payment. The page:

- Shows "Payment cancelled. Your cart is still saved."
- Links back to the cart page
- Simple static content

### Order List Enhancement (`/go/ecommerce/orders/page.tsx`)

Add a payment status indicator to each order in the list. The order's `status` field already reflects the saga outcome:

- `completed` — paid and fulfilled
- `pending` — awaiting payment
- `failed` — payment or saga failed

Use the existing `Badge` component with color variants to make status visually scannable. No new API calls needed — the status is already in the order list response.

---

## 2. Reporting Section on Analytics Page

### Location

Below the existing real-time Kafka dashboard on `/go/analytics/page.tsx`. A visual divider (heading: "Historical Reporting") separates the two sections.

### Sales Trends Chart

A Recharts `AreaChart` or `LineChart` showing:

- **X-axis:** dates over the last 30 days
- **Y-axis:** revenue in dollars (converted from cents)
- **Two lines:** rolling 7-day revenue and rolling 30-day revenue
- **Data source:** `GET /reporting/sales-trends?days=30` on order-service
- **Fetch behavior:** one-time on mount, no polling (materialized view data refreshed every 15 minutes server-side)

### Product Performance Table

A table showing per-product metrics:

- Product name, category
- Units sold, total revenue (formatted as currency)
- Return rate (percentage)
- Average order value (formatted as currency)
- **Data source:** `GET /reporting/product-performance` on order-service
- **Sort:** by revenue descending (API returns pre-sorted)
- **Fetch behavior:** one-time on mount, no polling

### API Client

Use the existing `goOrderFetch` wrapper from `frontend/src/lib/go-order-api.ts`. The reporting endpoints are on the order-service at the same base URL (`NEXT_PUBLIC_GO_ORDER_URL`). No new environment variables needed.

---

## 3. Configurable Frontend URL

The order-service orchestrator currently hardcodes `https://kylebradshaw.dev` when constructing Stripe success/cancel redirect URLs. This needs to be configurable for QA vs. prod.

### Changes

- Add `FRONTEND_URL` to order-service `Config` struct and `loadConfig()` (default: `http://localhost:3000`)
- Orchestrator reads `FRONTEND_URL` and constructs:
  - Success: `{FRONTEND_URL}/go/ecommerce/checkout/success?order={orderID}`
  - Cancel: `{FRONTEND_URL}/go/ecommerce/checkout/cancel?order={orderID}`
- Add `FRONTEND_URL` to order-service ConfigMap (`https://kylebradshaw.dev` for prod)
- Add `FRONTEND_URL` to QA Kustomize overlay (`https://qa.kylebradshaw.dev`)

---

## Out of Scope

- No Stripe JS SDK or embedded payment form — Checkout Sessions handle the UI
- No payment method selection UI — Stripe handles that
- No invoice/receipt page — Stripe sends receipts via email
- No admin payment management UI
- No new navigation items in GoSubHeader — reporting lives on the existing analytics page
- No polling for reporting data — one-time fetch is sufficient
- No new environment variables for the frontend
