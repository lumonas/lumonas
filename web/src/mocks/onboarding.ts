import { disks, pushActivity, server } from '@/mocks/db'
import { createJob } from '@/mocks/handlers-helpers'
import { protection } from '@/mocks/db'
import type { DiskRole, OnboardingDisk, OnboardingState } from '@/api/types'

let completed = false

const CLASSIFICATIONS: Record<string, { classification: OnboardingDisk['classification']; recommendedRole: DiskRole; recommendedLabel: string; dataFound?: boolean }> = {
  'disk-sys': { classification: 'system', recommendedRole: 'system', recommendedLabel: 'System' },
  'disk-apps': { classification: 'blank', recommendedRole: 'apps', recommendedLabel: 'Apps & cache' },
  'disk-parity': { classification: 'suspected-parity', recommendedRole: 'parity', recommendedLabel: 'Parity', dataFound: true },
  'disk-data-1': { classification: 'existing', recommendedRole: 'data', recommendedLabel: 'Import without modifying data', dataFound: true },
  'disk-data-2': { classification: 'blank', recommendedRole: 'data', recommendedLabel: 'Data' },
  'disk-data-3': { classification: 'blank', recommendedRole: 'data', recommendedLabel: 'Data' },
  'disk-data-4': { classification: 'blank', recommendedRole: 'data', recommendedLabel: 'Data' },
  'disk-external': { classification: 'removable', recommendedRole: 'external', recommendedLabel: 'External' },
}

export function onboardingState(): OnboardingState {
  const onboardingDisks: OnboardingDisk[] = disks
    .filter((disk) => disk.health !== 'offline')
    .map((disk) => {
      const meta = CLASSIFICATIONS[disk.id] ?? {
        classification: 'blank' as const,
        recommendedRole: 'data' as DiskRole,
        recommendedLabel: 'Data',
      }
      return {
        id: disk.id,
        model: disk.model,
        serialSuffix: disk.serial.slice(-4),
        sizeBytes: disk.sizeBytes,
        classification: meta.classification,
        filesystem: disk.filesystem,
        dataFound: meta.dataFound ?? false,
        recommendedRole: meta.recommendedRole,
        recommendedLabel: meta.recommendedLabel,
      }
    })
  return {
    // A real backend reports completed onboarding after it happened; the
    // mock honours the same local flag the UI store uses so E2E tests can
    // enter the authenticated shell directly.
    completed: completed || localStorage.getItem('lumonas-onboarded') === '1',
    server: {
      name: server.name,
      hostname: server.hostname,
      timezone: server.timezone,
      ip: server.ip,
      sshEnabled: true,
    },
    hardware: {
      cpu: 'Intel N100 (4 cores)',
      ramBytes: 32_000_000_000,
      diskCount: onboardingDisks.length,
    },
    disks: onboardingDisks,
  }
}

export function completeOnboarding(input: {
  serverName: string
  roles: Record<string, DiskRole>
  protection: { syncTime: string; scrubDay: string }
  recovery: { autoConfigBackup: boolean; destination: string; keyAcknowledged: boolean }
}) {
  completed = true
  server.name = input.serverName || server.name
  server.hostname = input.serverName.toLowerCase() || server.hostname
  pushActivity({
    category: 'config',
    title: 'First-time setup completed',
    description: `Storage roles assigned · parity schedule ${input.protection.syncTime} · config backup ${input.recovery.autoConfigBackup ? 'enabled' : 'skipped'}`,
  })
  let initialSyncStarted = false
  if (!protection.syncRunning) {
    createJob('snapraid.sync', 'Initial parity sync')
    protection.syncRunning = true
    initialSyncStarted = true
  }
  return { ok: true, initialSyncStarted }
}
