import type {
  BackupDestination,
  BackupJob,
  ConfigGeneration,
  RecoveryLayer,
  RecoveryReadiness,
  RestorePlan,
} from '@/api/types'

const now = Date.now()
const hoursAgo = (h: number) => new Date(now - h * 3_600_000).toISOString()
const daysAgo = (d: number) => new Date(now - d * 86_400_000).toISOString()

export const readinessLayers: RecoveryLayer[] = [
  { id: 'config', label: 'Configuration', status: 'current', detail: 'Generation 1843 · verified' },
  { id: 'storage', label: 'Storage metadata', status: 'current', detail: 'All 7 disks recorded' },
  { id: 'stacks', label: 'Docker stacks', status: 'current', detail: '7 of 7 stacks have config backups' },
  { id: 'appdata', label: 'Docker appdata', status: 'stale', detail: 'pihole has never been backed up' },
  { id: 'remote', label: 'Offsite backup', status: 'stale', detail: 'Last remote copy is 9 days old' },
  { id: 'key', label: 'Recovery key', status: 'current', detail: 'Configured · stored offline' },
]

const WEIGHTS: Record<RecoveryLayer['status'], number> = { current: 1, stale: 0.6, missing: 0 }

export const readiness: RecoveryReadiness = {
  score: Math.round(
    (readinessLayers.reduce((sum, layer) => sum + WEIGHTS[layer.status], 0) /
      readinessLayers.length) *
      100,
  ),
  layers: readinessLayers,
  coverage: [
    { id: 'share-documents', name: 'Documents', kind: 'share', status: 'current', detail: 'Included in the latest verified bundle', lastSuccessfulAt: hoursAgo(2) },
    { id: 'share-media', name: 'Media', kind: 'share', status: 'stale', detail: 'Included in a verified bundle, but the latest copy is overdue', lastSuccessfulAt: daysAgo(9) },
    { id: 'docker-appdata', name: 'Docker app data', kind: 'docker-appdata', status: 'stale', detail: 'pihole has never been backed up', lastSuccessfulAt: daysAgo(9) },
  ],
}
export const backupJobs: BackupJob[] = [
  {
    id: 'bj-apps',
    name: 'App backups (all stacks)',
    source: 'Apps SSD / appdata',
    destinationId: 'dest-usb',
    schedule: 'Daily at 03:30',
    strategy: 'Stop stack, then back up',
    jobType: 'backup.app',
    lastRun: { status: 'healthy', at: hoursAgo(11), detail: '12 apps · 14.6 GB · verified' },
    enabled: true,
  },
  {
    id: 'bj-config',
    name: 'Configuration snapshots',
    source: 'System config',
    destinationId: 'dest-usb',
    schedule: 'After every change + daily',
    strategy: 'Snapshot, verify, replicate to 2 disks',
    jobType: 'backup.config',
    lastRun: { status: 'healthy', at: hoursAgo(2), detail: 'Generation 1843 · verified' },
    enabled: true,
  },
  {
    id: 'bj-photos',
    name: 'Photos offsite sync',
    source: 'Media /Photos',
    destinationId: 'dest-s3',
    schedule: 'Nightly at 04:00',
    strategy: 'Encrypted, incremental',
    jobType: 'backup.sync',
    lastRun: { status: 'warning', at: daysAgo(9), detail: 'Failed — S3 quota reached' },
    enabled: true,
  },
  {
    id: 'bj-timemachine',
    name: 'Time Machine quota check',
    source: 'Apps SSD / timemachine',
    destinationId: 'dest-usb',
    schedule: 'Weekly on Saturdays',
    strategy: 'Verify sparsebundle integrity',
    jobType: 'backup.config',
    lastRun: { status: 'healthy', at: daysAgo(5), detail: '1.1 TB of 2 TB quota used' },
    enabled: false,
  },
]

export const destinations: BackupDestination[] = [
  {
    id: 'dest-usb',
    enabled: true,
    type: 'usb',
    label: 'Offline USB disk',
    target: 'Seagate Expansion 5TB',
    encrypted: true,
    status: 'healthy',
    lastVerifiedAt: hoursAgo(11),
    detail: 'Primary copy · kept disconnected between runs',
  },
  {
    id: 'dest-s3',
    enabled: true,
    type: 's3',
    label: 'Backblaze B2',
    target: 's3://us-west-004/backblaze.com/lumo-offsite',
    encrypted: true,
    status: 'attention',
    lastVerifiedAt: daysAgo(9),
    detail: 'Offsite copy · sync failing — check quota',
  },
  {
    id: 'dest-sftp',
    enabled: false,
    type: 'sftp',
    label: 'SFTP (friends NAS)',
    encrypted: true,
    status: 'offline',
    detail: 'Not configured yet — recommended for a third copy',
  },
]

export const generations: ConfigGeneration[] = [
  {
    id: 1843,
    createdAt: hoursAgo(2),
    actor: 'admin',
    status: 'committed',
    summary: 'Share “Projects” created on Main pool',
    config: `shares:
  - name: Projects
    resource: pool-main
    path: /Projects
    recycleBin: true
    protocols: [smb]
    access:
      administrators: read-write
      family: read-write`,
  },
  {
    id: 1842,
    createdAt: daysAgo(1),
    actor: 'admin',
    status: 'committed',
    summary: 'Jellyfin published port changed to 8096',
    config: `shares:
  - name: Media
    resource: pool-main
    path: /Media
    protocols: [smb, nfs]
docker:
  ports:
    jellyfin: 8096`,
  },
  {
    id: 1841,
    createdAt: daysAgo(3),
    actor: 'admin',
    status: 'committed',
    summary: 'DNS servers set to 192.168.1.53 (Pi-hole)',
    config: `network:
  dns:
    - 192.168.1.53
  interface: eth0
docker:
  ports:
    jellyfin: 8096`,
  },
  {
    id: 1840,
    createdAt: daysAgo(4),
    actor: 'system',
    status: 'committed',
    summary: 'Stack pihole deployed from catalog',
    config: `network:
  dns:
    - 1.1.1.1
  interface: eth0
docker:
  stacks:
    - pihole
  ports:
    jellyfin: 8096`,
  },
  {
    id: 1839,
    createdAt: daysAgo(5),
    actor: 'admin',
    status: 'failed',
    summary: 'Attempted NFS export with invalid host',
    config: `network:
  dns:
    - 1.1.1.1`,
  },
]

export const restorePlan: RestorePlan = {
  generationId: 1843,
  interfaces: [
    { old: 'Intel I219-V (eth0)', detail: 'Main LAN — was 192.168.1.74 static', options: ['enp3s0', 'enp4s0', 'wlp5s0'] },
    { old: 'Realtek RTL8125 (eth1)', detail: 'Second NIC — was DHCP', options: ['enp3s0', 'enp4s0', 'wlp5s0'] },
  ],
  apps: [
    { name: 'jellyfin', appdataAvailable: true },
    { name: 'immich', appdataAvailable: true },
    { name: 'homeassistant', appdataAvailable: true },
    { name: 'radarr', appdataAvailable: true },
    { name: 'nextcloud', appdataAvailable: true },
    { name: 'pihole', appdataAvailable: false },
  ],
  dataDisksNote:
    'Data disks are imported read-only until every disk identity has been verified against the recovery metadata.',
}
