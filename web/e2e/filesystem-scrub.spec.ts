import { expect, test } from '@playwright/test'

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('lumonas-onboarded', '1'))
})

test('schedules and starts a managed filesystem integrity scrub', async ({ page }) => {
  await page.goto('/monitoring?tab=jobs')

  await page.getByLabel('New scrub schedule name').fill('Weekly media check')
  await page.getByLabel('New scrub source').fill('/srv/pools/media')
  await page.getByRole('button', { name: 'Add scrub schedule' }).click()
  await expect(page.getByText('Filesystem scrub scheduled — Weekly media check')).toBeVisible()

  page.once('dialog', (dialog) => dialog.accept())
  await page.getByRole('button', { name: 'Run scrub now' }).click()
  await expect(page.getByText('Job queued')).toBeVisible()
  await expect(page.getByRole('table').getByText('Filesystem scrub — /srv/pools/media')).toBeVisible()
})
