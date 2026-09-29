import { expect, test } from '@playwright/test'

test('creates, runs, and reviews a scheduled snapshot replication task', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('lumonas-onboarded', '1'))
  await page.goto('/backups?tab=replication')

  await page.getByLabel('Task name').fill('Documents offsite')
  await page.getByLabel('Remote NAS').selectOption({ label: "Parents' NAS · online" })
  await page.getByLabel('Local Btrfs share').selectOption('share-documents')
  await page.getByLabel('Remote destination share ID').fill('share-offsite-documents')
  await page.getByLabel('Destination snapshot receive token').fill('lumo_receive_scoped_token')
  await page.getByLabel('Schedule').selectOption('weekly')
  await page.getByLabel('Run time (NAS local time)').fill('03:30')
  await page.getByLabel('Weekday').selectOption('saturday')
  await page.getByRole('button', { name: 'Add snapshot task' }).click()

  const task = page.getByText('Documents offsite').locator('..')
  await expect(task).toContainText('weekly')
  await expect(page.getByText(/share-offsite-documents/)).toBeVisible()
  await page.getByRole('button', { name: 'Run now' }).click()
  await expect(page.getByText(/last success/)).toBeVisible()
  await expect(page.getByText(/replica-/)).toBeVisible()
  await page.getByRole('button', { name: 'Edit' }).click()
  await page.getByLabel('Task name').fill('Documents offsite updated')
  await page.getByLabel('Schedule').selectOption('manual')
  await page.getByRole('button', { name: 'Save changes' }).click()
  await expect(page.getByText('Documents offsite updated')).toBeVisible()
  await expect(page.getByText('manual', { exact: true })).toBeVisible()
})
