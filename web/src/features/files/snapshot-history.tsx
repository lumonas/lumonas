import { useMemo, useState } from 'react'
import { AlertTriangle, ArrowLeft, Clock3, Folder, RotateCcw, ScanSearch } from 'lucide-react'
import { useRestoreSnapshotEntries, useStorageSnapshotDiff, useStorageSnapshotFiles, useStorageSnapshots } from '@/api/queries'
import type { SnapshotChange, SnapshotEntry, StorageSnapshot } from '@/api/types'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { formatBytes, formatDateTime } from '@/lib/format'

interface SnapshotHistoryProps {
  shareId: string | null
  sharePath: string | undefined
  currentPath: string
}

function parentPath(value: string) {
  const parts = value.split('/').filter(Boolean)
  parts.pop()
  return parts.join('/')
}

export function SnapshotHistory({ shareId, sharePath, currentPath }: SnapshotHistoryProps) {
  const [open, setOpen] = useState(false)
  const [snapshot, setSnapshot] = useState<StorageSnapshot | null>(null)
  const [compareSnapshotId, setCompareSnapshotId] = useState<string | null>(null)
  const [browsePath, setBrowsePath] = useState('')
  const [selected, setSelected] = useState<SnapshotEntry | null>(null)
  const [selectedSnapshotPath, setSelectedSnapshotPath] = useState('')
  const [createRecoveryFolders, setCreateRecoveryFolders] = useState(false)
  const [targetPath, setTargetPath] = useState(currentPath.replace(/^\/+/, ''))
  const snapshotsQuery = useStorageSnapshots(open ? sharePath : undefined)
  const filesQuery = useStorageSnapshotFiles(snapshot?.kind === 'btrfs' ? snapshot.id : null, browsePath)
  const compareQuery = useStorageSnapshotDiff(compareSnapshotId)
  const restore = useRestoreSnapshotEntries()

  const snapshots = useMemo(() => (snapshotsQuery.data ?? []).slice().sort((a, b) => b.createdAt.localeCompare(a.createdAt)), [snapshotsQuery.data])

  function close(next: boolean) {
    setOpen(next)
    if (!next) {
      setSnapshot(null)
      setCompareSnapshotId(null)
      setBrowsePath('')
      setSelected(null)
      setSelectedSnapshotPath('')
      setCreateRecoveryFolders(false)
    }
  }

  function restoreSelected() {
    if (!shareId || !snapshot || !selected) return
    restore.mutate({ snapshotId: snapshot.id, shareId, snapshotPath: selectedSnapshotPath, targetPath: targetPath.trim().replace(/^\/+|\/+$/g, ''), names: [selected.name], createTargetDirectories: createRecoveryFolders }, {
      onSuccess: () => { setSelected(null); setCreateRecoveryFolders(false) },
    })
  }

  function restoreChangedFile(change: SnapshotChange) {
    if (change.kind === 'added' || !snapshot) return
    const parts = change.path.split('/').filter(Boolean)
    const name = parts.pop()
    if (!name) return
    setSelected({ name, sizeBytes: change.sizeBytes, directory: false, modifiedAt: change.modifiedAt })
    setSelectedSnapshotPath(parts.join('/'))
    setTargetPath(['Recovered from ' + snapshot.name, ...parts].join('/'))
    setCreateRecoveryFolders(true)
  }

  return <>
    <Button size="sm" variant="outline" onClick={() => { setTargetPath(currentPath.replace(/^\/+/, '')); setOpen(true) }} disabled={!shareId || !sharePath}>
      <Clock3 />File history
    </Button>
    <Dialog open={open} onOpenChange={close}>
      <DialogContent className="max-h-[85dvh] max-w-3xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>File history</DialogTitle>
          <DialogDescription>Browse read-only Btrfs snapshots and restore entries into this share. Existing files are never overwritten.</DialogDescription>
        </DialogHeader>
        {!snapshot ? <div className="space-y-2">
          {snapshotsQuery.isLoading ? <p role="status" className="py-6 text-center text-sm text-muted-foreground">Loading snapshots…</p> : null}
          {snapshotsQuery.isError ? <p role="alert" className="py-4 text-sm text-destructive">Could not load snapshot history.</p> : null}
          {!snapshotsQuery.isLoading && snapshots.length === 0 ? <p className="py-6 text-center text-sm text-muted-foreground">No snapshots are available for this share yet.</p> : null}
          {snapshots.map((item) => <button key={item.id} type="button" disabled={item.kind !== 'btrfs'} onClick={() => { setSnapshot(item); setBrowsePath('') }} className="flex w-full items-center justify-between gap-3 rounded-md border px-3 py-3 text-left hover:bg-muted/50 disabled:cursor-not-allowed disabled:opacity-50">
            <span className="min-w-0"><span className="block truncate text-sm font-medium">{item.label || item.name}</span><span className="mt-1 block text-xs text-muted-foreground">{item.kind.toUpperCase()} · {formatDateTime(item.createdAt)}</span></span>
            <span className="text-xs text-muted-foreground">{item.kind === 'btrfs' ? 'Browse' : 'Browsing unavailable'}</span>
          </button>)}
        </div> : <div className="space-y-3">
          <div className="flex flex-wrap items-center gap-2"><Button size="sm" variant="ghost" onClick={() => browsePath ? setBrowsePath(parentPath(browsePath)) : setSnapshot(null)}><ArrowLeft />{browsePath ? 'Parent folder' : 'All snapshots'}</Button><p className="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground">{snapshot.label || snapshot.name} · /{browsePath}</p>{!browsePath ? <Button size="sm" variant="outline" onClick={() => setCompareSnapshotId(snapshot.id)} disabled={compareQuery.isFetching}><ScanSearch />{compareQuery.isFetching ? 'Comparing…' : 'Compare with current'}</Button> : null}</div>
          {compareSnapshotId === snapshot.id ? <div className="rounded-lg border p-3" aria-live="polite">
            {compareQuery.isLoading ? <p role="status" className="text-sm text-muted-foreground">Comparing file names, sizes, and modification times…</p> : null}
            {compareQuery.isError ? <p role="alert" className="text-sm text-destructive">Could not compare this snapshot with the current share.</p> : null}
            {compareQuery.data ? <>
              {compareQuery.data.reviewRecommended ? <p className="mb-2 flex items-center gap-2 text-sm font-medium text-amber-700 dark:text-amber-300"><AlertTriangle className="size-4" />Large change volume detected. Review this snapshot before deleting it or pruning older recovery points.</p> : <p className="mb-2 text-sm font-medium">Snapshot comparison</p>}
              <p className="text-xs text-muted-foreground">{compareQuery.data.added} added · {compareQuery.data.modified} modified · {compareQuery.data.deleted} deleted · {compareQuery.data.unchanged} unchanged. This checks metadata; it does not identify the cause of changes.</p>
              {compareQuery.data.changes.length ? <ul className="mt-3 max-h-48 space-y-1 overflow-y-auto border-t pt-2">{compareQuery.data.changes.map((change) => <li key={`${change.kind}:${change.path}`} className="flex items-center gap-2 text-xs"><span className={`w-16 shrink-0 capitalize ${change.kind === 'deleted' ? 'text-destructive' : change.kind === 'modified' ? 'text-amber-700 dark:text-amber-300' : 'text-muted-foreground'}`}>{change.kind}</span><span className="min-w-0 flex-1 truncate font-mono">{change.path}</span>{change.kind !== 'added' ? <Button size="sm" variant="outline" className="h-7 shrink-0" onClick={() => restoreChangedFile(change)}><RotateCcw />Restore copy</Button> : null}</li>)}</ul> : <p className="mt-2 text-xs text-muted-foreground">No file changes were found.</p>}
              {compareQuery.data.added + compareQuery.data.deleted + compareQuery.data.modified > compareQuery.data.changes.length ? <p className="mt-2 text-[11px] text-muted-foreground">Showing the first {compareQuery.data.changes.length} changes.</p> : null}
            </> : null}
          </div> : null}
          {filesQuery.isLoading ? <p role="status" className="py-6 text-center text-sm text-muted-foreground">Loading this snapshot folder…</p> : null}
          {filesQuery.isError ? <p role="alert" className="py-4 text-sm text-destructive">{filesQuery.error instanceof Error ? filesQuery.error.message : 'Could not browse this snapshot.'}</p> : null}
          {filesQuery.data?.entries.map((entry) => <div key={entry.name} className="flex items-center gap-3 rounded-md border px-3 py-2.5">
            {entry.directory ? <Folder className="size-4 shrink-0 text-primary" /> : <span className="size-4 shrink-0" />}
            <button type="button" className="min-w-0 flex-1 truncate text-left text-sm hover:underline" onClick={() => { if (entry.directory) setBrowsePath([browsePath, entry.name].filter(Boolean).join('/')); else { setSelected(entry); setSelectedSnapshotPath(browsePath); setTargetPath(currentPath.replace(/^\/+/, '')); setCreateRecoveryFolders(false) } }}>{entry.name}</button>
            {!entry.directory ? <span className="text-xs text-muted-foreground">{formatBytes(entry.sizeBytes)}</span> : null}
            {!entry.directory ? <Button size="sm" variant="outline" onClick={() => { setSelected(entry); setSelectedSnapshotPath(browsePath); setTargetPath(currentPath.replace(/^\/+/, '')); setCreateRecoveryFolders(false) }}><RotateCcw />Restore…</Button> : null}
          </div>)}
          {filesQuery.data?.entries.length === 0 ? <p className="py-6 text-center text-sm text-muted-foreground">This snapshot folder is empty.</p> : null}
        </div>}
      </DialogContent>
    </Dialog>
    <Dialog open={selected != null} onOpenChange={(next) => !next && setSelected(null)}>
      <DialogContent>
        <DialogHeader><DialogTitle>Restore {selected?.name}?</DialogTitle><DialogDescription>The entry will be copied from the read-only snapshot. LumoNAS will refuse to overwrite an existing item with the same name.{createRecoveryFolders ? ' Missing recovery folders will be created safely inside this share.' : ''}</DialogDescription></DialogHeader>
        <div className="space-y-2"><label htmlFor="snapshot-restore-folder" className="text-sm font-medium">Restore into share folder</label><Input id="snapshot-restore-folder" value={targetPath} onChange={(event) => setTargetPath(event.target.value)} placeholder="/ for the share root" /><p className="text-xs text-muted-foreground">Use a relative folder path inside this share. Leave blank to restore to its root.</p></div>
        <DialogFooter><Button variant="ghost" onClick={() => setSelected(null)}>Cancel</Button><Button disabled={!selected || restore.isPending} onClick={restoreSelected}>{restore.isPending ? 'Queueing…' : 'Restore copy'}</Button></DialogFooter>
      </DialogContent>
    </Dialog>
  </>
}
