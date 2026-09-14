import { expect, test } from '@playwright/test'

// Immutable OS image-slot flows against the MSW mock backend.

test.describe('updates image slots', () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('lumonas-onboarded', '1')
    })
    await page.goto('/updates')
  })

  test('image slot staging form is present with fields', async ({ page }) => {
    await expect(page.getByText('OS image slots (immutable A/B)')).toBeVisible()
    await expect(page.getByLabel('imagePath')).toBeVisible()
    await expect(page.getByRole('button', { name: 'Stage image' })).toBeDisabled()
  })

  test('staging an image reveals the slot activation actions', async ({ page }) => {
    await page.getByLabel('imagePath').fill('/var/lib/lumonas/updates/slot-b/image')
    await page.getByLabel('version', { exact: false }).last().fill('1.4.0')
    await page.getByLabel('Image SHA-256').fill('a'.repeat(64))
    await page.getByLabel('Image size (bytes)').fill('1073741824')
    await page.getByLabel('signature').last().fill('c2lnbmF0dXJl')
    await page.getByRole('button', { name: 'Stage image' }).click()

    await expect(page.getByText(/OS image staged/).first()).toBeVisible({ timeout: 15_000 })
    await expect(page.getByText('Pending slot')).toBeVisible()
    await expect(page.getByRole('button', { name: /Write to inactive slot/ })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Commit healthy slot' })).toBeVisible()
  })
})
