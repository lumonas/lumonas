import { expect, test } from '@playwright/test'

test('backup readiness shows share-level coverage and overdue recovery points', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('lumonas-onboarded', '1'))
  await page.goto('/backups?tab=overview')

  await expect(page.getByText('Recovery readiness', { exact: true })).toBeVisible()
  const coverage = page.getByText('Protected data coverage', { exact: true }).locator('..').locator('..')
  await expect(coverage.getByText('Documents', { exact: true })).toBeVisible()
  await expect(coverage.getByText('Media', { exact: true })).toBeVisible()
  await expect(coverage.getByText('Docker app data', { exact: true })).toBeVisible()
  await expect(coverage.getByText('Included in the latest verified bundle')).toBeVisible()
  await expect(coverage.getByText('Included in a verified bundle, but the latest copy is overdue')).toBeVisible()
})
