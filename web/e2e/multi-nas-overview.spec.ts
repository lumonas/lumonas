import { expect, test } from '@playwright/test'

test('shows peer reachability and recovery replication freshness', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('lumonas-onboarded', '1'))
  await page.goto('/backups?tab=replication')
  await expect(page.getByText('Paired systems').locator('..')).toContainText('1')
  await expect(page.getByText('Online now').locator('..')).toContainText('1')
  await expect(page.getByText('Need review').locator('..')).toContainText('1')
  await expect(page.getByText("Parents' NAS").last()).toBeVisible()
  await expect(page.getByText('online', { exact: true })).toBeVisible()
  await expect(page.getByText('Remote LumoNAS 0.0.9-dev · this NAS 0.1.0-dev · health healthy')).toBeVisible()
  await expect(page.getByText('version mismatch')).toBeVisible()
  await expect(page.getByText(/versions differ/)).toBeVisible()
  await expect(page.getByText(/last copy/)).toBeVisible()
  await expect(page.getByRole('link', { name: 'Open NAS' })).toHaveAttribute('href', 'https://parents.example.net')
})
