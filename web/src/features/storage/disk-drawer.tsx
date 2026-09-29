import { useState } from 'react'
import { Activity, HardDriveDownload, HardDriveUpload, Play, Wrench } from 'lucide-react'
import { useActivity, useCreateJob, useDisk, useJobs, useSMARTHistory, useStorageSafety, useUnlockEncryptedDisk, useUnlockStorageSafety } from '@/api/queries'
import { DangerZone } from '@/components/core/danger-zone'
import { DependencyList, type DependencyItem } from '@/components/core/dependency-list'
import { DiskIdentity } from '@/components/core/disk-identity'
import { EmptyState } from '@/components/core/empty-state'
import { HealthBadge } from '@/components/core/health-badge'
import { Metric } from '@/components/core/metric'
import { ResourceDrawer } from '@/components/core/resource-drawer'
import { StorageUsage } from '@/components/core/storage-usage'
import { Sparkline } from '@/components/core/sparkline'
import { TimelineEvent } from '@/components/core/timeline-event'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { formatBytes, timeAgo } from '@/lib/format'
import { DiskOperationsDialog, type DiskAction } from '@/features/storage/disk-operations-dialog'
import { DiskReplacementDialog } from '@/features/storage/disk-replacement-dialog'
import { ROLE_LABELS } from '@/features/storage/roles'
import { cn } from '@/lib/utils'

const RAW_SMART = `ID# ATTRIBUTE_NAME          FLAG VALUE WORST THRESH TYPE    UPDATED WHEN_FAILED RAW
  1 Raw_Read_Error_Rate     0x000f  117  099  006  Pre-fail Always       -       41_298_771
  5 Reallocated_Sector_Ct   0x0033  100  100  010  Pre-fail Always       -       0
194 Temperature_Celsius     0x0022  070  057  000  Old_age  Always       -       35
197 Current_Pending_Sector  0x0012  100  100  000  Old_age  Always       -       4
198 Offline_Uncorrectable   0x0010  100  100  000  Old_age  Offline      -       0`

function dependencies(diskRole: string, poolName: string | undefined): DependencyItem[] {
  if (diskRole === 'data') {
    return [
      { id: 'pool', label: `${poolName ?? 'Main'} pool`, sublabel: '/srv/pools/main' },
      { id: 'share-media', label: 'Share “Media”', sublabel: 'SMB · NFS' },
      { id: 'share-backups', label: 'Share “Backups”', sublabel: 'SMB · rsync' },
      { id: 'share-photos', label: 'Share “Photos”', sublabel: 'SMB' },
    ]
  }
  if (diskRole === 'apps') {
    return [
      { id: 'docker', label: 'Docker data-root', sublabel: '/var/lib/docker' },
      { id: 'appdata', label: 'App data (appdata)', sublabel: '13 running apps' },
    ]
  }
  if (diskRole === 'parity') {
    return [{ id: 'array', label: 'SnapRAID array', sublabel: 'parity for 4 data disks' }]
  }
  return []
}

export function DiskDrawer({
  diskId,
  onOpenChange,
}: {
  diskId: string | null
  onOpenChange: (open: boolean) => void
}) {
  const { data: disk } = useDisk(diskId)
  const { data: smartHistory } = useSMARTHistory(diskId)
  const { data: jobs } = useJobs()
  const { data: activity } = useActivity()
  const { data: storageSafety } = useStorageSafety()
  const unlockEncryptedDisk = useUnlockEncryptedDisk()
  const unlockSafety = useUnlockStorageSafety()
  const createJob = useCreateJob()
  const [standby, setStandby] = useState('30')
  const [smartSchedule, setSmartSchedule] = useState('weekly-short')
  const [tempAlerts, setTempAlerts] = useState(true)
  const [diskAction, setDiskAction] = useState<DiskAction | null>(null)
  const [replacementOpen, setReplacementOpen] = useState(false)
  const [encryptionPassphrase, setEncryptionPassphrase] = useState('')

  const smartRunning =
    disk != null &&
    jobs?.some(
      (j) =>
        j.resourceId === disk.id &&
        j.type.startsWith('smart.') &&
        ['queued', 'preparing', 'running'].includes(j.state),
    )
  const diskEvents = (activity ?? []).filter((e) => e.resource?.id === disk?.id)

  return (
    <ResourceDrawer
      open={diskId != null && disk != null}
      onOpenChange={onOpenChange}
      title={disk ? `${disk.model}` : ''}
      description={
        disk ? (
          <span className="tnum font-mono text-xs">
            {disk.name} · serial {disk.serial} · {formatBytes(disk.sizeBytes)}
          </span>
        ) : null
      }
    >
      {disk && (
        <>
          <DiskIdentity disk={disk} />

          <Tabs defaultValue="overview">
            <TabsList className="w-full justify-start overflow-x-auto">
              <TabsTrigger value="overview">Overview</TabsTrigger>
              <TabsTrigger value="smart">SMART</TabsTrigger>
              <TabsTrigger value="usage">Usage</TabsTrigger>
              <TabsTrigger value="activity">Activity</TabsTrigger>
              <TabsTrigger value="settings">Settings</TabsTrigger>
            </TabsList>

            <TabsContent value="overview">
              <div className="grid grid-cols-2 gap-4 sm:grid-cols-3">
                <Metric label="Role" value={ROLE_LABELS[disk.role]} />
                <Metric label="Temperature" value={disk.temperatureC != null ? `${disk.temperatureC}°C` : '—'} />
                <Metric label="Power-on" value={`${disk.smart.powerOnHours.toLocaleString()} h`} />
                <Metric label="Filesystem" value={disk.filesystem ?? '—'} />
                <Metric label="Interface" value={disk.interface.toUpperCase()} />
                <Metric label="Last seen" value={timeAgo(disk.lastSeen)} />
              </div>
              {disk.filesystem === 'crypto_LUKS' ? (
                <div className="grid gap-3 rounded-lg border p-3">
                  <div>
                    <p className="text-sm font-medium">LUKS encrypted volume</p>
                    <p className="text-xs text-muted-foreground">Unlock after reboot to mount this disk. LumoNAS does not save the passphrase.</p>
                  </div>
                  {storageSafety?.state !== 'unlocked' ? (
                    <Button size="sm" variant="outline" onClick={() => unlockSafety.mutate()} disabled={unlockSafety.isPending}>
                      {unlockSafety.isPending ? 'Unlocking storage safety…' : 'Unlock storage safety (15 min)'}
                    </Button>
                  ) : null}
                  <div className="grid gap-2">
                    <Label htmlFor="encrypted-disk-passphrase">Passphrase</Label>
                    <Input id="encrypted-disk-passphrase" type="password" autoComplete="current-password" value={encryptionPassphrase} onChange={(event) => setEncryptionPassphrase(event.target.value)} />
                    <Button size="sm" disabled={!encryptionPassphrase || storageSafety?.state !== 'unlocked' || unlockEncryptedDisk.isPending} onClick={() => unlockEncryptedDisk.mutate({ diskId: disk.id, passphrase: encryptionPassphrase }, { onSettled: () => setEncryptionPassphrase('') })}>
                      {unlockEncryptedDisk.isPending ? 'Unlocking…' : 'Unlock and mount'}
                    </Button>
                    {unlockEncryptedDisk.isError ? <p role="alert" className="text-sm text-destructive">{unlockEncryptedDisk.error instanceof Error ? unlockEncryptedDisk.error.message : 'The encrypted disk could not be unlocked.'}</p> : null}
                  </div>
                </div>
              ) : null}
              {disk.filesystem && disk.filesystem !== 'crypto_LUKS' && !disk.poolId ? (
                <div className="flex flex-wrap gap-2">
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={disk.mounted}
                    onClick={() => setDiskAction('mount')}
                  >
                    <HardDriveDownload />
                    {disk.mounted ? 'Mounted' : 'Mount'}
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={!disk.mounted}
                    onClick={() => setDiskAction('unmount')}
                  >
                    <HardDriveUpload />
                    Unmount
                  </Button>
                </div>
              ) : null}
              {disk.role === 'data' && (
                <Button size="sm" variant="outline" onClick={() => setReplacementOpen(true)}>
                  <Wrench /> Replace with parity recovery
                </Button>
              )}
              <div>
                <p className="mb-2 text-xs font-medium tracking-wide text-muted-foreground uppercase">
                  Used by
                </p>
                <DependencyList
                  items={dependencies(disk.role, 'Main')}
                  emptyLabel="No resources depend on this disk."
                />
              </div>
            </TabsContent>

            <TabsContent value="smart">
              <div className="rounded-lg border p-3">
                <div className="flex items-center justify-between gap-3">
                  <div>
                    <p className="text-sm font-medium">SMART trend</p>
                    <p className="text-xs text-muted-foreground">{smartHistory?.trend.summary ?? 'Collecting history…'}</p>
                  </div>
                  {smartHistory ? <HealthBadge state={smartHistory.trend.status} /> : null}
                </div>
                {smartHistory?.samples.length ? (
                  <>
                    <Sparkline
                      data={smartHistory.samples.slice().reverse().map((sample) => sample.summary.pendingSectors + sample.summary.uncorrectableSectors + sample.summary.reallocatedSectors)}
                      className="mt-3 h-12"
                    />
                    <div className="mt-2 grid grid-cols-2 gap-2 text-xs text-muted-foreground sm:grid-cols-4">
                      <span>{smartHistory.trend.sampleCount} samples</span>
                      <span>Reallocated {smartHistory.trend.reallocatedSlope.toFixed(2)}/day</span>
                      <span>Pending {smartHistory.trend.pendingSlope.toFixed(2)}/day</span>
                      <span>CRC {smartHistory.trend.crcSlope.toFixed(2)}/day</span>
                    </div>
                  </>
                ) : <p className="mt-3 text-xs text-muted-foreground">Trend appears after the first two samples.</p>}
              </div>
              <div className="flex flex-col divide-y rounded-lg border">
                <SmartRow label="Overall">
                  <HealthBadge state={disk.smart.overall} />
                </SmartRow>
                <SmartRow label="Reallocated sectors" value={disk.smart.reallocatedSectors} />
                <SmartRow label="Current pending sectors" value={disk.smart.pendingSectors} />
                <SmartRow label="Offline uncorrectable" value={disk.smart.uncorrectableSectors} />
                <SmartRow label="CRC errors" value={disk.smart.crcErrors} />
                <SmartRow label="Power-on hours" text={`${disk.smart.powerOnHours.toLocaleString()} h`} />
                {disk.smart.wearPercent != null && (
                  <SmartRow label="SSD wear used" text={`${disk.smart.wearPercent}%`} />
                )}
                <SmartRow
                  label="Last self-test"
                  text={
                    disk.smart.lastTest
                      ? `${disk.smart.lastTest.type} · ${disk.smart.lastTest.result} · ${timeAgo(disk.smart.lastTest.at)}`
                      : 'Never'
                  }
                />
              </div>
              <Button
                size="sm"
                variant="outline"
                disabled={smartRunning}
                onClick={() => createJob.mutate({ type: 'smart.short', resourceId: disk.id })}
              >
                <Play />
                {smartRunning ? 'Test running…' : 'Run short self-test'}
              </Button>
              <details className="rounded-lg border">
                <summary className="cursor-pointer px-3 py-2.5 text-sm text-muted-foreground select-none">
                  Raw SMART attributes
                </summary>
                <pre className="overflow-x-auto border-t px-3 py-3 font-mono text-xs leading-relaxed text-muted-foreground">
                  {RAW_SMART}
                </pre>
              </details>
            </TabsContent>

            <TabsContent value="usage">
              <StorageUsage
                usedBytes={disk.usedBytes ?? 0}
                totalBytes={disk.sizeBytes}
                label="Used"
              />
              <div className="grid grid-cols-2 gap-4 sm:grid-cols-3">
                <Metric label="Free" value={formatBytes(disk.sizeBytes - (disk.usedBytes ?? 0))} />
                <Metric label="Filesystem" value={disk.filesystem ?? '—'} />
                <Metric label="Rotation" value={disk.rotational ? '7200 RPM' : 'SSD'} />
              </div>
              {disk.role === 'parity' && (
                <p className="rounded-lg border bg-muted/40 px-3 py-2.5 text-sm text-muted-foreground">
                  This disk holds parity data for the SnapRAID array. Do not store files here —
                  parity needs free space equal to the largest protected data disk.
                </p>
              )}
            </TabsContent>

            <TabsContent value="activity">
              {diskEvents.length === 0 ? (
                <EmptyState
                  icon={<Activity />}
                  title="No recent events"
                  description="Events for this disk — tests, temperatures, errors — will appear here."
                  className="border-0"
                />
              ) : (
                <div className="flex flex-col gap-3">
                  {diskEvents.map((event) => (
                    <TimelineEvent key={event.id} event={event} />
                  ))}
                </div>
              )}
            </TabsContent>

            <TabsContent value="settings">
              <div className="flex flex-col gap-5">
                <div className="grid gap-2">
                  <Label htmlFor="standby">Standby timer</Label>
                  <Select value={standby} onValueChange={setStandby}>
                    <SelectTrigger id="standby" className="w-52">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="off">Never sleep</SelectItem>
                      <SelectItem value="15">After 15 min</SelectItem>
                      <SelectItem value="30">After 30 min</SelectItem>
                      <SelectItem value="60">After 60 min</SelectItem>
                      <SelectItem value="120">After 2 h</SelectItem>
                    </SelectContent>
                  </Select>
                  <p className="text-xs text-muted-foreground">
                    Frequent spin-up/spin-down cycles can wear disks faster.
                  </p>
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="smart-schedule">SMART tests</Label>
                  <Select value={smartSchedule} onValueChange={setSmartSchedule}>
                    <SelectTrigger id="smart-schedule" className="w-52">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="off">Off</SelectItem>
                      <SelectItem value="weekly-short">Short test weekly</SelectItem>
                      <SelectItem value="monthly-extended">Extended test monthly</SelectItem>
                      <SelectItem value="both">Short weekly + extended monthly</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="flex items-center justify-between gap-4">
                  <div>
                    <Label htmlFor="temp-alerts">Temperature alerts</Label>
                    <p className="text-xs text-muted-foreground">Notify above 45°C.</p>
                  </div>
                  <Switch id="temp-alerts" checked={tempAlerts} onCheckedChange={setTempAlerts} />
                </div>
              </div>
            </TabsContent>
          </Tabs>

          <DangerZone
            identity={<DiskIdentity disk={disk} />}
            onAction={(id) => setDiskAction(id as DiskAction)}
            actions={[
              ...(disk.poolId
                ? []
                : [
                    {
                      id: 'format',
                      label: 'Format disk',
                      description: `All data on ${disk.name} will be permanently destroyed. Shares and apps using this disk will stop working.`,
                      match: disk.name,
                    },
                    {
                      id: 'format-mount',
                      label: 'Format & mount disk',
                      description: `Destroys all data on ${disk.name}, creates a fresh filesystem, and mounts it at its canonical branch path for pool use.`,
                      match: disk.name,
                    },
                    {
                      id: 'erase',
                      label: 'Erase disk signatures',
                      description: `Wipes all filesystem signatures from ${disk.name} so it can be repurposed. The disk will show as blank.`,
                      match: disk.name,
                    },
                  ]),
            ]}
          />

          <DiskOperationsDialog
            disk={disk}
            action={diskAction}
            onOpenChange={(open) => {
              if (!open) setDiskAction(null)
            }}
          />
          <DiskReplacementDialog disk={disk} open={replacementOpen} onOpenChange={setReplacementOpen} />
        </>
      )}
    </ResourceDrawer>
  )
}

function SmartRow({
  label,
  value,
  text,
  children,
}: {
  label: string
  value?: number
  text?: string
  children?: React.ReactNode
}) {
  const warning = value != null && value > 0
  return (
    <div className="flex items-center justify-between gap-3 px-3 py-2.5">
      <span className="text-sm text-muted-foreground">{label}</span>
      {children ?? (
        <span className={cn('tnum text-sm font-medium', warning && 'text-warning')}>
          {text ?? (value != null ? value.toLocaleString() : '—')}
        </span>
      )}
    </div>
  )
}
