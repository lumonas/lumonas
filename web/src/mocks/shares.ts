import type {
  AccessLevel,
  FileUser,
  ManagementUser,
  Principal,
  Share,
  UserGroup,
} from '@/api/types'

const TB = 1_000_000_000_000
const now = Date.now()
const hoursAgo = (h: number) => new Date(now - h * 3_600_000).toISOString()
const daysAgo = (d: number) => new Date(now - d * 86_400_000).toISOString()

export const principals: Principal[] = [
  { id: 'p-admins', name: 'Administrators', type: 'group' },
  { id: 'p-family', name: 'family', type: 'group' },
  { id: 'p-kids', name: 'kids', type: 'group' },
  { id: 'p-anna', name: 'anna', type: 'user' },
  { id: 'p-tim', name: 'tim', type: 'user' },
  { id: 'p-bob', name: 'bob', type: 'user' },
  { id: 'p-nextcloud', name: 'nextcloud', type: 'service' },
]

export const shares: Share[] = [
  {
    id: 'share-media',
    name: 'Media',
    path: '/srv/pools/pool-main/Media',
    resourceId: 'pool-main',
    resourceLabel: 'Main pool',
    relativePath: '/Media',
    description: 'Movies, shows and music for the whole family',
    status: 'healthy',
    recycleBin: true,
    protocols: [
      { protocol: 'smb', enabled: true },
      { protocol: 'nfs', enabled: true, hosts: '192.168.1.0/24', readOnly: true },
    ],
    access: [
      { principalId: 'p-admins', level: 'write' },
      { principalId: 'p-family', level: 'write' },
      { principalId: 'p-kids', level: 'read' },
    ],
    usedBytes: 4.1 * TB,
  },
  {
    id: 'share-documents',
    name: 'Documents',
    path: '/srv/pools/pool-main/Documents',
    resourceId: 'pool-main',
    resourceLabel: 'Main pool',
    relativePath: '/Documents',
    description: 'Family paperwork and important files',
    status: 'healthy',
    recycleBin: true,
    protocols: [{ protocol: 'smb', enabled: true }],
    access: [
      { principalId: 'p-admins', level: 'write' },
      { principalId: 'p-anna', level: 'write' },
      { principalId: 'p-kids', level: 'none' },
    ],
    usedBytes: 84_000_000_000,
  },
  {
    id: 'share-photos',
    name: 'Photos',
    path: '/srv/pools/pool-main/Photos',
    resourceId: 'pool-main',
    resourceLabel: 'Main pool',
    relativePath: '/Photos',
    description: 'Also used by Immich for uploads',
    status: 'healthy',
    recycleBin: true,
    protocols: [{ protocol: 'smb', enabled: true }],
    access: [
      { principalId: 'p-admins', level: 'write' },
      { principalId: 'p-family', level: 'write' },
      { principalId: 'p-nextcloud', level: 'write' },
    ],
    usedBytes: 184 * TB / 1000,
  },
  {
    id: 'share-backups',
    name: 'Backups',
    path: '/srv/pools/pool-main/Backups',
    resourceId: 'pool-main',
    resourceLabel: 'Main pool',
    relativePath: '/Backups',
    description: 'PC backups and rsync target',
    status: 'healthy',
    recycleBin: false,
    protocols: [
      { protocol: 'smb', enabled: true },
      { protocol: 'rsync', enabled: true },
    ],
    access: [
      { principalId: 'p-admins', level: 'write' },
      { principalId: 'p-anna', level: 'write' },
    ],
    usedBytes: 1.2 * TB,
  },
  {
    id: 'share-cloud',
    name: 'Cloud',
    path: '/srv/pools/pool-main/Cloud',
    resourceId: 'pool-main',
    resourceLabel: 'Main pool',
    relativePath: '/Cloud',
    description: 'Nextcloud data root',
    status: 'healthy',
    recycleBin: true,
    protocols: [{ protocol: 'smb', enabled: true }],
    access: [
      { principalId: 'p-admins', level: 'write' },
      { principalId: 'p-nextcloud', level: 'write' },
      { principalId: 'p-family', level: 'write' },
    ],
    usedBytes: 312_000_000_000,
  },
  {
    id: 'share-timemachine',
    name: 'TimeMachine',
    path: '/srv/pools/apps/timemachine',
    resourceId: 'apps',
    resourceLabel: 'Apps SSD',
    relativePath: '/timemachine',
    description: 'Mac backups via Time Machine',
    status: 'healthy',
    recycleBin: false,
    protocols: [
      {
        protocol: 'timemachine',
        enabled: true,
        quotaBytes: 2 * TB,
      },
    ],
    access: [
      { principalId: 'p-admins', level: 'write' },
      { principalId: 'p-anna', level: 'write' },
      { principalId: 'p-tim', level: 'write' },
    ],
    usedBytes: 1.1 * TB,
  },
]

export const managementUsers: ManagementUser[] = [
  {
    id: 'mu-admin',
    username: 'admin',
    fullName: 'Dawid',
    role: 'owner',
    twoFactor: true,
    lastLoginAt: hoursAgo(0.5),
    enabled: true,
  },
  {
    id: 'mu-dawid',
    username: 'dawid-op',
    fullName: 'Dawid (operator)',
    role: 'operator',
    twoFactor: false,
    lastLoginAt: daysAgo(3),
    enabled: true,
  },
  {
    id: 'mu-audit',
    username: 'auditor',
    role: 'readonly',
    twoFactor: false,
    enabled: false,
  },
]

export const fileUsers: FileUser[] = [
  {
    id: 'fu-anna',
    username: 'anna',
    fullName: 'Anna',
    type: 'user',
    groups: ['family'],
    enabled: true,
    uid: 1002,
  },
  {
    id: 'fu-tim',
    username: 'tim',
    fullName: 'Tim',
    type: 'user',
    groups: ['family'],
    enabled: true,
    uid: 1003,
  },
  {
    id: 'fu-bob',
    username: 'bob',
    fullName: 'Bob',
    type: 'user',
    groups: ['family', 'kids'],
    enabled: true,
    uid: 1004,
  },
  {
    id: 'fu-nextcloud',
    username: 'nextcloud',
    type: 'service',
    groups: [],
    enabled: true,
    uid: 5000,
  },
]

export const groups: UserGroup[] = [
  { id: 'family', name: 'family', members: ['anna', 'tim', 'bob'] },
  { id: 'kids', name: 'kids', members: ['bob'] },
]

export const shareRuntime = {
  shareCounter: 100,
  userCounter: 100,
}

export function findShare(id: string): Share | undefined {
  return shares.find((s) => s.id === id)
}

export function levelLabel(level: AccessLevel): string {
  if (level === 'write') return 'Read & write'
  if (level === 'read') return 'Read only'
  return 'No access'
}
