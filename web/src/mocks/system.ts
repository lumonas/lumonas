import type { AlertRule, NotificationChannel, ServiceStatus } from '@/api/types'

export const services: ServiceStatus[] = [
  { id: 'svc-samba', name: 'SMB (Samba)', state: 'running', detail: '4 shares · 2 active sessions' },
  { id: 'svc-nfs', name: 'NFS', state: 'running', detail: '2 exports' },
  { id: 'svc-ssh', name: 'SSH', state: 'running', detail: 'Key-only authentication' },
  { id: 'svc-docker', name: 'Docker Engine', state: 'running', detail: '13 containers' },
  { id: 'svc-snapraid', name: 'SnapRAID', state: 'running', detail: 'Idle · next sync 02:00' },
  { id: 'svc-ftps', name: 'FTPS', state: 'stopped', detail: 'Disabled in share settings' },
  { id: 'svc-avahi', name: 'mDNS (Avahi)', state: 'degraded', detail: 'Time Machine advertisement failed to register' },
]

export const alertRules: AlertRule[] = [
  {
    id: 'rule-temp',
    name: 'Disk temperature high',
    condition: 'temperature > 45°C for 5 minutes',
    severity: 'warning',
    routes: ['web', 'telegram'],
    enabled: true,
    lastTriggeredAt: new Date(Date.now() - 3 * 86_400_000).toISOString(),
  },
  {
    id: 'rule-smart',
    name: 'SMART attribute changed',
    condition: 'pending/reallocated sectors increase',
    severity: 'warning',
    routes: ['web', 'telegram', 'email'],
    enabled: true,
    lastTriggeredAt: new Date(Date.now() - 2 * 86_400_000).toISOString(),
  },
  {
    id: 'rule-pool',
    name: 'Pool degraded or disk missing',
    condition: 'pool member offline',
    severity: 'critical',
    routes: ['web', 'telegram', 'email'],
    enabled: true,
  },
  {
    id: 'rule-sync',
    name: 'SnapRAID sync stale',
    condition: 'no successful sync in 48h',
    severity: 'attention',
    routes: ['web'],
    enabled: true,
  },
  {
    id: 'rule-backup',
    name: 'Backup job failed',
    condition: 'backup job state = failed',
    severity: 'warning',
    routes: ['web', 'telegram'],
    enabled: true,
  },
  {
    id: 'rule-container',
    name: 'Container unhealthy',
    condition: 'health check failing for 2 minutes',
    severity: 'warning',
    routes: ['web'],
    enabled: true,
    lastTriggeredAt: new Date(Date.now() - 6 * 3_600_000).toISOString(),
  },
  {
    id: 'rule-login',
    name: 'New admin sign-in',
    condition: 'login from unseen device',
    severity: 'info',
    routes: ['web'],
    enabled: false,
  },
]

export const notificationChannels: NotificationChannel[] = [
  { id: 'ch-web', type: 'web', label: 'Web UI', configured: true, enabled: true },
  { id: 'ch-telegram', type: 'telegram', label: 'Telegram', target: '@lumo_alerts', configured: true, enabled: true },
  { id: 'ch-email', type: 'email', label: 'Email (SMTP)', target: 'dawid@example.com', configured: true, enabled: false },
  { id: 'ch-ntfy', type: 'ntfy', label: 'ntfy', target: 'ntfy.sh/lumo-private', configured: true, enabled: true },
  { id: 'ch-discord', type: 'discord', label: 'Discord webhook', configured: false, enabled: false },
]

export const scheduledJobs = [
  { id: 'sched-sync', name: 'SnapRAID sync', schedule: 'Daily at 02:00', next: 'in 9h', enabled: true },
  { id: 'sched-scrub', name: 'SnapRAID scrub', schedule: 'Sundays at 03:00', next: 'in 3d', enabled: true },
  { id: 'sched-smart', name: 'SMART short tests', schedule: 'Saturdays at 04:00', next: 'in 2d', enabled: true },
  { id: 'sched-backup', name: 'App backups', schedule: 'Daily at 03:30', next: 'in 11h', enabled: true },
  { id: 'sched-config', name: 'Config snapshot', schedule: 'After every change', next: 'on change', enabled: true },
]
