import { expect, test } from '@playwright/test'

// Share management flows against the MSW mock backend.

test.describe('shares', () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('lumonas-onboarded', '1')
    })
    await page.goto('/shares')
  })

  test('shares page lists seeded shares', async ({ page }) => {
    await expect(page.getByRole('heading', { name: 'Shares' })).toBeVisible()
    // The page carries two "Create share" buttons: the header action, and the
    // backup card's fallback while no compatible backup share exists. Which of
    // them are present depends on whether the shares query has resolved, so an
    // unqualified locator is a race against the query and fails strict mode as
    // soon as it is evaluated during the loading window. The header button is
    // first in DOM order and always rendered, which is the same one the other
    // tests in this file open the wizard with.
    await expect(page.getByRole('button', { name: 'Create share' }).first()).toBeVisible()
  })

  test('create wizard adds a share to the list', async ({ page }) => {
    await page.getByRole('button', { name: 'Create share' }).first().click()
    await page.getByLabel('Name').fill('e2e-share')
    await page.getByLabel('Relative path').fill('/e2e')
    await page.getByRole('button', { name: 'Create share' }).last().click()

    await expect(page.getByText('e2e-share').first()).toBeVisible({ timeout: 15_000 })
  })

  test('new shares can be placed directly on a selected mounted disk', async ({ page }) => {
    await page.getByRole('button', { name: 'Create share' }).first().click()
    await page.getByLabel('Name').fill('usb-archive')
    await page.getByRole('combobox').click()
    await page.getByRole('option', { name: /sdb.*USB backup/ }).click()
    const response = page.waitForResponse((value) => value.url().endsWith('/api/v1/shares') && value.request().method() === 'POST')
    await page.getByRole('button', { name: 'Create share' }).last().click()
    const result = await response
    expect(result.status()).toBe(201)
    expect(result.request().postDataJSON()).toMatchObject({ resourceId: '/srv/disks/wwn_usb-backup', relativePath: '/', name: 'usb-archive' })
    await expect(page.getByText('usb-archive').first()).toBeVisible()
  })

  test('diagnoses effective path access and shows SMB client state', async ({ page }) => {
    await expect(page.getByText('e2e-client')).toBeVisible()
    await expect(page.getByText('report.pdf')).toBeVisible()
    await expect(page.getByRole('button', { name: 'Disconnect' })).toBeVisible()
    const documents = page.getByRole('row', { name: /Documents/ })
    await documents.getByRole('button', { name: /Preview/ }).click()
    await expect(page.getByRole('heading', { name: 'Effective share access' })).toBeVisible()
    await page.getByLabel('User to check').selectOption('p-anna')
    await page.getByLabel('Path inside share').fill('tax/2026.pdf')
    await page.getByRole('button', { name: 'Check access' }).click()
    await expect(page.getByText('Access appears allowed')).toBeVisible()
    await expect(page.getByText(/write share · read filesystem/)).toBeVisible()
  })

  test('disconnects an SMB client only after confirming the impact', async ({ page }) => {
    page.once('dialog', (dialog) => dialog.accept())
    await page.getByRole('button', { name: 'Disconnect' }).click()
    await expect(page.getByText('Disconnected SMB client 192.0.2.41')).toBeVisible()
  })

  test('previews and confirms a verified existing-share relocation', async ({ page }) => {
    await page.goto('/shares?share=share-documents')
    await page.getByRole('tab', { name: 'Location' }).click()
    await page.getByLabel('Relocation destination storage').selectOption('/srv/disks/wwn_usb-backup')
    await page.getByLabel('Relocation destination folder').fill('Documents')
    const previewResponse = page.waitForResponse((response) => response.url().endsWith('/api/v1/shares/share-documents/relocation/preview'))
    await page.getByRole('button', { name: 'Preview relocation' }).click()
    expect((await previewResponse).status()).toBe(200)
    await expect(page.getByText('17 files · 80 MB')).toBeVisible()
    await expect(page.getByText(/original files will remain untouched/i)).toBeVisible()
    await page.getByLabel(/I reviewed the destination/).check()
    const moveResponse = page.waitForResponse((response) => response.url().endsWith('/api/v1/shares/share-documents/relocation') && response.request().method() === 'POST')
    await page.getByRole('button', { name: 'Move share', exact: true }).click()
    const result = await moveResponse
    expect(result.status()).toBe(202)
    expect(result.request().postDataJSON()).toMatchObject({ confirmed: true, planHash: 'e2e-relocation-plan-hash' })
    await expect(page.getByRole('status')).toContainText('job-share-relocation-e2e')
  })

  test('schedules a share relocation after reviewing its destination', async ({ page }) => {
    await page.goto('/shares?share=share-documents')
    await page.getByRole('tab', { name: 'Location' }).click()
    await page.getByLabel('Relocation destination storage').selectOption('/srv/disks/wwn_usb-backup')
    await page.getByLabel('Relocation destination folder').fill('Documents')
    await page.getByLabel('Relocation schedule').selectOption('weekly')
    await page.getByLabel('Relocation schedule time').fill('03:15')
    await page.getByLabel('Relocation schedule weekday').selectOption('saturday')
    await page.getByRole('button', { name: 'Preview relocation' }).click()
    await page.getByLabel(/I reviewed the destination/).check()
    const scheduleResponse = page.waitForResponse((response) => response.url().endsWith('/api/v1/shares/share-documents/relocation') && response.request().method() === 'POST')
    await page.getByRole('button', { name: 'Schedule move' }).click()
    const result = await scheduleResponse
    expect(result.status()).toBe(201)
    expect(result.request().postDataJSON()).toMatchObject({ scheduleKind: 'weekly', timeOfDay: '03:15', weekday: 'saturday', confirmed: true })
    await expect(page.getByRole('status')).toContainText('Move scheduled Weekly at 03:15')
  })

  test('enables filtered SMB activity auditing from a share and reviews matching events', async ({ page }) => {
    await page.goto('/shares?share=share-documents')
    await page.getByRole('tab', { name: 'Protocols' }).click()
    await page.getByRole('switch', { name: 'Audit SMB changes' }).click()
    await expect(page.getByText('Operations to record')).toBeVisible()
    const updateAudit = page.waitForResponse((response) => response.url().endsWith('/api/v1/shares/share-documents/protocols/smb') && response.request().method() === 'PATCH')
    await page.getByLabel('Delete files').click()
    expect((await updateAudit).status()).toBe(200)
    await expect(page.getByLabel('Delete files')).not.toBeChecked()

    await page.goto('/monitoring?tab=logs')
    await page.getByLabel('Filter logs by service').selectOption('smb-audit')
    await expect(page.getByText(/anna\|192\.0\.2\.10\|renameat/)).toBeVisible()
    await expect(page.getByText('API ready; background jobs are healthy')).toHaveCount(0)
  })
})
