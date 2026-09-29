import { useMemo, useRef, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import {
  Archive,
  ClipboardPaste,
  Copy,
  Download,
  FolderPlus,
  HardDrive,
  Scissors,
  Search,
  Trash2,
  TriangleAlert,
  Upload,
  X,
} from 'lucide-react'
import { toast } from 'sonner'
import { apiDownload } from '@/api/client'
import {
  extractConflicts,
  useDeleteFiles,
  useFiles,
  useMkdir,
  useRecycleBin,
  useRenameEntry,
  useShares,
  useTransfer,
  useUploadFile,
  type TransferPayload,
} from '@/api/queries'
import { AlertBanner } from '@/components/core/alert-banner'
import { EmptyState } from '@/components/core/empty-state'
import { PageHeader } from '@/components/core/page-header'
import { FileSearchDialog } from '@/features/files/file-search'
import { SnapshotHistory } from '@/features/files/snapshot-history'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Kbd } from '@/components/ui/kbd'
import { cn } from '@/lib/utils'
import { formatBytes } from '@/lib/format'
import type { FileEntry } from '@/api/types'
import {
  ConflictDialog,
  DeleteDialog,
  MkdirDialog,
  PropertiesDialog,
  RenameDialog,
  type ConflictMode,
} from '@/features/files/file-dialogs'
import { FilesTable, type FileAction } from '@/features/files/files-table'
import { RecycleBin } from '@/features/files/recycle-bin'
import { FileRequestManager } from '@/features/files/file-requests'
import { FileIntegrityManager } from '@/features/files/file-integrity'

interface Clipboard {
  op: 'copy' | 'cut'
  shareId: string
  fromPath: string
  names: string[]
}

interface UploadItem {
  id: string
  name: string
  file: File | null
  shareId: string
  path: string
  status: 'submitting' | 'queued' | 'failed'
}

function shareIdOf(searchParams: URLSearchParams, shares?: { id: string }[]): string | null {
  const param = searchParams.get('share')
  if (param) return param
  return shares?.[0]?.id ?? null
}

export function FilesPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const { data: shares } = useShares()
  const shareId = shareIdOf(searchParams, shares)
  const path = searchParams.get('path') ?? '/'
  const binView = searchParams.get('bin') === '1'

  const files = useFiles(shareId, path)
  const recycle = useRecycleBin(shareId, !binView)
  const share = shares?.find((s) => s.id === shareId)

  const setParam = (key: string, value: string | null) => {
    const next = new URLSearchParams(searchParams)
    if (value == null) next.delete(key)
    else next.set(key, value)
    setSearchParams(next)
  }

  const [clipboard, setClipboard] = useState<Clipboard | null>(null)
  const [selection, setSelection] = useState<{ key: string; ids: Set<string> }>({
    key: '',
    ids: new Set(),
  })
  const [query, setQuery] = useState('')
  const [dragging, setDragging] = useState(false)
  const [uploadQueue, setUploadQueue] = useState<UploadItem[]>([])
  const [downloadProgress, setDownloadProgress] = useState<{ completed: number; total: number } | null>(null)
  const [archivePending, setArchivePending] = useState(false)
  const [mkdirOpen, setMkdirOpen] = useState(false)
  const [renameTarget, setRenameTarget] = useState<FileEntry | null>(null)
  const [deleteNames, setDeleteNames] = useState<string[] | null>(null)
  const [properties, setProperties] = useState<FileEntry | null>(null)
  const [preview, setPreview] = useState<FileEntry | null>(null)
  const [conflict, setConflict] = useState<{ names: string[]; payload: TransferPayload } | null>(
    null,
  )
  const [transferError, setTransferError] = useState<TransferPayload | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const mkdir = useMkdir()
  const renameEntry = useRenameEntry()
  const deleteFiles = useDeleteFiles()
  const transfer = useTransfer()
  const uploadFile = useUploadFile()

  const selKey = `${shareId}:${path}:${binView}`
  if (selection.key !== selKey) {
    setSelection({ key: selKey, ids: new Set() })
  }
  const selectedIds = selection.key === selKey ? selection.ids : new Set<string>()

  const entries = useMemo(() => files.data?.entries ?? [], [files.data])
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return entries
    return entries.filter((entry) => entry.name.toLowerCase().includes(q))
  }, [entries, query])

  const selectedNames = entries
    .filter((entry) => selectedIds.has(entry.id))
    .map((entry) => entry.name)

  function toggleSelect(id: string) {
    const next = new Set(selectedIds)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    setSelection({ key: selKey, ids: next })
  }

  function toggleAll() {
    const next =
      selectedIds.size === filtered.length ? new Set<string>() : new Set(filtered.map((e) => e.id))
    setSelection({ key: selKey, ids: next })
  }

  function openPath(nextPath: string) {
    setParam('path', nextPath === '/' ? null : nextPath)
    setQuery('')
  }

  function handleAction(action: FileAction, entry: FileEntry) {
    switch (action) {
      case 'preview':
        setPreview(entry)
        break
      case 'download':
        void downloadEntry(entry)
        break
      case 'copy':
      case 'cut':
        if (!shareId) return
        setClipboard({ op: action, shareId, fromPath: path, names: [entry.name] })
        toast.success(action === 'copy' ? 'Copied to clipboard' : 'Cut to clipboard')
        break
      case 'rename':
        setRenameTarget(entry)
        break
      case 'properties':
        setProperties(entry)
        break
      case 'delete':
        setDeleteNames([entry.name])
        break
    }
  }

  function bulk(action: 'copy' | 'cut' | 'delete' | 'download') {
    if (action === 'download') {
      const selected = entries.filter((entry) => selectedIds.has(entry.id) && entry.type === 'file')
      if (selected.length === 0) {
        toast.info('Select one or more files to download')
        return
      }
      void downloadSelected(selected)
      return
    }
    if (action === 'delete') {
      setDeleteNames(selectedNames)
      return
    }
    if (!shareId) return
    setClipboard({ op: action, shareId, fromPath: path, names: selectedNames })
    toast.success(action === 'copy' ? 'Copied to clipboard' : 'Cut to clipboard')
  }

  async function downloadEntry(entry: FileEntry, destinationShareId = shareId, destinationPath = path): Promise<boolean> {
    if (!destinationShareId) return false
    try {
      const blob = await apiDownload(
        `/files/download?share=${encodeURIComponent(destinationShareId)}&path=${encodeURIComponent(destinationPath)}&name=${encodeURIComponent(entry.name)}`,
      )
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = entry.name
      anchor.click()
      window.setTimeout(() => URL.revokeObjectURL(url), 1_000)
      return true
    } catch {
      toast.error(`Could not download “${entry.name}”`)
      return false
    }
  }

  async function downloadSelected(selected: FileEntry[]) {
    const destinationShareId = shareId
    const destinationPath = path
    setDownloadProgress({ completed: 0, total: selected.length })
    let failures = 0
    for (const [index, entry] of selected.entries()) {
      if (!(await downloadEntry(entry, destinationShareId, destinationPath))) failures += 1
      setDownloadProgress({ completed: index + 1, total: selected.length })
    }
    setDownloadProgress(null)
    if (failures === 0) toast.success(`Started ${selected.length} file downloads`)
    else toast.warning(`${selected.length - failures} of ${selected.length} downloads started`)
  }

  async function downloadArchive(selected: FileEntry[]) {
    if (!shareId || selected.length === 0) return
    if (selected.length > 100) {
      toast.error('Select up to 100 items for one archive')
      return
    }
    const params = new URLSearchParams({ share: shareId, path })
    selected.forEach((entry) => params.append('name', entry.name))
    setArchivePending(true)
    try {
      const blob = await apiDownload(`/files/download/archive?${params.toString()}`)
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = 'lumonas-files.zip'
      anchor.click()
      window.setTimeout(() => URL.revokeObjectURL(url), 1_000)
      toast.success('Archive download started')
    } catch {
      toast.error('Could not create the archive. Check the connection and try again.')
    } finally {
      setArchivePending(false)
    }
  }

  function paste(resolvedMode?: ConflictMode) {
    if (!clipboard || !shareId) return
    const payload: TransferPayload = {
      shareId: clipboard.shareId,
      sourcePath: clipboard.fromPath,
      names: clipboard.names,
      targetShareId: shareId,
      targetPath: path,
      op: clipboard.op === 'cut' ? 'move' : 'copy',
      conflict: resolvedMode,
    }
    setTransferError(null)
    transfer.mutate(payload, {
      onSuccess: (result) => {
        toast.success(
          `${clipboard.op === 'copy' ? 'Copied' : 'Moved'} ${result.transferred} item${result.transferred === 1 ? '' : 's'} — running as a background job`,
        )
        if (clipboard.op === 'cut') setClipboard(null)
      },
      onError: (error) => {
        const conflicts = extractConflicts(error)
        if (conflicts) setConflict({ names: conflicts, payload })
        else {
          setTransferError(payload)
          toast.error('Transfer could not be queued. You can retry it here.')
        }
      },
    })
  }

  function retryTransfer(payload: TransferPayload) {
    setTransferError(null)
    transfer.mutate(payload, {
      onSuccess: (result) => {
        toast.success(`Transfer queued — ${result.transferred} item${result.transferred === 1 ? '' : 's'} will run in the background`)
        if (payload.op === 'move') setClipboard(null)
      },
      onError: () => setTransferError(payload),
    })
  }

  function resolveConflict(mode: ConflictMode) {
    setConflict(null)
    paste(mode)
  }

  function submitUpload(item: UploadItem) {
    if (!item.file) return
    const file = item.file
    setUploadQueue((current) => current.map((entry) => entry.id === item.id ? { ...entry, status: 'submitting' } : entry))
    uploadFile.mutate(
      { shareId: item.shareId, path: item.path, file },
      {
        onSuccess: (result) => {
          setUploadQueue((current) => current.map((entry) => entry.id === item.id ? { ...entry, file: null, status: 'queued' } : entry))
          toast.success(`“${result.name}” added to background jobs`)
        },
        onError: () => {
          setUploadQueue((current) => current.map((entry) => entry.id === item.id ? { ...entry, status: 'failed' } : entry))
          toast.error(`Could not queue “${item.name}”`)
        },
      },
    )
  }

  function uploadFiles(list: FileList | null) {
    if (!list || !shareId) return
    const items = Array.from(list).map((file) => ({
      id: `${Date.now()}-${Math.random().toString(36).slice(2)}`,
      name: file.name,
      file,
      shareId,
      path,
      status: 'submitting' as const,
    }))
    setUploadQueue((current) => [...items, ...current].slice(0, 8))
    for (const item of items) submitUpload(item)
  }

  const crossShare =
    clipboard != null && clipboard.shareId !== shareId && clipboard.names.length > 0

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Files"
        description="Administrative file management inside your shares — large transfers run as background jobs."
        actions={<div className="flex flex-wrap gap-2">{!binView ? <><Button asChild size="sm" variant="outline"><Link to="/my-files">Simple view</Link></Button><FileRequestManager shareId={shareId} path={path} /><SnapshotHistory shareId={shareId} sharePath={share?.path} currentPath={path} /><FileIntegrityManager shareId={shareId} /></> : null}<FileSearchDialog shareId={shareId} /></div>}
      />

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-[220px_1fr]">
        <Card className="h-fit p-2">
          <p className="px-2 py-1.5 text-xs font-medium tracking-wide text-muted-foreground uppercase">
            Locations
          </p>
          <div className="flex flex-col gap-0.5">
            {(shares ?? []).map((s) => (
              <button
                key={s.id}
                type="button"
                onClick={() => {
                  setParam('share', s.id)
                  setParam('path', null)
                  setParam('bin', null)
                }}
                className={cn(
                  'flex items-center gap-2.5 rounded-md px-2.5 py-2 text-left text-sm outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring',
                  s.id === shareId
                    ? 'bg-accent font-medium text-accent-foreground'
                    : 'text-muted-foreground hover:bg-secondary hover:text-foreground',
                )}
              >
                <HardDrive className="size-4 shrink-0" />
                <span className="min-w-0 flex-1 truncate">{s.name}</span>
              </button>
            ))}
          </div>
        </Card>

        <div
          className="relative flex min-w-0 flex-col gap-3"
          onDragOver={(e) => {
            e.preventDefault()
            if (!dragging) setDragging(true)
          }}
          onDragLeave={(e) => {
            if (e.currentTarget === e.target) setDragging(false)
          }}
          onDrop={(e) => {
            e.preventDefault()
            setDragging(false)
            if (!binView) uploadFiles(e.dataTransfer.files)
          }}
        >
          <div className="flex flex-wrap items-center gap-1 text-sm">
            <button
              type="button"
              className="rounded px-1.5 py-0.5 font-medium outline-none hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring"
              onClick={() => openPath('/')}
            >
              {share?.name ?? 'Files'}
            </button>
            {path
              .split('/')
              .filter(Boolean)
              .map((segment, index, all) => {
                const crumbPath = '/' + all.slice(0, index + 1).join('/')
                const last = index === all.length - 1
                return (
                  <span key={crumbPath} className="flex items-center gap-1">
                    <span className="text-muted-foreground">/</span>
                    {last ? (
                      <span className="rounded px-1.5 py-0.5 font-medium">{segment}</span>
                    ) : (
                      <button
                        type="button"
                        className="rounded px-1.5 py-0.5 outline-none hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring"
                        onClick={() => openPath(crumbPath)}
                      >
                        {segment}
                      </button>
                    )}
                  </span>
                )
              })}
            {binView && (
              <span className="ml-2 rounded-md bg-muted px-2 py-0.5 text-xs text-muted-foreground">
                Recycle bin
              </span>
            )}
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <div className="relative min-w-44 flex-1 sm:max-w-xs">
              <Search className="absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Search in this folder…"
                className="pl-8"
                aria-label="Search files"
              />
            </div>
            {!binView && (
              <>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={!shareId}
                  onClick={() => setMkdirOpen(true)}
                >
                  <FolderPlus />
                  New folder
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={!clipboard || !shareId}
                  onClick={() => paste()}
                >
                  <ClipboardPaste />
                  Paste
                  {clipboard && (
                    <span className="tnum text-xs text-muted-foreground">
                      {clipboard.names.length}
                    </span>
                  )}
                </Button>
                <input
                  ref={fileInputRef}
                  type="file"
                  multiple
                  className="hidden"
                  onChange={(e) => {
                    uploadFiles(e.target.files)
                    e.target.value = ''
                  }}
                />
                <Button
                  variant="outline"
                  size="sm"
                  disabled={!shareId}
                  onClick={() => fileInputRef.current?.click()}
                >
                  <Upload />
                  Upload
                </Button>
              </>
            )}
            <Button
              variant={binView ? 'default' : 'outline'}
              size="sm"
              className="ml-auto"
              onClick={() => setParam('bin', binView ? null : '1')}
            >
              <Trash2 />
              Recycle bin
              {(recycle.data?.length ?? 0) > 0 && (
                <span className="tnum rounded-full bg-primary/15 px-1.5 text-xs text-primary">
                  {recycle.data?.length}
                </span>
              )}
            </Button>
          </div>

          {selectedIds.size > 0 && !binView && (
            <div className="flex flex-wrap items-center gap-2 rounded-lg border bg-accent/40 px-3 py-2">
              <span className="tnum text-sm font-medium">{selectedIds.size} selected</span>
              <span className="flex-1" />
              <Button variant="ghost" size="sm" onClick={() => bulk('copy')}>
                <Copy />
                Copy
              </Button>
              <Button variant="ghost" size="sm" onClick={() => bulk('cut')}>
                <Scissors />
                Cut
              </Button>
              <Button variant="ghost" size="sm" disabled={downloadProgress != null || archivePending} onClick={() => bulk('download')}>
                <Download />
                {downloadProgress ? `Starting ${downloadProgress.completed}/${downloadProgress.total}` : 'Download'}
              </Button>
              <Button variant="ghost" size="sm" disabled={archivePending || downloadProgress != null || selectedIds.size > 100} title={selectedIds.size > 100 ? 'Choose up to 100 items' : undefined} onClick={() => void downloadArchive(entries.filter((entry) => selectedIds.has(entry.id)))}>
                <Archive />
                {archivePending ? 'Building ZIP…' : 'Download ZIP'}
              </Button>
              <Button
                variant="ghost"
                size="sm"
                className="text-critical hover:text-critical"
                onClick={() => bulk('delete')}
              >
                <Trash2 />
                Delete
              </Button>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label="Clear selection"
                onClick={() => setSelection({ key: selKey, ids: new Set() })}
              >
                <X />
              </Button>
            </div>
          )}

          {downloadProgress && <p className="text-xs text-muted-foreground" role="status">Starting download {downloadProgress.completed} of {downloadProgress.total}…</p>}

          {crossShare && (
            <AlertBanner tone="info" title="Pasting from another storage resource">
              {clipboard?.op === 'cut'
                ? 'Moving across resources is performed as copy + delete by a background job.'
                : 'Copying across resources runs as a background job.'}
            </AlertBanner>
          )}

          {transferError && <AlertBanner tone="critical" title="Transfer failed to start" action={<Button size="sm" variant="outline" onClick={() => retryTransfer(transferError)} disabled={transfer.isPending}>Retry</Button>}>The selected files are still available to transfer. Check the connection and try again.</AlertBanner>}

          {uploadQueue.length > 0 && (
            <section className="rounded-lg border bg-card p-3" aria-label="Recent upload submissions">
              <div className="mb-2 flex items-center justify-between gap-3">
                <p className="text-xs font-medium">Recent uploads</p>
                <button type="button" className="text-xs text-muted-foreground underline-offset-4 hover:underline" onClick={() => setUploadQueue([])}>Clear</button>
              </div>
              <ul className="space-y-1.5">
                {uploadQueue.slice(0, 5).map((item) => (
                  <li key={item.id} className="flex items-center gap-2 text-xs">
                    <span className={item.status === 'failed' ? 'text-critical' : item.status === 'queued' ? 'text-success' : 'text-muted-foreground'}>
                      {item.status === 'failed' ? 'Failed' : item.status === 'queued' ? 'Queued' : 'Submitting'}
                    </span>
                    <span className="min-w-0 flex-1 truncate" title={item.name}>{item.name}</span>
                    {item.status === 'failed' ? <button type="button" className="font-medium text-primary underline-offset-4 hover:underline" onClick={() => submitUpload(item)}>Retry</button> : null}
                  </li>
                ))}
              </ul>
              <p className="mt-2 text-xs text-muted-foreground">Queued uploads continue as background jobs. Follow their progress in Jobs.</p>
            </section>
          )}

          {shareId == null ? (
            <EmptyState
              icon={<HardDrive />}
              title="No shares yet"
              description="Create a share first — then browse and manage its files here."
            />
          ) : binView ? (
            <RecycleBin shareId={shareId} onBack={() => setParam('bin', null)} />
          ) : files.isError ? (
            <EmptyState
              icon={<TriangleAlert />}
              title="Folder unavailable"
              description={
                files.error instanceof Error
                  ? files.error.message
                  : 'This share or folder could not be opened. It may have been removed.'
              }
            />
          ) : (
      <FilesTable
              key={selKey}
              entries={filtered}
              loading={files.isLoading}
              selectedIds={selectedIds}
              onToggle={toggleSelect}
              onToggleAll={toggleAll}
              onOpen={(entry) => openPath(`${path === '/' ? '' : path}/${entry.name}`)}
              onAction={handleAction}
              emptyState={
                <EmptyState
                  icon={<Search />}
                  title={query ? 'No matches' : 'This folder is empty'}
                  description={
                    query
                      ? 'Nothing in this folder matches the search.'
                      : 'Drop files here to upload, or create a new folder.'
                  }
                  className="border-0"
                />
              }
            />
          )}

          {!binView && (
            <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
              Tip: drag & drop files anywhere here to upload —
              <Kbd>jobs</Kbd>
              run in the background and survive closing the browser.
            </p>
          )}

          {dragging && (
            <div className="pointer-events-none absolute inset-0 z-10 flex items-center justify-center rounded-xl border-2 border-dashed border-primary/60 bg-primary/5">
              <p className="flex items-center gap-2 text-sm font-medium text-primary">
                <Upload className="size-4" />
                Drop files to upload to {path === '/' ? '/' : path}
              </p>
            </div>
          )}
        </div>
      </div>

      <MkdirDialog
        open={mkdirOpen}
        onOpenChange={setMkdirOpen}
        onCreate={(name) => {
          if (!shareId) return
          mkdir.mutate(
            { shareId, path, name },
            {
              onSuccess: () => {
                setMkdirOpen(false)
                toast.success(`Folder created — ${name}`)
              },
              onError: () => toast.error('Could not create folder — name may already exist'),
            },
          )
        }}
      />

      <FilePreviewDialog
        entry={preview}
        shareId={shareId}
        path={path}
        onOpenChange={(open) => { if (!open) setPreview(null) }}
      />
      <RenameDialog
        entry={renameTarget}
        onOpenChange={(open) => !open && setRenameTarget(null)}
        onRename={(newName) => {
          if (!renameTarget || !shareId) return
          renameEntry.mutate(
            { shareId, path, oldName: renameTarget.name, newName },
            {
              onSuccess: () => {
                setRenameTarget(null)
                toast.success('Renamed')
              },
              onError: () => toast.error('Rename failed — name may already exist'),
            },
          )
        }}
      />
      <DeleteDialog
        names={deleteNames}
        onOpenChange={(open) => !open && setDeleteNames(null)}
        onDelete={() => {
          if (!deleteNames || !shareId) return
          deleteFiles.mutate(
            { shareId, path, names: deleteNames },
            {
              onSuccess: (result) => {
                setDeleteNames(null)
                toast.success(
                  `${result.deleted} item${result.deleted === 1 ? '' : 's'} moved to the recycle bin`,
                )
              },
            },
          )
        }}
      />
      <PropertiesDialog
        entry={properties}
        share={share}
        onOpenChange={(open) => !open && setProperties(null)}
      />
      <ConflictDialog
        conflicts={conflict?.names ?? null}
        pending={transfer.isPending}
        onOpenChange={(open) => !open && setConflict(null)}
        onResolve={resolveConflict}
      />
    </div>
  )
}

function FilePreviewDialog({ entry, shareId, path, onOpenChange }: {
  entry: FileEntry | null
  shareId: string | null
  path: string
  onOpenChange: (open: boolean) => void
}) {
  if (!entry || !shareId) return null
  const extension = entry.name.split('.').pop()?.toLowerCase() ?? ''
  const image = ['avif', 'gif', 'jpg', 'jpeg', 'png', 'webp'].includes(extension)
  const audio = ['m4a', 'mp3', 'oga', 'ogg', 'wav'].includes(extension)
  const video = ['m4v', 'mp4', 'ogv', 'webm'].includes(extension)
  const text = ['csv', 'json', 'log', 'txt', 'xml', 'yaml', 'yml'].includes(extension)
  if (!image && !audio && !video && !text) return null
  if (text && entry.sizeBytes > 2 * 1024 * 1024) return null
  const params = new URLSearchParams({ share: shareId, path, name: entry.name, inline: '1' })
  const source = `/api/v1/files/download?${params.toString()}`

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] max-w-4xl overflow-auto">
        <DialogHeader>
          <DialogTitle className="truncate">{entry.name}</DialogTitle>
          <DialogDescription>{image ? 'Image preview' : audio ? 'Audio preview' : video ? 'Video preview' : 'Text preview'} · {formatBytes(entry.sizeBytes)}</DialogDescription>
        </DialogHeader>
        <div className="flex min-h-40 items-center justify-center overflow-hidden rounded-lg bg-muted/40 p-2">
          {image ? <img src={source} alt={entry.name} className="max-h-[68vh] max-w-full object-contain" /> : null}
          {audio ? <audio src={source} controls preload="metadata" className="w-full" /> : null}
          {video ? <video src={source} controls preload="metadata" className="max-h-[68vh] max-w-full" /> : null}
          {text ? <iframe src={source} title={`Preview of ${entry.name}`} sandbox="" className="h-[68vh] w-full rounded border bg-background" /> : null}
        </div>
      </DialogContent>
    </Dialog>
  )
}
