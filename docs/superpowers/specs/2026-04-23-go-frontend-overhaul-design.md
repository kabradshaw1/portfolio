# Go Frontend Overhaul

**Date:** 2026-04-23
**Status:** Draft

## Problem

The `/go` route has grown into a single long page that's hard to navigate. The Microservices section doesn't explain that cart, product, and order were originally one service. The AI Shopping Assistant is buried below the fold instead of having its own section. The Analytics and Admin features have no documentation explaining why they exist. The analytics page still shows a historical PostgreSQL section referencing removed APIs. The ecommerce store has error handling gaps — visiting `/go/ecommerce/orders` crashes the entire page instead of showing a contained error. There's no way to navigate to orders from the store UI. And the product catalog is thin (4 items per category) with clothing/electronics possibly not showing due to a seed idempotency bug.

## Solution

### 1. Restructure /go Page Into 5 Tabs

Extract each tab into its own component under `frontend/src/components/go/tabs/`:

- `MicroservicesTab.tsx`
- `OriginalTab.tsx`
- `AiAssistantTab.tsx`
- `AnalyticsTab.tsx` (new content)
- `AdminTab.tsx` (new content)

`page.tsx` becomes a thin shell: bio section, 5-tab bar, active tab component, three CTA buttons (View Store, Streaming Analytics, Admin Panel).

The tab type union expands from `"microservices" | "original"` to include `"ai-assistant" | "analytics" | "admin"`.

### 2. Microservices Tab Updates

**Origin story (new opening paragraph):** Explain that the cart, product, and order services were originally a single `ecommerce-service`. Direct readers to the "Original" tab to see that architecture. Then flow into the existing "Why Decompose" content.

**Add payment-service to the architecture diagram:** The Mermaid flowchart currently shows 6 services (auth, product, cart, order, ai, analytics). Add payment-service with its REST/gRPC ports and connections to the order-service (gRPC) and its own database (paymentdb). The order-service calls payment-service via gRPC as part of the checkout saga, so update the saga sequence diagram to include the payment step (reserve cart → validate stock → create payment → confirm/compensate).

**Update tech stack badges** if the count changes (currently says "6 Go microservices").

### 3. AI Assistant Tab (Moved)

Move the entire AI Shopping Assistant section from below the tab bar into its own tab. Content is unchanged:

- Tool Catalog (12 tools across 4 domains)
- Agent Loop diagram (ReAct pattern)
- Product Search Flow diagram
- Knowledge Query Flow diagram

### 4. Analytics Tab (New)

Problem-solution narrative for portfolio audience:

**Problem:** Batch analytics (polling a database) can't show real-time trends. For an ecommerce platform, you want to see revenue, cart abandonment, and trending products as they happen — not 15 minutes later.

**Solution:** Kafka streaming pipeline. Order-service and cart-service publish events to Kafka topics. The analytics-service consumes them in real-time, aggregates into sliding-window metrics (revenue per hour, cart abandonment rate, trending products by score), and exposes them via REST.

**Content to include:**
- Why streaming over batch (1-2 paragraphs)
- Architecture: Kafka topics, consumer groups, aggregation approach
- What metrics are surfaced (revenue, abandonment, trending)
- Link to the live Streaming Analytics dashboard

### 5. Admin Tab (New)

Problem-solution narrative for portfolio audience:

**Problem:** Distributed sagas fail. Messages get nacked to a dead-letter queue. Without visibility into the DLQ, failed orders are invisible — you'd need to SSH into a pod and query RabbitMQ manually.

**Solution:** Built an admin panel that exposes DLQ messages via a REST API. Shows routing key, timestamp, retry count. Supports one-click replay to re-process failed messages. Demonstrates operational tooling awareness.

**Content to include:**
- Why DLQ visibility matters (1-2 paragraphs)
- What the admin panel shows and does
- How replay works (message goes back to the saga exchange)
- Link to the live Admin Panel

### 6. Error Boundaries (Next.js)

Add `error.tsx` files at four route segments:

- `frontend/src/app/go/error.tsx`
- `frontend/src/app/go/ecommerce/error.tsx`
- `frontend/src/app/go/analytics/error.tsx`
- `frontend/src/app/go/admin/error.tsx`

Each renders a contained error message with a "Try again" button. The parent layout (GoSubHeader, navigation) survives above it.

### 7. HealthGate Rework

The current `HealthGate` in `frontend/src/app/go/ecommerce/layout.tsx` blocks the entire page when the order-service health check fails. Change the behavior:

- Instead of replacing children with a full-page error, render children normally
- Show an inline warning banner at the top ("Service is currently unavailable — some features may not work")
- Individual pages (orders, cart, checkout) handle their own fetch errors gracefully with inline error states
- This lets the page shell, navigation, and product browsing still function even if one downstream service is temporarily down

### 8. Orders Page Access

**Already implemented.** The `GoUserDropdown` component (line 40) already has an "Orders" menu item linking to `/go/ecommerce/orders` for logged-in users. No changes needed here — the real issue was the HealthGate blocking the orders page from loading (fixed in Section 7).

### 9. Seed Data Expansion

**Fix idempotency bug:** The current `go/product-service/seed.sql` uses `WHERE NOT EXISTS (SELECT 1 FROM products)` which prevents any new products from being inserted once the table has any rows. Change to per-product guards: `WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = '<product_name>')`.

**Expand catalog:** Add ~20 more products across all categories, bringing each to 8-9 items. Particularly fill out Electronics and Clothing which currently have only 4 each. Keep prices realistic, descriptions concise.

### 10. Analytics Page Cleanup

Remove the "Historical (PostgreSQL Materialized Views)" section from `/go/analytics/page.tsx`. This section references old API endpoints (revenue trends area chart, product performance table) that no longer exist after the service decomposition.

Keep the real-time Kafka streaming section (revenue per hour, trending products, cart abandonment).

Add a brief intro paragraph at the top explaining what the dashboard shows and that it's powered by Kafka streaming events. This complements the deeper documentation in the Analytics tab on the `/go` page.

## Files Changed

### New Files
- `frontend/src/components/go/tabs/MicroservicesTab.tsx`
- `frontend/src/components/go/tabs/OriginalTab.tsx`
- `frontend/src/components/go/tabs/AiAssistantTab.tsx`
- `frontend/src/components/go/tabs/AnalyticsTab.tsx`
- `frontend/src/components/go/tabs/AdminTab.tsx`
- `frontend/src/app/go/error.tsx`
- `frontend/src/app/go/ecommerce/error.tsx`
- `frontend/src/app/go/analytics/error.tsx`
- `frontend/src/app/go/admin/error.tsx`

### Modified Files
- `frontend/src/app/go/page.tsx` — thin shell with tab bar, renders tab components
- `frontend/src/app/go/ecommerce/layout.tsx` — HealthGate rework (degraded mode)
- `frontend/src/app/go/analytics/page.tsx` — remove historical section, add intro
- `frontend/src/components/go/GoUserDropdown.tsx` — add "My Orders" menu item
- `frontend/src/components/HealthGate.tsx` — add degraded/banner mode option
- `go/product-service/seed.sql` — fix idempotency, expand catalog

## Out of Scope

- Deep-linkable tabs via URL routing (kept client-side tab switching for simplicity)
- Changes to the Original tab content
- Changes to the AI Assistant content (just moved to its own tab)
- Cart or checkout flow changes
- Backend service changes (other than seed.sql)
