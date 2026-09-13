import { expect, test } from '@playwright/test'

// LAN discovery and wake-known-hosts flows against the MSW mock backend.

test.describe('network LAN', () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('lumonas-onboarded', '1')
    })
    await page.goto('/network?tab=lan')
  })

  test('lists discovered devices with addresses', async ({ page }) => {
    await expect(page.getByText('LAN devices')).toBeVisible()
    await expect(page.getByText('aa:bb:cc:dd:ee:10')).toBeVisible()
    await expect(page.getByText('living-room-tv')).toBeVisible()
  })

  test('scan refreshes the inventory', async ({ page }) => {
    await page.getByRole('button', { name: 'Scan now' }).click()
    await expect(page.getByRole('button', { name: /Scanning…|Scan now/ }).first()).toBeVisible()
    await expect(page.getByText('aa:bb:cc:dd:ee:11')).toBeVisible()
  })

  test('wake sends a packet for a known host', async ({ page }) => {
    await page.getByRole('button', { name: 'Wake' }).first().click()
    await expect(page.getByRole('button', { name: 'Sent' })).toBeVisible({ timeout: 15_000 })
  })
})
