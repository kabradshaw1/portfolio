import { test, expect } from "./fixtures";

test.describe("/ai MCP Server section", () => {
  test("MCP Server is the first section heading on /ai", async ({ page }) => {
    await page.goto("/ai");
    const sectionHeadings = page.locator("section h2");
    await expect(sectionHeadings.first()).toHaveText("MCP Server");
  });

  test("RAG Evaluation appears as the second section on /ai", async ({ page }) => {
    await page.goto("/ai");
    const sectionHeadings = page.locator("section h2");
    await expect(sectionHeadings.nth(1)).toHaveText("RAG Evaluation");
  });

  test("MCP section shows the verified public endpoint", async ({ page }) => {
    await page.goto("/ai");
    await expect(
      page.getByText("https://api.kylebradshaw.dev/ai-api/mcp", { exact: false }).first(),
    ).toBeVisible();
  });

  test("MCP section renders the Claude Desktop config snippet", async ({ page }) => {
    await page.goto("/ai");
    await expect(
      page.getByRole("heading", { name: "Claude Desktop", exact: false }),
    ).toBeVisible();
    await expect(page.getByText('"mcpServers"', { exact: false })).toBeVisible();
  });

  test("MCP section renders the Codex CLI config snippet", async ({ page }) => {
    await page.goto("/ai");
    await expect(
      page.getByRole("heading", { name: "Codex CLI", exact: false }),
    ).toBeVisible();
  });

  test("MCP section renders the MCP Inspector command", async ({ page }) => {
    await page.goto("/ai");
    await expect(
      page.getByText("npx @modelcontextprotocol/inspector", { exact: false }),
    ).toBeVisible();
  });

  test("MCP section CTA links to the /go shopping assistant tab", async ({
    page,
  }) => {
    await page.goto("/ai");
    const cta = page.getByRole("link", { name: /Try it on the Go section/i });
    await expect(cta).toBeVisible();
    await expect(cta).toHaveAttribute("href", "/go");
  });

  test("MCP section links to the GitHub source for the MCP server", async ({
    page,
  }) => {
    await page.goto("/ai");
    const githubLink = page.getByRole("link", { name: /View source on GitHub/i });
    await expect(githubLink).toBeVisible();
    await expect(githubLink).toHaveAttribute(
      "href",
      /github\.com\/.*\/go\/ai-service\/internal\/mcp/,
    );
  });

  test("Tool catalog renders on /ai (shared component, identifying caption)", async ({
    page,
  }) => {
    await page.goto("/ai");
    await expect(
      page.getByText(/twelve tools/i).first(),
    ).toBeVisible();
  });

  test("Tool catalog renders on /go AI Assistant tab", async ({ page }) => {
    await page.goto("/go");
    await page.getByRole("button", { name: "AI Assistant" }).click();
    await expect(page.getByText(/twelve tools/i)).toBeVisible();
  });
});
