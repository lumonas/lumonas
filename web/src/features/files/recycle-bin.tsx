import { RotateCcw, Trash2 } from 'lucide-react'
import { usePurgeRecycle, useRecycleBin, useRestoreFile } from '@/api/queries'
import { EmptyState } from '@/components/core/empty-state'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatBytes, timeAgo } from '@/lib/format'
import type { RecycleEntry } from '@/api/types'

export function RecycleBin({
  shareId,
  onBack,
}: {
  shareId: string
  onBack: () => void
}) {
  const { data: entries, isLoading, isError } = useRecycleBin(shareId)
  const restore = useRestoreFile()
  const purge = usePurgeRecycle()

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">
          Deleted items are kept for 30 days. A recycle bin is not a backup.
        </p>
        <Button
          size="sm"
          variant="destructiveOutline"
          disabled={!entries || entries.length === 0 || purge.isPending}
          onClick={() => purge.mutate({ shareId })}
        >
          <Trash2 />
          Empty recycle bin
        </Button>
      </div>

      <div className="overflow-hidden rounded-xl border bg-card">
        <Table>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead>Name</TableHead>
              <TableHead>Original location</TableHead>
              <TableHead className="w-32">Size</TableHead>
              <TableHead className="w-32">Deleted</TableHead>
              <TableHead className="w-40" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              <TableRow>
                <TableCell colSpan={5} className="p-4 text-center text-sm text-muted-foreground">
                  Loading…
                </TableCell>
              </TableRow>
            ) : isError ? (
              <TableRow>
                <TableCell colSpan={5} className="p-0">
                  <EmptyState
                    icon={<Trash2 />}
                    title="Recycle bin unavailable"
                    description="This share could not be opened, so its recycle bin is not accessible."
                    className="border-0"
                  />
                </TableCell>
              </TableRow>
            ) : !entries || entries.length === 0 ? (
              <TableRow>
                <TableCell colSpan={5} className="p-0">
                  <EmptyState
                    icon={<Trash2 />}
                    title="Recycle bin is empty"
                    description="Deleted files from this share will appear here."
                    className="border-0"
                  />
                </TableCell>
              </TableRow>
            ) : (
              entries.map((entry) => (
                <RecycleRow
                  key={entry.id}
                  entry={entry}
                  onRestore={() => restore.mutate({ id: entry.id })}
                  onPurge={() => purge.mutate({ id: entry.id, shareId })}
                />
              ))
            )}
          </TableBody>
        </Table>
      </div>

      <div>
        <Button variant="ghost" size="sm" onClick={onBack}>
          Back to files
        </Button>
      </div>
    </div>
  )
}

function RecycleRow({
  entry,
  onRestore,
  onPurge,
}: {
  entry: RecycleEntry
  onRestore: () => void
  onPurge: () => void
}) {
  return (
    <TableRow>
      <TableCell className="text-[13px] font-medium">{entry.name}</TableCell>
      <TableCell className="font-mono text-xs text-muted-foreground">{entry.originalPath}</TableCell>
      <TableCell className="tnum text-xs text-muted-foreground">
        {formatBytes(entry.sizeBytes)}
      </TableCell>
      <TableCell className="text-xs text-muted-foreground">{timeAgo(entry.deletedAt)}</TableCell>
      <TableCell>
        <div className="flex justify-end gap-1">
          <Button size="icon-sm" variant="ghost" aria-label={`Restore ${entry.name}`} onClick={onRestore}>
            <RotateCcw />
          </Button>
          <Button
            size="icon-sm"
            variant="ghost"
            aria-label={`Delete forever ${entry.name}`}
            className="text-critical hover:text-critical"
            onClick={onPurge}
          >
            <Trash2 />
          </Button>
        </div>
      </TableCell>
    </TableRow>
  )
}
