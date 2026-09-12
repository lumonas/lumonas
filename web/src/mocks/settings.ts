import type { AppSettings } from '@/api/types'

const now = Date.now()
const minutesAgo = (m: number) => new Date(now - m * 60_000).toISOString()
const hoursAgo = (h: number) => new Date(now - h * 3_600_000).toISOString()

export const settings: AppSettings = {
  updates: {
    core: {
      channel: 'stable',
      current: '0.1.0-dev',
      available: '0.1.1',
      lastCheckedAt: minutesAgo(35),
      autoUpdate: false,
      channelUrl: 'https://updates.example.invalid/lumonas/stable.json',
      releaseNotes: 'Improved recovery verification and safer disk discovery.',
    },
    debian: {
      release: 'Debian 13 (Trixie)',
      pendingCount: 4,
      lastCheckedAt: minutesAgo(35),
      autoUpdate: false,
    },
    docker: { availableCount: 1, autoUpdate: false },
  },
  runtime: {
    writeProfile: 'balanced',
    zram: {
      enabled: true,
      sizeBytes: 8_000_000_000,
      compressedBytes: 1_100_000_000,
      ratio: 3.1,
      pressure: 'low',
    },
    dockerLogging: {
      driver: 'json-file',
      maxSizeMb: 10,
      maxFiles: 3,
      topConsumers: [
        { name: 'jellyfin', sizeBytes: 1_200_000_000 },
        { name: 'immich-machine-learning', sizeBytes: 810_000_000 },
        { name: 'homeassistant', sizeBytes: 640_000_000 },
        { name: 'nextcloud-app', sizeBytes: 300_000_000 },
      ],
    },
  },
  power: {
    maintenanceMode: false,
    wol: [
      { interface: 'eth0', mac: 'a8:a1:59:2c:11:04', supported: true, enabled: true },
      { interface: 'eth1', mac: 'a8:a1:59:2c:11:05', supported: true, enabled: false },
      { interface: 'wlp5s0', mac: 'a8:a1:59:2c:11:06', supported: false, enabled: false },
    ],
    schedule: { enabled: false, action: 'shutdown', time: '01:00', days: 'Daily' },
  },
  security: {
    https: { enabled: true, ca: 'LumoNAS Local CA', acme: false },
    ssh: { rootLogin: false, passwordAuth: false, keyCount: 1 },
    sessions: [
      {
        id: 'sess-1',
        device: 'Firefox on macOS',
        ip: '192.168.1.22',
        scope: 'LAN',
        lastActiveAt: minutesAgo(0),
        current: true,
      },
      {
        id: 'sess-2',
        device: 'Chrome on Android',
        ip: '192.168.1.34',
        scope: 'LAN',
        lastActiveAt: hoursAgo(2),
        current: false,
      },
      {
        id: 'sess-3',
        device: 'Safari on iPad',
        ip: '100.84.12.9',
        scope: 'Tailscale',
        lastActiveAt: hoursAgo(26),
        current: false,
      },
    ],
  },
}

export const ups = {
  model: 'APC Back-UPS ES 700',
  status: 'Online',
  batteryPercent: 100,
  runtimeMinutes: 32,
  lastSelfTest: daysAgo(9),
}

export const updateSlots = {
  activeSlot: 'A',
  previousSlot: undefined as string | undefined,
  pendingSlot: undefined as string | undefined,
  activeVersion: '0.1.0-dev',
  pendingVersion: undefined as string | undefined,
  bootAttempts: 0,
  lastError: undefined as string | undefined,
  updatedAt: daysAgo(2),
}

function daysAgo(d: number) {
  return new Date(now - d * 86_400_000).toISOString()
}
