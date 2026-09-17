import { test, expect } from '@playwright/test'
import { execSync } from 'child_process'

const API_TOKEN = process.env.MEVIUS_API_TOKEN || 'test-api-token-for-e2e'
const COMPOSE_DIR = process.env.COMPOSE_DIR || '.'

test.describe('mevius E2E smoke', () => {

  test('token gate → seed projects → slot bindings → accounts', async ({ page }) => {
    // 1. Token gate
    await page.goto('/')
    await expect(page.getByText('mevius')).toBeVisible()
    await expect(page.getByText('Enter your API token')).toBeVisible()

    // Enter token and submit
    await page.getByPlaceholder('API token').fill(API_TOKEN)
    await page.getByRole('button', { name: 'Save' }).click()

    // 2. Project list with seed data
    await expect(page.getByRole('heading', { name: 'Projects' })).toBeVisible()
    await expect(page.getByText('demo-shop')).toBeVisible()
    await expect(page.getByText('demo-blog')).toBeVisible()

    // 3. Navigate to demo-shop project detail
    await page.getByText('demo-shop').click()
    await expect(page.getByText('Demo e-commerce shop')).toBeVisible()
    await page.waitForTimeout(500)

    // 4. Slot list: expect repo and compute slots visible
    await expect(page.getByRole('link', { name: /Repo shop-repo/ })).toBeVisible()
    await expect(page.getByRole('link', { name: /Compute shop-worker/ })).toBeVisible()

    // 5. Click first slot to see binding detail
    await page.getByRole('link', { name: /Repo shop-repo/ }).click()
    await page.waitForTimeout(500)

    // 6. Binding status badges — should see "ok" and "error" badges
    await expect(page.getByText('ok', { exact: true }).first()).toBeVisible()
    await expect(page.getByText('error', { exact: true }).first()).toBeVisible()

    // 7. Navigate to accounts page
    await page.goto('/accounts')
    await page.waitForTimeout(2000)

    // 8. Three fixture accounts visible
    await expect(page.getByText('Fixture GitHub')).toBeVisible({ timeout: 10000 })
    await expect(page.getByText('Fixture Cloudflare')).toBeVisible()
    await expect(page.getByText('Fixture Vercel')).toBeVisible()
  })

  test('binding cards: all 4 sync_status badges render without crash', async ({ page }) => {
    await page.goto('/')
    await page.getByPlaceholder('API token').fill(API_TOKEN)
    await page.getByRole('button', { name: 'Save' }).click()
    await expect(page.getByRole('heading', { name: 'Projects' })).toBeVisible()

    // Navigate to demo-blog → static-site slot → see binding badges
    await page.getByText('demo-blog').click()
    await page.waitForTimeout(300)
    await page.getByRole('link', { name: /Static Site blog-site/ }).click()
    await page.waitForTimeout(500)

    // Should see auth_error badge on the auth error binding
    await expect(page.getByText('auth error', { exact: true }).first()).toBeVisible()
  })

  test('orphaned binding renders without error boundary', async ({ page }) => {
    await page.goto('/')
    await page.getByPlaceholder('API token').fill(API_TOKEN)
    await page.getByRole('button', { name: 'Save' }).click()
    await expect(page.getByRole('heading', { name: 'Projects' })).toBeVisible()

    // Navigate to demo-blog → dns-domain slot → see orphaned binding
    await page.getByText('demo-blog').click()
    await page.waitForTimeout(300)
    await page.getByRole('link', { name: /DNS Domain blog-domain/ }).click()
    await page.waitForTimeout(500)

    // Check orphaned badge is visible
    await expect(page.getByText('remote deleted', { exact: true }).first()).toBeVisible()
  })
})

test.describe('persistence', () => {
  test('create project survives compose down/up', async ({ page }) => {
    // Navigate to gate and enter token
    await page.goto('/')
    await page.getByPlaceholder('API token').fill(API_TOKEN)
    await page.getByRole('button', { name: 'Save' }).click()
    await expect(page.getByRole('heading', { name: 'Projects' })).toBeVisible()

    // Create new project "persist-check"
    await page.getByRole('button', { name: 'New Project' }).click()
    await page.getByPlaceholder('my-project').fill('persist-check')
    await page.getByRole('button', { name: 'Create' }).click()
    await page.waitForTimeout(500)
    await expect(page.getByText('persist-check')).toBeVisible()

    // Compose down and up
    execSync('docker compose down', { cwd: COMPOSE_DIR, timeout: 30000 })
    execSync('docker compose up -d', { cwd: COMPOSE_DIR, timeout: 60000 })

    // Wait for services to be healthy
    await page.waitForTimeout(5000)

    // Re-navigate and assert project persists
    await page.goto('/')
    await page.getByPlaceholder('API token').fill(API_TOKEN)
    await page.getByRole('button', { name: 'Save' }).click()
    await page.waitForTimeout(1000)
    await expect(page.getByText('persist-check')).toBeVisible()
  })
})