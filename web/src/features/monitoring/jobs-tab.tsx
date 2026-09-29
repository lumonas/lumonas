import { CalendarClock, RotateCcw, X } from 'lucide-react'
import {
  useJobs,
  useSchedules,
  useUpdateSchedule,
  useCreateSnapshotSchedule,
  useCreateFilesystemScrubSchedule,
  useDeleteCustomSchedule,
  useCreateJob,
  useCancelQueuedJob,
} from '@/api/queries'
import { JobProgress } from '@/components/core/job-progress'
import { ResourceTable, type Column } from '@/components/core/resource-table'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { formatDuration, timeAgo } from '@/lib/format'
import { cn } from '@/lib/utils'
import { useState } from 'react'
import type { Job, JobSchedule } from '@/api/types'

const STATE_STYLES: Record<Job['state'], string> = {
  queued: 'text-muted-foreground',
  preparing: 'text-info',
  running: 'text-primary',
  'waiting-confirmation': 'text-attention',
  successful: 'text-success',
  failed: 'text-critical',
  cancelled: 'text-muted-foreground',
}

const WEEKDAYS = [
  { value: 'monday', label: 'Mondays' },
  { value: 'tuesday', label: 'Tuesdays' },
  { value: 'wednesday', label: 'Wednesdays' },
  { value: 'thursday', label: 'Thursdays' },
  { value: 'friday', label: 'Fridays' },
  { value: 'saturday', label: 'Saturdays' },
  { value: 'sunday', label: 'Sundays' },
]

function ScheduleRow({ schedule }: { schedule: JobSchedule }) {
  const update = useUpdateSchedule()
  const remove = useDeleteCustomSchedule()
  const editable = schedule.kind !== 'event'
  const [time, setTime] = useState(schedule.timeOfDay)
  const [weekday, setWeekday] = useState(schedule.weekday ?? 'sunday')
  const [snapshotKind, setSnapshotKind] = useState(schedule.snapshotKind ?? 'btrfs')
  const [snapshotSource, setSnapshotSource] = useState(schedule.snapshotSource ?? '')
  const [snapshotLabel, setSnapshotLabel] = useState(schedule.snapshotLabel ?? 'scheduled')
  const [snapshotKeep, setSnapshotKeep] = useState(String(schedule.snapshotKeep ?? 7))
  const [snapshotLockDays, setSnapshotLockDays] = useState(String(schedule.snapshotLockDays ?? 0))
  const timeDirty = editable && time !== schedule.timeOfDay
  const weekdayDirty = schedule.kind === 'weekly' && weekday !== (schedule.weekday ?? 'sunday')
  const snapshotDirty = schedule.jobType === 'snapshot.create' && (
    snapshotKind !== (schedule.snapshotKind ?? 'btrfs') ||
    snapshotSource !== (schedule.snapshotSource ?? '') ||
    snapshotLabel !== (schedule.snapshotLabel ?? 'scheduled') ||
    snapshotKeep !== String(schedule.snapshotKeep ?? 7) ||
    snapshotLockDays !== String(schedule.snapshotLockDays ?? 0)
  )
  const [filesystemKind, setFilesystemKind] = useState<'btrfs' | 'zfs'>(schedule.filesystemKind ?? 'btrfs')
  const [filesystemSource, setFilesystemSource] = useState(schedule.filesystemSource ?? '')
  const filesystemDirty = schedule.jobType === 'filesystem.scrub' && (
    filesystemKind !== (schedule.filesystemKind ?? 'btrfs') || filesystemSource !== (schedule.filesystemSource ?? '')
  )

  function saveTime() {
    update.mutate({ id: schedule.id, timeOfDay: time })
  }
  function saveWeekday(day: string) {
    setWeekday(day)
    update.mutate({ id: schedule.id, weekday: day })
  }

  function saveSnapshot() {
    update.mutate({
      id: schedule.id,
      snapshotKind,
      snapshotSource,
      snapshotLabel,
      snapshotKeep: Number(snapshotKeep) || 0,
      snapshotLockDays: Number(snapshotLockDays) || 0,
    })
  }

  function saveFilesystem() {
    update.mutate({ id: schedule.id, filesystemKind, filesystemSource: filesystemSource.trim() })
  }

  return (
    <li className="flex items-center justify-between gap-3 px-3 py-2.5">
      <div className="min-w-0">
        <p className="truncate text-sm font-medium">{schedule.name}</p>
        <p className="text-xs text-muted-foreground">{schedule.schedule}</p>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <span className="text-xs text-muted-foreground">{schedule.next}</span>
        {schedule.kind === 'weekly' && (
          <Select
            value={weekday}
            onValueChange={saveWeekday}
            disabled={!schedule.enabled || update.isPending}
          >
            <SelectTrigger className="h-7 w-[110px] text-xs" aria-label="Weekday">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {WEEKDAYS.map((day) => (
                <SelectItem key={day.value} value={day.value} className="text-xs">
                  {day.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
        {editable && (
          <>
            <input
              type="time"
              value={time}
              disabled={!schedule.enabled || update.isPending}
              onChange={(event) => setTime(event.target.value)}
              className="h-7 rounded-md border bg-transparent px-2 text-xs tabular-nums outline-none disabled:cursor-not-allowed disabled:opacity-50"
              aria-label="Time of day"
            />
            {(timeDirty || weekdayDirty) && (
              <button
                type="button"
                onClick={saveTime}
                disabled={update.isPending}
                className="h-7 rounded-md bg-primary px-2 text-xs font-medium text-primary-foreground disabled:opacity-50"
              >
                Save
              </button>
            )}
          </>
        )}
        {schedule.jobType === 'snapshot.create' && (
          <div className="flex max-w-[520px] flex-wrap items-center justify-end gap-1.5">
            <Select value={snapshotKind} onValueChange={(value) => setSnapshotKind(value as 'btrfs' | 'zfs')} disabled={update.isPending}>
              <SelectTrigger className="h-7 w-[82px] text-xs" aria-label="Snapshot filesystem">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="btrfs" className="text-xs">Btrfs</SelectItem>
                <SelectItem value="zfs" className="text-xs">ZFS</SelectItem>
              </SelectContent>
            </Select>
            <Input value={snapshotSource} onChange={(event) => setSnapshotSource(event.target.value)} placeholder="/srv/pools/media" className="h-7 w-[170px] text-xs" aria-label="Snapshot source" />
            <Input value={snapshotLabel} onChange={(event) => setSnapshotLabel(event.target.value)} placeholder="Label" className="h-7 w-[92px] text-xs" aria-label="Snapshot label" />
            <Input type="number" min={1} max={365} value={snapshotKeep} onChange={(event) => setSnapshotKeep(event.target.value)} className="h-7 w-[58px] text-xs" aria-label="Snapshots to keep" />
            <Input type="number" min={0} max={3650} value={snapshotLockDays} onChange={(event) => setSnapshotLockDays(event.target.value)} className="h-7 w-[62px] text-xs" aria-label="Snapshot lock days" title="Prevent deletion until this many days have passed" />
            {snapshotDirty && (
              <button type="button" onClick={saveSnapshot} disabled={update.isPending} className="h-7 rounded-md bg-primary px-2 text-xs font-medium text-primary-foreground disabled:opacity-50">
                Save
              </button>
            )}
          </div>
        )}
        {schedule.jobType === 'filesystem.scrub' && (
          <div className="flex max-w-[430px] flex-wrap items-center justify-end gap-1.5">
            <Select value={filesystemKind} onValueChange={(value) => setFilesystemKind(value as 'btrfs' | 'zfs')} disabled={update.isPending}>
              <SelectTrigger className="h-7 w-[82px] text-xs" aria-label="Scrub filesystem"><SelectValue /></SelectTrigger>
              <SelectContent><SelectItem value="btrfs" className="text-xs">Btrfs</SelectItem><SelectItem value="zfs" className="text-xs">ZFS</SelectItem></SelectContent>
            </Select>
            <Input value={filesystemSource} onChange={(event) => setFilesystemSource(event.target.value)} placeholder={filesystemKind === 'btrfs' ? '/srv/pools/media' : 'tank/media'} className="h-7 w-[170px] text-xs" aria-label="Scrub source" />
            {filesystemDirty && <button type="button" onClick={saveFilesystem} disabled={update.isPending || !filesystemSource.trim()} className="h-7 rounded-md bg-primary px-2 text-xs font-medium text-primary-foreground disabled:opacity-50">Save</button>}
          </div>
        )}
        <Switch
          checked={schedule.enabled}
          disabled={!editable || update.isPending}
          onCheckedChange={(checked) => update.mutate({ id: schedule.id, enabled: checked })}
          aria-label={`Toggle ${schedule.name}`}
        />
        {schedule.id.startsWith('schedule-') && <Button size="sm" variant="ghost" aria-label={`Delete ${schedule.name}`} onClick={() => { if (window.confirm(`Delete the ${schedule.name} schedule?`)) remove.mutate(schedule.id) }} disabled={remove.isPending}>Delete</Button>}
      </div>
    </li>
  )
}

export function JobsTab() {
  const { data: jobs, isLoading } = useJobs()
  const { data: schedules } = useSchedules()
  const retryJob = useCreateJob()
  const cancelJob = useCancelQueuedJob()
  const createSnapshotSchedule = useCreateSnapshotSchedule()
  const createFilesystemScrubSchedule = useCreateFilesystemScrubSchedule()
  const [snapshotName, setSnapshotName] = useState('')
  const [snapshotSource, setSnapshotSource] = useState('')
  const [snapshotKind, setSnapshotKind] = useState<'btrfs' | 'zfs'>('btrfs')
  const [snapshotKeep, setSnapshotKeep] = useState('7')
  const [snapshotLockDays, setSnapshotLockDays] = useState('30')
  const [snapshotTime, setSnapshotTime] = useState('01:30')
  const [snapshotCadence, setSnapshotCadence] = useState<'daily' | 'weekly'>('daily')
  const [snapshotWeekday, setSnapshotWeekday] = useState('sunday')
  const [scrubName, setScrubName] = useState('')
  const [scrubSource, setScrubSource] = useState('')
  const [scrubKind, setScrubKind] = useState<'btrfs' | 'zfs'>('btrfs')
  const [scrubCadence, setScrubCadence] = useState<'daily' | 'weekly'>('weekly')
  const [scrubWeekday, setScrubWeekday] = useState('sunday')
  const [scrubTime, setScrubTime] = useState('03:00')

  const columns: Column<Job>[] = [
    {
      id: 'job',
      header: 'Job',
      sortValue: (j) => j.title,
      searchValue: (j) => `${j.title} ${j.type} ${j.state} ${j.resourceId ?? ''} ${j.correlationId ?? ''}`,
      cell: (j) => (
        <div className="flex flex-col">
          <span className="text-[13px] font-medium">{j.title}</span>
          <span className="font-mono text-xs text-muted-foreground">{j.type}</span>
          {j.stage ? <span className="text-xs text-muted-foreground">{j.stage}</span> : null}
          {j.error ? <span className="max-w-64 truncate text-xs text-critical" title={j.error}>{j.error}</span> : null}
        </div>
      ),
    },
    {
      id: 'state',
      header: 'State',
      sortValue: (j) => j.state,
      cell: (j) => (
        <span className={cn('text-xs font-medium capitalize', STATE_STYLES[j.state])}>
          {j.state.replace('-', ' ')}
        </span>
      ),
    },
    {
      id: 'progress',
      header: 'Progress',
      className: 'tnum',
      sortValue: (j) => j.progress ?? -1,
      cell: (j) =>
        j.state === 'running' || j.state === 'queued' ? (
          <span>{j.progress != null ? `${Math.floor(j.progress)}%` : '—'}</span>
        ) : j.state === 'successful' ? (
          <span className="text-muted-foreground">100%</span>
        ) : (
          <span className="text-muted-foreground">—</span>
        ),
    },
    {
      id: 'created',
      header: 'Started',
      sortValue: (j) => j.createdAt,
      cell: (j) => (
        <span className="text-xs text-muted-foreground">{timeAgo(j.startedAt ?? j.createdAt)}</span>
      ),
    },
    {
      id: 'duration',
      header: 'Duration',
      className: 'tnum',
      cell: (j) => (
        <span className="text-xs text-muted-foreground">
          {formatDuration(j.startedAt ?? j.createdAt, j.finishedAt)}
        </span>
      ),
    },
    {
      id: 'resource',
      header: 'Resource',
      sortValue: (job) => job.resourceId ?? '',
      cell: (job) => job.resourceId ? <span className="max-w-36 truncate font-mono text-xs text-muted-foreground" title={job.resourceId}>{job.resourceId}</span> : <span className="text-xs text-muted-foreground">—</span>,
    },
    {
      id: 'trace',
      header: 'Trace',
      cell: (j) => {
        const values = [
          j.operationId && `op: ${j.operationId}`,
          j.planHash && `plan: ${j.planHash}`,
          j.correlationId && `req: ${j.correlationId}`,
          j.generation != null && `generation: ${j.generation}`,
        ].filter(Boolean)
        return values.length > 0 ? (
          <span className="max-w-48 truncate font-mono text-[10px] text-muted-foreground" title={values.join(' · ')}>
            {values.join(' · ')}
          </span>
        ) : (
          <span className="text-muted-foreground">—</span>
        )
      },
    },
    {
      id: 'actions',
      header: 'Actions',
      cell: (job) => {
        const retryable = job.state === 'failed' && ['smart.short', 'smart.extended', 'snapraid.sync', 'snapraid.scrub'].includes(job.type)
        if (job.state === 'queued') return <Button size="sm" variant="outline" className="h-7 text-xs" disabled={cancelJob.isPending} onClick={() => cancelJob.mutate(job.id)}><X />Cancel</Button>
        if (job.state === 'running' && ['smart.short', 'smart.extended', 'share.relocate'].includes(job.type)) return <Button size="sm" variant="outline" className="h-7 text-xs" disabled={cancelJob.isPending} onClick={() => cancelJob.mutate(job.id)}><X />{job.type === 'share.relocate' ? 'Cancel move' : 'Stop safely'}</Button>
        return retryable ? <Button size="sm" variant="outline" className="h-7 text-xs" disabled={retryJob.isPending} onClick={() => {
          const warning = job.type === 'snapraid.sync' ? 'A retry runs SnapRAID sync and writes updated parity for the protected data. Continue?' : job.type === 'snapraid.scrub' ? 'A retry reads parity and data blocks to verify protection. It may create substantial disk I/O. Continue?' : 'A retry will read SMART data from this disk. Continue?'
          if (window.confirm(warning)) retryJob.mutate({ type: job.type, resourceId: job.resourceId, correlationId: job.correlationId ?? job.id })
        }}><RotateCcw />Retry</Button> : <span className="text-xs text-muted-foreground">—</span>
      },
    },
  ]

  return (
    <div className="flex flex-col gap-4">
      <ResourceTable columns={columns} rows={jobs ?? []} loading={isLoading} />

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader className="flex-row items-center justify-between space-y-0 pb-3">
            <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
              <CalendarClock className="size-4" />
              Scheduled jobs
            </CardTitle>
          </CardHeader>
          <CardContent>
            <ul className="flex flex-col divide-y rounded-lg border">
              {(schedules ?? []).map((schedule) => (
                <ScheduleRow key={schedule.id} schedule={schedule} />
              ))}
            </ul>
            <form className="mt-4 grid gap-2 rounded-lg border p-3 sm:grid-cols-2" onSubmit={(event) => {
              event.preventDefault()
              createSnapshotSchedule.mutate({ name: snapshotName.trim(), kind: snapshotCadence, timeOfDay: snapshotTime, weekday: snapshotCadence === 'weekly' ? snapshotWeekday : undefined, enabled: true, snapshotKind, snapshotSource: snapshotSource.trim(), snapshotLabel: 'scheduled', snapshotKeep: Number(snapshotKeep), snapshotLockDays: Number(snapshotLockDays) })
            }}>
              <p className="text-sm font-medium sm:col-span-2">Add snapshot policy</p>
              <Input value={snapshotName} onChange={(event) => setSnapshotName(event.target.value)} maxLength={80} placeholder="Policy name" aria-label="New snapshot schedule name" required />
              <Input value={snapshotSource} onChange={(event) => setSnapshotSource(event.target.value)} placeholder={snapshotKind === 'btrfs' ? '/srv/pools/media' : 'tank/media'} aria-label="New snapshot source" required />
              <select value={snapshotKind} onChange={(event) => setSnapshotKind(event.target.value as 'btrfs' | 'zfs')} className="h-9 rounded-md border bg-background px-2 text-sm" aria-label="New snapshot filesystem"><option value="btrfs">Btrfs</option><option value="zfs">ZFS</option></select>
              <select value={snapshotCadence} onChange={(event) => setSnapshotCadence(event.target.value as 'daily' | 'weekly')} className="h-9 rounded-md border bg-background px-2 text-sm" aria-label="New snapshot cadence"><option value="daily">Daily</option><option value="weekly">Weekly</option></select>
              {snapshotCadence === 'weekly' && <select value={snapshotWeekday} onChange={(event) => setSnapshotWeekday(event.target.value)} className="h-9 rounded-md border bg-background px-2 text-sm" aria-label="New snapshot weekday">{WEEKDAYS.map((day) => <option key={day.value} value={day.value}>{day.label}</option>)}</select>}
              <Input type="time" value={snapshotTime} onChange={(event) => setSnapshotTime(event.target.value)} aria-label="New snapshot time" required />
              <Input type="number" min={1} max={365} value={snapshotKeep} onChange={(event) => setSnapshotKeep(event.target.value)} aria-label="New snapshot retention" required />
              <Input type="number" min={0} max={3650} value={snapshotLockDays} onChange={(event) => setSnapshotLockDays(event.target.value)} aria-label="New snapshot lock days" title="0 disables the deletion lock" required />
              <p className="text-xs text-muted-foreground sm:col-span-2">Deletion lock days protect scheduled restore points from manual deletion and automatic retention cleanup. Set 0 to disable.</p>
              <Button type="submit" disabled={createSnapshotSchedule.isPending || !snapshotName.trim() || !snapshotSource.trim()} className="sm:col-span-2">{createSnapshotSchedule.isPending ? 'Adding…' : 'Add snapshot policy'}</Button>
            </form>
            <form className="mt-3 grid gap-2 rounded-lg border p-3 sm:grid-cols-2" onSubmit={(event) => {
              event.preventDefault()
              createFilesystemScrubSchedule.mutate({ name: scrubName.trim(), kind: scrubCadence, timeOfDay: scrubTime, weekday: scrubCadence === 'weekly' ? scrubWeekday : undefined, enabled: true, filesystemKind: scrubKind, filesystemSource: scrubSource.trim() })
            }}>
              <p className="text-sm font-medium sm:col-span-2">Add filesystem integrity scrub</p>
              <Input value={scrubName} onChange={(event) => setScrubName(event.target.value)} maxLength={80} placeholder="Policy name" aria-label="New scrub schedule name" required />
              <Input value={scrubSource} onChange={(event) => setScrubSource(event.target.value)} placeholder={scrubKind === 'btrfs' ? '/srv/pools/media' : 'tank/media'} aria-label="New scrub source" required />
              <select value={scrubKind} onChange={(event) => setScrubKind(event.target.value as 'btrfs' | 'zfs')} className="h-9 rounded-md border bg-background px-2 text-sm" aria-label="New scrub filesystem"><option value="btrfs">Btrfs</option><option value="zfs">ZFS</option></select>
              <select value={scrubCadence} onChange={(event) => setScrubCadence(event.target.value as 'daily' | 'weekly')} className="h-9 rounded-md border bg-background px-2 text-sm" aria-label="New scrub cadence"><option value="daily">Daily</option><option value="weekly">Weekly</option></select>
              {scrubCadence === 'weekly' && <select value={scrubWeekday} onChange={(event) => setScrubWeekday(event.target.value)} className="h-9 rounded-md border bg-background px-2 text-sm" aria-label="New scrub weekday">{WEEKDAYS.map((day) => <option key={day.value} value={day.value}>{day.label}</option>)}</select>}
              <Input type="time" value={scrubTime} onChange={(event) => setScrubTime(event.target.value)} aria-label="New scrub time" required />
              <Button type="submit" disabled={createFilesystemScrubSchedule.isPending || !scrubName.trim() || !scrubSource.trim()} className="sm:col-span-2">{createFilesystemScrubSchedule.isPending ? 'Adding…' : 'Add scrub schedule'}</Button>
              <Button type="button" variant="outline" className="sm:col-span-2" disabled={retryJob.isPending || !scrubSource.trim()} onClick={() => {
                if (window.confirm('Run a filesystem integrity scrub now? It reads the selected filesystem and may create sustained disk I/O.')) {
                  retryJob.mutate({ type: 'filesystem.scrub', filesystemKind: scrubKind, filesystemSource: scrubSource.trim() })
                }
              }}>{retryJob.isPending ? 'Starting…' : 'Run scrub now'}</Button>
            </form>
            <p className="mt-3 text-xs text-muted-foreground">
              Schedules run on the appliance clock — collisions are re-armed to the next slot,
              never run in parallel.
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Currently running
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            {(jobs ?? []).filter((j) => j.state === 'running' || j.state === 'queued').length ===
            0 ? (
              <p className="py-4 text-center text-sm text-muted-foreground">
                Nothing is running right now.
              </p>
            ) : (
              (jobs ?? [])
                .filter((j) => j.state === 'running' || j.state === 'queued')
                .map((job) => <JobProgress key={job.id} job={job} />)
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
