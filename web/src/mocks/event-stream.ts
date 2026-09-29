import { emit, listeners } from '@/mocks/emitter'
import { disks, findDisk, jobs, protection, pushActivity, runtime } from '@/mocks/db'
import { containers, findStack, logSeedsFor } from '@/mocks/docker'
import { settings } from '@/mocks/settings'
import type { Job, SystemMetrics } from '@/api/types'

let metricsTimer: ReturnType<typeof setInterval> | null = null
let tempTimer: ReturnType<typeof setInterval> | null = null
let jobTimer: ReturnType<typeof setInterval> | null = null
let logTimer: ReturnType<typeof setInterval> | null = null
let dockerMetricsTimer: ReturnType<typeof setInterval> | null = null
const logCounters = new Map<string, number>()

const STAGES: Record<string, string[]> = {
  'snapraid.sync': [
    'Reading previous state',
    'Hashing data disks',
    'Comparing parity',
    'Updating parity',
    'Verifying content files',
  ],
  'smart.short': ['Running self-test', 'Reading SMART log'],
  'smart.extended': ['Running extended self-test', 'Reading SMART log'],
  'docker.deploy': [
    'Resolving variables',
    'Validating Compose',
    'Checking port conflicts',
    'Pulling images',
    'Recreating containers',
    'Health check',
  ],
  'docker.update': [
    'Snapshotting configuration',
    'Backing up appdata',
    'Pulling new image',
    'Recreating containers',
    'Health check',
  ],
  'file.transfer': ['Scanning items', 'Transferring', 'Verifying'],
  'file.upload': ['Uploading', 'Verifying checksum'],
  'backup.app': [
    'Stopping stack',
    'Snapshotting',
    'Backing up appdata',
    'Verifying checksums',
    'Restarting stack',
  ],
  'backup.config': ['Snapshotting configuration', 'Verifying archive'],
  'backup.sync': ['Connecting', 'Syncing files', 'Verifying'],
  'update.check': ['Contacting update channel', 'Verifying signatures'],
  'update.apply': [
    'Pre-flight recovery verification',
    'Verified config backup',
    'Signed download',
    'Applying update',
    'Health check',
  ],
}

function currentMetrics(): SystemMetrics {
  const uptimeSeconds = Math.floor((Date.now() - runtime.uptimeStartedAt) / 1000)
  return { ...runtime.metrics, uptimeSeconds }
}

function drift(value: number, min: number, max: number, step = 1): number {
  const next = value + (Math.random() - 0.5) * 2 * step
  return Math.round(Math.min(max, Math.max(min, next)) * 10) / 10
}

function tickMetrics() {
  const m = runtime.metrics
  m.cpuPercent = Math.round(Math.min(96, Math.max(2, m.cpuPercent + (Math.random() - 0.5) * 9)))
  m.load = [
    Math.round((m.cpuPercent / 24 + Math.random() * 0.2) * 100) / 100,
    m.load[0],
    m.load[1],
  ]
  m.ramUsedBytes = Math.round(Math.min(28e9, Math.max(4e9, m.ramUsedBytes + (Math.random() - 0.5) * 4e8)))
  m.cpuTempC = drift(m.cpuTempC, 41, 58)
  m.net.upMbps = drift(m.net.upMbps, 0.2, 24, 2)
  m.net.downMbps = drift(m.net.downMbps, 0.2, 60, 6)
  emit('system.metrics', 'info', undefined, { ...currentMetrics() })
}

function tickTemperatures() {
  const online = disks.filter((d) => d.temperatureC != null && d.health !== 'offline')
  if (online.length === 0) return
  const disk = online[Math.floor(Math.random() * online.length)]
  const max = disk.interface === 'nvme' ? 52 : 46
  disk.temperatureC = drift(disk.temperatureC ?? 35, 28, max)
  emit('disk.temperature.changed', 'info', { type: 'disk', id: disk.id }, {
    diskId: disk.id,
    temperatureC: disk.temperatureC,
  })
}

function completeJob(job: Job) {
  job.state = 'successful'
  job.progress = 100
  job.finishedAt = new Date().toISOString()
  emit('job.state_changed', 'info', job.resourceId ? { type: 'job', id: job.id } : undefined, {
    ...job,
  })
  if (job.type === 'snapraid.sync') {
    protection.syncRunning = false
    protection.lastSyncAt = job.finishedAt
    protection.lastSyncResult = 'successful'
    protection.changesSinceSyncBytes = 0
    pushActivity({
      category: 'storage',
      title: 'SnapRAID sync completed',
      description: 'Parity updated · changes synced',
    })
    emit('snapraid.sync.completed', 'info', undefined, { jobId: job.id })
	} else if (job.type.startsWith('smart.')) {
		const disk = job.resourceId ? findDisk(job.resourceId) : undefined
		if (disk) {
			disk.smart.lastTest = {
				type: job.type === 'smart.extended' ? 'extended' : 'short',
				result: 'passed',
				at: job.finishedAt,
			}
			pushActivity({
				category: 'storage',
				title: `SMART test completed — ${disk.name}`,
				description: 'Result: passed',
				resource: { type: 'disk', id: disk.id, label: `Disk ${disk.name}` },
			})
		}
	} else if (job.type === 'file.transfer' || job.type === 'file.upload') {
		pushActivity({
			category: 'storage',
			title: job.title,
			description: 'Completed as a background job',
		})
	} else if (job.type.startsWith('backup.')) {
		pushActivity({
			category: 'backup',
			title: `${job.title} — completed`,
			description: 'Archive verified (checksum + decryptability)',
		})
	} else if (job.type === 'update.check') {
		pushActivity({
			category: 'update',
			title: 'Update check completed',
			description: 'All channels verified via signed manifests',
		})
	} else if (job.type === 'update.apply') {
		if (job.resourceId === 'core') {
			settings.updates.core.available = null
			settings.updates.core.current = '0.1.1'
		} else if (job.resourceId === 'debian') {
			settings.updates.debian.pendingCount = 0
		}
		pushActivity({
			category: 'update',
			title: `${job.title} — installed`,
			description: 'Health check passed · rollback point retained',
		})
	} else if (job.type === 'docker.deploy' || job.type === 'docker.update') {
    const stack = job.resourceId ? findStack(job.resourceId) : undefined
    if (stack) {
      stack.state = 'running'
      stack.status = 'healthy'
      stack.lastDeploy = job.finishedAt
      if (job.type === 'docker.update') stack.updateAvailable = undefined
      for (const container of containers) {
        if (container.stackId === stack.id) {
          container.state = 'running'
          container.startedAt = job.finishedAt
        }
      }
      pushActivity({
        category: 'docker',
        title:
          job.type === 'docker.update'
            ? `Stack updated — ${stack.name}`
            : `Stack deployed — ${stack.name}`,
        description: 'All containers healthy',
        resource: { type: 'stack', id: stack.id, label: stack.name },
      })
      emit('docker.stack.deployed', 'info', { type: 'stack', id: stack.id }, {
        stackId: stack.id,
        kind: job.type,
      })
    }
  }
}

function tickJobs() {
  for (const job of jobs) {
    if (job.state === 'queued') {
      job.state = 'running'
      job.startedAt = new Date().toISOString()
      job.progress = 0
      emit('job.state_changed', 'info', { type: 'job', id: job.id }, { ...job })
      continue
    }
    if (job.state !== 'running') continue
    const speed = job.type === 'snapraid.sync' ? 0.9 : 3.2
    const next = (job.progress ?? 0) + Math.random() * speed + 0.2
    if (next >= 100) {
      completeJob(job)
      continue
    }
    job.progress = Math.round(next * 10) / 10
    const stages = STAGES[job.type] ?? ['Working']
    job.stage = stages[Math.min(stages.length - 1, Math.floor((job.progress / 100) * stages.length))]
    emit('job.progress', 'info', { type: 'job', id: job.id }, { ...job })
  }
}

function tickDockerMetrics() {
  for (const container of containers) {
    if (container.state !== 'running') continue
    container.cpuPercent = Math.max(0, Math.round(container.cpuPercent + (Math.random() - 0.5) * 2))
    container.ramUsedBytes = Math.max(
      20e6,
      container.ramUsedBytes + Math.round((Math.random() - 0.5) * 4e7),
    )
  }
  emit('docker.container.metrics', 'info', undefined, {})
}

function tickLogs() {
  const running = containers.filter((c) => c.state === 'running')
  if (running.length === 0) return
  const container = running[Math.floor(Math.random() * running.length)]
  const seeds = logSeedsFor(container.name)
  const counter = logCounters.get(container.name) ?? Math.floor(Math.random() * seeds.length)
  logCounters.set(container.name, counter + 1)
  const message = seeds[counter % seeds.length]
  const line = {
    container: container.name,
    ts: new Date().toISOString(),
    level: (message.startsWith('error') ? 'error' : 'info') as 'info' | 'warn' | 'error',
    message,
  }
  emit('docker.log.line', 'info', { type: 'container', id: container.id }, { ...line })
}

function ensureStarted() {
  if (metricsTimer) return
  metricsTimer = setInterval(tickMetrics, 2000)
  tempTimer = setInterval(tickTemperatures, 4500)
  jobTimer = setInterval(tickJobs, 1500)
  logTimer = setInterval(tickLogs, 1400)
  dockerMetricsTimer = setInterval(tickDockerMetrics, 4000)
  setTimeout(tickMetrics, 300)
}

function maybeStop() {
  if (listeners.size > 0) return
  if (metricsTimer) clearInterval(metricsTimer)
  if (tempTimer) clearInterval(tempTimer)
  if (jobTimer) clearInterval(jobTimer)
  if (logTimer) clearInterval(logTimer)
  if (dockerMetricsTimer) clearInterval(dockerMetricsTimer)
  metricsTimer = null
  tempTimer = null
  jobTimer = null
  logTimer = null
  dockerMetricsTimer = null
}

export class MockEventSource {
  readonly readyState = 1

  constructor(_url: string) {
    ensureStarted()
  }

  addEventListener(type: 'message', listener: (event: { data: string }) => void): void
  addEventListener(type: 'open' | 'error', listener: (event: Event) => void): void
  addEventListener(type: 'message' | 'open' | 'error', listener: ((event: { data: string }) => void) | ((event: Event) => void)) {
    if (type === 'message') listeners.add(listener as (event: { data: string }) => void)
    else if (type === 'open') queueMicrotask(() => (listener as (event: Event) => void)(new Event('open')))
  }

  removeEventListener(type: 'message', listener: (event: { data: string }) => void): void
  removeEventListener(type: 'open' | 'error', listener: (event: Event) => void): void
  removeEventListener(type: 'message' | 'open' | 'error', listener: ((event: { data: string }) => void) | ((event: Event) => void)) {
    if (type === 'message') {
      listeners.delete(listener as (event: { data: string }) => void)
      maybeStop()
    }
  }

  close() {
    maybeStop()
  }
}
