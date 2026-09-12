import { createElement } from 'react'
import { Activity, Archive, CheckCircle2, Loader2, RefreshCw } from 'lucide-react'
import { Progress } from '@/components/ui/progress'
import { cn } from '@/lib/utils'
import type { Job } from '@/api/types'

const JOB_ICONS: Record<string, React.ElementType> = {
  'snapraid.sync': RefreshCw,
  'snapraid.scrub': RefreshCw,
  'backup.app': Archive,
  'smart.short': Activity,
  'smart.extended': Activity,
}

const ACTIVE_STATES: Job['state'][] = ['queued', 'preparing', 'running']

function jobIcon(type: string, className: string) {
  const Icon = JOB_ICONS[type] ?? Loader2
  return createElement(Icon, { className })
}

export function JobProgress({
  job,
  showState = true,
  className,
}: {
  job: Job
  showState?: boolean
  className?: string
}) {
  const active = ACTIVE_STATES.includes(job.state)

  return (
    <div className={cn('flex items-start gap-3', className)}>
      <div
        className={cn(
          'mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg border bg-secondary/60',
          active && 'border-primary/30 text-primary',
        )}
      >
        {jobIcon(job.type, cn('size-4', active && 'animate-spin [animation-duration:2.5s]'))}
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline justify-between gap-3">
          <p className="truncate text-sm font-medium">{job.title}</p>
          {job.progress != null && active ? (
            <span className="tnum shrink-0 text-xs text-muted-foreground">
              {Math.floor(job.progress)}%
            </span>
          ) : null}
        </div>
        {active ? (
          <>
            <p className="truncate text-xs text-muted-foreground">
              {job.stage ?? job.state}
            </p>
            <Progress value={job.progress ?? 0} className="mt-1.5" />
          </>
        ) : (
          <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
            {job.state === 'successful' ? (
              <>
                <CheckCircle2 className="size-3.5 text-success" />
                Completed
              </>
            ) : showState ? (
              <span className="capitalize">{job.state}</span>
            ) : null}
          </p>
        )}
      </div>
    </div>
  )
}
