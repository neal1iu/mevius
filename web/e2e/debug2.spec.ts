import { test, expect } from "@playwright/test";
test("catalog UI has no unhandled browser errors", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/");
  await page
    .getByPlaceholder("API token")
    .fill(process.env.MEVIUS_API_TOKEN || "test-api-token-for-e2e");
  await page.getByRole("button", { name: "Save" }).click();
  for (const name of ["Inventory", "Connections", "Projects"]) {
    await page.getByRole("link", { name, exact: true }).click();
    await expect(page.locator("h1")).toBeVisible();
  }
  expect(errors).toEqual([]);
});
