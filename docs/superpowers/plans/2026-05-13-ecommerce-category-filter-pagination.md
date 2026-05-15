# Ecommerce Category Filter Pagination Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix `/go/ecommerce` category filters by loading the complete fixed demo catalog once, so Clothing and Electronics render from the full 41-product catalog instead of the product-service default first page.

**Architecture:** Keep the existing category-tab UX and local in-memory filtering. Change the storefront's initial product request to `/products?limit=100`, add explicit loading/error/empty states, and add focused mocked plus smoke coverage to lock the behavior down. This is frontend-only; do not change seed data, migrations, product-service defaults, API pagination behavior, or live database contents.

**Tech Stack:** Next.js App Router, React client components, TypeScript, Playwright, existing `make preflight-frontend` and `make preflight-e2e` verification.

---

## File Structure

Modify these files only:

| File | Responsibility |
| --- | --- |
| `frontend/src/app/go/ecommerce/page.tsx` | Storefront product fetch, local category filtering, loading/error/empty rendering |
| `frontend/e2e/smoke-prod/smoke.spec.ts` | Production API smoke coverage for the full fixed catalog via `limit=100` |
| `frontend/e2e/smoke-go-compose/smoke-go-ci.spec.ts` | Local/compose API smoke coverage for the full fixed catalog via `limit=100` |

Create this file:

| File | Responsibility |
| --- | --- |
| `frontend/e2e/mocked/ecommerce-category-filter.spec.ts` | Mocked browser regression coverage proving `/go/ecommerce` requests `limit=100`, renders locally filtered categories, and does not refetch products when category tabs change |

Do not edit:

- `go/product-service/**`
- database migrations or seed files
- Kubernetes manifests
- Vercel/env configuration
- `frontend/src/components/go/GoStoreProvider.tsx`
- `frontend/src/components/go/GoSubHeader.tsx`

## Task 1: Create The Feature Worktree

**Files:**
- No file edits in this task

- [ ] **Step 1: Confirm the starting branch and tree**

Run from the repo root:

```bash
pwd
git branch --show-current
git status --short
```

Expected:

- `pwd` is `/Users/kylebradshaw/repos/gen_ai_engineer`.
- Current branch may be `main`; do not implement frontend behavior changes there.
- `docs/superpowers/specs/2026-05-13-ecommerce-category-filter-pagination-design.md` and this plan may appear as untracked doc files. Leave them alone unless Kyle explicitly asks to commit docs.

- [ ] **Step 2: Create a feature worktree targeting `qa`**

Run from the repo root:

```bash
git fetch origin qa
git worktree add .codex/worktrees/ecommerce-category-filter-pagination -b fix/ecommerce-category-filter-pagination origin/qa
```

Expected:

- A new worktree exists at `.codex/worktrees/ecommerce-category-filter-pagination`.
- The new branch is `fix/ecommerce-category-filter-pagination`.

- [ ] **Step 3: Switch all implementation work into the worktree**

Run:

```bash
cd .codex/worktrees/ecommerce-category-filter-pagination
pwd
git branch --show-current
git rev-parse --show-toplevel
```

Expected:

- `pwd` and `git rev-parse --show-toplevel` both resolve under `/Users/kylebradshaw/repos/gen_ai_engineer/.codex/worktrees/ecommerce-category-filter-pagination`.
- `git branch --show-current` prints `fix/ecommerce-category-filter-pagination`.

- [ ] **Step 4: Load narrow frontend instructions inside the worktree**

Run:

```bash
sed -n '1,220p' frontend/AGENTS.md
```

Expected:

- Frontend checks are `make preflight-frontend` and `make preflight-e2e`.
- Do not use Debian as a frontend build or test worker.

## Task 2: Add Mocked UI Regression Coverage

**Files:**
- Create: `frontend/e2e/mocked/ecommerce-category-filter.spec.ts`

- [ ] **Step 1: Write the failing mocked Playwright test**

Create `frontend/e2e/mocked/ecommerce-category-filter.spec.ts` with:

```ts
import { expect, test } from "./fixtures";

const products = [
  {
    id: "book-1",
    name: "Distributed Systems Handbook",
    category: "Books",
    price: 2400,
  },
  {
    id: "clothing-1",
    name: "Performance Hoodie",
    category: "Clothing",
    price: 6400,
  },
  {
    id: "electronics-1",
    name: "Smoke Test Widget",
    category: "Electronics",
    price: 1999,
  },
  {
    id: "electronics-2",
    name: "USB-C Debug Dock",
    category: "Electronics",
    price: 8900,
  },
];

test.describe("Go ecommerce category filtering", () => {
  test("loads the full catalog once and filters categories locally", async ({
    page,
  }) => {
    const productRequests: string[] = [];

    await page.route("**/categories", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          categories: ["Books", "Clothing", "Electronics"],
        }),
      }),
    );

    await page.route("**/products**", (route) => {
      const url = new URL(route.request().url());
      productRequests.push(`${url.pathname}${url.search}`);

      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          products,
          total: products.length,
          page: 1,
          limit: 100,
          hasMore: false,
        }),
      });
    });

    await page.goto("/go/ecommerce");

    await expect(page.getByText("Distributed Systems Handbook")).toBeVisible();
    await expect(page.getByText("Performance Hoodie")).toBeVisible();
    await expect(page.getByText("Smoke Test Widget")).toBeVisible();
    expect(productRequests).toHaveLength(1);
    expect(productRequests[0]).toContain("limit=100");

    await page.getByRole("button", { name: "Clothing" }).click();
    await expect(page.getByText("Performance Hoodie")).toBeVisible();
    await expect(page.getByText("Smoke Test Widget")).toBeHidden();
    expect(productRequests).toHaveLength(1);

    await page.getByRole("button", { name: "Electronics" }).click();
    await expect(page.getByText("Smoke Test Widget")).toBeVisible();
    await expect(page.getByText("USB-C Debug Dock")).toBeVisible();
    await expect(page.getByText("Performance Hoodie")).toBeHidden();
    expect(productRequests).toHaveLength(1);
  });

  test("shows fetch failures separately from an empty catalog", async ({
    page,
  }) => {
    await page.route("**/categories", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ categories: [] }),
      }),
    );

    await page.route("**/products**", (route) =>
      route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ error: "unavailable" }),
      }),
    );

    await page.goto("/go/ecommerce");

    await expect(
      page.getByText("Products are temporarily unavailable."),
    ).toBeVisible();
    await expect(page.getByText("No products found.")).toBeHidden();
  });
});
```

- [ ] **Step 2: Run the new mocked test and confirm it fails**

Run:

```bash
cd frontend
npx playwright test e2e/mocked/ecommerce-category-filter.spec.ts
```

Expected:

- The first test fails because `frontend/src/app/go/ecommerce/page.tsx` currently calls `/products` without `limit=100`.
- The second test fails because the current page swallows fetch errors and renders `No products found.`

## Task 3: Fix The Storefront Fetch And States

**Files:**
- Modify: `frontend/src/app/go/ecommerce/page.tsx`
- Test: `frontend/e2e/mocked/ecommerce-category-filter.spec.ts`

- [ ] **Step 1: Update `frontend/src/app/go/ecommerce/page.tsx`**

Replace the file with:

```tsx
"use client";

import { useEffect, useState } from "react";
import { ProductCard } from "@/components/go/ProductCard";
import { useGoStore } from "@/components/go/GoStoreProvider";
import { GO_PRODUCT_URL } from "@/lib/go-auth";

interface Product {
  id: string;
  name: string;
  category: string;
  price: number;
  imageUrl?: string;
}

type ProductFetchState = "loading" | "success" | "error";

export default function EcommercePage() {
  const { activeCategory } = useGoStore();
  const [products, setProducts] = useState<Product[]>([]);
  const [fetchState, setFetchState] = useState<ProductFetchState>("loading");

  useEffect(() => {
    const controller = new AbortController();
    const params = new URLSearchParams({ limit: "100" });

    async function loadProducts() {
      try {
        setFetchState("loading");
        const res = await fetch(`${GO_PRODUCT_URL}/products?${params.toString()}`, {
          signal: controller.signal,
        });

        if (!res.ok) {
          throw new Error(`Product fetch failed with status ${res.status}`);
        }

        const data = await res.json();
        setProducts(data?.products ?? []);
        setFetchState("success");
      } catch (error) {
        if (controller.signal.aborted) return;
        setProducts([]);
        setFetchState("error");
      }
    }

    loadProducts();

    return () => {
      controller.abort();
    };
  }, []);

  const filtered = activeCategory
    ? products.filter((p) => p.category === activeCategory)
    : products;

  return (
    <div className="mx-auto max-w-5xl px-6 py-8">
      {fetchState === "loading" && (
        <p className="mt-12 text-center text-muted-foreground">
          Loading products...
        </p>
      )}

      {fetchState === "error" && (
        <p className="mt-12 text-center text-muted-foreground">
          Products are temporarily unavailable.
        </p>
      )}

      {fetchState === "success" && (
        <>
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4">
            {filtered.map((product) => (
              <ProductCard
                key={product.id}
                id={product.id}
                name={product.name}
                category={product.category}
                priceCents={product.price}
                imageUrl={product.imageUrl}
              />
            ))}
          </div>

          {filtered.length === 0 && (
            <p className="mt-12 text-center text-muted-foreground">
              No products found.
            </p>
          )}
        </>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Run the focused mocked test again**

Run:

```bash
cd frontend
npx playwright test e2e/mocked/ecommerce-category-filter.spec.ts
```

Expected:

- PASS.
- The products route is called exactly once.
- The products route includes `limit=100`.
- Category button clicks do not trigger additional `/products` requests.

- [ ] **Step 3: Check for formatting/type issues in the changed file**

Run:

```bash
cd frontend
npm run lint -- --file src/app/go/ecommerce/page.tsx
```

Expected:

- PASS, or no lint errors for `src/app/go/ecommerce/page.tsx`.
- If this project's Next lint command does not accept `--file`, skip this command and rely on `make preflight-frontend` in Task 5.

## Task 4: Strengthen Smoke Coverage For Full Catalog Counts

**Files:**
- Modify: `frontend/e2e/smoke-prod/smoke.spec.ts`
- Modify: `frontend/e2e/smoke-go-compose/smoke-go-ci.spec.ts`

- [ ] **Step 1: Update production smoke catalog assertions**

In `frontend/e2e/smoke-prod/smoke.spec.ts`, replace the existing test named `"products endpoint returns a non-empty catalog"` with:

```ts
  test("products endpoint returns the full fixed catalog with limit=100", async ({
    request,
  }) => {
    const res = await request.get(`${API_URL}/go-products/products?limit=100`);
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body.products)).toBe(true);
    expect(body.products.length).toBe(41);
    expect(body.total).toBe(41);

    const clothing = body.products.filter(
      (p: { category: string }) => p.category === "Clothing",
    );
    const electronics = body.products.filter(
      (p: { category: string }) => p.category === "Electronics",
    );

    expect(clothing).toHaveLength(8);
    expect(electronics).toHaveLength(9);
    expect(
      electronics.some((p: { name: string }) => p.name === "Smoke Test Widget"),
    ).toBe(true);
  });
```

Do not change the auth, cart, order, or checkout smoke tests except for the catalog lookup limit in the checkout test if desired. If touching that lookup, change only:

```ts
`${API_URL}/go-products/products?limit=50`
```

to:

```ts
`${API_URL}/go-products/products?limit=100`
```

- [ ] **Step 2: Update compose smoke catalog assertions**

In `frontend/e2e/smoke-go-compose/smoke-go-ci.spec.ts`, replace the existing test named `"product catalog returns data"` with:

```ts
  test("product catalog returns the full fixed catalog with limit=100", async ({
    request,
  }) => {
    const productsRes = await request.get(`${PRODUCT_URL}/products?limit=100`);
    expect(productsRes.status()).toBe(200);
    const productsBody = await productsRes.json();
    expect(Array.isArray(productsBody.products)).toBe(true);
    expect(productsBody.products.length).toBe(41);
    expect(productsBody.total).toBe(41);

    const clothing = productsBody.products.filter(
      (p: { category: string }) => p.category === "Clothing",
    );
    const electronics = productsBody.products.filter(
      (p: { category: string }) => p.category === "Electronics",
    );

    expect(clothing).toHaveLength(8);
    expect(electronics).toHaveLength(9);
    expect(
      electronics.some((p: { name: string }) => p.name === "Smoke Test Widget"),
    ).toBe(true);

    const categoriesRes = await request.get(`${PRODUCT_URL}/categories`);
    expect(categoriesRes.status()).toBe(200);
    const categoriesBody = await categoriesRes.json();
    expect(Array.isArray(categoriesBody.categories)).toBe(true);
    expect(categoriesBody.categories).toEqual(
      expect.arrayContaining(["Books", "Clothing", "Electronics", "Home", "Sports"]),
    );
  });
```

In the cart flow test in the same file, change only:

```ts
const productsRes = await authCtx.get(`${PRODUCT_URL}/products?limit=50`);
```

to:

```ts
const productsRes = await authCtx.get(`${PRODUCT_URL}/products?limit=100`);
```

- [ ] **Step 3: Run targeted TypeScript formatting checks through frontend preflight later**

Do not run production smoke tests directly against production as a required local step. The mandatory local verification remains the repo preflight commands in Task 5.

## Task 5: Run Required Verification

**Files:**
- No new file edits unless verification fails

- [ ] **Step 1: Run frontend preflight**

Run from the feature worktree root:

```bash
make preflight-frontend
```

Expected:

- PASS.
- If it fails, fix only failures related to the files changed by this plan unless the failure is clearly a pre-existing environment/tooling issue.

- [ ] **Step 2: Run e2e preflight**

Run from the feature worktree root:

```bash
make preflight-e2e
```

Expected:

- PASS.
- If blocked by missing browsers, local service dependencies, or platform limits, capture the exact error and leave the remaining e2e verification to CI.

- [ ] **Step 3: Inspect the final diff**

Run:

```bash
git diff -- frontend/src/app/go/ecommerce/page.tsx frontend/e2e/mocked/ecommerce-category-filter.spec.ts frontend/e2e/smoke-prod/smoke.spec.ts frontend/e2e/smoke-go-compose/smoke-go-ci.spec.ts
git status --short
```

Expected:

- Diff is limited to the four planned frontend files.
- No backend, migration, seed, database, Kubernetes, or env files changed.

## Task 6: Commit, Push, And Open PR To `qa`

**Files:**
- Commit the four planned frontend files only

- [ ] **Step 1: Commit the implementation**

Run from the feature worktree root:

```bash
git add frontend/src/app/go/ecommerce/page.tsx frontend/e2e/mocked/ecommerce-category-filter.spec.ts frontend/e2e/smoke-prod/smoke.spec.ts frontend/e2e/smoke-go-compose/smoke-go-ci.spec.ts
git commit -m "fix: load full ecommerce catalog for category filters"
```

Expected:

- Commit succeeds.
- The commit does not include root docs/spec/plan files unless Kyle explicitly requested them in the implementation branch.

- [ ] **Step 2: Push the branch**

Run:

```bash
git push -u origin fix/ecommerce-category-filter-pagination
```

Expected:

- Branch is pushed to origin.

- [ ] **Step 3: Create a PR targeting `qa`**

Run:

```bash
gh pr create \
  --base qa \
  --head fix/ecommerce-category-filter-pagination \
  --title "Fix ecommerce category filters" \
  --body "## Summary
- fetch the fixed ecommerce demo catalog with limit=100 on initial store load
- keep category switching local with no product refetch on tab changes
- add mocked UI regression coverage and full-catalog smoke assertions

## Verification
- make preflight-frontend
- make preflight-e2e

## Rollout
Frontend-only change. After merge to qa, deploy the QA frontend through the normal Vercel flow and verify /go/ecommerce Clothing shows 8 products and Electronics shows 9, including Smoke Test Widget. Promote through the normal qa-to-main flow for production frontend deployment. No Go service image rebuild, database mutation, seed rerun, Kubernetes change, or env-var change is required."
```

Expected:

- PR opens against `qa`.
- Do not watch CI unless Kyle asks.

## Rollout Note

This is a frontend-only behavior fix. Deploy the QA frontend after the PR merges to `qa`, then verify `/go/ecommerce` category tabs against QA: Clothing should show 8 products and Electronics should show 9 products including `Smoke Test Widget`. Production receives the same fix after the normal `qa` to `main` promotion and production frontend deployment; no Go service image rebuild, database write, seed rerun, Kubernetes change, or env-var change is needed.

## Acceptance Checklist

- [ ] `/go/ecommerce` fetches `${GO_PRODUCT_URL}/products?limit=100` once on page load.
- [ ] Category switching continues to filter the loaded `products` array locally.
- [ ] Category switching does not issue additional `/products` requests.
- [ ] Product fetch failure renders `Products are temporarily unavailable.` instead of `No products found.`
- [ ] Successful empty responses still render `No products found.`
- [ ] Production smoke coverage asserts 41 total products, 8 Clothing, and 9 Electronics with `limit=100`.
- [ ] Compose smoke coverage asserts 41 total products, 8 Clothing, and 9 Electronics with `limit=100`.
- [ ] No product-service, migration, seed, database, Kubernetes, or env-var changes are included.
- [ ] `make preflight-frontend` passes or a blocker is documented.
- [ ] `make preflight-e2e` passes or a blocker is documented.
