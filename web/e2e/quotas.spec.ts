import { expect, test } from '@playwright/test'

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('lumonas-onboarded', '1'))
})

test('configures a share quota and displays measured usage', async ({ page }) => {
  await page.goto('/storage?tab=quotas')
  await page.getByLabel('Quota target type').selectOption('share')
  await expect(page.getByLabel('Quota target', { exact: true }).locator('option', { hasText: 'Media' })).toHaveCount(1)
  await page.getByLabel('Quota target', { exact: true }).selectOption({ label: 'Media' })
  await page.getByLabel('Quota limit GiB').fill('30')
  const submit = page.getByRole('button', { name: 'Add quota' })
  await expect(submit).toBeEnabled()
  const saved = page.waitForResponse((response) => response.url().endsWith('/api/v1/quotas') && response.request().method() === 'PUT')
  await submit.click()
  await expect((await saved).ok()).toBeTruthy()
  await expect(page.getByText('Limit reached')).toBeVisible()
  await expect(page.getByText(/32 GB of 30 GB/)).toBeVisible()
})
