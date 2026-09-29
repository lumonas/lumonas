import { expect, test } from '@playwright/test'

test('workstation backup instructions explain safe include and exclude selection', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('lumonas-onboarded', '1'))
  await page.goto('/backups?tab=workstations')

  await expect(page.getByText('selection patterns cannot escape the source folder')).toBeVisible()
  await expect(page.getByText(/--include Documents --include Projects/)).toBeVisible()
  await expect(page.getByText(/--exclude '.*node_modules'.*--exclude '\*\.tmp'/)).toBeVisible()
  await expect(page.getByText('Keep a workstation folder in sync')).toBeVisible()
  await expect(page.getByText(/NAS files are never deleted/)).toBeVisible()
  await expect(page.getByText(/--dry-run/)).toBeVisible()
})
