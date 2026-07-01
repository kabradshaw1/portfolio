import { test, expect } from "./fixtures";

test.describe("/ai MCP Server section", () => {
  test("Featured IR Agent leads, MCP Server follows, on /ai", async ({ page }) => {
    await page.goto("/ai");
    const sectionHeadings = page.locator("section h2");
    // The IR agent is the featured AI project, so it heads the page; the MCP
    // server section follows it.
    await expect(sectionHeadings.first()).toHaveText("Incident-Response Agent");
    await expect(sectionHeadings.nth(1)).toHaveText("MCP Server");
  });

  test("RAG Evaluation renders as a section on /ai", async ({ page }) => {
    await page.goto("/ai");
    await expect(page.locator("section#rag-evaluation h2")).toHaveText(
      "RAG Evaluation",
    );
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

  test("AI page exposes anchor navigation for the major systems", async ({
    page,
  }) => {
    await page.goto("/ai");

    const nav = page.getByRole("navigation", { name: "AI section navigation" });
    await expect(nav).toBeVisible();

    await expect(
      nav.getByRole("link", { name: "MCP Server" }),
    ).toHaveAttribute("href", "#mcp-server");
    await expect(
      nav.getByRole("link", { name: "Internal MCPs" }),
    ).toHaveAttribute("href", "#internal-mcps");
    await expect(
      nav.getByRole("link", { name: "RAG Evaluation" }),
    ).toHaveAttribute("href", "#rag-evaluation");
    await expect(
      nav.getByRole("link", { name: "Debugging Assistant" }),
    ).toHaveAttribute("href", "#debugging-assistant");
    await expect(
      nav.getByRole("link", { name: "Connect a Client" }),
    ).toHaveAttribute("href", "#connect-client");
  });

  test("MCP section highlights internal MCP ecosystems and deeper routes", async ({
    page,
  }) => {
    await page.goto("/ai");

    const section = page.locator("#internal-mcps");
    await expect(section).toBeVisible();
    await expect(
      section.getByRole("heading", {
        name: "Internal MCPs for Engineering Operations",
      }),
    ).toBeVisible();
    await expect(
      section.getByText("Evidence-backed engineering action"),
    ).toBeVisible();

    await expect(
      section.getByRole("heading", { name: "Observability MCP" }),
    ).toBeVisible();
    await expect(section.getByText("Prometheus")).toBeVisible();
    await expect(section.getByText("Loki")).toBeVisible();
    await expect(section.getByText("Jaeger")).toBeVisible();
    await expect(section.getByText("Grafana")).toBeVisible();
    await expect(section.getByText("5 dashboards")).toBeVisible();
    await expect(section.getByText("16 alert rules")).toBeVisible();
    await expect(section.getByText("get_system_health")).toBeVisible();
    await expect(
      section.getByText("investigate_checkout", { exact: true }),
    ).toBeVisible();
    await expect(section.getByText("get_trace", { exact: true })).toBeVisible();
    await expect(
      section.getByRole("link", { name: "See the full observability platform" }),
    ).toHaveAttribute("href", "/observability");

    await expect(
      section.getByRole("heading", { name: "Eval MCP service" }),
    ).toBeVisible();
    await expect(section.getByText("Eval API")).toBeVisible();
    await expect(section.getByText("RAG collections")).toBeVisible();
    await expect(section.getByText("dataset fixtures")).toBeVisible();
    await expect(section.getByText("evaluation runs")).toBeVisible();
    await expect(section.getByText("experiments")).toBeVisible();
    await expect(section.getByText("rerank")).toBeVisible();
    await expect(section.getByText("top_k")).toBeVisible();
    await expect(section.getByText("start_eval_run")).toBeVisible();
    await expect(section.getByText("compare_eval_runs")).toBeVisible();
    await expect(section.getByText("get_worst_eval_cases")).toBeVisible();
    await expect(
      section.getByRole("link", { name: "Open the RAG evaluation workflow" }),
    ).toHaveAttribute("href", "/ai/eval");

    await expect(
      section.getByRole("heading", { name: "QA MCP" }),
    ).toBeVisible();
    await expect(
      section.getByText(/A compact internal MCP.*weak-topic tracking/),
    ).toBeVisible();
    await expect(section.getByText("Example tool-call trace")).toBeVisible();
    await expect(
      section.getByText("observability.investigate_checkout"),
    ).toBeVisible();
    await expect(section.getByText("Why this matters")).toBeVisible();
  });

  test("Tool catalog renders on /go AI Assistant tab", async ({ page }) => {
    await page.goto("/go");
    await page.getByRole("button", { name: "AI Assistant" }).click();
    await expect(page.getByText(/twelve tools/i)).toBeVisible();
  });
});
