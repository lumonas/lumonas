import type { DiskRole } from '@/api/types'

export const ROLE_LABELS: Record<DiskRole, string> = {
  system: 'System',
  apps: 'Apps & cache',
  data: 'Data',
  parity: 'Parity',
  backup: 'Backup',
  external: 'External',
  unknown: 'Unknown',
}

export const ROLE_ORDER: DiskRole[] = ['system', 'apps', 'parity', 'data', 'backup', 'external', 'unknown']
