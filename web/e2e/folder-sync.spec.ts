import { expect, test } from '@playwright/test'

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('lumonas-onboarded', '1'))
})

test('creates a copy task and previews planned changes', async ({ page }) => {
  await page.goto('/backups?tab=sync')
  await page.getByLabel('Sync task name').fill('Photos to archive')
  await page.getByLabel('Sync source share').selectOption({ label: 'Media' })
  await page.getByLabel('Sync destination share').selectOption({ label: 'Backups' })
  await page.getByRole('button', { name: 'Create sync task' }).click()
  await expect(page.getByText('Photos to archive')).toBeVisible()
  await page.getByRole('button', { name: 'Preview', exact: true }).click()
  await expect(page.getByText(/Preview: 1 file transfers/)).toBeVisible()
  await expect(page.getByText('Photos/album.jpg')).toBeVisible()
})

test('mirror run requires preview before confirmation', async ({ page }) => {
  await page.goto('/backups?tab=sync')
  await page.getByLabel('Sync task name').fill('Mirror media')
  await page.getByLabel('Sync source share').selectOption({ label: 'Media' })
  await page.getByLabel('Sync destination share').selectOption({ label: 'Backups' })
  await page.getByLabel('Sync behavior').selectOption('mirror')
  await page.getByRole('button', { name: 'Create sync task' }).click()
  await expect(page.getByRole('button', { name: 'Preview before run' })).toBeVisible()
  await page.getByRole('button', { name: 'Preview', exact: true }).click()
  await expect(page.getByText(/1 planned deletions/)).toBeVisible()
  page.once('dialog', (dialog) => dialog.accept())
  await page.getByRole('button', { name: 'Run now' }).click()
  await expect(page.getByText('Folder sync started')).toBeVisible()
})

test('two-way merge previews conflicts and requires confirmation', async ({ page }) => {
  await page.goto('/backups?tab=sync')
  await page.getByLabel('Sync task name').fill('Family folders')
  await page.getByLabel('Sync direction').selectOption('two-way')
  await page.getByLabel('Sync source share').selectOption({ label: 'Media' })
  await page.getByLabel('Sync destination share').selectOption({ label: 'Backups' })
  await page.getByRole('button', { name: 'Create sync task' }).click()
  await expect(page.getByText('Family folders')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Preview before run' })).toBeVisible()
  await page.getByRole('button', { name: 'Preview', exact: true }).click()
  await expect(page.getByText(/1 conflicts/)).toBeVisible()
  await expect(page.getByText(/\.lumonas-versions\/notes\.txt/)).toBeVisible()
  page.once('dialog', (dialog) => dialog.accept())
  await page.getByRole('button', { name: 'Run now' }).click()
  await expect(page.getByText('Folder sync started')).toBeVisible()
})

test('creates a pull task from S3 into a managed share', async ({ page }) => {
  await page.goto('/backups?tab=sync')
  await page.getByLabel('Sync task name').fill('Cloud archive restore')
  await page.getByLabel('Sync source share').selectOption({ label: 'Backblaze B2 · S3' })
  await page.getByLabel('Sync destination share').selectOption({ label: 'Media' })
  await page.getByLabel('Sync remote prefix').fill('family/photos')
  await page.getByRole('button', { name: 'Create sync task' }).click()
  await expect(page.getByText('Cloud archive restore')).toBeVisible()
})

test('imports from a remote NAS after reviewing a verified copy-only plan', async ({ page }) => {
  await page.goto('/backups?tab=sync')
  await page.getByLabel('NAS import name').fill('Family archive import')
  await page.getByLabel('NAS import source').selectOption({ label: 'Backblaze B2 · S3' })
  await page.getByLabel('NAS import remote prefix').fill('family/photos')
  await page.getByLabel('NAS import destination', { exact: true }).selectOption('share:share-media')
  await page.getByLabel('NAS import destination path').fill('Imported/Photos')
  const createResponse = page.waitForResponse((response) => response.url().endsWith('/api/v1/folder-sync/tasks') && response.request().method() === 'POST')
  await page.getByRole('button', { name: 'Scan and preview import' }).click()
  const created = await createResponse
  expect(created.status()).toBe(200)
  expect(created.request().postDataJSON()).toMatchObject({
    name: 'Family archive import', mode: 'copy', deepCheck: true,
    source: { kind: 'destination', destinationId: 'dest-s3', prefix: 'family/photos' },
    destination: { kind: 'share', shareId: 'share-media', path: 'Imported/Photos' },
  })
  await expect(page.getByText('2 file copies')).toBeVisible()
  await expect(page.getByText(/0 deletions/)).toBeVisible()
  await page.getByLabel(/I reviewed the source/).check()
  const runResponse = page.waitForResponse((response) => response.url().includes('/api/v1/folder-sync/tasks/') && response.url().endsWith('/run'))
  await page.getByRole('button', { name: 'Import files' }).click()
  expect((await runResponse).status()).toBe(202)
  await expect(page.getByText('NAS import started')).toBeVisible()
})

test('offers an opt-in folder backup when a USB destination is connected', async ({ page }) => {
  await page.goto('/backups?tab=sync')
  await page.getByLabel('Sync task name').fill('USB photo rotation')
  await page.getByLabel('Sync source share').selectOption({ label: 'Photos' })
  await page.getByLabel('Sync destination share').selectOption({ label: '/srv/disks/disk-usb' })
  await expect(page.getByLabel('Run when this USB disk is connected')).toBeVisible()
  await page.getByLabel('Run when this USB disk is connected').check()
  await page.getByRole('button', { name: 'Create sync task' }).click()
  await expect(page.getByText(/runs on USB connect/)).toBeVisible()
})
