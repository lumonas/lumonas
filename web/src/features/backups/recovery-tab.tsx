import { useState } from 'react'
import { ArrowDownToLine, DatabaseBackup, Download, FileCheck, ShieldAlert } from 'lucide-react'
import {
  useExportRecovery,
  useDownloadRecovery,
  useRecoveryPlan,
  useRecoveryStatus,
  useRestoreDrills,
  useRestoreDrillSchedule,
  useUpdateRestoreDrillSchedule,
  useRunRestoreDrill,
  useStageRestore,
  useStageAppdata,
  useDockerStacks,
  useWorkloadRecoveryObjectives,
  useSetWorkloadRecoveryObjective,
  useBackupDestinations,
  useBackupRuns,
  useRestoreBackupVirtualMachine,
} from '@/api/queries'
import { AlertBanner } from '@/components/core/alert-banner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useCurrentTime } from '@/hooks/useCurrentTime'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { formatDateTime } from '@/lib/format'
import type { RecoveryPlan as RecoveryPlanType } from '@/api/types'

function CheckRow({ label, ok }: { label: string; ok: boolean }) {
  return (
    <div className="flex items-center justify-between rounded-md border px-3 py-2">
      <span className="text-sm">{label}</span>
      <Badge variant={ok ? 'success' : 'critical'}>{ok ? 'pass' : 'fail'}</Badge>
    </div>
  )
}

function PlanWarnings({ plan }: { plan: RecoveryPlanType }) {
  if (!plan.warnings || plan.warnings.length === 0) return null
  return (
    <AlertBanner tone="warning" title="Restore plan warnings">
      <ul className="list-inside list-disc">
        {plan.warnings.map((warning) => (
          <li key={warning}>{warning}</li>
        ))}
      </ul>
    </AlertBanner>
  )
}

export function RecoveryTab() {
  const now = useCurrentTime()
  const { data: status } = useRecoveryStatus()
  const { data: plan, isLoading: planLoading, error: planError } = useRecoveryPlan()
  const exportBundle = useExportRecovery()
  const downloadBundle = useDownloadRecovery()
  const stage = useStageRestore()
  const stageAppdata = useStageAppdata()
  const { data: drills } = useRestoreDrills()
  const { data: drillSchedule } = useRestoreDrillSchedule()
  const updateDrillSchedule = useUpdateRestoreDrillSchedule()
  const runDrill = useRunRestoreDrill()
  const { data: stacks } = useDockerStacks()
  const { data: workloadObjectives } = useWorkloadRecoveryObjectives()
  const saveWorkloadObjective = useSetWorkloadRecoveryObjective()
  const { data: backupDestinations } = useBackupDestinations()
  const { data: backupRuns } = useBackupRuns()
  const restoreBackupVM = useRestoreBackupVirtualMachine()
  const [workloadDrafts, setWorkloadDrafts] = useState<Record<string, { rpoHours: number; rtoMinutes: number }>>({})
  const [stageOpen, setStageOpen] = useState(false)
  const [rpoDraft, setRpoDraft] = useState<number | null>(null)
  const [rtoDraft, setRtoDraft] = useState<number | null>(null)
  const [restoreDestinationId, setRestoreDestinationId] = useState('')
  const [restoreRunId, setRestoreRunId] = useState('')
  const [restoreVMName, setRestoreVMName] = useState('')
  const [restoreConfirmName, setRestoreConfirmName] = useState('')
  const [restoreVMOpen, setRestoreVMOpen] = useState(false)
  const activeDestinationId = restoreDestinationId || backupDestinations?.find((item) => item.enabled)?.id || ''
  const eligibleRuns = (backupRuns ?? []).filter((entry) => entry.run.state === 'verified' && entry.copies.some((copy) => copy.destinationId === activeDestinationId && copy.verified && copy.object === `virtual-machines/${entry.run.id}/manifest.json`))
  const activeRunId = eligibleRuns.some((entry) => entry.run.id === restoreRunId) ? restoreRunId : eligibleRuns[0]?.run.id ?? ''
  const activeRun = eligibleRuns.find((entry) => entry.run.id === activeRunId)
  const activeRunCopies = activeRun?.copies ?? []
  const backedUpVMs = [...new Set(activeRunCopies.filter((copy) => {
    const parts = copy.object.split('/')
    if (copy.destinationId !== activeDestinationId || copy.runId !== activeRunId || !copy.verified || parts.length !== 5 || parts[0] !== 'virtual-machines' || parts[1] !== activeRunId || parts[3] !== 'disks' || !parts[4].endsWith('.qcow2.sparse')) return false
    const name = parts[2]
    return activeRunCopies.some((definition) => definition.destinationId === activeDestinationId && definition.runId === activeRunId && definition.verified && definition.object === `virtual-machines/${activeRunId}/${name}/definition.xml`)
  }).map((copy) => copy.object.split('/')[2]).filter(Boolean))]
  const latestPassingDrill = [...(drills ?? [])]
    .filter((drill) => drill.state === 'successful' && drill.generation === status?.manifest?.generation)
    .sort((a, b) => Date.parse(b.finishedAt ?? '') - Date.parse(a.finishedAt ?? ''))[0]
  const drillAgeDays = latestPassingDrill?.finishedAt
    ? Math.floor((now - Date.parse(latestPassingDrill.finishedAt)) / 86_400_000)
    : null
  const bundleAgeHours = status?.manifest?.createdAt ? Math.max(0, (now - Date.parse(status.manifest.createdAt)) / 3_600_000) : null
  const latestDrillDurationMinutes = latestPassingDrill?.finishedAt
    ? Math.max(0, (Date.parse(latestPassingDrill.finishedAt) - Date.parse(latestPassingDrill.startedAt)) / 60_000)
    : null

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardHeader className="flex-row items-center justify-between space-y-0 pb-3">
          <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
            <DatabaseBackup className="size-4" />
            Recovery bundle
          </CardTitle>
          {status ? (
            <Badge variant={status.configured ? (status.verified ? 'success' : 'warning') : 'offline'}>
              {status.configured ? (status.verified ? 'verified' : 'unverified') : 'not configured'}
            </Badge>
          ) : null}
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          {status?.warnings && status.warnings.length > 0 ? (
            <AlertBanner tone="attention" title="Bundle warnings">
              <ul className="list-inside list-disc">
                {status.warnings.map((warning) => (
                  <li key={warning}>{warning}</li>
                ))}
              </ul>
            </AlertBanner>
          ) : null}
          <p className="text-sm text-muted-foreground">
            {status?.configured && status.manifest
              ? `Latest bundle: ${status.latestPath} · generation ${status.manifest.generation} · created ${formatDateTime(status.manifest.createdAt)}`
              : 'No recovery bundle yet. Export one before installing updates or making risky storage changes.'}
          </p>
          <div className="flex flex-wrap gap-2">
            <Button size="sm" onClick={() => exportBundle.mutate()} disabled={exportBundle.isPending}>
              <Download />
              {exportBundle.isPending ? 'Exporting…' : 'Export bundle now'}
            </Button>
            <Button size="sm" variant="outline" onClick={() => downloadBundle.mutate()} disabled={!status?.verified || downloadBundle.isPending}>
              <ArrowDownToLine />
              {downloadBundle.isPending ? 'Preparing…' : 'Download for recovery'}
            </Button>
            <Button
              size="sm"
              variant="outline"
              onClick={() => setStageOpen(true)}
              disabled={!status?.configured}
            >
              <ShieldAlert />
              Stage restore
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-sm">Restore a virtual machine</CardTitle>
          <CardDescription>Install a VM from a verified remote backup generation. The guest stays stopped until you review it and start it.</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3 sm:grid-cols-3">
          <label className="grid gap-1 text-xs text-muted-foreground">Backup destination
            <select aria-label="VM backup destination" className="h-9 rounded-md border bg-background px-3 text-sm text-foreground" value={activeDestinationId} onChange={(event) => { setRestoreDestinationId(event.target.value); setRestoreRunId(''); setRestoreVMName('') }}>
              {(backupDestinations ?? []).filter((item) => item.enabled).map((item) => <option key={item.id} value={item.id}>{item.label}</option>)}
            </select>
          </label>
          <label className="grid gap-1 text-xs text-muted-foreground">Verified generation
            <select aria-label="VM backup generation" className="h-9 rounded-md border bg-background px-3 text-sm text-foreground" value={activeRunId} onChange={(event) => { setRestoreRunId(event.target.value); setRestoreVMName('') }}>
              {eligibleRuns.map((entry) => <option key={entry.run.id} value={entry.run.id}>Generation {entry.run.generation} · {formatDateTime(entry.run.startedAt)}</option>)}
            </select>
          </label>
          <div className="flex items-end gap-2">
            <label className="grid min-w-0 flex-1 gap-1 text-xs text-muted-foreground">Virtual machine
              <select aria-label="VM to restore" className="h-9 rounded-md border bg-background px-3 text-sm text-foreground" value={restoreVMName} onChange={(event) => setRestoreVMName(event.target.value)}>
                <option value="">Select a VM</option>{backedUpVMs.map((name) => <option key={name} value={name}>{name}</option>)}
              </select>
            </label>
            <Button size="sm" disabled={!restoreVMName || restoreBackupVM.isPending} onClick={() => { setRestoreConfirmName(''); restoreBackupVM.reset(); setRestoreVMOpen(true) }}>Restore…</Button>
          </div>
          {!eligibleRuns.length ? <p className="text-xs text-muted-foreground sm:col-span-3">No verified VM backup generation is available for this destination.</p> : null}
          {restoreBackupVM.isError ? <AlertBanner tone="critical" title="VM restore failed">{restoreBackupVM.error.message}</AlertBanner> : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-sm font-medium">App data coverage</CardTitle>
          <CardDescription>
            Compare each stack’s configured recovery paths and database dumps with the latest verified bundle.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-2">
          {planLoading ? (
            <p className="text-sm text-muted-foreground">Checking bundle coverage…</p>
          ) : planError ? (
            <AlertBanner tone="warning" title="App data coverage is unavailable">
              Export a recovery bundle to check which app data paths it contains.
            </AlertBanner>
          ) : (stacks ?? []).length === 0 ? (
            <p className="text-sm text-muted-foreground">No Docker stacks are installed.</p>
          ) : (
            (stacks ?? []).map((stack) => {
              const configured = stack.recovery?.appdataPaths ?? []
              const archived = new Set(
                (plan?.appdata ?? [])
                  .filter((record) => record.stack === stack.name)
                  .map((record) => record.containerPath),
              )
              const databaseDumps = (plan?.databaseDumps ?? []).filter((record) => record.stack === stack.name)
              const covered = configured.filter((path) => archived.has(path)).length
              const complete = configured.length > 0 && covered === configured.length
              const hasRecoveryPayload = archived.size > 0 || databaseDumps.length > 0
              return (
                <div key={stack.id} className="flex flex-wrap items-center justify-between gap-3 rounded-md border px-3 py-2">
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium">{stack.name}</p>
                    {configured.length > 0 ? (
                      <p className="mt-1 text-xs text-muted-foreground">
                        {covered} of {configured.length} configured path{configured.length === 1 ? '' : 's'} in latest bundle
                        {configured.filter((path) => !archived.has(path)).length > 0
                          ? ` · missing ${configured.filter((path) => !archived.has(path)).join(', ')}`
                          : ''}
                      </p>
                    ) : (
                      <p className="mt-1 text-xs text-muted-foreground">No app data recovery paths configured</p>
                    )}
                    {databaseDumps.length > 0 ? (
                      <p className="mt-1 text-xs text-muted-foreground">
                        Database dump{databaseDumps.length === 1 ? '' : 's'} included: {databaseDumps.map((record) => record.container).join(', ')}
                      </p>
                    ) : stack.recovery?.dbDump ? (
                      <p className="mt-1 text-xs text-warning">Database dump is configured but missing from the latest bundle</p>
                    ) : null}
                  </div>
                  <div className="flex items-center gap-2">
                    <Badge variant={complete && (!stack.recovery?.dbDump || databaseDumps.length > 0) ? 'success' : configured.length === 0 && !stack.recovery?.dbDump ? 'offline' : 'warning'}>
                      {complete && (!stack.recovery?.dbDump || databaseDumps.length > 0) ? 'covered' : configured.length === 0 && !stack.recovery?.dbDump ? 'not configured' : 'incomplete'}
                    </Badge>
                    {hasRecoveryPayload ? (
                      <Button size="sm" variant="outline" disabled={stageAppdata.isPending} onClick={() => {
                        if (window.confirm(`Stage only ${stack.name}'s verified app data and Compose file for offline recovery? The NAS will not be modified.`)) {
                          stageAppdata.mutate(stack.name)
                        }
                      }}>
                        {stageAppdata.isPending ? 'Staging…' : 'Stage app data'}
                      </Button>
                    ) : null}
                  </div>
                </div>
              )
            })
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex-row items-center justify-between space-y-0 pb-3">
          <div>
            <CardTitle className="text-sm font-medium">Restore drills</CardTitle>
            <CardDescription>Test the latest bundle in an isolated temporary root.</CardDescription>
          </div>
          <Button size="sm" variant="outline" onClick={() => runDrill.mutate()} disabled={runDrill.isPending || drills?.some((drill) => drill.state === 'running')}>
            {runDrill.isPending || drills?.some((drill) => drill.state === 'running') ? 'Drill running…' : 'Run drill now'}
          </Button>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          {!status?.manifest ? (
            <AlertBanner tone="warning" title="No recovery bundle is available to verify">
              Export a recovery bundle before running a drill.
            </AlertBanner>
          ) : !latestPassingDrill ? (
            <AlertBanner tone="warning" title="The latest bundle has not passed a restore drill">
              Run a restore drill for generation {status.manifest.generation} to confirm this recovery point.
            </AlertBanner>
          ) : drillAgeDays != null && drillAgeDays > 7 ? (
            <AlertBanner tone="attention" title="Restore drill is out of date">
              The last passing drill was {drillAgeDays} days ago. Run another to confirm the current recovery bundle.
            </AlertBanner>
          ) : (
            <p className="text-xs text-success">Last passing drill: {formatDateTime(latestPassingDrill.finishedAt!)}.</p>
          )}
          <p className="text-xs text-muted-foreground">
            Schedule: {drillSchedule?.enabled ? `every ${Math.round((drillSchedule.intervalSeconds ?? 604800) / 86400)} days` : 'disabled'}
            {drillSchedule?.nextDueAt ? ` · next ${formatDateTime(drillSchedule.nextDueAt)}` : ''}
          </p>
          <div className="grid gap-3 rounded-lg border p-3 sm:grid-cols-2">
            <label className="space-y-1 text-xs text-muted-foreground">RPO target (hours)
              <input aria-label="RPO target hours" type="number" min={1} max={8760} className="h-9 w-full rounded-md border bg-background px-3 text-sm text-foreground" value={rpoDraft ?? drillSchedule?.rpoHours ?? 24} onChange={(event) => setRpoDraft(Number(event.target.value))} />
              <span className="block">Latest recovery point: {bundleAgeHours == null ? 'unknown' : `${bundleAgeHours.toFixed(1)}h old`} · target {drillSchedule?.rpoHours ?? 24}h · {bundleAgeHours == null ? 'not measured' : bundleAgeHours <= (drillSchedule?.rpoHours ?? 24) ? 'within target' : 'outside target'}</span>
            </label>
            <label className="space-y-1 text-xs text-muted-foreground">RTO target (minutes)
              <input aria-label="RTO target minutes" type="number" min={1} max={10080} className="h-9 w-full rounded-md border bg-background px-3 text-sm text-foreground" value={rtoDraft ?? drillSchedule?.rtoMinutes ?? 60} onChange={(event) => setRtoDraft(Number(event.target.value))} />
              <span className="block">Latest canary restore: {latestDrillDurationMinutes == null ? 'not measured' : `${latestDrillDurationMinutes.toFixed(1)}m`} · target {drillSchedule?.rtoMinutes ?? 60}m · {latestDrillDurationMinutes == null ? 'not measured' : latestDrillDurationMinutes <= (drillSchedule?.rtoMinutes ?? 60) ? 'within target' : 'outside target'}</span>
            </label>
          </div>
          <Button size="sm" variant="outline" className="self-start" disabled={updateDrillSchedule.isPending || (rpoDraft == null && rtoDraft == null)} onClick={() => updateDrillSchedule.mutate({ ...(rpoDraft != null ? { rpoHours: Math.min(8760, Math.max(1, rpoDraft)) } : {}), ...(rtoDraft != null ? { rtoMinutes: Math.min(10080, Math.max(1, rtoDraft)) } : {}) }, { onSuccess: () => { setRpoDraft(null); setRtoDraft(null) } })}>Save recovery objectives</Button>
          {(stacks ?? []).length > 0 ? (
            <div className="rounded-lg border p-3">
              <p className="text-sm font-medium">Workload recovery objectives</p>
              <p className="mb-3 mt-1 text-xs text-muted-foreground">Targets are checked against bundle app-data coverage and the latest isolated restore drill.</p>
              <div className="grid gap-2">
                {(stacks ?? []).map((stack) => {
                  const objective = workloadObjectives?.find((item) => item.workloadId === stack.name)
                  const draft = workloadDrafts[stack.name]
                  const rpo = draft?.rpoHours ?? objective?.rpoHours ?? drillSchedule?.rpoHours ?? 24
                  const rto = draft?.rtoMinutes ?? objective?.rtoMinutes ?? drillSchedule?.rtoMinutes ?? 60
                  return <div key={stack.id} className="flex flex-wrap items-end gap-2 rounded-md border px-3 py-2">
                    <span className="min-w-32 flex-1 truncate text-sm font-medium">{stack.name}</span>
                    <label className="text-xs text-muted-foreground">RPO h<input aria-label={`${stack.name} RPO hours`} type="number" min={1} max={8760} className="mt-1 h-8 w-20 rounded-md border bg-background px-2 text-foreground" value={rpo} onChange={(event) => setWorkloadDrafts((current) => ({ ...current, [stack.name]: { rpoHours: Number(event.target.value), rtoMinutes: rto } }))} /></label>
                    <label className="text-xs text-muted-foreground">RTO min<input aria-label={`${stack.name} RTO minutes`} type="number" min={1} max={10080} className="mt-1 h-8 w-24 rounded-md border bg-background px-2 text-foreground" value={rto} onChange={(event) => setWorkloadDrafts((current) => ({ ...current, [stack.name]: { rpoHours: rpo, rtoMinutes: Number(event.target.value) } }))} /></label>
                    <Button size="sm" variant="outline" disabled={saveWorkloadObjective.isPending} onClick={() => saveWorkloadObjective.mutate({ workloadId: stack.name, rpoHours: Math.min(8760, Math.max(1, rpo)), rtoMinutes: Math.min(10080, Math.max(1, rto)), updatedAt: new Date().toISOString() })}>Save</Button>
                  </div>
                })}
              </div>
            </div>
          ) : null}
          {(drills ?? []).slice(0, 5).map((drill) => (
            <div key={drill.id} className="flex items-center justify-between gap-3 rounded-md border px-3 py-2">
              <div className="min-w-0">
                <p className="text-sm font-medium">{drill.trigger === 'scheduled' ? 'Scheduled drill' : 'Manual drill'}</p>
                <p className="text-xs text-muted-foreground">{drill.finishedAt ? formatDateTime(drill.finishedAt) : 'Running'} · generation {drill.generation || '—'}{drill.state === 'successful' ? ` · database ${drill.databaseRestored ? 'restored' : 'not restored'} · ${drill.appdataRestored?.length ?? 0} appdata volume(s) · ${drill.sharesRestored?.length ?? 0} shares · ${drill.servicesRehearsed?.length ?? 0} services healthy` : ''}</p>
                {drill.error ? <p className="truncate text-xs text-critical">{drill.error}</p> : null}
              </div>
              <Badge variant={drill.state === 'successful' ? 'success' : drill.state === 'failed' ? 'critical' : 'attention'}>{drill.state}</Badge>
            </div>
          ))}
          {!drills?.length ? <p className="text-sm text-muted-foreground">No restore drill has run yet.</p> : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
            <FileCheck className="size-4" />
            Restore plan
          </CardTitle>
          <CardDescription>What a restore from the latest bundle would look like.</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3">
          {planLoading ? (
            <p className="text-sm text-muted-foreground">Verifying bundle…</p>
          ) : planError ? (
            <AlertBanner tone="warning" title="Restore plan unavailable">
              {planError instanceof Error ? planError.message : null}
            </AlertBanner>
          ) : plan ? (
            <>
              <PlanWarnings plan={plan} />
              <div className="grid gap-2 sm:grid-cols-2">
                <CheckRow label="Bundle signature verified" ok={plan.verified} />
                <CheckRow label="Database integrity" ok={plan.databaseValid} />
                <CheckRow label="Desired state (shares, users)" ok={plan.desiredStateValid} />
                <CheckRow label="Compose files parse" ok={plan.composeValid} />
                <CheckRow label="Secrets encrypted" ok={plan.encryptedSecrets} />
              </div>
              <div>
                <p className="mb-2 text-xs font-medium tracking-wide text-muted-foreground uppercase">
                  Files restored ({plan.files.length})
                </p>
                <ul className="flex max-h-56 flex-col gap-1 overflow-auto rounded-md border p-2 font-mono text-xs">
                  {plan.files.map((file) => (
                    <li key={file}>{file}</li>
                  ))}
                </ul>
              </div>
            </>
          ) : null}
        </CardContent>
      </Card>

      <Dialog open={stageOpen} onOpenChange={setStageOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Stage a restore</DialogTitle>
            <DialogDescription>
              Decrypts the latest bundle into a staging directory so a restore can be completed
              from the console. The appliance itself is not modified.
            </DialogDescription>
          </DialogHeader>
          {stage.isError ? (
            <AlertBanner tone="critical" title="Staging failed">
              {stage.error instanceof Error ? stage.error.message : null}
            </AlertBanner>
          ) : null}
          <DialogFooter>
            <Button variant="ghost" onClick={() => setStageOpen(false)}>Cancel</Button>
            <Button
              onClick={() =>
                stage.mutate(undefined, {
                  onSuccess: () => setStageOpen(false),
                })
              }
              disabled={stage.isPending}
            >
              {stage.isPending ? 'Staging…' : 'Stage bundle'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={restoreVMOpen} onOpenChange={(open) => { if (!open && !restoreBackupVM.isPending) setRestoreVMOpen(false) }}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Restore {restoreVMName}?</DialogTitle>
            <DialogDescription>This copies and verifies its disk and definition, registers the guest, and leaves it stopped. Type the VM name to confirm.</DialogDescription>
          </DialogHeader>
          <label className="grid gap-1.5 text-sm">Confirm VM name
            <input aria-label="Confirm VM name" className="h-9 rounded-md border bg-background px-3 font-mono text-sm" value={restoreConfirmName} onChange={(event) => setRestoreConfirmName(event.target.value)} />
          </label>
          {restoreBackupVM.isError ? <AlertBanner tone="critical" title="VM restore failed">{restoreBackupVM.error.message}</AlertBanner> : null}
          <DialogFooter>
            <Button variant="outline" onClick={() => setRestoreVMOpen(false)} disabled={restoreBackupVM.isPending}>Cancel</Button>
            <Button disabled={restoreConfirmName !== restoreVMName || restoreBackupVM.isPending || !activeRunId} onClick={() => restoreBackupVM.mutate({ destinationId: activeDestinationId, runId: activeRunId, name: restoreVMName, confirmName: restoreConfirmName }, { onSuccess: () => setRestoreVMOpen(false) })}>{restoreBackupVM.isPending ? 'Restoring…' : 'Restore stopped VM'}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
