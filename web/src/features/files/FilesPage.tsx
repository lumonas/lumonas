import { useMemo, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import {
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
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Kbd } from '@/components/ui/kbd'
import { cn } from '@/lib/utils'
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

interface Clipboard {
  op: 'copy' | 'cut'
  shareId: string
  fromPath: string
  names: string[]
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
  const [mkdirOpen, setMkdirOpen] = useState(false)
  const [renameTarget, setRenameTarget] = useState<FileEntry | null>(null)
  const [deleteNames, setDeleteNames] = useState<string[] | null>(null)
  const [properties, setProperties] = useState<FileEntry | null>(null)
  const [conflict, setConflict] = useState<{ names: string[]; payload: TransferPayload } | null>(
    null,
  )
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
      void Promise.all(selected.map((entry) => downloadEntry(entry)))
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

  async function downloadEntry(entry: FileEntry) {
    if (!shareId) return
    try {
      const blob = await apiDownload(
        `/files/download?share=${encodeURIComponent(shareId)}&path=${encodeURIComponent(path)}&name=${encodeURIComponent(entry.name)}`,
      )
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = entry.name
      anchor.click()
      URL.revokeObjectURL(url)
    } catch {
      toast.error(`Could not download “${entry.name}”`)
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
        else toast.error('Transfer failed')
      },
    })
  }

  function resolveConflict(mode: ConflictMode) {
    setConflict(null)
    paste(mode)
  }

  function uploadFiles(list: FileList | null) {
    if (!list || !shareId) return
    for (const file of Array.from(list)) {
      uploadFile.mutate(
        { shareId, path, file },
        {
          onSuccess: (result) =>
            toast.success(`Uploading “${result.name}” — running as a background job`),
        },
      )
    }
  }

  const crossShare =
    clipboard != null && clipboard.shareId !== shareId && clipboard.names.length > 0

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Files"
        description="Administrative file management inside your shares — large transfers run as background jobs."
        actions={<FileSearchDialog shareId={shareId} />}
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
              <Button variant="ghost" size="sm" onClick={() => bulk('download')}>
                <Download />
                Download
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

          {crossShare && (
            <AlertBanner tone="info" title="Pasting from another storage resource">
              {clipboard?.op === 'cut'
                ? 'Moving across resources is performed as copy + delete by a background job.'
                : 'Copying across resources runs as a background job.'}
            </AlertBanner>
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
