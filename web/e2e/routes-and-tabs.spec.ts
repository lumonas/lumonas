import { expect, test } from '@playwright/test'

const pages = [
  { path: '/', title: 'Overview' },
  { path: '/install', title: 'Install LumoNAS' },
  { path: '/storage', title: 'Storage' },
  { path: '/shares', title: 'Shares' },
  { path: '/docker', title: 'Docker' },
  { path: '/files', title: 'Files' },
  { path: '/backups', title: 'Backups' },
  { path: '/network', title: 'Network' },
  { path: '/dependencies', title: 'Relationships' },
  { path: '/monitoring', title: 'Monitoring' },
  { path: '/updates', title: 'System updates' },
  { path: '/users', title: 'Users' },
  { path: '/settings', title: 'Settings' },
] as const

const tabPages = [
  { path: '/storage', title: 'Storage', tabs: [['overview', 'Overview'], ['disks', 'Disks'], ['pools', 'Pools'], ['protection', 'Protection'], ['mounts', 'Mounts'], ['topology', 'Topology'], ['activity', 'Activity'], ['quotas', 'Quotas']] },
  { path: '/docker', title: 'Docker', tabs: [['apps', 'Apps'], ['stacks', 'Stacks'], ['containers', 'Containers'], ['images', 'Images'], ['volumes', 'Volumes'], ['vms', 'Virtual machines']] },
  { path: '/backups', title: 'Backups', tabs: [['overview', 'Overview'], ['jobs', 'Jobs'], ['history', 'History'], ['destinations', 'Destinations'], ['recovery', 'Recovery'], ['replication', 'Replication'], ['sync', 'Folder sync'], ['workstations', 'Workstations']] },
  { path: '/network', title: 'Network', tabs: [['connections', 'Connections'], ['lan', 'LAN'], ['vpn', 'VPN'], ['diagnostics', 'Diagnostics']] },
  { path: '/monitoring', title: 'Monitoring', tabs: [['overview', 'Overview'], ['alerts', 'Alerts'], ['jobs', 'Jobs'], ['notifications', 'Notifications'], ['activity', 'Activity'], ['audit', 'Audit'], ['troubleshooting', 'Troubleshooting'], ['logs', 'Logs']] },
  { path: '/settings', title: 'Settings', tabs: [['updates', 'Updates'], ['runtime', 'Runtime'], ['power', 'Power & UPS'], ['security', 'Security']] },
] as const

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('lumonas-onboarded', '1'))
})

test.describe('direct route coverage', () => {
  for (const item of pages) {
    test(`${item.path} renders ${item.title}`, async ({ page }) => {
      await page.goto(item.path)
      await expect(page.getByRole('heading', { name: item.title, exact: true }).first()).toBeVisible()
    })
  }
})

test.describe('tab deep links', () => {
  for (const pageConfig of tabPages) {
    for (const [tab, label] of pageConfig.tabs) {
      test(`${pageConfig.path}?tab=${tab} opens the ${label} tab`, async ({ page }) => {
        await page.goto(`${pageConfig.path}?tab=${tab}`)
        await expect(page.getByRole('heading', { name: pageConfig.title, exact: true }).first()).toBeVisible()
        await expect(page.getByRole('tab', { name: label, exact: true })).toHaveAttribute('aria-selected', 'true')
      })
    }
  }
})

test('system logs can be searched and filtered by service', async ({ page }) => {
  await page.goto('/monitoring?tab=logs')
  await expect(page.getByText('API ready; background jobs are healthy')).toBeVisible()
  await page.getByLabel('Filter logs by service').selectOption('smbd.service')
  await expect(page.getByText('Client reconnect completed')).toBeVisible()
  await expect(page.getByText('API ready; background jobs are healthy')).toHaveCount(0)
})

test('security settings report certificate state without exposing unsupported TLS toggles', async ({ page }) => {
  await page.goto('/settings?tab=security')
  await expect(page.getByText('TLS certificate', { exact: true })).toBeVisible()
  await expect(page.getByText('Web listener certificate')).toBeVisible()
  await expect(page.getByText('Automatic certificate renewal', { exact: true })).toBeVisible()
  await expect(page.getByText('Not configured', { exact: true })).toBeVisible()
  await expect(page.getByRole('switch', { name: 'Toggle HTTPS' })).toHaveCount(0)
  await expect(page.getByRole('switch', { name: 'Toggle ACME' })).toHaveCount(0)
})

test('security settings queue a confirmed certificate rotation', async ({ page }) => {
  await page.goto('/settings?tab=security')
  await page.getByLabel('TLS certificate PEM file').setInputFiles({ name: 'server.crt', mimeType: 'application/x-pem-file', buffer: Buffer.from('-----BEGIN CERTIFICATE-----\nmock\n-----END CERTIFICATE-----') })
  await page.getByLabel('TLS private key PEM file').setInputFiles({ name: 'server.key', mimeType: 'application/x-pem-file', buffer: Buffer.from('-----BEGIN PRIVATE KEY-----\nmock\n-----END PRIVATE KEY-----') })
  const response = page.waitForResponse((value) => value.url().endsWith('/api/v1/security/certificate/import') && value.request().method() === 'POST')
  await page.getByLabel('Confirm certificate replacement').check()
  await page.getByRole('button', { name: 'Validate and replace certificate' }).click()
  expect((await response).status()).toBe(202)
  await expect(page.getByText('Certificate installation queued as job-tls-install')).toBeVisible()
})

test('security settings require consent before configuring automatic renewal', async ({ page }) => {
  await page.goto('/settings?tab=security')
  await page.getByLabel('ACME DNS name').fill('nas.example.com')
  await page.getByLabel('ACME contact email').fill('admin@example.com')
  await expect(page.getByRole('button', { name: 'Configure automatic certificate renewal' })).toBeDisabled()
  const response = page.waitForResponse((value) => value.url().endsWith('/api/v1/security/certificate/acme') && value.request().method() === 'POST')
  await page.getByLabel('Agree to ACME terms').check()
  await page.getByLabel('Allow ACME TCP port 80').check()
  await page.getByRole('button', { name: 'Configure automatic certificate renewal' }).click()
  expect((await response).status()).toBe(202)
  await expect(page.getByText("Let's Encrypt setup queued as job-tls-acme")).toBeVisible()
})

test.describe('network API states', () => {
  test.use({ serviceWorkers: 'block' })
  test.beforeEach(async ({ page }) => {
    // These tests replace selected API responses directly, so keep unrelated
    // requests inside the browser fixture instead of letting Vite proxy them
    // to a daemon that is intentionally not running.
    await page.route('**/api/v1/**', (route) => route.abort())
    await page.route('**/api/v1/auth/status', (route) =>
      route.fulfill({ contentType: 'application/json', body: JSON.stringify({ required: false, configured: false, authenticated: false }) }),
    )
    await page.route('**/api/v1/onboarding/state', (route) =>
      route.fulfill({ contentType: 'application/json', body: JSON.stringify({ completed: true }) }),
    )
  })

  test('connections show loading and empty states without leaking requests', async ({ page }) => {
    let releaseResponse!: () => void
    const responseGate = new Promise<void>((resolve) => { releaseResponse = resolve })
    await page.route('**/api/v1/network/connections', async (route) => {
      await responseGate
      await route.fulfill({ contentType: 'application/json', body: '[]' })
    })

    await page.goto('/network?tab=connections')
    await expect(page.getByText('Loading connections…')).toBeVisible()
    releaseResponse()
    await expect(page.getByText(/No connections yet/)).toBeVisible()
  })

  test('connections surface API errors with a recoverable message', async ({ page }) => {
    await page.route('**/api/v1/network/connections', (route) =>
      route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: 'network service unavailable' }) }),
    )

    await page.goto('/network?tab=connections')
    await expect(page.getByText('Unable to load network connections')).toBeVisible()
  })
})
