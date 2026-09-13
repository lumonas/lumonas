import { CalendarClock } from 'lucide-react'
import {
  useJobs,
  useSchedules,
  useUpdateSchedule,
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
  const editable = schedule.kind !== 'event'
  const [time, setTime] = useState(schedule.timeOfDay)
  const [weekday, setWeekday] = useState(schedule.weekday ?? 'sunday')
  const [snapshotKind, setSnapshotKind] = useState(schedule.snapshotKind ?? 'btrfs')
  const [snapshotSource, setSnapshotSource] = useState(schedule.snapshotSource ?? '')
  const [snapshotLabel, setSnapshotLabel] = useState(schedule.snapshotLabel ?? 'scheduled')
  const [snapshotKeep, setSnapshotKeep] = useState(String(schedule.snapshotKeep ?? 7))
  const timeDirty = editable && time !== schedule.timeOfDay
  const weekdayDirty = schedule.kind === 'weekly' && weekday !== (schedule.weekday ?? 'sunday')
  const snapshotDirty = schedule.jobType === 'snapshot.create' && (
    snapshotKind !== (schedule.snapshotKind ?? 'btrfs') ||
    snapshotSource !== (schedule.snapshotSource ?? '') ||
    snapshotLabel !== (schedule.snapshotLabel ?? 'scheduled') ||
    snapshotKeep !== String(schedule.snapshotKeep ?? 7)
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
    })
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
            {snapshotDirty && (
              <button type="button" onClick={saveSnapshot} disabled={update.isPending} className="h-7 rounded-md bg-primary px-2 text-xs font-medium text-primary-foreground disabled:opacity-50">
                Save
              </button>
            )}
          </div>
        )}
        <Switch
          checked={schedule.enabled}
          disabled={!editable || update.isPending}
          onCheckedChange={(checked) => update.mutate({ id: schedule.id, enabled: checked })}
          aria-label={`Toggle ${schedule.name}`}
        />
      </div>
    </li>
  )
}

export function JobsTab() {
  const { data: jobs, isLoading } = useJobs()
  const { data: schedules } = useSchedules()

  const columns: Column<Job>[] = [
    {
      id: 'job',
      header: 'Job',
      sortValue: (j) => j.title,
      cell: (j) => (
        <div className="flex flex-col">
          <span className="text-[13px] font-medium">{j.title}</span>
          <span className="font-mono text-xs text-muted-foreground">{j.type}</span>
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
