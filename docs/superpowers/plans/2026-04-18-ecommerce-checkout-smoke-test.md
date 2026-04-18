# Ecommerce Checkout Smoke Test Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the silent checkout failure caused by SameSite cookie misconfiguration, add a checkout smoke test, and upgrade QA smoke to Playwright.

**Architecture:** Make the auth service's SameSite cookie policy configurable via env var, set production/QA ConfigMaps to `SameSite=None` + `Secure` + `Domain=.kylebradshaw.dev`, add a Smoke Test Widget product with unlimited stock, write a Playwright API test covering login → cart → checkout → assertions, and upgrade the QA CI smoke job from curl to Playwright.

**Tech Stack:** Go (Gin), Playwright (TypeScript), Kustomize, GitHub Actions

**Spec:** `docs/superpowers/specs/2026-04-18-ecommerce-checkout-smoke-test-design.md`

---

### Task 1: Make SameSite cookie policy configurable

**Files:**
- Modify: `go/auth-service/cmd/server/main.go:130-136`
- Modify: `go/auth-service/internal/handler/auth_test.go:77-84`

- [ ] **Step 1: Write a test for SameSite=None cookie config**

Add this test to `go/auth-service/internal/handler/auth_test.go` after the existing `TestLogout_NoToken` test (end of file):

```go
func TestRegister_SameSiteNone_SetsCookieCorrectly(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := newMockUserRepo()
	svc := service.NewAuthService(repo, "test-secret", 900000, 604800000)
	cfg := handler.CookieConfig{
		Secure:   true,
		Domain:   ".example.com",
		SameSite: http.SameSiteNoneMode,
	}
	h := handler.NewAuthHandler(svc, nil, service.NewTokenDenylist(nil), 15*time.Minute, 7*24*time.Hour, cfg)

	router := testRouter()
	router.POST("/auth/register", h.Register)

	body, _ := json.Marshal(model.RegisterRequest{
		Email:    "samesite@example.com",
		Password: "password123456",
		Name:     "SameSite Test",
	})

	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	cookies := w.Result().Cookies()
	var accessCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "access_token" {
			accessCookie = c
			break
		}
	}
	if accessCookie == nil {
		t.Fatal("access_token cookie not found")
	}
	if accessCookie.Domain != ".example.com" {
		t.Errorf("expected domain .example.com, got %s", accessCookie.Domain)
	}
	if !accessCookie.Secure {
		t.Error("expected Secure flag to be true")
	}
	if accessCookie.SameSite != http.SameSiteNoneMode {
		t.Errorf("expected SameSite=None, got %v", accessCookie.SameSite)
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

The test uses the existing `CookieConfig` struct which already supports all these fields. The handler already reads them. This test should pass immediately.

Run: `cd go/auth-service && go test ./internal/handler/ -run TestRegister_SameSiteNone -v`

Expected: PASS

- [ ] **Step 3: Make SameSite configurable via env var in main.go**

In `go/auth-service/cmd/server/main.go`, replace lines 130-136:

```go
	cookieSecure := os.Getenv("COOKIE_SECURE") == "true"
	cookieDomain := os.Getenv("COOKIE_DOMAIN")
	cookieCfg := handler.CookieConfig{
		Secure:   cookieSecure,
		Domain:   cookieDomain,
		SameSite: http.SameSiteLaxMode,
	}
```

With:

```go
	cookieSecure := os.Getenv("COOKIE_SECURE") == "true"
	cookieDomain := os.Getenv("COOKIE_DOMAIN")
	cookieSameSite := http.SameSiteLaxMode
	switch os.Getenv("COOKIE_SAMESITE") {
	case "none":
		cookieSameSite = http.SameSiteNoneMode
	case "strict":
		cookieSameSite = http.SameSiteStrictMode
	}
	cookieCfg := handler.CookieConfig{
		Secure:   cookieSecure,
		Domain:   cookieDomain,
		SameSite: cookieSameSite,
	}
```

- [ ] **Step 4: Run all auth-service tests**

Run: `cd go/auth-service && go test ./... -v`

Expected: All tests PASS

- [ ] **Step 5: Commit**

```bash
git add go/auth-service/cmd/server/main.go go/auth-service/internal/handler/auth_test.go
git commit -m "feat(auth): make SameSite cookie policy configurable via COOKIE_SAMESITE env var"
```

---

### Task 2: Set cookie config in K8s ConfigMaps

**Files:**
- Modify: `go/k8s/configmaps/auth-service-config.yml`
- Modify: `k8s/overlays/qa-go/kustomization.yaml`

- [ ] **Step 1: Add cookie env vars to base auth-service ConfigMap**

In `go/k8s/configmaps/auth-service-config.yml`, add these three lines to the `data:` section after the existing entries:

```yaml
  COOKIE_DOMAIN: ".kylebradshaw.dev"
  COOKIE_SECURE: "true"
  COOKIE_SAMESITE: "none"
```

The full file should be:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: auth-service-config
  namespace: go-ecommerce
data:
  DATABASE_URL: postgres://taskuser:taskpass@postgres.java-tasks.svc.cluster.local:5432/ecommercedb?sslmode=disable
  ALLOWED_ORIGINS: http://localhost:3000,https://kylebradshaw.dev
  PORT: "8091"
  OTEL_EXPORTER_OTLP_ENDPOINT: "jaeger.monitoring.svc.cluster.local:4317"
  COOKIE_DOMAIN: ".kylebradshaw.dev"
  COOKIE_SECURE: "true"
  COOKIE_SAMESITE: "none"
```

No QA overlay patch needed — `.kylebradshaw.dev` covers both `qa.kylebradshaw.dev` and `qa-api.kylebradshaw.dev`, and `Secure=true` / `SameSite=None` apply to both environments.

- [ ] **Step 2: Commit**

```bash
git add go/k8s/configmaps/auth-service-config.yml
git commit -m "fix(k8s): add cookie domain/secure/samesite config for cross-origin auth"
```

---

### Task 3: Add Smoke Test Widget product to seed data

**Files:**
- Modify: `go/ecommerce-service/seed.sql`

- [ ] **Step 1: Add smoke product to seed.sql**

Append the following block to the end of `go/ecommerce-service/seed.sql` (after the existing smoke-test user insert):

```sql

-- Smoke-test product with effectively unlimited stock so automated tests
-- never deplete inventory or affect the demo catalog.
INSERT INTO products (name, description, price, category, image_url, stock)
SELECT 'Smoke Test Widget', 'Reserved for automated smoke tests', 100, 'Electronics', '', 999999
WHERE NOT EXISTS (SELECT 1 FROM products WHERE name = 'Smoke Test Widget');
```

- [ ] **Step 2: Commit**

```bash
git add go/ecommerce-service/seed.sql
git commit -m "feat(seed): add Smoke Test Widget product with unlimited stock"
```

---

### Task 4: Add checkout smoke test

**Files:**
- Modify: `frontend/e2e/smoke-prod/smoke.spec.ts`

- [ ] **Step 1: Add checkout lifecycle test**

In `frontend/e2e/smoke-prod/smoke.spec.ts`, add the following test inside the existing `test.describe("Go ecommerce smoke tests")` block, after the login test (before the closing `});` on line 285):

```typescript
  test("full checkout lifecycle: cart → order → verify", async ({ request }) => {
    expect(
      SMOKE_PASSWORD,
      "SMOKE_GO_PASSWORD env var must be set for this test"
    ).toBeTruthy();

    // Step 1: Login and get an authenticated API context with cookies
    const authContext = await request.newContext();
    const loginRes = await authContext.post(`${API_URL}/go-auth/auth/login`, {
      data: { email: SMOKE_EMAIL, password: SMOKE_PASSWORD },
    });
    expect(loginRes.status()).toBe(200);

    // Step 2: Find the Smoke Test Widget product
    const productsRes = await authContext.get(`${API_URL}/go-api/products`);
    expect(productsRes.status()).toBe(200);
    const productsBody = await productsRes.json();
    const smokeProduct = productsBody.products.find(
      (p: { name: string }) => p.name === "Smoke Test Widget"
    );
    expect(
      smokeProduct,
      "Smoke Test Widget must exist in product catalog (check seed.sql)"
    ).toBeDefined();

    // Step 3: Add to cart
    const addRes = await authContext.post(`${API_URL}/go-api/cart`, {
      data: { productId: smokeProduct.id, quantity: 1 },
      headers: { "Idempotency-Key": crypto.randomUUID() },
    });
    expect(addRes.status()).toBe(201);

    // Step 4: Verify cart contents
    const cartRes = await authContext.get(`${API_URL}/go-api/cart`);
    expect(cartRes.status()).toBe(200);
    const cartBody = await cartRes.json();
    expect(cartBody.items.length).toBeGreaterThan(0);
    const cartItem = cartBody.items.find(
      (i: { productId: string }) => i.productId === smokeProduct.id
    );
    expect(cartItem, "Smoke product must be in cart").toBeDefined();

    // Step 5: Checkout
    const orderRes = await authContext.post(`${API_URL}/go-api/orders`, {
      headers: { "Idempotency-Key": crypto.randomUUID() },
    });
    expect(orderRes.status()).toBe(201);
    const order = await orderRes.json();
    expect(order.status).toBe("pending");
    expect(order.total).toBe(smokeProduct.price);

    // Step 6: Cart should be empty after checkout
    const emptyCartRes = await authContext.get(`${API_URL}/go-api/cart`);
    expect(emptyCartRes.status()).toBe(200);
    const emptyCartBody = await emptyCartRes.json();
    expect(emptyCartBody.items).toHaveLength(0);

    // Step 7: Checkout on empty cart should fail
    const emptyCheckoutRes = await authContext.post(
      `${API_URL}/go-api/orders`,
      {
        headers: { "Idempotency-Key": crypto.randomUUID() },
      }
    );
    expect(emptyCheckoutRes.status()).toBe(400);
    const emptyErr = await emptyCheckoutRes.json();
    expect(emptyErr.error.code).toBe("EMPTY_CART");

    await authContext.dispose();
  });
```

- [ ] **Step 2: Run linting**

Run: `cd frontend && npx eslint e2e/smoke-prod/smoke.spec.ts`

Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add frontend/e2e/smoke-prod/smoke.spec.ts
git commit -m "test(smoke): add full ecommerce checkout lifecycle test"
```

---

### Task 5: Upgrade QA smoke job to Playwright

**Files:**
- Modify: `.github/workflows/ci.yml:991-1013`

- [ ] **Step 1: Replace QA smoke job with Playwright-based job**

Replace the `smoke-qa` job (lines 991-1013) in `.github/workflows/ci.yml` with:

```yaml
  smoke-qa:
    name: QA Smoke Tests
    runs-on: ubuntu-latest
    needs: [deploy-qa]
    if: needs.deploy-qa.result == 'success'
    defaults:
      run:
        working-directory: frontend
    steps:
      - uses: actions/checkout@v4

      - name: Set up Node
        uses: actions/setup-node@v4
        with:
          node-version: "20"
          cache: npm
          cache-dependency-path: frontend/package-lock.json

      - name: Install dependencies
        run: npm ci

      - name: Install Playwright browsers
        run: npx playwright install --with-deps chromium

      - name: Wait for deployment to stabilize
        run: sleep 30

      - name: Run smoke tests
        env:
          SMOKE_FRONTEND_URL: https://qa.kylebradshaw.dev
          SMOKE_API_URL: https://qa-api.kylebradshaw.dev
          SMOKE_GRAPHQL_URL: https://qa-api.kylebradshaw.dev/graphql
          SMOKE_GO_PASSWORD: ${{ secrets.SMOKE_GO_PASSWORD }}
        run: npx playwright test --config=playwright.smoke.config.ts

      - name: Upload Playwright artifacts on failure
        if: failure()
        uses: actions/upload-artifact@v4
        with:
          name: smoke-qa-playwright-report
          path: |
            frontend/test-results/
            frontend/playwright-report/
          retention-days: 14
          if-no-files-found: ignore
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci(smoke): upgrade QA smoke tests from curl to Playwright"
```

---

### Task 6: Run preflight checks and push

- [ ] **Step 1: Run Go preflight**

Run: `make preflight-go`

Expected: All lint + tests pass

- [ ] **Step 2: Run frontend preflight**

Run: `make preflight-frontend`

Expected: tsc + lint + build pass

- [ ] **Step 3: Push to qa**

```bash
git push origin qa
```

- [ ] **Step 4: Watch CI**

Run: `gh run list --branch qa --limit 1` to get the run ID, then `gh run watch <id> --exit-status`

Expected: All jobs pass, including the new Playwright-based QA smoke test

- [ ] **Step 5: Verify checkout works on QA**

Test the fix manually by visiting `qa.kylebradshaw.dev/go/ecommerce`, logging in, adding a product to cart, and completing checkout. The order should succeed (no silent failure).
