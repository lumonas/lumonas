import { useMemo, useState } from 'react'
import { ArrowDownToLine, ArrowLeft, Clock3, Folder, HardDriveUpload, Home, RotateCcw } from 'lucide-react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { apiDownload, apiGet, apiMultipart, apiPost } from '@/api/client'
import type { FileEntry, SnapshotEntry, StorageSnapshot } from '@/api/types'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { formatBytes, formatDateTime } from '@/lib/format'
import { toast } from 'sonner'

type PortalShare = { id: string; name: string; description?: string; access: 'read' | 'write' }
type PortalFiles = { shareId: string; path: string; entries: FileEntry[] }
type SnapshotFiles = { path: string; entries: SnapshotEntry[]; total: number }

export function MyFilesPage() {
  const client = useQueryClient()
  const [shareId, setShareId] = useState('')
  const [path, setPath] = useState('')
  const [filesToUpload, setFilesToUpload] = useState<File[]>([])
  const [uploading, setUploading] = useState(false)
  const [historyOpen, setHistoryOpen] = useState(false)
  const [snapshot, setSnapshot] = useState<StorageSnapshot | null>(null)
  const [snapshotPath, setSnapshotPath] = useState('')
  const [restoreEntry, setRestoreEntry] = useState<SnapshotEntry | null>(null)
  const [restoreTarget, setRestoreTarget] = useState('')
  const shares = useQuery({ queryKey: ['portal', 'shares'], queryFn: () => apiGet<PortalShare[]>('/portal/shares') })
  const activeShareId = shareId || shares.data?.[0]?.id || ''
  const activeShare = shares.data?.find((share) => share.id === activeShareId)
  const entriesQuery = useQuery({
    queryKey: ['portal', 'files', activeShareId, path],
    queryFn: () => apiGet<PortalFiles>(`/portal/files?shareId=${encodeURIComponent(activeShareId)}&path=${encodeURIComponent(path)}`),
    enabled: Boolean(activeShareId),
  })
  const snapshotsQuery = useQuery({
    queryKey: ['portal', 'snapshots', activeShareId],
    queryFn: () => apiGet<StorageSnapshot[]>(`/portal/snapshots?shareId=${encodeURIComponent(activeShareId)}`),
    enabled: historyOpen && Boolean(activeShareId),
  })
  const snapshotFilesQuery = useQuery({
    queryKey: ['portal', 'snapshot-files', activeShareId, snapshot?.id, snapshotPath],
    queryFn: () => apiGet<SnapshotFiles>(`/portal/snapshots/${encodeURIComponent(snapshot!.id)}/files?shareId=${encodeURIComponent(activeShareId)}&path=${encodeURIComponent(snapshotPath)}`),
    enabled: historyOpen && snapshot?.kind === 'btrfs',
  })
  const entries = useMemo(() => entriesQuery.data?.entries ?? [], [entriesQuery.data])

  function openFolder(name: string) {
    setPath([path, name].filter(Boolean).join('/'))
  }

  function download(name: string) {
    void apiDownload(`/portal/files/download?shareId=${encodeURIComponent(activeShareId)}&path=${encodeURIComponent(path)}&name=${encodeURIComponent(name)}`).then((blob) => {
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = name
      anchor.click()
      URL.revokeObjectURL(url)
    }).catch((error: Error) => toast.error(error.message || 'Download failed'))
  }

  async function uploadSelected() {
    if (!activeShare || activeShare.access !== 'write' || filesToUpload.length === 0) return
    setUploading(true)
    let completed = 0
    try {
      for (const file of filesToUpload) {
        const form = new FormData()
        form.append('file', file)
        await apiMultipart(`/portal/files/upload?shareId=${encodeURIComponent(activeShare.id)}&path=${encodeURIComponent(path)}`, form)
        completed += 1
      }
      setFilesToUpload([])
      toast.success(`${completed} file${completed === 1 ? '' : 's'} uploaded`)
      await client.invalidateQueries({ queryKey: ['portal', 'files', activeShare.id] })
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Upload failed')
      if (completed > 0) await client.invalidateQueries({ queryKey: ['portal', 'files', activeShare.id] })
    } finally {
      setUploading(false)
    }
  }

  async function restore() {
    if (!snapshot || !restoreEntry) return
    try {
      await apiPost(`/storage/snapshots/${encodeURIComponent(snapshot.id)}/restore`, {
        shareId: activeShareId,
        snapshotPath,
        targetPath: restoreTarget.trim().replace(/^\/+|\/+$/g, ''),
        names: [restoreEntry.name],
      })
      toast.success('Restore queued. Existing files will not be overwritten.')
      setRestoreEntry(null)
      await client.invalidateQueries({ queryKey: ['portal', 'files', activeShareId] })
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Could not restore this version')
    }
  }

  const parentPath = path.split('/').filter(Boolean).slice(0, -1).join('/')
  const snapshotParent = snapshotPath.split('/').filter(Boolean).slice(0, -1).join('/')

  return <main className="mx-auto flex min-h-dvh w-full max-w-4xl flex-col gap-5 px-4 py-6 sm:px-6 sm:py-10">
    <header><p className="text-xs font-semibold uppercase tracking-[0.16em] text-primary">LumoNAS</p><h1 className="mt-2 text-2xl font-semibold tracking-tight sm:text-3xl">My files</h1><p className="mt-1 text-sm text-muted-foreground">Browse the folders shared with your account, upload from this device, or recover an earlier file version.</p></header>
    {shares.isLoading ? <p role="status" className="py-8 text-center text-sm text-muted-foreground">Loading your shared folders…</p> : null}
    {shares.isError ? <p role="alert" className="text-sm text-destructive">Could not load your shared folders.</p> : null}
    {shares.data?.length === 0 ? <Card><CardContent className="py-10 text-center text-sm text-muted-foreground">No shared folders are assigned to this account yet. Ask your NAS owner to grant access.</CardContent></Card> : null}
    {shares.data?.length ? <>
      <Card><CardContent className="flex flex-col gap-3 p-4 sm:flex-row sm:items-end sm:justify-between"><label className="flex-1 space-y-1 text-xs font-medium text-muted-foreground">Shared folder<select aria-label="Shared folder" className="h-10 w-full rounded-md border bg-background px-3 text-sm text-foreground" value={activeShareId} onChange={(event) => { setShareId(event.target.value); setPath('') }}>{shares.data.map((share) => <option key={share.id} value={share.id}>{share.name} · {share.access === 'write' ? 'can upload' : 'read only'}</option>)}</select></label><div className="flex gap-2"><Button variant="outline" onClick={() => { setHistoryOpen(true); setSnapshot(null); setSnapshotPath('') }}><Clock3 />File history</Button>{activeShare?.access === 'write' ? <label className="inline-flex h-9 cursor-pointer items-center gap-2 rounded-md border px-3 text-sm font-medium hover:bg-muted"><HardDriveUpload className="size-4" />Choose files<input className="sr-only" type="file" multiple aria-label="Choose files to upload" onChange={(event) => setFilesToUpload(Array.from(event.target.files ?? []))} /></label> : null}</div></CardContent>
        {filesToUpload.length > 0 ? <CardContent className="flex flex-wrap items-center justify-between gap-3 border-t py-3"><p className="text-xs text-muted-foreground">{filesToUpload.length} selected · {formatBytes(filesToUpload.reduce((sum, file) => sum + file.size, 0))}</p><Button size="sm" onClick={() => void uploadSelected()} disabled={uploading}>{uploading ? 'Uploading…' : `Upload ${filesToUpload.length} file${filesToUpload.length === 1 ? '' : 's'}`}</Button></CardContent> : null}
      </Card>
      <Card><CardHeader className="pb-3"><div className="flex flex-wrap items-center gap-2"><Button variant="ghost" size="sm" disabled={!path} onClick={() => setPath(parentPath)}><ArrowLeft />Up</Button><Button variant="ghost" size="sm" onClick={() => setPath('')}><Home />Root</Button><CardTitle className="min-w-0 flex-1 truncate text-sm">{activeShare?.name} / {path || ''}</CardTitle></div><CardDescription>{activeShare?.description || (activeShare?.access === 'write' ? 'You can add files to this folder.' : 'You have read-only access to this folder.')}</CardDescription></CardHeader><CardContent className="space-y-2">
        {entriesQuery.isLoading ? <p role="status" className="py-8 text-center text-sm text-muted-foreground">Loading files…</p> : null}
        {entriesQuery.isError ? <p role="alert" className="py-4 text-sm text-destructive">Could not open this folder.</p> : null}
        {entries.map((entry) => <div key={entry.id} className="flex min-h-12 items-center gap-3 rounded-md border px-3 py-2"><Folder className="size-4 shrink-0 text-primary" /><button type="button" className="min-w-0 flex-1 truncate text-left text-sm font-medium hover:underline" onClick={() => entry.type === 'dir' ? openFolder(entry.name) : download(entry.name)}>{entry.name}</button><span className="hidden text-xs text-muted-foreground sm:inline">{entry.type === 'file' ? formatBytes(entry.sizeBytes) : 'Folder'}</span>{entry.type === 'file' ? <Button size="icon" variant="ghost" aria-label={`Download ${entry.name}`} onClick={() => download(entry.name)}><ArrowDownToLine /></Button> : null}</div>)}
        {!entriesQuery.isLoading && entries.length === 0 ? <p className="py-8 text-center text-sm text-muted-foreground">This folder is empty.</p> : null}
      </CardContent></Card>
    </> : null}

    <Dialog open={historyOpen} onOpenChange={(next) => { setHistoryOpen(next); if (!next) { setSnapshot(null); setRestoreEntry(null) } }}><DialogContent className="max-h-[85dvh] max-w-2xl overflow-y-auto"><DialogHeader><DialogTitle>File history</DialogTitle><DialogDescription>Browse read-only Btrfs restore points for this shared folder. Restores create a copy and never overwrite existing files.</DialogDescription></DialogHeader>
      {!snapshot ? <div className="space-y-2">{snapshotsQuery.isLoading ? <p role="status">Loading snapshots…</p> : null}{snapshotsQuery.isError ? <p role="alert" className="text-sm text-destructive">Could not load file history.</p> : null}{!snapshotsQuery.isLoading && snapshotsQuery.data?.length === 0 ? <p className="py-5 text-center text-sm text-muted-foreground">No snapshots are available for this folder.</p> : null}{snapshotsQuery.data?.map((item) => <button type="button" key={item.id} disabled={item.kind !== 'btrfs'} onClick={() => { setSnapshot(item); setSnapshotPath(''); setRestoreTarget(path) }} className="flex w-full items-center justify-between gap-3 rounded-md border p-3 text-left hover:bg-muted/50 disabled:opacity-50"><span><span className="block text-sm font-medium">{item.label || item.name}</span><span className="text-xs text-muted-foreground">{item.kind.toUpperCase()} · {formatDateTime(item.createdAt)}</span></span><span className="text-xs text-muted-foreground">{item.kind === 'btrfs' ? 'Browse' : 'Not browsable'}</span></button>)}</div> : <div className="space-y-2"><div className="flex items-center gap-2"><Button size="sm" variant="ghost" onClick={() => snapshotPath ? setSnapshotPath(snapshotParent) : setSnapshot(null)}><ArrowLeft />Back</Button><span className="truncate font-mono text-xs text-muted-foreground">/{snapshotPath}</span></div>{snapshotFilesQuery.isLoading ? <p role="status">Loading snapshot folder…</p> : null}{snapshotFilesQuery.isError ? <p role="alert" className="text-sm text-destructive">Could not browse this snapshot.</p> : null}{snapshotFilesQuery.data?.entries.map((entry) => <div key={entry.name} className="flex items-center gap-2 rounded-md border p-2"><Folder className="size-4 text-primary" /><button type="button" className="min-w-0 flex-1 truncate text-left text-sm" onClick={() => entry.directory ? setSnapshotPath([snapshotPath, entry.name].filter(Boolean).join('/')) : setRestoreEntry(entry)}>{entry.name}</button>{!entry.directory ? <Button size="sm" variant="outline" onClick={() => setRestoreEntry(entry)}><RotateCcw />Restore</Button> : null}</div>)}</div>}
    </DialogContent></Dialog>
    <Dialog open={restoreEntry != null} onOpenChange={(next) => !next && setRestoreEntry(null)}><DialogContent><DialogHeader><DialogTitle>Restore {restoreEntry?.name}?</DialogTitle><DialogDescription>This copies the older version into an existing folder. If a file with this name already exists, the restore is refused.</DialogDescription></DialogHeader><label className="space-y-1 text-sm">Restore into folder within this share<Input aria-label="Restore folder" value={restoreTarget} onChange={(event) => setRestoreTarget(event.target.value)} placeholder="Leave blank for root" /></label><DialogFooter><Button variant="ghost" onClick={() => setRestoreEntry(null)}>Cancel</Button><Button onClick={() => void restore()} disabled={!restoreEntry}><RotateCcw />Restore copy</Button></DialogFooter></DialogContent></Dialog>
  </main>
}
