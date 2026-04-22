# Analytics Frontend Update — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Update the frontend analytics page to use the new windowed API endpoints, and enrich trending product data with names from Kafka events.

**Architecture:** Two changes: (1) backend adds product name tracking to the trending aggregator/store/consumer pipeline, (2) frontend rewrites the top half of the analytics page to call the new `/analytics/revenue`, `/analytics/trending`, `/analytics/cart-abandonment` endpoints. The bottom half (Historical Reporting from order-service) is untouched.

**Tech Stack:** Go (analytics-service backend), TypeScript/React (Next.js frontend), Recharts (charts), Tailwind CSS (styling)

---

## File Structure

### Backend (product name enrichment)
| File | Action | Responsibility |
|------|--------|---------------|
| `go/analytics-service/internal/aggregator/trending.go` | Modify | Add `Names` map to `trendingData`, update `HandleView` to accept `productName` |
| `go/analytics-service/internal/aggregator/trending_test.go` | Modify | Update tests for new `HandleView` signature |
| `go/analytics-service/internal/store/store.go` | Modify | Add `ProductName` field to `TrendingProduct`, update `FlushTrending` signature |
| `go/analytics-service/internal/store/redis.go` | Modify | Store/retrieve product names hash alongside sorted set |
| `go/analytics-service/internal/store/mock.go` | Modify | Update MockStore for new `FlushTrending` signature |
| `go/analytics-service/internal/store/redis_test.go` | Modify | Update tests for new signature |
| `go/analytics-service/internal/consumer/consumer.go` | Modify | Pass `productName` to `HandleView` |
| `go/analytics-service/internal/consumer/consumer_test.go` | Modify | Update tests for new call |
| `go/analytics-service/internal/handler/analytics_test.go` | Modify | Update trending test expectations |

### Frontend
| File | Action | Responsibility |
|------|--------|---------------|
| `frontend/src/app/go/analytics/page.tsx` | Modify | Rewrite Kafka analytics section for new endpoints |

---

### Task 1: Add product name tracking to trending aggregator

**Files:**
- Modify: `go/analytics-service/internal/aggregator/trending.go`
- Modify: `go/analytics-service/internal/aggregator/trending_test.go`

- [ ] **Step 1: Update trendingData struct and merge function**

In `go/analytics-service/internal/aggregator/trending.go`, add `Names` map:

```go
type trendingData struct {
	Scores map[string]float64 // productID -> weighted score
	Names  map[string]string  // productID -> productName
}
```

Update the zero function in `NewTrendingAggregator` to initialize Names:
```go
func() trendingData {
	return trendingData{
		Scores: make(map[string]float64),
		Names:  make(map[string]string),
	}
},
```

Update the merge function to merge Names:
```go
func(dst, src *trendingData) {
	for pid, score := range src.Scores {
		dst.Scores[pid] += score
	}
	for pid, name := range src.Names {
		if name != "" {
			dst.Names[pid] = name
		}
	}
},
```

- [ ] **Step 2: Update HandleView to accept productName**

Change the `HandleView` signature:
```go
func (a *TrendingAggregator) HandleView(eventTime time.Time, productID, productName string) bool {
	return a.window.Add(eventTime, func(d *trendingData) {
		d.Scores[productID] += trendingViewWeight
		if productName != "" {
			d.Names[productID] = productName
		}
	})
}
```

- [ ] **Step 3: Update Flush to pass names to store**

Change the `Flush` method to pass `r.Data.Names`:
```go
if err := a.store.FlushTrending(ctx, r.Key, r.Data.Scores, r.Data.Names); err != nil {
```

- [ ] **Step 4: Update trending tests**

In `trending_test.go`, update all `HandleView` calls to include a product name:
```go
agg.HandleView(eventTime, "prod-1", "Test Widget")
```

Also add a test verifying names are tracked:
```go
func TestTrendingAggregator_HandleViewTracksName(t *testing.T) {
	// ... setup with MockClock + MockStore ...
	agg.HandleView(eventTime, "prod-1", "Widget A")
	// advance clock past window + grace
	// flush
	// verify store received the name
}
```

- [ ] **Step 5: Verify**

Run: `cd go/analytics-service && go test ./internal/aggregator/... -v -race`
Expected: All tests pass

- [ ] **Step 6: Commit**

```bash
git add go/analytics-service/internal/aggregator/
git commit -m "feat(analytics): track product names in trending aggregator"
```

---

### Task 2: Update Store interface and implementations for product names

**Files:**
- Modify: `go/analytics-service/internal/store/store.go`
- Modify: `go/analytics-service/internal/store/redis.go`
- Modify: `go/analytics-service/internal/store/mock.go`
- Modify: `go/analytics-service/internal/store/redis_test.go`

- [ ] **Step 1: Update Store interface and TrendingProduct type**

In `store.go`, add `ProductName` to `TrendingProduct`:
```go
type TrendingProduct struct {
	ProductID   string  `json:"product_id"`
	ProductName string  `json:"product_name"`
	Score       float64 `json:"score"`
	Views       int     `json:"views"`
	CartAdds    int     `json:"cart_adds"`
}
```

Update `FlushTrending` signature in the `Store` interface:
```go
FlushTrending(ctx context.Context, windowKey string, scores map[string]float64, names map[string]string) error
```

- [ ] **Step 2: Update RedisStore.FlushTrending**

In `redis.go`, update `FlushTrending` to accept and store names:
```go
func (s *RedisStore) FlushTrending(ctx context.Context, windowKey string, scores map[string]float64, names map[string]string) error {
	key := trendingPrefix + windowKey
	namesKey := trendingPrefix + "names:" + windowKey

	_, err := s.breaker.Execute(func() (any, error) {
		ctx2, span := tracing.RedisSpan(ctx, "FlushTrending", key)
		defer span.End()

		members := make([]redis.Z, 0, len(scores))
		for productID, score := range scores {
			members = append(members, redis.Z{Score: score, Member: productID})
		}

		pipe := s.client.Pipeline()
		pipe.ZAdd(ctx2, key, members...)
		pipe.Expire(ctx2, key, trendingTTL)

		if len(names) > 0 {
			nameFields := make([]string, 0, len(names)*2)
			for pid, name := range names {
				nameFields = append(nameFields, pid, name)
			}
			pipe.HSet(ctx2, namesKey, nameFields)
			pipe.Expire(ctx2, namesKey, trendingTTL)
		}

		_, err := pipe.Exec(ctx2)
		if err != nil {
			return nil, fmt.Errorf("flush trending pipeline: %w", err)
		}
		return nil, nil
	})
	return err
}
```

- [ ] **Step 3: Update RedisStore.GetTrending to include names**

In the `GetTrending` method, after fetching the sorted set members, also fetch the names hash:
```go
// After getting members from ZRevRangeWithScores...
namesKey := trendingPrefix + "names:" + latestKey[len(trendingPrefix):]
nameMap, err := s.client.HGetAll(ctx2, namesKey).Result()
if err != nil {
	// Log warning but continue — names are optional
	nameMap = nil
}

products := make([]TrendingProduct, len(members))
for i, m := range members {
	pid := m.Member.(string)
	products[i] = TrendingProduct{
		ProductID:   pid,
		ProductName: nameMap[pid],
		Score:       m.Score,
	}
}
```

- [ ] **Step 4: Update MockStore**

In `mock.go`, add a `names` map and update `FlushTrending`:
```go
type MockStore struct {
	mu          sync.Mutex
	revenue     map[string]*RevenueWindow
	trending    map[string]map[string]float64
	trendNames  map[string]map[string]string  // windowKey -> productID -> name
	abandonment map[string]*AbandonmentWindow
	users       map[string]map[string]bool
}
```

Update `NewMockStore` to initialize `trendNames`.

Update `FlushTrending`:
```go
func (m *MockStore) FlushTrending(_ context.Context, windowKey string, scores map[string]float64, names map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.trending[windowKey] = make(map[string]float64, len(scores))
	for k, v := range scores {
		m.trending[windowKey][k] = v
	}

	if len(names) > 0 {
		m.trendNames[windowKey] = make(map[string]string, len(names))
		for k, v := range names {
			m.trendNames[windowKey][k] = v
		}
	}
	return nil
}
```

Update `GetTrending` to include names in the returned products:
```go
// In the product building loop:
products[i] = TrendingProduct{
	ProductID:   pid,
	ProductName: m.trendNames[latestKey][pid],
	Score:       score,
}
```

- [ ] **Step 5: Update store tests**

In `redis_test.go`, update `FlushTrending` calls to include names map:
```go
err := s.FlushTrending(ctx, windowKey, scores, map[string]string{"prod-1": "Widget A"})
```

Add a test that verifies product names round-trip through the mock store.

- [ ] **Step 6: Verify**

Run: `cd go/analytics-service && go test ./internal/store/... -v -race`
Expected: All tests pass

Run: `cd go/analytics-service && go build ./...`
Expected: Compiles

- [ ] **Step 7: Commit**

```bash
git add go/analytics-service/internal/store/
git commit -m "feat(analytics): add product names to trending store interface"
```

---

### Task 3: Update consumer and handler for product names

**Files:**
- Modify: `go/analytics-service/internal/consumer/consumer.go`
- Modify: `go/analytics-service/internal/consumer/consumer_test.go`
- Modify: `go/analytics-service/internal/handler/analytics_test.go`

- [ ] **Step 1: Update consumer handleView to pass productName**

In `consumer.go`, change the `handleView` method:
```go
func (c *Consumer) handleView(env event, eventTime time.Time) {
	var data viewData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		slog.Error("unmarshal view data", "error", err)
		return
	}

	c.trending.HandleView(eventTime, data.ProductID, data.ProductName)
}
```

The `viewData` struct already has `ProductName string` — no change needed there.

- [ ] **Step 2: Update consumer tests**

In `consumer_test.go`, find any calls to `HandleView` in test doubles or assertions and update them to include the product name parameter.

- [ ] **Step 3: Update handler tests**

In `analytics_test.go`, update any mock store setup for trending tests to use the new `FlushTrending` signature with the names parameter:
```go
s.FlushTrending(ctx, windowKey, scores, names)
```

- [ ] **Step 4: Verify full service compiles and tests pass**

Run: `cd go/analytics-service && go test ./... -v -race`
Expected: All tests pass

Run: `cd go/analytics-service && go build ./cmd/server`
Expected: Compiles

- [ ] **Step 5: Commit**

```bash
git add go/analytics-service/internal/consumer/ go/analytics-service/internal/handler/
git commit -m "feat(analytics): pass product names through consumer to trending store"
```

---

### Task 4: Rewrite frontend analytics page

**Files:**
- Modify: `frontend/src/app/go/analytics/page.tsx`

**Important:** Before writing any Next.js code, check `node_modules/next/dist/docs/` for any relevant guides about current API conventions. The AGENTS.md warns that this version may have breaking changes.

- [ ] **Step 1: Replace the Kafka analytics section of the page**

The page has two sections:
1. **Kafka Streaming Analytics** (top half) — calls old endpoints, NEEDS REWRITING
2. **Historical Reporting** (bottom half, after `<div className="mt-12 border-t...">`) — calls order-service, KEEP AS-IS

Replace the top section (interfaces, state, fetchAll, rendering from line 24 through line 295) with the new windowed API calls and rendering.

New interfaces:
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
  product_name: string;
  score: number;
  views: number;
  cart_adds: number;
}

interface TrendingData {
  window_end: string;
  products: TrendingProduct[];
  stale: boolean;
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

New state:
```typescript
const [revenue, setRevenue] = useState<RevenueWindow[]>([]);
const [trending, setTrending] = useState<TrendingData | null>(null);
const [abandonment, setAbandonment] = useState<AbandonmentWindow[]>([]);
const [stale, setStale] = useState(false);
const [error, setError] = useState<string | null>(null);
```

New fetchAll (30s poll interval):
```typescript
const POLL_INTERVAL = 30_000; // 30 seconds

const fetchAll = useCallback(async () => {
  try {
    const [revRes, trendRes, abandRes] = await Promise.all([
      fetch(`${ANALYTICS_URL}/analytics/revenue?hours=24`),
      fetch(`${ANALYTICS_URL}/analytics/trending?limit=10`),
      fetch(`${ANALYTICS_URL}/analytics/cart-abandonment?hours=12`),
    ]);

    if (revRes.ok) {
      const data = await revRes.json();
      setRevenue(data.windows ?? []);
      setStale(data.stale);
    }
    if (trendRes.ok) {
      const data = await trendRes.json();
      setTrending(data);
    }
    if (abandRes.ok) {
      const data = await abandRes.json();
      setAbandonment(data.windows ?? []);
    }
    setError(null);
  } catch {
    setError("Unable to reach analytics service");
  }
}, []);
```

New rendering for the Kafka section (3 stacked sections):

**Revenue section:** 3 KPI cards (sum totals from all windows) + BarChart (Recharts) showing hourly revenue. Keep using `BarChart` from recharts (import `Bar` and `BarChart` instead of `Line`/`LineChart`).

Compute summary KPIs from the windows array:
```typescript
const totalRevenue = revenue.reduce((sum, w) => sum + w.total_cents, 0);
const totalOrders = revenue.reduce((sum, w) => sum + w.order_count, 0);
const avgOrder = totalOrders > 0 ? totalRevenue / totalOrders : 0;
```

**Trending section:** Table with columns: #, Product, Score, Views, Cart Adds. Use `product_name || product_id` for display. Column "Purchases" renamed to "Cart Adds".

**Abandonment section:** 3 KPI cards (latest window's rate, started, converted) + BarChart showing abandonment rate per 30-min window. Bar color amber (`#f59e0b`).

Get latest abandonment window for KPIs:
```typescript
const latestAbandonment = abandonment.length > 0 ? abandonment[abandonment.length - 1] : null;
```

- [ ] **Step 2: Update Recharts imports**

Replace the LineChart imports with BarChart:
```typescript
import {
  BarChart,
  Bar,
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

Remove unused `LineChart`, `Line` imports.

- [ ] **Step 3: Remove unused imports**

Remove the old `DashboardData`, `OrdersData`, `HourlyBucket` interfaces and any unused state variables.

Keep the `goOrderFetch` import and the Historical Reporting section exactly as-is.

- [ ] **Step 4: Verify**

Run: `cd frontend && npx tsc --noEmit`
Expected: No type errors

Run: `cd frontend && npx next lint`
Expected: No lint errors

- [ ] **Step 5: Commit**

```bash
git add frontend/src/app/go/analytics/page.tsx
git commit -m "feat(frontend): update analytics page for windowed Kafka metrics"
```

---

### Task 5: Final verification

- [ ] **Step 1: Run Go preflight**

Run: `make preflight-go`
Expected: All lint + tests pass for all services

- [ ] **Step 2: Run frontend preflight**

Run: `make preflight-frontend`
Expected: TypeScript compiles, lint passes

- [ ] **Step 3: Push**

```bash
git push
```
