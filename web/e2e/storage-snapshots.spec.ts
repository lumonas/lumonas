import { expect, test } from '@playwright/test'

// Storage snapshot flows against the MSW mock backend.

test.describe('storage snapshots', () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('lumonas-onboarded', '1')
    })
    await page.goto('/storage')
  })

  test('overview lists seeded snapshots', async ({ page }) => {
    await expect(page.getByText('Snapshots', { exact: true })).toBeVisible()
    await expect(page.getByText('nightly-20260912T020000Z')).toBeVisible()
  })

  test('creating a snapshot adds a record', async ({ page }) => {
    await expect(page.getByText('nightly-20260912T020000Z')).toBeVisible()
    const card = page.locator('div[data-slot="card"]', { has: page.getByText('Snapshots', { exact: true }) })
    const deleteButtons = card.getByRole('button', { name: 'Delete' })
    const before = await deleteButtons.count()
    await page.getByRole('button', { name: 'Snapshot pool' }).click()
    await expect(deleteButtons).toHaveCount(before + 1, { timeout: 15_000 })
  })

  test('deleting a snapshot removes its record', async ({ page }) => {
    const card = page.locator('div[data-slot="card"]', { has: page.getByText('Snapshots', { exact: true }) })
    await expect(page.getByText('nightly-20260912T020000Z')).toBeVisible()
    await card.getByRole('button', { name: 'Delete' }).first().click()
    await expect(page.getByText('nightly-20260912T020000Z')).toHaveCount(0, { timeout: 15_000 })
  })
})
