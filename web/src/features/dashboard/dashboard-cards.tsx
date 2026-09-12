import { Link } from 'react-router-dom'
import {
  ArrowRight,
  Archive,
  CircleCheck,
  Container,
  Cpu,
  HardDrive,
  MemoryStick,
  RefreshCw,
  ShieldCheck,
  Thermometer,
} from 'lucide-react'
import { useActivity, useAlerts, useCreateJob, useDisks, useDockerSummary, useJobs, useProtection, usePools, useServer } from '@/api/queries'
import { HealthBadge } from '@/components/core/health-badge'
import { HealthExplanation } from '@/components/core/health-explanation'
import { Metric } from '@/components/core/metric'
import { Sparkline } from '@/components/core/sparkline'
import { StorageUsage } from '@/components/core/storage-usage'
import { TimelineEvent } from '@/components/core/timeline-event'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useMetricsStore } from '@/stores/metrics'
import { formatBytes, formatUptime, timeAgo } from '@/lib/format'
import type { Alert } from '@/api/types'

export function HealthCard() {
  const { data: server } = useServer()
  const { data: alerts } = useAlerts()
  const { metrics, cpuHistory } = useMetricsStore()

  const firing = (alerts ?? []).filter((a) => a.state === 'firing')

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between space-y-0 pb-4">
        <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
          <ShieldCheck className="size-4" />
          Overall health
        </CardTitle>
        {server && <HealthBadge state={server.health} withIcon />}
      </CardHeader>
      <CardContent>
        <div className="grid grid-cols-2 gap-x-6 gap-y-4 sm:grid-cols-4">
          <Metric
            label="Uptime"
            value={formatUptime(metrics.uptimeSeconds)}
            sub={server?.version}
          />
          <Metric
            label="CPU"
            value={<span>{metrics.cpuPercent}%</span>}
            sub={`load ${metrics.load[0].toFixed(2)}`}
          />
          <div className="col-span-2 flex flex-col gap-1">
            <span className="flex items-center gap-1.5 text-xs font-medium tracking-wide text-muted-foreground uppercase">
              <Cpu className="size-3" />
              CPU trend
            </span>
            <Sparkline data={cpuHistory.length > 1 ? cpuHistory : [0, 0]} className="mt-1" />
          </div>
        </div>
        <div className="mt-5 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
          <span className="flex items-center gap-1.5">
            <CircleCheck className={firing.length === 0 ? 'size-3.5 text-success' : 'size-3.5 text-attention'} />
            {firing.length === 0 ? 'No active alerts' : `${firing.length} active alert${firing.length > 1 ? 's' : ''}`}
          </span>
          <span className="flex items-center gap-1.5">
            <Thermometer className="size-3.5" />
            CPU {metrics.cpuTempC}°C
          </span>
          <span className="flex items-center gap-1.5">
            <MemoryStick className="size-3.5" />
            RAM {formatBytes(metrics.ramUsedBytes)} / {formatBytes(metrics.ramTotalBytes)}
          </span>
        </div>
        <div className="mt-5 border-t pt-5">
          <HealthExplanation />
        </div>
      </CardContent>
    </Card>
  )
}

const SEVERITY_ORDER = { critical: 0, warning: 1, attention: 2, info: 3 } as const

export function NeedsAttentionCard() {
  const { data: alerts } = useAlerts()
  const firing = ((alerts ?? []).filter((a) => a.state === 'firing') as Alert[]).sort(
    (a, b) => SEVERITY_ORDER[a.severity] - SEVERITY_ORDER[b.severity],
  )

  return (
    <Card className="flex flex-col">
      <CardHeader className="pb-3">
        <CardTitle className="text-sm font-medium text-muted-foreground">Needs attention</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-1 flex-col gap-3">
        {firing.length === 0 ? (
          <div className="flex flex-1 flex-col items-center justify-center gap-2 py-4 text-center">
            <CircleCheck className="size-8 text-success" />
            <p className="text-sm text-muted-foreground">Nothing needs your attention right now.</p>
          </div>
        ) : (
          firing.map((alert) => (
            <div key={alert.id} className="flex items-start justify-between gap-3 rounded-lg border p-3">
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <Badge variant={alert.severity} className="capitalize">
                    {alert.severity}
                  </Badge>
                  <span className="text-xs text-muted-foreground">{timeAgo(alert.startedAt)}</span>
                </div>
                <p className="mt-1.5 text-sm font-medium leading-snug">{alert.title}</p>
                <p className="mt-0.5 text-xs text-muted-foreground">{alert.description}</p>
              </div>
              {alert.resource?.type === 'disk' && (
                <Button size="sm" variant="outline" className="h-7 shrink-0 text-xs" asChild>
                  <Link to={`/storage?disk=${alert.resource.id}`}>Inspect</Link>
                </Button>
              )}
            </div>
          ))
        )}
      </CardContent>
    </Card>
  )
}

export function SystemCard() {
  const { metrics } = useMetricsStore()
  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between space-y-0 pb-4">
        <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
          <Cpu className="size-4" />
          System
        </CardTitle>
        <Button variant="ghost" size="sm" className="h-7 text-xs" asChild>
          <Link to="/monitoring?tab=overview">
            Details
            <ArrowRight />
          </Link>
        </Button>
      </CardHeader>
      <CardContent className="grid grid-cols-2 gap-4">
        <Metric label="CPU" value={`${metrics.cpuPercent}%`} sub={`load ${metrics.load[0].toFixed(2)}`} />
        <Metric label="CPU temp" value={`${metrics.cpuTempC}°C`} />
        <Metric
          label="Memory"
          value={formatBytes(metrics.ramUsedBytes)}
          sub={`of ${formatBytes(metrics.ramTotalBytes)}`}
        />
        <Metric label="Uptime" value={formatUptime(metrics.uptimeSeconds)} />
      </CardContent>
    </Card>
  )
}

export function StorageSummaryCard() {
  const { data: pools } = usePools()
  const { data: disks } = useDisks()
  const pool = pools?.[0]

  const counts = (disks ?? []).reduce<Record<string, number>>((acc, disk) => {
    acc[disk.health] = (acc[disk.health] ?? 0) + 1
    return acc
  }, {})

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between space-y-0 pb-4">
        <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
          <HardDrive className="size-4" />
          Storage
        </CardTitle>
        <Button variant="ghost" size="sm" className="h-7 text-xs" asChild>
          <Link to="/storage">
            Open
            <ArrowRight />
          </Link>
        </Button>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {pool && (
          <div>
            <div className="mb-2 flex items-center justify-between">
              <span className="text-sm font-medium">{pool.name} pool</span>
              <HealthBadge state={pool.status} />
            </div>
            <StorageUsage usedBytes={pool.usedBytes} totalBytes={pool.sizeBytes} label="Used" />
          </div>
        )}
        <p className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
          <span>{disks?.length ?? 0} disks</span>
          <span aria-hidden>·</span>
          <span className="text-success">{counts.healthy ?? 0} healthy</span>
          {(counts.warning ?? 0) > 0 && <span className="text-warning">{counts.warning} warning</span>}
          {(counts.critical ?? 0) > 0 && (
            <span className="text-critical">{counts.critical} critical</span>
          )}
          {(counts.offline ?? 0) > 0 && <span className="text-offline">{counts.offline} offline</span>}
        </p>
      </CardContent>
    </Card>
  )
}

export function ProtectionCard() {
  const { data: protection } = useProtection()
  const { data: jobs } = useJobs()
  const createJob = useCreateJob()
  const syncJob = jobs?.find((j) => j.type === 'snapraid.sync' && j.state === 'running')

  if (!protection) return null

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between space-y-0 pb-4">
        <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
          <ShieldCheck className="size-4" />
          Protection
        </CardTitle>
        <Button variant="ghost" size="sm" className="h-7 text-xs" asChild>
          <Link to="/storage?tab=protection">
            Details
            <ArrowRight />
          </Link>
        </Button>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <div className="flex items-center justify-between">
          <span className="text-sm">Parity</span>
          <HealthBadge state={protection.status} />
        </div>
        <div className="grid grid-cols-2 gap-4">
          <Metric
            label="Last sync"
            value={protection.lastSyncAt ? timeAgo(protection.lastSyncAt) : '—'}
            sub={protection.syncSchedule}
          />
          <Metric
            label="Unsynced changes"
            value={formatBytes(protection.changesSinceSyncBytes)}
            sub="since last sync"
          />
        </div>
        <Button
          size="sm"
          variant="outline"
          className="w-full"
          disabled={protection.syncRunning}
          onClick={() => createJob.mutate({ type: 'snapraid.sync' })}
        >
          <RefreshCw className={syncJob ? 'animate-spin [animation-duration:2.5s]' : undefined} />
          {protection.syncRunning ? 'Sync running…' : 'Sync now'}
        </Button>
      </CardContent>
    </Card>
  )
}

export function DockerCard() {
  const { data: docker } = useDockerSummary()
  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between space-y-0 pb-4">
        <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
          <Container className="size-4" />
          Docker
        </CardTitle>
        <Button variant="ghost" size="sm" className="h-7 text-xs" asChild>
          <Link to="/docker">
            Open
            <ArrowRight />
          </Link>
        </Button>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <div className="grid grid-cols-2 gap-4">
          <Metric
            label="Containers running"
            value={docker?.appsRunning ?? '—'}
            sub={`${docker?.stacks ?? 0} stacks`}
          />
          <Metric label="Updates" value={docker?.updatesAvailable ?? '—'} sub="images available" />
        </div>
        {(docker?.updatesAvailable ?? 0) > 0 && (
          <Button variant="outline" size="sm" className="w-full" asChild>
            <Link to="/docker?tab=images">Review updates</Link>
          </Button>
        )}
      </CardContent>
    </Card>
  )
}

export function RecentActivityCard() {
  const { data: activity } = useActivity()
  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between space-y-0 pb-4">
        <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
          <Archive className="size-4" />
          Recent activity
        </CardTitle>
        <Button variant="ghost" size="sm" className="h-7 text-xs" asChild>
          <Link to="/monitoring?tab=timeline">
            View all
            <ArrowRight />
          </Link>
        </Button>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {(activity ?? []).slice(0, 5).map((event) => (
          <TimelineEvent key={event.id} event={event} />
        ))}
      </CardContent>
    </Card>
  )
}
