export type HealthState = 'healthy' | 'attention' | 'warning' | 'critical' | 'offline'

export type DiskRole = 'system' | 'apps' | 'data' | 'parity' | 'backup' | 'external' | 'unknown'

export interface SmartSummary {
  overall: HealthState
  reallocatedSectors: number
  pendingSectors: number
  uncorrectableSectors: number
  crcErrors: number
  powerOnHours: number
  wearPercent?: number
  lastTest?: { type: 'short' | 'extended'; result: 'passed' | 'failed'; at: string }
}

export interface Disk {
  id: string
  name: string
  model: string
  serial: string
  wwn?: string
  sizeBytes: number
  usedBytes?: number
  role: DiskRole
  rotational: boolean
  interface: 'sata' | 'nvme' | 'usb'
  health: HealthState
  temperatureC: number | null
  filesystem?: string
  poolId?: string
  standby?: boolean
  lastSeen: string
  smart: SmartSummary
}

export interface PoolMember {
  diskId: string
  enabled: boolean
}

export interface Pool {
  id: string
  name: string
  type: 'mergerfs'
  mountPath: string
  status: HealthState
  sizeBytes: number
  usedBytes: number
  members: PoolMember[]
}

export interface Protection {
  status: HealthState
  parityDisks: { diskId: string; sizeBytes: number; usedBytes: number }[]
  protectedDiskIds: string[]
  lastSyncAt: string | null
  lastSyncResult: 'successful' | 'failed' | null
  lastScrubAt: string | null
  changesSinceSyncBytes: number
  syncSchedule: string
  scrubSchedule: string
  syncRunning: boolean
}

export type JobState =
  | 'queued'
  | 'preparing'
  | 'running'
  | 'waiting-confirmation'
  | 'successful'
  | 'failed'
  | 'cancelled'

export interface Job {
  id: string
  type: string
  title: string
  resourceId?: string
  state: JobState
  progress: number | null
  stage?: string
  createdAt: string
  startedAt?: string
  finishedAt?: string
  error?: string
}

export type AlertSeverity = 'info' | 'attention' | 'warning' | 'critical'

export interface Alert {
  id: string
  severity: AlertSeverity
  title: string
  description: string
  resource?: { type: string; id: string; label: string }
  state: 'firing' | 'acknowledged' | 'resolved'
  startedAt: string
}

export type ActivityCategory =
  | 'config'
  | 'storage'
  | 'docker'
  | 'backup'
  | 'security'
  | 'update'
  | 'network'

export interface ActivityEvent {
  id: string
  timestamp: string
  category: ActivityCategory
  title: string
  description?: string
  resource?: { type: string; id: string; label: string }
}

export interface ServerInfo {
  id: string
  name: string
  hostname: string
  version: string
  nasUuid: string
  timezone: string
  health: HealthState
  ip: string
}

export interface SystemMetrics {
  cpuPercent: number
  load: [number, number, number]
  ramUsedBytes: number
  ramTotalBytes: number
  cpuTempC: number
  uptimeSeconds: number
  net: { interface: string; upMbps: number; downMbps: number }
}

export interface DockerSummary {
  stacks: number
  appsRunning: number
  updatesAvailable: number
}

export interface LumoEvent<T = Record<string, unknown>> {
  id: string
  type: string
  timestamp: string
  severity: 'info' | 'warning' | 'critical'
  resource?: { type: string; id: string }
  data: T
}
