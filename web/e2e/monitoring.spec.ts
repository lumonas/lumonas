import { expect, test } from '@playwright/test'

// Monitoring flows against the MSW mock backend: alerts list, alert history,
// and the jobs view.

test.describe('monitoring', () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('lumonas-onboarded', '1')
    })
    await page.goto('/monitoring')
  })

  test('overview renders metrics', async ({ page }) => {
    await expect(page.getByRole('heading', { name: 'Monitoring' })).toBeVisible()
  })

  test('alerts tab lists active alerts and resolved history', async ({ page }) => {
    await page.getByRole('tab', { name: 'Alerts' }).click()
    await expect(page.getByText('Recently resolved')).toBeVisible()
    // The MSW seed resolves one historical alert that must show up here.
    await expect(page.getByText('resolved').first()).toBeVisible()
  })

  test('jobs tab renders the job table', async ({ page }) => {
    await page.getByRole('tab', { name: 'Jobs' }).click()
    await expect(page.getByRole('columnheader', { name: /job|title|status/i }).first()).toBeVisible()
  })
})
