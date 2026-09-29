import { ArchiveRestore, CheckCircle2, CircleAlert, ShieldCheck } from 'lucide-react'
import { useApplyBackupPolicyTemplate, useBackupJobs, useBackupPolicyTemplates, useBackupReadiness, useBackupSchedule, useBackupDestinations, useUpdateBackupSchedule } from '@/api/queries'
import { HealthBadge } from '@/components/core/health-badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { timeAgo } from '@/lib/format'
import { RestoreWizard } from '@/features/backups/restore-wizard'
import { useState } from 'react'
import { cn } from '@/lib/utils'
import { Switch } from '@/components/ui/switch'
import { SafeNASChecklistCard } from '@/features/dashboard/dashboard-cards'

const LAYER_STATE = {
  current: { Icon: CheckCircle2, className: 'text-success' },
  stale: { Icon: CircleAlert, className: 'text-attention' },
  missing: { Icon: CircleAlert, className: 'text-critical' },
} as const

export function BackupsOverviewTab() {
  const { data: readiness } = useBackupReadiness()
  const { data: jobs } = useBackupJobs()
  const { data: schedule } = useBackupSchedule()
  const { data: destinations } = useBackupDestinations()
  const updateSchedule = useUpdateBackupSchedule()
  const { data: templates } = useBackupPolicyTemplates()
  const applyPolicy = useApplyBackupPolicyTemplate()
  const [restoreOpen, setRestoreOpen] = useState(false)
  const [templateID, setTemplateID] = useState('daily')
  const selectedTemplate = (templates ?? []).find((template) => template.id === templateID)

  const recent = (jobs ?? [])
    .filter((j) => j.lastRun)
    .sort((a, b) => (a.lastRun!.at < b.lastRun!.at ? 1 : -1))
    .slice(0, 4)

  return (
    <div className="flex flex-col gap-4">
      <SafeNASChecklistCard />
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-12">
        <Card className="lg:col-span-4">
          <CardHeader className="pb-3">
            <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
              <ShieldCheck className="size-4" />
              Recovery readiness
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col items-center gap-2 py-4">
            <span className="tnum text-5xl font-semibold tracking-tight">
              {readiness?.score ?? '—'}
              <span className="text-2xl text-muted-foreground">%</span>
            </span>
            <p className="max-w-[26ch] text-center text-xs text-muted-foreground">
              A failed system disk should be an inconvenience, not a disaster.
            </p>
            <Button
              size="sm"
              variant="outline"
              className="mt-2"
              onClick={() => setRestoreOpen(true)}
            >
              <ArchiveRestore />
              Disaster recovery…
            </Button>
          </CardContent>
        </Card>

        <Card className="lg:col-span-8">
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              What makes up the score
            </CardTitle>
          </CardHeader>
          <CardContent>
            <ul className="flex flex-col divide-y rounded-lg border">
              {(readiness?.layers ?? []).map((layer) => {
                const { Icon, className } = LAYER_STATE[layer.status]
                return (
                  <li key={layer.id} className="flex items-center justify-between gap-3 px-3 py-2.5">
                    <div className="flex min-w-0 items-center gap-2.5">
                      <Icon className={cn('size-4 shrink-0', className)} />
                      <div className="min-w-0">
                        <p className="truncate text-sm font-medium">{layer.label}</p>
                        <p className="truncate text-xs text-muted-foreground">{layer.detail}</p>
                      </div>
                    </div>
                    <span
                      className={cn(
                        'shrink-0 text-xs font-medium capitalize',
                        layer.status === 'current' && 'text-success',
                        layer.status === 'stale' && 'text-attention',
                        layer.status === 'missing' && 'text-critical',
                      )}
                    >
                      {layer.status}
                    </span>
                  </li>
                )
              })}
            </ul>
            <p className="mt-3 text-xs text-muted-foreground">
              Backup verification means checksum + decryptability — a file existing is not the same
              as a backup being healthy.
            </p>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-sm font-medium text-muted-foreground">Protected data coverage</CardTitle>
        </CardHeader>
        <CardContent>
          {(readiness?.coverage ?? []).length > 0 ? (
            <ul className="flex flex-col divide-y rounded-lg border">
              {readiness?.coverage?.map((item) => {
                const { Icon, className } = LAYER_STATE[item.status]
                return (
                  <li key={`${item.kind}:${item.id}`} className="flex items-center justify-between gap-3 px-3 py-2.5">
                    <div className="flex min-w-0 items-center gap-2.5">
                      <Icon className={cn('size-4 shrink-0', className)} />
                      <div className="min-w-0">
                        <p className="truncate text-sm font-medium">{item.name}</p>
                        <p className="truncate text-xs text-muted-foreground">{item.detail}</p>
                      </div>
                    </div>
                    <div className="shrink-0 text-right">
                      <span className={cn('text-xs font-medium capitalize', item.status === 'current' && 'text-success', item.status === 'stale' && 'text-attention', item.status === 'missing' && 'text-critical')}>
                        {item.status}
                      </span>
                      {item.lastSuccessfulAt ? <p className="text-[11px] text-muted-foreground">{timeAgo(item.lastSuccessfulAt)}</p> : null}
                    </div>
                  </li>
                )
              })}
            </ul>
          ) : (
            <p className="rounded-md border border-dashed p-4 text-center text-sm text-muted-foreground">Share-level recovery coverage will appear after a verified backup.</p>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-sm font-medium">Back up when USB storage connects</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-wrap items-center justify-between gap-3">
          <div className="max-w-2xl">
            <p className="text-sm">Create and verify a recovery copy when a configured USB destination is mounted.</p>
            <p className="mt-1 text-xs text-muted-foreground">
              {(destinations ?? []).some((destination) => destination.type === 'usb' && destination.enabled)
                ? 'Enabled USB destinations are checked once per connection. An unplugged disk will never be treated as a folder on the system disk.'
                : 'Add a local destination under /media, /mnt, or /run/media to enable this option.'}
            </p>
          </div>
          <Switch
            checked={schedule?.onUsbAttach ?? false}
            disabled={updateSchedule.isPending || !(destinations ?? []).some((destination) => destination.type === 'usb' && destination.enabled)}
            onCheckedChange={(onUsbAttach) => updateSchedule.mutate({
              enabled: schedule?.enabled ?? true,
              intervalSeconds: schedule?.intervalSeconds ?? 86_400,
              onUsbAttach,
            })}
            aria-label="Back up when USB storage connects"
          />
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-3"><CardTitle className="text-sm font-medium">Backup policy template</CardTitle></CardHeader>
        <CardContent className="flex flex-wrap items-end justify-between gap-3">
          <div className="min-w-64 flex-1 space-y-2">
            <label htmlFor="backup-policy-template" className="text-xs text-muted-foreground">Choose a schedule and retention baseline</label>
            <select id="backup-policy-template" value={templateID} onChange={(event) => setTemplateID(event.target.value)} className="h-9 w-full rounded-md border bg-background px-2 text-sm text-foreground">
              {(templates ?? []).map((template) => <option key={template.id} value={template.id}>{template.name}</option>)}
            </select>
            <p className="text-xs text-muted-foreground">{selectedTemplate ? `${selectedTemplate.description} Keeps ${selectedTemplate.generations} generations, ${selectedTemplate.daily} daily copies, and ${selectedTemplate.monthly} monthly copies.` : 'Loading policy templates…'}</p>
            <p className="text-[11px] text-muted-foreground">Applying updates every destination at once. Existing LumoNAS prune-protection days and provider Object Lock settings are preserved.</p>
          </div>
          <Button variant="outline" disabled={!selectedTemplate || applyPolicy.isPending} onClick={() => applyPolicy.mutate(templateID)}>{applyPolicy.isPending ? 'Applying…' : 'Apply policy'}</Button>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            Recent backup runs
          </CardTitle>
        </CardHeader>
        <CardContent>
          <ul className="flex flex-col divide-y rounded-lg border">
            {recent.map((job) => (
              <li key={job.id} className="flex items-center justify-between gap-3 px-3 py-2.5">
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{job.name}</p>
                  {job.lastRun?.detail ? (
                    <p className="truncate text-xs text-muted-foreground">{job.lastRun.detail}</p>
                  ) : null}
                </div>
                <div className="flex shrink-0 items-center gap-3">
                  <span className="text-xs text-muted-foreground">
                    {job.lastRun ? timeAgo(job.lastRun.at) : 'never'}
                  </span>
                  <HealthBadge state={job.lastRun?.status ?? 'offline'} />
                </div>
              </li>
            ))}
          </ul>
        </CardContent>
      </Card>

      <RestoreWizard open={restoreOpen} onOpenChange={setRestoreOpen} />
    </div>
  )
}
