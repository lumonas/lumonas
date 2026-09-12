import { useState } from 'react'
import { DatabaseBackup, Download, FileCheck, ShieldAlert } from 'lucide-react'
import {
  useExportRecovery,
  useRecoveryPlan,
  useRecoveryStatus,
  useStageRestore,
} from '@/api/queries'
import { AlertBanner } from '@/components/core/alert-banner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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
  const { data: status } = useRecoveryStatus()
  const { data: plan, isLoading: planLoading, error: planError } = useRecoveryPlan()
  const exportBundle = useExportRecovery()
  const stage = useStageRestore()
  const [stageOpen, setStageOpen] = useState(false)

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
    </div>
  )
}
