import { expect, test } from '@playwright/test'

test('serves the live UI and job events from the real daemon without MSW', async ({ page, request }) => {
  const apiResponse = await request.get('/api/v1/onboarding/state')
  expect(apiResponse.ok()).toBeTruthy()
  const onboarding = await apiResponse.json()
  expect(onboarding).toEqual(expect.objectContaining({ completed: false }))

  await page.goto('/')
  await expect(page.getByRole('heading', { name: /first-time setup/i })).toBeVisible()
  await expect(page.getByText('Welcome to your NAS')).toBeVisible()
  expect(await page.evaluate(() => navigator.serviceWorker.controller)).toBeNull()

  const disks = Array.isArray(onboarding.disks) ? onboarding.disks : []
  const systemDisk = disks.find((disk: { classification?: string }) => disk.classification === 'system') ?? disks[0]
  const roles = Object.fromEntries(
    disks.map((disk: { id: string }) => [disk.id, disk.id === systemDisk?.id ? 'system' : 'unknown']),
  )
  const completeResponse = await request.post('/api/v1/onboarding/complete', {
    data: {
      serverName: 'live-e2e',
      roles,
      protection: { syncTime: '02:00', scrubDay: 'sunday' },
      recovery: { autoConfigBackup: false, destination: '', keyAcknowledged: false },
    },
  })
  expect(completeResponse.ok()).toBeTruthy()

  const event = page.evaluate(() => new Promise<{ type: string; data?: { job?: { id?: string } } }>((resolve, reject) => {
    const source = new EventSource('/api/v1/events/stream')
    const timeout = window.setTimeout(() => {
      source.close()
      reject(new Error('timed out waiting for a live job event'))
    }, 15_000)
    source.onmessage = (message) => {
      const value = JSON.parse(message.data) as { type: string; data?: { job?: { id?: string } } }
      if (value.type === 'job.state_changed') {
        window.clearTimeout(timeout)
        source.close()
        resolve(value)
      }
    }
    source.onerror = () => {
      // EventSource may report transient reconnects; the timeout above is the
      // assertion, while the daemon remains responsible for the stream.
    }
  }))
  const jobResponse = await request.post('/api/v1/jobs', { data: { type: 'snapraid.sync' } })
  expect(jobResponse.status()).toBe(202)
  const liveEvent = await event
  expect(liveEvent.type).toBe('job.state_changed')
  expect(liveEvent.data?.job?.id).toBeTruthy()

  await page.reload()
  await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible()
  await page.goto('/storage')
  await expect(page.getByRole('heading', { name: 'Storage' })).toBeVisible()
  await page.goto('/monitoring')
  await expect(page.getByRole('heading', { name: 'Monitoring' })).toBeVisible()
  expect(await page.evaluate(() => navigator.serviceWorker.controller)).toBeNull()
})
