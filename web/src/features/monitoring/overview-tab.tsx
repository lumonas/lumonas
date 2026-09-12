import { Cpu, MemoryStick, Network, HardDrive } from 'lucide-react'
import { useServices } from '@/api/queries'
import { HealthBadge } from '@/components/core/health-badge'
import { Sparkline } from '@/components/core/sparkline'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useMetricsStore } from '@/stores/metrics'
import { formatBytes, formatUptime } from '@/lib/format'
import { cn } from '@/lib/utils'

const SERVICE_STATE: Record<string, 'healthy' | 'attention' | 'offline'> = {
  running: 'healthy',
  degraded: 'attention',
  stopped: 'offline',
}

export function MonitoringOverviewTab() {
  const { metrics, cpuHistory, ramHistory, netHistory, diskHistory } = useMetricsStore()
  const { data: services } = useServices()
  const disk = metrics.disk ?? { readMbps: 0, writeMbps: 0 }

  const ramPercent = metrics.ramTotalBytes
    ? Math.round((metrics.ramUsedBytes / metrics.ramTotalBytes) * 100)
    : 0

  return (
    <div className="grid grid-cols-1 gap-4 lg:grid-cols-12">
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:col-span-8">
        <Card>
          <CardHeader className="flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
              <Cpu className="size-4" />
              CPU
            </CardTitle>
            <span className="tnum text-sm font-semibold">{metrics.cpuPercent}%</span>
          </CardHeader>
          <CardContent>
            <Sparkline data={cpuHistory.length > 1 ? cpuHistory : [0, 0]} className="h-12" />
            <p className="mt-2 text-xs text-muted-foreground">
              Load {metrics.load.map((l) => l.toFixed(2)).join(' · ')} · {metrics.cpuTempC}°C
            </p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
              <MemoryStick className="size-4" />
              Memory
            </CardTitle>
            <span className="tnum text-sm font-semibold">{ramPercent}%</span>
          </CardHeader>
          <CardContent>
            <Sparkline data={ramHistory.length > 1 ? ramHistory : [0, 0]} className="h-12" />
            <p className="tnum mt-2 text-xs text-muted-foreground">
              {formatBytes(metrics.ramUsedBytes)} of {formatBytes(metrics.ramTotalBytes)}
            </p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
              <Network className="size-4" />
              Network
            </CardTitle>
            <span className="tnum text-sm font-semibold">
              {metrics.net.downMbps.toFixed(1)} Mbps ↓
            </span>
          </CardHeader>
          <CardContent>
            <Sparkline data={netHistory.length > 1 ? netHistory : [0, 0]} className="h-12" />
            <p className="tnum mt-2 text-xs text-muted-foreground">
              {metrics.net.interface} · ↑ {metrics.net.upMbps.toFixed(1)} Mbps
            </p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
              <HardDrive className="size-4" />
              Disk activity
            </CardTitle>
            <span className="tnum text-sm font-semibold">
              {disk.readMbps.toFixed(0)} MB/s
            </span>
          </CardHeader>
          <CardContent>
            <Sparkline data={diskHistory.length > 1 ? diskHistory : [0, 0]} className="h-12" />
            <p className="tnum mt-2 text-xs text-muted-foreground">
              read {disk.readMbps.toFixed(0)} · write {disk.writeMbps.toFixed(0)}{' '}
              MB/s
            </p>
          </CardContent>
        </Card>
      </div>

      <Card className="lg:col-span-4">
        <CardHeader className="pb-3">
          <CardTitle className="text-sm font-medium text-muted-foreground">Services</CardTitle>
        </CardHeader>
        <CardContent>
          <ul className="flex flex-col divide-y rounded-lg border">
            {(services ?? []).map((service) => (
              <li key={service.id} className="flex items-center justify-between gap-3 px-3 py-2.5">
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{service.name}</p>
                  {service.detail ? (
                    <p className={cn('truncate text-xs text-muted-foreground')}>{service.detail}</p>
                  ) : null}
                </div>
                <HealthBadge state={SERVICE_STATE[service.state]} />
              </li>
            ))}
          </ul>
          <p className="tnum mt-3 text-xs text-muted-foreground">
            Uptime {formatUptime(metrics.uptimeSeconds)}
          </p>
        </CardContent>
      </Card>

      <Card className="lg:col-span-12">
        <CardHeader className="pb-3">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            About this page
          </CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">
            Live values stream from the NAS over SSE and update every few seconds — no page
            refresh needed. Charts are intentionally minimal; detailed per-disk metrics live in
            Storage, per-container metrics in Docker.
          </p>
        </CardContent>
      </Card>
    </div>
  )
}
