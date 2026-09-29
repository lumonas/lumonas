import { MoreHorizontal } from 'lucide-react'
import { entryIcon, typeLabel } from '@/features/files/file-utils'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { cn } from '@/lib/utils'
import { formatDateTime, formatBytes } from '@/lib/format'
import type { FileEntry } from '@/api/types'

export type FileAction = 'preview' | 'download' | 'copy' | 'cut' | 'rename' | 'properties' | 'delete'

function isPreviewable(entry: FileEntry) {
  if (entry.type !== 'file') return false
  const text = /\.(csv|json|log|txt|xml|ya?ml)$/i.test(entry.name)
  if (text && entry.sizeBytes > 2 * 1024 * 1024) return false
  return text || /\.(avif|gif|jpe?g|png|webp|m4a|mp3|oga|ogg|wav|m4v|mp4|ogv|webm)$/i.test(entry.name)
}

export function FilesTable({
  entries,
  loading,
  selectedIds,
  onToggle,
  onToggleAll,
  onOpen,
  onAction,
  emptyState,
}: {
  entries: FileEntry[]
  loading?: boolean
  selectedIds: Set<string>
  onToggle: (id: string) => void
  onToggleAll: () => void
  onOpen: (entry: FileEntry) => void
  onAction: (action: FileAction, entry: FileEntry) => void
  emptyState?: React.ReactNode
}) {
  const allSelected = entries.length > 0 && entries.every((e) => selectedIds.has(e.id))

  return (
    <div className="overflow-hidden rounded-xl border bg-card">
      <Table>
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            <TableHead className="w-10 pl-3">
              <input
                type="checkbox"
                className="size-4 cursor-pointer accent-primary"
                checked={allSelected}
                onChange={onToggleAll}
                aria-label="Select all"
              />
            </TableHead>
            <TableHead>Name</TableHead>
            <TableHead className="w-28">Size</TableHead>
            <TableHead className="w-40">Modified</TableHead>
            <TableHead className="w-20">Type</TableHead>
            <TableHead className="w-12" />
          </TableRow>
        </TableHeader>
        <TableBody>
          {loading ? (
            Array.from({ length: 5 }).map((_, i) => (
              <TableRow key={i}>
                <TableCell colSpan={6}>
                  <Skeleton className="h-5 w-full" />
                </TableCell>
              </TableRow>
            ))
          ) : entries.length === 0 ? (
            <TableRow>
              <TableCell colSpan={6} className="p-0">
                {emptyState}
              </TableCell>
            </TableRow>
          ) : (
            entries.map((entry) => {
              const Icon = entryIcon(entry)
              const selected = selectedIds.has(entry.id)
              return (
                <TableRow
                  key={entry.id}
                  data-state={selected ? 'selected' : undefined}
                  className={cn(entry.type === 'dir' && 'cursor-pointer')}
                  onClick={() => {
                    if (entry.type === 'dir') onOpen(entry)
                    else onToggle(entry.id)
                  }}
                >
                  <TableCell className="pl-3" onClick={(e) => e.stopPropagation()}>
                    <input
                      type="checkbox"
                      className="size-4 cursor-pointer accent-primary"
                      checked={selected}
                      onChange={() => onToggle(entry.id)}
                      aria-label={`Select ${entry.name}`}
                    />
                  </TableCell>
                  <TableCell>
                    <span className="flex min-w-0 items-center gap-2.5">
                      <Icon
                        className={cn(
                          'size-4 shrink-0',
                          entry.type === 'dir' ? 'text-primary' : 'text-muted-foreground',
                        )}
                      />
                      <span className="truncate text-[13px] font-medium">{entry.name}</span>
                    </span>
                  </TableCell>
                  <TableCell className="tnum text-xs text-muted-foreground">
                    {entry.type === 'dir' ? '—' : formatBytes(entry.sizeBytes)}
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {formatDateTime(entry.modifiedAt)}
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {typeLabel(entry)}
                  </TableCell>
                  <TableCell onClick={(e) => e.stopPropagation()}>
                    <DropdownMenu>
                      <DropdownMenuTrigger
                        className="rounded-md p-1 text-muted-foreground outline-none hover:bg-accent hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
                        aria-label={`Actions for ${entry.name}`}
                      >
                        <MoreHorizontal className="size-4" />
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        {entry.type === 'file' && (
                          <>
                            {isPreviewable(entry) && <DropdownMenuItem onSelect={() => onAction('preview', entry)}>Preview</DropdownMenuItem>}
                            <DropdownMenuItem onSelect={() => onAction('download', entry)}>Download</DropdownMenuItem>
                          </>
                        )}
                        <DropdownMenuItem onSelect={() => onAction('copy', entry)}>
                          Copy
                        </DropdownMenuItem>
                        <DropdownMenuItem onSelect={() => onAction('cut', entry)}>
                          Cut
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem onSelect={() => onAction('rename', entry)}>
                          Rename
                        </DropdownMenuItem>
                        <DropdownMenuItem onSelect={() => onAction('properties', entry)}>
                          Properties
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem
                          className="text-critical focus:text-critical"
                          onSelect={() => onAction('delete', entry)}
                        >
                          Delete
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </TableCell>
                </TableRow>
              )
            })
          )}
        </TableBody>
      </Table>
    </div>
  )
}
