import { HardDrive, Plus, Trash2 } from 'lucide-react'
import { useCreateStorageSnapshot, useDeleteStorageSnapshot, useDisks, usePools, useProtection, useStorageSafety, useStorageSnapshots, useUnlockStorageSafety } from '@/api/queries'
import { Metric } from '@/components/core/metric'
import { StorageUsage } from '@/components/core/storage-usage'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { formatBytes } from '@/lib/format'
import { ROLE_LABELS } from '@/features/storage/roles'

export function OverviewTab({ onBrowseDisks }: { onBrowseDisks: () => void }) {
  const { data: disks } = useDisks()
  const { data: pools } = usePools()
  const { data: protection } = useProtection()
  const { data: snapshots } = useStorageSnapshots()
  const { data: storageSafety } = useStorageSafety()
  const unlockStorageSafety = useUnlockStorageSafety()
  const createSnapshot = useCreateStorageSnapshot()
  const deleteSnapshot = useDeleteStorageSnapshot()
  const pool = pools?.[0]
  const parity = disks?.find((d) => d.role === 'parity')
  const system = disks?.find((d) => d.role === 'system')
  const apps = disks?.find((d) => d.role === 'apps')

  const handleDeleteSnapshot = async (id: string) => {
    if (storageSafety?.state !== 'unlocked') {
      await unlockStorageSafety.mutateAsync()
    }
    deleteSnapshot.mutate(id)
  }

  return (
    <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
      <Card>
        <CardHeader className="pb-4">
          <CardTitle className="text-sm font-medium text-muted-foreground">Capacity</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {pool && <StorageUsage usedBytes={pool.usedBytes} totalBytes={pool.sizeBytes} label={`${pool.name} pool`} />}
          <div className="flex flex-col gap-3">
            {parity && (
              <UsageRow label={`${ROLE_LABELS[parity.role]} · ${parity.model}`} used={parity.usedBytes ?? 0} total={parity.sizeBytes} />
            )}
            {system && (
              <UsageRow label={`${ROLE_LABELS[system.role]} · ${system.model}`} used={system.usedBytes ?? 0} total={system.sizeBytes} />
            )}
            {apps && (
              <UsageRow label={`${ROLE_LABELS[apps.role]} · ${apps.model}`} used={apps.usedBytes ?? 0} total={apps.sizeBytes} />
            )}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-4">
          <CardTitle className="text-sm font-medium text-muted-foreground">Disk health</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="grid grid-cols-3 gap-4">
            <Metric label="Disks" value={disks?.length ?? '—'} />
            <Metric label="Healthy" value={disks?.filter((d) => d.health === 'healthy').length ?? '—'} />
            <Metric
              label="Needs care"
              value={
                disks?.filter((d) => ['warning', 'critical', 'attention'].includes(d.health))
                  .length ?? '—'
              }
            />
          </div>
          {disks
            ?.filter((d) => d.health !== 'healthy')
            .map((disk) => (
              <div key={disk.id} className="flex items-center justify-between gap-2 rounded-lg border border-warning/30 bg-warning/5 px-3 py-2">
                <span className="min-w-0 truncate text-sm">
                  <span className="font-mono text-xs">{disk.name}</span>
                  <span className="mx-2 text-muted-foreground">·</span>
                  {disk.model}
                </span>
                <span className="shrink-0 text-xs text-warning">
                  {disk.smart.pendingSectors > 0
                    ? `${disk.smart.pendingSectors} pending sectors`
                    : disk.health === 'offline'
                      ? 'not connected'
                      : disk.health}
                </span>
              </div>
            ))}
          <button
            type="button"
            onClick={onBrowseDisks}
            className="flex items-center gap-2 self-start text-sm font-medium text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <HardDrive className="size-4" />
            Browse all disks
          </button>
        </CardContent>
      </Card>

      {protection && (
        <Card className="xl:col-span-2">
          <CardHeader className="pb-4">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Protection at a glance
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-4">
              <Metric
                label="Parity"
                value={formatBytes(parity?.sizeBytes ?? 0)}
                sub={`${protection.parityDisks.length} disk · SnapRAID`}
              />
              <Metric
                label="Protected data"
                value={`${protection.protectedDiskIds.length} disks`}
                sub={`${formatBytes(pool?.sizeBytes ?? 0)} usable`}
              />
              <Metric label="Sync schedule" value={protection.syncSchedule} />
              <Metric label="Scrub schedule" value={protection.scrubSchedule} />
            </div>
          </CardContent>
        </Card>
      )}

      <Card className="xl:col-span-2">
        <CardHeader className="flex flex-row items-center justify-between pb-4">
          <CardTitle className="text-sm font-medium text-muted-foreground">Snapshots</CardTitle>
          <Button
            size="sm"
            variant="outline"
            className="h-7 gap-1.5 text-xs"
            disabled={createSnapshot.isPending || !pool}
            onClick={() =>
              createSnapshot.mutate({ kind: 'btrfs', source: pool?.mountPath ?? '' })
            }
          >
            <Plus className="size-3.5" />
            {createSnapshot.isPending ? 'Creating…' : 'Snapshot pool'}
          </Button>
        </CardHeader>
        <CardContent className="flex flex-col gap-2">
          {snapshots && snapshots.length > 0 ? (
            snapshots.map((snapshot) => (
              <div key={snapshot.id} className="flex items-center justify-between gap-3 rounded-lg border px-3 py-2">
                <div className="min-w-0">
                  <p className="flex items-center gap-2 truncate text-sm font-medium">
                    <span className="font-mono text-xs">{snapshot.name}</span>
                    <Badge variant="outline" className="text-[10px] uppercase text-muted-foreground">
                      {snapshot.kind}
                    </Badge>
                  </p>
                  <p className="truncate text-xs text-muted-foreground">
                    {snapshot.source} · {new Date(snapshot.createdAt).toLocaleString()}
                  </p>
                </div>
                <Button
                  size="sm"
                  variant="ghost"
                  className="h-7 shrink-0 gap-1.5 text-xs text-destructive hover:text-destructive"
                  disabled={deleteSnapshot.isPending || unlockStorageSafety.isPending}
                  onClick={() => void handleDeleteSnapshot(snapshot.id)}
                >
                  <Trash2 className="size-3.5" />
                  Delete
                </Button>
              </div>
            ))
          ) : (
            <p className="text-sm text-muted-foreground">
              No snapshots yet. Snapshots are read-only point-in-time copies kept on the source
              filesystem.
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  )
}

function UsageRow({ label, used, total }: { label: string; used: number; total: number }) {
  return (
    <div className="flex items-center gap-3">
      <span className="w-44 shrink-0 truncate text-xs text-muted-foreground">{label}</span>
      <div className="h-1.5 min-w-0 flex-1 overflow-hidden rounded-full bg-secondary">
        <div
          className="h-full rounded-full bg-primary/60"
          style={{ width: `${Math.min(100, (used / total) * 100)}%` }}
        />
      </div>
      <span className="tnum shrink-0 text-xs text-muted-foreground">
        {formatBytes(used)} / {formatBytes(total)}
      </span>
    </div>
  )
}
