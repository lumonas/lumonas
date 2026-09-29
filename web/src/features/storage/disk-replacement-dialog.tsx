import { useState } from 'react'
import { LoaderCircle } from 'lucide-react'
import { useConfirmDiskReplacement, useDisks, usePlanDiskReplacement, useStorageSafety } from '@/api/queries'
import { AlertBanner } from '@/components/core/alert-banner'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import type { Disk } from '@/api/types'
import { formatBytes } from '@/lib/format'

export function DiskReplacementDialog({ disk, open, onOpenChange }: { disk: Disk; open: boolean; onOpenChange: (open: boolean) => void }) {
  const { data: disks } = useDisks()
  const { data: safety } = useStorageSafety()
  const planReplacement = usePlanDiskReplacement()
  const confirmReplacement = useConfirmDiskReplacement()
  const [replacementDiskId, setReplacementDiskId] = useState('')
  const [reviewed, setReviewed] = useState(false)

  function close(next: boolean) {
    if (!next) {
      setReplacementDiskId('')
      setReviewed(false)
      planReplacement.reset()
      confirmReplacement.reset()
    }
    onOpenChange(next)
  }

  const candidates = (disks ?? []).filter((candidate) => candidate.id !== disk.id && !candidate.poolId && !candidate.mounted && candidate.role !== 'parity' && candidate.health !== 'critical')
  const plan = planReplacement.data
  const canExecute = plan && reviewed && safety?.state === 'unlocked'

  return <Dialog open={open} onOpenChange={close}>
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Replace protected data disk</DialogTitle>
        <DialogDescription>Preserves the SnapRAID slot name, then formats the new disk, recovers content from parity, and schedules a sync.</DialogDescription>
      </DialogHeader>
      {!plan ? <div className="space-y-4">
        <AlertBanner tone="warning" title="Recovery operation">The selected replacement will be formatted. Confirm the serial number before creating the plan.</AlertBanner>
        {planReplacement.isError && <AlertBanner tone="critical" title="Could not create a replacement plan">{planReplacement.error instanceof Error ? planReplacement.error.message : 'Review the selected disks and try again.'}</AlertBanner>}
        <div className="space-y-1"><Label>Failed disk</Label><p className="rounded-md border bg-muted/30 px-3 py-2 text-sm">{disk.name} · {disk.serial} · {formatBytes(disk.sizeBytes)}</p></div>
        <div className="space-y-2"><Label htmlFor="replacement-disk">Replacement disk</Label><Select value={replacementDiskId} onValueChange={setReplacementDiskId}><SelectTrigger id="replacement-disk"><SelectValue placeholder="Choose an unused, unmounted disk" /></SelectTrigger><SelectContent>{candidates.map((candidate) => <SelectItem key={candidate.id} value={candidate.id}>{candidate.name} · {candidate.serial} · {formatBytes(candidate.sizeBytes)}</SelectItem>)}</SelectContent></Select>
          {candidates.length === 0 && <p className="text-xs text-warning">No suitable unused replacement disk is currently discovered.</p>}
        </div>
      </div> : <div className="space-y-3">
        <AlertBanner title="Reviewed replacement plan">Data slot <strong>{plan.retiredDataName}</strong> will point at the selected disk. The plan expires {new Date(plan.expiresAt).toLocaleString()}.</AlertBanner>
        <ol className="list-decimal space-y-1 pl-5 text-sm text-muted-foreground"><li>Format and mount the replacement disk.</li><li>Apply the protected slot mapping.</li><li>Run SnapRAID fix for {plan.retiredDataName}.</li><li>Run a parity sync after recovery completes.</li></ol>
        <label className="flex items-start gap-2 rounded-lg border p-3 text-sm"><input type="checkbox" checked={reviewed} onChange={(event) => setReviewed(event.target.checked)} className="mt-0.5" />I verified the failed and replacement disk serial numbers.</label>
        {safety?.state !== 'unlocked' && <AlertBanner tone="warning" title="Storage safety is locked">Unlock storage safety in Storage before starting this destructive recovery operation.</AlertBanner>}
        {confirmReplacement.isError && <AlertBanner tone="critical" title="Recovery was not started">{confirmReplacement.error instanceof Error ? confirmReplacement.error.message : 'The plan may have expired. Close and create a new plan.'}</AlertBanner>}
        {confirmReplacement.isSuccess && <AlertBanner title="Recovery queued">{confirmReplacement.data.next}</AlertBanner>}
      </div>}
      <DialogFooter>
        <Button variant="ghost" onClick={() => close(false)}>Cancel</Button>
        {!plan ? <Button disabled={!replacementDiskId || planReplacement.isPending} onClick={() => planReplacement.mutate({ retiredDiskId: disk.id, replacementDiskId })}>{planReplacement.isPending && <LoaderCircle className="animate-spin" />}Review plan</Button> : <Button disabled={!canExecute || confirmReplacement.isPending || confirmReplacement.isSuccess} onClick={() => confirmReplacement.mutate({ operationId: plan.operationId, planHash: plan.planHash })}>{confirmReplacement.isPending && <LoaderCircle className="animate-spin" />}Start recovery</Button>}
      </DialogFooter>
    </DialogContent>
  </Dialog>
}
