import { useNavigate } from 'react-router-dom'
import { useBackupDestinations, useBackupJobs, useRunBackup } from '@/api/queries'
import { HealthBadge } from '@/components/core/health-badge'
import { ResourceTable, type Column } from '@/components/core/resource-table'
import { Button } from '@/components/ui/button'
import { timeAgo } from '@/lib/format'
import type { BackupJob } from '@/api/types'

export function BackupJobsTab() {
  const navigate = useNavigate()
  const { data: jobs, isLoading } = useBackupJobs()
  const { data: destinations } = useBackupDestinations()
  const runBackup = useRunBackup()

  const columns: Column<BackupJob>[] = [
    {
      id: 'name',
      header: 'Job',
      sortValue: (j) => j.name,
      cell: (j) => (
        <div className="flex flex-col">
          <span className="text-[13px] font-medium">{j.name}</span>
          <span className="text-xs text-muted-foreground">{j.source}</span>
        </div>
      ),
    },
    {
      id: 'destination',
      header: 'Destination',
      sortValue: (j) => destinations?.find((d) => d.id === j.destinationId)?.label ?? '',
      cell: (j) => (
        <span className="text-xs text-muted-foreground">
          {destinations?.find((d) => d.id === j.destinationId)?.label ?? '—'}
        </span>
      ),
    },
    {
      id: 'schedule',
      header: 'Schedule',
      cell: (j) => <span className="text-xs text-muted-foreground">{j.schedule}</span>,
    },
    {
      id: 'strategy',
      header: 'Strategy',
      advanced: true,
      cell: (j) => <span className="text-xs text-muted-foreground">{j.strategy}</span>,
    },
    {
      id: 'lastRun',
      header: 'Last run',
      sortValue: (j) => j.lastRun?.at ?? '',
      cell: (j) => (
        <div className="flex items-center gap-2">
          <HealthBadge state={j.lastRun?.status ?? 'offline'} />
          <span className="text-xs text-muted-foreground">
            {j.lastRun ? timeAgo(j.lastRun.at) : 'never'}
          </span>
        </div>
      ),
    },
    {
      id: 'actions',
      header: '',
      className: 'w-24 text-right',
      cell: (j) => (
        <Button
          size="sm"
          variant="outline"
          className="h-7 text-xs"
          disabled={j.jobType === undefined}
          onClick={() => runBackup.mutate(j.id)}
        >
          Run now
        </Button>
      ),
    },
  ]

  return (
    <div className="flex flex-col gap-3">
      <ResourceTable columns={columns} rows={jobs ?? []} loading={isLoading} />
      <p className="text-xs text-muted-foreground">
        Backup runs never overlap with SnapRAID sync — the job engine queues them sequentially.
        Separately from these jobs, disaster recovery restores a dead system disk from the
        readiness checklist.
      </p>
      <button
        type="button"
        className="self-start text-sm font-medium text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        onClick={() => navigate('/monitoring?tab=jobs')}
      >
        Open job history
      </button>
    </div>
  )
}
