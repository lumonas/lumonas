import { expect, test } from '@playwright/test'

// Docker flows against the MSW mock backend: catalog browsing, the guided
// install wizard, and the stack list.

test.describe('docker apps', () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('lumonas-onboarded', '1')
    })
    await page.goto('/docker')
  })

  test('catalog renders apps with categories', async ({ page }) => {
    await expect(page.getByRole('heading', { name: 'Docker' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Install again' }).first()).toBeVisible()
    await expect(page.getByRole('button', { name: 'Install' }).first()).toBeVisible()
  })

  test('catalog search filters apps', async ({ page }) => {
    await page.getByLabel('Search apps').fill('jellyfin')
    await expect(page.getByRole('button', { name: /install/i }).first()).toBeVisible()
    await page.getByLabel('Search apps').fill('zzzz-not-an-app')
    await expect(page.getByText('No apps match')).toBeVisible()
  })

  test('install wizard completes and reports success', async ({ page }) => {
    await page.getByRole('button', { name: 'Install' }).first().click()
    await expect(page.getByRole('heading', { name: /Install .+/ }).nth(1)).toBeVisible()

    await page.getByRole('button', { name: 'Continue' }).click()
    await page.getByRole('button', { name: 'Install app' }).click()
    await expect(page.getByText(/Installing .+/)).toBeVisible({ timeout: 15_000 })
    await page.getByRole('button', { name: 'Done' }).click()
  })

  test('stacks tab lists the seeded stack', async ({ page }) => {
    await page.getByRole('tab', { name: 'Stacks' }).click()
    await expect(page.getByText('jellyfin').first()).toBeVisible()
  })
})
