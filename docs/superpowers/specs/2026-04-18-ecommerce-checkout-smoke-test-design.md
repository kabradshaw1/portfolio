# Ecommerce Checkout Smoke Test & Cookie Fix

**Date:** 2026-04-18
**Status:** Approved
**Scope:** Fix silent checkout failure (cookie/CORS), add checkout smoke test, upgrade QA smoke pipeline

## Problem

Checkout on the Go ecommerce store silently fails in both QA and likely production. The frontend shows no error — the order just doesn't go through. Additionally, no smoke test validates the checkout flow, so this regression went undetected.

## Root Cause: SameSite Cookie Bug

The auth service sets cookies with `SameSite=Lax` (hardcoded in `go/auth-service/cmd/server/main.go:135`) and no `Domain` attribute. The frontend (`qa.kylebradshaw.dev`) makes `fetch` POST requests to the API (`qa-api.kylebradshaw.dev`). Browsers treat this as cross-site and `SameSite=Lax` blocks cookies on cross-site non-navigational requests (fetch, XHR). The ecommerce service receives no `access_token` cookie, returns 401, `goApiFetch` attempts a silent refresh (which also fails for the same reason), and the checkout silently fails.

Production has the same topology: `kylebradshaw.dev` → `api.kylebradshaw.dev`.

## Part 1: Cookie Configuration Fix

### Changes to `go/auth-service`

Make `SameSite` configurable via env var instead of hardcoded:

**`go/auth-service/cmd/server/main.go`:**
- Read `COOKIE_SAMESITE` env var (values: `none`, `lax`, `strict`; default: `lax`)
- Map string to `http.SameSite` constant
- Pass to `CookieConfig`

### ConfigMap Changes

**`go/k8s/configmaps/auth-service-config.yml` (base/production):**
```yaml
COOKIE_DOMAIN: ".kylebradshaw.dev"
COOKIE_SECURE: "true"
COOKIE_SAMESITE: "none"
```

**`k8s/overlays/qa-go/kustomization.yaml`:**
No patch needed for these values — `.kylebradshaw.dev` covers both `qa.kylebradshaw.dev` and `qa-api.kylebradshaw.dev`. `Secure=true` and `SameSite=None` apply to both environments.

### Why This Combination

- `Domain=.kylebradshaw.dev`: Cookie is sent to all `*.kylebradshaw.dev` subdomains, so `api.kylebradshaw.dev` and `qa-api.kylebradshaw.dev` both receive it.
- `SameSite=None`: Required for cross-origin fetch requests to include cookies.
- `Secure=true`: Required by browsers when `SameSite=None`. Both prod and QA use HTTPS via Cloudflare.

### Local Development

Local dev uses `localhost` — no subdomain split, so the defaults (`SameSite=Lax`, no domain, non-secure) still work. The env vars are only set in K8s ConfigMaps.

## Part 2: Smoke Test Product

Add a dedicated product to `go/ecommerce-service/seed.sql` with effectively unlimited stock:

```sql
INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Smoke Test Widget', 'Reserved for automated smoke tests', 100, 'Electronics', '', 999999
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Smoke Test Widget');
```

- Price: 100 cents ($1.00) — easy to assert in tests
- Stock: 999999 — never depletes from smoke test runs
- The smoke test finds this product by name, so it's decoupled from product ID

## Part 3: Checkout Smoke Test

Add a new test to the existing `test.describe("Go ecommerce smoke tests")` block in `frontend/e2e/smoke-prod/smoke.spec.ts`.

### Test: Full checkout lifecycle

1. **Login** — POST `/go-auth/auth/login` with smoke credentials, capture cookies from response
2. **Find smoke product** — GET `/go-api/products`, find "Smoke Test Widget" by name
3. **Add to cart** — POST `/go-api/cart` with `{ productId, quantity: 1 }` + `Idempotency-Key` header, using auth cookies
4. **Verify cart** — GET `/go-api/cart`, assert item is present with correct price
5. **Checkout** — POST `/go-api/orders` + `Idempotency-Key` header
6. **Assert order** — 201 status, order status is `pending`, total equals 100 (product price)
7. **Verify cart empty** — GET `/go-api/cart`, assert empty items array
8. **Verify empty checkout fails** — POST `/go-api/orders`, assert error code `EMPTY_CART`

### Cookie Handling in Playwright

Playwright's `request.newContext()` (API testing) handles cookies automatically — `Set-Cookie` headers from the login response are stored and sent on subsequent requests to the same domain. No manual cookie extraction needed.

### Cleanup

No cleanup needed. There's no `DELETE /orders` endpoint, and orders from the smoke user are harmless. The smoke product has unlimited stock.

## Part 4: Upgrade QA Smoke to Playwright

### Current State

`smoke-qa` in `.github/workflows/ci.yml` runs bare `curl` health checks against 7 endpoints. No E2E transaction tests.

### Change

Replace the curl-based job with a Playwright job that runs the same smoke test suite as production, pointed at QA URLs:

```yaml
env:
  SMOKE_FRONTEND_URL: https://qa.kylebradshaw.dev
  SMOKE_API_URL: https://qa-api.kylebradshaw.dev
  SMOKE_GRAPHQL_URL: https://qa-api.kylebradshaw.dev/graphql
  SMOKE_GO_PASSWORD: ${{ secrets.SMOKE_GO_PASSWORD }}
```

This includes Node setup, `npm ci`, Playwright browser install, and artifact upload on failure — mirroring the `smoke-prod` job structure.

### Health Checks

The existing health checks are covered by the smoke test suite (it tests service reachability as part of the E2E flows). No separate curl step needed.

## Files Changed

| File | Change |
|------|--------|
| `go/auth-service/cmd/server/main.go` | Read `COOKIE_SAMESITE` env var, map to `http.SameSite` |
| `go/k8s/configmaps/auth-service-config.yml` | Add `COOKIE_DOMAIN`, `COOKIE_SECURE`, `COOKIE_SAMESITE` |
| `go/ecommerce-service/seed.sql` | Add "Smoke Test Widget" product |
| `frontend/e2e/smoke-prod/smoke.spec.ts` | Add checkout lifecycle test |
| `.github/workflows/ci.yml` | Upgrade `smoke-qa` job to Playwright |

## Out of Scope

- Frontend error handling improvements (showing structured errors instead of raw text)
- Order cleanup/deletion endpoint
- Google OAuth for QA (password-only is sufficient)
