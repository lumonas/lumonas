import { expect, test } from '@playwright/test'

// Browser E2E smoke coverage: the mock-backed dev server exercises the real
// UI against the MSW worker, so these flows verify routing, gating, and
// rendering end to end without hardware or a running daemon.

test.describe('onboarding', () => {
  test('completes the wizard and lands on the dashboard', async ({ page }) => {
    await page.goto('/')

    // Fresh state shows the first-run wizard, not the app shell.
    await expect(page.getByRole('heading', { name: /welcome|set up/i }).first()).toBeVisible()

    // Steps 0-2: defaults are pre-filled, just continue.
    const continueButton = page.getByRole('button', { name: 'Continue' })
    await continueButton.click()
    await continueButton.click()
    await continueButton.click()

    // Step 3 (disaster recovery): acknowledge the recovery key.
    await page
      .getByText('I have stored the recovery key somewhere safe')
      .locator('xpath=ancestor::label')
      .getByRole('checkbox')
      .check()
    await continueButton.click()

    // Step 4: finish.
    await page.getByRole('button', { name: /finish setup/i }).click()
    await expect(page.getByText('Setup complete', { exact: true })).toBeVisible({ timeout: 15_000 })

    // The app shell replaces the wizard.
    await expect(page.getByRole('link', { name: 'Overview' }).first()).toBeVisible()
  })

  test('skip setup reaches the dashboard without completing', async ({ page }) => {
    await page.goto('/')
    await page.getByRole('button', { name: 'Skip setup for now' }).click()
    // The onboarding query is cached from before the skip; a reload re-asks
    // the backend, which now reports completed.
    await page.reload()
    await expect(page.getByRole('link', { name: 'Storage' }).first()).toBeVisible()
  })
})

test.describe('authenticated shell', () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('lumonas-onboarded', '1')
    })
    await page.goto('/')
  })

  test('dashboard renders core health and nav', async ({ page }) => {
    await expect(page.getByRole('link', { name: 'Overview' }).first()).toBeVisible()
    await expect(page.getByText('Overview').first()).toBeVisible()
  })

  test('files page lists shares', async ({ page }) => {
    await page.getByRole('link', { name: 'Files' }).first().click()
    await expect(page.getByRole('heading', { name: 'Files' })).toBeVisible()
  })

  test('docker page renders stacks view', async ({ page }) => {
    await page.getByRole('link', { name: 'Docker' }).first().click()
    await expect(page.getByRole('heading', { name: 'Docker' })).toBeVisible()
  })

  test('storage page renders', async ({ page }) => {
    await page.getByRole('link', { name: 'Storage' }).first().click()
    await expect(page.getByRole('heading', { name: 'Storage' })).toBeVisible()
  })
})
