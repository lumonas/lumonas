import { useNavigate } from 'react-router-dom'
import { Check, Clock3 } from 'lucide-react'
import { useAcknowledgeAlert, useActivity, useAlerts, useJobs, useSnoozeAlert } from '@/api/queries'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useNotificationsStore } from '@/stores/notifications'
import { timeAgo } from '@/lib/format'
import { cn } from '@/lib/utils'
import type { AlertSeverity } from '@/api/types'
import { BellIcon } from 'lucide-react'
import { useCurrentTime } from '@/hooks/useCurrentTime'

const SEVERITY_VARIANT: Record<AlertSeverity, 'info' | 'attention' | 'warning' | 'critical'> = {
  info: 'info',
  attention: 'attention',
  warning: 'warning',
  critical: 'critical',
}

function resourceRoute(type?: string, id?: string) {
  if (type === 'disk' && id) return `/storage?disk=${encodeURIComponent(id)}`
  if (type?.startsWith('docker') || type === 'container' || type === 'stack') return '/docker'
  if (type === 'backup') return '/backups?tab=recovery'
  if (type === 'network') return '/network'
  if (type === 'share' && id) return `/shares?share=${encodeURIComponent(id)}`
  if (type === 'job') return '/monitoring?tab=jobs'
  return '/monitoring?tab=alerts'
}

export function NotificationsPopover() {
  const navigate = useNavigate()
  const { data: alerts } = useAlerts()
  const { data: jobs } = useJobs()
  const { data: activity } = useActivity()
  const { markAllRead, readIds } = useNotificationsStore()
  const ack = useAcknowledgeAlert()
  const snooze = useSnoozeAlert()
  const now = useCurrentTime()

  const firing = (alerts ?? []).filter((a) => a.state === 'firing' && (!a.snoozedUntil || Date.parse(a.snoozedUntil) <= now))
  const alertGroups = new Map<string, typeof firing>()
  for (const alert of firing) {
    const key = `${alert.severity}:${alert.title}`
    alertGroups.set(key, [...(alertGroups.get(key) ?? []), alert])
  }
  const unread = firing.filter((a) => !readIds.has(a.id))
  const readStore = useNotificationsStore((s) => s.markRead)

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label="Notifications" className="relative">
          <BellIcon className="size-4" />
          {unread.length > 0 && (
            <span
              aria-hidden
              className="absolute top-1.5 right-1.5 size-2 rounded-full bg-attention"
            />
          )}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-96 p-0">
        <div className="flex items-center justify-between border-b px-4 py-3">
          <p className="text-sm font-semibold">Notifications</p>
          {firing.length > 0 && (
            <Button
              variant="ghost"
              size="sm"
              className="h-7 text-xs"
              onClick={() => markAllRead(firing.map((a) => a.id))}
            >
              Mark all read
            </Button>
          )}
        </div>
        <ScrollArea className="max-h-96">
          <div className="flex flex-col divide-y">
            {firing.length === 0 ? (
              <p className="px-4 py-8 text-center text-sm text-muted-foreground">
                No active notifications.
              </p>
            ) : (
              [...alertGroups.entries()].flatMap(([groupKey, group]) => [
                ...(group.length > 1 ? [<div key={`${groupKey}-group`} className="bg-muted/50 px-4 py-2 text-xs font-medium text-muted-foreground">{group.length} related alerts: {group[0].title}</div>] : []),
                ...group.map((alert) => {
                const isUnread = !readIds.has(alert.id)
                const relatedJob = (jobs ?? []).find((job) => job.resourceId && job.resourceId === alert.resource?.id && ['queued', 'preparing', 'running', 'waiting-confirmation', 'failed'].includes(job.state))
                const relatedChange = (activity ?? []).find((event) => event.resource?.id === alert.resource?.id)
                return (
                  <div
                    key={alert.id}
                    className={cn(
                      'flex flex-col gap-1.5 px-4 py-3',
                      isUnread && 'bg-accent/30',
                    )}
                  >
                    <div className="flex items-center justify-between gap-2">
                      <Badge variant={SEVERITY_VARIANT[alert.severity]} className="capitalize">
                        {alert.severity}
                      </Badge>
                      <time className="text-xs text-muted-foreground">{timeAgo(alert.startedAt)}</time>
                    </div>
                    <p className="text-sm font-medium leading-snug">{alert.title}</p>
                    <p className="text-xs leading-relaxed text-muted-foreground">
                      {alert.description}
                    </p>
                    {relatedJob ? <p className="text-xs text-muted-foreground">Related job: {relatedJob.title} · {relatedJob.state}</p> : null}
                    {relatedChange ? <p className="text-xs text-muted-foreground">Recent change: {relatedChange.title} · {timeAgo(relatedChange.timestamp)}</p> : null}
                    <div className="mt-0.5 flex items-center gap-2">
                      {(
                        <Button
                          variant="outline"
                          size="sm"
                          className="h-7 text-xs"
                          onClick={() => {
                            readStore(alert.id)
                            navigate(resourceRoute(alert.resource?.type, alert.resource?.id))
                          }}
                        >
                          Review
                        </Button>
                      )}
                      {alert.state === 'firing' && (
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-7 text-xs"
                          onClick={() => {
                            readStore(alert.id)
                            ack.mutate(alert.id)
                          }}
                        >
                          <Check />
                          Acknowledge
                        </Button>
                      )}
                      {alert.state === 'firing' && <div role="group" aria-label="Snooze notification" className="ml-auto flex items-center gap-1">
                        <Clock3 className="size-3.5 text-muted-foreground" />
                        {[1, 8, 24].map((hours) => <Button key={hours} variant="ghost" size="sm" className="h-7 px-2 text-xs" disabled={snooze.isPending} onClick={() => snooze.mutate({ id: alert.id, hours })}>{hours}h</Button>)}
                      </div>}
                    </div>
                  </div>
                )
                }),
              ])
            )}
          </div>
        </ScrollArea>
      </PopoverContent>
    </Popover>
  )
}
