import { expect, test } from '@playwright/test'

test('shows upstream image digest comparison after checking updates', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('lumonas-onboarded', '1'))
  await page.goto('/docker?tab=images')
  await page.getByRole('button', { name: 'Check upstream image digests' }).click()
  await expect(page.getByText(/local sha256:/).first()).toBeVisible()
  await expect(page.getByText(/registry sha256:/).first()).toBeVisible()
})
