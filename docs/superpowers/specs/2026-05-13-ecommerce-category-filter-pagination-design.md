# Ecommerce Category Filter Pagination Design

Date: 2026-05-13

## Goal

Fix the production ecommerce store category filters so every category shows the
products that exist in `productdb`, including Clothing and all Electronics
items. The UI should not depend on whichever products happen to appear on the
first paginated API response.

## Problem

The production and QA databases both contain the expected seeded catalog:

- Books: 8
- Clothing: 8
- Electronics: 9, including `Smoke Test Widget`
- Home: 8
- Sports: 8

The production store page still shows no Clothing products and only one
Electronics product because the frontend loads only the default first page:

```ts
fetch(`${GO_PRODUCT_URL}/products`)
```

The product API defaults to `limit=20`. With the current `created_at DESC` sort,
that first page contains:

- Electronics: 1
- Sports: 8
- Books: 8
- Home: 3
- Clothing: 0

The UI then filters that partial page in memory:

```ts
products.filter((p) => p.category === activeCategory)
```

This makes the category tabs look like a seed-data or migration failure even
though the database is correct.

## Current State

Relevant files:

| File | Current behavior |
| --- | --- |
| `frontend/src/app/go/ecommerce/page.tsx` | Fetches `/products` once on mount and filters the returned page client-side |
| `frontend/src/components/go/GoStoreProvider.tsx` | Fetches `/categories` and stores `activeCategory` |
| `frontend/src/components/go/GoSubHeader.tsx` | Renders category buttons and updates `activeCategory` |
| `go/product-service/internal/handler/product.go` | Defaults product list requests to `limit=20`; supports `category`, `page`, `limit`, `sort`, and `cursor` |
| `go/product-service/internal/repository/product.go` | Applies category filtering in SQL before pagination |

The API already has the correct primitive: `/products?category=Clothing` returns
category-filtered rows before pagination. The bug is that the store page is not
using it.

## Decision

Fetch the full demo catalog once and keep client-side category filtering.

The product catalog is intentionally fixed demo data and is not expected to
grow. In that context, the simplest correct fix is to request enough products
for the whole catalog on initial page load:

- All products view: `/products?limit=100`

Category buttons can continue filtering the in-memory `products` array.

This is the smallest production-grade fix for this demo catalog because:

- It uses the existing API contract and SQL filtering path.
- It avoids changing seed data, database state, or product-service behavior.
- It keeps the current category-tab UX intact.
- It avoids a network request on every category change.
- It fixes both QA and production once the frontend is deployed.
- It avoids an unbounded endpoint response while covering the complete fixed
  catalog.

The `limit=100` value matches the product-service validation max and the
existing categories endpoint cap. It is acceptable for this portfolio store
catalog because expanding the product catalog is not planned. If that changes
later, the store should move to backend-filtered pagination or a visible
load-more UI rather than silent client-side partial filtering.

## Design

### Frontend data flow

Update `frontend/src/app/go/ecommerce/page.tsx` so the initial product fetch
asks for the complete fixed demo catalog.

Build the URL with `URLSearchParams`:

```ts
const params = new URLSearchParams({ limit: "100" });
fetch(`${GO_PRODUCT_URL}/products?${params.toString()}`)
```

Keep the existing local category filtering:

```ts
const filtered = activeCategory
  ? products.filter((p) => p.category === activeCategory)
  : products;
```

The important change is that `products` now contains the complete demo catalog,
not only page 1.

### Loading and empty states

The current page silently swallows fetch errors and shows an empty state. Keep
the scope focused, but make the state less misleading:

- Show the existing grid once products are loaded.
- Show "No products found." only after a successful response with zero products.
- Track fetch failure separately and show a concise unavailable message.

This prevents a backend, network, or env-var issue from looking like an empty
category.

### Race handling

No category-change fetch is needed, so category switching stays synchronous
after the initial product load. Keep a local `cancelled` flag or
`AbortController` for the initial effect so the page does not update state after
unmount.

### Category behavior

Do not normalize category case in the frontend. Category values come from
`/categories` and product rows come from `/products`; both should be compared
exactly as provided by the API. This avoids introducing a second category
mapping layer.

### Product-service behavior

No product-service behavior change is required.

The backend already:

- Accepts `category`.
- Applies category filtering before pagination.
- Validates `limit` up to `100`.
- Returns `total`, `limit`, and `hasMore`.

## Files

| File | Change |
| --- | --- |
| `frontend/src/app/go/ecommerce/page.tsx` | Fetch the complete demo catalog with `limit=100`; keep local category filtering; add minimal loading/error state |
| `frontend/e2e/smoke-prod/smoke.spec.ts` | Add or strengthen API smoke assertions that product categories include Clothing and full Electronics counts with `limit=100` |
| `frontend/e2e/smoke-go-compose/smoke-go-ci.spec.ts` | Add local/compose coverage that `products?limit=100` includes Clothing and the expected Electronics rows |

## Acceptance Criteria

- Production store "All" view can render all 41 seeded products from the current
  catalog.
- Clothing category shows 8 products in production and QA.
- Electronics category shows 9 products in production and QA, including `Smoke
  Test Widget`.
- Category tabs continue to come from `/categories`.
- Category switching does not trigger additional product-list network requests.
- No database seed, migration, or live database mutation is needed.
- A product fetch failure does not render as "No products found."
- Rapid category switching filters the already-loaded product array
  synchronously.

## Verification

Local verification before commit:

```bash
make preflight-frontend
make preflight-e2e
```

Targeted manual/API checks:

```bash
curl -sS "https://api.kylebradshaw.dev/go-products/products?category=Clothing&limit=100"
curl -sS "https://api.kylebradshaw.dev/go-products/products?category=Electronics&limit=100"
curl -sS "https://qa-api.kylebradshaw.dev/go-products/products?category=Clothing&limit=100"
curl -sS "https://qa-api.kylebradshaw.dev/go-products/products?category=Electronics&limit=100"
curl -sS "https://api.kylebradshaw.dev/go-products/products?limit=100"
curl -sS "https://qa-api.kylebradshaw.dev/go-products/products?limit=100"
```

Expected counts:

- All products: 41
- Clothing: 8
- Electronics: 9

Browser verification after deployment:

- Visit `https://kylebradshaw.dev/go/ecommerce`.
- Select `Clothing`; confirm clothing cards appear.
- Select `Electronics`; confirm all electronics cards appear, not only `Smoke
  Test Widget`.
- In browser devtools, confirm category switching does not refetch `/products`.
- Repeat against the QA frontend if it is deployed.

## Rollout

This is a frontend-only behavior fix. Per branch rules, implement in a feature
worktree and open a PR to `qa`.

The fix does not require Kubernetes changes, database writes, seed reruns, or
image rebuilds for Go services. Vercel will need a frontend deployment for QA
and production after merge through the normal branch flow.

## Out Of Scope

- Redesigning the store page.
- Adding infinite scroll or a full pagination UI.
- Refetching products on every category change.
- Changing product-service default `limit`.
- Changing seed data.
- Mutating production or QA databases.
- Normalizing category values in the database.
