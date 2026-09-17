import { test, expect } from '@playwright/test';
const API_TOKEN = 'test-api-token-for-e2e';

test('debug accounts', async ({ page }) => {
  // Monitor network for errors
  page.on('console', msg => console.log('CONSOLE:', msg.type(), msg.text()));
  page.on('pageerror', err => console.log('PAGE ERROR:', err.message));
  page.on('response', response => {
    if (response.status() >= 400) {
      console.log('HTTP ERROR:', response.status(), response.url());
    }
  });
  
  await page.goto('/');
  await page.getByPlaceholder('API token').fill(API_TOKEN);
  await page.getByRole('button', { name: 'Save' }).click();
  await expect(page.getByRole('heading', { name: 'Projects' })).toBeVisible();
  
  await page.goto('/accounts');
  await page.waitForTimeout(5000);
  
  const html = await page.content();
  console.log('HTML after goto:', html.substring(0, 2000));
});
