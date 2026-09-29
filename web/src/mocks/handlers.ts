import { HttpResponse, http } from 'msw'
import {
  activity,
  alertHistory,
  alerts,
  disks,
  findDisk,
  jobs,
  lanHosts,
  pools,
  protection,
  pushActivity,
  runtime,
  server,
  storageSnapshots,
} from '@/mocks/db'
import {
  catalog,
  containers,
  dockerDeployments,
  findContainer,
  findStack,
  images,
  imagePacks,
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
  searchFiles,
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
  AuditEntry,
  BackupSchedule,
  CatalogApp,
  DiskRole,
  DockerStack,
  ReplicationPeer,
  SnapshotReplicationTask,
  RestoreDrill,
  RestoreDrillSchedule,
  RiskFlag,
  Share,
  SnapshotReplicationRun,
  StackEnvVar,
  VirtualizationMedia,
  VirtualMachine,
  VirtualMachineDeleteResult,
  RecoverableVirtualMachine,
  VirtualMachineSnapshot,
} from '@/api/types'

const BASE = '/api/v1'

let mockVirtualMachines: VirtualMachine[] = [
  { name: 'home-lab', uuid: '71d1b610-1cd5-4fef-9d6f-8d7fb9a0e8d1', state: 'running', vcpus: 2, memoryKiB: 2097152, maximumMemoryKiB: 4194304 },
]
let mockRecoverableVirtualMachines: RecoverableVirtualMachine[] = []
const mockVirtualizationMedia: VirtualizationMedia[] = []
const mockVirtualMachineSnapshots: Record<string, VirtualMachineSnapshot[]> = {}
let mockBackupSchedule: BackupSchedule = { id: 'default', enabled: true, intervalSeconds: 86400, onUsbAttach: false, updatedAt: new Date().toISOString() }

let mockUPSPolicy = { enabled: false, minimumRuntimeSec: 300, minimumCharge: 10 }
let mockUPSConfig = { names: [] as string[] }
let mockAuditRetentionDays = 365
let mockStorageUnlocked = false
let mockRestoreDrillSchedule: RestoreDrillSchedule = {
  enabled: true,
  intervalSeconds: 604800,
  rpoHours: 24,
  rtoMinutes: 60,
  nextDueAt: new Date(Date.now() + 604800_000).toISOString(),
  updatedAt: new Date().toISOString(),
}
const mockRestoreDrills: RestoreDrill[] = []
const mockWorkloadObjectives: { workloadId: string; rpoHours: number; rtoMinutes: number; updatedAt: string }[] = []
const mockAPITokens: { id: string; name: string; scopes: ('read' | 'backup:write' | 'replication:receive' | 'fleet:status' | `workstation:backup:${string}` | `replication:snapshot:receive:${string}`)[]; createdAt: string; expiresAt?: string }[] = []
const mockFileRequests: { id: string; shareId: string; path: string; createdAt: string; expiresAt: string; maxFiles: number; maxBytes: number; receivedFiles: number; receivedBytes: number; revokedAt?: string }[] = []
const mockFileShareLinks: { id: string; shareId: string; path: string; createdAt: string; expiresAt: string; downloads: number; revokedAt?: string; token: string }[] = []
const mockContentIndex = new Map<string, { documents: number; indexedAt: string | null }>()
const mockIntegrityStatus = new Map<string, { fileCount: number; baselineAt?: string; report?: { baselineAt: string; verifiedAt: string; unchanged: number; changedCount: number; missingCount: number; addedCount: number; changed: string[]; missing: string[]; added: string[] }; running: boolean }>()
const mockReplicationPeers: ReplicationPeer[] = [{
  id: 'peer-parents', name: "Parents' NAS", url: 'https://parents.example.net', createdAt: new Date(Date.now() - 90 * 86_400_000).toISOString(), lastSyncAt: new Date(Date.now() - 3 * 86_400_000).toISOString(), remoteStatus: 'online', remoteVersion: '0.0.9-dev', remoteHealth: 'healthy', remoteCheckedAt: new Date().toISOString(),
}]
const mockSnapshotReplicationTasks: SnapshotReplicationTask[] = []
const mockSnapshotReplicationRuns: SnapshotReplicationRun[] = []
const mockUploadSessions = new Map<string, { id: string; shareId: string; path: string; name: string; sizeBytes: number; receivedBytes: number }>()
const mockFolderSyncTasks: { id: string; name: string; direction?: 'one-way' | 'two-way'; source: { kind: 'share' | 'mount' | 'destination'; shareId?: string; mountPath?: string; destinationId?: string; prefix?: string }; destination: { kind: 'share' | 'mount' | 'destination'; shareId?: string; mountPath?: string; destinationId?: string; prefix?: string }; mode: 'copy' | 'mirror'; deepCheck: boolean; mirrorApproved: boolean; enabled: boolean; scheduleKind: 'manual' | 'daily' | 'weekly'; updatedAt: string }[] = []
const mockQuotas: { policy: { id: string; targetType: 'share' | 'user' | 'group'; targetId: string; limitBytes: number; warningPercent: number }; usedBytes: number; percent: number; state: 'healthy' | 'warning' | 'over'; measuredAt: string }[] = []
const mockAuditEntries: AuditEntry[] = [
  { id: 'audit-demo-1', timestamp: new Date().toISOString(), actor: 'admin', action: 'share.create', outcome: 'committed', resourceType: 'share', resourceId: 'share-media', generation: 1843 },
  { id: 'audit-demo-2', timestamp: new Date(Date.now() - 3_600_000).toISOString(), actor: 'admin', action: 'login.success', outcome: 'recorded', resourceType: 'session', resourceId: 'session-current' },
  { id: 'audit-demo-3', timestamp: new Date(Date.now() - 7_200_000).toISOString(), actor: 'system', action: 'backup.verify', outcome: 'failed', resourceType: 'backup', resourceId: 'dest-s3' },
]
mockAuditEntries.push(...Array.from({ length: 102 }, (_, index): AuditEntry => ({
  id: `audit-e2e-${index + 1}`,
  timestamp: new Date(Date.now() - (index + 3) * 60_000).toISOString(),
  actor: index % 2 ? 'admin' : 'system',
  action: index % 2 ? 'share.update' : 'backup.verify',
  outcome: index % 7 ? 'committed' : 'recorded',
  resourceType: index % 2 ? 'share' : 'backup',
  resourceId: index % 2 ? 'share-media' : 'dest-usb',
})))

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
  http.get(`${BASE}/ups/config`, () => HttpResponse.json(mockUPSConfig)),
  http.patch(`${BASE}/ups/config`, async ({ request }) => {
    mockUPSConfig = { names: ((await request.json() as { names?: string[] }).names ?? []).map((name) => name.trim()).filter(Boolean) }
    return HttpResponse.json(mockUPSConfig)
  }),
  http.get(`${BASE}/ups/policy`, () => HttpResponse.json(mockUPSPolicy)),
  http.patch(`${BASE}/ups/policy`, async ({ request }) => {
    mockUPSPolicy = { ...mockUPSPolicy, ...(await request.json() as typeof mockUPSPolicy) }
    return HttpResponse.json(mockUPSPolicy)
  }),

  http.get(`${BASE}/disks`, () => HttpResponse.json([
    ...disks,
    { id: 'disk-spare-e2e', name: 'sdf', model: 'E2E Replacement Disk', serial: 'E2E-SPARE-001', wwn: '0x5000e2e000000001', sizeBytes: 12_000_000_000_000, usedBytes: 0, role: 'unknown', rotational: true, interface: 'sata', health: 'healthy', temperatureC: 32, mounted: false, lastSeen: new Date().toISOString(), smart: { overall: 'healthy', reallocatedSectors: 0, pendingSectors: 0, uncorrectableSectors: 0, crcErrors: 0, powerOnHours: 0 } },
  ].sort((a, b) => ROLE_ORDER[a.role] - ROLE_ORDER[b.role]))),

  http.get(`${BASE}/disks/:id`, ({ params }) => {
    const disk = findDisk(params.id as string)
    if (!disk) return new HttpResponse(null, { status: 404 })
    return HttpResponse.json(disk)
  }),

  http.get(`${BASE}/storage/disks/:id/smart-history`, ({ params }) => {
    const disk = findDisk(params.id as string)
    if (!disk) return new HttpResponse(null, { status: 404 })
    const first = new Date(Date.now() - 7 * 86_400_000).toISOString()
    return HttpResponse.json({
      diskId: disk.id,
      samples: [
        { diskId: disk.id, capturedAt: first, summary: disk.smart, temperatureC: disk.temperatureC },
        { diskId: disk.id, capturedAt: new Date().toISOString(), summary: disk.smart, temperatureC: disk.temperatureC },
      ],
      trend: {
        diskId: disk.id, sampleCount: 2, since: first, temperatureSlope: 0,
        reallocatedSlope: 0, pendingSlope: 0, uncorrectableSlope: 0, crcSlope: 0,
        status: disk.smart.overall, summary: 'Stable across 2 samples',
      },
    })
  }),

  http.get(`${BASE}/pools`, () => HttpResponse.json(pools)),

  http.get(`${BASE}/storage/protection`, () => HttpResponse.json(protection)),

  http.get(`${BASE}/storage/safety`, () => HttpResponse.json({ state: mockStorageUnlocked ? 'unlocked' : 'locked', unlockedUntil: mockStorageUnlocked ? new Date(Date.now() + 900_000).toISOString() : null })),

  http.get(`${BASE}/storage/snapshots`, ({ request }) => {
    const source = new URL(request.url).searchParams.get('source')
    const list = source ? storageSnapshots.filter((s) => s.source === source) : storageSnapshots
    return HttpResponse.json(list)
  }),

  http.get(`${BASE}/storage/snapshots/:id/files`, ({ params, request }) => {
    const snapshot = storageSnapshots.find((item) => item.id === params.id)
    if (!snapshot || snapshot.kind !== 'btrfs') return new HttpResponse(null, { status: 422 })
    const path = new URL(request.url).searchParams.get('path') ?? ''
    const entries = path === 'Documents'
      ? [{ name: 'family-photo.jpg', sizeBytes: 8_192, directory: false, modifiedAt: snapshot.createdAt }]
      : [{ name: 'Documents', sizeBytes: 0, directory: true, modifiedAt: snapshot.createdAt }]
    return HttpResponse.json({ path, entries, total: entries.length })
  }),

  http.get(`${BASE}/storage/snapshots/:id/compare`, ({ params }) => {
    const snapshot = storageSnapshots.find((item) => item.id === params.id)
    if (!snapshot || snapshot.kind !== 'btrfs') return HttpResponse.json({ error: 'Snapshot not found' }, { status: 422 })
    return HttpResponse.json({
      snapshotFiles: 42, currentFiles: 39, added: 1, deleted: 4, modified: 2, unchanged: 35,
      reviewRecommended: false,
      changes: [
        { path: 'Documents/notes.txt', kind: 'modified', sizeBytes: 1024, modifiedAt: new Date().toISOString() },
        { path: 'Documents/old-contract.pdf', kind: 'deleted', sizeBytes: 4096, modifiedAt: snapshot.createdAt },
      ],
    })
  }),

  http.post(`${BASE}/storage/snapshots/:id/restore`, async ({ params, request }) => {
    const snapshot = storageSnapshots.find((item) => item.id === params.id)
    if (!snapshot) return new HttpResponse(null, { status: 404 })
    const body = await request.json() as { names?: string[] }
    if (!body.names?.length) return HttpResponse.json({ error: 'select entries' }, { status: 422 })
    return HttpResponse.json({ jobId: 'job-snapshot-restore-e2e', state: 'queued', restoring: body.names.length }, { status: 202 })
  }),

  http.post(`${BASE}/storage/snapshots`, async ({ request }) => {
    const body = (await request.json()) as { kind?: string; source?: string; label?: string; retentionLockDays?: number }
    if (!body.kind || !body.source) return new HttpResponse(null, { status: 422 })
    const stamp = new Date().toISOString().replace(/[-:]/g, '').replace(/\.\d+Z$/, 'Z')
    const snapshot = {
      id: `snap-${++runtime.snapshotCounter}`,
      kind: body.kind as 'btrfs' | 'zfs',
      source: body.source,
      name: body.label ? `${body.label}-${stamp}` : stamp,
      label: body.label,
      createdAt: new Date().toISOString(),
      protectedUntil: body.retentionLockDays && body.retentionLockDays > 0 ? new Date(Date.now() + body.retentionLockDays * 86_400_000).toISOString() : undefined,
    }
    storageSnapshots.unshift(snapshot)
    pushActivity({
      category: 'storage',
      title: `Snapshot created — ${snapshot.name}`,
      description: `${body.kind} · ${body.source}`,
      resource: { type: 'snapshot', id: snapshot.id, label: snapshot.name },
    })
    return HttpResponse.json(snapshot, { status: 201 })
  }),

  http.delete(`${BASE}/storage/snapshots/:id`, ({ params }) => {
    const index = storageSnapshots.findIndex((s) => s.id === params.id)
    if (index < 0) return new HttpResponse(null, { status: 404 })
    const [removed] = storageSnapshots.splice(index, 1)
    pushActivity({
      category: 'storage',
      title: `Snapshot deleted — ${removed.name}`,
      description: `${removed.kind} · ${removed.source}`,
    })
    return HttpResponse.json({ status: 'deleted' })
  }),

  http.post(`${BASE}/storage/safety/unlock`, () => {
    mockStorageUnlocked = true
    return HttpResponse.json({ state: 'unlocked', unlockedUntil: new Date(Date.now() + 15 * 60_000).toISOString() })
  }),
  http.post(`${BASE}/storage/safety/lock`, () => {
    mockStorageUnlocked = false
    return HttpResponse.json({ state: 'locked', unlockedUntil: null })
  }),
  http.post(`${BASE}/storage/disks/:id/unlock`, async ({ request }) => {
    const body = (await request.json()) as { passphrase?: string }
    if (!body.passphrase) return HttpResponse.json({ error: 'passphrase is required' }, { status: 422 })
    if (!mockStorageUnlocked) return HttpResponse.json({ error: 'unlock storage safety first' }, { status: 423 })
    return HttpResponse.json({ ok: true, mountPath: `/srv/disks/${String(request.url).split('/').at(-2)}` })
  }),

  http.post(`${BASE}/storage/operations/plan`, async ({ request }) => {
    const body = (await request.json()) as {
      action?: string
      diskId?: string
      requestedState?: Record<string, unknown>
    }
    const disk = body.diskId ? findDisk(body.diskId) : undefined
    if (!disk || !body.action) return new HttpResponse(null, { status: 422 })
    return HttpResponse.json(
      {
        operationId: `op-${Date.now()}`,
        action: body.action,
        target: { diskId: disk.id, wwn: disk.wwn, serial: disk.serial, model: disk.model, sizeBytes: disk.sizeBytes },
        requestedState: body.requestedState ?? {},
        dependencySnapshot: [],
        configGeneration: 1,
        expiresAt: new Date(Date.now() + 15 * 60_000).toISOString(),
        planHash: `mock-${Math.random().toString(16).slice(2)}`,
        status: 'planned',
      },
      { status: 201 },
    )
  }),

  http.post(`${BASE}/storage/operations/:id/confirm`, async ({ request }) => {
    const body = (await request.json()) as { planHash?: string; storageSafetyUnlocked?: boolean }
    if (!body.planHash) return HttpResponse.json({ error: 'plan hash mismatch' }, { status: 409 })
    if (!body.storageSafetyUnlocked) {
      return HttpResponse.json({ error: 'reauthentication and the storage safety unlock are required' }, { status: 423 })
    }
    return HttpResponse.json({ ok: true })
  }),

  http.get(`${BASE}/jobs`, () => HttpResponse.json(jobs)),

  http.post(`${BASE}/jobs`, async ({ request }) => {
    const body = (await request.json()) as { type?: string; resourceId?: string; filesystemKind?: 'btrfs' | 'zfs'; filesystemSource?: string }
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
    } else if (type === 'filesystem.scrub') {
      if (!body.filesystemSource || !body.filesystemKind) return new HttpResponse(null, { status: 422 })
      title = `Filesystem scrub — ${body.filesystemSource}`
    } else {
      return new HttpResponse(null, { status: 422 })
    }
    const job = createJob(type, title, body.resourceId)
    return HttpResponse.json(job, { status: 201 })
  }),

  http.get(`${BASE}/alerts`, () => HttpResponse.json(alerts)),

  http.get(`${BASE}/alerts/history`, () => HttpResponse.json(alertHistory)),

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
  http.get(`${BASE}/quotas`, () => HttpResponse.json(mockQuotas)),
  http.put(`${BASE}/quotas`, async ({ request }) => {
    const policies = await request.json() as { id: string; targetType: 'share' | 'user' | 'group'; targetId: string; limitBytes: number; warningPercent: number }[]
    mockQuotas.splice(0, mockQuotas.length, ...policies.map((policy) => {
      const percent = 32 / (policy.limitBytes / 1024 ** 3) * 100
      return { policy: { ...policy, id: policy.id || `quota-${Date.now()}-${Math.random()}` }, usedBytes: 32 * 1024 ** 3, percent, state: (percent >= 100 ? 'over' : percent >= policy.warningPercent ? 'warning' : 'healthy') as 'over' | 'warning' | 'healthy', measuredAt: new Date().toISOString() }
    }))
    return HttpResponse.json(policies)
  }),
  http.get(`${BASE}/system/logs`, ({ request }) => {
    const url = new URL(request.url)
    const unit = url.searchParams.get('unit') || ''
    const source = url.searchParams.get('source') || ''
    const query = (url.searchParams.get('q') || '').toLowerCase()
    const entries = [
      { timestamp: new Date().toISOString(), priority: '6', unit: 'lumonasd.service', message: 'API ready; background jobs are healthy' },
      { timestamp: new Date(Date.now() - 60_000).toISOString(), priority: '4', unit: 'smbd.service', message: 'Client reconnect completed' },
      { timestamp: new Date(Date.now() - 120_000).toISOString(), priority: '5', unit: 'smbd_audit', message: 'anna|192.0.2.10|renameat|ok|Documents|draft.txt|final.txt' },
    ].filter((entry) => (!unit || entry.unit === unit) && (!source || (source === 'smb-audit' && entry.unit === 'smbd_audit')) && (!query || `${entry.unit} ${entry.message}`.toLowerCase().includes(query)))
    return HttpResponse.json({ entries, unit, source, query })
  }),

  http.get(`${BASE}/docker/summary`, () =>
    HttpResponse.json({
      stacks: stacks.length,
      appsRunning: containers.filter((c) => c.state === 'running' || c.state === 'restarting')
        .length,
      updatesAvailable: images.filter((i) => i.updateAvailable).length,
    }),
  ),

  http.get(`${BASE}/docker/apps`, () => HttpResponse.json(catalog.map((app, index) => ({
    ...app,
    trustStatus: index === 0 ? 'verified' as const : index === 1 ? 'invalid' as const : 'unverified' as const,
    trustMessage: index === 0 ? 'Catalog signature verified' : index === 1 ? 'Catalog signature does not match the configured trusted key' : 'Catalog signature is missing',
  })))),

  http.get(`${BASE}/docker/stacks`, () => HttpResponse.json(stacks)),

  http.get(`${BASE}/virtualization/status`, () => HttpResponse.json({
    libvirtAvailable: true, kvmAvailable: true, architecture: 'amd64', cpuCount: 8, memoryBytes: 16 * 1024 ** 3,
  })),
  http.get(`${BASE}/virtualization/media`, () => HttpResponse.json(mockVirtualizationMedia)),
  http.post(`${BASE}/virtualization/media`, async ({ request }) => {
    const form = await request.formData()
    const file = form.get('file')
    if (!file || typeof file === 'string' || !file.name.toLowerCase().endsWith('.iso')) {
      return HttpResponse.json({ error: 'A local ISO file is required' }, { status: 422 })
    }
    if (mockVirtualizationMedia.some((media) => media.name === file.name)) {
      return HttpResponse.json({ error: 'Installation media already exists' }, { status: 409 })
    }
    const media = { name: file.name, sizeBytes: file.size }
    mockVirtualizationMedia.push(media)
    return HttpResponse.json(media, { status: 201 })
  }),
  http.post(`${BASE}/virtualization/vms`, async ({ request }) => {
    const input = await request.json() as { name: string; iso: string; vcpus: number; memoryMiB: number; diskGiB: number }
    if (!mockVirtualizationMedia.some((media) => media.name === input.iso)) {
      return HttpResponse.json({ error: 'Selected ISO is unavailable' }, { status: 422 })
    }
    if (mockVirtualMachines.some((machine) => machine.name === input.name)) {
      return HttpResponse.json({ error: 'A VM with this name already exists' }, { status: 422 })
    }
    const machine: VirtualMachine = {
      name: input.name, state: 'running', vcpus: input.vcpus,
      maximumMemoryKiB: input.memoryMiB * 1024,
      diskPath: `/var/lib/libvirt/images/lumonas/${input.name}.qcow2`, iso: input.iso,
    }
    mockVirtualMachines.push(machine)
    return HttpResponse.json(machine, { status: 201 })
  }),
  http.get(`${BASE}/virtualization/vms`, () => HttpResponse.json(mockVirtualMachines)),
  http.get(`${BASE}/virtualization/recoverable-vms`, () => HttpResponse.json(mockRecoverableVirtualMachines)),
  http.post(`${BASE}/virtualization/vms/:name/restore-definition`, ({ params }) => {
    const name = String(params.name)
    const saved = mockRecoverableVirtualMachines.find((value) => value.name === name)
    if (!saved) return HttpResponse.json({ error: 'Saved VM recovery files are unavailable' }, { status: 404 })
    const machine: VirtualMachine = { name, state: 'shut off', diskPath: saved.diskPath, vcpus: 2, maximumMemoryKiB: 2097152 }
    mockRecoverableVirtualMachines = mockRecoverableVirtualMachines.filter((value) => value.name !== name)
    mockVirtualMachines.push(machine)
    return HttpResponse.json(machine, { status: 201 })
  }),
  http.delete(`${BASE}/virtualization/vms/:name`, async ({ params, request }) => {
    const name = String(params.name)
    const input = await request.json() as { confirmName: string; deleteDisk: boolean }
    const machine = mockVirtualMachines.find((value) => value.name === name)
    if (!machine) return HttpResponse.json({ error: 'VM not found' }, { status: 404 })
    if (input.confirmName !== name) return HttpResponse.json({ error: 'type the VM name exactly to confirm deletion' }, { status: 400 })
    if (machine.state !== 'shut off') return HttpResponse.json({ error: 'shut down the VM before deleting it' }, { status: 409 })
    mockVirtualMachines = mockVirtualMachines.filter((value) => value.name !== name)
    const diskPath = `/var/lib/libvirt/images/lumonas/${name}.qcow2`
    mockRecoverableVirtualMachines = mockRecoverableVirtualMachines.filter((value) => value.name !== name)
    if (!input.deleteDisk) mockRecoverableVirtualMachines.push({ name, diskPath, diskBytes: 8 * 1024 ** 3 })
    const result: VirtualMachineDeleteResult = { name, diskPath, diskRemoved: input.deleteDisk, definitionRetained: !input.deleteDisk }
    return HttpResponse.json(result)
  }),
  http.get(`${BASE}/virtualization/vms/:name/console`, ({ request }) => {
    const cursor = Number(new URL(request.url).searchParams.get('cursor') ?? '0')
    return HttpResponse.json({ output: cursor === 0 ? 'Guest serial console ready\r\n' : '', cursor: 30, connected: true })
  }),
  http.post(`${BASE}/virtualization/vms/:name/console`, () => HttpResponse.json({ status: 'sent' }, { status: 202 })),
  http.delete(`${BASE}/virtualization/vms/:name/console`, () => new HttpResponse(null, { status: 204 })),
  http.get(`${BASE}/virtualization/vms/:name/snapshots`, ({ params }) => HttpResponse.json(mockVirtualMachineSnapshots[String(params.name)] ?? [])),
  http.post(`${BASE}/virtualization/vms/:name/snapshots`, async ({ params, request }) => {
    const { name } = await request.json() as { name: string }
    const vmName = String(params.name)
    const list = mockVirtualMachineSnapshots[vmName] ?? []
    if (list.some((snapshot) => snapshot.name === name)) return HttpResponse.json({ error: 'Snapshot name already exists' }, { status: 409 })
    const snapshot: VirtualMachineSnapshot = { name, state: 'running', creationTime: new Date().toISOString() }
    mockVirtualMachineSnapshots[vmName] = [snapshot, ...list]
    return HttpResponse.json(snapshot, { status: 201 })
  }),
  http.post(`${BASE}/virtualization/vms/:name/snapshots/:snapshot/revert`, () => HttpResponse.json({ status: 'reverting' }, { status: 202 })),
  http.delete(`${BASE}/virtualization/vms/:name/snapshots/:snapshot`, ({ params }) => {
    const vmName = String(params.name)
    const snapshotName = String(params.snapshot)
    mockVirtualMachineSnapshots[vmName] = (mockVirtualMachineSnapshots[vmName] ?? []).filter((snapshot) => snapshot.name !== snapshotName)
    return new HttpResponse(null, { status: 204 })
  }),
  http.post(`${BASE}/virtualization/vms/:name/action`, async ({ params, request }) => {
    const { action } = await request.json() as { action: string }
    const machine = mockVirtualMachines.find((value) => value.name === params.name)
    if (!machine) return HttpResponse.json({ error: 'VM not found' }, { status: 404 })
    if (action === 'start' || action === 'resume') machine.state = 'running'
    if (action === 'shutdown') machine.state = 'shut off'
    if (action === 'suspend') machine.state = 'paused'
    if (action === 'reboot') machine.state = 'running'
    return HttpResponse.json(machine, { status: 202 })
  }),

  http.get(`${BASE}/docker/deployments`, ({ request }) => {
    const stack = new URL(request.url).searchParams.get('stack')
    const list = stack
      ? dockerDeployments.filter((deployment) => deployment.stackName === stack)
      : dockerDeployments
    return HttpResponse.json(list)
  }),

  http.get(`${BASE}/docker/stacks/:id`, ({ params }) => {
    const stack = findStack(params.id as string)
    if (!stack) return new HttpResponse(null, { status: 404 })
    return HttpResponse.json(stack)
  }),

  http.put(`${BASE}/docker/stacks/:id/recovery`, async ({ params, request }) => {
    const stack = findStack(params.id as string)
    if (!stack) return new HttpResponse(null, { status: 404 })
    const { appdataPaths } = await request.json() as { appdataPaths: string[] }
    stack.recovery = { ...(stack.recovery ?? { strategy: 'stop-backup' as const }), appdataPaths }
    stack.recoveryCoverage = appdataPaths.length > 0 ? 0.75 : 0.5
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

  http.get(`${BASE}/docker/images/packs`, () => HttpResponse.json(imagePacks)),

  http.post(`${BASE}/docker/images/packs/import`, async ({ request }) => {
    const payload = (await request.json()) as { name?: string }
    const name = payload.name
    const pack = imagePacks.find((p) => p.name === name)
    if (!pack) return new HttpResponse(null, { status: 404 })
    createJob('docker.import', `Import pack ${pack.name}`, pack.name)
    pushActivity({
      category: 'docker',
      title: `Image pack imported — ${pack.name}`,
      description: `${pack.imageCount} images verified and loaded`,
    })
    return HttpResponse.json({
      pack: pack.name,
      imported: Array.from({ length: pack.imageCount }, (_, i) => `${pack.name}/image-${i + 1}:latest`),
    })
  }),

  http.post(`${BASE}/docker/images/check-updates`, () => {
    const updated = images.map((image, index) => ({
      ...image,
      inUse: image.inUse ?? index % 2 === 0,
      updateAvailable: index === 0,
      localDigest: `sha256:${String(index).padStart(2, '0')}${'a'.repeat(62)}`,
      remoteDigest: `sha256:${String(index === 0 ? 9 : index).padStart(2, '0')}${'b'.repeat(62)}`,
    }))
    images.splice(0, images.length, ...updated)
    return HttpResponse.json(updated)
  }),

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

  http.post(`${BASE}/schedules`, async ({ request }) => {
    const input = await request.json() as Record<string, unknown> & { name: string; kind: string; timeOfDay: string; jobType?: string; snapshotSource?: string; snapshotKind?: string; snapshotKeep?: number; snapshotLockDays?: number }
    const schedule = { ...input, id: `schedule-${Date.now()}`, jobType: input.jobType ?? 'snapshot.create', schedule: input.kind === 'weekly' ? `${String(input.weekday).replace(/^./, (letter) => letter.toUpperCase())}s at ${input.timeOfDay}` : `Daily at ${input.timeOfDay}`, next: 'in 1d' }
    scheduledJobs.push(schedule as typeof scheduledJobs[number])
    return HttpResponse.json(schedule, { status: 201 })
  }),

  http.delete(`${BASE}/schedules/:id`, ({ params }) => {
    const index = scheduledJobs.findIndex((schedule) => schedule.id === params.id)
    if (index < 0) return new HttpResponse(null, { status: 404 })
    if (!scheduledJobs[index].id.startsWith('schedule-')) return new HttpResponse(null, { status: 409 })
    scheduledJobs.splice(index, 1)
    return new HttpResponse(null, { status: 204 })
  }),

  http.patch(`${BASE}/schedules/:id`, async ({ params, request }) => {
    const schedule = scheduledJobs.find((s) => s.id === params.id)
    if (!schedule) return new HttpResponse(null, { status: 404 })
    if (schedule.kind === 'event') return new HttpResponse(null, { status: 422 })
    const body = (await request.json()) as {
      enabled?: boolean
      timeOfDay?: string
      weekday?: string
      snapshotLockDays?: number
      filesystemKind?: 'btrfs' | 'zfs'
      filesystemSource?: string
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
    if (body.snapshotLockDays !== undefined) schedule.snapshotLockDays = body.snapshotLockDays
    if (body.filesystemKind !== undefined) schedule.filesystemKind = body.filesystemKind
    if (body.filesystemSource !== undefined) schedule.filesystemSource = body.filesystemSource
    return HttpResponse.json(schedule)
  }),

  http.get(`${BASE}/shares`, () => HttpResponse.json(shares)),
  http.get(`${BASE}/shares/storage-resources`, () => HttpResponse.json([
    { id: '/srv/pools/main', label: 'main pool', path: '/srv/pools/main', kind: 'pool', totalBytes: 8 * 1024 ** 4, usedBytes: 3 * 1024 ** 4 },
    { id: '/srv/disks/wwn_usb-backup', label: 'sdb · USB backup', path: '/srv/disks/wwn_usb-backup', kind: 'disk', totalBytes: 2 * 1024 ** 4, usedBytes: 512 * 1024 ** 3 },
  ])),

  http.post(`${BASE}/shares/:id/relocation/preview`, async ({ params, request }) => {
    const share = findShare(params.id as string)
    if (!share) return new HttpResponse(null, { status: 404 })
    const target = await request.json() as { resourceId: string; relativePath: string }
    const resource = [
      { id: '/srv/pools/main', path: '/srv/pools/main', label: 'main pool' },
      { id: '/srv/disks/wwn_usb-backup', path: '/srv/disks/wwn_usb-backup', label: 'sdb · USB backup' },
    ].find((value) => value.id === target.resourceId)
    if (!resource) return HttpResponse.json({ error: 'Destination must be mounted' }, { status: 422 })
    return HttpResponse.json({
      shareId: share.id,
      shareName: share.name,
      sourcePath: share.path,
      destinationRoot: resource.path,
      relativePath: target.relativePath,
      destinationPath: `${resource.path}/${target.relativePath}`,
      fileCount: 17,
      bytes: 84_000_000,
      plan: { changes: [], files: 17, bytes: 84_000_000, deletes: 0 },
      planHash: 'e2e-relocation-plan-hash',
      retainsSource: true,
      requiresDowntime: true,
    })
  }),
  http.post(`${BASE}/shares/:id/relocation`, async ({ params, request }) => {
    const share = findShare(params.id as string)
    if (!share) return new HttpResponse(null, { status: 404 })
    const input = await request.json() as { confirmed: boolean; planHash: string; scheduleKind?: string; timeOfDay?: string; weekday?: string }
    if (!input.confirmed || input.planHash !== 'e2e-relocation-plan-hash') {
      return HttpResponse.json({ error: 'A current preview is required' }, { status: 409 })
    }
    const preview = {
      shareId: share.id, shareName: share.name, sourcePath: share.path,
      destinationRoot: '/srv/disks/wwn_usb-backup', relativePath: 'Documents',
      destinationPath: '/srv/disks/wwn_usb-backup/Documents', fileCount: 17,
      bytes: 84_000_000, plan: { changes: [], files: 17, bytes: 84_000_000, deletes: 0 },
      planHash: 'e2e-relocation-plan-hash', retainsSource: true, requiresDowntime: true,
    }
    if (input.scheduleKind && input.scheduleKind !== 'manual') {
      return HttpResponse.json({ schedule: { id: 'schedule-share-relocation-e2e', name: 'Move Documents', jobType: 'share.relocate', kind: input.scheduleKind, timeOfDay: input.timeOfDay ?? '02:00', weekday: input.weekday, enabled: true, schedule: `${input.scheduleKind === 'daily' ? 'Daily' : 'Weekly'} at ${input.timeOfDay ?? '02:00'}`, next: 'tomorrow' }, preview }, { status: 201 })
    }
    return HttpResponse.json({ jobId: 'job-share-relocation-e2e', state: 'queued', preview }, { status: 202 })
  }),

  http.get(`${BASE}/shares/clients`, () => HttpResponse.json({ sessions: [{ sessionId: 'smb-session-e2e', username: 'e2e-client', machine: '192.0.2.41', dialect: 'SMB3_11', share: 'Documents' }], openFiles: [{ path: 'report.pdf', sharePath: '/srv/pools/main/Documents', opens: 1 }], capturedAt: new Date().toISOString(), service: 'smbd.service' })),
  http.post(`${BASE}/shares/clients/disconnect`, async ({ request }) => {
    const input = await request.json() as { address: string; confirmed: boolean }
    if (!input.confirmed) return HttpResponse.json({ error: 'explicit confirmation is required' }, { status: 423 })
    return HttpResponse.json({ address: input.address, disconnected: true })
  }),

  http.get(`${BASE}/shares/:id`, ({ params }) => {
    const share = findShare(params.id as string)
    if (!share) return new HttpResponse(null, { status: 404 })
    return HttpResponse.json(share)
  }),

  http.get(`${BASE}/shares/:id/access-preview`, ({ params }) => {
    const share = findShare(params.id as string)
    if (!share) return new HttpResponse(null, { status: 404 })
    const entries = principals.map((principal) => {
      const direct = share.access.find((rule) => rule.principalId === principal.id)
      return {
        principalId: principal.id,
        name: principal.name,
        kind: principal.type,
        enabled: true,
        level: direct?.level ?? 'none',
        grantedBy: direct ? [direct.principalId] : [],
      }
    })
    return HttpResponse.json({
      shareId: share.id,
      name: share.name,
      enabled: share.status !== 'offline',
      guestAccess: false,
      entries,
      accessRules: share.access,
    })
  }),

  http.get(`${BASE}/shares/:id/access-check`, ({ params, request }) => {
    const share = findShare(params.id as string)
    if (!share) return new HttpResponse(null, { status: 404 })
    const search = new URL(request.url).searchParams
    const principal = principals.find((item) => item.id === search.get('principal'))
    if (!principal || principal.type !== 'user') return HttpResponse.json({ error: 'select an existing user account' }, { status: 422 })
    const rule = share.access.find((item) => item.principalId === principal.id)
    const level = rule?.level ?? 'none'
    const allowed = share.status !== 'offline' && level !== 'none'
    return HttpResponse.json({ shareId: share.id, principalId: principal.id, path: search.get('path') ?? '.', shareEnabled: share.status !== 'offline', accountEnabled: true, shareLevel: level, filesystemAccess: 'read', allowed, explanation: allowed ? 'The user can access this path based on the configured share rule and Unix mode bits.' : 'No direct or group share rule grants this user access.', approximate: true })
  }),

  http.post(`${BASE}/shares`, async ({ request }) => {
    const body = (await request.json()) as {
      name?: string
      resourceId?: string
      resourceLabel?: string
      relativePath?: string
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
      path: body.resourceId.startsWith('/') ? `${body.resourceId}/${(body.relativePath ?? '/').replace(/^\/+|\/+$/g, '') || body.name}` : `/srv/pools/${body.resourceId}/${body.name}`,
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
    return new HttpResponse(null, { status: 204 })
  }),

  http.patch(`${BASE}/users/:id`, async ({ params, request }) => {
    const body = (await request.json()) as { enabled?: boolean }
    const user =
      managementUsers.find((u) => u.id === params.id) ?? fileUsers.find((u) => u.id === params.id)
    if (!user) return new HttpResponse(null, { status: 404 })
    if ('enabled' in user) user.enabled = body.enabled ?? !user.enabled
    return HttpResponse.json(user)
  }),

  http.post(`${BASE}/users/:id/2fa/setup`, ({ params }) => {
    const user = managementUsers.find((u) => u.id === params.id)
    if (!user) return new HttpResponse(null, { status: 404 })
    return HttpResponse.json({
      secret: 'JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP',
      otpauthUri: `otpauth://totp/LumoNAS:${encodeURIComponent(user.username)}?secret=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP&issuer=LumoNAS&algorithm=SHA1&digits=6&period=30`,
      recoveryCodes: Array.from({ length: 8 }, (_, index) => `lumo-${(100000 + index * 7919) % 1000000}-${(200000 + index * 104729) % 1000000}`),
    })
  }),

  http.post(`${BASE}/users/:id/2fa/enable`, async ({ params, request }) => {
    const user = managementUsers.find((u) => u.id === params.id)
    if (!user) return new HttpResponse(null, { status: 404 })
    const body = (await request.json()) as { code?: string }
    if (!/^\d{6}$/.test(body.code ?? '')) return new HttpResponse(null, { status: 422 })
    user.twoFactor = true
    return HttpResponse.json({ twoFactor: true })
  }),

  http.post(`${BASE}/users/:id/2fa/disable`, ({ params }) => {
    const user = managementUsers.find((u) => u.id === params.id)
    if (!user) return new HttpResponse(null, { status: 404 })
    user.twoFactor = false
    return HttpResponse.json({ twoFactor: false })
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

  http.patch(`${BASE}/network/connections/:id`, async ({ params, request }) => {
    const connection = connections.find((item) => item.id === params.id)
    if (!connection) return new HttpResponse(null, { status: 404 })
    Object.assign(connection, await request.json())
    return HttpResponse.json(connection)
  }),

  http.post(`${BASE}/network/connections/:id/apply`, ({ params }) => {
    const connection = applyConnection(params.id as string)
    if (!connection) return new HttpResponse(null, { status: 404 })
    return HttpResponse.json(connection)
  }),

  http.get(`${BASE}/network/bindings`, () => HttpResponse.json(bindings)),

  http.get(`${BASE}/network/firewall/policy`, () => HttpResponse.json(firewall)),

  http.get(`${BASE}/install/targets`, () =>
    HttpResponse.json([
      {
        diskId: 'wwn:system-usb',
        name: 'sda',
        model: 'Installer Medium (USB)',
        sizeBytes: 32_000_000_000,
        role: 'system',
        filesystem: 'ext4',
        mounted: true,
        eligible: false,
        protectedBy: ['running system disk'],
        identity: { wwn: 'wwn:system-usb', serial: 'USB1', sizeBytes: String(32_000_000_000) },
      },
      {
        diskId: 'wwn:blank-target',
        name: 'sdb',
        model: 'WDC WD40EFRX',
        sizeBytes: 4_000_000_000_000,
        role: 'unknown',
        mounted: false,
        eligible: true,
        identity: { wwn: 'wwn:blank-target', serial: 'WD1', sizeBytes: String(4_000_000_000_000) },
      },
      {
        diskId: 'wwn:data-disk',
        name: 'sdc',
        model: 'Seagate IronWolf',
        sizeBytes: 8_000_000_000_000,
        role: 'data',
        filesystem: 'ext4',
        mounted: true,
        eligible: false,
        protectedBy: ['disk is mounted', 'disk holds an existing filesystem'],
        identity: { wwn: 'wwn:data-disk', serial: 'SG1', sizeBytes: String(8_000_000_000_000) },
      },
    ]),
  ),

  http.post(`${BASE}/install/plan`, async ({ request }) => {
    const body = (await request.json()) as {
      targetDiskId?: string
      hostname?: string
      adminUsername?: string
      filesystem?: string
    }
    if (!body.targetDiskId || !body.hostname) return new HttpResponse(null, { status: 422 })
    const now = new Date()
    const expires = new Date(now.getTime() + 10 * 60_000)
    return HttpResponse.json(
      {
        plan: {
          id: `install-${now.getTime()}`,
          targetDiskId: body.targetDiskId,
          expectedIdentity: { wwn: body.targetDiskId, sizeBytes: String(4_000_000_000_000) },
          hostname: body.hostname,
          adminUsername: body.adminUsername ?? 'admin',
          filesystem: body.filesystem ?? 'ext4',
          uefi: true,
          createdAt: now.toISOString(),
          expiresAt: expires.toISOString(),
        },
        hash: 'mock-plan-hash-0123456789abcdef',
        targets: [],
      },
      { status: 201 },
    )
  }),

  http.post(`${BASE}/install/apply`, async ({ request }) => {
    const body = (await request.json()) as { hash?: string; confirm?: boolean; adminPassword?: string }
    if (!body.hash || !body.confirm) return new HttpResponse(null, { status: 422 })
    if ((body.adminPassword ?? '').length < 12) {
      return new HttpResponse(null, { status: 422 })
    }
    pushActivity({
      category: 'config',
      title: 'Installation completed',
      description: 'Reboot and remove the installer medium',
    })
    return HttpResponse.json({ status: 'succeeded', hostname: 'lumonas' })
  }),

  http.get(`${BASE}/install/status`, () => HttpResponse.json({ stage: 'idle' })),

  http.get(`${BASE}/network/lan/hosts`, () => HttpResponse.json(lanHosts)),

  http.post(`${BASE}/network/lan/scan`, () => {
    for (const host of lanHosts) host.lastSeen = new Date().toISOString()
    return HttpResponse.json(lanHosts)
  }),

  http.post(`${BASE}/network/lan/hosts/wake`, async ({ request }) => {
    const body = (await request.json()) as { mac?: string; interface?: string }
    if (!body.mac || !body.interface) return new HttpResponse(null, { status: 422 })
    pushActivity({
      category: 'network',
      title: `Wake packet sent — ${body.mac.toLowerCase()}`,
      description: `via ${body.interface}`,
    })
    return HttpResponse.json({ status: 'wake packet sent' })
  }),

  http.post(`${BASE}/network/lan/hosts/rename`, async ({ request }) => {
    const body = (await request.json()) as { mac?: string; interface?: string; hostname?: string }
    const host = lanHosts.find((h) => h.mac === body.mac?.toLowerCase() && h.interface === body.interface)
    if (!host) return new HttpResponse(null, { status: 404 })
    host.hostname = body.hostname
    return HttpResponse.json({ status: 'renamed' })
  }),

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

  http.get(`${BASE}/file-requests`, ({ request }) => {
    const shareId = new URL(request.url).searchParams.get('shareId')
    return HttpResponse.json(mockFileRequests.filter((item) => item.shareId === shareId))
  }),
  http.post(`${BASE}/file-requests`, async ({ request }) => {
    const body = await request.json() as { shareId: string; path: string; expiresInHours: number; maxFiles: number; maxBytes: number }
    if (!body.shareId || body.maxFiles < 1 || body.maxBytes < 1) return HttpResponse.json({ error: 'Invalid request' }, { status: 422 })
    const id = `file-request-${Date.now()}`
    const token = `mock-secret-${Math.random().toString(36).slice(2)}`
    const item = { id, shareId: body.shareId, path: body.path, createdAt: new Date().toISOString(), expiresAt: new Date(Date.now() + body.expiresInHours * 3600_000).toISOString(), maxFiles: body.maxFiles, maxBytes: body.maxBytes, receivedFiles: 0, receivedBytes: 0 }
    mockFileRequests.unshift(item)
    return HttpResponse.json({ request: item, url: `/request/${token}` }, { status: 201 })
  }),
  http.delete(`${BASE}/file-requests/:id`, ({ params }) => {
    const item = mockFileRequests.find((entry) => entry.id === params.id)
    if (!item) return new HttpResponse(null, { status: 404 })
    item.revokedAt = new Date().toISOString()
    return new HttpResponse(null, { status: 204 })
  }),
  http.get(`${BASE}/file-share-links`, ({ request }) => {
    const shareId = new URL(request.url).searchParams.get('shareId')
    return HttpResponse.json(mockFileShareLinks.filter((item) => item.shareId === shareId).map(({ token: _token, ...item }) => item))
  }),
  http.post(`${BASE}/file-share-links`, async ({ request }) => {
    const body = await request.json() as { shareId: string; path: string; expiresInHours: number; password?: string }
    if (!body.shareId || body.expiresInHours < 1 || (body.password && body.password.length < 12)) return HttpResponse.json({ error: 'Invalid link settings' }, { status: 422 })
    const token = `mock-share-${Math.random().toString(36).slice(2)}`
    const item = { id: `file-share-${Date.now()}`, shareId: body.shareId, path: body.path, createdAt: new Date().toISOString(), expiresAt: new Date(Date.now() + body.expiresInHours * 3600_000).toISOString(), downloads: 0, token }
    mockFileShareLinks.unshift(item)
    return HttpResponse.json({ link: { id: item.id, shareId: item.shareId, path: item.path, createdAt: item.createdAt, expiresAt: item.expiresAt, downloads: item.downloads }, url: `/share/${token}` }, { status: 201 })
  }),
  http.delete(`${BASE}/file-share-links/:id`, ({ params }) => {
    const item = mockFileShareLinks.find((entry) => entry.id === params.id)
    if (!item) return new HttpResponse(null, { status: 404 })
    item.revokedAt = new Date().toISOString()
    return new HttpResponse(null, { status: 204 })
  }),
  http.get(`${BASE}/public/file-requests/:token`, () => HttpResponse.json({ shareName: 'Photos', expiresAt: new Date(Date.now() + 3600_000).toISOString(), maxFiles: 5, remainingFiles: 5, maxBytes: 5 * 1024 ** 3, remainingBytes: 5 * 1024 ** 3 })),
  http.post(`${BASE}/public/file-requests/:token/upload`, async ({ request }) => {
    const form = await request.formData()
    const file = form.get('file')
    if (!(file instanceof File)) return HttpResponse.json({ error: 'No file' }, { status: 400 })
    return HttpResponse.json({ name: file.name, sizeBytes: file.size, remainingFiles: 4, remainingBytes: 5 * 1024 ** 3 - file.size }, { status: 201 })
  }),
  http.get(`${BASE}/public/file-share-links/:token`, ({ params }) => {
    const item = mockFileShareLinks.find((entry) => entry.token === params.token)
    if (item?.revokedAt || (item && Date.parse(item.expiresAt) <= Date.now())) return HttpResponse.json({ error: 'Share link expired or revoked' }, { status: 404 })
    return HttpResponse.json({ shareName: 'Photos', path: item?.path ?? '/2025', expiresAt: item?.expiresAt ?? new Date(Date.now() + 3600_000).toISOString(), passwordProtected: false })
  }),
  http.get(`${BASE}/public/file-share-links/:token/files`, ({ request, params }) => {
    const item = mockFileShareLinks.find((entry) => entry.token === params.token)
    const requestedPath = new URL(request.url).searchParams.get('path') ?? ''
    const base = item?.path ?? '/2025'
    const scopedPath = [base === '/' ? '' : base.replace(/^\//, ''), requestedPath].filter(Boolean).join('/')
    return HttpResponse.json({ shareName: 'Photos', path: scopedPath || '/', entries: listDir('share-photos', scopedPath || '/') ?? [] })
  }),
  http.get(`${BASE}/public/file-share-links/:token/download`, ({ request, params }) => {
    const item = mockFileShareLinks.find((entry) => entry.token === params.token)
    if (item) item.downloads += 1
    const name = new URL(request.url).searchParams.get('name') ?? 'download'
    return new HttpResponse(`Mock public download for ${name}`, { headers: { 'Content-Type': 'application/octet-stream' } })
  }),

  http.get(`${BASE}/files/search`, ({ request }) => {
    const url = new URL(request.url)
    const shareId = url.searchParams.get('share') ?? ''
    const entries = searchFiles(shareId, url.searchParams.get('q') ?? '')
    return HttpResponse.json({ shareId, entries })
  }),
  http.get(`${BASE}/files/search-index`, ({ request }) => {
    const shareId = new URL(request.url).searchParams.get('shareId') ?? ''
    const current = mockContentIndex.get(shareId) ?? { documents: 0, indexedAt: null }
    return HttpResponse.json({ shareId, ...current, maxFileBytes: 512 * 1024, maxDocuments: 20000, maxIndexBytes: 64 * 1024 * 1024 })
  }),
  http.post(`${BASE}/files/search-index`, async ({ request }) => {
    const { shareId } = await request.json() as { shareId: string }
    const current = { documents: 42, indexedAt: new Date().toISOString() }
    mockContentIndex.set(shareId, current)
    return HttpResponse.json({ shareId, ...current, skipped: 2, maxFileBytes: 512 * 1024, maxDocuments: 20000, maxIndexBytes: 64 * 1024 * 1024 })
  }),
  http.get(`${BASE}/files/content-search`, ({ request }) => {
    const params = new URL(request.url).searchParams
    const shareId = params.get('share') ?? ''
    const current = mockContentIndex.get(shareId)
    if (!current?.indexedAt) return HttpResponse.json({ error: 'build a content index for this share before searching' }, { status: 409 })
    return HttpResponse.json({ shareId, indexedAt: current.indexedAt, results: params.get('q') ? [{ shareId, path: 'Documents', name: 'meeting-notes.txt', snippet: 'Project launch checklist: confirm ...', sizeBytes: 2048, modifiedAt: new Date().toISOString() }] : [] })
  }),
  http.get(`${BASE}/files/integrity`, ({ request }) => {
    const shareId = new URL(request.url).searchParams.get('shareId') ?? ''
    return HttpResponse.json({ shareId, ...(mockIntegrityStatus.get(shareId) ?? { fileCount: 0, running: false }) })
  }),
  http.post(`${BASE}/files/integrity/baseline`, async ({ request }) => {
    const { shareId } = await request.json() as { shareId: string }
    const baselineAt = new Date().toISOString()
    mockIntegrityStatus.set(shareId, { fileCount: 4, baselineAt, running: false })
    return HttpResponse.json({ id: `job-integrity-${Date.now()}`, type: 'file.transfer', title: 'Create file integrity baseline', state: 'successful', progress: 100, createdAt: baselineAt, finishedAt: baselineAt })
  }),
  http.post(`${BASE}/files/integrity/verify`, async ({ request }) => {
    const { shareId } = await request.json() as { shareId: string }
    const previous = mockIntegrityStatus.get(shareId)
    if (!previous?.baselineAt) return HttpResponse.json({ error: 'create an integrity baseline before verifying this share' }, { status: 409 })
    const verifiedAt = new Date().toISOString()
    mockIntegrityStatus.set(shareId, { ...previous, report: { baselineAt: previous.baselineAt, verifiedAt, unchanged: 2, changedCount: 1, missingCount: 0, addedCount: 1, changed: ['notes.txt'], missing: [], added: ['new.txt'] }, running: false })
    return HttpResponse.json({ id: `job-integrity-${Date.now()}`, type: 'file.transfer', title: 'Verify file integrity', state: 'successful', progress: 100, createdAt: verifiedAt, finishedAt: verifiedAt })
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
  http.get(`${BASE}/files/download/archive`, () => new HttpResponse('mock archive', { headers: { 'Content-Type': 'application/zip' } })),

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

  http.post(`${BASE}/files/uploads`, async ({ request }) => {
    const body = await request.json() as { shareId: string; path: string; name: string; sizeBytes: number }
    if (!body.shareId || !body.name || !Number.isFinite(body.sizeBytes) || body.sizeBytes < 0) {
      return HttpResponse.json({ error: 'Invalid upload request' }, { status: 422 })
    }
    const id = `upload-${Date.now()}-${Math.random().toString(16).slice(2, 8)}`
    const session = { id, ...body, receivedBytes: 0 }
    mockUploadSessions.set(id, session)
    return HttpResponse.json({ id, receivedBytes: 0, sizeBytes: body.sizeBytes }, { status: 201 })
  }),

  http.get(`${BASE}/files/uploads/:id`, ({ params }) => {
    const session = mockUploadSessions.get(params.id as string)
    if (!session) return new HttpResponse(null, { status: 404 })
    return HttpResponse.json({ id: session.id, receivedBytes: session.receivedBytes, sizeBytes: session.sizeBytes })
  }),

  http.put(`${BASE}/files/uploads/:id`, async ({ params, request }) => {
    const session = mockUploadSessions.get(params.id as string)
    if (!session) return new HttpResponse(null, { status: 404 })
    const offset = Number(request.headers.get('Upload-Offset') ?? -1)
    if (offset !== session.receivedBytes) return HttpResponse.json({ error: 'Upload offset mismatch' }, { status: 409 })
    session.receivedBytes += (await request.arrayBuffer()).byteLength
    return new HttpResponse(null, { status: 204 })
  }),

  http.post(`${BASE}/files/uploads/:id/complete`, ({ params }) => {
    const session = mockUploadSessions.get(params.id as string)
    if (!session) return new HttpResponse(null, { status: 404 })
    if (session.receivedBytes !== session.sizeBytes) return HttpResponse.json({ error: 'Upload is incomplete' }, { status: 409 })
    const name = insertFile(session.shareId, session.path, session.name, session.sizeBytes)
    mockUploadSessions.delete(session.id)
    const job = createJob('file.upload', `Upload ${name}`)
    return HttpResponse.json({ jobId: job.id, name }, { status: 202 })
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

  http.get(`${BASE}/backups/schedule`, () => HttpResponse.json(mockBackupSchedule)),
  http.get(`${BASE}/backups/policy-templates`, () => HttpResponse.json([
    { id: 'daily', name: 'Daily protection', description: 'One verified recovery copy each day with balanced retention.', intervalSeconds: 86400, generations: 20, daily: 30, monthly: 12 },
    { id: 'frequent', name: 'Frequent protection', description: 'Run twice a day and keep more recent recovery points.', intervalSeconds: 43200, generations: 40, daily: 60, monthly: 24 },
    { id: 'weekly', name: 'Weekly protection', description: 'Run weekly while retaining the baseline recovery history.', intervalSeconds: 604800, generations: 20, daily: 30, monthly: 12 },
  ])),
  http.post(`${BASE}/backups/policy-template`, async ({ request }) => {
    const { templateId } = await request.json() as { templateId: string }
    const template = { daily: { intervalSeconds: 86400 }, frequent: { intervalSeconds: 43200 }, weekly: { intervalSeconds: 604800 } }[templateId]
    if (!template) return HttpResponse.json({ error: 'unknown backup policy template' }, { status: 422 })
    mockBackupSchedule = { ...mockBackupSchedule, intervalSeconds: template.intervalSeconds, updatedAt: new Date().toISOString() }
    return HttpResponse.json(mockBackupSchedule)
  }),
  http.get(`${BASE}/backups/runs`, () => HttpResponse.json([{
    run: { id: 'run-e2e-vm', state: 'verified', generation: 9, startedAt: new Date().toISOString() },
    copies: [
      { id: 'copy-vm-manifest', runId: 'run-e2e-vm', destinationId: 'dest-usb', object: 'virtual-machines/run-e2e-vm/manifest.json', state: 'verified', verified: true },
      { id: 'copy-vm-disk', runId: 'run-e2e-vm', destinationId: 'dest-usb', object: 'virtual-machines/run-e2e-vm/media-vm/disks/vda.qcow2.sparse', state: 'verified', verified: true },
      { id: 'copy-vm-xml', runId: 'run-e2e-vm', destinationId: 'dest-usb', object: 'virtual-machines/run-e2e-vm/media-vm/definition.xml', state: 'verified', verified: true },
    ],
  }])),
  http.patch(`${BASE}/backups/schedule`, async ({ request }) => {
    mockBackupSchedule = { ...mockBackupSchedule, ...(await request.json() as Partial<BackupSchedule>), updatedAt: new Date().toISOString() }
    return HttpResponse.json(mockBackupSchedule)
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
  http.get(`${BASE}/folder-sync/tasks`, () => HttpResponse.json(mockFolderSyncTasks)),
  http.post(`${BASE}/folder-sync/tasks`, async ({ request }) => {
    const input = await request.json() as Omit<(typeof mockFolderSyncTasks)[number], 'id' | 'updatedAt'>
    const task = { ...input, id: `sync-${Date.now()}`, updatedAt: new Date().toISOString() }
    mockFolderSyncTasks.unshift(task)
    return HttpResponse.json(task, { status: 200 })
  }),
  http.delete(`${BASE}/folder-sync/tasks/:id`, ({ params }) => {
    const index = mockFolderSyncTasks.findIndex((task) => task.id === params.id)
    if (index < 0) return new HttpResponse(null, { status: 404 })
    mockFolderSyncTasks.splice(index, 1)
    return new HttpResponse(null, { status: 204 })
  }),
  http.post(`${BASE}/folder-sync/tasks/:id/preview`, ({ params }) => {
    const task = mockFolderSyncTasks.find((item) => item.id === params.id)
    if (task?.source.kind === 'destination' && task.mode === 'copy') return HttpResponse.json({
      plan: { changes: [{ path: 'Photos/album.jpg', action: 'copy', bytes: 2048 }, { path: 'Photos/favorites.jpg', action: 'copy', bytes: 4096 }], files: 2, bytes: 6144, deletes: 0, conflicts: 0 },
      planHash: 'nas-import-preview-hash', requiresConfirmation: false,
    })
    if (task?.direction === 'two-way') return HttpResponse.json({
      plan: { changes: [{ path: 'Photos/album.jpg', action: 'update', bytes: 2048, from: 'right', to: 'left' }, { path: 'notes.txt', action: 'archive', bytes: 128, from: 'left', to: 'right', archivePath: '.lumonas-versions/notes.txt.20260929.initial-left' }], files: 2, bytes: 2176, deletes: 0, conflicts: 1 },
      planHash: 'two-way-preview-hash', requiresConfirmation: true,
    })
    return HttpResponse.json({
      plan: { changes: [{ path: 'Photos/album.jpg', action: 'copy', bytes: 2048 }, { path: 'old.txt', action: 'delete', bytes: 64 }], files: 1, bytes: 2048, deletes: 1, conflicts: 0 },
      planHash: 'preview-hash', requiresConfirmation: task?.mode === 'mirror',
    })
  }),
  http.post(`${BASE}/folder-sync/tasks/:id/run`, () => HttpResponse.json({ id: `sync-run-${Date.now()}`, state: 'running', startedAt: new Date().toISOString(), files: 1, bytes: 2048, deleted: 1 }, { status: 202 })),
  http.get(`${BASE}/folder-sync/tasks/:id/runs`, () => HttpResponse.json([])),

  http.get(`${BASE}/backup/generations`, () => HttpResponse.json(generations)),

  http.get(`${BASE}/backup/restore/plan`, () => HttpResponse.json(restorePlan)),

  http.get(`${BASE}/recovery/status`, () => HttpResponse.json({
    configured: true,
    latestPath: '/var/lib/lumonas/recovery/latest.bundle',
    verified: true,
    manifest: {
      formatVersion: 1,
      configSchema: 1,
      lumonasVersion: '0.1.0-e2e',
      nasUuid: 'mock-nas-e2e',
      generation: 1843,
      createdAt: new Date(Date.now() - 3_600_000).toISOString(),
      diskIds: disks.map((disk) => disk.id),
      checksums: { 'config/state.json': 'e2e-checksum' },
    },
    warnings: [],
  })),
  http.get(`${BASE}/recovery/plan`, () => HttpResponse.json({
    manifest: {
      formatVersion: 1, configSchema: 1, lumonasVersion: '0.1.0-e2e', nasUuid: 'mock-nas-e2e',
      generation: 1843, createdAt: new Date(Date.now() - 3_600_000).toISOString(), diskIds: disks.map((disk) => disk.id), checksums: {},
    },
    files: ['config/state.json', 'docker/stacks/jellyfin/compose.yaml'],
    verified: true, databaseValid: true, desiredStateValid: true, composeValid: true, encryptedSecrets: true,
    appdata: [{ stack: 'jellyfin', containerPath: '/config', hostPath: '/srv/lumonas/docker/appdata/jellyfin', archivePath: 'docker/appdata/jellyfin/config.tar.gz', archiveBytes: 1024 }],
    warnings: [],
  })),
  http.get(`${BASE}/recovery/download`, () => new HttpResponse('mock verified recovery bundle', { headers: { 'Content-Type': 'application/vnd.lumonas.recovery' } })),
  http.post(`${BASE}/recovery/export`, () => HttpResponse.json({
    path: '/var/lib/lumonas/recovery/e2e.bundle',
    manifest: { formatVersion: 1, configSchema: 1, lumonasVersion: '0.1.0-e2e', nasUuid: 'mock-nas-e2e', generation: 1843, createdAt: new Date().toISOString(), diskIds: disks.map((disk) => disk.id), checksums: {} },
    verified: true, appdataArchives: 1, warnings: [],
  }, { status: 201 })),
  http.post(`${BASE}/recovery/restore/stage`, () => HttpResponse.json({ directory: '/tmp/lumonas-restore-e2e', files: ['config/state.json'], verified: true }, { status: 201 })),
  http.post(`${BASE}/recovery/restore/stage-appdata`, async ({ request }) => {
    const { stack } = await request.json() as { stack: string }
    return HttpResponse.json({ scope: 'appdata', stack, directory: `/tmp/lumonas-restore-${stack}`, files: [`docker/appdata/${stack}/config.tar.gz`, `docker/stacks/${stack}/compose.yaml`], verified: true, generation: 1843 }, { status: 202 })
  }),
  http.get(`${BASE}/recovery/drills`, () => HttpResponse.json(mockRestoreDrills)),
  http.get(`${BASE}/recovery/drills/schedule`, () => HttpResponse.json(mockRestoreDrillSchedule)),
  http.patch(`${BASE}/recovery/drills/schedule`, async ({ request }) => {
    mockRestoreDrillSchedule = { ...mockRestoreDrillSchedule, ...(await request.json() as Partial<RestoreDrillSchedule>), updatedAt: new Date().toISOString() }
    return HttpResponse.json(mockRestoreDrillSchedule)
  }),
  http.post(`${BASE}/recovery/drills/run`, () => {
    const now = new Date().toISOString()
    const drill: RestoreDrill = {
      id: `restore-drill-${Date.now()}`, trigger: 'manual', state: 'successful', generation: 1843,
      startedAt: now, finishedAt: now, verified: true, databaseValid: true, composeValid: true,
      secretsRestored: true, databaseRestored: true, appdataRestored: ['jellyfin'], sharesRestored: ['Media'],
      servicesRehearsed: ['samba'], servicesHealthy: true, appliedFiles: 2,
    }
    mockRestoreDrills.unshift(drill)
    return HttpResponse.json(drill, { status: 202 })
  }),
  http.get(`${BASE}/recovery/workload-objectives`, () => HttpResponse.json(mockWorkloadObjectives)),
  http.put(`${BASE}/recovery/workload-objectives`, async ({ request }) => {
    const objective = await request.json() as { workloadId: string; rpoHours: number; rtoMinutes: number; updatedAt: string }
    const existing = mockWorkloadObjectives.findIndex((item) => item.workloadId === objective.workloadId)
    if (existing < 0) mockWorkloadObjectives.push(objective)
    else mockWorkloadObjectives[existing] = objective
    return HttpResponse.json(objective)
  }),

  http.get(`${BASE}/troubleshooting`, () => HttpResponse.json({ status: 'clear', issues: [] })),
  http.get(`${BASE}/dependencies/graph`, () => HttpResponse.json({
    generatedAt: new Date().toISOString(),
    nodes: disks.map((disk) => ({ id: `disk:${disk.id}`, type: 'disk', label: disk.name, status: disk.health })),
    edges: [],
  })),

  http.get(`${BASE}/settings`, () => HttpResponse.json(settings)),
  http.get(`${BASE}/security/certificate`, () => HttpResponse.json({ state: 'valid', configured: true, issuer: 'LumoNAS Local CA', subject: 'nas.local', dnsNames: ['nas.local'], notBefore: new Date(Date.now() - 86_400_000).toISOString(), notAfter: new Date(Date.now() + 180 * 86_400_000).toISOString(), daysRemaining: 180, detail: 'Certificate is currently within its validity period.', managedBy: 'manual', autoRenewalConfigured: false, acmeFirewallConfigured: false })),
  http.post(`${BASE}/security/certificate/import`, async ({ request }) => {
    const input = (await request.json()) as { certificate?: string; privateKey?: string }
    if (!input.certificate?.includes('BEGIN CERTIFICATE') || !input.privateKey?.includes('PRIVATE KEY')) {
      return HttpResponse.json({ error: 'certificate and private key are required' }, { status: 422 })
    }
    return HttpResponse.json({ id: 'job-tls-install', type: 'security.certificate.install', title: 'Install TLS certificate', state: 'queued', createdAt: new Date().toISOString() }, { status: 202 })
  }),
  http.post(`${BASE}/security/certificate/acme`, async ({ request }) => {
    const input = (await request.json()) as { domain?: string; email?: string; agreeToTerms?: boolean; allowPort80?: boolean }
    if (!input.domain || !input.email || !input.agreeToTerms || !input.allowPort80) return HttpResponse.json({ error: 'domain, email, terms, and local firewall consent are required' }, { status: 422 })
    return HttpResponse.json({ id: 'job-tls-acme', type: 'security.certificate.acme', title: "Configure Let's Encrypt", state: 'queued', createdAt: new Date().toISOString() }, { status: 202 })
  }),
  http.post(`${BASE}/security/certificate/acme/disable`, () => HttpResponse.json({ autoRenewalConfigured: false, managedBy: 'manual' })),

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
  http.get(`${BASE}/updates/preflight`, () => HttpResponse.json({ ready: true, checks: [], blockers: [], warnings: [] })),

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

  http.post(`${BASE}/updates/slot/stage`, async ({ request }) => {
    const body = (await request.json()) as {
      imagePath?: string
      manifest?: { version?: string }
      signature?: string
    }
    if (!body.imagePath || !body.manifest?.version || !body.signature) {
      return new HttpResponse(null, { status: 422 })
    }
    updateSlots.pendingSlot = updateSlots.activeSlot === 'a' ? 'b' : 'a'
    updateSlots.pendingVersion = body.manifest.version
    updateSlots.bootAttempts = 0
    updateSlots.updatedAt = new Date().toISOString()
    pushActivity({
      category: 'update',
      title: `OS image staged — ${body.manifest.version}`,
      description: 'Verified for the inactive slot',
    })
    return HttpResponse.json(updateSlots, { status: 202 })
  }),

  http.post(`${BASE}/updates/slot/activate`, () => {
    if (!updateSlots.pendingSlot) return new HttpResponse(null, { status: 409 })
    pushActivity({
      category: 'update',
      title: `Slot image written — ${updateSlots.pendingVersion}`,
      description: 'BootNext armed; reboot to start the new slot',
    })
    return HttpResponse.json({ status: 'activated', slot: updateSlots.pendingSlot, version: updateSlots.pendingVersion }, { status: 202 })
  }),

  http.post(`${BASE}/updates/slot/confirm`, () => {
    if (!updateSlots.pendingSlot) return new HttpResponse(null, { status: 409 })
    updateSlots.activeSlot = updateSlots.pendingSlot
    updateSlots.activeVersion = updateSlots.pendingVersion ?? updateSlots.activeVersion
    updateSlots.previousSlot = updateSlots.activeSlot === 'a' ? 'b' : 'a'
    updateSlots.pendingSlot = undefined
    updateSlots.pendingVersion = undefined
    updateSlots.bootAttempts = 0
    updateSlots.updatedAt = new Date().toISOString()
    pushActivity({
      category: 'update',
      title: `OS slot committed — ${updateSlots.activeVersion}`,
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

  http.get(`${BASE}/storage/mounts`, () => HttpResponse.json({ entries: [
    { kind: 'pool', targetId: 'pool-main', mountPath: '/srv/pools/main', fstype: 'mergerfs', source: 'disk-a:disk-b', options: '', enabled: true },
    { kind: 'disk', targetId: 'disk-usb', mountPath: '/srv/disks/disk-usb', fstype: 'ext4', source: 'UUID=usb-e2e', options: '', enabled: true },
  ] })),
  http.get(`${BASE}/storage/protection/config`, () => HttpResponse.json({ configPath: '/etc/lumonas/snapraid.conf', configured: true, parityDiskIds: ['disk-parity'], dataDiskIds: disks.filter((disk) => disk.role === 'data').map((disk) => disk.id) })),
  http.get(`${BASE}/storage/capacity/forecast`, () => HttpResponse.json([])),
  http.get(`${BASE}/storage/disks/:id/smart-history`, () => HttpResponse.json({ diskId: 'disk-data-1', days: 90, samples: [], trend: 'stable' })),
  http.post(`${BASE}/storage/protection/replacement/plan`, async ({ request }) => {
    const body = await request.json() as { retiredDiskId: string; replacementDiskId: string }
    return HttpResponse.json({ operationId: 'replacement-e2e', retiredDiskId: body.retiredDiskId, retiredDataName: 'data-1', replacementDiskId: body.replacementDiskId, parityDiskId: 'disk-parity', configGeneration: 1843, expiresAt: new Date(Date.now() + 600_000).toISOString(), planHash: 'replacement-plan-hash', status: 'planned' }, { status: 201 })
  }),
  http.post(`${BASE}/storage/protection/replacement/confirm`, () => HttpResponse.json({ ok: true, jobId: 'job-replacement-e2e', slot: 'data-1', next: 'Run SnapRAID fix' }, { status: 202 })),
  http.post(`${BASE}/jobs/:id/cancel`, ({ params }) => {
    const job = jobs.find((item) => item.id === params.id)
    if (!job) return new HttpResponse(null, { status: 404 })
    job.state = 'cancelled'
    return HttpResponse.json(job)
  }),
  http.post(`${BASE}/jobs/:id/retry`, ({ params }) => {
    const job = jobs.find((item) => item.id === params.id)
    if (!job) return new HttpResponse(null, { status: 404 })
    return HttpResponse.json(createJob(job.type, job.title), { status: 202 })
  }),
  http.get(`${BASE}/network/diagnostics/:id`, ({ params }) => HttpResponse.json({
    job: { id: params.id, state: 'successful', progress: 100, title: 'Network diagnostic' },
    result: { reachable: true, target: '1.1.1.1' },
  })),
  http.post(`${BASE}/network/checkpoints`, () => HttpResponse.json({ operationId: 'checkpoint-e2e', status: 'open', expiresAt: new Date(Date.now() + 120_000).toISOString() }, { status: 201 })),
  http.post(`${BASE}/network/checkpoints/:id/:action`, () => HttpResponse.json({ ok: true })),
  http.get(`${BASE}/network/wireguard/status`, () => HttpResponse.json({ configured: false, running: false, interface: 'wg0', publicKey: '', address: '', peers: [] })),
  http.post(`${BASE}/network/wireguard/keygen`, () => HttpResponse.json({ privateKey: 'mock-private-key', publicKey: 'mock-public-key' })),
  http.post(`${BASE}/network/wireguard/apply`, () => HttpResponse.json({ ok: true })),
  http.get(`${BASE}/network/tailscale/status`, () => HttpResponse.json({ installed: true, running: true, connected: false, hostName: '', tailscaleIp4: '', tailscaleIp6: '', version: '', subnetRoutes: [] })),
  http.post(`${BASE}/network/tailscale/up`, () => HttpResponse.json({ ok: true })),
  http.post(`${BASE}/network/tailscale/down`, () => HttpResponse.json({ ok: true })),
  http.post(`${BASE}/network/tailscale/exit-node`, () => HttpResponse.json({ ok: true })),
  http.get(`${BASE}/api-tokens`, () => HttpResponse.json(mockAPITokens)),
  http.post(`${BASE}/api-tokens`, async ({ request }) => {
    const body = await request.json() as { name: string; scopes: ('read' | 'backup:write' | 'replication:receive' | 'fleet:status' | `workstation:backup:${string}` | `replication:snapshot:receive:${string}`)[]; expiresAt?: string }
    const summary = { id: `token-${Date.now()}`, name: body.name, scopes: body.scopes, createdAt: new Date().toISOString(), expiresAt: body.expiresAt }
    mockAPITokens.push(summary)
    return HttpResponse.json({ token: 'lumo_e2e_mock_token_once', summary }, { status: 201 })
  }),
  http.delete(`${BASE}/api-tokens/:id`, ({ params }) => {
    const index = mockAPITokens.findIndex((token) => token.id === params.id)
    if (index < 0) return new HttpResponse(null, { status: 404 })
    mockAPITokens.splice(index, 1)
    return new HttpResponse(null, { status: 204 })
  }),
  http.get(`${BASE}/replication/peers`, () => HttpResponse.json(mockReplicationPeers)),
  http.post(`${BASE}/replication/peers`, async ({ request }) => {
    const body = await request.json() as { name: string; url: string; statusToken?: string }
    const peer: ReplicationPeer = { id: `peer-${Date.now()}`, name: body.name, url: body.url, createdAt: new Date().toISOString(), remoteStatus: 'online', remoteVersion: body.statusToken ? '0.1.0-dev' : undefined, remoteHealth: body.statusToken ? 'healthy' : undefined, remoteCheckedAt: body.statusToken ? new Date().toISOString() : undefined }
    mockReplicationPeers.push(peer)
    return HttpResponse.json(peer, { status: 201 })
  }),
  http.post(`${BASE}/replication/peers/:id/run`, ({ params }) => {
    const peer = mockReplicationPeers.find((item) => item.id === params.id)
    if (!peer) return new HttpResponse(null, { status: 404 })
    peer.lastSyncAt = new Date().toISOString()
    peer.lastError = undefined
    return HttpResponse.json({ ok: true, peerId: peer.id, bytes: 1024 })
  }),
  http.delete(`${BASE}/replication/peers/:id`, ({ params }) => {
    const index = mockReplicationPeers.findIndex((peer) => peer.id === params.id)
    if (index < 0) return new HttpResponse(null, { status: 404 })
    mockReplicationPeers.splice(index, 1)
    return new HttpResponse(null, { status: 204 })
  }),
  http.get(`${BASE}/replication/snapshot-tasks`, ({ request }) => {
    const peerId = new URL(request.url).searchParams.get('peerId')
    return HttpResponse.json(peerId ? mockSnapshotReplicationTasks.filter((task) => task.peerId === peerId) : mockSnapshotReplicationTasks)
  }),
  http.post(`${BASE}/replication/snapshot-tasks`, async ({ request }) => {
    const body = await request.json() as Omit<SnapshotReplicationTask,'id'|'createdAt'|'lastSyncAt'|'lastAttemptAt'|'lastSnapshotName'|'lastError'|'running'>
    const task: SnapshotReplicationTask = { ...body, id: `snapshot-task-${Date.now()}`, createdAt: new Date().toISOString() }
    mockSnapshotReplicationTasks.push(task)
    return HttpResponse.json(task, { status: 201 })
  }),
  http.put(`${BASE}/replication/snapshot-tasks/:id`, async ({ params, request }) => {
    const task = mockSnapshotReplicationTasks.find((item) => item.id === params.id)
    if (!task) return new HttpResponse(null, { status: 404 })
    const body = await request.json() as Pick<SnapshotReplicationTask,'name'|'scheduleKind'|'timeOfDay'> & { weekday?: string }
    Object.assign(task, body)
    return HttpResponse.json(task)
  }),
  http.post(`${BASE}/replication/snapshot-tasks/:id/run`, ({ params }) => {
    const task = mockSnapshotReplicationTasks.find((item) => item.id === params.id)
    if (!task) return new HttpResponse(null, { status: 404 })
    const startedAt = new Date().toISOString()
    task.lastAttemptAt = new Date().toISOString()
    task.lastSyncAt = task.lastAttemptAt
    task.lastSnapshotName = `replica-${Date.now()}`
    task.lastError = undefined
    const run: SnapshotReplicationRun = { id: `run-${Date.now()}`, taskId: task.id, jobId: `job-${Date.now()}`, state: 'successful', stage: 'completed', progress: 100, bytes: 1024, snapshotName: task.lastSnapshotName, createdAt: startedAt, startedAt, finishedAt: new Date().toISOString() }
    mockSnapshotReplicationRuns.unshift(run)
    return HttpResponse.json({ ok: true, run, jobId: run.jobId }, { status: 202 })
  }),
  http.post(`${BASE}/replication/snapshot-tasks/:id/cancel`, ({ params }) => {
    const task = mockSnapshotReplicationTasks.find((item) => item.id === params.id)
    if (!task) return new HttpResponse(null, { status: 404 })
    task.running = false
    return HttpResponse.json({ status: 'cancellation_requested' }, { status: 202 })
  }),
  http.get(`${BASE}/replication/snapshot-tasks/:id/runs`, ({ params }) => HttpResponse.json(mockSnapshotReplicationRuns.filter((run) => run.taskId === params.id))),
  http.delete(`${BASE}/replication/snapshot-tasks/:id`, ({ params }) => {
    const index = mockSnapshotReplicationTasks.findIndex((task) => task.id === params.id)
    if (index < 0) return new HttpResponse(null, { status: 404 })
    mockSnapshotReplicationTasks.splice(index, 1)
    return new HttpResponse(null, { status: 204 })
  }),
  http.get(`${BASE}/audit/retention`, () => HttpResponse.json({ retentionDays: mockAuditRetentionDays })),
  http.put(`${BASE}/audit/retention`, async ({ request }) => {
    mockAuditRetentionDays = (await request.json() as { retentionDays: number }).retentionDays
    return HttpResponse.json({ retentionDays: mockAuditRetentionDays })
  }),
  http.get(`${BASE}/audit`, ({ request }) => {
    const url = new URL(request.url)
    const params = url.searchParams
    const entries = mockAuditEntries.filter((entry) => {
      const search = (params.get('q') ?? '').toLowerCase()
      return (!params.get('actor') || entry.actor.includes(params.get('actor')!)) &&
        (!params.get('action') || entry.action.includes(params.get('action')!)) &&
        (!params.get('outcome') || entry.outcome === params.get('outcome')) &&
        (!search || `${entry.actor} ${entry.action} ${entry.resourceType ?? ''} ${entry.resourceId ?? ''}`.toLowerCase().includes(search))
    })
    const offset = Number(params.get('cursor') ?? 0)
    const limit = Math.max(1, Math.min(100, Number(params.get('limit') ?? 100)))
    const page = entries.slice(offset, offset + limit)
    const hasMore = offset + limit < entries.length
    return HttpResponse.json({ entries: page, hasMore, ...(hasMore ? { nextCursor: String(offset + limit) } : {}) })
  }),
  http.get(`${BASE}/audit/export`, ({ request }) => {
    const url = new URL(request.url)
    const search = (url.searchParams.get('q') ?? '').toLowerCase()
    const rows = mockAuditEntries.filter((entry) => !search || `${entry.actor} ${entry.action} ${entry.resourceId ?? ''}`.toLowerCase().includes(search))
    const csv = ['timestamp,actor,action,resource,outcome', ...rows.map((entry) => [entry.timestamp, entry.actor, entry.action, `${entry.resourceType ?? ''}/${entry.resourceId ?? ''}`, entry.outcome].map((value) => `"${String(value).replaceAll('"', '""')}"`).join(','))].join('\n')
    return new HttpResponse(csv, { headers: { 'Content-Type': 'text/csv; charset=utf-8', 'Content-Disposition': 'attachment; filename="audit.csv"' } })
  }),
  http.get(`${BASE}/backups/destinations`, () => HttpResponse.json(destinations.map((item) => ({ id: item.id, name: item.label, type: item.type === 'nas' ? 'local' : item.type, enabled: item.enabled })))),
  http.post(`${BASE}/backups/destinations`, async ({ request }) => {
    const body = await request.json() as { name: string; type: string; target: string }
    const destination = { id: `dest-${Date.now()}`, enabled: true, type: body.type, label: body.name, target: body.target, encrypted: true, status: 'healthy', detail: 'E2E destination' }
    destinations.push(destination as typeof destinations[number])
    return HttpResponse.json(destination, { status: 201 })
  }),
  http.post(`${BASE}/backup/destinations/:id/restore-check`, ({ params }) => HttpResponse.json({ destinationId: params.id, verified: true, appdataRestored: ['jellyfin'], checkedAt: new Date().toISOString() })),
  http.post(`${BASE}/backup/destinations/:id/runs/:runId/virtual-machines/:name/restore`, async ({ params, request }) => {
    const body = await request.json() as { confirmName: string }
    const name = String(params.name)
    if (String(params.runId) !== 'run-e2e-vm' || name !== 'media-vm' || body.confirmName !== name) return HttpResponse.json({ error: 'verified VM backup is unavailable' }, { status: 409 })
    if (mockVirtualMachines.some((machine) => machine.name === name)) return HttpResponse.json({ error: 'a VM with this name already exists' }, { status: 409 })
    const machine: VirtualMachine = { name, state: 'shut off', diskPath: `/var/lib/libvirt/images/lumonas/${name}.qcow2`, vcpus: 2, maximumMemoryKiB: 2097152 }
    mockVirtualMachines.push(machine)
    return HttpResponse.json(machine, { status: 201 })
  }),
  http.get(`${BASE}/backup/history`, () => HttpResponse.json([])),
  http.get(`${BASE}/groups`, () => HttpResponse.json(groups.map((group) => ({ id: group.id, name: group.name, kind: 'group', enabled: true })))),
  http.post(`${BASE}/groups`, async ({ request }) => {
    const body = await request.json() as { name: string }
    if (!body.name.trim()) return new HttpResponse(null, { status: 422 })
    const group = { id: `group-${Date.now()}`, name: body.name, members: [] as string[] }
    groups.push(group)
    return HttpResponse.json({ id: group.id, name: group.name, type: 'group' }, { status: 201 })
  }),
  http.put(`${BASE}/groups/:id/members`, async ({ params, request }) => {
    const group = groups.find((item) => item.id === params.id)
    if (!group) return new HttpResponse(null, { status: 404 })
    group.members = (await request.json() as { memberIds: string[] }).memberIds
    return HttpResponse.json([])
  }),
  http.delete(`${BASE}/groups/:id`, ({ params }) => {
    const index = groups.findIndex((group) => group.id === params.id)
    if (index < 0) return new HttpResponse(null, { status: 404 })
    groups.splice(index, 1)
    return new HttpResponse(null, { status: 204 })
  }),
  http.post(`${BASE}/users/:id/password`, () => new HttpResponse(null, { status: 204 })),
  http.delete(`${BASE}/users/:id`, ({ params }) => {
    const managementIndex = managementUsers.findIndex((user) => user.id === params.id)
    if (managementIndex >= 0) managementUsers.splice(managementIndex, 1)
    const fileIndex = fileUsers.findIndex((user) => user.id === params.id)
    if (fileIndex >= 0) fileUsers.splice(fileIndex, 1)
    return new HttpResponse(null, { status: 204 })
  }),
  http.get(`${BASE}/users/self/passkeys`, () => HttpResponse.json([])),
  http.delete(`${BASE}/users/self/passkeys/:id`, () => new HttpResponse(null, { status: 204 })),
  http.get(`${BASE}/ssh/keys`, () => HttpResponse.json([])),
  http.post(`${BASE}/ssh/keys`, () => new HttpResponse(null, { status: 201 })),
  http.post(`${BASE}/ssh/keys/remove`, () => new HttpResponse(null, { status: 204 })),
  http.get(`${BASE}/notification-deliveries`, () => HttpResponse.json([])),
  http.post(`${BASE}/notification-channels`, async ({ request }) => {
    const body = await request.json() as { type: string; label: string; target: string; enabled: boolean }
    const channel = { id: `channel-${Date.now()}`, ...body, configured: true }
    notificationChannels.push(channel as typeof notificationChannels[number])
    return HttpResponse.json(channel, { status: 201 })
  }),
  http.patch(`${BASE}/notification-channels/:id`, async ({ params, request }) => {
    const channel = notificationChannels.find((item) => item.id === params.id)
    if (!channel) return new HttpResponse(null, { status: 404 })
    Object.assign(channel, await request.json())
    return HttpResponse.json(channel)
  }),
  http.post(`${BASE}/notification-channels/:id/test`, ({ params }) => HttpResponse.json({ sent: true, channelId: params.id })),
  http.delete(`${BASE}/notification-channels/:id`, ({ params }) => {
    const index = notificationChannels.findIndex((channel) => channel.id === params.id)
    if (index >= 0) notificationChannels.splice(index, 1)
    return new HttpResponse(null, { status: 204 })
  }),
  http.get(`${BASE}/notification-rules`, () => HttpResponse.json(alertRules)),
  http.post(`${BASE}/notification-rules`, async ({ request }) => {
    const body = await request.json() as { name: string; condition: string; severity?: string; routes?: string[] }
    const rule = { id: `notification-rule-${Date.now()}`, name: body.name, condition: body.condition, severity: body.severity ?? 'warning', routes: body.routes ?? [], enabled: true }
    alertRules.push(rule as typeof alertRules[number])
    return HttpResponse.json(rule, { status: 201 })
  }),
  http.patch(`${BASE}/notification-rules/:id`, async ({ params, request }) => {
    const rule = alertRules.find((item) => item.id === params.id)
    if (!rule) return new HttpResponse(null, { status: 404 })
    Object.assign(rule, await request.json())
    return HttpResponse.json(rule)
  }),
  http.get(`${BASE}/capacity/forecast`, () => HttpResponse.json([])),
  http.get(`${BASE}/capacity/thresholds`, () => HttpResponse.json([])),
  http.put(`${BASE}/capacity/thresholds`, async ({ request }) => HttpResponse.json(await request.json())),
  http.get(`${BASE}/diagnostics/support-bundle`, () => new HttpResponse('mock-support-bundle', { headers: { 'Content-Type': 'application/zip' } })),
  http.post(`${BASE}/settings/sessions`, () => HttpResponse.json({ revoked: 1 })),
  http.delete(`${BASE}/settings/sessions`, () => HttpResponse.json({ revoked: 1 })),
  http.delete(`${BASE}/settings/sessions/:id`, () => new HttpResponse(null, { status: 204 }))
]
