import { expect, test } from '@playwright/test'

test.use({ serviceWorkers: 'block' })

test('reviews an installer target and confirms the immutable plan', async ({ page }) => {
  let stage = 'idle'
  await page.route('**/api/v1/auth/status', (route) =>
    route.fulfill({ contentType: 'application/json', body: JSON.stringify({ required: false, configured: false, authenticated: false }) }),
  )
  await page.route('**/api/v1/onboarding/state', (route) =>
    route.fulfill({ contentType: 'application/json', body: JSON.stringify({ completed: false }) }),
  )
  await page.route('**/api/v1/install/targets', (route) =>
    route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify([
        {
          diskId: 'wwn:system', name: 'sda', model: 'System SSD', sizeBytes: 32 * 1024 ** 3,
          role: 'system', filesystem: 'ext4', mounted: true, eligible: false,
          protectedBy: ['running system disk'], identity: { wwn: 'wwn:system', serial: 'SYS1' },
        },
        {
          diskId: 'wwn:target', name: 'sdb', model: 'Blank SSD', sizeBytes: 16 * 1024 ** 3,
          role: 'unknown', mounted: false, eligible: true,
          identity: { wwn: 'wwn:target', serial: 'DATA1' },
        },
      ]),
    }),
  )
  await page.route('**/api/v1/install/status', (route) =>
    route.fulfill({ contentType: 'application/json', body: JSON.stringify({ stage }) }),
  )
  await page.route('**/api/v1/install/plan', (route) =>
    route.fulfill({
      status: 201,
      contentType: 'application/json',
      body: JSON.stringify({
        hash: 'plan-hash',
        plan: {
          id: 'install-test', targetDiskId: 'wwn:target', expectedIdentity: { wwn: 'wwn:target' },
          hostname: 'lumonas', adminUsername: 'admin', filesystem: 'ext4', uefi: true,
          createdAt: new Date().toISOString(), expiresAt: new Date(Date.now() + 600_000).toISOString(),
        },
        targets: [],
      }),
    }),
  )
  await page.route('**/api/v1/install/apply', (route) => {
    stage = 'succeeded'
    return route.fulfill({ contentType: 'application/json', body: JSON.stringify({ status: 'succeeded' }) })
  })

  await page.goto('/install')
  await expect(page.getByRole('heading', { name: 'Install LumoNAS' })).toBeVisible()
  await expect(page.getByText('running system disk')).toBeVisible()
  await page.getByRole('button', { name: /Blank SSD/ }).click()
  await page.getByLabel('Administrator password').fill('a-very-strong-test-password')
  await page.getByRole('button', { name: 'Generate installation plan' }).click()
  await expect(page.getByText('install-test')).toBeVisible()
  await page.getByRole('button', { name: /Erase target disk and install/ }).click()
  await expect(page.getByText('Installation succeeded. Reboot and remove the installer medium.')).toBeVisible()
})
