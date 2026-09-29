import { expect, test } from '@playwright/test'

test('shows a local-network setup guide for Immich phone backup', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('lumonas-onboarded', '1'))
  await page.goto('/docker?tab=apps')
  await page.getByRole('button', { name: 'Set up phone backup' }).click()
  await expect(page.getByText('Install Immich from the')).toBeVisible()
  await expect(page.getByText(/enable automatic backup/)).toBeVisible()
  await expect(page.getByLabel('NAS address on your home network')).toHaveValue(/^(localhost|127\.0\.0\.1)$/)
  await expect(page.getByRole('link', { name: 'Open Immich' })).toHaveAttribute('href', /^http:\/\/(localhost|127\.0\.0\.1):2283$/)
})
