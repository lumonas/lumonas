import {
  Archive,
  Container,
  Download,
  HardDrive,
  Network,
  Settings,
  ShieldCheck,
  type LucideIcon,
} from 'lucide-react'
import { formatDateTime } from '@/lib/format'
import { cn } from '@/lib/utils'
import type { ActivityEvent } from '@/api/types'

const CATEGORY_ICONS: Record<ActivityEvent['category'], LucideIcon> = {
  config: Settings,
  storage: HardDrive,
  docker: Container,
  backup: Archive,
  security: ShieldCheck,
  update: Download,
  network: Network,
}

export function TimelineEvent({
  event,
  className,
}: {
  event: ActivityEvent
  className?: string
}) {
  const Icon = CATEGORY_ICONS[event.category]
  return (
    <div className={cn('flex items-start gap-3', className)}>
      <div className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full bg-secondary">
        <Icon className="size-3.5 text-muted-foreground" />
      </div>
      <div className="min-w-0 flex-1 border-b pb-3 last:border-b-0">
        <div className="flex items-baseline justify-between gap-3">
          <p className="truncate text-sm font-medium text-foreground">{event.title}</p>
          <time
            dateTime={event.timestamp}
            title={formatDateTime(event.timestamp)}
            className="shrink-0 text-xs text-muted-foreground"
          >
            {formatDateTime(event.timestamp)}
          </time>
        </div>
        {event.description ? (
          <p className="mt-0.5 truncate text-sm text-muted-foreground">{event.description}</p>
        ) : null}
      </div>
    </div>
  )
}
