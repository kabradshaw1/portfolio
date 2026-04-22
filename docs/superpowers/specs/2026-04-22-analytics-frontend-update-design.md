# Analytics Frontend Update — Windowed Metrics UI

**Date:** 2026-04-22
**Status:** Proposed
**Scope:** `frontend/src/app/go/analytics/`, `go/analytics-service/internal/`

## Context

The analytics-service backend was rewritten from simple in-memory counters to a windowed Kafka stream processor (PR #144). The old REST endpoints (`/analytics/dashboard`, `/analytics/orders`, `/analytics/trending`) were replaced with new windowed endpoints (`/analytics/revenue`, `/analytics/trending`, `/analytics/cart-abandonment`). The frontend page at `src/app/go/analytics/page.tsx` calls the old endpoints and will break when the backend change lands.

This spec updates the frontend to use the new API and display windowed metrics.

## Goals

- Update the analytics page to call the new backend endpoints
- Display windowed revenue, trending products, and cart abandonment data
- Enrich trending products with names from the Kafka event payload (no gRPC dependency)
- Land in the same PR branch so frontend and backend deploy together

## Non-Goals

- New components or pages beyond the analytics page
- Real-time WebSocket streaming (polling is sufficient)
- Mobile-specific layout optimizations

## Frontend Changes

### Page Layout

Stacked sections, all visible without tabs or clicks. Same container pattern as current page (`mx-auto max-w-5xl px-6 py-8`).

**Section 1: Revenue per Hour**
- 3 KPI cards in a row: Total Revenue (24h sum), Total Orders (24h sum), Avg Order Value (24h weighted average)
- Recharts `BarChart` showing hourly revenue bars for the last 24 hours
- X-axis: hour labels (e.g., "14:00"), Y-axis: revenue in dollars
- Bar color: `hsl(var(--primary))`

**Section 2: Trending Products**
- Table with columns: Rank (#), Product Name, Score, Views, Cart Adds
- Top 10 products from the most recent 15-minute sliding window
- Same table styling as current page (`w-full text-sm`, border rows)

**Section 3: Cart Abandonment**
- 3 KPI cards: Abandonment Rate (latest window, amber colored), Carts Started, Carts Converted
- Recharts `BarChart` showing abandonment rate per 30-minute window over last 12 hours
- Bar color: amber (`#f59e0b`)

### API Calls

Three concurrent fetches via `Promise.all()`:

```typescript
GET ${ANALYTICS_URL}/analytics/revenue?hours=24
// Response: { windows: RevenueWindow[], stale: boolean }

GET ${ANALYTICS_URL}/analytics/trending?limit=10
// Response: { window_end: string, products: TrendingProduct[], stale: boolean }

GET ${ANALYTICS_URL}/analytics/cart-abandonment?hours=12
// Response: { windows: AbandonmentWindow[], stale: boolean }
```

### Polling

- Interval: 30 seconds (changed from 5s — windows close every 30-60 minutes, more frequent polling is wasteful)
- Same pattern: `useCallback` + `setInterval` with cancellation flag
- Same stale/error banner handling

### State Shape

```typescript
interface RevenueWindow {
  window_start: string;
  window_end: string;
  total_cents: number;
  order_count: number;
  avg_order_value_cents: number;
}

interface TrendingProduct {
  product_id: string;
  product_name: string;  // enriched from Kafka event
  score: number;
  views: number;
  cart_adds: number;
}

interface AbandonmentWindow {
  window_start: string;
  window_end: string;
  carts_started: number;
  carts_converted: number;
  carts_abandoned: number;
  abandonment_rate: number;
}
```

### Number Formatting

- Revenue: `$${(cents / 100).toFixed(2)}` (e.g., "$4,523.00")
- Abandonment rate: `${(rate * 100).toFixed(1)}%` (e.g., "44.7%")
- Counts: integer formatting (e.g., "47")
- Avg order value: `$${(cents / 100).toFixed(2)}`

### Empty State

Same pattern as current page — placeholder messages when no data:
- "No revenue data yet" in chart area
- "No trending products yet" in table area
- "No abandonment data yet" in chart area
- Stale banner with link to `/go/ecommerce` to generate test data

## Backend Changes

### Product Name Enrichment in Trending

The `ecommerce.views` Kafka events already include `productName` in their payload. The `ecommerce.cart` events include `productID` but not `productName`.

Changes needed:

1. **`internal/aggregator/trending.go`** — update `trendingData` to track product names alongside scores:
   ```go
   type trendingData struct {
       Scores map[string]float64
       Names  map[string]string  // productID -> productName
   }
   ```
   - `HandleView` receives and stores `productName`
   - `HandleCartAdd` may not have a name — only store if available

2. **`internal/store/store.go`** — update `TrendingProduct` to include `ProductName string`

3. **`internal/store/redis.go`** — store product names in a separate hash `analytics:trending:names:{windowKey}` alongside the sorted set scores

4. **`internal/store/mock.go`** — update MockStore to track names

5. **`internal/consumer/consumer.go`** — pass `productName` from view events to the trending aggregator

6. **`internal/handler/analytics.go`** — include `product_name` in the trending JSON response

### Updated Aggregator Signatures

```go
func (a *TrendingAggregator) HandleView(eventTime time.Time, productID, productName string) bool
func (a *TrendingAggregator) HandleCartAdd(eventTime time.Time, productID string) bool  // unchanged
```

### Updated Store Interface

```go
FlushTrending(ctx context.Context, windowKey string, scores map[string]float64, names map[string]string) error
```

## Files Changed

### Frontend
- `frontend/src/app/go/analytics/page.tsx` — complete rewrite of the page component

### Backend
- `go/analytics-service/internal/aggregator/trending.go` — add Names map, update HandleView signature
- `go/analytics-service/internal/aggregator/trending_test.go` — update tests
- `go/analytics-service/internal/store/store.go` — add ProductName to TrendingProduct, update FlushTrending signature
- `go/analytics-service/internal/store/redis.go` — store/retrieve names hash
- `go/analytics-service/internal/store/mock.go` — update MockStore
- `go/analytics-service/internal/store/redis_test.go` — update tests
- `go/analytics-service/internal/consumer/consumer.go` — pass productName to HandleView
- `go/analytics-service/internal/consumer/consumer_test.go` — update tests
- `go/analytics-service/internal/handler/analytics.go` — include product_name in response

## Testing

### Frontend
- `make preflight-frontend` — TypeScript compiles, lint passes
- Manual verification in browser (dev server)

### Backend
- `cd go/analytics-service && go test ./... -v -race` — all unit tests pass
- Verify trending endpoint returns product names in JSON response

## Verification

1. `make preflight-frontend` passes
2. `make preflight-go` passes
3. Start dev server, navigate to `/go/analytics` — page loads without errors
4. Revenue section shows chart (or empty state)
5. Trending section shows product names (or empty state)
6. Abandonment section shows rate chart (or empty state)
7. No console errors in browser
