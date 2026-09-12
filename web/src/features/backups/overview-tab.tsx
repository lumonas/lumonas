import { ArchiveRestore, CheckCircle2, CircleAlert, ShieldCheck } from 'lucide-react'
import { useBackupJobs, useBackupReadiness } from '@/api/queries'
import { HealthBadge } from '@/components/core/health-badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { timeAgo } from '@/lib/format'
import { RestoreWizard } from '@/features/backups/restore-wizard'
import { useState } from 'react'
import { cn } from '@/lib/utils'

const LAYER_STATE = {
  current: { Icon: CheckCircle2, className: 'text-success' },
  stale: { Icon: CircleAlert, className: 'text-attention' },
  missing: { Icon: CircleAlert, className: 'text-critical' },
} as const

export function BackupsOverviewTab() {
  const { data: readiness } = useBackupReadiness()
  const { data: jobs } = useBackupJobs()
  const [restoreOpen, setRestoreOpen] = useState(false)

  const recent = (jobs ?? [])
    .filter((j) => j.lastRun)
    .sort((a, b) => (a.lastRun!.at < b.lastRun!.at ? 1 : -1))
    .slice(0, 4)

  return (
    <div className="flex flex-col gap-4">
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-12">
        <Card className="lg:col-span-4">
          <CardHeader className="pb-3">
            <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
              <ShieldCheck className="size-4" />
              Recovery readiness
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col items-center gap-2 py-4">
            <span className="tnum text-5xl font-semibold tracking-tight">
              {readiness?.score ?? '—'}
              <span className="text-2xl text-muted-foreground">%</span>
            </span>
            <p className="max-w-[26ch] text-center text-xs text-muted-foreground">
              A failed system disk should be an inconvenience, not a disaster.
            </p>
            <Button
              size="sm"
              variant="outline"
              className="mt-2"
              onClick={() => setRestoreOpen(true)}
            >
              <ArchiveRestore />
              Disaster recovery…
            </Button>
          </CardContent>
        </Card>

        <Card className="lg:col-span-8">
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              What makes up the score
            </CardTitle>
          </CardHeader>
          <CardContent>
            <ul className="flex flex-col divide-y rounded-lg border">
              {(readiness?.layers ?? []).map((layer) => {
                const { Icon, className } = LAYER_STATE[layer.status]
                return (
                  <li key={layer.id} className="flex items-center justify-between gap-3 px-3 py-2.5">
                    <div className="flex min-w-0 items-center gap-2.5">
                      <Icon className={cn('size-4 shrink-0', className)} />
                      <div className="min-w-0">
                        <p className="truncate text-sm font-medium">{layer.label}</p>
                        <p className="truncate text-xs text-muted-foreground">{layer.detail}</p>
                      </div>
                    </div>
                    <span
                      className={cn(
                        'shrink-0 text-xs font-medium capitalize',
                        layer.status === 'current' && 'text-success',
                        layer.status === 'stale' && 'text-attention',
                        layer.status === 'missing' && 'text-critical',
                      )}
                    >
                      {layer.status}
                    </span>
                  </li>
                )
              })}
            </ul>
            <p className="mt-3 text-xs text-muted-foreground">
              Backup verification means checksum + decryptability — a file existing is not the same
              as a backup being healthy.
            </p>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            Recent backup runs
          </CardTitle>
        </CardHeader>
        <CardContent>
          <ul className="flex flex-col divide-y rounded-lg border">
            {recent.map((job) => (
              <li key={job.id} className="flex items-center justify-between gap-3 px-3 py-2.5">
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{job.name}</p>
                  {job.lastRun?.detail ? (
                    <p className="truncate text-xs text-muted-foreground">{job.lastRun.detail}</p>
                  ) : null}
                </div>
                <div className="flex shrink-0 items-center gap-3">
                  <span className="text-xs text-muted-foreground">
                    {job.lastRun ? timeAgo(job.lastRun.at) : 'never'}
                  </span>
                  <HealthBadge state={job.lastRun?.status ?? 'offline'} />
                </div>
              </li>
            ))}
          </ul>
        </CardContent>
      </Card>

      <RestoreWizard open={restoreOpen} onOpenChange={setRestoreOpen} />
    </div>
  )
}
