# Primary Nav + Go Sub-Header Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add active-route indicators to the top nav with section links for AI/Java/Go, plus a Go-specific sub-header with cart icon and user dropdown; remove the inline login form from the ecommerce page.

**Architecture:** The top `SiteHeader` replaces one hardcoded "Go" link with three section links (AI, Java, Go) that use `usePathname()` for prefix-matched active state. A new `GoSubHeader` mounts in `/go/layout.tsx` and contains a cart icon reading from a new `GoCartProvider` context and a shadcn `DropdownMenu` with conditional items. `/go/ecommerce/page.tsx` drops its auth UI.

**Tech Stack:** Next.js 15 (client components), TypeScript, shadcn/ui (`dropdown-menu`), Tailwind, lucide-react.

**Spec:** `docs/superpowers/specs/2026-04-07-go-subheader-and-primary-nav-design.md`

---

## File Structure

### Created
- `frontend/src/components/go/GoCartProvider.tsx` — cart context (items, count, refresh)
- `frontend/src/components/go/GoSubHeader.tsx` — sub-header layout shell
- `frontend/src/components/go/GoCartIcon.tsx` — cart icon + badge
- `frontend/src/components/go/GoUserDropdown.tsx` — user dropdown menu
- `frontend/src/components/ui/dropdown-menu.tsx` — shadcn primitive (installed by CLI)

### Modified
- `frontend/src/components/SiteHeader.tsx` — three section links with active indicator
- `frontend/src/app/go/layout.tsx` — wire providers + sub-header
- `frontend/src/app/go/ecommerce/page.tsx` — remove inline auth UI

---

## Task 0: Install shadcn dropdown-menu primitive

**Files:**
- Create: `frontend/src/components/ui/dropdown-menu.tsx` (via shadcn CLI)

- [ ] **Step 1: Verify it isn't already installed**

```bash
ls frontend/src/components/ui/dropdown-menu.tsx
```

Expected: `No such file or directory`. If the file exists, skip this task entirely.

- [ ] **Step 2: Install the primitive**

```bash
cd frontend && npx shadcn@latest add dropdown-menu
```

Accept any prompts with defaults. Expected output ends with something like `Done.` or `Created components/ui/dropdown-menu.tsx`.

- [ ] **Step 3: Verify the exports exist**

```bash
grep -E "export (const|function) (DropdownMenu|DropdownMenuTrigger|DropdownMenuContent|DropdownMenuItem|DropdownMenuLabel|DropdownMenuSeparator)" frontend/src/components/ui/dropdown-menu.tsx
```

Expected: all six named exports present (`DropdownMenu`, `DropdownMenuTrigger`, `DropdownMenuContent`, `DropdownMenuItem`, `DropdownMenuLabel`, `DropdownMenuSeparator`).

- [ ] **Step 4: Type-check**

```bash
cd frontend && npx tsc --noEmit
```

Expected: no errors. If `@radix-ui/react-dropdown-menu` import is missing from `package.json`, the CLI normally installs it — verify by running `npm ls @radix-ui/react-dropdown-menu` and re-running the installer if absent.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/ui/dropdown-menu.tsx frontend/package.json frontend/package-lock.json
git commit -m "chore(frontend): add shadcn dropdown-menu primitive"
```

---

## Task 1: Primary header active-route indicators

**Files:**
- Modify: `frontend/src/components/SiteHeader.tsx`

The existing header has a single `<Link href="/go">Go</Link>`. Replace it with three section links driven by `usePathname()`. External links and the Java auth block stay unchanged.

- [ ] **Step 1: Read the current file to confirm structure**

```bash
cat frontend/src/components/SiteHeader.tsx
```

Confirm the `<Link href="/go">Go</Link>` is present and there is exactly one top-level `<nav>` element.

- [ ] **Step 2: Replace the file**

Write `frontend/src/components/SiteHeader.tsx`:

```tsx
"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { FileText } from "lucide-react";
import { useAuth } from "@/components/java/AuthProvider";
import { NotificationBell } from "@/components/java/NotificationBell";

export function SiteHeader() {
  const { user, isLoggedIn, logout } = useAuth();
  const pathname = usePathname();

  const isActive = (prefix: string) =>
    pathname === prefix || pathname.startsWith(prefix + "/");

  const navLinkClass = (prefix: string) =>
    isActive(prefix)
      ? "text-sm text-foreground border-b-2 border-foreground pb-px transition-colors"
      : "text-sm text-muted-foreground hover:text-foreground transition-colors";

  return (
    <header className="border-b border-foreground/10 bg-background">
      <div className="mx-auto flex h-14 max-w-5xl items-center justify-between px-6">
        <Link href="/" className="text-lg font-semibold">
          Kyle Bradshaw
        </Link>

        <nav className="flex items-center gap-4">
          <Link href="/ai" className={navLinkClass("/ai")}>
            AI
          </Link>
          <Link href="/java" className={navLinkClass("/java")}>
            Java
          </Link>
          <Link href="/go" className={navLinkClass("/go")}>
            Go
          </Link>
          <a
            href="https://github.com/kabradshaw1/portfolio"
            target="_blank"
            rel="noopener noreferrer"
            className="text-sm text-muted-foreground hover:text-foreground transition-colors"
          >
            Portfolio
          </a>
          <a
            href="https://github.com/kabradshaw1"
            target="_blank"
            rel="noopener noreferrer"
            className="text-sm text-muted-foreground hover:text-foreground transition-colors"
          >
            GitHub
          </a>
          <a
            href="https://www.linkedin.com/in/kyle-bradshaw-15950988/"
            target="_blank"
            rel="noopener noreferrer"
            className="text-sm text-muted-foreground hover:text-foreground transition-colors"
          >
            LinkedIn
          </a>
          <a
            href="/resume.pdf"
            className="text-muted-foreground hover:text-foreground transition-colors"
          >
            <FileText className="size-5" />
          </a>

          {isLoggedIn && (
            <>
              <NotificationBell />
              <div className="flex items-center gap-2">
                {user?.avatarUrl && (
                  <img
                    src={user.avatarUrl}
                    alt=""
                    className="size-7 rounded-full"
                  />
                )}
                <span className="text-sm text-muted-foreground">
                  {user?.name}
                </span>
                <button
                  onClick={logout}
                  className="text-sm text-muted-foreground hover:text-foreground transition-colors"
                >
                  Sign out
                </button>
              </div>
            </>
          )}
        </nav>
      </div>
    </header>
  );
}
```

- [ ] **Step 3: Type-check and lint**

```bash
cd frontend && npx tsc --noEmit && npm run lint
```

Expected: no errors. Pre-existing lint warnings from other files are fine.

- [ ] **Step 4: Manual visual verification (with `npm run dev` running)**

1. Open `http://localhost:3000/`. None of AI/Java/Go is underlined or bright.
2. Navigate to `http://localhost:3000/ai`. "AI" is underlined and in the brighter `text-foreground` color; "Java" and "Go" are muted.
3. Navigate to `http://localhost:3000/ai/rag`. "AI" still active.
4. Navigate to `http://localhost:3000/java`. "Java" active.
5. Navigate to `http://localhost:3000/go/ecommerce`. "Go" active.
6. Navigate to `http://localhost:3000/go/login`. "Go" still active.

If any step is wrong, stop and report.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/SiteHeader.tsx
git commit -m "feat(frontend): add section nav links with active-route indicator"
```

---

## Task 2: GoCartProvider context

**Files:**
- Create: `frontend/src/components/go/GoCartProvider.tsx`

Server-side `GET /cart` returns `{items: CartItem[], total: int}`. The Go
service's `CartItem` has these JSON fields: `id`, `userId`, `productId`,
`quantity`, `createdAt`, `productName` (omitempty), `productPrice`
(omitempty), `productImage` (omitempty). The badge only depends on
`quantity`, so even if more fields are added later the badge still works.

- [ ] **Step 1: Create the provider file**

Write `frontend/src/components/go/GoCartProvider.tsx`:

```tsx
"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
} from "react";
import { useGoAuth } from "@/components/go/GoAuthProvider";
import { GO_ECOMMERCE_URL, getGoAccessToken } from "@/lib/go-auth";

export interface GoCartItem {
  id: string;
  userId: string;
  productId: string;
  quantity: number;
  createdAt: string;
  productName?: string;
  productPrice?: number;
  productImage?: string;
}

interface GoCartContextType {
  items: GoCartItem[];
  count: number;
  refresh: () => Promise<void>;
}

const GoCartContext = createContext<GoCartContextType>({
  items: [],
  count: 0,
  refresh: async () => {},
});

export function useGoCart() {
  return useContext(GoCartContext);
}

export function GoCartProvider({ children }: { children: React.ReactNode }) {
  const { isLoggedIn } = useGoAuth();
  const [items, setItems] = useState<GoCartItem[]>([]);

  const refresh = useCallback(async () => {
    const token = getGoAccessToken();
    if (!token) {
      setItems([]);
      return;
    }
    try {
      const res = await fetch(`${GO_ECOMMERCE_URL}/cart`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (!res.ok) return;
      const data = await res.json();
      setItems(data.items ?? []);
    } catch {
      /* swallow — badge stays stale on network failure */
    }
  }, []);

  useEffect(() => {
    if (isLoggedIn) {
      refresh();
    } else {
      setItems([]);
    }
  }, [isLoggedIn, refresh]);

  const count = items.reduce((sum, item) => sum + item.quantity, 0);

  return (
    <GoCartContext.Provider value={{ items, count, refresh }}>
      {children}
    </GoCartContext.Provider>
  );
}
```

- [ ] **Step 2: Type-check**

```bash
cd frontend && npx tsc --noEmit
```

Expected: no errors. (The provider isn't mounted yet, so no runtime test possible — that happens in Task 6 when the layout wires it.)

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/go/GoCartProvider.tsx
git commit -m "feat(frontend): add GoCartProvider with item count"
```

---

## Task 3: GoCartIcon component

**Files:**
- Create: `frontend/src/components/go/GoCartIcon.tsx`

- [ ] **Step 1: Create the component**

Write `frontend/src/components/go/GoCartIcon.tsx`:

```tsx
"use client";

import Link from "next/link";
import { ShoppingCart } from "lucide-react";
import { useGoAuth } from "@/components/go/GoAuthProvider";
import { useGoCart } from "@/components/go/GoCartProvider";

export function GoCartIcon() {
  const { isLoggedIn } = useGoAuth();
  const { count } = useGoCart();

  if (!isLoggedIn) return null;

  return (
    <Link
      href="/go/ecommerce/cart"
      className="relative text-muted-foreground hover:text-foreground transition-colors"
      aria-label="Cart"
    >
      <ShoppingCart className="size-5" />
      {count > 0 && (
        <span className="absolute -right-2 -top-2 flex size-4 items-center justify-center rounded-full bg-foreground text-[10px] font-semibold text-background">
          {count > 99 ? "99+" : count}
        </span>
      )}
    </Link>
  );
}
```

- [ ] **Step 2: Type-check**

```bash
cd frontend && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/go/GoCartIcon.tsx
git commit -m "feat(frontend): add GoCartIcon with badge count"
```

---

## Task 4: GoUserDropdown component

**Files:**
- Create: `frontend/src/components/go/GoUserDropdown.tsx`

- [ ] **Step 1: Create the component**

Write `frontend/src/components/go/GoUserDropdown.tsx`:

```tsx
"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useGoAuth } from "@/components/go/GoAuthProvider";

function initials(name: string): string {
  return name
    .split(" ")
    .map((w) => w[0])
    .filter(Boolean)
    .slice(0, 2)
    .join("")
    .toUpperCase();
}

export function GoUserDropdown() {
  const router = useRouter();
  const { user, isLoggedIn, logout } = useGoAuth();

  return (
    <DropdownMenu>
      <DropdownMenuTrigger className="flex items-center outline-none" aria-label="Account menu">
        {isLoggedIn && user?.avatarUrl ? (
          <img
            src={user.avatarUrl}
            alt=""
            className="size-7 rounded-full"
          />
        ) : isLoggedIn && user ? (
          <span className="flex size-7 items-center justify-center rounded-full bg-muted text-xs font-semibold">
            {initials(user.name)}
          </span>
        ) : (
          <span className="text-sm text-muted-foreground hover:text-foreground transition-colors">
            Welcome
          </span>
        )}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {isLoggedIn && user ? (
          <>
            <DropdownMenuLabel className="font-normal">
              <div className="flex flex-col">
                <span className="text-sm font-medium">{user.name}</span>
                <span className="text-xs text-muted-foreground">{user.email}</span>
              </div>
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem asChild>
              <Link href="/go/ecommerce/orders">Orders</Link>
            </DropdownMenuItem>
            <DropdownMenuItem
              onSelect={() => {
                logout();
                router.push("/go/ecommerce");
              }}
            >
              Sign out
            </DropdownMenuItem>
          </>
        ) : (
          <>
            <DropdownMenuItem asChild>
              <Link href="/go/login">Sign in</Link>
            </DropdownMenuItem>
            <DropdownMenuItem asChild>
              <Link href="/go/register">Register</Link>
            </DropdownMenuItem>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
```

- [ ] **Step 2: Type-check and lint**

```bash
cd frontend && npx tsc --noEmit && npm run lint
```

Expected: no errors from the new file.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/go/GoUserDropdown.tsx
git commit -m "feat(frontend): add GoUserDropdown with conditional menu items"
```

---

## Task 5: GoSubHeader shell

**Files:**
- Create: `frontend/src/components/go/GoSubHeader.tsx`

- [ ] **Step 1: Create the component**

Write `frontend/src/components/go/GoSubHeader.tsx`:

```tsx
"use client";

import { GoCartIcon } from "@/components/go/GoCartIcon";
import { GoUserDropdown } from "@/components/go/GoUserDropdown";

export function GoSubHeader() {
  return (
    <div className="border-b border-foreground/10 bg-background">
      <div className="mx-auto flex h-12 max-w-5xl items-center justify-end gap-4 px-6">
        <GoCartIcon />
        <GoUserDropdown />
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Type-check**

```bash
cd frontend && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/go/GoSubHeader.tsx
git commit -m "feat(frontend): add GoSubHeader shell"
```

---

## Task 6: Wire providers and sub-header into /go layout

**Files:**
- Modify: `frontend/src/app/go/layout.tsx`

The existing layout is a single `GoAuthProvider` wrapper. Add `GoCartProvider` inside it and render `<GoSubHeader />` before `{children}`.

- [ ] **Step 1: Replace the layout file**

Write `frontend/src/app/go/layout.tsx`:

```tsx
import { GoAuthProvider } from "@/components/go/GoAuthProvider";
import { GoCartProvider } from "@/components/go/GoCartProvider";
import { GoSubHeader } from "@/components/go/GoSubHeader";

export default function GoLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <GoAuthProvider>
      <GoCartProvider>
        <GoSubHeader />
        {children}
      </GoCartProvider>
    </GoAuthProvider>
  );
}
```

- [ ] **Step 2: Type-check**

```bash
cd frontend && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Manual smoke test (with `npm run dev` running)**

1. Visit `/go` (the section landing page). Sub-header renders at the top with "Welcome" on the right. No cart icon.
2. Click "Welcome". Dropdown shows "Sign in" and "Register".
3. Click "Sign in". Routes to `/go/login`.
4. Go back, click "Welcome" → "Register". Routes to `/go/register`.
5. Visit `/go/ecommerce`. Sub-header still visible at the top.

If any of these fails, stop and report.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/app/go/layout.tsx
git commit -m "feat(frontend): wire GoCartProvider and GoSubHeader into go layout"
```

---

## Task 7: Remove inline auth UI from ecommerce page

**Files:**
- Modify: `frontend/src/app/go/ecommerce/page.tsx`

The ecommerce page currently has a header bar with an inline login form, a user chip, and sign-out. All of that moves to the sub-header. Strip it.

- [ ] **Step 1: Replace the file**

Write `frontend/src/app/go/ecommerce/page.tsx`:

```tsx
"use client";

import { useEffect, useState } from "react";
import { ProductCard } from "@/components/go/ProductCard";
import { GO_ECOMMERCE_URL } from "@/lib/go-auth";

interface Product {
  id: string;
  name: string;
  category: string;
  price: number;
  imageUrl?: string;
}

type Category = string;

export default function EcommercePage() {
  const [products, setProducts] = useState<Product[]>([]);
  const [categories, setCategories] = useState<Category[]>([]);
  const [activeCategory, setActiveCategory] = useState<string | null>(null);

  useEffect(() => {
    fetch(`${GO_ECOMMERCE_URL}/products`)
      .then((r) => r.json())
      .then((data) => setProducts(data?.products ?? []))
      .catch(() => {});
    fetch(`${GO_ECOMMERCE_URL}/categories`)
      .then((r) => r.json())
      .then((data) => setCategories(data?.categories ?? []))
      .catch(() => {});
  }, []);

  const filtered = activeCategory
    ? products.filter((p) => p.category === activeCategory)
    : products;

  return (
    <div className="mx-auto max-w-5xl px-6 py-8">
      <h1 className="text-2xl font-bold">Store</h1>

      {/* Category filter */}
      <div className="mt-6 flex flex-wrap gap-2">
        <button
          onClick={() => setActiveCategory(null)}
          className={`rounded-full px-3 py-1 text-sm transition-colors ${
            activeCategory === null
              ? "bg-primary text-primary-foreground"
              : "bg-muted text-muted-foreground hover:text-foreground"
          }`}
        >
          All
        </button>
        {categories.map((cat) => (
          <button
            key={cat}
            onClick={() => setActiveCategory(cat)}
            className={`rounded-full px-3 py-1 text-sm transition-colors ${
              activeCategory === cat
                ? "bg-primary text-primary-foreground"
                : "bg-muted text-muted-foreground hover:text-foreground"
            }`}
          >
            {cat}
          </button>
        ))}
      </div>

      {/* Product grid */}
      <div className="mt-8 grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4">
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
    </div>
  );
}
```

- [ ] **Step 2: Type-check and lint**

```bash
cd frontend && npx tsc --noEmit && npm run lint
```

Expected: no errors.

- [ ] **Step 3: Manual smoke test (dev server running)**

1. Visit `/go/ecommerce`. The page has only the "Store" heading, category filter, and product grid. No login form or user chip.
2. The sub-header (from the layout) still shows above the page.
3. Category filter still works.
4. Products still render.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/app/go/ecommerce/page.tsx
git commit -m "refactor(frontend): remove inline auth UI from ecommerce page"
```

---

## Task 8: Full manual smoke test

- [ ] **Step 1: Primary nav active indicator**

With dev server running:

1. `/` — none highlighted.
2. `/ai`, `/ai/rag`, `/ai/debug` — AI highlighted.
3. `/java`, `/java/tasks` — Java highlighted.
4. `/go`, `/go/ecommerce`, `/go/login`, `/go/register` — Go highlighted.

- [ ] **Step 2: Go sub-header — logged out**

1. Visit `/go/ecommerce` (log out first if needed).
2. Sub-header shows no cart icon, trigger reads "Welcome".
3. Dropdown items: Sign in, Register. Both route correctly.

- [ ] **Step 3: Go sub-header — logged in with email**

1. Register a fresh account via `/go/register`.
2. Cart icon visible, no badge (count = 0).
3. Dropdown trigger shows initials circle.
4. Open dropdown: label shows name + email; items Orders and Sign out.
5. Click Orders → routes to `/go/ecommerce/orders`.
6. Click Sign out → dropdown reverts to "Welcome", cart icon hides.

- [ ] **Step 4: Go sub-header — logged in with Google**

1. Sign in via `/go/login` → "Sign in with Google".
2. Complete consent, land back on `/go/login?code=...` → auto-redirect to `/go/ecommerce`.
3. Dropdown trigger shows Google profile picture.
4. Dropdown label shows name + email from Google.

- [ ] **Step 5: Cart badge**

1. While logged in, visit `/go/ecommerce`.
2. Click a product → go through whatever existing add-to-cart flow exists on the cart page.
3. Navigate back to any `/go/*` route. Cart icon shows badge with current quantity.

   **Note:** The cart page may or may not call `refresh()` on the provider today. If the badge doesn't update until you reload, add a `refresh()` call on the cart page after mutations. This is a known potential follow-up — flag it if it happens, don't fix it in this task.

- [ ] **Step 6: Ecommerce page cleanup**

`/go/ecommerce` shows only "Store", filter chips, grid. No auth UI.

- [ ] **Step 7: Final commit (none expected)**

No commit for this task — it's verification only. If everything passes, report back to Kyle with a summary so he can push the branch.
