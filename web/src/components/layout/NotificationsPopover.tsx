import { useNavigate } from 'react-router-dom'
import { Check } from 'lucide-react'
import { useAcknowledgeAlert, useAlerts } from '@/api/queries'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useNotificationsStore } from '@/stores/notifications'
import { timeAgo } from '@/lib/format'
import { cn } from '@/lib/utils'
import type { AlertSeverity } from '@/api/types'
import { BellIcon } from 'lucide-react'

const SEVERITY_VARIANT: Record<AlertSeverity, 'info' | 'attention' | 'warning' | 'critical'> = {
  info: 'info',
  attention: 'attention',
  warning: 'warning',
  critical: 'critical',
}

export function NotificationsPopover() {
  const navigate = useNavigate()
  const { data: alerts } = useAlerts()
  const { markAllRead, readIds } = useNotificationsStore()
  const ack = useAcknowledgeAlert()

  const firing = (alerts ?? []).filter((a) => a.state === 'firing')
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
              firing.map((alert) => {
                const isUnread = !readIds.has(alert.id)
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
                    <div className="mt-0.5 flex items-center gap-2">
                      {alert.resource?.type === 'disk' && (
                        <Button
                          variant="outline"
                          size="sm"
                          className="h-7 text-xs"
                          onClick={() => {
                            readStore(alert.id)
                            navigate(`/storage?disk=${alert.resource?.id}`)
                          }}
                        >
                          Inspect disk
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
                    </div>
                  </div>
                )
              })
            )}
          </div>
        </ScrollArea>
      </PopoverContent>
    </Popover>
  )
}
