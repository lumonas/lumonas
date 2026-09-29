import { expect, test } from '@playwright/test'

test('creates a scoped file request link and uploads through the public route', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('lumonas-onboarded', '1'))
  await page.goto('/files?share=share-photos&path=/')
  await page.getByRole('button', { name: 'Request files' }).click()
  await expect(page.getByRole('dialog')).toContainText('Destination:')
  await page.getByRole('button', { name: 'Create link' }).click()
  await expect(page.getByLabel('New upload link')).toBeVisible()
  const link = await page.getByLabel('New upload link').inputValue()
  expect(link).toContain('/request/')
  await expect(page.getByRole('dialog').locator('svg').first()).toBeVisible()

  await page.goto('/request/mock-public-token')
  await expect(page.getByText('Send files to LumoNAS', { exact: true })).toBeVisible()
  await page.getByLabel('Choose file to upload').setInputFiles({ name: 'phone-photo.jpg', mimeType: 'image/jpeg', buffer: Buffer.from('photo') })
  await page.getByRole('button', { name: 'Upload file' }).click()
  await expect(page.getByRole('status')).toContainText('uploaded successfully')
})

test('public file request uploads a selected batch one file at a time', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('lumonas-onboarded', '1'))
  await page.goto('/request/mock-public-token')
  await page.getByLabel('Choose file to upload').setInputFiles([
    { name: 'phone-one.jpg', mimeType: 'image/jpeg', buffer: Buffer.from('one') },
    { name: 'phone-two.jpg', mimeType: 'image/jpeg', buffer: Buffer.from('two') },
  ])
  await expect(page.getByText(/2 selected/)).toBeVisible()
  await page.getByRole('button', { name: 'Upload 2 files' }).click()
  await expect(page.getByRole('status')).toContainText('2 files uploaded successfully')
  await expect(page.getByRole('status')).toContainText('phone-one.jpg')
  await expect(page.getByRole('status')).toContainText('phone-two.jpg')
})

test('creates a password protected read-only link scoped to a folder', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('lumonas-onboarded', '1'))
  await page.goto('/files?share=share-photos&path=%2F2025')
  await page.getByRole('button', { name: 'Share folder' }).click()
  await page.getByLabel('Read-only link password').fill('correct horse battery staple')
  await page.getByRole('button', { name: 'Create read-only link' }).click()
  await expect(page.getByLabel('New read-only link')).toBeVisible()
  const link = await page.getByLabel('New read-only link').inputValue()
  const token = new URL(link).pathname.split('/').pop()
  expect(link).toContain('/share/')
  await page.goto(`/share/${token}`)
  await page.getByLabel('Share link password').fill('correct horse battery staple')
  await page.getByRole('button', { name: 'Open shared folder' }).click()
  await expect(page.getByText('Shared folder: Photos', { exact: true })).toBeVisible()
  await expect(page.getByText('IMG_5501.jpg')).toBeVisible()
  await expect(page.getByText('IMG_5502.jpg')).toBeVisible()
})

test('builds a per-share text index before searching file contents', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('lumonas-onboarded', '1'))
  await page.goto('/files?share=share-documents')
  await page.getByRole('button', { name: 'Search', exact: true }).click()
  await page.getByLabel('Search inside indexed text files (this share only)').check()
  await page.getByPlaceholder('vacation-2024.jpg').fill('launch')
  await page.getByRole('button', { name: 'Build index' }).click()
  await expect(page.getByText('meeting-notes.txt', { exact: true })).toBeVisible()
  await expect(page.getByText('Project launch checklist: confirm ...')).toBeVisible()
})
