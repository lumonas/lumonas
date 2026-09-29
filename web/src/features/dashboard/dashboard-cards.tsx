import { Link } from 'react-router-dom'
import {
  ArrowRight,
  Archive,
  ClipboardCheck,
  CircleCheck,
  Container,
  Cpu,
  HardDrive,
  MemoryStick,
  RefreshCw,
  ShieldCheck,
  Thermometer,
} from 'lucide-react'
import { useActivity, useAlerts, useBackupReadiness, useCreateJob, useDisks, useDockerSummary, useJobs, useMetrics, useProtection, usePools, useServer, useRecoveryStatus, useRestoreDrills, useRestoreDrillSchedule } from '@/api/queries'
import { HealthBadge } from '@/components/core/health-badge'
import { HealthExplanation } from '@/components/core/health-explanation'
import { Metric } from '@/components/core/metric'
import { Sparkline } from '@/components/core/sparkline'
import { StorageUsage } from '@/components/core/storage-usage'
import { TimelineEvent } from '@/components/core/timeline-event'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useCurrentTime } from '@/hooks/useCurrentTime'
import { useMetricsStore } from '@/stores/metrics'
import { formatBytes, formatUptime, timeAgo } from '@/lib/format'
import type { Alert } from '@/api/types'
import { useLiveConnection } from '@/stores/live-connection'

export function HealthCard() {
  const { data: server } = useServer()
  const { data: alerts } = useAlerts()
  const metricsQuery = useMetrics()
  const liveState = useLiveConnection((state) => state.state)
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
        <p className="mt-2 text-[11px] text-muted-foreground" role="status">
          {liveState === 'live'
            ? 'Metrics are updating live.'
            : metricsQuery.dataUpdatedAt
              ? `Last metrics snapshot ${timeAgo(new Date(metricsQuery.dataUpdatedAt).toISOString())}.`
              : 'Waiting for the first metrics snapshot.'}
        </p>
        <div className="mt-5 border-t pt-5">
          <HealthExplanation />
        </div>
      </CardContent>
    </Card>
  )
}

const SEVERITY_ORDER = { critical: 0, warning: 1, attention: 2, info: 3 } as const

function alertDestination(alert: Alert) {
  const resource = alert.resource
  if (resource?.type === 'disk' && resource.id) return `/storage?disk=${encodeURIComponent(resource.id)}`
  if (resource?.type.startsWith('docker') || resource?.type === 'container' || resource?.type === 'stack') return '/docker'
  if (resource?.type === 'backup') return '/backups?tab=recovery'
  if (resource?.type === 'network') return '/network'
  if (resource?.type === 'share' && resource.id) return `/shares?share=${encodeURIComponent(resource.id)}`
  if (resource?.type === 'job') return '/monitoring?tab=jobs'
  return '/monitoring?tab=alerts'
}

export function NeedsAttentionCard() {
  const { data: alerts } = useAlerts()
  const firing = ((alerts ?? []).filter((a) => a.state === 'firing') as Alert[]).sort(
    (a, b) => SEVERITY_ORDER[a.severity] - SEVERITY_ORDER[b.severity],
  )

  return (
    <Card className="flex flex-col">
      <CardHeader className="flex-row items-center justify-between pb-3">
        <CardTitle className="text-sm font-medium text-muted-foreground">Needs attention</CardTitle>
        <Button size="sm" variant="ghost" className="h-7 text-xs" asChild>
          <Link to="/monitoring?tab=alerts">All alerts <ArrowRight /></Link>
        </Button>
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
              <Button size="sm" variant="outline" className="h-7 shrink-0 text-xs" asChild>
                <Link to={alertDestination(alert)}>
                  {alert.resource?.type === 'disk' ? 'Inspect disk' : 'Review'}
                </Link>
              </Button>
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
        {!docker?.available && docker ? (
          <p className="rounded-lg border border-attention/40 bg-attention/5 px-3 py-2 text-xs text-muted-foreground">
            Docker Engine is unavailable. Start <span className="font-mono">docker.service</span> to refresh app status.
          </p>
        ) : null}
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

export function WhatChangedCard() {
  const { data: activity } = useActivity()
  const changes = (activity ?? []).filter((event) => ['config', 'security', 'update', 'network'].includes(event.category)).slice(0, 4)
  return <Card>
    <CardHeader className="flex-row items-center justify-between space-y-0 pb-4"><CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground"><Archive className="size-4" />What changed?</CardTitle><Button variant="ghost" size="sm" className="h-7 text-xs" asChild><Link to="/monitoring?tab=timeline">Timeline <ArrowRight /></Link></Button></CardHeader>
    <CardContent className="flex flex-col gap-3">{changes.length === 0 ? <p className="py-2 text-sm text-muted-foreground">No configuration or security changes recorded yet.</p> : changes.map((event) => <TimelineEvent key={event.id} event={event} />)}</CardContent>
  </Card>
}

export function SafeNASChecklistCard() {
  const { data: readiness } = useBackupReadiness()
  const { data: protection } = useProtection()
  const { data: disks } = useDisks()
  const { data: recovery } = useRecoveryStatus()
  const { data: drills } = useRestoreDrills()
  const { data: drillSchedule } = useRestoreDrillSchedule()
  const now = useCurrentTime()
  const rpoHours = drillSchedule?.rpoHours ?? 24
  const bundleAgeHours = recovery?.manifest?.createdAt ? (now - Date.parse(recovery.manifest.createdAt)) / 3_600_000 : null
  const latestDrill = (drills ?? []).find((drill) => drill.state === 'successful' && drill.generation === recovery?.manifest?.generation)
  const drillCurrent = !!latestDrill?.finishedAt && now - Date.parse(latestDrill.finishedAt) <= (drillSchedule?.intervalSeconds ?? 604800) * 1000
  const smartCurrent = (disks ?? []).length > 0 && (disks ?? []).every((disk) => {
    const test = disk.smart.lastTest
    return !!test && test.result === 'passed' && now - Date.parse(test.at) <= 90 * 86_400_000
  })
  const scrubCurrent = !!protection?.lastScrubAt && now - Date.parse(protection.lastScrubAt) <= 30 * 86_400_000
  const recoveryChecks = (readiness?.layers ?? []).map((layer) => ({
    label: layer.label,
    detail: layer.detail,
    complete: layer.status === 'current',
    link: layer.id.includes('destination') || layer.id.includes('key') ? '/backups?tab=destinations' : '/backups?tab=recovery',
  }))
  const checks = [
    ...recoveryChecks,
    { label: `Recovery point within ${rpoHours}h RPO`, detail: bundleAgeHours == null ? 'No verified recovery bundle is available' : `Latest bundle is ${bundleAgeHours.toFixed(1)}h old`, complete: bundleAgeHours != null && bundleAgeHours <= rpoHours, link: '/backups?tab=recovery' },
    { label: 'Latest recovery generation passed a canary restore', detail: drillCurrent ? `Last successful drill ${timeAgo(latestDrill!.finishedAt!)}` : 'Run a restore drill for the current bundle generation', complete: drillCurrent, link: '/backups?tab=recovery' },
    { label: 'Parity scrub completed in the last 30 days', detail: protection?.lastScrubAt ? `Last scrub ${timeAgo(protection.lastScrubAt)}` : 'No scrub is recorded', complete: scrubCurrent, link: '/storage?tab=protection' },
    { label: 'SMART tests passed in the last 90 days', detail: smartCurrent ? 'All discovered disks have a recent passing test' : 'At least one disk has no recent passing SMART test', complete: smartCurrent, link: '/storage?tab=disks' },
    { label: 'Parity protection healthy', complete: protection?.status === 'healthy', link: '/storage?tab=protection' },
    { label: 'All discovered disks healthy', complete: (disks ?? []).length > 0 && (disks ?? []).every((disk) => disk.health === 'healthy'), link: '/storage?tab=disks' },
  ].sort((a, b) => Number(a.complete) - Number(b.complete))
  const complete = checks.filter((check) => check.complete).length
  return <Card>
    <CardHeader className="flex-row items-center justify-between space-y-0 pb-4"><CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground"><ClipboardCheck className="size-4" />Safe NAS checklist</CardTitle><span className="text-xs text-muted-foreground">{complete}/{checks.length}</span></CardHeader>
    <CardContent className="space-y-2">{checks.map((check) => <Link key={check.label} to={check.link} className="flex items-start gap-2 rounded-md px-1 py-1 text-sm hover:bg-muted/60"><CircleCheck className={check.complete ? 'mt-0.5 size-4 shrink-0 text-success' : 'mt-0.5 size-4 shrink-0 text-muted-foreground'} /><span className={check.complete ? '' : 'text-muted-foreground'}>{check.label}{'detail' in check && check.detail ? <span className="mt-0.5 block text-xs text-muted-foreground">{check.detail}</span> : null}</span>{!check.complete && <ArrowRight className="ml-auto mt-0.5 size-3.5 shrink-0 text-muted-foreground" />}</Link>)}</CardContent>
  </Card>
}
