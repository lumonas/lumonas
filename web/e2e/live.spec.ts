import { expect, test } from '@playwright/test'
import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import path from 'node:path'

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

test('live daemon syncs files between temporary managed shares', async ({ request }) => {
  const root = await mkdtemp(path.join(tmpdir(), 'lumonas-sync-live-'))
  const sourcePath = path.join(root, 'source')
  const destinationPath = path.join(root, 'destination')
  await mkdir(sourcePath)
  await mkdir(destinationPath)
  await writeFile(path.join(sourcePath, 'sample.txt'), 'live sync payload')
  const createdShareIds: string[] = []
  let taskId = ''
  try {
    async function createShare(name: string, sharePath: string) {
      const response = await request.post('/api/v1/shares', { data: {
        name,
        path: sharePath,
        enabled: true,
        protocols: [{ protocol: 'rsync', enabled: true, readOnly: true }],
        access: [],
      } })
      expect(response.ok(), `share creation failed (${response.status()}): ${await response.text()}`).toBeTruthy()
      const value = await response.json() as { id: string }
      createdShareIds.push(value.id)
      return value.id
    }
    const sourceId = await createShare('Sync source', sourcePath)
    const destinationId = await createShare('Sync destination', destinationPath)
    const taskResponse = await request.post('/api/v1/folder-sync/tasks', { data: {
      name: 'Live local sync',
      source: { kind: 'share', shareId: sourceId },
      destination: { kind: 'share', shareId: destinationId },
      mode: 'copy',
      deepCheck: true,
      mirrorApproved: false,
      scheduleKind: 'manual',
      enabled: false,
    } })
    expect(taskResponse.ok()).toBeTruthy()
    taskId = (await taskResponse.json() as { id: string }).id
    const previewResponse = await request.post(`/api/v1/folder-sync/tasks/${taskId}/preview`)
    expect(previewResponse.ok()).toBeTruthy()
    const preview = await previewResponse.json() as { plan: { files: number; changes: { path: string }[] } }
    expect(preview.plan.files).toBe(1)
    expect(preview.plan.changes.map((change) => change.path)).toContain('sample.txt')
    const started = await request.post(`/api/v1/folder-sync/tasks/${taskId}/run`, { data: {} })
    expect(started.status()).toBe(202)
    let state = 'running'
    for (let attempt = 0; attempt < 50 && state === 'running'; attempt++) {
      await new Promise((resolve) => setTimeout(resolve, 100))
      const runs = await request.get(`/api/v1/folder-sync/tasks/${taskId}/runs`)
      expect(runs.ok()).toBeTruthy()
      const history = await runs.json() as { state: string }[]
      state = history[0]?.state ?? 'missing'
    }
    expect(state).toBe('successful')
    expect(await readFile(path.join(destinationPath, 'sample.txt'), 'utf8')).toBe('live sync payload')
  } finally {
    if (taskId) await request.delete(`/api/v1/folder-sync/tasks/${taskId}`)
    for (const id of createdShareIds.reverse()) await request.delete(`/api/v1/shares/${id}`)
    await rm(root, { recursive: true, force: true })
  }
})
