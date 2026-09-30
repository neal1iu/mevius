import { test, expect, type Page } from "@playwright/test";
const API_TOKEN = process.env.MEVIUS_API_TOKEN || "test-api-token-for-e2e";
async function enter(page: Page) {
  await page.goto("/");
  await page.getByPlaceholder("API token").fill(API_TOKEN);
  await page.getByRole("button", { name: "Save" }).click();
  await expect(
    page.getByRole("heading", { name: "Projects", exact: true }),
  ).toBeVisible();
}
test("offline directory and independent project aliases", async ({ page }) => {
  await enter(page);
  await page.getByRole("link", { name: "Example application" }).click();
  await expect(
    page.getByRole("link", { name: "source", exact: true }),
  ).toBeVisible();
  await page
    .getByLabel("Resource")
    .selectOption({ label: "Demo repository (offline)" });
  await page.getByLabel("Alias").fill(`secondary-${Date.now()}`);
  await page.getByLabel("Environment label").fill("sandbox");
  await page
    .getByRole("button", { name: "Attach resource", exact: true })
    .click();
  await expect(page.getByText(/source ·.*sandbox/)).toBeVisible();
  await page.getByRole("link", { name: "Inventory", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Resource directory" }),
  ).toBeVisible();
  await page.getByRole("link", { name: /Demo repository/ }).click();
  await expect(page.getByText(/origin external/)).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Submit Delete remote resource" }),
  ).toBeDisabled();
  await page.getByRole("link", { name: "Connections", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Provider authorizations" }),
  ).toBeVisible();
});
test("creating a project preserves shared inventory", async ({ page }) => {
  await enter(page);
  const name = `project-${Date.now()}`;
  await page.getByLabel("Project name").fill(name);
  await page.getByRole("button", { name: "Create project" }).click();
  await expect(page.getByRole("link", { name })).toBeVisible();
  await page.getByRole("link", { name: "Inventory", exact: true }).click();
  await expect(
    page.getByRole("link", { name: /Demo repository/ }),
  ).toBeVisible();
});
