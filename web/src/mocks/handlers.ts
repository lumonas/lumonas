import { HttpResponse, http } from 'msw'
import {
  activity,
  alerts,
  disks,
  dockerSummary,
  findDisk,
  jobs,
  pools,
  protection,
  runtime,
  server,
} from '@/mocks/db'
import type { Job } from '@/api/types'

const BASE = '/api/v1'

const ROLE_ORDER: Record<string, number> = {
  system: 0,
  apps: 1,
  parity: 2,
  data: 3,
  backup: 4,
  external: 5,
  unknown: 6,
}

function serverView() {
  return { ...server, uptimeSeconds: Math.floor((Date.now() - runtime.uptimeStartedAt) / 1000) }
}

export const handlers = [
  http.get(`${BASE}/server`, () => HttpResponse.json(serverView())),

  http.get(`${BASE}/disks`, () =>
    HttpResponse.json([...disks].sort((a, b) => ROLE_ORDER[a.role] - ROLE_ORDER[b.role])),
  ),

  http.get(`${BASE}/disks/:id`, ({ params }) => {
    const disk = findDisk(params.id as string)
    if (!disk) return new HttpResponse(null, { status: 404 })
    return HttpResponse.json(disk)
  }),

  http.get(`${BASE}/pools`, () => HttpResponse.json(pools)),

  http.get(`${BASE}/storage/protection`, () => HttpResponse.json(protection)),

  http.get(`${BASE}/jobs`, () => HttpResponse.json(jobs)),

  http.post(`${BASE}/jobs`, async ({ request }) => {
    const body = (await request.json()) as { type?: string; resourceId?: string }
    const type = body.type ?? 'unknown'
    let title: string
    if (type === 'snapraid.sync') {
      if (jobs.some((j) => j.type === 'snapraid.sync' && j.state === 'running')) {
        return new HttpResponse(null, { status: 409 })
      }
      title = 'SnapRAID sync'
      protection.syncRunning = true
    } else if (type === 'smart.short' || type === 'smart.extended') {
      const disk = body.resourceId ? findDisk(body.resourceId) : undefined
      if (!disk) return new HttpResponse(null, { status: 422 })
      title = `SMART ${type === 'smart.short' ? 'short' : 'extended'} test — ${disk.name}`
    } else {
      return new HttpResponse(null, { status: 422 })
    }
    const job: Job = {
      id: `job-${++runtime.jobCounter}`,
      type,
      title,
      resourceId: body.resourceId,
      state: 'queued',
      progress: null,
      createdAt: new Date().toISOString(),
    }
    jobs.unshift(job)
    return HttpResponse.json(job, { status: 201 })
  }),

  http.get(`${BASE}/alerts`, () => HttpResponse.json(alerts)),

  http.patch(`${BASE}/alerts/:id/ack`, ({ params }) => {
    const alert = alerts.find((a) => a.id === params.id)
    if (!alert) return new HttpResponse(null, { status: 404 })
    alert.state = 'acknowledged'
    return HttpResponse.json(alert)
  }),

  http.get(`${BASE}/activity`, () => HttpResponse.json(activity)),

  http.get(`${BASE}/system/metrics`, () =>
    HttpResponse.json({
      ...runtime.metrics,
      uptimeSeconds: Math.floor((Date.now() - runtime.uptimeStartedAt) / 1000),
    }),
  ),

  http.get(`${BASE}/docker/summary`, () => HttpResponse.json(dockerSummary)),
]
