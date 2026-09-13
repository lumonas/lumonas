import { expect, test } from '@playwright/test'

// Share management flows against the MSW mock backend.

test.describe('shares', () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('lumonas-onboarded', '1')
    })
    await page.goto('/shares')
  })

  test('shares page lists seeded shares', async ({ page }) => {
    await expect(page.getByRole('heading', { name: 'Shares' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Create share' })).toBeVisible()
  })

  test('create wizard adds a share to the list', async ({ page }) => {
    await page.getByRole('button', { name: 'Create share' }).click()
    await page.getByLabel('Name').fill('e2e-share')
    await page.getByLabel('Relative path').fill('/e2e')
    await page.getByRole('button', { name: 'Create share' }).last().click()

    await expect(page.getByText('e2e-share').first()).toBeVisible({ timeout: 15_000 })
  })
})
