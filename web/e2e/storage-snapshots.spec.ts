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
    await expect(page.getByText('nightly', { exact: true })).toBeVisible()
  })

  test('creating a snapshot adds a record', async ({ page }) => {
    await expect(page.getByText('nightly', { exact: true })).toBeVisible()
    const card = page.locator('div[data-slot="card"]', { has: page.getByText('Snapshots', { exact: true }) })
    const deleteButtons = card.getByRole('button', { name: 'Delete' })
    const before = await deleteButtons.count()
    await page.getByRole('button', { name: 'Snapshot pool' }).click()
    await expect(deleteButtons).toHaveCount(before + 1, { timeout: 15_000 })
  })

  test('manual snapshot creation can lock its restore point against deletion', async ({ page }) => {
    const card = page.locator('div[data-slot="card"]', { has: page.getByText('Snapshots', { exact: true }) })
    const createResponse = page.waitForResponse((response) => response.url().endsWith('/api/v1/storage/snapshots') && response.request().method() === 'POST')
    await page.getByRole('button', { name: 'Snapshot pool' }).click()
    const response = await createResponse
    expect((await response.json()).protectedUntil).toBeTruthy()
    await expect(card.getByText(/Protected from deletion until/)).toBeVisible()
    await expect(card.getByRole('button', { name: 'Delete' }).first()).toBeDisabled()
  })

  test('deleting a snapshot removes its record', async ({ page }) => {
    const card = page.locator('div[data-slot="card"]', { has: page.getByText('Snapshots', { exact: true }) })
    await expect(page.getByText('nightly', { exact: true })).toBeVisible()
    await card.getByRole('button', { name: 'Delete' }).first().click()
    await expect(page.getByText('nightly', { exact: true })).toHaveCount(0, { timeout: 15_000 })
  })
})
