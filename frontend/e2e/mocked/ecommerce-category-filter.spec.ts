import { expect, test } from "./fixtures";

const products = [
  {
    id: "book-1",
    name: "Domain-Driven Design",
    category: "Books",
    price: 4500,
  },
  {
    id: "clothing-1",
    name: "Canvas Jacket",
    category: "Clothing",
    price: 7900,
  },
  {
    id: "electronics-1",
    name: "Smoke Test Widget",
    category: "Electronics",
    price: 1299,
  },
  {
    id: "electronics-2",
    name: "USB-C Dock",
    category: "Electronics",
    price: 8999,
  },
];

test.describe("ecommerce category filtering", () => {
  test.beforeEach(async ({ page }) => {
    await page.route("**/categories", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ categories: ["Books", "Clothing", "Electronics"] }),
      }),
    );
  });

  test("loads all products once and filters categories locally", async ({ page }) => {
    const productRequests: URL[] = [];
    const unexpectedProductRequests: string[] = [];
    let guardCategoryRequests = false;

    await page.route("**/products**", (route) => {
      const url = new URL(route.request().url());
      productRequests.push(url);
      if (guardCategoryRequests) {
        unexpectedProductRequests.push(`${url.pathname}${url.search}`);
      }

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

    await expect(page.getByText("Domain-Driven Design")).toBeVisible();
    await expect(page.getByText("Canvas Jacket")).toBeVisible();
    await expect(page.getByText("Smoke Test Widget")).toBeVisible();
    await expect(page.getByText("USB-C Dock")).toBeVisible();
    expect(productRequests.length).toBeGreaterThan(0);
    expect(
      productRequests.every(
        (request) => request.searchParams.get("limit") === "100",
      ),
    ).toBe(true);
    const initialProductRequestCount = productRequests.length;
    guardCategoryRequests = true;

    await page.getByRole("button", { name: "Clothing" }).click();
    await expect(page.getByText("Canvas Jacket")).toBeVisible();
    await expect(page.getByText("Smoke Test Widget")).toBeHidden();
    expect(productRequests).toHaveLength(initialProductRequestCount);
    expect(unexpectedProductRequests).toEqual([]);

    await page.getByRole("button", { name: "Electronics" }).click();
    await expect(page.getByText("Smoke Test Widget")).toBeVisible();
    await expect(page.getByText("USB-C Dock")).toBeVisible();
    await expect(page.getByText("Canvas Jacket")).toBeHidden();
    expect(productRequests).toHaveLength(initialProductRequestCount);
    expect(unexpectedProductRequests).toEqual([]);
  });

  test("shows a temporary unavailable message when products fail to load", async ({
    page,
  }) => {
    await page.route("**/products**", (route) =>
      route.fulfill({
        status: 503,
        contentType: "text/plain",
        body: "Service unavailable",
      }),
    );

    await page.goto("/go/ecommerce");

    await expect(page.getByText("Products are temporarily unavailable.")).toBeVisible();
    await expect(page.getByText("No products found.")).toBeHidden();
  });
});
