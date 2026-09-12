import { CalendarClock } from 'lucide-react'
import { useJobs, useSchedules } from '@/api/queries'
import { JobProgress } from '@/components/core/job-progress'
import { ResourceTable, type Column } from '@/components/core/resource-table'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { formatDuration, timeAgo } from '@/lib/format'
import { cn } from '@/lib/utils'
import type { Job } from '@/api/types'

const STATE_STYLES: Record<Job['state'], string> = {
  queued: 'text-muted-foreground',
  preparing: 'text-info',
  running: 'text-primary',
  'waiting-confirmation': 'text-attention',
  successful: 'text-success',
  failed: 'text-critical',
  cancelled: 'text-muted-foreground',
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
                <li key={schedule.id} className="flex items-center justify-between gap-3 px-3 py-2.5">
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">{schedule.name}</p>
                    <p className="text-xs text-muted-foreground">{schedule.schedule}</p>
                  </div>
                  <div className="flex shrink-0 items-center gap-2">
                    <span className="text-xs text-muted-foreground">{schedule.next}</span>
                    {schedule.enabled ? (
                      <Badge variant="success">On</Badge>
                    ) : (
                      <Badge variant="secondary">Off</Badge>
                    )}
                  </div>
                </li>
              ))}
            </ul>
            <p className="mt-3 text-xs text-muted-foreground">
              Jobs that would collide are queued sequentially — never run in parallel.
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
