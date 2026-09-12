import { Activity, ArrowDown, ArrowUp, AlertTriangle, Wifi, WifiOff } from 'lucide-react'
import { Sparkline } from '@/components/core/sparkline'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { useMetricsStore } from '@/stores/metrics'

function formatRate(mbps: number): string {
  if (mbps >= 1000) return `${(mbps / 1000).toFixed(1)} Gbps`
  if (mbps >= 1) return `${mbps.toFixed(1)} Mbps`
  return `${(mbps * 1000).toFixed(0)} Kbps`
}

export function NetworkMonitoring() {
  const { metrics, ifaceHistories } = useMetricsStore()
  const ifaces = metrics.netInterfaces ?? []

  if (ifaces.length === 0) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
            <Activity className="size-4" />
            Interface monitoring
          </CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">Waiting for interface metrics…</p>
        </CardContent>
      </Card>
    )
  }

  return (
    <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
      {ifaces.map((iface) => {
        const hist = ifaceHistories[iface.interface] ?? { up: [], down: [] }
        const hasErrors = iface.errorsIn > 0 || iface.errorsOut > 0
        const hasDrops = iface.droppedIn > 0 || iface.droppedOut > 0

        return (
          <Card key={iface.interface}>
            <CardHeader className="flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="flex items-center gap-2 text-sm font-medium">
                {iface.up ? (
                  <Wifi className="size-4 text-success" />
                ) : (
                  <WifiOff className="size-4 text-offline" />
                )}
                {iface.interface}
              </CardTitle>
              <Badge variant={iface.up ? (hasErrors || hasDrops ? 'warning' : 'success') : 'offline'}>
                {iface.up ? 'Up' : 'Down'}
              </Badge>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="grid grid-cols-2 gap-3">
                <div className="flex items-center gap-1.5">
                  <ArrowDown className="size-3.5 text-muted-foreground" />
                  <span className="tnum text-sm font-semibold">{formatRate(iface.downMbps)}</span>
                </div>
                <div className="flex items-center gap-1.5">
                  <ArrowUp className="size-3.5 text-muted-foreground" />
                  <span className="tnum text-sm font-semibold">{formatRate(iface.upMbps)}</span>
                </div>
              </div>

              <Sparkline
                data={hist.down.length > 1 ? hist.down : [0, 0]}
                className="h-10"
              />

              {(hasErrors || hasDrops) && (
                <div className="flex items-center gap-1.5 text-xs text-warning">
                  <AlertTriangle className="size-3" />
                  <span>
                    {iface.errorsIn + iface.errorsOut} err · {iface.droppedIn + iface.droppedOut} drop
                  </span>
                </div>
              )}

              {!hasErrors && !hasDrops && (
                <p className="text-xs text-muted-foreground">
                  No errors or drops
                </p>
              )}
            </CardContent>
          </Card>
        )
      })}
    </div>
  )
}
