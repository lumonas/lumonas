import { HttpResponse, http } from 'msw'
import {
  activity,
  alerts,
  disks,
  findDisk,
  jobs,
  pools,
  protection,
  pushActivity,
  runtime,
  server,
} from '@/mocks/db'
import {
  catalog,
  containers,
  findContainer,
  findStack,
  images,
  seedLogs,
  stacks,
  volumes,
} from '@/mocks/docker'
import { STORAGE_RESOURCES } from '@/api/resources'
import { emit } from '@/mocks/emitter'
import { createJob } from '@/mocks/handlers-helpers'
import {
  backupJobs,
  destinations,
  generations,
  readiness,
  restorePlan,
} from '@/mocks/backup'
import { settings, updateSlots } from '@/mocks/settings'
import { completeOnboarding, onboardingState } from '@/mocks/onboarding'
import {
  deleteNodes,
  insertFile,
  listDir,
  listTrash,
  mkdir,
  performTransfer,
  purgeTrash,
  rename,
  restoreFromTrash,
} from '@/mocks/files'
import {
  applyConnection,
  bindings,
  connections,
  createDiagnosticJob,
  firewall,
  interfaces,
  wifiNetworks,
  type NetworkConnection,
} from '@/mocks/network'
import { alertRules, notificationChannels, scheduledJobs, services } from '@/mocks/system'
import {
  fileUsers,
  findShare,
  groups,
  managementUsers,
  principals,
  shareRuntime,
  shares,
} from '@/mocks/shares'
import type {
  AccessLevel,
  AppSettings,
  CatalogApp,
  DiskRole,
  DockerStack,
  RiskFlag,
  Share,
  StackEnvVar,
} from '@/api/types'

const BASE = '/api/v1'

let mockUPSPolicy = { enabled: false, minimumRuntimeSec: 300, minimumCharge: 10 }

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

function syncStackState(stack: DockerStack) {
  const own = containers.filter((c) => c.stackId === stack.id)
  if (own.length === 0) return
  if (own.every((c) => c.state === 'exited' || c.state === 'created')) {
    stack.state = 'stopped'
    stack.status = 'offline'
  } else if (own.some((c) => c.state === 'unhealthy')) {
    stack.state = 'unhealthy'
    stack.status = 'critical'
  } else {
    stack.state = 'running'
    stack.status = 'healthy'
  }
}

function emitContainer(containerId: string) {
  const container = findContainer(containerId)
  if (container) {
    emit(
      'docker.container.state_changed',
      container.state === 'unhealthy' ? 'critical' : 'info',
      { type: 'container', id: container.id },
      { containerId: container.id, state: container.state },
    )
  }
}

function resourceLabel(id: string): string {
  return STORAGE_RESOURCES.find((r) => r.id === id)?.label ?? id
}

interface InstallPayload {
  catalogId?: string
  name?: string
  composeYaml?: string
  env?: Record<string, string>
  storageMap?: { fieldId: string; containerPath: string; resourceId: string }[]
}

function detectRisks(yaml: string): RiskFlag[] {
  const risks: RiskFlag[] = []
  if (/privileged:\s*true/.test(yaml)) risks.push('privileged')
  if (/\/var\/run\/docker\.sock/.test(yaml)) risks.push('docker_socket')
  if (/\s-\s\/:\//.test(yaml)) risks.push('host_root_bind')
  if (/pid:\s*host/.test(yaml)) risks.push('host_pid')
  if (/network_mode:\s*host/.test(yaml)) risks.push('host_network')
  if (/devices:/.test(yaml)) risks.push('devices')
  return risks
}

function buildStackFromCatalog(
  app: CatalogApp,
  payload: InstallPayload,
): DockerStack {
  const baseName = (payload.name || app.name).toLowerCase().replace(/[^a-z0-9-]+/g, '-')
  let name = baseName
  let suffix = 2
  while (stacks.some((s) => s.name === name)) {
    name = `${baseName}-${suffix++}`
  }
  const env: StackEnvVar[] = app.form
    .filter((f) => f.type !== 'timezone')
    .map((f) => {
      const value = payload.env?.[f.id] ?? f.defaultValue ?? ''
      if (f.type === 'secret') {
        return { name: f.id, value: `secret:${name}-${f.id.toLowerCase()}`, scope: 'secret' as const }
      }
      if (f.type === 'storage_ref') {
        return { name: f.id, value: `${resourceLabel(value)} → /`, scope: 'global' as const }
      }
      return { name: f.id, value, scope: 'stack' as const }
    })
  env.push({ name: 'TZ', value: 'Europe/Warsaw', scope: 'builtin' })

  const storage = [
    { containerPath: '/config', resourceId: 'apps', resourceLabel: `Apps SSD / ${name}` },
    ...app.form
      .filter((f) => f.type === 'storage_ref')
      .map((f) => {
        const resourceId = payload.storageMap?.find((m) => m.fieldId === f.id)?.resourceId ?? f.defaultResource ?? 'share-media'
        return { containerPath: f.containerPath ?? `/${f.id.toLowerCase()}`, resourceId, resourceLabel: resourceLabel(resourceId) }
      }),
  ]

  const portField = app.form.find((f) => f.type === 'port')
  const hostPort = portField ? Number(payload.env?.[portField.id] ?? portField.defaultValue ?? app.ports[0]) : app.ports[0]
  const ports = [{ host: hostPort, container: app.ports[0], label: 'Web UI' }]

  const composeYaml = `services:
  ${name}:
    image: ${app.image}
    container_name: ${name}
    environment:
      - TZ=\${TZ}
${app.form.filter((f) => f.type !== 'timezone').map((f) => `      - ${f.id}=\${${f.id}}`).join('\n')}
    volumes:
      - /srv/lumonas/apps/${name}/config:/config
${app.form.filter((f) => f.type === 'storage_ref').map((f) => `      - \${${f.id}}:${f.containerPath}`).join('\n')}
    ports:
      - ${hostPort}:${app.ports[0]}
    restart: unless-stopped`

  return {
    id: `stack-${++runtime.stackCounter}`,
    name,
    catalogId: app.id,
    category: app.category,
    status: 'attention',
    state: 'deploying',
    images: [app.image],
    composeYaml,
    env,
    storage,
    ports,
    risks: [],
    cpuPercent: 0,
    ramUsedBytes: 0,
    restarts: 0,
    lastDeploy: new Date().toISOString(),
    backup: { strategy: 'stop-backup', appdataSizeBytes: 0 },
    recoveryCoverage: 0,
  }
}

export const handlers = [
  http.get(`${BASE}/auth/status`, () => HttpResponse.json({ required: false, configured: false, authenticated: true })),
  http.post(`${BASE}/auth/login`, () => HttpResponse.json({ username: 'admin', expiresAt: new Date(Date.now() + 3600_000).toISOString() })),
  http.post(`${BASE}/auth/logout`, () => HttpResponse.json({ status: 'logged_out' })),
  http.get(`${BASE}/server`, () => HttpResponse.json(serverView())),
  http.get(`${BASE}/health/components`, () => {
    const diskStatus = disks.some((d) => d.health === 'critical')
      ? 'critical'
      : disks.some((d) => d.health === 'warning')
        ? 'warning'
        : disks.some((d) => d.health === 'attention')
          ? 'attention'
          : 'healthy'
    const components = [
      { id: 'disks', label: 'Disk health', status: diskStatus, message: `${disks.length} disk(s) detected`, recommended: diskStatus === 'critical' ? 'Replace failing disks immediately' : '' },
      { id: 'protection', label: 'SnapRAID protection', status: protection.status, message: protection.lastSyncAt ? `Last sync ${protection.lastSyncAt}` : 'No parity configured', recommended: protection.status === 'attention' ? 'Run a sync to protect recent changes' : '' },
    ]
    return HttpResponse.json({ status: server.health, score: diskStatus === 'healthy' && protection.status === 'healthy' ? 100 : diskStatus === 'critical' || protection.status === 'critical' ? 30 : 70, components })
  }),
  http.get(`${BASE}/power/ups`, () => HttpResponse.json([])),
  http.get(`${BASE}/ups/policy`, () => HttpResponse.json(mockUPSPolicy)),
  http.patch(`${BASE}/ups/policy`, async ({ request }) => {
    mockUPSPolicy = { ...mockUPSPolicy, ...(await request.json() as typeof mockUPSPolicy) }
    return HttpResponse.json(mockUPSPolicy)
  }),

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
    const job = createJob(type, title, body.resourceId)
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

  http.get(`${BASE}/docker/summary`, () =>
    HttpResponse.json({
      stacks: stacks.length,
      appsRunning: containers.filter((c) => c.state === 'running' || c.state === 'restarting')
        .length,
      updatesAvailable: images.filter((i) => i.updateAvailable).length,
    }),
  ),

  http.get(`${BASE}/docker/apps`, () => HttpResponse.json(catalog)),

  http.get(`${BASE}/docker/stacks`, () => HttpResponse.json(stacks)),

  http.get(`${BASE}/docker/stacks/:id`, ({ params }) => {
    const stack = findStack(params.id as string)
    if (!stack) return new HttpResponse(null, { status: 404 })
    return HttpResponse.json(stack)
  }),

  http.post(`${BASE}/docker/stacks`, async ({ request }) => {
    const payload = (await request.json()) as InstallPayload
    if (payload.catalogId) {
      const app = catalog.find((a) => a.id === payload.catalogId)
      if (!app) return new HttpResponse(null, { status: 422 })
      const stack = buildStackFromCatalog(app, payload)
      stacks.unshift(stack)
      containers.push({
        id: `ctr-${stack.id}`,
        name: stack.name,
        stackId: stack.id,
        image: app.image,
        state: 'created',
        cpuPercent: 0,
        ramUsedBytes: 0,
        restarts: 0,
        ports: stack.ports.map((p) => ({ host: p.host, container: p.container })),
      })
      createJob('docker.deploy', `Install ${stack.name}`, stack.id)
      pushActivity({
        category: 'docker',
        title: `App installed — ${stack.name}`,
        description: `From catalog · image ${app.image}`,
        resource: { type: 'stack', id: stack.id, label: stack.name },
      })
      return HttpResponse.json(stack, { status: 201 })
    }
    if (payload.composeYaml && payload.name) {
      const imageMatches = [...payload.composeYaml.matchAll(/image:\s*(\S+)/g)].map((m) => m[1])
      const portMatches = [...payload.composeYaml.matchAll(/-\s*(\d+):(\d+)/g)].map((m) => ({
        host: Number(m[1]),
        container: Number(m[2]),
        label: 'Imported',
      }))
      const stack: DockerStack = {
        id: `stack-${++runtime.stackCounter}`,
        name: payload.name.toLowerCase().replace(/[^a-z0-9-]+/g, '-'),
        category: 'Custom',
        status: 'attention',
        state: 'deploying',
        images: imageMatches,
        composeYaml: payload.composeYaml,
        env: [],
        storage: [],
        ports: portMatches,
        risks: detectRisks(payload.composeYaml),
        cpuPercent: 0,
        ramUsedBytes: 0,
        restarts: 0,
        lastDeploy: new Date().toISOString(),
        backup: { strategy: 'stop-backup', appdataSizeBytes: 0 },
        recoveryCoverage: 0,
      }
      if (stacks.some((s) => s.name === stack.name)) {
        return new HttpResponse(null, { status: 409 })
      }
      stacks.unshift(stack)
      containers.push({
        id: `ctr-${stack.id}`,
        name: stack.name,
        stackId: stack.id,
        image: imageMatches[0] ?? 'unknown',
        state: 'created',
        cpuPercent: 0,
        ramUsedBytes: 0,
        restarts: 0,
        ports: portMatches.map((p) => ({ host: p.host, container: p.container })),
      })
      createJob('docker.deploy', `Deploy ${stack.name}`, stack.id)
      pushActivity({
        category: 'docker',
        title: `Stack imported — ${stack.name}`,
        description: 'Compose pasted by user',
        resource: { type: 'stack', id: stack.id, label: stack.name },
      })
      return HttpResponse.json(stack, { status: 201 })
    }
    return new HttpResponse(null, { status: 422 })
  }),

  http.post(`${BASE}/docker/stacks/:id/deploy`, async ({ params, request }) => {
    const stack = findStack(params.id as string)
    if (!stack) return new HttpResponse(null, { status: 404 })
    const body = (await request.json().catch(() => ({}))) as { composeYaml?: string }
    if (body.composeYaml) {
      stack.composeYaml = body.composeYaml
      stack.risks = detectRisks(body.composeYaml)
    }
    stack.state = 'deploying'
    stack.status = 'attention'
    createJob('docker.deploy', `Deploy ${stack.name}`, stack.id)
    return HttpResponse.json(stack)
  }),

  http.post(`${BASE}/docker/stacks/:id/update`, async ({ params, request }) => {
    const stack = findStack(params.id as string)
    if (!stack) return new HttpResponse(null, { status: 404 })
    const body = (await request.json().catch(() => ({}))) as { backup?: boolean }
    stack.state = 'deploying'
    stack.status = 'attention'
    const job = createJob('docker.update', `Update ${stack.name}`, stack.id)
    pushActivity({
      category: 'docker',
      title: `Stack update started — ${stack.name}`,
      description: body.backup === false ? 'Without appdata backup' : 'With appdata backup',
      resource: { type: 'stack', id: stack.id, label: stack.name },
    })
    return HttpResponse.json(job, { status: 201 })
  }),

  http.post(`${BASE}/docker/stacks/:id/start`, ({ params }) => {
    const stack = findStack(params.id as string)
    if (!stack) return new HttpResponse(null, { status: 404 })
    for (const container of containers) {
      if (container.stackId === stack.id) {
        container.state = 'running'
        container.startedAt = new Date().toISOString()
        emitContainer(container.id)
      }
    }
    syncStackState(stack)
    pushActivity({
      category: 'docker',
      title: `Stack started — ${stack.name}`,
      resource: { type: 'stack', id: stack.id, label: stack.name },
    })
    return HttpResponse.json(stack)
  }),

  http.post(`${BASE}/docker/stacks/:id/stop`, ({ params }) => {
    const stack = findStack(params.id as string)
    if (!stack) return new HttpResponse(null, { status: 404 })
    for (const container of containers) {
      if (container.stackId === stack.id) {
        container.state = 'exited'
        container.startedAt = undefined
        emitContainer(container.id)
      }
    }
    syncStackState(stack)
    pushActivity({
      category: 'docker',
      title: `Stack stopped — ${stack.name}`,
      resource: { type: 'stack', id: stack.id, label: stack.name },
    })
    return HttpResponse.json(stack)
  }),

  http.post(`${BASE}/docker/stacks/:id/restart`, ({ params }) => {
    const stack = findStack(params.id as string)
    if (!stack) return new HttpResponse(null, { status: 404 })
    for (const container of containers) {
      if (container.stackId === stack.id) {
        container.state = 'restarting'
        emitContainer(container.id)
      }
    }
    setTimeout(() => {
      for (const container of containers) {
        if (container.stackId === stack.id) {
          container.state = 'running'
          container.startedAt = new Date().toISOString()
          emitContainer(container.id)
        }
      }
      syncStackState(stack)
    }, 1600)
    return HttpResponse.json(stack)
  }),

  http.get(`${BASE}/docker/containers`, () => HttpResponse.json(containers)),

  http.post(`${BASE}/docker/containers/:id/:action`, ({ params }) => {
    const container = findContainer(params.id as string)
    const action = params.action as 'start' | 'stop' | 'restart'
    if (!container || !['start', 'stop', 'restart'].includes(action)) {
      return new HttpResponse(null, { status: 422 })
    }
    if (action === 'stop') {
      container.state = 'exited'
      container.startedAt = undefined
    } else {
      container.state = action === 'restart' ? 'restarting' : 'running'
      if (action === 'start') container.startedAt = new Date().toISOString()
    }
    emitContainer(container.id)
    if (container.stackId) {
      const stack = findStack(container.stackId)
      if (stack) setTimeout(() => syncStackState(stack), action === 'restart' ? 1600 : 0)
    }
    pushActivity({
      category: 'docker',
      title: `Container ${action === 'restart' ? 'restarted' : action === 'stop' ? 'stopped' : 'started'} — ${container.name}`,
    })
    return HttpResponse.json(container)
  }),

  http.get(`${BASE}/docker/images`, () => HttpResponse.json(images)),

  http.post(`${BASE}/docker/images/:id/update`, ({ params }) => {
    const image = images.find((i) => i.id === params.id)
    if (!image) return new HttpResponse(null, { status: 404 })
    image.updateAvailable = false
    image.createdDaysAgo = 0
    for (const stack of stacks) {
      if (stack.images.some((ref) => ref.startsWith(`${image.repo}:`))) {
        stack.updateAvailable = undefined
      }
    }
    emit('docker.image.updated', 'info', { type: 'image', id: image.id }, { imageId: image.id })
    pushActivity({
      category: 'update',
      title: `Image updated — ${image.repo}:${image.tag}`,
      description: 'Previous image retained for rollback',
    })
    return HttpResponse.json(image)
  }),

  http.get(`${BASE}/docker/volumes`, () => HttpResponse.json(volumes)),

  http.get(`${BASE}/docker/logs/:container`, ({ params }) =>
    HttpResponse.json(seedLogs(params.container as string)),
  ),

  http.post(`${BASE}/auth/login`, async ({ request }) => {
    const body = (await request.json()) as { username?: string; password?: string }
    if (body.username === 'admin' && (body.password?.length ?? 0) >= 4) {
      return HttpResponse.json({ ok: true })
    }
    return HttpResponse.json({ message: 'Invalid credentials' }, { status: 401 })
  }),

  http.post(`${BASE}/auth/logout`, () => HttpResponse.json({ ok: true })),

  http.get(`${BASE}/services`, () => HttpResponse.json(services)),

  http.get(`${BASE}/alert-rules`, () => HttpResponse.json(alertRules)),

  http.patch(`${BASE}/alert-rules/:id`, async ({ params, request }) => {
    const rule = alertRules.find((r) => r.id === params.id)
    if (!rule) return new HttpResponse(null, { status: 404 })
    const body = (await request.json()) as { enabled?: boolean }
    rule.enabled = body.enabled ?? !rule.enabled
    return HttpResponse.json(rule)
  }),

  http.get(`${BASE}/notification-channels`, () => HttpResponse.json(notificationChannels)),

  http.get(`${BASE}/schedules`, () => HttpResponse.json(scheduledJobs)),

  http.patch(`${BASE}/schedules/:id`, async ({ params, request }) => {
    const schedule = scheduledJobs.find((s) => s.id === params.id)
    if (!schedule) return new HttpResponse(null, { status: 404 })
    if (schedule.kind === 'event') return new HttpResponse(null, { status: 422 })
    const body = (await request.json()) as {
      enabled?: boolean
      timeOfDay?: string
      weekday?: string
    }
    if (body.enabled !== undefined) schedule.enabled = body.enabled
    if (body.timeOfDay) {
      schedule.timeOfDay = body.timeOfDay
      schedule.schedule =
        schedule.kind === 'weekly'
          ? `${(schedule.weekday ?? '').charAt(0).toUpperCase()}${(schedule.weekday ?? '').slice(1)}s at ${body.timeOfDay}`
          : `Daily at ${body.timeOfDay}`
    }
    if (body.weekday) {
      schedule.weekday = body.weekday
      schedule.schedule = `${body.weekday.charAt(0).toUpperCase()}${body.weekday.slice(1)}s at ${schedule.timeOfDay}`
    }
    return HttpResponse.json(schedule)
  }),

  http.get(`${BASE}/shares`, () => HttpResponse.json(shares)),

  http.get(`${BASE}/shares/:id`, ({ params }) => {
    const share = findShare(params.id as string)
    if (!share) return new HttpResponse(null, { status: 404 })
    return HttpResponse.json(share)
  }),

  http.post(`${BASE}/shares`, async ({ request }) => {
    const body = (await request.json()) as {
      name?: string
      resourceId?: string
      resourceLabel?: string
      access?: { principalId: string; level: AccessLevel }[]
      protocols?: Share['protocols']
    }
    if (!body.name || !body.resourceId) return new HttpResponse(null, { status: 422 })
    if (shares.some((s) => s.name.toLowerCase() === body.name!.toLowerCase())) {
      return new HttpResponse(null, { status: 409 })
    }
    const share: Share = {
      id: `share-${++shareRuntime.shareCounter}`,
      name: body.name,
      resourceId: body.resourceId,
      resourceLabel: body.resourceLabel ?? 'Main pool',
      relativePath: `/${body.name}`,
      status: 'healthy',
      recycleBin: true,
      protocols: body.protocols ?? [{ protocol: 'smb', enabled: true }],
      access: body.access ?? [{ principalId: 'p-admins', level: 'write' }],
    }
    shares.push(share)
    pushActivity({
      category: 'config',
      title: `Share created — ${share.name}`,
      description: `Located at ${share.resourceLabel}${share.relativePath}`,
      resource: { type: 'share', id: share.id, label: share.name },
    })
    emit('share.created', 'info', { type: 'share', id: share.id }, { shareId: share.id })
    return HttpResponse.json(share, { status: 201 })
  }),

  http.patch(`${BASE}/shares/:id/access`, async ({ params, request }) => {
    const share = findShare(params.id as string)
    if (!share) return new HttpResponse(null, { status: 404 })
    const body = (await request.json()) as { principalId: string; level: AccessLevel }
    const existing = share.access.find((a) => a.principalId === body.principalId)
    if (existing) {
      existing.level = body.level
    } else {
      share.access.push({ principalId: body.principalId, level: body.level })
    }
    return HttpResponse.json(share)
  }),

  http.patch(`${BASE}/shares/:id/protocols/:protocol`, async ({ params, request }) => {
    const share = findShare(params.id as string)
    if (!share) return new HttpResponse(null, { status: 404 })
    const body = (await request.json()) as Partial<Share['protocols'][number]>
    const existing = share.protocols.find((p) => p.protocol === params.protocol)
    if (existing) {
      Object.assign(existing, body)
    } else {
      share.protocols.push({
        protocol: params.protocol as Share['protocols'][number]['protocol'],
        enabled: body.enabled ?? true,
        ...body,
      })
    }
    return HttpResponse.json(share)
  }),

  http.patch(`${BASE}/shares/:id`, async ({ params, request }) => {
    const share = findShare(params.id as string)
    if (!share) return new HttpResponse(null, { status: 404 })
    const body = (await request.json()) as { description?: string; recycleBin?: boolean }
    if (body.description != null) share.description = body.description
    if (body.recycleBin != null) share.recycleBin = body.recycleBin
    return HttpResponse.json(share)
  }),

  http.delete(`${BASE}/shares/:id`, ({ params }) => {
    const index = shares.findIndex((s) => s.id === params.id)
    if (index === -1) return new HttpResponse(null, { status: 404 })
    const [removed] = shares.splice(index, 1)
    pushActivity({
      category: 'config',
      title: `Share removed — ${removed.name}`,
      description: 'Share definition removed · data on disk was not deleted',
    })
    return new HttpResponse(null, { status: 204 })
  }),

  http.get(`${BASE}/principals`, () => HttpResponse.json(principals)),

  http.get(`${BASE}/users`, () =>
    HttpResponse.json({ management: managementUsers, file: fileUsers, groups }),
  ),

  http.post(`${BASE}/users`, async ({ request }) => {
    const body = (await request.json()) as {
      type?: 'management' | 'file' | 'service'
      username?: string
      fullName?: string
      role?: 'owner' | 'operator' | 'readonly'
      group?: string
      password?: string
    }
    if (!body.username || (body.password?.length ?? 0) < 4) {
      return new HttpResponse(null, { status: 422 })
    }
    const allNames = [
      ...managementUsers.map((u) => u.username),
      ...fileUsers.map((u) => u.username),
    ]
    if (allNames.includes(body.username)) return new HttpResponse(null, { status: 409 })
    if (body.type === 'management') {
      managementUsers.push({
        id: `mu-${++shareRuntime.userCounter}`,
        username: body.username,
        fullName: body.fullName,
        role: body.role ?? 'operator',
        twoFactor: false,
        enabled: true,
      })
    } else {
      fileUsers.push({
        id: `fu-${++shareRuntime.userCounter}`,
        username: body.username,
        fullName: body.fullName,
        type: 'user',
        groups: body.group ? [body.group] : [],
        enabled: true,
        uid: 1005 + shareRuntime.userCounter - 100,
      })
    }
    pushActivity({
      category: 'security',
      title: `${body.type === 'management' ? 'Management' : 'File'} user created — ${body.username}`,
      description: 'Password stored encrypted in the LumoNAS secret store',
    })
    return new HttpResponse(null, { status: 201 })
  }),

  http.patch(`${BASE}/users/:id`, async ({ params, request }) => {
    const body = (await request.json()) as { enabled?: boolean }
    const user =
      managementUsers.find((u) => u.id === params.id) ?? fileUsers.find((u) => u.id === params.id)
    if (!user) return new HttpResponse(null, { status: 404 })
    if ('enabled' in user) user.enabled = body.enabled ?? !user.enabled
    return HttpResponse.json(user)
  }),

  http.get(`${BASE}/network/connections`, () => HttpResponse.json(connections)),

  http.get(`${BASE}/network/interfaces`, () => HttpResponse.json(interfaces)),

  http.get(`${BASE}/network/wifi/scan`, () =>
    HttpResponse.json({ available: true, networks: wifiNetworks }),
  ),

  http.post(`${BASE}/network/connections`, async ({ request }) => {
    const body = (await request.json()) as Partial<NetworkConnection>
    if (!body.name || (body.type !== 'wifi' && !body.interface)) {
      return new HttpResponse(null, { status: 422 })
    }
    const connection: NetworkConnection = {
      id: body.id ?? `connection-${++runtime.jobCounter}`,
      name: body.name,
      interface: body.interface ?? '',
      type: body.type ?? 'ethernet',
      enabled: body.enabled ?? true,
      status: 'pending-checkpoint',
      ...(body.type === 'wifi' ? { ssid: body.ssid, wifiOpen: body.wifiOpen } : {}),
      ipv4: body.ipv4 ?? { method: 'auto' },
      ipv6: body.ipv6 ?? { method: 'disabled' },
    }
    connections.push(connection)
    return HttpResponse.json(connection, { status: 201 })
  }),

  http.post(`${BASE}/network/connections/:id/apply`, ({ params }) => {
    const connection = applyConnection(params.id as string)
    if (!connection) return new HttpResponse(null, { status: 404 })
    return HttpResponse.json(connection)
  }),

  http.get(`${BASE}/network/bindings`, () => HttpResponse.json(bindings)),

  http.get(`${BASE}/network/firewall/policy`, () => HttpResponse.json(firewall)),

  http.post(`${BASE}/network/diagnostics`, async ({ request }) => {
    const body = (await request.json()) as { kind?: string; target?: string }
    const kind = body.kind ?? 'ping'
    const job = createDiagnosticJob(
      kind,
      body.target,
    )
    jobs.push({
      id: `job-${++runtime.jobCounter}`,
      type: 'diagnostic',
      title: `Diagnostic: ${kind}${body.target ? ` → ${body.target}` : ''}`,
      state: 'successful',
      progress: 100,
      createdAt: new Date().toISOString(),
      startedAt: new Date().toISOString(),
      finishedAt: new Date().toISOString(),
    })
    return HttpResponse.json({ ...job, id: `job-${runtime.jobCounter}` })
  }),

  http.get(`${BASE}/files`, ({ request }) => {
    const url = new URL(request.url)
    const shareId = url.searchParams.get('share') ?? ''
    const path = url.searchParams.get('path') ?? '/'
    const entries = listDir(shareId, path)
    if (entries == null) return new HttpResponse(null, { status: 404 })
    return HttpResponse.json({ shareId, path, entries })
  }),

  http.get(`${BASE}/files/download`, ({ request }) => {
    const url = new URL(request.url)
    const name = url.searchParams.get('name') ?? 'download'
    return new HttpResponse(`Mock download for ${name}\n`, {
      headers: {
        'Content-Type': 'application/octet-stream',
        'Content-Disposition': `attachment; filename="${name.replaceAll('"', '')}"`,
      },
    })
  }),

  http.post(`${BASE}/files/mkdir`, async ({ request }) => {
    const body = (await request.json()) as { shareId: string; path: string; name: string }
    const ok = mkdir(body.shareId, body.path, body.name)
    if (!ok) return new HttpResponse(null, { status: 409 })
    return HttpResponse.json({ ok: true }, { status: 201 })
  }),

  http.post(`${BASE}/files/rename`, async ({ request }) => {
    const body = (await request.json()) as {
      shareId: string
      path: string
      oldName: string
      newName: string
    }
    const ok = rename(body.shareId, body.path, body.oldName, body.newName)
    if (!ok) return new HttpResponse(null, { status: 409 })
    return HttpResponse.json({ ok: true })
  }),

  http.post(`${BASE}/files/delete`, async ({ request }) => {
    const body = (await request.json()) as { shareId: string; path: string; names: string[] }
    const deleted = deleteNodes(body.shareId, body.path, body.names)
    pushActivity({
      category: 'storage',
      title: `Deleted ${deleted} item${deleted === 1 ? '' : 's'} — moved to recycle bin`,
    })
    return HttpResponse.json({ deleted })
  }),

  http.post(`${BASE}/files/transfer`, async ({ request }) => {
    const body = (await request.json()) as {
      shareId: string
      sourcePath: string
      names: string[]
      targetShareId: string
      targetPath: string
      op: 'copy' | 'move'
      conflict?: 'overwrite' | 'skip' | 'rename'
    }
    const result = performTransfer(body)
    if ('conflicts' in result) {
      return HttpResponse.json({ conflicts: result.conflicts }, { status: 409 })
    }
    const targetShare = shares.find((s) => s.id === body.targetShareId)
    const job = createJob(
      'file.transfer',
      `${body.op === 'copy' ? 'Copy' : 'Move'} ${body.names.length} item${body.names.length === 1 ? '' : 's'} → ${targetShare?.name ?? ''}${body.targetPath}`,
    )
    return HttpResponse.json({ jobId: job.id, transferred: result.transferred }, { status: 202 })
  }),

  http.post(`${BASE}/files/upload`, async ({ request }) => {
    const contentType = request.headers.get('content-type') ?? ''
    const body = contentType.includes('multipart/form-data')
      ? await (async () => {
          const form = await request.formData()
          const uploaded = form.get('file')
          return {
            shareId: String(form.get('shareId') ?? ''),
            path: String(form.get('path') ?? '/'),
            name: uploaded instanceof File ? uploaded.name : String(form.get('name') ?? ''),
            sizeBytes: uploaded instanceof File ? uploaded.size : Number(form.get('sizeBytes') ?? 0),
          }
        })()
      : (await request.json()) as { shareId: string; path: string; name: string; sizeBytes: number }
    const finalName = insertFile(body.shareId, body.path, body.name, body.sizeBytes)
    const job = createJob('file.upload', `Upload ${finalName}`)
    return HttpResponse.json({ jobId: job.id, name: finalName }, { status: 202 })
  }),

  http.get(`${BASE}/files/recycle`, ({ request }) => {
    const url = new URL(request.url)
    return HttpResponse.json(listTrash(url.searchParams.get('share') ?? ''))
  }),

  http.post(`${BASE}/files/recycle/restore`, async ({ request }) => {
    const body = (await request.json()) as { id: string }
    const ok = restoreFromTrash(body.id)
    if (!ok) return new HttpResponse(null, { status: 404 })
    return HttpResponse.json({ ok: true })
  }),

  http.post(`${BASE}/files/recycle/purge`, async ({ request }) => {
    const body = (await request.json()) as { id?: string; shareId?: string }
    const purged = purgeTrash(body.id, body.shareId)
    return HttpResponse.json({ purged })
  }),

  http.get(`${BASE}/backup/readiness`, () => HttpResponse.json(readiness)),

  http.get(`${BASE}/backup/jobs`, () => HttpResponse.json(backupJobs)),

  http.post(`${BASE}/backup/jobs/:id/run`, ({ params }) => {
    const backupJob = backupJobs.find((j) => j.id === params.id)
    if (!backupJob) return new HttpResponse(null, { status: 404 })
    const job = createJob(backupJob.jobType, `Backup — ${backupJob.name}`)
    return HttpResponse.json(job, { status: 202 })
  }),

  http.get(`${BASE}/backup/destinations`, () => HttpResponse.json(destinations)),

  http.get(`${BASE}/backup/generations`, () => HttpResponse.json(generations)),

  http.get(`${BASE}/backup/restore/plan`, () => HttpResponse.json(restorePlan)),

  http.get(`${BASE}/settings`, () => HttpResponse.json(settings)),

  http.patch(`${BASE}/settings`, async ({ request }) => {
    const body = (await request.json()) as {
      section: keyof AppSettings
      patch: Record<string, unknown>
    }
    const section = settings[body.section]
    if (!section) return new HttpResponse(null, { status: 422 })
    Object.assign(section, body.patch)
    return HttpResponse.json(settings)
  }),

  http.post(`${BASE}/updates/check`, () => {
    settings.updates.core.lastCheckedAt = new Date().toISOString()
    settings.updates.debian.lastCheckedAt = new Date().toISOString()
    const job = createJob('update.check', 'Check for updates')
    return HttpResponse.json(job, { status: 202 })
  }),

  http.get(`${BASE}/updates/status`, () => HttpResponse.json(updateSlots)),

  http.post(`${BASE}/updates/apply`, async ({ request }) => {
    const body = (await request.json()) as {
      track?: 'core' | 'debian'
      manifest?: { version?: string }
      signature?: string
      packagePath?: string
    }
    if (body.track) {
      const job = createJob(
        'update.apply',
        body.track === 'core'
          ? 'System update — LumoNAS core'
          : 'Security update — Debian packages',
        body.track,
      )
      return HttpResponse.json(job, { status: 202 })
    }
    if (body.manifest?.version && body.signature && body.packagePath) {
      updateSlots.pendingSlot = 'B'
      updateSlots.pendingVersion = body.manifest.version
      updateSlots.bootAttempts = 0
      updateSlots.updatedAt = new Date().toISOString()
      pushActivity({
        category: 'update',
        title: `Update staged — ${body.manifest.version}`,
        description: 'Signed package verified into inactive slot B',
      })
      return HttpResponse.json(updateSlots, { status: 201 })
    }
    return new HttpResponse(null, { status: 422 })
  }),

  http.post(`${BASE}/updates/health`, async ({ request }) => {
    const body = (await request.json()) as { healthy?: boolean; version?: string }
    if (!body.healthy) return new HttpResponse(null, { status: 422 })
    updateSlots.previousSlot = updateSlots.activeSlot
    updateSlots.activeSlot = updateSlots.pendingSlot ?? updateSlots.activeSlot
    updateSlots.activeVersion = body.version ?? updateSlots.pendingVersion ?? updateSlots.activeVersion
    updateSlots.pendingSlot = undefined
    updateSlots.pendingVersion = undefined
    updateSlots.bootAttempts = 0
    updateSlots.updatedAt = new Date().toISOString()
    pushActivity({
      category: 'update',
      title: `Update confirmed healthy — ${updateSlots.activeVersion}`,
      description: `Now active on slot ${updateSlots.activeSlot}`,
    })
    return HttpResponse.json(updateSlots)
  }),

  http.post(`${BASE}/updates/rollback`, async () => {
    if (!updateSlots.previousSlot) return new HttpResponse(null, { status: 409 })
    const previous = updateSlots.previousSlot
    updateSlots.pendingSlot = undefined
    updateSlots.pendingVersion = undefined
    updateSlots.activeSlot = previous
    updateSlots.lastError = 'Manual rollback performed from administration UI'
    updateSlots.updatedAt = new Date().toISOString()
    pushActivity({
      category: 'update',
      title: 'Rolled back to previous slot',
      description: 'Reason: manual rollback from administration UI',
    })
    return HttpResponse.json(updateSlots)
  }),

  http.get(`${BASE}/onboarding/state`, () => HttpResponse.json(onboardingState())),

  http.post(`${BASE}/recovery/key`, () =>
    HttpResponse.json({ key: 'demo-recovery-key-keep-offline', generated: true }),
  ),

  http.post(`${BASE}/onboarding/complete`, async ({ request }) => {
    const body = (await request.json()) as {
      serverName: string
      roles: Record<string, DiskRole>
      protection: { syncTime: string; scrubDay: string }
      recovery: { autoConfigBackup: boolean; destination: string; keyAcknowledged: boolean }
    }
    return HttpResponse.json(completeOnboarding(body))
  }),
]
