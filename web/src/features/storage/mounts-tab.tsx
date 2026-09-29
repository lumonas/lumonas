import { useState } from 'react'
import { HardDrive, Lock, LockOpen, Plus, TrendingUp, Unplug } from 'lucide-react'
import {
  useCapacityForecast,
  useCapacityThresholds,
  useSetCapacityThreshold,
  usePools,
  usePlanPool,
  usePlanPoolUnmount,
  useConfirmPool,
  useConfirmPoolUnmount,
  useStorageMounts,
  useStorageSafety,
  useLockStorageSafety,
  useUnlockStorageSafety,
} from '@/api/queries'
import { AlertBanner } from '@/components/core/alert-banner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { OperationReview } from '@/components/core/operation-review'
import { useDisks } from '@/api/queries'
import { formatBytes, formatDateTime } from '@/lib/format'
import type { PoolPlan, PoolUnmountPlan } from '@/api/types'

export function SafetyBanner() {
  const { data: safety } = useStorageSafety()
  const unlock = useUnlockStorageSafety()
  const lock = useLockStorageSafety()
  if (!safety) return null
  const unlocked = safety.state === 'unlocked'
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border px-4 py-3">
      <div className="flex items-center gap-2.5">
        {unlocked ? (
          <LockOpen className="size-4 text-warning" />
        ) : (
          <Lock className="size-4 text-muted-foreground" />
        )}
        <div>
          <p className="text-sm font-medium">
            Storage safety {unlocked ? 'unlocked' : 'locked'}
          </p>
          <p className="text-xs text-muted-foreground">
            {unlocked
              ? `Destructive storage operations are allowed until ${safety.unlockedUntil ? formatDateTime(safety.unlockedUntil) : 'the window closes'}.`
              : 'Destructive operations require unlocking with re-authentication.'}
          </p>
        </div>
      </div>
      {unlocked ? (
        <Button size="sm" variant="outline" onClick={() => lock.mutate()} disabled={lock.isPending}>
          Lock now
        </Button>
      ) : (
        <Button size="sm" onClick={() => unlock.mutate()} disabled={unlock.isPending}>
          {unlock.isPending ? 'Unlocking…' : 'Unlock (15 min)'}
        </Button>
      )}
    </div>
  )
}

function PoolCreateDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { data: disks } = useDisks()
  const plan = usePlanPool()
  const confirm = useConfirmPool()
  const [name, setName] = useState('')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [planResult, setPlanResult] = useState<PoolPlan | null>(null)

  const candidates = (disks ?? []).filter((disk) => disk.role === 'data' || disk.role === 'unknown')

  function toggle(diskId: string) {
    setSelected((previous) => {
      const next = new Set(previous)
      if (next.has(diskId)) next.delete(diskId)
      else next.add(diskId)
      return next
    })
  }

  function close() {
    onOpenChange(false)
    setPlanResult(null)
    setName('')
    setSelected(new Set())
  }

  function submitPlan() {
    plan.mutate(
      { name: name.trim(), diskIds: [...selected] },
      { onSuccess: (value) => setPlanResult(value) },
    )
  }

  return (
    <OperationReview
      open={open}
      onOpenChange={(next) => (next ? onOpenChange(true) : close())}
      title={planResult ? `Confirm pool — ${planResult.name}` : 'Create a mergerfs pool'}
      description={
        planResult
          ? 'Review the member disks. Mounting happens via a privileged plan/confirm pair.'
          : 'Pick a name and the disks to merge. Each disk keeps its own filesystem; parity is configured separately.'
      }
      steps={
        planResult
          ? [
              `Mount point: ${planResult.mountPath}`,
              `Policy: ${planResult.policy}`,
              ...planResult.members.map(
                (member) => `${member.model ?? member.diskId} — ${member.branchPath}`,
              ),
            ]
          : undefined
      }
      warnings={
        planResult
          ? [{ label: 'Existing data on member disks stays untouched', description: 'Files are merged view-side only; nothing is reformatted.' }]
          : undefined
      }
      confirmLabel={planResult ? 'Mount pool' : 'Plan pool'}
      loading={plan.isPending || confirm.isPending}
      onConfirm={() => {
        if (!planResult) {
          submitPlan()
          return
        }
        confirm.mutate(
          { operationId: planResult.operationId, planHash: planResult.planHash },
          { onSuccess: () => close() },
        )
      }}
    >
      {!planResult ? (
        <div className="grid gap-3">
          <div className="grid gap-2">
            <Label htmlFor="pool-name">Pool name</Label>
            <Input
              id="pool-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder="media"
            />
          </div>
          <div className="grid gap-1.5">
            <Label>Member disks</Label>
            {candidates.length === 0 ? (
              <p className="text-sm text-muted-foreground">No eligible disks discovered.</p>
            ) : (
              candidates.map((disk) => (
                <label
                  key={disk.id}
                  className="flex items-center gap-2.5 rounded-md border px-3 py-2 text-sm"
                >
                  <input
                    type="checkbox"
                    checked={selected.has(disk.id)}
                    onChange={() => toggle(disk.id)}
                    className="size-4 accent-primary"
                  />
                  <span className="min-w-0 flex-1 truncate">
                    <span className="font-mono text-xs">{disk.name}</span>
                    <span className="mx-2 text-muted-foreground">·</span>
                    {disk.model}
                  </span>
                  <span className="tnum text-xs text-muted-foreground">
                    {formatBytes(disk.usedBytes ?? 0)} / {formatBytes(disk.sizeBytes)}
                  </span>
                </label>
              ))
            )}
          </div>
          {plan.isError ? (
            <AlertBanner tone="critical" title="Plan rejected">
              {plan.error instanceof Error ? plan.error.message : null}
            </AlertBanner>
          ) : null}
        </div>
      ) : null}
    </OperationReview>
  )
}

function PoolUnmountDialog({ poolName, open, onOpenChange }: { poolName: string | null; open: boolean; onOpenChange: (open: boolean) => void }) {
  const plan = usePlanPoolUnmount()
  const confirm = useConfirmPoolUnmount()
  const [planResult, setPlanResult] = useState<PoolUnmountPlan | null>(null)

  function close() {
    onOpenChange(false)
    setPlanResult(null)
  }

  return (
    <OperationReview
      open={open}
      onOpenChange={(next) => (next ? onOpenChange(true) : close())}
      title={planResult ? `Confirm unmount — ${planResult.name}` : `Unmount pool ${poolName ?? ''}`}
      description="Apps and shares serving from this pool will stop responding until it is mounted again."
      steps={planResult ? [`Mount point: ${planResult.mountPath}`, 'Members are unmounted after the pool'] : undefined}
      warnings={[
        { label: 'Docker apps using this pool will stop', description: 'Stop dependent stacks before unmounting.' },
      ]}
      confirmLabel={planResult ? 'Unmount now' : 'Plan unmount'}
      loading={plan.isPending || confirm.isPending}
      onConfirm={() => {
        if (!poolName) return
        if (!planResult) {
          plan.mutate(
            { name: poolName },
            { onSuccess: (value) => setPlanResult(value) },
          )
          return
        }
        confirm.mutate(
          { operationId: planResult.operationId, planHash: planResult.planHash },
          { onSuccess: () => close() },
        )
      }}
    />
  )
}

function CapacityForecastCard() {
  const { data: forecasts } = useCapacityForecast(90)
  const { data: thresholds } = useCapacityThresholds()
  const updateThreshold = useSetCapacityThreshold()
  if (!forecasts || forecasts.length === 0) return null
  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
          <TrendingUp className="size-4" />
          Capacity forecast
        </CardTitle>
        <p className="mt-1 text-xs text-muted-foreground">Thresholds are stored on this appliance and generate alerts for every signed-in administrator.</p>
      </CardHeader>
      <CardContent className="grid gap-3">
        {forecasts.map((forecast) => {
          const threshold = thresholds?.find((item) => item.resourceId === forecast.resourceId)?.thresholdPercent ?? 80
          const utilization = forecast.totalBytes > 0 ? (forecast.usedBytes / forecast.totalBytes) * 100 : 0
          const needsAttention = utilization >= threshold || (forecast.daysToNinetyPercent != null && forecast.daysToNinetyPercent < 60)
          return (
          <div key={forecast.resourceId} className="rounded-lg border px-3 py-2.5">
            <div className="flex items-center justify-between gap-3">
              <span className="truncate font-mono text-xs">{forecast.resourceId}</span>
              {utilization >= threshold ? (
                <Badge variant="warning">{utilization.toFixed(0)}% · above {threshold}%</Badge>
              ) : forecast.available ? (
                <Badge variant={needsAttention ? 'warning' : 'success'}>
                  {forecast.daysToNinetyPercent != null
                    ? `90% in ~${Math.round(forecast.daysToNinetyPercent)}d`
                    : 'stable'}
                </Badge>
              ) : (
                <Badge variant="offline">no trend</Badge>
              )}
            </div>
            <p className="mt-1 text-xs text-muted-foreground">
              {formatBytes(forecast.usedBytes)} / {formatBytes(forecast.totalBytes)}
              {forecast.available
                ? ` · growing ${formatBytes(forecast.growthBytesPerDay)}/day · full around ${forecast.estimatedFullAt ? new Date(forecast.estimatedFullAt).toLocaleDateString() : 'unknown'} · ${forecast.confidence} confidence`
                : ` · ${forecast.message ?? 'not enough history'}`}
              {forecast.stale ? ` · history is ${Math.round(forecast.sampleAgeHours)}h old` : ''}
            </p>
            <label className="mt-2 flex items-center gap-2 text-xs text-muted-foreground">Warn at
              <select aria-label={`Capacity warning threshold for ${forecast.resourceId}`} className="h-7 rounded-md border bg-background px-2 text-foreground" value={threshold} disabled={updateThreshold.isPending} onChange={(event) => updateThreshold.mutate({ resourceId: forecast.resourceId, thresholdPercent: Number(event.target.value) })}>{[70, 75, 80, 85, 90, 95].map((value) => <option key={value} value={value}>{value}%</option>)}</select>
              <span>{utilization.toFixed(0)}% used</span>
            </label>
          </div>
          )
        })}
      </CardContent>
    </Card>
  )
}

export function MountsTab() {
  const { data: mounts, isLoading } = useStorageMounts()
  const { data: pools } = usePools()
  const [createOpen, setCreateOpen] = useState(false)
  const [unmountTarget, setUnmountTarget] = useState<string | null>(null)
  const mountedPools = new Set((pools ?? []).map((pool) => pool.mountPath))

  return (
    <div className="flex flex-col gap-4">
      <SafetyBanner />
      <Card>
        <CardHeader className="flex-row items-center justify-between space-y-0 pb-3">
          <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
            <HardDrive className="size-4" />
            Mount units
          </CardTitle>
          <Button size="sm" onClick={() => setCreateOpen(true)}>
            <Plus />
            New pool
          </Button>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <p className="text-sm text-muted-foreground">Loading mounts…</p>
          ) : (mounts?.length ?? 0) === 0 ? (
            <p className="text-sm text-muted-foreground">No persistent mounts recorded yet.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Kind</TableHead>
                  <TableHead>Target</TableHead>
                  <TableHead>Mount path</TableHead>
                  <TableHead>Filesystem</TableHead>
                  <TableHead>Options</TableHead>
                  <TableHead />
                </TableRow>
              </TableHeader>
              <TableBody>
                {mounts?.map((mount) => (
                  <TableRow key={`${mount.kind}-${mount.targetId}`}>
                    <TableCell>
                      <Badge variant={mount.kind === 'pool' ? 'info' : 'secondary'}>{mount.kind}</Badge>
                    </TableCell>
                    <TableCell className="font-mono text-xs">{mount.targetId}</TableCell>
                    <TableCell className="font-mono text-xs">{mount.mountPath}</TableCell>
                    <TableCell className="text-xs">{mount.fstype}</TableCell>
                    <TableCell className="max-w-[220px] truncate font-mono text-xs text-muted-foreground">
                      {mount.options || '—'}
                    </TableCell>
                    <TableCell>
                      {mount.kind === 'pool' && mountedPools.has(mount.mountPath) ? (
                        <Button size="sm" variant="outline" className="h-7 text-xs" onClick={() => setUnmountTarget(mount.mountPath)}>
                          <Unplug />
                          Unmount
                        </Button>
                      ) : null}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
      <CapacityForecastCard />
      <PoolCreateDialog open={createOpen} onOpenChange={setCreateOpen} />
      <PoolUnmountDialog poolName={unmountTarget} open={unmountTarget != null} onOpenChange={(open) => { if (!open) setUnmountTarget(null) }} />
    </div>
  )
}
