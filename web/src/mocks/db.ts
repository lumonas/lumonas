import type {
  ActivityEvent,
  Alert,
  Disk,
  Job,
  Pool,
  Protection,
  ServerInfo,
} from '@/api/types'

const now = Date.now()
const minutesAgo = (m: number) => new Date(now - m * 60_000).toISOString()
const hoursAgo = (h: number) => new Date(now - h * 3_600_000).toISOString()
const daysAgo = (d: number) => new Date(now - d * 86_400_000).toISOString()

const TB = 1_000_000_000_000
const GB = 1_000_000_000

function healthySmart(hours: number, extra?: Partial<Disk['smart']>): Disk['smart'] {
  return {
    overall: 'healthy',
    reallocatedSectors: 0,
    pendingSectors: 0,
    uncorrectableSectors: 0,
    crcErrors: 0,
    powerOnHours: hours,
    lastTest: { type: 'short', result: 'passed', at: daysAgo(6) },
    ...extra,
  }
}

export const disks: Disk[] = [
  {
    id: 'disk-sys',
    name: 'nvme0n1',
    model: 'WD Red SN700',
    serial: '23013A8009A4',
    wwn: 'eui.002538b1710b4f52',
    sizeBytes: 500_107_862_016,
    usedBytes: 214 * GB,
    role: 'system',
    rotational: false,
    interface: 'nvme',
    health: 'healthy',
    temperatureC: 42,
    filesystem: 'ext4',
    standby: false,
    lastSeen: minutesAgo(0),
    smart: healthySmart(8_742, { wearPercent: 3 }),
  },
  {
    id: 'disk-apps',
    name: 'nvme1n1',
    model: 'Crucial P3 Plus',
    serial: 'CT1000P3PSSD8',
    wwn: 'eui.0025385b71c4a901',
    sizeBytes: 1_000_204_886_016,
    usedBytes: 412 * GB,
    role: 'apps',
    rotational: false,
    interface: 'nvme',
    health: 'healthy',
    temperatureC: 39,
    filesystem: 'ext4',
    standby: false,
    lastSeen: minutesAgo(0),
    smart: healthySmart(6_211, { wearPercent: 6 }),
  },
  {
    id: 'disk-parity',
    name: 'sda',
    model: 'Seagate Exos X18 12TB',
    serial: 'ZHZ9XK2Z',
    wwn: '0x5000c301d4a12b71',
    sizeBytes: 12 * TB,
    usedBytes: 6.2 * TB,
    role: 'parity',
    rotational: true,
    interface: 'sata',
    health: 'healthy',
    temperatureC: 35,
    filesystem: 'ext4',
    poolId: undefined,
    standby: false,
    lastSeen: minutesAgo(0),
    smart: healthySmart(8_766),
  },
  {
    id: 'disk-data-1',
    name: 'sdb',
    model: 'WD Red Plus 8TB',
    serial: 'VK8H2A7R',
    wwn: '0x50014ee20d3f8a61',
    sizeBytes: 8 * TB,
    usedBytes: 6.2 * TB,
    role: 'data',
    rotational: true,
    interface: 'sata',
    health: 'healthy',
    temperatureC: 36,
    filesystem: 'ext4',
    poolId: 'pool-main',
    standby: false,
    lastSeen: minutesAgo(0),
    smart: healthySmart(19_884),
  },
  {
    id: 'disk-data-2',
    name: 'sdc',
    model: 'WD Red Plus 8TB',
    serial: 'VK8J4P2M',
    wwn: '0x50014ee20d3f8a62',
    sizeBytes: 8 * TB,
    usedBytes: 5.8 * TB,
    role: 'data',
    rotational: true,
    interface: 'sata',
    health: 'healthy',
    temperatureC: 37,
    filesystem: 'ext4',
    poolId: 'pool-main',
    standby: false,
    lastSeen: minutesAgo(0),
    smart: healthySmart(20_113),
  },
  {
    id: 'disk-data-3',
    name: 'sdd',
    model: 'Seagate Exos X18 12TB',
    serial: 'ZHZ9XK7B',
    wwn: '0x5000c301d4a12b72',
    sizeBytes: 12 * TB,
    usedBytes: 4.9 * TB,
    role: 'data',
    rotational: true,
    interface: 'sata',
    health: 'healthy',
    temperatureC: 34,
    filesystem: 'ext4',
    poolId: 'pool-main',
    standby: false,
    lastSeen: minutesAgo(0),
    smart: healthySmart(9_102),
  },
  {
    id: 'disk-data-4',
    name: 'sde',
    model: 'Toshiba MG08 12TB',
    serial: '61C0A0KVFQ7G',
    wwn: '0x5000039cb4e21a05',
    sizeBytes: 12 * TB,
    usedBytes: 5.1 * TB,
    role: 'data',
    rotational: true,
    interface: 'sata',
    health: 'warning',
    temperatureC: 41,
    filesystem: 'ext4',
    poolId: 'pool-main',
    standby: false,
    lastSeen: minutesAgo(0),
    smart: {
      overall: 'warning',
      reallocatedSectors: 0,
      pendingSectors: 4,
      uncorrectableSectors: 0,
      crcErrors: 0,
      powerOnHours: 21_450,
      lastTest: { type: 'short', result: 'passed', at: daysAgo(6) },
    },
  },
  {
    id: 'disk-external',
    name: 'sdh',
    model: 'Seagate Expansion 5TB',
    serial: 'NA9B1KP9',
    sizeBytes: 5 * TB,
    usedBytes: 2.1 * TB,
    role: 'external',
    rotational: true,
    interface: 'usb',
    health: 'offline',
    temperatureC: null,
    filesystem: 'ntfs',
    standby: false,
    lastSeen: daysAgo(6),
    smart: {
      overall: 'offline',
      reallocatedSectors: 0,
      pendingSectors: 0,
      uncorrectableSectors: 0,
      crcErrors: 0,
      powerOnHours: 1_204,
    },
  },
]

export const pools: Pool[] = [
  {
    id: 'pool-main',
    name: 'Main',
    type: 'mergerfs',
    mountPath: '/srv/pools/main',
    status: 'attention',
    sizeBytes: 40 * TB,
    usedBytes: 22 * TB,
    members: [
      { diskId: 'disk-data-1', enabled: true },
      { diskId: 'disk-data-2', enabled: true },
      { diskId: 'disk-data-3', enabled: true },
      { diskId: 'disk-data-4', enabled: true },
    ],
  },
]

export const protection: Protection = {
  status: 'healthy',
  parityDisks: [{ diskId: 'disk-parity', sizeBytes: 12 * TB, usedBytes: 6.2 * TB }],
  protectedDiskIds: ['disk-data-1', 'disk-data-2', 'disk-data-3', 'disk-data-4'],
  lastSyncAt: hoursAgo(5),
  lastSyncResult: 'successful',
  lastScrubAt: daysAgo(3),
  changesSinceSyncBytes: 8.3 * GB,
  syncSchedule: 'Daily at 02:00',
  scrubSchedule: 'Sundays at 03:00',
  syncRunning: true,
}

export const jobs: Job[] = [
  {
    id: 'job-1',
    type: 'snapraid.sync',
    title: 'SnapRAID sync',
    state: 'running',
    progress: 34,
    stage: 'Hashing data disks',
    createdAt: minutesAgo(26),
    startedAt: minutesAgo(26),
  },
  {
    id: 'job-2',
    type: 'backup.app',
    title: 'App backup — Immich',
    resourceId: 'stack-immich',
    state: 'successful',
    progress: 100,
    createdAt: hoursAgo(3),
    startedAt: hoursAgo(3),
    finishedAt: hoursAgo(3),
  },
  {
    id: 'job-3',
    type: 'smart.extended',
    title: 'SMART extended test — sdb',
    resourceId: 'disk-data-1',
    state: 'successful',
    progress: 100,
    createdAt: daysAgo(2),
    startedAt: daysAgo(2),
    finishedAt: daysAgo(2),
  },
]

export const alerts: Alert[] = [
  {
    id: 'alert-1',
    severity: 'warning',
    title: 'Toshiba MG08 12TB — sectors pending reallocation',
    description: '4 sectors are pending reallocation. Data is still readable — plan a replacement soon.',
    resource: { type: 'disk', id: 'disk-data-4', label: 'Disk sde' },
    state: 'firing',
    startedAt: daysAgo(2),
  },
  {
    id: 'alert-2',
    severity: 'attention',
    title: 'Recovery readiness at 87%',
    description: 'Appdata backup is missing for pihole — the offsite copy is also 9 days old.',
    state: 'firing',
    startedAt: daysAgo(1),
  },
  {
    id: 'alert-3',
    severity: 'info',
    title: 'Updates available',
    description: '3 apps have newer images available.',
    resource: { type: 'docker', id: 'updates', label: 'Docker' },
    state: 'firing',
    startedAt: hoursAgo(8),
  },
  {
    id: 'alert-4',
    severity: 'info',
    title: 'New admin sign-in',
    description: 'Signed in from a new device (192.168.1.22, Firefox on LAN).',
    state: 'acknowledged',
    startedAt: hoursAgo(26),
  },
]

export const alertHistory: Alert[] = [
  {
    id: 'alert-history-1',
    severity: 'warning',
    title: 'Disk temperature recovered',
    description: 'Disk sdc returned below the configured temperature threshold.',
    resource: { type: 'disk', id: 'disk-data-2', label: 'Disk sdc' },
    state: 'resolved',
    startedAt: daysAgo(4),
    resolvedAt: daysAgo(3),
  },
]

export const activity: ActivityEvent[] = [
  {
    id: 'act-1',
    timestamp: minutesAgo(26),
    category: 'security',
    title: 'Admin signed in',
    description: 'Session started from 192.168.1.22 (Firefox, LAN)',
  },
  {
    id: 'act-2',
    timestamp: hoursAgo(1),
    category: 'config',
    title: 'Configuration committed',
    description: 'Generation 1842 — share “Documents” access updated',
  },
  {
    id: 'act-3',
    timestamp: hoursAgo(2),
    category: 'backup',
    title: 'App backup verified',
    description: 'Immich appdata · 2.1 GB · checksum verified',
    resource: { type: 'docker', id: 'stack-immich', label: 'Immich' },
  },
  {
    id: 'act-4',
    timestamp: hoursAgo(3),
    category: 'docker',
    title: 'Stack deployed',
    description: 'Jellyfin updated to 10.10.6',
    resource: { type: 'docker', id: 'stack-jellyfin', label: 'Jellyfin' },
  },
  {
    id: 'act-5',
    timestamp: hoursAgo(5),
    category: 'storage',
    title: 'SnapRAID sync completed',
    description: '8.3 GB synced · parity healthy',
  },
  {
    id: 'act-6',
    timestamp: daysAgo(2),
    category: 'update',
    title: 'System update installed',
    description: 'Package baseline refreshed',
  },
  {
    id: 'act-7',
    timestamp: daysAgo(3),
    category: 'storage',
    title: 'Scrub completed',
    description: '2% of blocks scrubbed · 0 errors',
  },
]

export const server: ServerInfo = {
  id: 'srv-1',
  name: 'lumo-one',
  hostname: 'lumo-one',
  version: '0.1.0-dev',
  nasUuid: 'f81d4fae-7dec-4d30-a345-2b6601c0e6f4',
  timezone: 'Europe/Warsaw',
  health: 'attention',
  ip: '192.168.1.74',
}

export const dockerSummary = {
  stacks: 7,
  appsRunning: 13,
  updatesAvailable: 3,
}

export const runtime = {
  jobCounter: 100,
  eventCounter: 0,
  activityCounter: 100,
  stackCounter: 100,
  uptimeStartedAt: now - (14 * 86_400 + 3 * 3_600) * 1000,
  metrics: {
    cpuPercent: 9,
    load: [0.42, 0.38, 0.31] as [number, number, number],
    ramUsedBytes: 6.2 * GB,
    ramTotalBytes: 32 * GB,
    cpuTempC: 47,
    net: { interface: 'eth0', upMbps: 2.1, downMbps: 18.4 },
    filesystems: [
      { path: '/var/lib/lumonas', totalBytes: 500 * GB, usedBytes: 214 * GB, availableBytes: 286 * GB, usedPercent: 42.8, state: 'healthy' as const },
      { path: '/srv/pools', totalBytes: 24 * TB, usedBytes: 18.2 * TB, availableBytes: 5.8 * TB, usedPercent: 75.8, state: 'healthy' as const },
    ],
    disk: { readMbps: 34, writeMbps: 12 },
  },
}

export function pushActivity(event: Omit<ActivityEvent, 'id' | 'timestamp'>) {
  activity.unshift({
    id: `act-${++runtime.activityCounter}`,
    timestamp: new Date().toISOString(),
    ...event,
  })
  if (activity.length > 40) activity.length = 40
}

export function findDisk(id: string): Disk | undefined {
  return disks.find((d) => d.id === id)
}
