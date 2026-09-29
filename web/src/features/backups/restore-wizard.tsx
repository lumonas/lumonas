import { useState } from 'react'
import { ArrowDownToLine, CheckCircle2, FileDown } from 'lucide-react'
import { useDownloadRecovery, useRestorePlan } from '@/api/queries'
import { AlertBanner } from '@/components/core/alert-banner'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'

const STEPS = ['Inspect', 'Plan', 'Approve'] as const

export function RestoreWizard({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogTitle className="sr-only">Disaster recovery</DialogTitle>
        {open ? <WizardBody onClose={() => onOpenChange(false)} /> : null}
      </DialogContent>
    </Dialog>
  )
}

function WizardBody({ onClose }: { onClose: () => void }) {
  const { data: plan } = useRestorePlan()
  const downloadBundle = useDownloadRecovery()
  const [step, setStep] = useState(0)
  const [mapping, setMapping] = useState<Record<string, string>>({})
  const [apps, setApps] = useState<Record<string, boolean>>({})
  const [acknowledged, setAcknowledged] = useState(false)

  if (!plan) {
    return (
      <div className="py-6 text-center text-sm text-muted-foreground">
        Could not load the restore plan.
      </div>
    )
  }

  const currentPlan = plan
  const unmapped = plan.interfaces.filter((iface) => !mapping[iface.old]).length

  function downloadMigrationChecklist() {
    const content = {
      format: 'lumonas-migration-checklist-v1',
      createdAt: new Date().toISOString(),
      recoveryGeneration: currentPlan.generationId,
      networkInterfaceMapping: mapping,
      stacksToReinstall: currentPlan.apps.filter((app) => apps[app.name] ?? app.appdataAvailable).map((app) => ({ name: app.name, appdataAvailable: app.appdataAvailable })),
      dataDiskNote: currentPlan.dataDisksNote,
      execution: 'Use the replacement-system recovery environment. This checklist records operator choices; the running NAS is not changed by this wizard.',
    }
    const blob = new Blob([JSON.stringify(content, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `lumonas-migration-generation-${currentPlan.generationId}.json`
    link.click()
    URL.revokeObjectURL(url)
  }

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h2 className="text-lg font-semibold leading-none">Replacement NAS migration</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          Restores system configuration and apps onto a fresh system disk. Data disks are imported
          — never erased.
        </p>
      </div>

      <ol className="flex items-center gap-2" aria-label="Steps">
        {STEPS.map((label, index) => (
          <li key={label} className="flex flex-1 items-center gap-2">
            <span
              className={cn(
                'flex size-5 shrink-0 items-center justify-center rounded-full border text-[11px] font-semibold',
                index < step && 'border-success bg-success/15 text-success',
                index === step && 'border-primary bg-primary/15 text-primary',
                index > step && 'border-border text-muted-foreground',
              )}
            >
              {index < step ? <CheckCircle2 className="size-3.5" /> : index + 1}
            </span>
            <span
              className={cn(
                'text-xs',
                index === step ? 'font-medium text-foreground' : 'text-muted-foreground',
              )}
            >
              {label}
            </span>
            {index < STEPS.length - 1 && <span className="h-px flex-1 bg-border" />}
          </li>
        ))}
      </ol>

      {step === 0 && (
        <div className="flex flex-col gap-3">
          <AlertBanner tone="info" title="What a restore does">
            The offline installer writes a fresh system disk, then restores configuration
            generation {plan.generationId}, reinstalls your Docker stacks and re-imports data
            disks after verifying their identities.
          </AlertBanner>
          <ul className="flex flex-col gap-1.5 text-sm">
            <li>• Restore config generation {plan.generationId} (newest verified)</li>
            <li>• Remap network interfaces to the new mainboard</li>
            <li>• Reinstall stacks and restore appdata where a backup exists</li>
            <li>• Import data disks read-only until identities are verified</li>
          </ul>
          <p className="text-xs text-muted-foreground">
            Nothing here touches the data on your storage disks — imports are read-only first.
          </p>
        </div>
      )}

      {step === 1 && (
        <div className="flex max-h-[50vh] flex-col gap-4 overflow-y-auto pr-1">
          <div>
            <p className="mb-2 text-xs font-medium tracking-wide text-muted-foreground uppercase">
              Hardware remapping — old interfaces to new ports
            </p>
            <div className="flex flex-col gap-2">
              {plan.interfaces.map((iface) => (
                <div
                  key={iface.old}
                  className="flex flex-wrap items-center justify-between gap-2 rounded-lg border px-3 py-2.5"
                >
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">{iface.old}</p>
                    <p className="truncate text-xs text-muted-foreground">{iface.detail}</p>
                  </div>
                  <Select
                    value={mapping[iface.old] ?? ''}
                    onValueChange={(v) => setMapping((current) => ({ ...current, [iface.old]: v }))}
                  >
                    <SelectTrigger className="h-8 w-32 font-mono text-xs">
                      <SelectValue placeholder="New port…" />
                    </SelectTrigger>
                    <SelectContent>
                      {iface.options.map((option) => (
                        <SelectItem key={option} value={option}>
                          {option}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              ))}
            </div>
            {unmapped > 0 && (
              <p className="mt-2 text-xs text-attention">
                {unmapped} interface{unmapped === 1 ? '' : 's'} unmapped — they will be skipped and
                can be configured after restore.
              </p>
            )}
          </div>

          <div>
            <p className="mb-2 text-xs font-medium tracking-wide text-muted-foreground uppercase">
              Apps to reinstall
            </p>
            <ul className="flex flex-col divide-y rounded-lg border">
              {plan.apps.map((app) => (
                <li key={app.name} className="flex items-center justify-between gap-3 px-3 py-2">
                  <span className="text-sm">
                    <span className="font-mono text-xs">{app.name}</span>
                    {!app.appdataAvailable && (
                      <span className="ml-2 text-xs text-attention">
                        no appdata backup — config only
                      </span>
                    )}
                  </span>
                  <Switch
                    checked={apps[app.name] ?? app.appdataAvailable}
                    onCheckedChange={(next) =>
                      setApps((current) => ({ ...current, [app.name]: next }))
                    }
                    aria-label={`Reinstall ${app.name}`}
                  />
                </li>
              ))}
            </ul>
          </div>

          <p className="rounded-lg bg-muted/40 px-3 py-2.5 text-xs text-muted-foreground">
            {plan.dataDisksNote}
          </p>
        </div>
      )}

      {step === 2 && (
        <div className="flex flex-col gap-3">
          <ul className="flex flex-col gap-1.5 text-sm">
            <li>• Restore configuration generation {plan.generationId}</li>
            <li>
              • Remap {Object.keys(mapping).length} of {plan.interfaces.length} interfaces
            </li>
            <li>
              • Reinstall{' '}
              {plan.apps.filter((app) => apps[app.name] ?? app.appdataAvailable).length} of{' '}
              {plan.apps.length} apps
            </li>
            <li>• Import all data disks read-only, verify identities, then unfreeze</li>
          </ul>
          <label className="flex cursor-pointer items-start gap-3 rounded-lg border p-3">
            <input
              type="checkbox"
              className="mt-0.5 accent-primary"
              checked={acknowledged}
              onChange={(e) => setAcknowledged(e.target.checked)}
            />
            <span className="text-sm">
              I understand this plan is applied to a <strong>fresh system disk</strong> and that
              data disks stay untouched.
            </span>
          </label>
        </div>
      )}

      {step === 3 && (
        <div className="flex flex-col items-center gap-3 py-6 text-center">
          <CheckCircle2 className="size-10 text-success" />
          <p className="text-sm font-medium">Migration checklist ready</p>
          <p className="max-w-md text-sm text-muted-foreground">
            Download both files before replacing hardware. The recovery bundle contains the verified system configuration and backed-up app data; the JSON checklist records the interface and stack choices made here.
          </p>
          <div className="flex flex-wrap justify-center gap-2">
            <Button size="sm" onClick={() => downloadBundle.mutate()} disabled={downloadBundle.isPending}>
              <ArrowDownToLine />{downloadBundle.isPending ? 'Preparing bundle…' : 'Download recovery bundle'}
            </Button>
            <Button size="sm" variant="outline" onClick={downloadMigrationChecklist}>
              <FileDown />Download migration checklist
            </Button>
          </div>
          <p className="max-w-md text-xs text-muted-foreground">This wizard does not write to the NAS or apply the migration. Boot the recovery installer on the replacement system and verify disk identities before enabling writes.</p>
        </div>
      )}

      {step < 2 && (
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          {step > 0 && (
            <Button variant="ghost" onClick={() => setStep(step - 1)}>
              Back
            </Button>
          )}
          <Button onClick={() => setStep(step + 1)}>Continue</Button>
        </div>
      )}
      {step === 2 && (
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={() => setStep(1)}>
            Back
          </Button>
          <Button disabled={!acknowledged} onClick={() => setStep(3)}>
            Approve restore plan
          </Button>
        </div>
      )}
      {step === 3 && (
        <div className="flex justify-end">
          <Button variant="outline" onClick={onClose}>
            Close
          </Button>
        </div>
      )}
    </div>
  )
}
