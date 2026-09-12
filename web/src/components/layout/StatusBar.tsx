import { useEffect } from 'react'
import { ArrowDown, ArrowUp, Cpu, MemoryStick } from 'lucide-react'
import { useMetrics } from '@/api/queries'
import { HealthBadge } from '@/components/core/health-badge'
import { useServer } from '@/api/queries'
import { useMetricsStore } from '@/stores/metrics'
import { formatBytes, formatUptime } from '@/lib/format'
import { cn } from '@/lib/utils'

export function StatusBar({ className }: { className?: string }) {
  const { data: server } = useServer()
  const { data: initial } = useMetrics()
  const { metrics } = useMetricsStore()
  const setMetrics = useMetricsStore((s) => s.setMetrics)

  useEffect(() => {
    if (initial) setMetrics(initial)
  }, [initial, setMetrics])

  const ramPercent = metrics.ramTotalBytes
    ? Math.round((metrics.ramUsedBytes / metrics.ramTotalBytes) * 100)
    : 0

  return (
    <footer
      className={cn(
        'flex h-8 shrink-0 items-center gap-4 border-t bg-background/80 px-4 text-xs text-muted-foreground backdrop-blur md:px-6',
        className,
      )}
    >
      {server && <HealthBadge state={server.health} className="py-0 text-[11px]" />}
      <span className="tnum flex items-center gap-1.5">
        <Cpu className="size-3" />
        CPU {metrics.cpuPercent}%
      </span>
      <span className="tnum flex items-center gap-1.5">
        <MemoryStick className="size-3" />
        RAM {formatBytes(metrics.ramUsedBytes)} / {formatBytes(metrics.ramTotalBytes)} ({ramPercent}%)
      </span>
      <span className="tnum ml-auto hidden items-center gap-1.5 lg:flex">
        {metrics.net.interface}
        <ArrowUp className="size-3" />
        {metrics.net.upMbps.toFixed(1)}
        <ArrowDown className="size-3" />
        {metrics.net.downMbps.toFixed(1)} Mbps
      </span>
      <span className="tnum hidden sm:inline">up {formatUptime(metrics.uptimeSeconds)}</span>
    </footer>
  )
}
