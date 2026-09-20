import { test, expect, type Page } from '@playwright/test'

const API_TOKEN = process.env.MEVIUS_API_TOKEN || 'test-api-token-for-e2e'

async function enter(page: Page) {
  await page.goto('/')
  await page.getByPlaceholder('API token').fill(API_TOKEN)
  await page.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByRole('heading', { name: 'Projects' })).toBeVisible()
}

test.describe('resource model smoke', () => {
  test('seeded project shows attached resources and relation subgraph', async ({ page }) => {
    await enter(page)
    await page.getByText('demo-blog').click()
    await expect(page.getByRole('link', { name: 'site' })).toBeVisible()
    await expect(page.getByRole('link', { name: 'repo' })).toBeVisible()
    await expect(page.getByText('Frontend', { exact: true })).toBeVisible()
    await expect(page.getByText(/source_repo/)).toBeVisible()
  })

  test('inventory and scoped connections render', async ({ page }) => {
    await enter(page)
    await page.getByRole('link', { name: 'Inventory' }).click()
    await expect(page.getByRole('heading', { name: 'Inventory' })).toBeVisible()
    await expect(page.getByRole('link', { name: /^demo-shop/ })).toBeVisible()
    await expect(page.getByText('example.com')).toBeVisible()
    await page.getByRole('link', { name: 'Connections' }).click()
    await expect(page.getByText('Fixture GitHub')).toBeVisible()
    await expect(page.getByText(/organization: Mevius/)).toBeVisible()
  })

  test('creating a project leaves global inventory untouched', async ({ page }) => {
    await enter(page)
    const name = `project-${Date.now()}`
    await page.getByRole('button', { name: 'New Project' }).click()
    await page.getByPlaceholder('my-project').fill(name)
    await page.getByRole('button', { name: 'Create' }).click()
    await expect(page.getByText(name)).toBeVisible()
    await page.getByRole('link', { name: 'Inventory' }).click()
    await expect(page.getByRole('link', { name: /^demo-shop/ })).toBeVisible()
  })
})
