import { expect, test } from "./fixtures";

test.describe("GalaxyVoyagers portfolio case study", () => {
  test("links the landing card to the Galaxy architecture page", async ({
    page,
  }) => {
    await page.goto("/");

    const cardLink = page.getByRole("link", {
      name: /GalaxyVoyagers\.com/i,
    });

    await expect(cardLink).toBeVisible();
    await expect(
      page.getByText(/View the architecture walkthrough/i)
    ).toBeVisible();

    await cardLink.click();

    await expect(page).toHaveURL(/\/galaxy$/);
    await expect(
      page.getByRole("heading", { name: "GalaxyVoyagers.com" })
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: /Open GalaxyVoyagers\.com/i })
    ).toHaveAttribute("href", "https://galaxyvoyagers.com");
    await expect(
      page.getByRole("heading", {
        name: "Why GraphQL Was The Right Boundary",
      })
    ).toBeVisible();
    await expect(
      page.getByText(/one operation/i)
    ).toBeVisible();
  });
});
