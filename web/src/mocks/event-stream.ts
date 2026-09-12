import { disks, findDisk, jobs, protection, pushActivity, runtime } from '@/mocks/db'
import type { Job, LumoEvent, SystemMetrics } from '@/api/types'

type Listener = (event: { data: string }) => void

const listeners = new Set<Listener>()
let metricsTimer: ReturnType<typeof setInterval> | null = null
let tempTimer: ReturnType<typeof setInterval> | null = null
let jobTimer: ReturnType<typeof setInterval> | null = null

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
}

function emit(
  type: string,
  severity: LumoEvent['severity'],
  resource: LumoEvent['resource'],
  data: Record<string, unknown>,
) {
  const envelope: LumoEvent = {
    id: `evt-${++runtime.eventCounter}`,
    type,
    timestamp: new Date().toISOString(),
    severity,
    resource,
    data,
  }
  const payload = JSON.stringify(envelope)
  for (const listener of listeners) {
    listener({ data: payload })
  }
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
    }
    pushActivity({
      category: 'storage',
      title: `SMART test completed — ${disk?.name ?? 'disk'}`,
      description: 'Result: passed',
      resource: disk ? { type: 'disk', id: disk.id, label: `Disk ${disk.name}` } : undefined,
    })
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

function ensureStarted() {
  if (metricsTimer) return
  metricsTimer = setInterval(tickMetrics, 2000)
  tempTimer = setInterval(tickTemperatures, 4500)
  jobTimer = setInterval(tickJobs, 1500)
  setTimeout(tickMetrics, 300)
}

function maybeStop() {
  if (listeners.size > 0) return
  if (metricsTimer) clearInterval(metricsTimer)
  if (tempTimer) clearInterval(tempTimer)
  if (jobTimer) clearInterval(jobTimer)
  metricsTimer = null
  tempTimer = null
  jobTimer = null
}

export class MockEventSource {
  constructor(_url: string) {
    ensureStarted()
  }

  addEventListener(_type: 'message', listener: Listener) {
    listeners.add(listener)
  }

  removeEventListener(_type: 'message', listener: Listener) {
    listeners.delete(listener)
    maybeStop()
  }

  close() {
    maybeStop()
  }
}
