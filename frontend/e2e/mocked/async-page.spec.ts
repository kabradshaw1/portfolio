import { test, expect } from "./fixtures";

test.describe("/async page", () => {
  test("renders the recruiter-facing async systems narrative", async ({ page }) => {
    await page.goto("/async");

    await expect(
      page.getByRole("heading", {
        name: "Asynchronous Systems Engineering",
        level: 1,
      }),
    ).toBeVisible();
    await expect(page.getByText("Go, Kafka, and RabbitMQ", { exact: false })).toBeVisible();
    await expect(page.getByText("command/reply queues", { exact: false })).toBeVisible();
    await expect(page.getByText("Kafka DLQ envelopes", { exact: false })).toBeVisible();
    await expect(page.getByText("Prometheus metrics", { exact: false })).toBeVisible();
    await expect(page.getByText("trace propagation", { exact: false })).toBeVisible();
  });

  test("links to the related demos", async ({ page }) => {
    await page.goto("/async");

    const relatedDemos = page
      .locator("section")
      .filter({ has: page.getByRole("heading", { name: "Related Demos" }) });

    await expect(page.getByRole("link", { name: "Go ecommerce" })).toHaveAttribute(
      "href",
      "/go/ecommerce",
    );
    await expect(page.getByRole("link", { name: "Streaming Analytics" })).toHaveAttribute(
      "href",
      "/go/analytics",
    );
    await expect(page.getByRole("link", { name: "Order Timeline" })).toHaveAttribute(
      "href",
      "/go/ecommerce/orders",
    );
    await expect(page.getByRole("link", { name: "Admin Panel" })).toHaveAttribute(
      "href",
      "/go/admin",
    );
    await expect(relatedDemos.getByRole("link", { name: "Grafana" })).toHaveAttribute(
      "href",
      /grafana\.kylebradshaw\.dev/,
    );
  });

  test("homepage and primary navigation link to /async", async ({ page }) => {
    await page.goto("/");

    await expect(page.getByRole("link", { name: "Async", exact: true })).toHaveAttribute(
      "href",
      "/async",
    );
    await expect(
      page.getByRole("link", { name: /Asynchronous Systems Engineering/ }),
    ).toHaveAttribute("href", "/async");
  });
});
