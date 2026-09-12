import { Activity, Play, RefreshCw, ShieldCheck } from 'lucide-react'
import { useActivity, useAlerts, useCreateJob, useDisks, useJobs, useProtection, usePools } from '@/api/queries'
import { AlertBanner } from '@/components/core/alert-banner'
import { HealthBadge } from '@/components/core/health-badge'
import { Metric } from '@/components/core/metric'
import { StorageUsage } from '@/components/core/storage-usage'
import { TimelineEvent } from '@/components/core/timeline-event'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { formatBytes, formatDateTime, timeAgo } from '@/lib/format'
import { ROLE_LABELS } from '@/features/storage/roles'

export function ProtectionTab() {
  const { data: protection } = useProtection()
  const { data: disks } = useDisks()
  const { data: jobs } = useJobs()
  const createJob = useCreateJob()
  const { data: alerts } = useAlerts()

  if (!protection) return null
  const parityDisk = disks?.find((d) => d.role === 'parity')
  const protectedDisks = (disks ?? []).filter((d) =>
    protection.protectedDiskIds.includes(d.id),
  )
  const syncJob = jobs?.find((j) => j.type === 'snapraid.sync' && j.state === 'running')
  const stale = protection.changesSinceSyncBytes > 50e9

  return (
    <div className="flex flex-col gap-4">
      {stale && !protection.syncRunning && (
        <AlertBanner tone="attention" title="Changes are not yet protected">
          {formatBytes(protection.changesSinceSyncBytes)} changed since the last successful sync.
          Files written after the last sync are not fully represented in parity yet.
        </AlertBanner>
      )}
      {alerts?.some((a) => a.severity === 'critical') && (
        <AlertBanner tone="critical" title="A protected disk is unavailable">
          SnapRAID automation is frozen until the missing disk is replaced or removed.
        </AlertBanner>
      )}

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
        <Card>
          <CardHeader className="flex-row items-center justify-between space-y-0 pb-4">
            <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
              <ShieldCheck className="size-4" />
              Parity status
            </CardTitle>
            <HealthBadge state={protection.status} />
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <div className="grid grid-cols-2 gap-4">
              <Metric
                label="Last sync"
                value={protection.lastSyncAt ? timeAgo(protection.lastSyncAt) : '—'}
                sub={
                  protection.lastSyncAt
                    ? `${protection.lastSyncResult} · ${formatDateTime(protection.lastSyncAt)}`
                    : undefined
                }
              />
              <Metric
                label="Unsynced changes"
                value={formatBytes(protection.changesSinceSyncBytes)}
                sub="since last sync"
              />
              <Metric label="Sync schedule" value={protection.syncSchedule} />
              <Metric
                label="Last scrub"
                value={protection.lastScrubAt ? timeAgo(protection.lastScrubAt) : '—'}
                sub={protection.scrubSchedule}
              />
            </div>
            {syncJob ? (
              <div className="flex items-center gap-2 rounded-lg border bg-accent/30 px-3 py-2.5 text-sm">
                <RefreshCw className="size-4 animate-spin text-primary [animation-duration:2.5s]" />
                <span className="flex-1 truncate">{syncJob.stage}</span>
                <span className="tnum text-xs text-muted-foreground">
                  {Math.floor(syncJob.progress ?? 0)}%
                </span>
              </div>
            ) : (
              <Button
                size="sm"
                onClick={() => createJob.mutate({ type: 'snapraid.sync' })}
                disabled={protection.syncRunning}
              >
                <Play />
                Sync now
              </Button>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-4">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Parity disk
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            {parityDisk ? (
              <>
                <div className="flex items-center justify-between gap-2">
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">{parityDisk.model}</p>
                    <p className="font-mono text-xs text-muted-foreground">
                      serial {parityDisk.serial}
                    </p>
                  </div>
                  <HealthBadge state={parityDisk.health} />
                </div>
                <StorageUsage
                  usedBytes={parityDisk.usedBytes ?? 0}
                  totalBytes={parityDisk.sizeBytes}
                  label="Parity used"
                />
                <p className="text-xs text-muted-foreground">
                  Parity size must accommodate the largest protected data disk.
                </p>
              </>
            ) : (
              <p className="text-sm text-muted-foreground">No parity disk configured.</p>
            )}
            <div className="mt-1">
              <p className="mb-2 text-xs font-medium tracking-wide text-muted-foreground uppercase">
                Protected data disks
              </p>
              <ul className="flex flex-col divide-y rounded-lg border">
                {protectedDisks.map((disk) => (
                  <li key={disk.id} className="flex items-center justify-between gap-2 px-3 py-2">
                    <span className="min-w-0 truncate text-sm">
                      <span className="font-mono text-xs">{disk.name}</span>
                      <span className="mx-2 text-muted-foreground">·</span>
                      {disk.model}
                    </span>
                    <HealthBadge state={disk.health} />
                  </li>
                ))}
              </ul>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

export function ProtectionSummary() {
  const { data: protection } = useProtection()
  const createJob = useCreateJob()
  if (!protection) return null
  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between space-y-0 pb-4">
        <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
          <ShieldCheck className="size-4" />
          Protection
        </CardTitle>
        <HealthBadge state={protection.status} />
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="grid grid-cols-2 gap-4">
          <Metric
            label="Last sync"
            value={protection.lastSyncAt ? timeAgo(protection.lastSyncAt) : '—'}
            sub={protection.syncSchedule}
          />
          <Metric
            label="Unsynced"
            value={formatBytes(protection.changesSinceSyncBytes)}
            sub="since last sync"
          />
        </div>
        <p className="text-xs text-muted-foreground">
          Parity is scheduled, not realtime — changes since the last sync are not yet protected.
        </p>
        <Button
          size="sm"
          variant="outline"
          disabled={protection.syncRunning}
          onClick={() => createJob.mutate({ type: 'snapraid.sync' })}
        >
          <RefreshCw />
          {protection.syncRunning ? 'Sync running…' : 'Sync now'}
        </Button>
      </CardContent>
    </Card>
  )
}

export function StorageActivityTab() {
  const { data: activity } = useActivity()
  const storageEvents = (activity ?? []).filter((e) => e.category === 'storage' || e.category === 'backup')
  return (
    <Card>
      <CardHeader className="pb-4">
        <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
          <Activity className="size-4" />
          Storage events
        </CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {storageEvents.length === 0 ? (
          <p className="py-4 text-center text-sm text-muted-foreground">No storage events yet.</p>
        ) : (
          storageEvents.map((event) => <TimelineEvent key={event.id} event={event} />)
        )}
      </CardContent>
    </Card>
  )
}

export function PoolsTab() {
  const { data: pools } = usePools()
  const { data: disks } = useDisks()

  if (!pools) return null

  return (
    <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
      {pools.map((pool) => {
        const members = pool.members
          .map((m) => disks?.find((d) => d.id === m.diskId))
          .filter((d): d is NonNullable<typeof d> => d != null)
        return (
          <Card key={pool.id}>
            <CardHeader className="flex-row items-center justify-between space-y-0 pb-4">
              <div>
                <CardTitle className="text-base">{pool.name}</CardTitle>
                <p className="mt-0.5 font-mono text-xs text-muted-foreground">{pool.mountPath}</p>
              </div>
              <div className="flex items-center gap-2">
                <span className="text-xs text-muted-foreground">mergerfs</span>
                <HealthBadge state={pool.status} />
              </div>
            </CardHeader>
            <CardContent className="flex flex-col gap-4">
              <StorageUsage usedBytes={pool.usedBytes} totalBytes={pool.sizeBytes} label="Used" />
              <div>
                <p className="mb-2 text-xs font-medium tracking-wide text-muted-foreground uppercase">
                  Member disks
                </p>
                <ul className="flex flex-col divide-y rounded-lg border">
                  {members.map((disk) => (
                    <li key={disk.id} className="flex items-center justify-between gap-2 px-3 py-2">
                      <span className="min-w-0 truncate text-sm">
                        <span className="font-mono text-xs">{disk.name}</span>
                        <span className="mx-2 text-muted-foreground">·</span>
                        {disk.model}
                        <span className="ml-2 text-xs text-muted-foreground">
                          {ROLE_LABELS[disk.role]}
                        </span>
                      </span>
                      <span className="tnum shrink-0 text-xs text-muted-foreground">
                        {formatBytes(disk.usedBytes ?? 0)} / {formatBytes(disk.sizeBytes)}
                      </span>
                    </li>
                  ))}
                </ul>
              </div>
              <p className="text-xs text-muted-foreground">
                Files are merged across member disks. A disk failure only affects the files stored
                on that disk; parity protects them.
              </p>
            </CardContent>
          </Card>
        )
      })}
    </div>
  )
}
