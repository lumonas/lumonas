import { expect, test } from '@playwright/test'

test.describe('backup policy templates', () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => localStorage.setItem('lumonas-onboarded', '1'))
    await page.goto('/backups')
  })

  test('applies a cadence and retention template', async ({ page }) => {
    await page.getByLabel('Choose a schedule and retention baseline').selectOption('weekly')
    await expect(page.getByText(/Run weekly while retaining/)).toBeVisible()
    await page.getByRole('button', { name: 'Apply policy' }).click()
    await expect(page.getByText('Backup policy applied')).toBeVisible()
  })
})
