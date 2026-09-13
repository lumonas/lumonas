import { History } from 'lucide-react'
import { useAlertHistory } from '@/api/queries'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { timeAgo } from '@/lib/format'
import type { AlertSeverity } from '@/api/types'

const SEVERITY_VARIANT: Record<AlertSeverity, 'info' | 'attention' | 'warning' | 'critical'> = {
  info: 'info',
  attention: 'attention',
  warning: 'warning',
  critical: 'critical',
}

export function AlertHistoryCard() {
  const { data: history } = useAlertHistory(20)

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
          <History className="size-4" /> Recently resolved
        </CardTitle>
      </CardHeader>
      <CardContent>
        {!history || history.length === 0 ? (
          <p className="text-sm text-muted-foreground">No resolved alerts yet.</p>
        ) : (
          <ul className="flex flex-col divide-y rounded-lg border">
            {history.map((alert) => (
              <li key={alert.id} className="flex items-center justify-between gap-3 px-3 py-2.5">
                <div className="min-w-0">
                  <p className="truncate text-sm">{alert.title}</p>
                  <p className="truncate text-xs text-muted-foreground">
                    Fired {timeAgo(alert.startedAt)} · resolved{' '}
                    {alert.resolvedAt ? timeAgo(alert.resolvedAt) : 'recently'}
                  </p>
                </div>
                <Badge variant={SEVERITY_VARIANT[alert.severity] ?? 'info'}>resolved</Badge>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
