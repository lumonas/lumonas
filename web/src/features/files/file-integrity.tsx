import { useFileIntegrityStatus, useStartFileIntegrityScan } from '@/api/queries'
import { ShieldCheck } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog'
import { formatDateTime } from '@/lib/format'

export function FileIntegrityManager({ shareId }: { shareId: string | null }) {
  const status = useFileIntegrityStatus(shareId)
  const baseline = useStartFileIntegrityScan('baseline')
  const verify = useStartFileIntegrityScan('verify')
  const report = status.data?.report

  return <Dialog>
    <DialogTrigger asChild><Button size="sm" variant="outline" disabled={!shareId}><ShieldCheck />Integrity</Button></DialogTrigger>
    <DialogContent className="max-w-xl">
      <DialogHeader><DialogTitle>File integrity</DialogTitle><DialogDescription>Hash files in this share and compare them with a baseline to spot unexpected changes, missing files, or new files. Symlinks are excluded.</DialogDescription></DialogHeader>
      {status.isLoading ? <p role="status" className="text-sm text-muted-foreground">Loading integrity status…</p> : null}
      {status.isError ? <p role="alert" className="text-sm text-destructive">Could not load integrity status.</p> : null}
      {status.data?.baselineAt ? <div className="space-y-3 rounded-md border p-3 text-sm"><p>Baseline: <strong>{status.data.fileCount.toLocaleString()}</strong> files · {formatDateTime(status.data.baselineAt)}</p>
        {report ? <><p className="text-xs text-muted-foreground">Last verified {formatDateTime(report.verifiedAt)}</p><div className="grid grid-cols-2 gap-2 text-xs sm:grid-cols-4"><span className="rounded bg-muted p-2">{report.unchanged} unchanged</span><span className="rounded bg-muted p-2">{report.changedCount} changed</span><span className="rounded bg-muted p-2">{report.missingCount} missing</span><span className="rounded bg-muted p-2">{report.addedCount} added</span></div>{[['Changed',report.changed],['Missing',report.missing],['Added',report.added]].map(([label,items]) => Array.isArray(items) && items.length ? <div key={String(label)}><p className="text-xs font-medium">{String(label)} files (up to 100 shown)</p><ul className="max-h-24 overflow-auto text-xs text-muted-foreground">{items.map((name) => <li key={String(name)} className="truncate">{String(name)}</li>)}</ul></div> : null)}</> : <p className="text-xs text-muted-foreground">No integrity verification has run since this baseline was created.</p>}
      </div> : <p className="rounded-md border bg-muted/30 p-3 text-sm text-muted-foreground">Create a baseline before verifying changes. The first scan stores SHA-256 checksums for regular files in this share.</p>}
      {status.data?.running ? <p role="status" className="text-sm text-muted-foreground">Integrity scan running. Large shares can take a while; this dialog will update automatically.</p> : null}
      <DialogFooter className="gap-2 sm:justify-between"><Button variant="outline" disabled={!shareId || status.data?.running || baseline.isPending} onClick={() => shareId && baseline.mutate(shareId)}>{baseline.isPending ? 'Queueing…' : status.data?.baselineAt ? 'Replace baseline' : 'Create baseline'}</Button>{status.data?.baselineAt ? <Button disabled={!shareId || status.data.running || verify.isPending} onClick={() => shareId && verify.mutate(shareId)}>{verify.isPending ? 'Queueing…' : 'Verify now'}</Button> : null}</DialogFooter>
    </DialogContent>
  </Dialog>
}
